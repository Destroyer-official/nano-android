package apk_test

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"nano_android/engine/apk/dex"
	"nano_android/engine/apk/ingest"
	"nano_android/engine/apk/manifest"
)

func buildMinimalBinaryAXML(pkg, launcherActivity string) []byte {
	// Build minimal valid AAPT2 binary AndroidManifest.xml with string pool + <manifest> + <uses-sdk> + <application> + <activity><intent-filter>
	stringsList := []string{
		"manifest", "package", "versionCode", "versionName",
		"uses-sdk", "minSdkVersion", "targetSdkVersion",
		"uses-permission", "name",
		"application", "label",
		"activity", "intent-filter", "action", "category",
		pkg, "1.0.0", "android.permission.INTERNET", "TestApp",
		launcherActivity, "android.intent.action.MAIN", "android.intent.category.LAUNCHER",
	}

	// Build UTF-8 string pool
	var strData bytes.Buffer
	offsets := make([]uint32, len(stringsList))
	for i, s := range stringsList {
		offsets[i] = uint32(strData.Len())
		strData.WriteByte(byte(len(s)))
		strData.WriteByte(byte(len(s)))
		strData.WriteString(s)
		strData.WriteByte(0)
	}
	for strData.Len()%4 != 0 {
		strData.WriteByte(0)
	}

	poolHdrSize := 28 + len(stringsList)*4
	poolChunkSize := poolHdrSize + strData.Len()
	var pool bytes.Buffer
	_ = binary.Write(&pool, binary.LittleEndian, uint16(0x0001)) // RES_STRING_POOL_TYPE
	_ = binary.Write(&pool, binary.LittleEndian, uint16(28))
	_ = binary.Write(&pool, binary.LittleEndian, uint32(poolChunkSize))
	_ = binary.Write(&pool, binary.LittleEndian, uint32(len(stringsList)))
	_ = binary.Write(&pool, binary.LittleEndian, uint32(0)) // styleCount
	_ = binary.Write(&pool, binary.LittleEndian, uint32(1<<8)) // UTF8_FLAG
	_ = binary.Write(&pool, binary.LittleEndian, uint32(poolHdrSize))
	_ = binary.Write(&pool, binary.LittleEndian, uint32(0))
	for _, off := range offsets {
		_ = binary.Write(&pool, binary.LittleEndian, off)
	}
	pool.Write(strData.Bytes())

	strIdx := func(target string) uint32 {
		for i, s := range stringsList {
			if s == target {
				return uint32(i)
			}
		}
		return 0xFFFFFFFF
	}

	type attrSpec struct {
		name  string
		dType uint8
		dVal  uint32
		raw   string
	}
	var body bytes.Buffer
	writeStart := func(tag string, attrs []attrSpec) {
		cSize := 36 + len(attrs)*20
		_ = binary.Write(&body, binary.LittleEndian, uint16(0x0102))
		_ = binary.Write(&body, binary.LittleEndian, uint16(16))
		_ = binary.Write(&body, binary.LittleEndian, uint32(cSize))
		_ = binary.Write(&body, binary.LittleEndian, uint32(1))
		_ = binary.Write(&body, binary.LittleEndian, uint32(0xFFFFFFFF))
		_ = binary.Write(&body, binary.LittleEndian, uint32(0xFFFFFFFF))
		_ = binary.Write(&body, binary.LittleEndian, strIdx(tag))
		_ = binary.Write(&body, binary.LittleEndian, uint16(0x14))
		_ = binary.Write(&body, binary.LittleEndian, uint16(0x14))
		_ = binary.Write(&body, binary.LittleEndian, uint16(len(attrs)))
		_ = binary.Write(&body, binary.LittleEndian, uint16(0))
		_ = binary.Write(&body, binary.LittleEndian, uint16(0))
		_ = binary.Write(&body, binary.LittleEndian, uint16(0))
		for _, a := range attrs {
			_ = binary.Write(&body, binary.LittleEndian, uint32(0xFFFFFFFF))
			_ = binary.Write(&body, binary.LittleEndian, strIdx(a.name))
			rawIdx := uint32(0xFFFFFFFF)
			if a.raw != "" {
				rawIdx = strIdx(a.raw)
			}
			_ = binary.Write(&body, binary.LittleEndian, rawIdx)
			_ = binary.Write(&body, binary.LittleEndian, uint16(8))
			body.WriteByte(0)
			body.WriteByte(a.dType)
			_ = binary.Write(&body, binary.LittleEndian, a.dVal)
		}
	}
	writeEnd := func(tag string) {
		_ = binary.Write(&body, binary.LittleEndian, uint16(0x0103))
		_ = binary.Write(&body, binary.LittleEndian, uint16(16))
		_ = binary.Write(&body, binary.LittleEndian, uint32(24))
		_ = binary.Write(&body, binary.LittleEndian, uint32(1))
		_ = binary.Write(&body, binary.LittleEndian, uint32(0xFFFFFFFF))
		_ = binary.Write(&body, binary.LittleEndian, uint32(0xFFFFFFFF))
		_ = binary.Write(&body, binary.LittleEndian, strIdx(tag))
	}

	writeStart("manifest", []attrSpec{
		{name: "package", dType: 0x03, dVal: strIdx(pkg), raw: pkg},
		{name: "versionCode", dType: 0x10, dVal: 42},
		{name: "versionName", dType: 0x03, dVal: strIdx("1.0.0"), raw: "1.0.0"},
	})
	writeStart("uses-sdk", []attrSpec{
		{name: "minSdkVersion", dType: 0x10, dVal: 26},
		{name: "targetSdkVersion", dType: 0x10, dVal: 37},
	})
	writeEnd("uses-sdk")
	writeStart("uses-permission", []attrSpec{
		{name: "name", dType: 0x03, dVal: strIdx("android.permission.INTERNET"), raw: "android.permission.INTERNET"},
	})
	writeEnd("uses-permission")
	writeStart("application", []attrSpec{
		{name: "label", dType: 0x03, dVal: strIdx("TestApp"), raw: "TestApp"},
	})
	writeStart("activity", []attrSpec{
		{name: "name", dType: 0x03, dVal: strIdx(launcherActivity), raw: launcherActivity},
	})
	writeStart("intent-filter", nil)
	writeStart("action", []attrSpec{{name: "name", dType: 0x03, dVal: strIdx("android.intent.action.MAIN"), raw: "android.intent.action.MAIN"}})
	writeEnd("action")
	writeStart("category", []attrSpec{{name: "name", dType: 0x03, dVal: strIdx("android.intent.category.LAUNCHER"), raw: "android.intent.category.LAUNCHER"}})
	writeEnd("category")
	writeEnd("intent-filter")
	writeEnd("activity")
	writeEnd("application")
	writeEnd("manifest")

	totalSize := 8 + pool.Len() + body.Len()
	var root bytes.Buffer
	_ = binary.Write(&root, binary.LittleEndian, uint16(0x0003))
	_ = binary.Write(&root, binary.LittleEndian, uint16(8))
	_ = binary.Write(&root, binary.LittleEndian, uint32(totalSize))
	root.Write(pool.Bytes())
	root.Write(body.Bytes())
	return root.Bytes()
}

