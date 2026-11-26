package mcp_compiler

import (
	"bufio"
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
)

// Helper to run client-server RPC roundtrips over in-memory pipes.
func setupTestServer(t *testing.T) (*Server, *ToolRegistry, *MockCoordinator, *io.PipeWriter, *bufio.Scanner, context.CancelFunc) {
	t.Helper()
	coord := NewMockCoordinator()
	compiler := NewMaestroCompiler("com.prism.v")
	reg := NewToolRegistry(coord, compiler)

	var logBuf bytes.Buffer
	server := NewServer(reg, &logBuf)

	clientInR, serverInW := io.Pipe()
	serverOutR, clientOutW := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = server.ServeIO(ctx, clientInR, clientOutW)
		_ = clientOutW.Close()
	}()

	scanner := bufio.NewScanner(serverOutR)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	return server, reg, coord, serverInW, scanner, cancel
}

func sendRequest(w io.Writer, req any) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

func readResponse(scanner *bufio.Scanner) (*JSONRPCResponse, error) {
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	var resp JSONRPCResponse
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w (raw: %s)", err, scanner.Text())
	}
	return &resp, nil
}

// Test 1: Initialize, initialized notification, and tools/list
func TestServer_InitializeAndToolsList(t *testing.T) {
	_, _, _, serverIn, scanner, cancel := setupTestServer(t)
	defer cancel()
	defer serverIn.Close()

	// 1. Send initialize
	initReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: json.RawMessage(`{
			"protocolVersion": "2024-11-05",
			"clientInfo": {"name": "Antigravity", "version": "2.0"}
		}`),
	}
	if err := sendRequest(serverIn, initReq); err != nil {
		t.Fatalf("send initialize failed: %v", err)
	}

	resp, err := readResponse(scanner)
	if err != nil {
		t.Fatalf("read initialize response failed: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("initialize returned error: %v", resp.Error)
	}

	initData, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal result failed: %v", err)
	}
	var initResult InitializeResult
	if err := json.Unmarshal(initData, &initResult); err != nil {
		t.Fatalf("unmarshal initResult failed: %v", err)
	}
	if initResult.ProtocolVersion != MCPProtocolVersion {
		t.Errorf("expected protocol %s, got %s", MCPProtocolVersion, initResult.ProtocolVersion)
	}
	if initResult.ServerInfo.Name != "nano-android-mcp" {
		t.Errorf("expected server name nano-android-mcp, got %s", initResult.ServerInfo.Name)
	}

	// 2. Send initialized notification (should not produce response)
	notifReq := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	if err := sendRequest(serverIn, notifReq); err != nil {
		t.Fatalf("send notification failed: %v", err)
	}

	// 3. Send ping
	pingReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "ping",
	}
	if err := sendRequest(serverIn, pingReq); err != nil {
		t.Fatalf("send ping failed: %v", err)
	}
	pingResp, err := readResponse(scanner)
	if err != nil {
		t.Fatalf("read ping failed: %v", err)
	}
	if pingResp.Error != nil {
		t.Errorf("ping returned error: %v", pingResp.Error)
	}

	// 4. Send tools/list
	listReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/list",
	}
	if err := sendRequest(serverIn, listReq); err != nil {
		t.Fatalf("send tools/list failed: %v", err)
	}
	listResp, err := readResponse(scanner)
	if err != nil {
		t.Fatalf("read tools/list failed: %v", err)
	}
	if listResp.Error != nil {
		t.Fatalf("tools/list error: %v", listResp.Error)
	}

	rawList, _ := json.Marshal(listResp.Result)
	var listResult ToolsListResult
	if err := json.Unmarshal(rawList, &listResult); err != nil {
		t.Fatalf("unmarshal ToolsListResult failed: %v", err)
	}

	expectedTools := map[string]bool{
		"nano_launch_app":       false,
		"nano_observe_screen":   false,
		"nano_dispatch_action":  false,
		"nano_step_time":        false,
		"nano_inspect_state":    false,
		"nano_freeze_test":      false,
		"nano_assert_ui":        false,
		"nano_get_session_flow": false,
		"nano_clear_session":    false,
		"nano_doctor":           false,
	}

	for _, tool := range listResult.Tools {
		if _, exists := expectedTools[tool.Name]; exists {
			expectedTools[tool.Name] = true
			if tool.Description == "" {
				t.Errorf("tool %s has empty description", tool.Name)
			}
			if tool.InputSchema == nil {
				t.Errorf("tool %s has nil inputSchema", tool.Name)
			}
		}
	}

	for toolName, found := range expectedTools {
		if !found {
			t.Errorf("expected tool %s not found in tools/list", toolName)
		}
	}
}

