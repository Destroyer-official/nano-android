package render

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image/color"
	"io"
	"math"
	"strings"
)

// MeasureSpec modes matching Android AOSP android.view.View.MeasureSpec
const (
	MeasureSpecModeShift   = 30
	MeasureSpecModeMask    = 0x3 << MeasureSpecModeShift
	MeasureSpecUnspecified = 0 << MeasureSpecModeShift
	MeasureSpecExactly     = 1 << MeasureSpecModeShift
	MeasureSpecAtMost      = 2 << MeasureSpecModeShift
)

// Android LayoutParams constants
const (
	MatchParent = -1
	WrapContent = -2
)

// MakeMeasureSpec creates a measure specification from size and mode.
func MakeMeasureSpec(size int, mode int) int {
	return (size & ^MeasureSpecModeMask) | (mode & MeasureSpecModeMask)
}

// GetMeasureSpecMode extracts the mode from a measure specification.
func GetMeasureSpecMode(measureSpec int) int {
	return measureSpec & MeasureSpecModeMask
}

// GetMeasureSpecSize extracts the size from a measure specification.
func GetMeasureSpecSize(measureSpec int) int {
	return measureSpec & ^MeasureSpecModeMask
}

// Rect defines pixel boundaries [Left, Top, Right, Bottom].
type Rect struct {
	Left   int `json:"left"`
	Top    int `json:"top"`
	Right  int `json:"right"`
	Bottom int `json:"bottom"`
}

// Width returns the horizontal pixel extent.
func (r Rect) Width() int {
	if r.Right < r.Left {
		return 0
	}
	return r.Right - r.Left
}

// Height returns the vertical pixel extent.
func (r Rect) Height() int {
	if r.Bottom < r.Top {
		return 0
	}
	return r.Bottom - r.Top
}

// Contains returns true if the point (x, y) lies inside the rectangle.
func (r Rect) Contains(x, y int) bool {
	return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom
}

// RelativeConstraints defines rules for RelativeLayout / ConstraintLayout positioning.
type RelativeConstraints struct {
	ToLeftOf         string `json:"toLeftOf,omitempty"`
	ToRightOf        string `json:"toRightOf,omitempty"`
	Above            string `json:"above,omitempty"`
	Below            string `json:"below,omitempty"`
	AlignParentTop    bool   `json:"alignParentTop,omitempty"`
	AlignParentBottom bool   `json:"alignParentBottom,omitempty"`
	AlignParentLeft   bool   `json:"alignParentLeft,omitempty"`
	AlignParentRight  bool   `json:"alignParentRight,omitempty"`
	CenterInParent   bool   `json:"centerInParent,omitempty"`
	CenterHorizontal bool   `json:"centerHorizontal,omitempty"`
	CenterVertical   bool   `json:"centerVertical,omitempty"`
}

// SemanticNode represents an accessibility and screen triage node for AI IDE agents.
type SemanticNode struct {
	ID                 string `json:"id,omitempty"`
	Type               string `json:"type"`
	Text               string `json:"text,omitempty"`
	ContentDescription string `json:"contentDescription,omitempty"`
	Bounds             Rect   `json:"bounds"`
	Clickable          bool   `json:"clickable"`
	Role               string `json:"role,omitempty"` // "button", "header", "input", "card", "container"
}

