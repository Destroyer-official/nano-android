package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/mcp"
)

func TestCLI_Init(t *testing.T) {
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "myapp")

	if err := run([]string{"init", targetDir}); err != nil {
		t.Fatalf("run init failed: %v", err)
	}

	for _, file := range []string{"droidforge.yaml", ".mcp.json", "AGENTS.md", "droidforge.policy.json"} {
		p := filepath.Join(targetDir, file)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Errorf("file %s was not created", p)
		}
	}
}

func TestCLI_Doctor(t *testing.T) {
	// Doctor runs without crashing
	err := run([]string{"doctor"})
	_ = err
}

func TestCLI_Serve_Piped(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.yaml")
	cfgContent := "schema_version: 1\nproject:\n  name: test-project\n"
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatal(err)
	}

	binName := "droidforge-mcp"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(tmpDir, binName)
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = "."
	if _, err := exec.LookPath("go"); err != nil && runtime.GOOS == "windows" {
		buildCmd.Env = append(os.Environ(), `PATH=C:\Program Files\Go\bin;`+os.Getenv("PATH"))
	}
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\nOutput: %s", err, string(out))
	}

	cmd := exec.Command(binPath, "serve", "--config", cfgPath)
	cmd.Dir = tmpDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start droidforge-mcp: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
	}()

	// Send tools/list request
	req := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n"
	if _, err := io.WriteString(stdin, req); err != nil {
		t.Fatalf("failed to write to stdin: %v", err)
	}

	// Read response line
	scanner := bufio.NewScanner(stdout)
	done := make(chan string, 1)
	go func() {
		if scanner.Scan() {
			done <- scanner.Text()
		} else {
			done <- ""
		}
	}()

	var respStr string
	select {
	case respStr = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for response from droidforge-mcp serve")
	}

	if respStr == "" {
		t.Fatalf("empty response from serve")
	}

	var resp mcp.JSONRPCResponse
	if err := json.Unmarshal([]byte(respStr), &resp); err != nil {
		t.Fatalf("failed to parse JSON-RPC response: %v, raw: %s", err, respStr)
	}

	if resp.ID != float64(1) {
		t.Errorf("expected ID=1, got %v", resp.ID)
	}

	resBytes, _ := json.Marshal(resp.Result)
	var listRes mcp.ToolsListResult
	if err := json.Unmarshal(resBytes, &listRes); err != nil {
		t.Fatalf("failed to unmarshal ToolsListResult: %v", err)
	}

	if len(listRes.Tools) != 26 {
		t.Errorf("expected 26 canonical tools from serve, got %d", len(listRes.Tools))
	}
}

func TestCLI_UsageAndUnknownCommand(t *testing.T) {
	if err := run([]string{"--help"}); err != nil {
		t.Errorf("expected nil on --help, got %v", err)
	}
	if err := run([]string{"version"}); err != nil {
		t.Errorf("expected nil on version, got %v", err)
	}
	if err := run([]string{"serve", "--help"}); err != nil {
		t.Errorf("expected nil on serve --help, got %v", err)
	}
	if err := run([]string{"doctor", "--help"}); err != nil {
		t.Errorf("expected nil on doctor --help, got %v", err)
	}
	if err := run([]string{"init", "--help"}); err != nil {
		t.Errorf("expected nil on init --help, got %v", err)
	}
	if err := run([]string{"invalid_command"}); err == nil {
		t.Errorf("expected error on unknown command, got nil")
	}
}

func TestCLI_DoctorJSON(t *testing.T) {
	if err := run([]string{"doctor", "--json"}); err != nil {
		t.Logf("doctor returned error: %v", err)
	}
}

func TestCLI_InitWithName(t *testing.T) {
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "named_app")
	if err := run([]string{"init", targetDir, "--name", "custom-android-app"}); err != nil {
		t.Fatalf("run init --name failed: %v", err)
	}
	cfg, err := mcp.LoadConfig(filepath.Join(targetDir, "droidforge.yaml"))
	if err != nil {
		t.Fatalf("failed to load scaffolded config: %v", err)
	}
	if cfg.Project.Name != "custom-android-app" {
		t.Errorf("expected project.name 'custom-android-app', got %q", cfg.Project.Name)
	}
}
