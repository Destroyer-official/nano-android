package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// CommandExecutor abstracts OS command execution for testing and platform portability.
type CommandExecutor func(ctx context.Context, name string, args ...string) ([]byte, error)

// DefaultCommandExecutor is the real os/exec implementation.
var DefaultCommandExecutor CommandExecutor = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// MaestroRunner handles running Maestro YAML flows, coordinating companion stimuli,
// and capturing forensic failure bundles.
type MaestroRunner struct {
	executor    CommandExecutor
	runDir      string
	maestroPath string
}

// NewMaestroRunner creates a new MaestroRunner.
func NewMaestroRunner(executor CommandExecutor) *MaestroRunner {
	if executor == nil {
		executor = DefaultCommandExecutor
	}
	return &MaestroRunner{
		executor: executor,
		runDir:   filepath.Join(".droidforge", "runs"),
	}
}

// SetRunDir sets the base directory for run artifacts and failure bundles.
func (r *MaestroRunner) SetRunDir(dir string) {
	r.runDir = dir
}

// SetMaestroPath sets an explicit path to the maestro executable.
func (r *MaestroRunner) SetMaestroPath(path string) {
	r.maestroPath = path
}

// FailureSummary represents summary.json in a failure artifact bundle.
type FailureSummary struct {
	FlowPath   string `json:"flow_path"`
	Device     string `json:"device_serial,omitempty"`
	RunIndex   int    `json:"run_index"`
	Error      string `json:"error"`
	DurationMS int64  `json:"duration_ms"`
	Timestamp  string `json:"timestamp"`
	FailedStep string `json:"failed_step,omitempty"`
	ExitCode   int    `json:"exit_code"`
}

