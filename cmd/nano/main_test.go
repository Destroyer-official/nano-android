package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNanoCLICommands(t *testing.T) {
	if code := runCLI(context.Background(), []string{"help"}); code != 0 {
		t.Fatalf("help returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"profiles"}); code != 0 {
		t.Fatalf("profiles returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"security", "/etc/passwd.apk"}); code != 2 {
		t.Fatalf("expected exit 2 (policy containment block) for escaping path, got %d", code)
	}

	root, _ := filepath.Abs(filepath.Join("..", ".."))
	t.Chdir(root)
	if code := runCLI(context.Background(), []string{"compatibility", "--json", filepath.Join("fixtures", "apks", "minimal_valid.apk")}); code != 0 {
		t.Fatalf("compatibility on minimal_valid.apk returned %d", code)
	}
	for _, sub := range []string{"inspect", "manifest", "dex", "resources"} {
		if code := runCLI(context.Background(), []string{sub, "--json", filepath.Join("fixtures", "apks", "minimal_valid.apk")}); code != 0 {
			t.Fatalf("%s on minimal_valid.apk returned %d", sub, code)
		}
	}
	if code := runCLI(context.Background(), []string{"diff", filepath.Join("fixtures", "apks", "minimal_valid.apk"), filepath.Join("fixtures", "apks", "compose_v2signed.apk")}); code != 0 {
		t.Fatalf("diff on APKs returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"policy"}); code != 0 {
		t.Fatalf("policy returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"cache", "status"}); code != 0 {
		t.Fatalf("cache status returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"doctor", "--json"}); code != 0 {
		t.Fatalf("doctor returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"worker"}); code != 0 {
		t.Fatalf("worker check returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"devices"}); code != 0 {
		t.Fatalf("devices check returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"export", "--format=sarif", filepath.Join("fixtures", "apks", "minimal_valid.apk")}); code != 0 {
		t.Fatalf("export sarif returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"benchmark", filepath.Join("tools", "prism-v.jar")}); code != 0 {
		t.Fatalf("benchmark on prism-v.jar returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"lab", "list"}); code != 0 {
		t.Fatalf("lab list returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"profile", filepath.Join("tools", "prism-v.jar")}); code != 0 {
		t.Fatalf("profile on prism-v.jar returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"media", "verify", filepath.Join("fixtures", "apks", "minimal_valid.apk"), filepath.Join("fixtures", "apks", "minimal_valid.apk")}); code != 0 {
		t.Fatalf("media bit-exact self-verify returned %d", code)
	}

	logFile := filepath.Join(root, "cache", "cli_crash_test.log")
	_ = os.WriteFile(logFile, []byte("FATAL EXCEPTION: main\njava.lang.IllegalStateException: test\n\tat com.example.A.b(A.java:10)"), 0644)
	t.Cleanup(func() { _ = os.Remove(logFile) })
	if code := runCLI(context.Background(), []string{"crash", "--log=" + logFile}); code != 0 {
		t.Fatalf("crash --log returned %d", code)
	}

	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(root, "cache", "baselines", "clean_install", "cli_vis.png")) })
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(root, "cache", "baselines", "clean_install", "cli_vis.json")) })
	if code := runCLI(context.Background(), []string{"visual", "approve", "--name=cli_vis", "--approver=ci_test", "--activity=com.prismv.ui.MainActivity", filepath.Join("tools", "prism-v.jar")}); code != 0 {
		t.Fatalf("visual approve returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"visual", "compare", "--name=cli_vis", "--mode=hybrid", "--activity=com.prismv.ui.MainActivity", filepath.Join("tools", "prism-v.jar")}); code != 0 {
		t.Fatalf("visual compare returned %d", code)
	}

	tmpInitName := "cli_test_init"
	t.Cleanup(func() { _ = os.Remove(filepath.Join(root, "cache", tmpInitName+"_suite.json")) })
	if code := runCLI(context.Background(), []string{"init", "--dir=" + tmpInitName}); code != 0 {
		t.Fatalf("init returned %d", code)
	}

	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(root, "cache", "intelligence")) })
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(root, "cache", "audit", "nano_ai_cli_audit.jsonl")) })
	minAPK := filepath.Join("fixtures", "apks", "minimal_valid.apk")
	if code := runCLI(context.Background(), []string{"graph", "build", minAPK}); code != 0 {
		t.Fatalf("graph build returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"graph", "impact", "--changed=MainActivity", minAPK}); code != 0 {
		t.Fatalf("graph impact returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"graph", "query", "--kind=activity"}); code != 0 {
		t.Fatalf("graph query returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"ai", "understand", "--max-steps=10", minAPK}); code != 0 {
		t.Fatalf("ai understand returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"ai", "generate-tests", minAPK}); code != 0 {
		t.Fatalf("ai generate-tests returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"ai", "diagnose", "--trace=java.lang.NullPointerException: test"}); code != 0 {
		t.Fatalf("ai diagnose returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"ai", "memory", "--key=cli_k", "--value=ok", "--actor=cli_ci"}); code != 0 {
		t.Fatalf("ai memory write returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"ai", "proposals"}); code != 0 {
		t.Fatalf("ai proposals returned %d", code)
	}

	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(root, "cache", "reporting")) })
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(root, "cache", "plugins")) })
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(root, "cache", "keys")) })
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(root, "cache", "audit", "nano_plugin_cli_audit.jsonl")) })
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(root, "cache", "audit", "nano_pool_cli_audit.jsonl")) })
	if code := runCLI(context.Background(), []string{"doctor", "--json"}); code != 0 {
		t.Fatalf("doctor --json returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"plugin", "list"}); code != 0 {
		t.Fatalf("plugin list returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"plugin", "keygen"}); code != 0 {
		t.Fatalf("plugin keygen returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"adapter", "list"}); code != 0 {
		t.Fatalf("adapter list returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"adapter", "translate", "--framework=maestro", "--target=flows/smoke.yaml"}); code != 0 {
		t.Fatalf("adapter translate returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"pool", "health"}); code != 0 {
		t.Fatalf("pool health returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"pool", "reserve"}); code != 0 {
		t.Fatalf("pool reserve returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"pool", "shard"}); code != 0 {
		t.Fatalf("pool shard returned %d", code)
	}
	for _, fmtName := range []string{"unified-json", "html", "pdf", "junit"} {
		if code := runCLI(context.Background(), []string{"report", "--format=" + fmtName, minAPK}); code != 0 {
			t.Fatalf("report --format=%s returned %d", fmtName, code)
		}
	}
	if code := runCLI(context.Background(), []string{"analytics"}); code != 0 {
		t.Fatalf("analytics returned %d", code)
	}
	if code := runCLI(context.Background(), []string{"sbom"}); code != 0 {
		t.Fatalf("sbom returned %d", code)
	}
}
