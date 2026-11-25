package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

var sensitiveKeyWords = []string{
	"password", "token", "secret", "auth", "api_key",
	"credential", "private_key", "access_token", "refresh_token",
}

// RedactArgs recursively redacts sensitive parameters in an argument map.
func RedactArgs(args map[string]any) map[string]any {
	if args == nil {
		return nil
	}
	redacted := make(map[string]any, len(args))
	for k, v := range args {
		lowerK := strings.ToLower(k)
		isSensitive := false
		for _, s := range sensitiveKeyWords {
			if strings.Contains(lowerK, s) {
				isSensitive = true
				break
			}
		}
		if isSensitive {
			redacted[k] = "[REDACTED]"
		} else {
			switch val := v.(type) {
			case map[string]any:
				redacted[k] = RedactArgs(val)
			default:
				redacted[k] = v
			}
		}
	}
	return redacted
}

// RecordFilter specifies criteria for searching forensic audit logs.
type RecordFilter struct {
	SessionID string
	Tool      string
	RiskTier  string
	Decision  string
	Since     time.Time
	Until     time.Time
}

// JSONLLogger implements types.AuditLogger by writing append-only JSONL files,
// one per day (YYYY-MM-DD.jsonl), with configurable retention.
type JSONLLogger struct {
	mu            sync.Mutex
	dir           string
	retentionDays int
}

// NewJSONLLogger initializes a new JSONL audit logger in the given directory.
// If dir is empty, it defaults to ~/.droidforge/audit/.
// If retentionDays <= 0, it defaults to 90 days.
func NewJSONLLogger(dir string, retentionDays int) (*JSONLLogger, error) {
	resolvedDir := dir
	if resolvedDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve user home dir: %w", err)
		}
		resolvedDir = filepath.Join(home, ".droidforge", "audit")
	} else if strings.HasPrefix(resolvedDir, "~/") || strings.HasPrefix(resolvedDir, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve user home dir: %w", err)
		}
		resolvedDir = filepath.Join(home, resolvedDir[2:])
	}

	if retentionDays <= 0 {
		retentionDays = 90
	}

	if err := os.MkdirAll(resolvedDir, 0755); err != nil {
		return nil, fmt.Errorf("create audit log dir %s: %w", resolvedDir, err)
	}

	logger := &JSONLLogger{
		dir:           resolvedDir,
		retentionDays: retentionDays,
	}

	// Automatic startup cleanup of expired records
	_, _ = logger.PruneOldLogs()

	return logger, nil
}

// Dir returns the directory where audit logs are stored.
func (l *JSONLLogger) Dir() string {
	return l.dir
}

// RetentionDays returns the configured retention policy in days.
func (l *JSONLLogger) RetentionDays() int {
	return l.retentionDays
}

// Log writes one audit record to today's append-only JSONL log file.
// Sensitive arguments in ArgsRedacted are automatically redacted if present.
// The file is opened with os.O_APPEND|os.O_CREATE|os.O_WRONLY and synced to disk.
func (l *JSONLLogger) Log(record types.AuditRecord) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	ts := record.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
		record.Timestamp = ts
	}

	// Ensure sensitive parameters are redacted
	if record.ArgsRedacted != nil {
		record.ArgsRedacted = RedactArgs(record.ArgsRedacted)
	}

	filename := fmt.Sprintf("%s.jsonl", ts.Format("2006-01-02"))
	filePath := filepath.Join(l.dir, filename)

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open audit file %s: %w", filePath, err)
	}
	defer f.Close()

	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal audit record: %w", err)
	}

	payload = append(payload, '\n')
	if _, err := f.Write(payload); err != nil {
		return fmt.Errorf("append to audit file %s: %w", filePath, err)
	}

	// Ensure durable write to physical disk
	_ = f.Sync()

	return nil
}

// PruneOldLogs deletes log files older than the retention period.
func (l *JSONLLogger) PruneOldLogs() (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return 0, fmt.Errorf("read audit dir %s: %w", l.dir, err)
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -l.retentionDays)
	prunedCount := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		datePart := strings.TrimSuffix(entry.Name(), ".jsonl")
		fileDate, err := time.Parse("2006-01-02", datePart)
		if err != nil {
			// Not a standard daily log file, skip
			continue
		}

		if fileDate.Before(cutoff) {
			fullPath := filepath.Join(l.dir, entry.Name())
			if err := os.Remove(fullPath); err == nil {
				prunedCount++
			}
		}
	}

	return prunedCount, nil
}

// ReadRecords reads and parses all records from the log file for the specified date.
func (l *JSONLLogger) ReadRecords(date time.Time) ([]types.AuditRecord, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	filename := fmt.Sprintf("%s.jsonl", date.Format("2006-01-02"))
	filePath := filepath.Join(l.dir, filename)

	return l.readFromFile(filePath)
}

// FindRecords searches across all daily log files within the directory matching the filter.
func (l *JSONLLogger) FindRecords(filter RecordFilter) ([]types.AuditRecord, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, fmt.Errorf("read audit dir %s: %w", l.dir, err)
	}

	var matched []types.AuditRecord

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		datePart := strings.TrimSuffix(entry.Name(), ".jsonl")
		fileDate, err := time.Parse("2006-01-02", datePart)
		if err != nil {
			continue
		}

		if !filter.Since.IsZero() && fileDate.Add(24*time.Hour).Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && fileDate.After(filter.Until) {
			continue
		}

		filePath := filepath.Join(l.dir, entry.Name())
		records, err := l.readFromFile(filePath)
		if err != nil {
			continue
		}

		for _, r := range records {
			if filter.SessionID != "" && r.SessionID != filter.SessionID {
				continue
			}
			if filter.Tool != "" && r.Tool != filter.Tool {
				continue
			}
			if filter.RiskTier != "" && r.RiskTier != filter.RiskTier {
				continue
			}
			if filter.Decision != "" && r.Decision != filter.Decision {
				continue
			}
			if !filter.Since.IsZero() && r.Timestamp.Before(filter.Since) {
				continue
			}
			if !filter.Until.IsZero() && r.Timestamp.After(filter.Until) {
				continue
			}
			matched = append(matched, r)
		}
	}

	return matched, nil
}

func (l *JSONLLogger) readFromFile(filePath string) ([]types.AuditRecord, error) {
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open audit file %s: %w", filePath, err)
	}
	defer f.Close()

	var records []types.AuditRecord
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var r types.AuditRecord
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, fmt.Errorf("parse audit line %d in %s: %w", lineNum, filePath, err)
		}
		records = append(records, r)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan audit file %s: %w", filePath, err)
	}

	return records, nil
}
