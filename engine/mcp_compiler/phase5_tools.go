package mcp_compiler

import (
	"context"
	"strings"
)

// Phase5Hooks allows top-level binaries (`nano`, `nano-mcp`) to attach Phase 5 Application Behavior Graph
// and AI Autonomous Engineer capabilities without introducing circular Go module imports.
type Phase5Hooks struct {
	BuildGraph    func(ctx context.Context, artifactPath, sourceRoot string) (any, error)
	QueryGraph    func(ctx context.Context, kind, match string, exportedOnly bool, limit int) (any, error)
	AnalyzeImpact func(ctx context.Context, artifactPath string, changed []string) (any, error)
	Understand    func(ctx context.Context, artifactPath string, maxSteps int) (any, error)
	Explore       func(ctx context.Context, artifactPath, activity string, maxSteps, maxScreens int) (any, error)
	GenerateTests func(ctx context.Context, artifactPath string, maxSteps int) (any, error)
	Diagnose      func(ctx context.Context, trace, mappingPath, suspectedAPI string) (any, error)
	SelfHeal      func(ctx context.Context, resourceID, text, contentDesc, className string) (any, error)
	Coordinate    func(ctx context.Context, artifactPath string, maxSteps, maxScreens int) (any, error)
	Memory        func(ctx context.Context, op, key, category, value, actor string) (any, error)
	Approve       func(ctx context.Context, proposalID, approvedBy string, apply bool) (any, error)
}

// SetPhase5Hooks attaches Phase 5 AI & Behavior Graph handlers onto the MCP tool registry.
func (r *ToolRegistry) SetPhase5Hooks(h Phase5Hooks) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks5 = h
}

func (r *ToolRegistry) getHooks5() Phase5Hooks {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hooks5
}

func (r *ToolRegistry) registerPhase5Tools() {
	r.Register(ToolDefinition{
		Name:        "nano_graph_build",
		Description: "Builds and persists the Application Behavior Graph (components, DEX dependencies, resources, permissions, native JNI libs) and returns architecture metrics.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"artifact":    map[string]any{"type": "string"},
				"source_root": map[string]any{"type": "string"},
			},
			"required": []string{"artifact"},
		},
		Handler: r.handlePhase5GraphBuild,
	})
	r.Register(ToolDefinition{
		Name:        "nano_graph_impact",
		Description: "Runs change-impact graph traversal to identify impacted components, screens, risk level, and regression tests to run.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"artifact": map[string]any{"type": "string"},
				"changed":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"changed"},
		},
		Handler: r.handlePhase5GraphImpact,
	})
	r.Register(ToolDefinition{
		Name:        "nano_ai_understand",
		Description: "Runs I1 Intelligent APK Understanding behind Policy Guard, nonced untrusted delimiters, and audit logging.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"artifact":  map[string]any{"type": "string"},
				"max_steps": map[string]any{"type": "integer"},
			},
			"required": []string{"artifact"},
		},
		Handler: r.handlePhase5AIUnderstand,
	})
	r.Register(ToolDefinition{
		Name:        "nano_ai_explore",
		Description: "Runs bounded I3 autonomous UTG explorer & bug hunter against live runtime and produces a PENDING_APPROVAL Maestro repro proposal.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"artifact":    map[string]any{"type": "string"},
				"activity":    map[string]any{"type": "string"},
				"max_steps":   map[string]any{"type": "integer"},
				"max_screens": map[string]any{"type": "integer"},
			},
			"required": []string{"artifact"},
		},
		Handler: r.handlePhase5AIExplore,
	})
	r.Register(ToolDefinition{
		Name:        "nano_ai_generate_tests",
		Description: "Runs I2 autonomous test generation emitting reviewable Maestro YAML and JUnit 5 drafts (always PENDING_APPROVAL).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"artifact":  map[string]any{"type": "string"},
				"max_steps": map[string]any{"type": "integer"},
			},
			"required": []string{"artifact"},
		},
		Handler: r.handlePhase5AIGenTests,
	})
	r.Register(ToolDefinition{
		Name:        "nano_ai_diagnose",
		Description: "Runs I4/I5 failure analysis, R8 retrace, root-cause ranking, and remediation patch drafting.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"trace":         map[string]any{"type": "string"},
				"mapping_path":  map[string]any{"type": "string"},
				"suspected_api": map[string]any{"type": "string"},
			},
			"required": []string{"trace"},
		},
		Handler: r.handlePhase5AIDiagnose,
	})
	r.Register(ToolDefinition{
		Name:        "nano_ai_approve",
		Description: "Explicit human approval gate for reviewable AI proposals (never permits empty approver or silent baseline edits).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"proposal_id": map[string]any{"type": "string"},
				"approved_by": map[string]any{"type": "string"},
				"apply":       map[string]any{"type": "boolean"},
			},
			"required": []string{"proposal_id", "approved_by"},
		},
		Handler: r.handlePhase5AIApprove,
	})
	r.Register(ToolDefinition{
		Name:        "nano_graph_query",
		Description: "Queries persisted Application Behavior Graph vertices and incident edges by kind, substring, or exported status.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"kind":          map[string]any{"type": "string"},
				"name_contains": map[string]any{"type": "string"},
				"exported_only": map[string]any{"type": "boolean"},
				"limit":         map[string]any{"type": "integer"},
			},
		},
		Handler: r.handlePhase5GraphQuery,
	})
	r.Register(ToolDefinition{
		Name:        "nano_ai_self_heal",
		Description: "Proposes ranked replacement UI selectors across 5 AST signals (requires approval before updating baselines).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"resource_id":         map[string]any{"type": "string"},
				"text":                map[string]any{"type": "string"},
				"content_description": map[string]any{"type": "string"},
				"class_name":          map[string]any{"type": "string"},
			},
		},
		Handler: r.handlePhase5SelfHeal,
	})
	r.Register(ToolDefinition{
		Name:        "nano_ai_coordinate",
		Description: "Runs bounded multi-agent coordination (Architect + Explorer + TestAuthor) under shared Governor budgets.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"artifact":    map[string]any{"type": "string"},
				"max_steps":   map[string]any{"type": "integer"},
				"max_screens": map[string]any{"type": "integer"},
			},
			"required": []string{"artifact"},
		},
		Handler: r.handlePhase5Coordinate,
	})
	r.Register(ToolDefinition{
		Name:        "nano_ai_memory",
		Description: "Reads or updates vetted project engineering memory (requires non-empty actor on put).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"op":       map[string]any{"type": "string", "enum": []string{"get", "put"}},
				"key":      map[string]any{"type": "string"},
				"category": map[string]any{"type": "string"},
				"value":    map[string]any{"type": "string"},
				"actor":    map[string]any{"type": "string"},
			},
		},
		Handler: r.handlePhase5Memory,
	})
}

