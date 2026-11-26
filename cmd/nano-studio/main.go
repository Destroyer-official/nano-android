package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"nano_android/engine/api"
	mcp "nano_android/engine/mcp_compiler"
	"nano_android/engine/security/archive"
	"nano_android/engine/security/audit"
	"nano_android/engine/security/httpauth"
	"nano_android/engine/security/pathcontainment"
	"nano_android/engine/security/policy"
	"nano_android/studio/frontend"
)

type AppOption struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	Kind      string `json:"kind"` // "apk", "jar", "uploaded"
}

type RecordedAction struct {
	Timestamp string  `json:"timestamp"`
	Type      string  `json:"type"`
	Target    string  `json:"target,omitempty"`
	Text      string  `json:"text,omitempty"`
	X         float64 `json:"x,omitempty"`
	Y         float64 `json:"y,omitempty"`
	StartX    float64 `json:"start_x,omitempty"`
	StartY    float64 `json:"start_y,omitempty"`
	EndX      float64 `json:"end_x,omitempty"`
	EndY      float64 `json:"end_y,omitempty"`
	Direction string  `json:"direction,omitempty"`
	Status    string  `json:"status"`
	Message   string  `json:"message"`
}

type StudioServer struct {
	coord     *mcp.IntegratedCoordinator
	mu        sync.Mutex
	actions   []RecordedAction
	uploads   string
	maestroMu sync.Mutex
	guard     *policy.Guard
	ledger    *audit.Ledger
	apiSvc    *api.Service
	bindHost  string
}

func NewStudioServer() *StudioServer {
	uploadDir := filepath.Join("cache", "uploads")
	_ = os.MkdirAll(uploadDir, 0750)
	_ = os.MkdirAll(filepath.Join("cache", "staged_files"), 0750)
	wd, _ := os.Getwd()
	coord := mcp.NewIntegratedCoordinator()
	guard := policy.NewGuard(policy.DefaultConfig())
	ledger, _ := audit.OpenLedger(filepath.Join("cache", "audit", "studio_audit.jsonl"))
	return &StudioServer{
		coord:   coord,
		uploads: uploadDir,
		guard:   guard,
		ledger:  ledger,
		apiSvc:  api.NewService(wd, coord, guard, ledger),
	}
}

func (s *StudioServer) allowedWorkspaceRoots() []string {
	roots := []string{
		s.uploads,
		filepath.Join("cache", "staged_files"),
		"cache",
		"downloads",
		"tools",
		"fixtures",
	}
	if extraDirs := strings.TrimSpace(os.Getenv("NANO_SCAN_DIRS")); extraDirs != "" {
		for _, dir := range filepath.SplitList(extraDirs) {
			if dir = strings.TrimSpace(dir); dir != "" {
				roots = append(roots, dir)
			}
		}
	}
	return roots
}

func (s *StudioServer) auditEvent(action, resource, decision, reason string, meta map[string]any) {
	if s.ledger == nil {
		return
	}
	_, _ = s.ledger.Record("studio", "http_client", action, resource, decision, reason, meta)
}

