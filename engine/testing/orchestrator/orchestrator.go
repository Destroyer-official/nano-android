package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"nano_android/engine/apk/ingest"
	"nano_android/engine/compatibility/registry"
	"nano_android/engine/mcp_compiler"
	"nano_android/engine/network/mock"
	"nano_android/engine/security/archive"
	"nano_android/engine/security/pathcontainment"
	"nano_android/engine/security/policy"
	"nano_android/engine/testing/suite"
	"nano_android/engine/ui/accessibility"
	"nano_android/engine/ui/profile"
)

// Orchestrator executes test suites through policy gates, adaptive routing, isolated Nano JVM workers, and flake classifiers.
type Orchestrator struct {
	AllowedRoots []string
	Guard        *policy.Guard
	CacheDir     string
}

// New constructs a default Orchestrator grounded to workspace cache/tools/fixtures roots.
func New(workspaceRoot string) *Orchestrator {
	return &Orchestrator{
		AllowedRoots: []string{
			filepath.Join(workspaceRoot, "cache"),
			filepath.Join(workspaceRoot, "tools"),
			filepath.Join(workspaceRoot, "fixtures"),
			filepath.Join(workspaceRoot, "downloads"),
		},
		Guard:    policy.NewGuard(policy.DefaultConfig()),
		CacheDir: filepath.Join(workspaceRoot, "cache"),
	}
}

// RunSuite executes an entire test suite with bounded parallel workers, mock network fixtures, retry policy, and flake classification.
func (o *Orchestrator) RunSuite(ctx context.Context, s suite.Suite) (*suite.SuiteReport, error) {
	startAll := time.Now()
	rep := &suite.SuiteReport{
		SuiteName: s.Name,
	}

	contained, err := pathcontainment.ResolveExistingContained(s.Artifact, o.AllowedRoots, []string{".apk", ".jar", ".aab", ".apks", ".xapk"})
	if err != nil {
		rep.OverallStatus = string(registry.StatusBlockedPolicy)
		rep.PrimaryBackend = string(registry.BackendStatic)
		return rep, fmt.Errorf("BLOCKED_POLICY: %w", err)
	}
	if _, valErr := archive.ValidateArchive(contained, archive.DefaultLimits()); valErr != nil {
		rep.OverallStatus = string(registry.StatusBlockedPolicy)
		rep.PrimaryBackend = string(registry.BackendStatic)
		return rep, fmt.Errorf("archive validation failed: %w", valErr)
	}

	requestedMode := strings.TrimSpace(s.Mode)
	if requestedMode == "" {
		requestedMode = "auto"
	}

	primaryBackend := registry.BackendNanoJVM
	var nonReproduced []string
	var intel *ingest.IntelligenceReport

	if strings.HasSuffix(strings.ToLower(contained), ".apk") ||
		strings.HasSuffix(strings.ToLower(contained), ".aab") ||
		strings.HasSuffix(strings.ToLower(contained), ".apks") ||
		strings.HasSuffix(strings.ToLower(contained), ".xapk") {
		eng := ingest.NewEngine(filepath.Join(o.CacheDir, "apk_intel"), "")
		if r, aErr := eng.Analyze(contained); aErr == nil && r != nil {
			intel = r
			rec := registry.EvaluateAPK(intel, requestedMode)
			primaryBackend = rec.PrimaryBackend
			nonReproduced = append(nonReproduced, rec.Limitations...)
		}
	} else if requestedMode != "auto" {
		primaryBackend = registry.Backend(requestedMode)
	}

	rep.PrimaryBackend = string(primaryBackend)

	if o.Guard != nil {
		ev := o.Guard.EvaluateTool(s.Name, "nano_test_run", map[string]any{"apk_path": contained})
		if ev.Decision != policy.DecisionAllow {
			rep.OverallStatus = string(registry.StatusBlockedPolicy)
			return rep, fmt.Errorf("BLOCKED_POLICY (%s): %s", ev.Decision, ev.Reason)
		}
	}

	// Refuse false-positive simulation if backend routes to ART/Physical/Robolectric outside local JVM execution.
	if primaryBackend != registry.BackendNanoJVM && primaryBackend != registry.BackendStatic {
		for _, tc := range s.Cases {
			rep.Outcomes = append(rep.Outcomes, suite.CaseOutcome{
				Name:                   tc.Name,
				Status:                 string(registry.StatusUnsupported),
				Iterations:             0,
				BackendUsed:            string(primaryBackend),
				EnvironmentDisclosure:  fmt.Sprintf("Artifact requires backend %q (explicitly routed instead of faking Nano JVM execution)", primaryBackend),
				NonReproducedBehaviors: nonReproduced,
				Error:                  fmt.Sprintf("Target requires %s backend which is not attached in offline Nano JVM worker", primaryBackend),
			})
			rep.Failed++
		}
		rep.OverallStatus = string(registry.StatusUnsupported)
		rep.DurationMs = time.Since(startAll).Milliseconds()
		return rep, nil
	}

	var netSrv *mock.Server
	if len(s.NetworkStubs) > 0 || s.Offline {
		srv, srvErr := mock.Start(42)
		if srvErr != nil {
			return rep, fmt.Errorf("start fixture mock server: %w", srvErr)
		}
		defer srv.Close()
		for _, stub := range s.NetworkStubs {
			srv.AddRoute(stub)
		}
		srv.SetOffline(s.Offline)
		netSrv = srv
	}

	workers := s.WorkerCount
	if workers <= 0 {
		workers = 1
	}
	if workers > 8 {
		workers = 8
	}

	outcomes := make([]suite.CaseOutcome, len(s.Cases))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for idx, tc := range s.Cases {
		wg.Add(1)
		go func(i int, c suite.TestCase) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			repeats := c.RepeatCount
			if repeats <= 0 {
				repeats = s.RepeatCount
			}
			if repeats <= 0 {
				repeats = 1
			}
			outcomes[i] = o.runCaseRepeated(ctx, contained, c, repeats, nonReproduced, netSrv)
		}(idx, tc)
	}
	wg.Wait()

	rep.Outcomes = outcomes
	for _, out := range outcomes {
		if out.Flaky {
			rep.Flaky++
			rep.Failed++
		} else if out.Status == string(registry.StatusNanoSimulationPass) {
			rep.Passed++
		} else {
			rep.Failed++
		}
	}

	if rep.Failed == 0 {
		rep.OverallStatus = string(registry.StatusNanoSimulationPass)
	} else if rep.Flaky > 0 {
		rep.OverallStatus = string(registry.StatusInconclusive)
	} else {
		rep.OverallStatus = string(registry.StatusFail)
	}
	rep.DurationMs = time.Since(startAll).Milliseconds()
	return rep, nil
}

