// Command droidforge-mcp is the single-binary MCP server for AI-driven
// Android build, test, and observation loops. It is designed to be launched
// by AI IDEs (Antigravity, VS Code, Cursor, Claude Code) over stdio.
//
// Subcommands:
//
//	serve   — Start the MCP server on stdio (default)
//	doctor  — Check host environment (ADB, emulator, scrcpy, Gradle, disk space)
//	init    — Scaffold droidforge.yaml + .mcp.json + AGENTS.md into a project
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/anthropic/droidforge-mcp/internal/audit"
	"github.com/anthropic/droidforge-mcp/internal/mcp"
	"github.com/anthropic/droidforge-mcp/internal/policy"
	"github.com/anthropic/droidforge-mcp/internal/types"
)

var (
	version = "0.1.0"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		printUsage()
		return nil
	}
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version" || args[0] == "-v") {
		fmt.Printf("droidforge-mcp v%s (MCP protocol: %s)\n", version, mcp.ProtocolVersion)
		return nil
	}

	subcommand := "serve"
	subArgs := args

	if len(args) > 0 && !isFlag(args[0]) {
		subcommand = args[0]
		subArgs = args[1:]
	}

	switch subcommand {
	case "serve":
		return runServe(subArgs)
	case "doctor":
		return runDoctor(subArgs)
	case "init":
		return runInit(subArgs)
	default:
		return fmt.Errorf("unknown command %q (use 'serve', 'doctor', or 'init')", subcommand)
	}
}

func isFlag(arg string) bool {
	return len(arg) > 0 && arg[0] == '-'
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := fs.String("config", "droidforge.yaml", "path to droidforge.yaml configuration")
	policyPath := fs.String("policy", "droidforge.policy.json", "path to droidforge.policy.json security policy")
	allowPhysical := fs.Bool("allow-physical", false, "allow connection to physical USB/Wi-Fi devices (disables emulator-only safety check)")
	verbose := fs.Bool("verbose", false, "enable verbose logging to stderr")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	// 1. Load config (fallback to default if file doesn't exist)
	var cfg *types.DroidForgeConfig
	var err error
	if _, statErr := os.Stat(*configPath); statErr == nil {
		cfg, err = mcp.LoadConfig(*configPath)
		if err != nil {
			return fmt.Errorf("config error: %w", err)
		}
	} else {
		cfg = mcp.DefaultConfig()
	}

	// 2. Load policy (fallback to default if file doesn't exist)
	var pol *types.PolicyConfig
	if _, statErr := os.Stat(*policyPath); statErr == nil {
		pol, err = mcp.LoadPolicy(*policyPath)
		if err != nil {
			return fmt.Errorf("policy error: %w", err)
		}
	} else {
		pol = mcp.DefaultPolicy()
	}

	// 3. Construct Security Guard & Forensic Audit Logger
	guard := policy.NewGuard(".", cfg.Project, *pol, cfg.Execution)
	if *allowPhysical {
		guard.SetEmulatorOnly(false)
	}
	logger, _ := audit.NewJSONLLogger(pol.Audit.LogDir, pol.Audit.RetentionDays)

	// 4. Construct MCP server
	srv := mcp.NewServer(cfg, pol, guard, logger)

	// 5. Wire all 26 production engines
	wiringOpts := mcp.WiringOptions{
		AllowPhysical: *allowPhysical,
		Verbose:       *verbose,
	}
	mcp.WireProductionEnginesWithOptions(srv, cfg, pol, guard, logger, wiringOpts)

	// 6. Setup context with OS signal cancellation
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintf(os.Stderr, "[droidforge-mcp] starting stdio server (session: %s, tools: %d, allow_physical: %t)\n",
		srv.SessionID(), len(srv.ListHandlers()), *allowPhysical)

	return srv.Serve(ctx)
}

func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	configPath := fs.String("config", "droidforge.yaml", "path to droidforge.yaml configuration")
	jsonOutput := fs.Bool("json", false, "output doctor results as JSON")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	var cfg *types.DroidForgeConfig
	if _, statErr := os.Stat(*configPath); statErr == nil {
		cfg, _ = mcp.LoadConfig(*configPath)
	}

	ctx := context.Background()
	results := mcp.RunDoctor(ctx, cfg)

	if *jsonOutput {
		return mcp.PrintDoctorJSON(os.Stdout, results)
	}

	passed := mcp.PrintDoctorReport(os.Stdout, results)
	if !passed {
		return fmt.Errorf("doctor check found failing dependencies")
	}
	return nil
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	projectNameFlag := fs.String("name", "", "project name override")

	// Separate flags and positional arguments so flags work before or after positional args
	var flagsOnly, posArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagsOnly = append(flagsOnly, arg)
			if !strings.Contains(arg, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flagsOnly = append(flagsOnly, args[i])
			}
		} else {
			posArgs = append(posArgs, arg)
		}
	}

	if err := fs.Parse(flagsOnly); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	targetDir := "."
	if len(posArgs) > 0 {
		targetDir = posArgs[0]
	}

	projectName := *projectNameFlag
	if projectName == "" && len(posArgs) > 1 {
		projectName = posArgs[1]
	}

	if err := mcp.ScaffoldProject(targetDir, projectName); err != nil {
		return fmt.Errorf("scaffold failed: %w", err)
	}

	fmt.Printf("[droidforge-mcp] Successfully scaffolded target %q:\n", targetDir)
	fmt.Println("  + droidforge.yaml")
	fmt.Println("  + droidforge.policy.json")
	fmt.Println("  + .mcp.json")
	fmt.Println("  + AGENTS.md")
	fmt.Println("  + .droidforge/ (appcard.md, flows/, baselines/)")
	return nil
}

func printUsage() {
	fmt.Println(`droidforge-mcp: AI-driven Android build, test, and observation MCP server

Usage:
  droidforge-mcp [command] [flags]

Commands:
  serve     Start MCP stdio transport loop (default)
  doctor    Run environment checks (ADB, emulator, scrcpy, Gradle, disk space)
  init      Scaffold droidforge.yaml, .mcp.json, and AGENTS.md into target directory
  version   Print version information
  help      Show help for droidforge-mcp

Flags for serve:
  --config          Path to droidforge.yaml (default: droidforge.yaml)
  --policy          Path to droidforge.policy.json (default: droidforge.policy.json)
  --allow-physical  Allow physical device connection (disables emulator-only safety check)
  --verbose         Enable verbose stderr logging

Flags for doctor:
  --config          Path to droidforge.yaml (default: droidforge.yaml)
  --json            Output doctor check results as JSON

Flags for init:
  --name            Project name override (default: basename of target directory)

Examples:
  droidforge-mcp serve --config ./droidforge.yaml
  droidforge-mcp doctor
  droidforge-mcp doctor --json
  droidforge-mcp init ./myapp --name my-cool-app`)
}
