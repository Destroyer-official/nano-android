package mcp_compiler

import (
	"context"
	"strings"
)

// Phase6Hooks allows binaries to attach Plugin SDK, IDE/Framework Adapters, Remote Device Pools,
// and Verifiable Unified Reporting / Analytics without module import cycles.
type Phase6Hooks struct {
	PluginList       func(ctx context.Context) (any, error)
	PluginInvoke     func(ctx context.Context, id string, payload map[string]any) (any, error)
	AdaptersList     func(ctx context.Context) (any, error)
	IDEConfig        func(ctx context.Context, client, binPath string) (any, error)
	PoolStatus       func(ctx context.Context) (any, error)
	PoolShard        func(ctx context.Context, actor string, maxShards int, cases []string) (any, error)
	ReportUnified    func(ctx context.Context, artifact string) (any, error)
	AnalyticsSummary func(ctx context.Context, baseVer, targetVer string) (any, error)
}

// SetPhase6Hooks attaches Phase 6 handlers onto ToolRegistry.
func (r *ToolRegistry) SetPhase6Hooks(h Phase6Hooks) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks6 = h
}

func (r *ToolRegistry) getHooks6() Phase6Hooks {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hooks6
}

func (r *ToolRegistry) registerPhase6Tools() {
	r.Register(ToolDefinition{
		Name:        "nano_plugin_list",
		Description: "Lists supported Plugin SDK kinds and installed Ed25519-verified plugins.",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(ctx context.Context, args map[string]any) (*ToolResult, error) {
			h := r.getHooks6()
			if h.PluginList == nil {
				return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 6 PluginList hook not attached")
			}
			res, err := h.PluginList(ctx)
			if err != nil {
				return makeErrorResult(err.Error())
			}
			return makeJSONResult(res)
		},
	})
	r.Register(ToolDefinition{
		Name:        "nano_plugin_invoke",
		Description: "Invokes an Ed25519-verified plugin inside a scrubbed, isolated subprocess.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"plugin_id": map[string]any{"type": "string"},
				"payload":   map[string]any{"type": "object"},
			},
			"required": []string{"plugin_id"},
		},
		Handler: func(ctx context.Context, args map[string]any) (*ToolResult, error) {
			h := r.getHooks6()
			if h.PluginInvoke == nil {
				return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 6 PluginInvoke hook not attached")
			}
			id, _ := args["plugin_id"].(string)
			payload, _ := args["payload"].(map[string]any)
			res, err := h.PluginInvoke(ctx, strings.TrimSpace(id), payload)
			if err != nil {
				return makeErrorResult(err.Error())
			}
			return makeJSONResult(res)
		},
	})
	r.Register(ToolDefinition{
		Name:        "nano_adapters_list",
		Description: "Probes all 12 native automation framework adapters and 7 supported IDE clients.",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(ctx context.Context, args map[string]any) (*ToolResult, error) {
			h := r.getHooks6()
			if h.AdaptersList == nil {
				return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 6 AdaptersList hook not attached")
			}
			res, err := h.AdaptersList(ctx)
			if err != nil {
				return makeErrorResult(err.Error())
			}
			return makeJSONResult(res)
		},
	})
	r.Register(ToolDefinition{
		Name:        "nano_ide_config",
		Description: "Generates IDE/MCP client configuration for Android Studio, IntelliJ, VS Code, Cursor, Antigravity, or Claude Code.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"client":      map[string]any{"type": "string"},
				"binary_path": map[string]any{"type": "string"},
			},
			"required": []string{"client"},
		},
		Handler: func(ctx context.Context, args map[string]any) (*ToolResult, error) {
			h := r.getHooks6()
			if h.IDEConfig == nil {
				return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 6 IDEConfig hook not attached")
			}
			c, _ := args["client"].(string)
			bp, _ := args["binary_path"].(string)
			res, err := h.IDEConfig(ctx, strings.TrimSpace(c), strings.TrimSpace(bp))
			if err != nil {
				return makeErrorResult(err.Error())
			}
			return makeJSONResult(res)
		},
	})
	r.Register(ToolDefinition{
		Name:        "nano_pool_status",
		Description: "Discovers available local AVD, USB, TLS wireless, managed on-prem, and Firebase Test Lab pool targets.",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(ctx context.Context, args map[string]any) (*ToolResult, error) {
			h := r.getHooks6()
			if h.PoolStatus == nil {
				return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 6 PoolStatus hook not attached")
			}
			res, err := h.PoolStatus(ctx)
			if err != nil {
				return makeErrorResult(err.Error())
			}
			return makeJSONResult(res)
		},
	})
	r.Register(ToolDefinition{
		Name:        "nano_pool_shard",
		Description: "Submits a parallel sharded test job across reserved pool workers with quota enforcement.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"actor":      map[string]any{"type": "string"},
				"max_shards": map[string]any{"type": "integer"},
				"cases":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"actor"},
		},
		Handler: func(ctx context.Context, args map[string]any) (*ToolResult, error) {
			h := r.getHooks6()
			if h.PoolShard == nil {
				return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 6 PoolShard hook not attached")
			}
			actor, _ := args["actor"].(string)
			maxShards := intArg(args["max_shards"], 2)
			var cases []string
			if raw, ok := args["cases"].([]any); ok {
				for _, item := range raw {
					if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
						cases = append(cases, s)
					}
				}
			}
			res, err := h.PoolShard(ctx, strings.TrimSpace(actor), maxShards, cases)
			if err != nil {
				return makeErrorResult(err.Error())
			}
			return makeJSONResult(res)
		},
	})
	r.Register(ToolDefinition{
		Name:        "nano_report_unified",
		Description: "Generates a cryptographically signed 22-field UnifiedReport + Ed25519 signature.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"artifact": map[string]any{"type": "string"}},
		},
		Handler: func(ctx context.Context, args map[string]any) (*ToolResult, error) {
			h := r.getHooks6()
			if h.ReportUnified == nil {
				return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 6 ReportUnified hook not attached")
			}
			art, _ := args["artifact"].(string)
			res, err := h.ReportUnified(ctx, strings.TrimSpace(art))
			if err != nil {
				return makeErrorResult(err.Error())
			}
			return makeJSONResult(res)
		},
	})
	r.Register(ToolDefinition{
		Name:        "nano_analytics_summary",
		Description: "Returns longitudinal test/flake/duration trends and optional release-to-release deltas.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"base_version":   map[string]any{"type": "string"},
				"target_version": map[string]any{"type": "string"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (*ToolResult, error) {
			h := r.getHooks6()
			if h.AnalyticsSummary == nil {
				return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 6 AnalyticsSummary hook not attached")
			}
			bv, _ := args["base_version"].(string)
			tv, _ := args["target_version"].(string)
			res, err := h.AnalyticsSummary(ctx, strings.TrimSpace(bv), strings.TrimSpace(tv))
			if err != nil {
				return makeErrorResult(err.Error())
			}
			return makeJSONResult(res)
		},
	})
}
