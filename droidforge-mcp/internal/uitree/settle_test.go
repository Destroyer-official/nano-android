package uitree

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

func TestSettleDetector_ConvergesOnStaticScreenInUnder200ms(t *testing.T) {
	detector := NewSettleDetector(100*time.Millisecond, 1500*time.Millisecond)

	staticSnapshot := &types.UITreeSnapshot{
		Hash: "9f3a1c",
		Nodes: []types.PrunedNode{
			{ID: "n1", Class: "Button", Text: "Static Button"},
		},
	}

	pollCount := 0
	poller := func(ctx context.Context) (*types.UITreeSnapshot, error) {
		pollCount++
		return staticSnapshot, nil
	}

	ctx := context.Background()
	startTime := time.Now()
	res, err := detector.WaitForSettle(ctx, poller, 1500*time.Millisecond)
	elapsed := time.Since(startTime)

	if err != nil {
		t.Fatalf("WaitForSettle failed: %v", err)
	}

	if !res.Settled {
		t.Errorf("expected screen to settle, got Settled = false")
	}

	t.Logf("Settle completed in %d ms (%v wall-clock) after %d polls", res.DurationMS, elapsed, pollCount)

	// Done Criteria / Test Plan: converges on a static screen in <200ms
	if elapsed >= 200*time.Millisecond {
		t.Errorf("expected settle convergence in <200ms, took %v", elapsed)
	}
	if pollCount != 2 {
		t.Errorf("expected exactly 2 polls for 2 consecutive identical hashes, got %d", pollCount)
	}
}

func TestSettleDetector_TransitionsThenSettles(t *testing.T) {
	detector := NewSettleDetector(50*time.Millisecond, 1500*time.Millisecond)

	pollCount := 0
	poller := func(ctx context.Context) (*types.UITreeSnapshot, error) {
		pollCount++
		// Poll 1: hash A
		// Poll 2: hash B
		// Poll 3: hash B -> settles here!
		hash := "aaaaaa"
		if pollCount >= 2 {
			hash = "bbbbbb"
		}
		return &types.UITreeSnapshot{
			Hash:  hash,
			Nodes: []types.PrunedNode{{ID: "n1", Text: hash}},
		}, nil
	}

	res, err := detector.WaitForSettle(context.Background(), poller, 1500*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForSettle failed: %v", err)
	}

	if !res.Settled {
		t.Errorf("expected settled = true")
	}
	if res.Snapshot.Hash != "bbbbbb" {
		t.Errorf("expected settled hash 'bbbbbb', got %q", res.Snapshot.Hash)
	}
	if pollCount != 3 {
		t.Errorf("expected 3 polls, got %d", pollCount)
	}
}

func TestSettleDetector_TimeoutHandling(t *testing.T) {
	detector := NewSettleDetector(30*time.Millisecond, 100*time.Millisecond)

	pollCount := 0
	poller := func(ctx context.Context) (*types.UITreeSnapshot, error) {
		pollCount++
		// Always changing hash
		return &types.UITreeSnapshot{
			Hash:  fmt.Sprintf("hash%04d", pollCount),
			Nodes: []types.PrunedNode{{ID: "n1"}},
		}, nil
	}

	res, err := detector.WaitForSettle(context.Background(), poller, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil error on timeout, got %v", err)
	}

	if res.Settled {
		t.Errorf("expected Settled = false due to timeout")
	}
	if res.DurationMS < 90 {
		t.Errorf("expected duration near timeout (100ms), got %d ms", res.DurationMS)
	}
}
