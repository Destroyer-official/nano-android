package compatibility_test

import (
	"testing"

	"nano_android/engine/apk/dex"
	"nano_android/engine/apk/ingest"
	"nano_android/engine/apk/manifest"
	"nano_android/engine/apk/signing"
	"nano_android/engine/compatibility/registry"
)

func TestAdaptiveRouterAndStatusTaxonomy(t *testing.T) {
	// 1. Pure JVM APK routes to nano_jvm
	pure := &ingest.IntelligenceReport{
		Manifest: &manifest.Info{Package: "com.example.pure"},
		DEX:      &dex.Inventory{},
	}
	rPure := registry.EvaluateAPK(pure, "auto")
	if rPure.PrimaryBackend != registry.BackendNanoJVM {
		t.Fatalf("expected nano_jvm for pure APK, got %s", rPure.PrimaryBackend)
	}

	// 2. Native JNI APK routes to android_art
	jni := &ingest.IntelligenceReport{
		Manifest:   &manifest.Info{Package: "com.example.jni"},
		NativeLibs: []signing.NativeLibInfo{{Path: "lib/arm64-v8a/libnative.so", ABI: "arm64-v8a"}},
	}
	rJni := registry.EvaluateAPK(jni, "auto")
	if rJni.PrimaryBackend != registry.BackendAndroidART {
		t.Fatalf("expected android_art for native JNI APK, got %s", rJni.PrimaryBackend)
	}

	// 3. Explicit user-requested mode must never be overridden
	rOverride := registry.EvaluateAPK(jni, "nano_jvm")
	if rOverride.PrimaryBackend != registry.BackendNanoJVM {
		t.Fatalf("expected explicit user override nano_jvm to be preserved, got %s", rOverride.PrimaryBackend)
	}

	// 4. Reject ambiguous PASS/FAIL
	if err := registry.ValidateStatusResult("PASS"); err == nil {
		t.Fatal("expected bare 'PASS' to be rejected by taxonomy validator")
	}
	if err := registry.ValidateStatusResult(registry.StatusNanoSimulationPass); err != nil {
		t.Fatalf("expected NANO_SIMULATION_PASS to validate: %v", err)
	}
}
