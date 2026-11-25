package harness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// mockDeviceManager implements types.DeviceManager for unit tests.
type mockDeviceManager struct {
	execADBOutputs        map[string]string
	emulatorConsoleOutput map[string]string
	executedADBCommands   [][]string
	executedConsoleCmds   []string
}

func newMockDeviceManager() *mockDeviceManager {
	return &mockDeviceManager{
		execADBOutputs:        make(map[string]string),
		emulatorConsoleOutput: make(map[string]string),
		executedADBCommands:   make([][]string, 0),
		executedConsoleCmds:   make([]string, 0),
	}
}

func (m *mockDeviceManager) ListDevices(includeAVDs bool) ([]types.DeviceInfo, error) {
	return nil, nil
}

func (m *mockDeviceManager) BootSnapshot(avdName, snapshot string, headless bool, ramMB int) (*types.DeviceInfo, error) {
	return nil, nil
}

func (m *mockDeviceManager) ResetState(serial, strategy, pkgID, snapshotName string) error {
	return nil
}

func (m *mockDeviceManager) InstallAndLaunch(serial, apkPath, activity, deepLink string, grantPerms bool) (*types.AppLaunchResult, error) {
	return nil, nil
}

func (m *mockDeviceManager) ExecADB(serial string, args ...string) (string, error) {
	m.executedADBCommands = append(m.executedADBCommands, append([]string{serial}, args...))
	key := strings.Join(args, " ")
	if out, ok := m.execADBOutputs[key]; ok {
		return out, nil
	}
	// Prefix match if exact key not found
	for k, out := range m.execADBOutputs {
		if strings.HasPrefix(key, k) {
			return out, nil
		}
	}
	return "", nil
}

func (m *mockDeviceManager) ShutdownEmulator(serial string) error {
	return nil
}

func (m *mockDeviceManager) EmulatorConsole(serial, command string) (string, error) {
	m.executedConsoleCmds = append(m.executedConsoleCmds, command)
	if out, ok := m.emulatorConsoleOutput[command]; ok {
		return out, nil
	}
	return "OK\r\n", nil
}

// mockSidecarRunner implements SidecarRunner for unit tests.
type mockSidecarRunner struct {
	verifyResult *types.MediaVerifyResult
	verifyErr    error
}

func (m *mockSidecarRunner) VerifyMedia(ctx context.Context, srcPath, dstPath, mode, goldenHash, outPath string, minVMAF, minSSIM float64) (*types.MediaVerifyResult, error) {
	if m.verifyErr != nil {
		return nil, m.verifyErr
	}
	if m.verifyResult != nil {
		return m.verifyResult, nil
	}
	return &types.MediaVerifyResult{
		OutputSHA256: "dummy_output_hash",
		HashMatch:    true,
		VMAF:         95.0,
		SSIM:         0.98,
		DecodeErrors: 0,
	}, nil
}

// TestSMSConsoleCommandBuilder tests telnet command formatting for SMS.
func TestSMSConsoleCommandBuilder(t *testing.T) {
	cmd := BuildSMSConsoleCmd("+15550199", "Your OTP is 123456")
	expected := "sms send +15550199 Your OTP is 123456"
	if cmd != expected {
		t.Errorf("expected %q, got %q", expected, cmd)
	}
}

// TestCallConsoleCommandBuilder tests telnet command formatting for GSM call.
func TestCallConsoleCommandBuilder(t *testing.T) {
	tests := []struct {
		action   string
		number   string
		expected string
		wantErr  bool
	}{
		{"call", "+15550199", "gsm call +15550199", false},
		{"", "+15550199", "gsm call +15550199", false},
		{"accept", "+15550199", "gsm accept +15550199", false},
		{"cancel", "+15550199", "gsm cancel +15550199", false},
		{"hold", "+15550199", "gsm hold +15550199", false},
		{"invalid", "+15550199", "", true},
	}

	for _, tt := range tests {
		got, err := BuildCallConsoleCmd(tt.action, tt.number)
		if (err != nil) != tt.wantErr {
			t.Errorf("BuildCallConsoleCmd(%q, %q) error = %v, wantErr %v", tt.action, tt.number, err, tt.wantErr)
			continue
		}
		if got != tt.expected {
			t.Errorf("BuildCallConsoleCmd(%q, %q) = %q, expected %q", tt.action, tt.number, got, tt.expected)
		}
	}
}

// TestDetectAndroid17OTP tests OTP keyword + code detection regex logic.
func TestDetectAndroid17OTP(t *testing.T) {
	tests := []struct {
		body     string
		expected bool
	}{
		{"Your verification code is 482910.", true},
		{"URGENT: Bank OTP is 1234. Do not share.", true},
		{"123456 is your login code.", true},
		{"One-time password: 98765432", true},
		{"Your security PIN is 5566", true},
		{"Hello, are you coming to dinner tonight?", false},
		{"Call me at 5551234 later", false}, // phone number without OTP keyword
		{"Verification required without any digits", false},
		{"", false},
	}

	for _, tt := range tests {
		got := DetectAndroid17OTP(tt.body)
		if got != tt.expected {
			t.Errorf("DetectAndroid17OTP(%q) = %v, expected %v", tt.body, got, tt.expected)
		}
	}
}

