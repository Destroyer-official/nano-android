package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nano_android/engine/apk/ingest"
	"nano_android/engine/apk/security"
	"nano_android/engine/backend/remote"
	"nano_android/engine/compatibility/intel"
	"nano_android/engine/compatibility/registry"
	"nano_android/engine/mcp_compiler"
	"nano_android/engine/profiler/jvm"
	"nano_android/engine/reporting"
	"nano_android/engine/security/archive"
	"nano_android/engine/security/audit"
	"nano_android/engine/security/httpauth"
	"nano_android/engine/security/pathcontainment"
	"nano_android/engine/security/policy"
	"nano_android/engine/testing/labs"
	"nano_android/engine/testing/orchestrator"
	"nano_android/engine/testing/suite"
	"nano_android/engine/ui/accessibility"
	"nano_android/engine/ui/profile"
	"nano_android/engine/ui/visual"
)

// ArtifactEntry describes one discovered/uploaded APK, JAR, AAB, APKS, or XAPK in the workspace.
type ArtifactEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	Kind      string `json:"kind"`
	SHA256    string `json:"sha256"`
}

// ExecutionRecord tracks an active or completed execution session exposed by /api/v1/executions.
type ExecutionRecord struct {
	ID         string                     `json:"id"`
	Artifact   string                     `json:"artifact"`
	PackageID  string                     `json:"package_id,omitempty"`
	Activity   string                     `json:"activity,omitempty"`
	Backend    registry.Backend           `json:"backend"`
	StartedUTC string                     `json:"started_utc"`
	Status     string                     `json:"status"`
	Launch     *mcp_compiler.LaunchResult `json:"launch,omitempty"`
}

// BreakpointRule defines a lifecycle/assertion/API breakpoint configured in Studio.
type BreakpointRule struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"` // "lifecycle", "assertion", "api", "exception"
	Target  string `json:"target"`
	Enabled bool   `json:"enabled"`
}

// ReportRecord stores generated suite/lab/security/visual reports under /api/v1/reports/{id}.
type ReportRecord struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	CreatedUTC string `json:"created_utc"`
	Payload    any    `json:"payload"`
}

// Service implements the shared REST /api/v1/* and WebSocket developer platform backend.
type Service struct {
	WorkspaceRoot string
	Coord         *mcp_compiler.IntegratedCoordinator
	Guard         *policy.Guard
	Ledger        *audit.Ledger
	Baselines     *visual.BaselineStore
	Bus           *EventBus
	BindHost      string

	mu          sync.RWMutex
	executions  map[string]*ExecutionRecord
	execOrder   []string
	reports     map[string]*ReportRecord
	reportOrder []string
	breakpoints map[string]BreakpointRule
	seq         atomic.Int64
	poolOnce    sync.Once
	poolMgr     *remote.PoolManager
}

// NewService constructs a shared API service anchored to workspaceRoot.
func NewService(workspaceRoot string, coord *mcp_compiler.IntegratedCoordinator, guard *policy.Guard, ledger *audit.Ledger) *Service {
	if workspaceRoot == "" {
		workspaceRoot, _ = os.Getwd()
	}
	if coord == nil {
		coord = mcp_compiler.NewIntegratedCoordinator()
	}
	if guard == nil {
		guard = policy.NewGuard(policy.DefaultConfig())
	}
	if ledger == nil {
		ledger, _ = audit.OpenLedger(filepath.Join(workspaceRoot, "cache", "audit", "nano_api_audit.jsonl"))
	}
	return &Service{
		WorkspaceRoot: workspaceRoot,
		Coord:         coord,
		Guard:         guard,
		Ledger:        ledger,
		Baselines:     visual.NewBaselineStore(filepath.Join(workspaceRoot, "cache", "baselines")),
		Bus:           NewEventBus(),
		BindHost:      "127.0.0.1",
		executions:    make(map[string]*ExecutionRecord),
		reports:       make(map[string]*ReportRecord),
		breakpoints:   make(map[string]BreakpointRule),
	}
}

