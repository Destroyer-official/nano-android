package dex

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
)

// ClassInfo describes one DEX class_def_item.
type ClassInfo struct {
	Descriptor  string   `json:"descriptor"` // e.g. Lcom/example/MainActivity;
	JavaName    string   `json:"java_name"`  // e.g. com.example.MainActivity
	SuperClass  string   `json:"super_class,omitempty"`
	Interfaces  []string `json:"interfaces,omitempty"`
	AccessFlags uint32   `json:"access_flags"`
	IsInterface bool     `json:"is_interface,omitempty"`
	IsEnum      bool     `json:"is_enum,omitempty"`
	IsAbstract  bool     `json:"is_abstract,omitempty"`
	SourceFile  string   `json:"source_file,omitempty"`
	DexFile     string   `json:"dex_file"`
}

// MethodRef describes one method_id_item referenced or defined in DEX.
type MethodRef struct {
	Owner      string `json:"owner"`
	Name       string `json:"name"`
	ReturnType string `json:"return_type,omitempty"`
	Params     string `json:"params,omitempty"`
}

// CompatibilityIssue flags an Android/Compose/JNI API dependency found in DEX.
type CompatibilityIssue struct {
	API             string `json:"api"`
	Category        string `json:"category"` // "COMPOSE", "JNI", "ART_REQUIRED", "HARDWARE_REQUIRED", "DESUGAR"
	Severity        string `json:"severity"` // "INFO", "WARNING", "REQUIRES_ART", "REQUIRES_HARDWARE"
	RecommendedTier string `json:"recommended_tier"`
}

// Inventory summarizes parsed classes*.dex across an APK.
type Inventory struct {
	DexFiles             []string               `json:"dex_files"`
	DexVersions          []string               `json:"dex_versions"`
	TotalClasses         int                    `json:"total_classes"`
	TotalMethods         int                    `json:"total_methods"`
	TotalFields          int                    `json:"total_fields"`
	Classes              []ClassInfo            `json:"classes"`
	MethodReferences     []MethodRef            `json:"method_references"`
	ClassDependencyGraph map[string][]string    `json:"class_dependency_graph"`
	DetectedLibraries    []string               `json:"detected_libraries"`
	DuplicateClasses     []string               `json:"duplicate_classes,omitempty"`
	CompatibilityIssues  []CompatibilityIssue   `json:"compatibility_issues,omitempty"`
}