// ViewNode represents a node in an Android View hierarchy or Jetpack Compose tree.
type ViewNode struct {
	ID                 string              `json:"id,omitempty"`
	Type               string              `json:"type"` // "LinearLayout", "TextView", "Button", "CardView", "FrameLayout", "RelativeLayout", "Column", "Row", "Box"
	Text               string              `json:"text,omitempty"`
	TextSize           float64             `json:"textSize,omitempty"`
	TextColor          color.RGBA          `json:"textColor,omitempty"`
	BackgroundColor    color.RGBA          `json:"backgroundColor,omitempty"`
	BorderColor        color.RGBA          `json:"borderColor,omitempty"`
	BorderWidth        int                 `json:"borderWidth,omitempty"`
	CornerRadius       int                 `json:"cornerRadius,omitempty"`
	Elevation          int                 `json:"elevation,omitempty"`
	Padding            [4]int              `json:"padding,omitempty"` // [Left, Top, Right, Bottom]
	Margin             [4]int              `json:"margin,omitempty"`  // [Left, Top, Right, Bottom]
	LayoutWidth        int                 `json:"layoutWidth"`       // MatchParent (-1), WrapContent (-2), or fixed px
	LayoutHeight       int                 `json:"layoutHeight"`      // MatchParent (-1), WrapContent (-2), or fixed px
	Weight             float64             `json:"weight,omitempty"`  // layout_weight for LinearLayout
	Orientation        string              `json:"orientation,omitempty"` // "vertical", "horizontal"
	Gravity            string              `json:"gravity,omitempty"`     // "center", "start", "end", "top", "bottom"
	Relative           RelativeConstraints `json:"relative,omitempty"`
	Modifiers          []string            `json:"modifiers,omitempty"` // Compose Modifiers
	ScrollX            int                 `json:"scrollX,omitempty"`
	ScrollY            int                 `json:"scrollY,omitempty"`
	ContentDescription string              `json:"contentDescription,omitempty"`
	Clickable          bool                `json:"clickable,omitempty"`
	Children           []*ViewNode         `json:"children,omitempty"`
	CustomAttrs        map[string]string   `json:"customAttrs,omitempty"`

	// Measured outputs calculated by Measure()
	MeasuredWidth  int  `json:"measuredWidth"`
	MeasuredHeight int  `json:"measuredHeight"`
	Bounds         Rect `json:"bounds"`
}

// Clone creates a deep copy of the ViewNode.
func (v *ViewNode) Clone() *ViewNode {
	if v == nil {
		return nil
	}
	cpy := *v
	if len(v.Children) > 0 {
		cpy.Children = make([]*ViewNode, len(v.Children))
		for i, child := range v.Children {
			cpy.Children[i] = child.Clone()
		}
	}
	if len(v.Modifiers) > 0 {
		cpy.Modifiers = make([]string, len(v.Modifiers))
		copy(cpy.Modifiers, v.Modifiers)
	}
	if v.CustomAttrs != nil {
		cpy.CustomAttrs = make(map[string]string, len(v.CustomAttrs))
		for k, val := range v.CustomAttrs {
			cpy.CustomAttrs[k] = val
		}
	}
	return &cpy
}

// ApplyComposeModifiers parses and sets layout/styling properties from Jetpack Compose Modifier tokens.
func (v *ViewNode) ApplyComposeModifiers() {
	if len(v.Modifiers) == 0 {
		return
	}
	for _, mod := range v.Modifiers {
		m := strings.TrimSpace(mod)
		lower := strings.ToLower(m)

		if strings.HasPrefix(lower, "fillmaxwidth") {
			v.LayoutWidth = MatchParent
		} else if strings.HasPrefix(lower, "fillmaxheight") {
			v.LayoutHeight = MatchParent
		} else if strings.HasPrefix(lower, "wrapcontentsize") {
			v.LayoutWidth = WrapContent
			v.LayoutHeight = WrapContent
		} else if strings.HasPrefix(lower, "clickable") {
			v.Clickable = true
		} else if strings.HasPrefix(lower, "padding(") {
			var p int
			fmt.Sscanf(lower, "padding(%d)", &p)
			if p > 0 {
				v.Padding = [4]int{p, p, p, p}
			}
		} else if strings.HasPrefix(lower, "size(") {
			var s int
			fmt.Sscanf(lower, "size(%d)", &s)
			if s > 0 {
				v.LayoutWidth = s
				v.LayoutHeight = s
			}
		} else if strings.HasPrefix(lower, "height(") {
			var h int
			fmt.Sscanf(lower, "height(%d)", &h)
			if h > 0 {
				v.LayoutHeight = h
			}
		} else if strings.HasPrefix(lower, "width(") {
			var w int
			fmt.Sscanf(lower, "width(%d)", &w)
			if w > 0 {
				v.LayoutWidth = w
			}
		} else if strings.HasPrefix(lower, "weight(") {
			var wt float64
			fmt.Sscanf(lower, "weight(%f)", &wt)
			if wt > 0 {
				v.Weight = wt
			}
		} else if strings.HasPrefix(lower, "clip(roundedcorner(") || strings.HasPrefix(lower, "clip(roundedcornershape(") {
			var r int
			if strings.Contains(lower, "roundedcorner(") {
				fmt.Sscanf(lower, "clip(roundedcorner(%d))", &r)
			} else {
				fmt.Sscanf(lower, "clip(roundedcornershape(%d))", &r)
			}
			if r > 0 {
				v.CornerRadius = r
			}
		} else if strings.HasPrefix(lower, "shadow(") {
			var elev int
			fmt.Sscanf(lower, "shadow(%d)", &elev)
			if elev > 0 {
				v.Elevation = elev
			}
		}
	}
}

