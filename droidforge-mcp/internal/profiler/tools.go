package profiler

import (
	"context"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// PerfCaptureTraceHandler implements types.ToolHandler for perf_capture_trace (#21).
type PerfCaptureTraceHandler struct {
	DM      types.DeviceManager
	Sidecar SidecarRunner
}

func (h *PerfCaptureTraceHandler) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "perf_capture_trace",
		Description: "Captures a Perfetto system trace on Android and analyzes frame latency, memory, and throttling via Python sidecar",
		RiskTier:    types.RiskWRITESCOPED,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"serial": map[string]any{
					"type":        "string",
					"description": "Target device serial",
				},
				"package_id": map[string]any{
					"type":        "string",
					"description": "Target package ID to profile",
				},
				"duration_sec": map[string]any{
					"type":        "integer",
					"description": "Trace duration in seconds (default: 10)",
				},
				"categories": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Trace categories (sched, freq, mem, gfx, am, binder)",
				},
				"output_path": map[string]any{
					"type":        "string",
					"description": "Local host destination for .perfetto-trace file",
				},
			},
			"required": []string{"serial"},
		},
	}
}

func (h *PerfCaptureTraceHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	serial, _ := args["serial"].(string)
	pkg, _ := args["package_id"].(string)
	outputPath, _ := args["output_path"].(string)

	durationSec := 10
	if d, ok := args["duration_sec"].(float64); ok && d > 0 {
		durationSec = int(d)
	} else if d, ok := args["duration_sec"].(int); ok && d > 0 {
		durationSec = d
	}

	var categories []string
	if rawCats, ok := args["categories"].([]any); ok {
		for _, c := range rawCats {
			if s, ok := c.(string); ok {
				categories = append(categories, s)
			}
		}
	} else if strCats, ok := args["categories"].([]string); ok {
		categories = strCats
	}

	req := PerfTraceRequest{
		Serial:      serial,
		PackageID:   pkg,
		DurationSec: durationSec,
		Categories:  categories,
		OutputPath:  outputPath,
	}

	runner := h.Sidecar
	if runner == nil {
		runner = NewPythonSidecarRunner("")
	}

	res, err := CapturePerfettoTrace(ctx, h.DM, runner, req)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &types.ToolResult{
		Success: true,
		Data: map[string]any{
			"trace_path":            res.TracePath,
			"p95_frame_ms":          res.P95FrameMS,
			"jank_pct":              res.JankPct,
			"peak_rss_mb":           res.PeakRSSMB,
			"peak_anonswap_mb":      res.PeakAnonSwapMB,
			"memory_limiter_events": res.MemoryLimiterEvents,
		},
	}, nil
}

// PowerThermalSnapshotHandler implements types.ToolHandler for power_thermal_snapshot (#22).
type PowerThermalSnapshotHandler struct {
	DM types.DeviceManager
}

func (h *PowerThermalSnapshotHandler) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "power_thermal_snapshot",
		Description: "Captures instant or delta thermal status and battery power metrics via dumpsys",
		RiskTier:    types.RiskREAD,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"serial": map[string]any{
					"type":        "string",
					"description": "Target device serial",
				},
				"duration_s": map[string]any{
					"type":        "integer",
					"description": "Duration in seconds to measure delta (default: 0 for instant)",
				},
				"mode": map[string]any{
					"type":        "string",
					"enum":        []string{"instant", "odpm_benchmark", "batterystats_delta"},
					"description": "Capture mode",
				},
				"package_id": map[string]any{
					"type":        "string",
					"description": "Package ID for process-specific power attribution",
				},
			},
			"required": []string{"serial"},
		},
	}
}

