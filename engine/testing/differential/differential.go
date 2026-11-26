package differential

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"nano_android/engine/compatibility/intel"
	"nano_android/engine/compatibility/registry"
	"nano_android/engine/mcp_compiler"
	"nano_android/engine/testing/suite"
	"nano_android/engine/ui/profile"
)

// ComparisonMode enumerates supported differential testing matrix pairs.
type ComparisonMode string

const (
	ModeNanoVsART          ComparisonMode = "nano_jvm_vs_android_art"
	ModeNanoVsRobolectric  ComparisonMode = "nano_jvm_vs_robolectric"
	ModeEmulatorVsPhysical ComparisonMode = "emulator_vs_physical"
	ModeAPIXVsAPIY         ComparisonMode = "api_x_vs_api_y"
	ModeDeviceAVsDeviceB   ComparisonMode = "device_a_vs_device_b"
	ModeAppV1VsV2          ComparisonMode = "app_v1_vs_v2"
)

// All 11 behavioral comparison dimensions.
const (
	DimLifecycle      = "activity_lifecycle"
	DimNavStack       = "navigation_stack"
	DimUIHierarchy    = "ui_hierarchy"
	DimVisibleTexts   = "visible_texts"
	DimWidgetProps    = "widget_properties"
	DimScreenshots    = "screenshots"
	DimSQLiteState    = "sqlite_state"
	DimPreferences    = "preferences_datastore"
	DimNetworkReqs    = "network_requests"
	DimExceptions     = "exceptions"
	DimCallbacks      = "callback_ordering"
)

// ExecutionStepSnapshot captures all 11 observable dimensions after executing one step on a backend.
type ExecutionStepSnapshot struct {
	StepIndex        int               `json:"step_index"`
	StepLabel        string            `json:"step_label"`
	Backend          registry.Backend  `json:"backend"`
	APILevel         int               `json:"api_level"`
	DeviceProfile    string            `json:"device_profile"`
	LifecycleEvents  []string          `json:"lifecycle_events,omitempty"`
	NavStack         []string          `json:"nav_stack,omitempty"`
	UIHierarchy      []string          `json:"ui_hierarchy,omitempty"`
	VisibleTexts     []string          `json:"visible_texts,omitempty"`
	WidgetProperties map[string]string `json:"widget_properties,omitempty"`
	ScreenshotSHA256 string            `json:"screenshot_sha256,omitempty"`
	SQLiteTables     map[string]string `json:"sqlite_tables,omitempty"`
	Preferences      map[string]string `json:"preferences,omitempty"`
	NetworkRequests  []string          `json:"network_requests,omitempty"`
	Exceptions       []string          `json:"exceptions,omitempty"`
	Callbacks        []string          `json:"callbacks,omitempty"`
}

// DimensionDiff records a specific divergence on one of the 11 dimensions.
type DimensionDiff struct {
	StepIndex    int    `json:"step_index"`
	StepLabel    string `json:"step_label"`
	Dimension    string `json:"dimension"`
	Expected     string `json:"expected_a"`
	Actual       string `json:"actual_b"`
	SuspectedAPI string `json:"suspected_api"`
	Details      string `json:"details"`
}

// DivergenceFinding identifies the earliest observable divergence and responsible API/backend.
type DivergenceFinding struct {
	FirstStepIndex int             `json:"first_step_index"`
	StepLabel      string          `json:"step_label"`
	Dimension      string          `json:"dimension"`
	SuspectedAPI   string          `json:"suspected_api"`
	DiffsAtStep    []DimensionDiff `json:"diffs_at_step"`
	ReproConfirmed bool            `json:"repro_confirmed"`
	Summary        string          `json:"summary"`
}

// TargetSpec defines one side (A or B) of a differential comparison run.
type TargetSpec struct {
	Label     string           `json:"label"`
	Backend   registry.Backend `json:"backend"`
	Artifact  string           `json:"artifact"`
	Activity  string           `json:"activity,omitempty"`
	ProfileID string           `json:"profile_id,omitempty"`
	APILevel  int              `json:"api_level,omitempty"`
}

