package build

import (
	"context"
	"fmt"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// ProjectBuildHandler implements the `project_build` MCP tool.
type ProjectBuildHandler struct {
	engine *GradleEngine
}

func NewProjectBuildHandler(engine *GradleEngine) *ProjectBuildHandler {
	return &ProjectBuildHandler{engine: engine}
}

func (h *ProjectBuildHandler) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "project_build",
		Description: "Triggers a Gradle build and returns status, duration, APK path, SHA-256, and compiler errors.",
		RiskTier:    types.RiskWRITESCOPED,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"variant": map[string]any{
					"type":        "string",
					"description": "Build variant (e.g. debug, release). Defaults to debug.",
				},
				"module": map[string]any{
					"type":        "string",
					"description": "Target module name (e.g. app). Defaults to app.",
				},
				"clean": map[string]any{
					"type":        "boolean",
					"description": "Whether to run a clean build prior to assembling.",
				},
			},
		},
	}
}

func (h *ProjectBuildHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	variant, _ := args["variant"].(string)
	module, _ := args["module"].(string)
	clean, _ := args["clean"].(bool)

	res, err := h.engine.BuildWithContext(ctx, variant, module, clean)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	data := map[string]any{
		"status":          res.Status,
		"duration_ms":     res.DurationMS,
		"apk_path":        res.APKPath,
		"apk_sha256":      res.APKSHA256,
		"compiler_errors": res.Errors,
	}

	return &types.ToolResult{
		Success: res.Status == "success",
		Data:    data,
	}, nil
}

// ProjectTestJVMHandler implements the `project_test_jvm` MCP tool.
type ProjectTestJVMHandler struct {
	engine *GradleEngine
}

func NewProjectTestJVMHandler(engine *GradleEngine) *ProjectTestJVMHandler {
	return &ProjectTestJVMHandler{engine: engine}
}

func (h *ProjectTestJVMHandler) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "project_test_jvm",
		Description: "Runs Robolectric/JVM unit tests and Roborazzi golden visual comparisons, parsing JUnit XML.",
		RiskTier:    types.RiskREAD,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"test_filter": map[string]any{
					"type":        "string",
					"description": "Optional test class/method filter pattern.",
				},
				"include_roborazzi": map[string]any{
					"type":        "boolean",
					"description": "Whether to run compareRoborazziDebug.",
				},
				"record_goldens": map[string]any{
					"type":        "boolean",
					"description": "Whether to run recordRoborazziDebug to update golden images.",
				},
			},
		},
	}
}

func (h *ProjectTestJVMHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	filter, _ := args["test_filter"].(string)
	if filter == "" {
		filter, _ = args["filter"].(string)
	}
	includeRoborazzi, _ := args["include_roborazzi"].(bool)
	recordGoldens, _ := args["record_goldens"].(bool)

	res, err := h.engine.TestJVMWithContext(ctx, filter, includeRoborazzi, recordGoldens)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	data := map[string]any{
		"passed":      res.Passed,
		"failed":      res.Failed,
		"skipped":     res.Skipped,
		"duration_ms": res.DurationMS,
		"failures":    res.Failures,
	}

	return &types.ToolResult{
		Success: res.Failed == 0,
		Data:    data,
	}, nil
}

// APKInspectHandler implements the `apk_inspect` MCP tool.
type APKInspectHandler struct {
	engine *GradleEngine
}

func NewAPKInspectHandler(engine *GradleEngine) *APKInspectHandler {
	return &APKInspectHandler{engine: engine}
}

func (h *APKInspectHandler) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "apk_inspect",
		Description: "Performs static APK analysis without source code, extracting manifest, ABIs, permissions, and certs.",
		RiskTier:    types.RiskREAD,
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"apk_path"},
			"properties": map[string]any{
				"apk_path": map[string]any{
					"type":        "string",
					"description": "Path to the APK file.",
				},
				"decompile_classes": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional bounded list of class names to decompile with jadx.",
				},
			},
		},
	}
}