func (h *PowerThermalSnapshotHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	serial, _ := args["serial"].(string)
	mode, _ := args["mode"].(string)
	pkg, _ := args["package_id"].(string)

	durationS := 0
	if d, ok := args["duration_s"].(float64); ok {
		durationS = int(d)
	} else if d, ok := args["duration_s"].(int); ok {
		durationS = d
	}

	res, err := PowerThermalSnapshot(ctx, h.DM, serial, durationS, mode, pkg)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	data := map[string]any{
		"thermal_status":   res.ThermalStatus,
		"skin_temp_c":      res.SkinTempC,
		"battery_temp_c":   res.BatteryTempC,
		"charge_delta_mah": res.ChargeDeltaMAh,
	}
	if len(res.ODPMRailsMWh) > 0 {
		data["odpm_rails_mwh"] = res.ODPMRailsMWh
	}

	return &types.ToolResult{
		Success: true,
		Data:    data,
	}, nil
}

// LogcatReadHandler implements types.ToolHandler for logcat_read (#14).
type LogcatReadHandler struct {
	DM types.DeviceManager
}

func (h *LogcatReadHandler) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "logcat_read",
		Description: "Reads filtered logcat entries with PID/tag filtering, level threshold, and untrusted-data nonce fencing",
		RiskTier:    types.RiskREAD,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"serial": map[string]any{
					"type":        "string",
					"description": "Target device serial",
				},
				"package_id": map[string]any{
					"type":        "string",
					"description": "Package ID to resolve PID for filtering",
				},
				"pid_only": map[string]any{
					"type":        "boolean",
					"description": "Filter logcat to package PID only",
				},
				"min_level": map[string]any{
					"type":        "string",
					"enum":        []string{"V", "D", "I", "W", "E", "F"},
					"description": "Minimum log priority level",
				},
				"tags": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Logcat tag filters",
				},
				"since_last_call": map[string]any{
					"type":        "boolean",
					"description": "Only return entries since the previous call for this device",
				},
				"max_lines": map[string]any{
					"type":        "integer",
					"description": "Maximum log lines to return (default: 100, capped at 1000)",
				},
			},
			"required": []string{"serial"},
		},
	}
}

func (h *LogcatReadHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	serial, _ := args["serial"].(string)
	pkg, _ := args["package_id"].(string)
	minLevel, _ := args["min_level"].(string)
	pidOnly, _ := args["pid_only"].(bool)
	sinceLastCall, _ := args["since_last_call"].(bool)

	maxLines := 100
	if m, ok := args["max_lines"].(float64); ok && m > 0 {
		maxLines = int(m)
	} else if m, ok := args["max_lines"].(int); ok && m > 0 {
		maxLines = m
	}

	var tags []string
	if rawTags, ok := args["tags"].([]any); ok {
		for _, t := range rawTags {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
	} else if strTags, ok := args["tags"].([]string); ok {
		tags = strTags
	}

	req := LogcatReadRequest{
		Serial:        serial,
		PackageID:     pkg,
		PIDOnly:       pidOnly,
		MinLevel:      minLevel,
		Tags:          tags,
		SinceLastCall: sinceLastCall,
		MaxLines:      maxLines,
	}

	res, err := ReadLogcat(ctx, h.DM, req)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &types.ToolResult{
		Success: true,
		Data: map[string]any{
			"nonce":     res.Nonce,
			"lines":     res.Lines,
			"truncated": res.Truncated,
		},
	}, nil
}

// CrashReportCollectHandler implements types.ToolHandler for crash_report_collect (#15).
type CrashReportCollectHandler struct {
	DM types.DeviceManager
}

func (h *CrashReportCollectHandler) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "crash_report_collect",
		Description: "Collects app crashes, ANRs, tombstones, and exit reasons including MemoryLimiter:AnonSwap",
		RiskTier:    types.RiskREAD,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"serial": map[string]any{
					"type":        "string",
					"description": "Target device serial",
				},
				"package_id": map[string]any{
					"type":        "string",
					"description": "Target package ID",
				},
				"include_tombstone_summary": map[string]any{
					"type":        "boolean",
					"description": "Include tombstones summary if available",
				},
			},
			"required": []string{"serial"},
		},
	}
}

