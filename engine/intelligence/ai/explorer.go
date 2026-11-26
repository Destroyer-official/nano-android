package ai

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"nano_android/engine/apk/ingest"
	"nano_android/engine/intelligence/graph"
	"nano_android/engine/mcp_compiler"
)

// BugFinding classifies an anomaly uncovered during bounded I3 exploration.
type BugFinding struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"` // "crash", "ui_loop", "unreachable_screen", "accessibility_gap", "input_boundary_anomaly"
	Severity        string `json:"severity"`
	ScreenKey       string `json:"screen_key"`
	DetailsFenced   string `json:"details_fenced"`
	ReproStepIndex  int    `json:"repro_step_index"`
}

// ExploreStep logs one reproducible action executed by the autonomous explorer.
type ExploreStep struct {
	StepIndex        int      `json:"step_index"`
	Action           string   `json:"action"` // "launch", "tap", "type", "step_time", "back"
	Target           string   `json:"target,omitempty"`
	Payload          string   `json:"payload,omitempty"`
	ScreenBefore     string   `json:"screen_before"`
	ScreenAfter      string   `json:"screen_after"`
	UIHash           string   `json:"ui_hash"`
	ScreenshotSHA256 string   `json:"screenshot_sha256,omitempty"`
	ScreenshotBytes  int      `json:"screenshot_bytes,omitempty"`
	VisibleFenced    []string `json:"visible_fenced,omitempty"`
}

// ExplorationReport captures the complete deterministic I3 UTG crawl + bug hunt result.
type ExplorationReport struct {
	PackageName          string         `json:"package_name"`
	ArtifactPath         string         `json:"artifact_path"`
	VisitedScreens       []string       `json:"visited_screens"`
	UnreachableScreens   []string       `json:"unreachable_screens,omitempty"`
	StepsExecuted        []ExploreStep  `json:"steps_executed"`
	Findings             []BugFinding   `json:"findings,omitempty"`
	ReproMaestroYAML     string         `json:"repro_maestro_yaml"`
	ReproProposal        *Proposal      `json:"repro_proposal,omitempty"`
	EnvironmentDisclosed string         `json:"environment_disclosed"`
	Budget               BudgetStatus   `json:"budget"`
}

var safeBoundaryFuzzInputs = []string{
	"",
	"0",
	"nano_boundary_123",
	"' OR '1'='1",
}

