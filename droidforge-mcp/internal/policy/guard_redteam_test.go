package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

type RedTeamFixture struct {
	ID              int            `json:"id"`
	Name            string         `json:"name"`
	Attack          string         `json:"attack"`
	Description     string         `json:"description"`
	Tool            string         `json:"tool"`
	RiskTier        string         `json:"risk_tier"`
	Args            map[string]any `json:"args,omitempty"`
	RawText         string         `json:"raw_text,omitempty"`
	SessionID       string         `json:"session_id,omitempty"`
	UIHashes        []string       `json:"ui_hashes,omitempty"`
	ExpectedVerdict string         `json:"expected_verdict"`
}

func findRedteamDir() (string, error) {
	candidates := []string{
		"../../redteam",
		"../redteam",
		"./redteam",
		"redteam",
	}
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err == nil {
			if stat, err := os.Stat(abs); err == nil && stat.IsDir() {
				return abs, nil
			}
		}
	}
	return "", fmt.Errorf("redteam directory not found in candidate paths")
}

func loadAllFixtures(t *testing.T, redteamDir string) []RedTeamFixture {
	entries, err := os.ReadDir(redteamDir)
	if err != nil {
		t.Fatalf("failed to read redteam dir %s: %v", redteamDir, err)
	}

	var fixtures []RedTeamFixture
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(redteamDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read fixture %s: %v", path, err)
		}
		var f RedTeamFixture
		if err := json.Unmarshal(data, &f); err != nil {
			t.Fatalf("failed to parse fixture %s: %v", path, err)
		}
		fixtures = append(fixtures, f)
	}

	sort.Slice(fixtures, func(i, j int) bool {
		return fixtures[i].ID < fixtures[j].ID
	})

	return fixtures
}

func parseRiskTier(tierStr string) types.RiskTier {
	switch strings.ToUpper(tierStr) {
	case "READ":
		return types.RiskREAD
	case "WRITE_SCOPED":
		return types.RiskWRITESCOPED
	case "APPROVAL_GATED":
		return types.RiskAPPROVALGATED
	default:
		return types.RiskWRITESCOPED
	}
}

