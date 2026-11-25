package device

import (
	"strings"
	"testing"
)

func TestBuildDockerRunArgsDefault(t *testing.T) {
	cfg := DefaultRedroidConfig()
	args := BuildDockerRunArgs(cfg)

	// Verify required Docker flags
	joined := strings.Join(args, " ")

	expectedTokens := []string{
		"run -d",
		"--name droidforge-redroid",
		"--privileged",
		"-v /dev/binder:/dev/binder",
		"-v /dev/ashmem:/dev/ashmem",
		"-p 5555:5555",
		"redroid/redroid:14.0.0-latest",
		"androidboot.redroid_width=1080",
		"androidboot.redroid_height=1920",
		"androidboot.redroid_dpi=420",
		"androidboot.redroid_fps=60",
		"androidboot.redroid_gpu_mode=guest",
	}

	for _, token := range expectedTokens {
		if !strings.Contains(joined, token) {
			t.Errorf("BuildDockerRunArgs: missing expected token %q in args: %s", token, joined)
		}
	}
}

func TestBuildDockerRunArgsCustom(t *testing.T) {
	cfg := RedroidConfig{
		Image:         "redroid/redroid:13.0.0-latest",
		ContainerName: "ci-redroid-runner",
		HostADBPort:   5565,
		Width:         720,
		Height:        1280,
		DPI:           320,
		FPS:           30,
		GPUMode:       "host",
		DataDir:       "/var/lib/redroid-data",
		Privileged:    true,
		ExtraDockerArgs: []string{
			"--memory=4g",
			"--cpus=2",
		},
		ExtraBootArgs: []string{
			"androidboot.use_memfd=1",
		},
	}

	args := BuildDockerRunArgs(cfg)
	joined := strings.Join(args, " ")

	expectedTokens := []string{
		"--name ci-redroid-runner",
		"-p 5565:5555",
		"-v /var/lib/redroid-data:/data",
		"--memory=4g",
		"--cpus=2",
		"redroid/redroid:13.0.0-latest",
		"androidboot.redroid_width=720",
		"androidboot.redroid_height=1280",
		"androidboot.redroid_dpi=320",
		"androidboot.redroid_fps=30",
		"androidboot.redroid_gpu_mode=host",
		"androidboot.use_memfd=1",
	}

	for _, token := range expectedTokens {
		if !strings.Contains(joined, token) {
			t.Errorf("BuildDockerRunArgs custom: missing expected token %q in args: %s", token, joined)
		}
	}
}

func TestFindFreePort(t *testing.T) {
	port, err := FindFreePort()
	if err != nil {
		t.Fatalf("FindFreePort failed: %v", err)
	}
	if port <= 1024 || port >= 65535 {
		t.Errorf("unexpected free port: %d", port)
	}
}

func TestCheckHostPrerequisites(t *testing.T) {
	// On Windows, binder is not present so it should return an informative error
	err := CheckHostPrerequisites()
	if err != nil {
		t.Logf("Expected host prerequisite message: %v", err)
		if !strings.Contains(err.Error(), "kernel nodes") {
			t.Errorf("unexpected error format: %v", err)
		}
	}
}