// LocateMaestro attempts to resolve the maestro binary across standard paths.
func (r *MaestroRunner) LocateMaestro() (string, error) {
	if r.maestroPath != "" {
		if _, err := os.Stat(r.maestroPath); err == nil {
			return r.maestroPath, nil
		}
	}

	binName := "maestro"
	if runtime.GOOS == "windows" {
		binName = "maestro.bat"
	}

	// 1. Check PATH
	if path, err := exec.LookPath(binName); err == nil {
		return path, nil
	}
	if runtime.GOOS == "windows" {
		if path, err := exec.LookPath("maestro"); err == nil {
			return path, nil
		}
	}

	// 2. Check ~/.maestro/bin
	home, err := os.UserHomeDir()
	if err == nil {
		candidate := filepath.Join(home, ".maestro", "bin", binName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		if runtime.GOOS == "windows" {
			candidateExe := filepath.Join(home, ".maestro", "bin", "maestro.exe")
			if _, err := os.Stat(candidateExe); err == nil {
				return candidateExe, nil
			}
		}
	}

	return "", fmt.Errorf("maestro CLI binary not found in PATH or ~/.maestro/bin. Install with: curl -Ls 'https://get.maestro.mobile.dev' | bash")
}

// Run executes a Maestro flow with optional burn-in repeat count.
func (r *MaestroRunner) Run(ctx context.Context, serial, flowPath string, repeatCount int) (*types.MaestroRunResult, error) {
	if repeatCount <= 0 {
		repeatCount = 1
	}

	// Verify flow file exists
	if _, err := os.Stat(flowPath); err != nil {
		return nil, fmt.Errorf("maestro flow file not found: %w", err)
	}

	// Resolve maestro binary
	maestroBin, err := r.LocateMaestro()
	if err != nil {
		// If executor is custom/mocked, fallback to "maestro" for tests
		maestroBin = "maestro"
	}

	// Check for companion hooks (.hooks.json)
	hooksPath := strings.TrimSuffix(flowPath, filepath.Ext(flowPath)) + ".hooks.json"
	var hooks *HooksManifest
	if data, err := os.ReadFile(hooksPath); err == nil {
		var m HooksManifest
		if jsonErr := json.Unmarshal(data, &m); jsonErr == nil {
			hooks = &m
		}
	}

	flowBase := filepath.Base(flowPath)
	testName := strings.TrimSuffix(flowBase, filepath.Ext(flowBase))

	var passed, failed int
	var totalDurationMS int64
	var failureBundles []string

	for i := 1; i <= repeatCount; i++ {
		start := time.Now()

		// Execute companion hooks if present
		if hooks != nil {
			for _, hook := range hooks.Hooks {
				_ = r.executeHook(ctx, serial, hook)
			}
		}

		args := []string{"test"}
		if serial != "" {
			args = append(args, "--device", serial)
		}
		args = append(args, flowPath)

		output, runErr := r.executor(ctx, maestroBin, args...)
		dur := time.Since(start).Milliseconds()
		totalDurationMS += dur

		if runErr == nil {
			passed++
		} else {
			failed++
			// Create failure bundle layout
			runID := fmt.Sprintf("run_%s_%d", time.Now().Format("20060102_150405"), i)
			bundleDir := filepath.Join(r.runDir, runID, fmt.Sprintf("failure_%s", testName))
			_ = os.MkdirAll(bundleDir, 0o755)

			summary := FailureSummary{
				FlowPath:   flowPath,
				Device:     serial,
				RunIndex:   i,
				Error:      fmt.Sprintf("%v: %s", runErr, string(output)),
				DurationMS: dur,
				Timestamp:  time.Now().UTC().Format(time.RFC3339),
				ExitCode:   1,
			}

			sumBytes, _ := json.MarshalIndent(summary, "", "  ")
			_ = os.WriteFile(filepath.Join(bundleDir, "summary.json"), sumBytes, 0o644)

			// Capture recent logcat
			logFile := filepath.Join(bundleDir, "logcat_target_pid.txt")
			if serial != "" {
				logcatOut, adbErr := r.executor(ctx, "adb", "-s", serial, "logcat", "-d", "-t", "500")
				if adbErr == nil && len(logcatOut) > 0 {
					_ = os.WriteFile(logFile, logcatOut, 0o644)
				} else {
					_ = os.WriteFile(logFile, output, 0o644)
				}
			} else {
				_ = os.WriteFile(logFile, output, 0o644)
			}

			// Capture screen at failure
			screenFile := filepath.Join(bundleDir, "final_screen.png")
			if serial != "" {
				screenOut, adbErr := r.executor(ctx, "adb", "-s", serial, "exec-out", "screencap", "-p")
				if adbErr == nil && len(screenOut) > 0 {
					_ = os.WriteFile(screenFile, screenOut, 0o644)
				} else {
					_ = os.WriteFile(screenFile, []byte("EMPTY_SCREENSHOT_FALLBACK"), 0o644)
				}
			} else {
				_ = os.WriteFile(screenFile, []byte("EMPTY_SCREENSHOT_NO_SERIAL"), 0o644)
			}

			failureBundles = append(failureBundles, bundleDir)
		}
	}

	avgDur := int64(0)
	if repeatCount > 0 {
		avgDur = totalDurationMS / int64(repeatCount)
	}

	return &types.MaestroRunResult{
		RunsPassed:         passed,
		RunsFailed:         failed,
		AvgDurationMS:      avgDur,
		FailureBundlePaths: failureBundles,
	}, nil
}

func (r *MaestroRunner) executeHook(ctx context.Context, serial string, hook HookStep) error {
	switch hook.Action {
	case "sim_inject_sms_or_call":
		if serial != "" && strings.HasPrefix(serial, "emulator-") {
			_, err := r.executor(ctx, "adb", "-s", serial, "emu", "sms", "send", hook.Target, hook.Text)
			return err
		}
	case "sim_post_notification":
		if serial != "" {
			_, err := r.executor(ctx, "adb", "-s", serial, "shell", "cmd", "notification", "post", "-S", "bigtext", "-t", hook.Target, "droidforge_tag", hook.Text)
			return err
		}
	case "sim_set_permissions":
		if serial != "" && hook.Target != "" {
			parts := strings.Fields(hook.Target)
			if len(parts) >= 2 {
				// example target: "grant com.example.app android.permission.POST_NOTIFICATIONS"
				args := append([]string{"-s", serial, "shell", "pm"}, parts...)
				_, err := r.executor(ctx, "adb", args...)
				return err
			}
		}
	}
	return nil
}