// Measure calculates measuredWidth and measuredHeight according to AOSP MeasureSpec rules.
func (v *ViewNode) Measure(widthMeasureSpec, heightMeasureSpec int) {
	v.ApplyComposeModifiers()

	wMode := GetMeasureSpecMode(widthMeasureSpec)
	wSize := GetMeasureSpecSize(widthMeasureSpec)
	hMode := GetMeasureSpecMode(heightMeasureSpec)
	hSize := GetMeasureSpecSize(heightMeasureSpec)

	padH := v.Padding[0] + v.Padding[2]
	padV := v.Padding[1] + v.Padding[3]

	isContainer := len(v.Children) > 0 || isContainerType(v.Type)

	if !isContainer {
		// Leaf view (TextView, Button, Image, Spacer)
		intrinsicW := padH
		intrinsicH := padV

		fontSize := v.TextSize
		if fontSize <= 0 {
			fontSize = 14
		}

		if len(v.Text) > 0 {
			charWidth := fontSize * 0.6
			intrinsicW += int(math.Ceil(float64(len(v.Text)) * charWidth))
			intrinsicH += int(math.Ceil(fontSize * 1.4))
		} else {
			intrinsicW += int(fontSize * 2)
			intrinsicH += int(fontSize * 2)
		}

		if isButtonType(v.Type) {
			if intrinsicW < 88 {
				intrinsicW = 88
			}
			if intrinsicH < 40 {
				intrinsicH = 40
			}
		}

		v.MeasuredWidth = resolveSizeAndState(intrinsicW, wSize, wMode, v.LayoutWidth)
		v.MeasuredHeight = resolveSizeAndState(intrinsicH, hSize, hMode, v.LayoutHeight)
		return
	}

	// Container view
	isVertical := strings.EqualFold(v.Orientation, "vertical") ||
		strings.EqualFold(v.Type, "Column") ||
		strings.EqualFold(v.Type, "ComposeColumn") ||
		(strings.EqualFold(v.Type, "LinearLayout") && !strings.EqualFold(v.Orientation, "horizontal"))

	if isVertical {
		v.measureVertical(widthMeasureSpec, heightMeasureSpec, padH, padV, wSize, wMode, hSize, hMode)
	} else if strings.EqualFold(v.Orientation, "horizontal") ||
		strings.EqualFold(v.Type, "Row") ||
		strings.EqualFold(v.Type, "ComposeRow") {
		v.measureHorizontal(widthMeasureSpec, heightMeasureSpec, padH, padV, wSize, wMode, hSize, hMode)
	} else if strings.EqualFold(v.Type, "RelativeLayout") || strings.EqualFold(v.Type, "ConstraintLayout") {
		v.measureRelative(widthMeasureSpec, heightMeasureSpec, padH, padV, wSize, wMode, hSize, hMode)
	} else {
		// FrameLayout / Box / CardView (stacked overlay)
		v.measureStack(widthMeasureSpec, heightMeasureSpec, padH, padV, wSize, wMode, hSize, hMode)
	}
}

func (v *ViewNode) measureVertical(widthMeasureSpec, heightMeasureSpec, padH, padV, wSize, wMode, hSize, hMode int) {
	totalChildHeight := 0
	maxChildWidth := 0
	totalWeight := 0.0

	for _, child := range v.Children {
		if child == nil {
			continue
		}
		if child.Weight > 0 {
			totalWeight += child.Weight
			continue
		}

		marginH := child.Margin[0] + child.Margin[2]
		marginV := child.Margin[1] + child.Margin[3]

		childWS := getChildMeasureSpec(widthMeasureSpec, padH+marginH, child.LayoutWidth)
		childHS := getChildMeasureSpec(heightMeasureSpec, padV+marginV+totalChildHeight, child.LayoutHeight)
		child.Measure(childWS, childHS)

		childW := child.MeasuredWidth + marginH
		if childW > maxChildWidth {
			maxChildWidth = childW
		}
		totalChildHeight += child.MeasuredHeight + marginV
	}

	if totalWeight > 0 && hMode == MeasureSpecExactly {
		remainingHeight := hSize - padV - totalChildHeight
		if remainingHeight < 0 {
			remainingHeight = 0
		}
		for _, child := range v.Children {
			if child == nil || child.Weight <= 0 {
				continue
			}
			marginH := child.Margin[0] + child.Margin[2]
			marginV := child.Margin[1] + child.Margin[3]

			weightedH := int(float64(remainingHeight) * (child.Weight / totalWeight))
			childWS := getChildMeasureSpec(widthMeasureSpec, padH+marginH, child.LayoutWidth)
			childHS := MakeMeasureSpec(weightedH, MeasureSpecExactly)
			child.Measure(childWS, childHS)

			childW := child.MeasuredWidth + marginH
			if childW > maxChildWidth {
				maxChildWidth = childW
			}
			totalChildHeight += child.MeasuredHeight + marginV
		}
	}

	contentW := maxChildWidth + padH
	contentH := totalChildHeight + padV

	v.MeasuredWidth = resolveSizeAndState(contentW, wSize, wMode, v.LayoutWidth)
	v.MeasuredHeight = resolveSizeAndState(contentH, hSize, hMode, v.LayoutHeight)
}

