package mcp_compiler

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LayoutEntry represents one compiled binary AXML layout file inside an APK.
type LayoutEntry struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// LayoutPreviewResult reports the result of inflating a static AXML layout.
type LayoutPreviewResult struct {
	Status string `json:"status"`
	Layout string `json:"layout,omitempty"`
	ResID  int    `json:"res_id,omitempty"`
	Error  string `json:"error,omitempty"`
}

// RuntimeMetrics holds real JVM memory, GC, classloader, and virtual uptime stats from HeadlessAppHost.
type RuntimeMetrics struct {
	UsedMB          float64 `json:"used_mb"`
	FreeMB          float64 `json:"free_mb"`
	MaxMB           float64 `json:"max_mb"`
	GCCycles        int64   `json:"gc_cycles"`
	LoadedClasses   int64   `json:"loaded_classes"`
	TotalViews      int64   `json:"total_views"`
	VirtualUptimeMs int64   `json:"virtual_uptime_ms"`
	Package         string  `json:"package"`
	Activity        string  `json:"activity"`
}

// LiveHeadlessBridge manages the zero-emulator HeadlessAppHost JVM process.
type LiveHeadlessBridge struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	reader  *bufio.Reader
	active  bool
	jarPath string
	apkPath string
}

// NewLiveHeadlessBridge initializes a new headless bridge.
func NewLiveHeadlessBridge() *LiveHeadlessBridge {
	return &LiveHeadlessBridge{}
}

func resolvePath(candidates ...string) string {
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			abs, err := filepath.Abs(p)
			if err == nil {
				return abs
			}
			return p
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}

func resolveCacheDir() string {
	candidates := []string{
		"cache",
		filepath.Join("..", "..", "cache"),
		`d:\code\Main_projects\android_mcp\cache`,
	}
	for _, c := range candidates {
		parent := filepath.Dir(c)
		if info, err := os.Stat(parent); err == nil && info.IsDir() {
			_ = os.MkdirAll(c, 0755)
			abs, err := filepath.Abs(c)
			if err == nil {
				return abs
			}
			return c
		}
	}
	_ = os.MkdirAll(`d:\code\Main_projects\android_mcp\cache`, 0755)
	return `d:\code\Main_projects\android_mcp\cache`
}

