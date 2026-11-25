package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/compiler"
	"github.com/anthropic/droidforge-mcp/internal/types"
)

// --- Test Mocks ---

type mockToolHandler struct {
	def       types.ToolDefinition
	invokeCnt int
	lastArgs  map[string]any
	res       *types.ToolResult
	err       error
	panics    bool
}

func (m *mockToolHandler) Definition() types.ToolDefinition {
	return m.def
}

func (m *mockToolHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	m.invokeCnt++
	m.lastArgs = args
	if m.panics {
		panic("simulated fatal handler panic")
	}
	return m.res, m.err
}

type mockPolicyGuard struct {
	decision types.PolicyDecision
	reason   string
	calls    []types.ToolDefinition
}

func (g *mockPolicyGuard) Evaluate(ctx context.Context, tool types.ToolDefinition, args map[string]any) (*types.PolicyEvaluation, error) {
	g.calls = append(g.calls, tool)
	return &types.PolicyEvaluation{
		Decision: g.decision,
		Reason:   g.reason,
	}, nil
}

func (g *mockPolicyGuard) FenceUntrustedText(raw string) (string, string) {
	return "<DF_UNTRUSTED_abc>" + raw + "</DF_UNTRUSTED_abc>", "abc"
}

func (g *mockPolicyGuard) IncrementStepCounter(sessionID string) {}

func (g *mockPolicyGuard) RecordUIHash(sessionID, hash string) int {
	return 1
}

type mockAuditLogger struct {
	mu      sync.Mutex
	records []types.AuditRecord
}

func (l *mockAuditLogger) Log(record types.AuditRecord) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, record)
	return nil
}

func (l *mockAuditLogger) Records() []types.AuditRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	copied := make([]types.AuditRecord, len(l.records))
	copy(copied, l.records)
	return copied
}

// --- Unit Tests ---

