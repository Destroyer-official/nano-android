package vision

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"strings"

	"github.com/mayahiro/go-webp"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// DecodeImage decodes raw image bytes into image.Image, detecting format (PNG, JPEG, WebP).
func DecodeImage(data []byte) (image.Image, string, error) {
	if len(data) == 0 {
		return nil, "", fmt.Errorf("empty image data")
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode image: %w", err)
	}
	return img, format, nil
}

// ResizeImage resizes src so its height is at most maxHeight, preserving aspect ratio.
// If maxHeight <= 0 or the image's height is already <= maxHeight, src is returned unchanged.
func ResizeImage(src image.Image, maxHeight int) image.Image {
	if src == nil {
		return nil
	}
	bounds := src.Bounds()
	origW := bounds.Dx()
	origH := bounds.Dy()
	if maxHeight <= 0 || origH <= maxHeight {
		return src
	}

	newH := maxHeight
	newW := int(math.Round(float64(origW) * float64(newH) / float64(origH)))
	if newW < 1 {
		newW = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	return dst
}

// EncodeImage compresses img into format ("webp", "jpeg", "jpg", "png") with quality (1-100).
func EncodeImage(img image.Image, format string, quality int) ([]byte, error) {
	if img == nil {
		return nil, fmt.Errorf("nil image provided to EncodeImage")
	}
	if quality <= 0 {
		quality = 75
	} else if quality > 100 {
		quality = 100
	}

	var buf bytes.Buffer
	normFormat := strings.ToLower(strings.TrimSpace(format))
	switch normFormat {
	case "jpeg", "jpg":
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, fmt.Errorf("jpeg encode failed: %w", err)
		}
	case "png":
		if err := png.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("png encode failed: %w", err)
		}
	case "webp", "":
		if err := webp.Encode(&buf, img, &webp.Options{
			Compression: webp.CompressionLossy,
			Quality:     quality,
		}); err != nil {
			return nil, fmt.Errorf("webp encode failed: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported image format: %s", format)
	}
	return buf.Bytes(), nil
}

// EnsureRGBA converts any image.Image into *image.RGBA for direct pixel modification.
func EnsureRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
	return rgba
}

// RedactStatusBar blacks out the top statusBarHeightPx rows of the image.
func RedactStatusBar(img *image.RGBA, statusBarHeightPx int) {
	if img == nil || statusBarHeightPx <= 0 {
		return
	}
	bounds := img.Bounds()
	h := statusBarHeightPx
	if h > bounds.Dy() {
		h = bounds.Dy()
	}
	black := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	for y := bounds.Min.Y; y < bounds.Min.Y+h; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.Set(x, y, black)
		}
	}
}