// DifferentialRequest configures a live cross-backend, cross-profile, or cross-version differential comparison.
type DifferentialRequest struct {
	Name              string         `json:"name"`
	Mode              ComparisonMode `json:"mode"`
	TargetA           TargetSpec     `json:"target_a"`
	TargetB           TargetSpec     `json:"target_b"`
	Steps             []suite.Step   `json:"steps"`
	CaptureScreenshot bool           `json:"capture_screenshot,omitempty"`
	AutoReproOnDiff   bool           `json:"auto_repro_on_diff,omitempty"`
	CompatDBPath      string         `json:"compat_db_path,omitempty"`
}

// EquivalenceReport is the canonical output of the Differential Testing Engine.
type EquivalenceReport struct {
	Name               string               `json:"name"`
	Mode               ComparisonMode       `json:"mode"`
	Status             registry.ResultStatus `json:"status"`
	Equivalent         bool                 `json:"equivalent"`
	EquivalenceScore   float64              `json:"equivalence_score"` // 0.0..1.0 across all evaluated dimension checks
	StepsEvaluated     int                  `json:"steps_evaluated"`
	DimensionsChecked  int                  `json:"dimensions_checked"`
	DivergingCount     int                  `json:"diverging_count"`
	FirstDivergence    *DivergenceFinding   `json:"first_divergence,omitempty"`
	AllDiffs           []DimensionDiff      `json:"all_diffs,omitempty"`
	TargetA            TargetSpec           `json:"target_a"`
	TargetB            TargetSpec           `json:"target_b"`
	EnvironmentNote    string               `json:"environment_note"`
}

// BackendRunner abstracts step-by-step live execution producing 11-dimension snapshots.
type BackendRunner interface {
	ExecuteStepByStep(ctx context.Context, spec TargetSpec, steps []suite.Step, captureScreenshot bool) ([]ExecutionStepSnapshot, error)
}

// NanoJVMRunner executes a TargetSpec cleanly against an isolated HeadlessAppHost JVM worker.
type NanoJVMRunner struct {
	TempDir string
}

