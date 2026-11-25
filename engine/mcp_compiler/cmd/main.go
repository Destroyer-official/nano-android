package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	mcp "nano_android/engine/mcp_compiler"
)

const (
	Version   = "1.2.0-prod"
	BuildDate = "2026-11-24"
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
		fmt.Printf("Nano-Android MCP Server & Maestro Compiler v%s (%s)\n", Version, BuildDate)
		fmt.Printf("Protocol: MCP %s | JSON-RPC 2.0\n", mcp.MCPProtocolVersion)
		os.Exit(0)
	}

	if *helpFlag {
		fmt.Printf("Nano-Android MCP Bridge Server & Maestro Test Compiler\n\n")
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
	compiler := mcp.NewMaestroCompiler(cfg.AppID)
	reg := mcp.NewToolRegistry(coord, compiler)
	server := mcp.NewServerWithConfig(reg, cfg, os.Stderr)

	server.Log("INFO", "Initializing Nano-Android MCP Bridge v%s for %s", Version, cfg.AppID)

	if err := server.Serve(ctx); err != nil && err != context.Canceled {
		server.Log("ERROR", "Server terminated: %v", err)
		os.Exit(1)
	}
	server.Log("INFO", "Server shutdown complete.")
}