func (s *Service) allowedRoots() []string {
	roots := []string{
		filepath.Join(s.WorkspaceRoot, "cache"),
		filepath.Join(s.WorkspaceRoot, "cache", "uploads"),
		filepath.Join(s.WorkspaceRoot, "cache", "staged_files"),
		filepath.Join(s.WorkspaceRoot, "downloads"),
		filepath.Join(s.WorkspaceRoot, "tools"),
		filepath.Join(s.WorkspaceRoot, "fixtures"),
	}
	if extra := strings.TrimSpace(os.Getenv("NANO_SCAN_DIRS")); extra != "" {
		for _, d := range filepath.SplitList(extra) {
			if d = strings.TrimSpace(d); d != "" {
				roots = append(roots, d)
			}
		}
	}
	return roots
}

// MountRoutes attaches all /api/v1/* HTTP and WebSocket endpoints onto mux.
func (s *Service) MountRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/api/v1/projects", s.handleProjects)
	mux.HandleFunc("/api/v1/apks", s.handleAPKs)
	mux.HandleFunc("/api/v1/apks/", s.handleAPKDispatch)
	mux.HandleFunc("/api/v1/executions", s.handleExecutions)
	mux.HandleFunc("/api/v1/executions/", s.handleExecutionByID)
	mux.HandleFunc("/api/v1/tests", s.handleTestsList)
	mux.HandleFunc("/api/v1/tests/", s.handleTestRun)
	mux.HandleFunc("/api/v1/reports", s.handleReportsList)
	mux.HandleFunc("/api/v1/reports/", s.handleReportByID)
	mux.HandleFunc("/api/v1/visual/baselines", s.handleVisualBaselines)
	mux.HandleFunc("/api/v1/visual/compare", s.handleVisualCompare)
	mux.HandleFunc("/api/v1/visual/approve", s.handleVisualApprove)
	mux.HandleFunc("/api/v1/devices", s.handleDevices)
	mux.HandleFunc("/api/v1/workers", s.handleWorkers)
	mux.HandleFunc("/api/v1/policies", s.handlePolicies)
	mux.HandleFunc("/api/v1/git", s.handleGit)
	mux.HandleFunc("/api/v1/ws", s.Bus.HandleWebSocket)
	s.mountPhase5Routes(mux)
	s.mountPhase6Routes(mux)
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Service) recordAudit(action, resource, decision, reason string, meta map[string]any) {
	if s.Ledger != nil {
		_, _ = s.Ledger.Record("studio", "api_v1", action, resource, decision, reason, meta)
	}
}

func (s *Service) storeReport(kind, title, status string, payload any) *ReportRecord {
	id := fmt.Sprintf("rep_%d_%d", time.Now().Unix(), s.seq.Add(1))
	rec := &ReportRecord{
		ID:         id,
		Kind:       kind,
		Title:      title,
		Status:     status,
		CreatedUTC: time.Now().UTC().Format(time.RFC3339),
		Payload:    payload,
	}
	s.mu.Lock()
	s.reports[id] = rec
	s.reportOrder = append(s.reportOrder, id)
	s.mu.Unlock()
	s.Bus.Broadcast("REPORT_CREATED", map[string]any{"id": id, "kind": kind, "title": title, "status": status})
	return rec
}

