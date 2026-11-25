package render

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	"github.com/mayahiro/go-webp"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// SoftwareRasterizer renders ViewNode trees into RGBA8888 buffers and encodes WebP streams.
type SoftwareRasterizer struct{}

// NewSoftwareRasterizer creates a new software rasterizer.
func NewSoftwareRasterizer() *SoftwareRasterizer {
	return &SoftwareRasterizer{}
}

// Rasterize creates an in-memory RGBA8888 buffer and renders the measured view hierarchy into it.
func (r *SoftwareRasterizer) Rasterize(root *ViewNode, width, height int) (*image.RGBA, error) {
	if root == nil {
		return nil, fmt.Errorf("root node cannot be nil")
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid canvas dimensions: %dx%d", width, height)
	}

	buf := image.NewRGBA(image.Rect(0, 0, width, height))

	// Canvas default background fast clear
	bg := root.BackgroundColor
	if bg.A == 0 {
		bg = color.RGBA{R: 245, G: 245, B: 247, A: 255} // Material Light surface background
	}
	pixLen := len(buf.Pix)
	for i := 0; i < pixLen; i += 4 {
		buf.Pix[i] = bg.R
		buf.Pix[i+1] = bg.G
		buf.Pix[i+2] = bg.B
		buf.Pix[i+3] = bg.A
	}

	// Recursively render view tree
	r.renderNode(buf, root, buf.Bounds())

	return buf, nil
}

// renderNode renders an individual view and its children with clipping.
func (r *SoftwareRasterizer) renderNode(dst *image.RGBA, node *ViewNode, clip image.Rectangle) {
	if node == nil {
		return
	}

	b := node.Bounds
	if b.Width() <= 0 || b.Height() <= 0 {
		return
	}

	nodeRect := image.Rect(b.Left, b.Top, b.Right, b.Bottom).Intersect(clip)
	if nodeRect.Empty() {
		return
	}

	// 1. Draw elevation drop shadow
	if node.Elevation > 0 {
		r.drawElevationShadow(dst, b, node.CornerRadius, node.Elevation, clip)
	}

	// 2. Draw background (rounded or rect or circle)
	bgColor := node.BackgroundColor
	if isButtonType(node.Type) && bgColor.A == 0 {
		bgColor = color.RGBA{R: 37, G: 99, B: 235, A: 255} // Material Blue #2563EB
	} else if isCardType(node.Type) && bgColor.A == 0 {
		bgColor = color.RGBA{R: 255, G: 255, B: 255, A: 255} // White surface
	}

	isCircle := strings.EqualFold(node.CustomAttrs["shape"], "circle") ||
		strings.EqualFold(node.Type, "FloatingActionButton") ||
		strings.EqualFold(node.Type, "Avatar")

	if bgColor.A > 0 {
		if isCircle {
			r.drawCircle(dst, b, bgColor, clip)
		} else if node.CornerRadius > 0 {
			r.drawRoundedRect(dst, b, node.CornerRadius, bgColor, clip)
		} else {
			r.drawSolidRect(dst, nodeRect, bgColor)
		}
	}

	// 3. Draw border
	if node.BorderWidth > 0 && node.BorderColor.A > 0 {
		r.drawBorder(dst, b, node.CornerRadius, node.BorderWidth, node.BorderColor, clip)
	}

	// 4. Draw text (with multi-line wrapping support)
	if len(node.Text) > 0 {
		textColor := node.TextColor
		if textColor.A == 0 {
			if isButtonType(node.Type) {
				textColor = color.RGBA{R: 255, G: 255, B: 255, A: 255}
			} else {
				textColor = color.RGBA{R: 17, G: 24, B: 39, A: 255} // Neutral-900
			}
		}

		fontSize := node.TextSize
		if fontSize <= 0 {
			if isButtonType(node.Type) {
				fontSize = 15
			} else {
				fontSize = 14
			}
		}

		r.drawText(dst, node, b, textColor, fontSize, clip)
	}

	// 5. Recursively render children
	for _, child := range node.Children {
		r.renderNode(dst, child, nodeRect)
	}
}

