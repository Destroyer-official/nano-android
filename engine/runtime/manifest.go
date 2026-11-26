package runtime

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf16"
)

// AXML Chunk Types
const (
	ChunkResXMLTree          = 0x0003
	ChunkStringPool          = 0x0001
	ChunkResXMLResourceMap   = 0x0180
	ChunkResXMLStartNamespace = 0x0100
	ChunkResXMLEndNamespace   = 0x0101
	ChunkResXMLStartElement   = 0x0102
	ChunkResXMLEndElement     = 0x0103
	ChunkResXMLCData          = 0x0104
)

// ActivityManifest represents an <activity> declaration
type ActivityManifest struct {
	Name          string
	Exported      bool
	IntentFilters []IntentFilterManifest
}

// IntentFilterManifest represents an <intent-filter>
type IntentFilterManifest struct {
	Actions    []string
	Categories []string
}

// AndroidManifest holds key metadata extracted from AndroidManifest.xml
type AndroidManifest struct {
	PackageName      string
	VersionCode      int
	VersionName      string
	MinSdkVersion    int
	TargetSdkVersion int
	Activities       []ActivityManifest
	LauncherActivity string
	Permissions      []string
}

// ParseManifestFromAPK reads and parses AndroidManifest.xml from an APK
func ParseManifestFromAPK(apkPath string) (*AndroidManifest, error) {
	zr, err := zip.OpenReader(apkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open APK: %w", err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name == "AndroidManifest.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
			return ParseManifest(data)
		}
	}

	return nil, errors.New("AndroidManifest.xml not found in APK")
}

// ParseManifest parses either binary AXML or plain text XML AndroidManifest
func ParseManifest(data []byte) (*AndroidManifest, error) {
	if len(data) < 8 {
		return nil, errors.New("manifest data too short")
	}

	// Check for binary XML signature (0x0003 or 0x00080003)
	chunkType := binary.LittleEndian.Uint16(data[0:2])
	if chunkType == ChunkResXMLTree {
		return parseBinaryAXML(data)
	}

	// Fallback to standard UTF-8 XML parsing
	return parsePlainTextManifest(data)
}

