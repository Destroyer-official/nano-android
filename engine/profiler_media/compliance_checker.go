package profiler_media

import (
	"archive/zip"
	"bytes"
	"debug/elf"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	// PageSize16KB is the 16 KB boundary required by Google Play 2027 and Android 15+.
	PageSize16KB uint64 = 0x4000 // 16384 bytes

	// TargetAPI37 represents Android 17 (2026/2027 standard).
	TargetAPI37 int = 37
	// TargetAPI36 represents Android 16.
	TargetAPI36 int = 36
)

// ELFSegmentAudit details alignment properties of an individual ELF PT_LOAD segment.
type ELFSegmentAudit struct {
	Index         int    `json:"index"`
	Flags         string `json:"flags"`
	Offset        uint64 `json:"offset"`
	VAddr         uint64 `json:"vaddr"`
	Align         uint64 `json:"align"`
	AlignHex      string `json:"align_hex"`
	Is16KBAligned bool   `json:"is_16kb_aligned"`
	Congruent     bool   `json:"congruent"`
	Reason        string `json:"reason,omitempty"`
}

// SOAuditResult summarizes 16 KB compliance for a native .so library.
type SOAuditResult struct {
	Path          string            `json:"path"`
	Filename      string            `json:"filename"`
	ABI           string            `json:"abi"`
	Machine       string            `json:"machine"`
	MaxAlignment  uint64            `json:"max_alignment"`
	MaxAlignHex   string            `json:"max_align_hex"`
	Is16KBAligned bool              `json:"is_16kb_aligned"`
	LoadSegments  []ELFSegmentAudit `json:"load_segments"`
	Violations    []string          `json:"violations"`
}

// ZipNativeLibAudit details the APK archive alignment of an uncompressed native library.
type ZipNativeLibAudit struct {
	EntryName     string `json:"entry_name"`
	Compression   string `json:"compression"` // STORED or DEFLATED
	DataOffset    int64  `json:"data_offset"`
	OffsetHex     string `json:"offset_hex"`
	Is16KBAligned bool   `json:"is_16kb_aligned"`
	Remediation   string `json:"remediation,omitempty"`
}

// PermissionAudit details security audit of an Android permission.
type PermissionAudit struct {
	Name        string `json:"name"`
	IsRestricted bool   `json:"is_restricted"`
	RiskLevel   string `json:"risk_level"` // CRITICAL, HIGH, MEDIUM, NORMAL
	Category    string `json:"category"`   // LOCATION, STORAGE, SMS, PHONE, MICROPHONE, CAMERA, PACKAGE_QUERY, FOREGROUND_SERVICE, HEALTH_CONNECT
	Remediation string `json:"remediation,omitempty"`
}

// ComplianceViolation details an issue blocking Google Play or Android 17 compliance.
type ComplianceViolation struct {
	Category    string `json:"category"` // ELF_16KB, ZIPALIGN_16KB, TARGET_SDK, RESTRICTED_PERMISSION
	Target      string `json:"target"`
	Description string `json:"description"`
	Severity    string `json:"severity"` // BLOCKER, WARNING
	Remediation string `json:"remediation,omitempty"`
}

// ComplianceReport aggregates native ELF, SDK target, permission, and zip alignment audits.
type ComplianceReport struct {
	IsCompliant     bool                  `json:"is_compliant"`
	Android17Ready  bool                  `json:"android_17_ready"`
	TargetSdkVersion int                  `json:"target_sdk_version"`
	MinSdkVersion    int                  `json:"min_sdk_version"`
	PackageName      string               `json:"package_name"`
	NativeLibraries  []SOAuditResult      `json:"native_libraries"`
	ZipAlignments    []ZipNativeLibAudit  `json:"zip_alignments,omitempty"`
	Permissions      []PermissionAudit    `json:"permissions"`
	Violations       []ComplianceViolation `json:"violations"`
	Summary          string               `json:"summary"`
}

