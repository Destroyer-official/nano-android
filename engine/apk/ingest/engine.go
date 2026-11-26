package ingest

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nano_android/engine/apk/dex"
	"nano_android/engine/apk/manifest"
	"nano_android/engine/apk/resources"
	"nano_android/engine/apk/signing"
	"nano_android/engine/security/archive"
	"nano_android/engine/security/worker_isolation"
)

// PackageFormat identifies ingested Android package container types.
type PackageFormat string

const (
	FormatAPK        PackageFormat = "apk"
	FormatJAR        PackageFormat = "jar"
	FormatAAB        PackageFormat = "aab"
	FormatAPKS       PackageFormat = "apks"
	FormatXAPK       PackageFormat = "xapk"
	FormatSplitSet   PackageFormat = "split_set"
)

// SplitComponent describes a base/config/dynamic-feature APK inside an AAB/.apks/.xapk.
type SplitComponent struct {
	Name   string `json:"name"`
	Role   string `json:"role"` // "base", "abi_split", "density_split", "locale_split", "dynamic_feature"
	Size   int64  `json:"size_bytes"`
}

// ProvenanceRecord tracks immutable ingestion history for an analyzed artifact.
type ProvenanceRecord struct {
	TimestampUTC string        `json:"timestamp_utc"`
	Path         string        `json:"path"`
	Format       PackageFormat `json:"format"`
	SHA256       string        `json:"sha256"`
	SizeBytes    int64         `json:"size_bytes"`
	PackageName  string        `json:"package_name,omitempty"`
}

// PermissionRisk classifies an Android permission by security sensitivity.
type PermissionRisk struct {
	Permission string `json:"permission"`
	RiskLevel  string `json:"risk_level"` // "HIGH", "MEDIUM", "NORMAL"
	Category   string `json:"category"`
	Reason     string `json:"reason"`
}

// IntelligenceReport is the complete Tier-0 Static APK Intelligence payload.
type IntelligenceReport struct {
	Format               PackageFormat           `json:"format"`
	Path                 string                  `json:"path"`
	SHA256               string                  `json:"sha256"`
	SizeBytes            int64                   `json:"size_bytes"`
	ArchiveSummary       *archive.AuditSummary   `json:"archive_summary"`
	Manifest             *manifest.Info          `json:"manifest"`
	DEX                  *dex.Inventory          `json:"dex"`
	Resources            *resources.Table        `json:"resources,omitempty"`
	SizeDistribution     []resources.SizeBucket  `json:"size_distribution,omitempty"`
	DuplicateAssets      []resources.DuplicateAsset `json:"duplicate_assets,omitempty"`
	Signing              signing.SigningReport   `json:"signing"`
	NativeLibs           []signing.NativeLibInfo `json:"native_libs,omitempty"`
	PermissionRisks      []PermissionRisk        `json:"permission_risks,omitempty"`
	Splits               []SplitComponent        `json:"splits,omitempty"`
	Warnings             []string                `json:"warnings,omitempty"`
}

// ComparisonReport describes structural deltas between two APK versions or artifacts.
type ComparisonReport struct {
	BasePackage       string   `json:"base_package"`
	TargetPackage     string   `json:"target_package"`
	SizeDeltaBytes    int64    `json:"size_delta_bytes"`
	AddedPermissions  []string `json:"added_permissions,omitempty"`
	RemovedPermissions []string `json:"removed_permissions,omitempty"`
	AddedActivities   []string `json:"added_activities,omitempty"`
	RemovedActivities []string `json:"removed_activities,omitempty"`
	AddedClasses      []string `json:"added_classes,omitempty"`
	RemovedClasses    []string `json:"removed_classes,omitempty"`
	SignerConsistent  bool     `json:"signer_consistent"`
}

// Engine executes secure Tier-0 ingestion, static analysis, AAB conversion, and comparisons.
type Engine struct {
	bundletoolJar   string
	provenanceFile  string
	cacheRoot       string
}

// NewEngine constructs an APK Intelligence Engine.
func NewEngine(cacheRoot, bundletoolJar string) *Engine {
	if strings.TrimSpace(cacheRoot) == "" {
		cacheRoot = filepath.Join("cache", "apk_intel")
	}
	return &Engine{
		bundletoolJar:  bundletoolJar,
		provenanceFile: filepath.Join(cacheRoot, "provenance.jsonl"),
		cacheRoot:      cacheRoot,
	}
}