// ExecuteStepByStep launches TargetSpec in Nano JVM and snapshots all observable dimensions at boot and each step.
func (r *NanoJVMRunner) ExecuteStepByStep(ctx context.Context, spec TargetSpec, steps []suite.Step, captureScreenshot bool) ([]ExecutionStepSnapshot, error) {
	if spec.Backend != "" && spec.Backend != registry.BackendNanoJVM {
		return nil, fmt.Errorf("UNSUPPORTED_BACKEND: %s requires live hardware/emulator/Robolectric attachment (cannot simulate via NanoJVMRunner)", spec.Backend)
	}
	bridge := mcp_compiler.NewLiveHeadlessBridge()
	defer bridge.Close()
	if isolatedSandbox, sbErr := os.MkdirTemp("", "nano_diff_sandbox_*"); sbErr == nil {
		bridge.SandboxRootOverride = isolatedSandbox
		defer func() { _ = os.RemoveAll(isolatedSandbox) }()
	}

	launchRes, err := bridge.Launch(spec.Artifact, spec.Activity)
	if err != nil {
		return nil, err
	}
	if spec.ProfileID != "" {
		if p, pErr := profile.Get(spec.ProfileID); pErr == nil && p.WidthPx > 0 && p.HeightPx > 0 {
			_ = bridge.SetProfile(p.WidthPx, p.HeightPx)
		}
	}

	var snapshots []ExecutionStepSnapshot
	capture := func(stepIdx int, label string) (ExecutionStepSnapshot, error) {
		snap := ExecutionStepSnapshot{
			StepIndex:        stepIdx,
			StepLabel:        label,
			Backend:          registry.BackendNanoJVM,
			APILevel:         spec.APILevel,
			DeviceProfile:    spec.ProfileID,
			WidgetProperties: map[string]string{},
			SQLiteTables:     map[string]string{},
			Preferences:      map[string]string{},
		}
		det, oErr := bridge.ObserveDetailed()
		if oErr == nil && det != nil {
			if len(det.ActivityStack) > 0 {
				snap.NavStack = append([]string(nil), det.ActivityStack...)
			} else if det.Activity != "" {
				snap.NavStack = []string{det.Activity}
			}
			flattenAST(det.Screen, &snap.UIHierarchy, &snap.VisibleTexts, snap.WidgetProperties)
		}
		if launchRes != nil && launchRes.ExceptionTrace != "" && stepIdx == 0 {
			snap.Exceptions = append(snap.Exceptions, launchRes.ExceptionTrace)
		}
		if tl, tlErr := bridge.InspectTimeline(); tlErr == nil {
			for _, ev := range tl {
				switch ev.EventType {
				case "ACTIVITY_CREATED":
					snap.LifecycleEvents = append(snap.LifecycleEvents, ev.EventType+":"+ev.Source)
				case "CALLBACK_EXECUTED":
					snap.Callbacks = append(snap.Callbacks, ev.Source+":"+ev.Details)
				case "EXCEPTION_RAISED":
					snap.Exceptions = append(snap.Exceptions, ev.Source+":"+ev.Details)
				case "NETWORK_EVENT":
					snap.NetworkRequests = append(snap.NetworkRequests, ev.Source+":"+ev.Details)
				case "DB_MUTATION":
					snap.SQLiteTables[ev.Source] = ev.Details
				}
			}
		}
		if stor, sErr := bridge.InspectStorage(); sErr == nil && stor != nil {
			for k, v := range stor.SQLiteTables {
				snap.SQLiteTables[k] = v
			}
			for k, v := range stor.Preferences {
				snap.Preferences[k] = v
			}
		}
		if captureScreenshot {
			outDir := r.TempDir
			if outDir == "" {
				outDir = os.TempDir()
			}
			pngPath := filepath.Join(outDir, fmt.Sprintf("diff_%s_step%d.png", sanitizeFile(spec.Label), stepIdx))
			if rendered, rErr := bridge.RenderScreen(pngPath); rErr == nil {
				if data, err := os.ReadFile(rendered); err == nil {
					h := sha256.Sum256(data)
					snap.ScreenshotSHA256 = hex.EncodeToString(h[:])
				}
				_ = os.Remove(rendered)
			}
		}
		return snap, nil
	}

	s0, err := capture(0, "initial_launch")
	if err != nil {
		return nil, err
	}
	snapshots = append(snapshots, s0)

	for i, st := range steps {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		switch st.Action {
		case "tap", "type", "swipe", "scroll", "back", "long_click":
			extra := st.Text
			if extra == "" {
				extra = st.Direction
			}
			if _, aErr := bridge.DispatchAction(st.Action, st.Target, extra); aErr != nil {
				return nil, aErr
			}
		case "step_time":
			delta := st.DeltaMs
			if delta <= 0 {
				delta = 300
			}
			if _, sErr := bridge.StepTime(delta); sErr != nil {
				return nil, sErr
			}
		case "simulate_process_death":
			if pErr := bridge.SimulateProcessDeath(); pErr != nil {
				return nil, pErr
			}
		case "observe":
			// passive observation snapshot
		}
		snap, err := capture(i+1, fmt.Sprintf("%s(%s)", st.Action, st.Target))
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, nil
}

func flattenAST(n *mcp_compiler.ASTNode, out, texts *[]string, props map[string]string) {
	if n == nil {
		return
	}
	*out = append(*out, fmt.Sprintf("%s#%s@%v", n.Type, n.ResourceID, n.Bounds))
	if n.Visible && strings.TrimSpace(n.Text) != "" {
		*texts = append(*texts, n.Text)
	}
	props[n.ID] = fmt.Sprintf("clickable=%v,visible=%v,checkable=%v,desc=%s", n.Clickable, n.Visible, n.Checkable, n.ContentDescription)
	for _, c := range n.Children {
		flattenAST(c, out, texts, props)
	}
}

func sanitizeFile(s string) string {
	if s == "" {
		return "target"
	}
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, s)
}

