package visual

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"nano_android/engine/security/pathcontainment"
	"nano_android/engine/ui/accessibility"
)

// ComparisonMode selects one of the 6 visual/UI comparison strategies.
type ComparisonMode string

const (
	ModePixelExact    ComparisonMode = "pixel_exact"
	ModeTolerantPixel ComparisonMode = "tolerant_pixel"
	ModeStructural    ComparisonMode = "structural"
	ModeSemantic      ComparisonMode = "semantic"
	ModeText          ComparisonMode = "text"
	ModeHybrid        ComparisonMode = "hybrid"
)

// MaskRect defines a normalized rectangle to ignore during pixel comparison (e.g. dynamic clock/ad banner).
type MaskRect struct {
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}

// DiffRegion localizes differing areas on the screen canvas.
type DiffRegion struct {
	X1          int    `json:"x1"`
	Y1          int    `json:"y1"`
	X2          int    `json:"x2"`
	Y2          int    `json:"y2"`
	PixelCount  int    `json:"pixel_count"`
	ComponentID string `json:"component_id,omitempty"`
}

// CompareOptions configures threshold, exclusion masks, and artifact output encoding.
type CompareOptions struct {
	Mode              ComparisonMode `json:"mode"`
	ChannelTolerance  uint8          `json:"channel_tolerance,omitempty"`   // per-channel RGBA tolerance (0..255, default 8)
	MaxDiffRatio      float64        `json:"max_diff_ratio,omitempty"`      // fraction of pixels permitted to differ (default 0.005 for tolerant)
	MaskRegions       []MaskRect     `json:"mask_regions,omitempty"`        // regions ignored by pixel diff
	IncludeImagesB64  bool           `json:"include_images_b64,omitempty"`  // embed heatmap & overlay PNGs
}

// CompareReport captures pixel, structural, semantic, and text diffs + localized heatmap images.
type CompareReport struct {
	Mode             ComparisonMode `json:"mode"`
	Passed           bool           `json:"passed"`
	SimilarityScore  float64        `json:"similarity_score"` // 0.0..1.0
	TotalPixels      int            `json:"total_pixels,omitempty"`
	DiffPixels       int            `json:"diff_pixels,omitempty"`
	DiffRatio        float64        `json:"diff_ratio,omitempty"`
	BaselineSHA256   string         `json:"baseline_sha256,omitempty"`
	CandidateSHA256  string         `json:"candidate_sha256,omitempty"`
	DiffRegions      []DiffRegion   `json:"diff_regions,omitempty"`
	StructuralDiffs  []string       `json:"structural_diffs,omitempty"`
	SemanticDiffs    []string       `json:"semantic_diffs,omitempty"`
	TextDiffs        []string       `json:"text_diffs,omitempty"`
	HeatmapBase64PNG    string         `json:"heatmap_base64_png,omitempty"`
	OverlayBase64PNG    string         `json:"overlay_base64_png,omitempty"`
	SideBySideBase64PNG string         `json:"side_by_side_base64_png,omitempty"`
	Summary             string         `json:"summary"`
}

// BaselineMetadata records Git-versionable metadata for an approved golden snapshot.
type BaselineMetadata struct {
	Name          string                `json:"name"`
	ProfileID     string                `json:"profile_id"`
	ApprovedBy    string                `json:"approved_by"`
	ApprovedAtUTC string                `json:"approved_at_utc"`
	ImageSHA256   string                `json:"image_sha256"`
	Width         int                   `json:"width"`
	Height        int                   `json:"height"`
	AST           *accessibility.UINode `json:"ast,omitempty"`
}