// Analyze inspects an .apk, .aab, .apks, or .xapk file safely after archive validation.
func (e *Engine) Analyze(artifactPath string) (*IntelligenceReport, error) {
	sum, err := archive.ValidateArchive(artifactPath, archive.DefaultLimits())
	if err != nil {
		return nil, fmt.Errorf("archive security check failed: %w", err)
	}
	shaHex, sizeBytes, err := hashFile(artifactPath)
	if err != nil {
		return nil, err
	}

	low := strings.ToLower(artifactPath)
	rep := &IntelligenceReport{
		Path:           artifactPath,
		SHA256:         shaHex,
		SizeBytes:      sizeBytes,
		ArchiveSummary: sum,
	}

	switch {
	case strings.HasSuffix(low, ".apk"):
		rep.Format = FormatAPK
		if err := e.analyzeSingleAPK(artifactPath, rep); err != nil {
			return nil, err
		}
	case strings.HasSuffix(low, ".jar"):
		rep.Format = FormatJAR
		if err := e.analyzeJAR(artifactPath, rep); err != nil {
			return nil, err
		}
	case strings.HasSuffix(low, ".aab"):
		rep.Format = FormatAAB
		if err := e.analyzeAAB(artifactPath, rep); err != nil {
			return nil, err
		}
	case strings.HasSuffix(low, ".apks") || strings.HasSuffix(low, ".xapk"):
		if strings.HasSuffix(low, ".xapk") {
			rep.Format = FormatXAPK
		} else {
			rep.Format = FormatAPKS
		}
		if err := e.analyzeSplitArchive(artifactPath, rep); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported package format: %s", filepath.Base(artifactPath))
	}

	pkgName := ""
	if rep.Manifest != nil {
		pkgName = rep.Manifest.Package
	}
	_ = e.appendProvenance(ProvenanceRecord{
		TimestampUTC: time.Now().UTC().Format(time.RFC3339),
		Path:         artifactPath,
		Format:       rep.Format,
		SHA256:       shaHex,
		SizeBytes:    sizeBytes,
		PackageName:  pkgName,
	})

	return rep, nil
}

// AnalyzeBatch inspects multiple APK/AAB/APKS artifacts deterministically.
func (e *Engine) AnalyzeBatch(paths []string) ([]*IntelligenceReport, []error) {
	reports := make([]*IntelligenceReport, 0, len(paths))
	var errs []error
	for _, p := range paths {
		r, err := e.Analyze(p)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(p), err))
			continue
		}
		reports = append(reports, r)
	}
	return reports, errs
}

func (e *Engine) analyzeSingleAPK(apkPath string, rep *IntelligenceReport) error {
	f, err := os.Open(apkPath)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		return err
	}

	var dexParts []*dex.Inventory
	for _, zf := range zr.File {
		switch {
		case zf.Name == "AndroidManifest.xml":
			if data, err := readZipEntry(zf, 16<<20); err == nil {
				mInfo, mErr := manifest.ParseBinary(data)
				if mErr != nil {
					rep.Warnings = append(rep.Warnings, fmt.Sprintf("manifest decode warning: %v", mErr))
				} else {
					rep.Manifest = mInfo
				}
			}
		case zf.Name == "resources.arsc":
			if data, err := readZipEntry(zf, 64<<20); err == nil {
				if tbl, rErr := resources.ParseARSC(data); rErr == nil {
					rep.Resources = tbl
				} else {
					rep.Warnings = append(rep.Warnings, fmt.Sprintf("resources.arsc decode warning: %v", rErr))
				}
			}
		case strings.HasPrefix(zf.Name, "classes") && strings.HasSuffix(zf.Name, ".dex"):
			if data, err := readZipEntry(zf, 64<<20); err == nil {
				if dInv, dErr := dex.ParseSingle(zf.Name, data); dErr == nil {
					dexParts = append(dexParts, dInv)
				} else {
					rep.Warnings = append(rep.Warnings, fmt.Sprintf("DEX decode warning (%s): %v", zf.Name, dErr))
				}
			}
		}
	}

	rep.DEX = dex.MergeInventories(dexParts)
	rep.SizeDistribution, rep.DuplicateAssets = resources.AnalyzeArchiveSizes(zr)
	rep.Signing = signing.InspectSigning(apkPath, zr)
	rep.NativeLibs = signing.InspectNativeLibs(zr)
	if rep.Manifest != nil {
		rep.PermissionRisks = ClassifyPermissions(rep.Manifest.Permissions)
	}
	return nil
}

