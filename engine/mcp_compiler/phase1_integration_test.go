package mcp_compiler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPhase1InspectAPKToolAndPolicy verifies Tier-0 static ingestion + compatibility routing via MCP nano_inspect_apk.
func TestPhase1InspectAPKToolAndPolicy(t *testing.T) {
	// Keep artifact cleanly inside workspace cache/test_fixtures so pathcontainment approves it.
	fixDir := filepath.Join(resolveCacheDir(), "phase1_test_fixtures")
	if err := os.MkdirAll(fixDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(fixDir) })

	v1 := filepath.Join(fixDir, "ref_v1.apk")
	v2 := filepath.Join(fixDir, "ref_v2.apk")
	buildMinimalTestAPK(t, v1, "com.example.nanoref", []string{"MainActivity"}, false)
	buildMinimalTestAPK(t, v2, "com.example.nanoref", []string{"MainActivity", "SettingsActivity"}, true)

	reg := NewToolRegistry(NewMockCoordinator(), NewMaestroCompiler("com.example.nanoref"))
	res, err := reg.Call(context.Background(), "nano_inspect_apk", map[string]any{
		"apk_path":          v1,
		"compare_with_path": v2,
		"requested_mode":    "auto",
	})
	if err != nil {
		t.Fatalf("nano_inspect_apk call failed: %v", err)
	}
	if res == nil || res.IsError || len(res.Content) == 0 {
		t.Fatalf("expected successful nano_inspect_apk result, got: %+v", res)
	}

	var parsed struct {
		Status         string         `json:"status"`
		Tier           string         `json:"tier"`
		Report         map[string]any `json:"report"`
		Recommendation struct {
			PackageName    string `json:"package_name"`
			PrimaryBackend string `json:"primary_backend"`
		} `json:"recommendation"`
		Comparison struct {
			AddedActivities []string `json:"added_activities"`
		} `json:"comparison"`
	}
	if err := json.Unmarshal([]byte(res.Content[0].Text), &parsed); err != nil {
		t.Fatalf("unmarshal nano_inspect_apk output: %v\nraw=%s", err, res.Content[0].Text)
	}
	if parsed.Tier != "static" || parsed.Recommendation.PackageName != "com.example.nanoref" {
		t.Fatalf("unexpected recommendation payload: %+v", parsed)
	}
	if len(parsed.Comparison.AddedActivities) == 0 {
		t.Fatalf("expected added activity in comparison diff, got %+v", parsed.Comparison)
	}

	// Verify explicit path containment rejection for out-of-workspace files
	badRes, _ := reg.Call(context.Background(), "nano_inspect_apk", map[string]any{
		"apk_path": "/etc/passwd.apk",
	})
	if badRes == nil || !badRes.IsError || !strings.Contains(badRes.Content[0].Text, "BLOCKED_POLICY") {
		t.Fatalf("expected BLOCKED_POLICY for escaping apk_path, got: %+v", badRes)
	}
}

