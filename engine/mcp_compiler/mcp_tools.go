package mcp_compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ToolResult represents standard MCP tool response content.
type ToolResult struct {
	Content []TextContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// TextContent represents text block in MCP tool response.
type TextContent struct {
	Type string `json:"type"` // always "text"
	Text string `json:"text"`
}

// ToolDefinition describes an MCP tool and its invocation handler.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Handler     func(ctx context.Context, args map[string]any) (*ToolResult, error)
}

// ToolRegistry manages registration and dispatch of Nano-Android MCP tools.
type ToolRegistry struct {
	mu       sync.RWMutex
	tools    map[string]ToolDefinition
	coord    Coordinator
	compiler *MaestroCompiler
}

// NewToolRegistry constructs a registry with all Nano-Android tools registered.
func NewToolRegistry(coord Coordinator, compiler *MaestroCompiler) *ToolRegistry {
	if compiler == nil {
		compiler = NewMaestroCompiler("com.prism.v")
	}

	reg := &ToolRegistry{
		tools:    make(map[string]ToolDefinition),
		coord:    coord,
		compiler: compiler,
	}

	reg.registerAllTools()
	return reg
}

// GetCoordinator returns the bound Coordinator.
func (r *ToolRegistry) GetCoordinator() Coordinator {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.coord
}

// GetCompiler returns the bound MaestroCompiler.
func (r *ToolRegistry) GetCompiler() *MaestroCompiler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.compiler
}

// Register registers a tool definition.
func (r *ToolRegistry) Register(tool ToolDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name] = tool
}

// Get returns the definition for a tool by name.
func (r *ToolRegistry) Get(name string) (ToolDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tool definitions.
func (r *ToolRegistry) List() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}

// Call invokes a registered tool by name with arguments.
func (r *ToolRegistry) Call(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	r.mu.RLock()
	tool, ok := r.tools[name]
	r.mu.RUnlock()

	if !ok {
		return &ToolResult{
			Content: []TextContent{{Type: "text", Text: fmt.Sprintf("Error: unknown tool %q", name)}},
			IsError: true,
		}, fmt.Errorf("tool not found: %s", name)
	}

	if args == nil {
		args = make(map[string]any)
	}

	return tool.Handler(ctx, args)
}