func parseBinaryAXML(data []byte) (*AndroidManifest, error) {
	if len(data) < 8 {
		return nil, errors.New("invalid AXML size")
	}

	offset := 8
	var stringsPool []string
	manifest := &AndroidManifest{
		Activities:  make([]ActivityManifest, 0),
		Permissions: make([]string, 0),
	}

	var currentActivity *ActivityManifest
	var currentFilter *IntentFilterManifest

	for offset+8 <= len(data) {
		chunkType := binary.LittleEndian.Uint16(data[offset : offset+2])
		headerSize := binary.LittleEndian.Uint16(data[offset+2 : offset+4])
		chunkSize := binary.LittleEndian.Uint32(data[offset+4 : offset+8])

		if chunkSize < 8 || int(offset)+int(chunkSize) > len(data) {
			break
		}

		switch chunkType {
		case ChunkStringPool:
			pool, err := parseStringPool(data[offset : offset+int(chunkSize)])
			if err == nil {
				stringsPool = pool
			}

		case ChunkResXMLStartElement:
			if int(offset)+int(headerSize) <= len(data) && len(stringsPool) > 0 {
				elemNameIdx := binary.LittleEndian.Uint32(data[offset+20 : offset+24])
				elemName := ""
				if int(elemNameIdx) < len(stringsPool) {
					elemName = stringsPool[elemNameIdx]
				}

				attrStart := binary.LittleEndian.Uint16(data[offset+24 : offset+26])
				attrSize := binary.LittleEndian.Uint16(data[offset+26 : offset+28])
				attrCount := binary.LittleEndian.Uint16(data[offset+28 : offset+30])

				attrs := make(map[string]string)
				attrOffset := int(offset) + 16 + int(attrStart)
				if attrStart > 30 {
					attrOffset = int(offset) + int(attrStart)
				}
				for i := 0; i < int(attrCount); i++ {
					if attrOffset+20 <= len(data) {
						nameIdx := binary.LittleEndian.Uint32(data[attrOffset+4 : attrOffset+8])
						valIdx := binary.LittleEndian.Uint32(data[attrOffset+8 : attrOffset+12])
						dataType := data[attrOffset+15]
						dataVal := binary.LittleEndian.Uint32(data[attrOffset+16 : attrOffset+20])

						name := ""
						val := ""
						if int(nameIdx) < len(stringsPool) {
							name = stringsPool[nameIdx]
						}
						if valIdx != 0xFFFFFFFF && int(valIdx) < len(stringsPool) {
							val = stringsPool[valIdx]
						} else if dataType == 0x03 && int(dataVal) < len(stringsPool) {
							val = stringsPool[dataVal]
						} else if dataType == 0x10 || dataType == 0x11 {
							val = fmt.Sprintf("%d", dataVal)
						} else if dataType == 0x12 {
							if dataVal != 0 {
								val = "true"
							} else {
								val = "false"
							}
						}
						if name != "" {
							attrs[name] = val
						}
					}
					attrOffset += int(attrSize)
				}

				switch elemName {
				case "manifest":
					if pkg, ok := attrs["package"]; ok {
						manifest.PackageName = pkg
					}
					if vc, ok := attrs["versionCode"]; ok {
						manifest.VersionCode, _ = strconv.Atoi(vc)
					}
					if vn, ok := attrs["versionName"]; ok {
						manifest.VersionName = vn
					}
				case "uses-sdk":
					if tsdk, ok := attrs["targetSdkVersion"]; ok {
						manifest.TargetSdkVersion, _ = strconv.Atoi(tsdk)
					}
					if msdk, ok := attrs["minSdkVersion"]; ok {
						manifest.MinSdkVersion, _ = strconv.Atoi(msdk)
					}
				case "activity":
					act := ActivityManifest{
						Name:          attrs["name"],
						Exported:      attrs["exported"] == "true",
						IntentFilters: make([]IntentFilterManifest, 0),
					}
					currentActivity = &act
				case "intent-filter":
					currentFilter = &IntentFilterManifest{
						Actions:    make([]string, 0),
						Categories: make([]string, 0),
					}
				case "action":
					if currentFilter != nil && attrs["name"] != "" {
						currentFilter.Actions = append(currentFilter.Actions, attrs["name"])
					}
				case "category":
					if currentFilter != nil && attrs["name"] != "" {
						currentFilter.Categories = append(currentFilter.Categories, attrs["name"])
					}
				case "uses-permission":
					if p, ok := attrs["name"]; ok && p != "" {
						manifest.Permissions = append(manifest.Permissions, p)
					}
				}
			}

		case ChunkResXMLEndElement:
			if len(stringsPool) > 0 && offset+24 <= len(data) {
				elemNameIdx := binary.LittleEndian.Uint32(data[offset+20 : offset+24])
				elemName := ""
				if int(elemNameIdx) < len(stringsPool) {
					elemName = stringsPool[elemNameIdx]
				}

				if elemName == "intent-filter" && currentActivity != nil && currentFilter != nil {
					currentActivity.IntentFilters = append(currentActivity.IntentFilters, *currentFilter)

					// Check if this intent-filter qualifies as LAUNCHER
					isMain := false
					isLauncher := false
					for _, a := range currentFilter.Actions {
						if a == "android.intent.action.MAIN" {
							isMain = true
							break
						}
					}
					for _, c := range currentFilter.Categories {
						if c == "android.intent.category.LAUNCHER" {
							isLauncher = true
							break
						}
					}
					if isMain && isLauncher && manifest.LauncherActivity == "" {
						manifest.LauncherActivity = currentActivity.Name
					}
					currentFilter = nil
				} else if elemName == "activity" && currentActivity != nil {
					manifest.Activities = append(manifest.Activities, *currentActivity)
					currentActivity = nil
				}
			}
		}

		offset += int(chunkSize)
	}

	// Fallback package resolution if attribute package was stored differently
	if manifest.PackageName == "" || strings.HasPrefix(manifest.PackageName, "com.google.android") || strings.HasPrefix(manifest.PackageName, "com.android.") {
		for _, s := range stringsPool {
			if !strings.Contains(s, " ") && !strings.Contains(s, "/") && !strings.Contains(s, ":") && strings.Count(s, ".") >= 2 {
				if (strings.HasPrefix(s, "org.") || strings.HasPrefix(s, "com.") || strings.HasPrefix(s, "de.") || strings.HasPrefix(s, "net.")) &&
					!strings.Contains(s, "android.gms") && !strings.Contains(s, "androidx") && !strings.Contains(s, "android.support") && !strings.Contains(s, "com.android.") {
					manifest.PackageName = s
					break
				}
			}
		}
	}
	// Fallback permissions scan from string pool
	if len(manifest.Permissions) == 0 {
		for _, s := range stringsPool {
			if strings.HasPrefix(s, "android.permission.") {
				manifest.Permissions = append(manifest.Permissions, s)
			}
		}
	}

	// Normalize launcher activity name if relative (e.g. ".MainActivity")
	if manifest.LauncherActivity != "" && strings.HasPrefix(manifest.LauncherActivity, ".") {
		manifest.LauncherActivity = manifest.PackageName + manifest.LauncherActivity
	}

	// If no launcher identified yet, fallback to the first declared activity or stringsPool
	if manifest.LauncherActivity == "" && len(manifest.Activities) > 0 {
		manifest.LauncherActivity = manifest.Activities[0].Name
		if strings.HasPrefix(manifest.LauncherActivity, ".") {
			manifest.LauncherActivity = manifest.PackageName + manifest.LauncherActivity
		}
	}
	if manifest.LauncherActivity == "" || strings.Contains(manifest.LauncherActivity, "tooling") || strings.Contains(manifest.LauncherActivity, "processphoenix") {
		for _, s := range stringsPool {
			if strings.HasSuffix(s, "MainActivity") && strings.Contains(s, ".") && !strings.Contains(s, "androidx") {
				manifest.LauncherActivity = s
				break
			}
		}
	}

	return manifest, nil
}