// ExploreAndHunt runs bounded I3 autonomous UTG crawling & safe boundary input fuzzing against coord.
func ExploreAndHunt(ctx context.Context, coord *mcp_compiler.IntegratedCoordinator, artifactPath, activity string, rep *ingest.IntelligenceReport, g *graph.BehaviorGraph, gov *Governor, store *ProposalStore) (*ExplorationReport, error) {
	if coord == nil {
		return nil, fmt.Errorf("coordinator is required")
	}
	if err := gov.CheckStep("nano_ai_explore", map[string]any{"artifact": artifactPath, "activity": activity}); err != nil {
		return nil, err
	}

	launchRes, err := coord.LaunchApp(ctx, artifactPath, activity, true)
	if err != nil {
		return nil, fmt.Errorf("launch for exploration failed: %w", err)
	}
	pkg := launchRes.AppID
	if pkg == "" && rep != nil && rep.Manifest != nil {
		pkg = rep.Manifest.Package
	}
	if g == nil {
		g = graph.NewBehaviorGraph(pkg)
	}

	out := &ExplorationReport{
		PackageName:          pkg,
		ArtifactPath:         artifactPath,
		EnvironmentDisclosed: fmt.Sprintf("backend=%s (bounded budget max_steps=%d max_screens=%d)", coord.GetExecutionMode(), gov.budget.MaxSteps, gov.budget.MaxScreens),
	}

	compiler := mcp_compiler.NewMaestroCompiler(pkg)
	visited := make(map[string]bool)
	clickedTargets := make(map[string]bool)
	fuzzedTargets := make(map[string]bool)
	var findings []BugFinding

	if launchRes.ExceptionTrace != "" {
		findings = append(findings, BugFinding{
			ID:            "bug_launch_crash",
			Kind:          "crash",
			Severity:      "CRITICAL",
			ScreenKey:     launchRes.Activity,
			DetailsFenced: gov.FenceUntrusted("launch_exception", launchRes.ExceptionTrace, 1024),
		})
	}

	observeState := func(prevScreen, actLabel string) (screenKey, uiHash, shotSHA string, shotSize int, ast *mcp_compiler.ASTNode, fenced []string, obsErr error) {
		obs, oErr := coord.ObserveScreen(ctx, true, "png", 0)
		if oErr != nil {
			return "", "", "", 0, nil, nil, oErr
		}
		screenKey = obs.CurrentScreen
		if screenKey == "" {
			screenKey = obs.ActiveActivity
		}
		if screenKey == "" {
			screenKey = launchRes.Activity
		}
		ast = obs.AST
		uiHash = hashAST(ast, obs.VisibleTexts)
		if obs.Screenshot != nil && obs.Screenshot.Base64Data != "" {
			rawB64 := strings.TrimPrefix(obs.Screenshot.Base64Data, "data:image/png;base64,")
			if decoded, decErr := base64.StdEncoding.DecodeString(rawB64); decErr == nil {
				sSum := sha256.Sum256(decoded)
				shotSHA = hex.EncodeToString(sSum[:8])
				shotSize = len(decoded)
			}
		}
		for i, t := range obs.VisibleTexts {
			fenced = append(fenced, gov.FenceUntrusted(fmt.Sprintf("visible_%d", i), t, 256))
		}
		if obs.ActiveDialog != "" {
			fenced = append(fenced, gov.FenceUntrusted("dialog", obs.ActiveDialog, 256))
		}
		g.EnrichFromRuntime(launchRes.Activity, prevScreen, screenKey, actLabel, ast, nil, nil)
		return screenKey, uiHash, shotSHA, shotSize, ast, fenced, nil
	}

	curScreen, curHash, curShotSHA, curShotSize, curAST, fencedTexts, err := observeState("", "launch")
	if err != nil {
		return nil, err
	}
	visited[curScreen] = true
	_, _ = gov.ObserveScreenState(curScreen, curHash)
	compiler.RecordLaunch(pkg, true)
	out.StepsExecuted = append(out.StepsExecuted, ExploreStep{
		StepIndex:        0,
		Action:           "launch",
		ScreenAfter:      curScreen,
		UIHash:           curHash,
		ScreenshotSHA256: curShotSHA,
		ScreenshotBytes:  curShotSize,
		VisibleFenced:    fencedTexts,
	})

	// Check initial accessibility labeling on visible clickable widgets.
	checkA11yGaps(curAST, curScreen, 0, &findings)

	stepIdx := 1
	fuzzIdx := 0
	for {
		if ctx.Err() != nil {
			break
		}
		clickables, editables := collectTargets(curAST)
		chosenAction := ""
		chosenTarget := ""
		chosenPayload := ""

		// Prefer fuzzing unvisited EditText boundary inputs first, then tapping unvisited clickable targets.
		for _, ed := range editables {
			if !fuzzedTargets[curScreen+"::"+ed] {
				fuzzedTargets[curScreen+"::"+ed] = true
				chosenAction = "type"
				chosenTarget = ed
				chosenPayload = safeBoundaryFuzzInputs[fuzzIdx%len(safeBoundaryFuzzInputs)]
				fuzzIdx++
				break
			}
		}
		if chosenAction == "" {
			for _, cl := range clickables {
				if !clickedTargets[curScreen+"::"+cl] {
					clickedTargets[curScreen+"::"+cl] = true
					chosenAction = "tap"
					chosenTarget = cl
					break
				}
			}
		}
		if chosenAction == "" {
			break
		}

		if err := gov.CheckStep("nano_ai_explore_step", map[string]any{
			"action": chosenAction,
			"target": chosenTarget,
		}); err != nil {
			break
		}

		var actRes *mcp_compiler.ActionResult
		var aErr error
		if chosenAction == "type" {
			actRes, aErr = coord.DispatchAction(ctx, mcp_compiler.Action{Type: "type", NodeID: chosenTarget, Text: chosenPayload})
			compiler.RecordType(chosenPayload, chosenTarget)
		} else {
			actRes, aErr = coord.DispatchAction(ctx, mcp_compiler.Action{Type: "tap", NodeID: chosenTarget})
			compiler.RecordTap(chosenTarget, chosenTarget, [2]float64{}, false)
		}
		_, _ = coord.StepTime(ctx, 300)

		if aErr != nil || (actRes != nil && actRes.Status != "success" && actRes.Message != "") {
			errTxt := ""
			if aErr != nil {
				errTxt = aErr.Error()
			} else if actRes != nil {
				errTxt = actRes.Message
			}
			findings = append(findings, BugFinding{
				ID:             fmt.Sprintf("bug_step_%d", stepIdx),
				Kind:           "input_boundary_anomaly",
				Severity:       "MEDIUM",
				ScreenKey:      curScreen,
				DetailsFenced:  gov.FenceUntrusted("action_error", errTxt, 512),
				ReproStepIndex: stepIdx,
			})
		}

		nextScreen, nextHash, nextShotSHA, nextShotSize, nextAST, nextFenced, obsErr := observeState(curScreen, chosenAction+":"+chosenTarget)
		if obsErr != nil {
			findings = append(findings, BugFinding{
				ID:             fmt.Sprintf("bug_crash_%d", stepIdx),
				Kind:           "crash",
				Severity:       "HIGH",
				ScreenKey:      curScreen,
				DetailsFenced:  gov.FenceUntrusted("observe_error", obsErr.Error(), 512),
				ReproStepIndex: stepIdx,
			})
			break
		}

		visited[nextScreen] = true
		loopHit, loopReason := gov.ObserveScreenState(nextScreen, nextHash)
		out.StepsExecuted = append(out.StepsExecuted, ExploreStep{
			StepIndex:        stepIdx,
			Action:           chosenAction,
			Target:           chosenTarget,
			Payload:          chosenPayload,
			ScreenBefore:     curScreen,
			ScreenAfter:      nextScreen,
			UIHash:           nextHash,
			ScreenshotSHA256: nextShotSHA,
			ScreenshotBytes:  nextShotSize,
			VisibleFenced:    nextFenced,
		})
		checkA11yGaps(nextAST, nextScreen, stepIdx, &findings)

		curScreen = nextScreen
		curHash = nextHash
		curAST = nextAST
		stepIdx++

		if loopHit {
			findings = append(findings, BugFinding{
				ID:             fmt.Sprintf("bug_loop_%d", stepIdx),
				Kind:           "ui_loop",
				Severity:       "LOW",
				ScreenKey:      curScreen,
				DetailsFenced:  loopReason,
				ReproStepIndex: stepIdx - 1,
			})
			break
		}
	}

	// Identify declared Activities in manifest that were never reached by the crawl
	if rep != nil && rep.Manifest != nil {
		for _, a := range rep.Manifest.Activities {
			reached := false
			for vs := range visited {
				if strings.EqualFold(vs, a.Name) || strings.HasSuffix(strings.ToLower(a.Name), "."+strings.ToLower(vs)) {
					reached = true
					break
				}
			}
			if !reached {
				out.UnreachableScreens = append(out.UnreachableScreens, a.Name)
			}
		}
	}

	for vs := range visited {
		out.VisitedScreens = append(out.VisitedScreens, vs)
	}
	out.Findings = findings
	opts := mcp_compiler.DefaultCompileOptions()
	opts.AppID = pkg
	opts.FlowName = "ai_explore_repro"
	opts.Optimize = true
	compiledYAML, _ := compiler.Compile(opts)
	out.ReproMaestroYAML = compiledYAML

	if store != nil {
		prop, pErr := store.Add(Proposal{
			Kind:            ProposalExploreRepro,
			Title:           fmt.Sprintf("AI Exploration Repro (%d steps, %d findings)", len(out.StepsExecuted), len(out.Findings)),
			Description:     "Minimal reproducible Maestro YAML captured by bounded I3 autonomous UTG explorer.",
			TargetRelPath:   "ai_explore_repro.yaml",
			Content:         out.ReproMaestroYAML,
			EnvironmentTier: coord.GetExecutionMode(),
			CreatedBy:       "ai_bug_hunter",
			Metadata: map[string]any{
				"visited_screens":     len(out.VisitedScreens),
				"unreachable_screens": len(out.UnreachableScreens),
				"findings_count":      len(out.Findings),
			},
		})
		if pErr == nil {
			out.ReproProposal = prop
		}
	}

	out.Budget = gov.Status()
	return out, nil
}

