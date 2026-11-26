package mcp_compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"nano_android/engine/apk/ingest"
	"nano_android/engine/apk/security"
	"nano_android/engine/compatibility/registry"
	"nano_android/engine/security/audit"
	"nano_android/engine/security/pathcontainment"
	"nano_android/engine/ui/accessibility"
	"nano_android/engine/ui/profile"
)

func (r *ToolRegistry) registerPhase2GroupTools() {
	// --- Group 1: Project & APK ---
	r.Register(ToolDefinition{
		Name:        "nano_apk_inspect",
		Description: "Inspects an APK/AAB/APKS/XAPK package statically and returns complete Tier-0 metadata.",
		InputSchema: r.apkOnlySchema(),
		Handler:     r.handleInspectAPK,
	})
	r.Register(ToolDefinition{
		Name:        "nano_apk_manifest",
		Description: "Extracts AndroidManifest.xml components, SDK levels, exported flags, permissions, and deep links.",
		InputSchema: r.apkOnlySchema(),
		Handler:     func(ctx context.Context, args map[string]any) (*ToolResult, error) { return r.handleAPKSubset(args, "manifest") },
	})
	r.Register(ToolDefinition{
		Name:        "nano_apk_dex_inspect",
		Description: "Analyzes classes*.dex bytecode for class inventory, dependencies, libraries, and framework issues.",
		InputSchema: r.apkOnlySchema(),
		Handler:     func(ctx context.Context, args map[string]any) (*ToolResult, error) { return r.handleAPKSubset(args, "dex") },
	})
	r.Register(ToolDefinition{
		Name:        "nano_apk_resource_inspect",
		Description: "Inspects resources.arsc entries, APK size distribution, and duplicate asset payloads.",
		InputSchema: r.apkOnlySchema(),
		Handler:     func(ctx context.Context, args map[string]any) (*ToolResult, error) { return r.handleAPKSubset(args, "resources") },
	})
	r.Register(ToolDefinition{
		Name:        "nano_apk_native_inspect",
		Description: "Audits .so ELF native libraries for ABI, JNI entrypoints, and Android 17 16 KB alignment.",
		InputSchema: r.apkOnlySchema(),
		Handler:     func(ctx context.Context, args map[string]any) (*ToolResult, error) { return r.handleAPKSubset(args, "native") },
	})
	r.Register(ToolDefinition{
		Name:        "nano_apk_security_audit",
		Description: "Runs OWASP MASVS v2.1.0 static security checks and emits findings + SARIF v2.1.0 report.",
		InputSchema: r.apkOnlySchema(),
		Handler:     r.handleAPKSecurityAudit,
	})
	r.Register(ToolDefinition{
		Name:        "nano_apk_compatibility",
		Description: "Evaluates APK API compatibility and recommends optimal execution tier (static/nano_jvm/robolectric/android_art/physical_device).",
		InputSchema: r.apkOnlySchema(),
		Handler:     func(ctx context.Context, args map[string]any) (*ToolResult, error) { return r.handleAPKSubset(args, "compatibility") },
	})
	r.Register(ToolDefinition{
		Name:        "nano_apk_compare",
		Description: "Compares two APK/AAB artifacts for size, permission, component, class, and signer differences.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"apk_path":          map[string]any{"type": "string"},
				"compare_with_path": map[string]any{"type": "string"},
			},
			"required": []string{"apk_path", "compare_with_path"},
		},
		Handler: r.handleInspectAPK,
	})

	// --- Group 2: Execution ---
	r.Register(ToolDefinition{
		Name:        "nano_runtime_launch",
		Description: "Launches an APK/JAR inside an isolated deterministic Nano JVM runtime.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"apk_path":    map[string]any{"type": "string"},
				"activity":    map[string]any{"type": "string"},
				"clear_state": map[string]any{"type": "boolean"},
			},
			"required": []string{"apk_path"},
		},
		Handler: r.handleLaunchApp,
	})
	r.Register(ToolDefinition{
		Name:        "nano_runtime_advance_time",
		Description: "Advances deterministic virtual Looper clock by delta_ms.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"delta_ms": map[string]any{"type": "integer"}},
			"required":   []string{"delta_ms"},
		},
		Handler: r.handleStepTime,
	})
	r.Register(ToolDefinition{
		Name:        "nano_runtime_capabilities",
		Description: "Lists registered device/scenario profiles and canonical ResultStatus / API compatibility contracts.",
		InputSchema: map[string]any{"type": "object"},
		Handler:     r.handleRuntimeCapabilities,
	})

	// --- Group 3: UI & Accessibility ---
	r.Register(ToolDefinition{
		Name:        "nano_ui_observe",
		Description: "Returns pruned UI accessibility tree and optional framebuffer capture.",
		InputSchema: map[string]any{"type": "object"},
		Handler:     r.handleObserveScreen,
	})
	r.Register(ToolDefinition{
		Name:        "nano_ui_tap",
		Description: "Dispatches a tap click to a UI node ID, text, or coordinate.",
		InputSchema: r.uiTargetSchema(),
		Handler: func(ctx context.Context, args map[string]any) (*ToolResult, error) {
			return r.dispatchCanonicalUI(ctx, "tap", args)
		},
	})
	r.Register(ToolDefinition{
		Name:        "nano_ui_long_press",
		Description: "Dispatches a long press (performLongClick) to a UI node.",
		InputSchema: r.uiTargetSchema(),
		Handler: func(ctx context.Context, args map[string]any) (*ToolResult, error) {
			return r.dispatchCanonicalUI(ctx, "long_click", args)
		},
	})
	r.Register(ToolDefinition{
		Name:        "nano_ui_type",
		Description: "Types text into a target EditText/TextView node.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node_id": map[string]any{"type": "string"},
				"text":    map[string]any{"type": "string"},
			},
			"required": []string{"node_id", "text"},
		},
		Handler: func(ctx context.Context, args map[string]any) (*ToolResult, error) {
			return r.dispatchCanonicalUI(ctx, "type", args)
		},
	})
	r.Register(ToolDefinition{
		Name:        "nano_ui_swipe",
		Description: "Performs a directional swipe or scroll gesture on the active UI.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"direction": map[string]any{"type": "string"},
				"node_id":   map[string]any{"type": "string"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (*ToolResult, error) {
			return r.dispatchCanonicalUI(ctx, "swipe", args)
		},
	})
	r.Register(ToolDefinition{
		Name:        "nano_ui_back",
		Description: "Dispatches an Android BACK key event / activity stack pop.",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(ctx context.Context, args map[string]any) (*ToolResult, error) {
			return r.dispatchCanonicalUI(ctx, "back", args)
		},
	})
	r.Register(ToolDefinition{
		Name:        "nano_ui_assert",
		Description: "Evaluates UI visibility or text assertions and records them into the regression flow.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"assertion_type": map[string]any{"type": "string"},
				"target":         map[string]any{"type": "string"},
				"expected_text":  map[string]any{"type": "string"},
			},
			"required": []string{"assertion_type", "target"},
		},
		Handler: r.handleAssertUI,
	})
	r.Register(ToolDefinition{
		Name:        "nano_ui_accessibility_audit",
		Description: "Audits current observed UI hierarchy for 48dp touch targets, missing labels, large-font clipping, and Compose surfaces.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"profile_id": map[string]any{"type": "string"}},
		},
		Handler: r.handleUIAccessibilityAudit,
	})

	// --- Group 4: Test Engineering ---
	r.Register(ToolDefinition{
		Name:        "nano_test_generate_regression",
		Description: "Freezes interactive session into deterministic Maestro regression YAML with repeat_count >= 3.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"app_id":       map[string]any{"type": "string"},
				"flow_name":    map[string]any{"type": "string"},
				"repeat_count": map[string]any{"type": "integer"},
				"output_path":  map[string]any{"type": "string"},
			},
		},
		Handler: r.handleFreezeTest,
	})
	r.Register(ToolDefinition{
		Name:        "nano_test_get_flow",
		Description: "Previews recorded Maestro YAML actions without writing disk.",
		InputSchema: map[string]any{"type": "object"},
		Handler:     r.handleGetSessionFlow,
	})
	r.Register(ToolDefinition{
		Name:        "nano_test_clear",
		Description: "Clears recorded actions in the active compiler session.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"confirm": map[string]any{"type": "boolean"}},
			"required":   []string{"confirm"},
		},
		Handler: r.handleClearSession,
	})

	// --- Group 6: Diagnostics ---
	r.Register(ToolDefinition{
		Name:        "nano_profile_state",
		Description: "Queries runtime JVM memory, GC, loaded classes, and Looper state.",
		InputSchema: map[string]any{"type": "object"},
		Handler:     r.handleInspectState,
	})

	// --- Group 7: Governance ---
	r.Register(ToolDefinition{
		Name:        "nano_policy_inspect",
		Description: "Inspects active Nano-Android security policy guard configuration and blocked tool rules.",
		InputSchema: map[string]any{"type": "object"},
		Handler:     r.handlePolicyInspect,
	})
	r.Register(ToolDefinition{
		Name:        "nano_security_verify",
		Description: "Verifies immutable audit ledger hash-chain integrity and active path containment configuration.",
		InputSchema: map[string]any{"type": "object"},
		Handler:     r.handleSecurityVerify,
	})
	r.Register(ToolDefinition{
		Name:        "nano_audit_history",
		Description: "Returns recent SHA-256 hash-chained, secret-redacted entries from the active audit ledger.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"limit": map[string]any{"type": "integer"}},
		},
		Handler: r.handleAuditHistory,
	})

	// --- Additional Group 1 / 2 / 3 / 5 Tools ---
	r.Register(ToolDefinition{
		Name:        "nano_project_discover",
		Description: "Scans workspace directories for Gradle build scripts, Android manifests, APKs, and JARs.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"root": map[string]any{"type": "string"}},
		},
		Handler: r.handleProjectDiscover,
	})
	r.Register(ToolDefinition{
		Name:        "nano_ui_screenshot",
		Description: "Captures current Java2D rendered screen framebuffer from the running Nano JVM activity.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"format": map[string]any{"type": "string"}},
		},
		Handler: r.handleUIScreenshot,
	})
	r.Register(ToolDefinition{
		Name:        "nano_ui_compare",
		Description: "Compares two visible-text slices or snapshot names for visual/AST hierarchy regressions.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"baseline_texts": map[string]any{"type": "array"},
			},
		},
		Handler: r.handleUICompare,
	})
	r.Register(ToolDefinition{
		Name:        "nano_device_list",
		Description: "Lists connected physical Android devices and local emulators via ADB (explicitly reporting if ADB is unavailable).",
		InputSchema: map[string]any{"type": "object"},
		Handler:     r.handleDeviceList,
	})
	r.Register(ToolDefinition{
		Name:        "nano_device_status",
		Description: "Checks ADB, JDK, and Nano JVM runtime worker readiness.",
		InputSchema: map[string]any{"type": "object"},
		Handler:     r.handleDeviceStatus,
	})
}