// drawSolidRect fast fills a rectangle with a color using copy optimization.
func (r *SoftwareRasterizer) drawSolidRect(dst *image.RGBA, rect image.Rectangle, c color.RGBA) {
	if c.A == 0 || rect.Empty() {
		return
	}
	if c.A == 255 {
		rowPixels := rect.Max.X - rect.Min.X
		rowBytes := rowPixels * 4
		if rowBytes <= 0 {
			return
		}
		rowBuf := make([]byte, rowBytes)
		for i := 0; i < rowBytes; i += 4 {
			rowBuf[i] = c.R
			rowBuf[i+1] = c.G
			rowBuf[i+2] = c.B
			rowBuf[i+3] = 255
		}
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			offset := y*dst.Stride + rect.Min.X*4
			copy(dst.Pix[offset:offset+rowBytes], rowBuf)
		}
		return
	}
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		offset := y * dst.Stride
		for x := rect.Min.X; x < rect.Max.X; x++ {
			idx := offset + x*4
			blendPixel(dst.Pix, idx, c)
		}
	}
}

// drawCircle fills an anti-aliased circular shape (e.g. Floating Action Button, Avatar).
func (r *SoftwareRasterizer) drawCircle(dst *image.RGBA, b Rect, c color.RGBA, clip image.Rectangle) {
	cx := float64(b.Left+b.Right) / 2.0
	cy := float64(b.Top+b.Bottom) / 2.0
	rad := float64(min(b.Width(), b.Height())) / 2.0
	if rad <= 0 {
		return
	}

	minX := max(int(cx-rad), clip.Min.X)
	maxX := min(int(cx+rad)+1, clip.Max.X)
	minY := max(int(cy-rad), clip.Min.Y)
	maxY := min(int(cy+rad)+1, clip.Max.Y)

	for y := minY; y < maxY; y++ {
		py := float64(y) + 0.5
		offset := y * dst.Stride
		for x := minX; x < maxX; x++ {
			px := float64(x) + 0.5
			cov := coverage(px, py, cx, cy, rad)
			if cov <= 0 {
				continue
			}
			pixelColor := c
			if cov < 1.0 {
				pixelColor.A = uint8(float64(c.A) * cov)
			}
			blendPixel(dst.Pix, offset+x*4, pixelColor)
		}
	}
}

// drawRoundedRect fills a rounded rectangle by decomposing into solid rectangular bodies and 4 anti-aliased corners.
func (r *SoftwareRasterizer) drawRoundedRect(dst *image.RGBA, b Rect, radius int, c color.RGBA, clip image.Rectangle) {
	w := b.Width()
	h := b.Height()
	rad := radius
	if rad > w/2 {
		rad = w / 2
	}
	if rad > h/2 {
		rad = h / 2
	}

	if rad <= 0 {
		r.drawSolidRect(dst, image.Rect(b.Left, b.Top, b.Right, b.Bottom).Intersect(clip), c)
		return
	}

	// 1. Center solid body
	centerRect := image.Rect(b.Left, b.Top+rad, b.Right, b.Bottom-rad).Intersect(clip)
	if !centerRect.Empty() {
		r.drawSolidRect(dst, centerRect, c)
	}

	// 2. Top solid strip between corners
	topRect := image.Rect(b.Left+rad, b.Top, b.Right-rad, b.Top+rad).Intersect(clip)
	if !topRect.Empty() {
		r.drawSolidRect(dst, topRect, c)
	}

	// 3. Bottom solid strip between corners
	bottomRect := image.Rect(b.Left+rad, b.Bottom-rad, b.Right-rad, b.Bottom).Intersect(clip)
	if !bottomRect.Empty() {
		r.drawSolidRect(dst, bottomRect, c)
	}

	// 4. Four anti-aliased corner boxes
	cx1 := float64(b.Left) + float64(rad)
	cy1 := float64(b.Top) + float64(rad)
	cx2 := float64(b.Right) - float64(rad)
	cy2 := float64(b.Bottom) - float64(rad)
	frad := float64(rad)

	r.drawCorner(dst, image.Rect(b.Left, b.Top, b.Left+rad, b.Top+rad).Intersect(clip), cx1, cy1, frad, c)
	r.drawCorner(dst, image.Rect(b.Right-rad, b.Top, b.Right, b.Top+rad).Intersect(clip), cx2, cy1, frad, c)
	r.drawCorner(dst, image.Rect(b.Left, b.Bottom-rad, b.Left+rad, b.Bottom).Intersect(clip), cx1, cy2, frad, c)
	r.drawCorner(dst, image.Rect(b.Right-rad, b.Bottom-rad, b.Right, b.Bottom).Intersect(clip), cx2, cy2, frad, c)
}