// RestrictedPermissionsRegistry lists Android permissions requiring policy approval or special audit.
var RestrictedPermissionsRegistry = map[string]PermissionAudit{
	"android.permission.ACCESS_BACKGROUND_LOCATION": {
		Name:         "android.permission.ACCESS_BACKGROUND_LOCATION",
		IsRestricted: true,
		RiskLevel:    "CRITICAL",
		Category:     "LOCATION",
		Remediation:  "Requires Google Play background location declaration and user consent justification.",
	},
	"android.permission.MANAGE_EXTERNAL_STORAGE": {
		Name:         "android.permission.MANAGE_EXTERNAL_STORAGE",
		IsRestricted: true,
		RiskLevel:    "CRITICAL",
		Category:     "STORAGE",
		Remediation:  "Restricted by Google Play; migrate to Storage Access Framework or MediaStore API.",
	},
	"android.permission.QUERY_ALL_PACKAGES": {
		Name:         "android.permission.QUERY_ALL_PACKAGES",
		IsRestricted: true,
		RiskLevel:    "HIGH",
		Category:     "PACKAGE_QUERY",
		Remediation:  "Explicit declaration required; migrate to specific <queries> elements in manifest.",
	},
	"android.permission.SYSTEM_ALERT_WINDOW": {
		Name:         "android.permission.SYSTEM_ALERT_WINDOW",
		IsRestricted: true,
		RiskLevel:    "HIGH",
		Category:     "OVERLAY",
		Remediation:  "Draw-over-other-apps requires explicit runtime intent and user settings action.",
	},
	"android.permission.REQUEST_INSTALL_PACKAGES": {
		Name:         "android.permission.REQUEST_INSTALL_PACKAGES",
		IsRestricted: true,
		RiskLevel:    "CRITICAL",
		Category:     "INSTALL",
		Remediation:  "Restricted to verified app store or enterprise MDM apps.",
	},
	"android.permission.READ_SMS": {
		Name:         "android.permission.READ_SMS",
		IsRestricted: true,
		RiskLevel:    "CRITICAL",
		Category:     "SMS",
		Remediation:  "SMS & Call Log policy gate: restricted unless default SMS handler.",
	},
	"android.permission.RECEIVE_SMS": {
		Name:         "android.permission.RECEIVE_SMS",
		IsRestricted: true,
		RiskLevel:    "CRITICAL",
		Category:     "SMS",
		Remediation:  "Restricted unless default SMS handler; migrate to SMS Retriever API for OTP.",
	},
	"android.permission.SEND_SMS": {
		Name:         "android.permission.SEND_SMS",
		IsRestricted: true,
		RiskLevel:    "CRITICAL",
		Category:     "SMS",
		Remediation:  "Restricted unless default SMS handler.",
	},
	"android.permission.READ_CALL_LOG": {
		Name:         "android.permission.READ_CALL_LOG",
		IsRestricted: true,
		RiskLevel:    "CRITICAL",
		Category:     "PHONE",
		Remediation:  "Restricted unless default Phone / Dialer handler.",
	},
	"android.permission.READ_PRIVILEGED_PHONE_STATE": {
		Name:         "android.permission.READ_PRIVILEGED_PHONE_STATE",
		IsRestricted: true,
		RiskLevel:    "CRITICAL",
		Category:     "PHONE",
		Remediation:  "System signature permission only; not grantable to 3rd-party apps.",
	},
	"android.permission.RECORD_AUDIO": {
		Name:         "android.permission.RECORD_AUDIO",
		IsRestricted: false,
		RiskLevel:    "MEDIUM",
		Category:     "MICROPHONE",
		Remediation:  "Runtime dangerous permission; requires runtime request.",
	},
	"android.permission.CAMERA": {
		Name:         "android.permission.CAMERA",
		IsRestricted: false,
		RiskLevel:    "MEDIUM",
		Category:     "CAMERA",
		Remediation:  "Runtime dangerous permission; requires runtime request.",
	},
	"android.permission.FOREGROUND_SERVICE_LOCATION": {
		Name:         "android.permission.FOREGROUND_SERVICE_LOCATION",
		IsRestricted: true,
		RiskLevel:    "HIGH",
		Category:     "FOREGROUND_SERVICE",
		Remediation:  "Android 14+ / 17 requires declared foreground service type 'location' and user-facing rationale.",
	},
	"android.permission.FOREGROUND_SERVICE_CAMERA": {
		Name:         "android.permission.FOREGROUND_SERVICE_CAMERA",
		IsRestricted: true,
		RiskLevel:    "HIGH",
		Category:     "FOREGROUND_SERVICE",
		Remediation:  "Android 14+ / 17 requires declared foreground service type 'camera'.",
	},
	"android.permission.FOREGROUND_SERVICE_MICROPHONE": {
		Name:         "android.permission.FOREGROUND_SERVICE_MICROPHONE",
		IsRestricted: true,
		RiskLevel:    "HIGH",
		Category:     "FOREGROUND_SERVICE",
		Remediation:  "Android 14+ / 17 requires declared foreground service type 'microphone'.",
	},
	"android.permission.POST_NOTIFICATIONS": {
		Name:         "android.permission.POST_NOTIFICATIONS",
		IsRestricted: false,
		RiskLevel:    "NORMAL",
		Category:     "NOTIFICATIONS",
		Remediation:  "Runtime notification prompt required on Android 13+.",
	},
	"android.permission.SCHEDULE_EXACT_ALARM": {
		Name:         "android.permission.SCHEDULE_EXACT_ALARM",
		IsRestricted: true,
		RiskLevel:    "HIGH",
		Category:     "ALARM",
		Remediation:  "Restricted to alarm/calendar apps; migrate to USE_EXACT_ALARM or WorkManager.",
	},
	"android.permission.health.READ_STEPS": {
		Name:         "android.permission.health.READ_STEPS",
		IsRestricted: true,
		RiskLevel:    "HIGH",
		Category:     "HEALTH_CONNECT",
		Remediation:  "Requires Health Connect privacy declaration and Play Console approval.",
	},
}