func (v *ViewNode) measureHorizontal(widthMeasureSpec, heightMeasureSpec, padH, padV, wSize, wMode, hSize, hMode int) {
	totalChildWidth := 0
	maxChildHeight := 0
	totalWeight := 0.0

	for _, child := range v.Children {
		if child == nil {
			continue
		}
		if child.Weight > 0 {
			totalWeight += child.Weight
			continue
		}

		marginH := child.Margin[0] + child.Margin[2]
		marginV := child.Margin[1] + child.Margin[3]

		childWS := getChildMeasureSpec(widthMeasureSpec, padH+marginH+totalChildWidth, child.LayoutWidth)
		childHS := getChildMeasureSpec(heightMeasureSpec, padV+marginV, child.LayoutHeight)
		child.Measure(childWS, childHS)

		childH := child.MeasuredHeight + marginV
		if childH > maxChildHeight {
			maxChildHeight = childH
		}
		totalChildWidth += child.MeasuredWidth + marginH
	}

	if totalWeight > 0 && wMode == MeasureSpecExactly {
		remainingWidth := wSize - padH - totalChildWidth
		if remainingWidth < 0 {
			remainingWidth = 0
		}
		for _, child := range v.Children {
			if child == nil || child.Weight <= 0 {
				continue
			}
			marginH := child.Margin[0] + child.Margin[2]
			marginV := child.Margin[1] + child.Margin[3]

			weightedW := int(float64(remainingWidth) * (child.Weight / totalWeight))
			childWS := MakeMeasureSpec(weightedW, MeasureSpecExactly)
			childHS := getChildMeasureSpec(heightMeasureSpec, padV+marginV, child.LayoutHeight)
			child.Measure(childWS, childHS)

			childH := child.MeasuredHeight + marginV
			if childH > maxChildHeight {
				maxChildHeight = childH
			}
			totalChildWidth += child.MeasuredWidth + marginH
		}
	}

	contentW := totalChildWidth + padH
	contentH := maxChildHeight + padV

	v.MeasuredWidth = resolveSizeAndState(contentW, wSize, wMode, v.LayoutWidth)
	v.MeasuredHeight = resolveSizeAndState(contentH, hSize, hMode, v.LayoutHeight)
}

func (v *ViewNode) measureRelative(widthMeasureSpec, heightMeasureSpec, padH, padV, wSize, wMode, hSize, hMode int) {
	// First pass: measure all children with available space
	maxW := 0
	maxH := 0
	for _, child := range v.Children {
		if child == nil {
			continue
		}
		marginH := child.Margin[0] + child.Margin[2]
		marginV := child.Margin[1] + child.Margin[3]

		childWS := getChildMeasureSpec(widthMeasureSpec, padH+marginH, child.LayoutWidth)
		childHS := getChildMeasureSpec(heightMeasureSpec, padV+marginV, child.LayoutHeight)
		child.Measure(childWS, childHS)

		w := child.MeasuredWidth + marginH
		h := child.MeasuredHeight + marginV
		if w > maxW {
			maxW = w
		}
		if h > maxH {
			maxH = h
		}
	}

	v.MeasuredWidth = resolveSizeAndState(maxW+padH, wSize, wMode, v.LayoutWidth)
	v.MeasuredHeight = resolveSizeAndState(maxH+padV, hSize, hMode, v.LayoutHeight)
}

