package profiler_test

import (
	"strings"
	"testing"

	"nano_android/engine/mcp_compiler"
	"nano_android/engine/profiler/crash"
	"nano_android/engine/profiler/device"
	"nano_android/engine/profiler/jvm"
	"nano_android/engine/profiler/timeline"
)

func TestR8RetraceAndCrashClustering(t *testing.T) {
	mappingTxt := `com.prismv.ui.MainActivity -> a.b.c:
    12:18:void onCreate(android.os.Bundle):104:110 -> a
    19:24:void switchTab(int):240:245 -> b
`
	mapping := crash.ParseR8Mapping(mappingTxt)

	raw1 := "FATAL EXCEPTION: main\n" +
		"java.lang.IllegalStateException: Token Bearer super-secret-jwt-123456 failed at @1a2b3c4\n" +
		"\tat a.b.c.b(SourceFile:21)\n" +
		"Caused by: java.lang.NullPointerException: null ref"

	raw2 := "FATAL EXCEPTION: main\n" +
		"java.lang.IllegalStateException: Token Bearer other-secret-9999 failed at @99ff001\n" +
		"\tat a.b.c.b(SourceFile:21)\n" +
		"Caused by: java.lang.NullPointerException: null ref"

	rep1 := crash.AnalyzeTrace(raw1, "v1.0", "nano_jvm", mapping)
	rep2 := crash.AnalyzeTrace(raw2, "v1.1", "nano_jvm", mapping)

	if !rep1.Retraced {
		t.Fatal("expected R8 retrace to transform obfuscated frame")
	}
	if !strings.Contains(rep1.RetracedTrace, "com.prismv.ui.MainActivity.switchTab(MainActivity.java:242)") {
		t.Fatalf("unexpected retraced frame: %s", rep1.RetracedTrace)
	}
	if strings.Contains(rep1.RetracedTrace, "super-secret-jwt-123456") {
		t.Fatal("secret was not redacted in crash trace")
	}
	if rep1.Signature != rep2.Signature {
		t.Fatalf("expected identical cluster signature for same normalized crash (%s != %s)", rep1.Signature, rep2.Signature)
	}

	clusters := crash.ClusterCrashes([]crash.CrashReport{rep1, rep2})
	if len(clusters) != 1 || clusters[0].Count != 2 {
		t.Fatalf("expected 1 cluster with count 2, got %+v", clusters)
	}

	diff := crash.CompareClusters(clusters, nil)
	if len(diff.ResolvedSignatures) != 1 {
		t.Fatalf("expected resolved signature in cluster diff, got %+v", diff)
	}
}

func TestLiveJVMProfilerAndForensicTimeline(t *testing.T) {
	bridge := mcp_compiler.NewLiveHeadlessBridge()
	defer bridge.Close()

	if _, err := bridge.Launch("../../tools/prism-v.jar", "com.prismv.ui.MainActivity"); err != nil {
		t.Fatalf("Launch failed: %v", err)
	}
	if _, err := bridge.DispatchAction("click", "Duo", ""); err != nil {
		t.Fatalf("Click failed: %v", err)
	}
	if _, err := bridge.StepTime(300); err != nil {
		t.Fatalf("StepTime failed: %v", err)
	}

	snap, err := jvm.CaptureSnapshot(bridge, jvm.QueryTiming{Operation: "sqlite.query", Target: "sessions", DurationMs: 0.42})
	if err != nil {
		t.Fatalf("CaptureSnapshot failed: %v", err)
	}
	if snap.Backend != "nano_jvm" {
		t.Fatalf("expected nano_jvm backend disclosure, got %s", snap.Backend)
	}
	if snap.Metrics.ThreadCount <= 0 || snap.Threads == nil || len(snap.Threads.Threads) == 0 {
		t.Fatalf("expected live JVM thread inspection, got metrics=%+v threads=%+v", snap.Metrics, snap.Threads)
	}
	if snap.Metrics.DeadlockedThreads != 0 {
		t.Fatalf("unexpected deadlocks: %d", snap.Metrics.DeadlockedThreads)
	}
	if len(snap.Timeline) < 3 {
		t.Fatalf("expected >= 3 timeline events (APK_LAUNCHED, ACTIVITY_CREATED, ACTION_DISPATCHED), got %d", len(snap.Timeline))
	}
}

func TestDeviceProfilerParsers(t *testing.T) {
	thermal := "mThermalStatus: MODERATE\nTemperature{mValue=41.5, mType=3, mName=skin, mStatus=2}\nTemperature{mValue=37.2, mType=2, mName=battery, mStatus=1}"
	mem := "TOTAL PSS: 102400  TOTAL RSS: 163840  TOTAL SWAP PSS: 4096\nMemoryLimiter:AnonSwap killed pid 1234"
	bat := "level: 82\nODPM Rail energy: CPU_BIG=3600000uWs GPU=1800000uWs"
	gfx := "Total frames rendered: 100\nJanky frames: 4\n95th percentile: 14.2ms"
	amStart := "LaunchState: COLD\nActivity: com.example/.MainActivity\nThisTime: 412\nTotalTime: 412\nWaitTime: 425"

	rep := device.ParseDumpsysBundle("physical_device", "FA123", "com.example", thermal, mem, bat, gfx, amStart)
	if rep.ThermalStatus != "MODERATE" || rep.SkinTempC != 41.5 || rep.TotalPSSMB != 100 || rep.MemoryLimiterEvents != 1 {
		t.Fatalf("unexpected parsed device report: %+v", rep)
	}
	if rep.ODPMRailsMWh["CPU_BIG"] != 1.0 || rep.Startup == nil || rep.Startup.TotalTimeMs != 412 {
		t.Fatalf("unexpected ODPM/startup parse: %+v", rep)
	}

	macroJSON := []byte(`{"benchmarks":[{"name":"startupCold","metrics":{"timeToInitialDisplayMs":{"median":415.5,"minimum":398.0,"maximum":440.1}}}]}`)
	macros, err := device.ParseMacrobenchmarkJSON(macroJSON)
	if err != nil || len(macros) != 1 || macros[0].Median != 415.5 {
		t.Fatalf("macrobenchmark parse failed: err=%v macros=%+v", err, macros)
	}
	_ = timeline.EventAPKLaunched
}
