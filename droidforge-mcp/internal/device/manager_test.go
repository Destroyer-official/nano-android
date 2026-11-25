package device

import (
	"context"
	"math"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

func TestParseADBDevicesList(t *testing.T) {
	sampleOutput := `List of devices attached
adb-RZCX80CEE7M-xUSPIh._adb-tls-connect._tcp device product:m15xnndins model:SM_E156B device:m15x transport_id:1
emulator-5554          device product:sdk_gphone64_x86_64 model:sdk_gphone64_x86_64 device:emu64x transport_id:2
192.168.1.150:5555     unauthorized transport_id:3
emulator-5556          offline transport_id:4
`

	devices, err := ParseADBDevicesList(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected error parsing devices: %v", err)
	}

	if len(devices) != 4 {
		t.Fatalf("expected 4 devices, got %d", len(devices))
	}

	// Device 0: Physical phone over TLS
	d0 := devices[0]
	if d0.Serial != "adb-RZCX80CEE7M-xUSPIh._adb-tls-connect._tcp" {
		t.Errorf("d0 serial: expected TLS serial, got %q", d0.Serial)
	}
	if d0.State != "device" {
		t.Errorf("d0 state: expected 'device', got %q", d0.State)
	}
	if d0.Model != "SM_E156B" {
		t.Errorf("d0 model: expected 'SM_E156B', got %q", d0.Model)
	}
	if d0.IsEmulator {
		t.Errorf("d0 should not be marked as emulator")
	}
	if d0.ApprovedClass != "dedicated_test" {
		t.Errorf("d0 approved class: expected 'dedicated_test', got %q", d0.ApprovedClass)
	}

	// Device 1: Running AVD Emulator
	d1 := devices[1]
	if d1.Serial != "emulator-5554" {
		t.Errorf("d1 serial: expected 'emulator-5554', got %q", d1.Serial)
	}
	if d1.State != "device" {
		t.Errorf("d1 state: expected 'device', got %q", d1.State)
	}
	if !d1.IsEmulator {
		t.Errorf("d1 should be marked as emulator")
	}
	if d1.ApprovedClass != "emulator" {
		t.Errorf("d1 approved class: expected 'emulator', got %q", d1.ApprovedClass)
	}

	// Device 2: Unauthorized device
	d2 := devices[2]
	if d2.State != "unauthorized" {
		t.Errorf("d2 state: expected 'unauthorized', got %q", d2.State)
	}

	// Device 3: Offline emulator
	d3 := devices[3]
	if d3.Serial != "emulator-5556" || d3.State != "offline" {
		t.Errorf("d3: unexpected serial %s or state %s", d3.Serial, d3.State)
	}
	if !d3.IsEmulator {
		t.Errorf("d3 should be recognized as emulator")
	}
}

func TestParseBatteryDump(t *testing.T) {
	batteryOutput := `Current Battery Service state:
  AC powered: false
  USB powered: true
  status: 2
  health: 2
  present: true
  level: 82
  scale: 100
  voltage: 4120
  temperature: 285
  technology: Li-ion
`
	lvl, temp := parseBatteryDump(batteryOutput)
	if lvl != 82 {
		t.Fatalf("expected battery level 82, got %d", lvl)
	}
	if math.Abs(temp-28.5) > 0.01 {
		t.Fatalf("expected temperature 28.5 C, got %f", temp)
	}

	// Test malformed / empty output
	lvlEmpty, tempEmpty := parseBatteryDump("Some random text without battery headers")
	if lvlEmpty != -1 {
		t.Errorf("expected level -1 on empty, got %d", lvlEmpty)
	}
	if tempEmpty != 0.0 {
		t.Errorf("expected temp 0.0 on empty, got %f", tempEmpty)
	}
}

func TestValidatePackageForMutation(t *testing.T) {
	m := NewDeviceManager()

	// 1. Valid application packages
	validPkgs := []string{
		"com.example.app",
		"org.droidforge.harness",
		"com.android.sample",
	}
	for _, pkg := range validPkgs {
		if err := m.validatePackageForMutation(pkg); err != nil {
			t.Errorf("expected package %q to be valid, got: %v", pkg, err)
		}
	}

	// 2. Protected system packages must be rejected
	systemPkgs := []string{
		"android",
		"com.android.systemui",
		"com.android.settings",
		"com.google.android.gms",
	}
	for _, pkg := range systemPkgs {
		if err := m.validatePackageForMutation(pkg); err == nil {
			t.Errorf("expected security rejection for system package %q, got nil", pkg)
		}
	}

	// 3. Invalid syntax / injection attacks
	invalidPkgs := []string{
		"",
		"   ",
		"com.example; rm -rf /",
		"com.example/../hack",
		"com.example$(whoami)",
		"singleword",
	}
	for _, pkg := range invalidPkgs {
		if err := m.validatePackageForMutation(pkg); err == nil {
			t.Errorf("expected syntax rejection for invalid package %q, got nil", pkg)
		}
	}

	// 4. Allowlist scope lock enforcement
	mWithScope := NewDeviceManager(WithAllowlistedPackages([]string{"com.example.allowed"}))
	if err := mWithScope.validatePackageForMutation("com.example.allowed"); err != nil {
		t.Errorf("expected allowed package to pass, got: %v", err)
	}
	if err := mWithScope.validatePackageForMutation("com.example.other"); err == nil {
		t.Errorf("expected out-of-scope package to be rejected, got nil")
	}
}

func TestManagerValidationEdgeCases(t *testing.T) {
	m := NewDeviceManager()

	// 1. BootSnapshot invalid AVD names
	badAVDNames := []string{"", "   ", "avd/name", "avd\\name", "../avd", "avd.."}
	for _, name := range badAVDNames {
		if _, err := m.BootSnapshot(name, "clean", true, 2048); err == nil {
			t.Errorf("BootSnapshot(%q): expected validation error, got nil", name)
		}
	}

	// 2. ResetState invalid parameters
	if err := m.ResetState("", "pm_clear", "com.example.app", ""); err == nil {
		t.Error("ResetState with empty serial: expected error, got nil")
	}
	if err := m.ResetState("serial", "invalid_strategy", "com.example.app", ""); err == nil {
		t.Error("ResetState with invalid strategy: expected error, got nil")
	}
	if err := m.ResetState("serial", "snapshot_restore", "com.example.app", ""); err == nil {
		t.Error("ResetState snapshot_restore with empty snapshot: expected error, got nil")
	}
	if err := m.ResetState("serial", "pm_clear", "", ""); err == nil {
		t.Error("ResetState pm_clear with empty package: expected error, got nil")
	}

	// 3. InstallAndLaunch invalid parameters
	if _, err := m.InstallAndLaunch("", "app.apk", "com.example/.MainActivity", "", false); err == nil {
		t.Error("InstallAndLaunch empty serial: expected error, got nil")
	}
	if _, err := m.InstallAndLaunch("serial", "", "com.example/.MainActivity", "", false); err == nil {
		t.Error("InstallAndLaunch empty apkPath: expected error, got nil")
	}
	if _, err := m.InstallAndLaunch("serial", "app.apk", "", "", false); err == nil {
		t.Error("InstallAndLaunch empty activity and deepLink: expected error, got nil")
	}
}

func TestExtractPackageFromActivity(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"com.example.app/.MainActivity", "com.example.app"},
		{"com.example.app/com.example.app.ui.HomeActivity", "com.example.app"},
		{"org.droidforge.sample/org.droidforge.sample.MainActivity", "org.droidforge.sample"},
		{"com.example.app", "com.example.app"},
		{"invalidActivityNoPackage", ""},
	}

	for _, tt := range tests {
		got := extractPackageFromActivity(tt.input)
		if got != tt.expected {
			t.Errorf("extractPackageFromActivity(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestParseColdStartTime(t *testing.T) {
	amStartOutput := `Starting: Intent { act=android.intent.action.MAIN cat=[android.intent.category.LAUNCHER] cmp=com.example/.MainActivity }
Status: ok
LaunchState: COLD
Activity: com.example/.MainActivity
TotalTime: 312
WaitTime: 325
Complete
`
	coldStart := parseColdStartTime(amStartOutput)
	if coldStart != 312 {
		t.Fatalf("expected TotalTime 312, got %d", coldStart)
	}

	fallbackOutput := `Starting: Intent { cmp=com.example/.MainActivity }
WaitTime: 180
`
	coldStartFallback := parseColdStartTime(fallbackOutput)
	if coldStartFallback != 180 {
		t.Fatalf("expected WaitTime 180, got %d", coldStartFallback)
	}
}

func TestIsEmulatorDevice(t *testing.T) {
	if !isEmulatorDevice("emulator-5554", "sdk_gphone64_x86_64", "sdk_gphone64_x86_64") {
		t.Error("expected emulator-5554 to be recognized as emulator")
	}
	if !isEmulatorDevice("127.0.0.1:5555", "Android SDK built for x86", "sdk_x86") {
		t.Error("expected 127.0.0.1:5555 to be recognized as emulator")
	}
	if isEmulatorDevice("RF8M10ABCD", "Pixel 7 Pro", "cheetah") {
		t.Error("physical Pixel 7 Pro should not be recognized as emulator")
	}
}

// MockDeviceManager for ToolHandler unit tests
type mockDeviceManager struct {
	types.DeviceManager
	listDevicesFn      func(includeAVDs bool) ([]types.DeviceInfo, error)
	bootSnapshotFn     func(avdName, snapshot string, headless bool, ramMB int) (*types.DeviceInfo, error)
	resetStateFn       func(serial, strategy, pkgID, snapshotName string) error
	installAndLaunchFn func(serial, apkPath, activity, deepLink string, grantPerms bool) (*types.AppLaunchResult, error)
}

func (m *mockDeviceManager) ListDevices(includeAVDs bool) ([]types.DeviceInfo, error) {
	if m.listDevicesFn != nil {
		return m.listDevicesFn(includeAVDs)
	}
	return nil, nil
}

func (m *mockDeviceManager) BootSnapshot(avdName, snapshot string, headless bool, ramMB int) (*types.DeviceInfo, error) {
	if m.bootSnapshotFn != nil {
		return m.bootSnapshotFn(avdName, snapshot, headless, ramMB)
	}
	return &types.DeviceInfo{Serial: "emulator-5554", State: "device", APILevel: 37, Model: avdName, IsEmulator: true}, nil
}

func (m *mockDeviceManager) ResetState(serial, strategy, pkgID, snapshotName string) error {
	if m.resetStateFn != nil {
		return m.resetStateFn(serial, strategy, pkgID, snapshotName)
	}
	return nil
}

func (m *mockDeviceManager) InstallAndLaunch(serial, apkPath, activity, deepLink string, grantPerms bool) (*types.AppLaunchResult, error) {
	if m.installAndLaunchFn != nil {
		return m.installAndLaunchFn(serial, apkPath, activity, deepLink, grantPerms)
	}
	return &types.AppLaunchResult{
		PackageID:   "com.example.app",
		PID:         12345,
		InstallMS:   1500,
		ColdStartMS: 280,
		Activity:    activity,
	}, nil
}

func TestDeviceTools(t *testing.T) {
	mockDM := &mockDeviceManager{
		listDevicesFn: func(includeAVDs bool) ([]types.DeviceInfo, error) {
			return []types.DeviceInfo{
				{Serial: "emulator-5554", State: "device", Model: "Pixel_7", IsEmulator: true, APILevel: 34},
				{Serial: "physical-usb", State: "device", Model: "Galaxy_S24", IsEmulator: false, APILevel: 34},
			}, nil
		},
	}

	ctx := context.Background()

	// 1. Test DeviceListTool
	listTool := NewDeviceListTool(mockDM)
	def := listTool.Definition()
	if def.Name != "device_list" || def.RiskTier != types.RiskREAD {
		t.Fatalf("unexpected definition for device_list: %+v", def)
	}

	res, err := listTool.Execute(ctx, map[string]any{"include_avds": true})
	if err != nil || !res.Success {
		t.Fatalf("listTool execute failed: %v, res: %+v", err, res)
	}
	if count, ok := res.Data["count"].(int); !ok || count != 2 {
		t.Fatalf("expected count 2, got %v", res.Data["count"])
	}

	// 2. Test EmulatorBootSnapshotTool
	bootTool := NewEmulatorBootSnapshotTool(mockDM)
	bootDef := bootTool.Definition()
	if bootDef.Name != "emulator_boot_snapshot" || bootDef.RiskTier != types.RiskWRITESCOPED {
		t.Fatalf("unexpected definition for emulator_boot_snapshot: %+v", bootDef)
	}

	bootRes, err := bootTool.Execute(ctx, map[string]any{
		"avd_name":      "api37_clean",
		"snapshot_name": "clean_state",
	})
	if err != nil || !bootRes.Success {
		t.Fatalf("bootTool execute failed: %v, res: %+v", err, bootRes)
	}
	if bootRes.Data["serial"] != "emulator-5554" {
		t.Fatalf("expected serial 'emulator-5554', got %v", bootRes.Data["serial"])
	}

	// 3. Test DeviceResetStateTool
	resetTool := NewDeviceResetStateTool(mockDM)
	resetDef := resetTool.Definition()
	if resetDef.Name != "device_reset_state" || resetDef.RiskTier != types.RiskWRITESCOPED {
		t.Fatalf("unexpected definition for device_reset_state: %+v", resetDef)
	}

	resetRes, err := resetTool.Execute(ctx, map[string]any{
		"serial":     "emulator-5554",
		"strategy":   "pm_clear",
		"package_id": "com.example.app",
	})
	if err != nil || !resetRes.Success {
		t.Fatalf("resetTool execute failed: %v, res: %+v", err, resetRes)
	}

	// 4. Test AppInstallAndLaunchTool
	launchTool := NewAppInstallAndLaunchTool(mockDM)
	launchDef := launchTool.Definition()
	if launchDef.Name != "app_install_and_launch" || launchDef.RiskTier != types.RiskWRITESCOPED {
		t.Fatalf("unexpected definition for app_install_and_launch: %+v", launchDef)
	}

	launchRes, err := launchTool.Execute(ctx, map[string]any{
		"serial":   "emulator-5554",
		"apk_path": "/tmp/test.apk",
		"activity": "com.example.app/.MainActivity",
	})
	if err != nil || !launchRes.Success {
		t.Fatalf("launchTool execute failed: %v, res: %+v", err, launchRes)
	}
	if launchRes.Data["pid"] != 12345 {
		t.Fatalf("expected pid 12345, got %v", launchRes.Data["pid"])
	}
}

func TestLiveManagerListDevices(t *testing.T) {
	// Tests manager against real host ADB and connected device
	m := NewDeviceManager()
	devices, err := m.ListDevices(false)
	if err != nil {
		t.Skipf("skipping live manager test: %v", err)
	}
	t.Logf("Live manager discovered %d device(s)", len(devices))
	for _, d := range devices {
		t.Logf(" - Serial: %s, State: %s, Model: %s, API: %d, ABI: %s, Battery: %d%%, Temp: %.1f C, IsEmulator: %v",
			d.Serial, d.State, d.Model, d.APILevel, d.ABI, d.BatteryPct, d.TempC, d.IsEmulator)
		if d.State == "device" {
			if d.APILevel <= 0 {
				t.Errorf("expected positive API level for device %s, got %d", d.Serial, d.APILevel)
			}
		}
	}
}
