package main

import (
	"archive/zip"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if err := os.MkdirAll(filepath.Join("fixtures", "apks"), 0755); err != nil {
		panic(err)
	}
	if err := os.MkdirAll(filepath.Join("fixtures", "malformed_apks"), 0755); err != nil {
		panic(err)
	}

	minAXML := buildAXML("com.example.nanoref", ".MainActivity", []string{"android.permission.INTERNET"})
	minDEX := buildDEX("Lcom/example/nanoref/MainActivity;", "Landroid/app/Activity;")
	if err := writeZipAPK(filepath.Join("fixtures", "apks", "minimal_valid.apk"), minAXML, minDEX, false); err != nil {
		panic(err)
	}

	compAXML := buildAXML("com.example.composehw", ".ComposeActivity", []string{
		"android.permission.READ_SMS",
		"android.permission.SYSTEM_ALERT_WINDOW",
		"android.permission.INTERNET",
	})
	compDEX := buildDEX("Lcom/example/composehw/ComposeActivity;", "Landroidx/compose/ui/platform/ComposeView;")
	compPath := filepath.Join("fixtures", "apks", "compose_v2signed.apk")
	if err := writeZipAPK(compPath, compAXML, compDEX, true); err != nil {
		panic(err)
	}
	if err := appendV2SigningBlock(compPath); err != nil {
		panic(err)
	}
	if err := writeV4Idsig(compPath + ".idsig"); err != nil {
		panic(err)
	}

	// Malformed 1: zip-slip path traversal
	var zBuf bytes.Buffer
	zw := zip.NewWriter(&zBuf)
	fw, _ := zw.Create("../evil_escape.txt")
	_, _ = fw.Write([]byte("zipslip"))
	_ = zw.Close()
	if err := os.WriteFile(filepath.Join("fixtures", "malformed_apks", "zipslip_traversal.apk"), zBuf.Bytes(), 0644); err != nil {
		panic(err)
	}

	// Malformed 2: truncated AXML manifest
	var tBuf bytes.Buffer
	tw := zip.NewWriter(&tBuf)
	mfw, _ := tw.Create("AndroidManifest.xml")
	_, _ = mfw.Write([]byte{0x03, 0x00, 0x08, 0x00})
	_ = tw.Close()
	if err := os.WriteFile(filepath.Join("fixtures", "malformed_apks", "truncated_manifest.apk"), tBuf.Bytes(), 0644); err != nil {
		panic(err)
	}

	fmt.Println("Generated reproducible Phase 1 fixtures under fixtures/apks and fixtures/malformed_apks")
}

func writeZipAPK(outPath string, axml, dexBytes []byte, withMediaCodec bool) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w1, err := zw.Create("AndroidManifest.xml")
	if err != nil {
		return err
	}
	if _, err := w1.Write(axml); err != nil {
		return err
	}
	w2, err := zw.Create("classes.dex")
	if err != nil {
		return err
	}
	if _, err := w2.Write(dexBytes); err != nil {
		return err
	}
	if withMediaCodec {
		w3, err := zw.Create("lib/arm64-v8a/libnative.so")
		if err != nil {
			return err
		}
		_, _ = w3.Write([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0})
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(outPath, buf.Bytes(), 0644)
}

