package mcp

import (
	"context"
	"fmt"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// StubToolHandler provides a fallback implementation for a tool definition
// until the dedicated worker module implements the real engine.
type StubToolHandler struct {
	def types.ToolDefinition
}

// NewStubHandler creates a stub handler for the given definition.
func NewStubHandler(def types.ToolDefinition) *StubToolHandler {
	return &StubToolHandler{def: def}
}

func (h *StubToolHandler) Definition() types.ToolDefinition {
	return h.def
}

func (h *StubToolHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	return &types.ToolResult{
		Success: true,
		Data: map[string]any{
			"tool":    h.def.Name,
			"status":  "stub_ready",
			"message": fmt.Sprintf("Tool %s ready for worker engine integration", h.def.Name),
			"args":    args,
		},
	}, nil
}

// AllCanonicalToolDefinitions returns all 26 DroidForge MCP tool definitions.
func AllCanonicalToolDefinitions() []types.ToolDefinition {
	return []types.ToolDefinition{
		// 1. project_build
		{
			Name:        "project_build",
			Description: "Trigger Gradle build and return APK path and compiler errors",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"variant": map[string]any{"type": "string", "description": "Build variant (e.g., debug, release)"},
					"module":  map[string]any{"type": "string", "description": "Gradle module name (e.g., app)"},
					"clean":   map[string]any{"type": "boolean", "description": "Whether to run clean before build"},
				},
			},
		},
		// 2. project_test_jvm
		{
			Name:        "project_test_jvm",
			Description: "Run Robolectric and Compose UI tests on JVM without device",
			RiskTier:    types.RiskREAD,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"test_filter":       map[string]any{"type": "string", "description": "Class or method filter pattern"},
					"include_roborazzi": map[string]any{"type": "boolean", "description": "Include Roborazzi visual diff tests"},
					"record_goldens":    map[string]any{"type": "boolean", "description": "Record new golden screenshots"},
				},
			},
		},
		// 3. apk_inspect
		{
			Name:        "apk_inspect",
			Description: "Perform static analysis on APK via apkanalyzer/aapt2/jadx without source",
			RiskTier:    types.RiskREAD,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"apk_path":          map[string]any{"type": "string", "description": "Path to APK file"},
					"decompile_classes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Bounded list of classes to decompile"},
				},
				"required": []string{"apk_path"},
			},
		},
		// 4. apk_audit_compliance
		{
			Name:        "apk_audit_compliance",
			Description: "Check Play policy and Android 17 compatibility (16 KB ELF, restricted permissions)",
			RiskTier:    types.RiskREAD,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"apk_path": map[string]any{"type": "string", "description": "Path to APK file"},
					"profile":  map[string]any{"type": "string", "description": "Audit profile (e.g. standard, scam-shield, video-codec)"},
				},
				"required": []string{"apk_path"},
			},
		},
		// 5. device_list
		{
			Name:        "device_list",
			Description: "List connected ADB devices and available Android Virtual Devices (AVDs)",
			RiskTier:    types.RiskREAD,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"include_avds": map[string]any{"type": "boolean", "description": "Include available emulator AVDs in response"},
				},
			},
		},
		// 6. emulator_boot_snapshot
		{
			Name:        "emulator_boot_snapshot",
			Description: "Boot headless or GUI AVD from named golden snapshot",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"avd_name":      map[string]any{"type": "string", "description": "Name of the AVD to boot"},
					"snapshot_name": map[string]any{"type": "string", "description": "Name of the snapshot to restore"},
					"headless":      map[string]any{"type": "boolean", "description": "Whether to run headless without window"},
				},
				"required": []string{"avd_name"},
			},
		},
		// 7. device_reset_state
		{
			Name:        "device_reset_state",
			Description: "Restore device to clean state via snapshot restore or pm clear",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial":         map[string]any{"type": "string", "description": "Device serial or emulator id"},
					"strategy":       map[string]any{"type": "string", "description": "Reset strategy (snapshot_restore or pm_clear)"},
					"clear_fixtures": map[string]any{"type": "boolean", "description": "Whether to remove pushed test fixtures"},
				},
				"required": []string{"serial"},
			},
		},
		// 8. app_install_and_launch
		{
			Name:        "app_install_and_launch",
			Description: "Install APK over ADB and launch main or specified activity/deep link",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial":                map[string]any{"type": "string", "description": "Device serial"},
					"apk_path":              map[string]any{"type": "string", "description": "Path to APK file to install"},
					"activity":              map[string]any{"type": "string", "description": "Target activity name to launch"},
					"deep_link":             map[string]any{"type": "string", "description": "Deep link URI to launch via intent"},
					"grant_all_debug_perms": map[string]any{"type": "boolean", "description": "Grant all runtime permissions on install"},
				},
				"required": []string{"serial", "apk_path"},
			},
		},
		// 9. observe_screen
		{
			Name:        "observe_screen",
			Description: "Capture pruned accessibility UI tree, hashes, and optional screenshot",
			RiskTier:    types.RiskREAD,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial":             map[string]any{"type": "string", "description": "Device serial"},
					"include_screenshot": map[string]any{"type": "boolean", "description": "Attach base64 screenshot"},
					"draw_som_boxes":     map[string]any{"type": "boolean", "description": "Draw Set-of-Mark visual bounding boxes"},
					"diff_from_last":     map[string]any{"type": "boolean", "description": "Only return diff from previous observation"},
				},
				"required": []string{"serial"},
			},
		},
		// 10. act_and_observe
		{
			Name:        "act_and_observe",
			Description: "Execute atomic UI action (tap, type, swipe, key, intent) and return updated observation",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial":             map[string]any{"type": "string", "description": "Device serial"},
					"action":             map[string]any{"type": "string", "description": "Action type (tap_node, tap_xy, type, swipe, intent)"},
					"target":             map[string]any{"type": "string", "description": "Target node id, coordinates, or intent URI"},
					"text":               map[string]any{"type": "string", "description": "Text to type if action is type"},
					"settle_ms":          map[string]any{"type": "integer", "description": "Time in ms to wait for UI to settle"},
					"include_screenshot": map[string]any{"type": "boolean", "description": "Attach base64 screenshot"},
				},
				"required": []string{"serial", "action"},
			},
		},
		// 11. device_clipboard_and_text
		{
			Name:        "device_clipboard_and_text",
			Description: "Set or paste clipboard text bypassing Android 10+ clipboard restrictions",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial": map[string]any{"type": "string", "description": "Device serial"},
					"op":     map[string]any{"type": "string", "description": "Operation (set or paste)"},
					"text":   map[string]any{"type": "string", "description": "Text content to set on clipboard"},
				},
				"required": []string{"serial", "op"},
			},
		},
		// 12. device_press_system
		{
			Name:        "device_press_system",
			Description: "Send system hardware keys (back, home, recents, notifications, volume)",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial": map[string]any{"type": "string", "description": "Device serial"},
					"key":    map[string]any{"type": "string", "description": "Hardware key (back, home, recents, notifications, volume_up, volume_down)"},
				},
				"required": []string{"serial", "key"},
			},
		},
		// 13. device_wait_idle
		{
			Name:        "device_wait_idle",
			Description: "Wait until UI stabilizes or specific node appears or disappears",
			RiskTier:    types.RiskREAD,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial":     map[string]any{"type": "string", "description": "Device serial"},
					"condition":  map[string]any{"type": "string", "description": "Wait condition (ui_stable, node_visible, node_gone)"},
					"selector":   map[string]any{"type": "string", "description": "Node selector or resource ID"},
					"timeout_ms": map[string]any{"type": "integer", "description": "Timeout in milliseconds"},
				},
				"required": []string{"serial", "condition"},
			},
		},
		// 14. logcat_read
		{
			Name:        "logcat_read",
			Description: "Read filtered, nonce-fenced logcat lines since last call or with buffer cap",
			RiskTier:    types.RiskREAD,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial":          map[string]any{"type": "string", "description": "Device serial"},
					"pid_only":        map[string]any{"type": "boolean", "description": "Filter logcat to target app PID only"},
					"min_level":       map[string]any{"type": "string", "description": "Minimum log level (V, D, I, W, E, F)"},
					"tags":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Allowlisted logcat tags"},
					"since_last_call": map[string]any{"type": "boolean", "description": "Only return new log entries since last call"},
					"max_lines":       map[string]any{"type": "integer", "description": "Maximum lines to return (capped to prevent context bombs)"},
				},
				"required": []string{"serial"},
			},
		},
		// 15. crash_report_collect
		{
			Name:        "crash_report_collect",
			Description: "Collect stack traces, ANR traces, and ApplicationExitInfo exit reasons",
			RiskTier:    types.RiskREAD,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial":                    map[string]any{"type": "string", "description": "Device serial"},
					"package_id":                map[string]any{"type": "string", "description": "Target package ID"},
					"include_tombstone_summary": map[string]any{"type": "boolean", "description": "Include native crash tombstone summary"},
				},
				"required": []string{"serial", "package_id"},
			},
		},
		// 16. app_state_inspect
		{
			Name:        "app_state_inspect",
			Description: "Inspect running services, granted permissions, standby bucket, and WorkManager jobs",
			RiskTier:    types.RiskREAD,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial":     map[string]any{"type": "string", "description": "Device serial"},
					"package_id": map[string]any{"type": "string", "description": "Target package ID"},
					"sections":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Sections: services, permissions, workmanager, standby_bucket"},
				},
				"required": []string{"serial", "package_id"},
			},
		},
		// 17. sim_inject_sms_or_call
		{
			Name:        "sim_inject_sms_or_call",
			Description: "Inject incoming SMS or phone call into emulator via GSM console",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial": map[string]any{"type": "string", "description": "Emulator serial"},
					"kind":   map[string]any{"type": "string", "description": "Stimulus kind (sms or call)"},
					"sender": map[string]any{"type": "string", "description": "Phone number or sender ID"},
					"body":   map[string]any{"type": "string", "description": "SMS message body"},
				},
				"required": []string{"serial", "kind", "sender", "body"},
			},
		},
		// 18. sim_post_notification
		{
			Name:        "sim_post_notification",
			Description: "Post test notification to device status bar for NotificationListener testing",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial":  map[string]any{"type": "string", "description": "Device serial"},
					"title":   map[string]any{"type": "string", "description": "Notification title"},
					"body":    map[string]any{"type": "string", "description": "Notification content body"},
					"channel": map[string]any{"type": "string", "description": "Notification channel ID"},
					"tag":     map[string]any{"type": "string", "description": "Notification tag"},
				},
				"required": []string{"serial", "title", "body"},
			},
		},
		// 19. sim_set_permissions
		{
			Name:        "sim_set_permissions",
			Description: "Modify runtime permissions, appops, or battery standby bucket",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial":         map[string]any{"type": "string", "description": "Device serial"},
					"package_id":     map[string]any{"type": "string", "description": "Target package ID"},
					"grants":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Permissions to grant"},
					"revokes":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Permissions to revoke"},
					"appops":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Appops settings to apply"},
					"standby_bucket": map[string]any{"type": "string", "description": "App standby bucket (active, working_set, frequent, rare, restricted)"},
				},
				"required": []string{"serial", "package_id"},
			},
		},
		// 20. media_push_and_verify
		{
			Name:        "media_push_and_verify",
			Description: "Push video fixture to device, pull output, and compute SHA-256/VMAF/SSIM",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial":             map[string]any{"type": "string", "description": "Device serial"},
					"fixture_id":         map[string]any{"type": "string", "description": "Fixture identifier from fixtures manifest"},
					"output_device_path": map[string]any{"type": "string", "description": "Device file path where processed output is stored"},
					"oracle":             map[string]any{"type": "string", "description": "Verification oracle (sha256 or vmaf_ssim)"},
					"golden_hash":        map[string]any{"type": "string", "description": "Expected SHA-256 hash for bit-exact comparison"},
				},
				"required": []string{"serial", "fixture_id", "output_device_path", "oracle"},
			},
		},
		// 21. perf_capture_trace
		{
			Name:        "perf_capture_trace",
			Description: "Record system Perfetto trace and extract P95 frame times, jank %, and memory RSS",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial":       map[string]any{"type": "string", "description": "Device serial"},
					"duration_sec": map[string]any{"type": "integer", "description": "Trace recording duration in seconds"},
					"categories":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Perfetto trace categories"},
					"package_id":   map[string]any{"type": "string", "description": "Target package ID to scope metrics"},
				},
				"required": []string{"serial", "duration_sec"},
			},
		},
		// 22. power_thermal_snapshot
		{
			Name:        "power_thermal_snapshot",
			Description: "Read thermal throttling status, skin/battery temperatures, and ODPM power rails",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial":     map[string]any{"type": "string", "description": "Device serial"},
					"mode":       map[string]any{"type": "string", "description": "Measurement mode (instant, odpm_benchmark, batterystats_delta)"},
					"package_id": map[string]any{"type": "string", "description": "Target package ID"},
				},
				"required": []string{"serial"},
			},
		},
		// 23. session_freeze_to_maestro
		{
			Name:        "session_freeze_to_maestro",
			Description: "Compile recorded interactive session into deterministic Maestro YAML flow",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id":  map[string]any{"type": "string", "description": "Exploration session ID to compile"},
					"flow_name":   map[string]any{"type": "string", "description": "Target flow name"},
					"output_path": map[string]any{"type": "string", "description": "Output path for the generated Maestro YAML"},
				},
				"required": []string{"session_id", "flow_name", "output_path"},
			},
		},
		// 24. test_run_maestro
		{
			Name:        "test_run_maestro",
			Description: "Execute Maestro flow on device with optional burn-in repeat count",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"serial":       map[string]any{"type": "string", "description": "Device serial"},
					"flow_path":    map[string]any{"type": "string", "description": "Path to Maestro YAML flow file"},
					"repeat_count": map[string]any{"type": "integer", "description": "Number of burn-in executions (default 1, 3 for flake check)"},
				},
				"required": []string{"serial", "flow_path"},
			},
		},
		// 25. session_export_appcard
		{
			Name:        "session_export_appcard",
			Description: "Generate AppCard markdown summarizing discovered screens and deep links",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id":  map[string]any{"type": "string", "description": "Exploration session ID"},
					"output_path": map[string]any{"type": "string", "description": "Output path for appcard.md"},
				},
				"required": []string{"session_id", "output_path"},
			},
		},
		// 26. approval_request_status
		{
			Name:        "approval_request_status",
			Description: "Check or poll human approval status for gated high-risk actions",
			RiskTier:    types.RiskAPPROVALGATED,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action_id":       map[string]any{"type": "string", "description": "ID of requested gated action"},
					"justification":   map[string]any{"type": "string", "description": "Reason human approval is required"},
					"target_resource": map[string]any{"type": "string", "description": "Target device or package resource"},
				},
				"required": []string{"action_id"},
			},
		},
	}
}

// RegisterAllStubs registers fallback stub handlers for all 26 canonical tools.
func RegisterAllStubs(s *Server) {
	for _, def := range AllCanonicalToolDefinitions() {
		// Only register if not already registered
		if _, exists := s.GetHandler(def.Name); !exists {
			s.Register(NewStubHandler(def))
		}
	}
}