func (e *Engine) analyzeJAR(jarPath string, rep *IntelligenceReport) error {
	f, err := os.Open(jarPath)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		return err
	}

	var classes []dex.ClassInfo
	var activities []manifest.Component
	libSet := map[string]bool{}
	var compatIssues []dex.CompatibilityIssue
	seenCompose := false
	launcher := ""
	inferredPkg := ""

	for _, zf := range zr.File {
		if strings.HasSuffix(zf.Name, ".class") && !strings.Contains(zf.Name, "$") {
			internal := strings.TrimSuffix(zf.Name, ".class")
			javaName := strings.ReplaceAll(internal, "/", ".")
			classes = append(classes, dex.ClassInfo{
				Descriptor: "L" + internal + ";",
				JavaName:   javaName,
				DexFile:    "jvm_classes.jar",
			})
			switch {
			case strings.HasPrefix(internal, "androidx/compose/") && !seenCompose:
				seenCompose = true
				libSet["Jetpack Compose"] = true
				compatIssues = append(compatIssues, dex.CompatibilityIssue{
					API:             "androidx.compose.*",
					Category:        "COMPOSE",
					Severity:        "REQUIRES_ART",
					RecommendedTier: "robolectric",
				})
			case strings.HasPrefix(internal, "androidx/"):
				libSet["AndroidX"] = true
			case strings.HasPrefix(internal, "kotlin/"):
				libSet["Kotlin Stdlib"] = true
			}
			isFrameworkPkg := strings.HasPrefix(javaName, "android.") || strings.HasPrefix(javaName, "androidx.") || strings.HasPrefix(javaName, "kotlin.") || strings.HasPrefix(javaName, "java.") || strings.HasPrefix(javaName, "META-INF.")
			if !isFrameworkPkg {
				if inferredPkg == "" {
					if idx := strings.LastIndex(javaName, "."); idx > 0 {
						parts := strings.Split(javaName[:idx], ".")
						if len(parts) >= 2 {
							inferredPkg = strings.Join(parts[:2], ".")
						} else {
							inferredPkg = parts[0]
						}
					}
				}
				if strings.HasSuffix(javaName, "Activity") {
					activities = append(activities, manifest.Component{Kind: "activity", Name: javaName})
					if launcher == "" || strings.HasSuffix(javaName, "MainActivity") {
						launcher = javaName
					}
				}
			}
		}
	}

	if inferredPkg == "" {
		base := strings.TrimSuffix(filepath.Base(jarPath), filepath.Ext(jarPath))
		inferredPkg = "jvm." + strings.NewReplacer("-", "_", " ", "_").Replace(strings.ToLower(base))
	}

	var libs []string
	for l := range libSet {
		libs = append(libs, l)
	}
	sort.Strings(libs)

	inv := dex.MergeInventories([]*dex.Inventory{{
		DexFiles:             []string{"jvm_classes.jar"},
		DexVersions:          []string{"JVM"},
		TotalClasses:         len(classes),
		Classes:              classes,
		ClassDependencyGraph: map[string][]string{},
		DetectedLibraries:    libs,
		CompatibilityIssues:  compatIssues,
	}})

	rep.DEX = inv
	rep.Manifest = &manifest.Info{
		Package:          inferredPkg,
		AppLabel:         filepath.Base(jarPath),
		MinSDK:           26,
		TargetSDK:        35,
		LauncherActivity: launcher,
		Permissions:      []string{},
		Activities:       activities,
	}
	rep.SizeDistribution, rep.DuplicateAssets = resources.AnalyzeArchiveSizes(zr)
	rep.Signing = signing.InspectSigning(jarPath, zr)
	rep.NativeLibs = signing.InspectNativeLibs(zr)
	rep.Warnings = append(rep.Warnings, "Standalone JVM JAR archive (no AndroidManifest.xml/resources.arsc); analyzed compiled .class entries")
	return nil
}

