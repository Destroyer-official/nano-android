package types

// BuildResult holds the outcome of a Gradle build.
type BuildResult struct {
	Status     string         `json:"status"` // "success" | "failure"
	DurationMS int64          `json:"duration_ms"`
	APKPath    string         `json:"apk_path,omitempty"`
	APKSHA256  string         `json:"apk_sha256,omitempty"`
	Errors     []CompileError `json:"compiler_errors,omitempty"`
}

// CompileError is a single Gradle/Kotlin compiler error.
type CompileError struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"msg"`
}

// TestResult holds the outcome of running JVM tests.
type TestResult struct {
	Passed     int           `json:"passed"`
	Failed     int           `json:"failed"`
	Skipped    int           `json:"skipped"`
	DurationMS int64         `json:"duration_ms"`
	Failures   []TestFailure `json:"failures,omitempty"`
}

// TestFailure describes one failed test case.
type TestFailure struct {
	Class       string `json:"class"`
	Method      string `json:"method"`
	StackTrace  string `json:"stack"`
	DiffImgPath string `json:"diff_img_path,omitempty"` // Roborazzi visual diff
}

// APKInfo holds static analysis results from apkanalyzer/aapt2/apksigner/jadx.
type APKInfo struct {
	PackageID          string              `json:"package_id"`
	VersionCode        int                 `json:"version_code"`
	VersionName        string              `json:"version_name"`
	MinSDK             int                 `json:"min_sdk"`
	TargetSDK          int                 `json:"target_sdk"`
	LauncherActivity   string              `json:"launcher_activity"`
	ABIs               []string            `json:"abis"`
	Permissions        []string            `json:"permissions"`
	ExportedComponents []ExportedComponent `json:"exported_components,omitempty"`
	SigningCerts       []string            `json:"signing_certs,omitempty"`
	DeepLinkSchemes    []string            `json:"deep_link_schemes,omitempty"`
}

// ExportedComponent is an exported Activity, Service, or Receiver.
type ExportedComponent struct {
	Type string `json:"type"` // "activity", "service", "receiver", "provider"
	Name string `json:"name"`
}

// ComplianceResult holds Play policy and Android 17 audit findings.
type ComplianceResult struct {
	Pass                 bool                `json:"pass"`
	ELF16KBAligned       bool                `json:"elf_16kb_aligned"`
	LargeScreenResizable bool                `json:"large_screen_resizable"`
	PlayPolicyFindings   []ComplianceFinding `json:"play_policy_findings,omitempty"`
	Android17Findings    []ComplianceFinding `json:"android17_findings,omitempty"`
}

// ComplianceFinding is one policy or compatibility issue.
type ComplianceFinding struct {
	Severity string `json:"severity"` // "error", "warning", "info"
	Rule     string `json:"rule"`
	Message  string `json:"message"`
	Detail   string `json:"detail,omitempty"`
}

// BuildEngine wraps Gradle, apkanalyzer, jadx, and compliance checks.
type BuildEngine interface {
	// Build triggers a Gradle build and returns the APK path and metadata.
	Build(variant, module string, clean bool) (*BuildResult, error)
	// TestJVM runs Robolectric/Compose JVM tests and parses results.
	TestJVM(filter string, roborazzi, recordGoldens bool) (*TestResult, error)
	// InspectAPK performs static analysis on an APK without source code.
	InspectAPK(apkPath string, decompileClasses []string) (*APKInfo, error)
	// AuditCompliance checks Play policy and Android 17 compatibility.
	AuditCompliance(apkPath, profile string) (*ComplianceResult, error)
}
