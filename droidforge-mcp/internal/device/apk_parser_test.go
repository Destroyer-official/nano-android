package device

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"testing"
)

// buildSyntheticAXML creates a valid binary AndroidManifest.xml byte buffer for testing.
func buildSyntheticAXML(pkgName, mainAct string, perms []string) []byte {
	// Strings to put in string pool
	stringsList := []string{
		"manifest",
		"package",
		"versionCode",
		"versionName",
		"uses-sdk",
		"minSdkVersion",
		"targetSdkVersion",
		"uses-permission",
		"name",
		"application",
		"activity",
		"intent-filter",
		"action",
		"category",
		"android.intent.action.MAIN",
		"android.intent.category.LAUNCHER",
		pkgName,
		"1.0.0",
		mainAct,
	}
	stringsList = append(stringsList, perms...)

	// Build string pool chunk
	var spBuf bytes.Buffer
	stringCount := uint32(len(stringsList))
	flags := uint32(0) // UTF-16
	stringsStart := uint32(28 + 4*stringCount)

	// String offsets and encoded strings
	var strDataBuf bytes.Buffer
	var offsets []uint32

	for _, s := range stringsList {
		offsets = append(offsets, uint32(strDataBuf.Len()))
		// UTF-16 length prefix
		u16Len := uint16(len(s))
		_ = binary.Write(&strDataBuf, binary.LittleEndian, u16Len)
		for _, r := range s {
			_ = binary.Write(&strDataBuf, binary.LittleEndian, uint16(r))
		}
		// Null terminator
		_ = binary.Write(&strDataBuf, binary.LittleEndian, uint16(0))
	}

	spChunkSize := uint32(28 + 4*stringCount + uint32(strDataBuf.Len()))
	// Align to 4 bytes
	if spChunkSize%4 != 0 {
		pad := 4 - (spChunkSize % 4)
		spChunkSize += pad
		strDataBuf.Write(make([]byte, pad))
	}

	_ = binary.Write(&spBuf, binary.LittleEndian, chunkStringPool)
	_ = binary.Write(&spBuf, binary.LittleEndian, spChunkSize)
	_ = binary.Write(&spBuf, binary.LittleEndian, stringCount)
	_ = binary.Write(&spBuf, binary.LittleEndian, uint32(0)) // styleCount
	_ = binary.Write(&spBuf, binary.LittleEndian, flags)
	_ = binary.Write(&spBuf, binary.LittleEndian, stringsStart)
	_ = binary.Write(&spBuf, binary.LittleEndian, uint32(0)) // stylesStart

	for _, off := range offsets {
		_ = binary.Write(&spBuf, binary.LittleEndian, off)
	}
	spBuf.Write(strDataBuf.Bytes())

	// String lookup helper
	findStrIdx := func(val string) uint32 {
		for i, s := range stringsList {
			if s == val {
				return uint32(i)
			}
		}
		return 0
	}

	// Build XML elements
	var elemBuf bytes.Buffer

	// 1. START_ELEMENT: <manifest package=pkgName versionCode=101 versionName="1.0.0">
	writeStartElement(&elemBuf, findStrIdx("manifest"), []xmlAttr{
		{nameIdx: findStrIdx("package"), rawValIdx: findStrIdx(pkgName)},
		{nameIdx: findStrIdx("versionCode"), rawValIdx: 0, dataType: 0x10, intVal: 101},
		{nameIdx: findStrIdx("versionName"), rawValIdx: findStrIdx("1.0.0")},
	})

	// 2. <uses-sdk minSdkVersion=26 targetSdkVersion=34>
	writeStartElement(&elemBuf, findStrIdx("uses-sdk"), []xmlAttr{
		{nameIdx: findStrIdx("minSdkVersion"), rawValIdx: 0, dataType: 0x10, intVal: 26},
		{nameIdx: findStrIdx("targetSdkVersion"), rawValIdx: 0, dataType: 0x10, intVal: 34},
	})
	writeEndElement(&elemBuf, findStrIdx("uses-sdk"))

	// 3. Permissions: <uses-permission name=perm />
	for _, perm := range perms {
		writeStartElement(&elemBuf, findStrIdx("uses-permission"), []xmlAttr{
			{nameIdx: findStrIdx("name"), rawValIdx: findStrIdx(perm)},
		})
		writeEndElement(&elemBuf, findStrIdx("uses-permission"))
	}

	// 4. <application>
	writeStartElement(&elemBuf, findStrIdx("application"), nil)

	// 5. <activity name=mainAct>
	writeStartElement(&elemBuf, findStrIdx("activity"), []xmlAttr{
		{nameIdx: findStrIdx("name"), rawValIdx: findStrIdx(mainAct)},
	})

	// 6. <intent-filter>
	writeStartElement(&elemBuf, findStrIdx("intent-filter"), nil)

	// 7. <action name="android.intent.action.MAIN" />
	writeStartElement(&elemBuf, findStrIdx("action"), []xmlAttr{
		{nameIdx: findStrIdx("name"), rawValIdx: findStrIdx("android.intent.action.MAIN")},
	})
	writeEndElement(&elemBuf, findStrIdx("action"))

	// 8. <category name="android.intent.category.LAUNCHER" />
	writeStartElement(&elemBuf, findStrIdx("category"), []xmlAttr{
		{nameIdx: findStrIdx("name"), rawValIdx: findStrIdx("android.intent.category.LAUNCHER")},
	})
	writeEndElement(&elemBuf, findStrIdx("category"))

	// End intent-filter, activity, application, manifest
	writeEndElement(&elemBuf, findStrIdx("intent-filter"))
	writeEndElement(&elemBuf, findStrIdx("activity"))
	writeEndElement(&elemBuf, findStrIdx("application"))
	writeEndElement(&elemBuf, findStrIdx("manifest"))

	// Final AXML File
	var fileBuf bytes.Buffer
	totalSize := uint32(8 + spBuf.Len() + elemBuf.Len())
	_ = binary.Write(&fileBuf, binary.LittleEndian, chunkAXMLFile)
	_ = binary.Write(&fileBuf, binary.LittleEndian, totalSize)
	fileBuf.Write(spBuf.Bytes())
	fileBuf.Write(elemBuf.Bytes())

	return fileBuf.Bytes()
}

