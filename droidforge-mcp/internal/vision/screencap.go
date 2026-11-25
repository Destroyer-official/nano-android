package vision

import (
	"bytes"
	"fmt"
	"image/png"
)

// ScreencapBackend captures frames using adb exec-out screencap -p as a robust fallback.
type ScreencapBackend struct {
	binaryRunner ADBBinaryRunner
}

// NewScreencapBackend creates a new ScreencapBackend instance.
func NewScreencapBackend(runner ADBBinaryRunner) *ScreencapBackend {
	if runner == nil {
		runner = DefaultADBBinaryRunner
	}
	return &ScreencapBackend{binaryRunner: runner}
}

// GrabFrame captures the screen via adb exec-out screencap -p, resizes it, and encodes to WebP/JPEG.
func (s *ScreencapBackend) GrabFrame(serial string, maxHeight int, format string) ([]byte, error) {
	pngBytes, err := s.binaryRunner(serial, "exec-out", "screencap", "-p")
	if err != nil {
		return nil, fmt.Errorf("screencap failed for device %q: %w", serial, err)
	}
	if len(pngBytes) == 0 {
		return nil, fmt.Errorf("screencap returned empty output for device %q", serial)
	}

	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decode screencap PNG for device %q: %w", serial, err)
	}

	if maxHeight <= 0 {
		maxHeight = 720
	}
	resized := ResizeImage(img, maxHeight)

	if format == "" {
		format = "webp"
	}
	return EncodeImage(resized, format, 75)
}