func (s *StudioServer) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *StudioServer) discoverApps() []AppOption {
	seen := make(map[string]bool)
	seenSize := make(map[string]bool)
	var out []AppOption

	addFile := func(path, kind, labelPrefix string) {
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		norm := strings.ToLower(abs)
		if seen[norm] {
			return
		}
		st, err := os.Stat(abs)
		if err != nil || st.IsDir() || st.Size() == 0 {
			return
		}
		seen[norm] = true
		base := filepath.Base(abs)
		dedupKey := fmt.Sprintf("%s:%d", strings.ToLower(base), st.Size())
		if labelPrefix == "" || labelPrefix == "[Uploaded] " {
			if seenSize[dedupKey] {
				return
			}
			seenSize[dedupKey] = true
		}
		out = append(out, AppOption{
			Name:      labelPrefix + base,
			Path:      abs,
			SizeBytes: st.Size(),
			Kind:      kind,
		})
	}

	// Currently active app
	if last := s.coord.GetLastLaunch(); last != nil && last.ApkPath != "" {
		addFile(last.ApkPath, "apk", "[Active] ")
	}

	// Uploaded user APKs
	if entries, err := os.ReadDir(s.uploads); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				addFile(filepath.Join(s.uploads, e.Name()), "uploaded", "[Uploaded] ")
			}
		}
	}

	// Downloads & workspace APKs
	scanDirs := []struct {
		dir    string
		kind   string
		prefix string
	}{
		{"downloads", "apk", ""},
		{"tools", "jar", ""},
	}
	if extraDirs := strings.TrimSpace(os.Getenv("NANO_SCAN_DIRS")); extraDirs != "" {
		for _, dir := range filepath.SplitList(extraDirs) {
			if dir = strings.TrimSpace(dir); dir != "" {
				scanDirs = append(scanDirs, struct {
					dir    string
					kind   string
					prefix string
				}{dir, "apk", ""})
			}
		}
	}
	for _, sd := range scanDirs {
		entries, err := os.ReadDir(sd.dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			low := strings.ToLower(e.Name())
			if strings.HasSuffix(low, ".apk") {
				addFile(filepath.Join(sd.dir, e.Name()), "apk", sd.prefix)
			} else if sd.kind == "jar" && strings.HasSuffix(low, ".jar") && !strings.HasPrefix(low, "asm") {
				addFile(filepath.Join(sd.dir, e.Name()), "jar", sd.prefix)
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (s *StudioServer) handleApps(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{
		"apps":          s.discoverApps(),
		"last_launch":   s.coord.GetLastLaunch(),
		"device_status": s.coord.GetDeviceStatus(),
	})
}

func (s *StudioServer) buildFullSnapshot(ctx context.Context) map[string]any {
	obs, obsErr := s.coord.ObserveScreen(ctx, false, "", 0)
	insp, _ := s.coord.InspectState(ctx, "memory")
	layouts, _ := s.coord.ListLayouts()
	devStatus := s.coord.GetDeviceStatus()
	last := s.coord.GetLastLaunch()

	s.mu.Lock()
	acts := make([]RecordedAction, len(s.actions))
	copy(acts, s.actions)
	s.mu.Unlock()

	resp := map[string]any{
		"observe":       obs,
		"inspect":       insp,
		"layouts":       layouts,
		"device_status": devStatus,
		"last_launch":   last,
		"staged_files":  s.coord.ListStagedFiles(),
		"actions":       acts,
	}
	if obsErr != nil {
		resp["observe_error"] = obsErr.Error()
	}
	return resp
}

func (s *StudioServer) handleObserve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if r.URL.Query().Get("refresh") == "1" {
		_, _ = s.coord.ObserveScreen(ctx, true, "png", 0)
	}
	s.writeJSON(w, http.StatusOK, s.buildFullSnapshot(ctx))
}

func (s *StudioServer) handleScreenPNG(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Content-Type", "image/png")

	if r.URL.Query().Get("refresh") == "1" {
		if obs, err := s.coord.ObserveScreen(r.Context(), true, "png", 0); err == nil && obs != nil && obs.Screenshot != nil && obs.Screenshot.SizeBytes > 0 {
			// File was freshly written to s.coord.RenderPath()
		}
	}

	if data, err := os.ReadFile(s.coord.RenderPath()); err == nil && len(data) > 0 {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}

	// Dynamic blank PNG placeholder if no APK has rendered a frame yet
	img := image.NewRGBA(image.Rect(0, 0, 1080, 2400))
	bg := color.RGBA{R: 15, G: 23, B: 42, A: 255}
	for y := 0; y < 2400; y++ {
		for x := 0; x < 1080; x++ {
			img.SetRGBA(x, y, bg)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func (s *StudioServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	// Limit upload to 300 MB
	r.Body = http.MaxBytesReader(w, r.Body, 300<<20)
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("multipart parse failed: %v", err)})
		return
	}

	file, header, err := r.FormFile("apk")
	if err != nil {
		file, header, err = r.FormFile("file")
	}
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing form field 'apk' or 'file'"})
		return
	}
	defer file.Close()

	base := filepath.Base(header.Filename)
	low := strings.ToLower(base)
	if !strings.HasSuffix(low, ".apk") && !strings.HasSuffix(low, ".jar") {
		s.auditEvent("upload", base, "DENY", "invalid extension", nil)
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "only .apk or .jar files are supported"})
		return
	}

	safeName, sErr := pathcontainment.SanitizeFilename(base)
	if sErr != nil {
		s.auditEvent("upload", base, "DENY", sErr.Error(), nil)
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("invalid filename: %v", sErr)})
		return
	}

	destPath, cErr := pathcontainment.ResolveWriteContained(s.uploads, safeName, []string{".apk", ".jar"})
	if cErr != nil {
		s.auditEvent("upload", safeName, "DENY", cErr.Error(), nil)
		s.writeJSON(w, http.StatusForbidden, map[string]any{"error": fmt.Sprintf("path containment error: %v", cErr)})
		return
	}

	outFile, err := os.Create(destPath)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if _, err := io.Copy(outFile, file); err != nil {
		outFile.Close()
		_ = os.Remove(destPath)
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	outFile.Close()

	// P0: Validate archive before booting isolated worker.
	if _, valErr := archive.ValidateArchive(destPath, archive.DefaultLimits()); valErr != nil {
		_ = os.Remove(destPath)
		s.auditEvent("upload", safeName, "DENY", valErr.Error(), nil)
		s.writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error": fmt.Sprintf("archive security validation failed: %v", valErr),
		})
		return
	}

	activity := strings.TrimSpace(r.FormValue("activity"))
	clearState := r.FormValue("clear_state") == "true"

	launchRes, lErr := s.coord.LaunchApp(r.Context(), destPath, activity, clearState)
	if lErr != nil {
		s.auditEvent("upload_launch", safeName, "ERROR", lErr.Error(), nil)
		s.writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":       lErr.Error(),
			"uploaded_to": destPath,
		})
		return
	}
	s.auditEvent("upload_launch", safeName, "ALLOW", launchRes.Status, map[string]any{"app_id": launchRes.AppID})

	s.mu.Lock()
	s.actions = []RecordedAction{{
		Timestamp: time.Now().Format("15:04:05"),
		Type:      "install_launch",
		Target:    base,
		Status:    launchRes.Status,
		Message:   fmt.Sprintf("Installed %s -> %s (%d ms)", base, launchRes.Activity, launchRes.LaunchDurationMs),
	}}
	s.mu.Unlock()

	snap := s.buildFullSnapshot(r.Context())
	snap["launch"] = launchRes
	snap["apps"] = s.discoverApps()
	s.writeJSON(w, http.StatusOK, snap)
}

