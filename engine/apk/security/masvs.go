package security

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"nano_android/engine/apk/ingest"
)

// FindingSeverity follows OWASP MASVS / SARIF levels.
type FindingSeverity string

const (
	SeverityCritical FindingSeverity = "CRITICAL"
	SeverityHigh     FindingSeverity = "HIGH"
	SeverityMedium   FindingSeverity = "MEDIUM"
	SeverityLow      FindingSeverity = "LOW"
)

// Finding represents one static OWASP MASVS v2.1.0 security issue.
type Finding struct {
	RuleID         string          `json:"rule_id"`
	MASVSCategory  string          `json:"masvs_category"`
	Severity       FindingSeverity `json:"severity"`
	Title          string          `json:"title"`
	Component      string          `json:"component,omitempty"`
	Description    string          `json:"description"`
	Remediation    string          `json:"remediation"`
}

// AuditReport summarizes OWASP MASVS v2.1.0 checks for an APK.
type AuditReport struct {
	PackageName string    `json:"package_name"`
	SHA256      string    `json:"sha256"`
	Findings    []Finding `json:"findings"`
	RiskScore   int       `json:"risk_score"`
	PassedGate  bool      `json:"passed_gate"`
}

var (
	reHardcodedSecret = regexp.MustCompile(`(?i)(AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z\-_]{35}|-----BEGIN [A-Z ]*PRIVATE KEY-----|sk_live_[0-9a-zA-Z]{16,})`)
	reCleartextURL    = regexp.MustCompile(`http://[a-zA-Z0-9._-]+`)
)

// AuditAPK executes OWASP MASVS v2.1.0 static checks against an analyzed APK/AAB report.
func AuditAPK(rep *ingest.IntelligenceReport) AuditReport {
	out := AuditReport{PassedGate: true}
	if rep == nil {
		return out
	}
	out.SHA256 = rep.SHA256

	if rep.Manifest != nil {
		out.PackageName = rep.Manifest.Package
		if rep.Manifest.Debuggable {
			out.Findings = append(out.Findings, Finding{
				RuleID:        "MASVS-RESILIENCE-DEBUGGABLE",
				MASVSCategory: "MASVS-RESILIENCE-1",
				Severity:      SeverityCritical,
				Title:         "Application is debuggable (android:debuggable=true)",
				Component:     "AndroidManifest.xml",
				Description:   "Production APKs with android:debuggable=true allow JDWP memory inspection and code tampering.",
				Remediation:   "Set android:debuggable=false for release builds.",
			})
		}
		if rep.Manifest.UsesCleartextTraffic != nil && *rep.Manifest.UsesCleartextTraffic {
			out.Findings = append(out.Findings, Finding{
				RuleID:        "MASVS-NETWORK-CLEARTEXT",
				MASVSCategory: "MASVS-NETWORK-1",
				Severity:      SeverityHigh,
				Title:         "Cleartext HTTP traffic globally permitted",
				Component:     "AndroidManifest.xml",
				Description:   "android:usesCleartextTraffic=true permits unencrypted HTTP connections vulnerable to MitM interception.",
				Remediation:   "Set android:usesCleartextTraffic=false and configure a strict Network Security Config.",
			})
		}
		if rep.Manifest.AllowBackup != nil && *rep.Manifest.AllowBackup {
			out.Findings = append(out.Findings, Finding{
				RuleID:        "MASVS-STORAGE-BACKUP",
				MASVSCategory: "MASVS-STORAGE-2",
				Severity:      SeverityMedium,
				Title:         "ADB backup enabled (android:allowBackup=true)",
				Component:     "AndroidManifest.xml",
				Description:   "Application data may be extracted via backups unless restricted.",
				Remediation:   "Disable android:allowBackup or supply explicit fullBackupContent exclusions.",
			})
		}
		checkExported := func(kind, name string, exported bool, perm string) {
			if exported && perm == "" && kind != "activity" {
				out.Findings = append(out.Findings, Finding{
					RuleID:        "MASVS-PLATFORM-EXPORTED-" + strings.ToUpper(kind),
					MASVSCategory: "MASVS-PLATFORM-1",
					Severity:      SeverityHigh,
					Title:         fmt.Sprintf("Unprotected exported %s component", kind),
					Component:     name,
					Description:   fmt.Sprintf("%s %q is exported without a restricting android:permission attribute.", kind, name),
					Remediation:   "Set android:exported=\"false\" or protect with a signature-level android:permission.",
				})
			}
		}
		for _, s := range rep.Manifest.Services {
			checkExported("service", s.Name, s.IsEffectivelyExported(), s.Permission)
		}
		for _, r := range rep.Manifest.Receivers {
			checkExported("receiver", r.Name, r.IsEffectivelyExported(), r.Permission)
		}
		for _, p := range rep.Manifest.Providers {
			checkExported("provider", p.Name, p.IsEffectivelyExported(), p.Permission)
		}
	}

	if !rep.Signing.IsSigned {
		out.Findings = append(out.Findings, Finding{
			RuleID:        "MASVS-CODE-UNSIGNED",
			MASVSCategory: "MASVS-CODE-1",
			Severity:      SeverityHigh,
			Title:         "APK lacks v1/v2/v3/v4 signature",
			Component:     rep.Path,
			Description:   "Unsigned APK packages fail OS signature verification and cannot verify signer identity.",
			Remediation:   "Sign release APKs with v2/v3 scheme via apksigner.",
		})
	}

	for _, lib := range rep.NativeLibs {
		if strings.Contains(lib.ABI, "arm64") && !lib.Is16KBAligned {
			out.Findings = append(out.Findings, Finding{
				RuleID:        "ANDROID-17-16KB-ELF-ALIGN",
				MASVSCategory: "MASVS-CODE-4",
				Severity:      SeverityHigh,
				Title:         "Native .so library missing 16 KB PT_LOAD ELF alignment",
				Component:     lib.Path,
				Description:   fmt.Sprintf("%s has PT_LOAD p_align=0x%x (< 0x4000), incompatible with Android 15+/17 16 KB page devices.", lib.Path, lib.MaxAlign),
				Remediation:   "Rebuild native C/C++ libraries with NDK r27+ / `-Wl,-z,max-page-size=16384`.",
			})
		}
	}

	if rep.Resources != nil {
		for _, s := range rep.Resources.Strings {
			if reHardcodedSecret.MatchString(s) {
				out.Findings = append(out.Findings, Finding{
					RuleID:        "MASVS-STORAGE-HARDCODED-SECRET",
					MASVSCategory: "MASVS-STORAGE-1",
					Severity:      SeverityCritical,
					Title:         "Potential credential or private key embedded in resources",
					Component:     "resources.arsc",
					Description:   "Matched embedded cloud API key / private key pattern in resource strings.",
					Remediation:   "Remove static secrets from bundled assets/resources; obtain tokens dynamically.",
				})
				break
			}
			if reCleartextURL.MatchString(s) && !strings.Contains(s, "http://schemas.android.com") && !strings.Contains(s, "http://127.0.0.1") && !strings.Contains(s, "http://localhost") {
				out.Findings = append(out.Findings, Finding{
					RuleID:        "MASVS-NETWORK-HARDCODED-HTTP",
					MASVSCategory: "MASVS-NETWORK-1",
					Severity:      SeverityMedium,
					Title:         "Non-loopback cleartext http:// URL embedded in resources",
					Component:     "resources.arsc",
					Description:   "Cleartext HTTP endpoint reference found in packaged resource strings.",
					Remediation:   "Migrate production endpoints to HTTPS with TLS certificate verification.",
				})
				break
			}
		}
	}

	if rep.DEX != nil {
		for _, m := range rep.DEX.MethodReferences {
			if strings.Contains(m.Owner, "WebView") && m.Name == "addJavascriptInterface" {
				out.Findings = append(out.Findings, Finding{
					RuleID:        "MASVS-PLATFORM-WEBVIEW-JSBRIDGE",
					MASVSCategory: "MASVS-PLATFORM-2",
					Severity:      SeverityMedium,
					Title:         "WebView.addJavascriptInterface attack surface detected",
					Component:     m.Owner,
					Description:   "Exposed JavaScript bridge requires strict origin validation and @JavascriptInterface scoping.",
					Remediation:   "Restrict loaded origins to trusted HTTPS domains and minimize exposed native methods.",
				})
				break
			}
		}
	}

	for _, f := range out.Findings {
		switch f.Severity {
		case SeverityCritical:
			out.RiskScore += 30
			out.PassedGate = false
		case SeverityHigh:
			out.RiskScore += 15
			out.PassedGate = false
		case SeverityMedium:
			out.RiskScore += 5
		default:
			out.RiskScore += 1
		}
	}
	return out
}

