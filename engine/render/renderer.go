package render

import (
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

// RenderEngine defines the high-performance interface for headless layout and rendering subsystems.
type RenderEngine interface {
	RenderView(rootView any, width int, height int) (*RenderResult, error)
}

// RenderResult contains the output of a rendering operation.
type RenderResult struct {
	WebPData   []byte
	Base64URI  string
	FrameHash  uint64
	RenderTime time.Duration

	// Detailed sub-system latencies
	LayoutTime time.Duration
	RasterTime time.Duration
	EncodeTime time.Duration

	// Output resolution & accessibility tree
	Width     int
	Height    int
	Semantics []SemanticNode
}

// DiffResult describes visual differences between two view states.
type DiffResult struct {
	PreviousHash   uint64
	CurrentHash    uint64
	Changed        bool
	ChangedPixels  int
	DiffPercentage float64
	BoundingBox    Rect
	DiffWebPData   []byte
	DiffBase64URI  string
	RenderTime     time.Duration
}

// EngineOptions configures the headless rendering engine.
type EngineOptions struct {
	Quality         int           // WebP quality (1-100, default: 75)
	Lossless        bool          // Lossless WebP compression (default: true)
	DownscaleFactor float64       // Optional downscaling factor (e.g. 0.5 for 1080x2400 -> 540x1200)
	TargetWidth     int           // Downscale to target width if > 0
	TargetHeight    int           // Downscale to target height if > 0
	HashAlgorithm   HashAlgorithm // XXH64 (default) or CRC64
	EnableCache     bool          // Reuse encoded WebP bytes if frame pixels are unchanged
	CacheCapacity   int           // Max number of frames in LRU cache (default: 32)
	UseJavaBridge   bool          // If true and Java is available, invoke LayoutRenderer.java
}

// DefaultEngineOptions returns balanced production defaults.
func DefaultEngineOptions() EngineOptions {
	return EngineOptions{
		Quality:         75,
		Lossless:        true,
		HashAlgorithm:   HashXXH64,
		EnableCache:     true,
		CacheCapacity:   32,
		UseJavaBridge:   false,
	}
}

// Option is a functional option for configuring RenderEngine.
type Option func(*EngineOptions)

// WithQuality sets WebP compression quality (1-100).
func WithQuality(q int) Option {
	return func(o *EngineOptions) {
		o.Quality = q
	}
}

// WithLossless enables lossless WebP encoding.
func WithLossless(lossless bool) Option {
	return func(o *EngineOptions) {
		o.Lossless = lossless
	}
}

// WithDownscale sets viewport downscaling (e.g. 0.5).
func WithDownscale(factor float64) Option {
	return func(o *EngineOptions) {
		o.DownscaleFactor = factor
	}
}

// WithTargetResolution sets explicit output pixel resolution.
func WithTargetResolution(w, h int) Option {
	return func(o *EngineOptions) {
		o.TargetWidth = w
		o.TargetHeight = h
	}
}

// WithHashAlgorithm configures the visual hashing algorithm (XXH64 or CRC64).
func WithHashAlgorithm(algo HashAlgorithm) Option {
	return func(o *EngineOptions) {
		o.HashAlgorithm = algo
	}
}

// WithCache enables or disables frame delta caching.
func WithCache(enable bool) Option {
	return func(o *EngineOptions) {
		o.EnableCache = enable
	}
}

// WithCacheCapacity sets the LRU frame cache capacity.
func WithCacheCapacity(cap int) Option {
	return func(o *EngineOptions) {
		o.CacheCapacity = cap
	}
}

// HeadlessRenderEngine implements RenderEngine with headless layout, Skia-like rasterization, and LRU caching.
type HeadlessRenderEngine struct {
	opts       EngineOptions
	hasher     *VisualHasher
	layout     *LayoutBridge
	rasterizer *SoftwareRasterizer
	lruCache   *LRUFrameCache

	mu sync.RWMutex
}

// NewEngine creates a new headless RenderEngine with custom options.
func NewEngine(opts ...Option) RenderEngine {
	cfg := DefaultEngineOptions()
	for _, opt := range opts {
		opt(&cfg)
	}

	return &HeadlessRenderEngine{
		opts:       cfg,
		hasher:     NewVisualHasher(cfg.HashAlgorithm),
		layout:     NewLayoutBridge(),
		rasterizer: NewSoftwareRasterizer(),
		lruCache:   NewLRUFrameCache(cfg.CacheCapacity),
	}
}

// NewLayoutLibRenderer creates a RenderEngine configured for AOSP LayoutLib semantics.
func NewLayoutLibRenderer() RenderEngine {
	return NewEngine(
		WithQuality(75),
		WithLossless(true),
		WithHashAlgorithm(HashXXH64),
		WithCache(true),
	)
}

// NewSkiaRenderer creates a RenderEngine configured for high-performance Skia-like rasterization.
func NewSkiaRenderer() RenderEngine {
	return NewEngine(
		WithQuality(80),
		WithLossless(true),
		WithHashAlgorithm(HashXXH64),
		WithCache(true),
	)
}

// RenderView calculates layout, rasterizes pixels, hashes the buffer, and encodes WebP.
func (e *HeadlessRenderEngine) RenderView(rootView any, width int, height int) (*RenderResult, error) {
	start := time.Now()

	if rootView == nil {
		return nil, fmt.Errorf("rootView cannot be nil")
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid render dimensions: %dx%d (must be > 0)", width, height)
	}

	// 1. Parse/normalize the view tree
	tree, err := e.layout.ParseHierarchy(rootView)
	if err != nil {
		return nil, fmt.Errorf("failed to parse view hierarchy: %w", err)
	}

	// 2. Headless measure and layout pass
	tLayoutStart := time.Now()
	effectiveWidth := width
	effectiveHeight := height

	if err := e.layout.MeasureAndLayout(tree, effectiveWidth, effectiveHeight); err != nil {
		return nil, fmt.Errorf("layout pass failed: %w", err)
	}
	layoutDuration := time.Since(tLayoutStart)

	// 3. Rasterize into in-memory RGBA8888 buffer
	tRastStart := time.Now()
	rgbaBuffer, err := e.rasterizer.Rasterize(tree, effectiveWidth, effectiveHeight)
	if err != nil {
		return nil, fmt.Errorf("rasterization failed: %w", err)
	}

	// Optional downscaling pass (e.g. 1080x2400 -> 540x1200)
	targetW := effectiveWidth
	targetH := effectiveHeight
	if e.opts.DownscaleFactor > 0 && e.opts.DownscaleFactor != 1.0 {
		targetW = int(float64(effectiveWidth) * e.opts.DownscaleFactor)
		targetH = int(float64(effectiveHeight) * e.opts.DownscaleFactor)
	} else if e.opts.TargetWidth > 0 && e.opts.TargetHeight > 0 {
		targetW = e.opts.TargetWidth
		targetH = e.opts.TargetHeight
	}

	if targetW != effectiveWidth || targetH != effectiveHeight {
		if targetW < 1 {
			targetW = 1
		}
		if targetH < 1 {
			targetH = 1
		}
		rgbaBuffer = e.rasterizer.Downscale(rgbaBuffer, targetW, targetH)
	}
	rastDuration := time.Since(tRastStart)

	// 4. Compute fast visual hash over the raw RGBA frame buffer
	frameHash := e.hasher.HashRGBA(rgbaBuffer)

	// 5. Check LRU cache to reuse encoded WebP
	var webpData []byte
	var base64URI string
	var encodeDuration time.Duration

	if e.opts.EnableCache {
		if entry, found := e.lruCache.Get(frameHash); found {
			webpData = entry.WebPData
			base64URI = entry.Base64URI
		}
	}

	if len(webpData) == 0 {
		tEncodeStart := time.Now()
		data, err := e.rasterizer.EncodeWebP(rgbaBuffer, e.opts.Quality, e.opts.Lossless)
		if err != nil {
			return nil, fmt.Errorf("webp encoding failed: %w", err)
		}
		webpData = data
		base64URI = "data:image/webp;base64," + base64.StdEncoding.EncodeToString(data)
		encodeDuration = time.Since(tEncodeStart)

		if e.opts.EnableCache {
			e.lruCache.Put(frameHash, webpData, base64URI)
		}
	}

	// Extract semantics for accessibility / IDE inspection
	semantics := tree.ExtractSemantics()

	return &RenderResult{
		WebPData:   webpData,
		Base64URI:  base64URI,
		FrameHash:  frameHash,
		RenderTime: time.Since(start),
		LayoutTime: layoutDuration,
		RasterTime: rastDuration,
		EncodeTime: encodeDuration,
		Width:      targetW,
		Height:     targetH,
		Semantics:  semantics,
	}, nil
}

// RenderDiff compares two view hierarchies, calculates pixel delta, and produces an annotated diff WebP.
func (e *HeadlessRenderEngine) RenderDiff(prevView, currView any, width, height int) (*DiffResult, error) {
	start := time.Now()

	// 1. Layout and rasterize previous frame
	treeA, err := e.layout.ParseHierarchy(prevView)
	if err != nil {
		return nil, fmt.Errorf("failed to parse prevView: %w", err)
	}
	if err := e.layout.MeasureAndLayout(treeA, width, height); err != nil {
		return nil, fmt.Errorf("prevView layout failed: %w", err)
	}
	bufA, err := e.rasterizer.Rasterize(treeA, width, height)
	if err != nil {
		return nil, fmt.Errorf("prevView raster failed: %w", err)
	}
	hashA := e.hasher.HashRGBA(bufA)

	// 2. Layout and rasterize current frame
	treeB, err := e.layout.ParseHierarchy(currView)
	if err != nil {
		return nil, fmt.Errorf("failed to parse currView: %w", err)
	}
	if err := e.layout.MeasureAndLayout(treeB, width, height); err != nil {
		return nil, fmt.Errorf("currView layout failed: %w", err)
	}
	bufB, err := e.rasterizer.Rasterize(treeB, width, height)
	if err != nil {
		return nil, fmt.Errorf("currView raster failed: %w", err)
	}
	hashB := e.hasher.HashRGBA(bufB)

	// 3. Pixel comparison
	diffRes := CompareFrames(bufA, bufB, 10, true)

	var diffWebP []byte
	var diffURI string

	if diffRes.Changed {
		// Overlay visual diff highlight on current frame
		e.rasterizer.DrawDiffOverlay(bufB, diffRes)
		data, err := e.rasterizer.EncodeWebP(bufB, e.opts.Quality, e.opts.Lossless)
		if err == nil {
			diffWebP = data
			diffURI = "data:image/webp;base64," + base64.StdEncoding.EncodeToString(data)
		}
	}

	return &DiffResult{
		PreviousHash:   hashA,
		CurrentHash:    hashB,
		Changed:        diffRes.Changed,
		ChangedPixels:  diffRes.ChangedPixels,
		DiffPercentage: diffRes.DiffPercentage,
		BoundingBox:    diffRes.BoundingBox,
		DiffWebPData:   diffWebP,
		DiffBase64URI:  diffURI,
		RenderTime:     time.Since(start),
	}, nil
}

// HitTest finds the deepest interactive/leaf node at pixel coordinate (x, y).
func (e *HeadlessRenderEngine) HitTest(rootView any, width, height, x, y int) (*ViewNode, error) {
	tree, err := e.layout.ParseHierarchy(rootView)
	if err != nil {
		return nil, err
	}
	if err := e.layout.MeasureAndLayout(tree, width, height); err != nil {
		return nil, err
	}
	return tree.HitTest(x, y), nil
}
