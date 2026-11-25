package compiler

import (
	"testing"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

func TestSchemaValidator_ValidManifest(t *testing.T) {
	validYAML := []byte(`
schema_version: 1
project:
  name: "test-app"
  mode: "source"
  allowlisted_packages:
    - "com.example.test"
execution:
  max_steps_per_task: 15
workloads:
  profile: "scam-shield"
`)
	cfg, errs, err := ValidateDroidForgeYAML(validYAML)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("expected 0 validation errors, got %d: %v", len(errs), errs)
	}
	if cfg.Project.Name != "test-app" {
		t.Errorf("expected project name 'test-app', got %s", cfg.Project.Name)
	}
}

func TestSchemaValidator_InvalidManifest(t *testing.T) {
	invalidYAML := []byte(`
schema_version: 0
project:
  name: ""
  mode: "invalid-mode"
  allowlisted_packages:
    - "bad package name"
workloads:
  profile: "unknown-profile"
`)
	_, errs, err := ValidateDroidForgeYAML(invalidYAML)
	if err != nil {
		t.Fatalf("parse should succeed despite validation errors: %v", err)
	}
	if len(errs) < 4 {
		t.Errorf("expected at least 4 validation errors for invalid manifest, got %d: %v", len(errs), errs)
	}
}

func TestSchemaValidator_ValidPolicy(t *testing.T) {
	validJSON := []byte(`{
  "schema_version": 1,
  "network": {
    "bind_host": "127.0.0.1",
    "allow_unencrypted_adb_tcpip_5555": false,
    "allow_tls_wireless_debugging": true
  },
  "execution": {
    "allow_raw_shell": false,
    "enforce_package_scope_lock": true,
    "allow_contacts_sms_read_on_physical": false,
    "blocked_binaries": ["fastboot", "sh"],
    "allowed_settings_keys": ["system:screen_brightness"]
  },
  "sanitization": {
    "wrap_device_text_with_random_nonce": true,
    "strip_zero_width_and_bidi_chars": true,
    "max_node_text_bytes": 256,
    "max_logcat_line_bytes": 512
  },
  "human_approval_required": ["register_new_physical_device"],
  "audit": {
    "log_dir": "~/.droidforge/audit",
    "retention_days": 90
  }
}`)
	pol, errs, err := ValidateDroidForgePolicy(validJSON)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("expected 0 validation errors, got %d: %v", len(errs), errs)
	}
	if pol.Network.BindHost != "127.0.0.1" {
		t.Errorf("expected bind_host 127.0.0.1, got %s", pol.Network.BindHost)
	}
}

func TestSchemaValidator_InvalidPolicy(t *testing.T) {
	insecureJSON := []byte(`{
  "schema_version": 1,
  "network": { "bind_host": "0.0.0.0" },
  "execution": { "allow_raw_shell": true, "enforce_package_scope_lock": false },
  "audit": { "log_dir": "", "retention_days": 0 }
}`)
	_, errs, err := ValidateDroidForgePolicy(insecureJSON)
	if err != nil {
		t.Fatalf("parse should succeed: %v", err)
	}
	if len(errs) < 4 {
		t.Errorf("expected security rejections for 0.0.0.0, raw shell, and empty audit, got %d: %v", len(errs), errs)
	}
}

func TestSchemaValidator_FixturesLock(t *testing.T) {
	validJSON := []byte(`{
  "schema_version": 1,
  "fixtures": [
    {
      "id": "fix_01",
      "name": "Test Clip",
      "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "size_bytes": 1024,
      "relative_path": "fixtures/test.mp4"
    }
  ]
}`)
	lock, errs, err := ValidateFixturesLock(validJSON)
	if err != nil || len(errs) != 0 {
		t.Fatalf("valid fixtures.lock failed: %v, errs: %v", err, errs)
	}
	if len(lock.Fixtures) != 1 {
		t.Errorf("expected 1 fixture, got %d", len(lock.Fixtures))
	}

	invalidJSON := []byte(`{
  "schema_version": 1,
  "fixtures": [
    {
      "id": "",
      "name": "Bad Clip",
      "sha256": "not-a-valid-sha",
      "size_bytes": -10,
      "relative_path": ""
    }
  ]
}`)
	_, badErrs, err := ValidateFixturesLock(invalidJSON)
	if err != nil {
		t.Fatalf("parse should succeed: %v", err)
	}
	if len(badErrs) < 3 {
		t.Errorf("expected at least 3 errors for invalid fixture fields, got %d: %v", len(badErrs), badErrs)
	}
}

func TestSchemaValidator_AuditRecord(t *testing.T) {
	validRec := &types.AuditRecord{
		Timestamp:  time.Now(),
		SessionID:  "sess_123",
		ClientIDE:  "antigravity",
		Tool:       "act_and_observe",
		RiskTier:   "WRITE_SCOPED",
		Decision:   "ALLOW",
		ExitCode:   0,
		DurationMS: 150,
	}
	if errs := ValidateAuditRecord(validRec); len(errs) != 0 {
		t.Errorf("expected valid audit record, got errs: %v", errs)
	}

	badRec := &types.AuditRecord{
		SessionID: "",
		RiskTier:  "INVALID_TIER",
		Decision:  "INVALID_DECISION",
	}
	if errs := ValidateAuditRecord(badRec); len(errs) < 4 {
		t.Errorf("expected at least 4 errors for bad audit record, got %d: %v", len(errs), errs)
	}
}