// AuditSOReader checks an ELF binary from an io.ReaderAt for 16 KB segment alignment.
func AuditSOReader(r io.ReaderAt, name string) (*SOAuditResult, error) {
	elfFile, err := elf.NewFile(r)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ELF file %s: %w", name, err)
	}
	defer elfFile.Close()

	result := &SOAuditResult{
		Path:          name,
		Filename:      filepath.Base(name),
		Machine:       elfFile.Machine.String(),
		Is16KBAligned: true,
		LoadSegments:  make([]ELFSegmentAudit, 0),
		Violations:    make([]string, 0),
	}

	// Deduce ABI
	switch elfFile.Machine {
	case elf.EM_AARCH64:
		result.ABI = "arm64-v8a"
	case elf.EM_ARM:
		result.ABI = "armeabi-v7a"
	case elf.EM_X86_64:
		result.ABI = "x86_64"
	case elf.EM_386:
		result.ABI = "x86"
	case elf.EM_RISCV:
		result.ABI = "riscv64"
	default:
		result.ABI = elfFile.Machine.String()
	}

	var maxAlign uint64 = 0
	loadIdx := 0

	for _, prog := range elfFile.Progs {
		if prog.Type != elf.PT_LOAD {
			continue
		}

		if prog.Align > maxAlign {
			maxAlign = prog.Align
		}

		isAligned := prog.Align >= PageSize16KB
		congruent := (prog.Vaddr % prog.Align) == (prog.Off % prog.Align)

		audit := ELFSegmentAudit{
			Index:         loadIdx,
			Flags:         prog.Flags.String(),
			Offset:        prog.Off,
			VAddr:         prog.Vaddr,
			Align:         prog.Align,
			AlignHex:      fmt.Sprintf("0x%x", prog.Align),
			Is16KBAligned: isAligned,
			Congruent:     congruent,
		}

		if !isAligned {
			audit.Reason = fmt.Sprintf("Alignment 0x%x is smaller than required 16 KB (0x4000)", prog.Align)
			result.Is16KBAligned = false
			result.Violations = append(result.Violations,
				fmt.Sprintf("PT_LOAD segment #%d has alignment 0x%x (< 0x4000)", loadIdx, prog.Align))
		}

		if !congruent {
			audit.Reason = fmt.Sprintf("Vaddr (0x%x) %% Align != Offset (0x%x) %% Align", prog.Vaddr, prog.Off)
			result.Violations = append(result.Violations,
				fmt.Sprintf("PT_LOAD segment #%d is not congruent", loadIdx))
		}

		result.LoadSegments = append(result.LoadSegments, audit)
		loadIdx++
	}

	result.MaxAlignment = maxAlign
	result.MaxAlignHex = fmt.Sprintf("0x%x", maxAlign)

	if len(result.LoadSegments) == 0 {
		result.Violations = append(result.Violations, "No PT_LOAD segments found in ELF binary")
		result.Is16KBAligned = false
	}

	return result, nil
}

