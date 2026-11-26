package manifest

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
)

const (
	chunkStringPool  = 0x0001
	chunkXML         = 0x0003
	chunkXMLStartNS  = 0x0100
	chunkXMLEndNS    = 0x0101
	chunkXMLStartTag = 0x0102
	chunkXMLEndTag   = 0x0103
	chunkXMLCdata    = 0x0104
	chunkXMLResMap   = 0x0180
)

// IntentFilter describes an <intent-filter> declared on a component.
type IntentFilter struct {
	Actions    []string   `json:"actions,omitempty"`
	Categories []string   `json:"categories,omitempty"`
	Schemes    []string   `json:"schemes,omitempty"`
	Hosts      []string   `json:"hosts,omitempty"`
	Paths      []string   `json:"paths,omitempty"`
	MimeTypes  []string   `json:"mime_types,omitempty"`
}

// Component describes an Activity, Service, BroadcastReceiver, or ContentProvider.
type Component struct {
	Kind          string         `json:"kind"` // "activity", "activity-alias", "service", "receiver", "provider"
	Name          string         `json:"name"`
	Exported      *bool          `json:"exported,omitempty"`
	Permission    string         `json:"permission,omitempty"`
	Authority     string         `json:"authority,omitempty"`
	Target        string         `json:"target_activity,omitempty"`
	Process       string         `json:"process,omitempty"`
	IntentFilters []IntentFilter `json:"intent_filters,omitempty"`
}

// IsEffectivelyExported determines exported status following Android 12+ rules.
func (c Component) IsEffectivelyExported() bool {
	if c.Exported != nil {
		return *c.Exported
	}
	return len(c.IntentFilters) > 0
}

// Info holds decoded metadata from binary AndroidManifest.xml.
type Info struct {
	Package               string      `json:"package"`
	VersionCode           int64       `json:"version_code"`
	VersionName           string      `json:"version_name,omitempty"`
	MinSDK                int         `json:"min_sdk"`
	TargetSDK             int         `json:"target_sdk"`
	CompileSDK            int         `json:"compile_sdk,omitempty"`
	ApplicationClass      string      `json:"application_class,omitempty"`
	AppLabel              string      `json:"app_label,omitempty"`
	Debuggable            bool        `json:"debuggable"`
	AllowBackup           *bool       `json:"allow_backup,omitempty"`
	UsesCleartextTraffic  *bool       `json:"uses_cleartext_traffic,omitempty"`
	NetworkSecurityConfig string      `json:"network_security_config,omitempty"`
	LauncherActivity      string      `json:"launcher_activity,omitempty"`
	Permissions           []string    `json:"permissions"`
	Features              []string    `json:"features,omitempty"`
	QueriesPackages       []string    `json:"queries_packages,omitempty"`
	Activities            []Component `json:"activities"`
	Services              []Component `json:"services"`
	Receivers             []Component `json:"receivers"`
	Providers             []Component `json:"providers"`
	DeepLinks             []string    `json:"deep_links,omitempty"`
}

type axmlAttr struct {
	Namespace string
	Name      string
	RawValue  string
	DataType  uint8
	Data      uint32
}

