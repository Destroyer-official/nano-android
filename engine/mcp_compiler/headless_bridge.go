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
	"runtime"
	"strings"
	"sync"
	"time"

	"nano_android/engine/apk/ingest"
	"nano_android/engine/compatibility/registry"
	"nano_android/engine/security/archive"
	"nano_android/engine/security/pathcontainment"
	"nano_android/engine/security/worker_isolation"
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

// RuntimeMetrics holds real JVM memory, GC, classloader, thread/looper, and virtual uptime stats from HeadlessAppHost.
type RuntimeMetrics struct {
	UsedMB            float64 `json:"used_mb"`
	FreeMB            float64 `json:"free_mb"`
	MaxMB             float64 `json:"max_mb"`
	GCCycles          int64   `json:"gc_cycles"`
	LoadedClasses     int64   `json:"loaded_classes"`
	TotalViews        int64   `json:"total_views"`
	VirtualUptimeMs   int64   `json:"virtual_uptime_ms"`
	ThreadCount       int     `json:"thread_count"`
	DeadlockedThreads int     `json:"deadlocked_threads"`
	LooperQueueDepth  int     `json:"looper_queue_depth"`
	TimelineEvents    int     `json:"timeline_events"`
	Package           string  `json:"package"`
	Activity          string  `json:"activity"`
}

// HostTimelineEvent matches ForensicTimeline JSON entries emitted by HeadlessAppHost.
type HostTimelineEvent struct {
	Seq             int64  `json:"seq"`
	TimestampNs     int64  `json:"timestamp_ns"`
	VirtualUptimeMs int64  `json:"virtual_uptime_ms"`
	EventType       string `json:"event_type"`
	Source          string `json:"source"`
	Backend         string `json:"backend"`
	CorrelationID   string `json:"correlation_id"`
	Details         string `json:"details"`
}

// HostThreadInfo reports live JVM thread inspection and deadlock detection results.
type HostThreadInfo struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	State        string `json:"state"`
	Deadlocked   bool   `json:"deadlocked"`
	BlockedCount int64  `json:"blocked_count"`
	WaitedCount  int64  `json:"waited_count"`
}

// HostThreadReport aggregates JVM thread telemetry and Looper queue depth.
type HostThreadReport struct {
	ThreadCount      int              `json:"thread_count"`
	DeadlockedCount  int              `json:"deadlocked_count"`
	LooperQueueDepth int              `json:"looper_queue_depth"`
	Threads          []HostThreadInfo `json:"threads"`
}

// HostStorageSnapshot captures live SQLite table row counts and SharedPreferences keys/values.
type HostStorageSnapshot struct {
	SQLiteTables map[string]string `json:"sqlite_tables"`
	Preferences  map[string]string `json:"preferences"`
}

// LiveHeadlessBridge manages the OS-isolated, zero-emulator HeadlessAppHost JVM worker.
type LiveHeadlessBridge struct {
	mu                  sync.Mutex
	cmd                 *exec.Cmd
	workerHandle        *worker_isolation.IsolatedHandle
	workerWorkspace     *worker_isolation.WorkerWorkspace
	stdin               io.WriteCloser
	reader              *bufio.Reader
	active              bool
	jarPath             string
	apkPath             string
	SandboxRootOverride string
}

// NewLiveHeadlessBridge initializes a new headless bridge.
func NewLiveHeadlessBridge() *LiveHeadlessBridge {
	return &LiveHeadlessBridge{}
}

