package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGradleBuildOutput_Success(t *testing.T) {
	output := `
> Task :app:preBuild UP-TO-DATE
> Task :app:compileDebugKotlin
> Task :app:compileDebugJavaWithJavac
> Task :app:packageDebug
> Task :app:assembleDebug
APK: app/build/outputs/apk/debug/app-debug.apk

BUILD SUCCESSFUL in 12s
35 actionable tasks: 4 executed, 31 up-to-date
`
	res := ParseGradleBuildOutput(output, 0, 12000)

	if res.Status != "success" {
		t.Fatalf("expected status 'success', got %q", res.Status)
	}
	if res.DurationMS != 12000 {
		t.Errorf("expected duration 12000 ms, got %d", res.DurationMS)
	}
	if len(res.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(res.Errors))
	}
	if !strings.HasSuffix(res.APKPath, "app-debug.apk") {
		t.Errorf("expected APK path to end with app-debug.apk, got %q", res.APKPath)
	}
}

func TestParseGradleBuildOutput_FailureWithErrors(t *testing.T) {
	output := `
> Task :app:compileDebugKotlin FAILED
e: /workspace/app/src/main/kotlin/com/example/scamshield/MainActivity.kt: (24, 15): Unresolved reference: computeHash
e: /workspace/app/src/main/kotlin/com/example/scamshield/SmsReceiver.kt:42:9: Type mismatch: inferred type is String but Int was expected
/workspace/app/src/main/java/com/example/scamshield/LegacyBridge.java:18: error: cannot find symbol

BUILD FAILED in 1s 200ms
`
	res := ParseGradleBuildOutput(output, 1, 1200)

	if res.Status != "failure" {
		t.Fatalf("expected status 'failure', got %q", res.Status)
	}
	if len(res.Errors) != 3 {
		t.Fatalf("expected 3 compiler errors, got %d", len(res.Errors))
	}

	err1 := res.Errors[0]
	if !strings.HasSuffix(err1.File, "MainActivity.kt") || err1.Line != 24 || !strings.Contains(err1.Message, "Unresolved reference") {
		t.Errorf("unexpected error 1: %+v", err1)
	}

	err2 := res.Errors[1]
	if !strings.HasSuffix(err2.File, "SmsReceiver.kt") || err2.Line != 42 || !strings.Contains(err2.Message, "Type mismatch") {
		t.Errorf("unexpected error 2: %+v", err2)
	}

	err3 := res.Errors[2]
	if !strings.HasSuffix(err3.File, "LegacyBridge.java") || err3.Line != 18 || !strings.Contains(err3.Message, "cannot find symbol") {
		t.Errorf("unexpected error 3: %+v", err3)
	}
}

func TestGradleEngine_Build_Execution(t *testing.T) {
	tempDir := t.TempDir()

	// Create mock output directory and APK file
	apkDir := filepath.Join(tempDir, "app", "build", "outputs", "apk", "debug")
	if err := os.MkdirAll(apkDir, 0755); err != nil {
		t.Fatal(err)
	}
	mockAPKPath := filepath.Join(apkDir, "app-debug.apk")
	if err := os.WriteFile(mockAPKPath, []byte("MOCK_APK_BINARY_CONTENT"), 0644); err != nil {
		t.Fatal(err)
	}

	mockExec := NewMockCommandExecutor()
	mockExec.RegisterHandler("gradlew.bat", func(dir string, env []string, args []string) ([]byte, error) {
		return []byte("BUILD SUCCESSFUL in 3s\n"), nil
	})
	mockExec.RegisterHandler("./gradlew", func(dir string, env []string, args []string) ([]byte, error) {
		return []byte("BUILD SUCCESSFUL in 3s\n"), nil
	})

	engine := NewGradleEngine(tempDir, 4096, WithExecutor(mockExec))

	res, err := engine.Build("debug", "app", true)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if res.Status != "success" {
		t.Errorf("expected success, got %s", res.Status)
	}
	if res.APKPath == "" {
		t.Errorf("expected APK path to be resolved")
	}
	if len(res.APKSHA256) != 64 {
		t.Errorf("expected 64-char SHA256 hex, got %q", res.APKSHA256)
	}

	// Verify command calls
	if len(mockExec.Calls) != 1 {
		t.Fatalf("expected 1 command call, got %d", len(mockExec.Calls))
	}
	call := mockExec.Calls[0]
	joinedArgs := strings.Join(call.Args, " ")
	if !strings.Contains(joinedArgs, "clean") {
		t.Errorf("expected clean flag, got %s", joinedArgs)
	}
	if !strings.Contains(joinedArgs, ":app:assembleDebug") {
		t.Errorf("expected :app:assembleDebug, got %s", joinedArgs)
	}
	if !strings.Contains(joinedArgs, "--configuration-cache") || !strings.Contains(joinedArgs, "--build-cache") {
		t.Errorf("expected cache flags, got %s", joinedArgs)
	}

	// Verify JAVA_OPTS
	hasJavaOpts := false
	for _, env := range call.Env {
		if strings.Contains(env, "JAVA_OPTS=-Xmx4096m") {
			hasJavaOpts = true
			break
		}
	}
	if !hasJavaOpts {
		t.Errorf("expected JAVA_OPTS=-Xmx4096m in env, got %v", call.Env)
	}
}
