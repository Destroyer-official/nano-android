package signing

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"debug/elf"
	"encoding/asn1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CertificateInfo holds parsed X.509 signing certificate details.
type CertificateInfo struct {
	Subject       string `json:"subject"`
	Issuer        string `json:"issuer"`
	SerialHex     string `json:"serial_hex"`
	NotBefore     string `json:"not_before"`
	NotAfter      string `json:"not_after"`
	SHA256        string `json:"sha256_fingerprint"`
	SignatureAlgo string `json:"signature_algorithm"`
}

// SigningReport summarizes APK signing schemes (v1/v2/v3/v4) and certificates.
type SigningReport struct {
	HasV1        bool              `json:"has_v1"`
	HasV2        bool              `json:"has_v2"`
	HasV3        bool              `json:"has_v3"`
	HasV4        bool              `json:"has_v4"`
	IsSigned     bool              `json:"is_signed"`
	Certificates []CertificateInfo `json:"certificates,omitempty"`
	Warnings     []string          `json:"warnings,omitempty"`
}

// NativeLibInfo holds ELF metadata, ABI, JNI entrypoints, and 16 KB alignment status.
type NativeLibInfo struct {
	Path          string   `json:"path"`
	ABI           string   `json:"abi"`
	Machine       string   `json:"machine"`
	NeededLibs    []string `json:"needed_libs,omitempty"`
	JNISymbols    []string `json:"jni_symbols,omitempty"`
	MaxAlign      uint64   `json:"max_align"`
	Is16KBAligned bool     `json:"is_16kb_aligned"`
}

// InspectSigning inspects APK Signature Block (v2/v3), v4 (.idsig), and v1 META-INF certs.
func InspectSigning(apkPath string, zr *zip.Reader) SigningReport {
	rep := SigningReport{}
	seenCerts := map[string]bool{}
	appendUniqueCerts := func(certs []CertificateInfo) {
		for _, c := range certs {
			if c.SHA256 != "" && !seenCerts[c.SHA256] {
				seenCerts[c.SHA256] = true
				rep.Certificates = append(rep.Certificates, c)
			}
		}
	}

	if raw, err := readAPKSigningBlock(apkPath); err == nil && len(raw) > 0 {
		for _, kv := range parseSigningBlockPairs(raw) {
			switch kv.id {
			case 0x7109871a:
				rep.HasV2 = true
				appendUniqueCerts(extractCertsFromV2V3Block(kv.value))
			case 0xf05368c0, 0x1b93ad61:
				rep.HasV3 = true
				appendUniqueCerts(extractCertsFromV2V3Block(kv.value))
			}
		}
	}

	// Check APK Signature Scheme v4 (.idsig companion file)
	if idsigData, err := os.ReadFile(apkPath + ".idsig"); err == nil && len(idsigData) >= 8 {
		ver := binary.LittleEndian.Uint32(idsigData[0:4])
		if ver == 2 {
			rep.HasV4 = true
		}
	}

	if zr != nil {
		for _, f := range zr.File {
			up := strings.ToUpper(f.Name)
			if strings.HasPrefix(up, "META-INF/") && (strings.HasSuffix(up, ".RSA") || strings.HasSuffix(up, ".DSA") || strings.HasSuffix(up, ".EC")) {
				rep.HasV1 = true
				if rc, err := f.Open(); err == nil {
					data, _ := io.ReadAll(io.LimitReader(rc, 2<<20))
					_ = rc.Close()
					appendUniqueCerts(extractCertsFromPKCS7(data))
				}
			}
		}
	}

	rep.IsSigned = rep.HasV1 || rep.HasV2 || rep.HasV3 || rep.HasV4
	if !rep.IsSigned {
		rep.Warnings = append(rep.Warnings, "APK has no v1/v2/v3/v4 signature; cannot install on retail Android devices without debug/re-signing")
	}
	return rep
}

type sigBlockPair struct {
	id    uint32
	value []byte
}

func parseSigningBlockPairs(block []byte) []sigBlockPair {
	// APK Signing Block layout: uint64 sizeOfBlock | (uint64 pairLen, uint32 id, value)... | uint64 sizeOfBlock | "APK Sig Block 42"
	if len(block) < 32 {
		return nil
	}
	pos := 8
	end := len(block) - 24
	var out []sigBlockPair
	for pos+12 <= end {
		pairLen := int(binary.LittleEndian.Uint64(block[pos : pos+8]))
		pos += 8
		if pairLen < 4 || pos+pairLen > end {
			break
		}
		id := binary.LittleEndian.Uint32(block[pos : pos+4])
		val := block[pos+4 : pos+pairLen]
		out = append(out, sigBlockPair{id: id, value: val})
		pos += pairLen
	}
	return out
}

func readLenPrefixedSlice(b []byte) ([]byte, []byte, bool) {
	if len(b) < 4 {
		return nil, nil, false
	}
	n := int(binary.LittleEndian.Uint32(b[0:4]))
	if n < 0 || 4+n > len(b) {
		return nil, nil, false
	}
	return b[4 : 4+n], b[4+n:], true
}