func (r *ToolRegistry) apkOnlySchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{"apk_path": map[string]any{"type": "string"}},
		"required":   []string{"apk_path"},
	}
}

func (r *ToolRegistry) uiTargetSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"node_id": map[string]any{"type": "string"},
			"x":       map[string]any{"type": "number"},
			"y":       map[string]any{"type": "number"},
		},
	}
}

func (r *ToolRegistry) dispatchCanonicalUI(ctx context.Context, act string, args map[string]any) (*ToolResult, error) {
	clone := make(map[string]any, len(args)+1)
	for k, v := range args {
		clone[k] = v
	}
	clone["action"] = act
	return r.handleDispatchAction(ctx, clone)
}

func (r *ToolRegistry) analyzeContainedArtifact(apkPath string) (*ingest.IntelligenceReport, error) {
	allowedRoots := []string{
		resolveCacheDir(),
		resolvePath("downloads"),
		resolvePath("tools"),
		resolvePath("fixtures"),
	}
	contained, err := pathcontainment.ResolveExistingContained(apkPath, allowedRoots, []string{".apk", ".aab", ".apks", ".xapk"})
	if err != nil {
		return nil, fmt.Errorf("BLOCKED_POLICY apk_path outside workspace roots: %w", err)
	}
	bundletoolJar := resolvePath(
		filepath.Join("tools", "libs", "bundletool-all-1.18.1.jar"),
		filepath.Join("..", "..", "tools", "libs", "bundletool-all-1.18.1.jar"),
	)
	eng := ingest.NewEngine(filepath.Join(resolveCacheDir(), "apk_intel"), bundletoolJar)
	return eng.Analyze(contained)
}

