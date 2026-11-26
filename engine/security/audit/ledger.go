package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	sensitiveKeys = []string{
		"password", "token", "secret", "auth", "api_key", "apikey",
		"credential", "private_key", "access_token", "refresh_token", "cookie",
	}

	secretValuePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]{12,}`),
		regexp.MustCompile(`AIza[0-9A-Za-z\-_]{35}`),
		regexp.MustCompile(`gh[pousr]_[0-9A-Za-z]{24,}`),
		regexp.MustCompile(`sk-[A-Za-z0-9\-_]{20,}`),
		regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
	}
)

// RedactString strips embedded secrets and private keys from arbitrary strings.
func RedactString(s string) string {
	out := s
	for _, pat := range secretValuePatterns {
		out = pat.ReplaceAllString(out, "[REDACTED_SECRET]")
	}
	return out
}

// RedactMap recursively redacts sensitive keys and embedded secret tokens.
func RedactMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		low := strings.ToLower(k)
		redactKey := false
		for _, sk := range sensitiveKeys {
			if strings.Contains(low, sk) {
				redactKey = true
				break
			}
		}
		if redactKey {
			out[k] = "[REDACTED]"
			continue
		}
		switch val := v.(type) {
		case string:
			out[k] = RedactString(val)
		case map[string]any:
			out[k] = RedactMap(val)
		default:
			out[k] = val
		}
	}
	return out
}

// Entry is one immutable, SHA-256 hash-chained audit record.
type Entry struct {
	Seq          uint64         `json:"seq"`
	TimestampUTC string         `json:"timestamp_utc"`
	Surface      string         `json:"surface"` // "studio", "mcp", "cli", "worker"
	Actor        string         `json:"actor"`
	Action       string         `json:"action"`
	Resource     string         `json:"resource,omitempty"`
	Decision     string         `json:"decision"` // "ALLOW", "DENY", "ERROR", "APPROVAL_REQUIRED"
	Reason       string         `json:"reason,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	PrevHash     string         `json:"prev_hash"`
	EntryHash    string         `json:"entry_hash"`
}

// Ledger maintains an append-only SHA-256 chained audit log.
type Ledger struct {
	mu       sync.Mutex
	filePath string
	seq      uint64
	lastHash string
}

var (
	ledgerRegistryMu sync.Mutex
	openLedgers      = make(map[string]*Ledger)
)

// OpenLedger opens or creates a hash-chained JSONL audit ledger file, sharing one mutex-protected
// instance per canonical path in-process to prevent interleaved sequence races.
func OpenLedger(filePath string) (*Ledger, error) {
	if strings.TrimSpace(filePath) == "" {
		return nil, fmt.Errorf("empty audit ledger path")
	}
	abs, err := filepath.Abs(filePath)
	if err != nil {
		abs = filePath
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0750); err != nil {
		return nil, fmt.Errorf("create audit directory: %w", err)
	}

	key := strings.ToLower(abs)
	ledgerRegistryMu.Lock()
	defer ledgerRegistryMu.Unlock()
	if existing, ok := openLedgers[key]; ok {
		return existing, nil
	}

	l := &Ledger{
		filePath: abs,
		lastHash: strings.Repeat("0", 64),
	}
	if err := l.loadTail(); err != nil {
		return nil, err
	}
	openLedgers[key] = l
	return l, nil
}

func (l *Ledger) loadTail() error {
	f, err := os.Open(l.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return fmt.Errorf("corrupt existing audit entry: %w", err)
		}
		l.seq = e.Seq
		l.lastHash = e.EntryHash
	}
	return scanner.Err()
}

// Record appends a secret-redacted, hash-chained entry to disk and fsyncs.
func (l *Ledger) Record(surface, actor, action, resource, decision, reason string, metadata map[string]any) (Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, sErr := os.Stat(l.filePath); os.IsNotExist(sErr) {
		l.seq = 0
		l.lastHash = strings.Repeat("0", 64)
	} else {
		_ = l.loadTail()
	}

	l.seq++
	entry := Entry{
		Seq:          l.seq,
		TimestampUTC: time.Now().UTC().Format(time.RFC3339Nano),
		Surface:      surface,
		Actor:        actor,
		Action:       action,
		Resource:     RedactString(resource),
		Decision:     decision,
		Reason:       RedactString(reason),
		Metadata:     RedactMap(metadata),
		PrevHash:     l.lastHash,
	}
	entry.EntryHash = computeEntryHash(entry)

	line, err := json.Marshal(entry)
	if err != nil {
		return Entry{}, err
	}
	f, err := os.OpenFile(l.filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return Entry{}, err
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return Entry{}, err
	}
	_ = f.Sync()
	l.lastHash = entry.EntryHash
	return entry, nil
}

// VerifyChain validates the cryptographic integrity of every entry in the ledger file.
func (l *Ledger) FilePath() string {
	if l == nil {
		return ""
	}
	return l.filePath
}

func VerifyChain(filePath string) (int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	expectedPrev := strings.Repeat("0", 64)
	var expectedSeq uint64 = 1
	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return count, fmt.Errorf("malformed entry at #%d: %w", expectedSeq, err)
		}
		if e.Seq != expectedSeq {
			return count, fmt.Errorf("sequence mismatch: got %d want %d", e.Seq, expectedSeq)
		}
		if e.PrevHash != expectedPrev {
			return count, fmt.Errorf("chain broken at seq %d", e.Seq)
		}
		wantHash := computeEntryHash(e)
		if e.EntryHash != wantHash {
			return count, fmt.Errorf("hash mismatch at seq %d", e.Seq)
		}
		expectedPrev = e.EntryHash
		expectedSeq++
		count++
	}
	return count, scanner.Err()
}

func computeEntryHash(e Entry) string {
	e.EntryHash = ""
	raw, _ := json.Marshal(e)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
