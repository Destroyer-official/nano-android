package device

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

var (
	pkgNameRegex   = regexp.MustCompile(`^[a-zA-Z0-9_]+(\.[a-zA-Z0-9_]+)+$`)
	totalTimeRegex = regexp.MustCompile(`(?i)TotalTime:\s*(\d+)`)
	waitTimeRegex  = regexp.MustCompile(`(?i)WaitTime:\s*(\d+)`)
	thisTimeRegex  = regexp.MustCompile(`(?i)ThisTime:\s*(\d+)`)
	batteryLvlRe   = regexp.MustCompile(`(?i)^\s*level:\s*(\d+)`)
	batteryTempRe  = regexp.MustCompile(`(?i)^\s*temperature:\s*(\d+)`)

	// System packages that can NEVER be cleared or mutated for safety.
	protectedPackages = map[string]bool{
		"android":                        true,
		"com.android.systemui":           true,
		"com.android.settings":           true,
		"com.android.shell":              true,
		"com.android.providers.settings": true,
		"com.google.android.gms":         true,
		"com.google.android.gsf":         true,
	}
)

// Manager implements types.DeviceManager, managing device discovery,
// headless AVD snapshot lifecycle, state resets, and app installation.
type Manager struct {
	adb                 *ADB
	emulatorPath        string
	emulatorOnly        bool
	allowlistedPackages map[string]bool
	deviceLocks         sync.Map // serial -> *sync.Mutex
	mu                  sync.RWMutex
	runningEmulators    map[string]*os.Process // avdName -> process
}

// ManagerOption configures a Manager instance.
type ManagerOption func(*Manager)

// WithADB sets a custom ADB driver.
func WithADB(adb *ADB) ManagerOption {
	return func(m *Manager) {
		if adb != nil {
			m.adb = adb
		}
	}
}

// WithEmulatorPath sets the custom path to the emulator binary.
func WithEmulatorPath(path string) ManagerOption {
	return func(m *Manager) {
		m.emulatorPath = path
	}
}

// WithEmulatorOnly restricts device discovery to virtual devices only.
func WithEmulatorOnly(emulatorOnly bool) ManagerOption {
	return func(m *Manager) {
		m.emulatorOnly = emulatorOnly
	}
}

// WithAllowlistedPackages sets the permitted package names for mutation.
func WithAllowlistedPackages(pkgs []string) ManagerOption {
	return func(m *Manager) {
		m.allowlistedPackages = make(map[string]bool, len(pkgs))
		for _, p := range pkgs {
			p = strings.TrimSpace(p)
			if p != "" {
				m.allowlistedPackages[p] = true
			}
		}
	}
}

