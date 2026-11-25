package types

// DeviceInfo describes a connected Android device or available AVD.
type DeviceInfo struct {
	Serial        string  `json:"serial"`
	State         string  `json:"state"` // "device", "offline", "unauthorized", "avd_available"
	Model         string  `json:"model,omitempty"`
	APILevel      int     `json:"api_level,omitempty"`
	ABI           string  `json:"abi,omitempty"`
	IsEmulator    bool    `json:"is_emulator"`
	ApprovedClass string  `json:"approved_class,omitempty"` // "emulator", "dedicated_test"
	BatteryPct    int     `json:"battery_pct,omitempty"`
	TempC         float64 `json:"temp_c,omitempty"`
}

// AppLaunchResult holds the outcome of installing and launching an APK.
type AppLaunchResult struct {
	PackageID   string `json:"package_id"`
	PID         int    `json:"pid"`
	InstallMS   int64  `json:"install_ms"`
	ColdStartMS int64  `json:"cold_start_ms"`
	Activity    string `json:"activity"`
}

// DeviceManager handles ADB commands, AVD lifecycle, and emulator management.
type DeviceManager interface {
	// ListDevices returns connected devices and available AVDs.
	ListDevices(includeAVDs bool) ([]DeviceInfo, error)

	// BootSnapshot starts an AVD and loads a named snapshot.
	BootSnapshot(avdName, snapshot string, headless bool, ramMB int) (*DeviceInfo, error)

	// ResetState restores device state via snapshot load or pm clear.
	ResetState(serial, strategy, pkgID, snapshotName string) error

	// InstallAndLaunch installs an APK and launches the specified activity.
	InstallAndLaunch(serial, apkPath, activity, deepLink string, grantPerms bool) (*AppLaunchResult, error)

	// ExecADB runs a safe ADB command (never shell-interpolated).
	ExecADB(serial string, args ...string) (string, error)

	// ShutdownEmulator gracefully shuts down an emulator.
	ShutdownEmulator(serial string) error

	// EmulatorConsole sends a command to the emulator telnet console.
	EmulatorConsole(serial, command string) (string, error)
}