func (s *Service) discoverArtifacts() []ArtifactEntry {
	seenPath := map[string]bool{}
	seenHash := map[string]bool{}
	var out []ArtifactEntry
	scan := []string{
		filepath.Join(s.WorkspaceRoot, "fixtures", "apks"),
		filepath.Join(s.WorkspaceRoot, "downloads"),
		filepath.Join(s.WorkspaceRoot, "tools"),
		filepath.Join(s.WorkspaceRoot, "cache", "uploads"),
	}
	for _, dir := range scan {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			low := strings.ToLower(e.Name())
			ext := filepath.Ext(low)
			isToolJar := strings.Contains(dir, string(filepath.Separator)+"tools") && (strings.HasPrefix(low, "asm") || strings.Contains(low, "sqlite") || strings.Contains(low, "junit"))
			if ext != ".apk" && ext != ".aab" && ext != ".apks" && ext != ".xapk" && (ext != ".jar" || isToolJar) {
				continue
			}
			full := filepath.Join(dir, e.Name())
			abs, _ := filepath.Abs(full)
			key := strings.ToLower(abs)
			if seenPath[key] {
				continue
			}
			seenPath[key] = true
			info, statErr := e.Info()
			if statErr != nil || info.Size() == 0 {
				continue
			}
			sumHex := shortFileSHA(abs)
			// Avoid duplicate entries when an uploaded APK matches an existing workspace file byte-for-byte.
			if sumHex != "unreadable" && seenHash[sumHex] {
				continue
			}
			seenHash[sumHex] = true
			out = append(out, ArtifactEntry{
				ID:        sumHex,
				Name:      e.Name(),
				Path:      abs,
				SizeBytes: info.Size(),
				Kind:      strings.TrimPrefix(ext, "."),
				SHA256:    sumHex,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func shortFileSHA(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "unreadable"
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

func (s *Service) resolveArtifactByID(id string) (ArtifactEntry, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ArtifactEntry{}, fmt.Errorf("missing artifact id")
	}
	arts := s.discoverArtifacts()
	for _, a := range arts {
		if a.ID == id || strings.EqualFold(a.Name, id) {
			return a, nil
		}
	}
	// Try direct contained path fallback
	contained, err := pathcontainment.ResolveExistingContained(id, s.allowedRoots(), []string{".apk", ".jar", ".aab", ".apks", ".xapk"})
	if err != nil {
		return ArtifactEntry{}, fmt.Errorf("BLOCKED_POLICY artifact %q not found inside workspace: %w", id, err)
	}
	st, _ := os.Stat(contained)
	return ArtifactEntry{
		ID:        shortFileSHA(contained),
		Name:      filepath.Base(contained),
		Path:      contained,
		SizeBytes: st.Size(),
		Kind:      strings.TrimPrefix(strings.ToLower(filepath.Ext(contained)), "."),
	}, nil
}

func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	_, javaErr := exec.LookPath("java")
	_, adbErr := exec.LookPath("adb")
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"api_version":    "v1",
		"loopback_only":  httpauth.IsLoopbackHost(s.BindHost),
		"java_available": javaErr == nil,
		"adb_available":  adbErr == nil,
		"last_launch":    s.Coord.GetLastLaunch(),
		"device_status":  s.Coord.GetDeviceStatus(),
	})
}

func (s *Service) handleProjects(w http.ResponseWriter, r *http.Request) {
	var manifests, gradleFiles []string
	_ = filepath.WalkDir(s.WorkspaceRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "bin" || d.Name() == "build" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		name := d.Name()
		if name == "AndroidManifest.xml" {
			manifests = append(manifests, p)
		} else if name == "build.gradle" || name == "build.gradle.kts" || name == "settings.gradle" || name == "settings.gradle.kts" {
			gradleFiles = append(gradleFiles, p)
		}
		return nil
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_root": s.WorkspaceRoot,
		"manifests":      manifests,
		"gradle_files":   gradleFiles,
		"artifacts":      s.discoverArtifacts(),
	})
}

func (s *Service) handleAPKs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"items": s.discoverArtifacts(),
	})
}