func (r *ToolRegistry) handlePhase5GraphBuild(ctx context.Context, args map[string]any) (*ToolResult, error) {
	h := r.getHooks5()
	if h.BuildGraph == nil {
		return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 5 GraphBuild hook not attached")
	}
	art, _ := args["artifact"].(string)
	src, _ := args["source_root"].(string)
	res, err := h.BuildGraph(ctx, strings.TrimSpace(art), strings.TrimSpace(src))
	if err != nil {
		return makeErrorResult(err.Error())
	}
	return makeJSONResult(res)
}

func (r *ToolRegistry) handlePhase5GraphImpact(ctx context.Context, args map[string]any) (*ToolResult, error) {
	h := r.getHooks5()
	if h.AnalyzeImpact == nil {
		return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 5 AnalyzeImpact hook not attached")
	}
	art, _ := args["artifact"].(string)
	var changed []string
	if rawList, ok := args["changed"].([]any); ok {
		for _, item := range rawList {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				changed = append(changed, strings.TrimSpace(s))
			}
		}
	}
	if s, ok := args["changed"].(string); ok && strings.TrimSpace(s) != "" {
		changed = append(changed, strings.Split(s, ",")...)
	}
	res, err := h.AnalyzeImpact(ctx, strings.TrimSpace(art), changed)
	if err != nil {
		return makeErrorResult(err.Error())
	}
	return makeJSONResult(res)
}

func (r *ToolRegistry) handlePhase5AIUnderstand(ctx context.Context, args map[string]any) (*ToolResult, error) {
	h := r.getHooks5()
	if h.Understand == nil {
		return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 5 Understand hook not attached")
	}
	art, _ := args["artifact"].(string)
	maxSteps := intArg(args["max_steps"], 20)
	res, err := h.Understand(ctx, strings.TrimSpace(art), maxSteps)
	if err != nil {
		return makeErrorResult(err.Error())
	}
	return makeJSONResult(res)
}

func (r *ToolRegistry) handlePhase5AIExplore(ctx context.Context, args map[string]any) (*ToolResult, error) {
	h := r.getHooks5()
	if h.Explore == nil {
		return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 5 Explore hook not attached")
	}
	art, _ := args["artifact"].(string)
	act, _ := args["activity"].(string)
	maxSteps := intArg(args["max_steps"], 15)
	maxScreens := intArg(args["max_screens"], 10)
	res, err := h.Explore(ctx, strings.TrimSpace(art), strings.TrimSpace(act), maxSteps, maxScreens)
	if err != nil {
		return makeErrorResult(err.Error())
	}
	return makeJSONResult(res)
}