// AuditSOFile checks a local .so file on disk for 16 KB alignment.
func AuditSOFile(filePath string) (*SOAuditResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open .so file: %w", err)
	}
	defer file.Close()

	return AuditSOReader(file, filePath)
}

// ManifestAuditResult contains parsed AndroidManifest metadata.
type ManifestAuditResult struct {
	PackageName      string            `json:"package_name"`
	TargetSdkVersion int               `json:"target_sdk_version"`
	MinSdkVersion    int               `json:"min_sdk_version"`
	Permissions      []PermissionAudit `json:"permissions"`
	RawPermissions   []string          `json:"raw_permissions"`
}

type xmlUsesSdk struct {
	TargetSdkVersion string `xml:"http://schemas.android.com/apk/res/android targetSdkVersion,attr"`
	MinSdkVersion    string `xml:"http://schemas.android.com/apk/res/android minSdkVersion,attr"`
	TargetSdkBare    string `xml:"targetSdkVersion,attr"`
	MinSdkBare       string `xml:"minSdkVersion,attr"`
}

type xmlUsesPermission struct {
	Name     string `xml:"http://schemas.android.com/apk/res/android name,attr"`
	NameBare string `xml:"name,attr"`
}

type xmlManifest struct {
	XMLName          xml.Name            `xml:"manifest"`
	Package          string              `xml:"package,attr"`
	UsesSdk          []xmlUsesSdk        `xml:"uses-sdk"`
	UsesPermissions  []xmlUsesPermission `xml:"uses-permission"`
}

// ParsePlainXMLManifest parses standard text AndroidManifest.xml.
func ParsePlainXMLManifest(data []byte) (*ManifestAuditResult, error) {
	var m xmlManifest
	decoder := xml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&m); err != nil {
		// Fallback regex parsing if namespace or formatting is slightly non-standard
		return parseManifestRegex(string(data))
	}

	res := &ManifestAuditResult{
		PackageName:    m.Package,
		Permissions:    make([]PermissionAudit, 0),
		RawPermissions: make([]string, 0),
	}

	for _, sdk := range m.UsesSdk {
		tgt := sdk.TargetSdkVersion
		if tgt == "" {
			tgt = sdk.TargetSdkBare
		}
		if v, err := strconv.Atoi(strings.TrimSpace(tgt)); err == nil && v > res.TargetSdkVersion {
			res.TargetSdkVersion = v
		}

		min := sdk.MinSdkVersion
		if min == "" {
			min = sdk.MinSdkBare
		}
		if v, err := strconv.Atoi(strings.TrimSpace(min)); err == nil && v > res.MinSdkVersion {
			res.MinSdkVersion = v
		}
	}

	for _, p := range m.UsesPermissions {
		pName := p.Name
		if pName == "" {
			pName = p.NameBare
		}
		pName = strings.TrimSpace(pName)
		if pName != "" {
			res.RawPermissions = append(res.RawPermissions, pName)
			if reg, ok := RestrictedPermissionsRegistry[pName]; ok {
				res.Permissions = append(res.Permissions, reg)
			} else {
				res.Permissions = append(res.Permissions, PermissionAudit{
					Name:        pName,
					IsRestricted: false,
					RiskLevel:   "NORMAL",
					Category:    "CUSTOM_OR_STANDARD",
				})
			}
		}
	}

	return res, nil
}