func (o *Orchestrator) runCaseRepeated(ctx context.Context, artifactPath string, tc suite.TestCase, repeats int, baseNonRepro []string, netSrv *mock.Server) suite.CaseOutcome {
	t0 := time.Now()
	out := suite.CaseOutcome{
		Name:                   tc.Name,
		Iterations:             repeats,
		BackendUsed:            string(registry.BackendNanoJVM),
		EnvironmentDisclosure:  "Isolated OpenJDK 25 Nano-Android Micro-Runtime (deterministic virtual Looper & SQLite JDBC; no Bionic/ART native hardware)",
		ReproducedBehaviors:    []string{"JVM bytecode", "Activity/Application/Service/Provider lifecycle", "AXML view layout & AST", "Deterministic Looper clock"},
		NonReproducedBehaviors: append([]string(nil), baseNonRepro...),
	}
	if netSrv != nil {
		out.MockServerURL = netSrv.URL()
	}

	prof := profile.ReusableProfiles()["clean_install"]
	if tc.ProfileID != "" {
		if p, err := profile.Get(tc.ProfileID); err == nil {
			prof = p
		}
	}

	maxRetries := tc.RetryOnFailure
	if maxRetries < 0 {
		maxRetries = 0
	}

	var firstErr string
	for iter := 0; iter < repeats; iter++ {
		if err := ctx.Err(); err != nil {
			firstErr = err.Error()
			out.FailedCount++
			break
		}
		var runErr error
		for attempt := 0; attempt <= maxRetries; attempt++ {
			runErr = o.runSingleIteration(ctx, artifactPath, tc, prof, netSrv)
			if runErr == nil {
				break
			}
			if attempt < maxRetries {
				out.RetriesUsed++
			}
		}
		if runErr == nil {
			out.PassedCount++
		} else {
			out.FailedCount++
			if firstErr == "" {
				firstErr = runErr.Error()
			}
		}
	}

	out.DurationMs = time.Since(t0).Milliseconds()
	if out.FailedCount == 0 && out.PassedCount == repeats {
		out.Status = string(registry.StatusNanoSimulationPass)
	} else if out.PassedCount > 0 && out.FailedCount > 0 {
		out.Flaky = true
		out.Status = string(registry.StatusInconclusive)
		out.Error = fmt.Sprintf("FLAKY: passed %d/%d iterations; first failure: %s", out.PassedCount, repeats, firstErr)
	} else {
		out.Status = string(registry.StatusFail)
		out.Error = firstErr
	}
	return out
}

