package graph

import (
	"os"
	"path/filepath"
	"testing"

	"nano_android/engine/apk/dex"
	"nano_android/engine/apk/ingest"
	"nano_android/engine/apk/manifest"
	"nano_android/engine/apk/resources"
	"nano_android/engine/apk/signing"
	"nano_android/engine/profiler/crash"
	"nano_android/engine/testing/suite"
)

func TestBehaviorGraphBuildArchitectureImpactAndPersistence(t *testing.T) {
	trueVal := true
	rep := &ingest.IntelligenceReport{
		Format: ingest.FormatAPK,
		Path:   "fixtures/apks/sample.apk",
		SHA256: "deadbeef1234",
		Manifest: &manifest.Info{
			Package:          "com.example.shop",
			LauncherActivity: "com.example.shop.MainActivity",
			Permissions:      []string{"android.permission.CAMERA"},
			Activities: []manifest.Component{
				{
					Kind:     "activity",
					Name:     "com.example.shop.MainActivity",
					Exported: &trueVal,
					IntentFilters: []manifest.IntentFilter{
						{Actions: []string{"android.intent.action.MAIN"}, Schemes: []string{"shop"}, Hosts: []string{"home"}},
					},
				},
				{
					Kind: "activity",
					Name: "com.example.shop.CheckoutActivity",
				},
			},
		},
		DEX: &dex.Inventory{
			Classes: []dex.ClassInfo{
				{JavaName: "com.example.shop.MainActivity", SuperClass: "android.app.Activity", DexFile: "classes.dex"},
				{JavaName: "com.example.shop.CheckoutActivity", SuperClass: "android.app.Activity", DexFile: "classes.dex"},
				{JavaName: "com.example.shop.CartCycleA", SuperClass: "java.lang.Object", DexFile: "classes.dex"},
				{JavaName: "com.example.shop.CartCycleB", SuperClass: "java.lang.Object", DexFile: "classes.dex"},
			},
			ClassDependencyGraph: map[string][]string{
				"com.example.shop.MainActivity": {"com.example.shop.CheckoutActivity"},
				"com.example.shop.CartCycleA":   {"com.example.shop.CartCycleB"},
				"com.example.shop.CartCycleB":   {"com.example.shop.CartCycleA"},
			},
			MethodReferences: []dex.MethodRef{
				{Owner: "Landroid/hardware/camera2/CameraManager;", Name: "openCamera"},
				{Owner: "Landroid/database/sqlite/SQLiteDatabase;", Name: "execSQL"},
			},
			DetectedLibraries: []string{"androidx.core", "okhttp3"},
		},
		Resources: &resources.Table{
			Entries: map[uint32]resources.Entry{
				0x7f010001: {IDHex: "0x7f010001", Type: "layout", Name: "activity_main"},
				0x7f020001: {IDHex: "0x7f020001", Type: "string", Name: "unused_banner_label", Value: "Promo"},
			},
			Strings: []string{"https://api.example.com/v1/cart", "CREATE TABLE orders (id INTEGER PRIMARY KEY)"},
		},
		NativeLibs: []signing.NativeLibInfo{
			{Path: "lib/arm64-v8a/libnative.so", ABI: "arm64-v8a", Machine: "EM_AARCH64", Is16KBAligned: true, JNISymbols: []string{"JNI_OnLoad"}},
		},
	}

	g := BuildFromArtifact(rep, "")
	g.EnrichFromRuntime("com.example.shop.MainActivity", "screen:MainActivity", "screen:CheckoutActivity", "tap:Checkout", nil, nil, nil)
	g.AttachCrash(crash.CrashReport{Signature: "sig_npe_cart", Kind: "EXCEPTION", ExceptionClass: "NullPointerException", RootCauseClass: "com.example.shop.CheckoutActivity"}, "com.example.shop.CheckoutActivity")
	g.AttachTestCase(suite.TestCase{Name: "CheckoutFlowTest", Activity: "com.example.shop.MainActivity", Tags: []string{"regression"}})

	arch := g.ExploreArchitecture()
	if arch.NodeCount < 10 || arch.EdgeCount < 8 {
		t.Fatalf("expected rich graph, got nodes=%d edges=%d", arch.NodeCount, arch.EdgeCount)
	}
	if len(arch.CircularDependencies) != 1 || len(arch.CircularDependencies[0]) != 2 {
		t.Fatalf("expected 1 circular dependency pair (CartCycleA/B), got %+v", arch.CircularDependencies)
	}
	if len(arch.UnusedResources) == 0 {
		t.Fatalf("expected unused resource string/unused_banner_label to be flagged")
	}
	if len(arch.JNIBoundaries) != 1 {
		t.Fatalf("expected 1 JNI boundary, got %+v", arch.JNIBoundaries)
	}
	if len(arch.NetworkEndpoints) == 0 || len(arch.DatabaseTables) == 0 {
		t.Fatalf("expected endpoints and db tables, got endpoints=%v tables=%v", arch.NetworkEndpoints, arch.DatabaseTables)
	}

	impact := g.AnalyzeChangeImpact([]string{"CheckoutActivity"})
	if len(impact.ImpactedComponents) == 0 || len(impact.SelectedTests) == 0 {
		t.Fatalf("expected impact traversal to select CheckoutFlowTest & MainActivity, got %+v", impact)
	}

	outPath := filepath.Join(t.TempDir(), "behavior_graph.json")
	if err := g.Save(outPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	reloaded, err := Load(outPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(reloaded.Nodes) != len(g.Nodes) || len(reloaded.Edges) != len(g.Edges) {
		t.Fatalf("roundtrip graph mismatch: got %d/%d vs %d/%d", len(reloaded.Nodes), len(reloaded.Edges), len(g.Nodes), len(g.Edges))
	}

	qr := reloaded.Query(QueryFilter{Kind: NodeActivity, ExportedOnly: true})
	if qr.TotalMatching != 1 || qr.Nodes[0].Name != "com.example.shop.MainActivity" {
		t.Fatalf("expected 1 exported activity from Query, got %+v", qr)
	}
}

func TestGraphQueryAndGradleSourceEnrichment(t *testing.T) {
	srcDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(srcDir, "build.gradle.kts"), []byte(`dependencies { implementation("androidx.room:room-runtime:2.6.1") }`), 0600)
	_ = os.WriteFile(filepath.Join(srcDir, "CheckoutFragment.kt"), []byte(`
		class CheckoutFragment : Fragment() {
			val api = "https://api.service.local/pay"
			val sql = "CREATE TABLE tx (id INT)"
			val perm = android.permission.CAMERA
		}
	`), 0600)

	g := BuildFromArtifact(nil, srcDir)
	arch := g.ExploreArchitecture()
	if len(arch.PermissionToCodeMap["android.permission.CAMERA"]) == 0 {
		t.Fatalf("expected source permission reference mapped, got %+v", arch.PermissionToCodeMap)
	}
	if len(arch.NetworkEndpoints) == 0 || len(arch.DatabaseTables) == 0 {
		t.Fatalf("expected source endpoint and table extracted, got endpoints=%v tables=%v", arch.NetworkEndpoints, arch.DatabaseTables)
	}
	qFrag := g.Query(QueryFilter{Kind: NodeFragment, NameContains: "Checkout"})
	if qFrag.TotalMatching != 1 {
		t.Fatalf("expected 1 CheckoutFragment node, got %+v", qFrag)
	}
	qDep := g.Query(QueryFilter{Kind: NodeDependency, NameContains: "room"})
	if qDep.TotalMatching != 1 {
		t.Fatalf("expected 1 Gradle room dependency node, got %+v", qDep)
	}
}
