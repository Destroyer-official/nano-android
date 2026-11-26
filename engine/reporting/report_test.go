package reporting

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"nano_android/engine/apk/security"
	"nano_android/engine/compatibility/registry"
	"nano_android/engine/testing/suite"
)

func TestUnifiedReportSigningAllExportsAndAnalyticsDeltas(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privHex := hex.EncodeToString(priv)

	rep := UnifiedReport{
		ProjectName:            "SampleAndroidApp",
		PackageName:            "com.example.shop",
		VersionCode:            10,
		VersionName:            "1.0.0",
		ArtifactSHA256:         "deadbeef",
		ExecutionTier:          registry.BackendNanoJVM,
		ResultStatus:           registry.StatusNanoSimulationPass,
		EnvironmentDisclosure:  "Nano-Android JVM headless execution (OpenJDK 25)",
		ReproducedBehaviors:    []string{"Activity lifecycle", "SQLite queries"},
		NonReproducedBehaviors: []string{"Hardware MediaCodec"},
		TestSummary:            TestSummaryMetrics{Total: 2, Passed: 2, DurationMs: 120},
		TestCases: []suite.CaseOutcome{
			{Name: "LaunchTest", Status: "NANO_SIMULATION_PASS", BackendUsed: "nano_jvm", DurationMs: 50},
			{Name: "CartTest", Status: "NANO_SIMULATION_PASS", BackendUsed: "nano_jvm", DurationMs: 70},
		},
		CompatibilitySummary: CompatibilitySummaryMetrics{PrimaryBackend: registry.BackendNanoJVM, ConfidenceScore: 0.92},
		SecuritySummary: SecuritySummaryMetrics{
			PassedGate: true,
			Findings:   []security.Finding{{RuleID: "MASVS-STORAGE-2", Severity: security.SeverityMedium, Title: "Backup enabled"}},
		},
		ArtifactsManifest: []string{"junit.xml", "report.html"},
		ProvenanceAndAudit: ProvenanceMetadata{Actor: "ci_pipeline", AuditChainValid: true},
	}

	if err := rep.Sign(privHex); err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	if err := Verify(rep); err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	// Tamper -> must fail Verify
	tampered := rep
	tampered.VersionName = "1.0.1"
	if err := Verify(tampered); err == nil {
		t.Fatalf("expected tampered report to fail signature verification")
	}

	// Validate all export formats
	pdfB, _ := rep.ExportPDF()
	if !strings.HasPrefix(string(pdfB), "%PDF-1.4") || !strings.Contains(string(pdfB), "startxref") {
		t.Fatalf("expected valid PDF 1.4 binary bytes")
	}
	htmlB, _ := rep.ExportHTML()
	if !strings.Contains(string(htmlB), "com.example.shop") {
		t.Fatalf("expected HTML report content")
	}
	xmlB, _ := rep.ExportJUnitXML()
	if !strings.Contains(string(xmlB), `<testsuite`) {
		t.Fatalf("expected JUnit XML")
	}
	sarifB, _ := rep.ExportSARIF()
	if !strings.Contains(string(sarifB), `"version": "2.1.0"`) {
		t.Fatalf("expected SARIF v2.1.0")
	}

	zipPath := filepath.Join(t.TempDir(), "forensic_bundle.zip")
	if _, err := rep.ExportForensicBundleZIP(zipPath); err != nil {
		t.Fatalf("ExportForensicBundleZIP: %v", err)
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil || len(zr.File) != 6 {
		t.Fatalf("expected 6 files in forensic zip, got len=%d err=%v", len(zr.File), err)
	}
	_ = zr.Close()

	// Analytics + release-to-release deltas
	as, err := OpenAnalyticsStore(filepath.Join(t.TempDir(), "analytics.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = as.RecordRun(rep)
	repV2 := rep
	repV2.VersionName = "2.0.0"
	repV2.VersionCode = 20
	repV2.TestSummary = TestSummaryMetrics{Total: 2, Passed: 1, Failed: 1, DurationMs: 220}
	repV2.TestCases[1].FailedCount = 1
	repV2.TestCases[1].Status = "FAIL"
	_ = as.RecordRun(repV2)

	trends := as.ComputeTrends()
	if trends.TotalRuns != 2 {
		t.Fatalf("expected 2 analytics runs, got %d", trends.TotalRuns)
	}
	delta, err := as.CompareReleases("1.0.0", "2.0.0")
	if err != nil || len(delta.NewlyFailingCases) != 1 || delta.NewlyFailingCases[0] != "CartTest" {
		t.Fatalf("unexpected release delta: %+v err=%v", delta, err)
	}
}