// Test 2: Sequential tool execution across standard tools
func TestServer_ToolDispatch_AllTools(t *testing.T) {
	_, _, coord, serverIn, scanner, cancel := setupTestServer(t)
	defer cancel()
	defer serverIn.Close()

	// 1. nano_launch_app
	launchReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      101,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "nano_launch_app",
			"arguments": {
				"apk_path": "d:/apks/prism_video.apk",
				"activity": "com.prism.v.MainActivity",
				"clear_state": true
			}
		}`),
	}
	_ = sendRequest(serverIn, launchReq)
	launchResp, err := readResponse(scanner)
	if err != nil {
		t.Fatalf("launch read error: %v", err)
	}
	if launchResp.Error != nil {
		t.Fatalf("launch error: %v", launchResp.Error)
	}

	// 2. nano_observe_screen
	obsReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      102,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "nano_observe_screen",
			"arguments": {
				"include_screenshot": true,
				"format": "webp"
			}
		}`),
	}
	_ = sendRequest(serverIn, obsReq)
	obsResp, err := readResponse(scanner)
	if err != nil {
		t.Fatalf("observe read error: %v", err)
	}
	if obsResp.Error != nil {
		t.Fatalf("observe error: %v", obsResp.Error)
	}

	// 3. nano_dispatch_action (tap "Studio")
	tapReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      103,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "nano_dispatch_action",
			"arguments": {
				"action": "tap",
				"node_id": "tab_studio",
				"assert_visible": "Resolution Preset"
			}
		}`),
	}
	_ = sendRequest(serverIn, tapReq)
	tapResp, err := readResponse(scanner)
	if err != nil {
		t.Fatalf("tap read error: %v", err)
	}
	if tapResp.Error != nil {
		t.Fatalf("tap error: %v", tapResp.Error)
	}

	// 4. nano_step_time
	stepReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      104,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "nano_step_time",
			"arguments": {
				"delta_ms": 160
			}
		}`),
	}
	_ = sendRequest(serverIn, stepReq)
	stepResp, err := readResponse(scanner)
	if err != nil {
		t.Fatalf("step read error: %v", err)
	}
	if stepResp.Error != nil {
		t.Fatalf("step error: %v", stepResp.Error)
	}

	// 5. nano_inspect_state
	inspectReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      105,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "nano_inspect_state",
			"arguments": {
				"target": "heap"
			}
		}`),
	}
	_ = sendRequest(serverIn, inspectReq)
	inspectResp, err := readResponse(scanner)
	if err != nil {
		t.Fatalf("inspect read error: %v", err)
	}
	if inspectResp.Error != nil {
		t.Fatalf("inspect error: %v", inspectResp.Error)
	}

	// 6. nano_freeze_test
	freezeReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      106,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "nano_freeze_test",
			"arguments": {
				"app_id": "com.prism.v",
				"repeat_count": 3
			}
		}`),
	}
	_ = sendRequest(serverIn, freezeReq)
	freezeResp, err := readResponse(scanner)
	if err != nil {
		t.Fatalf("freeze read error: %v", err)
	}
	if freezeResp.Error != nil {
		t.Fatalf("freeze error: %v", freezeResp.Error)
	}

	if coord.totalCalls < 5 {
		t.Errorf("expected at least 5 coordinator calls, got %d", coord.totalCalls)
	}
}

