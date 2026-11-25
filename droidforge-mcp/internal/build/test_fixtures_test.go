package build

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

// createMockELF64 generates a valid 64-bit ELF binary buffer with the given LOAD segment alignment.
func createMockELF64(align uint64) []byte {
	buf := new(bytes.Buffer)

	// ELF Ident (16 bytes)
	ident := [16]byte{
		0x7f, 'E', 'L', 'F',
		2, // ELFCLASS64
		1, // ELFDATA2LSB (little endian)
		1, // EV_CURRENT
		0, // ELFOSABI_NONE
	}
	buf.Write(ident[:])

	// ELF Header 64-bit (48 bytes remaining, total 64 bytes)
	_ = binary.Write(buf, binary.LittleEndian, uint16(3))   // e_type = ET_DYN (shared library)
	_ = binary.Write(buf, binary.LittleEndian, uint16(183)) // e_machine = EM_AARCH64 (ARM64)
	_ = binary.Write(buf, binary.LittleEndian, uint32(1))   // e_version = EV_CURRENT
	_ = binary.Write(buf, binary.LittleEndian, uint64(0))   // e_entry
	_ = binary.Write(buf, binary.LittleEndian, uint64(64))  // e_phoff = 64 (program header offset)
	_ = binary.Write(buf, binary.LittleEndian, uint64(0))   // e_shoff
	_ = binary.Write(buf, binary.LittleEndian, uint32(0))   // e_flags
	_ = binary.Write(buf, binary.LittleEndian, uint16(64))  // e_ehsize
	_ = binary.Write(buf, binary.LittleEndian, uint16(56))  // e_phentsize
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))   // e_phnum = 1
	_ = binary.Write(buf, binary.LittleEndian, uint16(64))  // e_shentsize
	_ = binary.Write(buf, binary.LittleEndian, uint16(0))   // e_shnum
	_ = binary.Write(buf, binary.LittleEndian, uint16(0))   // e_shstrndx

	// Program Header (56 bytes)
	_ = binary.Write(buf, binary.LittleEndian, uint32(1))     // p_type = PT_LOAD
	_ = binary.Write(buf, binary.LittleEndian, uint32(5))     // p_flags = PF_R | PF_X
	_ = binary.Write(buf, binary.LittleEndian, uint64(0))     // p_offset
	_ = binary.Write(buf, binary.LittleEndian, uint64(0))     // p_vaddr
	_ = binary.Write(buf, binary.LittleEndian, uint64(0))     // p_paddr
	_ = binary.Write(buf, binary.LittleEndian, uint64(256))   // p_filesz
	_ = binary.Write(buf, binary.LittleEndian, uint64(256))   // p_memsz
	_ = binary.Write(buf, binary.LittleEndian, uint64(align)) // p_align

	// Pad to 256 bytes payload
	pad := make([]byte, 256-buf.Len())
	buf.Write(pad)

	return buf.Bytes()
}

// createTestAPK creates a ZIP/APK file on disk with given manifest, SO files, and DEX strings.
func createTestAPK(t *testing.T, targetPath string, manifestXML []byte, soFiles map[string][]byte, dexStrings []string) {
	t.Helper()

	f, err := os.Create(targetPath)
	if err != nil {
		t.Fatalf("failed to create APK file %s: %v", targetPath, err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)

	// Write AndroidManifest.xml
	if len(manifestXML) > 0 {
		mw, err := zw.Create("AndroidManifest.xml")
		if err != nil {
			t.Fatalf("failed to create AndroidManifest.xml in zip: %v", err)
		}
		if _, err := mw.Write(manifestXML); err != nil {
			t.Fatalf("failed to write AndroidManifest.xml: %v", err)
		}
	}

	// Write .so files
	for path, data := range soFiles {
		sw, err := zw.Create(path)
		if err != nil {
			t.Fatalf("failed to create so file %s in zip: %v", path, err)
		}
		if _, err := sw.Write(data); err != nil {
			t.Fatalf("failed to write so data: %v", err)
		}
	}

	// Write mock classes.dex
	if len(dexStrings) > 0 {
		dw, err := zw.Create("classes.dex")
		if err != nil {
			t.Fatalf("failed to create classes.dex in zip: %v", err)
		}
		dexBuf := new(bytes.Buffer)
		dexBuf.WriteString("dex\n039\x00") // DEX magic header
		for _, s := range dexStrings {
			dexBuf.WriteString(s)
			dexBuf.WriteByte(0)
		}
		if _, err := dw.Write(dexBuf.Bytes()); err != nil {
			t.Fatalf("failed to write classes.dex: %v", err)
		}
	}

	// Write mock cert in META-INF
	cw, err := zw.Create("META-INF/CERT.RSA")
	if err == nil {
		_, _ = cw.Write([]byte("MOCK_CERTIFICATE_PAYLOAD_TESTING_PURPOSES_ONLY_1234567890"))
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}
}