// ParseBinary decodes an AAPT2 binary AndroidManifest.xml buffer.
func ParseBinary(data []byte) (*Info, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("manifest buffer too short (%d bytes)", len(data))
	}
	rootType := binary.LittleEndian.Uint16(data[0:2])
	rootHdrSize := binary.LittleEndian.Uint16(data[2:4])
	if rootType != chunkXML {
		return nil, fmt.Errorf("expected binary AXML chunk 0x0003, got 0x%04x", rootType)
	}
	if int(rootHdrSize) > len(data) {
		return nil, fmt.Errorf("invalid AXML root header size %d", rootHdrSize)
	}

	var stringsPool []string
	info := &Info{
		Permissions: []string{},
		Activities:  []Component{},
		Services:    []Component{},
		Receivers:   []Component{},
		Providers:   []Component{},
	}

	var currentComp *Component
	var currentFilter *IntentFilter
	inQueries := false

	offset := int(rootHdrSize)
	for offset+8 <= len(data) {
		cType := binary.LittleEndian.Uint16(data[offset : offset+2])
		cHdrSize := int(binary.LittleEndian.Uint16(data[offset+2 : offset+4]))
		cSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		if cSize < 8 || offset+cSize > len(data) || cHdrSize < 8 || cHdrSize > cSize {
			break
		}

		switch cType {
		case chunkStringPool:
			pool, err := parseStringPool(data[offset : offset+cSize])
			if err != nil {
				return nil, fmt.Errorf("parse AXML string pool: %w", err)
			}
			stringsPool = pool

		case chunkXMLResMap:
			// Resource ID map; skipped cleanly.

		case chunkXMLStartTag:
			if offset+36 > offset+cSize {
				break
			}
			nameIdx := binary.LittleEndian.Uint32(data[offset+20 : offset+24])
			attrCount := int(binary.LittleEndian.Uint16(data[offset+28 : offset+30]))
			tagName := poolGet(stringsPool, nameIdx)

			attrs := make([]axmlAttr, 0, attrCount)
			attrOff := offset + 36
			for i := 0; i < attrCount && attrOff+20 <= offset+cSize; i++ {
				nsIdx := binary.LittleEndian.Uint32(data[attrOff : attrOff+4])
				anIdx := binary.LittleEndian.Uint32(data[attrOff+4 : attrOff+8])
				rawIdx := binary.LittleEndian.Uint32(data[attrOff+8 : attrOff+12])
				dType := data[attrOff+15]
				dVal := binary.LittleEndian.Uint32(data[attrOff+16 : attrOff+20])
				attrs = append(attrs, axmlAttr{
					Namespace: poolGet(stringsPool, nsIdx),
					Name:      poolGet(stringsPool, anIdx),
					RawValue:  poolGet(stringsPool, rawIdx),
					DataType:  dType,
					Data:      dVal,
				})
				attrOff += 20
			}

			switch tagName {
			case "manifest":
				for _, a := range attrs {
					switch a.Name {
					case "package":
						info.Package = attrString(a, stringsPool)
					case "versionCode":
						info.VersionCode = int64(a.Data)
					case "versionName":
						info.VersionName = attrString(a, stringsPool)
					case "compileSdkVersion":
						info.CompileSDK = int(a.Data)
					}
				}
			case "uses-sdk":
				for _, a := range attrs {
					if a.Name == "minSdkVersion" {
						info.MinSDK = int(a.Data)
					} else if a.Name == "targetSdkVersion" {
						info.TargetSDK = int(a.Data)
					}
				}
			case "uses-permission", "uses-permission-sdk-23":
				for _, a := range attrs {
					if a.Name == "name" {
						if v := attrString(a, stringsPool); v != "" {
							info.Permissions = append(info.Permissions, v)
						}
					}
				}
			case "uses-feature":
				for _, a := range attrs {
					if a.Name == "name" {
						if v := attrString(a, stringsPool); v != "" {
							info.Features = append(info.Features, v)
						}
					}
				}
			case "queries":
				inQueries = true
			case "package":
				if inQueries {
					for _, a := range attrs {
						if a.Name == "name" {
							if v := attrString(a, stringsPool); v != "" {
								info.QueriesPackages = append(info.QueriesPackages, v)
							}
						}
					}
				}
			case "application":
				for _, a := range attrs {
					switch a.Name {
					case "name":
						info.ApplicationClass = qualifyName(info.Package, attrString(a, stringsPool))
					case "label":
						if s := attrString(a, stringsPool); s != "" {
							info.AppLabel = s
						}
					case "debuggable":
						info.Debuggable = a.Data != 0
					case "allowBackup":
						b := a.Data != 0
						info.AllowBackup = &b
					case "usesCleartextTraffic":
						b := a.Data != 0
						info.UsesCleartextTraffic = &b
					case "networkSecurityConfig":
						info.NetworkSecurityConfig = fmt.Sprintf("@0x%08x", a.Data)
					}
				}
			case "activity", "activity-alias", "service", "receiver", "provider":
				comp := Component{Kind: tagName}
				for _, a := range attrs {
					switch a.Name {
					case "name":
						comp.Name = qualifyName(info.Package, attrString(a, stringsPool))
					case "exported":
						b := a.Data != 0
						comp.Exported = &b
					case "permission":
						comp.Permission = attrString(a, stringsPool)
					case "authorities":
						comp.Authority = attrString(a, stringsPool)
					case "targetActivity":
						comp.Target = qualifyName(info.Package, attrString(a, stringsPool))
					case "process":
						comp.Process = attrString(a, stringsPool)
					}
				}
				currentComp = &comp
			case "intent-filter":
				if currentComp != nil {
					currentFilter = &IntentFilter{}
				}
			case "action":
				if currentFilter != nil {
					for _, a := range attrs {
						if a.Name == "name" {
							if v := attrString(a, stringsPool); v != "" {
								currentFilter.Actions = append(currentFilter.Actions, v)
							}
						}
					}
				}
			case "category":
				if currentFilter != nil {
					for _, a := range attrs {
						if a.Name == "name" {
							if v := attrString(a, stringsPool); v != "" {
								currentFilter.Categories = append(currentFilter.Categories, v)
							}
						}
					}
				}
			case "data":
				if currentFilter != nil {
					var scheme, host, path string
					for _, a := range attrs {
						switch a.Name {
						case "scheme":
							scheme = attrString(a, stringsPool)
							if scheme != "" {
								currentFilter.Schemes = append(currentFilter.Schemes, scheme)
							}
						case "host":
							host = attrString(a, stringsPool)
							if host != "" {
								currentFilter.Hosts = append(currentFilter.Hosts, host)
							}
						case "path", "pathPrefix", "pathPattern":
							path = attrString(a, stringsPool)
							if path != "" {
								currentFilter.Paths = append(currentFilter.Paths, path)
							}
						case "mimeType":
							if m := attrString(a, stringsPool); m != "" {
								currentFilter.MimeTypes = append(currentFilter.MimeTypes, m)
							}
						}
					}
					if scheme != "" {
						dl := scheme + "://" + host + path
						info.DeepLinks = append(info.DeepLinks, dl)
					}
				}
			}

		case chunkXMLEndTag:
			if offset+24 > offset+cSize {
				break
			}
			nameIdx := binary.LittleEndian.Uint32(data[offset+20 : offset+24])
			tagName := poolGet(stringsPool, nameIdx)
			switch tagName {
			case "queries":
				inQueries = false
			case "intent-filter":
				if currentComp != nil && currentFilter != nil {
					currentComp.IntentFilters = append(currentComp.IntentFilters, *currentFilter)
					if (currentComp.Kind == "activity" || currentComp.Kind == "activity-alias") &&
						contains(currentFilter.Actions, "android.intent.action.MAIN") &&
						contains(currentFilter.Categories, "android.intent.category.LAUNCHER") &&
						info.LauncherActivity == "" {
						info.LauncherActivity = currentComp.Name
					}
					currentFilter = nil
				}
			case "activity", "activity-alias":
				if currentComp != nil {
					info.Activities = append(info.Activities, *currentComp)
					currentComp = nil
				}
			case "service":
				if currentComp != nil {
					info.Services = append(info.Services, *currentComp)
					currentComp = nil
				}
			case "receiver":
				if currentComp != nil {
					info.Receivers = append(info.Receivers, *currentComp)
					currentComp = nil
				}
			case "provider":
				if currentComp != nil {
					info.Providers = append(info.Providers, *currentComp)
					currentComp = nil
				}
			}
		}
		offset += cSize
	}

	if info.Package == "" {
		return nil, fmt.Errorf("missing package attribute in AndroidManifest.xml")
	}
	return info, nil
}

