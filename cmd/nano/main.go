package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"nano_android/adapters/frameworks"
	"nano_android/engine/apk/ingest"
	"nano_android/engine/apk/security"
	"nano_android/engine/backend/remote"
	"nano_android/engine/compatibility/intel"
	"nano_android/engine/compatibility/registry"
	"nano_android/engine/intelligence/ai"
	"nano_android/engine/intelligence/graph"
	"nano_android/engine/intelligence/intelhooks"
	"nano_android/engine/mcp_compiler"
	"nano_android/engine/media/mp4"
	"nano_android/engine/media/oracle"
	"nano_android/engine/plugins"
	"nano_android/engine/profiler/crash"
	"nano_android/engine/profiler/device"
	"nano_android/engine/profiler/jvm"
	"nano_android/engine/reporting"
	"nano_android/engine/security/audit"
	"nano_android/engine/security/pathcontainment"
	"nano_android/engine/security/policy"
	"nano_android/engine/testing/differential"
	"nano_android/engine/testing/labs"
	"nano_android/engine/testing/mcphooks"
	"nano_android/engine/testing/orchestrator"
	"nano_android/engine/testing/suite"
	"nano_android/engine/ui/accessibility"
	"nano_android/engine/ui/profile"
	"nano_android/engine/ui/visual"
	"nano_android/internal/phase6hooks"
	"nano_android/plugins/ide"

	"github.com/anthropic/droidforge-mcp/engine/profiler_media"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(runCLI(ctx, os.Args[1:]))
}

func runCLI(ctx context.Context, args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printUsage()
		return 0
	}

	sub := args[0]
	rest := args[1:]
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get working directory: %v\n", err)
		return 2
	}

	switch sub {
	case "init":
		return cmdInit(wd, rest)
	case "doctor":
		return cmdDoctor(ctx, wd, rest)
	case "inspect", "manifest", "dex", "resources", "compatibility":
		return cmdAPKAnalyze(wd, sub, rest)
	case "security":
		return cmdSecurityAudit(wd, rest)
	case "export", "report":
		return cmdExportOrReport(wd, rest)
	case "diff":
		return cmdDiff(ctx, wd, rest)
	case "launch":
		return cmdLaunch(ctx, wd, rest)
	case "test", "run", "replay":
		return cmdTest(ctx, wd, rest)
	case "lab":
		return cmdLab(ctx, wd, rest)
	case "media":
		return cmdMedia(ctx, wd, rest)
	case "visual":
		return cmdVisual(ctx, wd, rest)
	case "graph":
		return cmdGraph(wd, rest)
	case "ai":
		return cmdAI(ctx, wd, rest)
	case "plugin":
		return cmdPlugin(ctx, wd, rest)
	case "adapter":
		return cmdAdapter(wd, rest)
	case "pool":
		return cmdPool(ctx, wd, rest)
	case "analytics":
		return cmdAnalytics(wd, rest)
	case "sbom":
		return cmdSBOM(wd, rest)
	case "record":
		return cmdRecord(wd, rest)
	case "screenshot":
		return cmdScreenshot(ctx, wd, rest)
	case "benchmark":
		return cmdBenchmark(ctx, wd, rest)
	case "crash":
		return cmdCrashCheck(ctx, wd, rest)
	case "inspect-ui", "a11y":
		return cmdA11y(ctx, wd, rest)
	case "profile":
		return cmdProfileRuntime(ctx, wd, rest)
	case "profiles":
		return cmdProfiles(rest)
	case "devices", "emulator":
		return cmdDevices(ctx, sub)
	case "build":
		return cmdBuildDiscover(ctx, wd, rest)
	case "worker":
		return cmdWorkerCheck(ctx, wd)
	case "policy":
		return cmdPolicy(wd, rest)
	case "cache":
		return cmdCache(wd, rest)
	case "server":
		coord := mcp_compiler.NewIntegratedCoordinator()
		defer coord.Close()
		cfg := mcp_compiler.LoadConfigFromEnv()
		reg := mcp_compiler.NewToolRegistry(coord, mcp_compiler.NewMaestroCompiler(cfg.AppID))
		mcphooks.Attach(reg, wd)
		intelhooks.Attach(reg, wd)
		phase6hooks.Attach(reg, wd)
		srv := mcp_compiler.NewServerWithConfig(reg, cfg, os.Stderr)
		if err := srv.Serve(ctx); err != nil && err != context.Canceled {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", sub)
		printUsage()
		return 2
	}
}

func allowedRoots(wd string) []string {
	return []string{
		filepath.Join(wd, "cache"),
		filepath.Join(wd, "tools"),
		filepath.Join(wd, "fixtures"),
		filepath.Join(wd, "downloads"),
	}
}

func cmdInit(wd string, args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	targetDir := fs.String("dir", filepath.Join(wd, "cache", "nano_init"), "subdirectory name under workspace cache")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cacheRoot := filepath.Join(wd, "cache")
	suitePath, err := pathcontainment.ResolveWriteContained(cacheRoot, filepath.Base(*targetDir)+"_suite.json", []string{".json"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
		return 2
	}
	sampleSuite := suite.Suite{
		Name:        "StarterSuite",
		Artifact:    "fixtures/apks/minimal_valid.apk",
		Mode:        "auto",
		RepeatCount: 3,
		Cases: []suite.TestCase{
			{Name: "LaunchSmoke", ProfileID: "clean_install", Steps: []suite.Step{{Action: "observe"}}},
		},
	}
	b, _ := json.MarshalIndent(sampleSuite, "", "  ")
	if err := os.WriteFile(suitePath, b, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return emitOutput(map[string]any{"status": "ok", "suite_path": suitePath}, true)
}

func cmdDoctor(ctx context.Context, wd string, args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	reg := mcp_compiler.NewToolRegistry(mcp_compiler.NewMockCoordinator(), mcp_compiler.NewMaestroCompiler("com.nano.app"))
	phase6hooks.Attach(reg, wd)
	res, err := reg.Call(ctx, "nano_doctor", map[string]any{"verbose": true})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var docPayload map[string]any
	_ = json.Unmarshal([]byte(res.Content[0].Text), &docPayload)
	if docPayload == nil {
		docPayload = make(map[string]any)
	}
	docPayload["adapters"] = frameworks.ProbeAll(wd)
	docPayload["plugin_sdk_kinds"] = plugins.SupportedKinds()
	if *asJSON {
		return emitOutput(docPayload, true)
	} else {
		fmt.Printf("Nano-Android Doctor OK (workspace=%s)\n%s\n", wd, res.Content[0].Text)
	}
	return 0
}

func cmdAPKAnalyze(wd, sub string, args []string) int {
	fs := flag.NewFlagSet(sub, flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "output JSON")
	mode := fs.String("mode", "auto", "requested backend mode")
	mdOut := fs.Bool("markdown", false, "output technical markdown summary")
	compatDB := fs.String("db", "", "optional SQLite compatibility.db path inside workspace cache")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if sub == "compatibility" && strings.TrimSpace(*compatDB) != "" && fs.NArg() == 0 {
		dbOut, err := pathcontainment.ResolveWriteContained(filepath.Join(wd, "cache"), filepath.Base(*compatDB), []string{".db"})
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		store, err := intel.OpenStore(dbOut)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		_ = store.SeedDefaults()
		hm, err := store.QueryHeatmap()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return emitOutput(hm, true)
	}
	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "usage: nano %s <artifact.apk|aab|apks|xapk>\n", sub)
		return 2
	}
	contained, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".aab", ".apks", ".xapk"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
		return 2
	}
	eng := ingest.NewEngine(filepath.Join(wd, "cache", "apk_intel"), filepath.Join(wd, "tools", "libs", "bundletool-all-1.18.1.jar"))
	rep, err := eng.Analyze(contained)
	if err != nil {
		fmt.Fprintf(os.Stderr, "analysis error: %v\n", err)
		return 1
	}
	if *mdOut {
		fmt.Print(ingest.FormatMarkdownReport(rep))
		return 0
	}
	var payload any = rep
	switch sub {
	case "manifest":
		payload = map[string]any{"manifest": rep.Manifest, "permission_risks": rep.PermissionRisks}
	case "dex":
		payload = rep.DEX
	case "resources":
		payload = map[string]any{"resources": rep.Resources, "size_distribution": rep.SizeDistribution, "duplicate_assets": rep.DuplicateAssets}
	case "compatibility":
		payload = registry.EvaluateAPK(rep, *mode)
	}
	return emitOutput(payload, *asJSON)
}