// Test 3: 50+ Rapid-fire concurrent requests over piped stdio buffers verifying zero deadlocks
func TestServer_RapidFireConcurrentRequests_50Plus(t *testing.T) {
	_, _, _, serverIn, scanner, cancel := setupTestServer(t)
	defer cancel()
	defer serverIn.Close()

	const requestCount = 60

	var mu sync.Mutex
	received := make(map[string]bool)
	doneChan := make(chan struct{})

	go func() {
		defer close(doneChan)
		for {
			resp, err := readResponse(scanner)
			if err != nil {
				return
			}
			idKey := fmt.Sprintf("%v", resp.ID)
			mu.Lock()
			received[idKey] = true
			count := len(received)
			mu.Unlock()

			if count == requestCount {
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 1; i <= requestCount; i++ {
		wg.Add(1)
		go func(reqID int) {
			defer wg.Done()

			var req JSONRPCRequest
			switch reqID % 4 {
			case 0:
				req = JSONRPCRequest{
					JSONRPC: "2.0",
					ID:      reqID,
					Method:  "ping",
				}
			case 1:
				req = JSONRPCRequest{
					JSONRPC: "2.0",
					ID:      reqID,
					Method:  "tools/list",
				}
			case 2:
				req = JSONRPCRequest{
					JSONRPC: "2.0",
					ID:      reqID,
					Method:  "tools/call",
					Params: json.RawMessage(fmt.Sprintf(`{
						"name": "nano_step_time",
						"arguments": {"delta_ms": %d}
					}`, reqID*10)),
				}
			case 3:
				req = JSONRPCRequest{
					JSONRPC: "2.0",
					ID:      reqID,
					Method:  "tools/call",
					Params: json.RawMessage(`{
						"name": "nano_inspect_state",
						"arguments": {"target": "all"}
					}`),
				}
			}

			_ = sendRequest(serverIn, req)
		}(i)
	}

	wg.Wait()

	select {
	case <-doneChan:
		// Success
	case <-time.After(10 * time.Second):
		mu.Lock()
		got := len(received)
		mu.Unlock()
		t.Fatalf("DEADLOCK DETECTED! Expected %d responses, but only got %d within timeout", requestCount, got)
	}

	mu.Lock()
	defer mu.Unlock()
	for i := 1; i <= requestCount; i++ {
		idKey := fmt.Sprintf("%d", i)
		if !received[idKey] {
			t.Errorf("missing response for request ID %s", idKey)
		}
	}
}

// Test 4: Maestro Compiler generating exact YAML flow from interactive session
func TestMaestroCompiler_InteractiveSessionToYAML(t *testing.T) {
	compiler := NewMaestroCompiler("com.prism.v")
	sampleAST := BuildPrismSampleAST()
	compiler.UpdateAST(sampleAST)

	// Step 1: launch app
	compiler.RecordLaunch("com.prism.v", false)

	// Step 2: tap on "Studio" (node ID tab_studio resolves to text "Studio")
	compiler.RecordTap("tab_studio", "", [2]float64{0, 0}, false)

	// Step 3: assertVisible: "Resolution Preset"
	compiler.RecordAssertVisible("Resolution Preset", "")

	// Step 4: tap on "Ultra 4K" (node ID btn_ultra_4k resolves to "Ultra 4K")
	compiler.RecordTap("btn_ultra_4k", "", [2]float64{0, 0}, false)

	// Step 5: assertVisible: "Estimated Size: 148.6 MB"
	compiler.RecordAssertVisible("Estimated Size: 148.6 MB", "")

	opts := CompileOptions{
		AppID:           "com.prism.v",
		FlowName:        "prism_ultra_4k_regression",
		RepeatCount:     3,
		IncludeComments: true,
	}

	yamlOutput, err := compiler.Compile(opts)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	expectedSnippets := []string{
		"appId: com.prism.v",
		"---",
		"- launchApp",
		`- tapOn: "Studio"`,
		`- assertVisible: "Resolution Preset"`,
		`- tapOn: "Ultra 4K"`,
		`- assertVisible: "Estimated Size: 148.6 MB"`,
		"# repeat_count: 3",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(yamlOutput, snippet) {
			t.Errorf("compiled YAML missing expected snippet:\nSnippet: %q\nFull YAML:\n%s", snippet, yamlOutput)
		}
	}

	tempDir, err := os.MkdirTemp("", "maestro_test")
	if err != nil {
		t.Fatalf("create temp dir failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "flow.yaml")
	opts.OutputPath = filePath
	if err := compiler.CompileToFile(filePath, opts); err != nil {
		t.Fatalf("compile to file failed: %v", err)
	}

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read written flow failed: %v", err)
	}
	if string(fileContent) != yamlOutput {
		t.Errorf("written file does not match compiled string")
	}
}

// Test 5: Error handling, parse errors, and invalid methods
func TestServer_ErrorHandlingAndMalformedJSON(t *testing.T) {
	_, _, _, serverIn, scanner, cancel := setupTestServer(t)
	defer cancel()
	defer serverIn.Close()

	// 1. Send malformed JSON line
	_, _ = serverIn.Write([]byte("{ malformed json string\n"))
	resp1, err := readResponse(scanner)
	if err != nil {
		t.Fatalf("read malformed resp failed: %v", err)
	}
	if resp1.Error == nil || resp1.Error.Code != CodeParseError {
		t.Errorf("expected CodeParseError (-32700), got %v", resp1.Error)
	}

	// 2. Send unknown method
	unknownReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      999,
		Method:  "non_existent_method",
	}
	_ = sendRequest(serverIn, unknownReq)
	resp2, err := readResponse(scanner)
	if err != nil {
		t.Fatalf("read unknown method resp failed: %v", err)
	}
	if resp2.Error == nil || resp2.Error.Code != CodeMethodNotFound {
		t.Errorf("expected CodeMethodNotFound (-32601), got %v", resp2.Error)
	}

	// 3. Send tools/call with unknown tool name
	toolReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1000,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "non_existent_tool",
			"arguments": {}
		}`),
	}
	_ = sendRequest(serverIn, toolReq)
	resp3, err := readResponse(scanner)
	if err != nil {
		t.Fatalf("read tool call resp failed: %v", err)
	}
	if resp3.Result != nil {
		resRaw, _ := json.Marshal(resp3.Result)
		var tr ToolResult
		_ = json.Unmarshal(resRaw, &tr)
		if !tr.IsError {
			t.Errorf("expected isError=true for unknown tool")
		}
	} else if resp3.Error == nil {
		t.Errorf("expected error for unknown tool call")
	}

	// 4. Send cancellation notification
	cancelNotif := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/cancelled",
		Params:  json.RawMessage(`{"requestId": 1000, "reason": "user cancelled"}`),
	}
	_ = sendRequest(serverIn, cancelNotif)
}

// Test 6: Advanced gestures, repeat blocks, and key events
func TestMaestroCompiler_AdvancedGesturesAndRepeatBlock(t *testing.T) {
	compiler := NewMaestroCompiler("com.prism.v")

	compiler.RecordLaunch("com.prism.v", true)
	compiler.RecordType("SecretProject", "edt_title")
	compiler.RecordSwipe("UP")
	compiler.RecordSleep(500)
	compiler.RecordPressKey("ENTER")
	compiler.RecordBack()

	opts := CompileOptions{
		AppID:       "com.prism.v",
		FlowName:    "gestures_flow",
		RepeatCount: 3,
		WrapRepeat:  true,
	}

	yaml, err := compiler.Compile(opts)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	if !strings.Contains(yaml, "clearState: true") {
		t.Errorf("missing clearState in launchApp")
	}
	if !strings.Contains(yaml, `- repeat:`) || !strings.Contains(yaml, `times: 3`) {
		t.Errorf("missing repeat block in YAML:\n%s", yaml)
	}
	if !strings.Contains(yaml, `inputText: "SecretProject"`) {
		t.Errorf("missing inputText in YAML:\n%s", yaml)
	}
	if !strings.Contains(yaml, `direction: UP`) {
		t.Errorf("missing swipe direction in YAML:\n%s", yaml)
	}
	if !strings.Contains(yaml, `pressKey: ENTER`) {
		t.Errorf("missing pressKey in YAML:\n%s", yaml)
	}
	if !strings.Contains(yaml, `sleep: 500`) {
		t.Errorf("missing sleep in YAML:\n%s", yaml)
	}
	if !strings.Contains(yaml, `- back`) {
		t.Errorf("missing back in YAML:\n%s", yaml)
	}
}

// Test 7: Coordinator Sub-Mocks independently
func TestCoordinatorMocks(t *testing.T) {
	ctx := context.Background()
	coord := NewMockCoordinator()

	lRes, err := coord.LaunchApp(ctx, "prism.apk", "MainActivity", false)
	if err != nil || lRes.Status != "running" {
		t.Fatalf("mock launch failed: %v", err)
	}

	oRes, err := coord.ObserveScreen(ctx, true, "webp", 0)
	if err != nil || oRes.Screenshot == nil || oRes.AST == nil {
		t.Fatalf("mock observe failed: %v", err)
	}

	dRes, err := coord.DispatchAction(ctx, Action{
		Type:          "tap",
		NodeID:        "tab_studio",
		AssertVisible: "Resolution Preset",
	})
	if err != nil || !dRes.AssertionMet {
		t.Fatalf("mock dispatch failed: %v", err)
	}

	sRes, err := coord.StepTime(ctx, 32)
	if err != nil || sRes.SteppedMs != 32 {
		t.Fatalf("mock step failed: %v", err)
	}

	iRes, err := coord.InspectState(ctx, "heap")
	if err != nil || iRes.Heap == nil {
		t.Fatalf("mock inspect failed: %v", err)
	}
}

// Test 8: JSON-RPC 2.0 Batch Requests Handling
func TestServer_BatchRequests(t *testing.T) {
	_, _, _, serverIn, scanner, cancel := setupTestServer(t)
	defer cancel()
	defer serverIn.Close()

	// Send a batch of 3 requests: [ping, tools/list, ping]
	batch := []JSONRPCRequest{
		{JSONRPC: "2.0", ID: "batch_1", Method: "ping"},
		{JSONRPC: "2.0", ID: "batch_2", Method: "tools/list"},
		{JSONRPC: "2.0", ID: "batch_3", Method: "ping"},
	}
	data, _ := json.Marshal(batch)
	data = append(data, '\n')
	_, _ = serverIn.Write(data)

	if !scanner.Scan() {
		t.Fatalf("failed reading batch response: %v", scanner.Err())
	}

	var responses []JSONRPCResponse
	if err := json.Unmarshal(scanner.Bytes(), &responses); err != nil {
		t.Fatalf("failed unmarshaling batch response: %v (raw: %s)", err, scanner.Text())
	}

	if len(responses) != 3 {
		t.Fatalf("expected 3 responses in batch, got %d", len(responses))
	}

	idMap := make(map[string]bool)
	for _, r := range responses {
		idMap[fmt.Sprintf("%v", r.ID)] = true
	}
	if !idMap["batch_1"] || !idMap["batch_2"] || !idMap["batch_3"] {
		t.Errorf("batch responses missing expected IDs: %v", idMap)
	}
}

// Test 9: MCP logging/setLevel Support
func TestServer_LoggingSetLevel(t *testing.T) {
	server, _, _, serverIn, scanner, cancel := setupTestServer(t)
	defer cancel()
	defer serverIn.Close()

	setLvlReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      501,
		Method:  "logging/setLevel",
		Params:  json.RawMessage(`{"level": "warning"}`),
	}
	_ = sendRequest(serverIn, setLvlReq)

	resp, err := readResponse(scanner)
	if err != nil {
		t.Fatalf("read setLevel response failed: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("logging/setLevel returned error: %v", resp.Error)
	}

	server.mu.RLock()
	lvl := server.logLevel
	server.mu.RUnlock()

	if lvl != LogLevelWarning {
		t.Errorf("expected server logLevel %v, got %v", LogLevelWarning, lvl)
	}
}

// Test 10: Optimizer and Linting Validation
func TestMaestroCompiler_OptimizerAndValidation(t *testing.T) {
	compiler := NewMaestroCompiler("com.prism.v")

	// Add consecutive sleeps
	compiler.RecordSleep(100)
	compiler.RecordSleep(150)
	compiler.RecordSleep(50)

	// Add duplicate assertions
	compiler.RecordAssertVisible("Studio", "tab_studio")
	compiler.RecordAssertVisible("Studio", "tab_studio")

	if compiler.CommandCount() != 5 {
		t.Fatalf("expected 5 initial commands, got %d", compiler.CommandCount())
	}

	eliminated := compiler.Optimize()
	if eliminated != 3 {
		t.Errorf("expected 3 commands eliminated by optimizer, got %d", eliminated)
	}

	cmds := compiler.GetCommands()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 optimized commands, got %d", len(cmds))
	}

	if cmds[0].Type != CmdSleep || cmds[0].DurationMs != 300 {
		t.Errorf("expected coalesced sleep of 300ms, got %+v", cmds[0])
	}
	if cmds[1].Type != CmdAssertVisible || cmds[1].Target != "Studio" {
		t.Errorf("expected single assertVisible, got %+v", cmds[1])
	}

	// Test Validation
	errors := compiler.Validate()
	if len(errors) != 0 {
		t.Errorf("expected 0 validation errors on clean commands, got %v", errors)
	}

	// Invalid test: empty tap target
	compiler.commands = append(compiler.commands, MaestroCommand{Type: CmdTapOn})
	errors = compiler.Validate()
	if len(errors) == 0 {
		t.Errorf("expected validation error on empty tapOn target")
	}
}

// Test 11: Production Tools: nano_assert_ui, nano_doctor, nano_get_session_flow, nano_clear_session
func TestMCPTools_ProductionSuite(t *testing.T) {
	_, _, _, serverIn, scanner, cancel := setupTestServer(t)
	defer cancel()
	defer serverIn.Close()

	// 1. nano_doctor
	docReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      601,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "nano_doctor",
			"arguments": {"verbose": true}
		}`),
	}
	_ = sendRequest(serverIn, docReq)
	docResp, err := readResponse(scanner)
	if err != nil || docResp.Error != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	// 2. nano_assert_ui (verify "Studio" is visible)
	assertReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      602,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "nano_assert_ui",
			"arguments": {
				"assertion_type": "visible",
				"target": "Studio",
				"record": true
			}
		}`),
	}
	_ = sendRequest(serverIn, assertReq)
	assertResp, err := readResponse(scanner)
	if err != nil || assertResp.Error != nil {
		t.Fatalf("assert_ui failed: %v", err)
	}

	// 3. nano_get_session_flow
	flowReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      603,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "nano_get_session_flow",
			"arguments": {"repeat_count": 3}
		}`),
	}
	_ = sendRequest(serverIn, flowReq)
	flowResp, err := readResponse(scanner)
	if err != nil || flowResp.Error != nil {
		t.Fatalf("get_session_flow failed: %v", err)
	}

	// 4. nano_clear_session
	clearReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      604,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "nano_clear_session",
			"arguments": {"confirm": true}
		}`),
	}
	_ = sendRequest(serverIn, clearReq)
	clearResp, err := readResponse(scanner)
	if err != nil || clearResp.Error != nil {
		t.Fatalf("clear_session failed: %v", err)
	}
}

// Test 12: Coordinator Snapshots and Health Diagnostics
func TestCoordinator_SnapshotsAndHealth(t *testing.T) {
	ctx := context.Background()
	coord := NewMockCoordinator()

	// 1. Health check
	health, err := coord.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	if !health.OverallHealthy || !health.RuntimeHealthy {
		t.Errorf("expected coordinator healthy, got %+v", health)
	}

	// 2. Save snapshot
	if err := coord.SaveSnapshot("before_mutation"); err != nil {
		t.Fatalf("save snapshot failed: %v", err)
	}

	// 3. Mutate AST state by dispatching typing action
	_, err = coord.DispatchAction(ctx, Action{
		Type:   "type",
		NodeID: "tab_studio",
		Text:   "Mutated Studio",
	})
	if err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}

	// Verify mutation
	astAfter, _ := coord.ObserveScreen(ctx, false, "webp", 0)
	node := astAfter.AST.FindByID("tab_studio")
	if node == nil || node.Text != "Mutated Studio" {
		t.Fatalf("expected mutated text, got %v", node)
	}

	// 4. Restore snapshot
	if err := coord.RestoreSnapshot("before_mutation"); err != nil {
		t.Fatalf("restore snapshot failed: %v", err)
	}

	// Verify restored state
	astRestored, _ := coord.ObserveScreen(ctx, false, "webp", 0)
	restoredNode := astRestored.AST.FindByID("tab_studio")
	if restoredNode == nil || restoredNode.Text != "Studio" {
		t.Errorf("expected restored text 'Studio', got %v", restoredNode)
	}
}

// Benchmark 1: High-throughput JSON-RPC dispatch
func BenchmarkJSONRPCDispatch(b *testing.B) {
	coord := NewMockCoordinator()
	compiler := NewMaestroCompiler("com.prism.v")
	reg := NewToolRegistry(coord, compiler)
	server := NewServer(reg, io.Discard)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "nano_step_time",
			"arguments": {"delta_ms": 16}
		}`),
	}

	ctx := context.Background()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp := server.handleRequest(ctx, &req)
			if resp == nil || resp.Error != nil {
				b.Fatalf("benchmark request failed: %v", resp)
			}
		}
	})
}

