package types

import (
	"context"
	"time"
)

// PolicyDecision is the outcome of the security guard evaluating a tool call.
type PolicyDecision int

const (
	PolicyALLOW PolicyDecision = iota
	PolicyDENY
	PolicyAPPROVALREQUIRED
	PolicyHALT // Circuit breaker triggered
)

// String returns the human-readable policy decision label.
func (p PolicyDecision) String() string {
	switch p {
	case PolicyALLOW:
		return "ALLOW"
	case PolicyDENY:
		return "DENY"
	case PolicyAPPROVALREQUIRED:
		return "APPROVAL_REQUIRED"
	case PolicyHALT:
		return "HALT"
	default:
		return "UNKNOWN"
	}
}

// PolicyEvaluation contains the full result of a policy guard check.
type PolicyEvaluation struct {
	Decision PolicyDecision `json:"decision"`
	Reason   string         `json:"reason,omitempty"`
	Nonce    string         `json:"nonce,omitempty"` // For untrusted-data fencing
}

// AuditRecord is one append-only entry in the JSONL audit log.
type AuditRecord struct {
	Timestamp    time.Time      `json:"ts"`
	SessionID    string         `json:"session_id"`
	ClientIDE    string         `json:"client_ide"`
	Tool         string         `json:"tool"`
	RiskTier     string         `json:"risk_tier"`
	DeviceSerial string         `json:"device_serial,omitempty"`
	PackageLock  string         `json:"package_lock,omitempty"`
	ArgsRedacted map[string]any `json:"args_redacted,omitempty"`
	Decision     string         `json:"policy_decision"`
	ExitCode     int            `json:"exit_code"`
	DurationMS   int64          `json:"duration_ms"`
	UIHashAfter  string         `json:"ui_hash_after,omitempty"`
	Artifacts    []string       `json:"artifacts,omitempty"`
}

// PolicyGuard is called by the MCP router before every tool execution.
// It enforces package scope locks, argument validation, circuit breakers,
// and nonce-fences all device-originated text on return.
type PolicyGuard interface {
	// Evaluate checks whether a tool call should be allowed.
	Evaluate(ctx context.Context, tool ToolDefinition, args map[string]any) (*PolicyEvaluation, error)

	// FenceUntrustedText wraps device-sourced text in random nonce tags
	// after stripping bidi/zero-width characters and truncating.
	FenceUntrustedText(raw string) (fenced string, nonce string)

	// IncrementStepCounter tracks steps per session for circuit breaker.
	IncrementStepCounter(sessionID string)

	// RecordUIHash tracks UI hashes for stuck-loop detection.
	RecordUIHash(sessionID, hash string) (identicalStreak int)
}

// AuditLogger records every tool call for forensic replay.
type AuditLogger interface {
	// Log writes one audit record to the append-only JSONL ledger.
	Log(record AuditRecord) error
}
