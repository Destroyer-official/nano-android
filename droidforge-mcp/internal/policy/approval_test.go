package policy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

func TestApprovalHandler_AutoApproveEmulator(t *testing.T) {
	handler := NewApprovalHandler(true, nil)

	ctx := context.Background()
	args := map[string]any{
		"action_id":       "action-emu-001",
		"target_resource": "emulator-5554",
		"justification":   "Launch sample app on emulator",
	}

	res, err := handler.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got error: %s", res.Error)
	}
	if res.Data["status"] != "APPROVED" {
		t.Errorf("expected APPROVED, got %v", res.Data["status"])
	}
	if res.Data["approver"] != "auto:emulator-policy" {
		t.Errorf("expected auto:emulator-policy approver, got %v", res.Data["approver"])
	}
}

func TestApprovalHandler_RecordedDecision(t *testing.T) {
	handler := NewApprovalHandler(true, nil)
	handler.SetDecision("action-cloud-001", "APPROVED")

	ctx := context.Background()
	args := map[string]any{
		"action_id":       "action-cloud-001",
		"target_resource": "cloud_farm_device",
	}

	res, err := handler.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Data["status"] != "APPROVED" {
		t.Errorf("expected APPROVED, got %v", res.Data["status"])
	}
}

func TestApprovalHandler_StdinPrompt(t *testing.T) {
	in := strings.NewReader("approve\n")
	handler := NewApprovalHandler(false, in)

	ctx := context.Background()
	args := map[string]any{
		"action_id":       "action-human-001",
		"target_resource": "pixel-physical-01",
	}

	res, err := handler.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Data["status"] != "APPROVED" {
		t.Errorf("expected APPROVED from stdin, got %v", res.Data["status"])
	}
	if res.Data["approver"] != "human:stdin" {
		t.Errorf("expected human:stdin approver, got %v", res.Data["approver"])
	}
}

func TestApprovalHandler_PendingDefault(t *testing.T) {
	// Empty reader -> EOF -> returns PENDING
	in := strings.NewReader("")
	handler := NewApprovalHandler(false, in)

	ctx := context.Background()
	args := map[string]any{
		"action_id":       "action-pending-001",
		"target_resource": "cloud_farm_billing",
	}

	res, err := handler.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Data["status"] != "PENDING" {
		t.Errorf("expected PENDING status, got %v", res.Data["status"])
	}
}

func TestApprovalHandler_Definition(t *testing.T) {
	handler := NewApprovalHandler(true, nil)
	def := handler.Definition()
	if def.Name != "approval_request_status" {
		t.Errorf("expected tool name approval_request_status, got %s", def.Name)
	}
	if def.RiskTier != types.RiskAPPROVALGATED {
		t.Errorf("expected RiskAPPROVALGATED, got %v", def.RiskTier)
	}
}

func TestApprovalHandler_ApprovalsFile_NoStdin(t *testing.T) {
	tmpDir := t.TempDir()
	approvalsPath := filepath.Join(tmpDir, "approvals.json")
	if err := os.WriteFile(approvalsPath, []byte(`{"action-file-001":"APPROVED"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DROIDFORGE_APPROVALS_FILE", approvalsPath)

	// nil reader must never fall back to os.Stdin, and must honor DROIDFORGE_APPROVALS_FILE
	handler := NewApprovalHandler(false, nil)
	res, err := handler.Execute(context.Background(), map[string]any{
		"action_id":       "action-file-001",
		"target_resource": "pixel-physical-01",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Data["status"] != "APPROVED" {
		t.Errorf("expected APPROVED from file, got %v", res.Data["status"])
	}
	if res.Data["approver"] != "human:approvals-file" {
		t.Errorf("expected human:approvals-file approver, got %v", res.Data["approver"])
	}

	// Unlisted action returns PENDING immediately without blocking on os.Stdin
	resPending, err := handler.Execute(context.Background(), map[string]any{
		"action_id":       "action-unlisted-002",
		"target_resource": "pixel-physical-01",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resPending.Data["status"] != "PENDING" {
		t.Errorf("expected PENDING, got %v", resPending.Data["status"])
	}
}
