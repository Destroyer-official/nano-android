package ui_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"nano_android/engine/ui/accessibility"
	"nano_android/engine/ui/visual"
)

func makeSolidTestPNG(w, h int, bg color.RGBA, patchRect *image.Rectangle, patchCol color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := bg
			if patchRect != nil && image.Pt(x, y).In(*patchRect) {
				c = patchCol
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestVisualComparisonAllSixModesAndMasks(t *testing.T) {
	bg := color.RGBA{15, 23, 42, 255}
	clockRect := image.Rect(10, 5, 30, 15)
	btnRect := image.Rect(20, 40, 80, 70)

	basePNG := makeSolidTestPNG(100, 100, bg, &clockRect, color.RGBA{255, 255, 255, 255})
	clockChangedPNG := makeSolidTestPNG(100, 100, bg, &clockRect, color.RGBA{200, 220, 255, 255})
	plainPNG := makeSolidTestPNG(100, 100, bg, nil, color.RGBA{})
	btnChangedPNG := makeSolidTestPNG(100, 100, bg, &btnRect, color.RGBA{239, 68, 68, 255})

	astBase := &accessibility.UINode{
		ID: "root", Class: "LinearLayout", Visible: true, Bounds: [4]int{0, 0, 100, 100},
		Children: []*accessibility.UINode{
			{ID: "btn1", ResourceID: "submit_btn", Class: "Button", Text: "Submit", Clickable: true, Visible: true, ContentDescription: "Submit form", Bounds: [4]int{15, 35, 90, 80}},
		},
	}
	astMutText := &accessibility.UINode{
		ID: "root", Class: "LinearLayout", Visible: true, Bounds: [4]int{0, 0, 100, 100},
		Children: []*accessibility.UINode{
			{ID: "btn1", ResourceID: "submit_btn", Class: "Button", Text: "Confirm", Clickable: true, Visible: true, ContentDescription: "Submit form", Bounds: [4]int{15, 35, 90, 80}},
		},
	}

	// 1. pixel_exact fails on changed pixels, passes on identical
	if r, err := visual.Compare(basePNG, basePNG, astBase, astBase, visual.CompareOptions{Mode: visual.ModePixelExact}); err != nil || !r.Passed {
		t.Fatalf("expected identical pixel_exact pass: err=%v r=%+v", err, r)
	}
	if r, err := visual.Compare(basePNG, clockChangedPNG, nil, nil, visual.CompareOptions{Mode: visual.ModePixelExact}); err != nil || r.Passed {
		t.Fatalf("expected changed pixels to fail pixel_exact: err=%v r=%+v", err, r)
	}

	// 2. tolerant_pixel with exclusion mask ignores dynamic clock region
	mask := []visual.MaskRect{{X1: 10, Y1: 5, X2: 30, Y2: 15}}
	if r, err := visual.Compare(basePNG, clockChangedPNG, nil, nil, visual.CompareOptions{Mode: visual.ModeTolerantPixel, MaskRegions: mask}); err != nil || !r.Passed {
		t.Fatalf("expected exclusion mask to pass tolerant_pixel: err=%v r=%+v", err, r)
	}

	// 3. localized diff region maps back to component AST + emits heatmap & overlay PNGs
	rBtn, err := visual.Compare(plainPNG, btnChangedPNG, astBase, astBase, visual.CompareOptions{Mode: visual.ModeTolerantPixel, IncludeImagesB64: true})
	if err != nil || rBtn.Passed || len(rBtn.DiffRegions) != 1 || rBtn.DiffRegions[0].ComponentID != "submit_btn" {
		t.Fatalf("expected submit_btn localization failure: err=%v r=%+v", err, rBtn)
	}
	if !strings.HasPrefix(rBtn.HeatmapBase64PNG, "data:image/png;base64,") || !strings.HasPrefix(rBtn.OverlayBase64PNG, "data:image/png;base64,") || !strings.HasPrefix(rBtn.SideBySideBase64PNG, "data:image/png;base64,") {
		t.Fatalf("expected base64 heatmap, overlay, and side-by-side PNG outputs")
	}

	// 4. structural mode passes when hierarchy unchanged even if text differs
	if r, err := visual.Compare(nil, nil, astBase, astMutText, visual.CompareOptions{Mode: visual.ModeStructural}); err != nil || !r.Passed {
		t.Fatalf("expected identical structural hierarchy to pass: err=%v r=%+v", err, r)
	}

	// 5. semantic mode catches contentDescription / clickable changes
	if r, err := visual.Compare(nil, nil, astBase, astMutText, visual.CompareOptions{Mode: visual.ModeSemantic}); err != nil || !r.Passed {
		t.Fatalf("expected unchanged semantics to pass: err=%v r=%+v", err, r)
	}

	// 6. text + hybrid modes catch text change
	if r, err := visual.Compare(nil, nil, astBase, astMutText, visual.CompareOptions{Mode: visual.ModeText}); err != nil || r.Passed {
		t.Fatalf("expected changed text to fail ModeText: err=%v r=%+v", err, r)
	}
	if r, err := visual.Compare(basePNG, basePNG, astBase, astMutText, visual.CompareOptions{Mode: visual.ModeHybrid}); err != nil || r.Passed {
		t.Fatalf("expected changed text to fail ModeHybrid: err=%v r=%+v", err, r)
	}
}

func TestGoldenBaselineStoreRequiresExplicitApprover(t *testing.T) {
	store := visual.NewBaselineStore(t.TempDir())
	pngBytes := makeSolidTestPNG(32, 32, color.RGBA{0, 0, 0, 255}, nil, color.RGBA{})

	if _, err := store.ApproveBaseline("home", "clean_install", "", pngBytes, nil); err == nil {
		t.Fatal("expected empty approved_by actor to be rejected (no silent baseline overwrites)")
	}
	meta, err := store.ApproveBaseline("home", "clean_install", "qa_lead", pngBytes, nil)
	if err != nil || meta.ApprovedBy != "qa_lead" {
		t.Fatalf("ApproveBaseline failed: err=%v meta=%+v", err, meta)
	}
	loadedPNG, loadedMeta, err := store.LoadBaseline("home", "clean_install")
	if err != nil || len(loadedPNG) == 0 || loadedMeta.ImageSHA256 != meta.ImageSHA256 {
		t.Fatalf("LoadBaseline mismatch: err=%v", err)
	}
	all, _ := store.ListBaselines()
	if len(all) != 1 {
		t.Fatalf("expected 1 baseline stored, got %d", len(all))
	}
}
