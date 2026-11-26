package policy

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// ApprovalHandler implements types.ToolHandler for approval_request_status (tool #26).
type ApprovalHandler struct {
	mu           sync.Mutex
	emulatorOnly bool
	in           io.Reader
	decisions    map[string]string // action_id -> status ("APPROVED", "DENIED", "PENDING")
}

// ApprovalManager wraps ApprovalHandler for subsystems that require a manager handle.
type ApprovalManager struct {
	*ApprovalHandler
}

// NewApprovalManager creates an ApprovalManager with default emulator auto-approval.
func NewApprovalManager() *ApprovalManager {
	return &ApprovalManager{
		ApprovalHandler: newApprovalHandlerInternal(true, nil),
	}
}

func newApprovalHandlerInternal(emulatorOnly bool, in io.Reader) *ApprovalHandler {
	// Never default `in` to os.Stdin: in stdio MCP mode, os.Stdin is owned by the JSON-RPC loop.
	return &ApprovalHandler{
		emulatorOnly: emulatorOnly,
		in:           in,
		decisions:    make(map[string]string),
	}
}

// NewApprovalHandler creates a new approval_request_status tool handler.
// It flexibily accepts:
// - NewApprovalHandler(mgr *ApprovalManager)
// - NewApprovalHandler(emulatorOnly bool, in io.Reader)
// - NewApprovalHandler()
func NewApprovalHandler(args ...any) *ApprovalHandler {
	if len(args) == 0 {
		return newApprovalHandlerInternal(true, nil)
	}
	if len(args) == 1 {
		if mgr, ok := args[0].(*ApprovalManager); ok && mgr != nil {
			return mgr.ApprovalHandler
		}
		if h, ok := args[0].(*ApprovalHandler); ok && h != nil {
			return h
		}
		if emu, ok := args[0].(bool); ok {
			return newApprovalHandlerInternal(emu, nil)
		}
	}
	if len(args) >= 2 {
		emu, _ := args[0].(bool)
		in, _ := args[1].(io.Reader)
		return newApprovalHandlerInternal(emu, in)
	}
	return newApprovalHandlerInternal(true, nil)
}

// Definition returns the tool schema for approval_request_status.
func (h *ApprovalHandler) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "approval_request_status",
		Description: "Check or update interactive human approval status for high-risk device or infrastructure operations",
		RiskTier:    types.RiskAPPROVALGATED,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action_id": map[string]any{
					"type":        "string",
					"description": "Unique identifier for the action requiring approval",
				},
				"justification": map[string]any{
					"type":        "string",
					"description": "Justification for executing the action",
				},
				"target_resource": map[string]any{
					"type":        "string",
					"description": "Target device serial or infrastructure resource",
				},
			},
			"required": []string{"action_id"},
		},
	}
}

// SetDecision records an explicit decision for an action ID (useful for programmatic approval and tests).
func (h *ApprovalHandler) SetDecision(actionID, status string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.decisions[actionID] = strings.ToUpper(status)
}

func (h *ApprovalHandler) isEmulatorResource(target string) bool {
	t := strings.TrimSpace(target)
	if t == "" {
		return true
	}
	if strings.HasPrefix(t, "emulator-") || strings.HasPrefix(t, "127.0.0.1:") || t == "emulator" {
		return true
	}
	return false
}

// Execute handles the approval_request_status tool invocation.
func (h *ApprovalHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	actionID, _ := args["action_id"].(string)
	if strings.TrimSpace(actionID) == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "action_id is required",
		}, nil
	}

	targetResource, _ := args["target_resource"].(string)

	// For emulator-only mode: auto-approve emulator operations
	if h.emulatorOnly && h.isEmulatorResource(targetResource) {
		return &types.ToolResult{
			Success: true,
			Data: map[string]any{
				"action_id":  actionID,
				"status":     "APPROVED",
				"approver":   "auto:emulator-policy",
				"expires_at": time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339),
			},
		}, nil
	}

	// Check if a decision was already recorded
	if decision, ok := h.decisions[actionID]; ok {
		return &types.ToolResult{
			Success: true,
			Data: map[string]any{
				"action_id":  actionID,
				"status":     decision,
				"approver":   "human:recorded",
				"expires_at": time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339),
			},
		}, nil
	}

	// If an interactive reader is configured and readable, read prompt line
	if h.in != nil {
		scanner := bufio.NewScanner(h.in)
		if scanner.Scan() {
			text := strings.TrimSpace(strings.ToLower(scanner.Text()))
			if text == "approve" || text == "yes" || text == "y" {
				h.decisions[actionID] = "APPROVED"
				return &types.ToolResult{
					Success: true,
					Data: map[string]any{
						"action_id":  actionID,
						"status":     "APPROVED",
						"approver":   "human:stdin",
						"expires_at": time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339),
					},
				}, nil
			} else if text == "deny" || text == "no" || text == "n" {
				h.decisions[actionID] = "DENIED"
				return &types.ToolResult{
					Success: true,
					Data: map[string]any{
						"action_id":  actionID,
						"status":     "DENIED",
						"approver":   "human:stdin",
						"expires_at": time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339),
					},
				}, nil
			}
		}
	}

	// Check out-of-band approvals file if configured (avoids reading from MCP stdio)
	if approvalsFile := strings.TrimSpace(os.Getenv("DROIDFORGE_APPROVALS_FILE")); approvalsFile != "" {
		if raw, err := os.ReadFile(approvalsFile); err == nil {
			var fileDecisions map[string]string
			if json.Unmarshal(raw, &fileDecisions) == nil {
				if d, ok := fileDecisions[actionID]; ok && d != "" {
					status := strings.ToUpper(strings.TrimSpace(d))
					h.decisions[actionID] = status
					return &types.ToolResult{
						Success: true,
						Data: map[string]any{
							"action_id":  actionID,
							"status":     status,
							"approver":   "human:approvals-file",
							"expires_at": time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339),
						},
					}, nil
				}
			}
		}
	}

	// Default to PENDING if approval is required and no interactive input arrived
	return &types.ToolResult{
		Success: true,
		Data: map[string]any{
			"action_id":  actionID,
			"status":     "PENDING",
			"approver":   "pending_human",
			"message":    "Human approval required out-of-band; record decision via DROIDFORGE_APPROVALS_FILE or SetDecision before retrying",
			"expires_at": time.Now().UTC().Add(15 * time.Minute).Format(time.RFC3339),
		},
	}, nil
}
