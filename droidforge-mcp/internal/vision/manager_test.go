package vision

import (
	"bytes"
	"image/color"
	"image/png"
	"sync"
	"testing"
	"time"
)

func TestManagerCoordinationAndFallback(t *testing.T) {
	testImg := createTestImage(400, 800, color.RGBA{R: 50, G: 100, B: 150, A: 255})
	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, testImg)
	mockBytes := pngBuf.Bytes()

	mockBinRunner := func(serial string, args ...string) ([]byte, error) {
		return mockBytes, nil
	}

	mgr := NewManager(ManagerConfig{
		ScrcpyJarPath:   "non-existent-jar.jar", // forces fallback to screencap
		ADBBinaryRunner: mockBinRunner,
		IdleTimeout:     10 * time.Minute,
	})
	defer mgr.Close()

	frame, err := mgr.GrabFrame("dev-1", 720, "webp")
	if err != nil {
		t.Fatalf("GrabFrame fallback failed: %v", err)
	}
	if len(frame) == 0 {
		t.Fatal("GrabFrame returned 0 bytes")
	}

	decoded, format, err := DecodeImage(frame)
	if err != nil {
		t.Fatalf("failed to decode frame: %v", err)
	}
	if format != "webp" {
		t.Errorf("expected 'webp', got %q", format)
	}
	if decoded.Bounds().Dy() != 720 {
		t.Errorf("expected height 720, got %d", decoded.Bounds().Dy())
	}

	// Verify streaming is false in fallback mode
	if mgr.IsStreaming("dev-1") {
		t.Error("expected IsStreaming=false in screencap fallback mode")
	}
}

func TestManagerConcurrentDevices(t *testing.T) {
	testImg := createTestImage(300, 600, color.White)
	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, testImg)
	mockBytes := pngBuf.Bytes()

	mockBinRunner := func(serial string, args ...string) ([]byte, error) {
		return mockBytes, nil
	}

	mgr := NewManager(ManagerConfig{
		ScrcpyJarPath:   "non-existent.jar",
		ADBBinaryRunner: mockBinRunner,
	})
	defer mgr.Close()

	devices := []string{"emulator-5554", "emulator-5556", "emulator-5558"}
	var wg sync.WaitGroup

	for _, d := range devices {
		wg.Add(1)
		go func(serial string) {
			defer wg.Done()
			f, err := mgr.GrabFrame(serial, 300, "webp")
			if err != nil {
				t.Errorf("GrabFrame error for %s: %v", serial, err)
			}
			if len(f) == 0 {
				t.Errorf("GrabFrame empty for %s", serial)
			}
		}(d)
	}
	wg.Wait()

	// Verify all devices tracked independently
	for _, d := range devices {
		if err := mgr.Teardown(d); err != nil {
			t.Errorf("Teardown error for %s: %v", d, err)
		}
	}
}

func TestManagerAutoTeardownSweeper(t *testing.T) {
	testImg := createTestImage(200, 400, color.White)
	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, testImg)
	mockBytes := pngBuf.Bytes()

	mockBinRunner := func(serial string, args ...string) ([]byte, error) {
		return mockBytes, nil
	}

	// Set short idle timeout (200ms) and short check interval (40ms) for testing
	mgr := NewManager(ManagerConfig{
		ScrcpyJarPath:   "non-existent.jar",
		ADBBinaryRunner: mockBinRunner,
		IdleTimeout:     200 * time.Millisecond,
		CheckInterval:   40 * time.Millisecond,
	})
	defer mgr.Close()

	_, err := mgr.GrabFrame("temp-device", 200, "webp")
	if err != nil {
		t.Fatalf("GrabFrame failed: %v", err)
	}

	// Verify device is in manager
	mgr.mu.RLock()
	_, exists := mgr.lastActive["temp-device"]
	mgr.mu.RUnlock()
	if !exists {
		t.Fatal("expected temp-device in lastActive")
	}

	// Wait for idle timeout and sweeper to fire
	time.Sleep(260 * time.Millisecond)

	// Verify device was automatically swept
	mgr.mu.RLock()
	_, existsAfter := mgr.lastActive["temp-device"]
	mgr.mu.RUnlock()
	if existsAfter {
		t.Errorf("expected temp-device to be automatically swept after idle timeout")
	}
}
