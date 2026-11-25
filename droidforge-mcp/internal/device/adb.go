package device

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultADBTimeout is the standard timeout for typical ADB commands.
	DefaultADBTimeout = 30 * time.Second
	// DefaultInstallTimeout is the extended timeout for APK installation and large pushes.
	DefaultInstallTimeout = 120 * time.Second
)

// CommandRunner defines the low-level process execution interface.
// The default implementation strictly executes via exec.CommandContext
// without any shell interpolation.
type CommandRunner interface {
	RunCommand(ctx context.Context, bin string, args []string) (stdout string, stderr string, err error)
}

// defaultRunner implements CommandRunner with zero shell interpolation.
type defaultRunner struct{}

func (r *defaultRunner) RunCommand(ctx context.Context, bin string, args []string) (string, string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	return stdoutBuf.String(), stderrBuf.String(), err
}

// ADB provides a safe, shell-free interface to the Android Debug Bridge.
// All process execution uses pure exec.Command / exec.CommandContext with
// direct argument arrays. Shell interpreters (sh, bash, cmd.exe, powershell)
// are strictly forbidden and never invoked.
type ADB struct {
	adbPath        string
	defaultTimeout time.Duration
	installTimeout time.Duration
	runner         CommandRunner
	mu             sync.RWMutex
}

var (
	defaultDriverOnce sync.Once
	defaultDriver     *ADB
)

// GetDefaultADB returns the singleton default ADB driver instance.
func GetDefaultADB() *ADB {
	defaultDriverOnce.Do(func() {
		defaultDriver = NewADB("")
	})
	return defaultDriver
}

// NewADB creates an ADB driver. If adbPath is empty, it resolves "adb" from PATH
// or well-known Android SDK platform-tools locations.
func NewADB(adbPath string) *ADB {
	resolved := resolveADBPath(adbPath)
	return &ADB{
		adbPath:        resolved,
		defaultTimeout: DefaultADBTimeout,
		installTimeout: DefaultInstallTimeout,
		runner:         &defaultRunner{},
	}
}

// SetRunner allows injecting a custom CommandRunner for unit testing.
func (a *ADB) SetRunner(runner CommandRunner) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if runner != nil {
		a.runner = runner
	}
}

// SetPath updates the ADB executable path.
func (a *ADB) SetPath(path string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.adbPath = resolveADBPath(path)
}

// Path returns the resolved ADB binary path.
func (a *ADB) Path() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.adbPath
}

// ExecADB runs a single ADB command using the default driver with strict argument arrays.
// It never invokes a shell and captures stderr for diagnostics.
func ExecADB(serial string, args ...string) (stdout string, err error) {
	return GetDefaultADB().ExecWithTimeout(serial, DefaultADBTimeout, args...)
}

// ExecADBWithTimeout runs an ADB command with a custom timeout.
func ExecADBWithTimeout(serial string, timeout time.Duration, args ...string) (stdout string, err error) {
	return GetDefaultADB().ExecWithTimeout(serial, timeout, args...)
}

// ExecADBContext runs an ADB command respecting the provided context.
func ExecADBContext(ctx context.Context, serial string, args ...string) (stdout string, err error) {
	return GetDefaultADB().ExecContext(ctx, serial, args...)
}

// Exec runs an ADB command with the default driver timeout.
func (a *ADB) Exec(serial string, args ...string) (string, error) {
	return a.ExecWithTimeout(serial, a.defaultTimeout, args...)
}

