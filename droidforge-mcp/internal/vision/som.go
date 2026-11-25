package vision

import (
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"github.com/anthropic/droidforge-mcp/internal/types"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// High-contrast color palette cycling red, green, blue, orange, purple, cyan.
var DefaultSoMColors = []color.RGBA{
	{R: 230, G: 25, B: 75, A: 255},  // Red
	{R: 60, G: 180, B: 75, A: 255},  // Green
	{R: 0, G: 130, B: 200, A: 255},  // Blue
	{R: 245, G: 130, B: 48, A: 255}, // Orange
	{R: 145, G: 30, B: 180, A: 255}, // Purple
	{R: 70, G: 240, B: 240, A: 255}, // Cyan
}

// SoMConfig defines options for Set-of-Marks overlay rendering.
type SoMConfig struct {
	Colors        []color.RGBA
	BorderWidth   int
	Quality       int
	Format        string
	IncludeLabels bool
	OrigWidth     int // Original device screen width (for coordinate scaling)
	OrigHeight    int // Original device screen height (for coordinate scaling)
}

// DefaultSoMConfig returns the canonical DroidForge SoM configuration.
func DefaultSoMConfig() SoMConfig {
	return SoMConfig{
		Colors:        DefaultSoMColors,
		BorderWidth:   2,
		Quality:       75,
		Format:        "webp",
		IncludeLabels: true,
	}
}

// DrawSoMBoxes draws 2px high-contrast numbered bounding boxes matching node IDs on the frame.
// Returns annotated WebP bytes.
func DrawSoMBoxes(frame []byte, nodes []types.PrunedNode) ([]byte, error) {
	return DrawSoMBoxesWithConfig(frame, nodes, DefaultSoMConfig())
}

// DrawSoMBoxesWithConfig draws bounding boxes according to custom configuration.
func DrawSoMBoxesWithConfig(frame []byte, nodes []types.PrunedNode, cfg SoMConfig) ([]byte, error) {
	if len(frame) == 0 {
		return nil, fmt.Errorf("empty frame provided to DrawSoMBoxes")
	}

	if len(cfg.Colors) == 0 {
		cfg.Colors = DefaultSoMColors
	}
	if cfg.BorderWidth <= 0 {
		cfg.BorderWidth = 2
	}
	if cfg.Quality <= 0 {
		cfg.Quality = 75
	}
	if cfg.Format == "" {
		cfg.Format = "webp"
	}

	img, _, err := DecodeImage(frame)
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame for SoM overlay: %w", err)
	}

	rgba := EnsureRGBA(img)
	bounds := rgba.Bounds()
	imgW := bounds.Dx()
	imgH := bounds.Dy()

	// Calculate coordinate scaling factor
	scaleX, scaleY := 1.0, 1.0
	if cfg.OrigWidth > 0 && cfg.OrigHeight > 0 {
		scaleX = float64(imgW) / float64(cfg.OrigWidth)
		scaleY = float64(imgH) / float64(cfg.OrigHeight)
	} else {
		// Auto-detect if node coordinates exceed image dimensions
		maxR, maxB := 0, 0
		for _, node := range nodes {
			if node.Bounds[2] > maxR {
				maxR = node.Bounds[2]
			}
			if node.Bounds[3] > maxB {
				maxB = node.Bounds[3]
			}
		}
		if (maxR > imgW || maxB > imgH) && maxR > 0 && maxB > 0 {
			scaleX = float64(imgW) / float64(maxR)
			scaleY = float64(imgH) / float64(maxB)
		}
	}

	for i, node := range nodes {
		col := cfg.Colors[i%len(cfg.Colors)]

		left := int(float64(node.Bounds[0]) * scaleX)
		top := int(float64(node.Bounds[1]) * scaleY)
		right := int(float64(node.Bounds[2]) * scaleX)
		bottom := int(float64(node.Bounds[3]) * scaleY)

		// Clamp coordinates to image boundaries
		if left < 0 {
			left = 0
		}
		if top < 0 {
			top = 0
		}
		if right > imgW {
			right = imgW
		}
		if bottom > imgH {
			bottom = imgH
		}

		// Skip degenerate or invalid boxes
		if left >= right || top >= bottom {
			continue
		}

		// 1. Draw high-contrast bounding box with configured border width
		drawBox(rgba, left, top, right, bottom, col, cfg.BorderWidth)

		// 2. Render badge label with node ID (e.g. "n1", "n2")
		if cfg.IncludeLabels {
			label := node.ID
			if label == "" {
				label = fmt.Sprintf("n%d", i+1)
			}
			drawBadgeLabel(rgba, left, top, right, bottom, label, col)
		}
	}

	return EncodeImage(rgba, cfg.Format, cfg.Quality)
}

