package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// CheckStatus indicates the health of an environment dependency.
type CheckStatus string

const (
	StatusOK   CheckStatus = "OK"
	StatusWarn CheckStatus = "WARN"
	StatusFail CheckStatus = "FAIL"
)

// CheckResult records findings for one environment requirement.
type CheckResult struct {
	Name        string      `json:"name"`
	Status      CheckStatus `json:"status"`
	Version     string      `json:"version,omitempty"`
	Path        string      `json:"path,omitempty"`
	Details     string      `json:"details,omitempty"`
	Remediation string      `json:"remediation,omitempty"`
}

// RunDoctor executes all host environment checks and returns results.
func RunDoctor(ctx context.Context, cfg *types.DroidForgeConfig) []CheckResult {
	var results []CheckResult

	// 1. Check ADB
	results = append(results, checkADB(ctx))

	// 2. Check Android Emulator & Acceleration
	results = append(results, checkEmulator(ctx))

	// 3. Check scrcpy
	results = append(results, checkScrcpy(ctx))

	// 4. Check Go Version
	results = append(results, checkGo())

	// 5. Check Gradle / Wrapper
	results = append(results, checkGradle(cfg))

	// 6. Check Disk Space
	results = append(results, checkDiskSpace())

	// 7. Check Java JDK
	results = append(results, checkJava(ctx))

	// 8. Check Android SDK Root
	results = append(results, checkAndroidSDK())

	// 9. Check FFmpeg (scrcpy H264 decoding & media verification)
	results = append(results, checkFFmpeg(ctx))

	// 10. Check Maestro CLI (Explore-to-Freeze regression runner)
	results = append(results, checkMaestro(ctx))

	// 11. Check APK Static Analysis Tools (aapt2 / apkanalyzer / jadx)
	results = append(results, checkAPKTools())

	// 12. Check Python Sidecar (Perfetto & media SSIM/VMAF analysis)
	results = append(results, checkPythonSidecar(ctx))

	return results
}

func checkADB(ctx context.Context) CheckResult {
	adbPath, err := exec.LookPath("adb")
	if err != nil {
		adbBin := "adb"
		if runtime.GOOS == "windows" {
			adbBin = "adb.exe"
		}
		candidates := []string{}
		for _, envVar := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
			if root := strings.TrimSpace(os.Getenv(envVar)); root != "" {
				candidates = append(candidates, filepath.Join(root, "platform-tools", adbBin))
			}
		}
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			candidates = append(candidates, filepath.Join(local, "Android", "Sdk", "platform-tools", adbBin))
		}
		if home, hErr := os.UserHomeDir(); hErr == nil {
			candidates = append(candidates,
				filepath.Join(home, "Android", "Sdk", "platform-tools", adbBin),
				filepath.Join(home, "Library", "Android", "sdk", "platform-tools", adbBin),
			)
		}
		for _, c := range candidates {
			if c != "" {
				if _, statErr := os.Stat(c); statErr == nil {
					adbPath = c
					err = nil
					break
				}
			}
		}
	}

	if err != nil {
		return CheckResult{
			Name:        "ADB",
			Status:      StatusFail,
			Details:     "adb binary not found in PATH or Android SDK platform-tools",
			Remediation: "Install Android SDK Platform-Tools and ensure 'adb' is on your PATH or ANDROID_HOME is set",
		}
	}

	cmd := exec.CommandContext(ctx, adbPath, "version")
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return CheckResult{
			Name:        "ADB",
			Status:      StatusFail,
			Path:        adbPath,
			Details:     fmt.Sprintf("failed running %s version: %v", adbPath, runErr),
			Remediation: "Verify adb permissions and ensure adb server is runnable",
		}
	}

	firstLine := strings.Split(strings.TrimSpace(string(out)), "\n")[0]
	return CheckResult{
		Name:    "ADB",
		Status:  StatusOK,
		Path:    adbPath,
		Version: firstLine,
		Details: strings.TrimSpace(string(out)),
	}
}