// MultiAgentCoordinate runs a bounded multi-agent engineering workflow (Architect + Explorer + TestAuthor)
// under a single shared Governor budget and ProposalStore.
func MultiAgentCoordinate(ctx context.Context, coord *mcp_compiler.IntegratedCoordinator, artifactPath string, rep *ingest.IntelligenceReport, sourceRoot string, gov *Governor, store *ProposalStore, mem *MemoryStore) (map[string]any, error) {
	if rep == nil {
		return nil, fmt.Errorf("intelligence report is required")
	}
	g := graph.BuildFromArtifact(rep, sourceRoot)
	understanding, err := UnderstandAPK(rep, g, gov)
	if err != nil {
		return nil, err
	}
	var exploreRep *ExplorationReport
	if coord != nil {
		exploreRep, _ = ExploreAndHunt(ctx, coord, artifactPath, "", rep, g, gov, store)
	}
	var liveAST *mcp_compiler.ASTNode
	if coord != nil {
		if obs, oErr := coord.ObserveScreen(ctx, false, "none", 0); oErr == nil {
			liveAST = obs.AST
		}
	}
	tests, tErr := GenerateTests(rep, g, liveAST, gov, store)
	if tErr != nil {
		return nil, tErr
	}
	if mem != nil {
		_ = mem.Put("last_coordinated_pkg", "screen_invariant", understanding.Summary, gov.actor)
	}
	return map[string]any{
		"understanding": understanding,
		"exploration":   exploreRep,
		"test_proposals": tests,
		"budget":        gov.Status(),
	}, nil
}

