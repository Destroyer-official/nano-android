package schemas

import (
	"encoding/json"
	"testing"
)

func TestSchemas_EmbeddedValidity(t *testing.T) {
	tests := []struct {
		name   string
		schema []byte
	}{
		{"DroidForgeYAMLSchema", DroidForgeYAMLSchema},
		{"DroidForgePolicySchema", DroidForgePolicySchema},
		{"FixturesLockSchema", FixturesLockSchema},
		{"AuditRecordSchema", AuditRecordSchema},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.schema) == 0 {
				t.Fatalf("schema %s is empty", tc.name)
			}

			if err := ValidateRawJSON(tc.schema); err != nil {
				t.Fatalf("schema %s is not valid JSON: %v", tc.name, err)
			}

			var schemaMap map[string]any
			if err := json.Unmarshal(tc.schema, &schemaMap); err != nil {
				t.Fatalf("failed to unmarshal schema %s: %v", tc.name, err)
			}

			if _, ok := schemaMap["$schema"]; !ok {
				t.Errorf("schema %s missing '$schema' key", tc.name)
			}
			if _, ok := schemaMap["title"]; !ok {
				t.Errorf("schema %s missing 'title' key", tc.name)
			}
			if _, ok := schemaMap["type"]; !ok {
				t.Errorf("schema %s missing 'type' key", tc.name)
			}
		})
	}
}

func TestValidateRawJSON_Invalid(t *testing.T) {
	invalid := []byte(`{ not valid json `)
	if err := ValidateRawJSON(invalid); err == nil {
		t.Errorf("expected error for invalid JSON, got nil")
	}
}
