package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// TestHarnessToolHandlers tests MCP tool handlers for all 4 harness tools.
func TestHarnessToolHandlers(t *testing.T) {
	ctx := context.Background()
	dm := newMockDeviceManager()
	sidecar := &mockSidecarRunner{}

	handlers := RegisterHarnessTools(dm, sidecar)
	if len(handlers) != 4 {
		t.Fatalf("expected 4 handlers, got %d", len(handlers))
	}

	handlerMap := make(map[string]types.ToolHandler)
	for _, h := range handlers {
		def := h.Definition()
		handlerMap[def.Name] = h
		if def.RiskTier != types.RiskWRITESCOPED {
			t.Errorf("tool %s expected RiskWRITESCOPED, got %v", def.Name, def.RiskTier)
		}
	}

	// 1. sim_inject_sms_or_call
	hSMS, ok := handlerMap["sim_inject_sms_or_call"]
	if !ok {
		t.Fatal("sim_inject_sms_or_call handler not found")
	}
	res, err := hSMS.Execute(ctx, map[string]any{
		"serial": "emulator-5554",
		"kind":   "sms",
		"sender": "+15550199",
		"body":   "Verification code: 123456",
	})
	if err != nil || !res.Success {
		t.Fatalf("sim_inject_sms_or_call failed: %v (%s)", err, res.Error)
	}
	if detected, _ := res.Data["android17_otp_pattern_detected"].(bool); !detected {
		t.Error("expected android17_otp_pattern_detected=true")
	}

	// 2. sim_post_notification
	hNotif, ok := handlerMap["sim_post_notification"]
	if !ok {
		t.Fatal("sim_post_notification handler not found")
	}
	resNotif, err := hNotif.Execute(ctx, map[string]any{
		"serial": "emulator-5554",
		"title":  "Alert",
		"body":   "Phishing intercepted",
	})
	if err != nil || !resNotif.Success {
		t.Fatalf("sim_post_notification failed: %v (%s)", err, resNotif.Error)
	}

	// 3. sim_set_permissions
	hPerm, ok := handlerMap["sim_set_permissions"]
	if !ok {
		t.Fatal("sim_set_permissions handler not found")
	}
	resPerm, err := hPerm.Execute(ctx, map[string]any{
		"serial":     "emulator-5554",
		"package_id": "com.example.app",
		"grants":     []any{"android.permission.RECEIVE_SMS"},
		"appops": []any{
			map[string]any{"op": "ACCESS_RESTRICTED_SETTINGS", "mode": "allow"},
		},
		"standby_bucket": "active",
		"doze_mode":      "force-idle",
	})
	if err != nil || !resPerm.Success {
		t.Fatalf("sim_set_permissions failed: %v (%s)", err, resPerm.Error)
	}

	// 4. media_push_and_verify
	hMedia, ok := handlerMap["media_push_and_verify"]
	if !ok {
		t.Fatal("media_push_and_verify handler not found")
	}
	validHash64 := strings.Repeat("a", 64)
	dm.execADBOutputs["shell sha256sum /sdcard/Movies/out.mp4"] = validHash64 + "  /sdcard/Movies/out.mp4"
	resMedia, err := hMedia.Execute(ctx, map[string]any{
		"serial":             "emulator-5554",
		"output_device_path": "/sdcard/Movies/out.mp4",
		"oracle":             "sha256",
		"golden_sha256":      validHash64,
	})
	if err != nil || !resMedia.Success {
		t.Fatalf("media_push_and_verify failed: %v (%s)", err, resMedia.Error)
	}
}

// TestPythonSidecarMediaVerificationEndToEnd tests real Python sidecar invocation from Go.
func TestPythonSidecarMediaVerificationEndToEnd(t *testing.T) {
	ctx := context.Background()
	sidecarDir := filepath.Join("..", "..", "sidecar-python")
	runner := NewPythonSidecarRunner(sidecarDir)

	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "src.bin")
	dstFile := filepath.Join(tmpDir, "dst.bin")

	content := []byte("end-to-end-media-sidecar-test-stream")
	if err := os.WriteFile(srcFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if err := os.WriteFile(dstFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Bit exact
	res, err := runner.VerifyMedia(ctx, srcFile, dstFile, "bit_exact", "", "", 0, 0)
	if err != nil {
		t.Fatalf("VerifyMedia bit_exact failed: %v", err)
	}
	if !res.HashMatch {
		t.Error("expected HashMatch=true for identical files")
	}

	// Quality bounded
	resQB, err := runner.VerifyMedia(ctx, srcFile, dstFile, "quality_bounded", "", "", 80.0, 0.90)
	if err != nil {
		t.Fatalf("VerifyMedia quality_bounded failed: %v", err)
	}
	if !resQB.HashMatch {
		t.Error("expected HashMatch=true for quality bounded test")
	}
}
