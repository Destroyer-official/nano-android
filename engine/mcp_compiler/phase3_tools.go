package mcp_compiler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"nano_android/engine/compatibility/intel"
	"nano_android/engine/security/audit"
	"nano_android/engine/security/pathcontainment"
)

// Phase3Hooks allows top-level binaries (`cmd/nano`) to attach specialized lab, differential, crash,
// and media verification implementations without introducing Go module import cycles.
type Phase3Hooks struct {
	ListLabs        func() any
	RunLab          func(ctx context.Context, labID, artifactPath string, opts map[string]string) (any, error)
	RunDifferential func(ctx context.Context, args map[string]any) (any, error)
	AnalyzeCrash    func(rawTrace, mappingPath, buildTag, envTier string) (any, error)
	InspectMedia    func(ctx context.Context, mediaPath string) (any, error)
}

// SetPhase3Hooks attaches external Phase 3 handlers for labs, differential, crash forensics, and media inspection.
func (r *ToolRegistry) SetPhase3Hooks(h Phase3Hooks) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks = h
}

func (r *ToolRegistry) getHooks() Phase3Hooks {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hooks
}

func (r *ToolRegistry) registerPhase3Tools() {
	// --- Group 6 Diagnostics: Live JVM & Timeline Profiling ---
	r.Register(ToolDefinition{
		Name:        "nano_profile_memory",
		Description: "Captures live JVM heap allocation, free/max memory, and GC cycles from active Nano-Android session.",
		InputSchema: map[string]any{"type": "object"},
		Handler:     r.handleProfileDimension("memory"),
	})
	r.Register(ToolDefinition{
		Name:        "nano_profile_cpu",
		Description: "Captures CPU/Looper dispatch metrics and virtual uptime progression for active runtime.",
		InputSchema: map[string]any{"type": "object"},
		Handler:     r.handleProfileDimension("cpu"),
	})
	r.Register(ToolDefinition{
		Name:        "nano_profile_threads",
		Description: "Lists live JVM threads, thread states, monitor contention, and deadlock checks.",
		InputSchema: map[string]any{"type": "object"},
		Handler:     r.handleProfileDimension("threads"),
	})
	r.Register(ToolDefinition{
		Name:        "nano_profile_gc",
		Description: "Inspects JVM garbage collection cycle count and heap reclamation headroom.",
		InputSchema: map[string]any{"type": "object"},
		Handler:     r.handleProfileDimension("gc"),
	})
	r.Register(ToolDefinition{
		Name:        "nano_profile_performance",
		Description: "Captures full JVM profiler snapshot (memory, GC, threads, Looper queue depth, timeline events) with explicit environment attribution.",
		InputSchema: map[string]any{"type": "object"},
		Handler:     r.handleProfileDimension("all"),
	})
	r.Register(ToolDefinition{
		Name:        "nano_timeline_inspect",
		Description: "Returns the monotonic correlated ForensicTimeline for the active session.",
		InputSchema: map[string]any{"type": "object"},
		Handler:     r.handleTimelineInspect,
	})
	r.Register(ToolDefinition{
		Name:        "nano_compatibility_db",
		Description: "Queries or seeds the SQLite compatibility.db API 26-37 matrix, confidence scores, and device quirks.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"seed":    map[string]any{"type": "boolean"},
				"db_path": map[string]any{"type": "string"},
			},
		},
		Handler: r.handleCompatibilityDB,
	})

	// --- Group 4 & 6 Phase 3 Specialized Engines ---
	r.Register(ToolDefinition{
		Name:        "nano_lab_list",
		Description: "Lists all 12 specialized Nano-Android test laboratories and the 23 stress scenarios.",
		InputSchema: map[string]any{"type": "object"},
		Handler:     r.handleLabList,
	})
	r.Register(ToolDefinition{
		Name:        "nano_lab_run",
		Description: "Executes one of the 12 specialized test laboratories against a workspace-contained artifact.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"lab_id":     map[string]any{"type": "string"},
				"artifact":   map[string]any{"type": "string"},
				"profile_id": map[string]any{"type": "string"},
				"activity":   map[string]any{"type": "string"},
			},
			"required": []string{"lab_id", "artifact"},
		},
		Handler: r.handleLabRun,
	})
	r.Register(ToolDefinition{
		Name:        "nano_test_differential",
		Description: "Executes an 11-dimension differential flow across backends, profiles, or APK versions to pinpoint earliest divergence.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"mode":       map[string]any{"type": "string"},
				"artifact_a": map[string]any{"type": "string"},
				"artifact_b": map[string]any{"type": "string"},
				"profile_a":  map[string]any{"type": "string"},
				"profile_b":  map[string]any{"type": "string"},
				"backend_a":  map[string]any{"type": "string"},
				"backend_b":  map[string]any{"type": "string"},
			},
			"required": []string{"artifact_a"},
		},
		Handler: r.handleDifferentialRun,
	})
	r.Register(ToolDefinition{
		Name:        "nano_crash_analyze",
		Description: "Parses, secret-redacts, optionally R8-retraces, and hashes a crash or ANR trace.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"trace":        map[string]any{"type": "string"},
				"mapping_path": map[string]any{"type": "string"},
				"build_tag":    map[string]any{"type": "string"},
			},
			"required": []string{"trace"},
		},
		Handler: r.handleCrashAnalyze,
	})
	r.Register(ToolDefinition{
		Name:        "nano_media_inspect",
		Description: "Inspects an MP4/MOV media artifact using ISO BMFF atom parsing and ffprobe verification.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"media_path": map[string]any{"type": "string"}},
			"required":   []string{"media_path"},
		},
		Handler: r.handleMediaInspect,
	})
}

