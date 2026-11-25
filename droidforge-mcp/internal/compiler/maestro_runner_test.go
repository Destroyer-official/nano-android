package compiler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMaestroRunner_BurnInPass(t *testing.T) {
	tempDir := t.TempDir()
	flowPath := filepath.Join(tempDir, "flow.yaml")
	_ = os.WriteFile(flowPath, []byte("appId: com.example\n---\n- launchApp\n"), 0o644)

	execCount := 0
	mockExec := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		execCount++
		return []byte("Test passed!"), nil
	}

	runner := NewMaestroRunner(mockExec)
	runner.SetRunDir(filepath.Join(tempDir, "runs"))

	res, err := runner.Run(context.Background(), "emulator-5554", flowPath, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if execCount != 3 {
		t.Errorf("expected 3 executions for repeat_count:3, got %d", execCount)
	}
	if res.RunsPassed != 3 || res.RunsFailed != 0 {
		t.Errorf("expected 3 passed, 0 failed; got %d passed, %d failed", res.RunsPassed, res.RunsFailed)
	}
	if len(res.FailureBundlePaths) != 0 {
		t.Errorf("expected 0 failure bundles, got %d", len(res.FailureBundlePaths))
	}
}

func TestMaestroRunner_FailureBundleCreation(t *testing.T) {
	tempDir := t.TempDir()
	flowPath := filepath.Join(tempDir, "fail_flow.yaml")
	_ = os.WriteFile(flowPath, []byte("appId: com.example\n---\n- tapOn: missing_node\n"), 0o644)

	mockExec := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("Error: element not found: missing_node\nFATAL EXCEPTION in main thread"), errors.New("exit status 1")
	}

	runner := NewMaestroRunner(mockExec)
	runner.SetRunDir(filepath.Join(tempDir, "runs"))

	res, err := runner.Run(context.Background(), "emulator-5554", flowPath, 1)
	if err != nil {
		t.Fatalf("unexpected runner error: %v", err)
	}

	if res.RunsPassed != 0 || res.RunsFailed != 1 {
		t.Errorf("expected 0 passed, 1 failed; got %d passed, %d failed", res.RunsPassed, res.RunsFailed)
	}
	if len(res.FailureBundlePaths) != 1 {
		t.Fatalf("expected 1 failure bundle path, got %d", len(res.FailureBundlePaths))
	}

	bundlePath := res.FailureBundlePaths[0]
	// Check failure bundle artifacts
	summaryFile := filepath.Join(bundlePath, "summary.json")
	logcatFile := filepath.Join(bundlePath, "logcat_target_pid.txt")
	screenFile := filepath.Join(bundlePath, "final_screen.png")

	if _, err := os.Stat(summaryFile); os.IsNotExist(err) {
		t.Errorf("missing summary.json in failure bundle %s", bundlePath)
	}
	if _, err := os.Stat(logcatFile); os.IsNotExist(err) {
		t.Errorf("missing logcat_target_pid.txt in failure bundle %s", bundlePath)
	}
	if _, err := os.Stat(screenFile); os.IsNotExist(err) {
		t.Errorf("missing final_screen.png in failure bundle %s", bundlePath)
	}
}
