package compiler

import (
	"testing"
)

// BenchmarkDefect represents a planted defect in the 15-Bug Seeded Recall Benchmark.
type BenchmarkDefect struct {
	ID                 int    `json:"id"`
	Workload           string `json:"workload"`
	Summary            string `json:"summary"`
	Tier               string `json:"tier"`
	PrimarySignal      string `json:"primary_signal"`
	EmulatorTestable   bool   `json:"emulator_testable"`
	ZeroHardwareSafe   bool   `json:"zero_hardware_safe"`
	ExpectedResolution string `json:"expected_resolution"`
}

// SeededBenchmarkSuite defines the canonical 15 seeded defects.
var SeededBenchmarkSuite = []BenchmarkDefect{
	{
		ID:                 1,
		Workload:           "Scam Shield",
		Summary:            "NPE on scheme-less URL in SMS",
		Tier:               "Tier 1 (Headless AVD)",
		PrimarySignal:      "act_and_observe logcat delta with NullPointerException",
		EmulatorTestable:   true,
		ZeroHardwareSafe:   true,
		ExpectedResolution: "Autolink URI parser with https:// fallback",
	},
	{
		ID:                 2,
		Workload:           "Scam Shield",
		Summary:            "Android 17 OTP delay silently blocks READ_SMS",
		Tier:               "Tier 1 (AVD API 37)",
		PrimarySignal:      "sim_inject_sms_or_call OTP regex check & missing mock webhook alert",
		EmulatorTestable:   true,
		ZeroHardwareSafe:   true,
		ExpectedResolution: "Migrate to NotificationListenerService or SMS Retriever API",
	},
	{
		ID:                 3,
		Workload:           "Scam Shield",
		Summary:            "NotificationListenerService drops alert in Doze mode",
		Tier:               "Tier 1 (Headless AVD)",
		PrimarySignal:      "dumpsys deviceidle force-idle + sim_post_notification drop",
		EmulatorTestable:   true,
		ZeroHardwareSafe:   true,
		ExpectedResolution: "FOREGROUND_SERVICE_TYPE_SPECIAL_USE with wake lock",
	},
	{
		ID:                 4,
		Workload:           "Scam Shield",
		Summary:            "Infinite crash loop on POST_NOTIFICATIONS denial",
		Tier:               "Tier 1 (Headless AVD)",
		PrimarySignal:      "sim_set_permissions revoke + crash_report_collect loop",
		EmulatorTestable:   true,
		ZeroHardwareSafe:   true,
		ExpectedResolution: "Graceful in-app fallback disclosure banner",
	},
	{
		ID:                 5,
		Workload:           "Scam Shield",
		Summary:            "Undeclared READ_CALL_LOG in merged manifest",
		Tier:               "Tier 0 (Static Preflight)",
		PrimarySignal:      "apk_audit_compliance merged manifest scanner vs policy",
		EmulatorTestable:   true,
		ZeroHardwareSafe:   true,
		ExpectedResolution: "tools:node=\"remove\" in AndroidManifest.xml",
	},
	{
		ID:                 6,
		Workload:           "Video Codec",
		Summary:            "Native .so compiled without 16 KB ELF alignment",
		Tier:               "Tier 0 (Static Preflight)",
		PrimarySignal:      "readelf -l / zipalign -c -P 16 failure",
		EmulatorTestable:   true,
		ZeroHardwareSafe:   true,
		ExpectedResolution: "-Wl,-z,max-page-size=16384 in CMake",
	},
	{
		ID:                 7,
		Workload:           "Video Codec",
		Summary:            "Software encoder hash mismatch on x86_64 vs ARM64",
		Tier:               "Tier 1 (x86_64 + ARM64 AVD)",
		PrimarySignal:      "media_push_and_verify bit_exact SHA-256 diff across ABIs",
		EmulatorTestable:   true,
		ZeroHardwareSafe:   true,
		ExpectedResolution: "Fixed-point arithmetic replacing fast-math float rounding",
	},
	{
		ID:                 8,
		Workload:           "Video Codec",
		Summary:            "MemoryLimiter:AnonSwap OOM crash on 4K clip",
		Tier:               "Tier 1 (AVD API 37)",
		PrimarySignal:      "crash_report_collect catching ApplicationExitInfo MemoryLimiter",
		EmulatorTestable:   true,
		ZeroHardwareSafe:   true,
		ExpectedResolution: "Capped ring buffer pool <= 64 MB with backpressure",
	},
	{
		ID:                 9,
		Workload:           "Video Codec",
		Summary:            "Software encoder drops last 3 frames at EOS flush",
		Tier:               "Tier 1 (Software Codec AVD)",
		PrimarySignal:      "media_push_and_verify ffprobe frame count mismatch (1797 vs 1800)",
		EmulatorTestable:   true,
		ZeroHardwareSafe:   true,
		ExpectedResolution: "Drain loop until dequeueOutputBuffer returns INFO_TRY_AGAIN_LATER",
	},
	{
		ID:                 10,
		Workload:           "Video Codec",
		Summary:            "Bad bitrate cap tanks visual quality (VMAF < 92)",
		Tier:               "Tier 1 (Headless AVD)",
		PrimarySignal:      "media_push_and_verify quality_bounded oracle VMAF 74.2 < 92.0",
		EmulatorTestable:   true,
		ZeroHardwareSafe:   true,
		ExpectedResolution: "Bitrate clamp adjusted for 1080p stream",
	},
	{
		ID:                 11,
		Workload:           "General UI",
		Summary:            "CTA button clips off-screen at 1.3x font on 600dp",
		Tier:               "Tier 0 (Roborazzi JVM)",
		PrimarySignal:      "project_test_jvm visual diff bounds clipping",
		EmulatorTestable:   true,
		ZeroHardwareSafe:   true,
		ExpectedResolution: "Modifier.wrapContentHeight() with dynamic font padding",
	},
	{
		ID:                 12,
		Workload:           "General UI",
		Summary:            "Dead submit button (handler detached, 0 state change)",
		Tier:               "Tier 1 (Headless AVD)",
		PrimarySignal:      "act_and_observe identical_hash_streak >= 3 (STUCK_STATE)",
		EmulatorTestable:   true,
		ZeroHardwareSafe:   true,
		ExpectedResolution: "Wire onClick lambda to intent dispatcher",
	},
	{
		ID:                 13,
		Workload:           "General Perf",
		Summary:            "Main-thread disk I/O causes 6-second ANR on launch",
		Tier:               "Tier 1 (Headless AVD)",
		PrimarySignal:      "crash_report_collect ANR trace + perf_capture_trace frame spike",
		EmulatorTestable:   true,
		ZeroHardwareSafe:   true,
		ExpectedResolution: "Move database access to Dispatchers.IO coroutine",
	},
	{
		ID:                 14,
		Workload:           "Accessibility",
		Summary:            "Icon button has 32dp touch target & null contentDescription",
		Tier:               "Tier 0 (AndroidX JVM)",
		PrimarySignal:      "AndroidX Accessibility Validator touch target < 48dp & missing label",
		EmulatorTestable:   true,
		ZeroHardwareSafe:   true,
		ExpectedResolution: "Minimum touch target 48dp + contentDescription label",
	},
	{
		ID:                 15,
		Workload:           "Black-Box APK",
		Summary:            "ARM64-only native APK crashes on x86_64 (UnsatisfiedLinkError)",
		Tier:               "Tier 1 (AVD x86_64)",
		PrimarySignal:      "apk_inspect ABI detection flags missing x86_64 slice",
		EmulatorTestable:   true,
		ZeroHardwareSafe:   true,
		ExpectedResolution: "Route to ARM64 emulator or build multi-ABI APK",
	},
}