func (h *APKInspectHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	apkPath, ok := args["apk_path"].(string)
	if !ok || apkPath == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "apk_path is required",
		}, nil
	}

	var decompileClasses []string
	if rawClasses, ok := args["decompile_classes"].([]any); ok {
		for _, c := range rawClasses {
			if s, ok := c.(string); ok {
				decompileClasses = append(decompileClasses, s)
			}
		}
	} else if rawStringClasses, ok := args["decompile_classes"].([]string); ok {
		decompileClasses = rawStringClasses
	}

	res, err := h.engine.InspectAPKWithContext(ctx, apkPath, decompileClasses)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	data := map[string]any{
		"package_id":          res.PackageID,
		"version_code":        res.VersionCode,
		"version_name":        res.VersionName,
		"min_sdk":             res.MinSDK,
		"target_sdk":          res.TargetSDK,
		"launcher_activity":   res.LauncherActivity,
		"abis":                res.ABIs,
		"permissions":         res.Permissions,
		"exported_components": res.ExportedComponents,
		"signing_certs":       res.SigningCerts,
		"deep_link_schemes":   res.DeepLinkSchemes,
	}

	return &types.ToolResult{
		Success: true,
		Data:    data,
	}, nil
}

// APKAuditComplianceHandler implements the `apk_audit_compliance` MCP tool.
type APKAuditComplianceHandler struct {
	engine *GradleEngine
}

func NewAPKAuditComplianceHandler(engine *GradleEngine) *APKAuditComplianceHandler {
	return &APKAuditComplianceHandler{engine: engine}
}

func (h *APKAuditComplianceHandler) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "apk_audit_compliance",
		Description: "Audits APK for 16 KB ELF alignment, Google Play policy restrictions, and Android 17 compatibility.",
		RiskTier:    types.RiskREAD,
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"apk_path"},
			"properties": map[string]any{
				"apk_path": map[string]any{
					"type":        "string",
					"description": "Path to the APK file.",
				},
				"profile": map[string]any{
					"type":        "string",
					"description": "Optional policy profile or path to play-policy-justification.yaml.",
				},
			},
		},
	}
}

func (h *APKAuditComplianceHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	apkPath, ok := args["apk_path"].(string)
	if !ok || apkPath == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "apk_path is required",
		}, nil
	}
	profile, _ := args["profile"].(string)

	res, err := h.engine.AuditComplianceWithContext(ctx, apkPath, profile)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	data := map[string]any{
		"pass":                   res.Pass,
		"elf_16kb_aligned":       res.ELF16KBAligned,
		"large_screen_resizable": res.LargeScreenResizable,
		"play_policy_findings":   res.PlayPolicyFindings,
		"android17_findings":     res.Android17Findings,
	}

	return &types.ToolResult{
		Success: res.Pass,
		Data:    data,
	}, nil
}

// ToolRegistry defines an interface for registering tool handlers with the MCP server.
type ToolRegistry interface {
	Register(h types.ToolHandler)
}

// RegisterTools registers all 4 tools with an MCP registry or router.
func RegisterTools(engine *GradleEngine) []types.ToolHandler {
	return []types.ToolHandler{
		NewProjectBuildHandler(engine),
		NewProjectTestJVMHandler(engine),
		NewAPKInspectHandler(engine),
		NewAPKAuditComplianceHandler(engine),
	}
}

// RegisterBuildToolsOnServer registers tools #1, #2, #3, #4 directly with an MCP server or registry.
func RegisterBuildToolsOnServer(registry ToolRegistry, engine *GradleEngine) {
	if registry == nil {
		return
	}
	for _, h := range RegisterTools(engine) {
		registry.Register(h)
	}
}

// FormatError converts an error into a standardized string.
func formatError(prefix string, err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%s: %v", prefix, err)
}
