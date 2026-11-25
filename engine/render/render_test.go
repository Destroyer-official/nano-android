package render

import (
	"bytes"
	"image"
	"image/color"
	"strings"
	"sync"
	"testing"
	"time"

	_ "golang.org/x/image/webp"
)

// createMockLayout builds a realistic Android View / Compose UI layout hierarchy.
func createMockLayout() *ViewNode {
	return &ViewNode{
		ID:              "root_layout",
		Type:            "LinearLayout",
		Orientation:     "vertical",
		BackgroundColor: color.RGBA{R: 243, G: 244, B: 246, A: 255}, // #F3F4F6
		Padding:         [4]int{16, 24, 16, 16},
		LayoutWidth:     MatchParent,
		LayoutHeight:    MatchParent,
		Children: []*ViewNode{
			// CardView container with title & subtitle
			{
				ID:              "card_container",
				Type:            "CardView",
				BackgroundColor: color.RGBA{R: 255, G: 255, B: 255, A: 255}, // White
				CornerRadius:    12,
				Elevation:       4,
				Padding:         [4]int{14, 14, 14, 14},
				Margin:          [4]int{0, 0, 0, 12},
				LayoutWidth:     MatchParent,
				LayoutHeight:    WrapContent,
				Orientation:     "vertical",
				Children: []*ViewNode{
					{
						ID:          "card_title",
						Type:        "TextView",
						Text:        "Nano-Android Layout",
						TextSize:    16,
						TextColor:   color.RGBA{R: 17, G: 24, B: 39, A: 255}, // #111827
						LayoutWidth: MatchParent,
						Margin:      [4]int{0, 0, 0, 4},
					},
					{
						ID:          "card_subtitle",
						Type:        "TextView",
						Text:        "Headless LayoutLib & Skia graphics",
						TextSize:    12,
						TextColor:   color.RGBA{R: 107, G: 114, B: 128, A: 255}, // #6B7280
						LayoutWidth: MatchParent,
					},
				},
			},
			// Primary Action Button
			{
				ID:              "btn_primary",
				Type:            "Button",
				Text:            "Execute Workflow",
				TextSize:        14,
				BackgroundColor: color.RGBA{R: 37, G: 99, B: 235, A: 255}, // #2563EB
				TextColor:       color.RGBA{R: 255, G: 255, B: 255, A: 255},
				CornerRadius:    8,
				Elevation:       3,
				Padding:         [4]int{14, 10, 14, 10},
				Margin:          [4]int{0, 0, 0, 10},
				LayoutWidth:     MatchParent,
				LayoutHeight:    WrapContent,
				Gravity:         "center",
				Clickable:       true,
			},
			// Row container with secondary buttons
			{
				ID:           "button_row",
				Type:         "LinearLayout",
				Orientation:  "horizontal",
				LayoutWidth:  MatchParent,
				LayoutHeight: WrapContent,
				Children: []*ViewNode{
					{
						ID:              "btn_cancel",
						Type:            "Button",
						Text:            "Cancel",
						TextSize:        12,
						BackgroundColor: color.RGBA{R: 255, G: 255, B: 255, A: 255},
						TextColor:       color.RGBA{R: 75, G: 85, B: 99, A: 255},
						BorderColor:     color.RGBA{R: 209, G: 213, B: 219, A: 255},
						BorderWidth:     1,
						CornerRadius:    6,
						Padding:         [4]int{10, 8, 10, 8},
						Margin:          [4]int{0, 0, 8, 0},
						Weight:          1.0,
						LayoutWidth:     0,
						LayoutHeight:    WrapContent,
						Gravity:         "center",
						Clickable:       true,
					},
					{
						ID:              "btn_confirm",
						Type:            "Button",
						Text:            "Confirm",
						TextSize:        12,
						BackgroundColor: color.RGBA{R: 16, G: 185, B: 129, A: 255}, // #10B981
						TextColor:       color.RGBA{R: 255, G: 255, B: 255, A: 255},
						CornerRadius:    6,
						Padding:         [4]int{10, 8, 10, 8},
						Weight:          1.0,
						LayoutWidth:     0,
						LayoutHeight:    WrapContent,
						Gravity:         "center",
						Clickable:       true,
					},
				},
			},
		},
	}
}