func checkEmulator(ctx context.Context) CheckResult {
	emuPath, err := exec.LookPath("emulator")
	if err != nil {
		candidates := []string{
			os.Getenv("ANDROID_HOME") + "/emulator/emulator",
			os.Getenv("ANDROID_SDK_ROOT") + "/emulator/emulator",
			os.Getenv("ANDROID_HOME") + "/emulator/emulator.exe",
			os.Getenv("ANDROID_SDK_ROOT") + "/emulator/emulator.exe",
			os.ExpandEnv("$LOCALAPPDATA/Android/Sdk/emulator/emulator.exe"),
		}
		for _, c := range candidates {
			if _, statErr := os.Stat(c); statErr == nil {
				emuPath = c
				err = nil
				break
			}
		}
	}

	if err != nil {
		return CheckResult{
			Name:        "Android Emulator",
			Status:      StatusWarn,
			Details:     "emulator binary not found in PATH or Android SDK",
			Remediation: "Install Android Emulator via SDK Manager (`sdkmanager --install emulator`)",
		}
	}

	// Run accel check
	cmd := exec.CommandContext(ctx, emuPath, "-accel-check")
	out, runErr := cmd.CombinedOutput()
	outputStr := strings.TrimSpace(string(out))

	if runErr != nil && !strings.Contains(strings.ToLower(outputStr), "accel") {
		return CheckResult{
			Name:        "Android Emulator",
			Status:      StatusWarn,
			Path:        emuPath,
			Details:     fmt.Sprintf("emulator found at %s but -accel-check failed: %s", emuPath, outputStr),
			Remediation: "Ensure hardware virtualization (VT-x/SVM) and WHPX (Windows) or KVM (Linux) are enabled in BIOS",
		}
	}

	return CheckResult{
		Name:    "Android Emulator",
		Status:  StatusOK,
		Path:    emuPath,
		Details: outputStr,
	}
}

func checkScrcpy(ctx context.Context) CheckResult {
	// First check for scrcpy-server jar (DroidForge zero-install deployment)
	jarCandidates := []string{
		"bin/scrcpy-server.jar",
		"bin/scrcpy-server-v5.0.1",
		"droidforge-mcp/bin/scrcpy-server.jar",
		"droidforge-mcp/bin/scrcpy-server-v5.0.1",
		os.Getenv("SCRCPY_SERVER_PATH"),
	}
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		jarCandidates = append([]string{
			filepath.Join(execDir, "bin", "scrcpy-server.jar"),
			filepath.Join(execDir, "bin", "scrcpy-server-v5.0.1"),
			filepath.Join(execDir, "scrcpy-server.jar"),
		}, jarCandidates...)
	}
	for _, c := range jarCandidates {
		if c != "" {
			if info, err := os.Stat(c); err == nil {
				return CheckResult{
					Name:    "scrcpy-server",
					Status:  StatusOK,
					Path:    c,
					Version: "v5.0.1",
					Details: fmt.Sprintf("Zero-install server jar ready (%d bytes) for transient deployment", info.Size()),
				}
			}
		}
	}

	scrcpyPath, err := exec.LookPath("scrcpy")
	if err != nil {
		return CheckResult{
			Name:        "scrcpy",
			Status:      StatusWarn,
			Details:     "scrcpy binary or scrcpy-server.jar not found",
			Remediation: "Download scrcpy-server.jar v5.0+ into bin/ for sub-100ms frame streaming; fallback adb screencap will be used",
		}
	}

	cmd := exec.CommandContext(ctx, scrcpyPath, "--version")
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return CheckResult{
			Name:        "scrcpy",
			Status:      StatusWarn,
			Path:        scrcpyPath,
			Details:     fmt.Sprintf("found scrcpy at %s but --version failed: %v", scrcpyPath, runErr),
			Remediation: "Verify scrcpy installation",
		}
	}

	firstLine := strings.Split(strings.TrimSpace(string(out)), "\n")[0]
	return CheckResult{
		Name:    "scrcpy",
		Status:  StatusOK,
		Path:    scrcpyPath,
		Version: firstLine,
		Details: strings.TrimSpace(string(out)),
	}
}

func checkGo() CheckResult {
	ver := runtime.Version()
	return CheckResult{
		Name:    "Go Runtime",
		Status:  StatusOK,
		Version: fmt.Sprintf("%s (%s/%s)", ver, runtime.GOOS, runtime.GOARCH),
		Details: fmt.Sprintf("Compiler: %s", runtime.Compiler),
	}
}

