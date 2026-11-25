package build

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckELF16KBAlignment(t *testing.T) {
	// 1. Aligned 16 KB ELF
	alignedELF := createMockELF64(16384)
	repAligned, err := CheckELF16KBAlignment(bytes.NewReader(alignedELF))
	if err != nil {
		t.Fatalf("CheckELF16KBAlignment failed: %v", err)
	}
	if !repAligned.Aligned {
		t.Errorf("expected aligned=true, got false: %v", repAligned.Violations)
	}
	if repAligned.MinAlign != 16384 {
		t.Errorf("expected minAlign=16384, got %d", repAligned.MinAlign)
	}

	// 2. Misaligned 4 KB ELF
	misalignedELF := createMockELF64(4096)
	repMisaligned, err := CheckELF16KBAlignment(bytes.NewReader(misalignedELF))
	if err != nil {
		t.Fatalf("CheckELF16KBAlignment failed: %v", err)
	}
	if repMisaligned.Aligned {
		t.Errorf("expected aligned=false for 4 KB alignment, got true")
	}
	if repMisaligned.MinAlign != 4096 {
		t.Errorf("expected minAlign=4096, got %d", repMisaligned.MinAlign)
	}
	if len(repMisaligned.Violations) == 0 {
		t.Errorf("expected violations for 4 KB alignment")
	}
}

func TestParseReadelfOutput(t *testing.T) {
	outputAligned := `
Program Headers:
  Type           Offset             VirtAddr           PhysAddr
                 FileSiz            MemSiz              Flags  Align
  LOAD           0x0000000000000000 0x0000000000000000 0x0000000000000000
                 0x0000000000001000 0x0000000000001000  R E    0x4000
  LOAD           0x0000000000004000 0x0000000000004000 0x0000000000004000
                 0x0000000000000500 0x0000000000000500  RW     0x4000
`
	rep1, err := ParseReadelfOutput(outputAligned)
	if err != nil || !rep1.Aligned || rep1.MinAlign != 16384 {
		t.Errorf("expected aligned report with 16384 minAlign, got %+v (err=%v)", rep1, err)
	}

	outputMisaligned := `
Program Headers:
  Type           Offset   VirtAddr   PhysAddr   FileSiz MemSiz  Flg Align
  LOAD           0x000000 0x00000000 0x00000000 0x01000 0x01000 R E 0x1000
`
	rep2, err := ParseReadelfOutput(outputMisaligned)
	if err != nil || rep2.Aligned || rep2.MinAlign != 4096 {
		t.Errorf("expected misaligned report with 4096 minAlign, got %+v (err=%v)", rep2, err)
	}
}

func TestAuditCompliance_Planted16KBMisalignment(t *testing.T) {
	tempDir := t.TempDir()
	apkPath := filepath.Join(tempDir, "misaligned.apk")

	manifest := `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="com.example.videocodec"
    android:versionCode="1"
    android:versionName="1.0">
    <uses-sdk android:minSdkVersion="26" android:targetSdkVersion="37" />
    <application android:resizeableActivity="true">
        <activity android:name=".MainActivity" android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>`

	// Plant a 4 KB aligned .so inside lib/arm64-v8a/
	soFiles := map[string][]byte{
		"lib/arm64-v8a/libcodec.so": createMockELF64(4096),
	}

	createTestAPK(t, apkPath, []byte(manifest), soFiles, nil)

	engine := NewGradleEngine(tempDir, 2048)
	res, err := engine.AuditCompliance(apkPath, "")
	if err != nil {
		t.Fatalf("AuditCompliance failed: %v", err)
	}

	if res.Pass {
		t.Errorf("expected compliance check to FAIL for 4 KB misaligned ELF, got pass=true")
	}
	if res.ELF16KBAligned {
		t.Errorf("expected ELF16KBAligned=false, got true")
	}

	hasELFError := false
	for _, f := range res.Android17Findings {
		if f.Rule == "ELF_16KB_PAGE_ALIGNMENT" && f.Severity == "error" {
			hasELFError = true
			break
		}
	}
	if !hasELFError {
		t.Errorf("expected ELF_16KB_PAGE_ALIGNMENT error finding in Android17Findings: %+v", res.Android17Findings)
	}
}

