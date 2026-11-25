package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
	"gopkg.in/yaml.v3"
)

func TestResolveSelector_PriorityRubric(t *testing.T) {
	// Case 1: Node with ResID (Gold Standard)
	nodeWithResID := types.PrunedNode{
		ID:          "n1",
		ResID:       "com.example.app:id/btn_submit",
		Class:       "Button",
		Text:        "Submit 2026",
		ContentDesc: "Submit Form",
		Bounds:      [4]int{100, 200, 300, 250},
	}
	sel1 := ResolveSelector("n1", []types.PrunedNode{nodeWithResID}, 1)
	if sel1.SelectorType != "id" || sel1.Value != "btn_submit" {
		t.Errorf("expected id:btn_submit, got %s:%s", sel1.SelectorType, sel1.Value)
	}
	if sel1.Warning != "" {
		t.Errorf("expected no warning for gold standard id, got %s", sel1.Warning)
	}

	// Case 2: Node without ResID, but unique static ContentDesc
	nodeWithDesc := types.PrunedNode{
		ID:          "n2",
		Class:       "ImageButton",
		ContentDesc: "Navigate Up",
		Text:        "12:00 PM Update",
		Bounds:      [4]int{10, 20, 50, 60},
	}
	sel2 := ResolveSelector("n2", []types.PrunedNode{nodeWithDesc}, 2)
	if sel2.SelectorType != "text" || sel2.Value != "Navigate Up" {
		t.Errorf("expected text:'Navigate Up', got %s:%s", sel2.SelectorType, sel2.Value)
	}
	if sel2.Warning != "" {
		t.Errorf("expected no warning for unique content description, got %s", sel2.Warning)
	}

	// Case 3: Node with only visible text, but dynamic (contains digits)
	nodeDynamicText := types.PrunedNode{
		ID:     "n3",
		Class:  "TextView",
		Text:   "Balance: $450.00",
		Bounds: [4]int{50, 100, 200, 140},
	}
	sel3 := ResolveSelector("n3", []types.PrunedNode{nodeDynamicText}, 3)
	if sel3.SelectorType != "text" || sel3.Value != "Balance: $450.00" {
		t.Errorf("expected text:'Balance: $450.00', got %s:%s", sel3.SelectorType, sel3.Value)
	}
	if sel3.Warning == "" || !strings.Contains(sel3.Warning, "may be dynamic") {
		t.Errorf("expected weak_selector_warning for dynamic text with numbers, got %q", sel3.Warning)
	}

	// Case 4: Node with text exceeding 32 chars
	nodeLongText := types.PrunedNode{
		ID:     "n4",
		Class:  "TextView",
		Text:   "This is an exceptionally long label text designed to exceed thirty-two characters limit",
		Bounds: [4]int{50, 150, 400, 200},
	}
	sel4 := ResolveSelector("n4", []types.PrunedNode{nodeLongText}, 4)
	if sel4.Warning == "" || !strings.Contains(sel4.Warning, "exceeds 32 characters") {
		t.Errorf("expected weak_selector_warning for text > 32 chars, got %q", sel4.Warning)
	}

	// Case 5: Node with no ID, no desc, no text -> Coordinate point
	nodeBlank := types.PrunedNode{
		ID:     "n5",
		Class:  "View",
		Bounds: [4]int{540, 1200, 540, 1200},
	}
	sel5 := ResolveSelector("n5", []types.PrunedNode{nodeBlank}, 5)
	if sel5.SelectorType != "point" {
		t.Errorf("expected point selector for blank node, got %s", sel5.SelectorType)
	}
	if sel5.Warning == "" || !strings.Contains(sel5.Warning, "raw coordinates") {
		t.Errorf("expected weak_selector_warning for raw coordinates, got %q", sel5.Warning)
	}
}