func checkGradle(cfg *types.DroidForgeConfig) CheckResult {
	root := "."
	if cfg != nil && cfg.Project.GradleRoot != "" {
		root = cfg.Project.GradleRoot
	}

	// Check local wrapper first
	wrapperSh := filepath.Join(root, "gradlew")
	wrapperBat := filepath.Join(root, "gradlew.bat")

	if _, err := os.Stat(wrapperBat); err == nil {
		return CheckResult{
			Name:    "Gradle Wrapper",
			Status:  StatusOK,
			Path:    wrapperBat,
			Details: "Found gradlew.bat in target project",
		}
	}
	if _, err := os.Stat(wrapperSh); err == nil {
		return CheckResult{
			Name:    "Gradle Wrapper",
			Status:  StatusOK,
			Path:    wrapperSh,
			Details: "Found gradlew in target project",
		}
	}

	// Check system gradle
	gradlePath, err := exec.LookPath("gradle")
	if err == nil {
		return CheckResult{
			Name:    "System Gradle",
			Status:  StatusOK,
			Path:    gradlePath,
			Details: "System gradle binary found",
		}
	}

	return CheckResult{
		Name:        "Gradle",
		Status:      StatusWarn,
		Details:     "Neither gradlew wrapper nor system gradle found in project root",
		Remediation: "Ensure target Android repository contains gradlew or install Gradle on host",
	}
}

func checkDiskSpace() CheckResult {
	freeBytes, totalBytes, err := getDiskFreeSpace(".")
	if err != nil {
		return CheckResult{
			Name:        "Disk Space",
			Status:      StatusWarn,
			Details:     fmt.Sprintf("unable to query disk space: %v", err),
			Remediation: "Ensure host filesystem permissions allow querying volume statistics",
		}
	}

	freeGB := float64(freeBytes) / (1024 * 1024 * 1024)
	totalGB := float64(totalBytes) / (1024 * 1024 * 1024)

	status := StatusOK
	remediation := ""
	if freeGB < 10.0 {
		status = StatusWarn
		remediation = "Less than 10 GB free space; Android AVDs and Gradle caches recommend >= 15 GB"
	}

	return CheckResult{
		Name:        "Disk Space",
		Status:      status,
		Details:     fmt.Sprintf("%.1f GB free / %.1f GB total (%.1f%% available)", freeGB, totalGB, (freeGB/totalGB)*100),
		Remediation: remediation,
	}
}

func checkJava(ctx context.Context) CheckResult {
	javaPath, err := exec.LookPath("java")
	if err != nil {
		jh := os.Getenv("JAVA_HOME")
		if jh != "" {
			javaBin := "java"
			if runtime.GOOS == "windows" {
				javaBin = "java.exe"
			}
			c := filepath.Join(jh, "bin", javaBin)
			if _, statErr := os.Stat(c); statErr == nil {
				javaPath = c
				err = nil
			}
		}
	}

	if err != nil {
		return CheckResult{
			Name:        "Java JDK",
			Status:      StatusWarn,
			Details:     "java binary not found in PATH or JAVA_HOME",
			Remediation: "Install OpenJDK 17 or 21 and export JAVA_HOME",
		}
	}

	cmd := exec.CommandContext(ctx, javaPath, "-version")
	out, _ := cmd.CombinedOutput()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	firstLine := ""
	if len(lines) > 0 {
		firstLine = lines[0]
	}

	return CheckResult{
		Name:    "Java JDK",
		Status:  StatusOK,
		Path:    javaPath,
		Version: firstLine,
		Details: strings.TrimSpace(string(out)),
	}
}