func fileSHA256Short(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// PrepareArtifacts resolves an arbitrary .apk or .jar into a runnable bytecode JAR and optional APK resource archive.
// Zero hardcoded fallback to other apps is ever permitted.
func (b *LiveHeadlessBridge) PrepareArtifacts(apkOrJarPath string) (jarPath string, apkPath string, err error) {
	if strings.TrimSpace(apkOrJarPath) == "" {
		return "", "", fmt.Errorf("empty APK or JAR path")
	}

	resolved := resolvePath(
		apkOrJarPath,
		filepath.Join("..", "..", apkOrJarPath),
		filepath.Join(`d:\code\Main_projects\android_mcp`, apkOrJarPath),
	)
	info, statErr := os.Stat(resolved)
	if statErr != nil || info.IsDir() {
		return "", "", fmt.Errorf("APK/JAR not found: %s", apkOrJarPath)
	}

	lower := strings.ToLower(resolved)
	if strings.HasSuffix(lower, ".jar") {
		return resolved, "", nil
	}
	if !strings.HasSuffix(lower, ".apk") {
		return "", "", fmt.Errorf("unsupported file extension (expected .apk or .jar): %s", filepath.Base(resolved))
	}

	hashHex, err := fileSHA256Short(resolved)
	if err != nil {
		return "", resolved, fmt.Errorf("hash apk failed: %w", err)
	}

	cacheDir := resolveCacheDir()
	cachedJar := filepath.Join(cacheDir, hashHex+"_classes.jar")
	if st, err := os.Stat(cachedJar); err == nil && st.Size() > 0 {
		return cachedJar, resolved, nil
	}

	d2j := resolvePath(
		filepath.Join("tools", "dex-tools-v2.4", "d2j-dex2jar.bat"),
		filepath.Join("..", "..", "tools", "dex-tools-v2.4", "d2j-dex2jar.bat"),
		`d:\code\Main_projects\android_mcp\tools\dex-tools-v2.4\d2j-dex2jar.bat`,
	)
	if _, err := os.Stat(d2j); err != nil {
		return "", resolved, fmt.Errorf("dex2jar translator not found at %s", d2j)
	}

	tmpJar := cachedJar + ".tmp"
	_ = os.Remove(tmpJar)
	cmd := exec.Command("cmd", "/C", d2j, "-f", "-o", tmpJar, resolved)
	out, convErr := cmd.CombinedOutput()
	if convErr != nil {
		_ = os.Remove(tmpJar)
		return "", resolved, fmt.Errorf("dex2jar conversion failed for %s: %v (%s)", filepath.Base(resolved), convErr, strings.TrimSpace(string(out)))
	}
	if st, err := os.Stat(tmpJar); err != nil || st.Size() == 0 {
		_ = os.Remove(tmpJar)
		return "", resolved, fmt.Errorf("dex2jar produced empty output for %s", filepath.Base(resolved))
	}
	if err := os.Rename(tmpJar, cachedJar); err != nil {
		return tmpJar, resolved, nil
	}
	return cachedJar, resolved, nil
}

// Launch starts a clean HeadlessAppHost JVM process and boots an arbitrary APK/JAR without -noverify.
func (b *LiveHeadlessBridge) Launch(apkOrJarPath, activity string) (*LaunchResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	jarPath, apkPath, err := b.PrepareArtifacts(apkOrJarPath)
	if err != nil {
		return nil, err
	}

	// Always spawn a fresh JVM per APK launch so statics, singletons, and ClassLoader state never leak across apps.
	b.closeInternal()

	if strings.TrimSpace(activity) == "" {
		activity = "auto"
	}

	frameworkJar := resolvePath(
		filepath.Join("engine", "runtime", "nano-framework.jar"),
		filepath.Join("..", "runtime", "nano-framework.jar"),
		`d:\code\Main_projects\android_mcp\engine\runtime\nano-framework.jar`,
	)
	androidJar := resolvePath(
		filepath.Join("engine", "runtime", "android.jar"),
		filepath.Join("..", "runtime", "android.jar"),
		`d:\code\Main_projects\android_mcp\engine\runtime\android.jar`,
	)
	if _, err := os.Stat(frameworkJar); err != nil {
		return nil, fmt.Errorf("nano-framework.jar missing at %s", frameworkJar)
	}
	if _, err := os.Stat(androidJar); err != nil {
		return nil, fmt.Errorf("android.jar missing at %s", androidJar)
	}
	cp := fmt.Sprintf("%s;%s", frameworkJar, androidJar)

	args := []string{"-cp", cp, "nanoandroid.host.HeadlessAppHost", jarPath, activity}
	if apkPath != "" {
		args = append(args, apkPath)
	}

	cmd := exec.Command("java", args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start HeadlessAppHost JVM: %w", err)
	}

	b.cmd = cmd
	b.stdin = stdin
	b.reader = bufio.NewReader(stdout)
	b.active = true
	b.jarPath = jarPath
	b.apkPath = apkPath

	line, err := b.readJsonLine()
	if err != nil {
		b.closeInternal()
		return nil, fmt.Errorf("failed reading HeadlessAppHost launch response: %w", err)
	}

	var resp struct {
		Status         string   `json:"status"`
		Error          string   `json:"error"`
		Package        string   `json:"package"`
		Activity       string   `json:"activity"`
		State          string   `json:"state"`
		StartupMs      int64    `json:"startup_ms"`
		AppLabel       string   `json:"app_label"`
		MinSDK         int      `json:"min_sdk"`
		TargetSDK      int      `json:"target_sdk"`
		Activities     []string `json:"activities"`
		NativeLibs     int      `json:"native_libs"`
		Warnings       []string `json:"warnings"`
		ExceptionTrace string   `json:"exception_trace"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		b.closeInternal()
		return nil, fmt.Errorf("invalid HeadlessAppHost launch JSON (%w): %s", err, line)
	}

	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}

	return &LaunchResult{
		AppID:            resp.Package,
		Activity:         resp.Activity,
		PID:              pid,
		Status:           resp.Status,
		SandboxID:        fmt.Sprintf("nano-jvm-%d", pid),
		VirtualTimeMs:    0,
		LaunchDurationMs: resp.StartupMs,
		ApkPath:          apkPath,
		AppLabel:         resp.AppLabel,
		MinSDK:           resp.MinSDK,
		TargetSDK:        resp.TargetSDK,
		Activities:       resp.Activities,
		NativeLibs:       resp.NativeLibs,
		Warnings:         resp.Warnings,
		ExceptionTrace:   resp.ExceptionTrace,
		Error:            resp.Error,
		ExecutionMode:    "nano_jvm",
	}, nil
}

// ObserveDetail holds AST plus live activity stack, active dialog, and toast telemetry.
type ObserveDetail struct {
	Activity      string
	ActivityStack []string
	ActiveDialog  string
	Toast         string
	Screen        *ASTNode
}

// Observe fetches the live Android View AST from the running JVM Activity or AXML preview.
func (b *LiveHeadlessBridge) Observe() (*ASTNode, error) {
	det, err := b.ObserveDetailed()
	if err != nil {
		return nil, err
	}
	return det.Screen, nil
}

// ObserveDetailed fetches the live View AST alongside Activity stack, Dialog, and Toast metadata.
func (b *LiveHeadlessBridge) ObserveDetailed() (*ObserveDetail, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.active {
		return nil, fmt.Errorf("headless bridge not active")
	}

	if _, err := fmt.Fprintln(b.stdin, "OBSERVE"); err != nil {
		return nil, err
	}
	line, err := b.readJsonLine()
	if err != nil {
		return nil, err
	}

	var resp struct {
		Status         string   `json:"status"`
		Error          string   `json:"error"`
		ExceptionTrace string   `json:"exception_trace"`
		Activity       string   `json:"activity"`
		ActivityStack  []string `json:"activity_stack"`
		ActiveDialog   string   `json:"active_dialog"`
		Toast          string   `json:"toast"`
		Screen         *ASTNode `json:"screen"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, err
	}
	if resp.Status == "error" || resp.Screen == nil {
		if resp.Error != "" {
			return nil, fmt.Errorf("observe error: %s", resp.Error)
		}
		return nil, fmt.Errorf("observe error: empty screen tree")
	}

	b.normalizeAST(resp.Screen)
	return &ObserveDetail{
		Activity:      resp.Activity,
		ActivityStack: resp.ActivityStack,
		ActiveDialog:  resp.ActiveDialog,
		Toast:         resp.Toast,
		Screen:        resp.Screen,
	}, nil
}

// DispatchAction dispatches a click/tap/type/scroll/back/swipe input event into the JVM View tree.
func (b *LiveHeadlessBridge) DispatchAction(actionType, target, extra string) (*ActionResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.active {
		return nil, fmt.Errorf("headless bridge not active")
	}

	safeTarget := strings.ReplaceAll(strings.TrimSpace(target), " ", "_")
	safeExtra := strings.ReplaceAll(extra, "\n", " ")
	var cmdStr string
	if safeExtra != "" {
		cmdStr = fmt.Sprintf("ACTION %s %s %s", actionType, safeTarget, safeExtra)
	} else if safeTarget != "" {
		cmdStr = fmt.Sprintf("ACTION %s %s", actionType, safeTarget)
	} else {
		cmdStr = fmt.Sprintf("ACTION %s", actionType)
	}
	if _, err := fmt.Fprintln(b.stdin, cmdStr); err != nil {
		return nil, err
	}

	line, err := b.readJsonLine()
	if err != nil {
		return nil, err
	}

	var resp struct {
		Status       string `json:"status"`
		Action       string `json:"action"`
		Target       string `json:"target"`
		ResolvedText string `json:"resolved_text"`
		Message      string `json:"message"`
		Error        string `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, err
	}

	status := "success"
	msg := resp.Message
	if resp.Status != "ok" {
		status = "failed"
		if msg == "" {
			msg = resp.Error
		}
	}

	return &ActionResult{
		Status:       status,
		Action:       actionType,
		TargetNodeID: resp.Target,
		ResolvedText: resp.ResolvedText,
		Message:      msg,
	}, nil
}

// StepTime advances the deterministic Android Looper clock inside HeadlessAppHost.
func (b *LiveHeadlessBridge) StepTime(deltaMs int64) (*StepResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.active {
		return nil, fmt.Errorf("headless bridge not active")
	}

	if _, err := fmt.Fprintf(b.stdin, "STEP %d\n", deltaMs); err != nil {
		return nil, err
	}

	line, err := b.readJsonLine()
	if err != nil {
		return nil, err
	}

	var resp struct {
		Status   string `json:"status"`
		DeltaMs  int64  `json:"delta_ms"`
		Executed int    `json:"executed"`
		UptimeMs int64  `json:"uptime_ms"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, err
	}

	return &StepResult{
		VirtualTimeMs: resp.UptimeMs,
		SteppedMs:     deltaMs,
		TasksExecuted: resp.Executed,
		IsIdle:        true,
	}, nil
}

// RenderScreen renders the real Java2D 1080x2400 framebuffer PNG from HeadlessAppHost.
func (b *LiveHeadlessBridge) RenderScreen(outputPath string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.active {
		return "", fmt.Errorf("headless bridge not active")
	}

	if outputPath == "" {
		outputPath = filepath.Join(resolveCacheDir(), "live_render.png")
	}
	if _, err := fmt.Fprintf(b.stdin, "RENDER %s\n", outputPath); err != nil {
		return "", err
	}

	line, err := b.readJsonLine()
	if err != nil {
		return "", err
	}

	var resp struct {
		Status string `json:"status"`
		File   string `json:"file"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return "", err
	}
	if resp.Status != "ok" {
		return "", fmt.Errorf("render error: %s", resp.Error)
	}
	return resp.File, nil
}

// InspectRuntime returns live JVM heap, GC, classloader, and view tree telemetry.
func (b *LiveHeadlessBridge) InspectRuntime() (*RuntimeMetrics, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.active {
		return nil, fmt.Errorf("headless bridge not active")
	}

	if _, err := fmt.Fprintln(b.stdin, "INSPECT"); err != nil {
		return nil, err
	}
	line, err := b.readJsonLine()
	if err != nil {
		return nil, err
	}

	var resp struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		RuntimeMetrics
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, err
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("inspect error: %s", resp.Error)
	}
	return &resp.RuntimeMetrics, nil
}

// ListLayouts lists every compiled AXML layout entry found in the loaded APK's resources.arsc.
func (b *LiveHeadlessBridge) ListLayouts() ([]LayoutEntry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.active {
		return nil, fmt.Errorf("headless bridge not active")
	}

	if _, err := fmt.Fprintln(b.stdin, "LIST_LAYOUTS"); err != nil {
		return nil, err
	}
	line, err := b.readJsonLine()
	if err != nil {
		return nil, err
	}

	var resp struct {
		Status  string        `json:"status"`
		Layouts []LayoutEntry `json:"layouts"`
		Error   string        `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, err
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("list layouts error: %s", resp.Error)
	}
	return resp.Layouts, nil
}

// PreviewLayout inflates any compiled AXML layout from the active APK into the HeadlessAppHost window.
func (b *LiveHeadlessBridge) PreviewLayout(nameOrId string) (*LayoutPreviewResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.active {
		return nil, fmt.Errorf("headless bridge not active")
	}

	if _, err := fmt.Fprintf(b.stdin, "PREVIEW_LAYOUT %s\n", strings.TrimSpace(nameOrId)); err != nil {
		return nil, err
	}
	line, err := b.readJsonLine()
	if err != nil {
		return nil, err
	}

	var resp LayoutPreviewResult
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, err
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("preview layout error: %s", resp.Error)
	}
	return &resp, nil
}

