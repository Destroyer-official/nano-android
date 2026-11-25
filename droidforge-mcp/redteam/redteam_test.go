package redteam

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/policy"
	"github.com/anthropic/droidforge-mcp/internal/types"
)

type RedTeamCase struct {
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

func parseRiskTier(s string) types.RiskTier {
	switch s {
	case "READ":
		return types.RiskREAD
	case "WRITE_SCOPED":
		return types.RiskWRITESCOPED
	case "APPROVAL_GATED":
		return types.RiskAPPROVALGATED
	default:
		return types.RiskREAD
	}
}

func newTestGuard(workspaceRoot string) *policy.Guard {
	proj := types.ProjectConfig{
		Name:                "test-project",
		AllowlistedPackages: []string{"com.example.myapp"},
	}
	pol := types.PolicyConfig{
		Network: types.NetworkPolicy{
			BindHost:                 "127.0.0.1",
			AllowUnencryptedADBTcpip: false,
		},
		Execution: types.ExecutionPolicy{
			AllowRawShell:           false,
			EnforcePackageScopeLock: true,
		},
		Sanitization: types.SanitizationPolicy{
			WrapWithRandomNonce:   true,
			StripZeroWidthAndBidi: true,
			MaxNodeTextBytes:      256,
			MaxLogcatLineBytes:    512,
		},
		HumanApproval: []string{"register_new_physical_device", "cloud_farm_billing"},
	}
	exec := types.ExecutionConfig{
		MaxStepsPerTask:      15,
		MaxIdenticalUIHashes: 3,
	}

	g := policy.NewGuard(workspaceRoot, proj, pol, exec)
	g.SetEmulatorOnly(true)
	return g
}

func TestRedTeam_All15AttackCases(t *testing.T) {
	files, err := filepath.Glob("case*.json")
	if err != nil {
		t.Fatalf("failed to glob test cases: %v", err)
	}
	if len(files) != 15 {
		t.Fatalf("expected 15 red team cases, found %d", len(files))
	}

	tempDir := t.TempDir()

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("failed to read %s: %v", file, err)
			}

			var tc RedTeamCase
			if err := json.Unmarshal(data, &tc); err != nil {
				t.Fatalf("failed to unmarshal %s: %v", file, err)
			}

			guard := newTestGuard(tempDir)
			ctx := context.Background()
			toolDef := types.ToolDefinition{
				Name:     tc.Tool,
				RiskTier: parseRiskTier(tc.RiskTier),
			}

			switch tc.ExpectedVerdict {
			case "DENY":
				eval, err := guard.Evaluate(ctx, toolDef, tc.Args)
				if err != nil {
					t.Fatalf("unexpected evaluate error: %v", err)
				}
				if eval.Decision != types.PolicyDENY {
					t.Errorf("expected decision DENY, got %s (reason: %s)", eval.Decision.String(), eval.Reason)
				}

			case "APPROVAL_REQUIRED":
				eval, err := guard.Evaluate(ctx, toolDef, tc.Args)
				if err != nil {
					t.Fatalf("unexpected evaluate error: %v", err)
				}
				if eval.Decision != types.PolicyAPPROVALREQUIRED {
					t.Errorf("expected decision APPROVAL_REQUIRED, got %s (reason: %s)", eval.Decision.String(), eval.Reason)
				}

			case "HALT":
				sessionID := tc.SessionID
				if sessionID == "" {
					sessionID = "default"
				}
				for _, h := range tc.UIHashes {
					guard.RecordUIHash(sessionID, h)
				}
				eval, err := guard.Evaluate(ctx, toolDef, map[string]any{"session_id": sessionID})
				if err != nil {
					t.Fatalf("unexpected evaluate error: %v", err)
				}
				if eval.Decision != types.PolicyHALT {
					t.Errorf("expected decision HALT, got %s (reason: %s)", eval.Decision.String(), eval.Reason)
				}

			case "SANITIZE":
				var sanitized, nonce string
				if tc.Tool == "logcat_read" {
					sanitized, nonce = guard.FenceLogcatLine(tc.RawText)
				} else {
					sanitized, nonce = guard.FenceUntrustedText(tc.RawText)
				}

				if nonce == "" {
					t.Errorf("expected non-empty nonce")
				}
				openTag := "<DF_UNTRUSTED_" + nonce + ">"
				closeTag := "</DF_UNTRUSTED_" + nonce + ">"
				if !strings.HasPrefix(sanitized, openTag) || !strings.HasSuffix(sanitized, closeTag) {
					t.Errorf("sanitized text missing nonce boundary tags: %s", sanitized)
				}
				// Verify bidi and zero-width characters removed
				for _, r := range sanitized {
					if (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) {
						t.Errorf("found unstripped bidi character: %U", r)
					}
					if (r >= 0x200B && r <= 0x200D) || r == 0xFEFF {
						t.Errorf("found unstripped zero-width character: %U", r)
					}
				}

			default:
				t.Fatalf("unknown expected_verdict %q", tc.ExpectedVerdict)
			}
		})
	}
}
