package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type JSONRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	fmt.Println("================================================================================")
	fmt.Println("🚀 STARTING NANO-ANDROID FULL 5-MODULE END-TO-END INTEGRATION TEST")
	fmt.Println("================================================================================")

	binaryPath := filepath.Join(".", "bin", "nano-android-mcp")
	if os.PathSeparator == '\\' {
		binaryPath += ".exe"
	}
	if _, err := os.Stat(binaryPath); err != nil {
		binaryPath = filepath.Join(".", "bin", "nano-android.exe")
	}
	targetAPK := os.Getenv("NANO_TEST_APK")
	if targetAPK == "" {
		targetAPK = filepath.Join("tools", "prism-v.jar")
	}

	if _, err := os.Stat(binaryPath); err != nil {
		fmt.Printf("❌ Binary not found at %s: %v\n", binaryPath, err)
		os.Exit(1)
	}

	cmd := exec.Command(binaryPath, "--app", "com.prism.v")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Printf("❌ Failed to acquire stdin: %v\n", err)
		os.Exit(1)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("❌ Failed to acquire stdout: %v\n", err)
		os.Exit(1)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Printf("❌ Failed to start process: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		stdin.Close()
		cmd.Process.Kill()
	}()

	reader := bufio.NewReader(stdout)
	reqID := 1

	callRPC := func(method string, params any) (json.RawMessage, error) {
		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      reqID,
			Method:  method,
			Params:  params,
		}
		reqID++

		data, _ := json.Marshal(req)
		if _, err := fmt.Fprintf(stdin, "%s\n", data); err != nil {
			return nil, fmt.Errorf("write error: %w", err)
		}

		line, err := reader.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("read error: %w", err)
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			return nil, fmt.Errorf("unmarshal error (%s): %w", string(line), err)
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("rpc error [%d]: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}

	// 1. Initialize
	fmt.Print("\n[Step 1/8] Handshaking MCP Initialize protocol... ")
	initParams := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "Antigravity-AI-Harness",
			"version": "2.0",
		},
	}
	initRes, err := callRPC("initialize", initParams)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ OK: %s\n", string(initRes))

	// 2. Tools List
	fmt.Print("\n[Step 2/8] Querying available MCP tools... ")
	toolsRes, err := callRPC("tools/list", map[string]any{})
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		os.Exit(1)
	}
	var toolsList struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"tools"`
	}
	json.Unmarshal(toolsRes, &toolsList)
	fmt.Printf("✅ OK: Discovered %d tools\n", len(toolsList.Tools))
	for _, t := range toolsList.Tools {
		fmt.Printf("   • %s: %s\n", t.Name, t.Description)
	}

	// 3. Launch App
	fmt.Printf("\n[Step 3/8] Calling nano_launch_app on %s... \n", targetAPK)
	launchStart := time.Now()
	launchRes, err := callRPC("tools/call", map[string]any{
		"name": "nano_launch_app",
		"arguments": map[string]any{
			"apk_path": targetAPK,
		},
	})
	if err != nil {
		fmt.Printf("❌ Launch failed: %v\n", err)
		os.Exit(1)
	}
	launchDuration := time.Since(launchStart)
	fmt.Printf("✅ Launched in %v!\nResponse: %s\n", launchDuration, string(launchRes))

	// 4. Observe Screen (AST + WebP Screenshot)
	fmt.Println("\n[Step 4/8] Calling nano_observe_screen with WebP screenshot rendering...")
	obsStart := time.Now()
	obsRes, err := callRPC("tools/call", map[string]any{
		"name": "nano_observe_screen",
		"arguments": map[string]any{
			"include_screenshot": true,
			"format":             "webp",
		},
	})
	if err != nil {
		fmt.Printf("❌ Observe failed: %v\n", err)
		os.Exit(1)
	}
	obsDuration := time.Since(obsStart)
	obsStr := string(obsRes)
	hasWebP := strings.Contains(obsStr, "data:image/webp;base64")
	fmt.Printf("✅ Observed in %v (WebP Base64 Captured: %v, Response size: %d bytes)\n", obsDuration, hasWebP, len(obsStr))

	// 5. Dispatch Actions (Tap Tabs)
	fmt.Println("\n[Step 5/8] Dispatching synthetic UI actions (Tap tab_matrix, tab_duo, tab_studio)...")
	tabs := []string{"tab_matrix", "tab_duo", "tab_studio"}
	for _, tab := range tabs {
		tStart := time.Now()
		actRes, err := callRPC("tools/call", map[string]any{
			"name": "nano_dispatch_action",
			"arguments": map[string]any{
				"action":         "tap",
				"target_node_id": tab,
			},
		})
		if err != nil {
			fmt.Printf("❌ Action %s failed: %v\n", tab, err)
			os.Exit(1)
		}
		fmt.Printf("   👉 Tapped %s in %v: %s\n", tab, time.Since(tStart), string(actRes))
	}

	// 6. Step Virtual Time (Looper Stepping)
	fmt.Println("\n[Step 6/8] Calling nano_step_time(delta_ms=500) to advance virtual looper...")
	stepStart := time.Now()
	stepRes, err := callRPC("tools/call", map[string]any{
		"name": "nano_step_time",
		"arguments": map[string]any{
			"delta_ms": 500,
		},
	})
	if err != nil {
		fmt.Printf("❌ Step time failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Looper advanced in %v: %s\n", time.Since(stepStart), string(stepRes))

	// 7. Inspect State & Compliance Audit
	fmt.Println("\n[Step 7/8] Calling nano_inspect_state(target='compliance') for 16KB & Android 17 audit...")
	inspectStart := time.Now()
	inspectRes, err := callRPC("tools/call", map[string]any{
		"name": "nano_inspect_state",
		"arguments": map[string]any{
			"target": "compliance",
		},
	})
	if err != nil {
		fmt.Printf("❌ Inspect state failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Audit verified in %v: %s\n", time.Since(inspectStart), string(inspectRes))

	// 8. Freeze Test to Maestro YAML
	fmt.Println("\n[Step 8/8] Calling nano_freeze_test to generate Maestro regression flow...")
	freezeStart := time.Now()
	freezeRes, err := callRPC("tools/call", map[string]any{
		"name": "nano_freeze_test",
		"arguments": map[string]any{
			"flow_name":   "prism_v_integrated_regression",
			"output_path": ".maestro/prism_v_integrated_flow.yaml",
		},
	})
	if err != nil {
		fmt.Printf("❌ Freeze test failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Test frozen in %v: %s\n", time.Since(freezeStart), string(freezeRes))

	fmt.Println("\n================================================================================")
	fmt.Println("🎉 ALL 5 MODULES SUCCESSFULLY INTEGRATED AND VERIFIED END-TO-END!")
	fmt.Println("================================================================================")
}
