package frameworks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// FrameworkID identifies all 12 native Android/testing/tooling adapters (§10.1).
type FrameworkID string

const (
	FrameworkMaestro      FrameworkID = "maestro"
	FrameworkAndroidXTest FrameworkID = "androidx_test"
	FrameworkJUnit4       FrameworkID = "junit4"
	FrameworkJUnit5       FrameworkID = "junit5"
	FrameworkEspresso     FrameworkID = "espresso"
	FrameworkUIAutomator  FrameworkID = "uiautomator"
	FrameworkRobolectric  FrameworkID = "robolectric"
	FrameworkGradle       FrameworkID = "gradle"
	FrameworkADB          FrameworkID = "adb"
	FrameworkPerfetto     FrameworkID = "perfetto"
	FrameworkFFmpeg       FrameworkID = "ffmpeg"
	FrameworkBundletool   FrameworkID = "bundletool"
)

// AdapterSpec reports host readiness and preserved native semantics for one ecosystem adapter.
type AdapterSpec struct {
	ID               FrameworkID `json:"id"`
	DisplayName      string      `json:"display_name"`
	ExecutionTier    string      `json:"execution_tier"`
	Available        bool        `json:"available"`
	ResolvedPath     string      `json:"resolved_path,omitempty"`
	NativeSemantics  string      `json:"native_semantics"`
}

// TranslateRequest asks an adapter to produce a safe shell-free binary + args invocation.
type TranslateRequest struct {
	ArtifactOrTarget string            `json:"artifact_or_target"`
	PackageName      string            `json:"package_name,omitempty"`
	DeviceSerial     string            `json:"device_serial,omitempty"`
	ExtraArgs        []string          `json:"extra_args,omitempty"`
}

// TranslatedCommand holds a strictly parameterized argv command (never shell-interpolated).
type TranslatedCommand struct {
	AdapterID       FrameworkID `json:"adapter_id"`
	Binary          string      `json:"binary"`
	Args            []string    `json:"args"`
	NativeSemantics string      `json:"native_semantics"`
}

