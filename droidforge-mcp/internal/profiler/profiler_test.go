package profiler

import (
	"context"
	"strings"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// mockDeviceManager implements types.DeviceManager for profiler tests.
type mockDeviceManager struct {
	execADBOutputs        map[string]string
	executedADBCommands   [][]string
	emulatorConsoleOutput map[string]string
}

func newMockDeviceManager() *mockDeviceManager {
	return &mockDeviceManager{
		execADBOutputs:        make(map[string]string),
		executedADBCommands:   make([][]string, 0),
		emulatorConsoleOutput: make(map[string]string),
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
	return "OK", nil
}

// mockProfilerSidecar implements SidecarRunner for profiler tests.
type mockProfilerSidecar struct {
	result *types.PerfTraceResult
	err    error
}

func (m *mockProfilerSidecar) AnalyzeTrace(ctx context.Context, tracePath, pkg, outPath string) (*types.PerfTraceResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.result != nil {
		return m.result, nil
	}
	return &types.PerfTraceResult{
		TracePath:           tracePath,
		P95FrameMS:          14.2,
		JankPct:             1.5,
		PeakRSSMB:           112.5,
		PeakAnonSwapMB:      10.2,
		MemoryLimiterEvents: 0,
	}, nil
}

// TestGeneratePerfettoConfig tests perfetto textproto configuration generator.
func TestGeneratePerfettoConfig(t *testing.T) {
	cfg := GeneratePerfettoConfig([]string{"sched", "freq", "mem", "gfx"}, "com.example.app", 15)

	if !strings.Contains(cfg, "sched/sched_switch") {
		t.Error("expected sched/sched_switch in ftrace events")
	}
	if !strings.Contains(cfg, "power/cpu_frequency") {
		t.Error("expected power/cpu_frequency in ftrace events")
	}
	if !strings.Contains(cfg, "atrace_apps: \"com.example.app\"") {
		t.Error("expected atrace_apps to contain com.example.app")
	}
	if !strings.Contains(cfg, "duration_ms: 15000") {
		t.Error("expected duration_ms: 15000")
	}
}

// TestParseThermalService tests dumpsys thermalservice output parsing.
func TestParseThermalService(t *testing.T) {
	rawDumpsys := `
Current Thermal Status: 0
mThermalStatus=0
Current temperatures:
    Temperature{mValue=33.5, mType=3, mName=skin, mStatus=0}
    Temperature{mValue=31.2, mType=2, mName=battery, mStatus=0}
    Temperature{mValue=42.0, mType=0, mName=cpu-0-0, mStatus=0}
`
	status, skin, bat := ParseThermalService(rawDumpsys)

	if status != "NONE" {
		t.Errorf("expected status NONE, got %s", status)
	}
	if skin != 33.5 {
		t.Errorf("expected skin temp 33.5, got %v", skin)
	}
	if bat != 31.2 {
		t.Errorf("expected battery temp 31.2, got %v", bat)
	}
}

// TestParseMeminfo tests dumpsys meminfo output parsing.
func TestParseMeminfo(t *testing.T) {
	rawDumpsys := `
Applications Memory Usage (in Kilobytes):
Uptime: 123456 Realtime: 123456

** MEMINFO in pid 2456 [com.example.video.codec] **
                   Pss  Private  Private  SwapPss      Rss
                 Total    Dirty    Clean    Dirty    Total
                ------   ------   ------   ------   ------
  Native Heap    45000    44000        0     2048    50000
       Dalvik    25000    24000        0     1024    30000
        TOTAL   128000   110000        0     8192   160000

 TOTAL PSS:   128000            TOTAL RSS:   160000      TOTAL SWAP PSS:     8192
`
	pss, rss, swap := ParseMeminfo(rawDumpsys)

	expectedPSS := 128000.0 / 1024.0
	expectedRSS := 160000.0 / 1024.0
	expectedSwap := 8192.0 / 1024.0

	if pss != expectedPSS {
		t.Errorf("expected PSS %v MB, got %v MB", expectedPSS, pss)
	}
	if rss != expectedRSS {
		t.Errorf("expected RSS %v MB, got %v MB", expectedRSS, rss)
	}
	if swap != expectedSwap {
		t.Errorf("expected Swap %v MB, got %v MB", expectedSwap, swap)
	}
}

// TestParseBatteryStats tests dumpsys batterystats parsing.
func TestParseBatteryStats(t *testing.T) {
	rawDumpsys := `
Battery History:
Statistics since last charge:
  System starts: 0, currently on battery: true
  Charge counter: 3450000 µAh
  level: 82
  temperature: 315
`
	charge, level, temp := ParseBatteryStats(rawDumpsys)

	if level != 82 {
		t.Errorf("expected level 82, got %d", level)
	}
	if temp != 31.5 {
		t.Errorf("expected temp 31.5 C, got %v", temp)
	}
	if charge != 3450.0 {
		t.Errorf("expected charge 3450 mAh, got %v", charge)
	}
}

// TestParseGfxInfo tests dumpsys gfxinfo parsing.
func TestParseGfxInfo(t *testing.T) {
	rawDumpsys := `
Applications Graphics Acceleration Info:
Uptime: 100000 Realtime: 100000

** GRAPHICS INFO FOR com.example.app [2456] **
Total frames rendered: 1450
Janky frames: 29 (2.00%)
50th percentile: 8ms
90th percentile: 12ms
95th percentile: 14.5ms
99th percentile: 18ms
Number Missed Vsync: 5
`
	total, janky, p95 := ParseGfxInfo(rawDumpsys)

	if total != 1450 {
		t.Errorf("expected total frames 1450, got %d", total)
	}
	if janky != 29 {
		t.Errorf("expected janky frames 29, got %d", janky)
	}
	if p95 != 14.5 {
		t.Errorf("expected p95 14.5ms, got %v", p95)
	}
}

// TestLogcatReadFilteringAndNonce tests logcat reading, level filter, and nonce fencing.
func TestLogcatReadFilteringAndNonce(t *testing.T) {
	ctx := context.Background()
	dm := newMockDeviceManager()

	rawLogcat := `
--------- beginning of main
11-20 20:01:00.100  1000  1000 D TestTag  : Debug line from PID 1000
11-20 20:01:00.200  2000  2000 I OtherTag : Info line from PID 2000
11-20 20:01:00.300  1000  1000 W TestTag  : Warning line from PID 1000
11-20 20:01:00.400  1000  1000 E TestTag  : Error line from PID 1000
`
	dm.execADBOutputs["logcat -d -v threadtime"] = rawLogcat

	req := LogcatReadRequest{
		Serial:   "emulator-5554",
		MinLevel: "W",
		Tags:     []string{"TestTag"},
		MaxLines: 50,
	}

	res, err := ReadLogcat(ctx, dm, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Nonce == "" {
		t.Error("expected non-empty cryptographic nonce")
	}

	// Should contain fence markers
	hasStartFence := false
	hasEndFence := false
	for _, l := range res.Lines {
		if strings.Contains(l, "<<<UNTRUSTED_DEVICE_DATA_NONCE:"+res.Nonce+">>>") {
			hasStartFence = true
		}
		if strings.Contains(l, "<<<END_UNTRUSTED_DEVICE_DATA_NONCE:"+res.Nonce+">>>") {
			hasEndFence = true
		}
	}

	if !hasStartFence || !hasEndFence {
		t.Error("expected lines to be fenced with nonce")
	}

	// Check that Debug line was filtered out by MinLevel=W
	for _, l := range res.Lines {
		if strings.Contains(l, "Debug line") {
			t.Error("Debug line should have been filtered out")
		}
		if strings.Contains(l, "OtherTag") {
			t.Error("OtherTag line should have been filtered out")
		}
	}
}

// TestParseLogcatCrashesAndANR tests logcat crash and ANR extraction.
func TestParseLogcatCrashesAndANR(t *testing.T) {
	rawLogcat := `
11-20 20:05:10.123  2456  2456 E AndroidRuntime: FATAL EXCEPTION: main
11-20 20:05:10.123  2456  2456 E AndroidRuntime: Process: com.example.app, PID: 2456
11-20 20:05:10.123  2456  2456 E AndroidRuntime: java.lang.NullPointerException: Attempt to invoke virtual method on a null object reference
11-20 20:05:10.123  2456  2456 E AndroidRuntime: 	at com.example.app.MainActivity.onCreate(MainActivity.kt:42)
11-20 20:05:10.123  2456  2456 E AndroidRuntime: 	at android.app.Activity.performCreate(Activity.java:8000)
11-20 20:05:15.456  1000  1050 E ActivityManager: ANR in com.example.app (com.example.app/.MainActivity)
11-20 20:05:15.456  1000  1050 E ActivityManager: PID: 2456
11-20 20:05:15.456  1000  1050 E ActivityManager: Reason: Input dispatching timed out
11-20 20:05:15.456  1000  1050 E ActivityManager: Load: 1.5 / 2.0 / 1.8
`
	crashes, anrs := ParseLogcatCrashes(rawLogcat, "com.example.app")

	if len(crashes) != 1 {
		t.Fatalf("expected 1 crash, got %d", len(crashes))
	}
	if !strings.Contains(crashes[0].StackTrace, "MainActivity.onCreate") {
		t.Errorf("expected stack trace to contain MainActivity.onCreate, got: %s", crashes[0].StackTrace)
	}

	if len(anrs) != 1 {
		t.Fatalf("expected 1 ANR, got %d", len(anrs))
	}
	if !strings.Contains(anrs[0].Message, "Input dispatching timed out") {
		t.Errorf("expected ANR message to contain timeout reason, got: %s", anrs[0].Message)
	}
}

// TestParseExitInfoMemoryLimiter tests ApplicationExitInfo parser with Android 17 MemoryLimiter.
func TestParseExitInfoMemoryLimiter(t *testing.T) {
	rawExitInfo := `
ApplicationExitInfo(timestamp=1732100000000 pid=2456 realUid=10123 package=com.example.video.codec reason=3 (REASON_LOW_MEMORY) subreason=1 (SUBREASON_EXCESSIVE_RESOURCE_USAGE) status=0 importance=100 description=MemoryLimiter:AnonSwap exceeded 1400MB threshold)
ApplicationExitInfo(timestamp=1732100050000 pid=2457 realUid=10123 package=com.example.video.codec reason=4 (REASON_CRASH) subreason=0 status=0 importance=100 description=SIGSEGV)
`
	reasons := ParseExitInfo(rawExitInfo)

	if len(reasons) != 2 {
		t.Fatalf("expected 2 exit reasons, got %d", len(reasons))
	}

	// Should specifically flag MemoryLimiter:AnonSwap
	if reasons[0].Reason != "MemoryLimiter:AnonSwap" {
		t.Errorf("expected reason MemoryLimiter:AnonSwap, got %s", reasons[0].Reason)
	}
	if !strings.Contains(reasons[0].Description, "MemoryLimiter:AnonSwap exceeded") {
		t.Errorf("expected description to contain threshold info, got: %s", reasons[0].Description)
	}
}

// TestParseODPMRails tests parsing Pixel 6+ on-device power monitor rails.
func TestParseODPMRails(t *testing.T) {
	rawDumpsys := `
Estimated power use (mAh):
  Capacity: 4500, Computed drain: 120.5
Rail energy: S10M_VDD_CPUCL0=3600000uWs, S11M_VDD_CPUCL1=7200000uWs, S12M_VDD_TPU=1800000uWs
Energy counter: 123456
`
	rails := ParseODPMRails(rawDumpsys)

	if len(rails) != 3 {
		t.Fatalf("expected 3 ODPM rails, got %d: %v", len(rails), rails)
	}

	// 3,600,000 uWs = 1.0 mWh
	if cpu0, ok := rails["S10M_VDD_CPUCL0"]; !ok || cpu0 != 1.0 {
		t.Errorf("expected S10M_VDD_CPUCL0 = 1.0 mWh, got %v", cpu0)
	}
	// 7,200,000 uWs = 2.0 mWh
	if cpu1, ok := rails["S11M_VDD_CPUCL1"]; !ok || cpu1 != 2.0 {
		t.Errorf("expected S11M_VDD_CPUCL1 = 2.0 mWh, got %v", cpu1)
	}
}

// TestStripZeroWidthAndBidiProfiler tests stripping invisible prompt injection characters.
func TestStripZeroWidthAndBidiProfiler(t *testing.T) {
	// Text containing zero-width space (U+200B), BOM (U+FEFF), and RTL override (U+202E)
	adversarial := "NormalText\u200B\uFEFFWithHidden\u202EPayload"
	cleaned := StripZeroWidthAndBidi(adversarial)
	expected := "NormalTextWithHiddenPayload"
	if cleaned != expected {
		t.Errorf("expected %q, got %q", expected, cleaned)
	}
}

// TestFlagInjectionRejectionProfiler tests that dangerous flags in package_id are rejected.
func TestFlagInjectionRejectionProfiler(t *testing.T) {
	ctx := context.Background()
	dm := newMockDeviceManager()

	dangerousPkg := "--user 0 com.android.settings"

	_, err := InspectAppState(ctx, dm, AppStateRequest{
		Serial:    "emulator-5554",
		PackageID: dangerousPkg,
	})
	if err == nil {
		t.Fatal("expected error rejecting flag injection in InspectAppState, got nil")
	}

	_, err = CollectCrashReport(ctx, dm, CrashCollectRequest{
		Serial:    "emulator-5554",
		PackageID: dangerousPkg,
	})
	if err == nil {
		t.Fatal("expected error rejecting flag injection in CollectCrashReport, got nil")
	}
}

// TestInspectAppStateParsers tests dumpsys services, package permissions, and jobscheduler parsers.
func TestInspectAppStateParsers(t *testing.T) {
	servicesRaw := `
ACTIVITY MANAGER SERVICES (dumpsys activity services)
  * ServiceRecord{a1b2c3d u0 com.example.scamshield/.ScamDetectionService}
  * ServiceRecord{e4f5a6b u0 com.example.scamshield/.NotificationAlertService}
  * ServiceRecord{1234567 u0 com.other.app/.OtherService}
`
	services := ParseServices(servicesRaw, "com.example.scamshield")
	if len(services) != 2 {
		t.Fatalf("expected 2 services for com.example.scamshield, got %d: %v", len(services), services)
	}

	pkgRaw := `
Packages:
  Package [com.example.scamshield] (1a2b3c):
    runtime permissions:
      android.permission.RECEIVE_SMS: granted=true
      android.permission.READ_SMS: granted=false
      android.permission.POST_NOTIFICATIONS: granted=true
`
	granted, denied := ParsePermissions(pkgRaw)
	if len(granted) != 2 {
		t.Errorf("expected 2 granted permissions, got %d: %v", len(granted), granted)
	}
	if len(denied) != 1 {
		t.Errorf("expected 1 denied permission, got %d: %v", len(denied), denied)
	}

	jobRaw := `
Registered Jobs:
  JOB #101: com.example.scamshield/androidx.work.impl.background.systemjob.SystemJobService
    Ready=true satisfied=0x7
    tag=WorkManager_SyncAlerts
`
	jobs := ParseWorkManagerJobs(jobRaw, "com.example.scamshield")
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].ID != "101" {
		t.Errorf("expected job ID 101, got %s", jobs[0].ID)
	}
	if len(jobs[0].Tags) == 0 || jobs[0].Tags[0] != "WorkManager_SyncAlerts" {
		t.Errorf("expected tag WorkManager_SyncAlerts, got: %v", jobs[0].Tags)
	}
}
