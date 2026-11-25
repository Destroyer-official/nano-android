package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanOpenSourceAPKs_ZeroFalsePositives(t *testing.T) {
	tempDir := t.TempDir()

	// -------------------------------------------------------------
	// APK 1: Clean Family Scam Shield APK (targetSdk 37, justified notification listener & location, 16KB ARM64 .so)
	// -------------------------------------------------------------
	apk1Path := filepath.Join(tempDir, "clean_scamshield.apk")
	manifest1 := `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="com.example.scamshield"
    android:versionCode="100"
    android:versionName="1.0.0">
    <uses-sdk android:minSdkVersion="26" android:targetSdkVersion="37" />
    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.BIND_NOTIFICATION_LISTENER_SERVICE" />
    <uses-permission android:name="android.permission.ACCESS_FINE_LOCATION" />
    <application
        android:label="ScamShield"
        android:resizeableActivity="true">
        <activity
            android:name=".MainActivity"
            android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
        <service
            android:name=".ScamNotificationListener"
            android:permission="android.permission.BIND_NOTIFICATION_LISTENER_SERVICE"
            android:exported="true">
            <intent-filter>
                <action android:name="android.service.notification.NotificationListenerService" />
            </intent-filter>
        </service>
    </application>
</manifest>`

	yaml1Content := `
schema_version: 1
app_package: "com.example.scamshield"
target_sdk: 37
restricted_permissions:
  - permission: "android.permission.BIND_NOTIFICATION_LISTENER_SERVICE"
    approved_role: "user_opt_in_scam_alerting"
    in_app_disclosure_screen_test: ".droidforge/flows/disclosure_notification_access.yaml"
    android17_otp_fallback: true
    notes: "Primary real-time scam URL detection path; avoids READ_SMS 3-hour OTP delay on API 37."
  - permission: "android.permission.ACCESS_FINE_LOCATION"
    in_app_disclosure_screen_test: ".droidforge/flows/disclosure_location.yaml"
    notes: "Location button rationale present for emergency relative proximity alert."
`
	yaml1Path := filepath.Join(tempDir, "scamshield-policy.yaml")
	if err := os.WriteFile(yaml1Path, []byte(yaml1Content), 0644); err != nil {
		t.Fatal(err)
	}

	soFiles1 := map[string][]byte{
		"lib/arm64-v8a/libscamdetector.so": createMockELF64(16384),
	}
	createTestAPK(t, apk1Path, []byte(manifest1), soFiles1, nil)

	// -------------------------------------------------------------
	// APK 2: Clean Video Compression App (targetSdk 37, 16KB native ARM64 engine, resizable)
	// -------------------------------------------------------------
	apk2Path := filepath.Join(tempDir, "clean_videocodec.apk")
	manifest2 := `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="com.example.videocodec"
    android:versionCode="20"
    android:versionName="2.0.0">
    <uses-sdk android:minSdkVersion="28" android:targetSdkVersion="37" />
    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.READ_MEDIA_VIDEO" />
    <application
        android:label="VideoCompressor"
        android:resizeableActivity="true">
        <activity
            android:name=".MainActivity"
            android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>`

	soFiles2 := map[string][]byte{
		"lib/arm64-v8a/libavcodec.so": createMockELF64(16384),
		"lib/arm64-v8a/libavutil.so":  createMockELF64(16384),
	}
	createTestAPK(t, apk2Path, []byte(manifest2), soFiles2, nil)

	// -------------------------------------------------------------
	// APK 3: Clean Utilities Open-Source App (targetSdk 35, pure JVM Compose, resizable)
	// -------------------------------------------------------------
	apk3Path := filepath.Join(tempDir, "clean_utility.apk")
	manifest3 := `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="org.example.utilities"
    android:versionCode="5"
    android:versionName="1.0.5">
    <uses-sdk android:minSdkVersion="24" android:targetSdkVersion="35" />
    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.POST_NOTIFICATIONS" />
    <application
        android:label="QuickTools"
        android:resizeableActivity="true">
        <activity
            android:name=".ui.MainActivity"
            android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>`

	createTestAPK(t, apk3Path, []byte(manifest3), nil, nil)

	// -------------------------------------------------------------
	// Verify all 3 clean APKs
	// -------------------------------------------------------------
	engine := NewGradleEngine(tempDir, 2048)

	// Test APK 1
	res1, err := engine.AuditCompliance(apk1Path, yaml1Path)
	if err != nil {
		t.Fatalf("AuditCompliance failed on clean APK 1: %v", err)
	}
	if !res1.Pass {
		t.Errorf("APK 1 (ScamShield) expected pass=true, got pass=false. Findings: %+v, %+v",
			res1.PlayPolicyFindings, res1.Android17Findings)
	}
	if !res1.ELF16KBAligned {
		t.Errorf("APK 1 expected ELF16KBAligned=true")
	}
	if !res1.LargeScreenResizable {
		t.Errorf("APK 1 expected LargeScreenResizable=true")
	}
	for _, f := range res1.PlayPolicyFindings {
		if f.Severity == "error" {
			t.Errorf("APK 1 unexpected Play Policy error: %+v", f)
		}
	}
	for _, f := range res1.Android17Findings {
		if f.Severity == "error" {
			t.Errorf("APK 1 unexpected Android 17 error: %+v", f)
		}
	}

	// Test APK 2
	res2, err := engine.AuditCompliance(apk2Path, "")
	if err != nil {
		t.Fatalf("AuditCompliance failed on clean APK 2: %v", err)
	}
	if !res2.Pass {
		t.Errorf("APK 2 (VideoCodec) expected pass=true, got pass=false. Findings: %+v, %+v",
			res2.PlayPolicyFindings, res2.Android17Findings)
	}
	if !res2.ELF16KBAligned {
		t.Errorf("APK 2 expected ELF16KBAligned=true")
	}
	if !res2.LargeScreenResizable {
		t.Errorf("APK 2 expected LargeScreenResizable=true")
	}
	if len(res2.PlayPolicyFindings) != 0 {
		t.Errorf("APK 2 unexpected Play Policy findings: %+v", res2.PlayPolicyFindings)
	}
	if len(res2.Android17Findings) != 0 {
		t.Errorf("APK 2 unexpected Android 17 findings: %+v", res2.Android17Findings)
	}

	// Test APK 3
	res3, err := engine.AuditCompliance(apk3Path, "")
	if err != nil {
		t.Fatalf("AuditCompliance failed on clean APK 3: %v", err)
	}
	if !res3.Pass {
		t.Errorf("APK 3 (Utility App) expected pass=true, got pass=false. Findings: %+v, %+v",
			res3.PlayPolicyFindings, res3.Android17Findings)
	}
	if !res3.ELF16KBAligned {
		t.Errorf("APK 3 expected ELF16KBAligned=true")
	}
	if !res3.LargeScreenResizable {
		t.Errorf("APK 3 expected LargeScreenResizable=true")
	}
	if len(res3.PlayPolicyFindings) != 0 {
		t.Errorf("APK 3 unexpected Play Policy findings: %+v", res3.PlayPolicyFindings)
	}
	if len(res3.Android17Findings) != 0 {
		t.Errorf("APK 3 unexpected Android 17 findings: %+v", res3.Android17Findings)
	}
}
