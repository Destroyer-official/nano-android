package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegration_FullComposeAppWorkflow(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Structure a sample Compose app workspace
	appDir := filepath.Join(tempDir, "app")
	apkDir := filepath.Join(appDir, "build", "outputs", "apk", "debug")
	testResultsDir := filepath.Join(appDir, "build", "test-results", "testDebugUnitTest")
	if err := os.MkdirAll(apkDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(testResultsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 2. Generate a valid Compose APK in the output directory
	apkPath := filepath.Join(apkDir, "app-debug.apk")
	manifest := `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="com.example.composeapp"
    android:versionCode="1"
    android:versionName="1.0.0">
    <uses-sdk android:minSdkVersion="26" android:targetSdkVersion="37" />
    <uses-permission android:name="android.permission.INTERNET" />
    <application
        android:label="ComposeApp"
        android:resizeableActivity="true">
        <activity
            android:name=".MainActivity"
            android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
            <intent-filter>
                <action android:name="android.intent.action.VIEW" />
                <category android:name="android.intent.category.DEFAULT" />
                <data android:scheme="composeapp" />
            </intent-filter>
        </activity>
    </application>
</manifest>`

	soFiles := map[string][]byte{
		"lib/arm64-v8a/libcomposenative.so": createMockELF64(16384),
	}
	createTestAPK(t, apkPath, []byte(manifest), soFiles, nil)

	// Compute expected SHA-256 of the generated APK
	apkBytes, err := os.ReadFile(apkPath)
	if err != nil {
		t.Fatal(err)
	}
	expectedHash := sha256.Sum256(apkBytes)
	expectedHashHex := hex.EncodeToString(expectedHash[:])

	// 3. Write JUnit XML results
	junitXML := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.example.composeapp.ComposeUiTest" tests="2" skipped="0" failures="0" errors="0" time="1.25">
  <testcase name="testGreetingTextDisplayed" classname="com.example.composeapp.ComposeUiTest" time="0.65"/>
  <testcase name="testButtonTriggersAction" classname="com.example.composeapp.ComposeUiTest" time="0.60"/>
</testsuite>`
	if err := os.WriteFile(filepath.Join(testResultsDir, "TEST-ComposeUiTest.xml"), []byte(junitXML), 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Configure MockCommandExecutor to simulate gradle wrapper
	mockExec := NewMockCommandExecutor()
	mockExec.RegisterHandler("gradlew.bat", func(dir string, env []string, args []string) ([]byte, error) {
		return []byte("BUILD SUCCESSFUL in 4s\n"), nil
	})
	mockExec.RegisterHandler("./gradlew", func(dir string, env []string, args []string) ([]byte, error) {
		return []byte("BUILD SUCCESSFUL in 4s\n"), nil
	})

	engine := NewGradleEngine(tempDir, 4096, WithExecutor(mockExec))

	// -------------------------------------------------------------
	// Tool 1: project_build integration test
	// -------------------------------------------------------------
	buildHandler := NewProjectBuildHandler(engine)
	buildRes, err := buildHandler.Execute(context.Background(), map[string]any{
		"variant": "debug",
		"module":  "app",
		"clean":   true,
	})
	if err != nil {
		t.Fatalf("project_build tool failed: %v", err)
	}
	if !buildRes.Success {
		t.Fatalf("project_build was not successful: %s", buildRes.Error)
	}
	if buildRes.Data["status"] != "success" {
		t.Errorf("expected status 'success', got %v", buildRes.Data["status"])
	}
	if buildRes.Data["apk_path"] != apkPath {
		t.Errorf("expected APK path %s, got %v", apkPath, buildRes.Data["apk_path"])
	}
	if buildRes.Data["apk_sha256"] != expectedHashHex {
		t.Errorf("expected APK SHA-256 %s, got %v", expectedHashHex, buildRes.Data["apk_sha256"])
	}

	// -------------------------------------------------------------
	// Tool 2: project_test_jvm integration test
	// -------------------------------------------------------------
	testHandler := NewProjectTestJVMHandler(engine)
	testRes, err := testHandler.Execute(context.Background(), map[string]any{
		"test_filter":       "com.example.composeapp.*",
		"include_roborazzi": false,
	})
	if err != nil {
		t.Fatalf("project_test_jvm tool failed: %v", err)
	}
	if !testRes.Success {
		t.Fatalf("project_test_jvm was not successful: %s", testRes.Error)
	}
	if testRes.Data["passed"] != 2 {
		t.Errorf("expected 2 passed tests, got %v", testRes.Data["passed"])
	}
	if testRes.Data["failed"] != 0 {
		t.Errorf("expected 0 failed tests, got %v", testRes.Data["failed"])
	}

	// -------------------------------------------------------------
	// Tool 3: apk_inspect integration test
	// -------------------------------------------------------------
	inspectHandler := NewAPKInspectHandler(engine)
	inspectRes, err := inspectHandler.Execute(context.Background(), map[string]any{
		"apk_path": apkPath,
	})
	if err != nil {
		t.Fatalf("apk_inspect tool failed: %v", err)
	}
	if !inspectRes.Success {
		t.Fatalf("apk_inspect was not successful: %s", inspectRes.Error)
	}
	if inspectRes.Data["package_id"] != "com.example.composeapp" {
		t.Errorf("expected package com.example.composeapp, got %v", inspectRes.Data["package_id"])
	}
	if inspectRes.Data["launcher_activity"] != ".MainActivity" && !strings.HasSuffix(inspectRes.Data["launcher_activity"].(string), "MainActivity") {
		t.Errorf("expected launcher MainActivity, got %v", inspectRes.Data["launcher_activity"])
	}

	// -------------------------------------------------------------
	// Tool 4: apk_audit_compliance integration test
	// -------------------------------------------------------------
	complianceHandler := NewAPKAuditComplianceHandler(engine)
	complianceRes, err := complianceHandler.Execute(context.Background(), map[string]any{
		"apk_path": apkPath,
	})
	if err != nil {
		t.Fatalf("apk_audit_compliance tool failed: %v", err)
	}
	if !complianceRes.Success {
		t.Fatalf("apk_audit_compliance was not successful: %s", complianceRes.Error)
	}
	if complianceRes.Data["pass"] != true {
		t.Errorf("expected pass=true, got %v", complianceRes.Data["pass"])
	}
	if complianceRes.Data["elf_16kb_aligned"] != true {
		t.Errorf("expected elf_16kb_aligned=true, got %v", complianceRes.Data["elf_16kb_aligned"])
	}
	if complianceRes.Data["large_screen_resizable"] != true {
		t.Errorf("expected large_screen_resizable=true, got %v", complianceRes.Data["large_screen_resizable"])
	}

	// Test RegisterTools
	tools := RegisterTools(engine)
	if len(tools) != 4 {
		t.Errorf("expected 4 registered tools, got %d", len(tools))
	}
}