// 1. Unit test: register 3 stub tools, send tools/list over in-memory pipe, verify response lists all 3
func TestToolsList_ThreeStubTools(t *testing.T) {
	srv := NewServer(nil, nil, nil, nil)

	tools := []types.ToolDefinition{
		{
			Name:        "tool_alpha",
			Description: "First test tool",
			RiskTier:    types.RiskREAD,
			InputSchema: map[string]any{"type": "object"},
		},
		{
			Name:        "tool_beta",
			Description: "Second test tool",
			RiskTier:    types.RiskWRITESCOPED,
			InputSchema: map[string]any{"type": "object"},
		},
		{
			Name:        "tool_gamma",
			Description: "Third test tool",
			RiskTier:    types.RiskAPPROVALGATED,
			InputSchema: map[string]any{"type": "object"},
		},
	}

	for _, def := range tools {
		srv.Register(NewStubHandler(def))
	}

	input := `{"jsonrpc":"2.0","id":101,"method":"tools/list","params":{}}` + "\n"
	r := strings.NewReader(input)
	var w bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := srv.ServeIO(ctx, r, &w); err != nil && err != io.EOF {
		t.Fatalf("ServeIO error: %v", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(w.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse JSON-RPC response: %v, raw: %s", err, w.String())
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	resBytes, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("failed to re-marshal result: %v", err)
	}

	var listResult ToolsListResult
	if err := json.Unmarshal(resBytes, &listResult); err != nil {
		t.Fatalf("failed to unmarshal ToolsListResult: %v", err)
	}

	if len(listResult.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(listResult.Tools))
	}

	expectedNames := map[string]bool{
		"tool_alpha": false,
		"tool_beta":  false,
		"tool_gamma": false,
	}
	for _, tool := range listResult.Tools {
		expectedNames[tool.Name] = true
	}
	for name, found := range expectedNames {
		if !found {
			t.Errorf("tool %s was not found in tools/list response", name)
		}
	}
}

// 2. Unit test: send tools/call for a registered tool, verify handler is invoked and result is returned
func TestToolsCall_Success(t *testing.T) {
	guard := &mockPolicyGuard{decision: types.PolicyALLOW}
	logger := &mockAuditLogger{}
	srv := NewServer(nil, nil, guard, logger)

	mockHandler := &mockToolHandler{
		def: types.ToolDefinition{
			Name:        "custom_builder",
			Description: "Custom test builder",
			RiskTier:    types.RiskWRITESCOPED,
		},
		res: &types.ToolResult{
			Success: true,
			Data: map[string]any{
				"apk_path": "/build/outputs/app-debug.apk",
				"ui_hash":  "ui_abc123",
			},
		},
	}
	srv.Register(mockHandler)

	input := `{"jsonrpc":"2.0","id":202,"method":"tools/call","params":{"name":"custom_builder","arguments":{"variant":"debug","package_id":"com.example.app"}}}` + "\n"
	r := strings.NewReader(input)
	var w bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := srv.ServeIO(ctx, r, &w); err != nil && err != io.EOF {
		t.Fatalf("ServeIO error: %v", err)
	}

	if mockHandler.invokeCnt != 1 {
		t.Fatalf("expected handler to be invoked 1 time, got %d", mockHandler.invokeCnt)
	}
	if mockHandler.lastArgs["variant"] != "debug" {
		t.Errorf("expected argument variant=debug, got %v", mockHandler.lastArgs["variant"])
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(w.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v, raw: %s", err, w.String())
	}

	if resp.Error != nil {
		t.Fatalf("expected no JSON-RPC error, got %+v", resp.Error)
	}

	resBytes, _ := json.Marshal(resp.Result)
	var callRes ToolCallResult
	if err := json.Unmarshal(resBytes, &callRes); err != nil {
		t.Fatalf("failed to unmarshal ToolCallResult: %v", err)
	}

	if callRes.IsError {
		t.Errorf("expected isError=false, got true")
	}
	if len(callRes.Content) == 0 || !strings.Contains(callRes.Content[0].Text, "app-debug.apk") {
		t.Errorf("expected return text to contain app-debug.apk, got: %+v", callRes.Content)
	}

	// Verify Audit Logger received record
	records := logger.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(records))
	}
	rec := records[0]
	if rec.Tool != "custom_builder" {
		t.Errorf("audit tool: expected custom_builder, got %s", rec.Tool)
	}
	if rec.Decision != "ALLOW" {
		t.Errorf("audit decision: expected ALLOW, got %s", rec.Decision)
	}
	if rec.ExitCode != 0 {
		t.Errorf("audit exit code: expected 0, got %d", rec.ExitCode)
	}
	if rec.PackageLock != "com.example.app" {
		t.Errorf("audit package lock: expected com.example.app, got %s", rec.PackageLock)
	}
	if rec.UIHashAfter != "ui_abc123" {
		t.Errorf("audit ui_hash: expected ui_abc123, got %s", rec.UIHashAfter)
	}
}

