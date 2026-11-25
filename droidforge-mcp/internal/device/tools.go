package device

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// DeviceListTool implements ToolHandler for tool #5: device_list.
type DeviceListTool struct {
	dm types.DeviceManager
}

// NewDeviceListTool creates a new device_list tool handler.
func NewDeviceListTool(dm types.DeviceManager) *DeviceListTool {
	return &DeviceListTool{dm: dm}
}

func (t *DeviceListTool) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "device_list",
		Description: "Lists connected Android devices (USB, Wi-Fi, emulators) and available AVDs with hardware specs, state, and API levels.",
		RiskTier:    types.RiskREAD,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"include_avds": map[string]any{
					"type":        "boolean",
					"description": "Whether to list offline available AVDs as well (default: true).",
				},
				"emulator_only": map[string]any{
					"type":        "boolean",
					"description": "If true, skips physical USB/Wi-Fi devices and returns only virtual devices.",
				},
			},
		},
	}
}

func (t *DeviceListTool) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	includeAVDs := coerceBool(args["include_avds"], true)

	devices, err := t.dm.ListDevices(includeAVDs)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("device_list failed: %v", err),
		}, nil
	}

	if emuOnly := coerceBool(args["emulator_only"], false); emuOnly {
		var filtered []types.DeviceInfo
		for _, d := range devices {
			if d.IsEmulator {
				filtered = append(filtered, d)
			}
		}
		devices = filtered
	}

	return &types.ToolResult{
		Success: true,
		Data: map[string]any{
			"devices": devices,
			"count":   len(devices),
		},
	}, nil
}

// EmulatorBootSnapshotTool implements ToolHandler for tool #6: emulator_boot_snapshot.
type EmulatorBootSnapshotTool struct {
	dm types.DeviceManager
}

// NewEmulatorBootSnapshotTool creates a new emulator_boot_snapshot tool handler.
func NewEmulatorBootSnapshotTool(dm types.DeviceManager) *EmulatorBootSnapshotTool {
	return &EmulatorBootSnapshotTool{dm: dm}
}

func (t *EmulatorBootSnapshotTool) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "emulator_boot_snapshot",
		Description: "Boots a headless Android Virtual Device (AVD), restores a golden snapshot, and disables animations for reliable automated testing.",
		RiskTier:    types.RiskWRITESCOPED,
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"avd_name"},
			"properties": map[string]any{
				"avd_name": map[string]any{
					"type":        "string",
					"description": "Name of the AVD to boot (e.g. 'api37_clean' or 'Pixel_7_API_34').",
				},
				"snapshot_name": map[string]any{
					"type":        "string",
					"description": "Optional snapshot name to restore after boot (e.g. 'default_boot').",
				},
				"headless": map[string]any{
					"type":        "boolean",
					"description": "Whether to run emulator in headless mode (-no-window -no-audio, default: true).",
				},
				"ram_mb": map[string]any{
					"type":        "integer",
					"description": "RAM allocation in megabytes (default: 4096).",
				},
			},
		},
	}
}

func (t *EmulatorBootSnapshotTool) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	avdName, _ := args["avd_name"].(string)
	avdName = strings.TrimSpace(avdName)
	if avdName == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "missing required parameter: avd_name",
		}, nil
	}

	snapshotName, _ := args["snapshot_name"].(string)
	snapshotName = strings.TrimSpace(snapshotName)

	headless := coerceBool(args["headless"], true)
	ramMB := coerceInt(args["ram_mb"], 4096)

	start := time.Now()
	devInfo, err := t.dm.BootSnapshot(avdName, snapshotName, headless, ramMB)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("emulator_boot_snapshot failed: %v", err),
		}, nil
	}

	bootMS := time.Since(start).Milliseconds()

	return &types.ToolResult{
		Success: true,
		Data: map[string]any{
			"serial":          devInfo.Serial,
			"boot_ms":         bootMS,
			"api_level":       devInfo.APILevel,
			"abi":             devInfo.ABI,
			"snapshot_loaded": snapshotName != "",
			"snapshot_name":   snapshotName,
			"device":          devInfo,
		},
	}, nil
}

// DeviceResetStateTool implements ToolHandler for tool #7: device_reset_state.
type DeviceResetStateTool struct {
	dm types.DeviceManager
}

// NewDeviceResetStateTool creates a new device_reset_state tool handler.
func NewDeviceResetStateTool(dm types.DeviceManager) *DeviceResetStateTool {
	return &DeviceResetStateTool{dm: dm}
}