// TestInjectSMSOrCall tests SMS and Call injection flows including emulator serial check.
func TestInjectSMSOrCall(t *testing.T) {
	ctx := context.Background()
	dm := newMockDeviceManager()

	// Physical device should be rejected
	_, err := InjectSMSOrCall(ctx, dm, "physical-phone-123", "sms", "+15550199", "Test message")
	if err == nil {
		t.Fatal("expected error on physical device, got nil")
	}

	// Valid emulator serial
	res, err := InjectSMSOrCall(ctx, dm, "emulator-5554", "sms", "+15550199", "Your OTP is 482910")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Injected {
		t.Error("expected Injected=true")
	}
	if !res.Android17OTPDetected {
		t.Error("expected Android17OTPDetected=true")
	}
	if len(dm.executedConsoleCmds) == 0 {
		t.Error("expected console command to be executed")
	}

	// GSM Call test
	callRes, err := InjectSMSOrCall(ctx, dm, "emulator-5554", "call", "+15550199", "call")
	if err != nil {
		t.Fatalf("unexpected call error: %v", err)
	}
	if !callRes.Injected {
		t.Error("expected Injected=true for call")
	}
}

// TestPostNotification tests notification injection and optional channel creation.
func TestPostNotification(t *testing.T) {
	ctx := context.Background()
	dm := newMockDeviceManager()

	req := NotificationRequest{
		Serial:    "emulator-5554",
		Title:     "Scam Alert",
		Body:      "Suspicious SMS intercepted",
		Channel:   "alerts_channel",
		Tag:       "scam_tag",
		PackageID: "com.example.scamshield",
	}

	res, err := PostNotification(ctx, dm, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Posted {
		t.Error("expected Posted=true")
	}
	if res.Method != "cmd_notification" {
		t.Errorf("expected method cmd_notification, got %s", res.Method)
	}

	// Verify channel creation and post command were issued
	if len(dm.executedADBCommands) != 2 {
		t.Fatalf("expected 2 adb commands (create_channel + post), got %d", len(dm.executedADBCommands))
	}
	cmd1 := strings.Join(dm.executedADBCommands[0], " ")
	cmd2 := strings.Join(dm.executedADBCommands[1], " ")
	if !strings.Contains(cmd1, "create_channel") {
		t.Errorf("command 1 should create channel, got: %s", cmd1)
	}
	if !strings.Contains(cmd2, "cmd notification post") {
		t.Errorf("command 2 should post notification, got: %s", cmd2)
	}
}

// TestSetPermissions tests granting, revoking, appops, standby bucket, and doze simulation.
func TestSetPermissions(t *testing.T) {
	ctx := context.Background()
	dm := newMockDeviceManager()

	req := PermissionsRequest{
		Serial:    "emulator-5554",
		PackageID: "com.example.app",
		Grants:    []string{"android.permission.POST_NOTIFICATIONS", "android.permission.RECEIVE_SMS"},
		Revokes:   []string{"android.permission.ACCESS_FINE_LOCATION"},
		AppOps: []AppOp{
			{Op: "ACCESS_RESTRICTED_SETTINGS", Mode: "allow"},
		},
		StandbyBucket: "active",
		DozeMode:      "force-idle",
	}

	res, err := SetPermissions(ctx, dm, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Errors) > 0 {
		t.Errorf("expected 0 errors, got: %v", res.Errors)
	}
	if len(res.Applied) != 6 {
		t.Errorf("expected 6 applied actions, got %d: %v", len(res.Applied), res.Applied)
	}
}

// TestMediaFixturePushCaching tests that media fixture push skips when hash matches.
func TestMediaFixturePushCaching(t *testing.T) {
	ctx := context.Background()
	dm := newMockDeviceManager()

	// Create a temp fixture file
	tmpDir := t.TempDir()
	localFile := filepath.Join(tmpDir, "sample.mp4")
	content := []byte("fixture-sample-video-content-stream")
	if err := os.WriteFile(localFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	h := sha256.Sum256(content)
	hashHex := hex.EncodeToString(h[:])

	// Case 1: On-device hash matches -> PUSH IS SKIPPED (cached)
	dm.execADBOutputs["shell sha256sum /sdcard/Download/sample.mp4"] = fmt.Sprintf("%s  /sdcard/Download/sample.mp4\n", hashHex)

	req := MediaPushVerifyRequest{
		Serial:           "emulator-5554",
		LocalFixturePath: localFile,
		DeviceDestPath:   "/sdcard/Download/sample.mp4",
	}

	res, err := PushAndVerifyMedia(ctx, dm, nil, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.PushedCached {
		t.Error("expected PushedCached=true when hash matches on device")
	}

	// Verify adb push was not called
	for _, cmd := range dm.executedADBCommands {
		cmdStr := strings.Join(cmd, " ")
		if strings.Contains(cmdStr, "push") {
			t.Errorf("adb push should not be called when cached, but got %s", cmdStr)
		}
	}

	// Case 2: On-device hash differs -> PUSH IS EXECUTED
	dm2 := newMockDeviceManager()
	dm2.execADBOutputs["shell sha256sum /sdcard/Download/sample.mp4"] = "differenthash1234567890abcdef1234567890abcdef1234567890abcdef1234  /sdcard/Download/sample.mp4\n"

	res2, err := PushAndVerifyMedia(ctx, dm2, nil, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res2.PushedCached {
		t.Error("expected PushedCached=false when hash differs on device")
	}

	pushed := false
	for _, cmd := range dm2.executedADBCommands {
		cmdStr := strings.Join(cmd, " ")
		if strings.Contains(cmdStr, "push") {
			pushed = true
		}
	}
	if !pushed {
		t.Error("expected adb push to be called when hash does not match")
	}
}

// TestMediaBitExactVerification tests bit-exact SHA-256 verification of output file.
func TestMediaBitExactVerification(t *testing.T) {
	ctx := context.Background()
	dm := newMockDeviceManager()

	goldenHash := "a1b2c3d4e5f67890123456789012345678901234567890123456789012345678"
	dm.execADBOutputs["shell sha256sum /sdcard/Movies/output.mp4"] = fmt.Sprintf("%s  /sdcard/Movies/output.mp4\n", goldenHash)

	req := MediaPushVerifyRequest{
		Serial:           "emulator-5554",
		OutputDevicePath: "/sdcard/Movies/output.mp4",
		Oracle:           "sha256",
		GoldenSHA256:     goldenHash,
	}

	res, err := PushAndVerifyMedia(ctx, dm, nil, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.HashMatch {
		t.Error("expected HashMatch=true for matching golden hash")
	}
	if res.OutputSHA256 != goldenHash {
		t.Errorf("expected OutputSHA256 %s, got %s", goldenHash, res.OutputSHA256)
	}
}

// TestStripZeroWidthAndBidiHarness tests invisible Unicode prompt injection stripping.
func TestStripZeroWidthAndBidiHarness(t *testing.T) {
	adversarial := "Hello\u200B\u200CWorld\u202DTest\uFEFF"
	cleaned := StripZeroWidthAndBidi(adversarial)
	expected := "HelloWorldTest"
	if cleaned != expected {
		t.Errorf("expected %q, got %q", expected, cleaned)
	}
}

// TestTelnetCarriageReturnSanitization tests that carriage returns / newlines are stripped from telnet console commands.
func TestTelnetCarriageReturnSanitization(t *testing.T) {
	injectedBody := "Verification code: 123456\r\nkill\r\nquit"
	cmd := BuildSMSConsoleCmd("+15550199", injectedBody)

	if strings.Contains(cmd, "\r") || strings.Contains(cmd, "\n") {
		t.Errorf("BuildSMSConsoleCmd must not contain carriage returns or newlines: %q", cmd)
	}
	if !strings.Contains(cmd, "sms send +15550199") {
		t.Errorf("expected sms send command prefix, got: %q", cmd)
	}
}

// TestFlagInjectionRejectionHarness tests that flag injection in package_id is rejected.
func TestFlagInjectionRejectionHarness(t *testing.T) {
	ctx := context.Background()
	dm := newMockDeviceManager()

	dangerousPkg := "--user 0 com.android.settings"
	_, err := SetPermissions(ctx, dm, PermissionsRequest{
		Serial:    "emulator-5554",
		PackageID: dangerousPkg,
		Grants:    []string{"android.permission.READ_SMS"},
	})
	if err == nil {
		t.Fatal("expected error rejecting flag injection in SetPermissions, got nil")
	}
}

// TestInvalidPhoneRejection tests that illegal characters in phone number are rejected.
func TestInvalidPhoneRejection(t *testing.T) {
	ctx := context.Background()
	dm := newMockDeviceManager()

	_, err := InjectSMSOrCall(ctx, dm, "emulator-5554", "sms", "+15550199; rm -rf /", "Hello")
	if err == nil {
		t.Fatal("expected error rejecting phone number with shell metachars, got nil")
	}

	_, err = InjectSMSOrCall(ctx, dm, "emulator-5554", "call", "123\r\nquit", "call")
	if err == nil {
		t.Fatal("expected error rejecting phone number with newlines, got nil")
	}
}
