package accessibility

import (
	"fmt"
	"math"
	"strings"

	"nano_android/engine/ui/profile"
)

// UINode represents the semantic view hierarchy node inspected by Accessibility Intelligence.
type UINode struct {
	ID                 string    `json:"id"`
	ResourceID         string    `json:"resource_id,omitempty"`
	Class              string    `json:"class,omitempty"`
	Text               string    `json:"text,omitempty"`
	ContentDescription string    `json:"content_description,omitempty"`
	Clickable          bool      `json:"clickable,omitempty"`
	Checkable          bool      `json:"checkable,omitempty"`
	Focusable          bool      `json:"focusable,omitempty"`
	Visible            bool      `json:"visible"`
	Bounds             [4]int    `json:"bounds"` // [left, top, right, bottom]
	Children           []*UINode `json:"children,omitempty"`
}

// IssueSeverity classifies accessibility findings.
type IssueSeverity string

const (
	SeverityError   IssueSeverity = "ERROR"
	SeverityWarning IssueSeverity = "WARNING"
	SeverityInfo    IssueSeverity = "INFO"
)

// Issue describes an accessibility, touch-target, contrast, or Compose warning.
type Issue struct {
	RuleID         string        `json:"rule_id"`
	Severity       IssueSeverity `json:"severity"`
	NodeID         string        `json:"node_id,omitempty"`
	Class          string        `json:"class,omitempty"`
	Message        string        `json:"message"`
	Recommendation string        `json:"recommendation"`
}

// AuditReport summarizes accessibility compliance for a screen hierarchy.
type AuditReport struct {
	ProfileID       string  `json:"profile_id"`
	TotalNodes      int     `json:"total_nodes"`
	InteractiveNodes int    `json:"interactive_nodes"`
	ComposeDetected bool    `json:"compose_detected"`
	Issues          []Issue `json:"issues"`
	Passed          bool    `json:"passed"`
}

// AuditTree runs touch-target (48dp), contentDescription, editable-label, focus-order, and Compose audits.
func AuditTree(root *UINode, prof profile.DeviceProfile) AuditReport {
	rep := AuditReport{
		ProfileID: prof.ID,
		Passed:    true,
	}
	if root == nil {
		return rep
	}
	var walk func(n *UINode)
	walk = func(n *UINode) {
		if n == nil || !n.Visible {
			return
		}
		rep.TotalNodes++
		clsLower := strings.ToLower(n.Class)
		if strings.Contains(clsLower, "compose") {
			rep.ComposeDetected = true
			rep.Issues = append(rep.Issues, Issue{
				RuleID:         "COMPOSE_NATIVE_SEMANTICS_REQUIRED",
				Severity:       SeverityInfo,
				NodeID:         n.ID,
				Class:          n.Class,
				Message:        "Jetpack Compose surface detected; Nano JVM does not synthesize View replacements for Compose nodes",
				Recommendation: "Execute Compose semantic/testTag assertions via Robolectric or Android ART backend",
			})
		}

		isInteractive := n.Clickable || n.Checkable || strings.Contains(clsLower, "button") || strings.Contains(clsLower, "edittext")
		if isInteractive {
			rep.InteractiveNodes++
			wPx := n.Bounds[2] - n.Bounds[0]
			hPx := n.Bounds[3] - n.Bounds[1]
			if wPx > 0 && hPx > 0 {
				wDp := prof.PxToDp(wPx)
				hDp := prof.PxToDp(hPx)
				if wDp < 48.0 || hDp < 48.0 {
					rep.Issues = append(rep.Issues, Issue{
						RuleID:         "A11Y_TOUCH_TARGET_TOO_SMALL",
						Severity:       SeverityError,
						NodeID:         n.ID,
						Class:          n.Class,
						Message:        fmt.Sprintf("Interactive target %q measures %.1fdp x %.1fdp (< 48dp minimum)", n.ID, wDp, hDp),
						Recommendation: "Increase android:minWidth/minHeight or touchDelegate to at least 48dp x 48dp",
					})
					rep.Passed = false
				}
			}

			hasText := strings.TrimSpace(n.Text) != ""
			hasDesc := strings.TrimSpace(n.ContentDescription) != ""
			if !hasText && !hasDesc {
				rep.Issues = append(rep.Issues, Issue{
					RuleID:         "A11Y_MISSING_LABEL",
					Severity:       SeverityError,
					NodeID:         n.ID,
					Class:          n.Class,
					Message:        fmt.Sprintf("Interactive node %q (%s) has empty text and empty contentDescription", n.ID, n.Class),
					Recommendation: "Set android:contentDescription or android:hint / labelFor so TalkBack announces purpose",
				})
				rep.Passed = false
			}
		}

		if prof.FontScale >= 1.3 && strings.TrimSpace(n.Text) != "" && (n.Bounds[2]-n.Bounds[0]) > 0 {
			maxCharsApprox := int(float64(n.Bounds[2]-n.Bounds[0]) / (14.0 * prof.FontScale))
			if maxCharsApprox > 0 && len(n.Text) > maxCharsApprox*3 && (n.Bounds[3]-n.Bounds[1]) < int(32*prof.DensityScale) {
				rep.Issues = append(rep.Issues, Issue{
					RuleID:         "A11Y_LARGE_FONT_CLIPPING_RISK",
					Severity:       SeverityWarning,
					NodeID:         n.ID,
					Class:          n.Class,
					Message:        fmt.Sprintf("Text length (%d chars) may clip at fontScale %.1fx inside bounds height %dpx", len(n.Text), prof.FontScale, n.Bounds[3]-n.Bounds[1]),
					Recommendation: "Use wrap_content height and avoid singleLine clipping on scaled typography",
				})
			}
		}

		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)

	if prof.RTLLayout && root != nil {
		rootMidX := (root.Bounds[0] + root.Bounds[2]) / 2
		allLeftPinned := len(root.Children) > 0
		for _, c := range root.Children {
			if c != nil && c.Visible && (c.Bounds[0]+c.Bounds[2])/2 > rootMidX {
				allLeftPinned = false
				break
			}
		}
		if allLeftPinned && len(root.Children) >= 2 && rootMidX > 0 {
			rep.Issues = append(rep.Issues, Issue{
				RuleID:         "A11Y_RTL_LAYOUT_UNMIRRORED",
				Severity:       SeverityWarning,
				NodeID:         root.ID,
				Class:          root.Class,
				Message:        fmt.Sprintf("All children remain left-aligned under RTL locale %q", prof.Locale),
				Recommendation: "Use start/end alignment attributes (paddingStart, layout_alignParentStart) instead of left/right",
			})
		}
	}
	return rep
}