func (v *ViewNode) measureStack(widthMeasureSpec, heightMeasureSpec, padH, padV, wSize, wMode, hSize, hMode int) {
	maxChildWidth := 0
	maxChildHeight := 0

	for _, child := range v.Children {
		if child == nil {
			continue
		}
		marginH := child.Margin[0] + child.Margin[2]
		marginV := child.Margin[1] + child.Margin[3]

		childWS := getChildMeasureSpec(widthMeasureSpec, padH+marginH, child.LayoutWidth)
		childHS := getChildMeasureSpec(heightMeasureSpec, padV+marginV, child.LayoutHeight)
		child.Measure(childWS, childHS)

		childW := child.MeasuredWidth + marginH
		childH := child.MeasuredHeight + marginV
		if childW > maxChildWidth {
			maxChildWidth = childW
		}
		if childH > maxChildHeight {
			maxChildHeight = childH
		}
	}

	contentW := maxChildWidth + padH
	contentH := maxChildHeight + padV

	v.MeasuredWidth = resolveSizeAndState(contentW, wSize, wMode, v.LayoutWidth)
	v.MeasuredHeight = resolveSizeAndState(contentH, hSize, hMode, v.LayoutHeight)
}

// Layout assigns exact pixel coordinates [left, top, right, bottom] and recursively positions children.
func (v *ViewNode) Layout(l, t, r, b int) {
	v.Bounds = Rect{Left: l, Top: t, Right: r, Bottom: b}

	if len(v.Children) == 0 {
		return
	}

	padLeft := v.Padding[0]
	padTop := v.Padding[1]
	padRight := v.Padding[2]
	padBottom := v.Padding[3]

	contentLeft := l + padLeft - v.ScrollX
	contentTop := t + padTop - v.ScrollY
	contentRight := r - padRight - v.ScrollX
	contentBottom := b - padBottom - v.ScrollY

	isVertical := strings.EqualFold(v.Orientation, "vertical") ||
		strings.EqualFold(v.Type, "Column") ||
		strings.EqualFold(v.Type, "ComposeColumn") ||
		(strings.EqualFold(v.Type, "LinearLayout") && !strings.EqualFold(v.Orientation, "horizontal"))

	if isVertical {
		currY := contentTop
		for _, child := range v.Children {
			if child == nil {
				continue
			}
			cL := contentLeft + child.Margin[0]
			cT := currY + child.Margin[1]
			cR := cL + child.MeasuredWidth
			cB := cT + child.MeasuredHeight

			if strings.EqualFold(child.Gravity, "center") || strings.EqualFold(child.Gravity, "center_horizontal") {
				availW := (contentRight - contentLeft) - (child.Margin[0] + child.Margin[2])
				cL = contentLeft + child.Margin[0] + (availW-child.MeasuredWidth)/2
				cR = cL + child.MeasuredWidth
			} else if strings.EqualFold(child.Gravity, "end") || strings.EqualFold(child.Gravity, "right") {
				cR = contentRight - child.Margin[2]
				cL = cR - child.MeasuredWidth
			}

			child.Layout(cL, cT, cR, cB)
			currY = cB + child.Margin[3]
		}
	} else if strings.EqualFold(v.Orientation, "horizontal") ||
		strings.EqualFold(v.Type, "Row") ||
		strings.EqualFold(v.Type, "ComposeRow") {
		currX := contentLeft
		for _, child := range v.Children {
			if child == nil {
				continue
			}
			cL := currX + child.Margin[0]
			cT := contentTop + child.Margin[1]
			cR := cL + child.MeasuredWidth
			cB := cT + child.MeasuredHeight

			if strings.EqualFold(child.Gravity, "center") || strings.EqualFold(child.Gravity, "center_vertical") {
				availH := (contentBottom - contentTop) - (child.Margin[1] + child.Margin[3])
				cT = contentTop + child.Margin[1] + (availH-child.MeasuredHeight)/2
				cB = cT + child.MeasuredHeight
			} else if strings.EqualFold(child.Gravity, "bottom") {
				cB = contentBottom - child.Margin[3]
				cT = cB - child.MeasuredHeight
			}

			child.Layout(cL, cT, cR, cB)
			currX = cR + child.Margin[2]
		}
	} else if strings.EqualFold(v.Type, "RelativeLayout") || strings.EqualFold(v.Type, "ConstraintLayout") {
		v.layoutRelative(contentLeft, contentTop, contentRight, contentBottom)
	} else {
		// FrameLayout / Box / CardView: stacked placement
		for _, child := range v.Children {
			if child == nil {
				continue
			}
			cL := contentLeft + child.Margin[0]
			cT := contentTop + child.Margin[1]
			cR := cL + child.MeasuredWidth
			cB := cT + child.MeasuredHeight

			if strings.EqualFold(child.Gravity, "center") {
				availW := (contentRight - contentLeft) - (child.Margin[0] + child.Margin[2])
				availH := (contentBottom - contentTop) - (child.Margin[1] + child.Margin[3])
				cL = contentLeft + child.Margin[0] + (availW-child.MeasuredWidth)/2
				cT = contentTop + child.Margin[1] + (availH-child.MeasuredHeight)/2
				cR = cL + child.MeasuredWidth
				cB = cT + child.MeasuredHeight
			}

			child.Layout(cL, cT, cR, cB)
		}
	}
}