// ParseSingle parses one binary classes*.dex byte slice.
func ParseSingle(dexName string, data []byte) (*Inventory, error) {
	if len(data) < 0x70 {
		return nil, fmt.Errorf("%s too short (%d bytes)", dexName, len(data))
	}
	if string(data[0:4]) != "dex\n" || data[7] != 0x00 {
		return nil, fmt.Errorf("%s invalid DEX magic", dexName)
	}
	dexVer := string(data[4:7])
	endianTag := binary.LittleEndian.Uint32(data[0x28:0x2C])
	if endianTag != 0x12345678 {
		return nil, fmt.Errorf("%s unsupported endian tag 0x%08x", dexName, endianTag)
	}

	strCount := int(binary.LittleEndian.Uint32(data[0x38:0x3C]))
	strOff := int(binary.LittleEndian.Uint32(data[0x3C:0x40]))
	typeCount := int(binary.LittleEndian.Uint32(data[0x40:0x44]))
	typeOff := int(binary.LittleEndian.Uint32(data[0x44:0x48]))
	protoCount := int(binary.LittleEndian.Uint32(data[0x48:0x4C]))
	protoOff := int(binary.LittleEndian.Uint32(data[0x4C:0x50]))
	fieldCount := int(binary.LittleEndian.Uint32(data[0x50:0x54]))
	methodCount := int(binary.LittleEndian.Uint32(data[0x58:0x5C]))
	methodOff := int(binary.LittleEndian.Uint32(data[0x5C:0x60]))
	classCount := int(binary.LittleEndian.Uint32(data[0x60:0x64]))
	classOff := int(binary.LittleEndian.Uint32(data[0x64:0x68]))

	stringsTable := make([]string, strCount)
	for i := 0; i < strCount && strOff+i*4+4 <= len(data); i++ {
		sDataOff := int(binary.LittleEndian.Uint32(data[strOff+i*4 : strOff+i*4+4]))
		if sDataOff > 0 && sDataOff < len(data) {
			stringsTable[i] = readDexMUTF8(data[sDataOff:])
		}
	}

	typesTable := make([]string, typeCount)
	for i := 0; i < typeCount && typeOff+i*4+4 <= len(data); i++ {
		descIdx := int(binary.LittleEndian.Uint32(data[typeOff+i*4 : typeOff+i*4+4]))
		if descIdx >= 0 && descIdx < len(stringsTable) {
			typesTable[i] = stringsTable[descIdx]
		}
	}

	protosTable := make([]string, protoCount)
	for i := 0; i < protoCount && protoOff+i*12+12 <= len(data); i++ {
		retIdx := int(binary.LittleEndian.Uint32(data[protoOff+i*12+4 : protoOff+i*12+8]))
		if retIdx >= 0 && retIdx < len(typesTable) {
			protosTable[i] = descriptorToJava(typesTable[retIdx])
		}
	}

	inv := &Inventory{
		DexFiles:             []string{dexName},
		DexVersions:          []string{dexVer},
		TotalClasses:         classCount,
		TotalMethods:         methodCount,
		TotalFields:          fieldCount,
		ClassDependencyGraph: make(map[string][]string),
	}

	// Method IDs (8 bytes each: class_idx u16, proto_idx u16, name_idx u32)
	maxMethodsCap := methodCount
	if maxMethodsCap > 200000 {
		maxMethodsCap = 200000
	}
	inv.MethodReferences = make([]MethodRef, 0, maxMethodsCap)
	for i := 0; i < maxMethodsCap && methodOff+i*8+8 <= len(data); i++ {
		off := methodOff + i*8
		clsIdx := int(binary.LittleEndian.Uint16(data[off : off+2]))
		protoIdx := int(binary.LittleEndian.Uint16(data[off+2 : off+4]))
		nameIdx := int(binary.LittleEndian.Uint32(data[off+4 : off+8]))
		owner := ""
		if clsIdx >= 0 && clsIdx < len(typesTable) {
			owner = descriptorToJava(typesTable[clsIdx])
		}
		mName := ""
		if nameIdx >= 0 && nameIdx < len(stringsTable) {
			mName = stringsTable[nameIdx]
		}
		ret := ""
		if protoIdx >= 0 && protoIdx < len(protosTable) {
			ret = protosTable[protoIdx]
		}
		inv.MethodReferences = append(inv.MethodReferences, MethodRef{
			Owner:      owner,
			Name:       mName,
			ReturnType: ret,
		})
	}

	// Class defs (32 bytes each)
	maxClassesCap := classCount
	if maxClassesCap > 100000 {
		maxClassesCap = 100000
	}
	inv.Classes = make([]ClassInfo, 0, maxClassesCap)
	for i := 0; i < maxClassesCap && classOff+i*32+32 <= len(data); i++ {
		off := classOff + i*32
		clsIdx := int(binary.LittleEndian.Uint32(data[off : off+4]))
		flags := binary.LittleEndian.Uint32(data[off+4 : off+8])
		superIdx := binary.LittleEndian.Uint32(data[off+8 : off+12])
		ifacesOff := int(binary.LittleEndian.Uint32(data[off+12 : off+16]))
		srcFileIdx := binary.LittleEndian.Uint32(data[off+16 : off+20])

		desc := ""
		if clsIdx >= 0 && clsIdx < len(typesTable) {
			desc = typesTable[clsIdx]
		}
		javaName := descriptorToJava(desc)
		superName := ""
		if superIdx != 0xFFFFFFFF && int(superIdx) < len(typesTable) {
			superName = descriptorToJava(typesTable[superIdx])
		}
		srcFile := ""
		if srcFileIdx != 0xFFFFFFFF && int(srcFileIdx) < len(stringsTable) {
			srcFile = stringsTable[srcFileIdx]
		}
		var ifaces []string
		if ifacesOff > 0 && ifacesOff+4 <= len(data) {
			nIf := int(binary.LittleEndian.Uint32(data[ifacesOff : ifacesOff+4]))
			if nIf > 0 && nIf < 256 && ifacesOff+4+nIf*2 <= len(data) {
				for k := 0; k < nIf; k++ {
					tIdx := int(binary.LittleEndian.Uint16(data[ifacesOff+4+k*2 : ifacesOff+6+k*2]))
					if tIdx >= 0 && tIdx < len(typesTable) {
						ifaces = append(ifaces, descriptorToJava(typesTable[tIdx]))
					}
				}
			}
		}

		ci := ClassInfo{
			Descriptor:  desc,
			JavaName:    javaName,
			SuperClass:  superName,
			Interfaces:  ifaces,
			AccessFlags: flags,
			IsInterface: (flags & 0x0200) != 0,
			IsAbstract:  (flags & 0x0400) != 0,
			IsEnum:      (flags & 0x4000) != 0,
			SourceFile:  srcFile,
			DexFile:     dexName,
		}
		inv.Classes = append(inv.Classes, ci)

		var deps []string
		if superName != "" && superName != "java.lang.Object" {
			deps = append(deps, superName)
		}
		deps = append(deps, ifaces...)
		if len(deps) > 0 {
			inv.ClassDependencyGraph[javaName] = deps
		}
	}

	analyzeLibrariesAndCompatibility(inv)
	return inv, nil
}