// Compare evaluates visual/UI equivalence across any of the 6 comparison modes.
func Compare(basePNG, candPNG []byte, baseAST, candAST *accessibility.UINode, opts CompareOptions) (*CompareReport, error) {
	if opts.Mode == "" {
		opts.Mode = ModeHybrid
	}
	rep := &CompareReport{
		Mode:            opts.Mode,
		SimilarityScore: 1.0,
	}

	if len(basePNG) > 0 {
		h := sha256.Sum256(basePNG)
		rep.BaselineSHA256 = hex.EncodeToString(h[:])
	}
	if len(candPNG) > 0 {
		h := sha256.Sum256(candPNG)
		rep.CandidateSHA256 = hex.EncodeToString(h[:])
	}

	needsPixel := opts.Mode == ModePixelExact || opts.Mode == ModeTolerantPixel || opts.Mode == ModeHybrid
	if needsPixel {
		if len(basePNG) == 0 || len(candPNG) == 0 {
			return nil, fmt.Errorf("mode %s requires both baseline and candidate PNG images", opts.Mode)
		}
		tol := opts.ChannelTolerance
		maxRatio := opts.MaxDiffRatio
		if opts.Mode == ModePixelExact {
			tol = 0
			maxRatio = 0
		} else {
			if tol == 0 {
				tol = 8
			}
			if maxRatio <= 0 {
				maxRatio = 0.005
			}
		}

		pixRes, err := comparePixels(basePNG, candPNG, tol, opts.MaskRegions, candAST, opts.IncludeImagesB64)
		if err != nil {
			return nil, err
		}
		rep.TotalPixels = pixRes.totalPixels
		rep.DiffPixels = pixRes.diffPixels
		rep.DiffRatio = pixRes.diffRatio
		rep.DiffRegions = pixRes.regions
		rep.HeatmapBase64PNG = pixRes.heatmapB64
		rep.OverlayBase64PNG = pixRes.overlayB64
		rep.SideBySideBase64PNG = pixRes.sideBySideB64

		pixOK := pixRes.diffRatio <= maxRatio
		if opts.Mode == ModePixelExact {
			pixOK = pixRes.diffPixels == 0
		}
		rep.SimilarityScore = math.Max(0.0, 1.0-pixRes.diffRatio)
		if opts.Mode != ModeHybrid {
			rep.Passed = pixOK
		}
	}

	if opts.Mode == ModeStructural || opts.Mode == ModeHybrid {
		rep.StructuralDiffs = diffStructural(baseAST, candAST)
	}
	if opts.Mode == ModeSemantic || opts.Mode == ModeHybrid {
		rep.SemanticDiffs = diffSemantic(baseAST, candAST)
	}
	if opts.Mode == ModeText || opts.Mode == ModeHybrid {
		rep.TextDiffs = diffTexts(baseAST, candAST)
	}

	switch opts.Mode {
	case ModeStructural:
		rep.Passed = len(rep.StructuralDiffs) == 0
		if !rep.Passed {
			rep.SimilarityScore = math.Max(0.0, 1.0-0.1*float64(len(rep.StructuralDiffs)))
		}
	case ModeSemantic:
		rep.Passed = len(rep.SemanticDiffs) == 0
		if !rep.Passed {
			rep.SimilarityScore = math.Max(0.0, 1.0-0.1*float64(len(rep.SemanticDiffs)))
		}
	case ModeText:
		rep.Passed = len(rep.TextDiffs) == 0
		if !rep.Passed {
			rep.SimilarityScore = math.Max(0.0, 1.0-0.1*float64(len(rep.TextDiffs)))
		}
	case ModeHybrid:
		maxRatio := opts.MaxDiffRatio
		if maxRatio <= 0 {
			maxRatio = 0.005
		}
		pixPass := rep.DiffRatio <= maxRatio
		astIssues := len(rep.StructuralDiffs) + len(rep.SemanticDiffs) + len(rep.TextDiffs)
		rep.Passed = pixPass && astIssues == 0
		astPenalty := math.Min(0.5, 0.05*float64(astIssues))
		rep.SimilarityScore = math.Max(0.0, (1.0-rep.DiffRatio)*(1.0-astPenalty))
	}

	rep.Summary = fmt.Sprintf("mode=%s passed=%v similarity=%.4f diff_pixels=%d/%d struct_diffs=%d semantic_diffs=%d text_diffs=%d",
		rep.Mode, rep.Passed, rep.SimilarityScore, rep.DiffPixels, rep.TotalPixels, len(rep.StructuralDiffs), len(rep.SemanticDiffs), len(rep.TextDiffs))
	return rep, nil
}

type pixelCompareOut struct {
	totalPixels   int
	diffPixels    int
	diffRatio     float64
	regions       []DiffRegion
	heatmapB64    string
	overlayB64    string
	sideBySideB64 string
}

