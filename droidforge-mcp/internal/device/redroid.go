package device

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

// RedroidConfig specifies container and runtime settings for running
// an Android instance inside Docker via Redroid (Remote Android).
type RedroidConfig struct {
	// Image is the Docker image to use (e.g. "redroid/redroid:14.0.0-latest").
	Image string
	// ContainerName is the Docker container name.
	ContainerName string
	// HostADBPort is the host port mapped to container port 5555.
	HostADBPort int
	// Width is the virtual display width in pixels (e.g. 1080).
	Width int
	// Height is the virtual display height in pixels (e.g. 1920).
	Height int
	// DPI is the virtual screen density (e.g. 420).
	DPI int
	// FPS is the virtual frame rate (e.g. 60).
	FPS int
	// GPUMode is the rendering backend: "auto", "host", or "guest".
	GPUMode string
	// DataDir is an optional host directory mounted to /data for state persistence.
	DataDir string
	// Privileged specifies whether to pass --privileged to docker run (required for binder).
	Privileged bool
	// ExtraDockerArgs allows specifying additional docker CLI arguments.
	ExtraDockerArgs []string
	// ExtraBootArgs allows specifying additional androidboot parameters.
	ExtraBootArgs []string
}

// DefaultRedroidConfig returns standard defaults for CI or headless container execution.
func DefaultRedroidConfig() RedroidConfig {
	return RedroidConfig{
		Image:         "redroid/redroid:14.0.0-latest",
		ContainerName: "droidforge-redroid",
		HostADBPort:   5555,
		Width:         1080,
		Height:        1920,
		DPI:           420,
		FPS:           60,
		GPUMode:       "guest",
		Privileged:    true,
	}
}

// RedroidContainerInfo holds runtime status inspected from Docker.
type RedroidContainerInfo struct {
	ID      string `json:"Id"`
	Name    string `json:"Name"`
	Running bool   `json:"Running"`
	Status  string `json:"Status"`
	IP      string `json:"IP"`
	Port    int    `json:"Port"`
}

// RedroidAdapter manages Docker lifecycle for Redroid instances.
type RedroidAdapter struct {
	dockerBin string
	adb       *ADB
}

// NewRedroidAdapter creates a new Redroid container manager.
func NewRedroidAdapter(adbDriver *ADB) *RedroidAdapter {
	if adbDriver == nil {
		adbDriver = GetDefaultADB()
	}
	return &RedroidAdapter{
		dockerBin: "docker",
		adb:       adbDriver,
	}
}

// CheckHostPrerequisites verifies if the Linux host has required binder kernel modules loaded.
func CheckHostPrerequisites() error {
	// Redroid requires host binder IPC
	binderNodes := []string{"/dev/binder", "/dev/ashmem"}
	var missing []string
	for _, node := range binderNodes {
		if _, err := os.Stat(node); err != nil {
			missing = append(missing, node)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("host missing required Redroid kernel nodes %v (run 'sudo modprobe binder_linux' or install binderfs)", missing)
	}
	return nil
}

// FindFreePort allocates a temporary free TCP port on localhost.
func FindFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// BuildDockerRunArgs constructs the strict argument array for `docker run`.
// Pure array construction guarantees zero shell interpolation.
func BuildDockerRunArgs(cfg RedroidConfig) []string {
	if cfg.Image == "" {
		cfg.Image = "redroid/redroid:14.0.0-latest"
	}
	if cfg.ContainerName == "" {
		cfg.ContainerName = "droidforge-redroid"
	}
	if cfg.HostADBPort <= 0 {
		cfg.HostADBPort = 5555
	}
	if cfg.Width <= 0 {
		cfg.Width = 1080
	}
	if cfg.Height <= 0 {
		cfg.Height = 1920
	}
	if cfg.DPI <= 0 {
		cfg.DPI = 420
	}
	if cfg.FPS <= 0 {
		cfg.FPS = 60
	}
	if cfg.GPUMode == "" {
		cfg.GPUMode = "guest"
	}

	args := []string{
		"run",
		"-d",
		"--name", cfg.ContainerName,
		"--pull=if-not-present",
	}

	if cfg.Privileged {
		args = append(args, "--privileged")
	}

	// Host binder device mounts required by Redroid
	args = append(args,
		"-v", "/dev/binder:/dev/binder",
		"-v", "/dev/ashmem:/dev/ashmem",
	)

	if cfg.DataDir != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/data", cfg.DataDir))
	}

	// Expose ADB port
	args = append(args, "-p", fmt.Sprintf("%d:5555", cfg.HostADBPort))

	// Append any custom docker flags
	args = append(args, cfg.ExtraDockerArgs...)

	// Container image
	args = append(args, cfg.Image)

	// Android boot arguments
	bootArgs := []string{
		fmt.Sprintf("androidboot.redroid_width=%d", cfg.Width),
		fmt.Sprintf("androidboot.redroid_height=%d", cfg.Height),
		fmt.Sprintf("androidboot.redroid_dpi=%d", cfg.DPI),
		fmt.Sprintf("androidboot.redroid_fps=%d", cfg.FPS),
		fmt.Sprintf("androidboot.redroid_gpu_mode=%s", cfg.GPUMode),
	}
	bootArgs = append(bootArgs, cfg.ExtraBootArgs...)

	args = append(args, bootArgs...)
	return args
}

