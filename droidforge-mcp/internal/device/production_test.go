package device

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

func TestADBExtendedFunctions(t *testing.T) {
	adb := NewADB("mock-adb")
	mock := &mockRunner{
		runFn: func(ctx context.Context, bin string, args []string) (string, string, error) {
			joined := strings.Join(args, " ")
			switch {
			case strings.Contains(joined, "pidof -s com.example.app"):
				return "9876\n", "", nil
			case strings.Contains(joined, "pidof com.example.fallback"):
				return "5432\n", "", nil
			case strings.Contains(joined, "cmd activity get-current-app-pid"):
				return "1122\n", "", nil
			case strings.Contains(joined, "install -r -t app.apk"):
				return "Performing Streamed Install\nSuccess\n", "", nil
			case strings.Contains(joined, "install --incremental"):
				return "Failure [INSTALL_FAILED_INCREMENTAL]\n", "error", errors.New("incremental failed")
			case strings.Contains(joined, "install-multiple"):
				return "Success\n", "", nil
			case strings.Contains(joined, "wait-for-device"):
				return "", "", nil
			case strings.Contains(joined, "getprop sys.boot_completed"):
				return "1\n", "", nil
			case strings.Contains(joined, "settings put global"):
				return "", "", nil
			case strings.Contains(joined, "input keyevent"):
				return "", "", nil
			case strings.Contains(joined, "wm dismiss-keyguard"):
				return "", "", nil
			case strings.Contains(joined, "logcat -d"):
				return "--------- beginning of main\n11-20 20:00:00.000 ActivityManager: Displayed com.example/.MainActivity: +250ms\n", "", nil
			case strings.Contains(joined, "emu avd snapshot load clean_state"):
				return "OK\n", "", nil
			case strings.Contains(joined, "emu avd snapshot load bad_snapshot"):
				return "KO: snapshot does not exist\n", "", nil
			case strings.Contains(joined, "pm clear com.example.app"):
				return "Success\n", "", nil
			case strings.Contains(joined, "pm clear com.example.fail"):
				return "Failed\n", "", nil
			case strings.Contains(joined, "rm -rf"):
				return "", "", nil
			case strings.Contains(joined, "pm grant"):
				return "", "", nil
			case strings.Contains(joined, "kill-server") || strings.Contains(joined, "start-server"):
				return "", "", nil
			default:
				return "OK\n", "", nil
			}
		},
	}
	adb.SetRunner(mock)
	ctx := context.Background()

	// 1. PidOf tests
	pid, err := adb.PidOf(ctx, "serial", "com.example.app")
	if err != nil || pid != 9876 {
		t.Errorf("PidOf com.example.app = %d, expected 9876, err=%v", pid, err)
	}

	pidFallback, err := adb.PidOf(ctx, "serial", "com.example.fallback")
	if err != nil || pidFallback != 5432 {
		t.Errorf("PidOf fallback = %d, expected 5432, err=%v", pidFallback, err)
	}

	// 2. Install & InstallMultiple
	dur, err := adb.Install(ctx, "serial", "app.apk")
	if err != nil || dur < 0 {
		t.Errorf("Install failed: %v", err)
	}

	durMult, err := adb.InstallMultiple(ctx, "serial", []string{"base.apk", "split1.apk"})
	if err != nil || durMult < 0 {
		t.Errorf("InstallMultiple failed: %v", err)
	}

	// 3. WaitForDevice & WaitForBootComplete
	if err := adb.WaitForDevice(ctx, "serial", 5*time.Second); err != nil {
		t.Errorf("WaitForDevice failed: %v", err)
	}
	if err := adb.WaitForBootComplete(ctx, "serial", 5*time.Second); err != nil {
		t.Errorf("WaitForBootComplete failed: %v", err)
	}

	// 4. DisableAnimations, SetStayAwake, WakeUp
	if err := adb.DisableAnimations(ctx, "serial"); err != nil {
		t.Errorf("DisableAnimations failed: %v", err)
	}
	if err := adb.SetStayAwake(ctx, "serial", true); err != nil {
		t.Errorf("SetStayAwake(true) failed: %v", err)
	}
	if err := adb.SetStayAwake(ctx, "serial", false); err != nil {
		t.Errorf("SetStayAwake(false) failed: %v", err)
	}
	if err := adb.WakeUp(ctx, "serial"); err != nil {
		t.Errorf("WakeUp failed: %v", err)
	}

	// 5. LogcatCapture
	logs, err := adb.LogcatCapture(ctx, "serial", "ActivityManager:I", 50)
	if err != nil || !strings.Contains(logs, "ActivityManager") {
		t.Errorf("LogcatCapture failed: %v, logs=%q", err, logs)
	}

	// 6. EmuSnapshotLoad
	if err := adb.EmuSnapshotLoad(ctx, "serial", "clean_state"); err != nil {
		t.Errorf("EmuSnapshotLoad clean_state failed: %v", err)
	}
	if err := adb.EmuSnapshotLoad(ctx, "serial", "bad_snapshot"); err == nil {
		t.Errorf("EmuSnapshotLoad bad_snapshot: expected error, got nil")
	}

	// 7. ClearPackage & RemoveFixtures & GrantPermission
	if err := adb.ClearPackage(ctx, "serial", "com.example.app"); err != nil {
		t.Errorf("ClearPackage failed: %v", err)
	}
	if err := adb.ClearPackage(ctx, "serial", "com.example.fail"); err == nil {
		t.Errorf("ClearPackage fail: expected error, got nil")
	}
	if err := adb.RemoveFixtures(ctx, "serial", "/sdcard/fixtures"); err != nil {
		t.Errorf("RemoveFixtures failed: %v", err)
	}
	if err := adb.GrantPermission(ctx, "serial", "com.example.app", "android.permission.CAMERA"); err != nil {
		t.Errorf("GrantPermission failed: %v", err)
	}

	// 8. RestartServer
	if err := adb.RestartServer(ctx); err != nil {
		t.Errorf("RestartServer failed: %v", err)
	}

	// 9. Path & SetPath
	origPath := adb.Path()
	adb.SetPath("custom-adb")
	if adb.Path() != "custom-adb" {
		t.Errorf("expected 'custom-adb', got %q", adb.Path())
	}
	adb.SetPath(origPath)

	// 10. Unwrap & containsString
	baseErr := errors.New("underlying error")
	adbErr := &ADBError{Err: baseErr}
	if adbErr.Unwrap() != baseErr {
		t.Errorf("Unwrap failed: expected %v, got %v", baseErr, adbErr.Unwrap())
	}
	if !containsString([]string{"alpha", "beta"}, "beta") || containsString([]string{"alpha", "beta"}, "gamma") {
		t.Error("containsString logic error")
	}

	// 11. Top-level package functions
	_, _ = ExecADB("")
	_, _ = ExecADBWithTimeout("", 1*time.Second)
	_, _ = ExecADBContext(ctx, "")
}