func collectTargets(root *mcp_compiler.ASTNode) (clickables, editables []string) {
	walkInteractive(root, &clickables, &editables)
	return clickables, editables
}

func checkA11yGaps(root *mcp_compiler.ASTNode, screen string, step int, findings *[]BugFinding) {
	if root == nil {
		return
	}
	var walk func(n *mcp_compiler.ASTNode)
	walk = func(n *mcp_compiler.ASTNode) {
		if n == nil {
			return
		}
		if n.Visible && n.Clickable && strings.TrimSpace(n.Text) == "" && strings.TrimSpace(n.ContentDescription) == "" {
			*findings = append(*findings, BugFinding{
				ID:             fmt.Sprintf("a11y_%s_%d_%s", n.ID, step, n.Class),
				Kind:           "accessibility_gap",
				Severity:       "LOW",
				ScreenKey:      screen,
				DetailsFenced:  fmt.Sprintf("Clickable %s (%s) has no visible text or contentDescription", n.Class, n.ID),
				ReproStepIndex: step,
			})
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(root)
}

func hashAST(ast *mcp_compiler.ASTNode, visible []string) string {
	raw, _ := json.Marshal(struct {
		AST *mcp_compiler.ASTNode `json:"a"`
		Vis []string              `json:"v"`
	}{AST: ast, Vis: visible})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}