// TestMockLayoutRendering verifies rendering a mock layout (buttons, text views, cards) in < 45ms,
// verifying non-zero WebP output, base64 URI formatting, and non-zero visual hashing.
func TestMockLayoutRendering(t *testing.T) {
	engine := NewEngine(
		WithQuality(75),
		WithLossless(true),
		WithHashAlgorithm(HashXXH64),
		WithCache(false),
	)

	mockLayout := createMockLayout()
	width := 120
	height := 200

	result, err := engine.RenderView(mockLayout, width, height)
	if err != nil {
		t.Fatalf("RenderView failed: %v", err)
	}

	// 1. Verify latency requirement: must render in < 45ms
	t.Logf("Render time: %v (budget: 45ms)", result.RenderTime)
	if result.RenderTime >= 45*time.Millisecond {
		t.Errorf("RenderView took %v, exceeded 45ms budget", result.RenderTime)
	}

	// 2. Verify non-zero WebP output
	if len(result.WebPData) == 0 {
		t.Fatal("RenderView returned empty WebP data buffer")
	}
	t.Logf("WebP byte length: %d bytes", len(result.WebPData))

	// 3. Verify base64 data URI format
	expectedPrefix := "data:image/webp;base64,"
	if !strings.HasPrefix(result.Base64URI, expectedPrefix) {
		t.Errorf("expected Base64URI to start with %q, got: %s", expectedPrefix, result.Base64URI[:min(30, len(result.Base64URI))])
	}

	// 4. Verify visual frame hash is non-zero
	if result.FrameHash == 0 {
		t.Error("FrameHash must be non-zero")
	}

	// 5. Verify valid WebP container by decoding
	img, format, err := image.Decode(bytes.NewReader(result.WebPData))
	if err != nil {
		t.Fatalf("Failed to decode generated WebP bytes: %v", err)
	}
	if format != "webp" {
		t.Errorf("Expected format 'webp', got %q", format)
	}
	if img.Bounds().Dx() != width || img.Bounds().Dy() != height {
		t.Errorf("Decoded image bounds %dx%d != expected %dx%d", img.Bounds().Dx(), img.Bounds().Dy(), width, height)
	}

	// 6. Verify semantics extracted
	if len(result.Semantics) == 0 {
		t.Error("Expected non-empty semantics accessibility tree")
	}
}

// TestVisualHashingAndDelta verifies stable visual hashing and frame change delta detection.
func TestVisualHashingAndDelta(t *testing.T) {
	engine := NewEngine(
		WithQuality(75),
		WithHashAlgorithm(HashXXH64),
		WithCache(true),
	)

	layout1 := createMockLayout()
	res1, err := engine.RenderView(layout1, 160, 280)
	if err != nil {
		t.Fatalf("res1 RenderView failed: %v", err)
	}

	// Stable visual hash check
	layout2 := createMockLayout()
	res2, err := engine.RenderView(layout2, 160, 280)
	if err != nil {
		t.Fatalf("res2 RenderView failed: %v", err)
	}

	if res1.FrameHash != res2.FrameHash {
		t.Fatalf("Visual hash unstable: %d != %d for identical trees", res1.FrameHash, res2.FrameHash)
	}

	if HasFrameChanged(res1.FrameHash, res2.FrameHash) {
		t.Errorf("HasFrameChanged should return false for identical frames")
	}

	// Modify one property
	layout3 := createMockLayout()
	btn := layout3.Children[1]
	btn.BackgroundColor = color.RGBA{R: 220, G: 38, B: 38, A: 255} // Red #DC2626
	btn.Text = "Stop Workflow"

	res3, err := engine.RenderView(layout3, 160, 280)
	if err != nil {
		t.Fatalf("res3 RenderView failed: %v", err)
	}

	if res3.FrameHash == res1.FrameHash {
		t.Fatalf("Visual hash failed to detect UI modification: %d == %d", res3.FrameHash, res1.FrameHash)
	}

	if !HasFrameChanged(res1.FrameHash, res3.FrameHash) {
		t.Errorf("HasFrameChanged should return true when pixels changed")
	}
}

