package reporting

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nano_android/engine/apk/security"
	"nano_android/engine/compatibility/registry"
	"nano_android/engine/profiler/crash"
	"nano_android/engine/profiler/jvm"
	"nano_android/engine/testing/suite"
	"nano_android/engine/ui/visual"
)

// TestSummaryMetrics summarizes executed tests cleanly.
type TestSummaryMetrics struct {
	Total      int   `json:"total"`
	Passed     int   `json:"passed"`
	Failed     int   `json:"failed"`
	Skipped    int   `json:"skipped"`
	Flaky      int   `json:"flaky"`
	DurationMs int64 `json:"duration_ms"`
}

// CompatibilitySummaryMetrics summarizes API coverage & routing confidence.
type CompatibilitySummaryMetrics struct {
	PrimaryBackend  registry.Backend `json:"primary_backend"`
	ConfidenceScore float64          `json:"confidence_score"`
	LimitationCount int              `json:"limitation_count"`
	Limitations     []string         `json:"limitations,omitempty"`
}

// SecuritySummaryMetrics captures OWASP MASVS & policy compliance counts.
type SecuritySummaryMetrics struct {
	PassedGate bool               `json:"passed_gate"`
	RiskScore  int                `json:"risk_score"`
	Findings   []security.Finding `json:"findings,omitempty"`
}

// ProvenanceMetadata provides tamper-evident Ed25519 + SHA-256 report verification.
type ProvenanceMetadata struct {
	Actor             string `json:"actor"`
	GitBranch         string `json:"git_branch,omitempty"`
	GitCommit         string `json:"git_commit,omitempty"`
	AuditChainValid   bool   `json:"audit_chain_valid"`
	ReportSHA256      string `json:"report_sha256"`
	SignatureEd25519  string `json:"signature_ed25519,omitempty"`
	PublicKeyEd25519  string `json:"public_key_ed25519,omitempty"`
}

// UnifiedReport implements all 22 required Phase 6 verifiable reporting fields + integrity signatures (§10.4).
type UnifiedReport struct {
	ReportID               string                      `json:"report_id"`                // 1
	SchemaVersion          string                      `json:"schema_version"`           // 2
	GeneratedUTC           string                      `json:"generated_utc"`            // 3
	ProjectName            string                      `json:"project_name"`             // 4
	PackageName            string                      `json:"package_name"`             // 5
	VersionCode            int64                       `json:"version_code"`             // 6
	VersionName            string                      `json:"version_name"`             // 7
	ArtifactSHA256         string                      `json:"artifact_sha256"`          // 8
	ExecutionTier          registry.Backend            `json:"execution_tier"`           // 9
	ResultStatus           registry.ResultStatus       `json:"result_status"`            // 10
	EnvironmentDisclosure  string                      `json:"environment_disclosure"`   // 11
	ReproducedBehaviors    []string                    `json:"reproduced_behaviors"`     // 12
	NonReproducedBehaviors []string                    `json:"non_reproduced_behaviors"` // 13
	TestSummary            TestSummaryMetrics          `json:"test_summary"`             // 14
	TestCases              []suite.CaseOutcome         `json:"test_cases"`               // 15
	CompatibilitySummary   CompatibilitySummaryMetrics `json:"compatibility_summary"`    // 16
	SecuritySummary        SecuritySummaryMetrics      `json:"security_summary"`         // 17
	PerformanceSummary     *jvm.Snapshot               `json:"performance_summary,omitempty"` // 18
	VisualDiffSummary      *visual.CompareReport       `json:"visual_diff_summary,omitempty"` // 19
	CrashSummary           []crash.CrashReport         `json:"crash_summary,omitempty"`       // 20
	ArtifactsManifest      []string                    `json:"artifacts_manifest"`            // 21
	ProvenanceAndAudit     ProvenanceMetadata          `json:"provenance_and_audit"`          // 22
}

