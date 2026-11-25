package vision

import (
	"bytes"
	"errors"
	"image/color"
	"image/png"
	"testing"
)

func TestScreencapBackendGrabFrame(t *testing.T) {
	// Generate mock PNG
	testImg := createTestImage(1080, 1920, color.RGBA{R: 120, G: 180, B: 240, A: 255})
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, testImg); err != nil {
		t.Fatalf("failed to encode mock png: %v", err)
	}
	mockBytes := pngBuf.Bytes()

	mockRunner := func(serial string, args ...string) ([]byte, error) {
		if serial != "test-serial" {
			t.Errorf("unexpected serial: %s", serial)
		}
		if len(args) < 3 || args[0] != "exec-out" || args[1] != "screencap" || args[2] != "-p" {
			t.Errorf("unexpected args: %v", args)
		}
		return mockBytes, nil
	}

	backend := NewScreencapBackend(mockRunner)
	frame, err := backend.GrabFrame("test-serial", 720, "webp")
	if err != nil {
		t.Fatalf("GrabFrame failed: %v", err)
	}
	if len(frame) == 0 {
		t.Fatal("GrabFrame returned 0 bytes")
	}

	decoded, format, err := DecodeImage(frame)
	if err != nil {
		t.Fatalf("failed to decode GrabFrame output: %v", err)
	}
	if format != "webp" {
		t.Errorf("expected 'webp', got %q", format)
	}
	if decoded.Bounds().Dy() != 720 {
		t.Errorf("expected height 720, got %d", decoded.Bounds().Dy())
	}
}

func TestScreencapBackendError(t *testing.T) {
	mockRunner := func(serial string, args ...string) ([]byte, error) {
		return nil, errors.New("device unauthorized")
	}

	backend := NewScreencapBackend(mockRunner)
	_, err := backend.GrabFrame("err-device", 720, "webp")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
