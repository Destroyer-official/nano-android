package intelhooks_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nano_android/engine/intelligence/intelhooks"
	"nano_android/engine/mcp_compiler"
)

func TestPhase5MCPToolsEndToEnd(t *testing.T) {
	tmpRoot := t.TempDir()
	srcAPK, _ := filepath.Abs(filepath.Join("..", "..", "..", "fixtures", "apks", "minimal_valid.apk"))
	rawAPK, err := os.ReadFile(srcAPK)
	if err != nil {
		t.Fatalf("read fixture apk: %v", err)
	}
	_ = os.MkdirAll(filepath.Join(tmpRoot, "fixtures", "apks"), 0750)
	dstAPK := filepath.Join(tmpRoot, "fixtures", "apks", "minimal_valid.apk")
	_ = os.WriteFile(dstAPK, rawAPK, 0600)

	coord := mcp_compiler.NewMockCoordinator()
	reg := mcp_compiler.NewToolRegistry(coord, mcp_compiler.NewMaestroCompiler("com.example.minimal"))
	intelhooks.Attach(reg, tmpRoot)

	ctx := context.Background()

	// Verify all 7 Phase 5 MCP tools are registered
	for _, name := range []string{
		"nano_graph_build", "nano_graph_query", "nano_graph_impact", "nano_ai_understand",
		"nano_ai_explore", "nano_ai_generate_tests", "nano_ai_diagnose",
		"nano_ai_self_heal", "nano_ai_coordinate", "nano_ai_memory", "nano_ai_approve",
	} {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("expected MCP tool %q registered", name)
		}
	}

	resBuild, err := reg.Call(ctx, "nano_graph_build", map[string]any{"artifact": dstAPK})
	if err != nil || resBuild.IsError || !strings.Contains(resBuild.Content[0].Text, "architecture") {
		t.Fatalf("nano_graph_build failed: res=%+v err=%v", resBuild, err)
	}

	resQuery, err := reg.Call(ctx, "nano_graph_query", map[string]any{"kind": "package"})
	if err != nil || resQuery.IsError || !strings.Contains(resQuery.Content[0].Text, "total_matching") {
		t.Fatalf("nano_graph_query failed: res=%+v err=%v", resQuery, err)
	}

	resMem, err := reg.Call(ctx, "nano_ai_memory", map[string]any{"op": "put", "key": "k_mcp", "category": "quirk", "value": "val", "actor": "mcp_ci"})
	if err != nil || resMem.IsError || !strings.Contains(resMem.Content[0].Text, "k_mcp") {
		t.Fatalf("nano_ai_memory failed: res=%+v err=%v", resMem, err)
	}

	resImpact, err := reg.Call(ctx, "nano_graph_impact", map[string]any{"artifact": dstAPK, "changed": []any{"MainActivity"}})
	if err != nil || resImpact.IsError || !strings.Contains(resImpact.Content[0].Text, "risk_level") {
		t.Fatalf("nano_graph_impact failed: res=%+v err=%v", resImpact, err)
	}

	resUnder, err := reg.Call(ctx, "nano_ai_understand", map[string]any{"artifact": dstAPK, "max_steps": float64(15)})
	if err != nil || resUnder.IsError || !strings.Contains(resUnder.Content[0].Text, "markdown_docs") {
		t.Fatalf("nano_ai_understand failed: res=%+v err=%v", resUnder, err)
	}

	resDiag, err := reg.Call(ctx, "nano_ai_diagnose", map[string]any{
		"trace":         "java.lang.RuntimeException: boom\n\tat com.example.Foo.bar(Foo.java:5)\n",
		"suspected_api": "android.webkit.WebView",
	})
	if err != nil || resDiag.IsError || !strings.Contains(resDiag.Content[0].Text, "ranked_causes") {
		t.Fatalf("nano_ai_diagnose failed: res=%+v err=%v", resDiag, err)
	}
}