func buildMinimalTestAPK(t *testing.T, outPath, pkg string, activities []string, addMediaCodecRef bool) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	fw, err := zw.Create("AndroidManifest.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(buildTestAXMLManifest(pkg, activities)); err != nil {
		t.Fatal(err)
	}

	// Minimal valid DEX header + type/proto stubs
	fd, err := zw.Create("classes.dex")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fd.Write(buildTestMinimalDEX(addMediaCodecRef)); err != nil {
		t.Fatal(err)
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outPath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

func buildTestAXMLManifest(pkg string, activities []string) []byte {
	stringsList := []string{"manifest", "package", "application", "activity", "name", pkg}
	stringsList = append(stringsList, activities...)

	// Encode UTF-8 string pool chunk
	var spData bytes.Buffer
	offsets := make([]uint32, len(stringsList))
	for i, s := range stringsList {
		offsets[i] = uint32(spData.Len())
		b := []byte(s)
		spData.WriteByte(byte(len(b)))
		spData.WriteByte(byte(len(b)))
		spData.Write(b)
		spData.WriteByte(0)
	}
	for spData.Len()%4 != 0 {
		spData.WriteByte(0)
	}
	headerSize := uint16(28)
	stringsStart := uint32(headerSize) + uint32(len(stringsList)*4)
	spSize := stringsStart + uint32(spData.Len())

	var spChunk bytes.Buffer
	_ = binary.Write(&spChunk, binary.LittleEndian, uint16(0x0001))
	_ = binary.Write(&spChunk, binary.LittleEndian, headerSize)
	_ = binary.Write(&spChunk, binary.LittleEndian, spSize)
	_ = binary.Write(&spChunk, binary.LittleEndian, uint32(len(stringsList)))
	_ = binary.Write(&spChunk, binary.LittleEndian, uint32(0))
	_ = binary.Write(&spChunk, binary.LittleEndian, uint32(0x00000100)) // UTF-8 flag
	_ = binary.Write(&spChunk, binary.LittleEndian, stringsStart)
	_ = binary.Write(&spChunk, binary.LittleEndian, uint32(0))
	for _, off := range offsets {
		_ = binary.Write(&spChunk, binary.LittleEndian, off)
	}
	spChunk.Write(spData.Bytes())

	writeStart := func(out *bytes.Buffer, nameIdx uint32, attrNameIdx, attrValIdx int32) {
		attrCount := uint16(0)
		if attrNameIdx >= 0 {
			attrCount = 1
		}
		chunkSz := uint32(36 + int(attrCount)*20)
		_ = binary.Write(out, binary.LittleEndian, uint16(0x0102))
		_ = binary.Write(out, binary.LittleEndian, uint16(16))
		_ = binary.Write(out, binary.LittleEndian, chunkSz)
		_ = binary.Write(out, binary.LittleEndian, uint32(1))
		_ = binary.Write(out, binary.LittleEndian, int32(-1))
		_ = binary.Write(out, binary.LittleEndian, int32(-1))
		_ = binary.Write(out, binary.LittleEndian, nameIdx)
		_ = binary.Write(out, binary.LittleEndian, uint16(20))
		_ = binary.Write(out, binary.LittleEndian, uint16(20))
		_ = binary.Write(out, binary.LittleEndian, attrCount)
		_ = binary.Write(out, binary.LittleEndian, uint16(0))
		_ = binary.Write(out, binary.LittleEndian, uint16(0))
		_ = binary.Write(out, binary.LittleEndian, uint16(0))
		if attrNameIdx >= 0 {
			_ = binary.Write(out, binary.LittleEndian, uint32(0xFFFFFFFF))
			_ = binary.Write(out, binary.LittleEndian, uint32(attrNameIdx))
			_ = binary.Write(out, binary.LittleEndian, attrValIdx)
			_ = binary.Write(out, binary.LittleEndian, uint16(8))
			_ = binary.Write(out, binary.LittleEndian, uint8(0))
			_ = binary.Write(out, binary.LittleEndian, uint8(0x03))
			_ = binary.Write(out, binary.LittleEndian, attrValIdx)
		}
	}
	writeEnd := func(out *bytes.Buffer, nameIdx uint32) {
		_ = binary.Write(out, binary.LittleEndian, uint16(0x0103))
		_ = binary.Write(out, binary.LittleEndian, uint16(16))
		_ = binary.Write(out, binary.LittleEndian, uint32(24))
		_ = binary.Write(out, binary.LittleEndian, uint32(1))
		_ = binary.Write(out, binary.LittleEndian, int32(-1))
		_ = binary.Write(out, binary.LittleEndian, int32(-1))
		_ = binary.Write(out, binary.LittleEndian, nameIdx)
	}

	var body bytes.Buffer
	body.Write(spChunk.Bytes())
	writeStart(&body, 0, 1, 5) // <manifest package=pkg>
	writeStart(&body, 2, -1, -1)
	for i := range activities {
		writeStart(&body, 3, 4, int32(6+i))
		writeEnd(&body, 3)
	}
	writeEnd(&body, 2)
	writeEnd(&body, 0)

	var doc bytes.Buffer
	_ = binary.Write(&doc, binary.LittleEndian, uint16(0x0003))
	_ = binary.Write(&doc, binary.LittleEndian, uint16(8))
	_ = binary.Write(&doc, binary.LittleEndian, uint32(8+body.Len()))
	doc.Write(body.Bytes())
	return doc.Bytes()
}

func buildTestMinimalDEX(addMediaCodec bool) []byte {
	// Minimal DEX 035 with 1 string ("Landroid/app/Activity;")
	strs := []string{"Landroid/app/Activity;"}
	if addMediaCodec {
		strs = append(strs, "Landroid/media/MediaCodec;")
	}
	headerSize := 0x70
	stringIDsOff := headerSize
	typeIDsOff := stringIDsOff + len(strs)*4
	dataOff := typeIDsOff + len(strs)*4

	var strData bytes.Buffer
	strOffsets := make([]uint32, len(strs))
	for i, s := range strs {
		strOffsets[i] = uint32(dataOff + strData.Len())
		strData.WriteByte(byte(len(s)))
		strData.WriteString(s)
		strData.WriteByte(0)
	}
	totalLen := dataOff + strData.Len()
	dex := make([]byte, totalLen)
	copy(dex[0:8], []byte("dex\n035\x00"))
	binary.LittleEndian.PutUint32(dex[32:36], uint32(totalLen))
	binary.LittleEndian.PutUint32(dex[36:40], uint32(headerSize))
	binary.LittleEndian.PutUint32(dex[40:44], 0x12345678)
	binary.LittleEndian.PutUint32(dex[56:60], uint32(len(strs)))
	binary.LittleEndian.PutUint32(dex[60:64], uint32(stringIDsOff))
	binary.LittleEndian.PutUint32(dex[64:68], uint32(len(strs)))
	binary.LittleEndian.PutUint32(dex[68:72], uint32(typeIDsOff))
	for i, off := range strOffsets {
		binary.LittleEndian.PutUint32(dex[stringIDsOff+i*4:], off)
		binary.LittleEndian.PutUint32(dex[typeIDsOff+i*4:], uint32(i))
	}
	copy(dex[dataOff:], strData.Bytes())
	return dex
}