// ExecWithTimeout executes an ADB command with a specified timeout.
func (a *ADB) ExecWithTimeout(serial string, timeout time.Duration, args ...string) (string, error) {
	if timeout <= 0 {
		timeout = a.defaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return a.ExecContext(ctx, serial, args...)
}

// ExecContext executes an ADB command with the given context.
// CRITICAL SECURITY INVARIANT:
// This function strictly constructs exec.CommandContext with the binary and a slice of string args.
// It NEVER invokes a shell wrapper (e.g. sh, bash, cmd.exe, or powershell.exe).
func (a *ADB) ExecContext(ctx context.Context, serial string, args ...string) (string, error) {
	a.mu.RLock()
	bin := a.adbPath
	runner := a.runner
	a.mu.RUnlock()

	var cmdArgs []string
	if serial != "" {
		cmdArgs = append(cmdArgs, "-s", serial)
	}
	cmdArgs = append(cmdArgs, args...)

	start := time.Now()
	stdoutStr, stderrStr, err := runner.RunCommand(ctx, bin, cmdArgs)
	duration := time.Since(start)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return stdoutStr, fmt.Errorf("adb command %q timed out after %v: %w (stderr: %s)",
				formatCommand(bin, cmdArgs), duration, ctx.Err(), strings.TrimSpace(stderrStr))
		}
		if ctx.Err() == context.Canceled {
			return stdoutStr, fmt.Errorf("adb command %q canceled: %w", formatCommand(bin, cmdArgs), ctx.Err())
		}

		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}

		return stdoutStr, &ADBError{
			Command:  formatCommand(bin, cmdArgs),
			Args:     cmdArgs,
			ExitCode: exitCode,
			Stdout:   stdoutStr,
			Stderr:   strings.TrimSpace(stderrStr),
			Err:      err,
		}
	}

	return stdoutStr, nil
}

// ADBError contains diagnostic information from a failed ADB execution.
type ADBError struct {
	Command  string
	Args     []string
	ExitCode int
	Stdout   string
	Stderr   string
	Err      error
}

func (e *ADBError) Error() string {
	msg := fmt.Sprintf("adb error (exit %d): %v", e.ExitCode, e.Err)
	if e.Stderr != "" {
		msg += fmt.Sprintf(" | stderr: %s", e.Stderr)
	}
	return msg
}

func (e *ADBError) Unwrap() error {
	return e.Err
}

// Shell runs "adb [-s serial] shell <args...>" safely.
func (a *ADB) Shell(ctx context.Context, serial string, args ...string) (string, error) {
	cmdArgs := append([]string{"shell"}, args...)
	return a.ExecContext(ctx, serial, cmdArgs...)
}

// GetProp reads a system property from the device via "getprop <prop>".
func (a *ADB) GetProp(ctx context.Context, serial, prop string) (string, error) {
	out, err := a.Shell(ctx, serial, "getprop", prop)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// PidOf returns the process PID for the specified package ID, or -1 if not running.
func (a *ADB) PidOf(ctx context.Context, serial, pkg string) (int, error) {
	if pkg == "" {
		return -1, fmt.Errorf("package name cannot be empty")
	}

	// Try pidof -s <pkg> first
	out, err := a.Shell(ctx, serial, "pidof", "-s", pkg)
	if err == nil {
		pidStr := strings.TrimSpace(out)
		if pid, parseErr := strconv.Atoi(pidStr); parseErr == nil && pid > 0 {
			return pid, nil
		}
	}

	// Fallback to pidof <pkg>
	out, err = a.Shell(ctx, serial, "pidof", pkg)
	if err == nil {
		fields := strings.Fields(out)
		if len(fields) > 0 {
			if pid, parseErr := strconv.Atoi(fields[0]); parseErr == nil && pid > 0 {
				return pid, nil
			}
		}
	}

	// Fallback to cmd activity get-current-app-pid
	out, err = a.Shell(ctx, serial, "cmd", "activity", "get-current-app-pid")
	if err == nil {
		pidStr := strings.TrimSpace(out)
		if pid, parseErr := strconv.Atoi(pidStr); parseErr == nil && pid > 0 {
			return pid, nil
		}
	}

	return -1, nil
}

// Install installs an APK file onto the target device with optional flags.
// Automatically attempts incremental install if requested and falls back to standard install.
func (a *ADB) Install(ctx context.Context, serial, apkPath string, flags ...string) (time.Duration, error) {
	if apkPath == "" {
		return 0, fmt.Errorf("apkPath cannot be empty")
	}

	cmdArgs := []string{"install"}
	if len(flags) > 0 {
		cmdArgs = append(cmdArgs, flags...)
	} else {
		cmdArgs = append(cmdArgs, "-r", "-t")
	}
	cmdArgs = append(cmdArgs, apkPath)

	start := time.Now()
	installCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		installCtx, cancel = context.WithTimeout(ctx, a.installTimeout)
		defer cancel()
	}

	out, err := a.ExecContext(installCtx, serial, cmdArgs...)
	duration := time.Since(start)

	if err != nil {
		// If incremental install was attempted and failed, fallback to standard install
		if containsString(flags, "--incremental") {
			return a.Install(ctx, serial, apkPath, "-r", "-t")
		}
		return duration, fmt.Errorf("adb install failed: %w", err)
	}

	if !strings.Contains(out, "Success") {
		return duration, fmt.Errorf("adb install did not report Success: %s", strings.TrimSpace(out))
	}

	return duration, nil
}

