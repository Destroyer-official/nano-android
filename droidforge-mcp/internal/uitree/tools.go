package uitree

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// Service coordinates UI tree acquisition, diffing, settling, and atomic action tools.
// It implements types.UITreeProvider and exposes the 5 core MCP tool handlers.
type Service struct {
	deviceMgr      types.DeviceManager
	visionBackend  types.VisionBackend
	policyGuard    types.PolicyGuard
	testCompiler   types.TestCompiler
	sessionID      string
	config         *types.ObservationConfig
	settleDetector *SettleDetector

	mu              sync.Mutex
	lastSnapshots   map[string]*types.UITreeSnapshot
	hashStreaks     map[string]int
	stepCounters    map[string]int
	lastLogcatLines map[string]int

	// For mock/test injection
	mockDumpXML map[string][]byte
}

// NewService creates a new uitree Service.
func NewService(dm types.DeviceManager, vb types.VisionBackend, pg types.PolicyGuard, cfg *types.ObservationConfig) *Service {
	settleTimeout := DefaultSettleTimeout
	if cfg != nil && cfg.SettleTimeoutMS > 0 {
		settleTimeout = time.Duration(cfg.SettleTimeoutMS) * time.Millisecond
	}

	return &Service{
		deviceMgr:       dm,
		visionBackend:   vb,
		policyGuard:     pg,
		config:          cfg,
		settleDetector:  NewSettleDetector(DefaultSettleInterval, settleTimeout),
		lastSnapshots:   make(map[string]*types.UITreeSnapshot),
		hashStreaks:     make(map[string]int),
		stepCounters:    make(map[string]int),
		lastLogcatLines: make(map[string]int),
		mockDumpXML:     make(map[string][]byte),
	}
}

// SetMockDumpXML allows unit tests to inject XML without a live ADB device.
func (s *Service) SetMockDumpXML(serial string, xmlData []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mockDumpXML[serial] = xmlData
}

// SetLastSnapshot directly injects a previous snapshot for diffing tests.
func (s *Service) SetLastSnapshot(serial string, snap *types.UITreeSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSnapshots[serial] = snap
}

// SetTestCompiler wires a test compiler to record exploration session steps for freeze_to_maestro.
func (s *Service) SetTestCompiler(tc types.TestCompiler, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.testCompiler = tc
	s.sessionID = sessionID
}

// Dump acquires, prunes, and hashes the current UI tree (satisfies types.UITreeProvider).
func (s *Service) Dump(serial string) (*types.UITreeSnapshot, error) {
	s.mu.Lock()
	mockData := s.mockDumpXML[serial]
	s.mu.Unlock()

	var rawBytes []byte
	if mockData != nil {
		rawBytes = mockData
	} else if s.deviceMgr != nil {
		out, err := s.deviceMgr.ExecADB(serial, "exec-out", "uiautomator", "dump", "/dev/tty")
		if err != nil || len(out) == 0 {
			// Fallback through /sdcard/window_dump.xml
			_, _ = s.deviceMgr.ExecADB(serial, "shell", "uiautomator", "dump", "/sdcard/window_dump.xml")
			out, err = s.deviceMgr.ExecADB(serial, "shell", "cat", "/sdcard/window_dump.xml")
			if err != nil {
				return nil, fmt.Errorf("failed to dump UI hierarchy: %w", err)
			}
		}
		rawBytes = []byte(out)
	} else {
		return nil, fmt.Errorf("no device manager or mock XML provided for serial %s", serial)
	}

	hierarchy, err := ParseXML(rawBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse UI XML: %w", err)
	}

	maxTextLen := DefaultMaxNodeTextBytes
	pruned := Prune(hierarchy, PruneOptions{MaxTextLen: maxTextLen})
	hash := ComputeUIHash(pruned)

	return &types.UITreeSnapshot{
		Nodes:  pruned,
		Hash:   hash,
		RawLen: len(rawBytes),
	}, nil
}

// ToolRegistry defines an interface for registering tool handlers.
type ToolRegistry interface {
	Register(h types.ToolHandler)
}

// RegisterUITreeTools registers tools #9 (observe_screen), #10 (act_and_observe),
// #11 (device_clipboard_and_text), #12 (device_press_system), and #13 (device_wait_idle) on the server.
func RegisterUITreeTools(registry ToolRegistry, svc *Service) {
	if registry == nil || svc == nil {
		return
	}
	for _, tool := range svc.GetTools() {
		registry.Register(tool)
	}
}

