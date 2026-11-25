package mcp_compiler

import (
	"context"
	"strings"
	"testing"
)

func TestIntegratedCoordinator_ScreenNavigationAndRaycasting(t *testing.T) {
	coord := NewIntegratedCoordinator()
	defer func() {
		if c, ok := interface{}(coord).(interface{ Close() error }); ok {
			_ = c.Close()
		}
	}()
	ctx := context.Background()

	// 1. Initial screen should be Studio
	obs, err := coord.ObserveScreen(ctx, false, "", 0)
	if err != nil {
		t.Fatalf("ObserveScreen failed: %v", err)
	}
	if obs.CurrentScreen != "Studio" {
		t.Fatalf("Expected initial screen Studio, got %s", obs.CurrentScreen)
	}

	// 2. Dispatch tap using text "Duo"
	res, err := coord.DispatchAction(ctx, Action{
		Type:   "tap",
		NodeID: "Duo",
	})
	if err != nil {
		t.Fatalf("DispatchAction Duo failed: %v", err)
	}
	if !strings.Contains(res.Message, "Duo") {
		t.Fatalf("Expected navigation message to Duo, got: %s", res.Message)
	}

	obsDuo, err := coord.ObserveScreen(ctx, false, "", 0)
	if err != nil {
		t.Fatalf("ObserveScreen Duo failed: %v", err)
	}
	if obsDuo.CurrentScreen != "Duo" {
		t.Fatalf("Expected CurrentScreen Duo, got %s", obsDuo.CurrentScreen)
	}

	// 3. Dispatch tap using NodeID from live AST
	// Find node for "Matrix" or "Settings"
	var targetNodeID string
	var targetText string
	if targetNode := obsDuo.AST.FindByText("Matrix"); targetNode != nil {
		targetNodeID = targetNode.ID
		targetText = "Matrix"
	} else if targetNode := obsDuo.AST.FindByText("Tools"); targetNode != nil {
		targetNodeID = targetNode.ID
		targetText = "Tools"
	}

	if targetNodeID != "" {
		resTarget, err := coord.DispatchAction(ctx, Action{
			Type:   "tap",
			NodeID: targetNodeID,
		})
		if err != nil {
			t.Fatalf("DispatchAction with NodeID %s failed: %v", targetNodeID, err)
		}
		if !strings.Contains(resTarget.Message, targetText) {
			t.Fatalf("Expected navigation to %s via node ID, got: %s", targetText, resTarget.Message)
		}

		obsTarget, _ := coord.ObserveScreen(ctx, false, "", 0)
		if obsTarget.CurrentScreen != targetText {
			t.Fatalf("Expected CurrentScreen %s, got %s", targetText, obsTarget.CurrentScreen)
		}
	}

	// 4. Test Coordinate Raycasting (x=756, y=100) -> Matrix
	resRaycast, err := coord.DispatchAction(ctx, Action{
		Type: "tap",
		X:    756,
		Y:    100,
	})
	if err != nil {
		t.Fatalf("Raycasting tap failed: %v", err)
	}
	t.Logf("Raycasting result: %s (target: %s, text: %s)", resRaycast.Message, resRaycast.TargetNodeID, resRaycast.ResolvedText)

	// 5. Test CheckHealth includes ADB bridge
	health, err := coord.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("CheckHealth failed: %v", err)
	}
	if !health.OverallHealthy {
		t.Fatalf("Expected healthy coordinator")
	}
	t.Logf("Subsystem info: %+v", health.SubsystemInfo)
}