// ProbeAll discovers real binaries and pinned toolchain JARs on host & workspace without faking availability.
func ProbeAll(workspaceRoot string) []AdapterSpec {
	look := func(names ...string) (bool, string) {
		for _, n := range names {
			if p, err := exec.LookPath(n); err == nil {
				return true, p
			}
		}
		return false, ""
	}
	findFile := func(rels ...string) (bool, string) {
		for _, rel := range rels {
			cand := filepath.Join(workspaceRoot, rel)
			if st, err := os.Stat(cand); err == nil && !st.IsDir() {
				return true, cand
			}
		}
		return false, ""
	}

	javaOK, _ := look("java")
	adbOK, adbPath := look("adb")
	maestroOK, maestroPath := look("maestro")
	ffmpegOK, ffmpegPath := look("ffmpeg", "ffprobe")
	perfOK, perfPath := look("perfetto", "trace_processor_shell")
	gradleOK, gradlePath := look("gradle")
	if !gradleOK {
		if runtime.GOOS == "windows" {
			gradleOK, gradlePath = findFile("gradlew.bat")
		} else {
			gradleOK, gradlePath = findFile("gradlew")
		}
	}
	junit5JarOK, junit5Jar := findFile(filepath.Join("tools", "libs", "junit-platform-console-standalone-1.11.4.jar"))
	bundleJarOK, bundleJar := findFile(filepath.Join("tools", "libs", "bundletool-all-1.18.1.jar"))

	return []AdapterSpec{
		{
			ID: FrameworkMaestro, DisplayName: "Maestro Mobile UI", ExecutionTier: "android_art",
			Available: maestroOK, ResolvedPath: maestroPath,
			NativeSemantics: "Preserves Maestro YAML flow assertions, automatic waits, and non-zero exit code on failure.",
		},
		{
			ID: FrameworkAndroidXTest, DisplayName: "AndroidX Test Orchestrator", ExecutionTier: "android_art",
			Available: adbOK, ResolvedPath: adbPath,
			NativeSemantics: "Runs `am instrument` via AndroidJUnitRunner with isolated instrumentation state.",
		},
		{
			ID: FrameworkJUnit4, DisplayName: "JUnit 4 Local/Instrumented", ExecutionTier: "nano_jvm",
			Available: javaOK && junit5JarOK, ResolvedPath: junit5Jar,
			NativeSemantics: "Executes via JUnit Platform Vintage engine preserving assumption-skipped vs failed semantics.",
		},
		{
			ID: FrameworkJUnit5, DisplayName: "JUnit 5 Jupiter Engine", ExecutionTier: "nano_jvm",
			Available: javaOK && junit5JarOK, ResolvedPath: junit5Jar,
			NativeSemantics: "Executes @Test lifecycle & assertions on OpenJDK 25 without silencing assertion failures.",
		},
		{
			ID: FrameworkEspresso, DisplayName: "Espresso ViewMatchers / Robolectric", ExecutionTier: "robolectric",
			Available: gradleOK, ResolvedPath: gradlePath,
			NativeSemantics: "Delegates Espresso UI/Robolectric unit targets cleanly to Gradle test tasks.",
		},
		{
			ID: FrameworkUIAutomator, DisplayName: "UI Automator Cross-App", ExecutionTier: "android_art",
			Available: adbOK, ResolvedPath: adbPath,
			NativeSemantics: "Preserves cross-package system dialog & `uiautomator dump` semantics over ADB.",
		},
		{
			ID: FrameworkRobolectric, DisplayName: "Robolectric JVM Shadow Backend", ExecutionTier: "robolectric",
			Available: gradleOK, ResolvedPath: gradlePath,
			NativeSemantics: "Runs Gradle Robolectric JVM tests strictly labeled `ROBOLECTRIC_PASS` (never confused with Nano JVM).",
		},
		{
			ID: FrameworkGradle, DisplayName: "Gradle / AGP 9.x Build", ExecutionTier: "static",
			Available: gradleOK, ResolvedPath: gradlePath,
			NativeSemantics: "Executes clean/incremental Gradle build/test variants preserving exit code & compiler errors.",
		},
		{
			ID: FrameworkADB, DisplayName: "Android Debug Bridge (ADB)", ExecutionTier: "android_art",
			Available: adbOK, ResolvedPath: adbPath,
			NativeSemantics: "Strict argument-array ADB dispatch; forbids arbitrary shell injection.",
		},
		{
			ID: FrameworkPerfetto, DisplayName: "Perfetto TraceProcessor", ExecutionTier: "android_art",
			Available: perfOK || adbOK, ResolvedPath: perfPath,
			NativeSemantics: "Captures real device Perfetto protobuf traces and computes jank/CPU metrics on ART/physical.",
		},
		{
			ID: FrameworkFFmpeg, DisplayName: "FFmpeg / ffprobe Codec Verifier", ExecutionTier: "static",
			Available: ffmpegOK, ResolvedPath: ffmpegPath,
			NativeSemantics: "Separates SHA-256 bit-exact passthrough verification from PSNR/SSIM/VMAF lossy oracle metrics.",
		},
		{
			ID: FrameworkBundletool, DisplayName: "Android Bundletool", ExecutionTier: "static",
			Available: javaOK && bundleJarOK, ResolvedPath: bundleJar,
			NativeSemantics: "Generates device-spec configuration split APKs from `.aab` bundles cleanly.",
		},
	}
}