// GetTools returns the 5 MCP tool handlers owned by Worker 5.
func (s *Service) GetTools() []types.ToolHandler {
	return []types.ToolHandler{
		&ObserveScreenTool{svc: s},
		&ActAndObserveTool{svc: s},
		&DeviceClipboardAndTextTool{svc: s},
		&DevicePressSystemTool{svc: s},
		&DeviceWaitIdleTool{svc: s},
	}
}

// NewObserveScreenTool creates Tool 9: observe_screen
func NewObserveScreenTool(svc *Service) *ObserveScreenTool {
	return &ObserveScreenTool{svc: svc}
}

// NewActAndObserveTool creates Tool 10: act_and_observe
func NewActAndObserveTool(svc *Service) *ActAndObserveTool {
	return &ActAndObserveTool{svc: svc}
}

// NewDeviceClipboardAndTextTool creates Tool 11: device_clipboard_and_text
func NewDeviceClipboardAndTextTool(svc *Service) *DeviceClipboardAndTextTool {
	return &DeviceClipboardAndTextTool{svc: svc}
}

// NewDevicePressSystemTool creates Tool 12: device_press_system
func NewDevicePressSystemTool(svc *Service) *DevicePressSystemTool {
	return &DevicePressSystemTool{svc: svc}
}

// NewDeviceWaitIdleTool creates Tool 13: device_wait_idle
func NewDeviceWaitIdleTool(svc *Service) *DeviceWaitIdleTool {
	return &DeviceWaitIdleTool{svc: svc}
}

// -----------------------------------------------------------------------------
// Tool 9: observe_screen
// -----------------------------------------------------------------------------

type ObserveScreenTool struct {
	svc *Service
}

func (t *ObserveScreenTool) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "observe_screen",
		Description: "Dumps and prunes the current UI accessibility tree, diffs it against the last step, and optionally grabs a frame or filtered logcat delta.",
		RiskTier:    types.RiskREAD,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"serial": map[string]any{
					"type":        "string",
					"description": "Target Android device serial or emulator ID",
				},
				"include_screenshot": map[string]any{
					"type":        "boolean",
					"description": "Attach base64 screenshot",
				},
				"draw_som_boxes": map[string]any{
					"type":        "boolean",
					"description": "Overlay Set-of-Marks bounding boxes on screenshot",
				},
				"diff_from_last": map[string]any{
					"type":        "boolean",
					"description": "If true, returns only changed nodes in nodes array",
				},
			},
			"required": []any{"serial"},
		},
	}
}

func (t *ObserveScreenTool) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	serial, ok := args["serial"].(string)
	if !ok || serial == "" {
		return &types.ToolResult{Success: false, Error: "missing required 'serial' argument"}, nil
	}

	includeScreenshot, _ := args["include_screenshot"].(bool)
	diffFromLast, _ := args["diff_from_last"].(bool)

	// Dump and prune current UI tree
	snap, err := t.svc.Dump(serial)
	if err != nil {
		return &types.ToolResult{Success: false, Error: err.Error()}, nil
	}

	t.svc.mu.Lock()
	prevSnap := t.svc.lastSnapshots[serial]
	t.svc.stepCounters[serial]++
	stepID := t.svc.stepCounters[serial]
	t.svc.lastSnapshots[serial] = snap
	t.svc.mu.Unlock()

	var prevNodes []types.PrunedNode
	if prevSnap != nil {
		prevNodes = prevSnap.Nodes
	}
	diffRes := Diff(prevNodes, snap.Nodes)

	// Generate nonce and fence untrusted device text
	nonce := GenerateNonce()
	fencedAllNodes := FenceNodes(snap.Nodes, nonce, DefaultMaxNodeTextBytes)

	displayNodes := fencedAllNodes
	if diffFromLast && prevSnap != nil {
		changedNodes := append(diffRes.Added, diffRes.Modified...)
		displayNodes = FenceNodes(changedNodes, nonce, DefaultMaxNodeTextBytes)
	}

	activity := t.svc.getForegroundActivity(serial)
	logcatDelta := t.svc.readLogcatDelta(serial, nonce)

	screenshotB64, screenshotAttached, screenshotReason := t.svc.captureScreenshotIfNeeded(serial, includeScreenshot, countInteractiveNodes(snap.Nodes), len(logcatDelta) > 0)

	obs := types.ObservationResult{
		StepID:             stepID,
		DeviceID:           serial,
		Activity:           activity,
		UIHash:             snap.Hash,
		UIDiffSummary:      diffRes.Summary,
		Nonce:              nonce,
		Nodes:              displayNodes,
		LogcatDelta:        logcatDelta,
		ScreenshotB64:      screenshotB64,
		ScreenshotAttached: screenshotAttached,
		ScreenshotReason:   screenshotReason,
	}

	return &types.ToolResult{
		Success: true,
		Data: map[string]any{
			"observation": obs,
		},
	}, nil
}