type xmlAttr struct {
	nameIdx   uint32
	rawValIdx uint32
	dataType  byte
	intVal    int
}

func writeStartElement(w *bytes.Buffer, nameIdx uint32, attrs []xmlAttr) {
	attrStart := uint16(36)
	attrCount := uint16(len(attrs))
	chunkSize := uint32(36 + int(attrCount)*20)

	_ = binary.Write(w, binary.LittleEndian, chunkStartElement)
	_ = binary.Write(w, binary.LittleEndian, chunkSize)
	_ = binary.Write(w, binary.LittleEndian, uint32(1)) // lineNumber
	_ = binary.Write(w, binary.LittleEndian, uint32(0)) // comment
	_ = binary.Write(w, binary.LittleEndian, uint32(0)) // ns
	_ = binary.Write(w, binary.LittleEndian, nameIdx)
	_ = binary.Write(w, binary.LittleEndian, attrStart)
	_ = binary.Write(w, binary.LittleEndian, uint16(20)) // attrSize
	_ = binary.Write(w, binary.LittleEndian, attrCount)
	_ = binary.Write(w, binary.LittleEndian, uint16(0)) // idIndex
	_ = binary.Write(w, binary.LittleEndian, uint16(0)) // classIndex
	_ = binary.Write(w, binary.LittleEndian, uint16(0)) // styleIndex

	for _, a := range attrs {
		_ = binary.Write(w, binary.LittleEndian, uint32(0)) // ns
		_ = binary.Write(w, binary.LittleEndian, a.nameIdx)
		_ = binary.Write(w, binary.LittleEndian, a.rawValIdx)
		_ = binary.Write(w, binary.LittleEndian, uint16(8)) // typedValue size
		_ = binary.Write(w, binary.LittleEndian, byte(0))   // res0
		_ = binary.Write(w, binary.LittleEndian, a.dataType)
		_ = binary.Write(w, binary.LittleEndian, uint32(a.intVal))
	}
}

