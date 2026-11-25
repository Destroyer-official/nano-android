package profiler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// TestProfilerToolHandlers tests MCP tool handlers for all 5 profiler tools.
func TestProfilerToolHandlers(t *testing.T) {
	ctx := context.Background()
	dm := newMockDeviceManager()
	sidecar := &mockProfilerSidecar{}

	handlers := RegisterProfilerTools(dm, sidecar)
	if len(handlers) != 5 {
		t.Fatalf("expected 5 handlers, got %d", len(handlers))
	}

	handlerMap := make(map[string]types.ToolHandler)
	for _, h := range handlers {
		def := h.Definition()
		handlerMap[def.Name] = h
	}

	// 1. perf_capture_trace
	hPerf, ok := handlerMap["perf_capture_trace"]
	if !ok {
		t.Fatal("perf_capture_trace handler not found")
	}
	if hPerf.Definition().RiskTier != types.RiskWRITESCOPED {
		t.Errorf("perf_capture_trace expected RiskWRITESCOPED, got %v", hPerf.Definition().RiskTier)
	}

	// 2. power_thermal_snapshot
	hPower, ok := handlerMap["power_thermal_snapshot"]
	if !ok {
		t.Fatal("power_thermal_snapshot handler not found")
	}
	if hPower.Definition().RiskTier != types.RiskREAD {
		t.Errorf("power_thermal_snapshot expected RiskREAD, got %v", hPower.Definition().RiskTier)
	}

	// 3. logcat_read
	hLogcat, ok := handlerMap["logcat_read"]
	if !ok {
		t.Fatal("logcat_read handler not found")
	}
	if hLogcat.Definition().RiskTier != types.RiskREAD {
		t.Errorf("logcat_read expected RiskREAD, got %v", hLogcat.Definition().RiskTier)
	}

	// 4. crash_report_collect
	hCrash, ok := handlerMap["crash_report_collect"]
	if !ok {
		t.Fatal("crash_report_collect handler not found")
	}
	if hCrash.Definition().RiskTier != types.RiskREAD {
		t.Errorf("crash_report_collect expected RiskREAD, got %v", hCrash.Definition().RiskTier)
	}

	// 5. app_state_inspect
	hAppState, ok := handlerMap["app_state_inspect"]
	if !ok {
		t.Fatal("app_state_inspect handler not found")
	}
	if hAppState.Definition().RiskTier != types.RiskREAD {
		t.Errorf("app_state_inspect expected RiskREAD, got %v", hAppState.Definition().RiskTier)
	}

	// Execute logcat_read handler
	dm.execADBOutputs["logcat -d -v threadtime"] = "11-20 20:00:00.123 1000 1000 I AppTag: Test message"
	resLogcat, err := hLogcat.Execute(ctx, map[string]any{
		"serial": "emulator-5554",
	})
	if err != nil || !resLogcat.Success {
		t.Fatalf("logcat_read failed: %v (%s)", err, resLogcat.Error)
	}
	if nonce, ok := resLogcat.Data["nonce"].(string); !ok || nonce == "" {
		t.Error("expected non-empty nonce in logcat_read")
	}

	// Execute power_thermal_snapshot handler
	dm.execADBOutputs["shell dumpsys thermalservice"] = "mThermalStatus=0\nTemperature{mValue=32.0, mType=3, mName=skin}\n"
	dm.execADBOutputs["shell dumpsys batterystats"] = "level: 90\ntemperature: 300\nCharge counter: 3000000 µAh\n"
	resPower, err := hPower.Execute(ctx, map[string]any{
		"serial": "emulator-5554",
	})
	if err != nil || !resPower.Success {
		t.Fatalf("power_thermal_snapshot failed: %v (%s)", err, resPower.Error)
	}
	if status, _ := resPower.Data["thermal_status"].(string); status != "NONE" {
		t.Errorf("expected thermal_status NONE, got %s", status)
	}

	// Execute app_state_inspect handler
	dm.execADBOutputs["shell am get-standby-bucket com.example.app"] = "10"
	resAppState, err := hAppState.Execute(ctx, map[string]any{
		"serial":     "emulator-5554",
		"package_id": "com.example.app",
		"sections":   []any{"standby_bucket"},
	})
	if err != nil || !resAppState.Success {
		t.Fatalf("app_state_inspect failed: %v (%s)", err, resAppState.Error)
	}
	if bucket, _ := resAppState.Data["standby_bucket"].(string); bucket != "ACTIVE" {
		t.Errorf("expected ACTIVE bucket, got %s", bucket)
	}
}

// TestPythonSidecarTraceAnalysisEndToEnd tests real Python sidecar trace analysis from Go.
func TestPythonSidecarTraceAnalysisEndToEnd(t *testing.T) {
	ctx := context.Background()
	sidecarDir := filepath.Join("..", "..", "sidecar-python")
	runner := NewPythonSidecarRunner(sidecarDir)

	tmpDir := t.TempDir()
	traceFile := filepath.Join(tmpDir, "test.perfetto-trace")
	traceData := `
# tracer: nop
   SurfaceFlinger-1000  [001] 1000.100: Choreographer#doFrame: dur=15.0
   SurfaceFlinger-1000  [001] 1000.116: Choreographer#doFrame: dur=16.2
   SurfaceFlinger-1000  [001] 1000.133: Choreographer#doFrame: dur=19.5
   com.example.app-2000 [002] 1000.140: mem.rss: size=134217728
   com.example.app-2000 [002] 1000.145: mem.swap: size=16777216
   lmkd-500             [000] 1000.150: MemoryLimiter:AnonSwap killed pid 2000
`
	if err := os.WriteFile(traceFile, []byte(traceData), 0644); err != nil {
		t.Fatalf("failed to write trace file: %v", err)
	}

	res, err := runner.AnalyzeTrace(ctx, traceFile, "com.example.app", "")
	if err != nil {
		t.Fatalf("AnalyzeTrace failed: %v", err)
	}

	if res.P95FrameMS <= 0 {
		t.Errorf("expected P95FrameMS > 0, got %v", res.P95FrameMS)
	}
	if res.PeakRSSMB <= 0 {
		t.Errorf("expected PeakRSSMB > 0, got %v", res.PeakRSSMB)
	}
	if res.MemoryLimiterEvents != 1 {
		t.Errorf("expected 1 MemoryLimiter event, got %d", res.MemoryLimiterEvents)
	}
}
