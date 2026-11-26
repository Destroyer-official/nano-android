package intel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"nano_android/engine/compatibility/registry"
)

// ContractRecord captures a tracked framework API contract + continuous learning evidence counters.
type ContractRecord struct {
	API             string  `json:"api"`
	Level           string  `json:"level"`
	SinceAPI        int     `json:"since_api"`
	UntilAPI        int     `json:"until_api"`
	FallbackBackend string  `json:"fallback_backend,omitempty"`
	Notes           string  `json:"notes"`
	PassCount       int     `json:"pass_count"`
	FailCount       int     `json:"fail_count"`
	DivergeCount    int     `json:"diverge_count"`
	Confidence      float64 `json:"confidence"`
}

// APILevelCoverage summarizes compatibility readiness for one Android API level (26..37).
type APILevelCoverage struct {
	APILevel       int     `json:"api_level"`
	TrackedAPIs    int     `json:"tracked_apis"`
	SupportedAPIs  int     `json:"supported_apis"`
	AvgConfidence  float64 `json:"avg_confidence"`
	EvidenceEvents int     `json:"evidence_events"`
}

// DeviceQuirk records an observed hardware, OEM, foldable, tablet, or multi-window quirk.
type DeviceQuirk struct {
	ProfileID    string `json:"profile_id"`
	FormFactor   string `json:"form_factor"`
	API          string `json:"api"`
	QuirkSummary string `json:"quirk_summary"`
	Severity     string `json:"severity"`
}

// CompatibilityHeatmap represents the queryable API 26-37 matrix, confidence scores, and device quirks.
type CompatibilityHeatmap struct {
	DBPath    string             `json:"db_path"`
	Contracts []ContractRecord   `json:"contracts"`
	APILevels []APILevelCoverage `json:"api_levels"`
	Quirks    []DeviceQuirk      `json:"quirks,omitempty"`
}

// Store persists continuous compatibility intelligence into a real SQLite database (`compatibility.db`)
// powered by pinned `sqlite-jdbc-3.54.1.0.jar` via `nanoandroid.compat.CompatibilityDbTool`.
type Store struct {
	mu           sync.Mutex
	dbPath       string
	classpath    string
}

func resolveToolArtifact(rel string) string {
	wd, _ := os.Getwd()
	cur := wd
	for i := 0; i < 5; i++ {
		cand := filepath.Join(cur, rel)
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return rel
}

// OpenStore initializes `compatibility.db` with schema, default API 26-37 contracts, and baseline device quirks.
func OpenStore(dbPath string) (*Store, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("dbPath is required")
	}
	absDB, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absDB), 0750); err != nil {
		return nil, err
	}

	fwJar := resolveToolArtifact(filepath.Join("engine", "runtime", "nano-framework.jar"))
	sqliteJar := resolveToolArtifact(filepath.Join("tools", "libs", "sqlite-jdbc-3.54.1.0.jar"))
	sep := string(os.PathListSeparator)
	cp := fwJar + sep + sqliteJar

	s := &Store{
		dbPath:    absDB,
		classpath: cp,
	}
	if _, err := s.exec("init"); err != nil {
		return nil, fmt.Errorf("initialize compatibility.db: %w", err)
	}
	return s, nil
}

// SeedDefaults populates `registry.DefaultContracts()` and canonical foldable/tablet/multi-window quirks.
func (s *Store) SeedDefaults() error {
	for _, c := range registry.DefaultContracts() {
		initConf := 0.85
		if c.Level == registry.SupportRequiresART || c.Level == registry.SupportRequiresHardware {
			initConf = 0.65
		} else if c.Level == registry.SupportFullyImplemented {
			initConf = 0.95
		}
		if _, err := s.exec(
			"upsert_contract",
			c.API,
			string(c.Level),
			strconv.Itoa(c.SinceAPI),
			strconv.Itoa(c.UntilAPI),
			string(c.FallbackBackend),
			c.Notes,
			fmt.Sprintf("%.2f", initConf),
		); err != nil {
			return err
		}
	}

	quirks := []DeviceQuirk{
		{ProfileID: "foldable_unfolded", FormFactor: "foldable", API: "android.app.Activity", QuirkSummary: "Posture transitions trigger screenSize/smallestScreenSize configuration changes", Severity: "MEDIUM"},
		{ProfileID: "tablet_12in", FormFactor: "tablet_multiwindow", API: "android.app.Activity", QuirkSummary: "Android 17 (API 37) enforces mandatory resizability on >=600dp large screens", Severity: "HIGH"},
		{ProfileID: "api37_restricted", FormFactor: "android_17", API: "android.Manifest.permission.READ_SMS", QuirkSummary: "Android 17 imposes 3-hour OTP SMS read delay unless SMS User Consent API is used", Severity: "HIGH"},
	}
	for _, q := range quirks {
		if err := s.UpsertQuirk(q); err != nil {
			return err
		}
	}
	return nil
}

// RecordConformanceEvidence updates contract/API-level confidence metrics from unit/conformance executions.
func (s *Store) RecordConformanceEvidence(api, methodName string, backend registry.Backend, apiLevel int, passed bool, details string) error {
	pFlag := "0"
	if passed {
		pFlag = "1"
	}
	if apiLevel <= 0 {
		apiLevel = 35
	}
	_, err := s.exec("record_evidence", "conformance", api, methodName, string(backend), strconv.Itoa(apiLevel), pFlag, details)
	return err
}

// RecordDifferentialEvidence logs cross-backend equivalence or divergence findings into `compatibility.db`.
func (s *Store) RecordDifferentialEvidence(backendA, backendB registry.Backend, api string, apiLevel int, equivalent bool, divergenceSummary string) error {
	pFlag := "0"
	if equivalent {
		pFlag = "1"
	}
	if apiLevel <= 0 {
		apiLevel = 35
	}
	pair := fmt.Sprintf("%s_vs_%s", backendA, backendB)
	_, err := s.exec("record_evidence", "differential", api, pair, pair, strconv.Itoa(apiLevel), pFlag, divergenceSummary)
	return err
}

// UpsertQuirk persists an observed hardware/OEM/foldable/tablet quirk into `compatibility.db`.
func (s *Store) UpsertQuirk(q DeviceQuirk) error {
	_, err := s.exec("upsert_quirk", q.ProfileID, q.FormFactor, q.API, q.QuirkSummary, q.Severity)
	return err
}

// QueryHeatmap returns all tracked contracts, API 26-37 matrix rollup, and device quirks.
func (s *Store) QueryHeatmap() (*CompatibilityHeatmap, error) {
	out, err := s.exec("heatmap")
	if err != nil {
		return nil, err
	}
	var res struct {
		Status    string             `json:"status"`
		Contracts []ContractRecord   `json:"contracts"`
		APILevels []APILevelCoverage `json:"api_levels"`
		Quirks    []DeviceQuirk      `json:"quirks"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("decode heatmap json: %w", err)
	}
	return &CompatibilityHeatmap{
		DBPath:    s.dbPath,
		Contracts: res.Contracts,
		APILevels: res.APILevels,
		Quirks:    res.Quirks,
	}, nil
}

func (s *Store) exec(args ...string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cmdArgs := append([]string{"--enable-native-access=ALL-UNNAMED", "-cp", s.classpath, "nanoandroid.compat.CompatibilityDbTool", s.dbPath}, args...)
	cmd := exec.Command("java", cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("CompatibilityDbTool %v failed: %w (stderr=%s)", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return bytes.TrimSpace(stdout.Bytes()), nil
}
