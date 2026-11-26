package mcp_compiler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPhase2SevenGroupMCPToolsAndGovernance(t *testing.T) {
	reg := NewToolRegistry(NewMockCoordinator(), NewMaestroCompiler("com.prism.v"))

	expected := []string{
		"nano_project_discover", "nano_apk_inspect", "nano_apk_manifest", "nano_apk_dex_inspect", "nano_apk_resource_inspect",
		"nano_apk_native_inspect", "nano_apk_security_audit", "nano_apk_compatibility", "nano_apk_compare",
		"nano_runtime_launch", "nano_runtime_advance_time", "nano_runtime_capabilities",
		"nano_ui_observe", "nano_ui_tap", "nano_ui_long_press", "nano_ui_type", "nano_ui_swipe",
		"nano_ui_back", "nano_ui_assert", "nano_ui_accessibility_audit", "nano_ui_screenshot", "nano_ui_compare",
		"nano_test_generate_regression", "nano_test_get_flow", "nano_test_clear",
		"nano_device_list", "nano_device_status",
		"nano_profile_state", "nano_policy_inspect", "nano_security_verify", "nano_audit_history",
	}
	for _, name := range expected {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("expected registered Phase 2 tool %q", name)
		}
	}

	// Verify capabilities, accessibility audit, screenshot, UI compare, device list, audit history
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"nano_runtime_capabilities", nil},
		{"nano_ui_accessibility_audit", map[string]any{"profile_id": "clean_install"}},
		{"nano_ui_screenshot", map[string]any{"format": "webp"}},
		{"nano_ui_compare", map[string]any{"baseline_texts": []any{"Studio", "MissingWidget"}}},
		{"nano_device_list", nil},
		{"nano_device_status", nil},
		{"nano_audit_history", map[string]any{"limit": float64(5)}},
	} {
		res, err := reg.Call(context.Background(), tc.name, tc.args)
		if err != nil || res == nil || res.IsError {
			t.Fatalf("tool %s failed: %+v (%v)", tc.name, res, err)
		}
	}

	capRes, _ := reg.Call(context.Background(), "nano_runtime_capabilities", nil)
	if !strings.Contains(capRes.Content[0].Text, "large_font") {
		t.Fatalf("expected large_font profile in capabilities: %s", capRes.Content[0].Text)
	}

	// Verify audit ledger integrity via nano_security_verify
	secRes, err := reg.Call(context.Background(), "nano_security_verify", nil)
	if err != nil || secRes.IsError {
		t.Fatalf("nano_security_verify failed: %+v (%v)", secRes, err)
	}
	var secPayload map[string]any
	if err := json.Unmarshal([]byte(secRes.Content[0].Text), &secPayload); err != nil {
		t.Fatal(err)
	}
	if valid, _ := secPayload["audit_chain_valid"].(bool); !valid {
		t.Fatalf("expected audit_chain_valid=true, got %+v", secPayload)
	}
}