func appendV2SigningBlock(apkPath string) error {
	orig, err := os.ReadFile(apkPath)
	if err != nil {
		return err
	}
	eocdOff := bytes.LastIndex(orig, []byte{0x50, 0x4b, 0x05, 0x06})
	if eocdOff < 0 {
		return fmt.Errorf("missing EOCD")
	}
	cdSize := binary.LittleEndian.Uint32(orig[eocdOff+12 : eocdOff+16])
	cdOff := int(binary.LittleEndian.Uint32(orig[eocdOff+16 : eocdOff+20]))
	if cdOff < 0 || cdOff > eocdOff {
		return fmt.Errorf("invalid cdOffset")
	}

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1001),
		Subject:      pkix.Name{CommonName: "Nano-Android Test Signer"},
		NotBefore:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:     time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}

	lp := func(b []byte) []byte {
		out := make([]byte, 4+len(b))
		binary.LittleEndian.PutUint32(out[0:4], uint32(len(b)))
		copy(out[4:], b)
		return out
	}
	// signedData = lp(emptyDigests) + lp(lp(certDER)) + lp(emptyAttrs)
	signedData := append(lp(nil), lp(lp(certDER))...)
	signedData = append(signedData, lp(nil)...)
	// signer = lp(signedData) + lp(emptySigs) + lp(emptyPubKey)
	signer := append(lp(signedData), lp(nil)...)
	signer = append(signer, lp(nil)...)
	v2Payload := lp(lp(signer))

	// pair = uint64(4+len(v2Payload)) | uint32(0x7109871a) | v2Payload
	var pair bytes.Buffer
	_ = binary.Write(&pair, binary.LittleEndian, uint64(4+len(v2Payload)))
	_ = binary.Write(&pair, binary.LittleEndian, uint32(0x7109871a))
	pair.Write(v2Payload)

	// Block size excluding first uint64 size field = pair.Len() + 8 (tail uint64) + 16 (magic)
	blockSize := uint64(pair.Len() + 8 + 16)
	var block bytes.Buffer
	_ = binary.Write(&block, binary.LittleEndian, blockSize)
	block.Write(pair.Bytes())
	_ = binary.Write(&block, binary.LittleEndian, blockSize)
	block.WriteString("APK Sig Block 42")

	var out bytes.Buffer
	out.Write(orig[:cdOff])
	newCDOff := uint32(out.Len())
	out.Write(block.Bytes())
	newCDOff = uint32(out.Len())
	out.Write(orig[cdOff:eocdOff])
	eocd := append([]byte(nil), orig[eocdOff:]...)
	_ = cdSize
	binary.LittleEndian.PutUint32(eocd[16:20], newCDOff)
	out.Write(eocd)
	return os.WriteFile(apkPath, out.Bytes(), 0644)
}

func writeV4Idsig(path string) error {
	var b [8]byte
	binary.LittleEndian.PutUint32(b[0:4], 2) // V4 format version
	return os.WriteFile(path, b[:], 0644)
}

func buildDEX(clsDesc, superDesc string) []byte {
	needed := 0x100 + len(clsDesc) + len(superDesc) + 16
	if needed < 0x200 {
		needed = 0x200
	}
	buf := make([]byte, needed)
	copy(buf[0:8], []byte("dex\n035\x00"))
	binary.LittleEndian.PutUint32(buf[0x20:0x24], uint32(len(buf)))
	binary.LittleEndian.PutUint32(buf[0x24:0x28], 0x70)
	binary.LittleEndian.PutUint32(buf[0x28:0x2C], 0x12345678)
	off0 := 0x100
	buf[off0] = byte(len(clsDesc))
	copy(buf[off0+1:], clsDesc)
	off1 := off0 + 2 + len(clsDesc)
	buf[off1] = byte(len(superDesc))
	copy(buf[off1+1:], superDesc)
	binary.LittleEndian.PutUint32(buf[0x38:0x3C], 2)
	binary.LittleEndian.PutUint32(buf[0x3C:0x40], 0x70)
	binary.LittleEndian.PutUint32(buf[0x70:0x74], uint32(off0))
	binary.LittleEndian.PutUint32(buf[0x74:0x78], uint32(off1))
	binary.LittleEndian.PutUint32(buf[0x40:0x44], 2)
	binary.LittleEndian.PutUint32(buf[0x44:0x48], 0x78)
	binary.LittleEndian.PutUint32(buf[0x78:0x7C], 0)
	binary.LittleEndian.PutUint32(buf[0x7C:0x80], 1)
	binary.LittleEndian.PutUint32(buf[0x60:0x64], 1)
	binary.LittleEndian.PutUint32(buf[0x64:0x68], 0x80)
	binary.LittleEndian.PutUint32(buf[0x80:0x84], 0)
	binary.LittleEndian.PutUint32(buf[0x84:0x88], 1)
	binary.LittleEndian.PutUint32(buf[0x88:0x8C], 1)
	binary.LittleEndian.PutUint32(buf[0x90:0x94], 0xFFFFFFFF)
	return buf
}