func TestManagerFullMockLifecycle(t *testing.T) {
	adb := NewADB("mock-adb")
	mock := &mockRunner{
		runFn: func(ctx context.Context, bin string, args []string) (string, string, error) {
			joined := strings.Join(args, " ")
			switch {
			case strings.Contains(joined, "devices -l"):
				return `List of devices attached
emulator-5554 device product:sdk_gphone64 model:api37_clean device:emu64x transport_id:1
`, "", nil
			case strings.Contains(joined, "getprop ro.build.version.sdk"):
				return "37\n", "", nil
			case strings.Contains(joined, "getprop ro.product.cpu.abi"):
				return "x86_64\n", "", nil
			case strings.Contains(joined, "getprop ro.product.model"):
				return "api37_clean\n", "", nil
			case strings.Contains(joined, "dumpsys battery"):
				return "level: 100\ntemperature: 250\n", "", nil
			case strings.Contains(joined, "install"):
				return "Success\n", "", nil
			case strings.Contains(joined, "am start -W"):
				return "Status: ok\nLaunchState: COLD\nTotalTime: 245\nWaitTime: 260\nThisTime: 245\n", "", nil
			case strings.Contains(joined, "pidof"):
				return "4455\n", "", nil
			case strings.Contains(joined, "emu avd snapshot load"):
				return "OK\n", "", nil
			case strings.Contains(joined, "emu kill"):
				return "OK\n", "", nil
			default:
				return "OK\n", "", nil
			}
		},
	}
	adb.SetRunner(mock)

	mgr := NewDeviceManager(
		WithADB(adb),
		WithEmulatorPath("mock-emulator"),
		WithEmulatorOnly(true),
		WithAllowlistedPackages([]string{"com.example.mock"}),
	)

	// 1. ListDevices
	devices, err := mgr.ListDevices(false)
	if err != nil || len(devices) != 1 {
		t.Fatalf("ListDevices failed: %v, count=%d", err, len(devices))
	}
	d := devices[0]
	if d.Serial != "emulator-5554" || d.APILevel != 37 || d.BatteryPct != 100 {
		t.Errorf("unexpected device fields: %+v", d)
	}

	// 2. BootSnapshot reusing already running AVD
	bootInfo, err := mgr.BootSnapshot("api37_clean", "clean_state", true, 2048)
	if err != nil {
		t.Fatalf("BootSnapshot existing AVD failed: %v", err)
	}
	if bootInfo.Serial != "emulator-5554" {
		t.Errorf("expected serial 'emulator-5554', got %q", bootInfo.Serial)
	}

	// 3. ResetState snapshot_restore
	if err := mgr.ResetState("emulator-5554", "snapshot_restore", "", "clean_state"); err != nil {
		t.Errorf("ResetState snapshot_restore failed: %v", err)
	}

	// 4. InstallAndLaunch
	launchRes, err := mgr.InstallAndLaunch("emulator-5554", "mock.apk", "com.example.mock/.MainActivity", "", true)
	if err != nil {
		t.Fatalf("InstallAndLaunch failed: %v", err)
	}
	if launchRes.PID != 4455 {
		t.Errorf("expected PID 4455, got %d", launchRes.PID)
	}
	if launchRes.ColdStartMS != 245 {
		t.Errorf("expected ColdStartMS 245, got %d", launchRes.ColdStartMS)
	}

	// 5. ExecADB, ShutdownEmulator, listAvailableAVDs error
	out, err := mgr.ExecADB("emulator-5554", "version")
	if err != nil || out == "" {
		t.Errorf("mgr.ExecADB failed: %v", err)
	}
	if err := mgr.ShutdownEmulator("emulator-5554"); err != nil {
		t.Errorf("mgr.ShutdownEmulator failed: %v", err)
	}

	mgrEmptyEmu := NewDeviceManager(WithEmulatorPath(""))
	if _, err := mgrEmptyEmu.listAvailableAVDs(context.Background()); err == nil {
		t.Error("expected error when emulatorPath is empty, got nil")
	}
}