func TestCompileToMaestro_ValidYAMLAndHooks(t *testing.T) {
	steps := []types.SessionStep{
		{
			StepIndex: 1,
			Action:    "tap_node",
			Target:    "btn_allow",
			Success:   true,
			NodesAfter: []types.PrunedNode{
				{
					ID:    "n1",
					ResID: "com.example.scamshield:id/btn_allow",
					Class: "Button",
					Text:  "Allow Access",
				},
				{
					ID:    "n2",
					ResID: "com.example.scamshield:id/input_phone",
					Class: "EditText",
				},
			},
		},
		{
			StepIndex: 2,
			Action:    "type",
			Target:    "input_phone",
			Text:      "+15551234567",
			Success:   true,
		},
		{
			StepIndex: 3,
			Action:    "sim_inject_sms_or_call",
			Target:    "+15551234567",
			Text:      "Urgent alert: verify at http://phish.example",
			Success:   true,
		},
		{
			StepIndex: 4,
			Action:    "key",
			Target:    "back",
			Success:   true,
		},
		{
			StepIndex: 5,
			Action:    "intent",
			Target:    "scamshield://alerts/latest",
			Success:   true,
		},
	}

	yamlStr, hooksBytes, warnings, err := CompileToMaestro("com.example.scamshield.debug", "Scam Alert Verification", steps)
	if err != nil {
		t.Fatalf("CompileToMaestro failed: %v", err)
	}

	// 1. Verify YAML validity
	parts := strings.Split(yamlStr, "---\n")
	if len(parts) != 2 {
		t.Fatalf("expected YAML header and body separated by '---', got %d parts", len(parts))
	}

	var header MaestroFlow
	if err := yaml.Unmarshal([]byte(parts[0]), &header); err != nil {
		t.Fatalf("failed to parse YAML header: %v", err)
	}
	if header.AppID != "com.example.scamshield.debug" {
		t.Errorf("expected appId com.example.scamshield.debug, got %s", header.AppID)
	}
	if len(header.Tags) == 0 || header.Tags[0] != "frozen-by-droidforge" {
		t.Errorf("expected tag frozen-by-droidforge, got %+v", header.Tags)
	}

	var body []map[string]any
	if err := yaml.Unmarshal([]byte(parts[1]), &body); err != nil {
		t.Fatalf("failed to parse YAML body: %v", err)
	}
	if len(body) < 4 {
		t.Errorf("expected at least 4 commands in body, got %d", len(body))
	}

	// 2. Verify companion hooks generated for sim_inject_sms_or_call
	if len(hooksBytes) == 0 {
		t.Fatalf("expected companion hooks JSON to be generated for external stimulus step")
	}

	var manifest HooksManifest
	if err := json.Unmarshal(hooksBytes, &manifest); err != nil {
		t.Fatalf("failed to unmarshal hooks JSON: %v", err)
	}
	if len(manifest.Hooks) != 1 {
		t.Fatalf("expected 1 hook step, got %d", len(manifest.Hooks))
	}
	if manifest.Hooks[0].Action != "sim_inject_sms_or_call" {
		t.Errorf("expected hook action sim_inject_sms_or_call, got %s", manifest.Hooks[0].Action)
	}

	_ = warnings
}

func TestFreezeSessionToMaestro_FileWriting(t *testing.T) {
	tempDir := t.TempDir()
	outYAML := filepath.Join(tempDir, "flows", "test_flow.yaml")

	rec := NewSessionRecorder("sess_freeze_test")
	rec.SetAppID("com.example.app")
	rec.RecordStep(types.SessionStep{
		Action:       "tap_node",
		Target:       "btn_start",
		UIHashBefore: "h0",
		UIHashAfter:  "h1",
		Success:      true,
	})
	rec.RecordStep(types.SessionStep{
		Action:       "sim_post_notification",
		Target:       "Security Alert",
		Text:         "Phishing detected",
		UIHashBefore: "h1",
		UIHashAfter:  "h1",
		Success:      true,
	})

	res, err := FreezeSessionToMaestro(rec, "TestFlow", outYAML)
	if err != nil {
		t.Fatalf("FreezeSessionToMaestro failed: %v", err)
	}

	if res.YAMLPath != outYAML {
		t.Errorf("expected YAML path %s, got %s", outYAML, res.YAMLPath)
	}
	if res.StepCount != 2 {
		t.Errorf("expected 2 steps, got %d", res.StepCount)
	}
	if res.HooksJSONPath == "" {
		t.Errorf("expected companion hooks path, got empty")
	}

	// Verify both files exist on disk
	if _, err := os.Stat(outYAML); os.IsNotExist(err) {
		t.Errorf("emitted YAML file does not exist at %s", outYAML)
	}
	if _, err := os.Stat(res.HooksJSONPath); os.IsNotExist(err) {
		t.Errorf("emitted companion hooks file does not exist at %s", res.HooksJSONPath)
	}
}