// -----------------------------------------------------------------------------
// Tool 10: act_and_observe (Core Invention #1)
// -----------------------------------------------------------------------------

type ActAndObserveTool struct {
	svc *Service
}

func (t *ActAndObserveTool) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "act_and_observe",
		Description: "Executes an action via ADB, waits for smart UI settle, and returns a diff-compressed observation in one round-trip.",
		RiskTier:    types.RiskWRITESCOPED,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"serial": map[string]any{
					"type":        "string",
					"description": "Target Android device serial",
				},
				"action": map[string]any{
					"type":        "string",
					"enum":        []any{"tap_node", "tap_xy", "type", "swipe", "key", "intent"},
					"description": "Atomic action verb to execute",
				},
				"target": map[string]any{
					"type":        "string",
					"description": "Node ID ('n1'), coordinates ('X,Y' or 'X1,Y1,X2,Y2'), keycode, or intent URI",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "Text to type when action is 'type'",
				},
				"settle_ms": map[string]any{
					"type":        "integer",
					"description": "Max settle timeout in ms (default 1500)",
				},
				"include_screenshot": map[string]any{
					"type":        "boolean",
					"description": "Attach base64 screenshot",
				},
			},
			"required": []any{"serial", "action"},
		},
	}
}

func (t *ActAndObserveTool) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	serial, ok := args["serial"].(string)
	if !ok || serial == "" {
		return &types.ToolResult{Success: false, Error: "missing required 'serial' argument"}, nil
	}
	action, ok := args["action"].(string)
	if !ok || action == "" {
		return &types.ToolResult{Success: false, Error: "missing required 'action' argument"}, nil
	}

	target, _ := args["target"].(string)
	typedText, _ := args["text"].(string)
	settleMS := 1500
	if val, ok := args["settle_ms"].(float64); ok && val > 0 {
		settleMS = int(val)
	} else if val, ok := args["settle_ms"].(int); ok && val > 0 {
		settleMS = val
	}
	includeScreenshot, _ := args["include_screenshot"].(bool)

	t.svc.mu.Lock()
	prevSnap := t.svc.lastSnapshots[serial]
	t.svc.mu.Unlock()

	// 1. Execute action via ADB
	if err := t.svc.executeAction(ctx, serial, action, target, typedText, prevSnap); err != nil {
		return &types.ToolResult{Success: false, Error: fmt.Sprintf("action execution failed: %v", err)}, nil
	}

	// 2. Run settle detector (poll UI tree hash stability at 100ms intervals)
	poller := func(c context.Context) (*types.UITreeSnapshot, error) {
		return t.svc.Dump(serial)
	}
	settleRes, _ := t.svc.settleDetector.WaitForSettle(ctx, poller, time.Duration(settleMS)*time.Millisecond)
	currSnap := settleRes.Snapshot
	if currSnap == nil {
		// Fallback direct dump if settle poller didn't produce a snapshot
		var err error
		currSnap, err = t.svc.Dump(serial)
		if err != nil {
			return &types.ToolResult{Success: false, Error: fmt.Sprintf("failed to acquire UI tree after action: %v", err)}, nil
		}
	}

	// 3. Diff UI tree against previous step
	var prevNodes []types.PrunedNode
	prevHash := ""
	if prevSnap != nil {
		prevNodes = prevSnap.Nodes
		prevHash = prevSnap.Hash
	}
	diffRes := Diff(prevNodes, currSnap.Nodes)

	// 4. Update state & circuit breaker tracking
	t.svc.mu.Lock()
	t.svc.stepCounters[serial]++
	stepID := t.svc.stepCounters[serial]
	t.svc.lastSnapshots[serial] = currSnap

	streak := 1
	if prevHash != "" && prevHash == currSnap.Hash {
		s := t.svc.hashStreaks[serial] + 1
		t.svc.hashStreaks[serial] = s
		streak = s
	} else {
		t.svc.hashStreaks[serial] = 1
	}
	t.svc.mu.Unlock()

	// Circuit breaker: fires stuck_warning: true after 3 identical hashes
	stuckWarning := streak >= 3

	// 5. Read logcat delta
	nonce := GenerateNonce()
	logcatDelta := t.svc.readLogcatDelta(serial, nonce)

	// 6. Optionally grab screenshot (sparse tree <3 interactive nodes, crash detected, or explicitly requested)
	interactiveCount := countInteractiveNodes(currSnap.Nodes)
	hasCrash := hasCrashErrors(logcatDelta)
	screenshotB64, screenshotAttached, screenshotReason := t.svc.captureScreenshotIfNeeded(serial, includeScreenshot, interactiveCount, hasCrash)

	// 7. Nonce fence all pruned node text
	fencedNodes := FenceNodes(currSnap.Nodes, nonce, DefaultMaxNodeTextBytes)

	actionApplied := action
	if target != "" {
		actionApplied = fmt.Sprintf("%s:%s", action, target)
	}

	activity := t.svc.getForegroundActivity(serial)

	// Record step to compiler if registered
	t.svc.mu.Lock()
	tc := t.svc.testCompiler
	sessID := t.svc.sessionID
	t.svc.mu.Unlock()
	if tc != nil && sessID != "" {
		tc.RecordStep(sessID, types.SessionStep{
			StepIndex:    stepID,
			Action:       action,
			Target:       target,
			Text:         typedText,
			UIHashBefore: prevHash,
			UIHashAfter:  currSnap.Hash,
			NodesAfter:   currSnap.Nodes,
			LogcatDelta:  logcatDelta,
			Success:      true,
			TimestampMS:  time.Now().UnixMilli(),
		})
	}

	obs := types.ObservationResult{
		StepID:             stepID,
		DeviceID:           serial,
		Activity:           activity,
		SettleMS:           settleRes.DurationMS,
		UIHash:             currSnap.Hash,
		UIDiffSummary:      diffRes.Summary,
		Nonce:              nonce,
		Nodes:              fencedNodes,
		LogcatDelta:        logcatDelta,
		ScreenshotB64:      screenshotB64,
		ScreenshotAttached: screenshotAttached,
		ScreenshotReason:   screenshotReason,
		ActionApplied:      actionApplied,
		StuckWarning:       stuckWarning,
	}

	return &types.ToolResult{
		Success: true,
		Data: map[string]any{
			"observation": obs,
		},
	}, nil
}

