package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseJUnitXML_SuccessAndFailures(t *testing.T) {
	tempDir := t.TempDir()

	xmlSuite1 := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.example.scamshield.SmsClassifierTest" tests="3" skipped="1" failures="0" errors="0" time="0.45">
  <testcase name="testNormalSms" classname="com.example.scamshield.SmsClassifierTest" time="0.15"/>
  <testcase name="testBankAlertSms" classname="com.example.scamshield.SmsClassifierTest" time="0.20"/>
  <testcase name="testPendingModelUpdate" classname="com.example.scamshield.SmsClassifierTest" time="0.10">
    <skipped/>
  </testcase>
</testsuite>
`

	xmlSuite2 := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.example.scamshield.ScamCardScreenshotTest" tests="2" skipped="0" failures="1" errors="0" time="1.85">
  <testcase name="testCardExpanded" classname="com.example.scamshield.ScamCardScreenshotTest" time="0.85"/>
  <testcase name="testCardCollapsed" classname="com.example.scamshield.ScamCardScreenshotTest" time="1.00">
    <failure message="Visual regression detected" type="java.lang.AssertionError">
java.lang.AssertionError: Visual difference exceeds 0.1%
Roborazzi diff generated: build/outputs/roborazzi/com_example_scamshield_ScamCardScreenshotTest_testCardCollapsed_compare.png
	at com.github.takahirom.roborazzi.Roborazzi.check(Roborazzi.kt:42)
	at com.example.scamshield.ScamCardScreenshotTest.testCardCollapsed(ScamCardScreenshotTest.kt:28)
    </failure>
  </testcase>
</testsuite>
`

	if err := os.WriteFile(filepath.Join(tempDir, "TEST-SmsClassifierTest.xml"), []byte(xmlSuite1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "TEST-ScamCardScreenshotTest.xml"), []byte(xmlSuite2), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseJUnitXML(tempDir)
	if err != nil {
		t.Fatalf("ParseJUnitXML failed: %v", err)
	}

	if result.Passed != 3 {
		t.Errorf("expected 3 passed tests, got %d", result.Passed)
	}
	if result.Failed != 1 {
		t.Errorf("expected 1 failed test, got %d", result.Failed)
	}
	if result.Skipped != 1 {
		t.Errorf("expected 1 skipped test, got %d", result.Skipped)
	}
	if result.DurationMS < 2000 {
		t.Errorf("expected >= 2000 ms duration, got %d", result.DurationMS)
	}

	if len(result.Failures) != 1 {
		t.Fatalf("expected 1 failure entry, got %d", len(result.Failures))
	}

	fail := result.Failures[0]
	if fail.Class != "com.example.scamshield.ScamCardScreenshotTest" {
		t.Errorf("expected failure class 'com.example.scamshield.ScamCardScreenshotTest', got %s", fail.Class)
	}
	if fail.Method != "testCardCollapsed" {
		t.Errorf("expected failure method 'testCardCollapsed', got %s", fail.Method)
	}
	if !strings.Contains(fail.StackTrace, "Visual difference exceeds") {
		t.Errorf("expected stack trace to contain failure text, got %s", fail.StackTrace)
	}
	if !strings.Contains(fail.DiffImgPath, "com_example_scamshield_ScamCardScreenshotTest_testCardCollapsed_compare.png") {
		t.Errorf("expected diff image path extracted from Roborazzi, got %q", fail.DiffImgPath)
	}
}

func TestGradleEngine_TestJVM_Execution(t *testing.T) {
	tempDir := t.TempDir()

	testResultsDir := filepath.Join(tempDir, "app", "build", "test-results")
	if err := os.MkdirAll(testResultsDir, 0755); err != nil {
		t.Fatal(err)
	}

	xmlSuite := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.example.app.MathTest" tests="2" skipped="0" failures="0" errors="0" time="0.10">
  <testcase name="testAdd" classname="com.example.app.MathTest" time="0.05"/>
  <testcase name="testSub" classname="com.example.app.MathTest" time="0.05"/>
</testsuite>`
	if err := os.WriteFile(filepath.Join(testResultsDir, "TEST-MathTest.xml"), []byte(xmlSuite), 0644); err != nil {
		t.Fatal(err)
	}

	mockExec := NewMockCommandExecutor()
	mockExec.RegisterHandler("gradlew.bat", func(dir string, env []string, args []string) ([]byte, error) {
		return []byte("BUILD SUCCESSFUL in 1s\n"), nil
	})
	mockExec.RegisterHandler("./gradlew", func(dir string, env []string, args []string) ([]byte, error) {
		return []byte("BUILD SUCCESSFUL in 1s\n"), nil
	})

	engine := NewGradleEngine(tempDir, 2048, WithExecutor(mockExec))

	res, err := engine.TestJVM("com.example.app.*", true, false)
	if err != nil {
		t.Fatalf("TestJVM failed: %v", err)
	}

	if res.Passed != 2 || res.Failed != 0 || res.Skipped != 0 {
		t.Errorf("unexpected test result: %+v", res)
	}

	if len(mockExec.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mockExec.Calls))
	}
	call := mockExec.Calls[0]
	joined := strings.Join(call.Args, " ")
	if !strings.Contains(joined, ":app:testDebugUnitTest") {
		t.Errorf("expected :app:testDebugUnitTest in args, got %s", joined)
	}
	if !strings.Contains(joined, "--tests com.example.app.*") {
		t.Errorf("expected filter in args, got %s", joined)
	}
	if !strings.Contains(joined, ":app:compareRoborazziDebug") {
		t.Errorf("expected :app:compareRoborazziDebug in args, got %s", joined)
	}
}