// CanonicalPayload returns deterministic JSON excluding the mutable hash/signature inside ProvenanceAndAudit.
func (r UnifiedReport) CanonicalPayload() []byte {
	cp := r
	cp.ProvenanceAndAudit.ReportSHA256 = ""
	cp.ProvenanceAndAudit.SignatureEd25519 = ""
	cp.ProvenanceAndAudit.PublicKeyEd25519 = ""
	b, _ := json.Marshal(cp)
	return b
}

// Sign computes SHA-256 over the canonical report payload and optionally signs it with an Ed25519 private key.
func (r *UnifiedReport) Sign(privKeyHex string) error {
	if r.ReportID == "" {
		r.ReportID = fmt.Sprintf("rep_%d", time.Now().UTC().UnixNano())
	}
	if r.SchemaVersion == "" {
		r.SchemaVersion = "nano-report/v1.0"
	}
	if r.GeneratedUTC == "" {
		r.GeneratedUTC = time.Now().UTC().Format(time.RFC3339)
	}
	payload := r.CanonicalPayload()
	sum := sha256.Sum256(payload)
	r.ProvenanceAndAudit.ReportSHA256 = hex.EncodeToString(sum[:])

	if strings.TrimSpace(privKeyHex) != "" {
		privBytes, err := hex.DecodeString(strings.TrimSpace(privKeyHex))
		if err != nil || len(privBytes) != ed25519.PrivateKeySize {
			return fmt.Errorf("invalid Ed25519 private key hex")
		}
		priv := ed25519.PrivateKey(privBytes)
		pub := priv.Public().(ed25519.PublicKey)
		r.ProvenanceAndAudit.PublicKeyEd25519 = hex.EncodeToString(pub)
		r.ProvenanceAndAudit.SignatureEd25519 = hex.EncodeToString(ed25519.Sign(priv, payload))
	}
	return nil
}

// Verify checks both the report SHA-256 hash and, if signed, the Ed25519 signature.
func Verify(r UnifiedReport) error {
	payload := r.CanonicalPayload()
	sum := sha256.Sum256(payload)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), r.ProvenanceAndAudit.ReportSHA256) {
		return fmt.Errorf("report SHA-256 integrity mismatch")
	}
	if r.ProvenanceAndAudit.SignatureEd25519 != "" {
		pubBytes, err := hex.DecodeString(r.ProvenanceAndAudit.PublicKeyEd25519)
		if err != nil || len(pubBytes) != ed25519.PublicKeySize {
			return fmt.Errorf("invalid public key on report")
		}
		sigBytes, err := hex.DecodeString(r.ProvenanceAndAudit.SignatureEd25519)
		if err != nil || len(sigBytes) != ed25519.SignatureSize {
			return fmt.Errorf("invalid signature bytes on report")
		}
		if !ed25519.Verify(ed25519.PublicKey(pubBytes), payload, sigBytes) {
			return fmt.Errorf("Ed25519 report signature verification failed")
		}
	}
	return nil
}