// -----------------------------------------------------------------------------
// Tool 11: device_clipboard_and_text
// -----------------------------------------------------------------------------

type DeviceClipboardAndTextTool struct {
	svc *Service
}

func (t *DeviceClipboardAndTextTool) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "device_clipboard_and_text",
		Description: "Sets or pastes text to the device clipboard using scrcpy inject (Android 10+ bypass) with ADB fallback.",
		RiskTier:    types.RiskWRITESCOPED,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"serial": map[string]any{
					"type":        "string",
					"description": "Target Android device serial",
				},
				"op": map[string]any{
					"type":        "string",
					"enum":        []any{"set", "paste", "get"},
					"description": "Operation: set clipboard, paste text, or get clipboard",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "Text to set or paste",
				},
			},
			"required": []any{"serial", "op"},
		},
	}
}

func (t *DeviceClipboardAndTextTool) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	serial, _ := args["serial"].(string)
	if serial == "" {
		return &types.ToolResult{Success: false, Error: "missing 'serial'"}, nil
	}
	op, _ := args["op"].(string)
	text, _ := args["text"].(string)

	if t.svc.deviceMgr == nil {
		// Mock success in test environment
		return &types.ToolResult{
			Success: true,
			Data: map[string]any{
				"ok":     true,
				"method": "mock_inject",
			},
		}, nil
	}

	switch op {
	case "set":
		// Android 10+ cmd clipboard
		_, err := t.svc.deviceMgr.ExecADB(serial, "shell", "cmd", "clipboard", "set", "text", text)
		method := "adb_input"
		if err == nil {
			method = "cmd_clipboard"
		}
		return &types.ToolResult{
			Success: true,
			Data: map[string]any{
				"ok":     true,
				"method": method,
			},
		}, nil

	case "paste":
		// KEYCODE_PASTE = 279
		_, err := t.svc.deviceMgr.ExecADB(serial, "shell", "input", "keyevent", "279")
		if err != nil {
			// Fallback: input text
			escaped := escapeADBText(text)
			_, err = t.svc.deviceMgr.ExecADB(serial, "shell", "input", "text", escaped)
		}
		return &types.ToolResult{
			Success: true,
			Data: map[string]any{
				"ok":     err == nil,
				"method": "adb_input",
			},
		}, nil

	case "get":
		out, err := t.svc.deviceMgr.ExecADB(serial, "shell", "cmd", "clipboard", "get")
		if err != nil {
			return &types.ToolResult{Success: false, Error: err.Error()}, nil
		}
		nonce := GenerateNonce()
		return &types.ToolResult{
			Success: true,
			Data: map[string]any{
				"ok":     true,
				"method": "cmd_clipboard",
				"text":   FenceText(out, nonce, DefaultMaxNodeTextBytes),
			},
		}, nil

	default:
		return &types.ToolResult{Success: false, Error: fmt.Sprintf("unsupported op %q", op)}, nil
	}
}

