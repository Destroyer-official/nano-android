package testing_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nano_android/engine/compatibility/registry"
	"nano_android/engine/mcp_compiler"
	"nano_android/engine/testing/labs"
	"nano_android/engine/testing/mcphooks"
)

func buildValidTestMP4() []byte {
	box := func(t string, p []byte) []byte {
		b := make([]byte, 8+len(p))
		binary.BigEndian.PutUint32(b[0:4], uint32(len(b)))
		copy(b[4:8], t)
		copy(b[8:], p)
		return b
	}
	ftyp := box("ftyp", []byte("isom\x00\x00\x02\x00isom"))
	mvhd := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhd[12:16], 1000)
	binary.BigEndian.PutUint32(mvhd[16:20], 1000)
	moov := box("moov", box("mvhd", mvhd))
	mdat := box("mdat", make([]byte, 128))
	return bytes.Join([][]byte{ftyp, moov, mdat}, nil)
}

func TestAllTwelveLabsEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	wsRoot, _ := filepath.Abs(filepath.Join("..", ".."))

	jarArtifact := filepath.Join(wsRoot, "tools", "prism-v.jar")
	apkArtifact := filepath.Join(wsRoot, "fixtures", "apks", "minimal_valid.apk")

	// Create temporary valid MP4 inside cache/ for Media Lab
	mp4Path := filepath.Join(wsRoot, "cache", "lab_media_test.mp4")
	if err := os.WriteFile(mp4Path, buildValidTestMP4(), 0644); err != nil {
		t.Fatalf("write test mp4: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(mp4Path) })
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(wsRoot, "cache", "compatibility.db")) })

	runCheck := func(labID, artifact string, opts map[string]string, expectStatus ...registry.ResultStatus) {
		t.Run("lab_"+labID, func(t *testing.T) {
			rep, err := labs.RunLab(ctx, wsRoot, labID, artifact, opts)
			if err != nil {
				t.Fatalf("RunLab(%s) error: %v", labID, err)
			}
			for _, st := range expectStatus {
				if rep.Status == st {
					return
				}
			}
			t.Fatalf("RunLab(%s) unexpected status %s (findings=%v)", labID, rep.Status, rep.Findings)
		})
	}

	optsPrism := map[string]string{"activity": "com.prismv.ui.MainActivity"}
	runCheck("crash", jarArtifact, optsPrism, registry.StatusNanoSimulationPass)
	runCheck("permission", apkArtifact, nil, registry.StatusNanoSimulationPass)
	runCheck("network", jarArtifact, nil, registry.StatusNanoSimulationPass)
	runCheck("storage", apkArtifact, nil, registry.StatusNanoSimulationPass)
	runCheck("visual", jarArtifact, optsPrism, registry.StatusNanoSimulationPass)
	runCheck("media", mp4Path, nil, registry.StatusNanoSimulationPass)
	// accessibility lab intentionally returns FAIL when real layout lacks contentDescriptions (truthfulness!)
	runCheck("accessibility", jarArtifact, map[string]string{"activity": "com.prismv.ui.MainActivity", "profile_id": "large_font"}, registry.StatusFail, registry.StatusNanoSimulationPass)
	runCheck("compatibility", apkArtifact, nil, registry.StatusNanoSimulationPass)
	runCheck("stress", jarArtifact, optsPrism, registry.StatusNanoSimulationPass)
	runCheck("security", apkArtifact, nil, registry.StatusFail, registry.StatusNanoSimulationPass)
	runCheck("differential", jarArtifact, map[string]string{"profile_b": "small_screen"}, registry.StatusFail, registry.StatusNanoSimulationPass)
	runCheck("performance", jarArtifact, optsPrism, registry.StatusNanoSimulationPass)
}

func TestMCPHooksEndToEndDispatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	wsRoot, _ := filepath.Abs(filepath.Join("..", ".."))

	mp4Path := filepath.Join(wsRoot, "cache", "mcp_hook_media.mp4")
	if err := os.WriteFile(mp4Path, buildValidTestMP4(), 0644); err != nil {
		t.Fatalf("write test mp4: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(mp4Path) })

	reg := mcp_compiler.NewToolRegistry(mcp_compiler.NewMockCoordinator(), mcp_compiler.NewMaestroCompiler("com.prism.v"))
	mcphooks.Attach(reg, wsRoot)

	// 1. nano_lab_list
	resList, err := reg.Call(ctx, "nano_lab_list", nil)
	if err != nil || resList.IsError || !strings.Contains(resList.Content[0].Text, "differential") {
		t.Fatalf("nano_lab_list failed: err=%v res=%+v", err, resList)
	}

	// 2. nano_lab_run
	resLab, err := reg.Call(ctx, "nano_lab_run", map[string]any{"lab_id": "network", "artifact": filepath.Join(wsRoot, "tools", "prism-v.jar")})
	if err != nil || resLab.IsError || !strings.Contains(resLab.Content[0].Text, "NANO_SIMULATION_PASS") {
		t.Fatalf("nano_lab_run(network) failed: err=%v res=%+v", err, resLab)
	}

	// 3. nano_media_inspect
	resMed, err := reg.Call(ctx, "nano_media_inspect", map[string]any{"media_path": mp4Path})
	if err != nil || resMed.IsError {
		t.Fatalf("nano_media_inspect failed: err=%v res=%+v", err, resMed)
	}
	var medOut map[string]any
	if err := json.Unmarshal([]byte(resMed.Content[0].Text), &medOut); err != nil {
		t.Fatalf("invalid json from nano_media_inspect: %v", err)
	}

	// 4. nano_crash_analyze
	resCrash, err := reg.Call(ctx, "nano_crash_analyze", map[string]any{
		"trace":     "java.lang.RuntimeException: boom Bearer super-secret-jwt-123456\n\tat com.example.Main.run(Main.java:10)",
		"build_tag": "ci-1",
	})
	if err != nil || resCrash.IsError || strings.Contains(resCrash.Content[0].Text, "super-secret-jwt-123456") {
		t.Fatalf("nano_crash_analyze failed or leaked secret: err=%v res=%+v", err, resCrash)
	}
}
