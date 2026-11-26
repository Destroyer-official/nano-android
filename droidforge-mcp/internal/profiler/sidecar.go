package profiler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// SidecarRunner abstracts Perfetto trace analysis via the Python sidecar.
type SidecarRunner interface {
	AnalyzeTrace(ctx context.Context, tracePath, pkg, outPath string) (*types.PerfTraceResult, error)
}

// PythonSidecarRunner invokes df_analyze_trace.py via uv run or python.
type PythonSidecarRunner struct {
	SidecarDir string
}

// NewPythonSidecarRunner creates a new SidecarRunner for trace analysis.
func NewPythonSidecarRunner(sidecarDir string) *PythonSidecarRunner {
	if sidecarDir == "" {
		if env := os.Getenv("DROIDFORGE_SIDECAR_DIR"); env != "" {
			sidecarDir = env
		} else {
			candidates := []string{
				"sidecar-python",
				"../sidecar-python",
				"../../sidecar-python",
				filepath.Join(".", "droidforge-mcp", "sidecar-python"),
			}
			for _, c := range candidates {
				if fi, err := os.Stat(filepath.Join(c, "df_analyze_trace.py")); err == nil && !fi.IsDir() {
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

// AnalyzeTrace runs df-analyze-trace on the given trace and reads back summary JSON.
func (r *PythonSidecarRunner) AnalyzeTrace(ctx context.Context, tracePath, pkg, outPath string) (*types.PerfTraceResult, error) {
	if tracePath == "" {
		return nil, fmt.Errorf("trace path is required")
	}

	tempOut := outPath
	if tempOut == "" {
		f, err := os.CreateTemp("", "df_trace_summary_*.json")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp summary file: %w", err)
		}
		tempOut = f.Name()
		f.Close()
		defer os.Remove(tempOut)
	}

	args := []string{
		"--trace", tracePath,
		"--out", tempOut,
	}
	if pkg != "" {
		args = append(args, "--pkg", pkg)
	}

	absSidecarDir, _ := filepath.Abs(r.SidecarDir)
	pythonExe := "python3"
	if _, pErr := exec.LookPath(pythonExe); pErr != nil {
		pythonExe = "python"
	}

	var cmd *exec.Cmd
	if _, err := exec.LookPath("uv"); err == nil {
		cmdArgs := append([]string{"run", "--project", absSidecarDir, "df-analyze-trace"}, args...)
		cmd = exec.CommandContext(ctx, "uv", cmdArgs...)
	} else {
		scriptPath := filepath.Join(absSidecarDir, "df_analyze_trace.py")
		cmdArgs := append([]string{scriptPath}, args...)
		cmd = exec.CommandContext(ctx, pythonExe, cmdArgs...)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback to python directly
		scriptPath := filepath.Join(absSidecarDir, "df_analyze_trace.py")
		cmdArgs := append([]string{scriptPath}, args...)
		fallbackCmd := exec.CommandContext(ctx, pythonExe, cmdArgs...)
		fallbackOut, fallbackErr := fallbackCmd.CombinedOutput()
		if fallbackErr != nil {
			return nil, fmt.Errorf("df-analyze-trace failed: %v (output: %s; fallback: %s)", err, string(output), string(fallbackOut))
		}
	}

	data, err := os.ReadFile(tempOut)
	if err != nil {
		return nil, fmt.Errorf("failed to read summary file %s: %w", tempOut, err)
	}

	var res types.PerfTraceResult
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("failed to parse trace summary JSON: %w", err)
	}

	return &res, nil
}