// CompareSnapshots checks all 11 dimensions between snapshot A and snapshot B at one step index.
func CompareSnapshots(a, b ExecutionStepSnapshot) ([]DimensionDiff, int) {
	var diffs []DimensionDiff
	checked := 11

	addDiff := func(dim, api, exp, act, details string) {
		diffs = append(diffs, DimensionDiff{
			StepIndex:    a.StepIndex,
			StepLabel:    a.StepLabel,
			Dimension:    dim,
			Expected:     exp,
			Actual:       act,
			SuspectedAPI: api,
			Details:      details,
		})
	}

	if !reflect.DeepEqual(a.LifecycleEvents, b.LifecycleEvents) {
		addDiff(DimLifecycle, "android.app.Activity", strings.Join(a.LifecycleEvents, ","), strings.Join(b.LifecycleEvents, ","), "Lifecycle transition sequence diverged")
	}
	if !reflect.DeepEqual(a.NavStack, b.NavStack) {
		addDiff(DimNavStack, "android.app.Activity", strings.Join(a.NavStack, ","), strings.Join(b.NavStack, ","), "Active activity / back stack diverged")
	}
	if !reflect.DeepEqual(a.UIHierarchy, b.UIHierarchy) {
		addDiff(DimUIHierarchy, "android.view.View", fmt.Sprintf("%d nodes", len(a.UIHierarchy)), fmt.Sprintf("%d nodes", len(b.UIHierarchy)), "View hierarchy structure/bounds diverged")
	}
	if !reflect.DeepEqual(a.VisibleTexts, b.VisibleTexts) {
		addDiff(DimVisibleTexts, "android.widget.TextView", strings.Join(a.VisibleTexts, "|"), strings.Join(b.VisibleTexts, "|"), "Rendered visible texts diverged")
	}
	if !mapsEqual(a.WidgetProperties, b.WidgetProperties) {
		addDiff(DimWidgetProps, "android.view.View", mapSummary(a.WidgetProperties), mapSummary(b.WidgetProperties), "Widget properties (clickable/visible/checked/desc) diverged")
	}
	if a.ScreenshotSHA256 != "" && b.ScreenshotSHA256 != "" && a.ScreenshotSHA256 != b.ScreenshotSHA256 {
		addDiff(DimScreenshots, "android.graphics.Canvas", a.ScreenshotSHA256[:12], b.ScreenshotSHA256[:12], "Rendered framebuffer digest diverged")
	}
	if !mapsEqual(a.SQLiteTables, b.SQLiteTables) {
		addDiff(DimSQLiteState, "android.database.sqlite.SQLiteDatabase", mapSummary(a.SQLiteTables), mapSummary(b.SQLiteTables), "SQLite table state diverged")
	}
	if !mapsEqual(a.Preferences, b.Preferences) {
		addDiff(DimPreferences, "android.content.SharedPreferences", mapSummary(a.Preferences), mapSummary(b.Preferences), "Preferences/DataStore state diverged")
	}
	if !reflect.DeepEqual(a.NetworkRequests, b.NetworkRequests) {
		addDiff(DimNetworkReqs, "java.net.HttpURLConnection", strings.Join(a.NetworkRequests, ","), strings.Join(b.NetworkRequests, ","), "Network request timeline diverged")
	}
	if !reflect.DeepEqual(a.Exceptions, b.Exceptions) {
		addDiff(DimExceptions, "java.lang.Throwable", strings.Join(a.Exceptions, "|"), strings.Join(b.Exceptions, "|"), "Exception / stack trace behavior diverged")
	}
	if !reflect.DeepEqual(a.Callbacks, b.Callbacks) {
		addDiff(DimCallbacks, "android.os.Looper", strings.Join(a.Callbacks, ","), strings.Join(b.Callbacks, ","), "Callback execution order diverged")
	}

	return diffs, checked
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

func mapSummary(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
		if len(parts) >= 4 {
			parts = append(parts, "...")
			break
		}
	}
	return strings.Join(parts, "; ")
}

