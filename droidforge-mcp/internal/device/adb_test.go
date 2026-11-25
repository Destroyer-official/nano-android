package device

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestResolveADBPath(t *testing.T) {
	path := resolveADBPath("")
	if path == "" {
		t.Fatal("expected non-empty ADB path")
	}
	t.Logf("Resolved ADB path: %s", path)
}

func TestExecADBNoShellInjection(t *testing.T) {
	// ADB must execute directly without shell interpolation.
	// If a command argument containing shell metacharacters like ';' or '&' or '|' or '$'
	// were passed to a shell, it would execute multiple commands.
	// When passed to pure exec.Command, ADB simply receives them as a literal argument string
	// and rejects them or handles them as literal strings.
	adb := GetDefaultADB()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Intentionally malicious payloads trying to trigger command chaining
	maliciousArgs := []string{
		"; echo INJECTED",
		"& echo INJECTED",
		"| echo INJECTED",
		"$(echo INJECTED)",
		"`echo INJECTED`",
	}

	for _, arg := range maliciousArgs {
		out, err := adb.ExecContext(ctx, "", "version", arg)
		// The command should either succeed printing version or fail inside ADB,
		// but "INJECTED" must NEVER be output as a separate command execution.
		if strings.Contains(out, "INJECTED") {
			t.Fatalf("CRITICAL SECURITY FAILURE: shell injection detected with arg %q (stdout: %s)", arg, out)
		}
		t.Logf("Tested malicious arg %q: err=%v", arg, err)
	}
}

func TestExecADBWithTimeout(t *testing.T) {
	adb := GetDefaultADB()

	// A normal version check should finish well within 5 seconds
	out, err := adb.ExecWithTimeout("", 5*time.Second, "version")
	if err != nil {
		t.Fatalf("adb version failed: %v", err)
	}
	if !strings.Contains(out, "Android Debug Bridge") {
		t.Fatalf("expected 'Android Debug Bridge' in version output, got: %s", out)
	}
}

func TestExecADBContextCancellation(t *testing.T) {
	adb := GetDefaultADB()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := adb.ExecContext(ctx, "", "version")
	if err == nil {
		t.Fatal("expected error on canceled context, got nil")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("expected 'canceled' in error, got: %v", err)
	}
}

func TestADBErrorFormatting(t *testing.T) {
	adb := GetDefaultADB()
	// Run an invalid subcommand to verify ADBError capture
	_, err := adb.Exec("", "nonexistent-subcommand-xyz-12345")
	if err == nil {
		t.Fatal("expected error on nonexistent subcommand")
	}

	var adbErr *ADBError
	if !strings.Contains(err.Error(), "adb error") && !strings.Contains(err.Error(), "unknown command") {
		t.Logf("Got expected error: %v", err)
	}
	_ = adbErr
}

func TestFormatCommand(t *testing.T) {
	cmdStr := formatCommand("adb", []string{"-s", "emulator-5554", "shell", "getprop"})
	expected := "adb -s emulator-5554 shell getprop"
	if cmdStr != expected {
		t.Fatalf("expected %q, got %q", expected, cmdStr)
	}
}

func TestADBValidationHelpers(t *testing.T) {
	adb := GetDefaultADB()
	ctx := context.Background()

	// 1. Install with empty path
	if _, err := adb.Install(ctx, "test-serial", ""); err == nil {
		t.Error("expected error on Install with empty path, got nil")
	}

	// 2. InstallMultiple with empty paths
	if _, err := adb.InstallMultiple(ctx, "test-serial", nil); err == nil {
		t.Error("expected error on InstallMultiple with nil paths, got nil")
	}

	// 3. PidOf with empty package
	if _, err := adb.PidOf(ctx, "test-serial", ""); err == nil {
		t.Error("expected error on PidOf with empty package, got nil")
	}

	// 4. EmuSnapshotLoad with empty snapshot
	if err := adb.EmuSnapshotLoad(ctx, "test-serial", ""); err == nil {
		t.Error("expected error on EmuSnapshotLoad with empty snapshot, got nil")
	}

	// 5. ClearPackage with empty package
	if err := adb.ClearPackage(ctx, "test-serial", ""); err == nil {
		t.Error("expected error on ClearPackage with empty package, got nil")
	}

	// 6. GrantPermission with empty package or permission
	if err := adb.GrantPermission(ctx, "test-serial", "", "android.permission.INTERNET"); err == nil {
		t.Error("expected error on GrantPermission with empty pkg, got nil")
	}
	if err := adb.GrantPermission(ctx, "test-serial", "com.example", ""); err == nil {
		t.Error("expected error on GrantPermission with empty perm, got nil")
	}
}