func (r *ToolRegistry) handleAPKSubset(args map[string]any, section string) (*ToolResult, error) {
	apkPath, _ := args["apk_path"].(string)
	rep, err := r.analyzeContainedArtifact(strings.TrimSpace(apkPath))
	if err != nil {
		return makeErrorResult(err.Error())
	}
	switch section {
	case "manifest":
		return makeJSONResult(map[string]any{"status": "ok", "tier": "static", "manifest": rep.Manifest, "permission_risks": rep.PermissionRisks})
	case "dex":
		return makeJSONResult(map[string]any{"status": "ok", "tier": "static", "dex": rep.DEX})
	case "resources":
		return makeJSONResult(map[string]any{"status": "ok", "tier": "static", "resources": rep.Resources, "size_distribution": rep.SizeDistribution, "duplicate_assets": rep.DuplicateAssets})
	case "native":
		return makeJSONResult(map[string]any{"status": "ok", "tier": "static", "native_libs": rep.NativeLibs})
	case "compatibility":
		rec := registry.EvaluateAPK(rep, "auto")
		return makeJSONResult(map[string]any{"status": "ok", "tier": "static", "recommendation": rec})
	default:
		return makeJSONResult(rep)
	}
}

func (r *ToolRegistry) handleAPKSecurityAudit(ctx context.Context, args map[string]any) (*ToolResult, error) {
	apkPath, _ := args["apk_path"].(string)
	rep, err := r.analyzeContainedArtifact(strings.TrimSpace(apkPath))
	if err != nil {
		return makeErrorResult(err.Error())
	}
	auditRep := security.AuditAPK(rep)
	sarifBytes, _ := auditRep.ExportSARIF()
	return makeJSONResult(map[string]any{
		"status": "ok",
		"tier":   "static",
		"audit":  auditRep,
		"sarif":  string(sarifBytes),
	})
}

