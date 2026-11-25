package compiler

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/anthropic/droidforge-mcp/internal/types"
	"github.com/anthropic/droidforge-mcp/schemas"
	"gopkg.in/yaml.v3"
)

var (
	packageRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*)+$`)
	sha256Regex  = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
)

// ValidateDroidForgeYAML parses and validates droidforge.yaml content against schema rules.
func ValidateDroidForgeYAML(data []byte) (*types.DroidForgeConfig, []string, error) {
	// First check raw schema bytes
	if len(schemas.DroidForgeYAMLSchema) == 0 {
		return nil, nil, fmt.Errorf("embedded droidforge.yaml schema is empty")
	}

	var cfg types.DroidForgeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, nil, fmt.Errorf("failed to parse droidforge.yaml: %w", err)
	}

	var errs []string

	if cfg.SchemaVersion < 1 {
		errs = append(errs, "schema_version must be >= 1")
	}
	if strings.TrimSpace(cfg.Project.Name) == "" {
		errs = append(errs, "project.name is required")
	}
	if cfg.Project.Mode != "source" && cfg.Project.Mode != "apk-only" {
		errs = append(errs, fmt.Sprintf("project.mode must be 'source' or 'apk-only', got %q", cfg.Project.Mode))
	}
	if len(cfg.Project.AllowlistedPackages) == 0 {
		errs = append(errs, "project.allowlisted_packages must contain at least one package ID")
	}
	for _, pkg := range cfg.Project.AllowlistedPackages {
		if !packageRegex.MatchString(pkg) {
			errs = append(errs, fmt.Sprintf("invalid package name format: %q", pkg))
		}
	}

	if cfg.Workloads.Profile != "" {
		p := cfg.Workloads.Profile
		if p != "standard" && p != "scam-shield" && p != "video-codec" {
			errs = append(errs, fmt.Sprintf("workloads.profile must be 'standard', 'scam-shield', or 'video-codec', got %q", p))
		}
	}

	if cfg.Execution.MaxStepsPerTask < 0 {
		errs = append(errs, "execution.max_steps_per_task must be >= 0")
	}
	if cfg.Execution.AVDRamMB < 0 {
		errs = append(errs, "execution.avd_ram_mb must be >= 0")
	}

	return &cfg, errs, nil
}

// ValidateDroidForgePolicy parses and validates droidforge.policy.json content against schema rules.
func ValidateDroidForgePolicy(data []byte) (*types.PolicyConfig, []string, error) {
	if len(schemas.DroidForgePolicySchema) == 0 {
		return nil, nil, fmt.Errorf("embedded droidforge.policy.json schema is empty")
	}

	var pol types.PolicyConfig
	if err := json.Unmarshal(data, &pol); err != nil {
		return nil, nil, fmt.Errorf("failed to parse droidforge.policy.json: %w", err)
	}

	var errs []string

	if pol.SchemaVersion < 1 {
		errs = append(errs, "schema_version must be >= 1")
	}
	if pol.Network.BindHost == "" {
		errs = append(errs, "network.bind_host is required")
	} else if pol.Network.BindHost != "127.0.0.1" && pol.Network.BindHost != "localhost" {
		errs = append(errs, fmt.Sprintf("network.bind_host must be localhost/127.0.0.1 for security, got %q", pol.Network.BindHost))
	}

	if pol.Execution.AllowRawShell {
		errs = append(errs, "execution.allow_raw_shell must be false to defeat OWASP MCP05 command injection")
	}
	if !pol.Execution.EnforcePackageScopeLock {
		errs = append(errs, "execution.enforce_package_scope_lock must be true to confine mutations to allowlisted packages")
	}

	if pol.Audit.LogDir == "" {
		errs = append(errs, "audit.log_dir is required")
	}
	if pol.Audit.RetentionDays < 1 {
		errs = append(errs, "audit.retention_days must be >= 1")
	}

	return &pol, errs, nil
}

// FixtureEntry represents an item in fixtures.lock.json.
type FixtureEntry struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	SHA256       string         `json:"sha256"`
	SizeBytes    int64          `json:"size_bytes"`
	RelativePath string         `json:"relative_path"`
	MIMEType     string         `json:"mime_type,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// FixturesLock represents the parsed fixtures.lock.json structure.
type FixturesLock struct {
	SchemaVersion int            `json:"schema_version"`
	Fixtures      []FixtureEntry `json:"fixtures"`
}

// ValidateFixturesLock validates a fixtures.lock.json manifest.
func ValidateFixturesLock(data []byte) (*FixturesLock, []string, error) {
	var lock FixturesLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, nil, fmt.Errorf("invalid fixtures.lock.json syntax: %w", err)
	}

	var errs []string
	if lock.SchemaVersion < 1 {
		errs = append(errs, "schema_version must be >= 1")
	}
	if len(lock.Fixtures) == 0 {
		errs = append(errs, "fixtures list must not be empty")
	}

	for idx, fix := range lock.Fixtures {
		if fix.ID == "" {
			errs = append(errs, fmt.Sprintf("fixtures[%d].id is required", idx))
		}
		if fix.Name == "" {
			errs = append(errs, fmt.Sprintf("fixtures[%d].name is required", idx))
		}
		if !sha256Regex.MatchString(fix.SHA256) {
			errs = append(errs, fmt.Sprintf("fixtures[%d].sha256 must be a 64-character hex string, got %q", idx, fix.SHA256))
		}
		if fix.SizeBytes < 0 {
			errs = append(errs, fmt.Sprintf("fixtures[%d].size_bytes must be >= 0", idx))
		}
		if fix.RelativePath == "" {
			errs = append(errs, fmt.Sprintf("fixtures[%d].relative_path is required", idx))
		}
	}

	return &lock, errs, nil
}

// ValidateAuditRecord checks if an audit record has all required fields and valid enums.
func ValidateAuditRecord(rec *types.AuditRecord) []string {
	var errs []string
	if rec.Timestamp.IsZero() {
		errs = append(errs, "ts (timestamp) must not be zero")
	}
	if rec.SessionID == "" {
		errs = append(errs, "session_id is required")
	}
	if rec.ClientIDE == "" {
		errs = append(errs, "client_ide is required")
	}
	if rec.Tool == "" {
		errs = append(errs, "tool name is required")
	}
	if rec.RiskTier != "READ" && rec.RiskTier != "WRITE_SCOPED" && rec.RiskTier != "APPROVAL_GATED" {
		errs = append(errs, fmt.Sprintf("risk_tier must be READ, WRITE_SCOPED, or APPROVAL_GATED; got %q", rec.RiskTier))
	}
	if rec.Decision != "ALLOW" && rec.Decision != "DENY" && rec.Decision != "APPROVAL_REQUIRED" && rec.Decision != "HALT" {
		errs = append(errs, fmt.Sprintf("policy_decision must be ALLOW, DENY, APPROVAL_REQUIRED, or HALT; got %q", rec.Decision))
	}
	return errs
}