// ExportJSON renders canonical indented JSON.
func (r *UnifiedReport) ExportJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// ExportHTML renders a standalone responsive HTML engineering report.
func (r *UnifiedReport) ExportHTML() ([]byte, error) {
	var rows strings.Builder
	for _, tc := range r.TestCases {
		fmt.Fprintf(&rows, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%dms</td></tr>\n",
			html.EscapeString(tc.Name), html.EscapeString(tc.Status), html.EscapeString(tc.BackendUsed), tc.DurationMs)
	}
	var secRows strings.Builder
	for _, f := range r.SecuritySummary.Findings {
		fmt.Fprintf(&secRows, "<tr><td>%s</td><td>%s</td><td>%s</td></tr>\n",
			html.EscapeString(f.RuleID), html.EscapeString(string(f.Severity)), html.EscapeString(f.Title))
	}

	page := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en"><head><meta charset="UTF-8"><title>Nano-Android Report: %s</title>
<style>body{font-family:system-ui,sans-serif;background:#0f172a;color:#e2e8f0;padding:24px;}
.card{background:#1e293b;border-radius:8px;padding:16px;margin-bottom:16px;}
table{width:100%%;border-collapse:collapse;}th,td{padding:8px;border-bottom:1px solid #334155;text-align:left;}</style></head>
<body>
<h1>Nano-Android Engineering Report — %s</h1>
<div class="card">
  <strong>Status:</strong> %s &middot; <strong>Tier:</strong> %s &middot; <strong>Version:</strong> %s (%d)<br>
  <strong>Disclosure:</strong> %s<br>
  <strong>SHA-256:</strong> <code>%s</code>
</div>
<div class="card">
  <h2>Test Summary (%d/%d passed, %d flaky, %dms)</h2>
  <table><tr><th>Case</th><th>Status</th><th>Backend</th><th>Duration</th></tr>%s</table>
</div>
<div class="card">
  <h2>Security Summary (Passed=%v, Findings=%d)</h2>
  <table><tr><th>Rule</th><th>Severity</th><th>Title</th></tr>%s</table>
</div>
</body></html>`,
		html.EscapeString(r.PackageName), html.EscapeString(r.PackageName),
		html.EscapeString(string(r.ResultStatus)), html.EscapeString(string(r.ExecutionTier)),
		html.EscapeString(r.VersionName), r.VersionCode,
		html.EscapeString(r.EnvironmentDisclosure),
		html.EscapeString(r.ProvenanceAndAudit.ReportSHA256),
		r.TestSummary.Passed, r.TestSummary.Total, r.TestSummary.Flaky, r.TestSummary.DurationMs, rows.String(),
		r.SecuritySummary.PassedGate, len(r.SecuritySummary.Findings), secRows.String(),
	)
	return []byte(page), nil
}

// ExportJUnitXML converts TestCases into standard JUnit XML cleanly.
func (r *UnifiedReport) ExportJUnitXML() ([]byte, error) {
	type testcase struct {
		XMLName xml.Name `xml:"testcase"`
		Name    string   `xml:"name,attr"`
		Time    string   `xml:"time,attr"`
		Failure string   `xml:"failure,omitempty"`
	}
	type testsuite struct {
		XMLName  xml.Name   `xml:"testsuite"`
		Name     string     `xml:"name,attr"`
		Tests    int        `xml:"tests,attr"`
		Failures int        `xml:"failures,attr"`
		Cases    []testcase `xml:"testcase"`
	}
	ts := testsuite{
		Name:     r.PackageName,
		Tests:    r.TestSummary.Total,
		Failures: r.TestSummary.Failed,
	}
	for _, c := range r.TestCases {
		tc := testcase{Name: c.Name, Time: fmt.Sprintf("%.3f", float64(c.DurationMs)/1000.0)}
		if c.FailedCount > 0 || c.Status == string(registry.StatusFail) {
			tc.Failure = c.Error
			if tc.Failure == "" {
				tc.Failure = c.Status
			}
		}
		ts.Cases = append(ts.Cases, tc)
	}
	b, err := xml.MarshalIndent(ts, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), b...), nil
}

// ExportSARIF emits a valid SARIF v2.1.0 security document.
func (r *UnifiedReport) ExportSARIF() ([]byte, error) {
	var results []map[string]any
	for _, f := range r.SecuritySummary.Findings {
		results = append(results, map[string]any{
			"ruleId":  f.RuleID,
			"level":   strings.ToLower(string(f.Severity)),
			"message": map[string]string{"text": f.Title + ": " + f.Description},
		})
	}
	doc := map[string]any{
		"$schema": "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		"version": "2.1.0",
		"runs": []map[string]any{
			{
				"tool": map[string]any{
					"driver": map[string]any{
						"name":    "Nano-Android OWASP MASVS Analyzer",
						"version": r.SchemaVersion,
					},
				},
				"results": results,
			},
		},
	}
	return json.MarshalIndent(doc, "", "  ")
}

// ExportPDF synthesizes a valid pure-Go PDF 1.4 binary with cross-reference offsets and executive summary lines.
func (r *UnifiedReport) ExportPDF() ([]byte, error) {
	lines := []string{
		fmt.Sprintf("Nano-Android Unified Engineering Report (%s)", r.SchemaVersion),
		fmt.Sprintf("Report ID: %s | Package: %s", r.ReportID, r.PackageName),
		fmt.Sprintf("Version: %s (%d) | Tier: %s | Status: %s", r.VersionName, r.VersionCode, r.ExecutionTier, r.ResultStatus),
		fmt.Sprintf("Disclosure: %s", truncatePDF(r.EnvironmentDisclosure, 85)),
		fmt.Sprintf("Tests: %d total, %d passed, %d failed, %d flaky (%d ms)", r.TestSummary.Total, r.TestSummary.Passed, r.TestSummary.Failed, r.TestSummary.Flaky, r.TestSummary.DurationMs),
		fmt.Sprintf("Compatibility: %s (confidence %.0f%%)", r.CompatibilitySummary.PrimaryBackend, r.CompatibilitySummary.ConfidenceScore*100),
		fmt.Sprintf("Security Gate Passed: %v | MASVS Findings: %d", r.SecuritySummary.PassedGate, len(r.SecuritySummary.Findings)),
		fmt.Sprintf("Report SHA-256: %s", r.ProvenanceAndAudit.ReportSHA256),
		fmt.Sprintf("Ed25519 Signed: %v", r.ProvenanceAndAudit.SignatureEd25519 != ""),
	}
	var stream strings.Builder
	stream.WriteString("BT\n/F1 11 Tf\n50 750 Td\n15 TL\n")
	for _, ln := range lines {
		esc := strings.ReplaceAll(strings.ReplaceAll(ln, "\\", "\\\\"), "(", "\\(")
		esc = strings.ReplaceAll(esc, ")", "\\)")
		fmt.Fprintf(&stream, "(%s) Tj T*\n", esc)
	}
	stream.WriteString("ET\n")
	contentStr := stream.String()

	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	offsets := make([]int, 6)

	writeObj := func(idx int, body string) {
		offsets[idx] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", idx, body)
	}
	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	writeObj(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>")
	writeObj(4, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(contentStr), contentStr))
	writeObj(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

	xrefPos := pdf.Len()
	pdf.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefPos)
	return pdf.Bytes(), nil
}

// ExportForensicBundleZIP writes report.json, report.html, report.pdf, junit.xml, security.sarif.json, and CHECKSUMS.txt into dstPath.
func (r *UnifiedReport) ExportForensicBundleZIP(dstPath string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0750); err != nil {
		return "", err
	}
	jsonB, _ := r.ExportJSON()
	htmlB, _ := r.ExportHTML()
	pdfB, _ := r.ExportPDF()
	xmlB, _ := r.ExportJUnitXML()
	sarifB, _ := r.ExportSARIF()

	files := []struct {
		name string
		data []byte
	}{
		{"report.json", jsonB},
		{"report.html", htmlB},
		{"report.pdf", pdfB},
		{"junit.xml", xmlB},
		{"security.sarif.json", sarifB},
	}

	var checksums strings.Builder
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	for _, f := range files {
		w, err := zw.Create(f.name)
		if err != nil {
			return "", err
		}
		if _, err := w.Write(f.data); err != nil {
			return "", err
		}
		sum := sha256.Sum256(f.data)
		fmt.Fprintf(&checksums, "%x  %s\n", sum, f.name)
	}
	cw, err := zw.Create("CHECKSUMS.txt")
	if err != nil {
		return "", err
	}
	_, _ = cw.Write([]byte(checksums.String()))
	if err := zw.Close(); err != nil {
		return "", err
	}

	tmp := dstPath + ".tmp"
	if err := os.WriteFile(tmp, zipBuf.Bytes(), 0600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dstPath); err != nil {
		return "", err
	}
	return dstPath, nil
}

func truncatePDF(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
