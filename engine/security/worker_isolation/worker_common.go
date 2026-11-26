package worker_isolation

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Limits defines OS-enforced resource and privilege ceilings for a worker process.
type Limits struct {
	MaxMemoryMB        uint64
	CPURatePercent     uint32 // 1..100 (0 = uncapped)
	MaxActiveProcesses uint32 // cap child process spawning (fork-bomb defense)
	ExecutionTimeout   time.Duration
	AllowNetwork       bool
}

// DefaultJVMLimits returns hardened resource limits for HeadlessAppHost JVM workers.
func DefaultJVMLimits() Limits {
	return Limits{
		MaxMemoryMB:        1024,
		CPURatePercent:     80,
		MaxActiveProcesses: 2,
		ExecutionTimeout:   120 * time.Second,
		AllowNetwork:       false,
	}
}

// DefaultTranslatorLimits returns limits for dex2jar / bundletool ingestion workers.
func DefaultTranslatorLimits() Limits {
	return Limits{
		MaxMemoryMB:        1536,
		CPURatePercent:     90,
		MaxActiveProcesses: 8, // batch/shell wrapper + java
		ExecutionTimeout:   180 * time.Second,
		AllowNetwork:       false,
	}
}

// WorkerWorkspace represents an isolated per-worker directory layout on disk.
type WorkerWorkspace struct {
	WorkerID   string
	RootDir    string
	SandboxDir string
	TempDir    string
}

// NewWorkerID generates a unique cryptographic worker identifier.
func NewWorkerID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("wrk_%d", time.Now().UnixNano())
	}
	return "wrk_" + hex.EncodeToString(b)
}

// PrepareWorkspace creates an isolated per-worker filesystem root with private sandbox & temp dirs.
func PrepareWorkspace(cacheRoot string) (*WorkerWorkspace, error) {
	if strings.TrimSpace(cacheRoot) == "" {
		cacheRoot = filepath.Join(os.TempDir(), "nano-workers")
	}
	absCache, err := filepath.Abs(cacheRoot)
	if err != nil {
		return nil, err
	}
	id := NewWorkerID()
	rootDir := filepath.Join(absCache, "workers", id)
	sandboxDir := filepath.Join(rootDir, "sandbox")
	tempDir := filepath.Join(rootDir, "tmp")

	for _, d := range []string{rootDir, sandboxDir, tempDir} {
		if err := os.MkdirAll(d, 0750); err != nil {
			return nil, fmt.Errorf("create worker dir %s: %w", d, err)
		}
	}
	return &WorkerWorkspace{
		WorkerID:   id,
		RootDir:    rootDir,
		SandboxDir: sandboxDir,
		TempDir:    tempDir,
	}, nil
}

// Cleanup removes the worker's temporary workspace directories.
func (w *WorkerWorkspace) Cleanup() error {
	if w == nil || strings.TrimSpace(w.RootDir) == "" {
		return nil
	}
	return os.RemoveAll(w.RootDir)
}

// SanitizedEnv constructs a minimal environment stripped of host user secrets, API keys,
// cloud tokens, and user-profile directories.
func SanitizedEnv(tempDir string, extraEnv []string) []string {
	var env []string
	preserveKeys := []string{"PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "JAVA_HOME", "LANG", "LC_ALL"}
	for _, k := range preserveKeys {
		if v := os.Getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	if tempDir != "" {
		if runtime.GOOS == "windows" {
			env = append(env, "TEMP="+tempDir, "TMP="+tempDir)
		} else {
			env = append(env, "TMPDIR="+tempDir, "HOME="+tempDir)
		}
	}
	env = append(env, "NANO_ISOLATED_WORKER=1")
	for _, e := range extraEnv {
		if strings.Contains(e, "=") {
			env = append(env, e)
		}
	}
	return env
}

// IsolatedHandle tracks an OS-sandboxed running worker process tree for deterministic teardown.
type IsolatedHandle struct {
	WorkerID string
	Backend  string // e.g., "windows_job_object", "linux_bwrap", "linux_pgid"
	PID      int
	closeFn  func() error
	statsFn  func() (WorkerStats, error)
}

// WorkerStats reports live OS-enforced worker resource telemetry.
type WorkerStats struct {
	WorkerID        string `json:"worker_id"`
	Backend         string `json:"backend"`
	PID             int    `json:"pid"`
	ActiveProcesses uint32 `json:"active_processes"`
	PeakMemoryBytes uint64 `json:"peak_memory_bytes"`
}

// Close terminates the entire worker process tree and releases OS handles.
func (h *IsolatedHandle) Close() error {
	if h == nil || h.closeFn == nil {
		return nil
	}
	return h.closeFn()
}

// Stats queries OS-level process tree accounting for the worker.
func (h *IsolatedHandle) Stats() (WorkerStats, error) {
	if h == nil || h.statsFn == nil {
		return WorkerStats{}, fmt.Errorf("worker handle inactive")
	}
	return h.statsFn()
}

// StartIsolated starts cmd inside the platform's OS-enforced isolation boundary.
func StartIsolated(cmd *exec.Cmd, workerID string, ws *WorkerWorkspace, limits Limits) (*IsolatedHandle, error) {
	if cmd == nil {
		return nil, fmt.Errorf("nil exec.Cmd")
	}
	tempDir := ""
	if ws != nil {
		tempDir = ws.TempDir
		if cmd.Dir == "" {
			cmd.Dir = ws.RootDir
		}
	}
	cmd.Env = SanitizedEnv(tempDir, cmd.Env)
	return startPlatformIsolated(cmd, workerID, ws, limits)
}