func (o *Orchestrator) runSingleIteration(ctx context.Context, artifactPath string, tc suite.TestCase, prof profile.DeviceProfile, netSrv *mock.Server) error {
	if _, err := os.Stat(artifactPath); err != nil {
		return err
	}
	bridge := mcp_compiler.NewLiveHeadlessBridge()
	defer bridge.Close()

	launchRes, err := bridge.Launch(artifactPath, tc.Activity)
	if err != nil {
		return err
	}
	if launchRes.Status == "error" || launchRes.ResultStatus == "UNSUPPORTED" {
		return fmt.Errorf("launch failure (%s): %s", launchRes.ResultStatus, launchRes.Error)
	}
	if prof.WidthPx > 0 && prof.HeightPx > 0 && (prof.WidthPx != 1080 || prof.HeightPx != 2400) {
		if pErr := bridge.SetProfile(prof.WidthPx, prof.HeightPx); pErr != nil {
			return pErr
		}
	}

	for _, step := range tc.Steps {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		switch step.Action {
		case "tap", "type", "swipe", "scroll", "back", "long_click":
			extra := step.Text
			if extra == "" {
				extra = step.Direction
			}
			res, aErr := bridge.DispatchAction(step.Action, step.Target, extra)
			if aErr != nil {
				return aErr
			}
			if res.Status != "success" {
				return fmt.Errorf("action %s(%q) failed: %s", step.Action, step.Target, res.Message)
			}
		case "step_time":
			delta := step.DeltaMs
			if delta <= 0 {
				delta = 300
			}
			if _, sErr := bridge.StepTime(delta); sErr != nil {
				return sErr
			}
		case "simulate_process_death":
			if err := bridge.SimulateProcessDeath(); err != nil {
				return err
			}
		case "observe":
			if _, oErr := bridge.Observe(); oErr != nil {
				return oErr
			}
		case "assert":
			if step.Assert == nil {
				continue
			}
			if step.Assert.Type == "http_captured" {
				if netSrv == nil {
					return fmt.Errorf("http_captured assertion requires network mock server")
				}
				found := false
				for _, ex := range netSrv.Captured() {
					if strings.Contains(ex.Path, step.Assert.Target) {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("assertion http_captured(%q) failed", step.Assert.Target)
				}
				continue
			}
			ast, oErr := bridge.Observe()
			if oErr != nil {
				return oErr
			}
			if err := evaluateAssertion(ast, step.Assert, prof); err != nil {
				return err
			}
		}
	}
	return nil
}

func evaluateAssertion(ast *mcp_compiler.ASTNode, a *suite.Assertion, prof profile.DeviceProfile) error {
	switch a.Type {
	case "visible":
		n := ast.FindByID(a.Target)
		if n == nil {
			n = ast.FindByText(a.Target)
		}
		if n == nil || !n.Visible {
			return fmt.Errorf("assertion visible(%q) failed", a.Target)
		}
	case "not_visible":
		n := ast.FindByID(a.Target)
		if n == nil {
			n = ast.FindByText(a.Target)
		}
		if n != nil && n.Visible {
			return fmt.Errorf("assertion not_visible(%q) failed", a.Target)
		}
	case "text_matches":
		n := ast.FindByID(a.Target)
		if n == nil {
			n = ast.FindByText(a.Target)
		}
		if n == nil {
			return fmt.Errorf("node %q not found", a.Target)
		}
		ok, err := regexp.MatchString(a.Expected, n.Text)
		if err != nil || !ok {
			return fmt.Errorf("text_matches %q failed on %q", a.Expected, n.Text)
		}
	case "a11y_passed":
		rep := accessibility.AuditTree(convertAST(ast), prof)
		if !rep.Passed {
			return fmt.Errorf("accessibility audit failed (%d issue(s))", len(rep.Issues))
		}
	default:
		return fmt.Errorf("unsupported assertion type %q", a.Type)
	}
	return nil
}

func convertAST(n *mcp_compiler.ASTNode) *accessibility.UINode {
	if n == nil {
		return nil
	}
	u := &accessibility.UINode{
		ID:                 n.ID,
		ResourceID:         n.ResourceID,
		Class:              n.Type,
		Text:               n.Text,
		ContentDescription: n.ContentDescription,
		Clickable:          n.Clickable,
		Checkable:          n.Checkable,
		Focusable:          n.Focused,
		Visible:            n.Visible,
		Bounds:             n.Bounds,
	}
	for _, c := range n.Children {
		u.Children = append(u.Children, convertAST(c))
	}
	return u
}
