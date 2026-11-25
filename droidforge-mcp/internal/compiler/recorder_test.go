package compiler

import (
	"sync"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

func TestSessionRecorder_BasicRecording(t *testing.T) {
	rec := NewSessionRecorder("test_session_1")
	rec.SetAppID("com.example.testapp")

	if rec.SessionID() != "test_session_1" {
		t.Fatalf("expected session ID test_session_1, got %s", rec.SessionID())
	}
	if rec.AppID() != "com.example.testapp" {
		t.Fatalf("expected app ID com.example.testapp, got %s", rec.AppID())
	}

	step1 := types.SessionStep{
		Action:       "tap_node",
		Target:       "n1",
		UIHashBefore: "hash_0",
		UIHashAfter:  "hash_1",
		Success:      true,
	}
	step2 := types.SessionStep{
		Action:       "type",
		Target:       "n2",
		Text:         "hello world",
		UIHashBefore: "hash_1",
		UIHashAfter:  "hash_2",
		Success:      true,
	}

	rec.RecordStep(step1)
	rec.RecordStep(step2)

	if rec.StepCount() != 2 {
		t.Fatalf("expected 2 steps, got %d", rec.StepCount())
	}

	steps := rec.Steps()
	if steps[0].StepIndex != 1 || steps[1].StepIndex != 2 {
		t.Errorf("step indices not auto-assigned correctly: %d, %d", steps[0].StepIndex, steps[1].StepIndex)
	}
	if steps[0].TimestampMS == 0 || steps[1].TimestampMS == 0 {
		t.Errorf("timestamps not auto-assigned")
	}

	rec.Clear()
	if rec.StepCount() != 0 {
		t.Errorf("expected 0 steps after clear, got %d", rec.StepCount())
	}
}

func TestSessionRecorder_BacktrackFilter(t *testing.T) {
	// Scenario:
	// Step 1: Screen A (hashA) -> Screen B (hashB) [Success]
	// Step 2: Screen B (hashB) -> Screen C (hashC) [Success] (detour start)
	// Step 3: Screen C (hashC) -> Screen D (hashD) [Success] (detour continued)
	// Step 4: Screen D (hashD) -> Screen B (hashB) [Success] (undo / back button hit)
	// Step 5: Screen B (hashB) -> Screen E (hashE) [Success] (desired path)
	// Step 6: Screen E (hashE) -> Screen F (hashF) [Failed tap] (should be omitted)
	steps := []types.SessionStep{
		{
			StepIndex:    1,
			Action:       "tap_node",
			Target:       "btn_b",
			UIHashBefore: "hashA",
			UIHashAfter:  "hashB",
			Success:      true,
		},
		{
			StepIndex:    2,
			Action:       "tap_node",
			Target:       "btn_c",
			UIHashBefore: "hashB",
			UIHashAfter:  "hashC",
			Success:      true,
		},
		{
			StepIndex:    3,
			Action:       "tap_node",
			Target:       "btn_d",
			UIHashBefore: "hashC",
			UIHashAfter:  "hashD",
			Success:      true,
		},
		{
			StepIndex:    4,
			Action:       "key",
			Target:       "back",
			UIHashBefore: "hashD",
			UIHashAfter:  "hashB", // Backtracked to state before Step 2!
			Success:      true,
		},
		{
			StepIndex:    5,
			Action:       "tap_node",
			Target:       "btn_e",
			UIHashBefore: "hashB",
			UIHashAfter:  "hashE",
			Success:      true,
		},
		{
			StepIndex:    6,
			Action:       "tap_node",
			Target:       "btn_f",
			UIHashBefore: "hashE",
			UIHashAfter:  "hashE",
			Success:      false, // Failed action
		},
	}

	net := FilterBacktracks(steps)

	// Expected net path:
	// Step 1 (A -> B)
	// Step 5 (B -> E)
	// (Steps 2, 3, 4 were pruned because Step 4 returned to hashB which was Step 2's UIHashBefore)
	// (Step 6 was omitted because Success == false)
	if len(net) != 2 {
		t.Fatalf("expected 2 net steps after backtrack filtering, got %d: %+v", len(net), net)
	}

	if net[0].Target != "btn_b" {
		t.Errorf("expected first net step target btn_b, got %s", net[0].Target)
	}
	if net[1].Target != "btn_e" {
		t.Errorf("expected second net step target btn_e, got %s", net[1].Target)
	}
}

func TestSessionRecorder_ConcurrentRecording(t *testing.T) {
	rec := NewSessionRecorder("concurrent_sess")
	var wg sync.WaitGroup

	numGoroutines := 20
	stepsPerGoroutine := 50

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for s := 0; s < stepsPerGoroutine; s++ {
				rec.RecordStep(types.SessionStep{
					Action:       "tap_node",
					Target:       "btn_test",
					UIHashBefore: "before",
					UIHashAfter:  "after",
					Success:      true,
				})
			}
		}(g)
	}

	wg.Wait()

	expectedTotal := numGoroutines * stepsPerGoroutine
	if rec.StepCount() != expectedTotal {
		t.Errorf("expected %d total steps, got %d", expectedTotal, rec.StepCount())
	}
}