// TestHashingAlgorithms verifies both XXH64 and CRC64 produce stable, non-zero hashes.
func TestHashingAlgorithms(t *testing.T) {
	data := []byte("Nano-Android Headless Visual Hashing Test Buffer 2027")

	hasherXXH := NewVisualHasher(HashXXH64)
	h1 := hasherXXH.HashBytes(data)
	h2 := hasherXXH.HashBytes(data)
	if h1 == 0 || h1 != h2 {
		t.Errorf("XXH64 unstable or zero: h1=%d, h2=%d", h1, h2)
	}

	hasherCRC := NewVisualHasher(HashCRC64)
	c1 := hasherCRC.HashBytes(data)
	c2 := hasherCRC.HashBytes(data)
	if c1 == 0 || c1 != c2 {
		t.Errorf("CRC64 unstable or zero: c1=%d, c2=%d", c1, c2)
	}

	if h1 == c1 {
		t.Errorf("XXH64 and CRC64 should produce distinct digests")
	}
}

// TestDownscaling1080x2400To540x1200 verifies downscaling 1080x2400 to 540x1200.
func TestDownscaling1080x2400To540x1200(t *testing.T) {
	engine := NewEngine(
		WithDownscale(0.5),
		WithQuality(75),
	)

	layout := createMockLayout()
	res, err := engine.RenderView(layout, 1080, 2400)
	if err != nil {
		t.Fatalf("Downscaled render failed: %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(res.WebPData))
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	expectedW := 540
	expectedH := 1200
	if img.Bounds().Dx() != expectedW || img.Bounds().Dy() != expectedH {
		t.Errorf("Expected downscaled bounds %dx%d, got %dx%d", expectedW, expectedH, img.Bounds().Dx(), img.Bounds().Dy())
	}
}

// TestMeasureAndLayoutCoordinates verifies exact pixel bounds calculation.
func TestMeasureAndLayoutCoordinates(t *testing.T) {
	bridge := NewLayoutBridge()

	layout := &ViewNode{
		Type:         "LinearLayout",
		Orientation:  "vertical",
		Padding:      [4]int{10, 20, 10, 20},
		LayoutWidth:  MatchParent,
		LayoutHeight: MatchParent,
		Children: []*ViewNode{
			{
				ID:           "view1",
				Type:         "TextView",
				Text:         "Item 1",
				TextSize:     14,
				Margin:       [4]int{5, 5, 5, 5},
				LayoutWidth:  MatchParent,
				LayoutHeight: WrapContent,
			},
			{
				ID:           "view2",
				Type:         "TextView",
				Text:         "Item 2",
				TextSize:     14,
				Margin:       [4]int{5, 10, 5, 5},
				LayoutWidth:  MatchParent,
				LayoutHeight: WrapContent,
			},
		},
	}

	width := 300
	height := 600

	err := bridge.MeasureAndLayout(layout, width, height)
	if err != nil {
		t.Fatalf("MeasureAndLayout failed: %v", err)
	}

	if layout.Bounds.Left != 0 || layout.Bounds.Top != 0 || layout.Bounds.Right != width || layout.Bounds.Bottom != height {
		t.Errorf("Root bounds mismatch: %+v", layout.Bounds)
	}

	v1 := layout.Children[0]
	if v1.Bounds.Left != 15 {
		t.Errorf("v1.Bounds.Left expected 15, got %d", v1.Bounds.Left)
	}
	if v1.Bounds.Top != 25 {
		t.Errorf("v1.Bounds.Top expected 25, got %d", v1.Bounds.Top)
	}

	v2 := layout.Children[1]
	expectedV2Top := v1.Bounds.Bottom + v1.Margin[3] + v2.Margin[1]
	if v2.Bounds.Top != expectedV2Top {
		t.Errorf("v2.Bounds.Top expected %d, got %d", expectedV2Top, v2.Bounds.Top)
	}
}

// TestXMLAndComposeParsing verifies parsing Android XML and Compose hierarchy inputs.
func TestXMLAndComposeParsing(t *testing.T) {
	xmlLayout := `<LinearLayout xmlns:android="http://schemas.android.com/apk/res/android"
		android:layout_width="match_parent"
		android:layout_height="match_parent"
		android:orientation="vertical">
		<TextView
			android:id="@+id/title"
			android:layout_width="match_parent"
			android:layout_height="wrap_content"
			android:text="Welcome to Nano-Android" />
		<Button
			android:id="@+id/btn_action"
			android:layout_width="match_parent"
			android:layout_height="wrap_content"
			android:text="Launch" />
	</LinearLayout>`

	engine := NewEngine()
	res, err := engine.RenderView(xmlLayout, 180, 320)
	if err != nil {
		t.Fatalf("RenderView XML failed: %v", err)
	}
	if len(res.WebPData) == 0 {
		t.Error("RenderView XML produced 0 bytes")
	}

	// Compose-style map
	composeTree := map[string]any{
		"type":        "Column",
		"layoutWidth": -1,
		"children": []any{
			map[string]any{
				"type": "Text",
				"text": "Compose Heading",
			},
			map[string]any{
				"type": "Button",
				"text": "Compose Click",
			},
		},
	}
	res2, err := engine.RenderView(composeTree, 180, 320)
	if err != nil {
		t.Fatalf("RenderView Compose failed: %v", err)
	}
	if len(res2.WebPData) == 0 {
		t.Error("RenderView Compose produced 0 bytes")
	}
}

// TestRelativeLayoutConstraints verifies RelativeLayout positioning rules.
func TestRelativeLayoutConstraints(t *testing.T) {
	bridge := NewLayoutBridge()

	layout := &ViewNode{
		Type:         "RelativeLayout",
		LayoutWidth:  MatchParent,
		LayoutHeight: MatchParent,
		Children: []*ViewNode{
			{
				ID:           "header",
				Type:         "TextView",
				Text:         "Top Header",
				LayoutWidth:  MatchParent,
				LayoutHeight: 40,
			},
			{
				ID:           "content",
				Type:         "TextView",
				Text:         "Middle Content",
				LayoutWidth:  MatchParent,
				LayoutHeight: 100,
				Relative: RelativeConstraints{
					Below: "header",
				},
			},
			{
				ID:           "bottom_action",
				Type:         "Button",
				Text:         "Submit",
				LayoutWidth:  120,
				LayoutHeight: 40,
				Relative: RelativeConstraints{
					AlignParentBottom: true,
					CenterHorizontal:  true,
				},
			},
		},
	}

	err := bridge.MeasureAndLayout(layout, 300, 500)
	if err != nil {
		t.Fatalf("RelativeLayout MeasureAndLayout failed: %v", err)
	}

	header := layout.Children[0]
	content := layout.Children[1]
	bottomAction := layout.Children[2]

	if content.Bounds.Top != header.Bounds.Bottom {
		t.Errorf("content.Bounds.Top expected %d, got %d", header.Bounds.Bottom, content.Bounds.Top)
	}

	if bottomAction.Bounds.Bottom != 500 {
		t.Errorf("bottomAction.Bounds.Bottom expected 500, got %d", bottomAction.Bounds.Bottom)
	}

	expectedX := (300 - 120) / 2
	if bottomAction.Bounds.Left != expectedX {
		t.Errorf("bottomAction.Bounds.Left expected %d, got %d", expectedX, bottomAction.Bounds.Left)
	}
}

// TestHitTestingAndSemantics verifies coordinate hit-testing and semantic tree extraction.
func TestHitTestingAndSemantics(t *testing.T) {
	engine := NewEngine().(*HeadlessRenderEngine)
	layout := createMockLayout()

	// 1. Hit test on button coordinates
	// Mock layout width: 120, height: 200
	hit, err := engine.HitTest(layout, 120, 200, 60, 95)
	if err != nil {
		t.Fatalf("HitTest error: %v", err)
	}
	if hit == nil {
		t.Fatal("HitTest returned nil at button coordinate")
	}
	t.Logf("Hit node: ID=%s, Type=%s, Bounds=%+v", hit.ID, hit.Type, hit.Bounds)

	// 2. Extract semantics
	semantics, err := engine.layout.ParseHierarchy(layout)
	if err != nil {
		t.Fatalf("ParseHierarchy failed: %v", err)
	}
	_ = engine.layout.MeasureAndLayout(semantics, 120, 200)
	semNodes := semantics.ExtractSemantics()
	if len(semNodes) < 3 {
		t.Fatalf("Expected >= 3 semantic nodes, got %d", len(semNodes))
	}
}

// TestVisualDiffEngine verifies visual diffing, pixel change detection, and overlay generation.
func TestVisualDiffEngine(t *testing.T) {
	engine := NewEngine().(*HeadlessRenderEngine)

	layoutA := createMockLayout()
	layoutB := createMockLayout()
	// Change button text and color in layoutB
	layoutB.Children[1].BackgroundColor = color.RGBA{R: 239, G: 68, B: 68, A: 255}
	layoutB.Children[1].Text = "Deleted"

	diffRes, err := engine.RenderDiff(layoutA, layoutB, 120, 200)
	if err != nil {
		t.Fatalf("RenderDiff failed: %v", err)
	}

	if !diffRes.Changed {
		t.Fatal("Expected diffRes.Changed to be true")
	}
	if diffRes.ChangedPixels == 0 {
		t.Fatal("Expected ChangedPixels > 0")
	}
	if diffRes.DiffPercentage <= 0 {
		t.Fatal("Expected DiffPercentage > 0")
	}
	if len(diffRes.DiffWebPData) == 0 {
		t.Fatal("Expected non-empty DiffWebPData")
	}
	t.Logf("Diff result: changed=%v, changedPixels=%d, diffPct=%.2f%%, bbox=%+v",
		diffRes.Changed, diffRes.ChangedPixels, diffRes.DiffPercentage, diffRes.BoundingBox)
}

// TestLRUFrameCacheEviction verifies LRU frame caching behavior and eviction under capacity.
func TestLRUFrameCacheEviction(t *testing.T) {
	cache := NewLRUFrameCache(3)

	cache.Put(1, []byte("data1"), "uri1")
	cache.Put(2, []byte("data2"), "uri2")
	cache.Put(3, []byte("data3"), "uri3")

	if cache.Len() != 3 {
		t.Errorf("Expected cache len 3, got %d", cache.Len())
	}

	// Access key 1 so it becomes most recently used
	if _, ok := cache.Get(1); !ok {
		t.Error("Key 1 should exist")
	}

	// Insert key 4 -> key 2 should be evicted (as oldest)
	cache.Put(4, []byte("data4"), "uri4")

	if _, ok := cache.Get(2); ok {
		t.Error("Key 2 should have been evicted")
	}
	if _, ok := cache.Get(1); !ok {
		t.Error("Key 1 should still exist")
	}
	if _, ok := cache.Get(4); !ok {
		t.Error("Key 4 should exist")
	}
}

// TestIPCStdioServer verifies stdio/JSON-RPC command processing.
func TestIPCStdioServer(t *testing.T) {
	engine := NewEngine()
	layout := createMockLayout()

	req := &IPCRequest{
		Command: "render",
		Width:   120,
		Height:  200,
		Root:    layout,
	}

	resp := ProcessIPCCommand(engine, req)
	if resp.Status != "ok" {
		t.Fatalf("ProcessIPCCommand error: %s", resp.Error)
	}
	if len(resp.WebPBase64) == 0 {
		t.Error("Expected non-empty WebPBase64 in IPC response")
	}
	if resp.FrameHash == 0 {
		t.Error("Expected non-zero FrameHash in IPC response")
	}
}

// TestConcurrentRendering verifies thread safety under high parallel load.
func TestConcurrentRendering(t *testing.T) {
	engine := NewEngine(WithCache(true))
	layout := createMockLayout()

	var wg sync.WaitGroup
	concurrency := 15
	errChan := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			res, err := engine.RenderView(layout, 120, 200)
			if err != nil {
				errChan <- err
				return
			}
			if len(res.WebPData) == 0 || res.FrameHash == 0 {
				errChan <- err
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			t.Fatalf("Concurrent render error: %v", err)
		}
	}
}

// BenchmarkRenderMockLayout measures speed and memory allocations per frame render.
func BenchmarkRenderMockLayout(b *testing.B) {
	engine := NewEngine(WithCache(false), WithQuality(75))
	layout := createMockLayout()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := engine.RenderView(layout, 120, 200)
		if err != nil {
			b.Fatalf("RenderView benchmark error: %v", err)
		}
	}
}

// BenchmarkVisualHasherXXH64 benchmarks hashing throughput on a 540x1200 frame buffer.
func BenchmarkVisualHasherXXH64(b *testing.B) {
	hasher := NewVisualHasher(HashXXH64)
	buf := make([]byte, 540*1200*4)
	for i := range buf {
		buf[i] = byte(i & 0xFF)
	}

	b.SetBytes(int64(len(buf)))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = hasher.HashBytes(buf)
	}
}
