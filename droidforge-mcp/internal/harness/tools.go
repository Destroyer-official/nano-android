package harness

import (
	"context"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// SimInjectSMSOrCallHandler implements types.ToolHandler for sim_inject_sms_or_call (#17).
type SimInjectSMSOrCallHandler struct {
	DM types.DeviceManager
}

func (h *SimInjectSMSOrCallHandler) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "sim_inject_sms_or_call",
		Description: "Injects an SMS or GSM call into an Android emulator with Android 17 OTP pattern detection",
		RiskTier:    types.RiskWRITESCOPED,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"serial": map[string]any{
					"type":        "string",
					"description": "Target emulator serial (e.g. emulator-5554)",
				},
				"kind": map[string]any{
					"type":        "string",
					"enum":        []string{"sms", "call"},
					"description": "Injection kind ('sms' or 'call')",
				},
				"sender": map[string]any{
					"type":        "string",
					"description": "Sender phone number or caller number",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "SMS text body, or call action ('call', 'accept', 'cancel', 'hold')",
				},
			},
			"required": []string{"serial", "kind", "sender"},
		},
	}
}

func (h *SimInjectSMSOrCallHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	serial, _ := args["serial"].(string)
	kind, _ := args["kind"].(string)
	sender, _ := args["sender"].(string)
	body, _ := args["body"].(string)

	res, err := InjectSMSOrCall(ctx, h.DM, serial, kind, sender, body)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &types.ToolResult{
		Success: true,
		Data: map[string]any{
			"injected":                       res.Injected,
			"timestamp_ms":                   res.TimestampMS,
			"android17_otp_pattern_detected": res.Android17OTPDetected,
		},
	}, nil
}

// SimPostNotificationHandler implements types.ToolHandler for sim_post_notification (#18).
type SimPostNotificationHandler struct {
	DM types.DeviceManager
}

func (h *SimPostNotificationHandler) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "sim_post_notification",
		Description: "Posts a system notification on an Android device via cmd notification post",
		RiskTier:    types.RiskWRITESCOPED,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"serial": map[string]any{
					"type":        "string",
					"description": "Target device serial",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "Notification title",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Notification text content",
				},
				"channel": map[string]any{
					"type":        "string",
					"description": "Notification channel ID (created if not present and package_id is set)",
				},
				"tag": map[string]any{
					"type":        "string",
					"description": "Notification tag (default: 'tag')",
				},
				"package_id": map[string]any{
					"type":        "string",
					"description": "Target package ID",
				},
			},
			"required": []string{"serial", "title", "body"},
		},
	}
}

func (h *SimPostNotificationHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	serial, _ := args["serial"].(string)
	title, _ := args["title"].(string)
	body, _ := args["body"].(string)
	channel, _ := args["channel"].(string)
	tag, _ := args["tag"].(string)
	pkg, _ := args["package_id"].(string)

	req := NotificationRequest{
		Serial:    serial,
		Title:     title,
		Body:      body,
		Channel:   channel,
		Tag:       tag,
		PackageID: pkg,
	}

	res, err := PostNotification(ctx, h.DM, req)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &types.ToolResult{
		Success: true,
		Data: map[string]any{
			"posted": res.Posted,
			"method": res.Method,
		},
	}, nil
}

// SimSetPermissionsHandler implements types.ToolHandler for sim_set_permissions (#19).
type SimSetPermissionsHandler struct {
	DM types.DeviceManager
}

func (h *SimSetPermissionsHandler) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "sim_set_permissions",
		Description: "Modifies app permissions, appops, standby bucket, and deviceidle doze state",
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
					"description": "Target application package ID",
				},
				"grants": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Permissions to grant (pm grant)",
				},
				"revokes": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Permissions to revoke (pm revoke)",
				},
				"appops": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "object"},
					"description": "AppOps to configure ({op: string, mode: string})",
				},
				"standby_bucket": map[string]any{
					"type":        "string",
					"enum":        []string{"active", "working_set", "frequent", "rare", "restricted", "never"},
					"description": "Set app standby bucket via am set-standby-bucket",
				},
				"doze_mode": map[string]any{
					"type":        "string",
					"enum":        []string{"force-idle", "unforce"},
					"description": "Simulate device doze state via dumpsys deviceidle",
				},
			},
			"required": []string{"serial"},
		},
	}
}

func (h *SimSetPermissionsHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	serial, _ := args["serial"].(string)
	pkg, _ := args["package_id"].(string)
	standbyBucket, _ := args["standby_bucket"].(string)
	dozeMode, _ := args["doze_mode"].(string)

	var grants []string
	if rawGrants, ok := args["grants"].([]any); ok {
		for _, g := range rawGrants {
			if s, ok := g.(string); ok {
				grants = append(grants, s)
			}
		}
	} else if stringGrants, ok := args["grants"].([]string); ok {
		grants = stringGrants
	}

	var revokes []string
	if rawRevokes, ok := args["revokes"].([]any); ok {
		for _, r := range rawRevokes {
			if s, ok := r.(string); ok {
				revokes = append(revokes, s)
			}
		}
	} else if stringRevokes, ok := args["revokes"].([]string); ok {
		revokes = stringRevokes
	}

	var appops []AppOp
	if rawOps, ok := args["appops"].([]any); ok {
		for _, raw := range rawOps {
			if m, ok := raw.(map[string]any); ok {
				op, _ := m["op"].(string)
				mode, _ := m["mode"].(string)
				if op != "" && mode != "" {
					appops = append(appops, AppOp{Op: op, Mode: mode})
				}
			}
		}
	}

	req := PermissionsRequest{
		Serial:        serial,
		PackageID:     pkg,
		Grants:        grants,
		Revokes:       revokes,
		AppOps:        appops,
		StandbyBucket: standbyBucket,
		DozeMode:      dozeMode,
	}

	res, err := SetPermissions(ctx, h.DM, req)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &types.ToolResult{
		Success: len(res.Errors) == 0,
		Data: map[string]any{
			"applied": res.Applied,
			"errors":  res.Errors,
		},
	}, nil
}