func (v *ViewNode) layoutRelative(cLeft, cTop, cRight, cBottom int) {
	idMap := make(map[string]*ViewNode, len(v.Children))
	for _, child := range v.Children {
		if child != nil && child.ID != "" {
			idMap[child.ID] = child
		}
	}

	availW := cRight - cLeft
	availH := cBottom - cTop

	for _, child := range v.Children {
		if child == nil {
			continue
		}
		cL := cLeft + child.Margin[0]
		cT := cTop + child.Margin[1]
		cR := cL + child.MeasuredWidth
		cB := cT + child.MeasuredHeight

		// Align Parent rules
		if child.Relative.AlignParentRight {
			cR = cRight - child.Margin[2]
			cL = cR - child.MeasuredWidth
		}
		if child.Relative.AlignParentBottom {
			cB = cBottom - child.Margin[3]
			cT = cB - child.MeasuredHeight
		}
		if child.Relative.CenterInParent {
			cL = cLeft + (availW-child.MeasuredWidth)/2
			cR = cL + child.MeasuredWidth
			cT = cTop + (availH-child.MeasuredHeight)/2
			cB = cT + child.MeasuredHeight
		} else {
			if child.Relative.CenterHorizontal {
				cL = cLeft + (availW-child.MeasuredWidth)/2
				cR = cL + child.MeasuredWidth
			}
			if child.Relative.CenterVertical {
				cT = cTop + (availH-child.MeasuredHeight)/2
				cB = cT + child.MeasuredHeight
			}
		}

		// Relative sibling positioning
		if child.Relative.Below != "" {
			if anchor, ok := idMap[child.Relative.Below]; ok {
				cT = anchor.Bounds.Bottom + child.Margin[1]
				cB = cT + child.MeasuredHeight
			}
		}
		if child.Relative.Above != "" {
			if anchor, ok := idMap[child.Relative.Above]; ok {
				cB = anchor.Bounds.Top - child.Margin[3]
				cT = cB - child.MeasuredHeight
			}
		}
		if child.Relative.ToRightOf != "" {
			if anchor, ok := idMap[child.Relative.ToRightOf]; ok {
				cL = anchor.Bounds.Right + child.Margin[0]
				cR = cL + child.MeasuredWidth
			}
		}
		if child.Relative.ToLeftOf != "" {
			if anchor, ok := idMap[child.Relative.ToLeftOf]; ok {
				cR = anchor.Bounds.Left - child.Margin[2]
				cL = cR - child.MeasuredWidth
			}
		}

		child.Layout(cL, cT, cR, cB)
	}
}

// HitTest finds the deepest view node encompassing the given (x, y) coordinates.
func (v *ViewNode) HitTest(x, y int) *ViewNode {
	if !v.Bounds.Contains(x, y) {
		return nil
	}

	// Traverse children in reverse order (topmost z-order first)
	for i := len(v.Children) - 1; i >= 0; i-- {
		child := v.Children[i]
		if child == nil {
			continue
		}
		if hit := child.HitTest(x, y); hit != nil {
			return hit
		}
	}
	return v
}

// ExtractSemantics extracts a flattened list of accessible semantic nodes for AI IDE triage.
func (v *ViewNode) ExtractSemantics() []SemanticNode {
	var nodes []SemanticNode
	v.collectSemantics(&nodes)
	return nodes
}

