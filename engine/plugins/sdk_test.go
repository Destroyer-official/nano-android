package plugins

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"nano_android/engine/security/audit"
)

func TestPluginSDKSignVerifyTamperAndIsolatedInvoke(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go compiler needed to build isolated test plugin binary")
	}
	tmp := t.TempDir()
	ledger, _ := audit.OpenLedger(filepath.Join(tmp, "plugin_audit.jsonl"))
	reg := NewRegistry(filepath.Join(tmp, "store"), ledger, true)

	pDir := filepath.Join(tmp, "sample_plugin")
	_ = os.MkdirAll(pDir, 0750)
	srcPath := filepath.Join(pDir, "main.go")
	_ = os.WriteFile(srcPath, []byte(`package main
import (
	"encoding/json"
	"os"
)
func main() {
	var in map[string]any
	_ = json.NewDecoder(os.Stdin).Decode(&in)
	out := map[string]any{
		"echo": in["msg"],
		"plugin_env": os.Getenv("NANO_PLUGIN_ID"),
		"leaked_secret": os.Getenv("NANO_SECRET_SHOULD_NOT_LEAK"),
	}
	_ = json.NewEncoder(os.Stdout).Encode(out)
}
`), 0600)
	binName := "plugin_entry"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	entryAbs := filepath.Join(pDir, binName)
	buildCmd := exec.Command("go", "build", "-o", entryAbs, srcPath)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build plugin helper failed: %v (%s)", err, string(out))
	}

	_, privHex, err := GenerateEd25519Keypair()
	if err != nil {
		t.Fatal(err)
	}
	m := PluginManifest{
		ID:         "com.example.echo_analyzer",
		Name:       "Echo Analyzer",
		Version:    "1.0.0",
		Kind:       KindAPKAnalyzer,
		Entrypoint: binName,
		MinSDK:     26,
		MaxSDK:     37,
	}
	if err := SignManifest(&m, entryAbs, privHex); err != nil {
		t.Fatalf("SignManifest failed: %v", err)
	}
	mBytes, _ := json.MarshalIndent(m, "", "  ")
	_ = os.WriteFile(filepath.Join(pDir, "plugin.json"), mBytes, 0600)

	rp, err := reg.InstallFromDir(pDir, []string{tmp})
	if err != nil || !rp.Verified {
		t.Fatalf("InstallFromDir failed: rp=%+v err=%v", rp, err)
	}

	t.Setenv("NANO_SECRET_SHOULD_NOT_LEAK", "topsecret_do_not_expose")
	res, err := reg.Invoke(context.Background(), m.ID, map[string]any{"msg": "hello_sdk"})
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if res["echo"] != "hello_sdk" || res["leaked_secret"] != "" || res["_verified_ed25519"] != true {
		t.Fatalf("unexpected isolated plugin response: %+v", res)
	}

	// Tamper entrypoint on disk -> must fail closed on Invoke
	_ = os.WriteFile(entryAbs, []byte("tampered"), 0600)
	if _, err := reg.Invoke(context.Background(), m.ID, map[string]any{"msg": "fail"}); err == nil {
		t.Fatalf("expected tampered plugin entrypoint to fail verification")
	}

	// Reject traversal in plugin ID
	badDir := filepath.Join(tmp, "bad_plugin")
	_ = os.MkdirAll(badDir, 0750)
	_ = os.WriteFile(filepath.Join(badDir, "run.py"), []byte("print('{}')"), 0600)
	badM := PluginManifest{ID: "../escape", Name: "Bad", Version: "1.0.0", Kind: KindAPKAnalyzer, Entrypoint: "run.py"}
	_ = SignManifest(&badM, filepath.Join(badDir, "run.py"), privHex)
	badBytes, _ := json.Marshal(badM)
	_ = os.WriteFile(filepath.Join(badDir, "plugin.json"), badBytes, 0600)
	if _, err := reg.InstallFromDir(badDir, []string{tmp}); err == nil {
		t.Fatalf("expected path traversal plugin ID rejected")
	}
}