// MergeInventories combines multiple classes*.dex inventories and detects duplicate classes.
func MergeInventories(parts []*Inventory) *Inventory {
	merged := &Inventory{
		ClassDependencyGraph: make(map[string][]string),
	}
	seenClass := make(map[string]string)
	dupSet := make(map[string]bool)
	libSet := make(map[string]bool)
	verSet := make(map[string]bool)
	compatSet := make(map[string]CompatibilityIssue)

	for _, p := range parts {
		if p == nil {
			continue
		}
		merged.DexFiles = append(merged.DexFiles, p.DexFiles...)
		for _, v := range p.DexVersions {
			if !verSet[v] {
				verSet[v] = true
				merged.DexVersions = append(merged.DexVersions, v)
			}
		}
		merged.TotalClasses += p.TotalClasses
		merged.TotalMethods += p.TotalMethods
		merged.TotalFields += p.TotalFields
		for _, c := range p.Classes {
			if prevDex, exists := seenClass[c.JavaName]; exists && prevDex != c.DexFile {
				dupSet[c.JavaName] = true
			} else {
				seenClass[c.JavaName] = c.DexFile
			}
			merged.Classes = append(merged.Classes, c)
		}
		merged.MethodReferences = append(merged.MethodReferences, p.MethodReferences...)
		for k, v := range p.ClassDependencyGraph {
			merged.ClassDependencyGraph[k] = v
		}
		for _, l := range p.DetectedLibraries {
			libSet[l] = true
		}
		for _, ci := range p.CompatibilityIssues {
			compatSet[ci.API] = ci
		}
	}
	for d := range dupSet {
		merged.DuplicateClasses = append(merged.DuplicateClasses, d)
	}
	for l := range libSet {
		merged.DetectedLibraries = append(merged.DetectedLibraries, l)
	}
	for _, ci := range compatSet {
		merged.CompatibilityIssues = append(merged.CompatibilityIssues, ci)
	}
	sort.Strings(merged.DuplicateClasses)
	sort.Strings(merged.DetectedLibraries)
	return merged
}

