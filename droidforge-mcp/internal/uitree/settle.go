package uitree

import (
	"context"
	"fmt"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

const (
	// DefaultSettleInterval is the polling interval for UI stability checks.
	DefaultSettleInterval = 100 * time.Millisecond
	// DefaultSettleTimeout is the hard cutoff if a UI fails to settle.
	DefaultSettleTimeout = 1500 * time.Millisecond
)

// TreePoller is a function that acquires and returns the current UI snapshot.
type TreePoller func(ctx context.Context) (*types.UITreeSnapshot, error)

// SettleDetector polls UI tree stability using noise-masked hashes.
type SettleDetector struct {
	PollInterval time.Duration
	Timeout      time.Duration
}

// NewSettleDetector creates a detector with default or specified interval/timeout.
func NewSettleDetector(pollInterval, timeout time.Duration) *SettleDetector {
	if pollInterval <= 0 {
		pollInterval = DefaultSettleInterval
	}
	if timeout <= 0 {
		timeout = DefaultSettleTimeout
	}
	return &SettleDetector{
		PollInterval: pollInterval,
		Timeout:      timeout,
	}
}

// WaitForSettle polls the UI tree until 2 consecutive identical ui_hash values are observed,
// or until the timeout is reached.
func (d *SettleDetector) WaitForSettle(ctx context.Context, poller TreePoller, overrideTimeout time.Duration) (SettleResult, error) {
	timeout := d.Timeout
	if overrideTimeout > 0 {
		timeout = overrideTimeout
	}

	startTime := time.Now()

	// Initial acquisition
	firstSnap, err := poller(ctx)
	if err != nil {
		return SettleResult{DurationMS: time.Since(startTime).Milliseconds(), Settled: false}, fmt.Errorf("initial UI poll failed: %w", err)
	}

	firstHash := firstSnap.Hash
	if firstHash == "" {
		firstHash = ComputeUIHash(firstSnap.Nodes)
		firstSnap.Hash = firstHash
	}

	lastSnap := firstSnap
	lastHash := firstHash

	ticker := time.NewTicker(d.PollInterval)
	defer ticker.Stop()

	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return SettleResult{
				Snapshot:   lastSnap,
				DurationMS: time.Since(startTime).Milliseconds(),
				Settled:    false,
			}, ctx.Err()

		case <-timeoutTimer.C:
			// Hard timeout reached
			return SettleResult{
				Snapshot:   lastSnap,
				DurationMS: time.Since(startTime).Milliseconds(),
				Settled:    false,
			}, nil

		case <-ticker.C:
			// Check if total elapsed time already exceeded timeout
			if time.Since(startTime) >= timeout {
				return SettleResult{
					Snapshot:   lastSnap,
					DurationMS: time.Since(startTime).Milliseconds(),
					Settled:    false,
				}, nil
			}

			currSnap, err := poller(ctx)
			if err != nil {
				// Retry on transient poll error until timeout
				continue
			}

			currHash := currSnap.Hash
			if currHash == "" {
				currHash = ComputeUIHash(currSnap.Nodes)
				currSnap.Hash = currHash
			}

			// Check for 2 consecutive identical hashes
			if currHash == lastHash {
				return SettleResult{
					Snapshot:   currSnap,
					DurationMS: time.Since(startTime).Milliseconds(),
					Settled:    true,
				}, nil
			}

			lastHash = currHash
			lastSnap = currSnap
		}
	}
}
