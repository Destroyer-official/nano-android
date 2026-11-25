package build

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropic/droidforge-mcp/internal/types"
	"gopkg.in/yaml.v3"
)

// PlayPolicyJustification mirrors the play-policy-justification.yaml specification.
type PlayPolicyJustification struct {
	SchemaVersion         int                         `yaml:"schema_version"`
	AppPackage            string                      `yaml:"app_package"`
	TargetSDK             int                         `yaml:"target_sdk"`
	RestrictedPermissions []RestrictedPermissionEntry `yaml:"restricted_permissions"`
}

// RestrictedPermissionEntry defines justification for one sensitive permission.
type RestrictedPermissionEntry struct {
	Permission                string `yaml:"permission"`
	ApprovedRole              string `yaml:"approved_role,omitempty"`
	InAppDisclosureScreenTest string `yaml:"in_app_disclosure_screen_test,omitempty"`
	Android17OTPFallback      bool   `yaml:"android17_otp_fallback,omitempty"`
	Notes                     string `yaml:"notes,omitempty"`
	Status                    string `yaml:"status,omitempty"` // e.g. "PROHIBITED", "APPROVED"
	FailCIIfPresent           bool   `yaml:"fail_ci_if_present,omitempty"`
}

// LoadPlayPolicyJustification attempts to find and parse play-policy-justification.yaml.
func LoadPlayPolicyJustification(searchPaths ...string) (*PlayPolicyJustification, error) {
	for _, p := range searchPaths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			data, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			var just PlayPolicyJustification
			if err := yaml.Unmarshal(data, &just); err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", p, err)
			}
			return &just, nil
		}
	}
	return nil, os.ErrNotExist
}

