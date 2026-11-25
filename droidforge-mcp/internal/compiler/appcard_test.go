package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

func TestExportAppCard_MarkdownGeneration(t *testing.T) {
	tempDir := t.TempDir()
	outPath := filepath.Join(tempDir, "appcard.md")

	rec := NewSessionRecorder("sess_appcard_test")
	rec.SetAppID("com.example.scamshield.debug")

	// Step 1 on MainActivity
	rec.RecordStep(types.SessionStep{
		Action:  "observe",
		Target:  "activity:com.example.scamshield.MainActivity",
		Success: true,
		NodesAfter: []types.PrunedNode{
			{
				ID:    "n1",
				ResID: "com.example.scamshield:id/btn_scan",
				Class: "Button",
				Flags: []string{"click", "enabled"},
			},
			{
				ID:    "n2",
				ResID: "com.example.scamshield:id/status_card",
				Class: "View",
			},
		},
	})

	// Step 2 with deep link intent to AlertDetailActivity
	rec.RecordStep(types.SessionStep{
		Action:  "intent",
		Target:  "scamshield://alert/phish_42",
		Success: true,
	})

	// Step 3 on AlertDetailActivity
	rec.RecordStep(types.SessionStep{
		Action:  "observe",
		Target:  "activity:com.example.scamshield.AlertDetailActivity",
		Success: true,
		NodesAfter: []types.PrunedNode{
			{
				ID:    "n3",
				ResID: "com.example.scamshield:id/btn_block_sender",
				Class: "Button",
				Flags: []string{"click"},
			},
			{
				ID:    "n4",
				ResID: "com.example.scamshield:id/txt_warning_details",
				Class: "TextView",
				Text:  "Phishing SMS Detected",
			},
		},
	})

	res, err := ExportAppCard(rec, outPath)
	if err != nil {
		t.Fatalf("ExportAppCard failed: %v", err)
	}

	if res.AppCardPath != outPath {
		t.Errorf("expected AppCard path %s, got %s", outPath, res.AppCardPath)
	}
	if res.ScreensMapped < 2 {
		t.Errorf("expected at least 2 screens mapped, got %d", res.ScreensMapped)
	}
	if len(res.DeepLinksFound) != 1 || res.DeepLinksFound[0] != "scamshield://alert/phish_42" {
		t.Errorf("expected deep link scamshield://alert/phish_42, got %+v", res.DeepLinksFound)
	}

	// Verify markdown file contents
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read written appcard: %v", err)
	}
	md := string(data)

	if !strings.Contains(md, "# AppCard: com.example.scamshield.debug") {
		t.Errorf("missing header with package ID in markdown")
	}
	if !strings.Contains(md, "scamshield://alert/phish_42") {
		t.Errorf("missing deep link in markdown")
	}
	if !strings.Contains(md, "btn_scan") || !strings.Contains(md, "btn_block_sender") {
		t.Errorf("missing tracked Compose/resource test tags in markdown")
	}
}
