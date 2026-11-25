package device

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf16"
)

// APKManifest holds metadata extracted directly from an APK's AndroidManifest.xml.
type APKManifest struct {
	PackageID    string   `json:"package_id"`
	VersionCode  int      `json:"version_code"`
	VersionName  string   `json:"version_name"`
	MinSDK       int      `json:"min_sdk"`
	TargetSDK    int      `json:"target_sdk"`
	MainActivity string   `json:"main_activity,omitempty"`
	Component    string   `json:"component,omitempty"` // "com.example/.MainActivity"
	Permissions  []string `json:"permissions,omitempty"`
	Activities   []string `json:"activities,omitempty"`
}

// ParseAPKFile opens an APK file, locates AndroidManifest.xml, and parses its binary XML.
func ParseAPKFile(apkPath string) (*APKManifest, error) {
	f, err := os.Open(apkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open APK %s: %w", apkPath, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	return ParseAPKReader(f, stat.Size())
}

// ParseAPKReader parses an APK from an io.ReaderAt.
func ParseAPKReader(r io.ReaderAt, size int64) (*APKManifest, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("failed reading APK zip structure: %w", err)
	}

	var manifestFile *zip.File
	for _, f := range zr.File {
		if f.Name == "AndroidManifest.xml" {
			manifestFile = f
			break
		}
	}

	if manifestFile == nil {
		return nil, errors.New("AndroidManifest.xml not found inside APK archive")
	}

	rc, err := manifestFile.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open AndroidManifest.xml: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("failed to read AndroidManifest.xml: %w", err)
	}

	return ParseAXML(data)
}

// Constants for Android Binary XML (AXML) chunk types.
const (
	chunkAXMLFile       uint32 = 0x00080003
	chunkStringPool     uint32 = 0x001C0001
	chunkResourceIDs    uint32 = 0x00080180
	chunkStartNamespace uint32 = 0x00100100
	chunkEndNamespace   uint32 = 0x00100101
	chunkStartElement   uint32 = 0x00100102
	chunkEndElement     uint32 = 0x00100103
	chunkText           uint32 = 0x00100104

	utf8Flag uint32 = 0x00000100
)

// ParseAXML decodes raw Android Binary XML bytes into an APKManifest.
func ParseAXML(data []byte) (*APKManifest, error) {
	if len(data) < 8 {
		return nil, errors.New("data too short for AXML header")
	}

	magic := binary.LittleEndian.Uint32(data[0:4])
	if magic != chunkAXMLFile && magic != 0x00080001 {
		return nil, fmt.Errorf("invalid AXML magic number: 0x%08X", magic)
	}

	fileSize := binary.LittleEndian.Uint32(data[4:8])
	if int(fileSize) > len(data) {
		// Tolerant of slight discrepancies in archive extraction
		fileSize = uint32(len(data))
	}

	manifest := &APKManifest{}
	var stringPool []string

	offset := 8
	var currentActivity string
	var inIntentFilter bool
	var hasMainAction bool
	var hasLauncherCategory bool

	for offset+8 <= len(data) {
		chunkType := binary.LittleEndian.Uint32(data[offset : offset+4])
		chunkSize := binary.LittleEndian.Uint32(data[offset+4 : offset+8])

		if chunkSize < 8 || offset+int(chunkSize) > len(data) {
			break
		}

		chunkData := data[offset : offset+int(chunkSize)]

		switch chunkType {
		case chunkStringPool:
			pool, err := parseStringPool(chunkData)
			if err == nil {
				stringPool = pool
			}

		case chunkStartElement:
			if len(chunkData) < 36 {
				break
			}
			nameIdx := binary.LittleEndian.Uint32(chunkData[20:24])
			tagName := getString(stringPool, nameIdx)

			attrStart := binary.LittleEndian.Uint16(chunkData[24:26])
			attrCount := binary.LittleEndian.Uint16(chunkData[28:30])

			attrs := parseAttributes(chunkData, attrStart, attrCount, stringPool)

			switch tagName {
			case "manifest":
				if pkg, ok := attrs["package"]; ok {
					manifest.PackageID = pkg.StrValue
				}
				if vc, ok := attrs["versionCode"]; ok {
					manifest.VersionCode = vc.IntValue
				}
				if vn, ok := attrs["versionName"]; ok {
					manifest.VersionName = vn.StrValue
				}

			case "uses-sdk":
				if min, ok := attrs["minSdkVersion"]; ok {
					manifest.MinSDK = min.IntValue
				}
				if target, ok := attrs["targetSdkVersion"]; ok {
					manifest.TargetSDK = target.IntValue
				}

			case "uses-permission":
				if perm, ok := attrs["name"]; ok {
					manifest.Permissions = append(manifest.Permissions, perm.StrValue)
				}

			case "activity", "activity-alias":
				if actName, ok := attrs["name"]; ok {
					currentActivity = actName.StrValue
					manifest.Activities = append(manifest.Activities, currentActivity)
				}
				inIntentFilter = false
				hasMainAction = false
				hasLauncherCategory = false

			case "intent-filter":
				inIntentFilter = true
				hasMainAction = false
				hasLauncherCategory = false

			case "action":
				if inIntentFilter {
					if a, ok := attrs["name"]; ok && a.StrValue == "android.intent.action.MAIN" {
						hasMainAction = true
					}
				}

			case "category":
				if inIntentFilter {
					if c, ok := attrs["name"]; ok && c.StrValue == "android.intent.category.LAUNCHER" {
						hasLauncherCategory = true
					}
				}
			}

		case chunkEndElement:
			if len(chunkData) < 24 {
				break
			}
			nameIdx := binary.LittleEndian.Uint32(chunkData[20:24])
			tagName := getString(stringPool, nameIdx)

			if tagName == "intent-filter" {
				if hasMainAction && hasLauncherCategory && currentActivity != "" {
					manifest.MainActivity = currentActivity
				}
				inIntentFilter = false
			} else if tagName == "activity" || tagName == "activity-alias" {
				currentActivity = ""
				inIntentFilter = false
				hasMainAction = false
				hasLauncherCategory = false
			}
		}

		offset += int(chunkSize)
	}

	// Format Component: "com.example/.MainActivity"
	if manifest.MainActivity != "" && manifest.PackageID != "" {
		act := manifest.MainActivity
		if strings.HasPrefix(act, ".") {
			manifest.Component = manifest.PackageID + "/" + act
		} else if strings.HasPrefix(act, manifest.PackageID+".") {
			rel := strings.TrimPrefix(act, manifest.PackageID)
			manifest.Component = manifest.PackageID + "/" + rel
		} else if !strings.Contains(act, ".") {
			manifest.Component = manifest.PackageID + "/." + act
		} else {
			manifest.Component = manifest.PackageID + "/" + act
		}
	}

	if manifest.PackageID == "" {
		return nil, errors.New("failed to extract package name from AndroidManifest.xml")
	}

	return manifest, nil
}

