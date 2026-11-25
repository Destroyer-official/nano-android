package types

// SMSInjectResult holds the outcome of injecting an SMS or call on an emulator.
type SMSInjectResult struct {
	Injected             bool  `json:"injected"`
	TimestampMS          int64 `json:"timestamp_ms"`
	Android17OTPDetected bool  `json:"android17_otp_pattern_detected"`
}

// NotificationPostResult holds the outcome of posting a test notification.
type NotificationPostResult struct {
	Posted bool   `json:"posted"`
	Method string `json:"method"` // "cmd_notification" | "instrumentation"
}

// PermissionSetResult holds the outcome of modifying app permissions.
type PermissionSetResult struct {
	Applied []string `json:"applied,omitempty"`
	Errors  []string `json:"errors,omitempty"`
}

// MediaVerifyResult holds the outcome of pushing and verifying media fixtures.
type MediaVerifyResult struct {
	PushedCached bool           `json:"pushed_cached"`
	OutputSHA256 string         `json:"output_sha256,omitempty"`
	HashMatch    bool           `json:"hash_match"`
	FFProbeMeta  map[string]any `json:"ffprobe_meta,omitempty"`
	VMAF         float64        `json:"vmaf,omitempty"`
	SSIM         float64        `json:"ssim,omitempty"`
	DecodeErrors int            `json:"decode_errors"`
}

// PerfTraceResult holds captured performance metrics from a Perfetto trace.
type PerfTraceResult struct {
	TracePath           string  `json:"trace_path"`
	P95FrameMS          float64 `json:"p95_frame_ms"`
	JankPct             float64 `json:"jank_pct"`
	PeakRSSMB           float64 `json:"peak_rss_mb"`
	PeakAnonSwapMB      float64 `json:"peak_anonswap_mb"`
	MemoryLimiterEvents int     `json:"memory_limiter_events"`
}

// ThermalPowerResult holds thermal and power state readings.
type ThermalPowerResult struct {
	ThermalStatus  string             `json:"thermal_status"`
	SkinTempC      float64            `json:"skin_temp_c"`
	BatteryTempC   float64            `json:"battery_temp_c"`
	ChargeDeltaMAh float64            `json:"charge_delta_mah,omitempty"`
	ODPMRailsMWh   map[string]float64 `json:"odpm_rails_mwh,omitempty"`
}

// LogcatReadResult holds filtered logcat output.
type LogcatReadResult struct {
	Nonce     string   `json:"nonce"`
	Lines     []string `json:"lines"`
	Truncated bool     `json:"truncated"`
}

// CrashReport holds collected crash and ANR information.
type CrashReport struct {
	Nonce       string       `json:"nonce,omitempty"`
	Crashes     []CrashEntry `json:"crashes,omitempty"`
	ANRs        []CrashEntry `json:"anrs,omitempty"`
	ExitReasons []ExitReason `json:"exit_reasons,omitempty"`
	BundlePath  string       `json:"bundle_path,omitempty"`
}

// CrashEntry is a single crash or ANR event.
type CrashEntry struct {
	Exception   string `json:"exception"`
	Message     string `json:"message"`
	StackTrace  string `json:"stack_trace"`
	TimestampMS int64  `json:"timestamp_ms"`
}

// ExitReason is a structured app exit event (including MemoryLimiter:AnonSwap).
type ExitReason struct {
	Reason      string `json:"reason"`
	Description string `json:"description"`
	TimestampMS int64  `json:"timestamp_ms"`
}

// AppStateResult holds structured dumpsys output for app state inspection.
type AppStateResult struct {
	Nonce           string           `json:"nonce,omitempty"`
	RunningServices []string         `json:"running_services,omitempty"`
	GrantedPerms    []string         `json:"granted_permissions,omitempty"`
	DeniedPerms     []string         `json:"denied_permissions,omitempty"`
	StandbyBucket   string           `json:"standby_bucket,omitempty"`
	WorkManagerJobs []WorkManagerJob `json:"workmanager_jobs,omitempty"`
}

// WorkManagerJob describes one scheduled WorkManager job.
type WorkManagerJob struct {
	ID    string   `json:"id"`
	State string   `json:"state"`
	Tags  []string `json:"tags,omitempty"`
}