func (r *ToolRegistry) registerAllTools() {
	// 1. nano_launch_app
	r.Register(ToolDefinition{
		Name:        "nano_launch_app",
		Description: "Boots an Android APK into the deterministic virtual sandbox and initializes the runtime looper.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"apk_path": map[string]any{
					"type":        "string",
					"description": "Path to the APK file to execute inside the sandbox.",
				},
				"activity": map[string]any{
					"type":        "string",
					"description": "Optional activity name to launch (e.g. '.MainActivity').",
				},
				"clear_state": map[string]any{
					"type":        "boolean",
					"description": "If true, clears application state and caches before launch.",
				},
			},
			"required": []string{"apk_path"},
		},
		Handler: r.handleLaunchApp,
	})

	// 2. nano_observe_screen
	r.Register(ToolDefinition{
		Name:        "nano_observe_screen",
		Description: "Returns pruned accessibility/Compose AST tree and optional base64 WebP screenshot of current screen.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"include_screenshot": map[string]any{
					"type":        "boolean",
					"description": "Whether to capture and encode a base64 WebP screenshot.",
				},
				"format": map[string]any{
					"type":        "string",
					"enum":        []string{"webp", "png"},
					"description": "Image encoding format (default 'webp').",
				},
				"prune_depth": map[string]any{
					"type":        "integer",
					"description": "Maximum AST depth limit for pruning off-screen elements (0 for full).",
				},
			},
		},
		Handler: r.handleObserveScreen,
	})

	// 3. nano_dispatch_action
	r.Register(ToolDefinition{
		Name:        "nano_dispatch_action",
		Description: "Dispatches input events ('tap', 'double_tap', 'long_press', 'type', 'erase_text', 'press_key', 'swipe', 'scroll', 'back') by node ID or coordinates and verifies assertions.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"tap", "double_tap", "long_press", "type", "erase_text", "press_key", "swipe", "scroll", "back"},
					"description": "The interaction action type to dispatch.",
				},
				"node_id": map[string]any{
					"type":        "string",
					"description": "Target UI node ID or resource ID from AST (e.g. 'tab_studio' or 'opt_ultra_4k').",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "Text to enter (required when action is 'type').",
				},
				"key": map[string]any{
					"type":        "string",
					"description": "Key code name when action is 'press_key' (e.g. 'ENTER', 'BACK', 'HOME').",
				},
				"direction": map[string]any{
					"type":        "string",
					"enum":        []string{"UP", "DOWN", "LEFT", "RIGHT"},
					"description": "Swipe or scroll direction (used when action is 'swipe' or 'scroll').",
				},
				"assert_visible": map[string]any{
					"type":        "string",
					"description": "Optional text or node expected to be visible immediately following the action.",
				},
				"duration_ms": map[string]any{
					"type":        "integer",
					"description": "Duration for gesture or hold in milliseconds.",
				},
				"x": map[string]any{
					"type":        "number",
					"description": "X coordinate on virtual screen (optional).",
				},
				"y": map[string]any{
					"type":        "number",
					"description": "Y coordinate on virtual screen (optional).",
				},
			},
			"required": []string{"action"},
		},
		Handler: r.handleDispatchAction,
	})

	// 4. nano_step_time
	r.Register(ToolDefinition{
		Name:        "nano_step_time",
		Description: "Deterministically advances the virtual Android looper by delta_ms and flushes pending message queues.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"delta_ms": map[string]any{
					"type":        "integer",
					"description": "Milliseconds of virtual looper time to step forward.",
				},
			},
			"required": []string{"delta_ms"},
		},
		Handler: r.handleStepTime,
	})

	// 5. nano_inspect_state
	r.Register(ToolDefinition{
		Name:        "nano_inspect_state",
		Description: "Queries runtime memory, Dalvik/ART heap, view tree hierarchy, or virtual looper queues.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{
					"type":        "string",
					"enum":        []string{"all", "memory", "heap", "view_tree", "looper"},
					"description": "Diagnostics subsystem to query (default 'all').",
				},
			},
		},
		Handler: r.handleInspectState,
	})

	// 6. nano_freeze_test
	r.Register(ToolDefinition{
		Name:        "nano_freeze_test",
		Description: "Compiles recorded interactive session and assertions into an autonomous Maestro regression YAML flow (repeat_count: 3).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"app_id": map[string]any{
					"type":        "string",
					"description": "Target package name / application ID (e.g. 'com.prism.v').",
				},
				"flow_name": map[string]any{
					"type":        "string",
					"description": "Name of the regression test flow.",
				},
				"repeat_count": map[string]any{
					"type":        "integer",
					"description": "Repetition iterations for flake verification (default 3).",
				},
				"output_path": map[string]any{
					"type":        "string",
					"description": "Optional host path where the compiled .yaml flow file should be written.",
				},
				"wrap_repeat": map[string]any{
					"type":        "boolean",
					"description": "If true, wraps commands inside a nested Maestro repeat block.",
				},
				"optimize": map[string]any{
					"type":        "boolean",
					"description": "If true, coalesces redundant sleeps and deduplicates consecutive assertions.",
				},
			},
		},
		Handler: r.handleFreezeTest,
	})

	// 7. nano_assert_ui
	r.Register(ToolDefinition{
		Name:        "nano_assert_ui",
		Description: "Asserts expected visibility, text match, check state, or node presence on current screen and records it.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"assertion_type": map[string]any{
					"type":        "string",
					"enum":        []string{"visible", "not_visible", "text_matches", "node_checked"},
					"description": "Assertion verification type.",
				},
				"target": map[string]any{
					"type":        "string",
					"description": "Target text, resource ID, or node ID to evaluate.",
				},
				"expected_text": map[string]any{
					"type":        "string",
					"description": "Expected text regex or string for 'text_matches'.",
				},
				"record": map[string]any{
					"type":        "boolean",
					"description": "Whether to record this assertion in the Maestro test session (default true).",
				},
			},
			"required": []string{"assertion_type", "target"},
		},
		Handler: r.handleAssertUI,
	})

	// 8. nano_get_session_flow
	r.Register(ToolDefinition{
		Name:        "nano_get_session_flow",
		Description: "Previews the uncommitted or compiled Maestro test flow YAML without modifying disk.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"repeat_count": map[string]any{
					"type":        "integer",
					"description": "Repeat count for the flow preview (default 3).",
				},
				"wrap_repeat": map[string]any{
					"type":        "boolean",
					"description": "Wrap actions inside nested repeat block.",
				},
			},
		},
		Handler: r.handleGetSessionFlow,
	})

	// 9. nano_clear_session
	r.Register(ToolDefinition{
		Name:        "nano_clear_session",
		Description: "Clears recorded actions in current test compiler session without restarting sandbox.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"confirm": map[string]any{
					"type":        "boolean",
					"description": "Must be true to confirm clearing recorded actions.",
				},
			},
			"required": []string{"confirm"},
		},
		Handler: r.handleClearSession,
	})

	// 10. nano_doctor
	r.Register(ToolDefinition{
		Name:        "nano_doctor",
		Description: "Runs comprehensive health diagnostic on all virtual modules, memory, AST status, and compiler readiness.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"verbose": map[string]any{
					"type":        "boolean",
					"description": "Include detailed subsystem dumps in doctor report.",
				},
			},
		},
		Handler: r.handleDoctor,
	})
}