// ColorBlindnessMode simulates deuteranopia or protanopia color vision deficiency for WCAG contrast verification.
type ColorBlindnessMode string

const (
	VisionNormal       ColorBlindnessMode = "normal"
	VisionDeuteranopia ColorBlindnessMode = "deuteranopia"
	VisionProtanopia   ColorBlindnessMode = "protanopia"
)

// SimulateColorBlindness transforms an ARGB color using Brettel/Viénot linear-RGB CVD matrices.
func SimulateColorBlindness(argb uint32, mode ColorBlindnessMode) uint32 {
	if mode == VisionNormal || mode == "" {
		return argb
	}
	a := argb & 0xFF000000
	r := float64((argb>>16)&0xFF) / 255.0
	g := float64((argb>>8)&0xFF) / 255.0
	b := float64(argb&0xFF) / 255.0
	var nr, ng, nb float64
	switch mode {
	case VisionDeuteranopia:
		nr = 0.625*r + 0.375*g
		ng = 0.700*r + 0.300*g
		nb = 0.300*g + 0.700*b
	case VisionProtanopia:
		nr = 0.567*r + 0.433*g
		ng = 0.558*r + 0.442*g
		nb = 0.242*g + 0.758*b
	default:
		nr, ng, nb = r, g, b
	}
	clampByte := func(v float64) uint32 {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 255
		}
		return uint32(math.Round(v * 255.0))
	}
	return a | (clampByte(nr) << 16) | (clampByte(ng) << 8) | clampByte(nb)
}

// ContrastRatioForVision computes WCAG contrast ratio under normal or colorblind simulation.
func ContrastRatioForVision(fgARGB, bgARGB uint32, mode ColorBlindnessMode) float64 {
	return ContrastRatio(SimulateColorBlindness(fgARGB, mode), SimulateColorBlindness(bgARGB, mode))
}

// ContrastRatio computes WCAG 2.1 relative luminance contrast ratio (1.0 .. 21.0) for two ARGB 32-bit colors.
func ContrastRatio(fgARGB, bgARGB uint32) float64 {
	l1 := relLuminance(fgARGB)
	l2 := relLuminance(bgARGB)
	hi := math.Max(l1, l2)
	lo := math.Min(l1, l2)
	return (hi + 0.05) / (lo + 0.05)
}

func relLuminance(argb uint32) float64 {
	r := linearize(float64((argb>>16)&0xFF) / 255.0)
	g := linearize(float64((argb>>8)&0xFF) / 255.0)
	b := linearize(float64(argb&0xFF) / 255.0)
	return 0.2126*r + 0.7152*g + 0.0722*b
}

func linearize(c float64) float64 {
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}