func parseManifestRegex(text string) (*ManifestAuditResult, error) {
	res := &ManifestAuditResult{
		Permissions:    make([]PermissionAudit, 0),
		RawPermissions: make([]string, 0),
	}

	pkgRe := regexp.MustCompile(`package\s*=\s*["']([^"']+)["']`)
	if match := pkgRe.FindStringSubmatch(text); len(match) > 1 {
		res.PackageName = match[1]
	}

	targetRe := regexp.MustCompile(`targetSdkVersion\s*=\s*["']?(\d+)["']?`)
	if match := targetRe.FindStringSubmatch(text); len(match) > 1 {
		res.TargetSdkVersion, _ = strconv.Atoi(match[1])
	}

	minRe := regexp.MustCompile(`minSdkVersion\s*=\s*["']?(\d+)["']?`)
	if match := minRe.FindStringSubmatch(text); len(match) > 1 {
		res.MinSdkVersion, _ = strconv.Atoi(match[1])
	}

	permRe := regexp.MustCompile(`uses-permission[^>]*name\s*=\s*["']([^"']+)["']`)
	matches := permRe.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) > 1 {
			pName := strings.TrimSpace(match[1])
			res.RawPermissions = append(res.RawPermissions, pName)
			if reg, ok := RestrictedPermissionsRegistry[pName]; ok {
				res.Permissions = append(res.Permissions, reg)
			} else {
				res.Permissions = append(res.Permissions, PermissionAudit{
					Name:        pName,
					IsRestricted: false,
					RiskLevel:   "NORMAL",
					Category:    "CUSTOM_OR_STANDARD",
				})
			}
		}
	}

	return res, nil
}

// ParseBinaryAXMLManifest handles compiled Android binary XML (AXML).
func ParseBinaryAXMLManifest(data []byte) (*ManifestAuditResult, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("AXML too short (%d bytes)", len(data))
	}

	magic := binary.LittleEndian.Uint32(data[0:4])
	if magic != 0x00080003 {
		return nil, fmt.Errorf("invalid AXML magic: 0x%08x", magic)
	}

	// Extract UTF-8 or UTF-16 strings from the string pool chunk
	stringsList := extractAXMLStrings(data)
	content := strings.Join(stringsList, " ")

	res := &ManifestAuditResult{
		Permissions:    make([]PermissionAudit, 0),
		RawPermissions: make([]string, 0),
	}

	// Look for package name and permissions in extracted strings
	for _, s := range stringsList {
		if strings.HasPrefix(s, "android.permission.") {
			res.RawPermissions = append(res.RawPermissions, s)
			if reg, ok := RestrictedPermissionsRegistry[s]; ok {
				res.Permissions = append(res.Permissions, reg)
			} else {
				res.Permissions = append(res.Permissions, PermissionAudit{
					Name:        s,
					IsRestricted: false,
					RiskLevel:   "NORMAL",
					Category:    "CUSTOM_OR_STANDARD",
				})
			}
		}
	}

	// Search for targetSdkVersion in strings or raw integers
	targetRe := regexp.MustCompile(`targetSdkVersion.*?(\d{2})`)
	if match := targetRe.FindStringSubmatch(content); len(match) > 1 {
		res.TargetSdkVersion, _ = strconv.Atoi(match[1])
	} else {
		// Scan for standard 36 or 37 values in raw bytes
		for i := 8; i < len(data)-4; i += 4 {
			val := binary.LittleEndian.Uint32(data[i : i+4])
			if val == 36 || val == 37 {
				res.TargetSdkVersion = int(val)
				break
			}
		}
	}

	return res, nil
}