// NewDeviceManager creates an initialized DeviceManager instance.
func NewDeviceManager(opts ...ManagerOption) *Manager {
	m := &Manager{
		adb:                 GetDefaultADB(),
		emulatorPath:        resolveEmulatorPath(""),
		runningEmulators:    make(map[string]*os.Process),
		allowlistedPackages: make(map[string]bool),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// lockDevice acquires a per-device mutex to prevent concurrent mutations on the same hardware/AVD.
func (m *Manager) lockDevice(serial string) func() {
	val, _ := m.deviceLocks.LoadOrStore(serial, &sync.Mutex{})
	mtx := val.(*sync.Mutex)
	mtx.Lock()
	return mtx.Unlock
}

// ListDevices returns all connected Android devices (USB, Wi-Fi, emulators)
// and optionally lists offline AVDs available to boot.
func (m *Manager) ListDevices(includeAVDs bool) ([]types.DeviceInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1. Query adb devices -l
	out, err := m.adb.ExecContext(ctx, "", "devices", "-l")
	if err != nil {
		return nil, fmt.Errorf("failed executing adb devices -l: %w", err)
	}

	devices, err := ParseADBDevicesList(out)
	if err != nil {
		return nil, fmt.Errorf("failed parsing adb devices output: %w", err)
	}

	var result []types.DeviceInfo
	for _, dev := range devices {
		// If emulator-only mode is active, skip physical devices
		if m.emulatorOnly && !dev.IsEmulator {
			continue
		}

		// For online devices, query hardware/OS details
		if dev.State == "device" {
			infoCtx, infoCancel := context.WithTimeout(context.Background(), 3*time.Second)
			dev = m.enrichDeviceInfo(infoCtx, dev)
			infoCancel()
		}

		result = append(result, dev)
	}

	// 2. Query available AVDs if requested
	if includeAVDs {
		avds, avdErr := m.listAvailableAVDs(ctx)
		if avdErr == nil {
			// Find names of AVDs already running to avoid duplicates
			runningNames := make(map[string]bool)
			for _, d := range result {
				if d.Model != "" {
					runningNames[d.Model] = true
				}
				runningNames[d.Serial] = true
			}

			for _, avdName := range avds {
				if runningNames[avdName] {
					continue
				}
				result = append(result, types.DeviceInfo{
					Serial:        avdName,
					State:         "avd_available",
					Model:         avdName,
					IsEmulator:    true,
					ApprovedClass: "emulator",
				})
			}
		}
	}

	return result, nil
}

// ParseADBDevicesList parses the raw output of "adb devices -l".
func ParseADBDevicesList(output string) ([]types.DeviceInfo, error) {
	var devices []types.DeviceInfo
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" ||
			strings.HasPrefix(line, "List of devices attached") ||
			strings.HasPrefix(line, "* daemon") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		serial := fields[0]
		state := fields[1]

		var model, product, abi string
		for _, field := range fields[2:] {
			parts := strings.SplitN(field, ":", 2)
			if len(parts) == 2 {
				k, v := parts[0], parts[1]
				switch k {
				case "model":
					model = v
				case "product":
					product = v
				case "abi":
					abi = v
				}
			}
		}

		isEmu := isEmulatorDevice(serial, model, product)
		approvedClass := "dedicated_test"
		if isEmu {
			approvedClass = "emulator"
		}

		devices = append(devices, types.DeviceInfo{
			Serial:        serial,
			State:         state,
			Model:         model,
			ABI:           abi,
			IsEmulator:    isEmu,
			ApprovedClass: approvedClass,
		})
	}

	return devices, scanner.Err()
}

// enrichDeviceInfo queries API level, ABI, and battery statistics for an active device.
func (m *Manager) enrichDeviceInfo(ctx context.Context, dev types.DeviceInfo) types.DeviceInfo {
	// Query API Level
	if apiStr, err := m.adb.GetProp(ctx, dev.Serial, "ro.build.version.sdk"); err == nil {
		if apiLvl, parseErr := strconv.Atoi(apiStr); parseErr == nil {
			dev.APILevel = apiLvl
		}
	}

	// Query ABI if not already parsed
	if dev.ABI == "" {
		if abiStr, err := m.adb.GetProp(ctx, dev.Serial, "ro.product.cpu.abi"); err == nil {
			dev.ABI = abiStr
		}
	}

	// Query Model if not already parsed
	if dev.Model == "" {
		if modelStr, err := m.adb.GetProp(ctx, dev.Serial, "ro.product.model"); err == nil {
			dev.Model = modelStr
		}
	}

	// Query Battery & Temperature
	if batteryOut, err := m.adb.Shell(ctx, dev.Serial, "dumpsys", "battery"); err == nil {
		level, temp := parseBatteryDump(batteryOut)
		if level >= 0 {
			dev.BatteryPct = level
		}
		if temp > 0 {
			dev.TempC = temp
		}
	}

	return dev
}

// parseBatteryDump parses "level: XX" and "temperature: XXX" from dumpsys battery.
func parseBatteryDump(output string) (level int, tempC float64) {
	level = -1
	tempC = 0.0

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if m := batteryLvlRe.FindStringSubmatch(line); len(m) == 2 {
			if l, err := strconv.Atoi(m[1]); err == nil {
				level = l
			}
		}
		if m := batteryTempRe.FindStringSubmatch(line); len(m) == 2 {
			if t, err := strconv.Atoi(m[1]); err == nil {
				// Android dumpsys battery temperature is reported in tenths of a degree Celsius (e.g. 299 -> 29.9 C)
				tempC = float64(t) / 10.0
			}
		}
	}
	return level, tempC
}

