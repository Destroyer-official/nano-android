//go:build !windows

package worker_isolation

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func startPlatformIsolated(cmd *exec.Cmd, workerID string, ws *WorkerWorkspace, limits Limits) (*IsolatedHandle, error) {
	backend := "unix_pgid"
	origPath := cmd.Path
	origArgs := cmd.Args

	// Prefer bwrap (bubblewrap) unprivileged mount/pid/net/ipc namespace sandbox on Linux when installed.
	if bwrapPath, err := exec.LookPath("bwrap"); err == nil && ws != nil {
		bwrapArgs := []string{
			"--die-with-parent",
			"--new-session",
			"--unshare-pid",
			"--unshare-ipc",
			"--unshare-uts",
		}
		if !limits.AllowNetwork {
			bwrapArgs = append(bwrapArgs, "--unshare-net")
		}
		for _, roDir := range []string{"/usr", "/lib", "/lib64", "/bin", "/etc/ssl", "/etc/alternatives"} {
			bwrapArgs = append(bwrapArgs, "--ro-bind-try", roDir, roDir)
		}
		if wd, wdErr := filepath.Abs("."); wdErr == nil {
			bwrapArgs = append(bwrapArgs, "--ro-bind-try", wd, wd)
		}
		bwrapArgs = append(bwrapArgs,
			"--proc", "/proc",
			"--dev", "/dev",
			"--tmpfs", "/tmp",
			"--bind", ws.RootDir, ws.RootDir,
			"--chdir", ws.RootDir,
			"--",
		)
		targetBin := origPath
		if len(origArgs) > 0 {
			bwrapArgs = append(bwrapArgs, targetBin)
			bwrapArgs = append(bwrapArgs, origArgs[1:]...)
		} else {
			bwrapArgs = append(bwrapArgs, targetBin)
		}
		cmd.Path = bwrapPath
		cmd.Args = append([]string{bwrapPath}, bwrapArgs...)
		backend = "linux_bwrap"
	}

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start isolated unix worker (%s): %w", backend, err)
	}

	pid := cmd.Process.Pid
	return &IsolatedHandle{
		WorkerID: workerID,
		Backend:  backend,
		PID:      pid,
		closeFn: func() error {
			// Negative PID kills the entire process group deterministically.
			err := syscall.Kill(-pid, syscall.SIGKILL)
			if err != nil && !strings.Contains(err.Error(), "no such process") {
				return err
			}
			return nil
		},
		statsFn: func() (WorkerStats, error) {
			return WorkerStats{
				WorkerID:        workerID,
				Backend:         backend,
				PID:             pid,
				ActiveProcesses: 1,
			}, nil
		},
	}, nil
}