// InstallMultiple installs a split APK bundle using "adb install-multiple".
func (a *ADB) InstallMultiple(ctx context.Context, serial string, apkPaths []string, flags ...string) (time.Duration, error) {
	if len(apkPaths) == 0 {
		return 0, fmt.Errorf("apkPaths list cannot be empty")
	}

	cmdArgs := []string{"install-multiple"}
	if len(flags) > 0 {
		cmdArgs = append(cmdArgs, flags...)
	} else {
		cmdArgs = append(cmdArgs, "-r", "-t")
	}
	cmdArgs = append(cmdArgs, apkPaths...)

	start := time.Now()
	installCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		installCtx, cancel = context.WithTimeout(ctx, a.installTimeout)
		defer cancel()
	}

	out, err := a.ExecContext(installCtx, serial, cmdArgs...)
	duration := time.Since(start)

	if err != nil {
		return duration, fmt.Errorf("adb install-multiple failed: %w", err)
	}

	if !strings.Contains(out, "Success") {
		return duration, fmt.Errorf("adb install-multiple did not report Success: %s", strings.TrimSpace(out))
	}

	return duration, nil
}

// WaitForDevice blocks until the device is online or the timeout elapses.
func (a *ADB) WaitForDevice(ctx context.Context, serial string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := a.ExecContext(waitCtx, serial, "wait-for-device")
	return err
}

// WaitForBootComplete polls sys.boot_completed until "1" is returned or timeout.
func (a *ADB) WaitForBootComplete(ctx context.Context, serial string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		val, err := a.GetProp(ctx, serial, "sys.boot_completed")
		if err == nil && val == "1" {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("device %s failed to complete boot within %v", serial, timeout)
}

// DisableAnimations sets window, transition, and animator scales to 0 for deterministic UI testing.
func (a *ADB) DisableAnimations(ctx context.Context, serial string) error {
	scales := []string{
		"window_animation_scale",
		"transition_animation_scale",
		"animator_duration_scale",
	}

	for _, scale := range scales {
		_, err := a.Shell(ctx, serial, "settings", "put", "global", scale, "0")
		if err != nil {
			return fmt.Errorf("failed to disable %s: %w", scale, err)
		}
	}
	return nil
}

// SetStayAwake configures the device display to stay on while connected to power/USB.
func (a *ADB) SetStayAwake(ctx context.Context, serial string, stayAwake bool) error {
	mode := "0"
	if stayAwake {
		// 7 = stay on for AC + USB + Wireless
		mode = "7"
	}
	_, err := a.Shell(ctx, serial, "settings", "put", "global", "stay_on_while_plugged_in", mode)
	return err
}

// WakeUp wakes the device screen and dismisses the lockscreen.
func (a *ADB) WakeUp(ctx context.Context, serial string) error {
	_, _ = a.Shell(ctx, serial, "input", "keyevent", "KEYCODE_WAKEUP")
	_, _ = a.Shell(ctx, serial, "wm", "dismiss-keyguard")
	return nil
}

// LogcatCapture captures the last N lines of logcat, optionally filtered by spec.
func (a *ADB) LogcatCapture(ctx context.Context, serial string, filterSpec string, lines int) (string, error) {
	if lines <= 0 {
		lines = 200
	}
	args := []string{"logcat", "-d", "-t", strconv.Itoa(lines)}
	if filterSpec != "" {
		args = append(args, filterSpec)
	}
	return a.ExecContext(ctx, serial, args...)
}

// EmuSnapshotLoad requests the emulator to load a specific snapshot via "emu avd snapshot load".
func (a *ADB) EmuSnapshotLoad(ctx context.Context, serial, snapshotName string) error {
	if snapshotName == "" {
		return fmt.Errorf("snapshotName cannot be empty")
	}

	out, err := a.ExecContext(ctx, serial, "emu", "avd", "snapshot", "load", snapshotName)
	if err != nil {
		return fmt.Errorf("failed to load snapshot %q: %w", snapshotName, err)
	}

	if strings.Contains(out, "KO:") {
		return fmt.Errorf("snapshot load error: %s", strings.TrimSpace(out))
	}
	return nil
}

// EmuSnapshotList lists available snapshots saved on the emulator.
func (a *ADB) EmuSnapshotList(ctx context.Context, serial string) ([]string, error) {
	out, err := a.ExecContext(ctx, serial, "emu", "avd", "snapshot", "list")
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots on %s: %w", serial, err)
	}

	var snapshots []string
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "OK") || strings.HasPrefix(trimmed, "Snapshot list:") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) > 0 {
			snapshots = append(snapshots, fields[0])
		}
	}
	return snapshots, nil
}

