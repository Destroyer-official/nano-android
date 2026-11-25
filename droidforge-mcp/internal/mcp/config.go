package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropic/droidforge-mcp/internal/types"
	"gopkg.in/yaml.v3"
)

// DefaultConfig returns a DroidForgeConfig populated with production defaults.
func DefaultConfig() *types.DroidForgeConfig {
	return &types.DroidForgeConfig{
		SchemaVersion: 1,
		Project: types.ProjectConfig{
			Name:                "android-app",
			Mode:                "source",
			GradleRoot:          ".",
			DefaultVariant:      "debug",
			AllowlistedPackages: []string{},
		},
		Execution: types.ExecutionConfig{
			PreferTier0JVM:       true,
			DefaultAVD:           "api37_clean",
			GoldenSnapshot:       "clean_base",
			AVDRamMB:             3072,
			AVDIdleSuspendMin:    10,
			GradleMaxHeapMB:      3072,
			MaxStepsPerTask:      15,
			MaxIdenticalUIHashes: 3,
			SequentialAVDMode:    true,
		},
		Observation: types.ObservationConfig{
			DefaultMode: "pruned_tree_diff",
			AttachScreenshotOn: []string{
				"empty_or_sparse_tree",
				"crash_or_anr",
				"explicit_request",
			},
			ScreenshotMaxHeight: 720,
			ScreenshotFormat:    "webp",
			ScreenshotQuality:   75,
			SettleTimeoutMS:     1500,
		},
		Workloads: types.WorkloadConfig{
			Profile:        "standard",
			MockServerPort: 8080,
		},
	}
}

// DefaultPolicy returns a PolicyConfig matching Section 8.2 security standards.
func DefaultPolicy() *types.PolicyConfig {
	return &types.PolicyConfig{
		SchemaVersion: 1,
		Network: types.NetworkPolicy{
			BindHost:                  "127.0.0.1",
			AllowUnencryptedADBTcpip:  false,
			AllowTLSWirelessDebugging: true,
		},
		Execution: types.ExecutionPolicy{
			AllowRawShell:              false,
			EnforcePackageScopeLock:    true,
			AllowContactsSMSOnPhysical: false,
			BlockedBinaries: []string{
				"fastboot",
				"sh",
				"bash",
				"cmd.exe",
				"powershell.exe",
			},
			AllowedSettingsKeys: []string{
				"global:window_animation_scale",
				"global:transition_animation_scale",
				"global:animator_duration_scale",
				"system:screen_brightness",
				"system:screen_brightness_mode",
			},
		},
		Sanitization: types.SanitizationPolicy{
			WrapWithRandomNonce:   true,
			StripZeroWidthAndBidi: true,
			MaxNodeTextBytes:      256,
			MaxLogcatLineBytes:    512,
		},
		HumanApproval: []string{
			"register_new_physical_device",
			"modify_non_allowlisted_package",
			"run_cloud_farm_beyond_free_quota",
			"sign_release_artifact",
		},
		Audit: types.AuditPolicy{
			LogDir:        "~/.droidforge/audit",
			RetentionDays: 90,
		},
	}
}

// LoadConfig parses a droidforge.yaml file from disk and validates required fields.
func LoadConfig(path string) (*types.DroidForgeConfig, error) {
	if path == "" {
		path = "droidforge.yaml"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read droidforge config %q: %w", path, err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse droidforge config %q: %w", path, err)
	}

	if errs := ValidateConfig(cfg); len(errs) > 0 {
		return nil, fmt.Errorf("invalid droidforge config %q: %s", path, strings.Join(errs, "; "))
	}

	return cfg, nil
}

// LoadPolicy parses a droidforge.policy.json file and validates security rules.
func LoadPolicy(path string) (*types.PolicyConfig, error) {
	if path == "" {
		path = "droidforge.policy.json"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read droidforge policy %q: %w", path, err)
	}

	pol := DefaultPolicy()
	if err := json.Unmarshal(data, pol); err != nil {
		return nil, fmt.Errorf("failed to parse droidforge policy %q: %w", path, err)
	}

	if errs := ValidatePolicy(pol); len(errs) > 0 {
		return nil, fmt.Errorf("invalid droidforge policy %q: %s", path, strings.Join(errs, "; "))
	}

	return pol, nil
}

// ValidateConfig verifies consistency and required settings in DroidForgeConfig.
func ValidateConfig(cfg *types.DroidForgeConfig) []string {
	var errs []string
	if cfg == nil {
		return []string{"config is nil"}
	}
	if cfg.SchemaVersion < 1 {
		errs = append(errs, "schema_version must be >= 1")
	}
	if strings.TrimSpace(cfg.Project.Name) == "" {
		errs = append(errs, "project.name is required")
	}
	if cfg.Project.Mode != "" && cfg.Project.Mode != "source" && cfg.Project.Mode != "apk-only" {
		errs = append(errs, fmt.Sprintf("project.mode %q must be 'source' or 'apk-only'", cfg.Project.Mode))
	}
	if cfg.Execution.MaxStepsPerTask < 0 {
		errs = append(errs, "execution.max_steps_per_task cannot be negative")
	}
	if cfg.Execution.MaxIdenticalUIHashes < 0 {
		errs = append(errs, "execution.max_identical_ui_hashes cannot be negative")
	}
	if cfg.Execution.AVDRamMB < 0 {
		errs = append(errs, "execution.avd_ram_mb cannot be negative")
	}
	return errs
}

// ValidatePolicy checks that critical security policies are not bypassed.
func ValidatePolicy(pol *types.PolicyConfig) []string {
	var errs []string
	if pol == nil {
		return []string{"policy is nil"}
	}
	if pol.SchemaVersion < 1 {
		errs = append(errs, "schema_version must be >= 1")
	}
	if pol.Execution.AllowRawShell {
		errs = append(errs, "execution.allow_raw_shell must be false to defeat OWASP MCP05")
	}
	if pol.Sanitization.MaxNodeTextBytes <= 0 {
		errs = append(errs, "sanitization.max_node_text_bytes must be > 0")
	}
	if pol.Sanitization.MaxLogcatLineBytes <= 0 {
		errs = append(errs, "sanitization.max_logcat_line_bytes must be > 0")
	}
	if strings.TrimSpace(pol.Audit.LogDir) == "" {
		errs = append(errs, "audit.log_dir is required")
	}
	return errs
}

// ExpandHomeDir expands leading ~/ or ~\\ into the user's home directory.
func ExpandHomeDir(path string) string {
	if !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, "~\\") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}
