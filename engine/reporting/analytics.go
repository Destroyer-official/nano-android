package reporting

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// AnalyticsRunEntry records summarized historical telemetry for one verified report run.
type AnalyticsRunEntry struct {
	ReportID       string  `json:"report_id"`
	GeneratedUTC   string  `json:"generated_utc"`
	PackageName    string  `json:"package_name"`
	VersionName    string  `json:"version_name"`
	VersionCode    int64   `json:"version_code"`
	Total          int     `json:"total"`
	Passed         int     `json:"passed"`
	Failed         int     `json:"failed"`
	Flaky          int     `json:"flaky"`
	DurationMs     int64   `json:"duration_ms"`
	CompatConf     float64 `json:"compat_confidence"`
	SecurityIssues int     `json:"security_issues"`
	FailedCaseIDs  []string `json:"failed_case_ids,omitempty"`
}

// AnalyticsTrends reports historical test/flakiness/perf trends and module coverage.
type AnalyticsTrends struct {
	TotalRuns         int                         `json:"total_runs"`
	AveragePassRate   float64                     `json:"average_pass_rate"`
	AverageFlakeRate  float64                     `json:"average_flake_rate"`
	AverageDurationMs float64                     `json:"average_duration_ms"`
	ByPackage         map[string]PackageCoverage  `json:"by_package"`
	RecentRuns        []AnalyticsRunEntry         `json:"recent_runs"`
}

// PackageCoverage tracks per-module / per-package reliability & compatibility confidence.
type PackageCoverage struct {
	PackageName   string  `json:"package_name"`
	Runs          int     `json:"runs"`
	PassRate      float64 `json:"pass_rate"`
	AvgCompatConf float64 `json:"avg_compat_confidence"`
}

// ReleaseDelta compares two app versions across pass rate, duration, and newly failing/fixed tests.
type ReleaseDelta struct {
	BaseVersion       string   `json:"base_version"`
	TargetVersion     string   `json:"target_version"`
	PassRateDelta     float64  `json:"pass_rate_delta"`
	DurationDeltaMs   int64    `json:"duration_delta_ms"`
	CompatConfDelta   float64  `json:"compat_conf_delta"`
	SecurityDelta     int      `json:"security_issues_delta"`
	NewlyFailingCases []string `json:"newly_failing_cases,omitempty"`
	FixedCases        []string `json:"fixed_cases,omitempty"`
}

// AnalyticsStore persists historical run entries and computes trends + release-to-release deltas.
type AnalyticsStore struct {
	mu      sync.Mutex
	path    string
	Entries []AnalyticsRunEntry `json:"entries"`
}

// OpenAnalyticsStore loads or creates a JSONL/JSON analytics store.
func OpenAnalyticsStore(path string) (*AnalyticsStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("analytics path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, err
	}
	as := &AnalyticsStore{path: path}
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &as.Entries)
	}
	return as, nil
}

// RecordRun extracts key analytics metrics from a UnifiedReport and persists them atomically.
func (as *AnalyticsStore) RecordRun(rep UnifiedReport) error {
	var failedIDs []string
	for _, tc := range rep.TestCases {
		if tc.FailedCount > 0 || tc.Status == "FAIL" {
			failedIDs = append(failedIDs, tc.Name)
		}
	}
	entry := AnalyticsRunEntry{
		ReportID:       rep.ReportID,
		GeneratedUTC:   rep.GeneratedUTC,
		PackageName:    rep.PackageName,
		VersionName:    rep.VersionName,
		VersionCode:    rep.VersionCode,
		Total:          rep.TestSummary.Total,
		Passed:         rep.TestSummary.Passed,
		Failed:         rep.TestSummary.Failed,
		Flaky:          rep.TestSummary.Flaky,
		DurationMs:     rep.TestSummary.DurationMs,
		CompatConf:     rep.CompatibilitySummary.ConfidenceScore,
		SecurityIssues: len(rep.SecuritySummary.Findings),
		FailedCaseIDs:  failedIDs,
	}
	as.mu.Lock()
	defer as.mu.Unlock()
	as.Entries = append(as.Entries, entry)
	b, err := json.MarshalIndent(as.Entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := as.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, as.path)
}