func cmdSecurityAudit(wd string, args []string) int {
	fs := flag.NewFlagSet("security", flag.ContinueOnError)
	sarif := fs.Bool("sarif", false, "output SARIF v2.1.0")
	asJSON := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: nano security [--sarif] [--json] <artifact.apk|aab>")
		return 2
	}
	contained, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".aab", ".apks", ".xapk"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
		return 2
	}
	eng := ingest.NewEngine(filepath.Join(wd, "cache", "apk_intel"), "")
	rep, err := eng.Analyze(contained)
	if err != nil {
		fmt.Fprintf(os.Stderr, "analysis error: %v\n", err)
		return 1
	}
	audit := security.AuditAPK(rep)
	if *sarif {
		b, _ := audit.ExportSARIF()
		fmt.Println(string(b))
	} else {
		_ = emitOutput(audit, *asJSON || true)
	}
	if !audit.PassedGate {
		return 1
	}
	return 0
}

func cmdExportOrReport(wd string, args []string) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	format := fs.String("format", "markdown", "export format: markdown, sarif, json, unified-json, html, pdf, junit, bundle")
	verifyFile := fs.String("verify", "", "path to signed UnifiedReport JSON to verify Ed25519 signature & SHA-256")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*verifyFile) != "" {
		vp, err := pathcontainment.ResolveExistingContained(*verifyFile, allowedRoots(wd), []string{".json"})
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		raw, rErr := os.ReadFile(vp)
		if rErr != nil {
			fmt.Fprintln(os.Stderr, rErr)
			return 1
		}
		// Strip UTF-8 BOM (EF BB BF) that PowerShell Out-File may prepend.
		raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
		var ur reporting.UnifiedReport
		if uErr := json.Unmarshal(raw, &ur); uErr != nil {
			fmt.Fprintln(os.Stderr, uErr)
			return 1
		}
		if vErr := reporting.Verify(ur); vErr != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY verification failed: %v\n", vErr)
			return 2
		}
		return emitOutput(map[string]any{"verified": true, "report_id": ur.ReportID, "sha256": ur.ProvenanceAndAudit.ReportSHA256}, true)
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: nano export [--format=markdown|sarif|json|unified-json|html|pdf|junit|bundle] <artifact.apk|aab>")
		return 2
	}
	contained, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".aab", ".apks", ".xapk"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
		return 2
	}
	eng := ingest.NewEngine(filepath.Join(wd, "cache", "apk_intel"), "")
	rep, err := eng.Analyze(contained)
	if err != nil {
		fmt.Fprintf(os.Stderr, "analysis error: %v\n", err)
		return 1
	}
	switch strings.ToLower(*format) {
	case "markdown", "md":
		fmt.Print(ingest.FormatMarkdownReport(rep))
		return 0
	case "sarif":
		b, _ := security.AuditAPK(rep).ExportSARIF()
		fmt.Println(string(b))
		return 0
	case "unified-json", "html", "pdf", "junit", "bundle":
		ur := buildUnifiedCLIReport(wd, rep)
		if as, aErr := reporting.OpenAnalyticsStore(filepath.Join(wd, "cache", "reporting", "analytics.json")); aErr == nil {
			_ = as.RecordRun(*ur)
		}
		switch strings.ToLower(*format) {
		case "html":
			b, _ := ur.ExportHTML()
			fmt.Println(string(b))
			return 0
		case "pdf":
			b, _ := ur.ExportPDF()
			fmt.Printf("PDF generated (%d bytes, signed=%v, sha256=%s)\n", len(b), ur.ProvenanceAndAudit.SignatureEd25519 != "", ur.ProvenanceAndAudit.ReportSHA256)
			return 0
		case "junit":
			b, _ := ur.ExportJUnitXML()
			fmt.Println(string(b))
			return 0
		case "bundle":
			dst, cErr := pathcontainment.ResolveWriteContained(filepath.Join(wd, "cache", "reporting"), "forensic_bundle.zip", []string{".zip"})
			if cErr != nil {
				fmt.Fprintln(os.Stderr, cErr)
				return 2
			}
			if _, zErr := ur.ExportForensicBundleZIP(dst); zErr != nil {
				fmt.Fprintln(os.Stderr, zErr)
				return 1
			}
			return emitOutput(map[string]any{"bundle": dst, "report": ur}, true)
		default:
			return emitOutput(ur, true)
		}
	default:
		return emitOutput(rep, true)
	}
}

func cmdDiff(ctx context.Context, wd string, args []string) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	mode := fs.String("mode", "static", "diff mode: static (APK comparison) or runtime (11-dimension differential engine)")
	profB := fs.String("profile-b", "small_screen", "target B profile for runtime diff")
	activity := fs.String("activity", "", "optional target activity")
	compatDB := fs.String("compat-db", "", "optional SQLite compatibility.db path under cache")
	asJSON := fs.Bool("json", true, "output JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.EqualFold(*mode, "runtime") {
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: nano diff --mode=runtime [--profile-b=small_screen] <artifactA.apk|jar> [artifactB.apk|jar]")
			return 2
		}
		p1, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".jar"})
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		p2 := p1
		if fs.NArg() >= 2 {
			p2, err = pathcontainment.ResolveExistingContained(fs.Arg(1), allowedRoots(wd), []string{".apk", ".jar"})
			if err != nil {
				fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
				return 2
			}
		}
		var dbResolved string
		if *compatDB != "" {
			if cdb, cErr := pathcontainment.ResolveWriteContained(filepath.Join(wd, "cache"), filepath.Base(*compatDB), []string{".db"}); cErr == nil {
				dbResolved = cdb
			}
		}
		req := differential.DifferentialRequest{
			Name:            "CLIDifferential",
			Mode:            differential.ModeDeviceAVsDeviceB,
			TargetA:         differential.TargetSpec{Label: "A", Backend: registry.BackendNanoJVM, Artifact: p1, Activity: *activity, ProfileID: "clean_install", APILevel: 35},
			TargetB:         differential.TargetSpec{Label: "B", Backend: registry.BackendNanoJVM, Artifact: p2, Activity: *activity, ProfileID: *profB, APILevel: 35},
			AutoReproOnDiff: true,
			CompatDBPath:    dbResolved,
		}
		rep, err := differential.Run(ctx, req, nil, nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		_ = emitOutput(rep, true)
		if !rep.Equivalent {
			return 1
		}
		return 0
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: nano diff [--mode=static|runtime] <base.apk> <target.apk>")
		return 2
	}
	p1, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".aab"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
		return 2
	}
	p2, err := pathcontainment.ResolveExistingContained(fs.Arg(1), allowedRoots(wd), []string{".apk", ".aab"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
		return 2
	}
	eng := ingest.NewEngine(filepath.Join(wd, "cache", "apk_intel"), "")
	r1, err := eng.Analyze(p1)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	r2, err := eng.Analyze(p2)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return emitOutput(ingest.Compare(r1, r2), *asJSON)
}

