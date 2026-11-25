// Package types defines all shared interfaces and data structures used across
// DroidForge MCP internal modules. Every worker builds against these contracts.
package types

import "context"

// RiskTier classifies every MCP tool for uniform security enforcement.
type RiskTier int

const (
	// RiskREAD is for pure observation tools with zero device mutation.
	RiskREAD RiskTier = iota
	// RiskWRITESCOPED mutates only allowlisted packages or emulators.
	RiskWRITESCOPED
	// RiskAPPROVALGATED requires explicit interactive human confirmation.
	RiskAPPROVALGATED
)

// String returns the human-readable risk tier label.
func (r RiskTier) String() string {
	switch r {
	case RiskREAD:
		return "READ"
	case RiskWRITESCOPED:
		return "WRITE_SCOPED"
	case RiskAPPROVALGATED:
		return "APPROVAL_GATED"
	default:
		return "UNKNOWN"
	}
}

// ToolDefinition describes one MCP tool for schema advertisement.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	RiskTier    RiskTier       `json:"risk_tier"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ToolResult is the unified return envelope for every tool handler.
type ToolResult struct {
	Success  bool           `json:"success"`
	Data     map[string]any `json:"data,omitempty"`
	Error    string         `json:"error,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
	AuditRef string         `json:"audit_ref,omitempty"`
}

// ToolHandler is implemented by every internal engine module to expose
// one MCP tool. The MCP router collects all handlers and dispatches calls.
type ToolHandler interface {
	// Definition returns the tool's schema for MCP tools/list advertisement.
	Definition() ToolDefinition
	// Execute runs the tool with validated arguments and returns a result.
	Execute(ctx context.Context, args map[string]any) (*ToolResult, error)
}