func extractAXMLStrings(data []byte) []string {
	var result []string
	if len(data) < 20 {
		return result
	}

	// String pool header starts around offset 8
	chunkType := binary.LittleEndian.Uint16(data[8:10])
	if chunkType != 0x0001 { // RES_STRING_POOL_TYPE
		return result
	}

	stringCount := int(binary.LittleEndian.Uint32(data[16:20]))
	flags := binary.LittleEndian.Uint32(data[24:28])
	isUTF8 := (flags & (1 << 8)) != 0
	stringsStart := int(binary.LittleEndian.Uint32(data[28:32])) + 8

	offsetTableStart := 36
	if len(data) < offsetTableStart+stringCount*4 {
		return result
	}

	for i := 0; i < stringCount; i++ {
		off := int(binary.LittleEndian.Uint32(data[offsetTableStart+i*4 : offsetTableStart+(i+1)*4]))
		pos := stringsStart + off
		if pos >= len(data) {
			continue
		}

		if isUTF8 {
			// Read UTF-8 string
			if pos+1 >= len(data) {
				continue
			}
			length := int(data[pos+1])
			pos += 2
			if pos+length <= len(data) {
				str := string(data[pos : pos+length])
				if len(str) > 0 {
					result = append(result, str)
				}
			}
		} else {
			// Read UTF-16 string
			if pos+2 > len(data) {
				continue
			}
			length := int(binary.LittleEndian.Uint16(data[pos:pos+2])) * 2
			pos += 2
			if pos+length <= len(data) {
				u16 := make([]uint16, length/2)
				for j := 0; j < len(u16); j++ {
					u16[j] = binary.LittleEndian.Uint16(data[pos+j*2 : pos+(j+1)*2])
				}
				runes := make([]rune, len(u16))
				for j, u := range u16 {
					runes[j] = rune(u)
				}
				str := string(runes)
				if len(str) > 0 {
					result = append(result, str)
				}
			}
		}
	}

	return result
}

// AuditManifest auto-detects XML format (binary vs plain text) and parses it.
func AuditManifest(data []byte) (*ManifestAuditResult, error) {
	if len(data) >= 4 && binary.LittleEndian.Uint32(data[0:4]) == 0x00080003 {
		return ParseBinaryAXMLManifest(data)
	}
	return ParsePlainXMLManifest(data)
}

