package mcp_compiler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPhase3MCPToolsAndPolicyContainment(t *testing.T) {
	coord := NewMockCoordinator()
	reg := NewToolRegistry(coord, NewMaestroCompiler("com.example.phase3"))
	ctx := context.Background()

	expectedTools := []string{
		"nano_profile_memory",
		"nano_profile_cpu",
		"nano_profile_threads",
		"nano_profile_gc",
		"nano_profile_performance",
		"nano_timeline_inspect",
		"nano_compatibility_db",
		"nano_lab_list",
		"nano_lab_run",
		"nano_test_differential",
		"nano_crash_analyze",
		"nano_media_inspect",
	}
	for _, name := range expectedTools {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("expected Phase 3 MCP tool %q to be registered", name)
		}
	}

	// Verify compatibility.db seeding + query through MCP
	compatRes, err := reg.Call(ctx, "nano_compatibility_db", map[string]any{"seed": true, "db_path": "phase3_mcp_test.db"})
	if err != nil || compatRes.IsError {
		t.Fatalf("nano_compatibility_db failed: err=%v res=%+v", err, compatRes)
	}
	if !strings.Contains(compatRes.Content[0].Text, "android.database.sqlite.SQLiteDatabase") {
		t.Fatalf("expected SQLiteDatabase contract in heatmap output")
	}

	// Verify memory/threads/performance profiler MCP call works via MockCoordinator
	perfRes, err := reg.Call(ctx, "nano_profile_performance", nil)
	if err != nil || perfRes.IsError {
		t.Fatalf("nano_profile_performance failed: err=%v res=%+v", err, perfRes)
	}
	var perfPayload map[string]any
	if err := json.Unmarshal([]byte(perfRes.Content[0].Text), &perfPayload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if perfPayload["backend"] != "nano_jvm" {
		t.Fatalf("expected nano_jvm backend disclosure, got %v", perfPayload["backend"])
	}

	// Verify path containment rejects outside media_path
	medRes, err := reg.Call(ctx, "nano_media_inspect", map[string]any{"media_path": "/etc/passwd.mp4"})
	if err != nil || !medRes.IsError || !strings.Contains(medRes.Content[0].Text, "BLOCKED_POLICY") {
		t.Fatalf("expected BLOCKED_POLICY for escaping media_path, got err=%v res=%+v", err, medRes)
	}
}
