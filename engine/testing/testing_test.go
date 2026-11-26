package testing_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"nano_android/engine/compatibility/registry"
	"nano_android/engine/testing/orchestrator"
	"nano_android/engine/testing/suite"
)

func TestOrchestratorRoutesComposeUnsupportedAndExportsJUnitXML(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	orch := orchestrator.New(root)

	composeSuite := suite.Suite{
		Name:     "ComposeGate",
		Artifact: filepath.Join(root, "fixtures", "apks", "compose_v2signed.apk"),
		Mode:     "auto",
		Cases: []suite.TestCase{
			{Name: "LaunchComposeScreen"},
		},
	}

	rep, err := orch.RunSuite(context.Background(), composeSuite)
	if err != nil {
		t.Fatalf("unexpected error routing Compose APK: %v", err)
	}
	if rep.OverallStatus != string(registry.StatusUnsupported) || rep.PrimaryBackend == string(registry.BackendNanoJVM) {
		t.Fatalf("expected non-Nano-JVM backend routing & UNSUPPORTED disclosure, got %+v", rep)
	}

	xmlBytes, err := rep.ExportJUnitXML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(xmlBytes), "<testsuites") || !strings.Contains(string(xmlBytes), "UNSUPPORTED") {
		t.Fatalf("expected valid JUnit XML containing UNSUPPORTED status, got:\n%s", string(xmlBytes))
	}
}

func TestOrchestratorExecutesLiveNanoJVMFixtureAndFlakeRepeat(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	orch := orchestrator.New(root)

	s := suite.Suite{
		Name:        "PrismVSmoke",
		Artifact:    filepath.Join(root, "tools", "prism-v.jar"),
		RepeatCount: 1,
		Cases: []suite.TestCase{
			{
				Name:     "BootAndSwitchToDuo",
				Activity: "com.prismv.ui.MainActivity",
				Steps: []suite.Step{
					{Action: "tap", Target: "Duo"},
					{Action: "step_time", DeltaMs: 300},
					{Action: "assert", Assert: &suite.Assertion{Type: "visible", Target: "DUO COMPARISON DECK"}},
				},
			},
		},
	}

	rep, err := orch.RunSuite(context.Background(), s)
	if err != nil {
		t.Fatalf("RunSuite failed: %v", err)
	}
	if rep.OverallStatus != string(registry.StatusNanoSimulationPass) || rep.Passed != 1 || rep.Failed != 0 {
		t.Fatalf("expected NANO_SIMULATION_PASS 1/1, got: %+v", rep)
	}
}

func TestOrchestratorParallelWorkersAndDeviceProfile(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	orch := orchestrator.New(root)

	s := suite.Suite{
		Name:        "ParallelPrismV",
		Artifact:    filepath.Join(root, "tools", "prism-v.jar"),
		WorkerCount: 2,
		Cases: []suite.TestCase{
			{
				Name:           "SmallScreenDuo",
				Activity:       "com.prismv.ui.MainActivity",
				ProfileID:      "small_screen",
				RetryOnFailure: 1,
				Steps: []suite.Step{
					{Action: "tap", Target: "Duo"},
					{Action: "step_time", DeltaMs: 200},
					{Action: "assert", Assert: &suite.Assertion{Type: "visible", Target: "DUO COMPARISON DECK"}},
				},
			},
			{
				Name:      "ProcessDeathAndMatrix",
				Activity:  "com.prismv.ui.MainActivity",
				ProfileID: "clean_install",
				Steps: []suite.Step{
					{Action: "simulate_process_death"},
					{Action: "tap", Target: "Matrix"},
					{Action: "assert", Assert: &suite.Assertion{Type: "visible", Target: "Diagnostics"}},
				},
			},
		},
	}

	rep, err := orch.RunSuite(context.Background(), s)
	if err != nil {
		t.Fatalf("parallel RunSuite failed: %v", err)
	}
	if rep.OverallStatus != string(registry.StatusNanoSimulationPass) || rep.Passed != 2 || rep.Failed != 0 {
		t.Fatalf("expected NANO_SIMULATION_PASS 2/2, got: %+v", rep)
	}
}