func parseStringPool(chunk []byte) ([]string, error) {
	if len(chunk) < 28 {
		return nil, errors.New("string pool chunk too small")
	}

	stringCount := binary.LittleEndian.Uint32(chunk[8:12])
	flags := binary.LittleEndian.Uint32(chunk[16:20])
	stringsStart := binary.LittleEndian.Uint32(chunk[20:24])

	isUTF8 := (flags & (1 << 8)) != 0

	offsets := make([]uint32, stringCount)
	for i := uint32(0); i < stringCount; i++ {
		offPos := 28 + (i * 4)
		if offPos+4 > uint32(len(chunk)) {
			break
		}
		offsets[i] = binary.LittleEndian.Uint32(chunk[offPos : offPos+4])
	}

	res := make([]string, stringCount)
	for i, off := range offsets {
		strPos := int(stringsStart + off)
		if strPos >= len(chunk) {
			continue
		}

		if isUTF8 {
			if strPos+1 >= len(chunk) {
				continue
			}
			p := strPos
			if (chunk[p] & 0x80) != 0 {
				p += 2
			} else {
				p += 1
			}
			if p >= len(chunk) {
				continue
			}
			var strLen int
			if (chunk[p] & 0x80) != 0 {
				if p+1 >= len(chunk) {
					continue
				}
				strLen = (int(chunk[p]&0x7f) << 8) | int(chunk[p+1])
				p += 2
			} else {
				strLen = int(chunk[p])
				p += 1
			}
			if p+strLen <= len(chunk) {
				res[i] = string(chunk[p : p+strLen])
			}
		} else {
			// UTF-16LE null-terminated
			p := strPos + 2 // skip 2-byte length
			var u16s []uint16
			for p+2 <= len(chunk) {
				val := binary.LittleEndian.Uint16(chunk[p : p+2])
				if val == 0 {
					break
				}
				u16s = append(u16s, val)
				p += 2
			}
			res[i] = string(utf16.Decode(u16s))
		}
	}

	return res, nil
}

