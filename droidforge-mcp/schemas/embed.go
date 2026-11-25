// Package schemas provides embedded JSON schemas for DroidForge configuration,
// security policy, fixtures lock, and audit records.
package schemas

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// DroidForgeYAMLSchema contains the schema for droidforge.yaml.
//
//go:embed droidforge.yaml.schema.json
var DroidForgeYAMLSchema []byte

// DroidForgePolicySchema contains the schema for droidforge.policy.json.
//
//go:embed droidforge.policy.schema.json
var DroidForgePolicySchema []byte

// FixturesLockSchema contains the schema for fixtures.lock.json.
//
//go:embed fixtures.lock.schema.json
var FixturesLockSchema []byte

// AuditRecordSchema contains the schema for single JSONL audit log records.
//
//go:embed audit-record.schema.json
var AuditRecordSchema []byte

// ValidateRawJSON checks if data is valid JSON.
func ValidateRawJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}
