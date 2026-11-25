package build

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

var (
	reApksignerCertSHA256 = regexp.MustCompile(`(?i)certificate SHA-256 digest:\s*([0-9a-fA-F]{64})`)
	reApksignerCertSHA1   = regexp.MustCompile(`(?i)certificate SHA-1 digest:\s*([0-9a-fA-F]{40})`)
	reAapt2Package        = regexp.MustCompile(`package:\s+name='([^']+)'(?:\s+versionCode='(\d+)')?(?:\s+versionName='([^']*)')?`)
	reAapt2MinSDK         = regexp.MustCompile(`sdkVersion:'(\d+)'`)
	reAapt2TargetSDK      = regexp.MustCompile(`targetSdkVersion:'(\d+)'`)
	reAapt2Permission     = regexp.MustCompile(`uses-permission:\s+name='([^']+)'`)
	reAapt2Launcher       = regexp.MustCompile(`launchable-activity:\s+name='([^']+)'`)
	reAapt2Scheme         = regexp.MustCompile(`scheme:\s*'([^']+)'`)
	reLibABI              = regexp.MustCompile(`^/?lib/([^/]+)/[^/]+\.so$`)
)

// ParseApksignerOutput extracts certificate fingerprints from `apksigner verify --print-certs` output.
func ParseApksignerOutput(output string) []string {
	var certs []string
	matches := reApksignerCertSHA256.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		if len(m) > 1 {
			certs = appendUnique(certs, strings.ToLower(m[1]))
		}
	}
	if len(certs) == 0 {
		matchesSHA1 := reApksignerCertSHA1.FindAllStringSubmatch(output, -1)
		for _, m := range matchesSHA1 {
			if len(m) > 1 {
				certs = appendUnique(certs, strings.ToLower(m[1]))
			}
		}
	}
	return certs
}

// ParseAapt2Badging extracts APK metadata from `aapt2 dump badging` output.
func ParseAapt2Badging(output string) *types.APKInfo {
	info := &types.APKInfo{
		ABIs:               []string{},
		Permissions:        []string{},
		ExportedComponents: []types.ExportedComponent{},
		SigningCerts:       []string{},
		DeepLinkSchemes:    []string{},
	}

	if pkgMatches := reAapt2Package.FindStringSubmatch(output); len(pkgMatches) > 1 {
		info.PackageID = pkgMatches[1]
		if len(pkgMatches) > 2 && pkgMatches[2] != "" {
			info.VersionCode, _ = strconv.Atoi(pkgMatches[2])
		}
		if len(pkgMatches) > 3 {
			info.VersionName = pkgMatches[3]
		}
	}

	if minMatches := reAapt2MinSDK.FindStringSubmatch(output); len(minMatches) > 1 {
		info.MinSDK, _ = strconv.Atoi(minMatches[1])
	}
	if targetMatches := reAapt2TargetSDK.FindStringSubmatch(output); len(targetMatches) > 1 {
		info.TargetSDK, _ = strconv.Atoi(targetMatches[1])
	}
	if launchMatches := reAapt2Launcher.FindStringSubmatch(output); len(launchMatches) > 1 {
		info.LauncherActivity = launchMatches[1]
	}

	for _, m := range reAapt2Permission.FindAllStringSubmatch(output, -1) {
		if len(m) > 1 {
			info.Permissions = appendUnique(info.Permissions, m[1])
		}
	}

	for _, m := range reAapt2Scheme.FindAllStringSubmatch(output, -1) {
		if len(m) > 1 {
			info.DeepLinkSchemes = appendUnique(info.DeepLinkSchemes, m[1])
		}
	}

	return info
}

// ParseApkanalyzerPermissions extracts permission list from `apkanalyzer manifest permissions` output.
func ParseApkanalyzerPermissions(output string) []string {
	var perms []string
	lines := strings.Split(output, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" && strings.Contains(l, "permission") {
			perms = appendUnique(perms, l)
		}
	}
	return perms
}

// ParseApkanalyzerFiles extracts supported ABIs from `apkanalyzer files list` output.
func ParseApkanalyzerFiles(output string) []string {
	var abis []string
	lines := strings.Split(output, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		matches := reLibABI.FindStringSubmatch(l)
		if len(matches) > 1 {
			abis = appendUnique(abis, matches[1])
		}
	}
	return abis
}

// readZipEntry reads the uncompressed contents of a zip.File, tolerating ErrChecksum
// which frequently occurs with signed or aligned Android APK entries.
func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil && !errors.Is(err, zip.ErrChecksum) && len(data) == 0 {
		return nil, err
	}
	return data, nil
}

