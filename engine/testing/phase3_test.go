package testing_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"nano_android/engine/compatibility/registry"
	"nano_android/engine/testing/differential"
	"nano_android/engine/testing/labs"
	"nano_android/engine/testing/suite"
)

func TestDifferentialEngine11DimensionsAndFirstDivergence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. Live equivalent Nano JVM self-differential run against prism-v.jar
	dbPath := filepath.Join(t.TempDir(), "compatibility.db")
	eqReq := differential.DifferentialRequest{
		Name: "PrismVSelfEquivalence",
		Mode: differential.ModeDeviceAVsDeviceB,
		TargetA: differential.TargetSpec{Label: "A", Backend: registry.BackendNanoJVM, Artifact: "../../tools/prism-v.jar", Activity: "com.prismv.ui.MainActivity", ProfileID: "clean_install", APILevel: 35},
		TargetB: differential.TargetSpec{Label: "B", Backend: registry.BackendNanoJVM, Artifact: "../../tools/prism-v.jar", Activity: "com.prismv.ui.MainActivity", ProfileID: "clean_install", APILevel: 35},
		Steps: []suite.Step{
			{Action: "tap", Target: "Tools"},
			{Action: "step_time", DeltaMs: 300},
		},
		CompatDBPath: dbPath,
	}
	eqRep, err := differential.Run(ctx, eqReq, nil, nil)
	if err != nil {
		t.Fatalf("differential.Run failed: %v", err)
	}
	if !eqRep.Equivalent || eqRep.Status != registry.StatusNanoSimulationPass || eqRep.EquivalenceScore != 1.0 {
		t.Fatalf("expected identical self-run to be equivalent (score=1.0), got %+v", eqRep)
	}

	// 2. Live cross-profile (clean_install vs small_screen) triggers earliest UI hierarchy/bounds divergence
	divReq := differential.DifferentialRequest{
		Name: "PrismVViewportDivergence",
		Mode: differential.ModeDeviceAVsDeviceB,
		TargetA: differential.TargetSpec{Label: "phone", Backend: registry.BackendNanoJVM, Artifact: "../../tools/prism-v.jar", Activity: "com.prismv.ui.MainActivity", ProfileID: "clean_install", APILevel: 35},
		TargetB: differential.TargetSpec{Label: "small", Backend: registry.BackendNanoJVM, Artifact: "../../tools/prism-v.jar", Activity: "com.prismv.ui.MainActivity", ProfileID: "small_screen", APILevel: 35},
		AutoReproOnDiff: true,
		CompatDBPath:    dbPath,
	}
	divRep, err := differential.Run(ctx, divReq, nil, nil)
	if err != nil {
		t.Fatalf("differential.Run cross-profile failed: %v", err)
	}
	if divRep.Equivalent || divRep.FirstDivergence == nil {
		t.Fatalf("expected small_screen vs clean_install bounds divergence, got %+v", divRep)
	}
	if divRep.FirstDivergence.Dimension != differential.DimUIHierarchy || !divRep.FirstDivergence.ReproConfirmed {
		t.Fatalf("expected confirmed ui_hierarchy divergence, got %+v", divRep.FirstDivergence)
	}

	// 3. Honest UNSUPPORTED disclosure if ART backend is requested without device attached
	artReq := eqReq
	artReq.Mode = differential.ModeNanoVsART
	artReq.TargetB.Backend = registry.BackendAndroidART
	artRep, err := differential.Run(ctx, artReq, nil, nil)
	if err != nil {
		t.Fatalf("differential.Run ART check returned unexpected error: %v", err)
	}
	if artRep.Status != registry.StatusUnsupported {
		t.Fatalf("expected honest UNSUPPORTED status when ART isn't attached, got %s", artRep.Status)
	}
}

func TestTwelveSpecializedLabsAndScenarios(t *testing.T) {
	if got := labs.ListLabs(); len(got) != 12 {
		t.Fatalf("expected 12 labs, got %d", len(got))
	}
	if got := labs.StressScenarios23(); len(got) != 23 {
		t.Fatalf("expected 23 stress scenarios, got %d", len(got))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	wsRoot, _ := filepath.Abs(filepath.Join("..", ".."))

	perfRep, err := labs.RunLab(ctx, wsRoot, "performance", filepath.Join(wsRoot, "tools", "prism-v.jar"), map[string]string{"activity": "com.prismv.ui.MainActivity"})
	if err != nil || perfRep.Status != registry.StatusNanoSimulationPass {
		t.Fatalf("performance lab failed: err=%v rep=%+v", err, perfRep)
	}

	secRep, err := labs.RunLab(ctx, wsRoot, "security", filepath.Join(wsRoot, "fixtures", "apks", "minimal_valid.apk"), nil)
	if err != nil || secRep == nil || secRep.LabID != "security" {
		t.Fatalf("security lab failed: err=%v rep=%+v", err, secRep)
	}
}