func (s *StudioServer) handleLaunch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		App        string `json:"app"`
		Activity   string `json:"activity"`
		ClearState bool   `json:"clear_state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.App) == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing 'app' APK/JAR path"})
		return
	}

	containedApp, cErr := pathcontainment.ResolveExistingContained(req.App, s.allowedWorkspaceRoots(), []string{".apk", ".jar"})
	if cErr != nil {
		s.auditEvent("launch", req.App, "DENY", cErr.Error(), nil)
		s.writeJSON(w, http.StatusForbidden, map[string]any{"error": fmt.Sprintf("launch path blocked by containment policy: %v", cErr)})
		return
	}
	if _, valErr := archive.ValidateArchive(containedApp, archive.DefaultLimits()); valErr != nil {
		s.auditEvent("launch", filepath.Base(containedApp), "DENY", valErr.Error(), nil)
		s.writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": fmt.Sprintf("archive validation failed: %v", valErr)})
		return
	}

	launchRes, err := s.coord.LaunchApp(r.Context(), containedApp, req.Activity, req.ClearState)
	if err != nil {
		s.auditEvent("launch", filepath.Base(containedApp), "ERROR", err.Error(), nil)
		s.writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	s.auditEvent("launch", filepath.Base(containedApp), "ALLOW", launchRes.Status, map[string]any{"app_id": launchRes.AppID})

	s.mu.Lock()
	s.actions = []RecordedAction{{
		Timestamp: time.Now().Format("15:04:05"),
		Type:      "launch",
		Target:    filepath.Base(req.App),
		Status:    launchRes.Status,
		Message:   fmt.Sprintf("Launched %s (%s) in %d ms", filepath.Base(req.App), launchRes.Activity, launchRes.LaunchDurationMs),
	}}
	s.mu.Unlock()
	if s.apiSvc != nil {
		s.apiSvc.Bus.Broadcast("STUDIO_LAUNCH", map[string]any{"app": filepath.Base(req.App), "activity": launchRes.Activity})
	}

	snap := s.buildFullSnapshot(r.Context())
	snap["launch"] = launchRes
	snap["apps"] = s.discoverApps()
	s.writeJSON(w, http.StatusOK, snap)
}

func (s *StudioServer) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req mcp.Action
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	res, err := s.coord.DispatchAction(r.Context(), req)
	if err != nil {
		s.writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}

	// Step virtual looper slightly so delayed handlers/animations settle cleanly before screenshot
	_, _ = s.coord.StepTime(r.Context(), 120)

	targetDesc := req.NodeID
	if targetDesc == "" && req.Text != "" && !strings.EqualFold(req.Type, "type") {
		targetDesc = req.Text
	} else if targetDesc == "" && (req.X != 0 || req.Y != 0) {
		targetDesc = fmt.Sprintf("(%.0f,%.0f)", req.X, req.Y)
	} else if targetDesc == "" && req.Direction != "" {
		targetDesc = req.Direction
	}

	s.mu.Lock()
	s.actions = append(s.actions, RecordedAction{
		Timestamp: time.Now().Format("15:04:05"),
		Type:      req.Type,
		Target:    targetDesc,
		Text:      req.Text,
		X:         req.X,
		Y:         req.Y,
		StartX:    req.StartX,
		StartY:    req.StartY,
		EndX:      req.EndX,
		EndY:      req.EndY,
		Direction: req.Direction,
		Status:    res.Status,
		Message:   res.Message,
	})
	s.mu.Unlock()
	if s.apiSvc != nil {
		s.apiSvc.Bus.Broadcast("STUDIO_ACTION", map[string]any{"type": req.Type, "target": targetDesc, "status": res.Status})
	}

	snap := s.buildFullSnapshot(r.Context())
	snap["action_result"] = res
	s.writeJSON(w, http.StatusOK, snap)
}

func (s *StudioServer) handleStep(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		DeltaMs int64 `json:"delta_ms"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.DeltaMs <= 0 {
		req.DeltaMs = 300
	}

	stepRes, err := s.coord.StepTime(r.Context(), req.DeltaMs)
	if err != nil {
		s.writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}

	s.mu.Lock()
	s.actions = append(s.actions, RecordedAction{
		Timestamp: time.Now().Format("15:04:05"),
		Type:      "step",
		Status:    "success",
		Message:   fmt.Sprintf("Stepped +%dms (%d tasks executed)", req.DeltaMs, stepRes.TasksExecuted),
	})
	s.mu.Unlock()

	snap := s.buildFullSnapshot(r.Context())
	snap["step"] = stepRes
	s.writeJSON(w, http.StatusOK, snap)
}

