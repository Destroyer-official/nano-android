package types

import "time"

// ApprovalRequest represents a pending or resolved human approval request.
type ApprovalRequest struct {
	ActionID       string    `json:"action_id"`
	Action         string    `json:"action"`
	Justification  string    `json:"justification"`
	TargetResource string    `json:"target_resource"`
	Status         string    `json:"status"` // "PENDING", "APPROVED", "DENIED", "EXPIRED"
	Approver       string    `json:"approver,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}
