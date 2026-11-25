package uitree

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// generate2000NodeXML generates a realistic, deeply nested layout containing 2000 raw nodes,
// with mostly non-interactive wrapper containers and only ~20 interactive widgets.
func generate2000NodeXML() string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?><hierarchy rotation="0">`)
	sb.WriteString(`<node index="0" class="android.widget.FrameLayout" bounds="[0,0][1080,2400]">`)

	totalNodes := 1 // root node
	interactiveTarget := 20
	interactiveAdded := 0

	for b := 0; b < 10; b++ {
		sb.WriteString(fmt.Sprintf(`<node index="%d" class="android.widget.LinearLayout" bounds="[0,%d][1080,%d]">`, b, b*200, (b+1)*200))
		totalNodes++

		// Deep nesting chain of 190 wrapper containers per branch (190 * 10 = 1900 nodes)
		for d := 0; d < 190; d++ {
			cls := "android.widget.FrameLayout"
			if d%2 == 0 {
				cls = "android.widget.LinearLayout"
			}
			sb.WriteString(fmt.Sprintf(`<node index="0" class="%s" bounds="[0,%d][1080,%d]">`, cls, b*200, (b+1)*200))
			totalNodes++
		}

		// Insert 2 interactive buttons inside this branch
		if interactiveAdded < interactiveTarget {
			sb.WriteString(fmt.Sprintf(`<node index="0" text="Action %d" resource-id="com.example:id/btn_%d" class="android.widget.Button" clickable="true" enabled="true" bounds="[50,%d][500,%d]" />`, interactiveAdded, interactiveAdded, b*200+10, b*200+80))
			interactiveAdded++
			totalNodes++

			sb.WriteString(fmt.Sprintf(`<node index="1" text="Option %d" resource-id="com.example:id/chk_%d" class="android.widget.CheckBox" checkable="true" checked="false" clickable="true" enabled="true" bounds="[520,%d][900,%d]" />`, interactiveAdded, interactiveAdded, b*200+10, b*200+80))
			interactiveAdded++
			totalNodes++
		}

		// Close wrapper tags
		for d := 0; d < 190; d++ {
			sb.WriteString(`</node>`)
		}
		sb.WriteString(`</node>`)
	}

	// Pad remaining nodes up to at least 2050 nodes
	for totalNodes < 2050 {
		sb.WriteString(fmt.Sprintf(`<node index="%d" class="android.view.View" bounds="[0,0][0,0]" />`, totalNodes))
		totalNodes++
	}

	sb.WriteString(`</node></hierarchy>`)
	return sb.String()
}

func TestPruner_Reduces2000NodesToUnder30(t *testing.T) {
	rawXML := generate2000NodeXML()
	hierarchy, err := ParseXML([]byte(rawXML))
	if err != nil {
		t.Fatalf("failed to parse generated 2000-node XML: %v", err)
	}

	rawCount := len(FlattenAllNodes(hierarchy))
	if rawCount < 2000 {
		t.Fatalf("expected at least 2000 raw nodes, got %d", rawCount)
	}

	pruned := Prune(hierarchy, PruneOptions{Viewport: DefaultViewport})
	t.Logf("Raw nodes: %d -> Pruned nodes: %d", rawCount, len(pruned))

	if len(pruned) >= 30 {
		t.Errorf("expected pruned node count < 30, got %d", len(pruned))
	}
	if len(pruned) < 15 {
		t.Errorf("expected at least 15 interactive nodes to be preserved, got %d", len(pruned))
	}

	// Verify sequential IDs are formatted correctly (n1, n2, ...)
	for i, n := range pruned {
		expectedID := fmt.Sprintf("n%d", i+1)
		if n.ID != expectedID {
			t.Errorf("node %d: expected ID %q, got %q", i, expectedID, n.ID)
		}
	}
}

func TestPruner_CollapseRuleHoisting(t *testing.T) {
	// Sample 3 has FrameLayout -> LinearLayout -> TextView("Block & Notify") inside Button
	hierarchy, err := ParseXML([]byte(sample3ScamShieldAlert))
	if err != nil {
		t.Fatalf("failed to parse sample 3: %v", err)
	}

	pruned := Prune(hierarchy, PruneOptions{Viewport: DefaultViewport})

	// Find the block sender button
	var blockBtn *types.PrunedNode
	for _, n := range pruned {
		if n.ResID == "btn_block_sender" {
			nCopy := n
			blockBtn = &nCopy
			break
		}
	}

	if blockBtn == nil {
		t.Fatalf("btn_block_sender not found in pruned output")
	}

	// Verify text was hoisted to the button
	if !strings.Contains(blockBtn.Text, "Block & Notify") {
		t.Errorf("expected text 'Block & Notify' hoisted to Button, got %q", blockBtn.Text)
	}

	// Verify button class is simplified to "Button"
	if blockBtn.Class != "Button" {
		t.Errorf("expected class 'Button', got %q", blockBtn.Class)
	}

	// Verify flags contain "click" and "enabled"
	hasClick := false
	hasEnabled := false
	for _, f := range blockBtn.Flags {
		if f == "click" {
			hasClick = true
		}
		if f == "enabled" {
			hasEnabled = true
		}
	}
	if !hasClick || !hasEnabled {
		t.Errorf("expected flags to contain 'click' and 'enabled', got %v", blockBtn.Flags)
	}
}

func TestPruner_TokenReductionRatioAcross10Screens(t *testing.T) {
	// 10 distinct sample screens
	screens := []string{
		sample1SettingsMain,
		sample2WiFiSettings,
		sample3ScamShieldAlert,
		sample4VideoCompressor,
		sample5ComposeUI,
		generate2000NodeXML(),
		// Screen 7: Dialog screen
		`<?xml version='1.0'?><hierarchy rotation="0"><node class="android.widget.FrameLayout" bounds="[0,0][1080,2400]"><node class="android.app.Dialog" bounds="[100,500][980,1200]"><node text="Delete Item?" class="android.widget.TextView" bounds="[150,550][900,650]"/><node text="Are you sure?" class="android.widget.TextView" bounds="[150,670][900,750]"/><node text="Cancel" class="android.widget.Button" clickable="true" enabled="true" bounds="[500,800][700,900]"/><node text="Delete" class="android.widget.Button" clickable="true" enabled="true" bounds="[720,800][920,900]"/></node></node></hierarchy>`,
		// Screen 8: Form screen
		`<?xml version='1.0'?><hierarchy rotation="0"><node class="android.widget.FrameLayout" bounds="[0,0][1080,2400]"><node class="android.widget.LinearLayout" bounds="[0,0][1080,2400]"><node text="First Name" class="android.widget.EditText" focusable="true" clickable="true" enabled="true" bounds="[50,100][1030,220]"/><node text="Last Name" class="android.widget.EditText" focusable="true" clickable="true" enabled="true" bounds="[50,240][1030,360]"/><node text="Submit" class="android.widget.Button" clickable="true" enabled="true" bounds="[50,400][1030,520]"/></node></node></hierarchy>`,
		// Screen 9: Media Player
		`<?xml version='1.0'?><hierarchy rotation="0"><node class="android.widget.FrameLayout" bounds="[0,0][1080,2400]"><node class="android.widget.LinearLayout" bounds="[0,0][1080,2400]"><node text="Track 1" class="android.widget.TextView" bounds="[50,100][1030,200]"/><node class="android.widget.SeekBar" clickable="true" enabled="true" bounds="[50,220][1030,300]"/><node text="Play" class="android.widget.Button" clickable="true" enabled="true" bounds="[400,320][680,440]"/></node></node></hierarchy>`,
		// Screen 10: Settings deep list
		`<?xml version='1.0'?><hierarchy rotation="0"><node class="android.widget.FrameLayout" bounds="[0,0][1080,2400]"><node class="androidx.recyclerview.widget.RecyclerView" scrollable="true" bounds="[0,0][1080,2400]"><node text="Option 1" class="android.widget.TextView" clickable="true" enabled="true" bounds="[50,100][1030,200]"/><node text="Option 2" class="android.widget.TextView" clickable="true" enabled="true" bounds="[50,220][1030,320]"/><node text="Option 3" class="android.widget.TextView" clickable="true" enabled="true" bounds="[50,340][1030,440]"/></node></node></hierarchy>`,
	}

	var totalRawBytes int
	var totalPrunedBytes int

	for i, s := range screens {
		h, err := ParseXML([]byte(s))
		if err != nil {
			t.Fatalf("failed to parse screen %d: %v", i+1, err)
		}

		pruned := Prune(h, PruneOptions{Viewport: DefaultViewport})
		prunedJSON, err := json.Marshal(pruned)
		if err != nil {
			t.Fatalf("failed to marshal pruned JSON: %v", err)
		}

		rawBytes := len(s)
		prunedBytes := len(prunedJSON)
		ratio := float64(rawBytes) / float64(prunedBytes)

		t.Logf("Screen %d: Raw %d bytes -> Pruned %d bytes (Ratio: %.2fx)", i+1, rawBytes, prunedBytes, ratio)

		totalRawBytes += rawBytes
		totalPrunedBytes += prunedBytes
	}

	overallRatio := float64(totalRawBytes) / float64(totalPrunedBytes)
	t.Logf("Total Raw: %d bytes | Total Pruned: %d bytes | Overall Reduction Ratio: %.2fx", totalRawBytes, totalPrunedBytes, overallRatio)

	// Done Criteria: Pruner reduces token count by 5–8x vs raw XML
	if overallRatio < 5.0 {
		t.Errorf("expected token/byte reduction ratio >= 5.0x, got %.2fx", overallRatio)
	}
}

func TestPruner_ViewportClipping(t *testing.T) {
	// A screen with offscreen nodes beyond height 2400
	xmlData := `<?xml version="1.0"?><hierarchy rotation="0">
	<node class="android.widget.FrameLayout" bounds="[0,0][1080,2400]">
		<node text="Visible Header" class="android.widget.TextView" bounds="[0,50][1080,150]" />
		<node text="Offscreen Scrolled Item" class="android.widget.TextView" bounds="[0,2500][1080,2600]" />
	</node>
	</hierarchy>`

	h, err := ParseXML([]byte(xmlData))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	pruned := Prune(h, PruneOptions{Viewport: DefaultViewport})
	for _, n := range pruned {
		if strings.Contains(n.Text, "Offscreen") {
			t.Errorf("expected offscreen item to be clipped, but found: %+v", n)
		}
	}
}
