package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"nano_android/engine/intelligence/intelhooks"
	mcp "nano_android/engine/mcp_compiler"
	"nano_android/engine/testing/mcphooks"
	"nano_android/internal/phase6hooks"
)

var (
	Version   = "1.3.0-prod"
	BuildDate = "2026-11-26"
	Commit    = "phase3"
)

func main() {
	appFlag := flag.String("app", "", "Target Android application ID (default: com.prism.v)")
	logLevelFlag := flag.String("log-level", "", "Logging level (DEBUG, INFO, NOTICE, WARNING, ERROR)")
	repeatFlag := flag.Int("repeat", 0, "Default repeat_count for Maestro regression flow generation")
	mockFlag := flag.Bool("mock", false, "Use mock coordinator instead of integrated live engines")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	helpFlag := flag.Bool("help", false, "Display usage instructions")

	flag.Parse()

	if *versionFlag {
		fmt.Printf("Nano-Android MCP Server & Unified Phase 3 Engine v%s (%s, commit: %s)\n", Version, BuildDate, Commit)
		fmt.Printf("Protocol: MCP %s | JSON-RPC 2.0\n", mcp.MCPProtocolVersion)
		os.Exit(0)
	}
	if *helpFlag {
		fmt.Printf("Nano-Android MCP Server (with 7-Group + Phase 3 Labs/Differential/Profiler/Media Hooks)\n\n")
		flag.Usage()
		os.Exit(0)
	}

	cfg := mcp.LoadConfigFromEnv()
	if *appFlag != "" {
		cfg.AppID = *appFlag
	}
	if *logLevelFlag != "" {
		cfg.LogLevel = mcp.ParseLogLevel(*logLevelFlag)
	}
	if *repeatFlag > 0 {
		cfg.DefaultRepeatCount = *repeatFlag
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var coord mcp.Coordinator
	if *mockFlag {
		coord = mcp.NewMockCoordinator()
	} else {
		coord = mcp.NewIntegratedCoordinator()
	}
	if closer, ok := coord.(interface{ Close() }); ok {
		defer closer.Close()
	}

	wd, _ := os.Getwd()
	compiler := mcp.NewMaestroCompiler(cfg.AppID)
	reg := mcp.NewToolRegistry(coord, compiler)
	mcphooks.Attach(reg, wd)
	intelhooks.Attach(reg, wd)
	phase6hooks.Attach(reg, wd)

	server := mcp.NewServerWithConfig(reg, cfg, os.Stderr)
	server.Log("INFO", "Initializing Nano-Android Unified MCP Server v%s for %s", Version, cfg.AppID)

	if err := server.Serve(ctx); err != nil && err != context.Canceled {
		server.Log("ERROR", "Server terminated: %v", err)
		os.Exit(1)
	}
	server.Log("INFO", "Server shutdown complete.")
}