func buildAXML(pkg, act string, perms []string) []byte {
	stringsList := []string{"manifest", "package", "versionCode", "uses-sdk", "minSdkVersion", "targetSdkVersion", "uses-permission", "name", "application", "activity", "intent-filter", "action", "category", pkg, act, "android.intent.action.MAIN", "android.intent.category.LAUNCHER"}
	stringsList = append(stringsList, perms...)
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
	var pool bytes.Buffer
	_ = binary.Write(&pool, binary.LittleEndian, uint16(0x0001))
	_ = binary.Write(&pool, binary.LittleEndian, uint16(28))
	_ = binary.Write(&pool, binary.LittleEndian, uint32(poolHdrSize+strData.Len()))
	_ = binary.Write(&pool, binary.LittleEndian, uint32(len(stringsList)))
	_ = binary.Write(&pool, binary.LittleEndian, uint32(0))
	_ = binary.Write(&pool, binary.LittleEndian, uint32(1<<8))
	_ = binary.Write(&pool, binary.LittleEndian, uint32(poolHdrSize))
	_ = binary.Write(&pool, binary.LittleEndian, uint32(0))
	for _, off := range offsets {
		_ = binary.Write(&pool, binary.LittleEndian, off)
	}
	pool.Write(strData.Bytes())

	idx := func(target string) uint32 {
		for i, s := range stringsList {
			if s == target {
				return uint32(i)
			}
		}
		return 0xFFFFFFFF
	}
	type attr struct {
		n   string
		t   uint8
		v   uint32
		raw string
	}
	var body bytes.Buffer
	ws := func(tag string, attrs []attr) {
		_ = binary.Write(&body, binary.LittleEndian, uint16(0x0102))
		_ = binary.Write(&body, binary.LittleEndian, uint16(16))
		_ = binary.Write(&body, binary.LittleEndian, uint32(36+len(attrs)*20))
		_ = binary.Write(&body, binary.LittleEndian, uint32(1))
		_ = binary.Write(&body, binary.LittleEndian, uint32(0xFFFFFFFF))
		_ = binary.Write(&body, binary.LittleEndian, uint32(0xFFFFFFFF))
		_ = binary.Write(&body, binary.LittleEndian, idx(tag))
		_ = binary.Write(&body, binary.LittleEndian, uint16(0x14))
		_ = binary.Write(&body, binary.LittleEndian, uint16(0x14))
		_ = binary.Write(&body, binary.LittleEndian, uint16(len(attrs)))
		_ = binary.Write(&body, binary.LittleEndian, uint16(0))
		_ = binary.Write(&body, binary.LittleEndian, uint16(0))
		_ = binary.Write(&body, binary.LittleEndian, uint16(0))
		for _, a := range attrs {
			_ = binary.Write(&body, binary.LittleEndian, uint32(0xFFFFFFFF))
			_ = binary.Write(&body, binary.LittleEndian, idx(a.n))
			r := uint32(0xFFFFFFFF)
			if a.raw != "" {
				r = idx(a.raw)
			}
			_ = binary.Write(&body, binary.LittleEndian, r)
			_ = binary.Write(&body, binary.LittleEndian, uint16(8))
			body.WriteByte(0)
			body.WriteByte(a.t)
			_ = binary.Write(&body, binary.LittleEndian, a.v)
		}
	}
	we := func(tag string) {
		_ = binary.Write(&body, binary.LittleEndian, uint16(0x0103))
		_ = binary.Write(&body, binary.LittleEndian, uint16(16))
		_ = binary.Write(&body, binary.LittleEndian, uint32(24))
		_ = binary.Write(&body, binary.LittleEndian, uint32(1))
		_ = binary.Write(&body, binary.LittleEndian, uint32(0xFFFFFFFF))
		_ = binary.Write(&body, binary.LittleEndian, uint32(0xFFFFFFFF))
		_ = binary.Write(&body, binary.LittleEndian, idx(tag))
	}
	ws("manifest", []attr{{n: "package", t: 0x03, v: idx(pkg), raw: pkg}, {n: "versionCode", t: 0x10, v: 1}})
	ws("uses-sdk", []attr{{n: "minSdkVersion", t: 0x10, v: 26}, {n: "targetSdkVersion", t: 0x10, v: 35}})
	we("uses-sdk")
	for _, p := range perms {
		ws("uses-permission", []attr{{n: "name", t: 0x03, v: idx(p), raw: p}})
		we("uses-permission")
	}
	ws("application", nil)
	ws("activity", []attr{{n: "name", t: 0x03, v: idx(act), raw: act}})
	ws("intent-filter", nil)
	ws("action", []attr{{n: "name", t: 0x03, v: idx("android.intent.action.MAIN"), raw: "android.intent.action.MAIN"}})
	we("action")
	ws("category", []attr{{n: "name", t: 0x03, v: idx("android.intent.category.LAUNCHER"), raw: "android.intent.category.LAUNCHER"}})
	we("category")
	we("intent-filter")
	we("activity")
	we("application")
	we("manifest")

	var doc bytes.Buffer
	_ = binary.Write(&doc, binary.LittleEndian, uint16(0x0003))
	_ = binary.Write(&doc, binary.LittleEndian, uint16(8))
	_ = binary.Write(&doc, binary.LittleEndian, uint32(8+pool.Len()+body.Len()))
	doc.Write(pool.Bytes())
	doc.Write(body.Bytes())
	return doc.Bytes()
}
