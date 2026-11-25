package build

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// AXMLNode represents a node in the parsed Android manifest.
type AXMLNode struct {
	Tag        string
	Attributes map[string]string
	Children   []*AXMLNode
}

// ManifestDetails contains structured metadata extracted from AndroidManifest.xml.
type ManifestDetails struct {
	PackageID          string                    `json:"package_id"`
	VersionCode        int                       `json:"version_code"`
	VersionName        string                    `json:"version_name"`
	MinSDK             int                       `json:"min_sdk"`
	TargetSDK          int                       `json:"target_sdk"`
	LauncherActivity   string                    `json:"launcher_activity"`
	Permissions        []string                  `json:"permissions"`
	ExportedComponents []types.ExportedComponent `json:"exported_components"`
	DeepLinkSchemes    []string                  `json:"deep_link_schemes"`
	ResizeableActivity *bool                     `json:"resizeable_activity,omitempty"`
	IsGame             bool                      `json:"is_game"`
}

// ParseManifest parses either binary AXML or plain XML into ManifestDetails.
func ParseManifest(data []byte) (*ManifestDetails, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty manifest data")
	}

	// Check if plain text XML
	if trimmed[0] == '<' {
		return parsePlainXMLManifest(trimmed)
	}

	// Otherwise parse binary AXML
	node, err := decodeAXML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode binary AXML: %w", err)
	}

	return extractManifestDetails(node), nil
}

// decodeAXML decodes an Android binary XML buffer into an AXMLNode tree.
func decodeAXML(data []byte) (*AXMLNode, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("data too short for AXML")
	}

	r := bytes.NewReader(data)
	var chunkType, headerSize uint16
	var chunkSize uint32

	if err := binary.Read(r, binary.LittleEndian, &chunkType); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &headerSize); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &chunkSize); err != nil {
		return nil, err
	}

	if chunkType != 0x0003 { // RES_XML_TYPE
		return nil, fmt.Errorf("invalid AXML magic chunk type: 0x%04x", chunkType)
	}

	var stringPool []string
	var root *AXMLNode
	var nodeStack []*AXMLNode

	offset := int64(headerSize)
	for offset < int64(len(data)) {
		if _, err := r.Seek(offset, io.SeekStart); err != nil {
			break
		}

		var cType, hSize uint16
		var cSize uint32
		if err := binary.Read(r, binary.LittleEndian, &cType); err != nil {
			break
		}
		if err := binary.Read(r, binary.LittleEndian, &hSize); err != nil {
			break
		}
		if err := binary.Read(r, binary.LittleEndian, &cSize); err != nil {
			break
		}

		if cSize == 0 {
			break
		}

		switch cType {
		case 0x0001: // RES_STRING_POOL_TYPE
			poolData := data[offset : offset+int64(cSize)]
			pool, err := parseStringPool(poolData)
			if err == nil {
				stringPool = pool
			}

		case 0x0180: // RES_XML_RESOURCE_MAP_TYPE
			// Optional resource map table, skip chunk payload

		case 0x0102: // RES_XML_START_ELEMENT_TYPE
			// Parse element start
			node, err := parseStartElement(data[offset:offset+int64(cSize)], stringPool)
			if err == nil {
				if root == nil {
					root = node
				}
				if len(nodeStack) > 0 {
					parent := nodeStack[len(nodeStack)-1]
					parent.Children = append(parent.Children, node)
				}
				nodeStack = append(nodeStack, node)
			}

		case 0x0103: // RES_XML_END_ELEMENT_TYPE
			if len(nodeStack) > 0 {
				nodeStack = nodeStack[:len(nodeStack)-1]
			}
		}

		offset += int64(cSize)
	}

	if root == nil {
		return nil, fmt.Errorf("no root element found in AXML")
	}

	return root, nil
}