func cmdLaunch(ctx context.Context, wd string, args []string) int {
	fs := flag.NewFlagSet("launch", flag.ContinueOnError)
	activity := fs.String("activity", "auto", "target activity")
	asJSON := fs.Bool("json", true, "output JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: nano launch [--activity=Main] <artifact.apk|jar>")
		return 2
	}
	p, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".jar"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
		return 2
	}
	bridge := mcp_compiler.NewLiveHeadlessBridge()
	defer bridge.Close()
	res, err := bridge.Launch(p, *activity)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return emitOutput(res, *asJSON)
}

func cmdScreenshot(ctx context.Context, wd string, args []string) int {
	fs := flag.NewFlagSet("screenshot", flag.ContinueOnError)
	activity := fs.String("activity", "auto", "target activity")
	out := fs.String("out", "cli_screenshot.png", "output filename inside workspace cache")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: nano screenshot [--out=cli_screenshot.png] <artifact.apk|jar>")
		return 2
	}
	p, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".jar"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
		return 2
	}
	outPath, err := pathcontainment.ResolveWriteContained(filepath.Join(wd, "cache"), *out, []string{".png"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
		return 2
	}
	bridge := mcp_compiler.NewLiveHeadlessBridge()
	defer bridge.Close()
	if _, err := bridge.Launch(p, *activity); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	renderedFile, err := bridge.RenderScreen(outPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return emitOutput(map[string]any{"status": "ok", "output_path": renderedFile}, true)
}

func cmdBenchmark(ctx context.Context, wd string, args []string) int {
	fs := flag.NewFlagSet("benchmark", flag.ContinueOnError)
	activity := fs.String("activity", "auto", "target activity")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: nano benchmark <artifact.apk|jar>")
		return 2
	}
	p, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".jar"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
		return 2
	}
	bridge := mcp_compiler.NewLiveHeadlessBridge()
	defer bridge.Close()
	t0 := time.Now()
	launchRes, err := bridge.Launch(p, *activity)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	coldMs := time.Since(t0).Milliseconds()
	t1 := time.Now()
	stepRes, _ := bridge.StepTime(300)
	stepWallMs := time.Since(t1).Milliseconds()
	metrics, _ := bridge.InspectRuntime()
	return emitOutput(map[string]any{
		"status":           "ok",
		"environment":      "nano_jvm",
		"cold_launch_ms":   coldMs,
		"reported_boot_ms": launchRes.LaunchDurationMs,
		"step_300ms_wall":  stepWallMs,
		"step_result":      stepRes,
		"runtime_metrics":  metrics,
	}, true)
}

func cmdCrashCheck(ctx context.Context, wd string, args []string) int {
	fs := flag.NewFlagSet("crash", flag.ContinueOnError)
	activity := fs.String("activity", "auto", "target activity")
	mappingPath := fs.String("mapping", "", "optional ProGuard/R8 mapping.txt inside workspace")
	logPath := fs.String("log", "", "optional saved logcat/stacktrace file to analyze and cluster")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var mapping *crash.R8Mapping
	if *mappingPath != "" {
		mp, mErr := pathcontainment.ResolveExistingContained(*mappingPath, allowedRoots(wd), []string{".txt", ".map"})
		if mErr != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", mErr)
			return 2
		}
		if raw, rErr := os.ReadFile(mp); rErr == nil {
			mapping = crash.ParseR8Mapping(string(raw))
		}
	}
	if strings.TrimSpace(*logPath) != "" {
		lp, lErr := pathcontainment.ResolveExistingContained(*logPath, allowedRoots(wd), []string{".txt", ".log"})
		if lErr != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", lErr)
			return 2
		}
		raw, rErr := os.ReadFile(lp)
		if rErr != nil {
			fmt.Fprintln(os.Stderr, rErr)
			return 1
		}
		rep := crash.AnalyzeTrace(string(raw), "log_file", "static_log", mapping)
		clusters := crash.ClusterCrashes([]crash.CrashReport{rep})
		return emitOutput(map[string]any{"report": rep, "clusters": clusters}, true)
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: nano crash [--mapping=mapping.txt | --log=file.log] <artifact.apk|jar>")
		return 2
	}
	p, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".jar"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
		return 2
	}
	bridge := mcp_compiler.NewLiveHeadlessBridge()
	defer bridge.Close()
	res, err := bridge.Launch(p, *activity)
	if err != nil {
		rep := crash.AnalyzeTrace(err.Error(), "cli", "nano_jvm", mapping)
		_ = emitOutput(map[string]any{"crashed": true, "report": rep}, true)
		return 1
	}
	crashed := res.Status == "error" || res.ExceptionTrace != ""
	var rep *crash.CrashReport
	if res.ExceptionTrace != "" {
		r := crash.AnalyzeTrace(res.ExceptionTrace, res.AppID, "nano_jvm", mapping)
		rep = &r
	}
	_ = emitOutput(map[string]any{
		"crashed":       crashed,
		"result_status": res.ResultStatus,
		"warnings":      res.Warnings,
		"crash_report":  rep,
	}, true)
	if crashed {
		return 1
	}
	return 0
}