func writeEndElement(w *bytes.Buffer, nameIdx uint32) {
	chunkSize := uint32(24)
	_ = binary.Write(w, binary.LittleEndian, chunkEndElement)
	_ = binary.Write(w, binary.LittleEndian, chunkSize)
	_ = binary.Write(w, binary.LittleEndian, uint32(1)) // lineNumber
	_ = binary.Write(w, binary.LittleEndian, uint32(0)) // comment
	_ = binary.Write(w, binary.LittleEndian, uint32(0)) // ns
	_ = binary.Write(w, binary.LittleEndian, nameIdx)
}

func TestParseAXML(t *testing.T) {
	pkg := "com.example.droidforge"
	act := ".MainActivity"
	perms := []string{
		"android.permission.INTERNET",
		"android.permission.POST_NOTIFICATIONS",
	}

	axmlBytes := buildSyntheticAXML(pkg, act, perms)

	manifest, err := ParseAXML(axmlBytes)
	if err != nil {
		t.Fatalf("ParseAXML failed: %v", err)
	}

	if manifest.PackageID != pkg {
		t.Errorf("PackageID = %q, expected %q", manifest.PackageID, pkg)
	}
	if manifest.MainActivity != act {
		t.Errorf("MainActivity = %q, expected %q", manifest.MainActivity, act)
	}
	if manifest.Component != "com.example.droidforge/.MainActivity" {
		t.Errorf("Component = %q, expected 'com.example.droidforge/.MainActivity'", manifest.Component)
	}
	if manifest.VersionCode != 101 {
		t.Errorf("VersionCode = %d, expected 101", manifest.VersionCode)
	}
	if manifest.VersionName != "1.0.0" {
		t.Errorf("VersionName = %q, expected '1.0.0'", manifest.VersionName)
	}
	if manifest.MinSDK != 26 {
		t.Errorf("MinSDK = %d, expected 26", manifest.MinSDK)
	}
	if manifest.TargetSDK != 34 {
		t.Errorf("TargetSDK = %d, expected 34", manifest.TargetSDK)
	}
	if len(manifest.Permissions) != 2 {
		t.Errorf("expected 2 permissions, got %d", len(manifest.Permissions))
	}
}

func TestParseAPKReader(t *testing.T) {
	pkg := "org.droidforge.sample"
	act := "com.example.ui.SplashActivity"
	axmlBytes := buildSyntheticAXML(pkg, act, nil)

	// Build in-memory zip archive
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	f, err := zw.Create("AndroidManifest.xml")
	if err != nil {
		t.Fatalf("failed creating zip entry: %v", err)
	}
	_, _ = f.Write(axmlBytes)
	_ = zw.Close()

	reader := bytes.NewReader(zipBuf.Bytes())
	manifest, err := ParseAPKReader(reader, int64(zipBuf.Len()))
	if err != nil {
		t.Fatalf("ParseAPKReader failed: %v", err)
	}

	if manifest.PackageID != pkg {
		t.Errorf("expected package %q, got %q", pkg, manifest.PackageID)
	}
	if manifest.MainActivity != act {
		t.Errorf("expected main activity %q, got %q", act, manifest.MainActivity)
	}
	if manifest.Component != "org.droidforge.sample/com.example.ui.SplashActivity" {
		t.Errorf("expected component 'org.droidforge.sample/com.example.ui.SplashActivity', got %q", manifest.Component)
	}
}

func TestParseAXMLCorrupt(t *testing.T) {
	// 1. Too short
	if _, err := ParseAXML([]byte{1, 2, 3}); err == nil {
		t.Error("expected error on too short buffer, got nil")
	}

	// 2. Bad magic number
	badMagic := []byte{0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00}
	if _, err := ParseAXML(badMagic); err == nil {
		t.Error("expected error on bad magic number, got nil")
	}
}