// Start launches a Redroid container, waits for ADB connectivity, and returns the device serial.
func (r *RedroidAdapter) Start(ctx context.Context, cfg RedroidConfig) (serial string, err error) {
	// First check if container already exists and remove or stop it
	_ = r.Stop(ctx, cfg.ContainerName)

	if cfg.HostADBPort <= 0 {
		if freePort, portErr := FindFreePort(); portErr == nil {
			cfg.HostADBPort = freePort
		} else {
			cfg.HostADBPort = 5555
		}
	}

	args := BuildDockerRunArgs(cfg)

	// Pure exec.CommandContext — no shell
	cmd := exec.CommandContext(ctx, r.dockerBin, args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed starting redroid container (args: %v): %w (stderr: %s)",
			args, err, strings.TrimSpace(stderrBuf.String()))
	}

	serial = fmt.Sprintf("127.0.0.1:%d", cfg.HostADBPort)

	// Wait up to 30 seconds for ADB port to open and connect
	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	connected := false
	for {
		select {
		case <-connectCtx.Done():
			_ = r.Stop(ctx, cfg.ContainerName)
			return "", fmt.Errorf("timed out waiting for redroid container at %s: %w", serial, connectCtx.Err())
		default:
		}

		out, connectErr := r.ConnectADB(connectCtx, cfg.HostADBPort)
		if connectErr == nil && (strings.Contains(out, "connected") || strings.Contains(out, "already")) {
			connected = true
			break
		}
		time.Sleep(1 * time.Second)
	}

	if !connected {
		_ = r.Stop(ctx, cfg.ContainerName)
		return "", fmt.Errorf("failed to establish ADB connection to redroid container %s", serial)
	}

	// Wait for device to complete boot
	if err := r.adb.WaitForDevice(ctx, serial, 30*time.Second); err != nil {
		_ = r.Stop(ctx, cfg.ContainerName)
		return "", fmt.Errorf("redroid device failed wait-for-device: %w", err)
	}

	if err := r.adb.WaitForBootComplete(ctx, serial, 60*time.Second); err != nil {
		_ = r.Stop(ctx, cfg.ContainerName)
		return "", fmt.Errorf("redroid boot completed wait failed: %w", err)
	}

	return serial, nil
}

// Stop cleanly terminates and deletes a Redroid Docker container.
func (r *RedroidAdapter) Stop(ctx context.Context, containerName string) error {
	if containerName == "" {
		return errors.New("containerName cannot be empty")
	}

	// Remove container forcefully: docker rm -f <containerName>
	cmd := exec.CommandContext(ctx, r.dockerBin, "rm", "-f", containerName)
	_ = cmd.Run() // Ignore errors if container does not exist
	return nil
}

// InspectContainer queries container state via docker inspect.
func (r *RedroidAdapter) InspectContainer(ctx context.Context, containerName string) (*RedroidContainerInfo, error) {
	cmd := exec.CommandContext(ctx, r.dockerBin, "inspect", "--format={{json .State}}", containerName)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker inspect failed for %s: %w", containerName, err)
	}

	var state struct {
		Running bool   `json:"Running"`
		Status  string `json:"Status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &state); err != nil {
		return nil, fmt.Errorf("failed parsing docker inspect JSON: %w", err)
	}

	return &RedroidContainerInfo{
		Name:    containerName,
		Running: state.Running,
		Status:  state.Status,
	}, nil
}

// ConnectADB runs "adb connect 127.0.0.1:<port>".
func (r *RedroidAdapter) ConnectADB(ctx context.Context, hostPort int) (string, error) {
	if hostPort <= 0 {
		return "", errors.New("invalid hostPort")
	}
	target := fmt.Sprintf("127.0.0.1:%d", hostPort)
	out, err := r.adb.ExecContext(ctx, "", "connect", target)
	if err != nil {
		return out, fmt.Errorf("adb connect %s error: %w", target, err)
	}
	return out, nil
}

// DisconnectADB runs "adb disconnect <serial>".
func (r *RedroidAdapter) DisconnectADB(ctx context.Context, serial string) error {
	if serial == "" {
		return errors.New("serial cannot be empty")
	}
	_, err := r.adb.ExecContext(ctx, "", "disconnect", serial)
	return err
}