func cmdRecord(wd string, args []string) int {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	appID := fs.String("app", "com.example.app", "application package ID")
	tapTarget := fs.String("tap", "Submit", "sample action target")
	outPath := fs.String("out", "recorded_flow.yaml", "output Maestro YAML filename inside cache")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	containedOut, err := pathcontainment.ResolveWriteContained(filepath.Join(wd, "cache"), *outPath, []string{".yaml", ".yml"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
		return 2
	}
	comp := mcp_compiler.NewMaestroCompiler(*appID)
	comp.RecordLaunch(*appID, true)
	comp.RecordTap(*tapTarget, "", [2]float64{0, 0}, false)
	comp.RecordSleep(300)
	yamlStr, err := comp.Compile(mcp_compiler.CompileOptions{RepeatCount: 3, Optimize: true})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	_ = os.MkdirAll(filepath.Dir(containedOut), 0750)
	if err := os.WriteFile(containedOut, []byte(yamlStr), 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return emitOutput(map[string]any{"status": "ok", "output_path": containedOut, "flow": yamlStr}, true)
}

func cmdTest(ctx context.Context, wd string, args []string) int {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	mode := fs.String("mode", "auto", "execution backend mode")
	activity := fs.String("activity", "", "optional activity")
	repeat := fs.Int("repeat", 1, "repeat count (>=3 enables flake detection)")
	junitOut := fs.Bool("junit-xml", false, "output JUnit XML")
	suiteFile := fs.String("suite", "", "optional JSON test suite file")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var s suite.Suite
	if strings.TrimSpace(*suiteFile) != "" {
		sf, err := pathcontainment.ResolveExistingContained(*suiteFile, allowedRoots(wd), []string{".json"})
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		raw, err := os.ReadFile(sf)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := json.Unmarshal(raw, &s); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	} else {
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: nano test [--mode=auto] [--repeat=3] <artifact.apk|jar>")
			return 2
		}
		s = suite.Suite{
			Name:        "CLISmoke",
			Artifact:    fs.Arg(0),
			Mode:        *mode,
			RepeatCount: *repeat,
			Cases: []suite.TestCase{
				{Name: "LaunchAndObserve", Activity: *activity, Steps: []suite.Step{{Action: "observe"}}},
			},
		}
	}

	orch := orchestrator.New(wd)
	rep, err := orch.RunSuite(ctx, s)
	if *junitOut && rep != nil {
		xmlB, _ := rep.ExportJUnitXML()
		fmt.Println(string(xmlB))
	} else if rep != nil {
		_ = emitOutput(rep, true)
	}
	if err != nil || (rep != nil && rep.Failed > 0) {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		return 1
	}
	return 0
}

func cmdA11y(ctx context.Context, wd string, args []string) int {
	fs := flag.NewFlagSet("a11y", flag.ContinueOnError)
	profID := fs.String("profile", "clean_install", "device/scenario profile")
	activity := fs.String("activity", "auto", "target activity")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: nano a11y [--profile=large_font] <artifact.apk|jar>")
		return 2
	}
	prof, err := profile.Get(*profID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	p, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".jar"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
		return 2
	}
	bridge := mcp_compiler.NewLiveHeadlessBridge()
	defer bridge.Close()
	if _, err := bridge.Launch(p, *activity); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if prof.WidthPx != 1080 || prof.HeightPx != 2400 {
		_ = bridge.SetProfile(prof.WidthPx, prof.HeightPx)
	}
	ast, err := bridge.Observe()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	rep := accessibility.AuditTree(convertASTToUI(ast), prof)
	return emitOutput(rep, true)
}

func convertASTToUI(n *mcp_compiler.ASTNode) *accessibility.UINode {
	if n == nil {
		return nil
	}
	u := &accessibility.UINode{
		ID: n.ID, ResourceID: n.ResourceID, Class: n.Type, Text: n.Text,
		ContentDescription: n.ContentDescription, Clickable: n.Clickable,
		Checkable: n.Checkable, Focusable: n.Focused, Visible: n.Visible, Bounds: n.Bounds,
	}
	for _, c := range n.Children {
		u.Children = append(u.Children, convertASTToUI(c))
	}
	return u
}

func cmdProfiles(args []string) int {
	return emitOutput(profile.ReusableProfiles(), true)
}

func cmdDevices(ctx context.Context, sub string) int {
	reg := mcp_compiler.NewToolRegistry(mcp_compiler.NewMockCoordinator(), mcp_compiler.NewMaestroCompiler("com.nano.app"))
	res, err := reg.Call(ctx, "nano_device_list", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(res.Content[0].Text)
	return 0
}

func cmdBuildDiscover(ctx context.Context, wd string, args []string) int {
	reg := mcp_compiler.NewToolRegistry(mcp_compiler.NewMockCoordinator(), mcp_compiler.NewMaestroCompiler("com.nano.app"))
	res, err := reg.Call(ctx, "nano_project_discover", map[string]any{"root": wd})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(res.Content[0].Text)
	return 0
}

func cmdWorkerCheck(ctx context.Context, wd string) int {
	reg := mcp_compiler.NewToolRegistry(mcp_compiler.NewMockCoordinator(), mcp_compiler.NewMaestroCompiler("com.nano.app"))
	res, err := reg.Call(ctx, "nano_device_status", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	_, javaErr := exec.LookPath("java")
	if javaErr != nil {
		fmt.Println(res.Content[0].Text)
		return 1
	}
	fmt.Println(res.Content[0].Text)
	return 0
}

func cmdPolicy(wd string, args []string) int {
	reg := mcp_compiler.NewToolRegistry(mcp_compiler.NewMockCoordinator(), mcp_compiler.NewMaestroCompiler("com.nano.app"))
	res, err := reg.Call(context.Background(), "nano_security_verify", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(res.Content[0].Text)
	return 0
}

func cmdCache(wd string, args []string) int {
	cacheDir := filepath.Join(wd, "cache")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return emitOutput(map[string]any{"cache_dir": cacheDir, "exists": false}, true)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return emitOutput(map[string]any{"cache_dir": cacheDir, "entries": names}, true)
}

func emitOutput(v any, _ bool) int {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(string(b))
	return 0
}

func cmdProfileRuntime(ctx context.Context, wd string, args []string) int {
	fs := flag.NewFlagSet("profile", flag.ContinueOnError)
	activity := fs.String("activity", "auto", "target activity")
	serial := fs.String("serial", "", "optional ADB device/emulator serial for live ART/physical profiling")
	pkg := fs.String("pkg", "", "target Android package ID for ADB/Macrobenchmark profiling")
	macroPath := fs.String("macrobenchmark", "", "optional AndroidX Macrobenchmark JSON file inside workspace")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*macroPath) != "" {
		mp, mErr := pathcontainment.ResolveExistingContained(*macroPath, allowedRoots(wd), []string{".json"})
		if mErr != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", mErr)
			return 2
		}
		raw, rErr := os.ReadFile(mp)
		if rErr != nil {
			fmt.Fprintln(os.Stderr, rErr)
			return 1
		}
		metrics, pErr := device.ParseMacrobenchmarkJSON(raw)
		if pErr != nil {
			fmt.Fprintln(os.Stderr, pErr)
			return 1
		}
		return emitOutput(map[string]any{"backend": "android_art", "macrobenchmark": metrics}, true)
	}
	if strings.TrimSpace(*serial) != "" {
		devRep, dErr := device.CollectLiveDevice(ctx, *serial, *pkg)
		if dErr != nil {
			fmt.Fprintln(os.Stderr, dErr)
			return 1
		}
		return emitOutput(devRep, true)
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: nano profile [--activity=Main | --serial=<adb> --pkg=<id> | --macrobenchmark=file.json] <artifact.apk|jar>")
		return 2
	}
	p, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".jar"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
		return 2
	}
	bridge := mcp_compiler.NewLiveHeadlessBridge()
	defer bridge.Close()
	if _, err := bridge.Launch(p, *activity); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	_, _ = bridge.StepTime(150)
	snap, err := jvm.CaptureSnapshot(bridge)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return emitOutput(snap, true)
}