// --- TOOL HANDLER IMPLEMENTATIONS ---

func (r *ToolRegistry) handleLaunchApp(ctx context.Context, args map[string]any) (*ToolResult, error) {
	apkPath, _ := args["apk_path"].(string)
	if apkPath == "" {
		return makeErrorResult("apk_path is required")
	}
	activity, _ := args["activity"].(string)
	clearState, _ := args["clear_state"].(bool)

	res, err := r.coord.LaunchApp(ctx, apkPath, activity, clearState)
	if err != nil {
		return makeErrorResult(fmt.Sprintf("launch failed: %v", err))
	}

	r.compiler.RecordLaunch(res.AppID, clearState)
	return makeJSONResult(res)
}

func (r *ToolRegistry) handleObserveScreen(ctx context.Context, args map[string]any) (*ToolResult, error) {
	includeScreenshot, _ := args["include_screenshot"].(bool)
	format, _ := args["format"].(string)
	if format == "" {
		format = "webp"
	}
	pruneDepth := 0
	if pd, ok := args["prune_depth"].(float64); ok {
		pruneDepth = int(pd)
	}

	res, err := r.coord.ObserveScreen(ctx, includeScreenshot, format, pruneDepth)
	if err != nil {
		return makeErrorResult(fmt.Sprintf("observe screen failed: %v", err))
	}

	if res.AST != nil {
		r.compiler.UpdateAST(res.AST)
	}

	return makeJSONResult(res)
}

func (r *ToolRegistry) handleDispatchAction(ctx context.Context, args map[string]any) (*ToolResult, error) {
	actionStr, _ := args["action"].(string)
	if actionStr == "" {
		if act, ok := args["action_type"].(string); ok {
			actionStr = act
		}
	}
	if actionStr == "" {
		return makeErrorResult("action is required ('tap', 'type', 'swipe', 'back', etc.)")
	}

	nodeID, _ := args["node_id"].(string)
	if nodeID == "" {
		if tid, ok := args["target_node_id"].(string); ok {
			nodeID = tid
		}
	}
	text, _ := args["text"].(string)
	key, _ := args["key"].(string)
	direction, _ := args["direction"].(string)
	assertVisible, _ := args["assert_visible"].(string)
	var x, y float64
	hasPoint := false
	if vx, ok := args["x"].(float64); ok {
		x = vx
		hasPoint = true
	}
	if vy, ok := args["y"].(float64); ok {
		y = vy
		hasPoint = true
	}
	durationMs := int64(0)
	if dm, ok := args["duration_ms"].(float64); ok {
		durationMs = int64(dm)
	}

	action := Action{
		Type:          actionStr,
		NodeID:        nodeID,
		Text:          text,
		Direction:     direction,
		AssertVisible: assertVisible,
		X:             x,
		Y:             y,
		DurationMs:    durationMs,
	}

	res, err := r.coord.DispatchAction(ctx, action)
	if err != nil {
		return makeErrorResult(fmt.Sprintf("dispatch action failed: %v", err))
	}

	// Record interaction in Maestro test compiler
	switch strings.ToLower(actionStr) {
	case "tap":
		r.compiler.RecordTap(nodeID, res.ResolvedText, [2]float64{x, y}, hasPoint)
	case "double_tap":
		r.compiler.RecordDoubleTap(nodeID, res.ResolvedText)
	case "long_press":
		r.compiler.RecordLongPress(nodeID, res.ResolvedText)
	case "type":
		r.compiler.RecordType(text, nodeID)
	case "erase_text":
		r.compiler.RecordEraseText()
	case "press_key":
		r.compiler.RecordPressKey(key)
	case "swipe":
		r.compiler.RecordSwipe(direction)
	case "scroll":
		r.compiler.RecordScroll()
	case "back":
		r.compiler.RecordBack()
	}

	if assertVisible != "" {
		r.compiler.RecordAssertVisible(assertVisible, "")
	}

	return makeJSONResult(res)
}