func (v *ViewNode) collectSemantics(acc *[]SemanticNode) {
	if v == nil {
		return
	}

	hasContent := len(v.Text) > 0 || len(v.ContentDescription) > 0 || v.Clickable || isButtonType(v.Type)
	if hasContent {
		role := "container"
		if isButtonType(v.Type) || v.Clickable {
			role = "button"
		} else if isCardType(v.Type) {
			role = "card"
		} else if strings.EqualFold(v.Type, "TextView") || strings.EqualFold(v.Type, "Text") {
			role = "text"
		}

		*acc = append(*acc, SemanticNode{
			ID:                 v.ID,
			Type:               v.Type,
			Text:               v.Text,
			ContentDescription: v.ContentDescription,
			Bounds:             v.Bounds,
			Clickable:          v.Clickable || isButtonType(v.Type),
			Role:               role,
		})
	}

	for _, child := range v.Children {
		child.collectSemantics(acc)
	}
}

func resolveSizeAndState(size, specSize, specMode, layoutParam int) int {
	switch specMode {
	case MeasureSpecExactly:
		return specSize
	case MeasureSpecAtMost:
		if layoutParam == MatchParent {
			return specSize
		}
		if size > specSize {
			return specSize
		}
		return size
	case MeasureSpecUnspecified:
		fallthrough
	default:
		if layoutParam > 0 {
			return layoutParam
		}
		return size
	}
}

func getChildMeasureSpec(parentSpec, padding, childDimension int) int {
	parentMode := GetMeasureSpecMode(parentSpec)
	parentSize := GetMeasureSpecSize(parentSpec)

	size := parentSize - padding
	if size < 0 {
		size = 0
	}

	var resultSize, resultMode int

	switch parentMode {
	case MeasureSpecExactly:
		if childDimension >= 0 {
			resultSize = childDimension
			resultMode = MeasureSpecExactly
		} else if childDimension == MatchParent {
			resultSize = size
			resultMode = MeasureSpecExactly
		} else if childDimension == WrapContent {
			resultSize = size
			resultMode = MeasureSpecAtMost
		}
	case MeasureSpecAtMost:
		if childDimension >= 0 {
			resultSize = childDimension
			resultMode = MeasureSpecExactly
		} else if childDimension == MatchParent {
			resultSize = size
			resultMode = MeasureSpecAtMost
		} else if childDimension == WrapContent {
			resultSize = size
			resultMode = MeasureSpecAtMost
		}
	case MeasureSpecUnspecified:
		if childDimension >= 0 {
			resultSize = childDimension
			resultMode = MeasureSpecExactly
		} else {
			resultSize = 0
			resultMode = MeasureSpecUnspecified
		}
	}

	return MakeMeasureSpec(resultSize, resultMode)
}

func isContainerType(t string) bool {
	switch strings.ToLower(t) {
	case "viewgroup", "linearlayout", "framelayout", "relativelayout", "constraintlayout", "cardview", "scrollview",
		"column", "row", "box", "card", "composecolumn", "composerow", "composebox":
		return true
	}
	return false
}

func isButtonType(t string) bool {
	switch strings.ToLower(t) {
	case "button", "composebutton", "materialbutton", "elevatedbutton":
		return true
	}
	return false
}

// LayoutBridge coordinates layout parsing, measure, and layout passes.
type LayoutBridge struct{}

// NewLayoutBridge constructs a new LayoutBridge.
func NewLayoutBridge() *LayoutBridge {
	return &LayoutBridge{}
}

// ParseHierarchy normalizes any input type (struct, pointer, JSON, XML, map) into a *ViewNode tree.
func (b *LayoutBridge) ParseHierarchy(input any) (*ViewNode, error) {
	if input == nil {
		return nil, fmt.Errorf("nil input")
	}

	switch v := input.(type) {
	case *ViewNode:
		return v.Clone(), nil
	case ViewNode:
		return v.Clone(), nil
	case string:
		return b.parseFromString(v)
	case []byte:
		return b.parseFromString(string(v))
	case io.Reader:
		bytes, err := io.ReadAll(v)
		if err != nil {
			return nil, err
		}
		return b.parseFromString(string(bytes))
	case map[string]any:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		var node ViewNode
		if err := json.Unmarshal(data, &node); err != nil {
			return nil, err
		}
		return &node, nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("unsupported view hierarchy type: %T", input)
		}
		var node ViewNode
		if err := json.Unmarshal(data, &node); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON into ViewNode: %w", err)
		}
		return &node, nil
	}
}