// Run executes a differential flow across runnerA and runnerB, identifies the earliest divergence, and optionally updates `compatibility.db`.
func Run(ctx context.Context, req DifferentialRequest, runnerA, runnerB BackendRunner) (*EquivalenceReport, error) {
	if runnerA == nil {
		runnerA = &NanoJVMRunner{}
	}
	if runnerB == nil {
		runnerB = &NanoJVMRunner{}
	}

	rep := &EquivalenceReport{
		Name:            req.Name,
		Mode:            req.Mode,
		TargetA:         req.TargetA,
		TargetB:         req.TargetB,
		EnvironmentNote: fmt.Sprintf("Differential run (%s): A=%s (API %d) vs B=%s (API %d)", req.Mode, req.TargetA.Backend, req.TargetA.APILevel, req.TargetB.Backend, req.TargetB.APILevel),
	}

	snapsA, errA := runnerA.ExecuteStepByStep(ctx, req.TargetA, req.Steps, req.CaptureScreenshot)
	if errA != nil {
		if strings.Contains(errA.Error(), "UNSUPPORTED_BACKEND") {
			rep.Status = registry.StatusUnsupported
			rep.EnvironmentNote = errA.Error()
			return rep, nil
		}
		rep.Status = registry.StatusInfrastructureErr
		return rep, errA
	}

	snapsB, errB := runnerB.ExecuteStepByStep(ctx, req.TargetB, req.Steps, req.CaptureScreenshot)
	if errB != nil {
		if strings.Contains(errB.Error(), "UNSUPPORTED_BACKEND") {
			rep.Status = registry.StatusUnsupported
			rep.EnvironmentNote = errB.Error()
			return rep, nil
		}
		rep.Status = registry.StatusInfrastructureErr
		return rep, errB
	}

	stepsN := len(snapsA)
	if len(snapsB) < stepsN {
		stepsN = len(snapsB)
	}
	rep.StepsEvaluated = stepsN

	totalChecks := 0
	for i := 0; i < stepsN; i++ {
		diffs, checked := CompareSnapshots(snapsA[i], snapsB[i])
		totalChecks += checked
		if len(diffs) > 0 {
			if rep.FirstDivergence == nil {
				rep.FirstDivergence = &DivergenceFinding{
					FirstStepIndex: i,
					StepLabel:      snapsA[i].StepLabel,
					Dimension:      diffs[0].Dimension,
					SuspectedAPI:   diffs[0].SuspectedAPI,
					DiffsAtStep:    diffs,
					Summary:        fmt.Sprintf("Earliest divergence at step %d (%s) on dimension %s: %s", i, snapsA[i].StepLabel, diffs[0].Dimension, diffs[0].Details),
				}
			}
			rep.AllDiffs = append(rep.AllDiffs, diffs...)
		}
	}

	rep.DimensionsChecked = totalChecks
	rep.DivergingCount = len(rep.AllDiffs)
	if totalChecks > 0 {
		rep.EquivalenceScore = float64(totalChecks-rep.DivergingCount) / float64(totalChecks)
		if rep.EquivalenceScore < 0 {
			rep.EquivalenceScore = 0
		}
	}
	rep.Equivalent = rep.DivergingCount == 0

	// Optional auto-repro confirmation for non-flaky deterministic divergence
	if !rep.Equivalent && req.AutoReproOnDiff && rep.FirstDivergence != nil {
		reSnapsA, rErrA := runnerA.ExecuteStepByStep(ctx, req.TargetA, req.Steps, req.CaptureScreenshot)
		reSnapsB, rErrB := runnerB.ExecuteStepByStep(ctx, req.TargetB, req.Steps, req.CaptureScreenshot)
		if rErrA == nil && rErrB == nil && len(reSnapsA) > rep.FirstDivergence.FirstStepIndex && len(reSnapsB) > rep.FirstDivergence.FirstStepIndex {
			reDiffs, _ := CompareSnapshots(reSnapsA[rep.FirstDivergence.FirstStepIndex], reSnapsB[rep.FirstDivergence.FirstStepIndex])
			for _, d := range reDiffs {
				if d.Dimension == rep.FirstDivergence.Dimension {
					rep.FirstDivergence.ReproConfirmed = true
					break
				}
			}
		}
	}

	// Continuous compatibility learning loop
	if strings.TrimSpace(req.CompatDBPath) != "" {
		if store, sErr := intel.OpenStore(req.CompatDBPath); sErr == nil {
			apiLvl := req.TargetB.APILevel
			if apiLvl <= 0 {
				apiLvl = 35
			}
			if rep.Equivalent {
				_ = store.RecordDifferentialEvidence(req.TargetA.Backend, req.TargetB.Backend, "android.app.Activity", apiLvl, true, "Equivalence verified")
			} else if rep.FirstDivergence != nil {
				_ = store.RecordDifferentialEvidence(req.TargetA.Backend, req.TargetB.Backend, rep.FirstDivergence.SuspectedAPI, apiLvl, false, rep.FirstDivergence.Summary)
			}
		}
	}

	if rep.Equivalent {
		rep.Status = registry.StatusNanoSimulationPass
		if req.TargetA.Backend == registry.BackendAndroidART || req.TargetB.Backend == registry.BackendAndroidART {
			rep.Status = registry.StatusArtEmulatorPass
		}
	} else {
		rep.Status = registry.StatusFail
	}
	return rep, nil
}