func parseStringPool(data []byte) ([]string, error) {
	r := bytes.NewReader(data)
	var header struct {
		ChunkType    uint16
		HeaderSize   uint16
		ChunkSize    uint32
		StringCount  uint32
		StyleCount   uint32
		Flags        uint32
		StringsStart uint32
		StylesStart  uint32
	}
	if err := binary.Read(r, binary.LittleEndian, &header); err != nil {
		return nil, err
	}

	isUTF8 := (header.Flags & (1 << 8)) != 0
	offsets := make([]uint32, header.StringCount)
	if err := binary.Read(r, binary.LittleEndian, &offsets); err != nil {
		return nil, err
	}

	strings := make([]string, header.StringCount)
	poolBase := int(header.StringsStart)

	for i := 0; i < int(header.StringCount); i++ {
		strOffset := poolBase + int(offsets[i])
		if strOffset >= len(data) {
			continue
		}

		if isUTF8 {
			// UTF-8 string: skip length bytes (1 or 2 bytes of u16 length, 1 or 2 bytes of u8 length)
			p := strOffset
			if p >= len(data) {
				continue
			}
			if (data[p] & 0x80) != 0 {
				p += 2
			} else {
				p += 1
			}
			if p >= len(data) {
				continue
			}
			var byteLen int
			if (data[p] & 0x80) != 0 {
				if p+1 < len(data) {
					byteLen = (int(data[p]&0x7f) << 8) | int(data[p+1])
					p += 2
				}
			} else {
				byteLen = int(data[p])
				p += 1
			}

			if p+byteLen <= len(data) {
				strings[i] = string(data[p : p+byteLen])
			}
		} else {
			// UTF-16 string
			p := strOffset
			if p+2 > len(data) {
				continue
			}
			u16Len := int(binary.LittleEndian.Uint16(data[p:]))
			p += 2
			if (u16Len & 0x8000) != 0 {
				if p+2 <= len(data) {
					u16Len = ((u16Len & 0x7fff) << 16) | int(binary.LittleEndian.Uint16(data[p:]))
					p += 2
				}
			}

			byteCount := u16Len * 2
			if p+byteCount <= len(data) {
				u16s := make([]uint16, u16Len)
				for j := 0; j < u16Len; j++ {
					u16s[j] = binary.LittleEndian.Uint16(data[p+j*2:])
				}
				strings[i] = string(utf16.Decode(u16s))
			}
		}
	}

	return strings, nil
}

func parseStartElement(chunk []byte, stringPool []string) (*AXMLNode, error) {
	r := bytes.NewReader(chunk)
	var header struct {
		ChunkType  uint16
		HeaderSize uint16
		ChunkSize  uint32
		LineNumber uint32
		Comment    uint32
		NsIndex    uint32
		NameIndex  uint32
		AttrStart  uint16
		AttrSize   uint16
		AttrCount  uint16
		IdIndex    uint16
		ClassIndex uint16
		StyleIndex uint16
	}
	if err := binary.Read(r, binary.LittleEndian, &header); err != nil {
		return nil, err
	}

	getString := func(idx uint32) string {
		if int(idx) >= 0 && int(idx) < len(stringPool) {
			return stringPool[idx]
		}
		return ""
	}

	node := &AXMLNode{
		Tag:        getString(header.NameIndex),
		Attributes: make(map[string]string),
	}

	attrOffset := int(header.HeaderSize) + int(header.AttrStart)
	attrSize := int(header.AttrSize)
	if attrSize == 0 {
		attrSize = 20
	}

	for i := 0; i < int(header.AttrCount); i++ {
		start := attrOffset + (i * attrSize)
		if start+20 > len(chunk) {
			break
		}

		attrReader := bytes.NewReader(chunk[start : start+20])
		var attr struct {
			NsIndex       uint32
			NameIndex     uint32
			RawValueIndex uint32
			Size          uint16
			Res0          uint8
			DataType      uint8
			Data          uint32
		}
		if err := binary.Read(attrReader, binary.LittleEndian, &attr); err != nil {
			continue
		}

		name := getString(attr.NameIndex)
		var val string
		if attr.RawValueIndex != 0xFFFFFFFF && int(attr.RawValueIndex) >= 0 && int(attr.RawValueIndex) < len(stringPool) {
			val = stringPool[attr.RawValueIndex]
		}
		if val == "" {
			// Format typed value
			switch attr.DataType {
			case 0x03: // TYPE_STRING
				val = getString(attr.Data)
			case 0x10: // TYPE_INT_DEC
				val = fmt.Sprintf("%d", int32(attr.Data))
			case 0x11: // TYPE_INT_HEX
				val = fmt.Sprintf("0x%x", attr.Data)
			case 0x12: // TYPE_INT_BOOLEAN
				if attr.Data != 0 {
					val = "true"
				} else {
					val = "false"
				}
			default:
				val = fmt.Sprintf("%d", attr.Data)
			}
		}

		if name != "" {
			node.Attributes[name] = val
		}
	}

	return node, nil
}