// ComputeTrends calculates longitudinal pass/flake/performance trends and per-package summaries.
func (as *AnalyticsStore) ComputeTrends() AnalyticsTrends {
	as.mu.Lock()
	defer as.mu.Unlock()
	tr := AnalyticsTrends{
		TotalRuns:  len(as.Entries),
		ByPackage:  make(map[string]PackageCoverage),
		RecentRuns: append([]AnalyticsRunEntry(nil), as.Entries...),
	}
	if len(tr.RecentRuns) > 20 {
		tr.RecentRuns = tr.RecentRuns[len(tr.RecentRuns)-20:]
	}
	var totTests, totPassed, totFlaky int
	var totDur int64
	pkgAgg := make(map[string]struct {
		runs, tests, passed int
		compatSum           float64
	})

	for _, e := range as.Entries {
		totTests += e.Total
		totPassed += e.Passed
		totFlaky += e.Flaky
		totDur += e.DurationMs
		cur := pkgAgg[e.PackageName]
		cur.runs++
		cur.tests += e.Total
		cur.passed += e.Passed
		cur.compatSum += e.CompatConf
		pkgAgg[e.PackageName] = cur
	}
	if totTests > 0 {
		tr.AveragePassRate = float64(totPassed) / float64(totTests)
		tr.AverageFlakeRate = float64(totFlaky) / float64(totTests)
	}
	if len(as.Entries) > 0 {
		tr.AverageDurationMs = float64(totDur) / float64(len(as.Entries))
	}
	for pkg, agg := range pkgAgg {
		pr := 0.0
		if agg.tests > 0 {
			pr = float64(agg.passed) / float64(agg.tests)
		}
		tr.ByPackage[pkg] = PackageCoverage{
			PackageName:   pkg,
			Runs:          agg.runs,
			PassRate:      pr,
			AvgCompatConf: agg.compatSum / float64(agg.runs),
		}
	}
	return tr
}

// CompareReleases calculates deltas between the latest runs for baseVersion and targetVersion.
func (as *AnalyticsStore) CompareReleases(baseVersion, targetVersion string) (ReleaseDelta, error) {
	as.mu.Lock()
	defer as.mu.Unlock()
	var baseEntry, targetEntry *AnalyticsRunEntry
	for i := len(as.Entries) - 1; i >= 0; i-- {
		e := as.Entries[i]
		if baseEntry == nil && e.VersionName == baseVersion {
			cp := e
			baseEntry = &cp
		}
		if targetEntry == nil && e.VersionName == targetVersion {
			cp := e
			targetEntry = &cp
		}
	}
	if baseEntry == nil || targetEntry == nil {
		return ReleaseDelta{}, fmt.Errorf("missing historical analytics for base=%q or target=%q", baseVersion, targetVersion)
	}
	rate := func(e *AnalyticsRunEntry) float64 {
		if e.Total == 0 {
			return 0
		}
		return float64(e.Passed) / float64(e.Total)
	}
	baseFail := mapSet(baseEntry.FailedCaseIDs)
	targFail := mapSet(targetEntry.FailedCaseIDs)
	var newlyFailing, fixed []string
	for id := range targFail {
		if !baseFail[id] {
			newlyFailing = append(newlyFailing, id)
		}
	}
	for id := range baseFail {
		if !targFail[id] {
			fixed = append(fixed, id)
		}
	}
	sort.Strings(newlyFailing)
	sort.Strings(fixed)

	return ReleaseDelta{
		BaseVersion:       baseVersion,
		TargetVersion:     targetVersion,
		PassRateDelta:     rate(targetEntry) - rate(baseEntry),
		DurationDeltaMs:   targetEntry.DurationMs - baseEntry.DurationMs,
		CompatConfDelta:   targetEntry.CompatConf - baseEntry.CompatConf,
		SecurityDelta:     targetEntry.SecurityIssues - baseEntry.SecurityIssues,
		NewlyFailingCases: newlyFailing,
		FixedCases:        fixed,
	}, nil
}

func mapSet(in []string) map[string]bool {
	out := make(map[string]bool, len(in))
	for _, s := range in {
		out[s] = true
	}
	return out
}