func analyzeLibrariesAndCompatibility(inv *Inventory) {
	libPrefixes := map[string]string{
		"kotlin.":               "Kotlin Stdlib",
		"kotlinx.coroutines.":   "Kotlin Coroutines",
		"androidx.compose.":     "Jetpack Compose",
		"androidx.appcompat.":   "AndroidX AppCompat",
		"androidx.recyclerview.":"AndroidX RecyclerView",
		"androidx.room.":        "AndroidX Room",
		"androidx.work.":        "AndroidX WorkManager",
		"okhttp3.":              "OkHttp3",
		"retrofit2.":            "Retrofit2",
		"dagger.hilt.":          "Hilt / Dagger",
		"org.koin.":             "Koin DI",
		"com.google.firebase.":  "Firebase SDK",
		"com.google.gson.":      "Gson",
		"io.reactivex.":         "RxJava",
		"j$.util.":              "D8 Core Library Desugaring",
	}
	libs := map[string]bool{}
	issues := map[string]CompatibilityIssue{}

	checkName := func(name string) {
		for pref, label := range libPrefixes {
			if strings.HasPrefix(name, pref) {
				libs[label] = true
			}
		}
		switch {
		case strings.HasPrefix(name, "androidx.compose."):
			issues["androidx.compose.*"] = CompatibilityIssue{
				API: "androidx.compose.*", Category: "COMPOSE", Severity: "REQUIRES_ART", RecommendedTier: "robolectric_or_android_art",
			}
		case strings.HasPrefix(name, "android.media.MediaCodec"):
			issues["android.media.MediaCodec"] = CompatibilityIssue{
				API: "android.media.MediaCodec", Category: "HARDWARE_REQUIRED", Severity: "REQUIRES_HARDWARE", RecommendedTier: "physical_device",
			}
		case strings.HasPrefix(name, "android.hardware.camera2."):
			issues["android.hardware.camera2"] = CompatibilityIssue{
				API: "android.hardware.camera2.*", Category: "HARDWARE_REQUIRED", Severity: "REQUIRES_HARDWARE", RecommendedTier: "physical_device",
			}
		case strings.HasPrefix(name, "android.webkit.WebView"):
			issues["android.webkit.WebView"] = CompatibilityIssue{
				API: "android.webkit.WebView", Category: "ART_REQUIRED", Severity: "REQUIRES_ART", RecommendedTier: "android_art",
			}
		case strings.HasPrefix(name, "j$.util.stream."):
			issues["j$.util.stream.*"] = CompatibilityIssue{
				API: "j$.util.stream.*", Category: "DESUGAR", Severity: "WARNING", RecommendedTier: "nano_jvm_with_desugar_shim",
			}
		}
	}

	for _, c := range inv.Classes {
		checkName(c.JavaName)
		if c.SuperClass != "" {
			checkName(c.SuperClass)
		}
		for _, iface := range c.Interfaces {
			checkName(iface)
		}
	}
	for _, m := range inv.MethodReferences {
		checkName(m.Owner)
	}
	for l := range libs {
		inv.DetectedLibraries = append(inv.DetectedLibraries, l)
	}
	for _, ci := range issues {
		inv.CompatibilityIssues = append(inv.CompatibilityIssues, ci)
	}
	sort.Strings(inv.DetectedLibraries)
}

func readDexMUTF8(b []byte) string {
	// Skip ULEB128 utf16_size
	idx := 0
	for idx < len(b) && idx < 5 {
		cont := b[idx] & 0x80
		idx++
		if cont == 0 {
			break
		}
	}
	end := idx
	for end < len(b) && b[end] != 0x00 {
		end++
	}
	if idx > end || end > len(b) {
		return ""
	}
	return string(b[idx:end])
}

func descriptorToJava(desc string) string {
	if desc == "" {
		return ""
	}
	if strings.HasPrefix(desc, "L") && strings.HasSuffix(desc, ";") {
		return strings.ReplaceAll(desc[1:len(desc)-1], "/", ".")
	}
	return desc
}