func TestRedTeam_15AdversarialCases(t *testing.T) {
	redteamDir, err := findRedteamDir()
	if err != nil {
		t.Fatalf("cannot locate redteam fixtures directory: %v", err)
	}

	fixtures := loadAllFixtures(t, redteamDir)
	if len(fixtures) != 15 {
		t.Fatalf("expected 15 red team fixtures, got %d", len(fixtures))
	}

	workspaceRoot := t.TempDir()

	for _, tc := range fixtures {
		tc := tc
		t.Run(fmt.Sprintf("Case%02d_%s", tc.ID, tc.Name), func(t *testing.T) {
			guard := newTestGuard(workspaceRoot)
			ctx := context.Background()

			switch {
			case (tc.ID >= 1 && tc.ID <= 9) || tc.ID == 13:
				// Assert DENY for cases 1–9 and 13
				if tc.ExpectedVerdict != "DENY" {
					t.Fatalf("case %d fixture expected_verdict must be DENY, got %s", tc.ID, tc.ExpectedVerdict)
				}
				toolDef := types.ToolDefinition{
					Name:     tc.Tool,
					RiskTier: parseRiskTier(tc.RiskTier),
				}
				eval, err := guard.Evaluate(ctx, toolDef, tc.Args)
				if err != nil {
					t.Fatalf("case %d evaluate error: %v", tc.ID, err)
				}
				if eval.Decision != types.PolicyDENY {
					t.Errorf("case %d: expected verdict DENY, got %s (reason: %s)", tc.ID, eval.Decision, eval.Reason)
				} else {
					t.Logf("case %d successfully DENIED: %s", tc.ID, eval.Reason)
				}

			case tc.ID >= 10 && tc.ID <= 12:
				// Assert SANITIZE for cases 10–12
				if tc.ExpectedVerdict != "SANITIZE" {
					t.Fatalf("case %d fixture expected_verdict must be SANITIZE, got %s", tc.ID, tc.ExpectedVerdict)
				}

				rawInput := tc.RawText
				if rawInput == "" && tc.Args != nil {
					if msg, ok := tc.Args["message"].(string); ok {
						rawInput = msg
					}
				}

				fenced, nonce := guard.FenceUntrustedText(rawInput)

				// Verify 6-byte hex nonce (12 characters)
				if len(nonce) != 12 {
					t.Errorf("case %d: expected 12-char hex nonce, got %s", tc.ID, nonce)
				}

				prefix := fmt.Sprintf("<DF_UNTRUSTED_%s>", nonce)
				suffix := fmt.Sprintf("</DF_UNTRUSTED_%s>", nonce)

				if !strings.HasPrefix(fenced, prefix) || !strings.HasSuffix(fenced, suffix) {
					t.Errorf("case %d: text not properly fenced: %s", tc.ID, fenced)
				}

				// Check bidi overrides (U+202A–U+202E, U+2066–U+2069)
				for r := rune(0x202A); r <= 0x202E; r++ {
					if strings.ContainsRune(fenced, r) {
						t.Errorf("case %d: bidi override %U not stripped", tc.ID, r)
					}
				}
				for r := rune(0x2066); r <= 0x2069; r++ {
					if strings.ContainsRune(fenced, r) {
						t.Errorf("case %d: bidi isolate %U not stripped", tc.ID, r)
					}
				}

				// Check zero-width characters (U+200B–U+200D, U+FEFF)
				for _, r := range []rune{0x200B, 0x200C, 0x200D, 0xFEFF} {
					if strings.ContainsRune(fenced, r) {
						t.Errorf("case %d: zero-width char %U not stripped", tc.ID, r)
					}
				}

				// Check length truncation (inner text <= 256 bytes)
				inner := strings.TrimSuffix(strings.TrimPrefix(fenced, prefix), suffix)
				if len([]byte(inner)) > 256 {
					t.Errorf("case %d: sanitized text length %d exceeds max 256 bytes", tc.ID, len([]byte(inner)))
				}

				t.Logf("case %d successfully SANITIZED (nonce: %s, len: %d)", tc.ID, nonce, len(inner))

			case tc.ID == 14:
				// Assert HALT for case 14 (infinite tap loop circuit breaker)
				if tc.ExpectedVerdict != "HALT" {
					t.Fatalf("case 14 fixture expected_verdict must be HALT, got %s", tc.ExpectedVerdict)
				}

				sessionID := tc.SessionID
				if sessionID == "" {
					sessionID = "session-case-14"
				}

				for _, h := range tc.UIHashes {
					guard.RecordUIHash(sessionID, h)
				}

				toolDef := types.ToolDefinition{
					Name:     tc.Tool,
					RiskTier: parseRiskTier(tc.RiskTier),
				}
				eval, err := guard.Evaluate(ctx, toolDef, map[string]any{
					"session_id": sessionID,
					"serial":     "emulator-5554",
					"package_id": "com.example.myapp",
				})
				if err != nil {
					t.Fatalf("case 14 evaluate error: %v", err)
				}
				if eval.Decision != types.PolicyHALT {
					t.Errorf("case 14: expected verdict HALT, got %s (reason: %s)", eval.Decision, eval.Reason)
				} else {
					t.Logf("case 14 successfully HALTED circuit breaker: %s", eval.Reason)
				}

			case tc.ID == 15:
				// Assert APPROVAL_REQUIRED for case 15
				if tc.ExpectedVerdict != "APPROVAL_REQUIRED" {
					t.Fatalf("case 15 fixture expected_verdict must be APPROVAL_REQUIRED, got %s", tc.ExpectedVerdict)
				}

				toolDef := types.ToolDefinition{
					Name:     tc.Tool,
					RiskTier: parseRiskTier(tc.RiskTier),
				}
				eval, err := guard.Evaluate(ctx, toolDef, tc.Args)
				if err != nil {
					t.Fatalf("case 15 evaluate error: %v", err)
				}
				if eval.Decision != types.PolicyAPPROVALREQUIRED {
					t.Errorf("case 15: expected verdict APPROVAL_REQUIRED, got %s (reason: %s)", eval.Decision, eval.Reason)
				} else {
					t.Logf("case 15 successfully triggered APPROVAL_REQUIRED: %s", eval.Reason)
				}

			default:
				t.Fatalf("unhandled case ID %d", tc.ID)
			}
		})
	}
}
