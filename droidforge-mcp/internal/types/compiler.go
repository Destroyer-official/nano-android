package types

// FreezeResult holds the outcome of compiling an exploration session into a Maestro YAML flow.
type FreezeResult struct {
	YAMLPath          string   `json:"yaml_path"`
	StepCount         int      `json:"step_count"`
	WeakSelectorWarns []string `json:"weak_selector_warnings,omitempty"`
	HooksJSONPath     string   `json:"hooks_json_path,omitempty"` // Companion file for external stimuli
}

// AppCardResult holds the outcome of exporting an AppCard.
type AppCardResult struct {
	AppCardPath    string   `json:"appcard_path"`
	ScreensMapped  int      `json:"screens_mapped"`
	DeepLinksFound []string `json:"deep_links_found,omitempty"`
}

// MaestroRunResult holds the outcome of running Maestro YAML flows.
type MaestroRunResult struct {
	RunsPassed         int      `json:"runs_passed"`
	RunsFailed         int      `json:"runs_failed"`
	AvgDurationMS      int64    `json:"avg_duration_ms"`
	FailureBundlePaths []string `json:"failure_bundle_paths,omitempty"`
}

// TestCompiler records exploration sessions and freezes them into deterministic tests.
type TestCompiler interface {
	// RecordStep appends an action step to the active session buffer.
	RecordStep(sessionID string, step SessionStep)
	// FreezeToMaestro compiles the net successful path into a Maestro YAML flow.
	FreezeToMaestro(sessionID, flowName, outputPath string) (*FreezeResult, error)
	// RunMaestro executes a Maestro YAML flow with optional burn-in.
	RunMaestro(serial, flowPath string, repeatCount int) (*MaestroRunResult, error)
	// ExportAppCard generates an AppCard markdown file from session data.
	ExportAppCard(sessionID, outputPath string) (*AppCardResult, error)
}