// drawBox draws a border with the given width.
func drawBox(img *image.RGBA, left, top, right, bottom int, col color.RGBA, borderWidth int) {
	b := img.Bounds()

	// Top horizontal lines
	for y := top; y < top+borderWidth && y < bottom; y++ {
		if y < b.Min.Y || y >= b.Max.Y {
			continue
		}
		for x := left; x < right; x++ {
			if x >= b.Min.X && x < b.Max.X {
				img.SetRGBA(x, y, col)
			}
		}
	}

	// Bottom horizontal lines
	for y := bottom - borderWidth; y < bottom; y++ {
		if y < top || y < b.Min.Y || y >= b.Max.Y {
			continue
		}
		for x := left; x < right; x++ {
			if x >= b.Min.X && x < b.Max.X {
				img.SetRGBA(x, y, col)
			}
		}
	}

	// Left vertical lines
	for x := left; x < left+borderWidth && x < right; x++ {
		if x < b.Min.X || x >= b.Max.X {
			continue
		}
		for y := top; y < bottom; y++ {
			if y >= b.Min.Y && y < b.Max.Y {
				img.SetRGBA(x, y, col)
			}
		}
	}

	// Right vertical lines
	for x := right - borderWidth; x < right; x++ {
		if x < left || x < b.Min.X || x >= b.Max.X {
			continue
		}
		for y := top; y < bottom; y++ {
			if y >= b.Min.Y && y < b.Max.Y {
				img.SetRGBA(x, y, col)
			}
		}
	}
}

// drawBadgeLabel renders a solid colored badge with contrasting text.
func drawBadgeLabel(img *image.RGBA, left, top, right, bottom int, label string, badgeCol color.RGBA) {
	f := basicfont.Face7x13
	charW := f.Width + 1 // ~7px
	charH := f.Height    // 13px

	textW := len(label) * charW
	padX := 3
	padY := 2
	badgeW := textW + padX*2
	badgeH := charH + padY*2

	// Position badge at top-left of box; if too close to top edge, place inside box
	badgeX := left
	badgeY := top - badgeH
	if badgeY < img.Bounds().Min.Y {
		badgeY = top
	}
	if badgeX+badgeW > img.Bounds().Max.X {
		badgeX = img.Bounds().Max.X - badgeW
	}
	if badgeX < img.Bounds().Min.X {
		badgeX = img.Bounds().Min.X
	}

	// Draw badge background rectangle
	badgeRect := image.Rect(badgeX, badgeY, badgeX+badgeW, badgeY+badgeH)
	draw.Draw(img, badgeRect, &image.Uniform{C: badgeCol}, image.Point{}, draw.Src)

	// Draw 1px border around badge for extra contrast
	borderColor := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	for x := badgeRect.Min.X; x < badgeRect.Max.X; x++ {
		img.SetRGBA(x, badgeRect.Min.Y, borderColor)
		img.SetRGBA(x, badgeRect.Max.Y-1, borderColor)
	}
	for y := badgeRect.Min.Y; y < badgeRect.Max.Y; y++ {
		img.SetRGBA(badgeRect.Min.X, y, borderColor)
		img.SetRGBA(badgeRect.Max.X-1, y, borderColor)
	}

	// Render text inside badge
	d := &font.Drawer{
		Dst:  img,
		Src:  image.White,
		Face: f,
		Dot: fixed.Point26_6{
			X: fixed.I(badgeX + padX),
			Y: fixed.I(badgeY + padY + f.Ascent),
		},
	}
	d.DrawString(label)
}

// AttachScreenshot converts compressed image bytes to base64 and populates ObservationResult.
func AttachScreenshot(res *types.ObservationResult, frameBytes []byte, reason string) {
	if res == nil || len(frameBytes) == 0 {
		return
	}
	res.ScreenshotB64 = base64.StdEncoding.EncodeToString(frameBytes)
	res.ScreenshotAttached = true
	res.ScreenshotReason = reason
}