// Benchmark 2: Maestro Compiler compilation speed
func BenchmarkMaestroCompile(b *testing.B) {
	compiler := NewMaestroCompiler("com.prism.v")
	compiler.RecordLaunch("com.prism.v", false)
	for i := 0; i < 20; i++ {
		compiler.RecordTap("tab_studio", "Studio", [2]float64{0, 0}, false)
		compiler.RecordAssertVisible("Resolution Preset", "")
		compiler.RecordSleep(50)
	}

	opts := DefaultCompileOptions()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := compiler.Compile(opts)
		if err != nil {
			b.Fatalf("compile failed: %v", err)
		}
	}
}

func TestSecurity_SanitizeAndPathValidation(t *testing.T) {
	coord := NewMockCoordinator()
	compiler := NewMaestroCompiler("com.prism.v")
	reg := NewToolRegistry(coord, compiler)
	ctx := context.Background()

	// 1. Reject bad apk_path extension and null bytes
	if res, _ := reg.Call(ctx, "nano_launch_app", map[string]any{"apk_path": "/etc/passwd"}); res == nil || !res.IsError {
		t.Fatalf("expected error for non-apk/jar apk_path, got %+v", res)
	}
	if res, _ := reg.Call(ctx, "nano_launch_app", map[string]any{"apk_path": "bad\x00.apk"}); res == nil || !res.IsError {
		t.Fatalf("expected error for null byte in apk_path, got %+v", res)
	}

	// 2. Reject relative path traversal and non-yaml output_path in nano_freeze_test
	if res, _ := reg.Call(ctx, "nano_freeze_test", map[string]any{"output_path": "../../evil.yaml"}); res == nil || !res.IsError {
		t.Fatalf("expected error for relative path traversal in output_path, got %+v", res)
	}
	if res, _ := reg.Call(ctx, "nano_freeze_test", map[string]any{"output_path": "flow.sh"}); res == nil || !res.IsError {
		t.Fatalf("expected error for non-yaml output_path, got %+v", res)
	}

	// 3. Verify bidi/zero-width sanitization
	obs := &ObserveResult{
		ActiveDialog: "Hello\u202EWorld\u200B!",
		Toast:        "\uFEFFToast\u2066Msg",
		VisibleTexts: []string{"Click\u200CMe"},
		AST: &ASTNode{
			ID:                 "n1",
			Text:               "Btn\u202AOk",
			ContentDescription: "Desc\u200DHere",
		},
	}
	sanitizeObserveResult(obs)
	if obs.ActiveDialog != "HelloWorld!" {
		t.Errorf("expected bidi/zwc stripped from ActiveDialog, got %q", obs.ActiveDialog)
	}
	if obs.Toast != "ToastMsg" {
		t.Errorf("expected bidi/zwc stripped from Toast, got %q", obs.Toast)
	}
	if obs.VisibleTexts[0] != "ClickMe" {
		t.Errorf("expected zwc stripped from VisibleTexts, got %q", obs.VisibleTexts[0])
	}
	if obs.AST.Text != "BtnOk" || obs.AST.ContentDescription != "DescHere" {
		t.Errorf("expected bidi/zwc stripped from AST node, got %q / %q", obs.AST.Text, obs.AST.ContentDescription)
	}
}