// MediaPushAndVerifyHandler implements types.ToolHandler for media_push_and_verify (#20).
type MediaPushAndVerifyHandler struct {
	DM      types.DeviceManager
	Sidecar SidecarRunner
}

func (h *MediaPushAndVerifyHandler) Definition() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "media_push_and_verify",
		Description: "Pushes large media fixtures with sha256 caching and verifies on-device output bit-exact or via VMAF/SSIM",
		RiskTier:    types.RiskWRITESCOPED,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"serial": map[string]any{
					"type":        "string",
					"description": "Target device serial",
				},
				"fixture_id": map[string]any{
					"type":        "string",
					"description": "Fixture ID defined in fixtures.lock.json",
				},
				"local_fixture_path": map[string]any{
					"type":        "string",
					"description": "Direct local path to fixture file (if fixture_id not used)",
				},
				"device_dest_path": map[string]any{
					"type":        "string",
					"description": "On-device target path (default: /sdcard/Download/<basename>)",
				},
				"output_device_path": map[string]any{
					"type":        "string",
					"description": "On-device generated output path to verify",
				},
				"oracle": map[string]any{
					"type":        "string",
					"enum":        []string{"sha256", "quality_bounded", "vmaf_ssim"},
					"description": "Verification oracle ('sha256' for bit-exact, 'quality_bounded' for VMAF/SSIM)",
				},
				"golden_sha256": map[string]any{
					"type":        "string",
					"description": "Expected SHA256 hex string for bit-exact check",
				},
				"min_vmaf": map[string]any{
					"type":        "number",
					"description": "Minimum acceptable VMAF score (default: 90.0)",
				},
				"min_ssim": map[string]any{
					"type":        "number",
					"description": "Minimum acceptable SSIM score (default: 0.95)",
				},
			},
			"required": []string{"serial"},
		},
	}
}

func (h *MediaPushAndVerifyHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	serial, _ := args["serial"].(string)
	fixtureID, _ := args["fixture_id"].(string)
	localPath, _ := args["local_fixture_path"].(string)
	deviceDest, _ := args["device_dest_path"].(string)
	outputDevice, _ := args["output_device_path"].(string)
	oracle, _ := args["oracle"].(string)
	goldenSHA, _ := args["golden_sha256"].(string)

	minVMAF := 0.0
	if v, ok := args["min_vmaf"].(float64); ok {
		minVMAF = v
	}
	minSSIM := 0.0
	if v, ok := args["min_ssim"].(float64); ok {
		minSSIM = v
	}

	req := MediaPushVerifyRequest{
		Serial:           serial,
		FixtureID:        fixtureID,
		LocalFixturePath: localPath,
		DeviceDestPath:   deviceDest,
		OutputDevicePath: outputDevice,
		Oracle:           oracle,
		GoldenSHA256:     goldenSHA,
		MinVMAF:          minVMAF,
		MinSSIM:          minSSIM,
	}

	sidecar := h.Sidecar
	if sidecar == nil {
		sidecar = NewPythonSidecarRunner("")
	}

	res, err := PushAndVerifyMedia(ctx, h.DM, sidecar, req)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	data := map[string]any{
		"pushed_cached": res.PushedCached,
		"hash_match":    res.HashMatch,
		"decode_errors": res.DecodeErrors,
	}
	if res.OutputSHA256 != "" {
		data["output_sha256"] = res.OutputSHA256
	}
	if res.FFProbeMeta != nil {
		data["ffprobe_meta"] = res.FFProbeMeta
	}
	if res.VMAF > 0 {
		data["vmaf"] = res.VMAF
	}
	if res.SSIM > 0 {
		data["ssim"] = res.SSIM
	}

	return &types.ToolResult{
		Success: true,
		Data:    data,
	}, nil
}

// RegisterHarnessTools returns instances of all tools provided by internal/harness.
func RegisterHarnessTools(dm types.DeviceManager, sidecar SidecarRunner) []types.ToolHandler {
	if sidecar == nil {
		sidecar = NewPythonSidecarRunner("")
	}
	return []types.ToolHandler{
		&SimInjectSMSOrCallHandler{DM: dm},
		&SimPostNotificationHandler{DM: dm},
		&SimSetPermissionsHandler{DM: dm},
		&MediaPushAndVerifyHandler{DM: dm, Sidecar: sidecar},
	}
}

// ToolRegistry defines an interface for registering tool handlers with the MCP server.
type ToolRegistry interface {
	Register(h types.ToolHandler)
}

// RegisterHarnessToolsOnServer registers tools #17, #18, #19, #20 directly with an MCP server or registry.
func RegisterHarnessToolsOnServer(registry ToolRegistry, dm types.DeviceManager, sidecar SidecarRunner) {
	if registry == nil {
		return
	}
	for _, h := range RegisterHarnessTools(dm, sidecar) {
		registry.Register(h)
	}
}
