package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/anthropic/droidforge-mcp/internal/types"
	"gopkg.in/yaml.v3"
)

// Simple JSON Schema validator in Go to test conformance without external heavy dependencies.
type JSONSchema struct {
	Title      string                  `json:"title"`
	Type       string                  `json:"type"`
	Required   []string                `json:"required"`
	Properties map[string]JSONProperty `json:"properties"`
	Additional *bool                   `json:"additionalProperties"`
}

type JSONProperty struct {
	Type       string                  `json:"type"`
	Enum       []string                `json:"enum"`
	Minimum    *float64                `json:"minimum"`
	Maximum    *float64                `json:"maximum"`
	MinItems   *int                    `json:"minItems"`
	Pattern    string                  `json:"pattern"`
	Required   []string                `json:"required"`
	Properties map[string]JSONProperty `json:"properties"`
}

func loadSchema(t *testing.T, relPath string) JSONSchema {
	t.Helper()
	data, err := os.ReadFile(relPath)
	if err != nil {
		t.Fatalf("failed to read schema at %s: %v", relPath, err)
	}

	var schema JSONSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("schema at %s is not valid JSON: %v", relPath, err)
	}
	return schema
}

func TestSchema_DroidForgeYAML(t *testing.T) {
	schemaPath := filepath.Join("..", "..", "schemas", "droidforge.yaml.schema.json")
	schema := loadSchema(t, schemaPath)

	if schema.Title == "" || schema.Type != "object" {
		t.Fatalf("invalid schema definition in %s", schemaPath)
	}

	// Read template YAML
	templatePath := filepath.Join("..", "..", "docs", "templates", "droidforge.yaml")
	yamlData, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("failed to read template %s: %v", templatePath, err)
	}

	// 1. Unmarshal into typed config struct
	var config types.DroidForgeConfig
	if err := yaml.Unmarshal(yamlData, &config); err != nil {
		t.Fatalf("failed to unmarshal droidforge.yaml into DroidForgeConfig: %v", err)
	}

	if config.SchemaVersion != 1 {
		t.Errorf("expected schema_version 1, got %d", config.SchemaVersion)
	}
	if config.Project.Name == "" || len(config.Project.AllowlistedPackages) == 0 {
		t.Errorf("expected project name and allowlisted packages, got %+v", config.Project)
	}
	if config.Workloads.Profile == "" {
		t.Errorf("expected non-empty profile in workloads")
	}

	// 2. Validate generic map against schema required fields
	var generic map[string]any
	if err := yaml.Unmarshal(yamlData, &generic); err != nil {
		t.Fatalf("failed to unmarshal into generic map: %v", err)
	}

	for _, req := range schema.Required {
		if _, exists := generic[req]; !exists {
			t.Errorf("droidforge.yaml missing required field %q per schema", req)
		}
	}
}

func TestSchema_DroidForgePolicy(t *testing.T) {
	schemaPath := filepath.Join("..", "..", "schemas", "droidforge.policy.schema.json")
	schema := loadSchema(t, schemaPath)

	policyPath := filepath.Join("..", "..", "docs", "templates", "droidforge.policy.json")
	policyData, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("failed to read policy file %s: %v", policyPath, err)
	}

	// 1. Unmarshal into typed policy struct
	var policy types.PolicyConfig
	if err := json.Unmarshal(policyData, &policy); err != nil {
		t.Fatalf("failed to unmarshal into PolicyConfig: %v", err)
	}

	if policy.SchemaVersion != 1 {
		t.Errorf("expected schema_version 1, got %d", policy.SchemaVersion)
	}
	if policy.Network.BindHost != "127.0.0.1" {
		t.Errorf("expected bind_host 127.0.0.1, got %s", policy.Network.BindHost)
	}
	if policy.Execution.AllowRawShell {
		t.Errorf("expected allow_raw_shell to be false in secure policy")
	}
	if !policy.Execution.EnforcePackageScopeLock {
		t.Errorf("expected enforce_package_scope_lock to be true")
	}

	// 2. Validate generic map against schema required fields
	var generic map[string]any
	if err := json.Unmarshal(policyData, &generic); err != nil {
		t.Fatalf("failed to unmarshal into generic map: %v", err)
	}

	for _, req := range schema.Required {
		if _, exists := generic[req]; !exists {
			t.Errorf("droidforge.policy.json missing required field %q per schema", req)
		}
	}
}