func parseStringPool(chunk []byte) ([]string, error) {
	if len(chunk) < 28 {
		return nil, fmt.Errorf("string pool header truncated")
	}
	stringCount := int(binary.LittleEndian.Uint32(chunk[8:12]))
	flags := binary.LittleEndian.Uint32(chunk[16:20])
	stringsStart := int(binary.LittleEndian.Uint32(chunk[20:24]))
	isUTF8 := (flags & (1 << 8)) != 0

	if stringCount < 0 || stringCount > 500000 || 28+stringCount*4 > len(chunk) || stringsStart > len(chunk) {
		return nil, fmt.Errorf("invalid string pool bounds (count=%d)", stringCount)
	}

	out := make([]string, stringCount)
	for i := 0; i < stringCount; i++ {
		strOff := int(binary.LittleEndian.Uint32(chunk[28+i*4 : 32+i*4]))
		pos := stringsStart + strOff
		if pos < 0 || pos >= len(chunk) {
			continue
		}
		if isUTF8 {
			// Skip UTF-16 char len, then read UTF-8 byte len
			_, n1 := readLen8(chunk[pos:])
			pos += n1
			if pos >= len(chunk) {
				continue
			}
			bLen, n2 := readLen8(chunk[pos:])
			pos += n2
			if pos+bLen <= len(chunk) {
				out[i] = string(chunk[pos : pos+bLen])
			}
		} else {
			cLen, n := readLen16(chunk[pos:])
			pos += n
			if pos+cLen*2 <= len(chunk) {
				u16 := make([]uint16, cLen)
				for k := 0; k < cLen; k++ {
					u16[k] = binary.LittleEndian.Uint16(chunk[pos+k*2 : pos+k*2+2])
				}
				out[i] = string(utf16.Decode(u16))
			}
		}
	}
	return out, nil
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

func poolGet(pool []string, idx uint32) string {
	if idx == 0xFFFFFFFF || int(idx) >= len(pool) {
		return ""
	}
	return pool[idx]
}

func attrString(a axmlAttr, pool []string) string {
	if a.RawValue != "" {
		return a.RawValue
	}
	if a.DataType == 0x03 {
		return poolGet(pool, a.Data)
	}
	if a.DataType == 0x01 {
		return fmt.Sprintf("@0x%08x", a.Data)
	}
	return ""
}

func qualifyName(pkg, name string) string {
	if name == "" || pkg == "" {
		return name
	}
	if strings.HasPrefix(name, ".") {
		return pkg + name
	}
	if !strings.Contains(name, ".") {
		return pkg + "." + name
	}
	return name
}

func contains(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}