// listAvailableAVDs runs "emulator -list-avds" and returns the names.
func (m *Manager) listAvailableAVDs(ctx context.Context) ([]string, error) {
	if m.emulatorPath == "" {
		return nil, errors.New("emulator binary not found")
	}

	cmd := exec.CommandContext(ctx, m.emulatorPath, "-list-avds")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("emulator -list-avds failed: %w", err)
	}

	var avds []string
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		avd := strings.TrimSpace(scanner.Text())
		if avd != "" {
			avds = append(avds, avd)
		}
	}
	return avds, scanner.Err()
}

// BootSnapshot starts a headless AVD, waits for boot completion, restores a named snapshot,
// and disables animations to optimize the environment for automated testing.
// If the AVD is already running under an active emulator serial, it reuses the instance safely.
func (m *Manager) BootSnapshot(avdName, snapshot string, headless bool, ramMB int) (*types.DeviceInfo, error) {
	avdName = strings.TrimSpace(avdName)
	if avdName == "" {
		return nil, errors.New("avdName cannot be empty")
	}
	if strings.Contains(avdName, "..") || strings.ContainsAny(avdName, `/\`) {
		return nil, fmt.Errorf("invalid AVD name %q", avdName)
	}
	if ramMB <= 0 {
		ramMB = 4096
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 1. Check if an emulator for this AVD is already running
	existingDevices, _ := m.ListDevices(false)
	var alreadyRunningSerial string
	existingSerials := make(map[string]bool)

	for _, d := range existingDevices {
		existingSerials[d.Serial] = true
		if d.IsEmulator && (d.Model == avdName || strings.EqualFold(d.Model, avdName)) {
			alreadyRunningSerial = d.Serial
			break
		}
	}

	targetSerial := alreadyRunningSerial
	bootStart := time.Now()

	if targetSerial == "" {
		// 2. Launch emulator command
		args := []string{
			"-avd", avdName,
			"-gpu", "swiftshader_indirect",
		}
		if headless {
			args = append(args, "-no-window", "-no-audio")
		}
		args = append(args, "-memory", strconv.Itoa(ramMB))

		emuCmd := exec.Command(m.emulatorPath, args...)
		emuCmd.Stdout = nil
		emuCmd.Stderr = nil

		if err := emuCmd.Start(); err != nil {
			return nil, fmt.Errorf("failed to start emulator for AVD %q: %w", avdName, err)
		}

		m.mu.Lock()
		m.runningEmulators[avdName] = emuCmd.Process
		m.mu.Unlock()

		// 3. Poll adb devices until a new emulator serial appears
		pollDeadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(pollDeadline) {
			select {
			case <-ctx.Done():
				_ = emuCmd.Process.Kill()
				return nil, ctx.Err()
			default:
			}

			currentDevs, err := m.ListDevices(false)
			if err == nil {
				for _, d := range currentDevs {
					if d.IsEmulator && !existingSerials[d.Serial] {
						targetSerial = d.Serial
						break
					}
				}
			}

			if targetSerial != "" {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		if targetSerial == "" {
			_ = emuCmd.Process.Kill()
			return nil, fmt.Errorf("timed out waiting for AVD %q to appear in adb devices", avdName)
		}
	}

	unlock := m.lockDevice(targetSerial)
	defer unlock()

	// 4. Wait for adb wait-for-device
	if err := m.adb.WaitForDevice(ctx, targetSerial, 60*time.Second); err != nil {
		return nil, fmt.Errorf("device %s failed wait-for-device: %w", targetSerial, err)
	}

	// 5. If snapshot provided, restore it
	if snapshot != "" {
		if err := m.adb.EmuSnapshotLoad(ctx, targetSerial, snapshot); err != nil {
			return nil, fmt.Errorf("failed to load snapshot %q on %s: %w", snapshot, targetSerial, err)
		}
	} else {
		// Wait for sys.boot_completed
		if err := m.adb.WaitForBootComplete(ctx, targetSerial, 60*time.Second); err != nil {
			return nil, fmt.Errorf("device %s boot did not complete: %w", targetSerial, err)
		}
	}

	// 6. Disable animations & wake screen
	_ = m.adb.DisableAnimations(ctx, targetSerial)
	_ = m.adb.SetStayAwake(ctx, targetSerial, true)
	_ = m.adb.WakeUp(ctx, targetSerial)

	bootDuration := time.Since(bootStart)

	// 7. Enrich device info
	info := types.DeviceInfo{
		Serial:        targetSerial,
		State:         "device",
		Model:         avdName,
		IsEmulator:    true,
		ApprovedClass: "emulator",
	}
	info = m.enrichDeviceInfo(ctx, info)

	fmt.Fprintf(os.Stderr, "[device-manager] Ready AVD %s as %s in %v (API %d)\n",
		avdName, targetSerial, bootDuration, info.APILevel)

	return &info, nil
}

// ResetState restores device state via either snapshot load or pm clear.
func (m *Manager) ResetState(serial, strategy, pkgID, snapshotName string) error {
	serial = strings.TrimSpace(serial)
	if serial == "" {
		return errors.New("serial cannot be empty")
	}

	unlock := m.lockDevice(serial)
	defer unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch strategy {
	case "snapshot_restore":
		if snapshotName == "" {
			return errors.New("snapshotName is required for snapshot_restore strategy")
		}
		return m.adb.EmuSnapshotLoad(ctx, serial, snapshotName)

	case "pm_clear":
		pkgID = strings.TrimSpace(pkgID)
		if pkgID == "" {
			return errors.New("package_id cannot be empty for pm_clear strategy")
		}

		// Security: Validate package name format and reject protected system packages
		if err := m.validatePackageForMutation(pkgID); err != nil {
			return fmt.Errorf("package reset rejected: %w", err)
		}

		// Execute pm clear
		if err := m.adb.ClearPackage(ctx, serial, pkgID); err != nil {
			return err
		}

		// Clean up fixtures directory if present
		_ = m.adb.RemoveFixtures(ctx, serial, "")
		return nil

	default:
		return fmt.Errorf("unknown reset strategy %q: expected 'snapshot_restore' or 'pm_clear'", strategy)
	}
}

// validatePackageForMutation enforces security guardrails against destructive mutations.
func (m *Manager) validatePackageForMutation(pkgID string) error {
	if !pkgNameRegex.MatchString(pkgID) {
		return fmt.Errorf("invalid package identifier syntax %q", pkgID)
	}

	if protectedPackages[strings.ToLower(pkgID)] {
		return fmt.Errorf("package %q is a protected system package and cannot be modified", pkgID)
	}

	if len(m.allowlistedPackages) > 0 && !m.allowlistedPackages[pkgID] {
		return fmt.Errorf("package %q is not in the allowlisted packages scope", pkgID)
	}

	return nil
}

// InstallAndLaunch installs an APK (or split APK bundle), launches the activity or deep link,
// and measures installation duration and cold-start latency.
// If activity is omitted, it auto-detects package name and launcher activity from the APK binary XML.
func (m *Manager) InstallAndLaunch(serial, apkPath, activity, deepLink string, grantPerms bool) (*types.AppLaunchResult, error) {
	serial = strings.TrimSpace(serial)
	if serial == "" {
		return nil, errors.New("serial cannot be empty")
	}
	apkPath = strings.TrimSpace(apkPath)
	if apkPath == "" {
		return nil, errors.New("apkPath cannot be empty")
	}

	unlock := m.lockDevice(serial)
	defer unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// 1. Try to auto-detect metadata from APK if single file on disk
	var parsedManifest *APKManifest
	paths := strings.Split(apkPath, ",")
	firstPath := strings.TrimSpace(paths[0])

	if info, err := os.Stat(firstPath); err == nil && !info.IsDir() {
		if manifest, parseErr := ParseAPKFile(firstPath); parseErr == nil {
			parsedManifest = manifest
		}
	}

	// 2. Extract or infer package ID and launch activity
	pkgID := extractPackageFromActivity(activity)
	if pkgID == "" && parsedManifest != nil {
		pkgID = parsedManifest.PackageID
	}

	if activity == "" && parsedManifest != nil && parsedManifest.Component != "" {
		activity = parsedManifest.Component
	}

	if pkgID == "" && deepLink == "" {
		return nil, errors.New("unable to determine package_id; please specify activity or deep_link")
	}

	// 3. Install APK(s)
	var installDuration time.Duration
	var err error

	if len(paths) > 1 {
		var trimmedPaths []string
		for _, p := range paths {
			trimmedPaths = append(trimmedPaths, strings.TrimSpace(p))
		}
		installDuration, err = m.adb.InstallMultiple(ctx, serial, trimmedPaths)
	} else {
		installDuration, err = m.adb.Install(ctx, serial, firstPath)
	}

	if err != nil {
		return nil, fmt.Errorf("installation failed: %w", err)
	}

	// 4. Grant runtime permissions if requested
	if grantPerms && pkgID != "" {
		permsToGrant := []string{
			"android.permission.POST_NOTIFICATIONS",
			"android.permission.READ_SMS",
			"android.permission.RECEIVE_SMS",
			"android.permission.ACCESS_FINE_LOCATION",
			"android.permission.ACCESS_COARSE_LOCATION",
		}
		if parsedManifest != nil && len(parsedManifest.Permissions) > 0 {
			permsToGrant = append(permsToGrant, parsedManifest.Permissions...)
		}
		m.grantPermissions(ctx, serial, pkgID, permsToGrant)
	}

	// 5. Wake screen and clear logcat before launch
	_ = m.adb.WakeUp(ctx, serial)
	_, _ = m.adb.ExecContext(ctx, serial, "logcat", "-c")

	// 6. Launch activity or deep link with -W (wait for launch completion)
	var launchArgs []string
	launchArgs = append(launchArgs, "am", "start", "-W")

	if deepLink != "" {
		launchArgs = append(launchArgs, "-d", deepLink)
	} else if activity != "" {
		launchArgs = append(launchArgs, "-n", activity)
	} else {
		// Fallback: launch by package using monkey
		launchArgs = []string{"monkey", "-p", pkgID, "-c", "android.intent.category.LAUNCHER", "1"}
	}

	launchStart := time.Now()
	launchOut, launchErr := m.adb.Shell(ctx, serial, launchArgs...)
	wallColdStartMS := time.Since(launchStart).Milliseconds()

	if launchErr != nil {
		return nil, fmt.Errorf("app launch failed: %w (output: %s)", launchErr, launchOut)
	}

	// Parse reported TotalTime, WaitTime, or ThisTime
	reportedColdStartMS := parseColdStartTime(launchOut)
	coldStartMS := wallColdStartMS
	if reportedColdStartMS > 0 {
		coldStartMS = reportedColdStartMS
	}

	// 7. Capture PID
	pid := -1
	if pkgID != "" {
		if p, err := m.adb.PidOf(ctx, serial, pkgID); err == nil && p > 0 {
			pid = p
		}
	}

	return &types.AppLaunchResult{
		PackageID:   pkgID,
		PID:         pid,
		InstallMS:   installDuration.Milliseconds(),
		ColdStartMS: coldStartMS,
		Activity:    activity,
	}, nil
}

// grantPermissions grants a set of permissions to the target package.
func (m *Manager) grantPermissions(ctx context.Context, serial, pkg string, perms []string) {
	seen := make(map[string]bool)
	for _, perm := range perms {
		perm = strings.TrimSpace(perm)
		if perm != "" && !seen[perm] {
			seen[perm] = true
			_ = m.adb.GrantPermission(ctx, serial, pkg, perm)
		}
	}
}

// ExecADB runs a safe ADB command using direct argument slices.
func (m *Manager) ExecADB(serial string, args ...string) (string, error) {
	return m.adb.Exec(serial, args...)
}

// ShutdownEmulator gracefully shuts down the emulator instance.
func (m *Manager) ShutdownEmulator(serial string) error {
	unlock := m.lockDevice(serial)
	defer unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := m.adb.ExecContext(ctx, serial, "emu", "kill")
	return err
}

// EmulatorConsole sends an arbitrary command to the emulator console.
func (m *Manager) EmulatorConsole(serial, command string) (string, error) {
	client, err := NewEmulatorConsoleClient(serial)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return client.Exec(ctx, command)
}

// extractPackageFromActivity extracts package name from "com.example/.MainActivity" or "com.example/com.example.MainActivity".
func extractPackageFromActivity(act string) string {
	act = strings.TrimSpace(act)
	if idx := strings.Index(act, "/"); idx != -1 {
		return strings.TrimSpace(act[:idx])
	}
	// If no slash, check if it's already a package name
	if pkgNameRegex.MatchString(act) {
		return act
	}
	return ""
}

// parseColdStartTime extracts TotalTime, WaitTime, or ThisTime from `am start -W` output.
func parseColdStartTime(output string) int64 {
	if m := totalTimeRegex.FindStringSubmatch(output); len(m) == 2 {
		if val, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			return val
		}
	}
	if m := waitTimeRegex.FindStringSubmatch(output); len(m) == 2 {
		if val, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			return val
		}
	}
	if m := thisTimeRegex.FindStringSubmatch(output); len(m) == 2 {
		if val, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			return val
		}
	}
	return -1
}

// isEmulatorDevice detects whether a device is an emulator or virtual container.
func isEmulatorDevice(serial, model, product string) bool {
	serialLower := strings.ToLower(serial)
	if strings.HasPrefix(serialLower, "emulator-") ||
		strings.HasPrefix(serialLower, "127.0.0.1:") ||
		strings.HasPrefix(serialLower, "localhost:") {
		return true
	}

	modelLower := strings.ToLower(model)
	productLower := strings.ToLower(product)

	if strings.Contains(modelLower, "sdk") ||
		strings.Contains(modelLower, "emulator") ||
		strings.Contains(productLower, "sdk") ||
		strings.Contains(productLower, "emulator") {
		return true
	}

	return false
}

// resolveEmulatorPath looks for emulator binary in PATH or Android SDK.
func resolveEmulatorPath(custom string) string {
	if custom != "" {
		if isExecutable(custom) {
			return custom
		}
		if p, err := exec.LookPath(custom); err == nil {
			return p
		}
		return custom
	}

	if p, err := exec.LookPath("emulator"); err == nil {
		return p
	}

	for _, envVar := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if val := os.Getenv(envVar); val != "" {
			cand := filepath.Join(val, "emulator", "emulator")
			if isExecutable(cand) {
				return cand
			}
			candExe := cand + ".exe"
			if isExecutable(candExe) {
				return candExe
			}
		}
	}

	candidates := []string{
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Android", "Sdk", "emulator", "emulator.exe"),
		filepath.Join(os.Getenv("HOME"), "Android", "Sdk", "emulator", "emulator"),
		filepath.Join(os.Getenv("HOME"), "Library", "Android", "sdk", "emulator", "emulator"),
	}

	for _, cand := range candidates {
		if isExecutable(cand) {
			return cand
		}
	}

	return "emulator"
}