// ClearPackage invokes "pm clear <pkg>" on the device.
func (a *ADB) ClearPackage(ctx context.Context, serial, pkg string) error {
	if pkg == "" {
		return fmt.Errorf("pkg cannot be empty")
	}

	out, err := a.Shell(ctx, serial, "pm", "clear", pkg)
	if err != nil {
		return fmt.Errorf("pm clear failed for %s: %w", pkg, err)
	}

	if !strings.Contains(out, "Success") {
		return fmt.Errorf("pm clear did not return Success for %s: %s", pkg, strings.TrimSpace(out))
	}
	return nil
}

// RemoveFixtures clears the test fixtures directory on device /sdcard.
func (a *ADB) RemoveFixtures(ctx context.Context, serial, fixtureDir string) error {
	if fixtureDir == "" {
		fixtureDir = "/sdcard/Download/droidforge_fixtures"
	}
	_, err := a.Shell(ctx, serial, "rm", "-rf", fixtureDir)
	return err
}

// GrantPermission grants a runtime permission to the specified package.
func (a *ADB) GrantPermission(ctx context.Context, serial, pkg, perm string) error {
	if pkg == "" || perm == "" {
		return fmt.Errorf("package and permission cannot be empty")
	}
	_, err := a.Shell(ctx, serial, "pm", "grant", pkg, perm)
	return err
}

// RestartServer terminates and restarts the host ADB server process.
func (a *ADB) RestartServer(ctx context.Context) error {
	_, _ = a.ExecContext(ctx, "", "kill-server")
	time.Sleep(500 * time.Millisecond)
	_, err := a.ExecContext(ctx, "", "start-server")
	return err
}

func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// resolveADBPath finds adb in PATH or common Android SDK directories.
func resolveADBPath(custom string) string {
	if custom != "" {
		if _, err := os.Stat(custom); err == nil {
			return custom
		}
		if p, err := exec.LookPath(custom); err == nil {
			return p
		}
		return custom
	}

	// 1. Check if 'adb' is in system PATH
	if p, err := exec.LookPath("adb"); err == nil {
		return p
	}

	// 2. Check ANDROID_HOME or ANDROID_SDK_ROOT
	for _, envVar := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if val := os.Getenv(envVar); val != "" {
			candidate := filepath.Join(val, "platform-tools", "adb")
			if isExecutable(candidate) {
				return candidate
			}
			candidateExe := candidate + ".exe"
			if isExecutable(candidateExe) {
				return candidateExe
			}
		}
	}

	// 3. Check well-known host installation directories
	candidates := []string{
		`D:\program_software\Android_toolkit\platform-tools\adb.exe`,
		`C:\Android\platform-tools\adb.exe`,
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Android", "Sdk", "platform-tools", "adb.exe"),
		filepath.Join(os.Getenv("HOME"), "Android", "Sdk", "platform-tools", "adb"),
		filepath.Join(os.Getenv("HOME"), "Library", "Android", "sdk", "platform-tools", "adb"),
	}

	for _, cand := range candidates {
		if isExecutable(cand) {
			return cand
		}
	}

	return "adb"
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return true
}

func formatCommand(bin string, args []string) string {
	parts := append([]string{bin}, args...)
	return strings.Join(parts, " ")
}