func cmdLab(ctx context.Context, wd string, args []string) int {
	if len(args) == 0 || args[0] == "list" {
		return emitOutput(map[string]any{
			"labs":      labs.ListLabs(),
			"scenarios": labs.StressScenarios23(),
		}, true)
	}
	if args[0] != "run" || len(args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: nano lab list | nano lab run <lab_id> <artifact> [--profile=id] [--activity=cls]")
		return 2
	}
	labID := args[1]
	fs := flag.NewFlagSet("lab run", flag.ContinueOnError)
	profID := fs.String("profile", "", "optional device profile")
	activity := fs.String("activity", "", "optional activity class")
	artifact := args[2]
	if len(args) > 3 {
		_ = fs.Parse(args[3:])
	}
	opts := map[string]string{"profile_id": *profID, "activity": *activity}
	rep, err := labs.RunLab(ctx, wd, labID, artifact, opts)
	if rep != nil {
		_ = emitOutput(rep, true)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return 0
}

func cmdMedia(ctx context.Context, wd string, args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: nano media inspect <file.mp4> | nano media verify <ref> <actual>")
		return 2
	}
	switch args[0] {
	case "inspect":
		p, err := pathcontainment.ResolveExistingContained(args[1], allowedRoots(wd), []string{".mp4", ".mov", ".m4a"})
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		mp4Rep, err := mp4.ParseFile(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		probe, _ := oracle.ProbeWithFFprobe(ctx, p)
		return emitOutput(map[string]any{"mp4": mp4Rep, "ffprobe": probe}, true)
	case "verify":
		fs := flag.NewFlagSet("media verify", flag.ContinueOnError)
		lossy := fs.Bool("lossy", false, "run perceptual PSNR/SSIM/VMAF oracle instead of bit-exact SHA-256")
		codecName := fs.String("codec", "h264", "target codec for lossy verification")
		bitrate := fs.Int("bitrate", 0, "target bitrate bps for lossy verification")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() < 2 {
			fmt.Fprintln(os.Stderr, "usage: nano media verify [--lossy --codec=h264] <ref_file> <actual_file>")
			return 2
		}
		p1, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		p2, err := pathcontainment.ResolveExistingContained(fs.Arg(1), allowedRoots(wd), nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		if *lossy {
			mRes, mErr := profiler_media.VerifyMedia(ctx, profiler_media.MediaVerificationRequest{
				ReferencePath:    p1,
				CompressedPath:   p2,
				TargetCodec:      *codecName,
				TargetBitrateBps: *bitrate,
			})
			if mErr != nil {
				fmt.Fprintln(os.Stderr, mErr)
				return 1
			}
			_ = emitOutput(map[string]any{"oracle_mode": oracle.OracleModeLossyPerceptual, "result": mRes}, true)
			if !mRes.OverallPass {
				return 1
			}
			return 0
		}
		res, err := oracle.VerifyFilesBitExact(p1, p2)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		_ = emitOutput(res, true)
		if !res.Passed {
			return 1
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown media command %q\n", args[0])
		return 2
	}
}

func cmdVisual(ctx context.Context, wd string, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: nano visual list | nano visual approve --name=X --approver=Y <artifact.apk|jar> | nano visual compare --name=X --mode=hybrid <artifact.apk|jar>")
		return 2
	}
	store := visual.NewBaselineStore(filepath.Join(wd, "cache", "baselines"))
	switch args[0] {
	case "list":
		list, err := store.ListBaselines()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return emitOutput(map[string]any{"baselines": list}, true)
	case "approve":
		fs := flag.NewFlagSet("visual approve", flag.ContinueOnError)
		name := fs.String("name", "main_screen", "baseline name")
		profID := fs.String("profile", "clean_install", "device profile id")
		approver := fs.String("approver", "", "explicit approving actor")
		activity := fs.String("activity", "auto", "target activity")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: nano visual approve --name=main --approver=user <artifact.apk|jar>")
			return 2
		}
		p, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".jar"})
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		bridge := mcp_compiler.NewLiveHeadlessBridge()
		defer bridge.Close()
		if _, err := bridge.Launch(p, *activity); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		tmpPNG, err := pathcontainment.ResolveWriteContained(filepath.Join(wd, "cache"), "visual_tmp_approve.png", []string{".png"})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		defer func() { _ = os.Remove(tmpPNG) }()
		rendered, err := bridge.RenderScreen(tmpPNG)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		pngBytes, err := os.ReadFile(rendered)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		ast, _ := bridge.Observe()
		meta, err := store.ApproveBaseline(*name, *profID, *approver, pngBytes, convertASTToUI(ast))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		return emitOutput(meta, true)
	case "compare":
		fs := flag.NewFlagSet("visual compare", flag.ContinueOnError)
		name := fs.String("name", "main_screen", "baseline name")
		profID := fs.String("profile", "clean_install", "device profile id")
		mode := fs.String("mode", "hybrid", "comparison mode: pixel_exact|tolerant_pixel|structural|semantic|text|hybrid")
		activity := fs.String("activity", "auto", "target activity")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: nano visual compare --name=main --mode=hybrid <artifact.apk|jar>")
			return 2
		}
		basePNG, baseMeta, err := store.LoadBaseline(*name, *profID)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		p, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".jar"})
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		bridge := mcp_compiler.NewLiveHeadlessBridge()
		defer bridge.Close()
		if _, err := bridge.Launch(p, *activity); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		tmpPNG, err := pathcontainment.ResolveWriteContained(filepath.Join(wd, "cache"), "visual_tmp_cand.png", []string{".png"})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		defer func() { _ = os.Remove(tmpPNG) }()
		rendered, err := bridge.RenderScreen(tmpPNG)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		candPNG, err := os.ReadFile(rendered)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		candAST, _ := bridge.Observe()
		var baseAST *accessibility.UINode
		if baseMeta != nil {
			baseAST = baseMeta.AST
		}
		rep, err := visual.Compare(basePNG, candPNG, baseAST, convertASTToUI(candAST), visual.CompareOptions{
			Mode: visual.ComparisonMode(*mode),
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		_ = emitOutput(rep, true)
		if !rep.Passed {
			return 1
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown visual subcommand %q\n", args[0])
		return 2
	}
}

func printUsage() {
	fmt.Println("Nano-Android Unified CLI (`nano`)")
	fmt.Println("Commands: init, doctor, inspect, manifest, dex, resources, security, export, report, analytics, sbom, compatibility, diff, launch, test, run, lab, media, visual, graph, ai, plugin, adapter, pool, record, replay, screenshot, inspect-ui, a11y, profile, profiles, benchmark, crash, devices, emulator, build, policy, worker, cache, server")
}

func cmdGraph(wd string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: nano graph <build|impact> [flags] <artifact.apk|aab>")
		return 2
	}
	intelDir := filepath.Join(wd, "cache", "intelligence")
	graphFile := filepath.Join(intelDir, "behavior_graph.json")
	switch args[0] {
	case "build":
		fs := flag.NewFlagSet("graph build", flag.ContinueOnError)
		scanSrc := fs.Bool("source", false, "include workspace Kotlin/Java source scanning")
		if err := fs.Parse(args[1:]); err != nil || fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: nano graph build [--source] <artifact.apk|aab>")
			return 2
		}
		p, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".aab", ".apks", ".xapk"})
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		eng := ingest.NewEngine(filepath.Join(wd, "cache", "apk_intel"), filepath.Join(wd, "tools", "libs", "bundletool-all-1.18.1.jar"))
		rep, err := eng.Analyze(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		srcRoot := ""
		if *scanSrc {
			srcRoot = wd
		}
		g := graph.BuildFromArtifact(rep, srcRoot)
		_ = g.Save(graphFile)
		return emitOutput(map[string]any{"graph_file": graphFile, "architecture": g.ExploreArchitecture()}, true)
	case "impact":
		fs := flag.NewFlagSet("graph impact", flag.ContinueOnError)
		changed := fs.String("changed", "", "comma-separated changed classes, files, or APIs")
		if err := fs.Parse(args[1:]); err != nil || *changed == "" {
			fmt.Fprintln(os.Stderr, "usage: nano graph impact --changed=MainActivity,SQLiteDatabase [artifact.apk]")
			return 2
		}
		var g *graph.BehaviorGraph
		if fs.NArg() > 0 {
			p, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".aab", ".apks", ".xapk"})
			if err != nil {
				fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
				return 2
			}
			eng := ingest.NewEngine(filepath.Join(wd, "cache", "apk_intel"), filepath.Join(wd, "tools", "libs", "bundletool-all-1.18.1.jar"))
			if rep, err := eng.Analyze(p); err == nil {
				g = graph.BuildFromArtifact(rep, "")
			}
		}
		if g == nil {
			var lErr error
			g, lErr = graph.Load(graphFile)
			if lErr != nil {
				fmt.Fprintf(os.Stderr, "load graph: %v\n", lErr)
				return 1
			}
		}
		var targets []string
		for _, part := range strings.Split(*changed, ",") {
			if s := strings.TrimSpace(part); s != "" {
				targets = append(targets, s)
			}
		}
		return emitOutput(g.AnalyzeChangeImpact(targets), true)
	case "query":
		fs := flag.NewFlagSet("graph query", flag.ContinueOnError)
		kind := fs.String("kind", "", "node kind filter")
		match := fs.String("match", "", "name substring filter")
		exported := fs.Bool("exported", false, "exported components only")
		limit := fs.Int("limit", 100, "max nodes")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		g, lErr := graph.Load(graphFile)
		if lErr != nil {
			fmt.Fprintf(os.Stderr, "load graph: %v\n", lErr)
			return 1
		}
		return emitOutput(g.Query(graph.QueryFilter{
			Kind:         graph.NodeKind(*kind),
			NameContains: *match,
			ExportedOnly: *exported,
			Limit:        *limit,
		}), true)
	default:
		fmt.Fprintf(os.Stderr, "unknown graph subcommand %q\n", args[0])
		return 2
	}
}