// ExportSARIF serializes the OWASP MASVS audit report into SARIF v2.1.0 JSON.
func (r AuditReport) ExportSARIF() ([]byte, error) {
	rulesMap := map[string]map[string]any{}
	results := make([]map[string]any, 0, len(r.Findings))

	for _, f := range r.Findings {
		rulesMap[f.RuleID] = map[string]any{
			"id":               f.RuleID,
			"name":             f.MASVSCategory,
			"shortDescription": map[string]string{"text": f.Title},
			"help":             map[string]string{"text": f.Remediation},
		}
		level := "note"
		if f.Severity == SeverityCritical || f.Severity == SeverityHigh {
			level = "error"
		} else if f.Severity == SeverityMedium {
			level = "warning"
		}
		uri := f.Component
		if uri == "" {
			uri = "AndroidManifest.xml"
		}
		results = append(results, map[string]any{
			"ruleId":  f.RuleID,
			"level":   level,
			"message": map[string]string{"text": f.Description},
			"locations": []map[string]any{
				{"physicalLocation": map[string]any{"artifactLocation": map[string]string{"uri": uri}}},
			},
		})
	}

	rules := make([]map[string]any, 0, len(rulesMap))
	for _, ru := range rulesMap {
		rules = append(rules, ru)
	}

	sarif := map[string]any{
		"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
		"version": "2.1.0",
		"runs": []map[string]any{
			{
				"tool": map[string]any{
					"driver": map[string]any{
						"name":    "Nano-Android OWASP MASVS Scanner",
						"version": "2.1.0",
						"rules":   rules,
					},
				},
				"results": results,
			},
		},
	}
	return json.MarshalIndent(sarif, "", "  ")
}
