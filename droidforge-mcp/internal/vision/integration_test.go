package vision

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// getConnectedDevice returns the serial of an approved connected test device.
// In automated package tests, it defaults to emulators (emulator-* or 127.0.0.1:*).
// Live physical or Wi-Fi devices can be targeted by setting DROIDFORGE_LIVE_DEVICE=1.
func getConnectedDevice() string {
	out, err := exec.Command(findADB(), "devices").Output()
	if err != nil {
		return ""
	}
	allowLive := os.Getenv("DROIDFORGE_LIVE_DEVICE") == "1"
	lines := strings.Split(string(out), "\n")
	for _, l := range lines[1:] {
		fields := strings.Fields(l)
		if len(fields) >= 2 && fields[1] == "device" {
			serial := fields[0]
			if allowLive || strings.HasPrefix(serial, "emulator-") || strings.HasPrefix(serial, "127.0.0.1:") {
				return serial
			}
		}
	}
	return ""
}

func TestLiveScrcpyDeployAndGrab10Frames(t *testing.T) {
	serial := getConnectedDevice()
	if serial == "" {
		t.Skip("no adb device available for live integration test")
	}
	t.Logf("Running live scrcpy test on device: %s", serial)

	// Cool-down and cleanup before starting test
	time.Sleep(500 * time.Millisecond)

	jarPath := findScrcpyJar()
	t.Logf("Using scrcpy jar: %s", jarPath)
	if _, err := os.Stat(jarPath); err != nil {
		t.Skipf("scrcpy-server jar not found at %s, skipping live test", jarPath)
	}

	backend := NewScrcpyBackend(ScrcpyConfig{
		JarPath: jarPath,
		Version: "5.0.1",
	})
	_ = backend.Teardown(serial)
	defer func() {
		_ = backend.Teardown(serial)
		time.Sleep(500 * time.Millisecond)
	}()

	err := backend.Deploy(serial)
	if err != nil {
		t.Fatalf("failed to deploy scrcpy-server: %v", err)
	}

	if !backend.IsStreaming(serial) {
		t.Fatalf("expected IsStreaming=true after deploy")
	}

	// Grab 10 frames and verify all decode to valid images
	var totalDuration time.Duration
	for i := 0; i < 10; i++ {
		start := time.Now()
		frame, err := backend.GrabFrame(serial, 720, "webp")
		elapsed := time.Since(start)
		totalDuration += elapsed

		if err != nil {
			t.Fatalf("GrabFrame #%d failed: %v", i+1, err)
		}
		if len(frame) == 0 {
			t.Fatalf("GrabFrame #%d returned 0 bytes", i+1)
		}

		img, format, err := DecodeImage(frame)
		if err != nil {
			t.Fatalf("failed to decode frame #%d: %v", i+1, err)
		}
		if format != "webp" {
			t.Errorf("expected webp, got %s", format)
		}

		t.Logf("Frame #%d: size=%d bytes, dimensions=%dx%d, latency=%v",
			i+1, len(frame), img.Bounds().Dx(), img.Bounds().Dy(), elapsed)
	}

	avgLatency := totalDuration / 10
	t.Logf("Average frame grab latency: %v (target <100ms)", avgLatency)
	if avgLatency > 150*time.Millisecond {
		t.Logf("Warning: average latency %v exceeds 100ms target", avgLatency)
	}
}

func TestLiveFallbackScreencap(t *testing.T) {
	serial := getConnectedDevice()
	if serial == "" {
		t.Skip("no adb device available for live integration test")
	}
	t.Logf("Running live fallback test on device: %s", serial)

	// Intentionally break scrcpy deploy with invalid jar path
	mgr := NewManager(ManagerConfig{
		ScrcpyJarPath: "invalid/path/nonexistent-scrcpy.jar",
	})
	defer mgr.Close()

	start := time.Now()
	frame, err := mgr.GrabFrame(serial, 720, "webp")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("GrabFrame fallback failed: %v", err)
	}
	if len(frame) == 0 {
		t.Fatal("GrabFrame returned 0 bytes")
	}

	img, format, err := DecodeImage(frame)
	if err != nil {
		t.Fatalf("failed to decode fallback frame: %v", err)
	}
	if format != "webp" {
		t.Errorf("expected format webp, got %s", format)
	}

	t.Logf("Fallback screencap succeeded in %v! Image dimensions: %dx%d, size: %d bytes",
		elapsed, img.Bounds().Dx(), img.Bounds().Dy(), len(frame))
}

func TestLiveBenchmarkScrcpyVsScreencap(t *testing.T) {
	serial := getConnectedDevice()
	if serial == "" {
		t.Skip("no adb device available for live benchmark")
	}
	t.Logf("Running benchmark scrcpy vs screencap on device: %s", serial)
	time.Sleep(1 * time.Second)

	// 1. Scrcpy Benchmark
	scrcpyBackend := NewScrcpyBackend(ScrcpyConfig{
		JarPath: findScrcpyJar(),
		Version: "5.0.1",
	})
	_ = scrcpyBackend.Teardown(serial)
	time.Sleep(500 * time.Millisecond)
	defer func() {
		_ = scrcpyBackend.Teardown(serial)
	}()

	err := scrcpyBackend.Deploy(serial)
	if err != nil {
		t.Fatalf("Scrcpy deploy failed: %v", err)
	}

	const samples = 20
	var scrcpyTotal time.Duration
	for i := 0; i < samples; i++ {
		t0 := time.Now()
		_, err := scrcpyBackend.GrabFrame(serial, 720, "webp")
		if err != nil {
			t.Fatalf("scrcpy grab #%d failed: %v", i+1, err)
		}
		scrcpyTotal += time.Since(t0)
	}
	scrcpyAvg := scrcpyTotal / samples

	// 2. Screencap Benchmark (5 samples to keep test snappy, since screencap is ~1.5-2s each)
	screencapBackend := NewScreencapBackend(nil)
	const screencapSamples = 5
	var screencapTotal time.Duration
	for i := 0; i < screencapSamples; i++ {
		t0 := time.Now()
		_, err := screencapBackend.GrabFrame(serial, 720, "webp")
		if err != nil {
			t.Fatalf("screencap grab #%d failed: %v", i+1, err)
		}
		screencapTotal += time.Since(t0)
	}
	screencapAvg := screencapTotal / screencapSamples

	t.Logf("================ BENCHMARK RESULTS ================")
	t.Logf("Scrcpy Average Frame Latency:    %v (%d samples)", scrcpyAvg, samples)
	t.Logf("Screencap Average Frame Latency: %v (%d samples)", screencapAvg, screencapSamples)
	t.Logf("Speedup Factor:                  %.1fx faster", float64(screencapAvg)/float64(scrcpyAvg))
	t.Logf("===================================================")
}