// executeAuditCompliance runs 16 KB ELF alignment, Google Play policy, and Android 17 compatibility audits.
func (b *GradleEngine) executeAuditCompliance(ctx context.Context, apkPath, profile string) (*types.ComplianceResult, error) {
	if _, err := os.Stat(apkPath); err != nil {
		return nil, fmt.Errorf("APK not found at %q: %w", apkPath, err)
	}

	zr, err := zip.OpenReader(apkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open APK zip: %w", err)
	}
	defer zr.Close()

	result := &types.ComplianceResult{
		Pass:                 true,
		ELF16KBAligned:       true,
		LargeScreenResizable: true,
		PlayPolicyFindings:   []types.ComplianceFinding{},
		Android17Findings:    []types.ComplianceFinding{},
	}

	// 1. Audit 16 KB ELF Alignment for native libraries in lib/arm64-v8a/
	hasARM64Native := false
	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "/")
		if strings.HasPrefix(name, "lib/arm64-v8a/") && strings.HasSuffix(name, ".so") {
			hasARM64Native = true
			soData, err := readZipEntry(f)
			if err != nil || len(soData) == 0 {
				continue
			}

			report, err := CheckELF16KBAlignment(bytes.NewReader(soData))
			if err != nil {
				result.ELF16KBAligned = false
				result.Android17Findings = append(result.Android17Findings, types.ComplianceFinding{
					Severity: "error",
					Rule:     "ELF_16KB_PAGE_ALIGNMENT",
					Message:  fmt.Sprintf("Failed to check ELF alignment of %s: %v", name, err),
				})
			} else if !report.Aligned {
				result.ELF16KBAligned = false
				for _, v := range report.Violations {
					result.Android17Findings = append(result.Android17Findings, types.ComplianceFinding{
						Severity: "error",
						Rule:     "ELF_16KB_PAGE_ALIGNMENT",
						Message:  fmt.Sprintf("%s: %s", name, v),
						Detail:   fmt.Sprintf("min_align=%d", report.MinAlign),
					})
				}
			}
		}
	}
	_ = hasARM64Native

	// 2. Read and parse manifest
	var manifestData []byte
	var dexEntries []*zip.File

	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "/")
		if strings.EqualFold(strings.Trim(name, "/\\"), "androidmanifest.xml") {
			data, err := readZipEntry(f)
			if err == nil && len(data) > 0 {
				manifestData = data
			}
		} else if strings.HasPrefix(name, "classes") && strings.HasSuffix(name, ".dex") {
			dexEntries = append(dexEntries, f)
		}
	}

	var details *ManifestDetails
	if len(manifestData) > 0 {
		details, _ = ParseManifest(manifestData)
	}
	if details == nil {
		details = &ManifestDetails{}
	}

	// Load Play policy justification
	apkDir := filepath.Dir(apkPath)
	justificationPaths := []string{
		profile, // profile argument may be path to justification file
		filepath.Join(apkDir, "play-policy-justification.yaml"),
		filepath.Join(b.gradleRoot, "play-policy-justification.yaml"),
		filepath.Join(b.gradleRoot, ".droidforge", "play-policy-justification.yaml"),
		"play-policy-justification.yaml",
		filepath.Join(".droidforge", "play-policy-justification.yaml"),
	}
	justification, _ := LoadPlayPolicyJustification(justificationPaths...)

	// Map justified permissions
	justifiedMap := make(map[string]RestrictedPermissionEntry)
	if justification != nil {
		for _, entry := range justification.RestrictedPermissions {
			justifiedMap[entry.Permission] = entry
		}
	}

	// 3. Play Policy Checks
	restrictedPerms := []string{
		"android.permission.READ_SMS",
		"android.permission.READ_CALL_LOG",
		"android.permission.BIND_ACCESSIBILITY_SERVICE",
	}

	for _, p := range details.Permissions {
		// Restricted permission check
		for _, rp := range restrictedPerms {
			if p == rp {
				entry, justified := justifiedMap[rp]
				if !justified {
					result.PlayPolicyFindings = append(result.PlayPolicyFindings, types.ComplianceFinding{
						Severity: "error",
						Rule:     "PLAY_POLICY_RESTRICTED_PERMISSION",
						Message:  fmt.Sprintf("Restricted permission %s present in manifest without approved entry in play-policy-justification.yaml", rp),
					})
				} else if entry.FailCIIfPresent || strings.ToUpper(entry.Status) == "PROHIBITED" {
					result.PlayPolicyFindings = append(result.PlayPolicyFindings, types.ComplianceFinding{
						Severity: "error",
						Rule:     "PLAY_POLICY_PROHIBITED_PERMISSION",
						Message:  fmt.Sprintf("Permission %s is marked PROHIBITED or fail_ci_if_present in play-policy-justification.yaml", rp),
						Detail:   entry.Notes,
					})
				}
			}
		}

		// Location rationale warning
		if p == "android.permission.ACCESS_FINE_LOCATION" {
			locEntry, ok := justifiedMap["android.permission.ACCESS_FINE_LOCATION"]
			if !ok || (locEntry.InAppDisclosureScreenTest == "" && locEntry.Notes == "") {
				result.PlayPolicyFindings = append(result.PlayPolicyFindings, types.ComplianceFinding{
					Severity: "warning",
					Rule:     "PLAY_POLICY_LOCATION_RATIONALE",
					Message:  "ACCESS_FINE_LOCATION declared without location-button rationale or in-app disclosure screen test in play-policy-justification.yaml",
				})
			}
		}
	}

	// 4. Android 17 (API 37) Findings
	if details.TargetSDK >= 37 {
		// 4a. Check large-screen resizability
		if details.ResizeableActivity != nil && !*details.ResizeableActivity && !details.IsGame {
			result.LargeScreenResizable = false
			result.Android17Findings = append(result.Android17Findings, types.ComplianceFinding{
				Severity: "error",
				Rule:     "ANDROID_17_LARGE_SCREEN_RESIZABLE",
				Message:  "Target SDK 37+ non-game app explicitly sets android:resizeableActivity=false, violating mandatory large-screen resizability",
			})
		}

		// 4b. Flag READ_SMS (3-hour OTP delay risk on Android 17+)
		for _, p := range details.Permissions {
			if p == "android.permission.READ_SMS" {
				result.Android17Findings = append(result.Android17Findings, types.ComplianceFinding{
					Severity: "warning",
					Rule:     "ANDROID_17_SMS_OTP_DELAY",
					Message:  "READ_SMS present on targetSdk >= 37: 3-hour OTP delay risk applies to non-default SMS apps under Android 17 rules",
				})
			}
		}

		// 4c. Check for ACCESS_LOCAL_NETWORK requirement
		hasLocalNetworkCode := detectLocalNetworkUsage(dexEntries)
		hasLocalNetworkPerm := false
		for _, p := range details.Permissions {
			if p == "android.permission.ACCESS_LOCAL_NETWORK" {
				hasLocalNetworkPerm = true
				break
			}
		}

		if hasLocalNetworkCode && !hasLocalNetworkPerm {
			result.Android17Findings = append(result.Android17Findings, types.ComplianceFinding{
				Severity: "error",
				Rule:     "ANDROID_17_LOCAL_NETWORK",
				Message:  "Local network socket usage detected in targetSdk >= 37 APK, but android.permission.ACCESS_LOCAL_NETWORK is missing",
			})
		}
	}

	// Final pass evaluation
	if !result.ELF16KBAligned || !result.LargeScreenResizable {
		result.Pass = false
	}
	for _, f := range result.PlayPolicyFindings {
		if f.Severity == "error" {
			result.Pass = false
			break
		}
	}
	for _, f := range result.Android17Findings {
		if f.Severity == "error" {
			result.Pass = false
			break
		}
	}

	return result, nil
}

// detectLocalNetworkUsage inspects DEX byte streams for local socket/network indicators.
func detectLocalNetworkUsage(dexFiles []*zip.File) bool {
	targets := [][]byte{
		[]byte("Ljava/net/Socket;"),
		[]byte("Ljava/net/ServerSocket;"),
		[]byte("192.168."),
		[]byte("127.0.0.1"),
		[]byte("localhost"),
		[]byte("10.0.0."),
	}

	for _, df := range dexFiles {
		data, err := readZipEntry(df)
		if err != nil || len(data) == 0 {
			continue
		}

		for _, t := range targets {
			if bytes.Contains(data, t) {
				return true
			}
		}
	}

	return false
}
