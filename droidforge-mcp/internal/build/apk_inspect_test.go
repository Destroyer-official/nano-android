package build

import (
	"path/filepath"
	"testing"
)

func TestParseApksignerOutput(t *testing.T) {
	output := `
Signer #1 certificate DN: CN=Android Debug, O=Android, C=US
Signer #1 certificate SHA-256 digest: a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90
Signer #1 certificate SHA-1 digest: 11223344556677889900aabbccddeeff11223344
Signer #1 key algorithm: RSA
Signer #1 key size (bits): 2048
`
	certs := ParseApksignerOutput(output)
	if len(certs) != 1 {
		t.Fatalf("expected 1 cert, got %d", len(certs))
	}
	expected := "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	if certs[0] != expected {
		t.Errorf("expected cert %s, got %s", expected, certs[0])
	}
}

func TestParseAapt2Badging(t *testing.T) {
	output := `
package: name='com.example.scamshield' versionCode='102' versionName='1.2.0' compileSdkVersion='37' compileSdkVersionCodename='17'
sdkVersion:'26'
targetSdkVersion:'37'
uses-permission: name='android.permission.INTERNET'
uses-permission: name='android.permission.BIND_NOTIFICATION_LISTENER_SERVICE'
application-label:'ScamShield'
launchable-activity: name='com.example.scamshield.MainActivity' label='ScamShield' icon=''
scheme: 'scamshield'
scheme: 'https'
`
	info := ParseAapt2Badging(output)

	if info.PackageID != "com.example.scamshield" {
		t.Errorf("expected package com.example.scamshield, got %s", info.PackageID)
	}
	if info.VersionCode != 102 {
		t.Errorf("expected versionCode 102, got %d", info.VersionCode)
	}
	if info.VersionName != "1.2.0" {
		t.Errorf("expected versionName 1.2.0, got %s", info.VersionName)
	}
	if info.MinSDK != 26 {
		t.Errorf("expected minSdk 26, got %d", info.MinSDK)
	}
	if info.TargetSDK != 37 {
		t.Errorf("expected targetSdk 37, got %d", info.TargetSDK)
	}
	if info.LauncherActivity != "com.example.scamshield.MainActivity" {
		t.Errorf("expected launcher com.example.scamshield.MainActivity, got %s", info.LauncherActivity)
	}
	if len(info.Permissions) != 2 {
		t.Errorf("expected 2 permissions, got %d", len(info.Permissions))
	}
	if len(info.DeepLinkSchemes) != 2 {
		t.Errorf("expected 2 deep link schemes, got %d", len(info.DeepLinkSchemes))
	}
}

func TestParseApkanalyzerPermissions(t *testing.T) {
	output := `
android.permission.INTERNET
android.permission.READ_SMS
android.permission.ACCESS_FINE_LOCATION
`
	perms := ParseApkanalyzerPermissions(output)
	if len(perms) != 3 {
		t.Fatalf("expected 3 permissions, got %d", len(perms))
	}
	if perms[0] != "android.permission.INTERNET" || perms[1] != "android.permission.READ_SMS" || perms[2] != "android.permission.ACCESS_FINE_LOCATION" {
		t.Errorf("unexpected permissions: %v", perms)
	}
}

func TestParseApkanalyzerFiles(t *testing.T) {
	output := `
/AndroidManifest.xml
/classes.dex
/lib/arm64-v8a/libnative.so
/lib/armeabi-v7a/libnative.so
/lib/x86_64/libnative.so
/res/layout/activity_main.xml
`
	abis := ParseApkanalyzerFiles(output)
	if len(abis) != 3 {
		t.Fatalf("expected 3 ABIs, got %d: %v", len(abis), abis)
	}
	hasArm64, hasArmV7, hasX86_64 := false, false, false
	for _, a := range abis {
		if a == "arm64-v8a" {
			hasArm64 = true
		}
		if a == "armeabi-v7a" {
			hasArmV7 = true
		}
		if a == "x86_64" {
			hasX86_64 = true
		}
	}
	if !hasArm64 || !hasArmV7 || !hasX86_64 {
		t.Errorf("missing expected ABIs in %v", abis)
	}
}