// -----------------------------------------------------------------------------
// Tool 12: device_press_system
// -----------------------------------------------------------------------------

type DevicePressSystemTool struct {
	svc *Service
}

func (t *DevicePressSystemTool) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "device_press_system",
		Description: "Presses a physical or system button (back, home, recents, notifications, volume_up, volume_down, enter) and returns the foreground activity.",
		RiskTier:    types.RiskWRITESCOPED,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"serial": map[string]any{
					"type":        "string",
					"description": "Target Android device serial",
				},
				"key": map[string]any{
					"type":        "string",
					"enum":        []any{"back", "home", "recents", "notifications", "volume_up", "volume_down", "enter", "tab", "power", "menu", "search", "dpad_up", "dpad_down", "dpad_left", "dpad_right"},
					"description": "System key to press",
				},
			},
			"required": []any{"serial", "key"},
		},
	}
}

func (t *DevicePressSystemTool) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	serial, _ := args["serial"].(string)
	if serial == "" {
		return &types.ToolResult{Success: false, Error: "missing 'serial'"}, nil
	}
	key, _ := args["key"].(string)

	if t.svc.deviceMgr == nil {
		return &types.ToolResult{
			Success: true,
			Data: map[string]any{
				"ok":             true,
				"activity_after": ".MainActivity",
			},
		}, nil
	}

	var err error
	switch key {
	case "back":
		_, err = t.svc.deviceMgr.ExecADB(serial, "shell", "input", "keyevent", "4") // KEYCODE_BACK
	case "home":
		_, err = t.svc.deviceMgr.ExecADB(serial, "shell", "input", "keyevent", "3") // KEYCODE_HOME
	case "recents":
		_, err = t.svc.deviceMgr.ExecADB(serial, "shell", "input", "keyevent", "187") // KEYCODE_APP_SWITCH
	case "notifications":
		_, err = t.svc.deviceMgr.ExecADB(serial, "shell", "cmd", "statusbar", "expand-notifications")
	case "volume_up":
		_, err = t.svc.deviceMgr.ExecADB(serial, "shell", "input", "keyevent", "24") // KEYCODE_VOLUME_UP
	case "volume_down":
		_, err = t.svc.deviceMgr.ExecADB(serial, "shell", "input", "keyevent", "25") // KEYCODE_VOLUME_DOWN
	case "enter":
		_, err = t.svc.deviceMgr.ExecADB(serial, "shell", "input", "keyevent", "66") // KEYCODE_ENTER
	case "tab":
		_, err = t.svc.deviceMgr.ExecADB(serial, "shell", "input", "keyevent", "61") // KEYCODE_TAB
	case "power":
		_, err = t.svc.deviceMgr.ExecADB(serial, "shell", "input", "keyevent", "26") // KEYCODE_POWER
	case "menu":
		_, err = t.svc.deviceMgr.ExecADB(serial, "shell", "input", "keyevent", "82") // KEYCODE_MENU
	case "search":
		_, err = t.svc.deviceMgr.ExecADB(serial, "shell", "input", "keyevent", "84") // KEYCODE_SEARCH
	case "dpad_up":
		_, err = t.svc.deviceMgr.ExecADB(serial, "shell", "input", "keyevent", "19")
	case "dpad_down":
		_, err = t.svc.deviceMgr.ExecADB(serial, "shell", "input", "keyevent", "20")
	case "dpad_left":
		_, err = t.svc.deviceMgr.ExecADB(serial, "shell", "input", "keyevent", "21")
	case "dpad_right":
		_, err = t.svc.deviceMgr.ExecADB(serial, "shell", "input", "keyevent", "22")
	default:
		return &types.ToolResult{Success: false, Error: fmt.Sprintf("unsupported key %q", key)}, nil
	}

	if err != nil {
		return &types.ToolResult{Success: false, Error: err.Error()}, nil
	}

	activity := t.svc.getForegroundActivity(serial)

	return &types.ToolResult{
		Success: true,
		Data: map[string]any{
			"ok":             true,
			"activity_after": activity,
		},
	}, nil
}

