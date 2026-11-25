package compiler

import (
	"sync"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// SessionRecorder records sequential action and observation steps during an
// interactive exploration session, and provides backtrack filtering.
type SessionRecorder struct {
	mu        sync.RWMutex
	sessionID string
	appID     string
	steps     []types.SessionStep
}

// NewSessionRecorder creates a new recorder for the specified session.
func NewSessionRecorder(sessionID string) *SessionRecorder {
	return &SessionRecorder{
		sessionID: sessionID,
		steps:     make([]types.SessionStep, 0),
	}
}

// SessionID returns the recorder's session ID.
func (r *SessionRecorder) SessionID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sessionID
}

// AppID returns the recorder's associated package/app ID.
func (r *SessionRecorder) AppID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.appID
}

// SetAppID sets the associated package/app ID for this session.
func (r *SessionRecorder) SetAppID(appID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.appID = appID
}

// RecordStep records an action step in the session buffer.
// It automatically assigns StepIndex if non-positive and sets TimestampMS if zero.
func (r *SessionRecorder) RecordStep(step types.SessionStep) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if step.StepIndex <= 0 {
		step.StepIndex = len(r.steps) + 1
	}
	if step.TimestampMS == 0 {
		step.TimestampMS = time.Now().UnixMilli()
	}

	r.steps = append(r.steps, step)
}

// Steps returns a copy of all recorded steps in order.
func (r *SessionRecorder) Steps() []types.SessionStep {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cp := make([]types.SessionStep, len(r.steps))
	copy(cp, r.steps)
	return cp
}

// StepCount returns the total number of recorded steps.
func (r *SessionRecorder) StepCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.steps)
}

// Clear removes all recorded steps.
func (r *SessionRecorder) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps = make([]types.SessionStep, 0)
}

// NetSteps computes the net successful path through the session:
//  1. Skips steps that failed (Success == false).
//  2. Skips backtracks (where UIHashAfter matches the UIHashBefore of an earlier step,
//     indicating that intervening actions were undone or dead-ended back to a previous state).
func (r *SessionRecorder) NetSteps() []types.SessionStep {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return FilterBacktracks(r.steps)
}

// FilterBacktracks implements the explore-to-freeze backtrack removal algorithm:
// It walks steps, discards failed steps, and prunes sub-paths that loop back to an earlier screen hash.
func FilterBacktracks(steps []types.SessionStep) []types.SessionStep {
	var net []types.SessionStep

	for _, step := range steps {
		if !step.Success {
			continue
		}

		// Check if this step returns the UI to a state that existed before an earlier step in net.
		// If step.UIHashAfter matches net[j].UIHashBefore, then all steps from j to the end of net
		// form an exploratory loop/backtrack that returned to net[j].UIHashBefore.
		backtrackIdx := -1
		if step.UIHashAfter != "" {
			for j := 0; j < len(net); j++ {
				if net[j].UIHashBefore != "" && net[j].UIHashBefore == step.UIHashAfter {
					backtrackIdx = j
					break
				}
			}
		}

		if backtrackIdx >= 0 {
			// Prune steps from backtrackIdx onwards
			net = net[:backtrackIdx]
		} else {
			net = append(net, step)
		}
	}

	return net
}