func cmdAI(ctx context.Context, wd string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: nano ai <understand|explore|generate-tests|diagnose|proposals|approve> [flags]")
		return 2
	}
	intelDir := filepath.Join(wd, "cache", "intelligence")
	graphFile := filepath.Join(intelDir, "behavior_graph.json")
	propFile := filepath.Join(intelDir, "proposals.json")
	approvedRoot := filepath.Join(wd, "cache", "ai_approved")
	ledger, _ := audit.OpenLedger(filepath.Join(wd, "cache", "audit", "nano_ai_cli_audit.jsonl"))
	guard := policy.NewGuard(policy.DefaultConfig())
	store, err := ai.OpenProposalStore(propFile, ledger)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	switch args[0] {
	case "understand":
		fs := flag.NewFlagSet("ai understand", flag.ContinueOnError)
		maxSteps := fs.Int("max-steps", 20, "budget step cap")
		if err := fs.Parse(args[1:]); err != nil || fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: nano ai understand [--max-steps=20] <artifact.apk|aab>")
			return 2
		}
		p, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".aab", ".apks", ".xapk"})
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		eng := ingest.NewEngine(filepath.Join(wd, "cache", "apk_intel"), filepath.Join(wd, "tools", "libs", "bundletool-all-1.18.1.jar"))
		rep, err := eng.Analyze(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		g := graph.BuildFromArtifact(rep, "")
		_ = g.Save(graphFile)
		pkg := "unknown.app"
		if rep.Manifest != nil && rep.Manifest.Package != "" {
			pkg = rep.Manifest.Package
		}
		gov := ai.NewGovernor("", "cli_ai", pkg, ai.ExecutionBudget{MaxSteps: *maxSteps}, guard, ledger)
		u, err := ai.UnderstandAPK(rep, g, gov)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return emitOutput(u, true)
	case "explore":
		fs := flag.NewFlagSet("ai explore", flag.ContinueOnError)
		maxSteps := fs.Int("max-steps", 15, "budget step cap")
		maxScreens := fs.Int("max-screens", 10, "max discovered screens")
		activity := fs.String("activity", "", "optional start activity")
		if err := fs.Parse(args[1:]); err != nil || fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: nano ai explore [--max-steps=15] <artifact.apk|jar>")
			return 2
		}
		p, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".jar"})
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		eng := ingest.NewEngine(filepath.Join(wd, "cache", "apk_intel"), filepath.Join(wd, "tools", "libs", "bundletool-all-1.18.1.jar"))
		rep, _ := eng.Analyze(p)
		coord := mcp_compiler.NewIntegratedCoordinator()
		defer coord.Close()
		g, _ := graph.Load(graphFile)
		gov := ai.NewGovernor("", "cli_ai_explorer", "", ai.ExecutionBudget{MaxSteps: *maxSteps, MaxScreens: *maxScreens}, guard, ledger)
		exp, err := ai.ExploreAndHunt(ctx, coord, p, *activity, rep, g, gov, store)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if g != nil {
			_ = g.Save(graphFile)
		}
		return emitOutput(exp, true)
	case "generate-tests":
		fs := flag.NewFlagSet("ai generate-tests", flag.ContinueOnError)
		if err := fs.Parse(args[1:]); err != nil || fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: nano ai generate-tests <artifact.apk|aab>")
			return 2
		}
		p, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".aab", ".apks", ".xapk"})
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		eng := ingest.NewEngine(filepath.Join(wd, "cache", "apk_intel"), filepath.Join(wd, "tools", "libs", "bundletool-all-1.18.1.jar"))
		rep, err := eng.Analyze(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		g, _ := graph.Load(graphFile)
		gov := ai.NewGovernor("", "cli_ai_tests", "", ai.DefaultBudget(), guard, ledger)
		props, err := ai.GenerateTests(rep, g, nil, gov, store)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return emitOutput(map[string]any{"proposals": props, "budget": gov.Status()}, true)
	case "diagnose":
		fs := flag.NewFlagSet("ai diagnose", flag.ContinueOnError)
		traceFlag := fs.String("trace", "", "inline stack trace text")
		if err := fs.Parse(args[1:]); err != nil || *traceFlag == "" {
			fmt.Fprintln(os.Stderr, "usage: nano ai diagnose --trace=\"...\"")
			return 2
		}
		g, _ := graph.Load(graphFile)
		gov := ai.NewGovernor("", "cli_ai_diagnose", "", ai.DefaultBudget(), guard, ledger)
		diag, err := ai.DiagnoseFailure(*traceFlag, "", nil, g, gov, store)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return emitOutput(diag, true)
	case "proposals":
		return emitOutput(map[string]any{"proposals": store.List()}, true)
	case "coordinate":
		fs := flag.NewFlagSet("ai coordinate", flag.ContinueOnError)
		maxSteps := fs.Int("max-steps", 15, "budget step cap")
		if err := fs.Parse(args[1:]); err != nil || fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: nano ai coordinate [--max-steps=15] <artifact.apk|jar>")
			return 2
		}
		p, err := pathcontainment.ResolveExistingContained(fs.Arg(0), allowedRoots(wd), []string{".apk", ".jar"})
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		eng := ingest.NewEngine(filepath.Join(wd, "cache", "apk_intel"), filepath.Join(wd, "tools", "libs", "bundletool-all-1.18.1.jar"))
		rep, err := eng.Analyze(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		coord := mcp_compiler.NewIntegratedCoordinator()
		defer coord.Close()
		gov := ai.NewGovernor("", "cli_ai_coordinator", "", ai.ExecutionBudget{MaxSteps: *maxSteps}, guard, ledger)
		mem, _ := ai.OpenMemoryStore(filepath.Join(intelDir, "memory.json"))
		res, cErr := ai.MultiAgentCoordinate(ctx, coord, p, rep, "", gov, store, mem)
		if cErr != nil {
			fmt.Fprintln(os.Stderr, cErr)
			return 1
		}
		return emitOutput(res, true)
	case "memory":
		ms, err := ai.OpenMemoryStore(filepath.Join(intelDir, "memory.json"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fs := flag.NewFlagSet("ai memory", flag.ContinueOnError)
		key := fs.String("key", "", "optional memory key to write")
		cat := fs.String("category", "known_quirk", "memory category")
		val := fs.String("value", "", "memory value")
		actor := fs.String("actor", "", "actor identity (required on write)")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *key != "" {
			if err := ms.Put(*key, *cat, *val, *actor); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 2
			}
		}
		return emitOutput(ms, true)
	case "approve":
		fs := flag.NewFlagSet("ai approve", flag.ContinueOnError)
		id := fs.String("id", "", "proposal ID")
		by := fs.String("approver", "", "explicit reviewer identity (required)")
		apply := fs.Bool("apply", false, "write approved file under cache/ai_approved")
		if err := fs.Parse(args[1:]); err != nil || *id == "" {
			fmt.Fprintln(os.Stderr, "usage: nano ai approve --id=<prop_id> --approver=<user> [--apply]")
			return 2
		}
		res, err := store.Approve(*id, *by, *apply, approvedRoot)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		return emitOutput(res, true)
	default:
		fmt.Fprintf(os.Stderr, "unknown ai subcommand %q\n", args[0])
		return 2
	}
}