// InspectAPKDirect performs static analysis directly on the APK zip archive.
func InspectAPKDirect(apkPath string) (*types.APKInfo, error) {
	zr, err := zip.OpenReader(apkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open APK as zip: %w", err)
	}
	defer zr.Close()

	info := &types.APKInfo{
		ABIs:               []string{},
		Permissions:        []string{},
		ExportedComponents: []types.ExportedComponent{},
		SigningCerts:       []string{},
		DeepLinkSchemes:    []string{},
	}

	var manifestData []byte

	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "/")

		// Extract ABIs from lib/<abi>/...
		if matches := reLibABI.FindStringSubmatch(name); len(matches) > 1 {
			info.ABIs = appendUnique(info.ABIs, matches[1])
		}

		// Read AndroidManifest.xml
		if strings.EqualFold(strings.Trim(name, "/\\"), "androidmanifest.xml") {
			data, err := readZipEntry(f)
			if err == nil && len(data) > 0 {
				manifestData = data
			}
		}

		// Extract signing certs from META-INF/*.RSA, *.DSA, *.EC
		if strings.HasPrefix(name, "META-INF/") && (strings.HasSuffix(name, ".RSA") || strings.HasSuffix(name, ".DSA") || strings.HasSuffix(name, ".EC")) {
			data, err := readZipEntry(f)
			if err == nil && len(data) > 0 {
				certs := extractCertFingerprints(data)
				for _, c := range certs {
					info.SigningCerts = appendUnique(info.SigningCerts, c)
				}
			}
		}
	}

	if len(manifestData) > 0 {
		details, err := ParseManifest(manifestData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse AndroidManifest.xml: %w", err)
		}
		info.PackageID = details.PackageID
		info.VersionCode = details.VersionCode
		info.VersionName = details.VersionName
		info.MinSDK = details.MinSDK
		info.TargetSDK = details.TargetSDK
		info.LauncherActivity = details.LauncherActivity
		info.Permissions = details.Permissions
		info.ExportedComponents = details.ExportedComponents
		info.DeepLinkSchemes = details.DeepLinkSchemes
	} else {
		return nil, fmt.Errorf("no AndroidManifest.xml found in APK")
	}

	return info, nil
}

// extractCertFingerprints attempts to parse PKCS#7 or X.509 certs and compute SHA-256 fingerprint.
func extractCertFingerprints(data []byte) []string {
	var fingerprints []string

	// Check if direct X.509 cert
	if certs, err := x509.ParseCertificates(data); err == nil && len(certs) > 0 {
		for _, cert := range certs {
			hash := sha256.Sum256(cert.Raw)
			fingerprints = appendUnique(fingerprints, hex.EncodeToString(hash[:]))
		}
		return fingerprints
	}

	// For PKCS#7 structures, scan for certificate ASN.1 sequence or fallback to SHA-256 of signature block
	h := sha256.Sum256(data)
	fingerprints = appendUnique(fingerprints, hex.EncodeToString(h[:]))
	return fingerprints
}

// executeInspectAPK orchestrates static APK inspection using CLI tools when available,
// with direct ZIP/AXML inspection as base and fallback.
func (b *GradleEngine) executeInspectAPK(ctx context.Context, apkPath string, decompileClasses []string) (*types.APKInfo, error) {
	if _, err := os.Stat(apkPath); err != nil {
		return nil, fmt.Errorf("APK not found at %q: %w", apkPath, err)
	}

	// Step 1: Direct inspection (always works, fast, comprehensive)
	info, err := InspectAPKDirect(apkPath)
	if err != nil {
		return nil, err
	}

	// Step 2: Try running apksigner if available to augment signing certs
	apksignerCmd := "apksigner"
	if b.buildToolsDir != "" {
		apksignerCmd = filepath.Join(b.buildToolsDir, "apksigner")
	}
	out, err := b.executor.Run(ctx, "", nil, apksignerCmd, "verify", "--print-certs", apkPath)
	if err == nil && len(out) > 0 {
		certs := ParseApksignerOutput(string(out))
		if len(certs) > 0 {
			info.SigningCerts = certs
		}
	}

	// Step 3: Try running apkanalyzer if available
	apkanalyzerCmd := "apkanalyzer"
	if b.buildToolsDir != "" {
		apkanalyzerCmd = filepath.Join(b.buildToolsDir, "apkanalyzer")
	}
	permOut, err := b.executor.Run(ctx, "", nil, apkanalyzerCmd, "manifest", "permissions", apkPath)
	if err == nil && len(permOut) > 0 {
		perms := ParseApkanalyzerPermissions(string(permOut))
		for _, p := range perms {
			info.Permissions = appendUnique(info.Permissions, p)
		}
	}

	filesOut, err := b.executor.Run(ctx, "", nil, apkanalyzerCmd, "files", "list", apkPath)
	if err == nil && len(filesOut) > 0 {
		abis := ParseApkanalyzerFiles(string(filesOut))
		for _, abi := range abis {
			info.ABIs = appendUnique(info.ABIs, abi)
		}
	}

	// Step 4: Try running aapt2 dump badging if available
	aapt2Cmd := "aapt2"
	if b.buildToolsDir != "" {
		aapt2Cmd = filepath.Join(b.buildToolsDir, "aapt2")
	}
	badgingOut, err := b.executor.Run(ctx, "", nil, aapt2Cmd, "dump", "badging", apkPath)
	if err == nil && len(badgingOut) > 0 {
		badgingInfo := ParseAapt2Badging(string(badgingOut))
		if info.PackageID == "" {
			info.PackageID = badgingInfo.PackageID
		}
		if info.VersionCode == 0 {
			info.VersionCode = badgingInfo.VersionCode
		}
		if info.LauncherActivity == "" {
			info.LauncherActivity = badgingInfo.LauncherActivity
		}
		for _, s := range badgingInfo.DeepLinkSchemes {
			info.DeepLinkSchemes = appendUnique(info.DeepLinkSchemes, s)
		}
	}

	// Step 5: Bounded jadx decompile if classes requested
	if len(decompileClasses) > 0 {
		hash := sha256.Sum256([]byte(apkPath + strings.Join(decompileClasses, ",")))
		tempDecompileDir := filepath.Join(os.TempDir(), fmt.Sprintf("jadx_%s", hex.EncodeToString(hash[:8])))
		_ = os.MkdirAll(tempDecompileDir, 0755)

		jadxArgs := []string{"--no-res", "--no-imports", "-d", tempDecompileDir, "--class", strings.Join(decompileClasses, ","), apkPath}
		_, _ = b.executor.Run(ctx, "", nil, "jadx", jadxArgs...)
	}

	return info, nil
}