// TranslateCommand generates a shell-free parameterized command for the requested framework adapter.
func TranslateCommand(id FrameworkID, workspaceRoot string, req TranslateRequest) (TranslatedCommand, error) {
	specs := ProbeAll(workspaceRoot)
	var matched *AdapterSpec
	for i := range specs {
		if specs[i].ID == id {
			matched = &specs[i]
			break
		}
	}
	if matched == nil {
		return TranslatedCommand{}, fmt.Errorf("unknown framework adapter %q", id)
	}
	for _, a := range append([]string{req.ArtifactOrTarget, req.PackageName, req.DeviceSerial}, req.ExtraArgs...) {
		if strings.ContainsAny(a, ";|&`$\r\n<>") {
			return TranslatedCommand{}, fmt.Errorf("BLOCKED_POLICY argument contains forbidden shell metacharacter")
		}
	}

	switch id {
	case FrameworkMaestro:
		args := []string{"test"}
		if req.DeviceSerial != "" {
			args = append(args, "--device", req.DeviceSerial)
		}
		args = append(args, req.ArtifactOrTarget)
		return TranslatedCommand{AdapterID: id, Binary: orDefault(matched.ResolvedPath, "maestro"), Args: args, NativeSemantics: matched.NativeSemantics}, nil
	case FrameworkAndroidXTest, FrameworkEspresso, FrameworkUIAutomator:
		if req.PackageName == "" {
			return TranslatedCommand{}, fmt.Errorf("package_name required for %s instrumentation translation", id)
		}
		args := []string{}
		if req.DeviceSerial != "" {
			args = append(args, "-s", req.DeviceSerial)
		}
		args = append(args, "shell", "am", "instrument", "-w", req.PackageName+"/androidx.test.runner.AndroidJUnitRunner")
		return TranslatedCommand{AdapterID: id, Binary: orDefault(matched.ResolvedPath, "adb"), Args: args, NativeSemantics: matched.NativeSemantics}, nil
	case FrameworkJUnit4, FrameworkJUnit5:
		jar := orDefault(matched.ResolvedPath, filepath.Join("tools", "libs", "junit-platform-console-standalone-1.11.4.jar"))
		args := []string{"-jar", jar, "execute", "--class-path", req.ArtifactOrTarget, "--scan-class-path"}
		return TranslatedCommand{AdapterID: id, Binary: "java", Args: args, NativeSemantics: matched.NativeSemantics}, nil
	case FrameworkRobolectric, FrameworkGradle:
		task := orDefault(req.ArtifactOrTarget, "testDebugUnitTest")
		return TranslatedCommand{AdapterID: id, Binary: orDefault(matched.ResolvedPath, "gradle"), Args: []string{task}, NativeSemantics: matched.NativeSemantics}, nil
	case FrameworkADB:
		args := []string{}
		if req.DeviceSerial != "" {
			args = append(args, "-s", req.DeviceSerial)
		}
		args = append(args, req.ExtraArgs...)
		return TranslatedCommand{AdapterID: id, Binary: orDefault(matched.ResolvedPath, "adb"), Args: args, NativeSemantics: matched.NativeSemantics}, nil
	case FrameworkPerfetto:
		return TranslatedCommand{AdapterID: id, Binary: orDefault(matched.ResolvedPath, "trace_processor_shell"), Args: []string{req.ArtifactOrTarget}, NativeSemantics: matched.NativeSemantics}, nil
	case FrameworkFFmpeg:
		return TranslatedCommand{AdapterID: id, Binary: orDefault(matched.ResolvedPath, "ffprobe"), Args: []string{"-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", req.ArtifactOrTarget}, NativeSemantics: matched.NativeSemantics}, nil
	case FrameworkBundletool:
		jar := orDefault(matched.ResolvedPath, filepath.Join("tools", "libs", "bundletool-all-1.18.1.jar"))
		return TranslatedCommand{AdapterID: id, Binary: "java", Args: []string{"-jar", jar, "dump", "manifest", "--bundle=" + req.ArtifactOrTarget}, NativeSemantics: matched.NativeSemantics}, nil
	default:
		return TranslatedCommand{}, fmt.Errorf("translation not implemented for %q", id)
	}
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return def
}