func (r *ToolRegistry) handleRuntimeCapabilities(ctx context.Context, args map[string]any) (*ToolResult, error) {
	return makeJSONResult(map[string]any{
		"status":    "ok",
		"profiles":  profile.ReusableProfiles(),
		"contracts": registry.DefaultContracts(),
	})
}

func (r *ToolRegistry) handleUIAccessibilityAudit(ctx context.Context, args map[string]any) (*ToolResult, error) {
	profID, _ := args["profile_id"].(string)
	if strings.TrimSpace(profID) == "" {
		profID = "clean_install"
	}
	prof, err := profile.Get(profID)
	if err != nil {
		return makeErrorResult(err.Error())
	}
	obs, err := r.coord.ObserveScreen(ctx, false, "webp", 0)
	if err != nil {
		return makeErrorResult(fmt.Sprintf("observe failed: %v", err))
	}
	uiRoot := toA11yNode(obs.AST)
	return makeJSONResult(accessibility.AuditTree(uiRoot, prof))
}

func toA11yNode(n *ASTNode) *accessibility.UINode {
	if n == nil {
		return nil
	}
	u := &accessibility.UINode{
		ID:                 n.ID,
		ResourceID:         n.ResourceID,
		Class:              n.Type,
		Text:               n.Text,
		ContentDescription: n.ContentDescription,
		Clickable:          n.Clickable,
		Checkable:          n.Checkable,
		Focusable:          n.Focused,
		Visible:            n.Visible,
		Bounds:             n.Bounds,
	}
	for _, c := range n.Children {
		u.Children = append(u.Children, toA11yNode(c))
	}
	return u
}

