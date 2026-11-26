package ui_test

import (
	"testing"

	"nano_android/engine/ui/accessibility"
	"nano_android/engine/ui/profile"
)

func TestProfilesAndAccessibilityAudit(t *testing.T) {
	all := profile.ReusableProfiles()
	if len(all) < 13 {
		t.Fatalf("expected >= 13 reusable profiles, got %d", len(all))
	}
	prof, err := profile.Get("large_font")
	if err != nil {
		t.Fatal(err)
	}

	root := &accessibility.UINode{
		ID:      "root",
		Class:   "LinearLayout",
		Visible: true,
		Bounds:  [4]int{0, 0, 1080, 2400},
		Children: []*accessibility.UINode{
			{
				ID:        "icon_btn",
				Class:     "ImageButton",
				Clickable: true,
				Visible:   true,
				Bounds:    [4]int{0, 0, 80, 80}, // ~30.5dp < 48dp and missing contentDescription
			},
			{
				ID:      "compose_host",
				Class:   "androidx.compose.ui.platform.ComposeView",
				Visible: true,
				Bounds:  [4]int{0, 100, 1080, 600},
			},
		},
	}

	rep := accessibility.AuditTree(root, prof)
	if rep.Passed || !rep.ComposeDetected || len(rep.Issues) < 3 {
		t.Fatalf("expected failed A11y audit with touch-target, missing-label, and compose notice, got: %+v", rep)
	}

	ratio := accessibility.ContrastRatio(0xFF000000, 0xFFFFFFFF)
	if ratio < 20.9 || ratio > 21.1 {
		t.Fatalf("expected 21.0 WCAG contrast for black on white, got %f", ratio)
	}

	// Verify Deuteranopia and Protanopia transforms shift luminance contrast accurately
	normRG := accessibility.ContrastRatioForVision(0xFFFF0000, 0xFF00AA00, accessibility.VisionNormal)
	deutRG := accessibility.ContrastRatioForVision(0xFFFF0000, 0xFF00AA00, accessibility.VisionDeuteranopia)
	protRG := accessibility.ContrastRatioForVision(0xFFFF0000, 0xFF00AA00, accessibility.VisionProtanopia)
	if deutRG == normRG || protRG == normRG {
		t.Fatalf("expected colorblind simulation contrast delta (norm=%f, deut=%f, prot=%f)", normRG, deutRG, protRG)
	}

	// Verify RTL layout unmirrored warning
	rtlProf, err := profile.Get("rtl_arabic")
	if err != nil {
		t.Fatal(err)
	}
	rtlTree := &accessibility.UINode{
		ID:      "root",
		Class:   "LinearLayout",
		Visible: true,
		Bounds:  [4]int{0, 0, 1080, 2400},
		Children: []*accessibility.UINode{
			{ID: "lbl1", Class: "TextView", Text: "A", Visible: true, Bounds: [4]int{16, 100, 200, 180}},
			{ID: "lbl2", Class: "TextView", Text: "B", Visible: true, Bounds: [4]int{16, 200, 200, 280}},
		},
	}
	rtlRep := accessibility.AuditTree(rtlTree, rtlProf)
	hasRTL := false
	for _, iss := range rtlRep.Issues {
		if iss.RuleID == "A11Y_RTL_LAYOUT_UNMIRRORED" {
			hasRTL = true
		}
	}
	if !hasRTL {
		t.Fatalf("expected A11Y_RTL_LAYOUT_UNMIRRORED finding, got %+v", rtlRep.Issues)
	}
}