func TestInspectAPK_FullDirect(t *testing.T) {
	tempDir := t.TempDir()
	apkPath := filepath.Join(tempDir, "test-sample.apk")

	manifest := `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="com.example.sampleapp"
    android:versionCode="42"
    android:versionName="2.1.0">
    <uses-sdk android:minSdkVersion="24" android:targetSdkVersion="37" />
    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.POST_NOTIFICATIONS" />
    <application
        android:label="SampleApp"
        android:resizeableActivity="true">
        <activity
            android:name="com.example.sampleapp.MainActivity"
            android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
            <intent-filter>
                <action android:name="android.intent.action.VIEW" />
                <category android:name="android.intent.category.DEFAULT" />
                <category android:name="android.intent.category.BROWSABLE" />
                <data android:scheme="sampleapp" />
            </intent-filter>
        </activity>
        <service
            android:name="com.example.sampleapp.BackgroundSyncService"
            android:exported="false" />
        <receiver
            android:name="com.example.sampleapp.BootReceiver"
            android:exported="true" />
    </application>
</manifest>`

	soFiles := map[string][]byte{
		"lib/arm64-v8a/libsample.so": createMockELF64(16384),
		"lib/x86_64/libsample.so":    createMockELF64(16384),
	}

	dexStrings := []string{
		"Lcom/example/sampleapp/MainActivity;",
		"sampleapp://open",
	}

	createTestAPK(t, apkPath, []byte(manifest), soFiles, dexStrings)

	engine := NewGradleEngine(tempDir, 2048)
	info, err := engine.InspectAPK(apkPath, nil)
	if err != nil {
		t.Fatalf("InspectAPK failed: %v", err)
	}

	if info.PackageID != "com.example.sampleapp" {
		t.Errorf("expected package com.example.sampleapp, got %s", info.PackageID)
	}
	if info.VersionCode != 42 {
		t.Errorf("expected versionCode 42, got %d", info.VersionCode)
	}
	if info.VersionName != "2.1.0" {
		t.Errorf("expected versionName 2.1.0, got %s", info.VersionName)
	}
	if info.MinSDK != 24 {
		t.Errorf("expected minSdk 24, got %d", info.MinSDK)
	}
	if info.TargetSDK != 37 {
		t.Errorf("expected targetSdk 37, got %d", info.TargetSDK)
	}
	if info.LauncherActivity != "com.example.sampleapp.MainActivity" {
		t.Errorf("expected launcher com.example.sampleapp.MainActivity, got %s", info.LauncherActivity)
	}

	// Verify ABIs
	if len(info.ABIs) != 2 {
		t.Errorf("expected 2 ABIs, got %d: %v", len(info.ABIs), info.ABIs)
	}

	// Verify permissions
	if len(info.Permissions) != 2 {
		t.Errorf("expected 2 permissions, got %d: %v", len(info.Permissions), info.Permissions)
	}

	// Verify deep links
	hasScheme := false
	for _, s := range info.DeepLinkSchemes {
		if s == "sampleapp" {
			hasScheme = true
		}
	}
	if !hasScheme {
		t.Errorf("expected 'sampleapp' deep link scheme in %v", info.DeepLinkSchemes)
	}

	// Verify exported components: MainActivity and BootReceiver
	if len(info.ExportedComponents) != 2 {
		t.Errorf("expected 2 exported components, got %d: %+v", len(info.ExportedComponents), info.ExportedComponents)
	}

	// Verify signing certs extracted
	if len(info.SigningCerts) == 0 {
		t.Errorf("expected at least 1 signing cert fingerprint extracted")
	}
}
