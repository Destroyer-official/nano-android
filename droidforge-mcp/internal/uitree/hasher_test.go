package uitree

import (
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

func TestHasher_MasksClockStrings_StableHash(t *testing.T) {
	// Screen A with status bar at 12:34 PM and battery 85%
	nodesA := []types.PrunedNode{
		{ID: "n1", Class: "TextView", Text: "12:34 PM", Bounds: [4]int{50, 20, 200, 60}},
		{ID: "n2", Class: "TextView", Text: "Battery: 85%", Bounds: [4]int{850, 20, 1000, 60}},
		{ID: "n3", Class: "TextView", Text: "Settings", Bounds: [4]int{50, 100, 400, 180}},
		{ID: "n4", Class: "Button", ResID: "btn_wifi", Text: "Network & internet", Bounds: [4]int{50, 200, 1030, 320}, Flags: []string{"click", "enabled"}},
	}

	// Screen B differing only by clock tick (12:35 PM) and battery drop (84%)
	nodesB := []types.PrunedNode{
		{ID: "n1", Class: "TextView", Text: "12:35 PM", Bounds: [4]int{50, 20, 200, 60}},
		{ID: "n2", Class: "TextView", Text: "Battery: 84%", Bounds: [4]int{850, 20, 1000, 60}},
		{ID: "n3", Class: "TextView", Text: "Settings", Bounds: [4]int{50, 100, 400, 180}},
		{ID: "n4", Class: "Button", ResID: "btn_wifi", Text: "Network & internet", Bounds: [4]int{50, 200, 1030, 320}, Flags: []string{"click", "enabled"}},
	}

	// Screen C differing by 24h clock (09:41:15) and 100% battery
	nodesC := []types.PrunedNode{
		{ID: "n1", Class: "TextView", Text: "09:41:15", Bounds: [4]int{50, 20, 200, 60}},
		{ID: "n2", Class: "TextView", Text: "Battery: 100%", Bounds: [4]int{850, 20, 1000, 60}},
		{ID: "n3", Class: "TextView", Text: "Settings", Bounds: [4]int{50, 100, 400, 180}},
		{ID: "n4", Class: "Button", ResID: "btn_wifi", Text: "Network & internet", Bounds: [4]int{50, 200, 1030, 320}, Flags: []string{"click", "enabled"}},
	}

	hashA := ComputeUIHash(nodesA)
	hashB := ComputeUIHash(nodesB)
	hashC := ComputeUIHash(nodesC)

	t.Logf("Hash A: %s, Hash B: %s, Hash C: %s", hashA, hashB, hashC)

	if len(hashA) != 6 {
		t.Fatalf("expected 6-char hash, got %q (len %d)", hashA, len(hashA))
	}

	if hashA != hashB {
		t.Errorf("hashA (%s) != hashB (%s) despite only clock and battery changes", hashA, hashB)
	}

	if hashA != hashC {
		t.Errorf("hashA (%s) != hashC (%s) despite only time format and battery changes", hashA, hashC)
	}
}

func TestHasher_MasksRelativeTimestamps(t *testing.T) {
	nodes1 := []types.PrunedNode{
		{ID: "n1", Class: "TextView", Text: "Received just now", Bounds: [4]int{10, 10, 300, 50}},
		{ID: "n2", Class: "Button", Text: "Reply", Bounds: [4]int{10, 60, 200, 120}, Flags: []string{"click"}},
	}
	nodes2 := []types.PrunedNode{
		{ID: "n1", Class: "TextView", Text: "Received 2m ago", Bounds: [4]int{10, 10, 300, 50}},
		{ID: "n2", Class: "Button", Text: "Reply", Bounds: [4]int{10, 60, 200, 120}, Flags: []string{"click"}},
	}
	nodes3 := []types.PrunedNode{
		{ID: "n1", Class: "TextView", Text: "Received 1 hour ago", Bounds: [4]int{10, 10, 300, 50}},
		{ID: "n2", Class: "Button", Text: "Reply", Bounds: [4]int{10, 60, 200, 120}, Flags: []string{"click"}},
	}

	h1 := ComputeUIHash(nodes1)
	h2 := ComputeUIHash(nodes2)
	h3 := ComputeUIHash(nodes3)

	if h1 != h2 || h2 != h3 {
		t.Errorf("hashes differ across relative timestamps: h1=%s, h2=%s, h3=%s", h1, h2, h3)
	}
}

func TestHasher_MasksProgressBarAnimation(t *testing.T) {
	// Progress bar at 25% vs 75% indeterminate animation
	nodes1 := []types.PrunedNode{
		{ID: "n1", Class: "ProgressBar", Text: "25%", ContentDesc: "Loading 25 percent", Bounds: [4]int{0, 0, 500, 50}},
		{ID: "n2", Class: "Button", Text: "Cancel", Bounds: [4]int{100, 100, 400, 200}, Flags: []string{"click"}},
	}
	nodes2 := []types.PrunedNode{
		{ID: "n1", Class: "ProgressBar", Text: "75%", ContentDesc: "Loading 75 percent", Bounds: [4]int{0, 0, 500, 50}},
		{ID: "n2", Class: "Button", Text: "Cancel", Bounds: [4]int{100, 100, 400, 200}, Flags: []string{"click"}},
	}

	h1 := ComputeUIHash(nodes1)
	h2 := ComputeUIHash(nodes2)

	if h1 != h2 {
		t.Errorf("progress bar animation caused hash change: h1=%s, h2=%s", h1, h2)
	}
}

func TestHasher_DetectsActualStructuralChange(t *testing.T) {
	nodes1 := []types.PrunedNode{
		{ID: "n1", Class: "Button", Text: "Block & Notify", Bounds: [4]int{10, 10, 500, 100}, Flags: []string{"click"}},
	}
	nodes2 := []types.PrunedNode{
		{ID: "n1", Class: "Button", Text: "Dismiss Alert", Bounds: [4]int{10, 10, 500, 100}, Flags: []string{"click"}},
	}

	h1 := ComputeUIHash(nodes1)
	h2 := ComputeUIHash(nodes2)

	if h1 == h2 {
		t.Errorf("expected different hashes for different button text, but got identical: %s", h1)
	}
}
