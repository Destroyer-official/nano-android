package phase6hooks_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nano_android/engine/mcp_compiler"
	"nano_android/internal/phase6hooks"
)

func TestPhase6MCPToolsEndToEnd(t *testing.T) {
	tmpRoot := t.TempDir()
	srcAPK, _ := filepath.Abs(filepath.Join("..", "..", "fixtures", "apks", "minimal_valid.apk"))
	rawAPK, err := os.ReadFile(srcAPK)
	if err != nil {
		t.Fatalf("read fixture apk: %v", err)
	}
	_ = os.MkdirAll(filepath.Join(tmpRoot, "fixtures", "apks"), 0750)
	dstAPK := filepath.Join(tmpRoot, "fixtures", "apks", "minimal_valid.apk")
	_ = os.WriteFile(dstAPK, rawAPK, 0600)

	reg := mcp_compiler.NewToolRegistry(mcp_compiler.NewMockCoordinator(), mcp_compiler.NewMaestroCompiler("com.example.minimal"))
	phase6hooks.Attach(reg, tmpRoot)
	ctx := context.Background()

	for _, name := range []string{
		"nano_plugin_list", "nano_plugin_invoke", "nano_adapters_list",
		"nano_ide_config", "nano_pool_status", "nano_pool_shard",
		"nano_report_unified", "nano_analytics_summary",
	} {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("expected MCP tool %q registered", name)
		}
	}

	if res, err := reg.Call(ctx, "nano_plugin_list", nil); err != nil || res.IsError || !strings.Contains(res.Content[0].Text, "apk_analyzer") {
		t.Fatalf("nano_plugin_list failed: %+v err=%v", res, err)
	}
	if res, err := reg.Call(ctx, "nano_adapters_list", nil); err != nil || res.IsError || !strings.Contains(res.Content[0].Text, "maestro") {
		t.Fatalf("nano_adapters_list failed: %+v err=%v", res, err)
	}
	if res, err := reg.Call(ctx, "nano_ide_config", map[string]any{"client": "cursor"}); err != nil || res.IsError || !strings.Contains(res.Content[0].Text, "mcpServers") {
		t.Fatalf("nano_ide_config failed: %+v err=%v", res, err)
	}
	if res, err := reg.Call(ctx, "nano_pool_shard", map[string]any{"actor": "mcp_runner", "max_shards": float64(2), "cases": []any{"T1", "T2"}}); err != nil || res.IsError || !strings.Contains(res.Content[0].Text, "COMPLETED") {
		t.Fatalf("nano_pool_shard failed: %+v err=%v", res, err)
	}
	if res, err := reg.Call(ctx, "nano_report_unified", map[string]any{"artifact": dstAPK}); err != nil || res.IsError || !strings.Contains(res.Content[0].Text, "signature_ed25519") {
		t.Fatalf("nano_report_unified failed: %+v err=%v", res, err)
	}
	if res, err := reg.Call(ctx, "nano_analytics_summary", nil); err != nil || res.IsError || !strings.Contains(res.Content[0].Text, "total_runs") {
		t.Fatalf("nano_analytics_summary failed: %+v err=%v", res, err)
	}
}