func (s *StudioServer) handleInspect(w http.ResponseWriter, r *http.Request) {
	res, err := s.coord.InspectState(r.Context(), "memory")
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, res)
}

func (s *StudioServer) handleLayouts(w http.ResponseWriter, r *http.Request) {
	layouts, err := s.coord.ListLayouts()
	if err != nil {
		s.writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"layouts": layouts})
}

func (s *StudioServer) handlePreviewLayout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Layout string `json:"layout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Layout) == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing 'layout' name or resource ID"})
		return
	}

	obs, err := s.coord.PreviewLayout(req.Layout)
	if err != nil {
		s.writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}

	s.mu.Lock()
	s.actions = append(s.actions, RecordedAction{
		Timestamp: time.Now().Format("15:04:05"),
		Type:      "preview_layout",
		Target:    req.Layout,
		Status:    "success",
		Message:   fmt.Sprintf("Inflated AXML layout res/layout/%s", req.Layout),
	})
	s.mu.Unlock()

	snap := s.buildFullSnapshot(r.Context())
	snap["observe"] = obs
	s.writeJSON(w, http.StatusOK, snap)
}

func (s *StudioServer) handleDevices(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, s.coord.GetDeviceStatus())
}

func (s *StudioServer) handleFiles(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{"files": s.coord.ListStagedFiles()})
}

func (s *StudioServer) handleStageFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, 500<<20)
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing form field 'file'"})
			return
		}
		defer file.Close()
		stagedDir := filepath.Join("cache", "staged_files")
		safeName, sErr := pathcontainment.SanitizeFilename(header.Filename)
		if sErr != nil {
			s.auditEvent("stage_file", header.Filename, "DENY", sErr.Error(), nil)
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": sErr.Error()})
			return
		}
		destPath, cErr := pathcontainment.ResolveWriteContained(stagedDir, safeName, nil)
		if cErr != nil {
			s.auditEvent("stage_file", safeName, "DENY", cErr.Error(), nil)
			s.writeJSON(w, http.StatusForbidden, map[string]any{"error": cErr.Error()})
			return
		}
		outF, err := os.Create(destPath)
		if err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		_, cpErr := io.Copy(outF, file)
		outF.Close()
		if cpErr != nil {
			_ = os.Remove(destPath)
			s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": cpErr.Error()})
			return
		}
		info, err := s.coord.StageFile(destPath)
		if err != nil {
			s.auditEvent("stage_file", safeName, "ERROR", err.Error(), nil)
			s.writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
			return
		}
		s.auditEvent("stage_file", safeName, "ALLOW", "staged", nil)
		snap := s.buildFullSnapshot(r.Context())
		snap["staged"] = info
		s.writeJSON(w, http.StatusOK, snap)
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Path) == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing 'path' or multipart 'file'"})
		return
	}
	containedStagePath, cErr := pathcontainment.ResolveExistingContained(req.Path, s.allowedWorkspaceRoots(), nil)
	if cErr != nil {
		s.auditEvent("stage_file", req.Path, "DENY", cErr.Error(), nil)
		s.writeJSON(w, http.StatusForbidden, map[string]any{"error": fmt.Sprintf("stage path denied: %v", cErr)})
		return
	}
	info, err := s.coord.StageFile(containedStagePath)
	if err != nil {
		s.auditEvent("stage_file", filepath.Base(containedStagePath), "ERROR", err.Error(), nil)
		s.writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	s.auditEvent("stage_file", filepath.Base(containedStagePath), "ALLOW", "staged", nil)
	snap := s.buildFullSnapshot(r.Context())
	snap["staged"] = info
	s.writeJSON(w, http.StatusOK, snap)
}

func (s *StudioServer) handleMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := s.coord.SetExecutionMode(req.Mode); err != nil {
		s.writeJSON(w, http.StatusConflict, map[string]any{
			"error":         err.Error(),
			"device_status": s.coord.GetDeviceStatus(),
		})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"execution_mode": s.coord.GetExecutionMode(),
		"device_status":  s.coord.GetDeviceStatus(),
	})
}

func (s *StudioServer) handleExportMaestro(w http.ResponseWriter, r *http.Request) {
	last := s.coord.GetLastLaunch()
	pkg := "com.example.app"
	if last != nil && last.AppID != "" {
		pkg = last.AppID
	}

	s.mu.Lock()
	acts := make([]RecordedAction, len(s.actions))
	copy(acts, s.actions)
	s.mu.Unlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("appId: %s\n", pkg))
	sb.WriteString("---\n")
	sb.WriteString("- repeat:\n")
	sb.WriteString("    times: 3\n")
	sb.WriteString("    commands:\n")
	sb.WriteString("      - launchApp:\n")
	sb.WriteString("          clearState: true\n")

	for _, a := range acts {
		switch strings.ToLower(a.Type) {
		case "tap", "click":
			if a.Text != "" {
				sb.WriteString(fmt.Sprintf("      - tapOn: %q\n", a.Text))
			} else if a.Target != "" && !strings.HasPrefix(a.Target, "(") && !strings.HasPrefix(a.Target, "node_") {
				sb.WriteString(fmt.Sprintf("      - tapOn:\n          id: %q\n", a.Target))
			} else if a.X > 0 || a.Y > 0 {
				px := int((a.X / 1080.0) * 100.0)
				py := int((a.Y / 2400.0) * 100.0)
				sb.WriteString(fmt.Sprintf("      - tapOn:\n          point: \"%d%%,%d%%\"\n", px, py))
			}
		case "type":
			if a.Text != "" {
				sb.WriteString(fmt.Sprintf("      - inputText: %q\n", a.Text))
			}
		case "scroll", "swipe":
			if a.StartX != 0 || a.StartY != 0 || a.EndX != 0 || a.EndY != 0 {
				sx := int((a.StartX / 1080.0) * 100.0)
				sy := int((a.StartY / 2400.0) * 100.0)
				ex := int((a.EndX / 1080.0) * 100.0)
				ey := int((a.EndY / 2400.0) * 100.0)
				sb.WriteString(fmt.Sprintf("      - swipe:\n          start: \"%d%%,%d%%\"\n          end: \"%d%%,%d%%\"\n", sx, sy, ex, ey))
			} else {
				sb.WriteString("      - scroll\n")
			}
		case "back":
			sb.WriteString("      - pressKey: back\n")
		}
	}

	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_flow.yaml\"", pkg))
	_, _ = w.Write([]byte(sb.String()))
}

func (s *StudioServer) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/favicon.ico" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	htmlBytes, err := frontend.FS.ReadFile("index.html")
	if err != nil {
		http.Error(w, "embedded studio frontend missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(htmlBytes)
}

func (s *StudioServer) mountRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleHome)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(frontend.FS))))
	mux.HandleFunc("/api/apps", s.handleApps)
	mux.HandleFunc("/api/upload", s.handleUpload)
	mux.HandleFunc("/api/launch", s.handleLaunch)
	mux.HandleFunc("/api/observe", s.handleObserve)
	mux.HandleFunc("/api/screen.png", s.handleScreenPNG)
	mux.HandleFunc("/api/action", s.handleAction)
	mux.HandleFunc("/api/step", s.handleStep)
	mux.HandleFunc("/api/inspect", s.handleInspect)
	mux.HandleFunc("/api/layouts", s.handleLayouts)
	mux.HandleFunc("/api/preview_layout", s.handlePreviewLayout)
	mux.HandleFunc("/api/devices", s.handleDevices)
	mux.HandleFunc("/api/files", s.handleFiles)
	mux.HandleFunc("/api/stage_file", s.handleStageFile)
	mux.HandleFunc("/api/mode", s.handleMode)
	mux.HandleFunc("/api/export_maestro", s.handleExportMaestro)
	mux.HandleFunc("/api/security", s.handleSecurityStatus)
	if s.apiSvc != nil {
		s.apiSvc.MountRoutes(mux)
	}
}

func (s *StudioServer) handleSecurityStatus(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{
		"auth_enforced":       true,
		"origin_enforced":     true,
		"loopback_only":       httpauth.IsLoopbackHost(s.bindHost),
		"path_containment":    true,
		"archive_validation":  true,
		"os_worker_isolation": true,
		"audit_ledger":        "cache/audit/studio_audit.jsonl",
	})
}

func main() {
	host := flag.String("host", "127.0.0.1", "Bind host address (default localhost-only for security)")
	port := flag.Int("port", 8088, "Port to serve Nano-Android Studio")
	allowRemoteAuth := flag.Bool("allow-remote-auth", false, "Allow binding outside loopback when TLS + auth token are configured")
	tlsCert := flag.String("tls-cert", "", "Path to TLS certificate PEM for HTTPS")
	tlsKey := flag.String("tls-key", "", "Path to TLS private key PEM for HTTPS")
	flag.Parse()

	authToken, tokErr := httpauth.ResolveAuthToken()
	if tokErr != nil {
		log.Fatalf("security bootstrap error: %v", tokErr)
	}
	tlsEnabled := strings.TrimSpace(*tlsCert) != "" && strings.TrimSpace(*tlsKey) != ""
	authCfg := httpauth.Config{
		BindHost:        *host,
		AuthToken:       authToken,
		AllowRemoteAuth: *allowRemoteAuth,
		TLSEnabled:      tlsEnabled,
		PublicGetPaths: map[string]bool{
			"/":                  true,
			"/static/styles.css": true,
			"/static/app.js":     true,
		},
	}
	if err := httpauth.ValidateBindConfig(authCfg); err != nil {
		log.Fatalf("refusing insecure server configuration: %v", err)
	}

	srv := NewStudioServer()
	srv.bindHost = *host
	if srv.apiSvc != nil {
		srv.apiSvc.BindHost = *host
	}
	defer srv.coord.Close()

	mux := http.NewServeMux()
	srv.mountRoutes(mux)

	addr := net.JoinHostPort(*host, fmt.Sprintf("%d", *port))
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           httpauth.Middleware(authCfg, mux),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Printf("Shutting down Nano-Android Studio...")
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	scheme := "http"
	if tlsEnabled {
		scheme = "https"
	}
	log.Printf("Nano-Android Studio listening on %s://%s/?token=%s", scheme, addr, authToken)
	var serveErr error
	if tlsEnabled {
		serveErr = httpSrv.ListenAndServeTLS(*tlsCert, *tlsKey)
	} else {
		serveErr = httpSrv.ListenAndServe()
	}
	if serveErr != nil && serveErr != http.ErrServerClosed {
		log.Fatalf("studio server error: %v", serveErr)
	}
}