func checkAndroidSDK() CheckResult {
	androidHome := os.Getenv("ANDROID_HOME")
	if androidHome == "" {
		androidHome = os.Getenv("ANDROID_SDK_ROOT")
	}

	if androidHome != "" {
		if _, err := os.Stat(androidHome); err == nil {
			return CheckResult{
				Name:    "Android SDK Root",
				Status:  StatusOK,
				Path:    androidHome,
				Details: fmt.Sprintf("ANDROID_HOME set to %s", androidHome),
			}
		}
	}

	// Check common cross-platform fallback locations
	var fallbacks []string
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		fallbacks = append(fallbacks, filepath.Join(local, "Android", "Sdk"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		fallbacks = append(fallbacks,
			filepath.Join(home, "Android", "Sdk"),
			filepath.Join(home, "Library", "Android", "sdk"),
		)
	}
	for _, fb := range fallbacks {
		if fb == "" {
			continue
		}
		if _, err := os.Stat(fb); err == nil {
			return CheckResult{
				Name:        "Android SDK Root",
				Status:      StatusOK,
				Path:        fb,
				Details:     fmt.Sprintf("Found SDK at %s (ANDROID_HOME not explicitly set)", fb),
				Remediation: fmt.Sprintf("Recommended: export ANDROID_HOME=%s", fb),
			}
		}
	}

	return CheckResult{
		Name:        "Android SDK Root",
		Status:      StatusWarn,
		Details:     "ANDROID_HOME / ANDROID_SDK_ROOT environment variable not set",
		Remediation: "Set ANDROID_HOME pointing to your Android SDK installation",
	}
}

func checkFFmpeg(ctx context.Context) CheckResult {
	if env := strings.TrimSpace(os.Getenv("FFMPEG_PATH")); env != "" {
		if _, err := os.Stat(env); err == nil {
			return CheckResult{Name: "FFmpeg", Status: StatusOK, Path: env, Details: "Configured via FFMPEG_PATH"}
		}
	}
	ffmpegBin := "ffmpeg"
	if runtime.GOOS == "windows" {
		ffmpegBin = "ffmpeg.exe"
	}
	candidates := []string{
		filepath.Join("bin", ffmpegBin),
		filepath.Join("droidforge-mcp", "bin", ffmpegBin),
	}
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		candidates = append([]string{
			filepath.Join(execDir, "bin", ffmpegBin),
			filepath.Join(execDir, ffmpegBin),
		}, candidates...)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return CheckResult{Name: "FFmpeg", Status: StatusOK, Path: c, Details: "Bundled ffmpeg binary ready"}
		}
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		cmd := exec.CommandContext(ctx, p, "-version")
		out, _ := cmd.CombinedOutput()
		firstLine := strings.Split(strings.TrimSpace(string(out)), "\n")[0]
		return CheckResult{Name: "FFmpeg", Status: StatusOK, Path: p, Version: firstLine}
	}
	return CheckResult{
		Name:        "FFmpeg",
		Status:      StatusWarn,
		Details:     "ffmpeg not found in PATH or bin/",
		Remediation: "Install ffmpeg or place binary at bin/ffmpeg(.exe) for scrcpy H.264 stream decoding and media verification",
	}
}

func checkMaestro(ctx context.Context) CheckResult {
	p, err := exec.LookPath("maestro")
	if err != nil {
		if home, hErr := os.UserHomeDir(); hErr == nil {
			cand := filepath.Join(home, ".maestro", "bin", "maestro")
			if runtime.GOOS == "windows" {
				cand += ".bat"
			}
			if _, sErr := os.Stat(cand); sErr == nil {
				p = cand
				err = nil
			}
		}
	}
	if err != nil {
		return CheckResult{
			Name:        "Maestro CLI",
			Status:      StatusWarn,
			Details:     "maestro binary not found in PATH",
			Remediation: "Install Maestro CLI (`curl -Ls https://get.maestro.mobile.dev | bash`) to run frozen YAML tests via test_run_maestro",
		}
	}
	cmd := exec.CommandContext(ctx, p, "--version")
	out, _ := cmd.CombinedOutput()
	return CheckResult{
		Name:    "Maestro CLI",
		Status:  StatusOK,
		Path:    p,
		Version: strings.TrimSpace(string(out)),
	}
}

func checkAPKTools() CheckResult {
	found := []string{}
	for _, tool := range []string{"aapt2", "apkanalyzer", "apksigner", "jadx"} {
		if _, err := exec.LookPath(tool); err == nil {
			found = append(found, tool)
		}
	}
	if len(found) > 0 {
		return CheckResult{
			Name:    "APK Analysis Tools",
			Status:  StatusOK,
			Details: fmt.Sprintf("Available on PATH: %s (plus built-in pure-Go AXML/ELF fallback)", strings.Join(found, ", ")),
		}
	}
	return CheckResult{
		Name:        "APK Analysis Tools",
		Status:      StatusWarn,
		Details:     "aapt2/apkanalyzer/apksigner/jadx not found on PATH (built-in Go parser still active)",
		Remediation: "Add Android SDK build-tools / cmdline-tools and jadx to PATH for full static inspection",
	}
}