func (b *LayoutBridge) parseFromString(str string) (*ViewNode, error) {
	trimmed := strings.TrimSpace(str)
	if strings.HasPrefix(trimmed, "<") {
		return parseXMLHierarchy(trimmed)
	}

	var node ViewNode
	if err := json.Unmarshal([]byte(trimmed), &node); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}
	return &node, nil
}

type xmlNode struct {
	XMLName        xml.Name
	ID             string    `xml:"id,attr"`
	Text           string    `xml:"text,attr"`
	Width          string    `xml:"layout_width,attr"`
	Height         string    `xml:"layout_height,attr"`
	Weight         float64   `xml:"layout_weight,attr"`
	Orient         string    `xml:"orientation,attr"`
	Gravity        string    `xml:"gravity,attr"`
	Padding        string    `xml:"padding,attr"`
	Margin         string    `xml:"layout_margin,attr"`
	Below          string    `xml:"layout_below,attr"`
	Above          string    `xml:"layout_above,attr"`
	ToLeft         string    `xml:"layout_toLeftOf,attr"`
	ToRight        string    `xml:"layout_toRightOf,attr"`
	AlignRight     bool      `xml:"layout_alignParentRight,attr"`
	AlignBottom    bool      `xml:"layout_alignParentBottom,attr"`
	CenterInParent bool      `xml:"layout_centerInParent,attr"`
	Nodes          []xmlNode `xml:",any"`
}

func parseXMLHierarchy(xmlStr string) (*ViewNode, error) {
	var xNode xmlNode
	if err := xml.Unmarshal([]byte(xmlStr), &xNode); err != nil {
		return nil, fmt.Errorf("failed to parse layout xml: %w", err)
	}

	return convertXMLToViewNode(&xNode), nil
}

func convertXMLToViewNode(x *xmlNode) *ViewNode {
	node := &ViewNode{
		ID:          cleanXMLAttr(x.ID),
		Type:        x.XMLName.Local,
		Text:        x.Text,
		Orientation: x.Orient,
		Gravity:     x.Gravity,
		Weight:      x.Weight,
		LayoutWidth: parseDim(x.Width),
		LayoutHeight: parseDim(x.Height),
		Relative: RelativeConstraints{
			Below:             cleanXMLAttr(x.Below),
			Above:             cleanXMLAttr(x.Above),
			ToLeftOf:          cleanXMLAttr(x.ToLeft),
			ToRightOf:         cleanXMLAttr(x.ToRight),
			AlignParentRight:  x.AlignRight,
			AlignParentBottom: x.AlignBottom,
			CenterInParent:   x.CenterInParent,
		},
	}

	if x.Padding != "" {
		p := parseDim(x.Padding)
		if p > 0 {
			node.Padding = [4]int{p, p, p, p}
		}
	}
	if x.Margin != "" {
		m := parseDim(x.Margin)
		if m > 0 {
			node.Margin = [4]int{m, m, m, m}
		}
	}

	if isButtonType(node.Type) {
		node.CornerRadius = 8
		node.Elevation = 4
		node.Clickable = true
	}

	if len(x.Nodes) > 0 {
		node.Children = make([]*ViewNode, len(x.Nodes))
		for i := range x.Nodes {
			node.Children[i] = convertXMLToViewNode(&x.Nodes[i])
		}
	}
	return node
}

func cleanXMLAttr(s string) string {
	s = strings.TrimPrefix(s, "@+id/")
	s = strings.TrimPrefix(s, "@id/")
	return s
}

func parseDim(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "match_parent", "fill_parent":
		return MatchParent
	case "wrap_content", "":
		return WrapContent
	default:
		var val int
		fmt.Sscanf(s, "%d", &val)
		if val > 0 {
			return val
		}
		return WrapContent
	}
}

// MeasureAndLayout triggers the standard measure(wSpec, hSpec) and layout(0, 0, w, h) passes.
func (b *LayoutBridge) MeasureAndLayout(root *ViewNode, width, height int) error {
	if root == nil {
		return fmt.Errorf("root node is nil")
	}

	wSpec := MakeMeasureSpec(width, MeasureSpecExactly)
	hSpec := MakeMeasureSpec(height, MeasureSpecExactly)

	root.Measure(wSpec, hSpec)
	root.Layout(0, 0, width, height)

	return nil
}