type attrValue struct {
	StrValue string
	IntValue int
}

func parseAttributes(data []byte, startOffset uint16, count uint16, pool []string) map[string]attrValue {
	attrs := make(map[string]attrValue)
	cur := int(startOffset)

	for i := 0; i < int(count); i++ {
		if cur+20 > len(data) {
			break
		}
		// Each attribute is 20 bytes:
		// ns(4), name(4), rawValue(4), typedValue.size(2), res0(1), dataType(1), data(4)
		nameIdx := binary.LittleEndian.Uint32(data[cur+4 : cur+8])
		rawValIdx := binary.LittleEndian.Uint32(data[cur+8 : cur+12])
		dataType := data[cur+15]
		typedData := binary.LittleEndian.Uint32(data[cur+16 : cur+20])

		attrName := getString(pool, nameIdx)
		strVal := getString(pool, rawValIdx)
		intVal := int(int32(typedData))

		// If string was not in rawValue, check if typed string
		if strVal == "" && dataType == 3 {
			strVal = getString(pool, typedData)
		}

		attrs[attrName] = attrValue{
			StrValue: strVal,
			IntValue: intVal,
		}

		cur += 20
	}

	return attrs
}

func parseStringPool(data []byte) ([]string, error) {
	if len(data) < 28 {
		return nil, errors.New("string pool chunk too short")
	}

	stringCount := binary.LittleEndian.Uint32(data[8:12])
	flags := binary.LittleEndian.Uint32(data[16:20])
	stringsStart := binary.LittleEndian.Uint32(data[20:24])

	isUTF8 := (flags & utf8Flag) != 0

	offsets := make([]uint32, stringCount)
	for i := 0; i < int(stringCount); i++ {
		offsetPos := 28 + i*4
		if offsetPos+4 > len(data) {
			break
		}
		offsets[i] = binary.LittleEndian.Uint32(data[offsetPos : offsetPos+4])
	}

	pool := make([]string, stringCount)
	for i := 0; i < int(stringCount); i++ {
		start := int(stringsStart) + int(offsets[i])
		if start >= len(data) {
			continue
		}

		if isUTF8 {
			pool[i] = readAXMLUTF8String(data[start:])
		} else {
			pool[i] = readAXMLUTF16String(data[start:])
		}
	}

	return pool, nil
}

func readAXMLUTF8String(data []byte) string {
	if len(data) < 2 {
		return ""
	}
	idx := 0
	// Skip char length (1 or 2 bytes)
	if data[idx]&0x80 != 0 {
		idx += 2
	} else {
		idx += 1
	}
	if idx >= len(data) {
		return ""
	}
	// Read byte length (1 or 2 bytes)
	byteLen := int(data[idx])
	if data[idx]&0x80 != 0 {
		if idx+1 >= len(data) {
			return ""
		}
		byteLen = ((int(data[idx]) & 0x7F) << 8) | int(data[idx+1])
		idx += 2
	} else {
		idx += 1
	}

	if idx+byteLen > len(data) {
		byteLen = len(data) - idx
	}

	strBytes := data[idx : idx+byteLen]
	// Strip null terminator if present
	strBytes = bytes.TrimSuffix(strBytes, []byte{0})
	return string(strBytes)
}

func readAXMLUTF16String(data []byte) string {
	if len(data) < 2 {
		return ""
	}
	idx := 0
	// Length in 16-bit words
	length := int(binary.LittleEndian.Uint16(data[idx : idx+2]))
	idx += 2
	if length&0x8000 != 0 {
		if idx+2 > len(data) {
			return ""
		}
		length = ((length & 0x7FFF) << 16) | int(binary.LittleEndian.Uint16(data[idx:idx+2]))
		idx += 2
	}

	u16 := make([]uint16, length)
	for i := 0; i < length; i++ {
		pos := idx + i*2
		if pos+2 > len(data) {
			break
		}
		u16[i] = binary.LittleEndian.Uint16(data[pos : pos+2])
	}

	return string(utf16.Decode(u16))
}

func getString(pool []string, idx uint32) string {
	if int(idx) >= 0 && int(idx) < len(pool) {
		return pool[idx]
	}
	return ""
}
