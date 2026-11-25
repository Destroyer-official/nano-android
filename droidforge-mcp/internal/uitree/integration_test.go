package uitree

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/device"
	"github.com/anthropic/droidforge-mcp/internal/types"
)

// TestIntegration_ActAndObserve_SettingsWiFi runs a full end-to-end cycle against a live AVD or connected device.
// If no device is attached, it skips cleanly.
func TestIntegration_ActAndObserve_SettingsWiFi(t *testing.T) {
	// 1. Probe for live ADB devices
	cmd := exec.Command("adb", "devices")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("adb not found or failed: %v", err)
	}

	lines := strings.Split(string(out), "\n")
	var liveSerial string
	for _, l := range lines[1:] {
		fields := strings.Fields(l)
		if len(fields) >= 2 && fields[1] == "device" {
			liveSerial = fields[0]
			break
		}
	}

	if liveSerial == "" {
		t.Skip("No live Android device or AVD attached; skipping integration test")
	}

	if !strings.HasPrefix(liveSerial, "emulator-") && !strings.HasPrefix(liveSerial, "127.0.0.1:") && os.Getenv("DROIDFORGE_LIVE_TEST") != "true" {
		t.Skipf("Connected device %s is a physical/wireless phone; skipping live integration test to avoid screen disruption", liveSerial)
	}

	t.Logf("Running integration test on live device: %s", liveSerial)

	// Use real ADB device manager
	dm := device.NewDeviceManager()
	svc := NewService(dm, nil, nil, &types.ObservationConfig{
		SettleTimeoutMS: 1500,
	})

	ctx := context.Background()

	// 2. Wake screen and launch Settings app
	_, _ = dm.ExecADB(liveSerial, "shell", "input", "keyevent", "KEYCODE_WAKEUP")
	_, err = dm.ExecADB(liveSerial, "shell", "am", "start", "-a", "android.settings.SETTINGS")
	if err != nil {
		t.Fatalf("failed to launch settings: %v", err)
	}
	time.Sleep(500 * time.Millisecond) // Allow initial activity launch

	// 3. Perform initial observe_screen
	obsTool := &ObserveScreenTool{svc: svc}
	obsRes, err := obsTool.Execute(ctx, map[string]any{"serial": liveSerial})
	if err != nil || !obsRes.Success {
		t.Fatalf("initial observe_screen failed: %v (%s)", err, obsRes.Error)
	}

	obsData := obsRes.Data["observation"].(types.ObservationResult)
	t.Logf("Initial screen activity: %s, UI hash: %s, node count: %d", obsData.Activity, obsData.UIHash, len(obsData.Nodes))

	// 4. Find WiFi / Network & internet node
	var targetNode *types.PrunedNode
	for _, n := range obsData.Nodes {
		textLower := strings.ToLower(n.Text)
		descLower := strings.ToLower(n.ContentDesc)
		resLower := strings.ToLower(n.ResID)
		if strings.Contains(textLower, "network") || strings.Contains(textLower, "wi-fi") ||
			strings.Contains(textLower, "wifi") || strings.Contains(textLower, "connections") ||
			strings.Contains(descLower, "wi-fi") || strings.Contains(resLower, "network") {
			targetCopy := n
			targetNode = &targetCopy
			break
		}
	}

	if targetNode == nil {
		t.Logf("No Wi-Fi/Network node found on main Settings screen; checking first clickable item")
		for _, n := range obsData.Nodes {
			for _, f := range n.Flags {
				if f == "click" {
					targetCopy := n
					targetNode = &targetCopy
					break
				}
			}
			if targetNode != nil {
				break
			}
		}
	}

	if targetNode == nil {
		t.Fatalf("no clickable node found on Settings screen to test act_and_observe")
	}

	t.Logf("Tapping target node: ID=%s, Class=%s, Text=%s, Bounds=%v", targetNode.ID, targetNode.Class, targetNode.Text, targetNode.Bounds)

	// 5. Execute act_and_observe and benchmark round-trip duration
	actTool := &ActAndObserveTool{svc: svc}
	startTime := time.Now()

	actRes, err := actTool.Execute(ctx, map[string]any{
		"serial":    liveSerial,
		"action":    "tap_node",
		"target":    targetNode.ID,
		"settle_ms": 1500,
	})
	elapsed := time.Since(startTime)

	if err != nil || !actRes.Success {
		t.Fatalf("act_and_observe failed: %v (%s)", err, actRes.Error)
	}

	actObs := actRes.Data["observation"].(types.ObservationResult)
	t.Logf("act_and_observe completed in %v (settle_ms: %d). Diff: %s, New hash: %s",
		elapsed, actObs.SettleMS, actObs.UIDiffSummary, actObs.UIHash)

	// Verify diff shows changes
	if actObs.UIDiffSummary == "" {
		t.Errorf("expected non-empty UIDiffSummary in observation result")
	}

	// Done Criteria: act_and_observe completes tap->settle->diff->logcat in <600ms on emulator / fast hardware
	t.Logf("Full round-trip elapsed: %v (target: <600ms on emulator)", elapsed)
}