// StageFile registers a host media/document file into the staged_files sandbox for Android picker intents.
func (b *LiveHeadlessBridge) StageFile(hostPath string) (*StagedFileInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	abs, err := filepath.Abs(hostPath)
	if err != nil {
		abs = hostPath
	}
	st, statErr := os.Stat(abs)
	if statErr != nil || st.IsDir() {
		return nil, fmt.Errorf("staged file not found: %s", hostPath)
	}
	stagedDir := filepath.Join(resolveCacheDir(), "staged_files")
	_ = os.MkdirAll(stagedDir, 0755)
	dst := filepath.Join(stagedDir, filepath.Base(abs))
	if !strings.EqualFold(abs, dst) {
		srcF, err := os.Open(abs)
		if err != nil {
			return nil, err
		}
		dstF, err := os.Create(dst)
		if err != nil {
			srcF.Close()
			return nil, err
		}
		_, cpErr := io.Copy(dstF, srcF)
		srcF.Close()
		dstF.Close()
		if cpErr != nil {
			return nil, cpErr
		}
	}

	if b.active && b.stdin != nil {
		_, _ = fmt.Fprintf(b.stdin, "STAGE_FILE %s\n", dst)
		if line, err := b.readJsonLine(); err == nil {
			var resp struct {
				Status    string `json:"status"`
				Name      string `json:"name"`
				Path      string `json:"path"`
				URI       string `json:"uri"`
				SizeBytes int64  `json:"size_bytes"`
				Error     string `json:"error"`
			}
			if json.Unmarshal([]byte(line), &resp) == nil && resp.Status == "ok" {
				return &StagedFileInfo{
					Name:      resp.Name,
					Path:      resp.Path,
					URI:       resp.URI,
					SizeBytes: resp.SizeBytes,
				}, nil
			}
		}
	}

	return &StagedFileInfo{
		Name:      filepath.Base(dst),
		Path:      dst,
		URI:       "file:///" + filepath.ToSlash(dst),
		SizeBytes: st.Size(),
	}, nil
}