func TestConsoleClientAuthFileAndTimeout(t *testing.T) {
	client, err := NewEmulatorConsoleClient("emulator-5554")
	if err != nil {
		t.Fatalf("NewEmulatorConsoleClient failed: %v", err)
	}

	client.SetTimeout(12 * time.Second)
	if client.timeout != 12*time.Second {
		t.Errorf("SetTimeout failed: expected 12s, got %v", client.timeout)
	}

	// Test readAuthTokenFile
	tmpToken := filepath.Join(t.TempDir(), "auth_token")
	_ = os.WriteFile(tmpToken, []byte("super_secret_token_123\n"), 0600)
	token, err := readAuthTokenFile(tmpToken)
	if err != nil || token != "super_secret_token_123" {
		t.Errorf("readAuthTokenFile failed: %v, token=%q", err, token)
	}

	// Test empty auth token file
	_ = os.WriteFile(tmpToken, []byte("   \n"), 0600)
	if _, err := readAuthTokenFile(tmpToken); err == nil {
		t.Error("expected error on whitespace-only token file, got nil")
	}
}

func TestParseAPKFileOnDisk(t *testing.T) {
	tmpDir := t.TempDir()
	apkPath := filepath.Join(tmpDir, "sample.apk")

	pkg := "com.example.ondisk"
	act := ".LaunchActivity"
	perms := []string{"android.permission.INTERNET"}
	axmlBytes := buildSyntheticAXML(pkg, act, perms)

	f, err := os.Create(apkPath)
	if err != nil {
		t.Fatalf("failed creating temp APK: %v", err)
	}
	zw := zip.NewWriter(f)
	ze, err := zw.Create("AndroidManifest.xml")
	if err != nil {
		t.Fatalf("failed creating zip entry: %v", err)
	}
	_, _ = ze.Write(axmlBytes)
	_ = zw.Close()
	_ = f.Close()

	manifest, err := ParseAPKFile(apkPath)
	if err != nil {
		t.Fatalf("ParseAPKFile failed: %v", err)
	}
	if manifest.PackageID != pkg {
		t.Errorf("expected %q, got %q", pkg, manifest.PackageID)
	}
	if manifest.Component != "com.example.ondisk/.LaunchActivity" {
		t.Errorf("expected 'com.example.ondisk/.LaunchActivity', got %q", manifest.Component)
	}

	// Test non-existent file
	if _, err := ParseAPKFile(filepath.Join(tmpDir, "nonexistent.apk")); err == nil {
		t.Error("expected error on nonexistent APK, got nil")
	}
}