// 3. Unit test: send tools/call with guard returning DENY, verify error response and audit log entry
func TestToolsCall_PolicyDeny(t *testing.T) {
	guard := &mockPolicyGuard{
		decision: types.PolicyDENY,
		reason:   "Package com.unauthorized not in allowlisted_packages",
	}
	logger := &mockAuditLogger{}
	srv := NewServer(nil, nil, guard, logger)

	mockHandler := &mockToolHandler{
		def: types.ToolDefinition{
			Name:        "dangerous_op",
			Description: "High risk operation",
			RiskTier:    types.RiskWRITESCOPED,
		},
		res: &types.ToolResult{Success: true},
	}
	srv.Register(mockHandler)

	input := `{"jsonrpc":"2.0","id":303,"method":"tools/call","params":{"name":"dangerous_op","arguments":{"package_id":"com.unauthorized"}}}` + "\n"
	r := strings.NewReader(input)
	var w bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := srv.ServeIO(ctx, r, &w); err != nil && err != io.EOF {
		t.Fatalf("ServeIO error: %v", err)
	}

	// Handler MUST NOT have been invoked
	if mockHandler.invokeCnt != 0 {
		t.Fatalf("handler was invoked despite guard returning DENY!")
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(w.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Error == nil {
		t.Fatalf("expected JSON-RPC error response for DENY decision, got nil")
	}
	if resp.Error.Code != CodePolicyDenied {
		t.Errorf("expected error code %d, got %d", CodePolicyDenied, resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "DENY") || !strings.Contains(resp.Error.Message, "not in allowlisted_packages") {
		t.Errorf("unexpected error message: %s", resp.Error.Message)
	}

	// Verify Audit Logger recorded DENY
	records := logger.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(records))
	}
	rec := records[0]
	if rec.Tool != "dangerous_op" {
		t.Errorf("audit tool: expected dangerous_op, got %s", rec.Tool)
	}
	if rec.Decision != "DENY" {
		t.Errorf("audit decision: expected DENY, got %s", rec.Decision)
	}
	if rec.ExitCode != 1 {
		t.Errorf("audit exit code: expected 1 on DENY, got %d", rec.ExitCode)
	}
}

// 4. Unit test: handler panic recovery middleware
func TestToolsCall_PanicRecovery(t *testing.T) {
	logger := &mockAuditLogger{}
	srv := NewServer(nil, nil, nil, logger)

	panickingHandler := &mockToolHandler{
		def: types.ToolDefinition{
			Name:        "buggy_tool",
			Description: "Tool that panics",
			RiskTier:    types.RiskREAD,
		},
		panics: true,
	}
	srv.Register(panickingHandler)

	input := `{"jsonrpc":"2.0","id":404,"method":"tools/call","params":{"name":"buggy_tool","arguments":{}}}` + "\n"
	r := strings.NewReader(input)
	var w bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Must NOT crash the process
	if err := srv.ServeIO(ctx, r, &w); err != nil && err != io.EOF {
		t.Fatalf("ServeIO crashed on panic: %v", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(w.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	resBytes, _ := json.Marshal(resp.Result)
	var callRes ToolCallResult
	_ = json.Unmarshal(resBytes, &callRes)

	if !callRes.IsError {
		t.Errorf("expected call result isError=true for panicking handler")
	}
	if len(callRes.Content) == 0 || !strings.Contains(callRes.Content[0].Text, "panicked") {
		t.Errorf("expected panic error message in content, got: %+v", callRes.Content)
	}

	records := logger.Records()
	if len(records) != 1 || records[0].ExitCode != 1 {
		t.Errorf("expected 1 audit record with exit_code=1 for panicking handler")
	}
}

// 5. Unit test: initialize, initialized notification, and ping
func TestInitializeAndPing(t *testing.T) {
	srv := NewServer(nil, nil, nil, nil)

	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","clientInfo":{"name":"antigravity","version":"2.0"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"ping","params":{}}` + "\n"

	r := strings.NewReader(input)
	var w bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := srv.ServeIO(ctx, r, &w); err != nil && err != io.EOF {
		t.Fatalf("ServeIO error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(w.String()), "\n")
	// Notification produces no response, so exactly 2 responses expected
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses (initialize + ping), got %d: %v", len(lines), lines)
	}

	var initResp JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &initResp); err != nil {
		t.Fatalf("failed to parse initialize response: %v", err)
	}
	if initResp.ID != float64(1) {
		t.Errorf("expected initialize ID=1, got %v", initResp.ID)
	}

	var pingResp JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[1]), &pingResp); err != nil {
		t.Fatalf("failed to parse ping response: %v", err)
	}
	if pingResp.ID != float64(2) {
		t.Errorf("expected ping ID=2, got %v", pingResp.ID)
	}
}

// 6. Unit test: All 26 canonical tools registered
func TestAll26CanonicalToolsRegistered(t *testing.T) {
	srv := NewServer(nil, nil, nil, nil)
	RegisterAllStubs(srv)

	handlers := srv.ListHandlers()
	if len(handlers) != 26 {
		t.Fatalf("expected exactly 26 canonical tools, got %d", len(handlers))
	}

	// Verify each canonical tool responds to tools/call
	for _, tool := range handlers {
		input := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":{}}}`+"\n", tool.Name)
		r := strings.NewReader(input)
		var w bytes.Buffer

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		err := srv.ServeIO(ctx, r, &w)
		cancel()

		if err != nil && err != io.EOF {
			t.Fatalf("ServeIO error for %s: %v", tool.Name, err)
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal(w.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response for %s: %v", tool.Name, err)
		}
		if resp.Error != nil {
			t.Errorf("tool %s returned unexpected error: %+v", tool.Name, resp.Error)
		}
	}
}

// 7. Unit test: Config and Policy loader and validator
func TestConfigAndPolicyLoader(t *testing.T) {
	tmpDir := t.TempDir()

	validYAML := `schema_version: 1
project:
  name: "test-app"
  mode: "source"
  gradle_root: "."
execution:
  max_steps_per_task: 12
  max_identical_ui_hashes: 2
`
	yamlPath := filepath.Join(tmpDir, "droidforge.yaml")
	if err := os.WriteFile(yamlPath, []byte(validYAML), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(yamlPath)
	if err != nil {
		t.Fatalf("failed to load valid config: %v", err)
	}
	if cfg.Project.Name != "test-app" || cfg.Execution.MaxStepsPerTask != 12 {
		t.Errorf("unexpected loaded config values: %+v", cfg)
	}

	// Test invalid config (schema_version 0)
	invalidYAML := `schema_version: 0
project:
  name: ""
`
	invalidYAMLPath := filepath.Join(tmpDir, "invalid.yaml")
	_ = os.WriteFile(invalidYAMLPath, []byte(invalidYAML), 0644)
	if _, err := LoadConfig(invalidYAMLPath); err == nil {
		t.Errorf("expected error for invalid config, got nil")
	}

	// Test valid policy
	validPolicy := `{
  "schema_version": 1,
  "execution": {
    "allow_raw_shell": false
  },
  "sanitization": {
    "max_node_text_bytes": 256,
    "max_logcat_line_bytes": 512
  },
  "audit": {
    "log_dir": "/tmp/audit"
  }
}`
	polPath := filepath.Join(tmpDir, "droidforge.policy.json")
	if err := os.WriteFile(polPath, []byte(validPolicy), 0644); err != nil {
		t.Fatal(err)
	}

	pol, err := LoadPolicy(polPath)
	if err != nil {
		t.Fatalf("failed to load valid policy: %v", err)
	}
	if pol.Audit.LogDir != "/tmp/audit" {
		t.Errorf("unexpected policy log_dir: %s", pol.Audit.LogDir)
	}

	// Test policy violation (allow_raw_shell: true must be rejected)
	unsafePolicy := `{
  "schema_version": 1,
  "execution": { "allow_raw_shell": true },
  "sanitization": { "max_node_text_bytes": 256, "max_logcat_line_bytes": 512 },
  "audit": { "log_dir": "/tmp/audit" }
}`
	unsafePolPath := filepath.Join(tmpDir, "unsafe.policy.json")
	_ = os.WriteFile(unsafePolPath, []byte(unsafePolicy), 0644)
	if _, err := LoadPolicy(unsafePolPath); err == nil {
		t.Errorf("expected error when allow_raw_shell is true, got nil")
	}
}

// 8. Unit test: Project Scaffolding
func TestScaffoldProject(t *testing.T) {
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "scaffold_target")

	if err := ScaffoldProject(targetDir, "sample-app"); err != nil {
		t.Fatalf("ScaffoldProject failed: %v", err)
	}

	expectedFiles := []string{
		"droidforge.yaml",
		".mcp.json",
		"AGENTS.md",
		"droidforge.policy.json",
		filepath.Join(".droidforge", "appcard.md"),
	}

	for _, file := range expectedFiles {
		p := filepath.Join(targetDir, file)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Errorf("expected scaffolded file %s does not exist", p)
		}
	}

	// Verify scaffolded config parses cleanly
	if _, err := LoadConfig(filepath.Join(targetDir, "droidforge.yaml")); err != nil {
		t.Errorf("scaffolded droidforge.yaml failed validation: %v", err)
	}
	// Verify scaffolded policy parses cleanly
	if _, err := LoadPolicy(filepath.Join(targetDir, "droidforge.policy.json")); err != nil {
		t.Errorf("scaffolded droidforge.policy.json failed validation: %v", err)
	}
}

// 9. Unit test: Doctor runs without crash on host
func TestDoctorExecution(t *testing.T) {
	ctx := context.Background()
	results := RunDoctor(ctx, nil)

	if len(results) == 0 {
		t.Fatalf("expected doctor results, got 0")
	}

	var buf bytes.Buffer
	passed := PrintDoctorReport(&buf, results)
	_ = passed

	out := buf.String()
	if !strings.Contains(out, "DroidForge MCP Environment Doctor") {
		t.Errorf("doctor output missing header: %s", out)
	}
	if !strings.Contains(out, "Summary:") {
		t.Errorf("doctor output missing summary: %s", out)
	}
}

// 10. Unit test: Request cancellation via notifications/cancelled
func TestRequestCancellation(t *testing.T) {
	srv := NewServer(nil, nil, nil, nil)

	handler := &mockBlockingToolHandler{
		def: types.ToolDefinition{
			Name:     "blocking_tool",
			RiskTier: types.RiskREAD,
		},
		started: make(chan struct{}),
	}
	srv.Register(handler)

	pr, pw := io.Pipe()
	var w bytes.Buffer

	go func() {
		_, _ = pw.Write([]byte(`{"jsonrpc":"2.0","id":"req_async","method":"tools/call","params":{"name":"blocking_tool"}}` + "\n"))
		<-handler.started
		_, _ = pw.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"req_async","reason":"user aborted"}}` + "\n"))
		_ = pw.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_ = srv.ServeIO(ctx, pr, &w)

	var resp JSONRPCResponse
	if err := json.Unmarshal(w.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse JSON-RPC: %v, raw: %s", err, w.String())
	}

	if resp.ID != "req_async" {
		t.Errorf("expected ID 'req_async', got %v", resp.ID)
	}

	resBytes, _ := json.Marshal(resp.Result)
	var callRes ToolCallResult
	_ = json.Unmarshal(resBytes, &callRes)
	if !callRes.IsError {
		t.Errorf("expected IsError=true on cancelled call, got false")
	}
	if len(callRes.Content) == 0 || !strings.Contains(callRes.Content[0].Text, "cancelled") {
		t.Errorf("expected cancellation message, got: %+v", callRes.Content)
	}
}

type mockBlockingToolHandler struct {
	def     types.ToolDefinition
	started chan struct{}
}

func (m *mockBlockingToolHandler) Definition() types.ToolDefinition {
	return m.def
}

func (m *mockBlockingToolHandler) Execute(ctx context.Context, args map[string]any) (*types.ToolResult, error) {
	close(m.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

// 11. Unit test: PrintDoctorJSON
func TestPrintDoctorJSON(t *testing.T) {
	results := []CheckResult{
		{Name: "TestADB", Status: StatusOK, Version: "1.0.41"},
		{Name: "TestEmulator", Status: StatusWarn, Details: "accel warning"},
	}
	var buf bytes.Buffer
	if err := PrintDoctorJSON(&buf, results); err != nil {
		t.Fatalf("PrintDoctorJSON failed: %v", err)
	}
	var parsed []CheckResult
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse json output: %v", err)
	}
	if len(parsed) != 2 {
		t.Errorf("expected 2 items, got %d", len(parsed))
	}
	if parsed[0].Name != "TestADB" || parsed[0].Status != StatusOK {
		t.Errorf("unexpected item: %+v", parsed[0])
	}
}

// 12. Unit test: End-to-end JSON-RPC invocation of Compiler Tools (#23, #24, #25)
func TestServer_CompilerTools_JSONRPC(t *testing.T) {
	tempDir := t.TempDir()
	flowPath := filepath.Join(tempDir, "sample_flow.yaml")
	appCardPath := filepath.Join(tempDir, "appcard.md")

	mockExec := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("Flow execution succeeded"), nil
	}

	runner := compiler.NewMaestroRunner(mockExec)
	compilerInstance := compiler.NewCompiler(runner)
	compilerInstance.SetDefaultAppID("com.example.testapp")

	sessionID := "sess_mcp_e2e"
	compilerInstance.RecordStep(sessionID, types.SessionStep{
		Action:       "tap_node",
		Target:       "button_ok",
		UIHashBefore: "hash_0",
		UIHashAfter:  "hash_1",
		Success:      true,
		NodesAfter: []types.PrunedNode{
			{ID: "n1", ResID: "com.example.testapp:id/button_ok", Class: "Button"},
		},
	})

	srv := NewServer(nil, nil, nil, nil)
	compiler.RegisterCompilerTools(srv, compilerInstance)

	// A. Check tools/list via dispatch
	reqList := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}
	respList := srv.dispatch(context.Background(), reqList)
	if respList == nil || respList.Error != nil {
		t.Fatalf("tools/list failed: %+v", respList)
	}
	listBytes, _ := json.Marshal(respList.Result)
	var listRes ToolsListResult
	_ = json.Unmarshal(listBytes, &listRes)

	foundFreeze, foundRun, foundAppCard := false, false, false
	for _, tool := range listRes.Tools {
		switch tool.Name {
		case "session_freeze_to_maestro":
			foundFreeze = true
		case "test_run_maestro":
			foundRun = true
		case "session_export_appcard":
			foundAppCard = true
		}
	}
	if !foundFreeze || !foundRun || !foundAppCard {
		t.Errorf("missing compiler tools in tools/list: freeze=%v, run=%v, appcard=%v",
			foundFreeze, foundRun, foundAppCard)
	}

	// B. Call session_freeze_to_maestro via dispatch
	freezeArgs, _ := json.Marshal(map[string]any{
		"name": "session_freeze_to_maestro",
		"arguments": map[string]any{
			"session_id":  sessionID,
			"flow_name":   "SampleFlow",
			"output_path": flowPath,
		},
	})
	reqFreeze := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params:  freezeArgs,
	}
	respFreeze := srv.dispatch(context.Background(), reqFreeze)
	if respFreeze == nil || respFreeze.Error != nil {
		t.Fatalf("tools/call session_freeze_to_maestro failed: %+v", respFreeze)
	}

	// Verify output YAML was created
	if _, err := os.Stat(flowPath); os.IsNotExist(err) {
		t.Fatalf("expected flow file at %s, not found", flowPath)
	}

	// C. Call test_run_maestro via dispatch
	runArgs, _ := json.Marshal(map[string]any{
		"name": "test_run_maestro",
		"arguments": map[string]any{
			"flow_path":    flowPath,
			"serial":       "emulator-5554",
			"repeat_count": 2,
		},
	})
	reqRun := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params:  runArgs,
	}
	respRun := srv.dispatch(context.Background(), reqRun)
	if respRun == nil || respRun.Error != nil {
		t.Fatalf("tools/call test_run_maestro failed: %+v", respRun)
	}

	// D. Call session_export_appcard via dispatch
	appCardArgs, _ := json.Marshal(map[string]any{
		"name": "session_export_appcard",
		"arguments": map[string]any{
			"session_id":  sessionID,
			"output_path": appCardPath,
		},
	})
	reqAppCard := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params:  appCardArgs,
	}
	respAppCard := srv.dispatch(context.Background(), reqAppCard)
	if respAppCard == nil || respAppCard.Error != nil {
		t.Fatalf("tools/call session_export_appcard failed: %+v", respAppCard)
	}

	if _, err := os.Stat(appCardPath); os.IsNotExist(err) {
		t.Fatalf("expected appcard file at %s, not found", appCardPath)
	}
}