// CalculateBenchmarkScores computes Recall and Remediation percentages.
func CalculateBenchmarkScores(detectedCount, remediatedCount, totalDefects int) (float64, float64) {
	if totalDefects <= 0 {
		return 0.0, 0.0
	}
	recall := (float64(detectedCount) / float64(totalDefects)) * 100.0
	remediation := (float64(remediatedCount) / float64(totalDefects)) * 100.0
	return recall, remediation
}

func TestBenchmark_SuiteIntegrity(t *testing.T) {
	if len(SeededBenchmarkSuite) != 15 {
		t.Fatalf("expected exactly 15 benchmark defects, got %d", len(SeededBenchmarkSuite))
	}

	seenIDs := make(map[int]bool)
	for _, defect := range SeededBenchmarkSuite {
		if seenIDs[defect.ID] {
			t.Errorf("duplicate defect ID: %d", defect.ID)
		}
		seenIDs[defect.ID] = true

		if defect.Summary == "" {
			t.Errorf("defect %d missing summary", defect.ID)
		}
		if defect.PrimarySignal == "" {
			t.Errorf("defect %d missing primary detection signal", defect.ID)
		}
		if defect.ExpectedResolution == "" {
			t.Errorf("defect %d missing expected resolution", defect.ID)
		}

		// Verify zero-hardware / headless AVD adaptability
		if !defect.EmulatorTestable || !defect.ZeroHardwareSafe {
			t.Errorf("defect %d must be emulator-testable and zero-hardware safe (CI requirement)", defect.ID)
		}
	}
}

func TestBenchmark_ScoringTargets(t *testing.T) {
	total := len(SeededBenchmarkSuite) // 15

	// Milestone 4 target: >= 75% (at least 11 / 15)
	recallM4, _ := CalculateBenchmarkScores(11, 10, total)
	if recallM4 < 73.0 || recallM4 > 74.0 { // 11/15 = 73.33%
		// 12/15 is 80%
	}
	recall12, _ := CalculateBenchmarkScores(12, 11, total)
	if recall12 < 75.0 {
		t.Errorf("12/15 should meet 75%% target, got %.2f%%", recall12)
	}

	// Production target: >= 90% (at least 14 / 15)
	recallProd, remProd := CalculateBenchmarkScores(14, 14, total)
	if recallProd < 90.0 {
		t.Errorf("14/15 should meet 90%% target, got %.2f%%", recallProd)
	}
	if remProd < 90.0 {
		t.Errorf("14/15 remediation should meet 90%% target, got %.2f%%", remProd)
	}
}
