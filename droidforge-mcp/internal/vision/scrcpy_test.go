package vision

import (
	"testing"
)

func TestScrcpyStateMachine(t *testing.T) {
	// Verify state constants
	states := []SessionState{
		StateIdle,
		StateDeploying,
		StateStreaming,
		StateReconnecting,
		StateFallbackScreencap,
		StateTornDown,
	}

	for _, s := range states {
		if s == "" {
			t.Errorf("empty session state found")
		}
	}
}

func TestScrcpyBackendIsStreaming(t *testing.T) {
	backend := NewScrcpyBackend(ScrcpyConfig{
		JarPath: "non-existent-jar.jar",
	})

	if backend.IsStreaming("device-1") {
		t.Error("expected IsStreaming=false for uninitialized device")
	}

	// Deploy will fail because jar doesn't exist, falling back
	err := backend.Deploy("device-1")
	if err == nil {
		t.Error("expected deploy error for missing jar")
	}

	session := backend.GetSession("device-1")
	if session == nil {
		t.Fatal("expected session to be tracked")
	}
	if session.state != StateFallbackScreencap {
		t.Errorf("expected StateFallbackScreencap, got %s", session.state)
	}

	// Teardown
	if err := backend.Teardown("device-1"); err != nil {
		t.Errorf("Teardown error: %v", err)
	}
	if backend.IsStreaming("device-1") {
		t.Error("expected IsStreaming=false after teardown")
	}
}