// ClassifyPermissions assigns explicit risk tiers to declared Android manifest permissions.
func ClassifyPermissions(perms []string) []PermissionRisk {
	high := map[string][2]string{
		"android.permission.READ_SMS":                {"SMS", "Access to SMS inbox contents"},
		"android.permission.RECEIVE_SMS":             {"SMS", "Interception of incoming SMS"},
		"android.permission.SEND_SMS":                {"SMS", "Can send premium or phishing SMS"},
		"android.permission.ACCESS_BACKGROUND_LOCATION": {"LOCATION", "Always-on background location access"},
		"android.permission.SYSTEM_ALERT_WINDOW":     {"OVERLAY", "Can draw over other applications (tapjacking vector)"},
		"android.permission.REQUEST_INSTALL_PACKAGES": {"INSTALLER", "Can prompt installation of arbitrary APKs"},
		"android.permission.MANAGE_EXTERNAL_STORAGE": {"STORAGE", "Broad filesystem access across external storage"},
		"android.permission.RECORD_AUDIO":            {"SENSOR_PRIVACY", "Microphone recording capability"},
		"android.permission.CAMERA":                  {"SENSOR_PRIVACY", "Camera capture capability"},
		"android.permission.QUERY_ALL_PACKAGES":      {"PRIVACY", "Enumerates all installed apps on device"},
	}
	medium := map[string][2]string{
		"android.permission.ACCESS_FINE_LOCATION":   {"LOCATION", "Precise GPS location access"},
		"android.permission.ACCESS_COARSE_LOCATION": {"LOCATION", "Approximate network location"},
		"android.permission.READ_CONTACTS":          {"PII", "Reads user contacts"},
		"android.permission.WRITE_CONTACTS":         {"PII", "Modifies user contacts"},
		"android.permission.READ_MEDIA_VIDEO":       {"MEDIA", "Reads shared media videos"},
		"android.permission.READ_MEDIA_IMAGES":      {"MEDIA", "Reads shared media images"},
	}
	out := make([]PermissionRisk, 0, len(perms))
	for _, p := range perms {
		if info, ok := high[p]; ok {
			out = append(out, PermissionRisk{Permission: p, RiskLevel: "HIGH", Category: info[0], Reason: info[1]})
		} else if info, ok := medium[p]; ok {
			out = append(out, PermissionRisk{Permission: p, RiskLevel: "MEDIUM", Category: info[0], Reason: info[1]})
		} else {
			out = append(out, PermissionRisk{Permission: p, RiskLevel: "NORMAL", Category: "STANDARD", Reason: "Standard or install-time permission"})
		}
	}
	return out
}

// VerifyBuildArtifact checks a built APK against expected package identity, min versionCode, and signer fingerprint.
func VerifyBuildArtifact(rep *IntelligenceReport, expectedPkg string, minVersionCode int64, requiredSignerSHA256 string) []string {
	var issues []string
	if rep == nil || rep.Manifest == nil {
		return []string{"missing decoded manifest in build artifact"}
	}
	if expectedPkg != "" && rep.Manifest.Package != expectedPkg {
		issues = append(issues, fmt.Sprintf("package mismatch: got %q, expected %q", rep.Manifest.Package, expectedPkg))
	}
	if minVersionCode > 0 && rep.Manifest.VersionCode < minVersionCode {
		issues = append(issues, fmt.Sprintf("versionCode %d < required %d", rep.Manifest.VersionCode, minVersionCode))
	}
	if !rep.Signing.IsSigned {
		issues = append(issues, "artifact is unsigned")
	} else if requiredSignerSHA256 != "" {
		matched := false
		for _, c := range rep.Signing.Certificates {
			if strings.EqualFold(c.SHA256, requiredSignerSHA256) {
				matched = true
				break
			}
		}
		if !matched {
			issues = append(issues, "signing certificate SHA-256 did not match expected key")
		}
	}
	return issues
}

