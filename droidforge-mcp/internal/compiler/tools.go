package compiler

import (
	"context"
	"fmt"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// FreezeToMaestroTool implements ToolHandler for session_freeze_to_maestro (#23).
type FreezeToMaestroTool struct {
	compiler *Compiler
}

// NewFreezeToMaestroTool creates a tool handler for session_freeze_to_maestro.
func NewFreezeToMaestroTool(compiler *Compiler) *FreezeToMaestroTool {
	return &FreezeToMaestroTool{compiler: compiler}
}

func (t *FreezeToMaestroTool) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "session_freeze_to_maestro",
		Description: "Compiles recorded interactive exploration session steps into a deterministic, runnable Maestro YAML flow with selector priority and companion external stimuli hooks.",
		RiskTier:    types.RiskWRITESCOPED,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{
					"type":        "string",
					"description": "Identifier of the active exploration session to freeze.",
				},
				"flow_name": map[string]any{
					"type":        "string",
					"description": "Name of the resulting Maestro flow test.",
				},
				"output_path": map[string]any{
					"type":        "string",
					"description": "Target file path for the emitted YAML flow (default: .droidforge/flows/<flow_name>.yaml).",
				},
				"app_id": map[string]any{
					"type":        "string",
					"description": "Android application package ID (optional override).",
				},
			},
			"required": []string{"session_id"},
		},
	}
}

func (t *FreezeToMaestroTool) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "missing required parameter: session_id",
		}, nil
	}

	flowName, _ := args["flow_name"].(string)
	outputPath, _ := args["output_path"].(string)
	if appID, ok := args["app_id"].(string); ok && appID != "" {
		rec := t.compiler.GetOrCreateRecorder(sessionID)
		rec.SetAppID(appID)
	}

	res, err := t.compiler.FreezeToMaestro(sessionID, flowName, outputPath)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	data := map[string]any{
		"yaml_path":  res.YAMLPath,
		"step_count": res.StepCount,
	}
	if res.HooksJSONPath != "" {
		data["hooks_json_path"] = res.HooksJSONPath
	}

	return &types.ToolResult{
		Success:  true,
		Data:     data,
		Warnings: res.WeakSelectorWarns,
	}, nil
}

// RunMaestroTool implements ToolHandler for test_run_maestro (#24).
type RunMaestroTool struct {
	compiler *Compiler
}

// NewRunMaestroTool creates a tool handler for test_run_maestro.
func NewRunMaestroTool(compiler *Compiler) *RunMaestroTool {
	return &RunMaestroTool{compiler: compiler}
}

func (t *RunMaestroTool) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "test_run_maestro",
		Description: "Executes a Maestro YAML flow with optional burn-in repeat counts, capturing failure bundles with logs and screenshots on failure.",
		RiskTier:    types.RiskWRITESCOPED,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"flow_path": map[string]any{
					"type":        "string",
					"description": "Path to the Maestro YAML flow file to execute.",
				},
				"serial": map[string]any{
					"type":        "string",
					"description": "Target device serial or emulator ID (e.g. emulator-5554).",
				},
				"repeat_count": map[string]any{
					"type":        "integer",
					"description": "Number of burn-in executions to verify test determinism (default: 1).",
				},
			},
			"required": []string{"flow_path"},
		},
	}
}

func (t *RunMaestroTool) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	flowPath, ok := args["flow_path"].(string)
	if !ok || flowPath == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "missing required parameter: flow_path",
		}, nil
	}

	serial, _ := args["serial"].(string)
	repeatCount := 1
	if rc, ok := args["repeat_count"].(float64); ok && rc > 0 {
		repeatCount = int(rc)
	} else if rc, ok := args["repeat_count"].(int); ok && rc > 0 {
		repeatCount = rc
	}

	res, err := t.compiler.RunMaestro(serial, flowPath, repeatCount)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	data := map[string]any{
		"runs_passed":          res.RunsPassed,
		"runs_failed":          res.RunsFailed,
		"avg_duration_ms":      res.AvgDurationMS,
		"failure_bundle_paths": res.FailureBundlePaths,
	}

	success := res.RunsFailed == 0

	var warnings []string
	if res.RunsFailed > 0 {
		warnings = append(warnings, fmt.Sprintf("%d of %d Maestro runs failed", res.RunsFailed, res.RunsPassed+res.RunsFailed))
	}

	return &types.ToolResult{
		Success:  success,
		Data:     data,
		Warnings: warnings,
	}, nil
}

// ExportAppCardTool implements ToolHandler for session_export_appcard (#25).
type ExportAppCardTool struct {
	compiler *Compiler
}

// NewExportAppCardTool creates a tool handler for session_export_appcard.
func NewExportAppCardTool(compiler *Compiler) *ExportAppCardTool {
	return &ExportAppCardTool{compiler: compiler}
}

func (t *ExportAppCardTool) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "session_export_appcard",
		Description: "Exports an AppCard markdown document aggregating screens visited, deep-link intent URIs, Compose test tags, and interactive element catalogs.",
		RiskTier:    types.RiskWRITESCOPED,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{
					"type":        "string",
					"description": "Identifier of the exploration session to export.",
				},
				"output_path": map[string]any{
					"type":        "string",
					"description": "Target markdown file path (default: .droidforge/appcard.md).",
				},
			},
			"required": []string{"session_id"},
		},
	}
}

func (t *ExportAppCardTool) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "missing required parameter: session_id",
		}, nil
	}

	outputPath, _ := args["output_path"].(string)

	res, err := t.compiler.ExportAppCard(sessionID, outputPath)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	data := map[string]any{
		"appcard_path":     res.AppCardPath,
		"screens_mapped":   res.ScreensMapped,
		"deep_links_found": res.DeepLinksFound,
	}

	return &types.ToolResult{
		Success: true,
		Data:    data,
	}, nil
}

// ToolRegistry defines an interface for registering tool handlers.
type ToolRegistry interface {
	Register(h types.ToolHandler)
}

// RegisterCompilerTools registers tools #23 (session_freeze_to_maestro),
// #24 (test_run_maestro), and #25 (session_export_appcard) on the server.
func RegisterCompilerTools(registry ToolRegistry, compiler *Compiler) {
	if registry == nil || compiler == nil {
		return
	}
	registry.Register(NewFreezeToMaestroTool(compiler))
	registry.Register(NewRunMaestroTool(compiler))
	registry.Register(NewExportAppCardTool(compiler))
}