func comparePixels(baseBytes, candBytes []byte, tol uint8, masks []MaskRect, astRoot *accessibility.UINode, includeImgs bool) (*pixelCompareOut, error) {
	imgA, err := png.Decode(bytes.NewReader(baseBytes))
	if err != nil {
		return nil, fmt.Errorf("decode baseline png: %w", err)
	}
	imgB, err := png.Decode(bytes.NewReader(candBytes))
	if err != nil {
		return nil, fmt.Errorf("decode candidate png: %w", err)
	}
	bA := imgA.Bounds()
	bB := imgB.Bounds()
	if bA.Dx() != bB.Dx() || bA.Dy() != bB.Dy() {
		return &pixelCompareOut{
			totalPixels: maxInt(bA.Dx()*bA.Dy(), bB.Dx()*bB.Dy()),
			diffPixels:  maxInt(bA.Dx()*bA.Dy(), bB.Dx()*bB.Dy()),
			diffRatio:   1.0,
			regions:     []DiffRegion{{X1: 0, Y1: 0, X2: maxInt(bA.Dx(), bB.Dx()), Y2: maxInt(bA.Dy(), bB.Dy()), PixelCount: maxInt(bA.Dx()*bA.Dy(), bB.Dx()*bB.Dy()), ComponentID: "viewport_dimension_mismatch"}},
		}, nil
	}

	w, h := bA.Dx(), bA.Dy()
	var heatImg, overImg, sideImg *image.RGBA
	const dividerW = 4
	if includeImgs {
		heatImg = image.NewRGBA(image.Rect(0, 0, w, h))
		overImg = image.NewRGBA(image.Rect(0, 0, w, h))
		sideImg = image.NewRGBA(image.Rect(0, 0, w*2+dividerW, h))
		for dy := 0; dy < h; dy++ {
			for dx := 0; dx < dividerW; dx++ {
				sideImg.SetRGBA(w+dx, dy, color.RGBA{56, 189, 248, 255})
			}
		}
	}

	diffPixels := 0
	minX, minY, maxX, maxY := w, h, -1, -1

	inMask := func(x, y int) bool {
		for _, m := range masks {
			if x >= m.X1 && x < m.X2 && y >= m.Y1 && y < m.Y2 {
				return true
			}
		}
		return false
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cA := color.NRGBAModel.Convert(imgA.At(bA.Min.X+x, bA.Min.Y+y)).(color.NRGBA)
			cB := color.NRGBAModel.Convert(imgB.At(bB.Min.X+x, bB.Min.Y+y)).(color.NRGBA)

			masked := inMask(x, y)
			dR := absDiffU8(cA.R, cB.R)
			dG := absDiffU8(cA.G, cB.G)
			dB := absDiffU8(cA.B, cB.B)
			dA := absDiffU8(cA.A, cB.A)
			differ := !masked && (dR > tol || dG > tol || dB > tol || dA > tol)

			if differ {
				diffPixels++
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}

			if includeImgs {
				if masked {
					heatImg.SetRGBA(x, y, color.RGBA{30, 41, 59, 255})
				} else if differ {
					intensity := uint8(minInt(255, int(maxU8(dR, maxU8(dG, dB)))+120))
					heatImg.SetRGBA(x, y, color.RGBA{intensity, 40, 40, 255})
				} else {
					lum := uint8((uint32(cB.R)*299 + uint32(cB.G)*587 + uint32(cB.B)*114) / 3000)
					heatImg.SetRGBA(x, y, color.RGBA{lum, lum, lum, 255})
				}
				overImg.SetRGBA(x, y, color.RGBA{
					R: uint8((uint16(cA.R) + uint16(cB.R)) / 2),
					G: uint8((uint16(cA.G) + uint16(cB.G)) / 2),
					B: uint8((uint16(cA.B) + uint16(cB.B)) / 2),
					A: 255,
				})
				sideImg.SetRGBA(x, y, color.RGBA(cA))
				sideImg.SetRGBA(w+dividerW+x, y, color.RGBA(cB))
			}
		}
	}

	out := &pixelCompareOut{
		totalPixels: w * h,
		diffPixels:  diffPixels,
	}
	if out.totalPixels > 0 {
		out.diffRatio = float64(diffPixels) / float64(out.totalPixels)
	}
	if diffPixels > 0 && maxX >= minX && maxY >= minY {
		compID := findSmallestContainingNode(astRoot, minX, minY, maxX+1, maxY+1)
		out.regions = []DiffRegion{{
			X1:          minX,
			Y1:          minY,
			X2:          maxX + 1,
			Y2:          maxY + 1,
			PixelCount:  diffPixels,
			ComponentID: compID,
		}}
	}
	if includeImgs {
		var hBuf, oBuf, sBuf bytes.Buffer
		_ = png.Encode(&hBuf, heatImg)
		_ = png.Encode(&oBuf, overImg)
		_ = png.Encode(&sBuf, sideImg)
		out.heatmapB64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(hBuf.Bytes())
		out.overlayB64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(oBuf.Bytes())
		out.sideBySideB64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(sBuf.Bytes())
	}
	return out, nil
}