func (r *ToolRegistry) handleProfileDimension(dim string) func(context.Context, map[string]any) (*ToolResult, error) {
	return func(ctx context.Context, args map[string]any) (*ToolResult, error) {
		state, err := r.coord.InspectState(ctx, dim)
		if err != nil {
			return makeErrorResult(fmt.Sprintf("inspect state failed: %v", err))
		}
		payload := map[string]any{
			"status":                 "ok",
			"dimension":              dim,
			"backend":                "nano_jvm",
			"environment_disclosure": "Host OpenJDK 25 JVM runtime telemetry (not Android ART RenderThread/ODPM silicon)",
			"inspect_state":          state,
		}
		if ic, ok := r.coord.(*IntegratedCoordinator); ok && ic.Bridge() != nil {
			if rm, rErr := ic.Bridge().InspectRuntime(); rErr == nil {
				payload["live_metrics"] = rm
			}
			if dim == "threads" || dim == "all" || dim == "cpu" {
				if th, tErr := ic.Bridge().InspectThreads(); tErr == nil {
					payload["threads"] = th
				}
			}
		}
		return makeJSONResult(payload)
	}
}

func (r *ToolRegistry) handleTimelineInspect(ctx context.Context, args map[string]any) (*ToolResult, error) {
	if ic, ok := r.coord.(*IntegratedCoordinator); ok && ic.Bridge() != nil {
		events, err := ic.Bridge().InspectTimeline()
		if err != nil {
			return makeErrorResult(err.Error())
		}
		for i := range events {
			events[i].Source = audit.RedactString(events[i].Source)
			events[i].Details = audit.RedactString(events[i].Details)
		}
		return makeJSONResult(map[string]any{
			"status":  "ok",
			"backend": "nano_jvm",
			"count":   len(events),
			"events":  events,
		})
	}
	return makeJSONResult(map[string]any{
		"status":  "ok",
		"backend": "mock",
		"count":   0,
		"events":  []any{},
		"note":    "No active LiveHeadlessBridge attached to coordinator",
	})
}

func (r *ToolRegistry) handleCompatibilityDB(ctx context.Context, args map[string]any) (*ToolResult, error) {
	cacheRoot := resolveCacheDir()
	dbName := "compatibility.db"
	if raw, _ := args["db_path"].(string); strings.TrimSpace(raw) != "" {
		dbName = raw
	}
	containedDB, err := pathcontainment.ResolveWriteContained(cacheRoot, filepath.Base(dbName), []string{".db"})
	if err != nil {
		return makeErrorResult(fmt.Sprintf("BLOCKED_POLICY db_path containment error: %v", err))
	}
	store, err := intel.OpenStore(containedDB)
	if err != nil {
		return makeErrorResult(err.Error())
	}
	seed, _ := args["seed"].(bool)
	if seed {
		if err := store.SeedDefaults(); err != nil {
			return makeErrorResult(err.Error())
		}
	} else if st, sErr := os.Stat(containedDB); sErr != nil || st.Size() == 0 {
		_ = store.SeedDefaults()
	}
	hm, err := store.QueryHeatmap()
	if err != nil {
		return makeErrorResult(err.Error())
	}
	return makeJSONResult(hm)
}