// extractManifestDetails walks an AXMLNode tree to extract structured manifest info.
func extractManifestDetails(root *AXMLNode) *ManifestDetails {
	details := &ManifestDetails{
		Permissions:        []string{},
		ExportedComponents: []types.ExportedComponent{},
		DeepLinkSchemes:    []string{},
	}

	if root == nil {
		return details
	}

	// Manifest tag attributes
	if root.Tag == "manifest" {
		details.PackageID = root.Attributes["package"]
		if vc, err := strconv.Atoi(root.Attributes["versionCode"]); err == nil {
			details.VersionCode = vc
		}
		details.VersionName = root.Attributes["versionName"]
	}

	walkNode(root, details, nil)
	return details
}

func walkNode(node *AXMLNode, details *ManifestDetails, currentComponent *types.ExportedComponent) {
	switch node.Tag {
	case "uses-sdk":
		if min, err := strconv.Atoi(node.Attributes["minSdkVersion"]); err == nil {
			details.MinSDK = min
		}
		if target, err := strconv.Atoi(node.Attributes["targetSdkVersion"]); err == nil {
			details.TargetSDK = target
		}

	case "uses-permission":
		if name, ok := node.Attributes["name"]; ok && name != "" {
			details.Permissions = appendUnique(details.Permissions, name)
		}

	case "application":
		if resizable, ok := node.Attributes["resizeableActivity"]; ok {
			b := (resizable == "true" || resizable == "1")
			details.ResizeableActivity = &b
		}
		if isGame, ok := node.Attributes["isGame"]; ok && (isGame == "true" || isGame == "1") {
			details.IsGame = true
		}
		if cat, ok := node.Attributes["appCategory"]; ok && strings.ToLower(cat) == "game" {
			details.IsGame = true
		}

	case "activity", "service", "receiver", "provider":
		compType := node.Tag
		name := node.Attributes["name"]
		exportedStr := node.Attributes["exported"]
		isExported := (exportedStr == "true" || exportedStr == "1")

		// Has intent filter?
		hasIntentFilter := false
		isLauncher := false
		for _, child := range node.Children {
			if child.Tag == "intent-filter" {
				hasIntentFilter = true
				if compType == "activity" && isLauncherIntentFilter(child) {
					isLauncher = true
				}
				extractDeepLinks(child, details)
			}
		}

		if isLauncher && details.LauncherActivity == "" {
			details.LauncherActivity = name
		}

		// In Android, components with intent filters default to exported=true unless explicitly false (pre-API 31)
		if isExported || (exportedStr == "" && hasIntentFilter) {
			details.ExportedComponents = append(details.ExportedComponents, types.ExportedComponent{
				Type: compType,
				Name: name,
			})
		}
	}

	for _, child := range node.Children {
		walkNode(child, details, currentComponent)
	}
}

func isLauncherIntentFilter(filter *AXMLNode) bool {
	hasMain := false
	hasLauncher := false
	for _, child := range filter.Children {
		if child.Tag == "action" && child.Attributes["name"] == "android.intent.action.MAIN" {
			hasMain = true
		}
		if child.Tag == "category" && child.Attributes["name"] == "android.intent.category.LAUNCHER" {
			hasLauncher = true
		}
	}
	return hasMain && hasLauncher
}

func extractDeepLinks(filter *AXMLNode, details *ManifestDetails) {
	for _, child := range filter.Children {
		if child.Tag == "data" {
			if scheme, ok := child.Attributes["scheme"]; ok && scheme != "" {
				details.DeepLinkSchemes = appendUnique(details.DeepLinkSchemes, scheme)
			}
		}
	}
}

func appendUnique(slice []string, val string) []string {
	for _, item := range slice {
		if item == val {
			return slice
		}
	}
	return append(slice, val)
}