func (s *Service) handleAPKDispatch(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/apks/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) < 2 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "expected /api/v1/apks/{id}/{inspect|compatibility}"})
		return
	}
	art, err := s.resolveArtifactByID(parts[0])
	if err != nil {
		status := http.StatusNotFound
		if strings.Contains(err.Error(), "BLOCKED_POLICY") {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}

	bundletoolJar := filepath.Join(s.WorkspaceRoot, "tools", "libs", "bundletool-all-1.18.1.jar")
	eng := ingest.NewEngine(filepath.Join(s.WorkspaceRoot, "cache", "apk_intel"), bundletoolJar)
	rep, aErr := eng.Analyze(art.Path)
	if aErr != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": aErr.Error()})
		return
	}

	switch parts[1] {
	case "inspect":
		secRep := security.AuditAPK(rep)
		sarifBytes, _ := secRep.ExportSARIF()
		s.storeReport("apk_inspect", "Inspect "+art.Name, "ok", map[string]any{"artifact": art, "intel": rep, "security": secRep})
		writeJSON(w, http.StatusOK, map[string]any{
			"artifact": art,
			"intel":    rep,
			"security": secRep,
			"sarif":    string(sarifBytes),
		})
	case "compatibility":
		mode := r.URL.Query().Get("mode")
		rec := registry.EvaluateAPK(rep, mode)
		dbPath := filepath.Join(s.WorkspaceRoot, "cache", "compatibility.db")
		var heatmap any
		if store, sErr := intel.OpenStore(dbPath); sErr == nil {
			_ = store.SeedDefaults()
			if hm, hErr := store.QueryHeatmap(); hErr == nil {
				heatmap = hm
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"artifact":       art,
			"recommendation": rec,
			"heatmap":        heatmap,
		})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown apk action " + parts[1]})
	}
}