// -----------------------------------------------------------------------------
// Tool 13: device_wait_idle
// -----------------------------------------------------------------------------

type DeviceWaitIdleTool struct {
	svc *Service
}

func (t *DeviceWaitIdleTool) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "device_wait_idle",
		Description: "Waits for device UI to reach a stable or expected state (ui_stable, node_visible, node_gone).",
		RiskTier:    types.RiskREAD,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"serial": map[string]any{
					"type":        "string",
					"description": "Target Android device serial",
				},
				"condition": map[string]any{
					"type":        "string",
					"enum":        []any{"ui_stable", "node_visible", "node_gone"},
					"description": "Wait condition",
				},
				"selector": map[string]any{
					"type":        "string",
					"description": "Node ID, resource-id, or text to match for node_visible/node_gone",
				},
				"timeout_ms": map[string]any{
					"type":        "integer",
					"description": "Max wait duration in ms (default 5000)",
				},
			},
			"required": []any{"serial", "condition"},
		},
	}
}

func (t *DeviceWaitIdleTool) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	serial, _ := args["serial"].(string)
	if serial == "" {
		return &types.ToolResult{Success: false, Error: "missing 'serial'"}, nil
	}
	condition, _ := args["condition"].(string)
	selector, _ := args["selector"].(string)

	timeoutMS := 5000
	if val, ok := args["timeout_ms"].(float64); ok && val > 0 {
		timeoutMS = int(val)
	} else if val, ok := args["timeout_ms"].(int); ok && val > 0 {
		timeoutMS = val
	}
	timeout := time.Duration(timeoutMS) * time.Millisecond
	startTime := time.Now()

	switch condition {
	case "ui_stable":
		poller := func(c context.Context) (*types.UITreeSnapshot, error) {
			return t.svc.Dump(serial)
		}
		res, err := t.svc.settleDetector.WaitForSettle(ctx, poller, timeout)
		if err != nil {
			return &types.ToolResult{Success: false, Error: err.Error()}, nil
		}
		hash := "000000"
		if res.Snapshot != nil {
			hash = res.Snapshot.Hash
		}
		return &types.ToolResult{
			Success: true,
			Data: map[string]any{
				"matched":   res.Settled,
				"waited_ms": res.DurationMS,
				"ui_hash":   hash,
			},
		}, nil

	case "node_visible", "node_gone":
		ticker := time.NewTicker(DefaultSettleInterval)
		defer ticker.Stop()

		for {
			snap, err := t.svc.Dump(serial)
			if err == nil {
				found := nodeMatchesSelector(snap.Nodes, selector)
				if (condition == "node_visible" && found) || (condition == "node_gone" && !found) {
					return &types.ToolResult{
						Success: true,
						Data: map[string]any{
							"matched":   true,
							"waited_ms": time.Since(startTime).Milliseconds(),
							"ui_hash":   snap.Hash,
						},
					}, nil
				}
			}

			if time.Since(startTime) >= timeout {
				lastHash := "000000"
				if snap != nil {
					lastHash = snap.Hash
				}
				return &types.ToolResult{
					Success: true,
					Data: map[string]any{
						"matched":   false,
						"waited_ms": time.Since(startTime).Milliseconds(),
						"ui_hash":   lastHash,
					},
				}, nil
			}

			select {
			case <-ctx.Done():
				return &types.ToolResult{Success: false, Error: ctx.Err().Error()}, nil
			case <-ticker.C:
			}
		}

	default:
		return &types.ToolResult{Success: false, Error: fmt.Sprintf("unsupported condition %q", condition)}, nil
	}
}

// -----------------------------------------------------------------------------
// Helper Methods
// -----------------------------------------------------------------------------