// Plain XML parsing for textual AndroidManifest.xml (from apkanalyzer or source files)
type xmlManifest struct {
	Package     string          `xml:"package,attr"`
	VersionCode int             `xml:"versionCode,attr"`
	VersionName string          `xml:"versionName,attr"`
	UsesSDK     xmlUsesSDK      `xml:"uses-sdk"`
	Permissions []xmlPermission `xml:"uses-permission"`
	Application xmlApplication  `xml:"application"`
}

type xmlUsesSDK struct {
	MinSDK    int `xml:"minSdkVersion,attr"`
	TargetSDK int `xml:"targetSdkVersion,attr"`
}

type xmlPermission struct {
	Name string `xml:"name,attr"`
}

type xmlApplication struct {
	ResizeableActivity string         `xml:"resizeableActivity,attr"`
	IsGame             string         `xml:"isGame,attr"`
	AppCategory        string         `xml:"appCategory,attr"`
	Activities         []xmlComponent `xml:"activity"`
	Services           []xmlComponent `xml:"service"`
	Receivers          []xmlComponent `xml:"receiver"`
	Providers          []xmlComponent `xml:"provider"`
}

type xmlComponent struct {
	Name          string            `xml:"name,attr"`
	Exported      string            `xml:"exported,attr"`
	IntentFilters []xmlIntentFilter `xml:"intent-filter"`
}

type xmlIntentFilter struct {
	Actions    []xmlNamed `xml:"action"`
	Categories []xmlNamed `xml:"category"`
	Data       []xmlData  `xml:"data"`
}

type xmlNamed struct {
	Name string `xml:"name,attr"`
}

type xmlData struct {
	Scheme string `xml:"scheme,attr"`
}

func parsePlainXMLManifest(data []byte) (*ManifestDetails, error) {
	var xm xmlManifest
	if err := xml.Unmarshal(data, &xm); err != nil {
		return nil, err
	}

	details := &ManifestDetails{
		PackageID:          xm.Package,
		VersionCode:        xm.VersionCode,
		VersionName:        xm.VersionName,
		MinSDK:             xm.UsesSDK.MinSDK,
		TargetSDK:          xm.UsesSDK.TargetSDK,
		Permissions:        make([]string, 0, len(xm.Permissions)),
		ExportedComponents: make([]types.ExportedComponent, 0),
		DeepLinkSchemes:    make([]string, 0),
	}

	for _, p := range xm.Permissions {
		if p.Name != "" {
			details.Permissions = appendUnique(details.Permissions, p.Name)
		}
	}

	if xm.Application.ResizeableActivity != "" {
		b := (xm.Application.ResizeableActivity == "true" || xm.Application.ResizeableActivity == "1")
		details.ResizeableActivity = &b
	}
	if xm.Application.IsGame == "true" || xm.Application.IsGame == "1" || strings.ToLower(xm.Application.AppCategory) == "game" {
		details.IsGame = true
	}

	processComponents := func(comps []xmlComponent, compType string) {
		for _, comp := range comps {
			isLauncher := false
			hasIntentFilter := len(comp.IntentFilters) > 0
			for _, filter := range comp.IntentFilters {
				hasMain := false
				hasLaunch := false
				for _, a := range filter.Actions {
					if a.Name == "android.intent.action.MAIN" {
						hasMain = true
					}
				}
				for _, c := range filter.Categories {
					if c.Name == "android.intent.category.LAUNCHER" {
						hasLaunch = true
					}
				}
				if hasMain && hasLaunch {
					isLauncher = true
				}
				for _, d := range filter.Data {
					if d.Scheme != "" {
						details.DeepLinkSchemes = appendUnique(details.DeepLinkSchemes, d.Scheme)
					}
				}
			}

			if isLauncher && details.LauncherActivity == "" && compType == "activity" {
				details.LauncherActivity = comp.Name
			}

			isExported := comp.Exported == "true" || comp.Exported == "1"
			if isExported || (comp.Exported == "" && hasIntentFilter) {
				details.ExportedComponents = append(details.ExportedComponents, types.ExportedComponent{
					Type: compType,
					Name: comp.Name,
				})
			}
		}
	}

	processComponents(xm.Application.Activities, "activity")
	processComponents(xm.Application.Services, "service")
	processComponents(xm.Application.Receivers, "receiver")
	processComponents(xm.Application.Providers, "provider")

	return details, nil
}
