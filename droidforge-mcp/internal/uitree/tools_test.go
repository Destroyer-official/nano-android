package uitree

import (
	"context"
	"strings"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// mockDeviceManager satisfies types.DeviceManager for unit testing.
type mockDeviceManager struct {
	lastArgs   []string
	adbReturns map[string]string
}

func newMockDM() *mockDeviceManager {
	return &mockDeviceManager{
		adbReturns: make(map[string]string),
	}
}

func (m *mockDeviceManager) ListDevices(includeAVDs bool) ([]types.DeviceInfo, error) {
	return []types.DeviceInfo{{Serial: "emulator-5554", State: "device"}}, nil
}
func (m *mockDeviceManager) BootSnapshot(avdName, snapshot string, headless bool, ramMB int) (*types.DeviceInfo, error) {
	return nil, nil
}
func (m *mockDeviceManager) ResetState(serial, strategy, pkgID, snapshotName string) error {
	return nil
}
func (m *mockDeviceManager) InstallAndLaunch(serial, apkPath, activity, deepLink string, grantPerms bool) (*types.AppLaunchResult, error) {
	return nil, nil
}
func (m *mockDeviceManager) ExecADB(serial string, args ...string) (string, error) {
	m.lastArgs = args
	cmdKey := strings.Join(args, " ")
	for k, v := range m.adbReturns {
		if strings.Contains(cmdKey, k) {
			return v, nil
		}
	}
	return "", nil
}
func (m *mockDeviceManager) ShutdownEmulator(serial string) error {
	return nil
}
func (m *mockDeviceManager) EmulatorConsole(serial, command string) (string, error) {
	return "", nil
}

func TestObserveScreen_FencesAllDeviceText(t *testing.T) {
	dm := newMockDM()
	svc := NewService(dm, nil, nil, nil)
	serial := "emulator-5554"
	svc.SetMockDumpXML(serial, []byte(sample3ScamShieldAlert))

	tools := svc.GetTools()
	var observeTool types.ToolHandler
	for _, tool := range tools {
		if tool.Definition().Name == "observe_screen" {
			observeTool = tool
			break
		}
	}

	if observeTool == nil {
		t.Fatalf("observe_screen tool not found")
	}

	res, err := observeTool.Execute(context.Background(), map[string]any{
		"serial": serial,
	})
	if err != nil {
		t.Fatalf("observe_screen failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("observe_screen reported failure: %s", res.Error)
	}

	obs, ok := res.Data["observation"].(types.ObservationResult)
	if !ok {
		t.Fatalf("missing observation in response: %+v", res.Data)
	}

	if !strings.HasPrefix(obs.Nonce, "DF_UNTRUSTED_") {
		t.Errorf("expected nonce in observation, got %q", obs.Nonce)
	}

	// Verify all returned nodes with text are fenced
	for _, n := range obs.Nodes {
		if n.Text != "" {
			if !strings.HasPrefix(n.Text, "<"+obs.Nonce+">") || !strings.HasSuffix(n.Text, "</"+obs.Nonce+">") {
				t.Errorf("node %s text not fenced with nonce %s: %q", n.ID, obs.Nonce, n.Text)
			}
		}
	}
}

func TestActAndObserve_CircuitBreakerFiresAt3IdenticalHashes(t *testing.T) {
	dm := newMockDM()
	svc := NewService(dm, nil, nil, nil)
	serial := "emulator-5554"

	// Same static XML across all 3 steps
	svc.SetMockDumpXML(serial, []byte(sample1SettingsMain))

	var actTool types.ToolHandler
	for _, tool := range svc.GetTools() {
		if tool.Definition().Name == "act_and_observe" {
			actTool = tool
			break
		}
	}

	if actTool == nil {
		t.Fatalf("act_and_observe tool not found")
	}

	ctx := context.Background()

	// Step 1: Initial action
	res1, err := actTool.Execute(ctx, map[string]any{
		"serial":    serial,
		"action":    "key",
		"target":    "home",
		"settle_ms": 100,
	})
	if err != nil || !res1.Success {
		t.Fatalf("step 1 failed: %v (%s)", err, res1.Error)
	}
	obs1 := res1.Data["observation"].(types.ObservationResult)
	if obs1.StuckWarning {
		t.Errorf("step 1: stuck_warning should be false, got true")
	}

	// Step 2: Second action on identical screen
	res2, err := actTool.Execute(ctx, map[string]any{
		"serial":    serial,
		"action":    "key",
		"target":    "home",
		"settle_ms": 100,
	})
	if err != nil || !res2.Success {
		t.Fatalf("step 2 failed: %v (%s)", err, res2.Error)
	}
	obs2 := res2.Data["observation"].(types.ObservationResult)
	if obs2.StuckWarning {
		t.Errorf("step 2: stuck_warning should be false, got true")
	}

	// Step 3: Third action with identical screen -> Circuit breaker fires stuck_warning: true!
	res3, err := actTool.Execute(ctx, map[string]any{
		"serial":    serial,
		"action":    "key",
		"target":    "home",
		"settle_ms": 100,
	})
	if err != nil || !res3.Success {
		t.Fatalf("step 3 failed: %v (%s)", err, res3.Error)
	}
	obs3 := res3.Data["observation"].(types.ObservationResult)

	// Done Criteria: Circuit breaker fires stuck_warning: true after 3 identical hashes
	if !obs3.StuckWarning {
		t.Errorf("step 3: expected stuck_warning = true after 3 identical hashes, got false")
	}
	if obs3.ActionApplied != "key:home" {
		t.Errorf("unexpected action_applied: %q", obs3.ActionApplied)
	}
}

func TestDeviceClipboardAndText_SetPasteGet(t *testing.T) {
	dm := newMockDM()
	dm.adbReturns["cmd clipboard get"] = "Copied text content"

	svc := NewService(dm, nil, nil, nil)
	serial := "emulator-5554"

	var clipTool types.ToolHandler
	for _, tool := range svc.GetTools() {
		if tool.Definition().Name == "device_clipboard_and_text" {
			clipTool = tool
			break
		}
	}

	// Set
	resSet, err := clipTool.Execute(context.Background(), map[string]any{
		"serial": serial,
		"op":     "set",
		"text":   "New clipboard value",
	})
	if err != nil || !resSet.Success {
		t.Fatalf("set clipboard failed: %v (%s)", err, resSet.Error)
	}

	// Paste
	resPaste, err := clipTool.Execute(context.Background(), map[string]any{
		"serial": serial,
		"op":     "paste",
	})
	if err != nil || !resPaste.Success {
		t.Fatalf("paste clipboard failed: %v (%s)", err, resPaste.Error)
	}

	// Get
	resGet, err := clipTool.Execute(context.Background(), map[string]any{
		"serial": serial,
		"op":     "get",
	})
	if err != nil || !resGet.Success {
		t.Fatalf("get clipboard failed: %v (%s)", err, resGet.Error)
	}
	textGot := resGet.Data["text"].(string)
	if !strings.Contains(textGot, "Copied text content") {
		t.Errorf("expected copied text in get response, got %q", textGot)
	}
}

func TestDevicePressSystem_Keys(t *testing.T) {
	dm := newMockDM()
	svc := NewService(dm, nil, nil, nil)
	serial := "emulator-5554"

	var pressTool types.ToolHandler
	for _, tool := range svc.GetTools() {
		if tool.Definition().Name == "device_press_system" {
			pressTool = tool
			break
		}
	}

	keys := []string{"back", "home", "recents", "notifications", "volume_up", "volume_down", "enter"}
	for _, k := range keys {
		res, err := pressTool.Execute(context.Background(), map[string]any{
			"serial": serial,
			"key":    k,
		})
		if err != nil || !res.Success {
			t.Errorf("press system key %q failed: %v (%s)", k, err, res.Error)
		}
	}
}

func TestDeviceWaitIdle_Conditions(t *testing.T) {
	dm := newMockDM()
	svc := NewService(dm, nil, nil, nil)
	serial := "emulator-5554"
	svc.SetMockDumpXML(serial, []byte(sample1SettingsMain))

	var waitTool types.ToolHandler
	for _, tool := range svc.GetTools() {
		if tool.Definition().Name == "device_wait_idle" {
			waitTool = tool
			break
		}
	}

	// 1. ui_stable condition
	resStable, err := waitTool.Execute(context.Background(), map[string]any{
		"serial":     serial,
		"condition":  "ui_stable",
		"timeout_ms": 500,
	})
	if err != nil || !resStable.Success {
		t.Fatalf("wait ui_stable failed: %v (%s)", err, resStable.Error)
	}
	if matched, ok := resStable.Data["matched"].(bool); !ok || !matched {
		t.Errorf("expected ui_stable to match, got: %+v", resStable.Data)
	}

	// 2. node_visible condition (Settings contains "Network & internet")
	resVisible, err := waitTool.Execute(context.Background(), map[string]any{
		"serial":     serial,
		"condition":  "node_visible",
		"selector":   "Network & internet",
		"timeout_ms": 300,
	})
	if err != nil || !resVisible.Success {
		t.Fatalf("wait node_visible failed: %v", err)
	}
	if matched, ok := resVisible.Data["matched"].(bool); !ok || !matched {
		t.Errorf("expected node_visible to match, got: %+v", resVisible.Data)
	}

	// 3. node_gone condition (non-existent node is already gone)
	resGone, err := waitTool.Execute(context.Background(), map[string]any{
		"serial":     serial,
		"condition":  "node_gone",
		"selector":   "NonExistentButtonXYZ",
		"timeout_ms": 300,
	})
	if err != nil || !resGone.Success {
		t.Fatalf("wait node_gone failed: %v", err)
	}
	if matched, ok := resGone.Data["matched"].(bool); !ok || !matched {
		t.Errorf("expected node_gone to match, got: %+v", resGone.Data)
	}
}