func (t *DeviceResetStateTool) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "device_reset_state",
		Description: "Resets device or application state via quick AVD snapshot restore or package data clearing (pm clear).",
		RiskTier:    types.RiskWRITESCOPED,
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"serial", "strategy"},
			"properties": map[string]any{
				"serial": map[string]any{
					"type":        "string",
					"description": "Device serial identifier (e.g. 'emulator-5554').",
				},
				"strategy": map[string]any{
					"type":        "string",
					"enum":        []string{"snapshot_restore", "pm_clear"},
					"description": "Reset strategy to apply: 'snapshot_restore' for AVDs or 'pm_clear' for app-only reset.",
				},
				"package_id": map[string]any{
					"type":        "string",
					"description": "Package identifier to clear when strategy is 'pm_clear' (e.g. 'com.example.app').",
				},
				"snapshot_name": map[string]any{
					"type":        "string",
					"description": "Name of the snapshot to restore when strategy is 'snapshot_restore'.",
				},
				"clear_fixtures": map[string]any{
					"type":        "boolean",
					"description": "Whether to delete files in /sdcard/Download/droidforge_fixtures/* (default: false).",
				},
			},
		},
	}
}

func (t *DeviceResetStateTool) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	serial, _ := args["serial"].(string)
	strategy, _ := args["strategy"].(string)
	pkgID, _ := args["package_id"].(string)
	snapshotName, _ := args["snapshot_name"].(string)

	serial = strings.TrimSpace(serial)
	strategy = strings.TrimSpace(strategy)

	if serial == "" || strategy == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "missing required parameters: serial and strategy",
		}, nil
	}

	start := time.Now()
	err := t.dm.ResetState(serial, strategy, pkgID, snapshotName)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("device_reset_state failed: %v", err),
		}, nil
	}

	resetMS := time.Since(start).Milliseconds()

	return &types.ToolResult{
		Success: true,
		Data: map[string]any{
			"serial":     serial,
			"strategy":   strategy,
			"package_id": pkgID,
			"reset_ms":   resetMS,
		},
	}, nil
}

// AppInstallAndLaunchTool implements ToolHandler for tool #8: app_install_and_launch.
type AppInstallAndLaunchTool struct {
	dm types.DeviceManager
}

// NewAppInstallAndLaunchTool creates a new app_install_and_launch tool handler.
func NewAppInstallAndLaunchTool(dm types.DeviceManager) *AppInstallAndLaunchTool {
	return &AppInstallAndLaunchTool{dm: dm}
}

func (t *AppInstallAndLaunchTool) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "app_install_and_launch",
		Description: "Installs an APK or split APK bundle onto a target device and launches the main activity or deep link, measuring cold-start latency and returning the process PID.",
		RiskTier:    types.RiskWRITESCOPED,
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"serial", "apk_path"},
			"properties": map[string]any{
				"serial": map[string]any{
					"type":        "string",
					"description": "Device serial identifier.",
				},
				"apk_path": map[string]any{
					"type":        "string",
					"description": "Path to the APK file (or comma-separated paths for split APKs).",
				},
				"activity": map[string]any{
					"type":        "string",
					"description": "Component to launch (e.g. 'com.example/.MainActivity'). Auto-detected from APK if omitted.",
				},
				"deep_link": map[string]any{
					"type":        "string",
					"description": "Optional deep link URI to launch instead of component name.",
				},
				"grant_perms": map[string]any{
					"type":        "boolean",
					"description": "Whether to automatically grant common test runtime permissions (default: true).",
				},
			},
		},
	}
}

func (t *AppInstallAndLaunchTool) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	serial, _ := args["serial"].(string)
	apkPath, _ := args["apk_path"].(string)
	activity, _ := args["activity"].(string)
	deepLink, _ := args["deep_link"].(string)

	serial = strings.TrimSpace(serial)
	apkPath = strings.TrimSpace(apkPath)
	activity = strings.TrimSpace(activity)
	deepLink = strings.TrimSpace(deepLink)

	grantPerms := coerceBool(args["grant_perms"], true)

	if serial == "" || apkPath == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "missing required parameters: serial and apk_path",
		}, nil
	}

	result, err := t.dm.InstallAndLaunch(serial, apkPath, activity, deepLink, grantPerms)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("app_install_and_launch failed: %v", err),
		}, nil
	}

	return &types.ToolResult{
		Success: true,
		Data: map[string]any{
			"package_id":    result.PackageID,
			"pid":           result.PID,
			"install_ms":    result.InstallMS,
			"cold_start_ms": result.ColdStartMS,
			"activity":      result.Activity,
		},
	}, nil
}

// Helpers for robust argument coercion from JSON-RPC / MCP maps
func coerceBool(v any, defaultVal bool) bool {
	if v == nil {
		return defaultVal
	}
	switch b := v.(type) {
	case bool:
		return b
	case string:
		bLower := strings.ToLower(strings.TrimSpace(b))
		if bLower == "true" || bLower == "1" || bLower == "yes" {
			return true
		}
		if bLower == "false" || bLower == "0" || bLower == "no" {
			return false
		}
	}
	return defaultVal
}

func coerceInt(v any, defaultVal int) int {
	if v == nil {
		return defaultVal
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return parsed
		}
	}
	return defaultVal
}