func TestAuditCompliance_PlantedUndeclaredReadCallLog(t *testing.T) {
	tempDir := t.TempDir()
	apkPath := filepath.Join(tempDir, "undeclared_calllog.apk")

	manifest := `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="com.example.scamshield"
    android:versionCode="1"
    android:versionName="1.0">
    <uses-sdk android:minSdkVersion="26" android:targetSdkVersion="37" />
    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.READ_CALL_LOG" />
    <application android:resizeableActivity="true">
        <activity android:name=".MainActivity" android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>`

	// Create justification file where READ_CALL_LOG is PROHIBITED
	yamlContent := `
schema_version: 1
app_package: "com.example.scamshield"
target_sdk: 37
restricted_permissions:
  - permission: "android.permission.READ_CALL_LOG"
    status: "PROHIBITED"
    fail_ci_if_present: true
`
	yamlPath := filepath.Join(tempDir, "play-policy-justification.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	createTestAPK(t, apkPath, []byte(manifest), nil, nil)

	engine := NewGradleEngine(tempDir, 2048)
	res, err := engine.AuditCompliance(apkPath, yamlPath)
	if err != nil {
		t.Fatalf("AuditCompliance failed: %v", err)
	}

	if res.Pass {
		t.Errorf("expected compliance check to FAIL for prohibited READ_CALL_LOG, got pass=true")
	}

	hasCallLogError := false
	for _, f := range res.PlayPolicyFindings {
		if (f.Rule == "PLAY_POLICY_RESTRICTED_PERMISSION" || f.Rule == "PLAY_POLICY_PROHIBITED_PERMISSION") && f.Severity == "error" {
			hasCallLogError = true
			break
		}
	}
	if !hasCallLogError {
		t.Errorf("expected Play Policy error finding for READ_CALL_LOG, got: %+v", res.PlayPolicyFindings)
	}
}

func TestAuditCompliance_PlantedLargeScreenResizabilityViolation(t *testing.T) {
	tempDir := t.TempDir()
	apkPath := filepath.Join(tempDir, "non_resizable.apk")

	// TargetSDK 37 + non-game + android:resizeableActivity="false"
	manifest := `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="com.example.productivity"
    android:versionCode="1"
    android:versionName="1.0">
    <uses-sdk android:minSdkVersion="26" android:targetSdkVersion="37" />
    <application
        android:resizeableActivity="false"
        android:isGame="false">
        <activity android:name=".MainActivity" android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>`

	createTestAPK(t, apkPath, []byte(manifest), nil, nil)

	engine := NewGradleEngine(tempDir, 2048)
	res, err := engine.AuditCompliance(apkPath, "")
	if err != nil {
		t.Fatalf("AuditCompliance failed: %v", err)
	}

	if res.Pass {
		t.Errorf("expected compliance check to FAIL for resizeableActivity=false on API 37 non-game")
	}
	if res.LargeScreenResizable {
		t.Errorf("expected LargeScreenResizable=false, got true")
	}

	hasResizableFinding := false
	for _, f := range res.Android17Findings {
		if f.Rule == "ANDROID_17_LARGE_SCREEN_RESIZABLE" && f.Severity == "error" {
			hasResizableFinding = true
			break
		}
	}
	if !hasResizableFinding {
		t.Errorf("expected ANDROID_17_LARGE_SCREEN_RESIZABLE error finding, got: %+v", res.Android17Findings)
	}
}

func TestAuditCompliance_Android17LocalNetworkAndOTPRisk(t *testing.T) {
	tempDir := t.TempDir()
	apkPath := filepath.Join(tempDir, "local_net_otp.apk")

	manifest := `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="com.example.localnet"
    android:versionCode="1"
    android:versionName="1.0">
    <uses-sdk android:minSdkVersion="26" android:targetSdkVersion="37" />
    <uses-permission android:name="android.permission.READ_SMS" />
    <application android:resizeableActivity="true">
        <activity android:name=".MainActivity" android:exported="true" />
    </application>
</manifest>`

	dexStrings := []string{
		"Ljava/net/Socket;",
		"connectSocket",
		"192.168.1.1",
	}

	createTestAPK(t, apkPath, []byte(manifest), nil, dexStrings)

	engine := NewGradleEngine(tempDir, 2048)
	res, err := engine.AuditCompliance(apkPath, "")
	if err != nil {
		t.Fatalf("AuditCompliance failed: %v", err)
	}

	hasLocalNetFinding := false
	hasOTPDelayFinding := false

	for _, f := range res.Android17Findings {
		if f.Rule == "ANDROID_17_LOCAL_NETWORK" && f.Severity == "error" {
			hasLocalNetFinding = true
		}
		if f.Rule == "ANDROID_17_SMS_OTP_DELAY" && f.Severity == "warning" {
			hasOTPDelayFinding = true
		}
	}

	if !hasLocalNetFinding {
		t.Errorf("expected ANDROID_17_LOCAL_NETWORK error finding, got: %+v", res.Android17Findings)
	}
	if !hasOTPDelayFinding {
		t.Errorf("expected ANDROID_17_SMS_OTP_DELAY warning finding, got: %+v", res.Android17Findings)
	}
}