func extractCertsFromV2V3Block(schemePayload []byte) []CertificateInfo {
	// v2/v3 payload: length-prefixed sequence of signers
	signersSeq, _, ok := readLenPrefixedSlice(schemePayload)
	if !ok {
		return nil
	}
	var out []CertificateInfo
	rem := signersSeq
	for len(rem) >= 4 {
		signer, nextRem, ok := readLenPrefixedSlice(rem)
		if !ok {
			break
		}
		rem = nextRem
		signedData, _, ok := readLenPrefixedSlice(signer)
		if !ok {
			continue
		}
		// signedData: digests (lp) | certificates (lp) | ...
		_, afterDigests, ok := readLenPrefixedSlice(signedData)
		if !ok {
			continue
		}
		certsSeq, _, ok := readLenPrefixedSlice(afterDigests)
		if !ok {
			continue
		}
		cRem := certsSeq
		for len(cRem) >= 4 {
			certDER, nextCRem, ok := readLenPrefixedSlice(cRem)
			if !ok {
				break
			}
			cRem = nextCRem
			if c, err := x509.ParseCertificate(certDER); err == nil {
				sum := sha256.Sum256(c.Raw)
				out = append(out, CertificateInfo{
					Subject:       c.Subject.String(),
					Issuer:        c.Issuer.String(),
					SerialHex:     fmt.Sprintf("%x", c.SerialNumber),
					NotBefore:     c.NotBefore.UTC().Format("2006-01-02T15:04:05Z"),
					NotAfter:      c.NotAfter.UTC().Format("2006-01-02T15:04:05Z"),
					SHA256:        hex.EncodeToString(sum[:]),
					SignatureAlgo: c.SignatureAlgorithm.String(),
				})
			}
		}
	}
	return out
}

// InspectNativeLibs audits all .so ELF binaries inside an APK.
func InspectNativeLibs(zr *zip.Reader) []NativeLibInfo {
	if zr == nil {
		return nil
	}
	var out []NativeLibInfo
	for _, f := range zr.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".so") || f.UncompressedSize64 > 128<<20 {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, rErr := io.ReadAll(io.LimitReader(rc, 128<<20))
		_ = rc.Close()
		if rErr != nil {
			continue
		}
		ef, err := elf.NewFile(bytes.NewReader(data))
		if err != nil {
			continue
		}
		info := NativeLibInfo{
			Path:          f.Name,
			ABI:           abiFromPath(f.Name),
			Machine:       ef.Machine.String(),
			Is16KBAligned: true,
		}
		if libs, err := ef.ImportedLibraries(); err == nil {
			info.NeededLibs = libs
		}
		if syms, err := ef.DynamicSymbols(); err == nil {
			for _, s := range syms {
				if s.Name == "JNI_OnLoad" || strings.HasPrefix(s.Name, "Java_") {
					info.JNISymbols = append(info.JNISymbols, s.Name)
					if len(info.JNISymbols) >= 64 {
						break
					}
				}
			}
		}
		for _, p := range ef.Progs {
			if p.Type == elf.PT_LOAD {
				if p.Align > info.MaxAlign {
					info.MaxAlign = p.Align
				}
				if p.Align < 0x4000 {
					info.Is16KBAligned = false
				}
			}
		}
		_ = ef.Close()
		out = append(out, info)
	}
	return out
}

func abiFromPath(p string) string {
	clean := filepath.ToSlash(p)
	parts := strings.Split(clean, "/")
	if len(parts) >= 3 && parts[0] == "lib" {
		return parts[1]
	}
	return "unknown"
}

func readAPKSigningBlock(apkPath string) ([]byte, error) {
	f, err := os.Open(apkPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() < 64 {
		return nil, fmt.Errorf("file too small")
	}
	tailSize := int64(256 * 1024)
	if st.Size() < tailSize {
		tailSize = st.Size()
	}
	buf := make([]byte, tailSize)
	if _, err := f.ReadAt(buf, st.Size()-tailSize); err != nil {
		return nil, err
	}
	magic := []byte("APK Sig Block 42")
	idx := bytes.Index(buf, magic)
	if idx < 8 {
		return nil, nil
	}
	blockSize := int(binary.LittleEndian.Uint64(buf[idx-8 : idx]))
	start := idx + 16 - blockSize - 8
	if start < 0 || start > idx {
		start = 0
	}
	return buf[start : idx+16], nil
}

type pkcs7ContentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"explicit,optional,tag:0"`
}

type pkcs7SignedData struct {
	Version          int
	DigestAlgorithms asn1.RawValue
	ContentInfo      asn1.RawValue
	Certificates     asn1.RawValue `asn1:"optional,tag:0"`
}

func extractCertsFromPKCS7(der []byte) []CertificateInfo {
	var ci pkcs7ContentInfo
	if _, err := asn1.Unmarshal(der, &ci); err != nil {
		return nil
	}
	var sd pkcs7SignedData
	if _, err := asn1.Unmarshal(ci.Content.Bytes, &sd); err != nil {
		return nil
	}
	certs, err := x509.ParseCertificates(sd.Certificates.Bytes)
	if err != nil || len(certs) == 0 {
		return nil
	}
	var out []CertificateInfo
	for _, c := range certs {
		sum := sha256.Sum256(c.Raw)
		out = append(out, CertificateInfo{
			Subject:       c.Subject.String(),
			Issuer:        c.Issuer.String(),
			SerialHex:     fmt.Sprintf("%x", c.SerialNumber),
			NotBefore:     c.NotBefore.UTC().Format("2006-01-02T15:04:05Z"),
			NotAfter:      c.NotAfter.UTC().Format("2006-01-02T15:04:05Z"),
			SHA256:        hex.EncodeToString(sum[:]),
			SignatureAlgo: c.SignatureAlgorithm.String(),
		})
	}
	return out
}