func checkPythonSidecar(ctx context.Context) CheckResult {
	sidecarDir := strings.TrimSpace(os.Getenv("DROIDFORGE_SIDECAR_DIR"))
	if sidecarDir == "" {
		for _, c := range []string{"sidecar-python", filepath.Join("droidforge-mcp", "sidecar-python")} {
			if _, err := os.Stat(filepath.Join(c, "df_analyze_trace.py")); err == nil {
				sidecarDir = c
				break
			}
		}
	}
	if sidecarDir == "" {
		return CheckResult{
			Name:        "Python Sidecar",
			Status:      StatusWarn,
			Details:     "sidecar-python directory not located relative to current directory",
			Remediation: "Set DROIDFORGE_SIDECAR_DIR to droidforge-mcp/sidecar-python",
		}
	}
	if uvPath, err := exec.LookPath("uv"); err == nil {
		return CheckResult{
			Name:    "Python Sidecar",
			Status:  StatusOK,
			Path:    uvPath,
			Details: fmt.Sprintf("uv + sidecar scripts ready at %s", sidecarDir),
		}
	}
	for _, py := range []string{"python3", "python"} {
		if pyPath, err := exec.LookPath(py); err == nil {
			cmd := exec.CommandContext(ctx, pyPath, "--version")
			out, _ := cmd.CombinedOutput()
			return CheckResult{
				Name:        "Python Sidecar",
				Status:      StatusOK,
				Path:        pyPath,
				Version:     strings.TrimSpace(string(out)),
				Details:     fmt.Sprintf("Python sidecar scripts ready at %s", sidecarDir),
				Remediation: "Optional: install `uv` for isolated sidecar dependency management",
			}
		}
	}
	return CheckResult{
		Name:        "Python Sidecar",
		Status:      StatusWarn,
		Details:     "Neither uv nor python found on PATH",
		Remediation: "Install uv or Python 3.11+ for Perfetto trace analysis and media quality verification",
	}
}

// PrintDoctorReport outputs a formatted diagnostic summary to w.
func PrintDoctorReport(w io.Writer, results []CheckResult) bool {
	var buf bytes.Buffer
	buf.WriteString("=======================================================\n")
	buf.WriteString("           DroidForge MCP Environment Doctor           \n")
	buf.WriteString("=======================================================\n\n")

	okCount, warnCount, failCount := 0, 0, 0

	for _, res := range results {
		tag := fmt.Sprintf("[%s]", res.Status)
		switch res.Status {
		case StatusOK:
			okCount++
			buf.WriteString(fmt.Sprintf("%-8s %-20s", tag, res.Name))
			if res.Version != "" {
				buf.WriteString(fmt.Sprintf(": %s", res.Version))
			} else if res.Path != "" {
				buf.WriteString(fmt.Sprintf(": %s", res.Path))
			} else if res.Details != "" {
				buf.WriteString(fmt.Sprintf(": %s", res.Details))
			}
			buf.WriteString("\n")
		case StatusWarn:
			warnCount++
			buf.WriteString(fmt.Sprintf("%-8s %-20s: %s\n", tag, res.Name, res.Details))
			if res.Remediation != "" {
				buf.WriteString(fmt.Sprintf("         -> Remediation: %s\n", res.Remediation))
			}
		case StatusFail:
			failCount++
			buf.WriteString(fmt.Sprintf("%-8s %-20s: %s\n", tag, res.Name, res.Details))
			if res.Remediation != "" {
				buf.WriteString(fmt.Sprintf("         -> Remediation: %s\n", res.Remediation))
			}
		}
	}

	buf.WriteString("\n-------------------------------------------------------\n")
	buf.WriteString(fmt.Sprintf("Summary: %d OK, %d WARN, %d FAIL\n", okCount, warnCount, failCount))
	if failCount == 0 {
		buf.WriteString("Result: READY FOR MCP LOOP (All mandatory tools passed)\n")
	} else {
		buf.WriteString("Result: BLOCKED (Resolve FAIL items above to run the loop)\n")
	}
	buf.WriteString("=======================================================\n")

	_, _ = w.Write(buf.Bytes())
	return failCount == 0
}

// PrintDoctorJSON outputs the diagnostic results as formatted JSON to w.
func PrintDoctorJSON(w io.Writer, results []CheckResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}