func (r *ToolRegistry) handlePhase5AIGenTests(ctx context.Context, args map[string]any) (*ToolResult, error) {
	h := r.getHooks5()
	if h.GenerateTests == nil {
		return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 5 GenerateTests hook not attached")
	}
	art, _ := args["artifact"].(string)
	maxSteps := intArg(args["max_steps"], 20)
	res, err := h.GenerateTests(ctx, strings.TrimSpace(art), maxSteps)
	if err != nil {
		return makeErrorResult(err.Error())
	}
	return makeJSONResult(res)
}

func (r *ToolRegistry) handlePhase5AIDiagnose(ctx context.Context, args map[string]any) (*ToolResult, error) {
	h := r.getHooks5()
	if h.Diagnose == nil {
		return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 5 Diagnose hook not attached")
	}
	trace, _ := args["trace"].(string)
	mappingPath, _ := args["mapping_path"].(string)
	suspectedAPI, _ := args["suspected_api"].(string)
	res, err := h.Diagnose(ctx, trace, strings.TrimSpace(mappingPath), strings.TrimSpace(suspectedAPI))
	if err != nil {
		return makeErrorResult(err.Error())
	}
	return makeJSONResult(res)
}

func (r *ToolRegistry) handlePhase5AIApprove(ctx context.Context, args map[string]any) (*ToolResult, error) {
	h := r.getHooks5()
	if h.Approve == nil {
		return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 5 Approve hook not attached")
	}
	id, _ := args["proposal_id"].(string)
	by, _ := args["approved_by"].(string)
	apply, _ := args["apply"].(bool)
	res, err := h.Approve(ctx, strings.TrimSpace(id), strings.TrimSpace(by), apply)
	if err != nil {
		return makeErrorResult(err.Error())
	}
	return makeJSONResult(res)
}

func intArg(v any, def int) int {
	switch x := v.(type) {
	case float64:
		if int(x) > 0 {
			return int(x)
		}
	case int:
		if x > 0 {
			return x
		}
	}
	return def
}

func (r *ToolRegistry) handlePhase5GraphQuery(ctx context.Context, args map[string]any) (*ToolResult, error) {
	h := r.getHooks5()
	if h.QueryGraph == nil {
		return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 5 QueryGraph hook not attached")
	}
	kind, _ := args["kind"].(string)
	match, _ := args["name_contains"].(string)
	exp, _ := args["exported_only"].(bool)
	limit := intArg(args["limit"], 100)
	res, err := h.QueryGraph(ctx, strings.TrimSpace(kind), strings.TrimSpace(match), exp, limit)
	if err != nil {
		return makeErrorResult(err.Error())
	}
	return makeJSONResult(res)
}

func (r *ToolRegistry) handlePhase5SelfHeal(ctx context.Context, args map[string]any) (*ToolResult, error) {
	h := r.getHooks5()
	if h.SelfHeal == nil {
		return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 5 SelfHeal hook not attached")
	}
	rid, _ := args["resource_id"].(string)
	txt, _ := args["text"].(string)
	cd, _ := args["content_description"].(string)
	cls, _ := args["class_name"].(string)
	res, err := h.SelfHeal(ctx, rid, txt, cd, cls)
	if err != nil {
		return makeErrorResult(err.Error())
	}
	return makeJSONResult(res)
}

func (r *ToolRegistry) handlePhase5Coordinate(ctx context.Context, args map[string]any) (*ToolResult, error) {
	h := r.getHooks5()
	if h.Coordinate == nil {
		return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 5 Coordinate hook not attached")
	}
	art, _ := args["artifact"].(string)
	maxSteps := intArg(args["max_steps"], 20)
	maxScreens := intArg(args["max_screens"], 10)
	res, err := h.Coordinate(ctx, strings.TrimSpace(art), maxSteps, maxScreens)
	if err != nil {
		return makeErrorResult(err.Error())
	}
	return makeJSONResult(res)
}

func (r *ToolRegistry) handlePhase5Memory(ctx context.Context, args map[string]any) (*ToolResult, error) {
	h := r.getHooks5()
	if h.Memory == nil {
		return makeErrorResult("INFRASTRUCTURE_ERROR: Phase 5 Memory hook not attached")
	}
	op, _ := args["op"].(string)
	key, _ := args["key"].(string)
	cat, _ := args["category"].(string)
	val, _ := args["value"].(string)
	actor, _ := args["actor"].(string)
	res, err := h.Memory(ctx, strings.TrimSpace(op), strings.TrimSpace(key), strings.TrimSpace(cat), val, strings.TrimSpace(actor))
	if err != nil {
		return makeErrorResult(err.Error())
	}
	return makeJSONResult(res)
}