func findSmallestContainingNode(n *accessibility.UINode, x1, y1, x2, y2 int) string {
	if n == nil {
		return ""
	}
	best := ""
	bestArea := int(^uint(0) >> 1)
	var walk func(cur *accessibility.UINode)
	walk = func(cur *accessibility.UINode) {
		if cur == nil {
			return
		}
		b := cur.Bounds
		if b[2] > b[0] && b[3] > b[1] && b[0] <= x1 && b[1] <= y1 && b[2] >= x2 && b[3] >= y2 {
			area := (b[2] - b[0]) * (b[3] - b[1])
			if area < bestArea {
				bestArea = area
				if cur.ResourceID != "" {
					best = cur.ResourceID
				} else {
					best = cur.ID
				}
			}
		}
		for _, ch := range cur.Children {
			walk(ch)
		}
	}
	walk(n)
	return best
}

func diffStructural(a, b *accessibility.UINode) []string {
	var sigA, sigB []string
	var walk func(n *accessibility.UINode, depth int, out *[]string)
	walk = func(n *accessibility.UINode, depth int, out *[]string) {
		if n == nil {
			return
		}
		*out = append(*out, fmt.Sprintf("d%d:%s#%s@%v", depth, n.Class, n.ResourceID, n.Bounds))
		for _, ch := range n.Children {
			walk(ch, depth+1, out)
		}
	}
	walk(a, 0, &sigA)
	walk(b, 0, &sigB)
	if reflect.DeepEqual(sigA, sigB) {
		return nil
	}
	return []string{fmt.Sprintf("structural tree mismatch: baseline_nodes=%d candidate_nodes=%d", len(sigA), len(sigB))}
}

func diffSemantic(a, b *accessibility.UINode) []string {
	mapA := map[string]string{}
	mapB := map[string]string{}
	var collect func(n *accessibility.UINode, dst map[string]string)
	collect = func(n *accessibility.UINode, dst map[string]string) {
		if n == nil {
			return
		}
		key := n.ResourceID
		if key == "" {
			key = n.ID
		}
		dst[key] = fmt.Sprintf("click=%v,check=%v,focus=%v,vis=%v,desc=%s", n.Clickable, n.Checkable, n.Focusable, n.Visible, n.ContentDescription)
		for _, ch := range n.Children {
			collect(ch, dst)
		}
	}
	collect(a, mapA)
	collect(b, mapB)
	var diffs []string
	for k, va := range mapA {
		if vb, ok := mapB[k]; !ok {
			diffs = append(diffs, "missing semantic node: "+k)
		} else if va != vb {
			diffs = append(diffs, fmt.Sprintf("node %s changed (%s -> %s)", k, va, vb))
		}
	}
	for k := range mapB {
		if _, ok := mapA[k]; !ok {
			diffs = append(diffs, "added semantic node: "+k)
		}
	}
	return diffs
}