func buildMinimalDEX() []byte {
	// Minimal valid DEX 035 with 2 strings, 2 types, 1 class_def (Lcom/example/MainActivity; extends Landroidx/compose/ui/Platform;)
	buf := make([]byte, 0x140)
	copy(buf[0:8], []byte("dex\n035\x00"))
	binary.LittleEndian.PutUint32(buf[0x20:0x24], uint32(len(buf))) // file_size
	binary.LittleEndian.PutUint32(buf[0x24:0x28], 0x70)             // header_size
	binary.LittleEndian.PutUint32(buf[0x28:0x2C], 0x12345678)       // ENDIAN_CONSTANT

	// String pool data starts at 0x100
	s0 := "Lcom/example/MainActivity;"
	s1 := "Landroidx/compose/ui/ComposeNode;"
	off0 := 0x100
	buf[off0] = byte(len(s0))
	copy(buf[off0+1:], s0)
	off1 := off0 + 2 + len(s0)
	buf[off1] = byte(len(s1))
	copy(buf[off1+1:], s1)

	// string_ids at 0x70
	binary.LittleEndian.PutUint32(buf[0x38:0x3C], 2)
	binary.LittleEndian.PutUint32(buf[0x3C:0x40], 0x70)
	binary.LittleEndian.PutUint32(buf[0x70:0x74], uint32(off0))
	binary.LittleEndian.PutUint32(buf[0x74:0x78], uint32(off1))

	// type_ids at 0x78
	binary.LittleEndian.PutUint32(buf[0x40:0x44], 2)
	binary.LittleEndian.PutUint32(buf[0x44:0x48], 0x78)
	binary.LittleEndian.PutUint32(buf[0x78:0x7C], 0)
	binary.LittleEndian.PutUint32(buf[0x7C:0x80], 1)

	// class_defs at 0x80 (1 class_def = 32 bytes)
	binary.LittleEndian.PutUint32(buf[0x60:0x64], 1)
	binary.LittleEndian.PutUint32(buf[0x64:0x68], 0x80)
	binary.LittleEndian.PutUint32(buf[0x80:0x84], 0) // class_idx = 0
	binary.LittleEndian.PutUint32(buf[0x84:0x88], 1) // ACC_PUBLIC
	binary.LittleEndian.PutUint32(buf[0x88:0x8C], 1) // superclass_idx = 1
	binary.LittleEndian.PutUint32(buf[0x90:0x94], 0xFFFFFFFF)
	return buf
}

