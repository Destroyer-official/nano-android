package compiler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
	"gopkg.in/yaml.v3"
)

func TestCompiler_Integration_RecordFreezeRun(t *testing.T) {
	tempDir := t.TempDir()
	flowPath := filepath.Join(tempDir, "flows", "login_flow.yaml")

	executedRuns := 0
	mockExec := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		executedRuns++
		return []byte("Flow execution succeeded"), nil
	}

	runner := NewMaestroRunner(mockExec)
	compiler := NewCompiler(runner)
	compiler.SetDefaultAppID("com.example.loginapp")

	sessionID := "sess_integration_42"

	// 1. Record 5 manual steps
	step1 := types.SessionStep{
		Action:       "tap_node",
		Target:       "btn_login",
		UIHashBefore: "hash_init",
		UIHashAfter:  "hash_form",
		Success:      true,
		NodesAfter: []types.PrunedNode{
			{ID: "n1", ResID: "com.example.loginapp:id/btn_login", Class: "Button"},
		},
	}
	step2 := types.SessionStep{
		Action:       "type",
		Target:       "input_user",
		Text:         "alice",
		UIHashBefore: "hash_form",
		UIHashAfter:  "hash_form_user",
		Success:      true,
		NodesAfter: []types.PrunedNode{
			{ID: "n2", ResID: "com.example.loginapp:id/input_user", Class: "EditText"},
		},
	}
	step3 := types.SessionStep{
		Action:       "type",
		Target:       "input_pass",
		Text:         "topsecret",
		UIHashBefore: "hash_form_user",
		UIHashAfter:  "hash_form_pass",
		Success:      true,
		NodesAfter: []types.PrunedNode{
			{ID: "n3", ResID: "com.example.loginapp:id/input_pass", Class: "EditText"},
		},
	}
	step4 := types.SessionStep{
		Action:       "tap_node",
		Target:       "btn_submit",
		UIHashBefore: "hash_form_pass",
		UIHashAfter:  "hash_dashboard",
		Success:      true,
		NodesAfter: []types.PrunedNode{
			{ID: "n4", ResID: "com.example.loginapp:id/btn_submit", Class: "Button"},
		},
	}
	step5 := types.SessionStep{
		Action:       "assert",
		Target:       "welcome_banner",
		UIHashBefore: "hash_dashboard",
		UIHashAfter:  "hash_dashboard",
		Success:      true,
		NodesAfter: []types.PrunedNode{
			{ID: "n5", ResID: "com.example.loginapp:id/welcome_banner", Class: "TextView", Text: "Welcome Alice"},
		},
	}

	compiler.RecordStep(sessionID, step1)
	compiler.RecordStep(sessionID, step2)
	compiler.RecordStep(sessionID, step3)
	compiler.RecordStep(sessionID, step4)
	compiler.RecordStep(sessionID, step5)

	// 2. Freeze session to Maestro
	freezeRes, err := compiler.FreezeToMaestro(sessionID, "LoginFlow", flowPath)
	if err != nil {
		t.Fatalf("FreezeToMaestro failed: %v", err)
	}

	if freezeRes.StepCount != 5 {
		t.Errorf("expected 5 frozen steps, got %d", freezeRes.StepCount)
	}
	if freezeRes.YAMLPath != flowPath {
		t.Errorf("expected YAML path %s, got %s", flowPath, freezeRes.YAMLPath)
	}

	// Read and validate emitted YAML
	yamlData, err := os.ReadFile(flowPath)
	if err != nil {
		t.Fatalf("failed to read emitted flow YAML: %v", err)
	}

	var parsedNode yaml.Node
	if err := yaml.Unmarshal(yamlData, &parsedNode); err != nil {
		t.Fatalf("emitted flow YAML is invalid syntax: %v", err)
	}

	// 3. Run frozen flow with burn-in (3x) on clean snapshot
	runRes, err := compiler.RunMaestro("emulator-5554", flowPath, 3)
	if err != nil {
		t.Fatalf("RunMaestro failed: %v", err)
	}

	if runRes.RunsPassed != 3 || runRes.RunsFailed != 0 {
		t.Errorf("expected 3 passed runs, got %d passed, %d failed", runRes.RunsPassed, runRes.RunsFailed)
	}
	if executedRuns != 3 {
		t.Errorf("expected runner to be called 3 times, got %d", executedRuns)
	}

	// 4. Test MCP tool handlers
	freezeTool := NewFreezeToMaestroTool(compiler)
	runTool := NewRunMaestroTool(compiler)
	appcardTool := NewExportAppCardTool(compiler)

	// Test Freeze tool
	outToolPath := filepath.Join(tempDir, "flows", "tool_flow.yaml")
	toolFreezeRes, err := freezeTool.Execute(context.Background(), map[string]any{
		"session_id":  sessionID,
		"flow_name":   "ToolFlow",
		"output_path": outToolPath,
	})
	if err != nil || !toolFreezeRes.Success {
		t.Fatalf("FreezeToMaestroTool failed: %v, error=%s", err, toolFreezeRes.Error)
	}

	// Test Run tool
	toolRunRes, err := runTool.Execute(context.Background(), map[string]any{
		"flow_path":    outToolPath,
		"serial":       "emulator-5554",
		"repeat_count": 3,
	})
	if err != nil || !toolRunRes.Success {
		t.Fatalf("RunMaestroTool failed: %v, error=%s", err, toolRunRes.Error)
	}

	// Test AppCard tool
	outAppCardPath := filepath.Join(tempDir, "appcard.md")
	toolAppcardRes, err := appcardTool.Execute(context.Background(), map[string]any{
		"session_id":  sessionID,
		"output_path": outAppCardPath,
	})
	if err != nil || !toolAppcardRes.Success {
		t.Fatalf("ExportAppCardTool failed: %v, error=%s", err, toolAppcardRes.Error)
	}

	if _, err := os.Stat(outAppCardPath); os.IsNotExist(err) {
		t.Errorf("AppCard markdown not created at %s", outAppCardPath)
	}
}