func (r *ToolRegistry) handlePolicyInspect(ctx context.Context, args map[string]any) (*ToolResult, error) {
	r.mu.RLock()
	hasGuard := r.guard != nil
	hasLedger := r.ledger != nil
	r.mu.RUnlock()
	return makeJSONResult(map[string]any{
		"status":        "ok",
		"guard_active":  hasGuard,
		"ledger_active": hasLedger,
		"tool_count":    len(r.tools),
	})
}

func (r *ToolRegistry) handleSecurityVerify(ctx context.Context, args map[string]any) (*ToolResult, error) {
	r.mu.RLock()
	ledger := r.ledger
	r.mu.RUnlock()
	valid := true
	errStr := ""
	verifiedEntries := 0
	if ledger != nil && ledger.FilePath() != "" {
		if n, err := audit.VerifyChain(ledger.FilePath()); err != nil && !os.IsNotExist(err) {
			valid = false
			errStr = err.Error()
		} else {
			verifiedEntries = n
		}
	}
	return makeJSONResult(map[string]any{
		"status":             "ok",
		"audit_chain_valid":  valid,
		"verified_entries":   verifiedEntries,
		"audit_chain_error":  errStr,
		"allowed_roots_hint": []string{"cache", "downloads", "tools", "fixtures"},
	})
}

func (r *ToolRegistry) handleAuditHistory(ctx context.Context, args map[string]any) (*ToolResult, error) {
	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	r.mu.RLock()
	ledger := r.ledger
	r.mu.RUnlock()
	if ledger == nil || ledger.FilePath() == "" {
		return makeJSONResult(map[string]any{"status": "ok", "entries": []any{}})
	}
	raw, err := os.ReadFile(ledger.FilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return makeJSONResult(map[string]any{"status": "ok", "entries": []any{}})
		}
		return makeErrorResult(err.Error())
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	start := 0
	if len(lines) > limit {
		start = len(lines) - limit
	}
	var entries []map[string]any
	for _, line := range lines[start:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj map[string]any
		if json.Unmarshal([]byte(line), &obj) == nil {
			entries = append(entries, obj)
		}
	}
	return makeJSONResult(map[string]any{"status": "ok", "count": len(entries), "entries": entries})
}

func (r *ToolRegistry) handleProjectDiscover(ctx context.Context, args map[string]any) (*ToolResult, error) {
	wd, _ := os.Getwd()
	scanRoot := wd
	if reqRoot, _ := args["root"].(string); strings.TrimSpace(reqRoot) != "" {
		allowed := []string{wd, resolveCacheDir(), resolvePath("fixtures"), resolvePath("tools")}
		c, err := pathcontainment.ResolveExistingDirContained(reqRoot, allowed)
		if err != nil {
			return makeErrorResult(fmt.Sprintf("BLOCKED_POLICY root outside workspace: %v", err))
		}
		scanRoot = c
	}
	var apks, jars, manifests, gradleFiles []string
	_ = filepath.WalkDir(scanRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "bin" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		name := d.Name()
		switch {
		case strings.HasSuffix(name, ".apk"):
			apks = append(apks, p)
		case strings.HasSuffix(name, ".jar") && !strings.Contains(p, string(filepath.Separator)+"libs"+string(filepath.Separator)):
			jars = append(jars, p)
		case name == "AndroidManifest.xml":
			manifests = append(manifests, p)
		case name == "build.gradle" || name == "build.gradle.kts" || name == "settings.gradle" || name == "settings.gradle.kts":
			gradleFiles = append(gradleFiles, p)
		}
		return nil
	})
	return makeJSONResult(map[string]any{
		"status":       "ok",
		"root":         scanRoot,
		"apks":         apks,
		"jars":         jars,
		"manifests":    manifests,
		"gradle_files": gradleFiles,
	})
}