func TestSchema_FixturesLock(t *testing.T) {
	schemaPath := filepath.Join("..", "..", "schemas", "fixtures.lock.schema.json")
	schema := loadSchema(t, schemaPath)

	sampleJSON := []byte(`{
		"schema_version": 1,
		"fixtures": [
			{
				"id": "motion_1080p60_synth",
				"name": "10s 1080p60 Synthetic Motion Pattern",
				"sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				"size_bytes": 14582920,
				"relative_path": "fixtures/video/synthetic_1080p60.mp4",
				"mime_type": "video/mp4",
				"metadata": {
					"duration_s": 10.0,
					"resolution": "1920x1080",
					"fps": 60,
					"codec": "h264"
				}
			}
		]
	}`)

	var generic map[string]any
	if err := json.Unmarshal(sampleJSON, &generic); err != nil {
		t.Fatalf("sample fixtures.lock.json is invalid JSON: %v", err)
	}

	for _, req := range schema.Required {
		if _, exists := generic[req]; !exists {
			t.Errorf("fixtures.lock.json missing required field %q", req)
		}
	}

	fixturesList, ok := generic["fixtures"].([]any)
	if !ok || len(fixturesList) == 0 {
		t.Fatalf("expected non-empty fixtures array")
	}

	first := fixturesList[0].(map[string]any)
	sha := first["sha256"].(string)
	shaPattern := regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	if !shaPattern.MatchString(sha) {
		t.Errorf("fixture SHA-256 does not match 64-char hex pattern: %s", sha)
	}
}

func TestSchema_AuditRecord(t *testing.T) {
	schemaPath := filepath.Join("..", "..", "schemas", "audit-record.schema.json")
	schema := loadSchema(t, schemaPath)

	sampleRecordJSON := []byte(`{
		"ts": "2026-11-20T19:30:12.441Z",
		"session_id": "sess_9c41f2",
		"client_ide": "antigravity",
		"tool": "act_and_observe",
		"risk_tier": "WRITE_SCOPED",
		"device_serial": "emulator-5554",
		"package_lock": "com.example.scamshield.debug",
		"args_redacted": { "action": "tap_node", "target": "n1" },
		"policy_decision": "ALLOW",
		"exit_code": 0,
		"duration_ms": 284,
		"ui_hash_after": "9f3a1c",
		"artifacts": [".droidforge/runs/sess_9c41f2/step_04.webp"]
	}`)

	// 1. Verify unmarshaling into types.AuditRecord
	var record types.AuditRecord
	if err := json.Unmarshal(sampleRecordJSON, &record); err != nil {
		t.Fatalf("failed to unmarshal into types.AuditRecord: %v", err)
	}

	if record.SessionID != "sess_9c41f2" {
		t.Errorf("expected session_id sess_9c41f2, got %s", record.SessionID)
	}
	if record.Tool != "act_and_observe" {
		t.Errorf("expected tool act_and_observe, got %s", record.Tool)
	}
	if record.Timestamp.IsZero() {
		t.Errorf("expected valid non-zero timestamp, got %v", record.Timestamp)
	}

	// 2. Verify schema required properties
	var generic map[string]any
	if err := json.Unmarshal(sampleRecordJSON, &generic); err != nil {
		t.Fatalf("failed to unmarshal into generic map: %v", err)
	}

	for _, req := range schema.Required {
		if _, exists := generic[req]; !exists {
			t.Errorf("audit record missing required field %q", req)
		}
	}
}

func TestTemplates_MCPJSONValid(t *testing.T) {
	mcpPath := filepath.Join("..", "..", "docs", "templates", "mcp.json")
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("failed to read mcp.json: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("docs/templates/mcp.json is not valid JSON: %v", err)
	}

	ideConfigs, ok := parsed["ide_configurations"].(map[string]any)
	if !ok {
		t.Fatalf("missing ide_configurations in mcp.json")
	}

	expectedIDEs := []string{"claude_code", "cursor", "vscode", "antigravity"}
	for _, ide := range expectedIDEs {
		if _, exists := ideConfigs[ide]; !exists {
			t.Errorf("mcp.json missing configuration for IDE %q", ide)
		}
	}
}