func (r *SoftwareRasterizer) drawCorner(dst *image.RGBA, rect image.Rectangle, cx, cy, rad float64, c color.RGBA) {
	if rect.Empty() {
		return
	}
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		py := float64(y) + 0.5
		offset := y * dst.Stride
		for x := rect.Min.X; x < rect.Max.X; x++ {
			px := float64(x) + 0.5
			cov := coverage(px, py, cx, cy, rad)
			if cov <= 0 {
				continue
			}
			pixelColor := c
			if cov < 1.0 {
				pixelColor.A = uint8(float64(c.A) * cov)
			}
			blendPixel(dst.Pix, offset+x*4, pixelColor)
		}
	}
}

func coverage(px, py, cx, cy, rad float64) float64 {
	dx := px - cx
	dy := py - cy
	dist := math.Sqrt(dx*dx + dy*dy)
	diff := rad - dist
	if diff >= 0.5 {
		return 1.0
	} else if diff <= -0.5 {
		return 0.0
	}
	return diff + 0.5
}

// drawElevationShadow renders a soft, multi-layered drop shadow for elevation.
func (r *SoftwareRasterizer) drawElevationShadow(dst *image.RGBA, b Rect, radius, elevation int, clip image.Rectangle) {
	if elevation <= 0 {
		return
	}

	shadowOffsetY := elevation / 2
	if shadowOffsetY < 1 {
		shadowOffsetY = 1
	}
	blurRadius := elevation * 2

	shadowBounds := Rect{
		Left:   b.Left - blurRadius/2,
		Top:    b.Top + shadowOffsetY,
		Right:  b.Right + blurRadius/2,
		Bottom: b.Bottom + shadowOffsetY + blurRadius,
	}

	minX := max(shadowBounds.Left, clip.Min.X)
	maxX := min(shadowBounds.Right, clip.Max.X)
	minY := max(shadowBounds.Top, clip.Min.Y)
	maxY := min(shadowBounds.Bottom, clip.Max.Y)

	maxAlpha := 40.0 + float64(min(elevation, 12))*3.0

	for y := minY; y < maxY; y++ {
		offset := y * dst.Stride
		distY := float64(y - shadowBounds.Top)
		spanY := float64(shadowBounds.Height())
		if spanY <= 0 {
			continue
		}
		ratioY := 1.0 - (distY / spanY)
		if ratioY < 0 {
			ratioY = 0
		}

		for x := minX; x < maxX; x++ {
			distX := math.Abs(float64(x - (shadowBounds.Left+shadowBounds.Right)/2))
			spanX := float64(shadowBounds.Width()) / 2.0
			ratioX := 1.0 - (distX / spanX)
			if ratioX < 0 {
				ratioX = 0
			}

			intensity := ratioX * ratioY
			if intensity <= 0.02 {
				continue
			}

			alpha := uint8(intensity * maxAlpha)
			if alpha > 0 {
				idx := offset + x*4
				blendPixel(dst.Pix, idx, color.RGBA{R: 0, G: 0, B: 0, A: alpha})
			}
		}
	}
}