func workspaceRootCandidates() []string {
	var roots []string
	if envRoot := strings.TrimSpace(os.Getenv("NANO_ANDROID_HOME")); envRoot != "" {
		roots = append(roots, envRoot)
	}
	if wd, err := os.Getwd(); err == nil {
		cur := wd
		for i := 0; i < 4; i++ {
			roots = append(roots, cur)
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
		}
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		roots = append(roots, exeDir, filepath.Dir(exeDir))
	}
	return roots
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
		if !filepath.IsAbs(p) {
			for _, root := range workspaceRootCandidates() {
				joined := filepath.Join(root, p)
				if _, err := os.Stat(joined); err == nil {
					if abs, err := filepath.Abs(joined); err == nil {
						return abs
					}
					return joined
				}
			}
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}

func resolveCacheDir() string {
	if envCache := strings.TrimSpace(os.Getenv("NANO_CACHE_DIR")); envCache != "" {
		_ = os.MkdirAll(envCache, 0755)
		if abs, err := filepath.Abs(envCache); err == nil {
			return abs
		}
		return envCache
	}
	for _, root := range workspaceRootCandidates() {
		if _, err := os.Stat(filepath.Join(root, "go.work")); err == nil {
			c := filepath.Join(root, "cache")
			if err := os.MkdirAll(c, 0755); err == nil {
				return c
			}
		}
	}
	candidates := []string{
		"cache",
		filepath.Join("..", "..", "cache"),
	}
	for _, c := range candidates {
		parent := filepath.Dir(c)
		if info, err := os.Stat(parent); err == nil && info.IsDir() {
			if err := os.MkdirAll(c, 0755); err == nil {
				if abs, err := filepath.Abs(c); err == nil {
					return abs
				}
				return c
			}
		}
	}
	fallback := filepath.Join(os.TempDir(), "nano-android-cache")
	_ = os.MkdirAll(fallback, 0755)
	return fallback
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
	)
	info, statErr := os.Stat(resolved)
	if statErr != nil || info.IsDir() {
		return "", "", fmt.Errorf("APK/JAR not found: %s", apkOrJarPath)
	}

	lower := strings.ToLower(resolved)
	if !strings.HasSuffix(lower, ".jar") && !strings.HasSuffix(lower, ".apk") {
		return "", "", fmt.Errorf("unsupported file extension (expected .apk or .jar): %s", filepath.Base(resolved))
	}

	// P0: Validate archive integrity, ZIP bomb limits, symlinks, zip-slip, and Janus polyglots before any ingestion.
	if _, valErr := archive.ValidateArchive(resolved, archive.DefaultLimits()); valErr != nil {
		return "", resolved, fmt.Errorf("archive security validation failed for %s: %w", filepath.Base(resolved), valErr)
	}

	if strings.HasSuffix(lower, ".jar") {
		return resolved, "", nil
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

	d2jScript := "d2j-dex2jar.sh"
	if runtime.GOOS == "windows" {
		d2jScript = "d2j-dex2jar.bat"
	}
	d2j := resolvePath(
		filepath.Join("tools", "dex-tools-v2.4", d2jScript),
		filepath.Join("..", "..", "tools", "dex-tools-v2.4", d2jScript),
	)
	if _, err := os.Stat(d2j); err != nil {
		return "", resolved, fmt.Errorf("dex2jar translator not found at %s", d2j)
	}

	// Run dex2jar inside an OS-isolated worker workspace with memory/process/timeout limits.
	ws, wsErr := worker_isolation.PrepareWorkspace(cacheDir)
	if wsErr != nil {
		return "", resolved, fmt.Errorf("prepare translator worker workspace: %w", wsErr)
	}
	defer func() { _ = ws.Cleanup() }()

	tmpJar := filepath.Join(ws.TempDir, hashHex+"_classes.tmp.jar")
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", d2j, "-f", "-o", tmpJar, resolved)
	} else {
		cmd = exec.Command("sh", d2j, "-f", "-o", tmpJar, resolved)
	}
	var outBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	limits := worker_isolation.DefaultTranslatorLimits()
	h, isoErr := worker_isolation.StartIsolated(cmd, ws.WorkerID, ws, limits)
	if isoErr != nil {
		return "", resolved, fmt.Errorf("start isolated dex2jar worker: %w", isoErr)
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	var convErr error
	select {
	case convErr = <-waitDone:
	case <-time.After(limits.ExecutionTimeout):
		_ = h.Close()
		convErr = fmt.Errorf("dex2jar translation timed out after %v", limits.ExecutionTimeout)
		<-waitDone
	}
	_ = h.Close()

	if convErr != nil {
		return "", resolved, fmt.Errorf("dex2jar conversion failed for %s: %v (%s)", filepath.Base(resolved), convErr, strings.TrimSpace(outBuf.String()))
	}
	if st, err := os.Stat(tmpJar); err != nil || st.Size() == 0 {
		return "", resolved, fmt.Errorf("dex2jar produced empty output for %s", filepath.Base(resolved))
	}
	if _, valJarErr := archive.ValidateArchive(tmpJar, archive.DefaultLimits()); valJarErr != nil {
		return "", resolved, fmt.Errorf("translated JAR failed security validation: %w", valJarErr)
	}
	if err := os.Rename(tmpJar, cachedJar); err != nil {
		// Cross-device fallback: copy into cache if rename fails
		if data, rErr := os.ReadFile(tmpJar); rErr == nil {
			if wErr := os.WriteFile(cachedJar, data, 0644); wErr == nil {
				return cachedJar, resolved, nil
			}
		}
		return "", resolved, fmt.Errorf("persist translated jar: %w", err)
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
	)
	androidJar := resolvePath(
		filepath.Join("engine", "runtime", "android.jar"),
		filepath.Join("..", "runtime", "android.jar"),
	)
	sqliteJar := resolvePath(
		filepath.Join("tools", "libs", "sqlite-jdbc-3.54.1.0.jar"),
		filepath.Join("..", "..", "tools", "libs", "sqlite-jdbc-3.54.1.0.jar"),
	)
	if _, err := os.Stat(frameworkJar); err != nil {
		return nil, fmt.Errorf("nano-framework.jar missing at %s", frameworkJar)
	}
	if _, err := os.Stat(androidJar); err != nil {
		return nil, fmt.Errorf("android.jar missing at %s", androidJar)
	}
	cpParts := []string{frameworkJar, androidJar}
	if _, err := os.Stat(sqliteJar); err == nil {
		cpParts = append(cpParts, sqliteJar)
	}
	cp := strings.Join(cpParts, string(os.PathListSeparator))

	ws, wsErr := worker_isolation.PrepareWorkspace(resolveCacheDir())
	if wsErr != nil {
		return nil, fmt.Errorf("prepare isolated worker workspace: %w", wsErr)
	}

	sandboxRoot := filepath.Join(resolveCacheDir(), "sandbox")
	if strings.TrimSpace(b.SandboxRootOverride) != "" {
		sandboxRoot = b.SandboxRootOverride
		_ = os.MkdirAll(sandboxRoot, 0750)
	} else if strings.EqualFold(strings.TrimSpace(os.Getenv("NANO_ISOLATION_PROFILE")), "hostile") {
		sandboxRoot = ws.SandboxDir
	}

	args := []string{
		"-Xmx768m",
		"-Dfile.encoding=UTF-8",
		"-Dsun.stdout.encoding=UTF-8",
		"-Dsun.stderr.encoding=UTF-8",
		"--enable-native-access=ALL-UNNAMED",
		"-Djava.io.tmpdir=" + ws.TempDir,
		"-Dnano.sandbox.dir=" + sandboxRoot,
		"-cp", cp,
		"nanoandroid.host.HeadlessAppHost",
		jarPath,
		activity,
	}
	if apkPath != "" {
		args = append(args, apkPath)
	}

	cmd := exec.Command("java", args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = ws.Cleanup()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = ws.Cleanup()
		return nil, err
	}
	cmd.Stderr = os.Stderr

	limits := worker_isolation.DefaultJVMLimits()
	handle, isoErr := worker_isolation.StartIsolated(cmd, ws.WorkerID, ws, limits)
	if isoErr != nil {
		_ = ws.Cleanup()
		return nil, fmt.Errorf("failed to start isolated HeadlessAppHost JVM: %w", isoErr)
	}

	b.cmd = cmd
	b.workerHandle = handle
	b.workerWorkspace = ws
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
		Status                string   `json:"status"`
		ResultStatus          string   `json:"result_status"`
		DegradedHeuristic     bool     `json:"degraded_heuristic"`
		TransformationCount   int      `json:"transformation_count"`
		CompatFaultCount      int      `json:"compat_fault_count"`
		KotlinMetadataClasses int      `json:"kotlin_metadata_classes"`
		Error                 string   `json:"error"`
		Package               string   `json:"package"`
		Activity              string   `json:"activity"`
		State                 string   `json:"state"`
		StartupMs             int64    `json:"startup_ms"`
		AppLabel              string   `json:"app_label"`
		MinSDK                int      `json:"min_sdk"`
		TargetSDK             int      `json:"target_sdk"`
		Activities            []string `json:"activities"`
		NativeLibs            int      `json:"native_libs"`
		Warnings              []string `json:"warnings"`
		ExceptionTrace        string   `json:"exception_trace"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		b.closeInternal()
		return nil, fmt.Errorf("invalid HeadlessAppHost launch JSON (%w): %s", err, line)
	}

	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}

	var compatRec *registry.ExecutionRecommendation
	if apkPath != "" && strings.HasSuffix(strings.ToLower(apkPath), ".apk") {
		eng := ingest.NewEngine(filepath.Join(resolveCacheDir(), "apk_intel"), "")
		if rep, ingestErr := eng.Analyze(apkPath); ingestErr == nil && rep != nil {
			rec := registry.EvaluateAPK(rep, "auto")
			compatRec = &rec
		}
	}

	return &LaunchResult{
		AppID:                 resp.Package,
		Activity:              resp.Activity,
		PID:                   pid,
		Status:                resp.Status,
		SandboxID:             fmt.Sprintf("nano-jvm-%d", pid),
		VirtualTimeMs:         0,
		LaunchDurationMs:      resp.StartupMs,
		ApkPath:               apkPath,
		AppLabel:              resp.AppLabel,
		MinSDK:                resp.MinSDK,
		TargetSDK:             resp.TargetSDK,
		Activities:            resp.Activities,
		NativeLibs:            resp.NativeLibs,
		Warnings:              resp.Warnings,
		ExceptionTrace:        resp.ExceptionTrace,
		Error:                 resp.Error,
		ExecutionMode:         "nano_jvm",
		ResultStatus:          resp.ResultStatus,
		DegradedHeuristic:     resp.DegradedHeuristic,
		TransformationCount:   resp.TransformationCount,
		CompatFaultCount:      resp.CompatFaultCount,
		KotlinMetadataClasses: resp.KotlinMetadataClasses,
		Compatibility:         compatRec,
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
		Status          string `json:"status"`
		DeltaMs         int64  `json:"delta_ms"`
		ProcessedTasks  int    `json:"processed_tasks"`
		VirtualUptimeMs int64  `json:"virtual_uptime_ms"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, err
	}

	return &StepResult{
		VirtualTimeMs: resp.VirtualUptimeMs,
		SteppedMs:     deltaMs,
		TasksExecuted: resp.ProcessedTasks,
		IsIdle:        true,
	}, nil
}