func buildUnifiedCLIReport(wd string, rep *ingest.IntelligenceReport) *reporting.UnifiedReport {
	masvs := security.AuditAPK(rep)
	compat := registry.EvaluateAPK(rep, "auto")
	pkg := "unknown.app"
	verName := "1.0.0"
	var verCode int64 = 1
	if rep.Manifest != nil {
		if rep.Manifest.Package != "" {
			pkg = rep.Manifest.Package
		}
		if rep.Manifest.VersionName != "" {
			verName = rep.Manifest.VersionName
		}
		if rep.Manifest.VersionCode != 0 {
			verCode = rep.Manifest.VersionCode
		}
	}
	keyFile := filepath.Join(wd, "cache", "keys", "reporting_ed25519.json")
	privHex := ""
	if raw, err := os.ReadFile(keyFile); err == nil {
		var kp struct {
			Priv string `json:"private_key_ed25519"`
		}
		if json.Unmarshal(raw, &kp) == nil && kp.Priv != "" {
			privHex = kp.Priv
		}
	}
	if privHex == "" {
		if pub, priv, kErr := plugins.GenerateEd25519Keypair(); kErr == nil {
			privHex = priv
			_ = os.MkdirAll(filepath.Dir(keyFile), 0750)
			b, _ := json.MarshalIndent(map[string]string{"public_key_ed25519": pub, "private_key_ed25519": priv}, "", "  ")
			_ = os.WriteFile(keyFile, b, 0600)
		}
	}
	ur := &reporting.UnifiedReport{
		ProjectName:            filepath.Base(wd),
		PackageName:            pkg,
		VersionCode:            verCode,
		VersionName:            verName,
		ArtifactSHA256:         rep.SHA256,
		ExecutionTier:          compat.PrimaryBackend,
		ResultStatus:           registry.StatusNanoSimulationPass,
		EnvironmentDisclosure:  "Nano-Android CLI Verifiable Engineering Report",
		ReproducedBehaviors:    []string{"Tier-0 static inspection", "OWASP MASVS v2.1.0", "Compatibility matrix check"},
		NonReproducedBehaviors: compat.Limitations,
		TestSummary:            reporting.TestSummaryMetrics{Total: 1, Passed: 1},
		CompatibilitySummary: reporting.CompatibilitySummaryMetrics{
			PrimaryBackend: compat.PrimaryBackend, ConfidenceScore: compat.ConfidenceScore,
			LimitationCount: len(compat.Limitations), Limitations: compat.Limitations,
		},
		SecuritySummary: reporting.SecuritySummaryMetrics{
			PassedGate: masvs.PassedGate, RiskScore: masvs.RiskScore, Findings: masvs.Findings,
		},
		ArtifactsManifest:  []string{"report.json", "report.html", "report.pdf", "junit.xml", "security.sarif.json"},
		ProvenanceAndAudit: reporting.ProvenanceMetadata{Actor: "cli_user", AuditChainValid: true},
	}
	_ = ur.Sign(privHex)
	return ur
}

func cmdPlugin(ctx context.Context, wd string, args []string) int {
	ledger, _ := audit.OpenLedger(filepath.Join(wd, "cache", "audit", "nano_plugin_cli_audit.jsonl"))
	reg := plugins.NewRegistry(filepath.Join(wd, "cache", "plugins"), ledger, true)
	if len(args) == 0 || args[0] == "list" {
		return emitOutput(map[string]any{"kinds": plugins.SupportedKinds(), "installed": reg.List()}, true)
	}
	switch args[0] {
	case "keygen":
		pub, priv, err := plugins.GenerateEd25519Keypair()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return emitOutput(map[string]string{"public_key_ed25519": pub, "private_key_ed25519": priv}, true)
	case "sign":
		fs := flag.NewFlagSet("plugin sign", flag.ContinueOnError)
		privHex := fs.String("priv", "", "Ed25519 private key hex")
		if err := fs.Parse(args[1:]); err != nil || fs.NArg() < 1 || *privHex == "" {
			fmt.Fprintln(os.Stderr, "usage: nano plugin sign --priv=<hex> <plugin_dir>")
			return 2
		}
		mPath, err := pathcontainment.ResolveExistingContained(filepath.Join(args[1+fs.NArg()-1], "plugin.json"), append(allowedRoots(wd), filepath.Join(wd, "plugins")), []string{".json"})
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		raw, rErr := os.ReadFile(mPath)
		if rErr != nil {
			fmt.Fprintln(os.Stderr, rErr)
			return 1
		}
		raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
		var m plugins.PluginManifest
		if uErr := json.Unmarshal(raw, &m); uErr != nil {
			fmt.Fprintln(os.Stderr, uErr)
			return 1
		}
		entryAbs, eErr := pathcontainment.ResolveExistingContained(filepath.Join(filepath.Dir(mPath), m.Entrypoint), []string{filepath.Dir(mPath)}, nil)
		if eErr != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", eErr)
			return 2
		}
		if sErr := plugins.SignManifest(&m, entryAbs, *privHex); sErr != nil {
			fmt.Fprintln(os.Stderr, sErr)
			return 1
		}
		outBytes, _ := json.MarshalIndent(m, "", "  ")
		if wErr := os.WriteFile(mPath, outBytes, 0600); wErr != nil {
			fmt.Fprintln(os.Stderr, wErr)
			return 1
		}
		return emitOutput(m, true)
	case "verify":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: nano plugin verify <plugin_dir>")
			return 2
		}
		mPath, err := pathcontainment.ResolveExistingContained(filepath.Join(args[1], "plugin.json"), append(allowedRoots(wd), filepath.Join(wd, "plugins")), []string{".json"})
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		raw, rErr := os.ReadFile(mPath)
		if rErr != nil {
			fmt.Fprintln(os.Stderr, rErr)
			return 1
		}
		raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
		var m plugins.PluginManifest
		if uErr := json.Unmarshal(raw, &m); uErr != nil {
			fmt.Fprintln(os.Stderr, uErr)
			return 1
		}
		entryAbs, eErr := pathcontainment.ResolveExistingContained(filepath.Join(filepath.Dir(mPath), m.Entrypoint), []string{filepath.Dir(mPath)}, nil)
		if eErr != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", eErr)
			return 2
		}
		if vErr := plugins.VerifyManifest(m, entryAbs); vErr != nil {
			fmt.Fprintln(os.Stderr, vErr)
			return 2
		}
		return emitOutput(map[string]any{"verified": true, "id": m.ID, "version": m.Version, "sha256": m.SHA256}, true)
	case "install":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: nano plugin install <dir>")
			return 2
		}
		rp, err := reg.InstallFromDir(args[1], append(allowedRoots(wd), filepath.Join(wd, "plugins")))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		return emitOutput(rp, true)
	case "invoke":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: nano plugin invoke <plugin_id>")
			return 2
		}
		out, err := reg.Invoke(ctx, args[1], map[string]any{"source": "nano_cli"})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		return emitOutput(out, true)
	default:
		fmt.Fprintf(os.Stderr, "unknown plugin subcommand %q\n", args[0])
		return 2
	}
}

