package mcp

import (
	"os"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/build"
	"github.com/anthropic/droidforge-mcp/internal/compiler"
	"github.com/anthropic/droidforge-mcp/internal/device"
	"github.com/anthropic/droidforge-mcp/internal/harness"
	"github.com/anthropic/droidforge-mcp/internal/policy"
	"github.com/anthropic/droidforge-mcp/internal/profiler"
	"github.com/anthropic/droidforge-mcp/internal/types"
	"github.com/anthropic/droidforge-mcp/internal/uitree"
	"github.com/anthropic/droidforge-mcp/internal/vision"
)

// WiringOptions configures engine behavior during server startup.
type WiringOptions struct {
	AllowPhysical bool
	Verbose       bool
}

// WireProductionEngines creates and registers all 26 real tool handlers
// using the initialized subsystem engines.
func WireProductionEngines(
	srv *Server,
	cfg *types.DroidForgeConfig,
	pol *types.PolicyConfig,
	guard types.PolicyGuard,
	logger types.AuditLogger,
) {
	WireProductionEnginesWithOptions(srv, cfg, pol, guard, logger, WiringOptions{})
}

// WireProductionEnginesWithOptions creates and registers all 26 real tool handlers
// with explicit options.
func WireProductionEnginesWithOptions(
	srv *Server,
	cfg *types.DroidForgeConfig,
	pol *types.PolicyConfig,
	guard types.PolicyGuard,
	logger types.AuditLogger,
	opts WiringOptions,
) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if pol == nil {
		pol = DefaultPolicy()
	}

	emulatorOnly := !opts.AllowPhysical

	// 1. Device Manager
	deviceOpts := []device.ManagerOption{
		device.WithEmulatorOnly(emulatorOnly),
		device.WithAllowlistedPackages(cfg.Project.AllowlistedPackages),
	}
	deviceMgr := device.NewDeviceManager(deviceOpts...)

	// 2. Vision Backend
	visionConfig := vision.ManagerConfig{
		ADBRunner:        deviceMgr.ExecADB,
		IdleTimeout:      10 * time.Minute,
		CheckInterval:    30 * time.Second,
		DefaultMaxHeight: cfg.Observation.ScreenshotMaxHeight,
		DefaultQuality:   cfg.Observation.ScreenshotQuality,
	}
	visionBackend := vision.NewManager(visionConfig)

	// 3. UI Tree Service & Tools (#9, #10, #11, #12, #13)
	uiTreeService := uitree.NewService(deviceMgr, visionBackend, guard, &cfg.Observation)
	uitree.RegisterUITreeTools(srv, uiTreeService)

	// 4. Build Engine & Tools (#1, #2, #3, #4)
	buildEngine := build.NewGradleEngine(cfg.Project.GradleRoot, cfg.Execution.GradleMaxHeapMB)
	build.RegisterBuildToolsOnServer(srv, buildEngine)

	// 5. Test Compiler & Maestro Runner Tools (#23, #24, #25)
	compilerInstance := compiler.NewCompiler(nil)
	if len(cfg.Project.AllowlistedPackages) > 0 {
		compilerInstance.SetDefaultAppID(cfg.Project.AllowlistedPackages[0])
	}
	compiler.RegisterCompilerTools(srv, compilerInstance)
	uiTreeService.SetTestCompiler(compilerInstance, srv.SessionID())

	// 6. Profiler & Harness Python Sidecars & Tools (#14, #15, #16, #17, #18, #19, #20, #21, #22)
	profilerSidecar := profiler.NewPythonSidecarRunner("")
	harnessSidecar := harness.NewPythonSidecarRunner("")

	profiler.RegisterProfilerToolsOnServer(srv, deviceMgr, profilerSidecar)
	harness.RegisterHarnessToolsOnServer(srv, deviceMgr, harnessSidecar)

	// 7. Device Lifecycle Tools (#5, #6, #7, #8)
	srv.Register(device.NewDeviceListTool(deviceMgr))
	srv.Register(device.NewEmulatorBootSnapshotTool(deviceMgr))
	srv.Register(device.NewDeviceResetStateTool(deviceMgr))
	srv.Register(device.NewAppInstallAndLaunchTool(deviceMgr))

	// 8. Governance & Approval Tool (#26)
	srv.Register(policy.NewApprovalHandler(emulatorOnly, os.Stdin))

	// 9. Guarantee 100% tool completeness via canonical stubs fallback
	RegisterAllStubs(srv)
}
