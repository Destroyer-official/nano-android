package types

// DroidForgeConfig is the parsed representation of droidforge.yaml.
type DroidForgeConfig struct {
	SchemaVersion int               `yaml:"schema_version" json:"schema_version"`
	Project       ProjectConfig     `yaml:"project" json:"project"`
	Execution     ExecutionConfig   `yaml:"execution" json:"execution"`
	Observation   ObservationConfig `yaml:"observation" json:"observation"`
	Workloads     WorkloadConfig    `yaml:"workloads" json:"workloads"`
}

// ProjectConfig defines the target app boundaries.
type ProjectConfig struct {
	Name                string   `yaml:"name" json:"name"`
	Mode                string   `yaml:"mode" json:"mode"` // "source" | "apk-only"
	GradleRoot          string   `yaml:"gradle_root" json:"gradle_root"`
	DefaultVariant      string   `yaml:"default_variant" json:"default_variant"`
	AllowlistedPackages []string `yaml:"allowlisted_packages" json:"allowlisted_packages"`
}

// ExecutionConfig controls runtime behavior and resource limits.
type ExecutionConfig struct {
	PreferTier0JVM       bool   `yaml:"prefer_tier0_jvm" json:"prefer_tier0_jvm"`
	DefaultAVD           string `yaml:"default_avd" json:"default_avd"`
	GoldenSnapshot       string `yaml:"golden_snapshot" json:"golden_snapshot"`
	AVDRamMB             int    `yaml:"avd_ram_mb" json:"avd_ram_mb"`
	AVDIdleSuspendMin    int    `yaml:"avd_idle_suspend_min" json:"avd_idle_suspend_min"`
	GradleMaxHeapMB      int    `yaml:"gradle_max_heap_mb" json:"gradle_max_heap_mb"`
	MaxStepsPerTask      int    `yaml:"max_steps_per_task" json:"max_steps_per_task"`
	MaxIdenticalUIHashes int    `yaml:"max_identical_ui_hashes" json:"max_identical_ui_hashes"`
	SequentialAVDMode    bool   `yaml:"sequential_avd_mode" json:"sequential_avd_mode"`
}

// ObservationConfig controls how the AI sees device state.
type ObservationConfig struct {
	DefaultMode               string   `yaml:"default_mode" json:"default_mode"` // "pruned_tree_diff" | "hybrid_som"
	AttachScreenshotOn        []string `yaml:"attach_screenshot_on" json:"attach_screenshot_on"`
	ScreenshotMaxHeight       int      `yaml:"screenshot_max_height" json:"screenshot_max_height"`
	ScreenshotFormat          string   `yaml:"screenshot_format" json:"screenshot_format"`
	ScreenshotQuality         int      `yaml:"screenshot_quality" json:"screenshot_quality"`
	SettleTimeoutMS           int      `yaml:"settle_timeout_ms" json:"settle_timeout_ms"`
	RedactStatusBarOnPhysical bool     `yaml:"redact_status_bar_on_physical" json:"redact_status_bar_on_physical"`
}

// WorkloadConfig holds workload-specific test parameters.
type WorkloadConfig struct {
	Profile        string            `yaml:"profile" json:"profile"` // "standard" | "scam-shield" | "video-codec"
	MockServerPort int               `yaml:"mock_server_port" json:"mock_server_port"`
	ScamShield     *ScamShieldConfig `yaml:"scam_shield,omitempty" json:"scam_shield,omitempty"`
	VideoCodec     *VideoCodecConfig `yaml:"video_codec,omitempty" json:"video_codec,omitempty"`
}

// ScamShieldConfig has test parameters specific to the Family Scam Shield workload.
type ScamShieldConfig struct {
	CheckAndroid17OTPDelay   bool     `yaml:"check_android17_otp_delay" json:"check_android17_otp_delay"`
	PlayPolicyJustification  string   `yaml:"play_policy_justification" json:"play_policy_justification"`
	ForbiddenUndeclaredPerms []string `yaml:"forbidden_undeclared_permissions" json:"forbidden_undeclared_permissions"`
}

// VideoCodecConfig has test parameters specific to the Video Compression workload.
type VideoCodecConfig struct {
	FixturesManifest        string  `yaml:"fixtures_manifest" json:"fixtures_manifest"`
	DeviceFixtureDir        string  `yaml:"device_fixture_dir" json:"device_fixture_dir"`
	OracleMode              string  `yaml:"oracle_mode" json:"oracle_mode"` // "bit_exact" | "quality_bounded"
	VMAFMin                 float64 `yaml:"vmaf_min" json:"vmaf_min"`
	SSIMMin                 float64 `yaml:"ssim_min" json:"ssim_min"`
	MaxStartSkinTempC       float64 `yaml:"max_start_skin_temp_c" json:"max_start_skin_temp_c"`
	Enforce16KBElfAlignment bool    `yaml:"enforce_16kb_elf_alignment" json:"enforce_16kb_elf_alignment"`
}

// PolicyConfig is the parsed representation of droidforge.policy.json.
type PolicyConfig struct {
	SchemaVersion int                `json:"schema_version"`
	Network       NetworkPolicy      `json:"network"`
	Execution     ExecutionPolicy    `json:"execution"`
	Sanitization  SanitizationPolicy `json:"sanitization"`
	HumanApproval []string           `json:"human_approval_required"`
	Audit         AuditPolicy        `json:"audit"`
}

// NetworkPolicy controls ADB network exposure.
type NetworkPolicy struct {
	BindHost                  string `json:"bind_host"`
	AllowUnencryptedADBTcpip  bool   `json:"allow_unencrypted_adb_tcpip_5555"`
	AllowTLSWirelessDebugging bool   `json:"allow_tls_wireless_debugging"`
}

// ExecutionPolicy controls command execution safety.
type ExecutionPolicy struct {
	AllowRawShell              bool     `json:"allow_raw_shell"`
	EnforcePackageScopeLock    bool     `json:"enforce_package_scope_lock"`
	AllowContactsSMSOnPhysical bool     `json:"allow_contacts_sms_read_on_physical"`
	BlockedBinaries            []string `json:"blocked_binaries"`
	AllowedSettingsKeys        []string `json:"allowed_settings_keys"`
}

// SanitizationPolicy controls untrusted device text handling.
type SanitizationPolicy struct {
	WrapWithRandomNonce   bool `json:"wrap_device_text_with_random_nonce"`
	StripZeroWidthAndBidi bool `json:"strip_zero_width_and_bidi_chars"`
	MaxNodeTextBytes      int  `json:"max_node_text_bytes"`
	MaxLogcatLineBytes    int  `json:"max_logcat_line_bytes"`
}

// AuditPolicy controls forensic logging.
type AuditPolicy struct {
	LogDir        string `json:"log_dir"`
	RetentionDays int    `json:"retention_days"`
}