func (s *Service) handleExecutions(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.RLock()
		items := make([]*ExecutionRecord, 0, len(s.execOrder))
		for i := len(s.execOrder) - 1; i >= 0; i-- {
			items = append(items, s.executions[s.execOrder[i]])
		}
		s.mu.RUnlock()
		writeJSON(w, http.StatusOK, map[string]any{"executions": items, "last_launch": s.Coord.GetLastLaunch()})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "GET or POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Artifact   string `json:"artifact"`
		Activity   string `json:"activity"`
		ClearState bool   `json:"clear_state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Artifact) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing 'artifact' path"})
		return
	}
	contained, cErr := pathcontainment.ResolveExistingContained(req.Artifact, s.allowedRoots(), []string{".apk", ".jar"})
	if cErr != nil {
		s.recordAudit("v1_launch", req.Artifact, "DENY", cErr.Error(), nil)
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "BLOCKED_POLICY: " + cErr.Error()})
		return
	}
	if _, valErr := archive.ValidateArchive(contained, archive.DefaultLimits()); valErr != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": valErr.Error()})
		return
	}
	launchRes, lErr := s.Coord.LaunchApp(r.Context(), contained, req.Activity, req.ClearState)
	if lErr != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": lErr.Error()})
		return
	}
	id := fmt.Sprintf("exec_%d_%d", time.Now().Unix(), s.seq.Add(1))
	rec := &ExecutionRecord{
		ID:         id,
		Artifact:   contained,
		PackageID:  launchRes.AppID,
		Activity:   launchRes.Activity,
		Backend:    registry.BackendNanoJVM,
		StartedUTC: time.Now().UTC().Format(time.RFC3339),
		Status:     launchRes.ResultStatus,
		Launch:     launchRes,
	}
	s.mu.Lock()
	s.executions[id] = rec
	s.execOrder = append(s.execOrder, id)
	s.mu.Unlock()
	s.recordAudit("v1_launch", filepath.Base(contained), "ALLOW", rec.Status, map[string]any{"execution_id": id})
	s.Bus.Broadcast("EXECUTION_STARTED", rec)
	writeJSON(w, http.StatusCreated, rec)
}

func (s *Service) handleExecutionByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/executions/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	id := parts[0]

	s.mu.RLock()
	rec, ok := s.executions[id]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "execution not found: " + id})
		return
	}

	if len(parts) == 2 && parts[1] == "events" {
		var tl []mcp_compiler.HostTimelineEvent
		if br := s.Coord.Bridge(); br != nil {
			tl, _ = br.InspectTimeline()
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"execution_id": id,
			"backend":      rec.Backend,
			"events":       tl,
			"bus_events":   s.Bus.Recent(),
		})
		return
	}
	if len(parts) == 2 && parts[1] == "snapshot" && r.Method == http.MethodPost {
		var req struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Name == "" {
			req.Name = "snap_" + id
		}
		if err := s.Coord.SaveSnapshot(req.Name); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
			return
		}
		s.Bus.Broadcast("SNAPSHOT_SAVED", map[string]any{"execution_id": id, "name": req.Name})
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "snapshot": req.Name})
		return
	}
	if len(parts) == 2 && parts[1] == "restore" && r.Method == http.MethodPost {
		var req struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Name == "" {
			req.Name = "snap_" + id
		}
		if err := s.Coord.RestoreSnapshot(req.Name); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
			return
		}
		s.Bus.Broadcast("SNAPSHOT_RESTORED", map[string]any{"execution_id": id, "name": req.Name})
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "restored": req.Name})
		return
	}
	if len(parts) == 2 && parts[1] == "process_death" && r.Method == http.MethodPost {
		br := s.Coord.Bridge()
		if br == nil {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "no active headless bridge"})
			return
		}
		if err := br.SimulateProcessDeath(); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
			return
		}
		s.Bus.Broadcast("PROCESS_DEATH_RESTORED", map[string]any{"execution_id": id})
		obs, _ := s.Coord.ObserveScreen(r.Context(), true, "png", 0)
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "observe": obs})
		return
	}
	if len(parts) == 2 && parts[1] == "breakpoint" {
		if r.Method == http.MethodPost {
			var bp BreakpointRule
			if err := json.NewDecoder(r.Body).Decode(&bp); err != nil || bp.Target == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid breakpoint payload"})
				return
			}
			if bp.ID == "" {
				bp.ID = bp.Kind + ":" + bp.Target
			}
			s.mu.Lock()
			s.breakpoints[bp.ID] = bp
			s.mu.Unlock()
			s.Bus.Broadcast("BREAKPOINT_UPDATED", bp)
		}
		s.mu.RLock()
		list := make([]BreakpointRule, 0, len(s.breakpoints))
		for _, v := range s.breakpoints {
			list = append(list, v)
		}
		s.mu.RUnlock()
		writeJSON(w, http.StatusOK, map[string]any{"breakpoints": list})
		return
	}

	obs, _ := s.Coord.ObserveScreen(r.Context(), false, "", 0)
	var metrics *mcp_compiler.RuntimeMetrics
	var threads *mcp_compiler.HostThreadReport
	var storage *mcp_compiler.HostStorageSnapshot
	if br := s.Coord.Bridge(); br != nil {
		metrics, _ = br.InspectRuntime()
		threads, _ = br.InspectThreads()
		storage, _ = br.InspectStorage()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"execution": rec,
		"observe":   obs,
		"metrics":   metrics,
		"threads":   threads,
		"storage":   storage,
	})
}

func (s *Service) handleTestsList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"labs":      labs.ListLabs(),
		"scenarios": labs.StressScenarios23(),
	})
}

func (s *Service) handleTestRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/tests/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) < 2 || parts[1] != "run" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "expected POST /api/v1/tests/{id}/run"})
		return
	}
	testID := parts[0]

	var req struct {
		Artifact  string `json:"artifact"`
		ProfileID string `json:"profile_id"`
		Activity  string `json:"activity"`
		Mode      string `json:"mode"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Artifact == "" {
		if last := s.Coord.GetLastLaunch(); last != nil && last.ApkPath != "" {
			req.Artifact = last.ApkPath
		} else {
			req.Artifact = filepath.Join(s.WorkspaceRoot, "tools", "prism-v.jar")
		}
	}

	if testID == "suite" {
		orch := orchestrator.New(s.WorkspaceRoot)
		rep, err := orch.RunSuite(r.Context(), suite.Suite{
			Name:     "StudioSuiteRun",
			Artifact: req.Artifact,
			Mode:     req.Mode,
			Cases:    []suite.TestCase{{Name: "SmokeObserve", ProfileID: req.ProfileID, Activity: req.Activity, Steps: []suite.Step{{Action: "observe"}}}},
		})
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
			return
		}
		rec := s.storeReport("suite", rep.SuiteName, rep.OverallStatus, rep)
		writeJSON(w, http.StatusOK, map[string]any{"report": rec, "result": rep})
		return
	}

	labRep, err := labs.RunLab(r.Context(), s.WorkspaceRoot, testID, req.Artifact, map[string]string{
		"profile_id": req.ProfileID,
		"activity":   req.Activity,
	})
	if err != nil {
		status := http.StatusUnprocessableEntity
		if strings.Contains(err.Error(), "BLOCKED_POLICY") {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]any{"error": err.Error(), "result": labRep})
		return
	}
	rec := s.storeReport("lab_"+testID, labRep.LabName, string(labRep.Status), labRep)
	s.Bus.Broadcast("LAB_COMPLETED", map[string]any{"lab_id": testID, "status": labRep.Status, "report_id": rec.ID})
	writeJSON(w, http.StatusOK, map[string]any{"report": rec, "result": labRep})
}