func TestBinaryManifestAndDEXAndIngestComparison(t *testing.T) {
	axml := buildMinimalBinaryAXML("com.example.phase1", ".MainActivity")
	mInfo, err := manifest.ParseBinary(axml)
	if err != nil {
		t.Fatalf("ParseBinary failed: %v", err)
	}
	if mInfo.Package != "com.example.phase1" || mInfo.TargetSDK != 37 || mInfo.LauncherActivity != "com.example.phase1.MainActivity" {
		t.Fatalf("unexpected manifest info: %+v", mInfo)
	}

	dexBytes := buildMinimalDEX()
	dInv, err := dex.ParseSingle("classes.dex", dexBytes)
	if err != nil {
		t.Fatalf("dex.ParseSingle failed: %v", err)
	}
	if dInv.TotalClasses != 1 || len(dInv.CompatibilityIssues) == 0 {
		t.Fatalf("expected 1 class and Compose compatibility flag, got %+v", dInv)
	}

	dir := t.TempDir()
	apk1 := filepath.Join(dir, "v1.apk")
	apk2 := filepath.Join(dir, "v2.apk")
	writeAPK := func(p string, pkg string) {
		f, _ := os.Create(p)
		zw := zip.NewWriter(f)
		w1, _ := zw.Create("AndroidManifest.xml")
		_, _ = w1.Write(buildMinimalBinaryAXML(pkg, ".MainActivity"))
		w2, _ := zw.Create("classes.dex")
		_, _ = w2.Write(dexBytes)
		_ = zw.Close()
		_ = f.Close()
	}
	writeAPK(apk1, "com.example.v1")
	writeAPK(apk2, "com.example.v2")

	eng := ingest.NewEngine(filepath.Join(dir, "intel"), "")
	r1, err := eng.Analyze(apk1)
	if err != nil {
		t.Fatalf("Analyze(v1) failed: %v", err)
	}
	r2, err := eng.Analyze(apk2)
	if err != nil {
		t.Fatalf("Analyze(v2) failed: %v", err)
	}
	diff := ingest.Compare(r1, r2)
	if diff.BasePackage != "com.example.v1" || diff.TargetPackage != "com.example.v2" {
		t.Fatalf("unexpected comparison: %+v", diff)
	}
}

func TestFixtureCorpusSigningV2V4AndPermissionRisks(t *testing.T) {
	composeAPK := filepath.Join("..", "..", "fixtures", "apks", "compose_v2signed.apk")
	if _, err := os.Stat(composeAPK); err != nil {
		t.Fatalf("missing checked-in fixture %s: %v", composeAPK, err)
	}
	eng := ingest.NewEngine(t.TempDir(), "")
	rep, err := eng.Analyze(composeAPK)
	if err != nil {
		t.Fatalf("Analyze(compose_v2signed.apk) failed: %v", err)
	}
	if !rep.Signing.HasV2 || !rep.Signing.HasV4 || !rep.Signing.IsSigned || len(rep.Signing.Certificates) == 0 {
		t.Fatalf("expected v2+v4 signing with parsed X.509 cert, got: %+v", rep.Signing)
	}
	if len(rep.PermissionRisks) == 0 {
		t.Fatalf("expected non-empty PermissionRisks")
	}
	hasHigh := false
	for _, pr := range rep.PermissionRisks {
		if pr.RiskLevel == "HIGH" {
			hasHigh = true
			break
		}
	}
	if !hasHigh {
		t.Fatalf("expected HIGH risk permission flag, got: %+v", rep.PermissionRisks)
	}
	md := ingest.FormatMarkdownReport(rep)
	if len(md) < 80 {
		t.Fatalf("expected markdown doc report, got %q", md)
	}
	if issues := ingest.VerifyBuildArtifact(rep, "com.example.composehw", 1, rep.Signing.Certificates[0].SHA256); len(issues) != 0 {
		t.Fatalf("expected VerifyBuildArtifact pass, got %v", issues)
	}

	malformedZipSlip := filepath.Join("..", "..", "fixtures", "malformed_apks", "zipslip_traversal.apk")
	if _, err := eng.Analyze(malformedZipSlip); err == nil {
		t.Fatalf("expected Analyze(zipslip_traversal.apk) to fail archive security validation")
	}

	prismJar := filepath.Join("..", "..", "tools", "prism-v.jar")
	if _, err := os.Stat(prismJar); err == nil {
		jarRep, jErr := eng.Analyze(prismJar)
		if jErr != nil {
			t.Fatalf("Analyze(prism-v.jar) failed: %v", jErr)
		}
		if jarRep.Format != ingest.FormatJAR || jarRep.DEX == nil || jarRep.DEX.TotalClasses == 0 || jarRep.Manifest == nil || jarRep.Manifest.Package == "" {
			t.Fatalf("unexpected JAR analysis report: %+v", jarRep)
		}
	}
}
