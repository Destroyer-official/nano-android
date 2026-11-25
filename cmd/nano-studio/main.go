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
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	mcp "nano_android/engine/mcp_compiler"
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
}

func NewStudioServer() *StudioServer {
	uploadDir := filepath.Join("cache", "uploads")
	_ = os.MkdirAll(uploadDir, 0755)
	return &StudioServer{
		coord:   mcp.NewIntegratedCoordinator(),
		uploads: uploadDir,
	}
}

func (s *StudioServer) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *StudioServer) discoverApps() []AppOption {
	seen := make(map[string]bool)
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
		{`C:\Users\des\OneDrive\Desktop\video_compression\release`, "apk", ""},
		{"tools", "jar", ""},
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
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "only .apk or .jar files are supported"})
		return
	}

	safeName := strings.Map(func(c rune) rune {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_' {
			return c
		}
		return '_'
	}, base)

	destPath := filepath.Join(s.uploads, safeName)
	outFile, err := os.Create(destPath)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if _, err := io.Copy(outFile, file); err != nil {
		outFile.Close()
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	outFile.Close()

	activity := strings.TrimSpace(r.FormValue("activity"))
	clearState := r.FormValue("clear_state") == "true"

	launchRes, lErr := s.coord.LaunchApp(r.Context(), destPath, activity, clearState)
	if lErr != nil {
		s.writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":       lErr.Error(),
			"uploaded_to": destPath,
		})
		return
	}

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

	launchRes, err := s.coord.LaunchApp(r.Context(), req.App, req.Activity, req.ClearState)
	if err != nil {
		s.writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}

	s.mu.Lock()
	s.actions = []RecordedAction{{
		Timestamp: time.Now().Format("15:04:05"),
		Type:      "launch",
		Target:    filepath.Base(req.App),
		Status:    launchRes.Status,
		Message:   fmt.Sprintf("Launched %s (%s) in %d ms", filepath.Base(req.App), launchRes.Activity, launchRes.LaunchDurationMs),
	}}
	s.mu.Unlock()

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
		_ = os.MkdirAll(stagedDir, 0755)
		safeName := strings.Map(func(c rune) rune {
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_' {
				return c
			}
			return '_'
		}, filepath.Base(header.Filename))
		destPath := filepath.Join(stagedDir, safeName)
		outF, err := os.Create(destPath)
		if err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		_, cpErr := io.Copy(outF, file)
		outF.Close()
		if cpErr != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": cpErr.Error()})
			return
		}
		info, err := s.coord.StageFile(destPath)
		if err != nil {
			s.writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
			return
		}
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
	info, err := s.coord.StageFile(req.Path)
	if err != nil {
		s.writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(studioHTML))
}

func main() {
	port := flag.Int("port", 8088, "Port to serve Nano-Android Studio")
	flag.Parse()

	srv := NewStudioServer()
	defer srv.coord.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handleHome)
	mux.HandleFunc("/api/apps", srv.handleApps)
	mux.HandleFunc("/api/upload", srv.handleUpload)
	mux.HandleFunc("/api/launch", srv.handleLaunch)
	mux.HandleFunc("/api/observe", srv.handleObserve)
	mux.HandleFunc("/api/screen.png", srv.handleScreenPNG)
	mux.HandleFunc("/api/action", srv.handleAction)
	mux.HandleFunc("/api/step", srv.handleStep)
	mux.HandleFunc("/api/inspect", srv.handleInspect)
	mux.HandleFunc("/api/layouts", srv.handleLayouts)
	mux.HandleFunc("/api/preview_layout", srv.handlePreviewLayout)
	mux.HandleFunc("/api/devices", srv.handleDevices)
	mux.HandleFunc("/api/files", srv.handleFiles)
	mux.HandleFunc("/api/stage_file", srv.handleStageFile)
	mux.HandleFunc("/api/mode", srv.handleMode)
	mux.HandleFunc("/api/export_maestro", srv.handleExportMaestro)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Nano-Android Universal APK Studio listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

const studioHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8" />
<meta name="viewport" content="width=device-width, initial-scale=1.0" />
<title>Nano-Android Studio — Universal APK &amp; AI Agent Inspector</title>
<style>
  :root {
    --bg: #090d16;
    --panel: #111827;
    --panel2: #0f172a;
    --border: #1e293b;
    --text: #e2e8f0;
    --muted: #94a3b8;
    --accent: #38bdf8;
    --green: #22c55e;
    --amber: #f59e0b;
    --red: #ef4444;
    --purple: #a78bfa;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    background: var(--bg);
    color: var(--text);
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Inter, sans-serif;
    height: 100vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  header {
    background: var(--panel);
    border-bottom: 1px solid var(--border);
    padding: 10px 16px;
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    align-items: center;
    justify-content: space-between;
    z-index: 20;
  }
  .brand { display: flex; align-items: center; gap: 10px; font-weight: 700; font-size: 15px; letter-spacing: 0.3px; }
  .brand .dot { width: 10px; height: 10px; border-radius: 50%; background: var(--green); box-shadow: 0 0 8px var(--green); }
  .brand span.sub { font-weight: 500; font-size: 12px; color: var(--muted); }
  .installer-bar {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
    flex: 1;
    justify-content: center;
  }
  select, input[type="text"] {
    background: #0b1220;
    color: var(--text);
    border: 1px solid #334155;
    border-radius: 6px;
    padding: 6px 10px;
    font-size: 12px;
    outline: none;
  }
  select:focus, input[type="text"]:focus { border-color: var(--accent); }
  .btn {
    background: #1e293b;
    color: var(--text);
    border: 1px solid #334155;
    border-radius: 6px;
    padding: 6px 12px;
    font-size: 12px;
    font-weight: 600;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    gap: 5px;
    transition: background 0.15s, border-color 0.15s;
  }
  .btn:hover { background: #334155; border-color: var(--accent); }
  .btn.primary { background: #0284c7; border-color: #38bdf8; color: #fff; }
  .btn.primary:hover { background: #0369a1; }
  .btn.upload-btn { background: #4c1d95; border-color: #8b5cf6; color: #fff; }
  .btn.upload-btn:hover { background: #5b21b6; }
  .mode-switch {
    display: flex;
    background: #0b1220;
    border: 1px solid #334155;
    border-radius: 8px;
    overflow: hidden;
  }
  .mode-pill {
    padding: 6px 11px;
    font-size: 11px;
    font-weight: 600;
    border: none;
    background: transparent;
    color: var(--muted);
    cursor: pointer;
  }
  .mode-pill.active { background: #0284c7; color: #fff; }

  main {
    flex: 1;
    display: grid;
    grid-template-columns: 390px 1fr 390px;
    gap: 12px;
    padding: 12px;
    min-height: 0;
  }
  .col {
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: 10px;
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
  }
  .col-head {
    padding: 10px 14px;
    border-bottom: 1px solid var(--border);
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 12px;
    font-weight: 700;
    color: #f8fafc;
    background: #0f172a;
  }
  .badge {
    font-size: 11px;
    font-weight: 600;
    padding: 2px 8px;
    border-radius: 999px;
    background: #1e293b;
    color: var(--accent);
    border: 1px solid #334155;
  }
  .badge.ok { color: #4ade80; border-color: #166534; background: #052e16; }
  .badge.partial { color: #fbbf24; border-color: #92400e; background: #451a03; }
  .badge.error { color: #f87171; border-color: #991b1b; background: #450a0a; }

  /* Phone Stage */
  .device-stage {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 10px;
    background: #060911;
    overflow: hidden;
    position: relative;
  }
  .phone-bezel {
    position: relative;
    height: calc(100vh - 235px);
    max-height: 760px;
    aspect-ratio: 1080 / 2400;
    border-radius: 26px;
    border: 4px solid #334155;
    box-shadow: 0 20px 50px rgba(0,0,0,0.75);
    overflow: hidden;
    background: #000;
    cursor: crosshair;
    user-select: none;
  }
  .phone-bezel img {
    width: 100%;
    height: 100%;
    display: block;
    object-fit: contain;
    pointer-events: none;
  }
  .hover-box {
    position: absolute;
    border: 2px solid #38bdf8;
    background: rgba(56, 189, 248, 0.14);
    pointer-events: none;
    display: none;
    z-index: 5;
  }
  .hover-tip {
    position: absolute;
    bottom: 8px;
    left: 8px;
    right: 8px;
    background: rgba(15, 23, 42, 0.92);
    border: 1px solid #38bdf8;
    color: #f8fafc;
    font-size: 11px;
    padding: 5px 8px;
    border-radius: 6px;
    pointer-events: none;
    display: none;
    z-index: 6;
    font-family: ui-monospace, monospace;
  }
  .controls-bar {
    padding: 8px 10px;
    border-top: 1px solid var(--border);
    background: var(--panel2);
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .controls-row {
    display: flex;
    gap: 5px;
    flex-wrap: wrap;
    align-items: center;
  }
  .controls-row .btn { padding: 5px 8px; font-size: 11px; flex: 1; justify-content: center; }

  /* Tabs inside middle/right panels */
  .tabs {
    display: flex;
    border-bottom: 1px solid var(--border);
    background: #0b1220;
  }
  .tab-btn {
    flex: 1;
    background: transparent;
    color: var(--muted);
    border: none;
    padding: 8px 10px;
    font-size: 12px;
    font-weight: 600;
    cursor: pointer;
    border-bottom: 2px solid transparent;
  }
  .tab-btn.active { color: var(--accent); border-bottom-color: var(--accent); background: #111827; }
  .scroll-pane {
    flex: 1;
    overflow: auto;
    padding: 10px 12px;
    font-size: 12px;
  }
  .filter-bar {
    display: flex;
    gap: 6px;
    padding: 8px 12px;
    border-bottom: 1px solid var(--border);
    align-items: center;
  }
  .ast-node {
    padding: 4px 7px;
    margin: 2px 0;
    border-radius: 5px;
    border-left: 2px solid #334155;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 11.5px;
    cursor: pointer;
  }
  .ast-node:hover { background: #1e293b; border-left-color: var(--accent); }
  .ast-node.clickable { border-left-color: var(--green); }
  .tag-cls { color: #38bdf8; font-weight: 600; }
  .tag-txt { color: #f8fafc; }
  .tag-id { color: #a78bfa; }
  .tag-bnd { color: #64748b; font-size: 10.5px; }

  .layout-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 7px 10px;
    border: 1px solid var(--border);
    border-radius: 6px;
    margin-bottom: 6px;
    cursor: pointer;
    background: #0b1220;
  }
  .layout-item:hover { border-color: var(--accent); background: #172033; }

  .meta-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 6px;
    margin-bottom: 10px;
  }
  .meta-card {
    background: #0b1220;
    border: 1px solid var(--border);
    border-radius: 7px;
    padding: 7px 9px;
  }
  .meta-card .lbl { font-size: 10px; text-transform: uppercase; color: var(--muted); }
  .meta-card .val { font-size: 12.5px; font-weight: 600; color: #f8fafc; word-break: break-all; margin-top: 2px; }

  pre.trace-box {
    background: #060911;
    border: 1px solid #334155;
    border-radius: 6px;
    padding: 8px;
    font-family: ui-monospace, monospace;
    font-size: 11px;
    color: #fca5a5;
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 220px;
    overflow: auto;
    margin-top: 6px;
  }
  .warn-box {
    background: #2b1804;
    border: 1px solid #92400e;
    color: #fcd34d;
    border-radius: 6px;
    padding: 7px 9px;
    font-size: 11.5px;
    margin-top: 6px;
    white-space: pre-wrap;
    word-break: break-word;
  }
  .log-entry {
    border-bottom: 1px solid var(--border);
    padding: 6px 0;
    font-size: 11.5px;
  }
  .drop-overlay {
    display: none;
    position: fixed;
    inset: 0;
    background: rgba(2, 132, 199, 0.25);
    border: 3px dashed #38bdf8;
    z-index: 100;
    align-items: center;
    justify-content: center;
    font-size: 22px;
    font-weight: 700;
    color: #fff;
    backdrop-filter: blur(4px);
  }
  .drop-overlay.show { display: flex; }
  .banner-toast {
    display: none;
    background: #0284c7;
    color: #fff;
    padding: 6px 14px;
    font-size: 12px;
    font-weight: 600;
    text-align: center;
  }
  .banner-toast.show { display: block; }
  .banner-toast.err { background: #b91c1c; }
</style>
</head>
<body>

<div id="dropOverlay" class="drop-overlay">Drop any Android .apk or .jar to Install &amp; Render Live UI</div>
<div id="toast" class="banner-toast"></div>

<header>
  <div class="brand">
    <div class="dot"></div>
    <div>Nano-Android Studio <span class="sub">Universal AI-Era APK Micro-Runtime</span></div>
  </div>

  <div class="installer-bar">
    <input type="file" id="fileInput" accept=".apk,.jar" style="display:none" />
    <button class="btn upload-btn" id="uploadTrigger">+ Upload Any .APK</button>
    <select id="appSelect" style="max-width:230px"></select>
    <select id="activitySelect" style="max-width:230px" title="Activity from AndroidManifest.xml">
      <option value="auto">Auto (Launcher Activity)</option>
    </select>
    <input type="text" id="customPath" placeholder="Or paste C:\path\to\app.apk..." style="width:210px" />
    <button class="btn primary" id="launchBtn">Install &amp; Boot</button>
  </div>

  <div style="display:flex;align-items:center;gap:8px;">
    <div class="mode-switch">
      <button class="mode-pill active" id="modeJvm" data-mode="nano_jvm">⚡ Nano JVM</button>
      <button class="mode-pill" id="modeAdb" data-mode="adb_device">📱 ADB Device</button>
    </div>
    <span id="deviceBadge" class="badge" title="">Probing...</span>
  </div>
</header>

<main>
  <!-- LEFT COLUMN: Live Device Viewport -->
  <section class="col">
    <div class="col-head">
      <span>LIVE APK VIEWPORT (1080×2400)</span>
      <span id="screenTitleBadge" class="badge">Idle</span>
    </div>
    <div class="device-stage">
      <div class="phone-bezel" id="phoneBezel">
        <img id="screenImg" src="/api/screen.png" alt="Live APK Framebuffer" />
        <div id="hoverBox" class="hover-box"></div>
        <div id="hoverTip" class="hover-tip"></div>
      </div>
    </div>
    <div class="controls-bar">
      <div class="controls-row">
        <input type="text" id="typeInput" placeholder="Text to send to focused EditText (Enter sends)..." style="flex:1;min-width:140px;" />
        <button class="btn" id="sendTypeBtn">Type</button>
        <input type="file" id="stageFileInput" style="display:none" />
        <button class="btn" id="btnStageFile" title="Stage media or document file for Android file pickers">📎 Stage File</button>
      </div>
      <div class="controls-row">
        <button class="btn" id="btnBack">◀ Back</button>
        <button class="btn" id="btnScrollUp">▲ Up</button>
        <button class="btn" id="btnScrollDown">▼ Down</button>
        <button class="btn" id="btnStep">⏱ +300ms</button>
        <button class="btn" id="btnRefresh">🔄 Frame</button>
      </div>
    </div>
  </section>

  <!-- MIDDLE COLUMN: AI Agent Semantic View & Layout Explorer -->
  <section class="col">
    <div class="col-head">
      <span>WHAT THE AI AGENT SEES</span>
      <span id="astTelemetry" class="badge">0 nodes</span>
    </div>
    <div class="tabs">
      <button class="tab-btn active" data-midtab="ast">Live View AST</button>
      <button class="tab-btn" data-midtab="json">Raw MCP JSON</button>
      <button class="tab-btn" data-midtab="layouts">APK Layouts (<span id="layoutCount">0</span>)</button>
    </div>
    <div class="filter-bar" id="astFilterBar">
      <input type="text" id="astSearch" placeholder="Filter nodes by text, id, class..." style="flex:1" />
      <label style="font-size:11px;display:flex;align-items:center;gap:4px;color:var(--muted);cursor:pointer;">
        <input type="checkbox" id="clickableOnly" /> Clickable only
      </label>
    </div>
    <div class="scroll-pane" id="midPaneAst"></div>
    <div class="scroll-pane" id="midPaneJson" style="display:none;"><pre id="rawJsonPre" style="font-family:monospace;font-size:11px;white-space:pre-wrap;color:#93c5fd;"></pre></div>
    <div class="scroll-pane" id="midPaneLayouts" style="display:none;"></div>
  </section>

  <!-- RIGHT COLUMN: Diagnostics, Telemetry & Maestro -->
  <section class="col">
    <div class="col-head">
      <span>APK INSPECTOR &amp; DIAGNOSTICS</span>
      <span id="launchStatusBadge" class="badge">—</span>
    </div>
    <div class="tabs">
      <button class="tab-btn active" data-righttab="diag">Manifest &amp; Errors</button>
      <button class="tab-btn" data-righttab="logs">Action Trace (<span id="logCount">0</span>)</button>
      <button class="tab-btn" data-righttab="maestro">Maestro Flow</button>
    </div>
    <div class="scroll-pane" id="rightPaneDiag"></div>
    <div class="scroll-pane" id="rightPaneLogs" style="display:none;"></div>
    <div class="scroll-pane" id="rightPaneMaestro" style="display:none;">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;">
        <span style="color:var(--muted);font-size:11px;">Real recorded session replay (` + "`repeat_count: 3`" + `)</span>
        <a class="btn primary" href="/api/export_maestro" target="_blank">Download YAML</a>
      </div>
      <pre id="maestroPre" style="font-family:monospace;font-size:11px;white-space:pre-wrap;color:#86efac;background:#060911;padding:8px;border-radius:6px;border:1px solid var(--border);"></pre>
    </div>
  </section>
</main>

<script>
let currentState = null;
let astFlatNodes = [];
let busy = false;

function showToast(msg, isErr) {
  const el = document.getElementById("toast");
  el.textContent = msg;
  el.className = "banner-toast show" + (isErr ? " err" : "");
  clearTimeout(window._toastTimer);
  window._toastTimer = setTimeout(() => { el.className = "banner-toast"; }, 5500);
}

function refreshImage() {
  document.getElementById("screenImg").src = "/api/screen.png?t=" + Date.now();
}

function escapeHtml(s) {
  return String(s || "").replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;");
}

function flattenAst(node, depth, out) {
  if (!node) return;
  out.push({ node, depth });
  if (node.children) {
    for (const c of node.children) flattenAst(c, depth + 1, out);
  }
}

function findDeepestHit(node, x, y) {
  if (!node) return null;
  const b = node.bounds || [0,0,0,0];
  if (x < b[0] || x > b[2] || y < b[1] || y > b[3]) return null;
  if (node.children) {
    for (let i = node.children.length - 1; i >= 0; i--) {
      const h = findDeepestHit(node.children[i], x, y);
      if (h) return h;
    }
  }
  return node;
}

function highlightNodeOnPhone(node) {
  const box = document.getElementById("hoverBox");
  const tip = document.getElementById("hoverTip");
  if (!node || !node.bounds) {
    box.style.display = "none";
    tip.style.display = "none";
    return;
  }
  const [l, t, r, b] = node.bounds;
  if (r <= l || b <= t) {
    box.style.display = "none";
    tip.style.display = "none";
    return;
  }
  box.style.display = "block";
  box.style.left = (l / 1080 * 100) + "%";
  box.style.top = (t / 2400 * 100) + "%";
  box.style.width = ((r - l) / 1080 * 100) + "%";
  box.style.height = ((b - t) / 2400 * 100) + "%";

  tip.style.display = "block";
  tip.textContent = (node.class || "View") +
    (node.resource_id ? " #" + node.resource_id : "") +
    (node.text ? ' "' + node.text.slice(0, 40) + '"' : "") +
    " [" + l + "," + t + "," + r + "," + b + "]" +
    (node.clickable ? " (clickable)" : "");
}

function renderASTTree() {
  const container = document.getElementById("midPaneAst");
  const obs = currentState && currentState.observe;
  if (!obs || !obs.ast) {
    container.innerHTML = "<div style='color:var(--muted)'>No active View tree. Upload or launch an APK above, or open the APK Layouts tab to inspect AXML layouts directly.</div>";
    document.getElementById("astTelemetry").textContent = "0 nodes";
    return;
  }

  astFlatNodes = [];
  flattenAst(obs.ast, 0, astFlatNodes);
  const jsonStr = JSON.stringify(obs.ast, null, 2);
  document.getElementById("rawJsonPre").textContent = jsonStr;
  const estTokens = Math.max(1, Math.round(jsonStr.length / 4));
  document.getElementById("astTelemetry").textContent = astFlatNodes.length + " nodes · ~" + estTokens + " tok (vs ~15k img)";

  const q = document.getElementById("astSearch").value.trim().toLowerCase();
  const onlyClick = document.getElementById("clickableOnly").checked;

  let html = "";
  for (const item of astFlatNodes) {
    const n = item.node;
    if (onlyClick && !n.clickable) continue;
    const text = (n.text || n.content_description || "");
    const hay = ((n.class||"") + " " + (n.resource_id||"") + " " + text).toLowerCase();
    if (q && !hay.includes(q)) continue;

    const pad = onlyClick || q ? 6 : Math.min(item.depth * 14, 160);
    const b = n.bounds || [0,0,0,0];
    html += '<div class="ast-node ' + (n.clickable ? 'clickable' : '') + '" style="margin-left:' + pad + 'px" data-nodeid="' + escapeHtml(n.id) + '">' +
      '<span class="tag-cls">' + escapeHtml(n.class || 'View') + '</span> ' +
      (n.resource_id ? '<span class="tag-id">#' + escapeHtml(n.resource_id) + '</span> ' : '') +
      (text ? '<span class="tag-txt">"' + escapeHtml(text) + '"</span> ' : '') +
      '<span class="tag-bnd">[' + b.join(',') + ']</span>' +
      '</div>';
  }
  container.innerHTML = html || "<div style='color:var(--muted)'>No nodes match current filter.</div>";

  container.querySelectorAll(".ast-node").forEach(el => {
    const nid = el.getAttribute("data-nodeid");
    const found = astFlatNodes.find(x => x.node.id === nid);
    if (!found) return;
    el.addEventListener("mouseenter", () => highlightNodeOnPhone(found.node));
    el.addEventListener("mouseleave", () => highlightNodeOnPhone(null));
    el.addEventListener("click", () => dispatchAction({ type: "tap", node_id: nid }));
  });
}

function renderLayouts() {
  const pane = document.getElementById("midPaneLayouts");
  const layouts = (currentState && currentState.layouts) || [];
  document.getElementById("layoutCount").textContent = layouts.length;
  if (!layouts.length) {
    pane.innerHTML = "<div style='color:var(--muted)'>No compiled AXML layouts detected (raw .jar without APK resources, or no APK launched).</div>";
    return;
  }
  let html = "<div style='color:var(--muted);margin-bottom:8px;font-size:11px;'>Click any compiled XML layout from resources.arsc to inflate &amp; render its exact layout on the phone viewport:</div>";
  for (const l of layouts) {
    html += '<div class="layout-item" data-layout="' + escapeHtml(l.name) + '">' +
      '<div><strong>' + escapeHtml(l.name) + '</strong><div style="color:var(--muted);font-size:10.5px;">' + escapeHtml(l.path) + '</div></div>' +
      '<span class="badge">Preview</span>' +
      '</div>';
  }
  pane.innerHTML = html;
  pane.querySelectorAll(".layout-item").forEach(el => {
    el.addEventListener("click", () => previewLayout(el.getAttribute("data-layout")));
  });
}

function renderDiagnostics() {
  const pane = document.getElementById("rightPaneDiag");
  const last = (currentState && currentState.last_launch) || {};
  const insp = (currentState && currentState.inspect) || {};
  const heap = insp.heap || {};
  const dev = (currentState && currentState.device_status) || {};

  const statusBadge = document.getElementById("launchStatusBadge");
  const st = (last.status || "idle").toLowerCase();
  statusBadge.textContent = st.toUpperCase();
  statusBadge.className = "badge " + (st === "ok" || st === "resumed" ? "ok" : (st === "partial" ? "partial" : (st === "error" ? "error" : "")));

  const obs = currentState && currentState.observe;
  const activeActName = (obs && obs.active_activity) || last.activity || "";
  const dlgNote = (obs && obs.active_dialog) ? (" · Dialog: " + obs.active_dialog) : "";
  document.getElementById("screenTitleBadge").textContent = ((obs && obs.current_screen) || activeActName || "Idle") + dlgNote;

  // Populate Manifest Activities dropdown cleanly
  const actSel = document.getElementById("activitySelect");
  const prevVal = actSel.value;
  let optsHtml = '<option value="auto">Auto (Launcher Activity)</option>';
  if (last.activities && last.activities.length) {
    for (const a of last.activities) {
      const sel = (a === last.activity) ? ' selected' : '';
      optsHtml += '<option value="' + escapeHtml(a) + '"' + sel + '>' + escapeHtml(a) + '</option>';
    }
  } else if (last.activity) {
    optsHtml += '<option value="' + escapeHtml(last.activity) + '" selected>' + escapeHtml(last.activity) + '</option>';
  }
  actSel.innerHTML = optsHtml;
  if (prevVal && Array.from(actSel.options).some(o => o.value === prevVal)) {
    actSel.value = prevVal;
  }

  // Device status pill
  const db = document.getElementById("deviceBadge");
  const activeMode = dev.execution_mode || "nano_jvm";
  document.getElementById("modeJvm").classList.toggle("active", activeMode === "nano_jvm");
  document.getElementById("modeAdb").classList.toggle("active", activeMode === "adb_device");
  if (dev.active_serial) {
    db.textContent = "ADB: " + dev.active_serial;
    db.className = "badge ok";
  } else {
    db.textContent = "0 ADB · " + (dev.hypervisor_ready ? "AEHD Ready" : "No Hypervisor/Device");
    db.className = "badge partial";
  }
  db.title = dev.hypervisor_note || "";

  const stackStr = (obs && obs.activity_stack && obs.activity_stack.length)
    ? obs.activity_stack.map(a => a.split('.').pop()).join(' → ')
    : (activeActName ? activeActName.split('.').pop() : "—");
  const staged = (currentState && currentState.staged_files) || [];

  let html = '<div class="meta-grid">' +
    '<div class="meta-card"><div class="lbl">App / Package</div><div class="val">' + escapeHtml(last.app_label || "—") + ' <span style="color:var(--muted);font-weight:400;">(' + escapeHtml(last.app_id || "—") + ')</span></div></div>' +
    '<div class="meta-card"><div class="lbl">SDK &amp; Startup</div><div class="val">API ' + (last.min_sdk||"?") + '–' + (last.target_sdk||"?") + ' · ' + (last.launch_duration_ms||0) + ' ms</div></div>' +
    '<div class="meta-card"><div class="lbl">Activity Stack</div><div class="val">' + escapeHtml(stackStr) + '</div></div>' +
    '<div class="meta-card"><div class="lbl">Native .so Libs</div><div class="val">' + (last.native_libs || 0) + ' native libs</div></div>' +
    '<div class="meta-card"><div class="lbl">Dialog / Toast</div><div class="val">' + escapeHtml((obs && obs.active_dialog) || (obs && obs.toast) || "None") + '</div></div>' +
    '<div class="meta-card"><div class="lbl">JVM Memory &amp; Clock</div><div class="val">' + (heap.allocated_mb ? heap.allocated_mb.toFixed(1) + ' MB · ' : '') + (heap.objects_live||0) + ' cls · ' + ((obs && obs.virtual_time_ms)||0) + 'ms</div></div>' +
    '</div>';

  html += '<div class="meta-card" style="margin-bottom:8px;"><div class="lbl">Sandbox Input Media / Staged Files (' + staged.length + ')</div>' +
    '<div style="font-size:11px;color:#cbd5e1;margin-top:4px;">' +
    (staged.length ? staged.map(f => '<span class="badge" style="margin:2px 4px 2px 0;display:inline-block;">' + escapeHtml(f.name) + ' (' + Math.max(1, Math.round((f.size_bytes||0)/1024)) + ' KB)</span>').join('') : 'None staged yet. Click 📎 Stage File below viewport.') +
    '</div></div>';

  if (last.warnings && last.warnings.length) {
    for (const w of last.warnings) {
      html += '<div class="warn-box">' + escapeHtml(w) + '</div>';
    }
  }
  if (last.error || last.exception_trace) {
    html += '<div style="margin-top:8px;font-weight:700;color:#f87171;font-size:11.5px;">Honest Runtime Diagnostic Trace (Host JVM Verification):</div>';
    html += '<pre class="trace-box">' + escapeHtml(last.exception_trace || last.error) + '</pre>';
  }
  if (!dev.active_serial && dev.hypervisor_note) {
    html += '<div class="warn-box" style="border-color:#334155;background:#0b1220;color:#94a3b8;"><strong>ADB / Emulator Host Note:</strong> ' + escapeHtml(dev.hypervisor_note) + '</div>';
  }
  pane.innerHTML = html;
}

function renderLogs() {
  const pane = document.getElementById("rightPaneLogs");
  const acts = (currentState && currentState.actions) || [];
  document.getElementById("logCount").textContent = acts.length;
  let html = "";
  for (let i = acts.length - 1; i >= 0; i--) {
    const a = acts[i];
    html += '<div class="log-entry">' +
      '<span style="color:var(--muted)">[' + escapeHtml(a.timestamp) + ']</span> ' +
      '<strong style="color:var(--accent)">' + escapeHtml(a.type) + '</strong> ' +
      (a.target ? '<code>' + escapeHtml(a.target) + '</code> ' : '') +
      '<span class="badge ' + (a.status === "error" || a.status === "failed" ? "error" : "ok") + '">' + escapeHtml(a.status) + '</span>' +
      '<div style="color:#cbd5e1;margin-top:2px;">' + escapeHtml(a.message) + '</div>' +
      '</div>';
  }
  pane.innerHTML = html || "<div style='color:var(--muted)'>No actions recorded yet.</div>";
}

async function syncMaestro() {
  try {
    const r = await fetch("/api/export_maestro");
    document.getElementById("maestroPre").textContent = await r.text();
  } catch (e) {}
}

function applySnapshot(data) {
  currentState = data;
  renderASTTree();
  renderLayouts();
  renderDiagnostics();
  renderLogs();
  refreshImage();
  syncMaestro();
}

async function fetchApps() {
  const r = await fetch("/api/apps");
  const data = await r.json();
  const sel = document.getElementById("appSelect");
  sel.innerHTML = "";
  for (const a of (data.apps || [])) {
    const o = document.createElement("option");
    o.value = a.path;
    o.textContent = a.name + " (" + (a.size_bytes / 1024 / 1024).toFixed(1) + " MB)";
    if (data.last_launch && data.last_launch.apk_path && data.last_launch.apk_path.toLowerCase() === a.path.toLowerCase()) {
      o.selected = true;
    }
    sel.appendChild(o);
  }
}

async function fetchObserve(refresh) {
  const r = await fetch("/api/observe" + (refresh ? "?refresh=1" : ""));
  applySnapshot(await r.json());
}

async function launchSelected() {
  if (busy) return;
  const path = document.getElementById("customPath").value.trim() || document.getElementById("appSelect").value;
  if (!path) return showToast("Please upload or choose an APK/JAR first.", true);
  const act = document.getElementById("activitySelect").value || "auto";
  busy = true;
  showToast("Translating & booting " + path.split(/[\\/]/).pop() + "...", false);
  try {
    const r = await fetch("/api/launch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ app: path, activity: act })
    });
    const d = await r.json();
    if (!r.ok || d.error) {
      showToast("Launch failed: " + (d.error || r.statusText), true);
    } else {
      applySnapshot(d);
      await fetchApps();
      showToast("Launched " + (d.last_launch?.app_label || d.last_launch?.app_id || "APK") + " [" + (d.last_launch?.status||"").toUpperCase() + "]", d.last_launch?.status === "error");
    }
  } finally {
    busy = false;
  }
}

async function uploadApkFile(file) {
  if (!file || busy) return;
  busy = true;
  showToast("Uploading & decompiling " + file.name + " via dex2jar + AAPT2 parser...", false);
  try {
    const fd = new FormData();
    fd.append("apk", file);
    const r = await fetch("/api/upload", { method: "POST", body: fd });
    const d = await r.json();
    if (!r.ok || d.error) {
      showToast("Upload/Launch failed: " + (d.error || r.statusText), true);
    } else {
      document.getElementById("customPath").value = "";
      applySnapshot(d);
      await fetchApps();
      showToast("Installed " + file.name + " (" + (d.last_launch?.status || "ok") + ")", d.last_launch?.status === "error");
    }
  } finally {
    busy = false;
  }
}

async function dispatchAction(payload) {
  if (busy) return;
  busy = true;
  try {
    const r = await fetch("/api/action", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });
    const d = await r.json();
    if (!r.ok || d.error) showToast(d.error || "Action failed", true);
    else applySnapshot(d);
  } finally {
    busy = false;
  }
}

async function previewLayout(layoutName) {
  if (busy) return;
  busy = true;
  try {
    const r = await fetch("/api/preview_layout", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ layout: layoutName })
    });
    const d = await r.json();
    if (!r.ok || d.error) showToast(d.error || "Preview failed", true);
    else {
      applySnapshot(d);
      showToast("Inflated res/layout/" + layoutName, false);
    }
  } finally {
    busy = false;
  }
}

async function switchMode(mode) {
  const r = await fetch("/api/mode", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ mode })
  });
  const d = await r.json();
  if (!r.ok) {
    showToast(d.error || "Cannot switch backend", true);
  } else {
    showToast("Backend switched to " + mode, false);
    await fetchObserve(true);
  }
}

// Event bindings
document.getElementById("uploadTrigger").onclick = () => document.getElementById("fileInput").click();
document.getElementById("fileInput").onchange = (e) => { if (e.target.files[0]) uploadApkFile(e.target.files[0]); };
document.getElementById("launchBtn").onclick = launchSelected;
document.getElementById("btnRefresh").onclick = () => fetchObserve(true);
document.getElementById("btnBack").onclick = () => dispatchAction({ type: "back" });
document.getElementById("btnScrollUp").onclick = () => dispatchAction({ type: "scroll", direction: "UP" });
document.getElementById("btnScrollDown").onclick = () => dispatchAction({ type: "scroll", direction: "DOWN" });
document.getElementById("btnStep").onclick = async () => {
  const r = await fetch("/api/step", { method: "POST", headers: {"Content-Type":"application/json"}, body: JSON.stringify({delta_ms: 300}) });
  applySnapshot(await r.json());
};
document.getElementById("sendTypeBtn").onclick = () => {
  const txt = document.getElementById("typeInput").value;
  if (txt) dispatchAction({ type: "type", text: txt });
};
document.getElementById("typeInput").addEventListener("keydown", (e) => {
  if (e.key === "Enter") {
    const txt = document.getElementById("typeInput").value;
    if (txt) dispatchAction({ type: "type", text: txt + "\n" });
  }
});
document.getElementById("btnStageFile").onclick = () => document.getElementById("stageFileInput").click();
document.getElementById("stageFileInput").onchange = async (e) => {
  const file = e.target.files[0];
  if (!file || busy) return;
  busy = true;
  showToast("Staging input file " + file.name + " into Android sandbox...", false);
  try {
    const fd = new FormData();
    fd.append("file", file);
    const r = await fetch("/api/stage_file", { method: "POST", body: fd });
    const d = await r.json();
    if (!r.ok || d.error) showToast("Stage failed: " + (d.error || r.statusText), true);
    else {
      applySnapshot(d);
      showToast("Staged " + file.name + " for Android file pickers!", false);
    }
  } finally {
    busy = false;
    e.target.value = "";
  }
};
document.getElementById("modeJvm").onclick = () => switchMode("nano_jvm");
document.getElementById("modeAdb").onclick = () => switchMode("adb_device");
document.getElementById("astSearch").oninput = renderASTTree;
document.getElementById("clickableOnly").onchange = renderASTTree;

// Tabs
document.querySelectorAll("[data-midtab]").forEach(btn => {
  btn.onclick = () => {
    document.querySelectorAll("[data-midtab]").forEach(b => b.classList.remove("active"));
    btn.classList.add("active");
    const t = btn.getAttribute("data-midtab");
    document.getElementById("midPaneAst").style.display = t === "ast" ? "" : "none";
    document.getElementById("astFilterBar").style.display = t === "ast" ? "flex" : "none";
    document.getElementById("midPaneJson").style.display = t === "json" ? "" : "none";
    document.getElementById("midPaneLayouts").style.display = t === "layouts" ? "" : "none";
  };
});
document.querySelectorAll("[data-righttab]").forEach(btn => {
  btn.onclick = () => {
    document.querySelectorAll("[data-righttab]").forEach(b => b.classList.remove("active"));
    btn.classList.add("active");
    const t = btn.getAttribute("data-righttab");
    document.getElementById("rightPaneDiag").style.display = t === "diag" ? "" : "none";
    document.getElementById("rightPaneLogs").style.display = t === "logs" ? "" : "none";
    document.getElementById("rightPaneMaestro").style.display = t === "maestro" ? "" : "none";
  };
});

// Interactive 1080x2400 Phone Canvas Hover, Tap/Drag-to-Swipe & Wheel Scroll
const bezel = document.getElementById("phoneBezel");
let pointerStart = null;
function canvasCoords(e) {
  const rect = bezel.getBoundingClientRect();
  return {
    x: Math.max(0, Math.min(1080, Math.round(((e.clientX - rect.left) / rect.width) * 1080))),
    y: Math.max(0, Math.min(2400, Math.round(((e.clientY - rect.top) / rect.height) * 2400)))
  };
}
bezel.addEventListener("mousemove", (e) => {
  if (!currentState || !currentState.observe || !currentState.observe.ast) return;
  const pt = canvasCoords(e);
  highlightNodeOnPhone(findDeepestHit(currentState.observe.ast, pt.x, pt.y));
});
bezel.addEventListener("mouseleave", () => {
  pointerStart = null;
  highlightNodeOnPhone(null);
});
bezel.addEventListener("pointerdown", (e) => {
  pointerStart = canvasCoords(e);
});
bezel.addEventListener("pointerup", (e) => {
  if (!pointerStart) return;
  const end = canvasCoords(e);
  const dist = Math.hypot(end.x - pointerStart.x, end.y - pointerStart.y);
  const start = pointerStart;
  pointerStart = null;
  if (dist >= 18) {
    dispatchAction({ type: "swipe", start_x: start.x, start_y: start.y, end_x: end.x, end_y: end.y });
  } else {
    dispatchAction({ type: "tap", x: end.x, y: end.y });
  }
});
bezel.addEventListener("wheel", (e) => {
  e.preventDefault();
  dispatchAction({ type: "scroll", direction: e.deltaY > 0 ? "DOWN" : "UP" });
}, { passive: false });

// Drag and drop APK support across entire window
window.addEventListener("dragover", (e) => { e.preventDefault(); document.getElementById("dropOverlay").classList.add("show"); });
window.addEventListener("dragleave", (e) => { if (e.relatedTarget === null) document.getElementById("dropOverlay").classList.remove("show"); });
window.addEventListener("drop", (e) => {
  e.preventDefault();
  document.getElementById("dropOverlay").classList.remove("show");
  if (e.dataTransfer.files && e.dataTransfer.files[0]) uploadApkFile(e.dataTransfer.files[0]);
});

fetchApps().then(() => fetchObserve(false));
</script>
</body>
</html>
`