// drawBorder draws an anti-aliased border stroke.
func (r *SoftwareRasterizer) drawBorder(dst *image.RGBA, b Rect, radius, strokeWidth int, c color.RGBA, clip image.Rectangle) {
	minX := max(b.Left, clip.Min.X)
	maxX := min(b.Right, clip.Max.X)
	minY := max(b.Top, clip.Min.Y)
	maxY := min(b.Bottom, clip.Max.Y)

	for y := minY; y < maxY; y++ {
		offset := y * dst.Stride
		for x := minX; x < maxX; x++ {
			isBorder := (x < b.Left+strokeWidth) || (x >= b.Right-strokeWidth) ||
				(y < b.Top+strokeWidth) || (y >= b.Bottom-strokeWidth)
			if isBorder {
				idx := offset + x*4
				blendPixel(dst.Pix, idx, c)
			}
		}
	}
}

// drawText renders text characters into the target buffer with multi-line wrapping and gravity.
func (r *SoftwareRasterizer) drawText(dst *image.RGBA, node *ViewNode, b Rect, c color.RGBA, fontSize float64, clip image.Rectangle) {
	txt := node.Text
	if len(txt) == 0 {
		return
	}

	padLeft := b.Left + node.Padding[0]
	padTop := b.Top + node.Padding[1]
	contentW := b.Width() - (node.Padding[0] + node.Padding[2])
	contentH := b.Height() - (node.Padding[1] + node.Padding[3])

	if contentW <= 0 || contentH <= 0 {
		return
	}

	scale := int(math.Round(fontSize / 13.0))
	if scale < 1 {
		scale = 1
	}

	charW := 7 * scale
	lineH := 14 * scale

	// Multi-line word wrapping
	maxCharsPerLine := contentW / charW
	if maxCharsPerLine < 1 {
		maxCharsPerLine = 1
	}

	lines := wrapText(txt, maxCharsPerLine)
	maxLines := contentH / lineH
	if maxLines < 1 {
		maxLines = 1
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		if len(lines[len(lines)-1]) > 3 {
			lines[len(lines)-1] = lines[len(lines)-1][:len(lines[len(lines)-1])-3] + "..."
		}
	}

	totalTextH := len(lines) * lineH
	startY := padTop
	isCenter := strings.EqualFold(node.Gravity, "center") ||
		strings.EqualFold(node.Gravity, "center_horizontal") ||
		isButtonType(node.Type)

	if (isCenter || isButtonType(node.Type)) && contentH > totalTextH {
		startY = padTop + (contentH-totalTextH)/2
	}

	face := basicfont.Face7x13

	for lineIdx, line := range lines {
		linePixelW := len(line) * charW
		startX := padLeft

		if isCenter && contentW > linePixelW {
			startX = padLeft + (contentW-linePixelW)/2
		} else if strings.EqualFold(node.Gravity, "end") && contentW > linePixelW {
			startX = padLeft + (contentW - linePixelW)
		}

		lineY := startY + lineIdx*lineH
		dotX := startX
		dotY := lineY + 11*scale

		if scale == 1 {
			d := &font.Drawer{
				Dst:  dst,
				Src:  &image.Uniform{C: c},
				Face: face,
				Dot:  fixed.Point26_6{X: fixed.I(dotX), Y: fixed.I(dotY)},
			}
			d.DrawString(line)
		} else {
			tmpImg := image.NewRGBA(image.Rect(0, 0, len(line)*7+2, 16))
			d := &font.Drawer{
				Dst:  tmpImg,
				Src:  &image.Uniform{C: c},
				Face: face,
				Dot:  fixed.Point26_6{X: fixed.I(0), Y: fixed.I(11)},
			}
			d.DrawString(line)

			srcBounds := tmpImg.Bounds()
			for sy := srcBounds.Min.Y; sy < srcBounds.Max.Y; sy++ {
				for sx := srcBounds.Min.X; sx < srcBounds.Max.X; sx++ {
					sc := tmpImg.RGBAAt(sx, sy)
					if sc.A == 0 {
						continue
					}
					for dy := 0; dy < scale; dy++ {
						targetY := lineY + sy*scale + dy
						if targetY < clip.Min.Y || targetY >= clip.Max.Y {
							continue
						}
						offset := targetY * dst.Stride
						for dx := 0; dx < scale; dx++ {
							targetX := startX + sx*scale + dx
							if targetX < clip.Min.X || targetX >= clip.Max.X {
								continue
							}
							blendPixel(dst.Pix, offset+targetX*4, sc)
						}
					}
				}
			}
		}
	}
}