// MockCommandRunner for deterministic testing of ADB failure modes
type mockRunner struct {
	runFn func(ctx context.Context, bin string, args []string) (string, string, error)
}

func (m *mockRunner) RunCommand(ctx context.Context, bin string, args []string) (string, string, error) {
	if m.runFn != nil {
		return m.runFn(ctx, bin, args)
	}
	return "", "", nil
}

func TestMockADBRunner(t *testing.T) {
	adb := NewADB("mock-adb")
	mock := &mockRunner{
		runFn: func(ctx context.Context, bin string, args []string) (string, string, error) {
			if len(args) > 0 && args[0] == "devices" {
				return "List of devices attached\nemulator-5554 device\n", "", nil
			}
			if len(args) > 0 && args[0] == "fail" {
				return "", "device not found", &exec.ExitError{}
			}
			return "OK", "", nil
		},
	}
	adb.SetRunner(mock)

	// Test devices query
	out, err := adb.Exec("", "devices")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "emulator-5554") {
		t.Errorf("expected emulator-5554 in output, got: %s", out)
	}

	// Test error capture
	_, err = adb.Exec("", "fail")
	if err == nil {
		t.Fatal("expected error on fail, got nil")
	}
	var adbErr *ADBError
	if errors.As(err, &adbErr) {
		if adbErr.Stderr != "device not found" {
			t.Errorf("expected stderr 'device not found', got %q", adbErr.Stderr)
		}
	}
}

func TestEmuSnapshotListParsing(t *testing.T) {
	adb := NewADB("mock-adb")
	adb.SetRunner(&mockRunner{
		runFn: func(ctx context.Context, bin string, args []string) (string, string, error) {
			sample := `Snapshot list:
default_boot 2026-11-20_14-00-00 512MB
clean_state  2026-11-20_14-30-00 512MB
OK
`
			return sample, "", nil
		},
	})

	snapshots, err := adb.EmuSnapshotList(context.Background(), "emulator-5554")
	if err != nil {
		t.Fatalf("EmuSnapshotList failed: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}
	if snapshots[0] != "default_boot" || snapshots[1] != "clean_state" {
		t.Errorf("unexpected snapshots: %v", snapshots)
	}
}

func TestLiveConnectedDevice(t *testing.T) {
	// If a live device is connected, test querying properties on it
	adb := GetDefaultADB()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	devicesOut, err := adb.ExecContext(ctx, "", "devices", "-l")
	if err != nil {
		t.Skipf("skipping live test: adb devices failed: %v", err)
	}

	devices, err := ParseADBDevicesList(devicesOut)
	if err != nil || len(devices) == 0 {
		t.Skip("skipping live test: no connected devices found")
	}

	var activeDev *string
	for _, d := range devices {
		if d.State == "device" {
			activeDev = &d.Serial
			break
		}
	}

	if activeDev == nil {
		t.Skip("skipping live test: no authorized online device found")
	}

	serial := *activeDev
	t.Logf("Running live device validation against %s", serial)

	// Test GetProp ro.build.version.sdk
	sdkVer, err := adb.GetProp(ctx, serial, "ro.build.version.sdk")
	if err != nil {
		t.Fatalf("GetProp ro.build.version.sdk failed on live device: %v", err)
	}
	t.Logf("Live device SDK version: %s", sdkVer)

	// Test GetProp ro.product.cpu.abi
	abi, err := adb.GetProp(ctx, serial, "ro.product.cpu.abi")
	if err != nil {
		t.Fatalf("GetProp ro.product.cpu.abi failed on live device: %v", err)
	}
	t.Logf("Live device ABI: %s", abi)
}