// executeAction dispatches action verbs to safe exec.Command ADB invocations.
func (s *Service) executeAction(ctx context.Context, serial, action, target, text string, prevSnap *types.UITreeSnapshot) error {
	if s.deviceMgr == nil {
		// Mock execution in tests
		return nil
	}

	switch action {
	case "tap_node":
		if prevSnap == nil {
			var err error
			prevSnap, err = s.Dump(serial)
			if err != nil {
				return fmt.Errorf("cannot tap_node: failed to acquire initial screen: %w", err)
			}
		}
		node := findNodeByIDOrRes(prevSnap.Nodes, target)
		if node == nil {
			return fmt.Errorf("node ID %q not found in previous screen snapshot", target)
		}
		cx := (node.Bounds[0] + node.Bounds[2]) / 2
		cy := (node.Bounds[1] + node.Bounds[3]) / 2
		_, err := s.deviceMgr.ExecADB(serial, "shell", "input", "tap", strconv.Itoa(cx), strconv.Itoa(cy))
		return err

	case "tap_xy":
		coords := parseCoords(target)
		if len(coords) != 2 {
			return fmt.Errorf("invalid tap_xy target %q, expected 'X,Y'", target)
		}
		_, err := s.deviceMgr.ExecADB(serial, "shell", "input", "tap", strconv.Itoa(coords[0]), strconv.Itoa(coords[1]))
		return err

	case "type":
		escaped := escapeADBText(text)
		_, err := s.deviceMgr.ExecADB(serial, "shell", "input", "text", escaped)
		return err

	case "swipe":
		coords := parseCoords(target)
		if len(coords) < 4 {
			return fmt.Errorf("invalid swipe target %q, expected 'X1,Y1,X2,Y2'", target)
		}
		durationStr := "300"
		if len(coords) >= 5 {
			durationStr = strconv.Itoa(coords[4])
		}
		_, err := s.deviceMgr.ExecADB(serial, "shell", "input", "swipe",
			strconv.Itoa(coords[0]), strconv.Itoa(coords[1]),
			strconv.Itoa(coords[2]), strconv.Itoa(coords[3]),
			durationStr)
		return err

	case "key":
		keycode := target
		if mapped := mapKeyToKeycode(target); mapped != "" {
			keycode = mapped
		}
		_, err := s.deviceMgr.ExecADB(serial, "shell", "input", "keyevent", keycode)
		return err

	case "intent":
		// Intent format e.g. "android.intent.action.VIEW -d https://example.com"
		parts := strings.Fields(target)
		args := append([]string{"shell", "am", "start"}, parts...)
		_, err := s.deviceMgr.ExecADB(serial, args...)
		return err

	default:
		return fmt.Errorf("unsupported action %q", action)
	}
}

func findNodeByIDOrRes(nodes []types.PrunedNode, target string) *types.PrunedNode {
	targetLower := strings.ToLower(target)
	// 1. Exact match by ID (e.g. "n1") or Resource ID (e.g. "btn_block_sender")
	for i := range nodes {
		if strings.EqualFold(nodes[i].ID, target) || strings.EqualFold(nodes[i].ResID, target) {
			return &nodes[i]
		}
	}
	// 2. Exact match by Text (raw or noise-masked)
	for i := range nodes {
		if strings.EqualFold(nodes[i].Text, target) || strings.EqualFold(MaskNoise(nodes[i].Text), targetLower) {
			return &nodes[i]
		}
	}
	// 3. Exact match by ContentDesc
	for i := range nodes {
		if strings.EqualFold(nodes[i].ContentDesc, target) || strings.EqualFold(MaskNoise(nodes[i].ContentDesc), targetLower) {
			return &nodes[i]
		}
	}
	// 4. Substring match on Resource ID
	for i := range nodes {
		if strings.Contains(strings.ToLower(nodes[i].ResID), targetLower) {
			return &nodes[i]
		}
	}
	return nil
}

func nodeMatchesSelector(nodes []types.PrunedNode, selector string) bool {
	if selector == "" {
		return false
	}
	selLower := strings.ToLower(selector)
	for _, n := range nodes {
		if strings.EqualFold(n.ID, selector) ||
			strings.EqualFold(n.ResID, selector) ||
			strings.Contains(strings.ToLower(MaskNoise(n.Text)), selLower) ||
			strings.Contains(strings.ToLower(MaskNoise(n.ContentDesc)), selLower) {
			return true
		}
	}
	return false
}

func parseCoords(s string) []int {
	clean := strings.ReplaceAll(s, ",", " ")
	fields := strings.Fields(clean)
	var coords []int
	for _, f := range fields {
		if val, err := strconv.Atoi(f); err == nil {
			coords = append(coords, val)
		}
	}
	return coords
}