func (h *CrashReportCollectHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	serial, _ := args["serial"].(string)
	pkg, _ := args["package_id"].(string)
	includeTombstone, _ := args["include_tombstone_summary"].(bool)

	req := CrashCollectRequest{
		Serial:                  serial,
		PackageID:               pkg,
		IncludeTombstoneSummary: includeTombstone,
	}

	res, err := CollectCrashReport(ctx, h.DM, req)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	data := map[string]any{
		"nonce":        res.Nonce,
		"crashes":      res.Crashes,
		"anrs":         res.ANRs,
		"exit_reasons": res.ExitReasons,
	}
	if res.BundlePath != "" {
		data["bundle_path"] = res.BundlePath
	}

	return &types.ToolResult{
		Success: true,
		Data:    data,
	}, nil
}

// AppStateInspectHandler implements types.ToolHandler for app_state_inspect (#16).
type AppStateInspectHandler struct {
	DM types.DeviceManager
}

func (h *AppStateInspectHandler) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "app_state_inspect",
		Description: "Inspects running services, granted permissions, WorkManager jobs, and standby bucket via allowlisted dumpsys",
		RiskTier:    types.RiskREAD,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"serial": map[string]any{
					"type":        "string",
					"description": "Target device serial",
				},
				"package_id": map[string]any{
					"type":        "string",
					"description": "Target application package ID",
				},
				"sections": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Sections to inspect (services, permissions, workmanager, standby_bucket)",
				},
			},
			"required": []string{"serial", "package_id"},
		},
	}
}

func (h *AppStateInspectHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	serial, _ := args["serial"].(string)
	pkg, _ := args["package_id"].(string)

	var sections []string
	if rawSections, ok := args["sections"].([]any); ok {
		for _, s := range rawSections {
			if str, ok := s.(string); ok {
				sections = append(sections, str)
			}
		}
	} else if strSections, ok := args["sections"].([]string); ok {
		sections = strSections
	}

	req := AppStateRequest{
		Serial:    serial,
		PackageID: pkg,
		Sections:  sections,
	}

	res, err := InspectAppState(ctx, h.DM, req)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	data := map[string]any{
		"nonce": res.Nonce,
	}
	if len(res.RunningServices) > 0 {
		data["running_services"] = res.RunningServices
	}
	if len(res.GrantedPerms) > 0 {
		data["granted_permissions"] = res.GrantedPerms
	}
	if len(res.DeniedPerms) > 0 {
		data["denied_permissions"] = res.DeniedPerms
	}
	if res.StandbyBucket != "" {
		data["standby_bucket"] = res.StandbyBucket
	}
	if len(res.WorkManagerJobs) > 0 {
		data["workmanager_jobs"] = res.WorkManagerJobs
	}

	return &types.ToolResult{
		Success: true,
		Data:    data,
	}, nil
}

// RegisterProfilerTools returns instances of all tools provided by internal/profiler.
func RegisterProfilerTools(dm types.DeviceManager, runner SidecarRunner) []types.ToolHandler {
	if runner == nil {
		runner = NewPythonSidecarRunner("")
	}
	return []types.ToolHandler{
		&PerfCaptureTraceHandler{DM: dm, Sidecar: runner},
		&PowerThermalSnapshotHandler{DM: dm},
		&LogcatReadHandler{DM: dm},
		&CrashReportCollectHandler{DM: dm},
		&AppStateInspectHandler{DM: dm},
	}
}

// ToolRegistry defines an interface for registering tool handlers with the MCP server.
type ToolRegistry interface {
	Register(h types.ToolHandler)
}

// RegisterProfilerToolsOnServer registers tools #14, #15, #16, #21, #22 directly with an MCP server or registry.
func RegisterProfilerToolsOnServer(registry ToolRegistry, dm types.DeviceManager, runner SidecarRunner) {
	if registry == nil {
		return
	}
	for _, h := range RegisterProfilerTools(dm, runner) {
		registry.Register(h)
	}
}