func (r *ToolRegistry) handleUIScreenshot(ctx context.Context, args map[string]any) (*ToolResult, error) {
	format, _ := args["format"].(string)
	if format == "" {
		format = "png"
	}
	obs, err := r.coord.ObserveScreen(ctx, true, format, 0)
	if err != nil {
		return makeErrorResult(fmt.Sprintf("screenshot observe failed: %v", err))
	}
	return makeJSONResult(map[string]any{
		"status":     "ok",
		"activity":   obs.ActiveActivity,
		"screenshot": obs.Screenshot,
	})
}

func (r *ToolRegistry) handleUICompare(ctx context.Context, args map[string]any) (*ToolResult, error) {
	obs, err := r.coord.ObserveScreen(ctx, false, "webp", 0)
	if err != nil {
		return makeErrorResult(fmt.Sprintf("observe failed: %v", err))
	}
	curSet := map[string]bool{}
	for _, t := range obs.VisibleTexts {
		curSet[t] = true
	}
	var baseline []string
	if rawList, ok := args["baseline_texts"].([]any); ok {
		for _, item := range rawList {
			if s, ok := item.(string); ok {
				baseline = append(baseline, s)
			}
		}
	}
	baseSet := map[string]bool{}
	var missing, added []string
	for _, b := range baseline {
		baseSet[b] = true
		if !curSet[b] {
			missing = append(missing, b)
		}
	}
	for _, c := range obs.VisibleTexts {
		if !baseSet[c] {
			added = append(added, c)
		}
	}
	return makeJSONResult(map[string]any{
		"status":        "ok",
		"matched":       len(missing) == 0 && len(added) == 0,
		"missing_texts": missing,
		"added_texts":   added,
		"current_count": len(obs.VisibleTexts),
	})
}

func (r *ToolRegistry) handleDeviceList(ctx context.Context, args map[string]any) (*ToolResult, error) {
	adbPath, err := exec.LookPath("adb")
	if err != nil {
		return makeJSONResult(map[string]any{
			"status":        "ok",
			"adb_available": false,
			"devices":       []any{},
			"message":       "adb executable not found on PATH; physical/emulator backends unavailable",
		})
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, cmdErr := exec.CommandContext(cctx, adbPath, "devices", "-l").CombinedOutput()
	if cmdErr != nil {
		return makeJSONResult(map[string]any{
			"status":        "ok",
			"adb_available": true,
			"devices":       []any{},
			"error":         cmdErr.Error(),
		})
	}
	var devices []map[string]string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			devices = append(devices, map[string]string{"serial": fields[0], "state": fields[1], "details": strings.Join(fields[2:], " ")})
		}
	}
	return makeJSONResult(map[string]any{"status": "ok", "adb_available": true, "devices": devices})
}

func (r *ToolRegistry) handleDeviceStatus(ctx context.Context, args map[string]any) (*ToolResult, error) {
	_, adbErr := exec.LookPath("adb")
	_, javaErr := exec.LookPath("java")
	return makeJSONResult(map[string]any{
		"status":         "ok",
		"adb_installed":  adbErr == nil,
		"java_installed": javaErr == nil,
		"nano_jvm_ready": javaErr == nil,
	})
}