func (r *ToolRegistry) handleStepTime(ctx context.Context, args map[string]any) (*ToolResult, error) {
	var deltaMs int64
	if dVal, ok := args["delta_ms"].(float64); ok {
		deltaMs = int64(dVal)
	} else if dInt, ok := args["delta_ms"].(int64); ok {
		deltaMs = dInt
	} else {
		return makeErrorResult("delta_ms must be an integer")
	}

	if deltaMs < 0 {
		return makeErrorResult("delta_ms cannot be negative")
	}

	res, err := r.coord.StepTime(ctx, deltaMs)
	if err != nil {
		return makeErrorResult(fmt.Sprintf("step time failed: %v", err))
	}

	return makeJSONResult(res)
}

func (r *ToolRegistry) handleInspectState(ctx context.Context, args map[string]any) (*ToolResult, error) {
	target, _ := args["target"].(string)
	if target == "" {
		target = "all"
	}

	res, err := r.coord.InspectState(ctx, target)
	if err != nil {
		return makeErrorResult(fmt.Sprintf("inspect state failed: %v", err))
	}

	return makeJSONResult(res)
}

func (r *ToolRegistry) handleFreezeTest(ctx context.Context, args map[string]any) (*ToolResult, error) {
	appID, _ := args["app_id"].(string)
	flowName, _ := args["flow_name"].(string)
	if flowName == "" {
		flowName = "nano_regression_flow"
	}
	repeatCount := 3
	if rc, ok := args["repeat_count"].(float64); ok && rc > 0 {
		repeatCount = int(rc)
	}
	outputPath, _ := args["output_path"].(string)
	wrapRepeat, _ := args["wrap_repeat"].(bool)
	optimize := true
	if opt, ok := args["optimize"].(bool); ok {
		optimize = opt
	}

	opts := CompileOptions{
		AppID:           appID,
		FlowName:        flowName,
		RepeatCount:     repeatCount,
		WrapRepeat:      wrapRepeat,
		IncludeComments: true,
		OutputPath:      outputPath,
		Optimize:        optimize,
	}

	yamlContent, err := r.compiler.Compile(opts)
	if err != nil {
		return makeErrorResult(fmt.Sprintf("test compile failed: %v", err))
	}

	if outputPath != "" {
		if err := r.compiler.CompileToFile(outputPath, opts); err != nil {
			return makeErrorResult(fmt.Sprintf("failed writing test file %s: %v", outputPath, err))
		}
	}

	validationErrors := r.compiler.Validate()

	summary := map[string]any{
		"status":            "frozen",
		"flow_name":         flowName,
		"app_id":            opts.AppID,
		"repeat_count":      repeatCount,
		"command_count":     r.compiler.CommandCount(),
		"output_path":       outputPath,
		"validation_errors": validationErrors,
		"yaml":              yamlContent,
	}

	return makeJSONResult(summary)
}