func (s *Service) handleReportsList(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	items := make([]*ReportRecord, 0, len(s.reportOrder))
	for i := len(s.reportOrder) - 1; i >= 0; i-- {
		items = append(items, s.reports[s.reportOrder[i]])
	}
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{"reports": items})
}

func (s *Service) handleReportByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/reports/"), "/")
	parts := strings.Split(rest, "/")
	id := parts[0]
	s.mu.RLock()
	rec, ok := s.reports[id]
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "report not found: " + id})
		return
	}
	if format := strings.ToLower(r.URL.Query().Get("format")); format != "" {
		if ur, ok := rec.Payload.(*reporting.UnifiedReport); ok {
			if s.exportUnifiedReportFormat(w, rec, ur, format) {
				return
			}
		}
		if format == "junit" {
			if sr, ok := rec.Payload.(*suite.SuiteReport); ok {
				if xmlBytes, err := sr.ExportJUnitXML(); err == nil {
					w.Header().Set("Content-Type", "application/xml; charset=utf-8")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(xmlBytes)
					return
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *Service) handleVisualBaselines(w http.ResponseWriter, r *http.Request) {
	list, err := s.Baselines.ListBaselines()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"baselines": list})
}

func (s *Service) handleVisualApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Name        string                `json:"name"`
		ProfileID   string                `json:"profile_id"`
		ApprovedBy  string                `json:"approved_by"`
		ImageB64PNG string                `json:"image_b64_png,omitempty"`
		AST         *accessibility.UINode `json:"ast,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if req.ProfileID == "" {
		req.ProfileID = "clean_install"
	}
	var pngBytes []byte
	if req.ImageB64PNG != "" {
		rawB64 := strings.TrimPrefix(req.ImageB64PNG, "data:image/png;base64,")
		decoded, decErr := base64.StdEncoding.DecodeString(rawB64)
		if decErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid base64 png"})
			return
		}
		pngBytes = decoded
	} else {
		_, _ = s.Coord.ObserveScreen(r.Context(), true, "png", 0)
		b, err := os.ReadFile(s.Coord.RenderPath())
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "no rendered screen png available to approve"})
			return
		}
		pngBytes = b
	}
	meta, err := s.Baselines.ApproveBaseline(req.Name, req.ProfileID, req.ApprovedBy, pngBytes, req.AST)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.recordAudit("visual_approve", meta.ProfileID+"/"+meta.Name, "ALLOW", "approved_by="+meta.ApprovedBy, nil)
	s.Bus.Broadcast("VISUAL_BASELINE_APPROVED", meta)
	writeJSON(w, http.StatusOK, meta)
}

func (s *Service) handleVisualCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Name         string                `json:"name"`
		ProfileID    string                `json:"profile_id"`
		Mode         visual.ComparisonMode `json:"mode"`
		CandidateB64 string                `json:"candidate_b64_png,omitempty"`
		CandidateAST *accessibility.UINode `json:"candidate_ast,omitempty"`
		MaskRegions  []visual.MaskRect     `json:"mask_regions,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if req.ProfileID == "" {
		req.ProfileID = "clean_install"
	}
	basePNG, baseMeta, err := s.Baselines.LoadBaseline(req.Name, req.ProfileID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "baseline not found: " + err.Error()})
		return
	}
	var candPNG []byte
	if req.CandidateB64 != "" {
		rawB64 := strings.TrimPrefix(req.CandidateB64, "data:image/png;base64,")
		decoded, decErr := base64.StdEncoding.DecodeString(rawB64)
		if decErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid candidate base64 png"})
			return
		}
		candPNG = decoded
	} else {
		_, _ = s.Coord.ObserveScreen(r.Context(), true, "png", 0)
		candPNG, err = os.ReadFile(s.Coord.RenderPath())
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "no live candidate screenshot"})
			return
		}
	}
	var baseAST *accessibility.UINode
	if baseMeta != nil {
		baseAST = baseMeta.AST
	}
	rep, cmpErr := visual.Compare(basePNG, candPNG, baseAST, req.CandidateAST, visual.CompareOptions{
		Mode:             req.Mode,
		MaskRegions:      req.MaskRegions,
		IncludeImagesB64: true,
	})
	if cmpErr != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": cmpErr.Error()})
		return
	}
	s.storeReport("visual_compare", fmt.Sprintf("Visual %s (%s)", req.Name, rep.Mode), fmt.Sprintf("passed=%v", rep.Passed), rep)
	s.Bus.Broadcast("VISUAL_DIFF_COMPLETED", map[string]any{"name": req.Name, "mode": rep.Mode, "passed": rep.Passed})
	writeJSON(w, http.StatusOK, rep)
}