// ListStagedFiles returns all test input files currently staged for Android file/media pickers.
func (b *LiveHeadlessBridge) ListStagedFiles() []StagedFileInfo {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active && b.stdin != nil {
		_, _ = fmt.Fprintln(b.stdin, "LIST_STAGED")
		if line, err := b.readJsonLine(); err == nil {
			var resp struct {
				Status string           `json:"status"`
				Files  []StagedFileInfo `json:"files"`
			}
			if json.Unmarshal([]byte(line), &resp) == nil && resp.Status == "ok" {
				return resp.Files
			}
		}
	}
	stagedDir := filepath.Join(resolveCacheDir(), "staged_files")
	_ = os.MkdirAll(stagedDir, 0755)
	entries, err := os.ReadDir(stagedDir)
	if err != nil {
		return []StagedFileInfo{}
	}
	out := []StagedFileInfo{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, _ := e.Info()
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		p := filepath.Join(stagedDir, e.Name())
		out = append(out, StagedFileInfo{
			Name:      e.Name(),
			Path:      p,
			URI:       "file:///" + filepath.ToSlash(p),
			SizeBytes: size,
		})
	}
	return out
}

func (b *LiveHeadlessBridge) readJsonLine() (string, error) {
	type readRes struct {
		line string
		err  error
	}
	ch := make(chan readRes, 1)
	go func() {
		for {
			l, err := b.reader.ReadString('\n')
			trimmed := strings.TrimSpace(l)
			if err != nil {
				if strings.HasPrefix(trimmed, "{") {
					ch <- readRes{trimmed, nil}
					return
				}
				ch <- readRes{"", err}
				return
			}
			if strings.HasPrefix(trimmed, "{") {
				ch <- readRes{trimmed, nil}
				return
			}
		}
	}()

	select {
	case res := <-ch:
		return res.line, res.err
	case <-time.After(18 * time.Second):
		return "", fmt.Errorf("timeout reading from HeadlessAppHost")
	}
}

func (b *LiveHeadlessBridge) normalizeAST(node *ASTNode) {
	if node == nil {
		return
	}
	node.Visible = true
	for _, child := range node.Children {
		b.normalizeAST(child)
	}
}

func (b *LiveHeadlessBridge) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closeInternal()
}

func (b *LiveHeadlessBridge) closeInternal() {
	if b.stdin != nil {
		_, _ = fmt.Fprintln(b.stdin, "EXIT")
		_ = b.stdin.Close()
		b.stdin = nil
	}
	if b.cmd != nil && b.cmd.Process != nil {
		done := make(chan error, 1)
		go func(c *exec.Cmd) { done <- c.Wait() }(b.cmd)
		select {
		case <-done:
		case <-time.After(400 * time.Millisecond):
			_ = b.cmd.Process.Kill()
			<-done
		}
		b.cmd = nil
	}
	b.reader = nil
	b.active = false
}

func (b *LiveHeadlessBridge) IsActive() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.active
}

func (b *LiveHeadlessBridge) CurrentApkPath() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.apkPath
}