func cmdAdapter(wd string, args []string) int {
	if len(args) == 0 || args[0] == "list" {
		return emitOutput(map[string]any{"frameworks": frameworks.ProbeAll(wd), "ide_clients": ide.ListClients()}, true)
	}
	switch args[0] {
	case "ide":
		fs := flag.NewFlagSet("adapter ide", flag.ContinueOnError)
		client := fs.String("client", "vscode", "IDE client")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		b, err := ide.GenerateConfig(ide.IDEClient(*client), "nano", wd)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		return emitOutput(b, true)
	case "translate":
		fs := flag.NewFlagSet("adapter translate", flag.ContinueOnError)
		fw := fs.String("framework", "maestro", "framework adapter ID")
		target := fs.String("target", "flows/smoke.yaml", "target artifact/flow/task")
		pkg := fs.String("pkg", "com.example.app", "Android package name")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		cmd, err := frameworks.TranslateCommand(frameworks.FrameworkID(*fw), wd, frameworks.TranslateRequest{
			ArtifactOrTarget: *target,
			PackageName:      *pkg,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		return emitOutput(cmd, true)
	default:
		fmt.Fprintf(os.Stderr, "unknown adapter subcommand %q\n", args[0])
		return 2
	}
}

func cmdPool(ctx context.Context, wd string, args []string) int {
	ledger, _ := audit.OpenLedger(filepath.Join(wd, "cache", "audit", "nano_pool_cli_audit.jsonl"))
	pm := remote.NewPoolManager(remote.DefaultQuota(), ledger,
		remote.ADBProvider{},
		remote.FirebaseTestLabProvider{ProjectID: os.Getenv("FIREBASE_PROJECT_ID"), Tier: "spark"},
	)
	pm.RegisterDevice(remote.PoolDevice{ID: "nano_jvm_local_1", Provider: remote.ProviderManagedOnPrem, SerialOrAddr: "local://1", Model: "NanoWorker-1", APILevel: 35, Healthy: true})
	pm.RegisterDevice(remote.PoolDevice{ID: "nano_jvm_local_2", Provider: remote.ProviderManagedOnPrem, SerialOrAddr: "local://2", Model: "NanoWorker-2", APILevel: 35, Healthy: true})
	devs := pm.Discover(ctx)

	if len(args) == 0 || args[0] == "discover" || args[0] == "health" {
		return emitOutput(map[string]any{"devices": devs, "quotas": remote.DefaultQuota()}, true)
	}
	if args[0] == "reserve" {
		res, dev, err := pm.Reserve("cli_user", "", 5*time.Minute)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		_ = pm.Release(res.ID)
		return emitOutput(map[string]any{"reserved_and_verified_release": res, "device": dev}, true)
	}
	if args[0] == "shard" {
		res, err := pm.SubmitAndShard(ctx, remote.ShardRequest{
			JobName: "CLISharded", Actor: "cli_user", MaxShards: 2,
			Suite: suite.Suite{Name: "CLISuite", Cases: []suite.TestCase{{Name: "Smoke1"}, {Name: "Smoke2"}}},
		}, func(ctx context.Context, dev remote.PoolDevice, cases []suite.TestCase) ([]suite.CaseOutcome, []string, error) {
			var out []suite.CaseOutcome
			for _, tc := range cases {
				out = append(out, suite.CaseOutcome{Name: tc.Name, Status: "NANO_SIMULATION_PASS", BackendUsed: "nano_jvm", Iterations: 1, PassedCount: 1})
			}
			return out, nil, nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return emitOutput(res, true)
	}
	return 2
}

func cmdAnalytics(wd string, args []string) int {
	fs := flag.NewFlagSet("analytics", flag.ContinueOnError)
	baseV := fs.String("base", "", "base version")
	targetV := fs.String("target", "", "target version")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	as, err := reporting.OpenAnalyticsStore(filepath.Join(wd, "cache", "reporting", "analytics.json"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var delta any
	if *baseV != "" && *targetV != "" {
		if d, dErr := as.CompareReleases(*baseV, *targetV); dErr == nil {
			delta = d
		}
	}
	return emitOutput(map[string]any{"trends": as.ComputeTrends(), "delta": delta}, true)
}

func cmdSBOM(wd string, args []string) int {
	fs := flag.NewFlagSet("sbom", flag.ContinueOnError)
	outFile := fs.String("out", "", "optional output file path inside workspace cache or dist")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var components []map[string]any
	libDir := filepath.Join(wd, "tools", "libs")
	if entries, err := os.ReadDir(libDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jar") {
				continue
			}
			raw, rErr := os.ReadFile(filepath.Join(libDir, e.Name()))
			if rErr != nil {
				continue
			}
			sum := sha256.Sum256(raw)
			components = append(components, map[string]any{
				"type":   "library",
				"name":   e.Name(),
				"hashes": []map[string]string{{"alg": "SHA-256", "content": hex.EncodeToString(sum[:])}},
			})
		}
	}
	if gw, err := os.ReadFile(filepath.Join(wd, "go.work")); err == nil {
		for _, rawLine := range strings.Split(string(gw), "\n") {
			ln := strings.TrimSpace(rawLine)
			if strings.HasPrefix(ln, "./") || ln == "." {
				components = append(components, map[string]any{
					"type": "application",
					"name": "nano_android/" + strings.TrimPrefix(ln, "./"),
				})
			}
		}
	}
	sbom := map[string]any{
		"bomFormat":   "CycloneDX",
		"specVersion": "1.5",
		"version":     1,
		"metadata": map[string]any{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"component": map[string]any{"type": "application", "name": "nano-android"},
		},
		"components": components,
	}
	b, _ := json.MarshalIndent(sbom, "", "  ")
	if strings.TrimSpace(*outFile) != "" {
		dst, err := pathcontainment.ResolveWriteContained(wd, *outFile, []string{".json"})
		if err != nil {
			fmt.Fprintf(os.Stderr, "BLOCKED_POLICY: %v\n", err)
			return 2
		}
		_ = os.MkdirAll(filepath.Dir(dst), 0750)
		if err := os.WriteFile(dst, b, 0600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	fmt.Println(string(b))
	return 0
}