// SetProfile configures the virtual viewport width/height inside HeadlessAppHost.
func (b *LiveHeadlessBridge) SetProfile(widthPx, heightPx int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.active {
		return fmt.Errorf("headless bridge not active")
	}
	if _, err := fmt.Fprintf(b.stdin, "SET_PROFILE %d %d\n", widthPx, heightPx); err != nil {
		return err
	}
	line, err := b.readJsonLine()
	if err != nil {
		return err
	}
	var resp struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return err
	}
	if resp.Status != "ok" {
		return fmt.Errorf("set profile error: %s", resp.Error)
	}
	return nil
}

// SimulateProcessDeath triggers onPause -> onSaveInstanceState -> onStop -> onDestroy and restores fresh Activity state.
func (b *LiveHeadlessBridge) SimulateProcessDeath() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.active {
		return fmt.Errorf("headless bridge not active")
	}
	if _, err := fmt.Fprintln(b.stdin, "SIMULATE_PROCESS_DEATH"); err != nil {
		return err
	}
	line, err := b.readJsonLine()
	if err != nil {
		return err
	}
	var resp struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return err
	}
	if resp.Status != "ok" {
		return fmt.Errorf("process death simulation error: %s", resp.Error)
	}
	return nil
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

// InspectTimeline retrieves the monotonic correlated forensic timeline recorded during the active session.
func (b *LiveHeadlessBridge) InspectTimeline() ([]HostTimelineEvent, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.active {
		return nil, fmt.Errorf("headless bridge not active")
	}
	if _, err := fmt.Fprintln(b.stdin, "TIMELINE"); err != nil {
		return nil, err
	}
	line, err := b.readJsonLine()
	if err != nil {
		return nil, err
	}
	var resp struct {
		Status string              `json:"status"`
		Error  string              `json:"error"`
		Events []HostTimelineEvent `json:"events"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, err
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("timeline error: %s", resp.Error)
	}
	return resp.Events, nil
}

// InspectThreads returns live JVM ThreadMXBean thread states, deadlock detection, and Looper queue depth.
func (b *LiveHeadlessBridge) InspectThreads() (*HostThreadReport, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.active {
		return nil, fmt.Errorf("headless bridge not active")
	}
	if _, err := fmt.Fprintln(b.stdin, "THREADS"); err != nil {
		return nil, err
	}
	line, err := b.readJsonLine()
	if err != nil {
		return nil, err
	}
	var resp struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		HostThreadReport
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, err
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("threads error: %s", resp.Error)
	}
	return &resp.HostThreadReport, nil
}

// InspectStorage returns live active SQLite table row counts and SharedPreferences entries.
func (b *LiveHeadlessBridge) InspectStorage() (*HostStorageSnapshot, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.active {
		return nil, fmt.Errorf("headless bridge not active")
	}
	if _, err := fmt.Fprintln(b.stdin, "INSPECT_STORAGE"); err != nil {
		return nil, err
	}
	line, err := b.readJsonLine()
	if err != nil {
		return nil, err
	}
	var resp struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		HostStorageSnapshot
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, err
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("inspect storage error: %s", resp.Error)
	}
	if resp.SQLiteTables == nil {
		resp.SQLiteTables = map[string]string{}
	}
	if resp.Preferences == nil {
		resp.Preferences = map[string]string{}
	}
	return &resp.HostStorageSnapshot, nil
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

// StageFile registers a contained workspace media/document file into the staged_files sandbox for Android picker intents.
func (b *LiveHeadlessBridge) StageFile(hostPath string) (*StagedFileInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := pathcontainment.CheckPathSyntax(hostPath); err != nil {
		return nil, fmt.Errorf("unsafe stage_file path: %w", err)
	}
	allowedRoots := []string{
		filepath.Join(resolveCacheDir(), "staged_files"),
		filepath.Join(resolveCacheDir(), "uploads"),
		resolveCacheDir(),
		resolvePath("fixtures"),
	}
	resolvedSrc, err := pathcontainment.ResolveExistingContained(hostPath, allowedRoots, nil)
	if err != nil {
		return nil, fmt.Errorf("staged file access denied: %w", err)
	}
	st, statErr := os.Stat(resolvedSrc)
	if statErr != nil || st.IsDir() {
		return nil, fmt.Errorf("staged file not found: %s", hostPath)
	}
	stagedDir := filepath.Join(resolveCacheDir(), "staged_files")
	safeName, sErr := pathcontainment.SanitizeFilename(filepath.Base(resolvedSrc))
	if sErr != nil {
		return nil, sErr
	}
	dst, wErr := pathcontainment.ResolveWriteContained(stagedDir, safeName, nil)
	if wErr != nil {
		return nil, wErr
	}
	if !strings.EqualFold(resolvedSrc, dst) {
		srcF, err := os.Open(resolvedSrc)
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
			if b.workerHandle != nil {
				_ = b.workerHandle.Close()
			} else {
				_ = b.cmd.Process.Kill()
			}
			<-done
		}
		b.cmd = nil
	}
	if b.workerHandle != nil {
		_ = b.workerHandle.Close()
		b.workerHandle = nil
	}
	if b.workerWorkspace != nil {
		_ = b.workerWorkspace.Cleanup()
		b.workerWorkspace = nil
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