// FormatMarkdownReport generates structured technical documentation for an analyzed APK/AAB.
func FormatMarkdownReport(rep *IntelligenceReport) string {
	if rep == nil {
		return "# APK Intelligence Report\n\nNo report available.\n"
	}
	var sb strings.Builder
	sb.WriteString("# APK Intelligence Report\n\n")
	sb.WriteString(fmt.Sprintf("- **File:** `%s` (`%s`, %d bytes)\n", filepath.Base(rep.Path), rep.Format, rep.SizeBytes))
	sb.WriteString(fmt.Sprintf("- **SHA-256:** `%s`\n", rep.SHA256))
	if rep.Manifest != nil {
		sb.WriteString(fmt.Sprintf("- **Package:** `%s` (versionCode `%d`, versionName `%s`)\n", rep.Manifest.Package, rep.Manifest.VersionCode, rep.Manifest.VersionName))
		sb.WriteString(fmt.Sprintf("- **SDK:** min=`%d`, target=`%d`, compile=`%d`\n", rep.Manifest.MinSDK, rep.Manifest.TargetSDK, rep.Manifest.CompileSDK))
		sb.WriteString(fmt.Sprintf("- **Components:** %d activities, %d services, %d receivers, %d providers\n", len(rep.Manifest.Activities), len(rep.Manifest.Services), len(rep.Manifest.Receivers), len(rep.Manifest.Providers)))
	}
	sb.WriteString(fmt.Sprintf("- **Signing:** v1=%t, v2=%t, v3=%t, v4=%t (%d certs)\n", rep.Signing.HasV1, rep.Signing.HasV2, rep.Signing.HasV3, rep.Signing.HasV4, len(rep.Signing.Certificates)))
	if rep.DEX != nil {
		sb.WriteString(fmt.Sprintf("- **DEX:** %d classes, %d methods across %d file(s)\n", rep.DEX.TotalClasses, rep.DEX.TotalMethods, len(rep.DEX.DexFiles)))
	}
	if len(rep.PermissionRisks) > 0 {
		sb.WriteString("\n## Permission Risks\n")
		for _, pr := range rep.PermissionRisks {
			sb.WriteString(fmt.Sprintf("- **%s** `%s`: %s\n", pr.RiskLevel, pr.Permission, pr.Reason))
		}
	}
	return sb.String()
}

func (e *Engine) analyzeAAB(aabPath string, rep *IntelligenceReport) error {
	f, err := os.Open(aabPath)
	if err != nil {
		return err
	}
	defer f.Close()
	st, _ := f.Stat()
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		return err
	}

	moduleSet := map[string]bool{}
	for _, zf := range zr.File {
		parts := strings.Split(zf.Name, "/")
		if len(parts) >= 2 && parts[0] != "BUNDLE-METADATA" && parts[0] != "META-INF" {
			moduleSet[parts[0]] = true
		}
		if zf.Name == "base/manifest/AndroidManifest.xml" {
			rep.Warnings = append(rep.Warnings, "AAB detected: base/manifest/AndroidManifest.xml uses proto-XML; converting via bundletool if available for APK-level inspection")
		}
	}
	for mod := range moduleSet {
		role := "dynamic_feature"
		if mod == "base" {
			role = "base"
		}
		rep.Splits = append(rep.Splits, SplitComponent{Name: mod, Role: role})
	}
	sort.Slice(rep.Splits, func(i, j int) bool { return rep.Splits[i].Name < rep.Splits[j].Name })

	// If bundletool JAR is available, convert AAB -> universal .apks in an isolated worker workspace.
	if e.bundletoolJar != "" {
		if _, err := os.Stat(e.bundletoolJar); err == nil {
			ws, wErr := worker_isolation.PrepareWorkspace(e.cacheRoot)
			if wErr == nil {
				defer func() { _ = ws.Cleanup() }()
				outApks := filepath.Join(ws.TempDir, "converted.apks")
				cmd := exec.Command("java", "-jar", e.bundletoolJar, "build-apks",
					"--bundle="+aabPath, "--output="+outApks, "--mode=universal")
				h, isoErr := worker_isolation.StartIsolated(cmd, ws.WorkerID, ws, worker_isolation.DefaultTranslatorLimits())
				if isoErr == nil {
					_ = cmd.Wait()
					_ = h.Close()
					if stOut, sErr := os.Stat(outApks); sErr == nil && stOut.Size() > 0 {
						_ = e.analyzeSplitArchive(outApks, rep)
					}
				}
			}
		}
	}
	return nil
}