// wrapText breaks words cleanly within maxChars per line.
func wrapText(text string, maxChars int) []string {
	if len(text) <= maxChars {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	var currentLine string

	for _, word := range words {
		if len(currentLine) == 0 {
			currentLine = word
		} else if len(currentLine)+1+len(word) <= maxChars {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}
	if len(currentLine) > 0 {
		lines = append(lines, currentLine)
	}
	return lines
}

// Downscale resizes src to target dimensions using high quality bilinear interpolation.
func (r *SoftwareRasterizer) Downscale(src *image.RGBA, targetW, targetH int) *image.RGBA {
	if src == nil {
		return nil
	}
	if targetW <= 0 || targetH <= 0 {
		return src
	}
	if src.Bounds().Dx() == targetW && src.Bounds().Dy() == targetH {
		return src
	}

	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}

// DrawDiffOverlay overlays visual diff highlights and a red bounding box onto the image.
func (r *SoftwareRasterizer) DrawDiffOverlay(dst *image.RGBA, diff FrameDiffResult) {
	if !diff.Changed || dst == nil {
		return
	}

	// 1. Overlay pixel diff mask if present
	if diff.DiffMask != nil {
		draw.Draw(dst, dst.Bounds(), diff.DiffMask, image.Point{}, draw.Over)
	}

	// 2. Draw red bounding box border
	bbox := diff.BoundingBox
	red := color.RGBA{R: 239, G: 68, B: 68, A: 255}
	r.drawBorder(dst, bbox, 4, 2, red, dst.Bounds())
}

// EncodeWebP compresses an image.Image into an optimized WebP byte stream.
func (r *SoftwareRasterizer) EncodeWebP(img image.Image, quality int, lossless bool) ([]byte, error) {
	if img == nil {
		return nil, fmt.Errorf("nil image provided")
	}

	if quality <= 0 {
		quality = 75
	} else if quality > 100 {
		quality = 100
	}

	compression := webp.CompressionLossy
	if lossless {
		compression = webp.CompressionLossless
	}

	var buf bytes.Buffer
	err := webp.Encode(&buf, img, &webp.Options{
		Compression: compression,
		Quality:     quality,
		Mode:        webp.ModeFast,
	})
	if err != nil {
		return nil, fmt.Errorf("webp encode error: %w", err)
	}

	return buf.Bytes(), nil
}

// FormatBase64URI encodes WebP bytes into a data URI.
func (r *SoftwareRasterizer) FormatBase64URI(webpData []byte) string {
	return "data:image/webp;base64," + base64.StdEncoding.EncodeToString(webpData)
}

func blendPixel(pix []byte, idx int, c color.RGBA) {
	if c.A == 255 {
		pix[idx] = c.R
		pix[idx+1] = c.G
		pix[idx+2] = c.B
		pix[idx+3] = 255
		return
	}
	if c.A == 0 {
		return
	}

	alpha := uint32(c.A)
	invAlpha := 255 - alpha

	pix[idx] = uint8((uint32(c.R)*alpha + uint32(pix[idx])*invAlpha) / 255)
	pix[idx+1] = uint8((uint32(c.G)*alpha + uint32(pix[idx+1])*invAlpha) / 255)
	pix[idx+2] = uint8((uint32(c.B)*alpha + uint32(pix[idx+2])*invAlpha) / 255)
	pix[idx+3] = uint8(min(255, int(uint32(pix[idx+3])+alpha)))
}

func isCardType(t string) bool {
	return strings.Contains(strings.ToLower(t), "card")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