func TestAXMLUTF8Strings(t *testing.T) {
	// Build AXML with UTF-8 flag set (bit 8 = 0x0100)
	stringsList := []string{
		"manifest",
		"package",
		"com.utf8.testapp",
		"activity",
		"name",
		".MainActivity",
	}

	var strDataBuf bytes.Buffer
	var offsets []uint32

	for _, s := range stringsList {
		offsets = append(offsets, uint32(strDataBuf.Len()))
		// UTF-8 encoding in AXML: 1 byte char len + 1 byte byte len + bytes + null
		strDataBuf.WriteByte(byte(len(s)))
		strDataBuf.WriteByte(byte(len(s)))
		strDataBuf.WriteString(s)
		strDataBuf.WriteByte(0)
	}

	stringCount := uint32(len(stringsList))
	flags := utf8Flag // 0x0100
	stringsStart := uint32(28 + 4*stringCount)
	spChunkSize := uint32(28 + 4*stringCount + uint32(strDataBuf.Len()))
	if spChunkSize%4 != 0 {
		pad := 4 - (spChunkSize % 4)
		spChunkSize += pad
		strDataBuf.Write(make([]byte, pad))
	}

	var spBuf bytes.Buffer
	_ = binary.Write(&spBuf, binary.LittleEndian, chunkStringPool)
	_ = binary.Write(&spBuf, binary.LittleEndian, spChunkSize)
	_ = binary.Write(&spBuf, binary.LittleEndian, stringCount)
	_ = binary.Write(&spBuf, binary.LittleEndian, uint32(0))
	_ = binary.Write(&spBuf, binary.LittleEndian, flags)
	_ = binary.Write(&spBuf, binary.LittleEndian, stringsStart)
	_ = binary.Write(&spBuf, binary.LittleEndian, uint32(0))

	for _, off := range offsets {
		_ = binary.Write(&spBuf, binary.LittleEndian, off)
	}
	spBuf.Write(strDataBuf.Bytes())

	// Test string pool parser with UTF-8
	pool, err := parseStringPool(spBuf.Bytes())
	if err != nil {
		t.Fatalf("parseStringPool with UTF-8 failed: %v", err)
	}
	if len(pool) != len(stringsList) {
		t.Fatalf("expected %d strings, got %d", len(stringsList), len(pool))
	}
	if pool[2] != "com.utf8.testapp" {
		t.Errorf("expected 'com.utf8.testapp', got %q", pool[2])
	}
}