func (e *Engine) analyzeSplitArchive(bundlePath string, rep *IntelligenceReport) error {
	f, err := os.Open(bundlePath)
	if err != nil {
		return err
	}
	defer f.Close()
	st, _ := f.Stat()
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		return err
	}

	ws, wErr := worker_isolation.PrepareWorkspace(e.cacheRoot)
	if wErr != nil {
		return wErr
	}
	defer func() { _ = ws.Cleanup() }()

	var baseExtracted string
	for _, zf := range zr.File {
		low := strings.ToLower(zf.Name)
		if !strings.HasSuffix(low, ".apk") {
			continue
		}
		role := classifySplitRole(zf.Name)
		rep.Splits = append(rep.Splits, SplitComponent{
			Name: zf.Name,
			Role: role,
			Size: int64(zf.UncompressedSize64),
		})
		if role == "base" && baseExtracted == "" {
			data, err := readZipEntry(zf, 256<<20)
			if err == nil {
				tmpBase := filepath.Join(ws.TempDir, "base.apk")
				if wErr := os.WriteFile(tmpBase, data, 0600); wErr == nil {
					baseExtracted = tmpBase
				}
			}
		}
	}
	if baseExtracted != "" {
		return e.analyzeSingleAPK(baseExtracted, rep)
	}
	return nil
}

func classifySplitRole(name string) string {
	low := strings.ToLower(filepath.Base(name))
	switch {
	case strings.Contains(low, "base") || low == "universal.apk":
		return "base"
	case strings.Contains(low, "arm64") || strings.Contains(low, "armeabi") || strings.Contains(low, "x86"):
		return "abi_split"
	case strings.Contains(low, "hdpi") || strings.Contains(low, "xxhdpi"):
		return "density_split"
	case strings.HasPrefix(low, "split_config."):
		return "locale_split"
	default:
		return "dynamic_feature"
	}
}

// Compare compares two IntelligenceReports across permissions, components, classes, size, and signing.
func Compare(base, target *IntelligenceReport) *ComparisonReport {
	out := &ComparisonReport{
		SizeDeltaBytes:   target.SizeBytes - base.SizeBytes,
		SignerConsistent: signersEqual(base.Signing, target.Signing),
	}
	if base.Manifest != nil {
		out.BasePackage = base.Manifest.Package
	}
	if target.Manifest != nil {
		out.TargetPackage = target.Manifest.Package
	}
	if base.Manifest != nil && target.Manifest != nil {
		out.AddedPermissions, out.RemovedPermissions = stringSetDiff(base.Manifest.Permissions, target.Manifest.Permissions)
		var baseActs, targetActs []string
		for _, a := range base.Manifest.Activities {
			baseActs = append(baseActs, a.Name)
		}
		for _, a := range target.Manifest.Activities {
			targetActs = append(targetActs, a.Name)
		}
		out.AddedActivities, out.RemovedActivities = stringSetDiff(baseActs, targetActs)
	}
	if base.DEX != nil && target.DEX != nil {
		var baseCls, targetCls []string
		for _, c := range base.DEX.Classes {
			baseCls = append(baseCls, c.JavaName)
		}
		for _, c := range target.DEX.Classes {
			targetCls = append(targetCls, c.JavaName)
		}
		out.AddedClasses, out.RemovedClasses = stringSetDiff(baseCls, targetCls)
	}
	return out
}

func signersEqual(a, b signing.SigningReport) bool {
	if len(a.Certificates) == 0 || len(b.Certificates) == 0 {
		return len(a.Certificates) == len(b.Certificates)
	}
	return a.Certificates[0].SHA256 == b.Certificates[0].SHA256
}

func stringSetDiff(before, after []string) (added, removed []string) {
	bSet := make(map[string]bool, len(before))
	aSet := make(map[string]bool, len(after))
	for _, s := range before {
		bSet[s] = true
	}
	for _, s := range after {
		aSet[s] = true
		if !bSet[s] {
			added = append(added, s)
		}
	}
	for _, s := range before {
		if !aSet[s] {
			removed = append(removed, s)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

func (e *Engine) appendProvenance(rec ProvenanceRecord) error {
	if err := os.MkdirAll(filepath.Dir(e.provenanceFile), 0750); err != nil {
		return err
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(e.provenanceFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(raw, '\n'))
	return err
}

func readZipEntry(zf *zip.File, maxBytes int64) ([]byte, error) {
	rc, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, maxBytes))
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
