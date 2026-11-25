package vision

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func createTestImage(w, h int, col color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: col}, image.Point{}, draw.Src)
	return img
}

func TestWebPEncodeDecode(t *testing.T) {
	orig := createTestImage(300, 200, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	data, err := EncodeImage(orig, "webp", 80)
	if err != nil {
		t.Fatalf("EncodeImage(webp) failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("EncodeImage(webp) returned 0 bytes")
	}

	decoded, format, err := DecodeImage(data)
	if err != nil {
		t.Fatalf("DecodeImage failed: %v", err)
	}
	if format != "webp" {
		t.Errorf("expected format 'webp', got %q", format)
	}
	if decoded.Bounds().Dx() != 300 || decoded.Bounds().Dy() != 200 {
		t.Errorf("expected bounds 300x200, got %dx%d", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}

func TestJPEGEncodeDecode(t *testing.T) {
	orig := createTestImage(100, 100, color.RGBA{R: 50, G: 150, B: 250, A: 255})
	data, err := EncodeImage(orig, "jpeg", 85)
	if err != nil {
		t.Fatalf("EncodeImage(jpeg) failed: %v", err)
	}

	decoded, format, err := DecodeImage(data)
	if err != nil {
		t.Fatalf("DecodeImage failed: %v", err)
	}
	if format != "jpeg" {
		t.Errorf("expected format 'jpeg', got %q", format)
	}
	if decoded.Bounds().Dx() != 100 || decoded.Bounds().Dy() != 100 {
		t.Errorf("expected bounds 100x100, got %dx%d", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}

func TestResizeImage(t *testing.T) {
	tests := []struct {
		name      string
		w, h      int
		maxHeight int
		expectedW int
		expectedH int
	}{
		{
			name:      "downscale 1080x1920 to 720p",
			w:         1080,
			h:         1920,
			maxHeight: 720,
			expectedW: 405, // 1080 * 720 / 1920 = 405
			expectedH: 720,
		},
		{
			name:      "no resize needed when h <= maxHeight",
			w:         500,
			h:         400,
			maxHeight: 720,
			expectedW: 500,
			expectedH: 400,
		},
		{
			name:      "no resize when maxHeight <= 0",
			w:         800,
			h:         1000,
			maxHeight: 0,
			expectedW: 800,
			expectedH: 1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := createTestImage(tt.w, tt.h, color.White)
			resized := ResizeImage(img, tt.maxHeight)
			if resized.Bounds().Dx() != tt.expectedW || resized.Bounds().Dy() != tt.expectedH {
				t.Errorf("got %dx%d, want %dx%d", resized.Bounds().Dx(), resized.Bounds().Dy(), tt.expectedW, tt.expectedH)
			}
		})
	}
}

func TestRedactStatusBar(t *testing.T) {
	img := createTestImage(100, 100, color.White)
	RedactStatusBar(img, 24)

	// Verify pixel in redacted zone is black
	blackPixel := img.RGBAAt(50, 10)
	if blackPixel.R != 0 || blackPixel.G != 0 || blackPixel.B != 0 {
		t.Errorf("expected black pixel in status bar, got %+v", blackPixel)
	}

	// Verify pixel below status bar is still white
	whitePixel := img.RGBAAt(50, 30)
	if whitePixel.R != 255 || whitePixel.G != 255 || whitePixel.B != 255 {
		t.Errorf("expected white pixel below status bar, got %+v", whitePixel)
	}
}