func TestToolsCoercionAndEdgeCases(t *testing.T) {
	// Test coerceBool
	if !coerceBool(true, false) || !coerceBool("true", false) || !coerceBool("1", false) || !coerceBool("yes", false) {
		t.Error("coerceBool failed for true values")
	}
	if coerceBool(false, true) || coerceBool("false", true) || coerceBool("0", true) || coerceBool("no", true) {
		t.Error("coerceBool failed for false values")
	}
	if !coerceBool(nil, true) || coerceBool(nil, false) {
		t.Error("coerceBool failed for nil default fallback")
	}

	// Test coerceInt
	if coerceInt(123, 0) != 123 || coerceInt(int64(456), 0) != 456 || coerceInt(float64(789), 0) != 789 || coerceInt("1024", 0) != 1024 {
		t.Error("coerceInt failed on numbers")
	}
	if coerceInt(nil, 4096) != 4096 || coerceInt("invalid", 4096) != 4096 {
		t.Error("coerceInt failed on invalid fallback")
	}

	// Test tools missing required args
	ctx := context.Background()
	mockDM := &mockDeviceManager{
		listDevicesFn: func(includeAVDs bool) ([]types.DeviceInfo, error) {
			return []types.DeviceInfo{
				{Serial: "emulator-5554", IsEmulator: true},
				{Serial: "usb-device", IsEmulator: false},
			}, nil
		},
	}

	// Test DeviceListTool with emulator_only: true
	listTool := NewDeviceListTool(mockDM)
	res, err := listTool.Execute(ctx, map[string]any{"emulator_only": true})
	if err != nil || !res.Success {
		t.Fatalf("listTool emulator_only failed: %v", err)
	}
	devs := res.Data["devices"].([]types.DeviceInfo)
	if len(devs) != 1 || devs[0].Serial != "emulator-5554" {
		t.Errorf("expected 1 emulator device, got: %+v", devs)
	}

	// Test DeviceListTool error path
	errDM := &mockDeviceManager{
		listDevicesFn: func(includeAVDs bool) ([]types.DeviceInfo, error) {
			return nil, errors.New("adb connection failure")
		},
	}
	listToolErr := NewDeviceListTool(errDM)
	resErr, _ := listToolErr.Execute(ctx, nil)
	if resErr.Success {
		t.Error("expected failure on manager error")
	}

	// Test tool missing args
	bootTool := NewEmulatorBootSnapshotTool(mockDM)
	res, _ = bootTool.Execute(ctx, map[string]any{"avd_name": ""})
	if res.Success {
		t.Error("expected failure on missing avd_name")
	}

	resetTool := NewDeviceResetStateTool(mockDM)
	res, _ = resetTool.Execute(ctx, map[string]any{"serial": ""})
	if res.Success {
		t.Error("expected failure on missing serial")
	}

	launchTool := NewAppInstallAndLaunchTool(mockDM)
	res, _ = launchTool.Execute(ctx, map[string]any{"serial": ""})
	if res.Success {
		t.Error("expected failure on missing serial")
	}
}

func TestRedroidAdapterMethods(t *testing.T) {
	adb := NewADB("mock-adb")
	mock := &mockRunner{
		runFn: func(ctx context.Context, bin string, args []string) (string, string, error) {
			joined := strings.Join(args, " ")
			if strings.Contains(joined, "connect 127.0.0.1:5555") {
				return "connected to 127.0.0.1:5555\n", "", nil
			}
			if strings.Contains(joined, "disconnect") {
				return "disconnected\n", "", nil
			}
			return "OK\n", "", nil
		},
	}
	adb.SetRunner(mock)

	adapter := NewRedroidAdapter(adb)
	ctx := context.Background()

	// 1. ConnectADB
	out, err := adapter.ConnectADB(ctx, 5555)
	if err != nil || !strings.Contains(out, "connected") {
		t.Errorf("ConnectADB failed: %v, out=%s", err, out)
	}
	if _, err := adapter.ConnectADB(ctx, -1); err == nil {
		t.Error("expected error on negative port, got nil")
	}

	// 2. DisconnectADB
	if err := adapter.DisconnectADB(ctx, "127.0.0.1:5555"); err != nil {
		t.Errorf("DisconnectADB failed: %v", err)
	}
	if err := adapter.DisconnectADB(ctx, ""); err == nil {
		t.Error("expected error on empty serial, got nil")
	}

	// 3. Stop
	if err := adapter.Stop(ctx, "test-container"); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
	if err := adapter.Stop(ctx, ""); err == nil {
		t.Error("expected error on empty containerName, got nil")
	}
}