// AuditAPK performs end-to-end 16 KB ELF and Android 17 compliance audit on an APK.
func AuditAPK(apkPath string) (*ComplianceReport, error) {
	r, err := zip.OpenReader(apkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open APK file as zip: %w", err)
	}
	defer r.Close()

	report := &ComplianceReport{
		IsCompliant:     true,
		Android17Ready:  true,
		NativeLibraries: make([]SOAuditResult, 0),
		Permissions:     make([]PermissionAudit, 0),
		Violations:      make([]ComplianceViolation, 0),
	}

	// 1. Audit AndroidManifest.xml
	var manifestFound bool
	for _, f := range r.File {
		if f.Name == "AndroidManifest.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to read AndroidManifest.xml: %w", err)
			}
			manifestBytes, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to load AndroidManifest.xml: %w", err)
			}

			manifestAudit, err := AuditManifest(manifestBytes)
			if err != nil {
				return nil, fmt.Errorf("manifest audit error: %w", err)
			}

			report.PackageName = manifestAudit.PackageName
			report.TargetSdkVersion = manifestAudit.TargetSdkVersion
			report.MinSdkVersion = manifestAudit.MinSdkVersion
			report.Permissions = manifestAudit.Permissions
			manifestFound = true
			break
		}
	}

	if !manifestFound {
		report.Violations = append(report.Violations, ComplianceViolation{
			Category:    "TARGET_SDK",
			Target:      "AndroidManifest.xml",
			Description: "AndroidManifest.xml not found in APK archive.",
			Severity:    "BLOCKER",
		})
		report.IsCompliant = false
		report.Android17Ready = false
	} else {
		// Verify Target SDK >= 37 for Android 17 compliance
		if report.TargetSdkVersion < TargetAPI37 {
			report.Android17Ready = false
			severity := "BLOCKER"
			if report.TargetSdkVersion >= TargetAPI36 {
				severity = "WARNING"
			} else {
				report.IsCompliant = false
			}
			report.Violations = append(report.Violations, ComplianceViolation{
				Category: "TARGET_SDK",
				Target:   "targetSdkVersion",
				Description: fmt.Sprintf(
					"targetSdkVersion is %d; Android 17 requires targetSdkVersion >= 37 (minimum >= 36).",
					report.TargetSdkVersion,
				),
				Severity: severity,
			})
		}

		// Audit restricted permissions
		for _, perm := range report.Permissions {
			if perm.IsRestricted {
				report.Violations = append(report.Violations, ComplianceViolation{
					Category:    "RESTRICTED_PERMISSION",
					Target:      perm.Name,
					Description: fmt.Sprintf("Restricted permission [%s]: %s", perm.Name, perm.Remediation),
					Severity:    "WARNING",
				})
			}
		}
	}

	// 2. Audit all native .so libraries in lib/
	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "lib/") && strings.HasSuffix(f.Name, ".so") {
			// Check zip container entry alignment if stored uncompressed
			if f.Method == zip.Store {
				dataOffset, err := f.DataOffset()
				if err == nil {
					isZipAligned := (dataOffset % int64(PageSize16KB)) == 0
					zipAudit := ZipNativeLibAudit{
						EntryName:     f.Name,
						Compression:   "STORED",
						DataOffset:    dataOffset,
						OffsetHex:     fmt.Sprintf("0x%x", dataOffset),
						Is16KBAligned: isZipAligned,
					}
					if !isZipAligned {
						zipAudit.Remediation = fmt.Sprintf("Run 'zipalign -p -f 16 input.apk output.apk' to align %s at 16 KB boundary", f.Name)
						report.Violations = append(report.Violations, ComplianceViolation{
							Category:    "ZIPALIGN_16KB",
							Target:      f.Name,
							Description: fmt.Sprintf("Uncompressed native library %s has unaligned zip offset 0x%x (requires 16 KB / 0x4000 alignment)", f.Name, dataOffset),
							Severity:    "BLOCKER",
							Remediation: zipAudit.Remediation,
						})
						report.IsCompliant = false
					}
					report.ZipAlignments = append(report.ZipAlignments, zipAudit)
				}
			} else {
				report.ZipAlignments = append(report.ZipAlignments, ZipNativeLibAudit{
					EntryName:     f.Name,
					Compression:   "DEFLATED",
					Is16KBAligned: true, // extracted by package installer
				})
			}

			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open library %s: %w", f.Name, err)
			}
			soBytes, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to read library %s: %w", f.Name, err)
			}

			soAudit, err := AuditSOReader(bytes.NewReader(soBytes), f.Name)
			if err != nil {
				report.Violations = append(report.Violations, ComplianceViolation{
					Category:    "ELF_16KB",
					Target:      f.Name,
					Description: fmt.Sprintf("ELF parsing failed: %v", err),
					Severity:    "BLOCKER",
				})
				report.IsCompliant = false
				continue
			}

			report.NativeLibraries = append(report.NativeLibraries, *soAudit)

			if !soAudit.Is16KBAligned {
				report.IsCompliant = false
				report.Violations = append(report.Violations, ComplianceViolation{
					Category: "ELF_16KB",
					Target:   f.Name,
					Description: fmt.Sprintf(
						"Native library %s is not 16 KB page-aligned (max alignment: 0x%x; requires 0x4000).",
						f.Name, soAudit.MaxAlignment,
					),
					Severity: "BLOCKER",
					Remediation: "Recompile with Android NDK r27+ using linker flag '-Wl,-z,max-page-size=16384'",
				})
			}
		}
	}

	// Summary generation
	if report.IsCompliant && report.Android17Ready {
		report.Summary = fmt.Sprintf(
			"PASS: APK %s satisfies Google Play 2027 standards (targetSdkVersion=%d, %d native libs verified 16 KB aligned).",
			report.PackageName, report.TargetSdkVersion, len(report.NativeLibraries),
		)
	} else {
		report.Summary = fmt.Sprintf(
			"FAIL: APK %s has %d compliance violations (targetSdkVersion=%d, %d native libraries audited).",
			report.PackageName, len(report.Violations), report.TargetSdkVersion, len(report.NativeLibraries),
		)
	}

	return report, nil
}