func (r *ToolRegistry) handleLabList(ctx context.Context, args map[string]any) (*ToolResult, error) {
	h := r.getHooks()
	if h.ListLabs != nil {
		return makeJSONResult(h.ListLabs())
	}
	return makeJSONResult(map[string]any{
		"status": "ok",
		"labs": []string{
			"crash", "permission", "network", "storage", "visual", "media",
			"accessibility", "compatibility", "stress", "security", "differential", "performance",
		},
	})
}

func (r *ToolRegistry) handleLabRun(ctx context.Context, args map[string]any) (*ToolResult, error) {
	h := r.getHooks()
	if h.RunLab == nil {
		return makeErrorResult("INFRASTRUCTURE_ERROR: RunLab hook not attached in this process (invoke via `nano server` or `nano lab run`)")
	}
	labID, _ := args["lab_id"].(string)
	artifact, _ := args["artifact"].(string)
	opts := map[string]string{}
	if p, _ := args["profile_id"].(string); p != "" {
		opts["profile_id"] = p
	}
	if a, _ := args["activity"].(string); a != "" {
		opts["activity"] = a
	}
	res, err := h.RunLab(ctx, strings.TrimSpace(labID), strings.TrimSpace(artifact), opts)
	if err != nil {
		return makeErrorResult(err.Error())
	}
	return makeJSONResult(res)
}

func (r *ToolRegistry) handleDifferentialRun(ctx context.Context, args map[string]any) (*ToolResult, error) {
	h := r.getHooks()
	if h.RunDifferential == nil {
		return makeErrorResult("INFRASTRUCTURE_ERROR: RunDifferential hook not attached in this process (invoke via `nano server` or `nano diff`)")
	}
	res, err := h.RunDifferential(ctx, args)
	if err != nil {
		return makeErrorResult(err.Error())
	}
	return makeJSONResult(res)
}

func (r *ToolRegistry) handleCrashAnalyze(ctx context.Context, args map[string]any) (*ToolResult, error) {
	trace, _ := args["trace"].(string)
	mappingPath, _ := args["mapping_path"].(string)
	buildTag, _ := args["build_tag"].(string)

	if mappingPath != "" {
		allowed := []string{resolveCacheDir(), resolvePath("fixtures"), resolvePath("tools")}
		containedMap, err := pathcontainment.ResolveExistingContained(mappingPath, allowed, []string{".txt", ".map"})
		if err != nil {
			return makeErrorResult(fmt.Sprintf("BLOCKED_POLICY mapping_path: %v", err))
		}
		mappingPath = containedMap
	}

	h := r.getHooks()
	if h.AnalyzeCrash != nil {
		res, err := h.AnalyzeCrash(trace, mappingPath, buildTag, "nano_jvm")
		if err != nil {
			return makeErrorResult(err.Error())
		}
		return makeJSONResult(res)
	}
	return makeJSONResult(map[string]any{
		"status":         "ok",
		"redacted_trace": audit.RedactString(trace),
		"note":           "Basic redaction applied; attach Phase3Hooks via `nano server` for full R8 retrace & crash clustering",
	})
}

func (r *ToolRegistry) handleMediaInspect(ctx context.Context, args map[string]any) (*ToolResult, error) {
	mediaPath, _ := args["media_path"].(string)
	allowed := []string{resolveCacheDir(), resolvePath("fixtures"), resolvePath("downloads")}
	contained, err := pathcontainment.ResolveExistingContained(strings.TrimSpace(mediaPath), allowed, []string{".mp4", ".mov", ".m4a"})
	if err != nil {
		return makeErrorResult(fmt.Sprintf("BLOCKED_POLICY media_path: %v", err))
	}
	h := r.getHooks()
	if h.InspectMedia == nil {
		return makeErrorResult("INFRASTRUCTURE_ERROR: InspectMedia hook not attached in this process (invoke via `nano server` or `nano media inspect`)")
	}
	res, err := h.InspectMedia(ctx, contained)
	if err != nil {
		return makeErrorResult(err.Error())
	}
	return makeJSONResult(res)
}
