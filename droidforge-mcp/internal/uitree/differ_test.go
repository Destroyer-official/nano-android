package uitree

import (
	"strings"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

func TestDiffer_CorrectlyIdentifiesAddedModifiedRemoved(t *testing.T) {
	prev := []types.PrunedNode{
		{ID: "n1", ResID: "btn_wifi", Class: "Button", Text: "Wi-Fi: Off", Bounds: [4]int{50, 100, 1000, 200}, Flags: []string{"click"}},
		{ID: "n2", ResID: "btn_bluetooth", Class: "Button", Text: "Bluetooth", Bounds: [4]int{50, 220, 1000, 320}, Flags: []string{"click"}},
		{ID: "n3", ResID: "btn_display", Class: "Button", Text: "Display", Bounds: [4]int{50, 340, 1000, 440}, Flags: []string{"click"}},
	}

	curr := []types.PrunedNode{
		// Modified: text changed from "Wi-Fi: Off" to "Wi-Fi: On"
		{ID: "n1", ResID: "btn_wifi", Class: "Button", Text: "Wi-Fi: On", Bounds: [4]int{50, 100, 1000, 200}, Flags: []string{"click"}},
		// Unchanged: Bluetooth
		{ID: "n2", ResID: "btn_bluetooth", Class: "Button", Text: "Bluetooth", Bounds: [4]int{50, 220, 1000, 320}, Flags: []string{"click"}},
		// Removed: Display (n3 not in curr)
		// Added: 2 new nodes (Hotspot and VPN)
		{ID: "n3", ResID: "btn_hotspot", Class: "Button", Text: "Hotspot", Bounds: [4]int{50, 340, 1000, 440}, Flags: []string{"click"}},
		{ID: "n4", ResID: "btn_vpn", Class: "Button", Text: "VPN", Bounds: [4]int{50, 460, 1000, 560}, Flags: []string{"click"}},
	}

	diff := Diff(prev, curr)

	t.Logf("Diff summary: %s", diff.Summary)

	// Check Added count
	if len(diff.Added) != 2 {
		t.Fatalf("expected 2 added nodes, got %d", len(diff.Added))
	}
	if diff.Added[0].ResID != "btn_hotspot" || diff.Added[1].ResID != "btn_vpn" {
		t.Errorf("unexpected added nodes: %+v", diff.Added)
	}

	// Check Modified count
	if len(diff.Modified) != 1 {
		t.Fatalf("expected 1 modified node, got %d", len(diff.Modified))
	}
	if diff.Modified[0].ResID != "btn_wifi" || diff.Modified[0].Text != "Wi-Fi: On" {
		t.Errorf("unexpected modified node: %+v", diff.Modified)
	}

	// Check Removed count
	if len(diff.RemovedIDs) != 1 {
		t.Fatalf("expected 1 removed node ID, got %d", len(diff.RemovedIDs))
	}
	if diff.RemovedIDs[0] != "n3" {
		t.Errorf("expected removed ID 'n3', got %q", diff.RemovedIDs[0])
	}

	// Check Unchanged count
	if diff.UnchangedCount != 1 {
		t.Errorf("expected 1 unchanged node, got %d", diff.UnchangedCount)
	}

	// Check summary string format: e.g. "2 nodes added, 1 changed, 1 removed (1 unchanged omitted)"
	if !strings.Contains(diff.Summary, "2 nodes added") {
		t.Errorf("summary missing '2 nodes added': %q", diff.Summary)
	}
	if !strings.Contains(diff.Summary, "1 changed") {
		t.Errorf("summary missing '1 changed': %q", diff.Summary)
	}
	if !strings.Contains(diff.Summary, "1 removed") {
		t.Errorf("summary missing '1 removed': %q", diff.Summary)
	}
	if !strings.Contains(diff.Summary, "(1 unchanged omitted)") {
		t.Errorf("summary missing '(1 unchanged omitted)': %q", diff.Summary)
	}
}

func TestDiffer_AllUnchanged(t *testing.T) {
	nodes := []types.PrunedNode{
		{ID: "n1", ResID: "btn1", Class: "Button", Text: "Hello", Bounds: [4]int{0, 0, 100, 100}},
		{ID: "n2", ResID: "btn2", Class: "Button", Text: "World", Bounds: [4]int{0, 100, 100, 200}},
	}

	diff := Diff(nodes, nodes)
	if len(diff.Added) != 0 || len(diff.Modified) != 0 || len(diff.RemovedIDs) != 0 {
		t.Errorf("expected zero changes, got: %+v", diff)
	}
	if diff.UnchangedCount != 2 {
		t.Errorf("expected 2 unchanged, got %d", diff.UnchangedCount)
	}
	if diff.Summary != "All 2 nodes unchanged" {
		t.Errorf("unexpected summary: %q", diff.Summary)
	}
}

func TestDiffer_InitialScreen(t *testing.T) {
	curr := []types.PrunedNode{
		{ID: "n1", ResID: "btn1", Class: "Button", Text: "Hello", Bounds: [4]int{0, 0, 100, 100}},
	}

	diff := Diff(nil, curr)
	if len(diff.Added) != 1 {
		t.Errorf("expected 1 added on initial, got %d", len(diff.Added))
	}
	if diff.Summary != "Initial screen with 1 nodes" {
		t.Errorf("unexpected summary: %q", diff.Summary)
	}
}
