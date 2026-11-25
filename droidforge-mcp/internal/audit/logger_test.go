package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

func TestJSONLLogger_LogAndRead(t *testing.T) {
	tempDir := t.TempDir()

	logger, err := NewJSONLLogger(tempDir, 90)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	recordDate := time.Date(2026, time.November, 20, 14, 30, 0, 0, time.UTC)

	record1 := types.AuditRecord{
		Timestamp:    recordDate,
		SessionID:    "session-test-01",
		ClientIDE:    "Antigravity",
		Tool:         "observe_screen",
		RiskTier:     "READ",
		DeviceSerial: "emulator-5554",
		PackageLock:  "com.example.myapp",
		ArgsRedacted: map[string]any{"serial": "emulator-5554", "auth_token": "super_secret_token_123"},
		Decision:     "ALLOW",
		ExitCode:     0,
		DurationMS:   142,
		UIHashAfter:  "hash-abc-001",
		Artifacts:    []string{"screen_001.webp"},
	}

	record2 := types.AuditRecord{
		Timestamp:    recordDate.Add(5 * time.Minute),
		SessionID:    "session-test-01",
		ClientIDE:    "Antigravity",
		Tool:         "act_and_observe",
		RiskTier:     "WRITE_SCOPED",
		DeviceSerial: "emulator-5554",
		PackageLock:  "com.example.myapp",
		ArgsRedacted: map[string]any{"action": "tap", "x": 100, "y": 200},
		Decision:     "ALLOW",
		ExitCode:     0,
		DurationMS:   230,
		UIHashAfter:  "hash-abc-002",
		Artifacts:    []string{"screen_002.webp"},
	}

	if err := logger.Log(record1); err != nil {
		t.Fatalf("failed to log record 1: %v", err)
	}
	if err := logger.Log(record2); err != nil {
		t.Fatalf("failed to log record 2: %v", err)
	}

	// Verify the daily log file exists: 2026-11-20.jsonl
	expectedFileName := "2026-11-20.jsonl"
	expectedPath := filepath.Join(tempDir, expectedFileName)
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatalf("expected log file %s to exist, but was not found", expectedPath)
	}

	// Read records back and verify
	records, err := logger.ReadRecords(recordDate)
	if err != nil {
		t.Fatalf("failed to read records: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	r1 := records[0]
	if r1.SessionID != "session-test-01" || r1.Tool != "observe_screen" || r1.RiskTier != "READ" {
		t.Errorf("record 1 contents mismatch: %+v", r1)
	}
	// Verify automatic redaction
	if r1.ArgsRedacted["auth_token"] != "[REDACTED]" {
		t.Errorf("expected auth_token to be redacted, got %v", r1.ArgsRedacted["auth_token"])
	}
	if r1.UIHashAfter != "hash-abc-001" || len(r1.Artifacts) != 1 || r1.Artifacts[0] != "screen_001.webp" {
		t.Errorf("record 1 metadata mismatch: %+v", r1)
	}

	r2 := records[1]
	if r2.Tool != "act_and_observe" || r2.RiskTier != "WRITE_SCOPED" || r2.DurationMS != 230 {
		t.Errorf("record 2 contents mismatch: %+v", r2)
	}
}

func TestJSONLLogger_FindRecords(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := NewJSONLLogger(tempDir, 90)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	baseTime := time.Date(2026, time.November, 20, 10, 0, 0, 0, time.UTC)

	_ = logger.Log(types.AuditRecord{
		Timestamp: baseTime,
		SessionID: "sess-A",
		Tool:      "observe_screen",
		Decision:  "ALLOW",
	})
	_ = logger.Log(types.AuditRecord{
		Timestamp: baseTime.Add(1 * time.Hour),
		SessionID: "sess-B",
		Tool:      "act_and_observe",
		Decision:  "DENY",
	})

	// Search by tool
	found, err := logger.FindRecords(RecordFilter{Tool: "act_and_observe"})
	if err != nil {
		t.Fatalf("FindRecords failed: %v", err)
	}
	if len(found) != 1 || found[0].SessionID != "sess-B" {
		t.Errorf("expected 1 record for act_and_observe, got %+v", found)
	}

	// Search by decision
	foundDeny, err := logger.FindRecords(RecordFilter{Decision: "DENY"})
	if err != nil {
		t.Fatalf("FindRecords failed: %v", err)
	}
	if len(foundDeny) != 1 || foundDeny[0].Tool != "act_and_observe" {
		t.Errorf("expected 1 DENY record, got %+v", foundDeny)
	}
}

func TestJSONLLogger_PruneOldLogs(t *testing.T) {
	tempDir := t.TempDir()

	logger, err := NewJSONLLogger(tempDir, 30) // 30 days retention
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Create an old file: 40 days ago
	oldDate := time.Now().UTC().AddDate(0, 0, -40)
	oldFileName := filepath.Join(tempDir, oldDate.Format("2006-01-02")+".jsonl")
	if err := os.WriteFile(oldFileName, []byte("{\"tool\":\"test\"}\n"), 0600); err != nil {
		t.Fatalf("failed to create old file: %v", err)
	}

	// Create a recent file: 5 days ago
	recentDate := time.Now().UTC().AddDate(0, 0, -5)
	recentFileName := filepath.Join(tempDir, recentDate.Format("2006-01-02")+".jsonl")
	if err := os.WriteFile(recentFileName, []byte("{\"tool\":\"test\"}\n"), 0600); err != nil {
		t.Fatalf("failed to create recent file: %v", err)
	}

	pruned, err := logger.PruneOldLogs()
	if err != nil {
		t.Fatalf("PruneOldLogs failed: %v", err)
	}

	if pruned != 1 {
		t.Errorf("expected 1 file pruned, got %d", pruned)
	}

	if _, err := os.Stat(oldFileName); !os.IsNotExist(err) {
		t.Errorf("expected old file %s to be deleted, but it still exists", oldFileName)
	}

	if _, err := os.Stat(recentFileName); os.IsNotExist(err) {
		t.Errorf("expected recent file %s to be kept, but it was deleted", recentFileName)
	}
}