func (s *Service) handleDevices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"device_status": s.Coord.GetDeviceStatus(),
		"profiles":      profile.ReusableProfiles(),
	})
}

func (s *Service) handleWorkers(w http.ResponseWriter, r *http.Request) {
	var metrics *mcp_compiler.RuntimeMetrics
	var threads *mcp_compiler.HostThreadReport
	var storage *mcp_compiler.HostStorageSnapshot
	var snap *jvm.Snapshot
	if br := s.Coord.Bridge(); br != nil {
		metrics, _ = br.InspectRuntime()
		threads, _ = br.InspectThreads()
		storage, _ = br.InspectStorage()
		snap, _ = jvm.CaptureSnapshot(br)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"execution_mode": s.Coord.GetExecutionMode(),
		"metrics":        metrics,
		"threads":        threads,
		"storage":        storage,
		"jvm_snapshot":   snap,
	})
}

func (s *Service) handlePolicies(w http.ResponseWriter, r *http.Request) {
	valid := true
	errMsg := ""
	entriesCount := 0
	var entries []map[string]any
	if s.Ledger != nil && s.Ledger.FilePath() != "" {
		if n, err := audit.VerifyChain(s.Ledger.FilePath()); err != nil && !os.IsNotExist(err) {
			valid = false
			errMsg = err.Error()
		} else {
			entriesCount = n
		}
		if raw, rErr := os.ReadFile(s.Ledger.FilePath()); rErr == nil {
			lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
			start := 0
			if len(lines) > 25 {
				start = len(lines) - 25
			}
			for _, ln := range lines[start:] {
				var obj map[string]any
				if json.Unmarshal([]byte(strings.TrimSpace(ln)), &obj) == nil {
					entries = append(entries, obj)
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"guard_active":      s.Guard != nil,
		"audit_chain_valid": valid,
		"verified_entries":  entriesCount,
		"audit_chain_error": errMsg,
		"recent_entries":    entries,
		"loopback_only":     httpauth.IsLoopbackHost(s.BindHost),
	})
}

func (s *Service) handleGit(w http.ResponseWriter, r *http.Request) {
	gitExe, err := exec.LookPath("git")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	cctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	branchOut, _ := exec.CommandContext(cctx, gitExe, "-C", s.WorkspaceRoot, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	statusOut, _ := exec.CommandContext(cctx, gitExe, "-C", s.WorkspaceRoot, "status", "--short").CombinedOutput()
	writeJSON(w, http.StatusOK, map[string]any{
		"available": true,
		"branch":    strings.TrimSpace(string(branchOut)),
		"status":    strings.TrimSpace(string(statusOut)),
	})
}