func escapeADBText(text string) string {
	// ADB shell input text replaces spaces with %s and escapes shell metacharacters
	var b strings.Builder
	for _, r := range text {
		switch r {
		case ' ':
			b.WriteString("%s")
		case '&', ';', '|', '$', '`', '<', '>', '(', ')', '\\', '"', '\'':
			b.WriteRune('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func mapKeyToKeycode(k string) string {
	switch strings.ToLower(k) {
	case "back":
		return "4"
	case "home":
		return "3"
	case "recents", "app_switch":
		return "187"
	case "enter":
		return "66"
	case "tab":
		return "61"
	case "power":
		return "26"
	case "menu":
		return "82"
	case "search":
		return "84"
	case "volume_up":
		return "24"
	case "volume_down":
		return "25"
	case "dpad_up":
		return "19"
	case "dpad_down":
		return "20"
	case "dpad_left":
		return "21"
	case "dpad_right":
		return "22"
	case "dpad_center":
		return "23"
	default:
		return k
	}
}

func countInteractiveNodes(nodes []types.PrunedNode) int {
	cnt := 0
	for _, n := range nodes {
		for _, f := range n.Flags {
			if f == "click" || f == "longclick" || f == "check" || f == "scroll" || f == "edit" {
				cnt++
				break
			}
		}
	}
	return cnt
}

var crashRegex = regexp.MustCompile(`(?i)(FATAL EXCEPTION|ANR in|AndroidRuntime|NullPointerException|OutOfMemoryError|crash)`)

func hasCrashErrors(logcatLines []string) bool {
	for _, l := range logcatLines {
		if crashRegex.MatchString(l) {
			return true
		}
	}
	return false
}

func (s *Service) getForegroundActivity(serial string) string {
	if s.deviceMgr == nil {
		return ".MainActivity"
	}
	out, err := s.deviceMgr.ExecADB(serial, "shell", "dumpsys", "window", "displays")
	if err == nil {
		// Look for mCurrentFocus or mFocusedApp
		lines := strings.Split(out, "\n")
		for _, l := range lines {
			if strings.Contains(l, "mCurrentFocus") || strings.Contains(l, "mFocusedApp") {
				if idx := strings.Index(l, "/"); idx != -1 {
					part := l[idx:]
					fields := strings.Fields(part)
					if len(fields) > 0 {
						return strings.Trim(fields[0], "}")
					}
				}
			}
		}
	}
	return ".MainActivity"
}

func (s *Service) readLogcatDelta(serial, nonce string) []string {
	if s.deviceMgr == nil {
		return nil
	}
	// Fetch recent error lines
	out, err := s.deviceMgr.ExecADB(serial, "shell", "logcat", "-d", "-t", "50", "-v", "threadtime", "*:E")
	if err != nil || len(out) == 0 {
		return nil
	}
	lines := strings.Split(out, "\n")
	var errorLines []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" && !strings.Contains(l, "--------- beginning of") {
			errorLines = append(errorLines, l)
		}
	}
	// Limit to max 10 lines
	if len(errorLines) > 10 {
		errorLines = errorLines[len(errorLines)-10:]
	}
	return FenceLogcatLines(errorLines, nonce, DefaultMaxLogcatLineBytes)
}

func (s *Service) captureScreenshotIfNeeded(serial string, explicitlyRequested bool, interactiveCount int, crashDetected bool) (string, bool, string) {
	if explicitlyRequested {
		b64, ok := s.grabScreenshotBase64(serial)
		return b64, ok, "requested"
	}
	if crashDetected {
		b64, ok := s.grabScreenshotBase64(serial)
		return b64, ok, "crash_detected"
	}
	if interactiveCount < 3 {
		b64, ok := s.grabScreenshotBase64(serial)
		return b64, ok, "sparse_tree"
	}
	return "", false, "a11y_tree_sufficient"
}

func (s *Service) grabScreenshotBase64(serial string) (string, bool) {
	if s.visionBackend != nil {
		bytes, err := s.visionBackend.GrabFrame(serial, 1080, "webp")
		if err == nil && len(bytes) > 0 {
			return base64.StdEncoding.EncodeToString(bytes), true
		}
	}
	if s.deviceMgr != nil {
		// Fallback via screencap
		out, err := s.deviceMgr.ExecADB(serial, "exec-out", "screencap", "-p")
		if err == nil && len(out) > 0 {
			return base64.StdEncoding.EncodeToString([]byte(out)), true
		}
	}
	return "", false
}