func (r *ToolRegistry) handleAssertUI(ctx context.Context, args map[string]any) (*ToolResult, error) {
	assertType, _ := args["assertion_type"].(string)
	target, _ := args["target"].(string)
	if assertType == "" || target == "" {
		return makeErrorResult("assertion_type and target are required")
	}

	expectedText, _ := args["expected_text"].(string)
	record := true
	if rec, ok := args["record"].(bool); ok {
		record = rec
	}

	obs, err := r.coord.ObserveScreen(ctx, false, "webp", 0)
	if err != nil {
		return makeErrorResult(fmt.Sprintf("screen observation failed: %v", err))
	}

	ast := obs.AST
	passed := false
	var matchedNode *ASTNode
	details := ""

	switch assertType {
	case "visible":
		matchedNode = ast.FindByID(target)
		if matchedNode == nil {
			matchedNode = ast.FindByText(target)
		}
		passed = (matchedNode != nil && matchedNode.Visible)
		if passed {
			details = fmt.Sprintf("Node %q is visible", target)
			if record {
				r.compiler.RecordAssertVisible(target, "")
			}
		} else {
			details = fmt.Sprintf("Node %q was not found or not visible", target)
		}

	case "not_visible":
		matchedNode = ast.FindByID(target)
		if matchedNode == nil {
			matchedNode = ast.FindByText(target)
		}
		passed = (matchedNode == nil || !matchedNode.Visible)
		if passed {
			details = fmt.Sprintf("Node %q is absent or not visible", target)
			if record {
				r.compiler.RecordAssertNotVisible(target, "")
			}
		} else {
			details = fmt.Sprintf("Node %q is unexpectedly visible", target)
		}

	case "text_matches":
		matchedNode = ast.FindByID(target)
		if matchedNode == nil {
			matchedNode = ast.FindByText(target)
		}
		if matchedNode != nil {
			pattern := expectedText
			matched, _ := regexp.MatchString(pattern, matchedNode.Text)
			passed = matched
			if passed {
				details = fmt.Sprintf("Text %q matched pattern %q", matchedNode.Text, pattern)
				if record {
					r.compiler.RecordAssertVisible(matchedNode.Text, matchedNode.ID)
				}
			} else {
				details = fmt.Sprintf("Text %q did not match pattern %q", matchedNode.Text, pattern)
			}
		} else {
			details = fmt.Sprintf("Node %q not found", target)
		}

	case "node_checked":
		matchedNode = ast.FindByID(target)
		if matchedNode != nil {
			passed = matchedNode.Checked
			details = fmt.Sprintf("Node %q checked status: %v", target, passed)
		} else {
			details = fmt.Sprintf("Node %q not found", target)
		}

	default:
		return makeErrorResult(fmt.Sprintf("unsupported assertion_type: %s", assertType))
	}

	res := map[string]any{
		"passed":         passed,
		"assertion_type": assertType,
		"target":         target,
		"details":        details,
		"timestamp_ms":   time.Now().UnixMilli(),
	}

	return makeJSONResult(res)
}

func (r *ToolRegistry) handleGetSessionFlow(ctx context.Context, args map[string]any) (*ToolResult, error) {
	repeatCount := 3
	if rc, ok := args["repeat_count"].(float64); ok && rc > 0 {
		repeatCount = int(rc)
	}
	wrapRepeat, _ := args["wrap_repeat"].(bool)

	opts := CompileOptions{
		RepeatCount:     repeatCount,
		WrapRepeat:      wrapRepeat,
		IncludeComments: true,
		Optimize:        true,
	}

	yamlContent, err := r.compiler.Compile(opts)
	if err != nil {
		return makeErrorResult(fmt.Sprintf("failed generating flow preview: %v", err))
	}

	return makeJSONResult(map[string]any{
		"flow_preview":  yamlContent,
		"command_count": r.compiler.CommandCount(),
		"app_id":        r.compiler.GetAppID(),
	})
}

func (r *ToolRegistry) handleClearSession(ctx context.Context, args map[string]any) (*ToolResult, error) {
	confirm, _ := args["confirm"].(bool)
	if !confirm {
		return makeErrorResult("confirm must be set to true to clear the recorded session")
	}

	r.compiler.Reset()
	return makeJSONResult(map[string]any{
		"status":        "cleared",
		"command_count": 0,
	})
}

func (r *ToolRegistry) handleDoctor(ctx context.Context, args map[string]any) (*ToolResult, error) {
	health, err := r.coord.CheckHealth(ctx)
	if err != nil {
		return makeErrorResult(fmt.Sprintf("health check failed: %v", err))
	}

	validationErrors := r.compiler.Validate()
	ready := health.OverallHealthy && len(validationErrors) == 0

	report := map[string]any{
		"status":             "ok",
		"ready_for_testing":  ready,
		"health":             health,
		"validation_errors":  validationErrors,
		"commands_recorded":  r.compiler.CommandCount(),
		"active_app_id":      r.compiler.GetAppID(),
		"registered_tools":   len(r.tools),
		"protocol_version":   MCPProtocolVersion,
	}

	return makeJSONResult(report)
}

// Helpers

func makeJSONResult(v any) (*ToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return &ToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(data),
			},
		},
		IsError: false,
	}, nil
}

func makeErrorResult(msg string) (*ToolResult, error) {
	return &ToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: fmt.Sprintf("Error: %s", msg),
			},
		},
		IsError: true,
	}, nil
}