// Plain text XML parsing structures for fallback
type xmlManifest struct {
	Package     string        `xml:"package,attr"`
	Application xmlApplication `xml:"application"`
	Permissions []xmlPerm     `xml:"uses-permission"`
}

type xmlPerm struct {
	Name string `xml:"http://schemas.android.com/apk/res/android name,attr"`
}

type xmlApplication struct {
	Activities []xmlActivity `xml:"activity"`
}

type xmlActivity struct {
	Name          string           `xml:"http://schemas.android.com/apk/res/android name,attr"`
	Exported      string           `xml:"http://schemas.android.com/apk/res/android exported,attr"`
	IntentFilters []xmlIntentFilter `xml:"intent-filter"`
}

type xmlIntentFilter struct {
	Actions    []xmlName `xml:"action"`
	Categories []xmlName `xml:"category"`
}

type xmlName struct {
	Name string `xml:"http://schemas.android.com/apk/res/android name,attr"`
}

func parsePlainTextManifest(data []byte) (*AndroidManifest, error) {
	var parsed xmlManifest
	if err := xml.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest xml: %w", err)
	}

	manifest := &AndroidManifest{
		PackageName: parsed.Package,
		Activities:  make([]ActivityManifest, len(parsed.Application.Activities)),
		Permissions: make([]string, len(parsed.Permissions)),
	}

	for i, p := range parsed.Permissions {
		manifest.Permissions[i] = p.Name
	}

	for i, act := range parsed.Application.Activities {
		actName := act.Name
		if strings.HasPrefix(actName, ".") {
			actName = parsed.Package + actName
		}
		manifest.Activities[i] = ActivityManifest{
			Name:     actName,
			Exported: act.Exported == "true",
		}
		for _, filter := range act.IntentFilters {
			filterManifest := IntentFilterManifest{
				Actions:    make([]string, len(filter.Actions)),
				Categories: make([]string, len(filter.Categories)),
			}
			isMain := false
			isLauncher := false
			for j, a := range filter.Actions {
				filterManifest.Actions[j] = a.Name
				if a.Name == "android.intent.action.MAIN" {
					isMain = true
				}
			}
			for j, c := range filter.Categories {
				filterManifest.Categories[j] = c.Name
				if c.Name == "android.intent.category.LAUNCHER" {
					isLauncher = true
				}
			}
			manifest.Activities[i].IntentFilters = append(manifest.Activities[i].IntentFilters, filterManifest)
			if isMain && isLauncher && manifest.LauncherActivity == "" {
				manifest.LauncherActivity = actName
			}
		}
	}

	if manifest.LauncherActivity == "" && len(manifest.Activities) > 0 {
		manifest.LauncherActivity = manifest.Activities[0].Name
	}

	return manifest, nil
}

// CreateTestAPKWithManifest builds an APK containing both classes.dex and AndroidManifest.xml
func CreateTestAPKWithManifest(apkPath string, packageName string, launcherActivity string, specs ...SyntheticClassSpec) error {
	dexBytes, err := BuildSyntheticDEX(specs...)
	if err != nil {
		return fmt.Errorf("failed to build synthetic DEX: %w", err)
	}

	manifestXML := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="%s">
    <application>
        <activity android:name="%s" android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.MAIN"/>
                <category android:name="android.intent.category.LAUNCHER"/>
            </intent-filter>
        </activity>
    </application>
</manifest>`, packageName, launcherActivity)

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	// Write classes.dex
	wDex, err := zw.Create("classes.dex")
	if err != nil {
		return err
	}
	if _, err := wDex.Write(dexBytes); err != nil {
		return err
	}

	// Write AndroidManifest.xml
	wMan, err := zw.Create("AndroidManifest.xml")
	if err != nil {
		return err
	}
	if _, err := wMan.Write([]byte(manifestXML)); err != nil {
		return err
	}

	if err := zw.Close(); err != nil {
		return err
	}

	return os.WriteFile(apkPath, buf.Bytes(), 0644)
}