func diffTexts(a, b *accessibility.UINode) []string {
	var tA, tB []string
	var walk func(n *accessibility.UINode, dst *[]string)
	walk = func(n *accessibility.UINode, dst *[]string) {
		if n == nil {
			return
		}
		if n.Visible && strings.TrimSpace(n.Text) != "" {
			*dst = append(*dst, n.Text)
		}
		for _, ch := range n.Children {
			walk(ch, dst)
		}
	}
	walk(a, &tA)
	walk(b, &tB)
	if reflect.DeepEqual(tA, tB) {
		return nil
	}
	return []string{fmt.Sprintf("visible texts diverged: baseline=%q vs candidate=%q", strings.Join(tA, "|"), strings.Join(tB, "|"))}
}

// BaselineStore manages workspace-contained Git-versionable visual regression golden baselines.
type BaselineStore struct {
	RootDir string
}

// NewBaselineStore initializes a contained store inside workspaceRoot/baselines.
func NewBaselineStore(rootDir string) *BaselineStore {
	_ = os.MkdirAll(rootDir, 0750)
	return &BaselineStore{RootDir: rootDir}
}

// ApproveBaseline explicitly saves an approved golden screenshot & metadata after human/CI review.
func (s *BaselineStore) ApproveBaseline(name, profileID, approvedBy string, pngBytes []byte, ast *accessibility.UINode) (*BaselineMetadata, error) {
	if strings.TrimSpace(approvedBy) == "" {
		return nil, fmt.Errorf("approval requires non-empty approved_by actor (silent baseline updates are forbidden)")
	}
	safeName, err := pathcontainment.SanitizeFilename(name)
	if err != nil {
		return nil, err
	}
	safeProf, err := pathcontainment.SanitizeFilename(profileID)
	if err != nil {
		return nil, err
	}
	img, err := png.DecodeConfig(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, fmt.Errorf("invalid png image: %w", err)
	}

	dir := filepath.Join(s.RootDir, safeProf)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}
	pngTarget, err := pathcontainment.ResolveWriteContained(dir, safeName+".png", []string{".png"})
	if err != nil {
		return nil, err
	}
	metaTarget, err := pathcontainment.ResolveWriteContained(dir, safeName+".json", []string{".json"})
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(pngBytes)
	meta := &BaselineMetadata{
		Name:          safeName,
		ProfileID:     safeProf,
		ApprovedBy:    approvedBy,
		ApprovedAtUTC: time.Now().UTC().Format(time.RFC3339),
		ImageSHA256:   hex.EncodeToString(sum[:]),
		Width:         img.Width,
		Height:        img.Height,
		AST:           ast,
	}
	if err := os.WriteFile(pngTarget, pngBytes, 0644); err != nil {
		return nil, err
	}
	mb, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(metaTarget, mb, 0644); err != nil {
		return nil, err
	}
	return meta, nil
}

// LoadBaseline retrieves a previously approved golden screenshot + metadata.
func (s *BaselineStore) LoadBaseline(name, profileID string) ([]byte, *BaselineMetadata, error) {
	safeName, err := pathcontainment.SanitizeFilename(name)
	if err != nil {
		return nil, nil, err
	}
	safeProf, err := pathcontainment.SanitizeFilename(profileID)
	if err != nil {
		return nil, nil, err
	}
	dir := filepath.Join(s.RootDir, safeProf)
	pngPath, err := pathcontainment.ResolveExistingContained(filepath.Join(dir, safeName+".png"), []string{s.RootDir}, []string{".png"})
	if err != nil {
		return nil, nil, err
	}
	pngBytes, err := os.ReadFile(pngPath)
	if err != nil {
		return nil, nil, err
	}
	var meta BaselineMetadata
	metaPath := filepath.Join(dir, safeName+".json")
	if mb, mErr := os.ReadFile(metaPath); mErr == nil {
		_ = json.Unmarshal(mb, &meta)
	}
	return pngBytes, &meta, nil
}

// ListBaselines enumerates all stored golden baselines across profiles.
func (s *BaselineStore) ListBaselines() ([]BaselineMetadata, error) {
	var out []BaselineMetadata
	_ = filepath.WalkDir(s.RootDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		if mb, rErr := os.ReadFile(p); rErr == nil {
			var m BaselineMetadata
			if json.Unmarshal(mb, &m) == nil {
				out = append(out, m)
			}
		}
		return nil
	})
	return out, nil
}

func absDiffU8(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

func maxU8(a, b uint8) uint8 {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
