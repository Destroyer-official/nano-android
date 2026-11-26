package resources

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
)

const (
	resStringPoolType = 0x0001
	resTableType      = 0x0002
	resTablePkgType   = 0x0200
	resTableTypeType  = 0x0201
	resTableSpecType  = 0x0202
	flagSparse        = 0x01
	flagOffset16      = 0x02
)

// Entry represents one resolved resource entry from resources.arsc.
type Entry struct {
	ID       uint32 `json:"id"`
	IDHex    string `json:"id_hex"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	DataType uint8  `json:"data_type"`
	Value    string `json:"value"`
}

// SizeBucket summarizes APK size distribution by category.
type SizeBucket struct {
	Category         string  `json:"category"`
	FileCount        int     `json:"file_count"`
	CompressedBytes  int64   `json:"compressed_bytes"`
	UncompressedBytes int64  `json:"uncompressed_bytes"`
	CompressionRatio float64 `json:"compression_ratio"`
}

// DuplicateAsset reports identical file payloads stored multiple times in an APK.
type DuplicateAsset struct {
	SHA256 string   `json:"sha256"`
	Size   int64    `json:"size_bytes"`
	Paths  []string `json:"paths"`
}

// Table holds decoded resources.arsc tables and APK archive resource metrics.
type Table struct {
	PackageID   uint32           `json:"package_id"`
	PackageName string           `json:"package_name"`
	Entries     map[uint32]Entry `json:"entries"`
	Strings     []string         `json:"strings"`
	SizeBuckets []SizeBucket     `json:"size_buckets,omitempty"`
	Duplicates  []DuplicateAsset `json:"duplicates,omitempty"`
}

// ParseARSC decodes a binary resources.arsc payload.
func ParseARSC(data []byte) (*Table, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("resources.arsc too short (%d bytes)", len(data))
	}
	cType := binary.LittleEndian.Uint16(data[0:2])
	hdrSize := int(binary.LittleEndian.Uint16(data[2:4]))
	if cType != resTableType || hdrSize > len(data) {
		return nil, fmt.Errorf("invalid resources.arsc root chunk 0x%04x", cType)
	}

	tbl := &Table{
		Entries: make(map[uint32]Entry),
	}

	offset := hdrSize
	for offset+8 <= len(data) {
		subType := binary.LittleEndian.Uint16(data[offset : offset+2])
		subSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		if subSize < 8 || offset+subSize > len(data) {
			break
		}
		chunk := data[offset : offset+subSize]
		switch subType {
		case resStringPoolType:
			tbl.Strings = parsePool(chunk)
		case resTablePkgType:
			parsePackageChunk(chunk, tbl)
		}
		offset += subSize
	}
	return tbl, nil
}

// ResolveID resolves a 32-bit Android resource ID (e.g. 0x7f010002).
func (t *Table) ResolveID(id uint32) (Entry, bool) {
	e, ok := t.Entries[id]
	return e, ok
}

// SearchStrings returns matching global/resource strings up to limit.
func (t *Table) SearchStrings(query string, limit int) []string {
	if limit <= 0 {
		limit = 50
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var out []string
	for _, s := range t.Strings {
		if q == "" || strings.Contains(strings.ToLower(s), q) {
			out = append(out, s)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

// AnalyzeArchiveSizes inspects an APK/ZIP for size distribution, compression ratios, and duplicate assets.
func AnalyzeArchiveSizes(zr *zip.Reader) ([]SizeBucket, []DuplicateAsset) {
	buckets := map[string]*SizeBucket{}
	hashMap := map[string][]string{}
	sizeMap := map[string]int64{}

	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		cat := categorizePath(f.Name)
		b, ok := buckets[cat]
		if !ok {
			b = &SizeBucket{Category: cat}
			buckets[cat] = b
		}
		b.FileCount++
		b.CompressedBytes += int64(f.CompressedSize64)
		b.UncompressedBytes += int64(f.UncompressedSize64)

		if f.UncompressedSize64 > 0 && f.UncompressedSize64 <= 16*1024*1024 {
			if rc, err := f.Open(); err == nil {
				h := sha256.New()
				if n, cpErr := io.Copy(h, io.LimitReader(rc, 16*1024*1024)); cpErr == nil && n > 0 {
					sum := hex.EncodeToString(h.Sum(nil))
					hashMap[sum] = append(hashMap[sum], f.Name)
					sizeMap[sum] = n
				}
				_ = rc.Close()
			}
		}
	}

	var outBuckets []SizeBucket
	for _, b := range buckets {
		if b.UncompressedBytes > 0 {
			b.CompressionRatio = float64(b.CompressedBytes) / float64(b.UncompressedBytes)
		}
		outBuckets = append(outBuckets, *b)
	}
	var dups []DuplicateAsset
	for sum, paths := range hashMap {
		if len(paths) > 1 {
			dups = append(dups, DuplicateAsset{SHA256: sum, Size: sizeMap[sum], Paths: paths})
		}
	}
	return outBuckets, dups
}

func categorizePath(name string) string {
	low := strings.ToLower(name)
	switch {
	case strings.HasPrefix(low, "classes") && strings.HasSuffix(low, ".dex"):
		return "dex"
	case strings.HasPrefix(low, "lib/"):
		return "native_libs"
	case strings.HasPrefix(low, "res/"):
		return "resources"
	case strings.HasPrefix(low, "assets/"):
		return "assets"
	case low == "resources.arsc":
		return "resource_table"
	case strings.HasPrefix(low, "meta-inf/"):
		return "signing_meta"
	default:
		return "other"
	}
}

func parsePackageChunk(chunk []byte, tbl *Table) {
	if len(chunk) < 288 {
		return
	}
	hdrSize := int(binary.LittleEndian.Uint16(chunk[2:4]))
	pkgID := binary.LittleEndian.Uint32(chunk[8:12])
	tbl.PackageID = pkgID
	tbl.PackageName = decodeUTF16Fixed(chunk[12:268])

	typeStringsOff := int(binary.LittleEndian.Uint32(chunk[268:272]))
	keyStringsOff := int(binary.LittleEndian.Uint32(chunk[276:280]))

	var typePool, keyPool []string
	if typeStringsOff > 0 && typeStringsOff+8 <= len(chunk) {
		typePool = parsePool(chunk[typeStringsOff:])
	}
	if keyStringsOff > 0 && keyStringsOff+8 <= len(chunk) {
		keyPool = parsePool(chunk[keyStringsOff:])
	}

	off := hdrSize
	for off+8 <= len(chunk) {
		cType := binary.LittleEndian.Uint16(chunk[off : off+2])
		cSize := int(binary.LittleEndian.Uint32(chunk[off+4 : off+8]))
		if cSize < 8 || off+cSize > len(chunk) {
			break
		}
		if cType == resTableTypeType {
			parseTypeChunk(chunk[off:off+cSize], pkgID, typePool, keyPool, tbl)
		}
		off += cSize
	}
}

func parseTypeChunk(chunk []byte, pkgID uint32, typePool, keyPool []string, tbl *Table) {
	if len(chunk) < 24 {
		return
	}
	hdrSize := int(binary.LittleEndian.Uint16(chunk[2:4]))
	typeID := uint32(chunk[8])
	flags := chunk[9]
	entryCount := int(binary.LittleEndian.Uint32(chunk[12:16]))
	entriesStart := int(binary.LittleEndian.Uint32(chunk[16:20]))
	if typeID == 0 || entryCount <= 0 || entryCount > 200000 || entriesStart > len(chunk) {
		return
	}

	typeName := ""
	if int(typeID-1) < len(typePool) {
		typeName = typePool[typeID-1]
	}
	isSparse := (flags & flagSparse) != 0
	isOffset16 := (flags & flagOffset16) != 0

	for i := 0; i < entryCount; i++ {
		var entryIdx uint32
		var entryOff int
		if isSparse {
			pos := hdrSize + i*4
			if pos+4 > len(chunk) {
				break
			}
			entryIdx = uint32(binary.LittleEndian.Uint16(chunk[pos : pos+2]))
			entryOff = int(binary.LittleEndian.Uint16(chunk[pos+2:pos+4])) * 4
		} else if isOffset16 {
			pos := hdrSize + i*2
			if pos+2 > len(chunk) {
				break
			}
			v := binary.LittleEndian.Uint16(chunk[pos : pos+2])
			if v == 0xFFFF {
				continue
			}
			entryIdx = uint32(i)
			entryOff = int(v) * 4
		} else {
			pos := hdrSize + i*4
			if pos+4 > len(chunk) {
				break
			}
			v := binary.LittleEndian.Uint32(chunk[pos : pos+4])
			if v == 0xFFFFFFFF {
				continue
			}
			entryIdx = uint32(i)
			entryOff = int(v)
		}

		ePos := entriesStart + entryOff
		if ePos+8 > len(chunk) {
			continue
		}
		eFlags := binary.LittleEndian.Uint16(chunk[ePos+2 : ePos+4])
		keyIdx := int(binary.LittleEndian.Uint32(chunk[ePos+4 : ePos+8]))
		keyName := ""
		if keyIdx >= 0 && keyIdx < len(keyPool) {
			keyName = keyPool[keyIdx]
		}
		resID := (pkgID << 24) | (typeID << 16) | entryIdx
		if (eFlags & 0x0001) != 0 {
			// Complex/bag entry
			if _, exists := tbl.Entries[resID]; !exists {
				tbl.Entries[resID] = Entry{
					ID:    resID,
					IDHex: fmt.Sprintf("0x%08x", resID),
					Type:  typeName,
					Name:  keyName,
					Value: "<style/bag>",
				}
			}
			continue
		}
		if ePos+16 > len(chunk) {
			continue
		}
		dType := chunk[ePos+11]
		dVal := binary.LittleEndian.Uint32(chunk[ePos+12 : ePos+16])
		valStr := formatResValue(dType, dVal, tbl.Strings)
		if _, exists := tbl.Entries[resID]; !exists {
			tbl.Entries[resID] = Entry{
				ID:       resID,
				IDHex:    fmt.Sprintf("0x%08x", resID),
				Type:     typeName,
				Name:     keyName,
				DataType: dType,
				Value:    valStr,
			}
		}
	}
}

func formatResValue(dType uint8, dVal uint32, pool []string) string {
	switch dType {
	case 0x03:
		if int(dVal) < len(pool) {
			return pool[dVal]
		}
	case 0x01:
		return fmt.Sprintf("@0x%08x", dVal)
	case 0x12:
		if dVal != 0 {
			return "true"
		}
		return "false"
	case 0x1c, 0x1d, 0x1e, 0x1f:
		return fmt.Sprintf("#%08x", dVal)
	}
	return fmt.Sprintf("0x%x", dVal)
}

func parsePool(chunk []byte) []string {
	if len(chunk) < 28 {
		return nil
	}
	cSize := int(binary.LittleEndian.Uint32(chunk[4:8]))
	if cSize > len(chunk) {
		cSize = len(chunk)
	}
	count := int(binary.LittleEndian.Uint32(chunk[8:12]))
	flags := binary.LittleEndian.Uint32(chunk[16:20])
	stringsStart := int(binary.LittleEndian.Uint32(chunk[20:24]))
	isUTF8 := (flags & (1 << 8)) != 0
	if count <= 0 || count > 500000 || 28+count*4 > cSize || stringsStart > cSize {
		return nil
	}
	out := make([]string, count)
	for i := 0; i < count; i++ {
		off := int(binary.LittleEndian.Uint32(chunk[28+i*4 : 32+i*4]))
		pos := stringsStart + off
		if pos < 0 || pos >= cSize {
			continue
		}
		if isUTF8 {
			_, n1 := readLen8(chunk[pos:cSize])
			pos += n1
			if pos >= cSize {
				continue
			}
			bLen, n2 := readLen8(chunk[pos:cSize])
			pos += n2
			if pos+bLen <= cSize {
				out[i] = string(chunk[pos : pos+bLen])
			}
		} else {
			cLen, n := readLen16(chunk[pos:cSize])
			pos += n
			if pos+cLen*2 <= cSize {
				u16 := make([]uint16, cLen)
				for k := 0; k < cLen; k++ {
					u16[k] = binary.LittleEndian.Uint16(chunk[pos+k*2 : pos+k*2+2])
				}
				out[i] = string(utf16.Decode(u16))
			}
		}
	}
	return out
}

func readLen8(b []byte) (int, int) {
	if len(b) == 0 {
		return 0, 0
	}
	if b[0]&0x80 == 0 {
		return int(b[0]), 1
	}
	if len(b) < 2 {
		return 0, 1
	}
	return (int(b[0]&0x7F) << 8) | int(b[1]), 2
}

func readLen16(b []byte) (int, int) {
	if len(b) < 2 {
		return 0, len(b)
	}
	first := binary.LittleEndian.Uint16(b[0:2])
	if first&0x8000 == 0 {
		return int(first), 2
	}
	if len(b) < 4 {
		return 0, 2
	}
	second := binary.LittleEndian.Uint16(b[2:4])
	return (int(first&0x7FFF) << 16) | int(second), 4
}

func decodeUTF16Fixed(b []byte) string {
	var u16 []uint16
	for i := 0; i+1 < len(b); i += 2 {
		v := binary.LittleEndian.Uint16(b[i : i+2])
		if v == 0 {
			break
		}
		u16 = append(u16, v)
	}
	return string(utf16.Decode(u16))
}
