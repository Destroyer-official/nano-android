package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// SidecarRunner abstracts media quality and integrity verification via Python sidecar.
type SidecarRunner interface {
	VerifyMedia(ctx context.Context, srcPath, dstPath, mode, goldenHash, outPath string, minVMAF, minSSIM float64) (*types.MediaVerifyResult, error)
}

// PythonSidecarRunner invokes df_verify_media.py via uv run or python.
type PythonSidecarRunner struct {
	SidecarDir string
}

// NewPythonSidecarRunner creates a new SidecarRunner resolving sidecar directory.
func NewPythonSidecarRunner(sidecarDir string) *PythonSidecarRunner {
	if sidecarDir == "" {
		if env := os.Getenv("DROIDFORGE_SIDECAR_DIR"); env != "" {
			sidecarDir = env
		} else {
			// Probe candidate paths
			candidates := []string{
				"sidecar-python",
				"../sidecar-python",
				"../../sidecar-python",
				filepath.Join(".", "droidforge-mcp", "sidecar-python"),
			}
			for _, c := range candidates {
				if fi, err := os.Stat(filepath.Join(c, "df_verify_media.py")); err == nil && !fi.IsDir() {
					sidecarDir = c
					break
				}
			}
			if sidecarDir == "" {
				sidecarDir = "sidecar-python"
			}
		}
	}
	return &PythonSidecarRunner{SidecarDir: sidecarDir}
}

// VerifyMedia invokes df-verify-media CLI and reads structured JSON results.
func (r *PythonSidecarRunner) VerifyMedia(ctx context.Context, srcPath, dstPath, mode, goldenHash, outPath string, minVMAF, minSSIM float64) (*types.MediaVerifyResult, error) {
	if dstPath == "" {
		return nil, fmt.Errorf("destination media path is required")
	}

	tempOut := outPath
	if tempOut == "" {
		f, err := os.CreateTemp("", "df_media_report_*.json")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp report file: %w", err)
		}
		tempOut = f.Name()
		f.Close()
		defer os.Remove(tempOut)
	}

	args := []string{
		"--dst", dstPath,
		"--mode", mode,
		"--out", tempOut,
	}
	if srcPath != "" {
		args = append(args, "--src", srcPath)
	}
	if goldenHash != "" {
		args = append(args, "--golden-hash", goldenHash)
	}
	if minVMAF > 0 {
		args = append(args, "--min-vmaf", strconv.FormatFloat(minVMAF, 'f', 2, 64))
	}
	if minSSIM > 0 {
		args = append(args, "--min-ssim", strconv.FormatFloat(minSSIM, 'f', 4, 64))
	}

	// Try uv run first if uv is installed
	var cmd *exec.Cmd
	absSidecarDir, _ := filepath.Abs(r.SidecarDir)
	pythonExe := "python3"
	if _, pErr := exec.LookPath(pythonExe); pErr != nil {
		pythonExe = "python"
	}
	if _, err := exec.LookPath("uv"); err == nil {
		cmdArgs := append([]string{"run", "--project", absSidecarDir, "df-verify-media"}, args...)
		cmd = exec.CommandContext(ctx, "uv", cmdArgs...)
	} else {
		scriptPath := filepath.Join(absSidecarDir, "df_verify_media.py")
		cmdArgs := append([]string{scriptPath}, args...)
		cmd = exec.CommandContext(ctx, pythonExe, cmdArgs...)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try fallback directly with python
		scriptPath := filepath.Join(absSidecarDir, "df_verify_media.py")
		cmdArgs := append([]string{scriptPath}, args...)
		fallbackCmd := exec.CommandContext(ctx, pythonExe, cmdArgs...)
		fallbackOut, fallbackErr := fallbackCmd.CombinedOutput()
		if fallbackErr != nil {
			return nil, fmt.Errorf("df-verify-media failed: %v (output: %s; fallback: %s)", err, string(output), string(fallbackOut))
		}
	}

	data, err := os.ReadFile(tempOut)
	if err != nil {
		return nil, fmt.Errorf("failed to read report file %s: %w", tempOut, err)
	}

	var res types.MediaVerifyResult
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("failed to parse verify report JSON: %w", err)
	}

	return &res, nil
}
