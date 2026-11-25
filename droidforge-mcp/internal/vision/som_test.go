package vision

import (
	"image/color"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

func TestDrawSoMBoxes(t *testing.T) {
	// Create a synthetic 720x1280 white image
	syntheticImg := createTestImage(720, 1280, color.White)
	frameBytes, err := EncodeImage(syntheticImg, "webp", 80)
	if err != nil {
		t.Fatalf("failed to encode synthetic image: %v", err)
	}

	nodes := []types.PrunedNode{
		{
			ID:     "n1",
			Bounds: [4]int{50, 100, 250, 200},
			Class:  "Button",
			Text:   "Submit",
		},
		{
			ID:     "n2",
			Bounds: [4]int{300, 100, 500, 200},
			Class:  "EditText",
			Text:   "Search",
		},
		{
			ID:     "n3",
			Bounds: [4]int{50, 300, 650, 500},
			Class:  "CardView",
			Text:   "Item Card",
		},
	}

	annotated, err := DrawSoMBoxes(frameBytes, nodes)
	if err != nil {
		t.Fatalf("DrawSoMBoxes failed: %v", err)
	}
	if len(annotated) == 0 {
		t.Fatal("DrawSoMBoxes returned empty bytes")
	}

	// Verify annotated bytes decode to a valid WebP image
	decoded, format, err := DecodeImage(annotated)
	if err != nil {
		t.Fatalf("failed to decode annotated image: %v", err)
	}
	if format != "webp" {
		t.Errorf("expected 'webp' format, got %q", format)
	}
	if decoded.Bounds().Dx() != 720 || decoded.Bounds().Dy() != 1280 {
		t.Errorf("expected dimensions 720x1280, got %dx%d", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}

	// Verify border pixel of first box (top edge: y=100, x=100) is colored (not white)
	rgba := EnsureRGBA(decoded)
	p := rgba.RGBAAt(100, 100)
	// Color should not be white
	if p.R == 255 && p.G == 255 && p.B == 255 {
		t.Errorf("expected colored box border pixel, but found white: %+v", p)
	}
}

func TestDrawSoMBoxesDegenerate(t *testing.T) {
	syntheticImg := createTestImage(400, 400, color.White)
	frameBytes, _ := EncodeImage(syntheticImg, "webp", 80)

	// Test with invalid, empty, or out-of-bounds nodes
	nodes := []types.PrunedNode{
		{ID: "n1", Bounds: [4]int{0, 0, 0, 0}},         // zero-size
		{ID: "n2", Bounds: [4]int{200, 200, 100, 100}}, // inverted
		{ID: "n3", Bounds: [4]int{-50, -50, 500, 500}}, // out-of-bounds
	}

	// Should not panic or error
	annotated, err := DrawSoMBoxes(frameBytes, nodes)
	if err != nil {
		t.Fatalf("DrawSoMBoxes with degenerate nodes failed: %v", err)
	}
	if len(annotated) == 0 {
		t.Fatal("DrawSoMBoxes returned empty bytes")
	}
}

func TestDrawSoMBoxesScaling(t *testing.T) {
	// Screen is 540x960 (downscaled from 1080x1920)
	syntheticImg := createTestImage(540, 960, color.White)
	frameBytes, _ := EncodeImage(syntheticImg, "webp", 80)

	// Node in 1080x1920 native coordinates
	nodes := []types.PrunedNode{
		{
			ID:     "n1",
			Bounds: [4]int{100, 200, 500, 600}, // in 1080x1920
			Class:  "Button",
		},
	}

	cfg := DefaultSoMConfig()
	cfg.OrigWidth = 1080
	cfg.OrigHeight = 1920

	annotated, err := DrawSoMBoxesWithConfig(frameBytes, nodes, cfg)
	if err != nil {
		t.Fatalf("DrawSoMBoxesWithConfig failed: %v", err)
	}

	decoded, _, err := DecodeImage(annotated)
	if err != nil {
		t.Fatalf("failed to decode image: %v", err)
	}

	// In 540x960, the scaled box should be at x: [50, 250], y: [100, 300]
	// Verify pixel at (150, 100) on top border is colored
	rgba := EnsureRGBA(decoded)
	p := rgba.RGBAAt(150, 100)
	if p.R == 255 && p.G == 255 && p.B == 255 {
		t.Errorf("expected colored border at scaled coordinate (150, 100), but found white: %+v", p)
	}
}
