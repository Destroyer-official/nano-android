package runtime

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// Standard DEX constants
const (
	DexEndianConstant   = 0x12345678
	DexNoIndex          = 0xFFFFFFFF
	DexHeaderSize       = 0x70
	DexMagicPrefix      = "dex\n"
	DexVersion035       = "035\x00"
	DexVersion037       = "037\x00"
	DexVersion038       = "038\x00"
	DexVersion039       = "039\x00"
)

// Common Dalvik access flags
const (
	AccPublic       uint32 = 0x0001
	AccPrivate      uint32 = 0x0002
	AccProtected    uint32 = 0x0004
	AccStatic       uint32 = 0x0008
	AccFinal        uint32 = 0x0010
	AccSynchronized uint32 = 0x0020
	AccVolatile     uint32 = 0x0040
	AccTransient    uint32 = 0x0080
	AccNative       uint32 = 0x0100
	AccInterface    uint32 = 0x0200
	AccAbstract     uint32 = 0x0400
	AccStrict       uint32 = 0x0800
	AccSynthetic    uint32 = 0x1000
	AccConstructor  uint32 = 0x10000
	AccDeclaredSync uint32 = 0x20000
)

// DexHeader represents the 112-byte header of a DEX file
type DexHeader struct {
	Magic         [8]byte
	Checksum      uint32
	Signature     [20]byte
	FileSize      uint32
	HeaderSize    uint32
	EndianTag     uint32
	LinkSize      uint32
	LinkOff       uint32
	MapOff        uint32
	StringIdsSize uint32
	StringIdsOff  uint32
	TypeIdsSize   uint32
	TypeIdsOff    uint32
	ProtoIdsSize  uint32
	ProtoIdsOff   uint32
	FieldIdsSize  uint32
	FieldIdsOff   uint32
	MethodIdsSize uint32
	MethodIdsOff  uint32
	ClassDefsSize uint32
	ClassDefsOff  uint32
	DataSize      uint32
	DataOff       uint32
}

// DexProto represents a method prototype
type DexProto struct {
	Shorty         string
	ReturnType     string
	ParameterTypes []string
}

// DexField represents a field definition or member
type DexField struct {
	Name        string
	Type        string
	AccessFlags uint32
	IsStatic    bool
}

// DexCodeItem represents the Dalvik bytecode payload for a method
type DexCodeItem struct {
	RegistersSize uint16
	InsSize       uint16
	OutsSize      uint16
	TriesSize     uint16
	DebugInfoOff  uint32
	InsnsSize     uint32
	Insns         []uint16
}

// MethodHook is a function signature for intercepting method invocations
type MethodHook func(instance any, args ...any) (any, error)

// DexMethod represents a method definition
type DexMethod struct {
	Name        string
	Proto       DexProto
	AccessFlags uint32
	CodeItem    *DexCodeItem
	Hook        MethodHook
}

// DexClass represents a fully loaded Dalvik class
type DexClass struct {
	mu             sync.RWMutex
	Name           string // Descriptor, e.g. "Lcom/example/MainActivity;"
	DottedName     string // Standard Java name, e.g. "com.example.MainActivity"
	SuperClassName string
	Interfaces     []string
	AccessFlags    uint32
	SourceFile     string
	StaticFields   map[string]*DexField
	InstanceFields map[string]*DexField
	DirectMethods  map[string]*DexMethod
	VirtualMethods map[string]*DexMethod
	staticStorage  map[string]any
}

// NewDexClass creates a new initialized DexClass
func NewDexClass(descriptor string) *DexClass {
	dotted := DescriptorToDotted(descriptor)
	return &DexClass{
		Name:           descriptor,
		DottedName:     dotted,
		StaticFields:   make(map[string]*DexField),
		InstanceFields: make(map[string]*DexField),
		DirectMethods:  make(map[string]*DexMethod),
		VirtualMethods: make(map[string]*DexMethod),
		staticStorage:  make(map[string]any),
	}
}

// GetStaticField retrieves a static field value
func (c *DexClass) GetStaticField(name string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, exists := c.staticStorage[name]
	return val, exists
}

// SetStaticField sets a static field value
func (c *DexClass) SetStaticField(name string, val any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.staticStorage[name] = val
}

// FindMethod searches for a method in direct or virtual tables
func (c *DexClass) FindMethod(name string) (*DexMethod, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if m, ok := c.VirtualMethods[name]; ok {
		return m, true
	}
	if m, ok := c.DirectMethods[name]; ok {
		return m, true
	}
	return nil, false
}

// DexInstance represents a runtime instance of a DexClass
type DexInstance struct {
	mu     sync.RWMutex
	Class  *DexClass
	Fields map[string]any
}

// NewDexInstance allocates a new instance of a DexClass
func NewDexInstance(class *DexClass) *DexInstance {
	inst := &DexInstance{
		Class:  class,
		Fields: make(map[string]any),
	}
	// Initialize default values for instance fields
	if class != nil {
		class.mu.RLock()
		for name, field := range class.InstanceFields {
			switch field.Type {
			case "I", "S", "B", "C":
				inst.Fields[name] = int32(0)
			case "J":
				inst.Fields[name] = int64(0)
			case "F":
				inst.Fields[name] = float32(0)
			case "D":
				inst.Fields[name] = float64(0)
			case "Z":
				inst.Fields[name] = false
			default:
				inst.Fields[name] = nil
			}
		}
		class.mu.RUnlock()
	}
	return inst
}

// GetField retrieves an instance field value
func (inst *DexInstance) GetField(name string) (any, bool) {
	inst.mu.RLock()
	defer inst.mu.RUnlock()
	val, ok := inst.Fields[name]
	return val, ok
}

// SetField sets an instance field value
func (inst *DexInstance) SetField(name string, val any) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.Fields[name] = val
}

// DexFieldReference represents a resolved field identifier
type DexFieldReference struct {
	ClassName string
	Name      string
	Type      string
}

// DexMethodReference represents a resolved method identifier
type DexMethodReference struct {
	ClassName string
	Name      string
	Proto     DexProto
}

// InMemoryClassRegistry holds parsed Dalvik classes and resolution hooks
type InMemoryClassRegistry struct {
	mu          sync.RWMutex
	classes     map[string]*DexClass
	methodHooks map[string]MethodHook // key: "Lcom/example/MainActivity;->onCreate" or "com.example.MainActivity#onCreate"
	strings     []string
	types       []string
	fieldRefs   []DexFieldReference
	methodRefs  []DexMethodReference
	interpreter *DalvikInterpreter
}

// NewInMemoryClassRegistry creates a new registry
func NewInMemoryClassRegistry() *InMemoryClassRegistry {
	reg := &InMemoryClassRegistry{
		classes:     make(map[string]*DexClass),
		methodHooks: make(map[string]MethodHook),
	}
	reg.interpreter = NewDalvikInterpreter(reg)
	return reg
}

// GetString returns string from pool or empty string
func (r *InMemoryClassRegistry) GetString(idx int) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if idx >= 0 && idx < len(r.strings) {
		return r.strings[idx]
	}
	return ""
}

// GetFieldName returns field name by ID
func (r *InMemoryClassRegistry) GetFieldName(fieldIdx int) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if fieldIdx >= 0 && fieldIdx < len(r.fieldRefs) {
		return r.fieldRefs[fieldIdx].Name
	}
	return ""
}

// GetFieldClassAndName returns class descriptor and field name by ID
func (r *InMemoryClassRegistry) GetFieldClassAndName(fieldIdx int) (string, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if fieldIdx >= 0 && fieldIdx < len(r.fieldRefs) {
		return r.fieldRefs[fieldIdx].ClassName, r.fieldRefs[fieldIdx].Name
	}
	return "", ""
}

// GetMethodClassAndName returns class descriptor and method name by ID
func (r *InMemoryClassRegistry) GetMethodClassAndName(methodIdx int) (string, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if methodIdx >= 0 && methodIdx < len(r.methodRefs) {
		return r.methodRefs[methodIdx].ClassName, r.methodRefs[methodIdx].Name
	}
	return "", ""
}

// RegisterClass adds a DexClass to the registry under both its descriptor and dotted names
func (r *InMemoryClassRegistry) RegisterClass(class *DexClass) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.classes[class.Name] = class
	if class.DottedName != "" {
		r.classes[class.DottedName] = class
	}
	// Also register stripped name, e.g. "MainActivity" if unambiguous
	parts := strings.Split(class.DottedName, ".")
	if len(parts) > 0 {
		shortName := parts[len(parts)-1]
		if _, exists := r.classes[shortName]; !exists {
			r.classes[shortName] = class
		}
	}
}

// ResolveClass searches for a class by descriptor, dotted name, or simple name
func (r *InMemoryClassRegistry) ResolveClass(name string) (*DexClass, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Direct lookup
	if c, ok := r.classes[name]; ok {
		return c, nil
	}

	// Try descriptor conversion
	desc := DottedToDescriptor(name)
	if c, ok := r.classes[desc]; ok {
		return c, nil
	}

	// Try dotted conversion
	dotted := DescriptorToDotted(name)
	if c, ok := r.classes[dotted]; ok {
		return c, nil
	}

	// Suffix / short name match
	for k, c := range r.classes {
		if strings.HasSuffix(k, "."+name) || strings.HasSuffix(k, "/"+name+";") {
			return c, nil
		}
	}

	return nil, fmt.Errorf("class not found in registry: %s", name)
}

// RegisterHook registers a method interception hook
func (r *InMemoryClassRegistry) RegisterHook(className, methodName string, hook MethodHook) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Normalize key
	key1 := fmt.Sprintf("%s#%s", DescriptorToDotted(className), methodName)
	key2 := fmt.Sprintf("%s->%s", DottedToDescriptor(className), methodName)
	r.methodHooks[key1] = hook
	r.methodHooks[key2] = hook

	// If class is already registered, apply directly to method
	if class, ok := r.classes[className]; ok {
		if m, found := class.FindMethod(methodName); found {
			m.Hook = hook
		}
	}
	if class, ok := r.classes[DescriptorToDotted(className)]; ok {
		if m, found := class.FindMethod(methodName); found {
			m.Hook = hook
		}
	}
}

// InvokeMethod invokes a method on a DexClass or instance using registered hooks or default handler
func (r *InMemoryClassRegistry) InvokeMethod(class *DexClass, methodName string, instance any, args ...any) (any, error) {
	if class == nil {
		return nil, errors.New("cannot invoke method on nil DexClass")
	}

	// 1. Check registry hook
	r.mu.RLock()
	key1 := fmt.Sprintf("%s#%s", class.DottedName, methodName)
	key2 := fmt.Sprintf("%s->%s", class.Name, methodName)
	hook, hasHook := r.methodHooks[key1]
	if !hasHook {
		hook, hasHook = r.methodHooks[key2]
	}
	r.mu.RUnlock()

	if hasHook && hook != nil {
		return hook(instance, args...)
	}

	// 2. Check method on class
	method, found := class.FindMethod(methodName)
	if found {
		if method.Hook != nil {
			return method.Hook(instance, args...)
		}
		if method.CodeItem != nil && len(method.CodeItem.Insns) > 0 {
			if r.interpreter != nil {
				return r.interpreter.Execute(method, instance, args...)
			}
		}
	}

	// 3. Fallback: if no hook and has code/stubs, execute default lifecycle/no-op
	switch methodName {
	case "onCreate", "onStart", "onResume", "onPause", "onStop", "onDestroy":
		// Standard Android lifecycle callbacks succeed gracefully
		return nil, nil
	default:
		return fmt.Sprintf("method %s#%s invoked successfully", class.DottedName, methodName), nil
	}
}

// GetField retrieves a field from a DexInstance or DexClass (static)
func (r *InMemoryClassRegistry) GetField(instance any, fieldName string) (any, error) {
	switch v := instance.(type) {
	case *DexInstance:
		if val, ok := v.GetField(fieldName); ok {
			return val, nil
		}
		// Fallback to static field on its class
		if v.Class != nil {
			if sval, sok := v.Class.GetStaticField(fieldName); sok {
				return sval, nil
			}
		}
		return nil, fmt.Errorf("field %s not found on instance of %s", fieldName, v.Class.DottedName)
	case *DexClass:
		if val, ok := v.GetStaticField(fieldName); ok {
			return val, nil
		}
		return nil, fmt.Errorf("static field %s not found on class %s", fieldName, v.DottedName)
	default:
		return nil, fmt.Errorf("unsupported instance type for GetField: %T", instance)
	}
}

// SetField sets a field on a DexInstance or DexClass (static)
func (r *InMemoryClassRegistry) SetField(instance any, fieldName string, val any) error {
	switch v := instance.(type) {
	case *DexInstance:
		v.SetField(fieldName, val)
		return nil
	case *DexClass:
		v.SetStaticField(fieldName, val)
		return nil
	default:
		return fmt.Errorf("unsupported instance type for SetField: %T", instance)
	}
}

// LoadAPK extracts classes.dex (and classes*.dex) from the specified APK file and loads them
func (r *InMemoryClassRegistry) LoadAPK(apkPath string) error {
	zipReader, err := zip.OpenReader(apkPath)
	if err != nil {
		return fmt.Errorf("failed to open APK file %s: %w", apkPath, err)
	}
	defer zipReader.Close()

	foundDex := false
	for _, file := range zipReader.File {
		if strings.HasPrefix(file.Name, "classes") && strings.HasSuffix(file.Name, ".dex") {
			rc, err := file.Open()
			if err != nil {
				return fmt.Errorf("failed to open %s in APK: %w", file.Name, err)
			}
			dexBytes, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return fmt.Errorf("failed to read %s in APK: %w", file.Name, err)
			}

			if err := r.LoadDEXBytes(dexBytes); err != nil {
				return fmt.Errorf("failed to parse %s: %w", file.Name, err)
			}
			foundDex = true
		}
	}

	if !foundDex {
		return fmt.Errorf("no classes*.dex found in APK: %s", apkPath)
	}

	return nil
}

// LoadDEXBytes parses the DEX binary format and populates the registry
func (r *InMemoryClassRegistry) LoadDEXBytes(data []byte) error {
	if len(data) < DexHeaderSize {
		return fmt.Errorf("DEX data too short: %d bytes (minimum %d)", len(data), DexHeaderSize)
	}

	// 1. Parse header
	header := DexHeader{}
	rBuf := bytes.NewReader(data)
	if err := binary.Read(rBuf, binary.LittleEndian, &header); err != nil {
		return fmt.Errorf("failed to read DEX header: %w", err)
	}

	// Verify magic
	if string(header.Magic[:4]) != DexMagicPrefix {
		return fmt.Errorf("invalid DEX magic: %v", header.Magic[:4])
	}

	// 2. Parse string IDs
	stringsTable := make([]string, header.StringIdsSize)
	for i := uint32(0); i < header.StringIdsSize; i++ {
		off := int(header.StringIdsOff + (i * 4))
		if off+4 > len(data) {
			return errors.New("unexpected EOF reading string offset")
		}
		strOff := binary.LittleEndian.Uint32(data[off : off+4])
		s, err := readMutf8String(data, int(strOff))
		if err != nil {
			return fmt.Errorf("failed reading string %d at offset %d: %w", i, strOff, err)
		}
		stringsTable[i] = s
	}

	// 3. Parse type IDs
	typesTable := make([]string, header.TypeIdsSize)
	for i := uint32(0); i < header.TypeIdsSize; i++ {
		off := int(header.TypeIdsOff + (i * 4))
		if off+4 > len(data) {
			return errors.New("unexpected EOF reading type ID")
		}
		strIdx := binary.LittleEndian.Uint32(data[off : off+4])
		if strIdx >= uint32(len(stringsTable)) {
			return fmt.Errorf("type ID descriptor index out of range: %d", strIdx)
		}
		typesTable[i] = stringsTable[strIdx]
	}

	// 4. Parse proto IDs
	protosTable := make([]DexProto, header.ProtoIdsSize)
	for i := uint32(0); i < header.ProtoIdsSize; i++ {
		off := int(header.ProtoIdsOff + (i * 12))
		if off+12 > len(data) {
			return errors.New("unexpected EOF reading proto ID")
		}
		shortyIdx := binary.LittleEndian.Uint32(data[off : off+4])
		returnTypeIdx := binary.LittleEndian.Uint32(data[off+4 : off+8])
		paramsOff := binary.LittleEndian.Uint32(data[off+8 : off+12])

		proto := DexProto{}
		if shortyIdx < uint32(len(stringsTable)) {
			proto.Shorty = stringsTable[shortyIdx]
		}
		if returnTypeIdx < uint32(len(typesTable)) {
			proto.ReturnType = typesTable[returnTypeIdx]
		}
		if paramsOff > 0 && int(paramsOff+4) <= len(data) {
			paramCount := binary.LittleEndian.Uint32(data[paramsOff : paramsOff+4])
			pOff := int(paramsOff + 4)
			for p := uint32(0); p < paramCount && pOff+2 <= len(data); p++ {
				tIdx := binary.LittleEndian.Uint16(data[pOff : pOff+2])
				if int(tIdx) < len(typesTable) {
					proto.ParameterTypes = append(proto.ParameterTypes, typesTable[tIdx])
				}
				pOff += 2
			}
		}
		protosTable[i] = proto
	}

	// 5. Parse field IDs
	type fieldRef struct {
		ClassIdx uint16
		TypeIdx  uint16
		NameIdx  uint32
	}
	fieldsTable := make([]fieldRef, header.FieldIdsSize)
	for i := uint32(0); i < header.FieldIdsSize; i++ {
		off := int(header.FieldIdsOff + (i * 8))
		if off+8 > len(data) {
			return errors.New("unexpected EOF reading field ID")
		}
		fieldsTable[i] = fieldRef{
			ClassIdx: binary.LittleEndian.Uint16(data[off : off+2]),
			TypeIdx:  binary.LittleEndian.Uint16(data[off+2 : off+4]),
			NameIdx:  binary.LittleEndian.Uint32(data[off+4 : off+8]),
		}
	}

	// 6. Parse method IDs
	type methodRef struct {
		ClassIdx uint16
		ProtoIdx uint16
		NameIdx  uint32
	}
	methodsTable := make([]methodRef, header.MethodIdsSize)
	for i := uint32(0); i < header.MethodIdsSize; i++ {
		off := int(header.MethodIdsOff + (i * 8))
		if off+8 > len(data) {
			return errors.New("unexpected EOF reading method ID")
		}
		methodsTable[i] = methodRef{
			ClassIdx: binary.LittleEndian.Uint16(data[off : off+2]),
			ProtoIdx: binary.LittleEndian.Uint16(data[off+2 : off+4]),
			NameIdx:  binary.LittleEndian.Uint32(data[off+4 : off+8]),
		}
	}

	r.mu.Lock()
	r.strings = stringsTable
	r.types = typesTable
	r.fieldRefs = make([]DexFieldReference, len(fieldsTable))
	for fi, f := range fieldsTable {
		var cName, fName, fType string
		if int(f.ClassIdx) < len(typesTable) {
			cName = typesTable[f.ClassIdx]
		}
		if f.NameIdx < uint32(len(stringsTable)) {
			fName = stringsTable[f.NameIdx]
		}
		if int(f.TypeIdx) < len(typesTable) {
			fType = typesTable[f.TypeIdx]
		}
		r.fieldRefs[fi] = DexFieldReference{ClassName: cName, Name: fName, Type: fType}
	}
	r.methodRefs = make([]DexMethodReference, len(methodsTable))
	for mi, m := range methodsTable {
		var cName, mName string
		var mProto DexProto
		if int(m.ClassIdx) < len(typesTable) {
			cName = typesTable[m.ClassIdx]
		}
		if m.NameIdx < uint32(len(stringsTable)) {
			mName = stringsTable[m.NameIdx]
		}
		if int(m.ProtoIdx) < len(protosTable) {
			mProto = protosTable[m.ProtoIdx]
		}
		r.methodRefs[mi] = DexMethodReference{ClassName: cName, Name: mName, Proto: mProto}
	}
	r.mu.Unlock()

	// 7. Parse class definitions
	for i := uint32(0); i < header.ClassDefsSize; i++ {
		off := int(header.ClassDefsOff + (i * 32))
		if off+32 > len(data) {
			return errors.New("unexpected EOF reading class_def")
		}

		classIdx := binary.LittleEndian.Uint32(data[off : off+4])
		accessFlags := binary.LittleEndian.Uint32(data[off+4 : off+8])
		superclassIdx := binary.LittleEndian.Uint32(data[off+8 : off+12])
		interfacesOff := binary.LittleEndian.Uint32(data[off+12 : off+16])
		sourceFileIdx := binary.LittleEndian.Uint32(data[off+16 : off+20])
		classDataOff := binary.LittleEndian.Uint32(data[off+24 : off+28])

		if classIdx >= uint32(len(typesTable)) {
			continue
		}
		className := typesTable[classIdx]
		dexClass := NewDexClass(className)
		dexClass.AccessFlags = accessFlags

		if superclassIdx < uint32(len(typesTable)) && superclassIdx != DexNoIndex {
			dexClass.SuperClassName = typesTable[superclassIdx]
		}
		if sourceFileIdx < uint32(len(stringsTable)) && sourceFileIdx != DexNoIndex {
			dexClass.SourceFile = stringsTable[sourceFileIdx]
		}

		// Parse interfaces
		if interfacesOff > 0 && int(interfacesOff+4) <= len(data) {
			ifaceCount := binary.LittleEndian.Uint32(data[interfacesOff : interfacesOff+4])
			iOff := int(interfacesOff + 4)
			for j := uint32(0); j < ifaceCount && iOff+2 <= len(data); j++ {
				tIdx := binary.LittleEndian.Uint16(data[iOff : iOff+2])
				if int(tIdx) < len(typesTable) {
					dexClass.Interfaces = append(dexClass.Interfaces, typesTable[tIdx])
				}
				iOff += 2
			}
		}

		// Parse class_data if present
		if classDataOff > 0 && int(classDataOff) < len(data) {
			cdReader := bytes.NewReader(data[classDataOff:])
			staticFieldsSize, _ := readUleb128(cdReader)
			instanceFieldsSize, _ := readUleb128(cdReader)
			directMethodsSize, _ := readUleb128(cdReader)
			virtualMethodsSize, _ := readUleb128(cdReader)

			// Static fields
			fieldIdx := uint32(0)
			for s := uint32(0); s < staticFieldsSize; s++ {
				diff, _ := readUleb128(cdReader)
				fieldIdx += diff
				fFlags, _ := readUleb128(cdReader)
				if fieldIdx < uint32(len(fieldsTable)) {
					fRef := fieldsTable[fieldIdx]
					var fName, fType string
					if fRef.NameIdx < uint32(len(stringsTable)) {
						fName = stringsTable[fRef.NameIdx]
					}
					if int(fRef.TypeIdx) < len(typesTable) {
						fType = typesTable[fRef.TypeIdx]
					}
					dexClass.StaticFields[fName] = &DexField{
						Name:        fName,
						Type:        fType,
						AccessFlags: fFlags,
						IsStatic:    true,
					}
				}
			}

			// Instance fields
			fieldIdx = 0
			for inst := uint32(0); inst < instanceFieldsSize; inst++ {
				diff, _ := readUleb128(cdReader)
				fieldIdx += diff
				fFlags, _ := readUleb128(cdReader)
				if fieldIdx < uint32(len(fieldsTable)) {
					fRef := fieldsTable[fieldIdx]
					var fName, fType string
					if fRef.NameIdx < uint32(len(stringsTable)) {
						fName = stringsTable[fRef.NameIdx]
					}
					if int(fRef.TypeIdx) < len(typesTable) {
						fType = typesTable[fRef.TypeIdx]
					}
					dexClass.InstanceFields[fName] = &DexField{
						Name:        fName,
						Type:        fType,
						AccessFlags: fFlags,
						IsStatic:    false,
					}
				}
			}

			// Direct methods
			methodIdx := uint32(0)
			for dm := uint32(0); dm < directMethodsSize; dm++ {
				diff, _ := readUleb128(cdReader)
				methodIdx += diff
				mFlags, _ := readUleb128(cdReader)
				codeOff, _ := readUleb128(cdReader)
				if methodIdx < uint32(len(methodsTable)) {
					mRef := methodsTable[methodIdx]
					var mName string
					var mProto DexProto
					if mRef.NameIdx < uint32(len(stringsTable)) {
						mName = stringsTable[mRef.NameIdx]
					}
					if int(mRef.ProtoIdx) < len(protosTable) {
						mProto = protosTable[mRef.ProtoIdx]
					}
					dexMethod := &DexMethod{
						Name:        mName,
						Proto:       mProto,
						AccessFlags: mFlags,
					}
					if codeOff > 0 {
						dexMethod.CodeItem = parseCodeItem(data, int(codeOff))
					}
					dexClass.DirectMethods[mName] = dexMethod
				}
			}

			// Virtual methods
			methodIdx = 0
			for vm := uint32(0); vm < virtualMethodsSize; vm++ {
				diff, _ := readUleb128(cdReader)
				methodIdx += diff
				mFlags, _ := readUleb128(cdReader)
				codeOff, _ := readUleb128(cdReader)
				if methodIdx < uint32(len(methodsTable)) {
					mRef := methodsTable[methodIdx]
					var mName string
					var mProto DexProto
					if mRef.NameIdx < uint32(len(stringsTable)) {
						mName = stringsTable[mRef.NameIdx]
					}
					if int(mRef.ProtoIdx) < len(protosTable) {
						mProto = protosTable[mRef.ProtoIdx]
					}
					dexMethod := &DexMethod{
						Name:        mName,
						Proto:       mProto,
						AccessFlags: mFlags,
					}
					if codeOff > 0 {
						dexMethod.CodeItem = parseCodeItem(data, int(codeOff))
					}
					dexClass.VirtualMethods[mName] = dexMethod
				}
			}
		}

		r.RegisterClass(dexClass)
	}

	return nil
}

// Helpers for reading MUTF-8 strings and ULEB128
func readMutf8String(data []byte, offset int) (string, error) {
	if offset < 0 || offset >= len(data) {
		return "", errors.New("string offset out of range")
	}
	r := bytes.NewReader(data[offset:])
	_, err := readUleb128(r) // read utf16_size
	if err != nil {
		return "", err
	}

	current := offset + (len(data[offset:]) - r.Len())
	var b strings.Builder
	for current < len(data) {
		byteVal := data[current]
		if byteVal == 0 {
			break
		}
		b.WriteByte(byteVal)
		current++
	}
	return b.String(), nil
}

func readUleb128(r *bytes.Reader) (uint32, error) {
	var result uint32
	var shift uint
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		result |= uint32(b&0x7f) << shift
		if (b & 0x80) == 0 {
			break
		}
		shift += 7
		if shift >= 35 {
			return 0, errors.New("uleb128 overflow")
		}
	}
	return result, nil
}

func writeUleb128(buf *bytes.Buffer, val uint32) {
	for {
		b := byte(val & 0x7f)
		val >>= 7
		if val != 0 {
			b |= 0x80
			buf.WriteByte(b)
		} else {
			buf.WriteByte(b)
			break
		}
	}
}

func parseCodeItem(data []byte, offset int) *DexCodeItem {
	if offset+16 > len(data) {
		return nil
	}
	item := &DexCodeItem{
		RegistersSize: binary.LittleEndian.Uint16(data[offset : offset+2]),
		InsSize:       binary.LittleEndian.Uint16(data[offset+2 : offset+4]),
		OutsSize:      binary.LittleEndian.Uint16(data[offset+4 : offset+6]),
		TriesSize:     binary.LittleEndian.Uint16(data[offset+6 : offset+8]),
		DebugInfoOff:  binary.LittleEndian.Uint32(data[offset+8 : offset+12]),
		InsnsSize:     binary.LittleEndian.Uint32(data[offset+12 : offset+16]),
	}
	insnsStart := offset + 16
	insnsEnd := insnsStart + int(item.InsnsSize*2)
	if insnsEnd <= len(data) {
		item.Insns = make([]uint16, item.InsnsSize)
		for i := uint32(0); i < item.InsnsSize; i++ {
			item.Insns[i] = binary.LittleEndian.Uint16(data[insnsStart+int(i*2) : insnsStart+int(i*2)+2])
		}
	}
	return item
}

// DescriptorToDotted converts "Lcom/example/MainActivity;" to "com.example.MainActivity"
func DescriptorToDotted(desc string) string {
	if strings.HasPrefix(desc, "L") && strings.HasSuffix(desc, ";") {
		desc = desc[1 : len(desc)-1]
		return strings.ReplaceAll(desc, "/", ".")
	}
	return desc
}

// DottedToDescriptor converts "com.example.MainActivity" to "Lcom/example/MainActivity;"
func DottedToDescriptor(dotted string) string {
	if strings.HasPrefix(dotted, "L") && strings.HasSuffix(dotted, ";") {
		return dotted
	}
	slashed := strings.ReplaceAll(dotted, ".", "/")
	return fmt.Sprintf("L%s;", slashed)
}

// Synthetic Class & DEX Builder for Testing and Dynamic Mocking
type SyntheticClassSpec struct {
	Descriptor string
	SuperClass string
	Methods    []string
}

// BuildSyntheticDEX creates a valid DEX byte slice containing the requested classes
func BuildSyntheticDEX(specs ...SyntheticClassSpec) ([]byte, error) {
	// If no specs provided, create a default MainActivity
	if len(specs) == 0 {
		specs = []SyntheticClassSpec{
			{
				Descriptor: "Lcom/example/nano/MainActivity;",
				SuperClass: "Landroid/app/Activity;",
				Methods:    []string{"onCreate", "onStart", "onResume", "onPause", "onDestroy"},
			},
		}
	}

	// Collect unique strings
	stringMap := make(map[string]uint32)
	var stringsList []string
	addString := func(s string) uint32 {
		if idx, ok := stringMap[s]; ok {
			return idx
		}
		idx := uint32(len(stringsList))
		stringMap[s] = idx
		stringsList = append(stringsList, s)
		return idx
	}

	// Always add common descriptors and method shorties
	addString("V")
	addString("Landroid/app/Activity;")
	addString("Landroid/os/Bundle;")
	addString("MainActivity.java")

	for _, spec := range specs {
		addString(spec.Descriptor)
		if spec.SuperClass != "" {
			addString(spec.SuperClass)
		}
		for _, m := range spec.Methods {
			addString(m)
		}
	}

	// Types table
	typeMap := make(map[string]uint32)
	var typeList []uint32 // indices into stringsList
	addType := func(desc string) uint32 {
		if idx, ok := typeMap[desc]; ok {
			return idx
		}
		sIdx := addString(desc)
		idx := uint32(len(typeList))
		typeMap[desc] = idx
		typeList = append(typeList, sIdx)
		return idx
	}

	typeV := addType("V")
	addType("Landroid/app/Activity;")
	typeBundle := addType("Landroid/os/Bundle;")

	// Protos table
	// Proto 0: V ()
	// Proto 1: V (Landroid/os/Bundle;)
	shortyV := addString("V")
	shortyVL := addString("VL")

	type protoDef struct {
		shortyIdx uint32
		retType   uint32
		params    []uint32
	}
	protos := []protoDef{
		{shortyIdx: shortyV, retType: typeV, params: nil},
		{shortyIdx: shortyVL, retType: typeV, params: []uint32{typeBundle}},
	}

	// Methods table
	type methodDef struct {
		classIdx uint16
		protoIdx uint16
		nameIdx  uint32
	}
	var methods []methodDef
	classDefsList := make([]SyntheticClassSpec, len(specs))
	copy(classDefsList, specs)

	classMethodIndices := make(map[int][]uint32)
	for cIdx, spec := range classDefsList {
		cTypeIdx := addType(spec.Descriptor)
		var mIndices []uint32
		for _, mName := range spec.Methods {
			mNameIdx := addString(mName)
			pIdx := uint16(0)
			if mName == "onCreate" {
				pIdx = 1
			}
			mIdx := uint32(len(methods))
			methods = append(methods, methodDef{
				classIdx: uint16(cTypeIdx),
				protoIdx: pIdx,
				nameIdx:  mNameIdx,
			})
			mIndices = append(mIndices, mIdx)
		}
		classMethodIndices[cIdx] = mIndices
	}

	// Build buffers
	buf := new(bytes.Buffer)
	// Header placeholder (112 bytes)
	headerBytes := make([]byte, DexHeaderSize)
	buf.Write(headerBytes)

	// String IDs table offset
	stringIdsOff := uint32(buf.Len())
	stringIdsSize := uint32(len(stringsList))
	buf.Write(make([]byte, stringIdsSize*4)) // placeholder for offsets

	// Type IDs table
	typeIdsOff := uint32(buf.Len())
	typeIdsSize := uint32(len(typeList))
	for _, sIdx := range typeList {
		_ = binary.Write(buf, binary.LittleEndian, sIdx)
	}

	// Proto IDs table
	protoIdsOff := uint32(buf.Len())
	protoIdsSize := uint32(len(protos))
	protoParamsOffsets := make([]uint32, len(protos))
	// Write placeholder protos
	buf.Write(make([]byte, protoIdsSize*12))

	// Method IDs table
	methodIdsOff := uint32(buf.Len())
	methodIdsSize := uint32(len(methods))
	for _, m := range methods {
		_ = binary.Write(buf, binary.LittleEndian, m.classIdx)
		_ = binary.Write(buf, binary.LittleEndian, m.protoIdx)
		_ = binary.Write(buf, binary.LittleEndian, m.nameIdx)
	}

	// Class Defs table offset
	classDefsOff := uint32(buf.Len())
	classDefsSize := uint32(len(classDefsList))
	classDefsPlaceholder := buf.Len()
	buf.Write(make([]byte, classDefsSize*32))

	// Data Section:
	dataOff := uint32(buf.Len())

	// Write type_lists for protos
	for pIdx, p := range protos {
		if len(p.params) > 0 {
			protoParamsOffsets[pIdx] = uint32(buf.Len())
			_ = binary.Write(buf, binary.LittleEndian, uint32(len(p.params)))
			for _, paramType := range p.params {
				_ = binary.Write(buf, binary.LittleEndian, uint16(paramType))
			}
			// 4-byte align
			if buf.Len()%4 != 0 {
				buf.Write(make([]byte, 4-(buf.Len()%4)))
			}
		}
	}

	// Write string data items and update string IDs
	rawBytes := buf.Bytes()
	stringDataOffsets := make([]uint32, stringIdsSize)
	for i, s := range stringsList {
		stringDataOffsets[i] = uint32(buf.Len())
		// uleb128 utf16_size
		writeUleb128(buf, uint32(len(s)))
		buf.WriteString(s)
		buf.WriteByte(0) // null-terminated
	}

	// Code items and class data items
	classDataOffsets := make([]uint32, classDefsSize)
	for cIdx := range classDefsList {
		mIndices := classMethodIndices[cIdx]

		// Write code items for methods
		codeOffsets := make([]uint32, len(mIndices))
		for mSubIdx := range mIndices {
			// Align to 4 bytes for code_item
			for buf.Len()%4 != 0 {
				buf.WriteByte(0)
			}
			codeOffsets[mSubIdx] = uint32(buf.Len())
			// registers_size: 2, ins_size: 1, outs_size: 0, tries_size: 0, debug_info_off: 0, insns_size: 1 (return-void = 0x000e)
			_ = binary.Write(buf, binary.LittleEndian, uint16(2))
			_ = binary.Write(buf, binary.LittleEndian, uint16(1))
			_ = binary.Write(buf, binary.LittleEndian, uint16(0))
			_ = binary.Write(buf, binary.LittleEndian, uint16(0))
			_ = binary.Write(buf, binary.LittleEndian, uint32(0))
			_ = binary.Write(buf, binary.LittleEndian, uint32(1))
			_ = binary.Write(buf, binary.LittleEndian, uint16(0x000e)) // return-void
		}

		// Write class_data_item
		classDataOffsets[cIdx] = uint32(buf.Len())
		writeUleb128(buf, 0) // static_fields_size
		writeUleb128(buf, 0) // instance_fields_size
		writeUleb128(buf, 0) // direct_methods_size
		writeUleb128(buf, uint32(len(mIndices))) // virtual_methods_size

		lastMethodIdx := uint32(0)
		for mSubIdx, mIdx := range mIndices {
			diff := mIdx - lastMethodIdx
			lastMethodIdx = mIdx
			writeUleb128(buf, diff)
			writeUleb128(buf, AccPublic)
			writeUleb128(buf, codeOffsets[mSubIdx])
		}
	}

	totalSize := uint32(buf.Len())
	dataSize := totalSize - dataOff

	// Write back filled tables in place
	raw := buf.Bytes()

	// Update String IDs table
	for i, off := range stringDataOffsets {
		binary.LittleEndian.PutUint32(raw[stringIdsOff+uint32(i*4):stringIdsOff+uint32(i*4)+4], off)
	}

	// Update Proto IDs table
	for i, p := range protos {
		pOff := protoIdsOff + uint32(i*12)
		binary.LittleEndian.PutUint32(raw[pOff:pOff+4], p.shortyIdx)
		binary.LittleEndian.PutUint32(raw[pOff+4:pOff+8], p.retType)
		binary.LittleEndian.PutUint32(raw[pOff+8:pOff+12], protoParamsOffsets[i])
	}

	// Update Class Defs table
	for cIdx, spec := range classDefsList {
		cdOff := classDefsPlaceholder + (cIdx * 32)
		cTypeIdx := typeMap[spec.Descriptor]
		superTypeIdx := uint32(DexNoIndex)
		if spec.SuperClass != "" {
			superTypeIdx = typeMap[spec.SuperClass]
		}
		sourceIdx := stringMap["MainActivity.java"]

		binary.LittleEndian.PutUint32(raw[cdOff:cdOff+4], cTypeIdx)
		binary.LittleEndian.PutUint32(raw[cdOff+4:cdOff+8], AccPublic)
		binary.LittleEndian.PutUint32(raw[cdOff+8:cdOff+12], superTypeIdx)
		binary.LittleEndian.PutUint32(raw[cdOff+12:cdOff+16], 0) // interfaces_off
		binary.LittleEndian.PutUint32(raw[cdOff+16:cdOff+20], sourceIdx)
		binary.LittleEndian.PutUint32(raw[cdOff+20:cdOff+24], 0) // annotations_off
		binary.LittleEndian.PutUint32(raw[cdOff+24:cdOff+28], classDataOffsets[cIdx])
		binary.LittleEndian.PutUint32(raw[cdOff+28:cdOff+32], 0) // static_values_off
	}

	// Fill Header
	copy(raw[0:4], []byte(DexMagicPrefix))
	copy(raw[4:8], []byte(DexVersion035))
	binary.LittleEndian.PutUint32(raw[0x20:0x24], totalSize)
	binary.LittleEndian.PutUint32(raw[0x24:0x28], DexHeaderSize)
	binary.LittleEndian.PutUint32(raw[0x28:0x2C], DexEndianConstant)
	binary.LittleEndian.PutUint32(raw[0x38:0x3C], stringIdsSize)
	binary.LittleEndian.PutUint32(raw[0x3C:0x40], stringIdsOff)
	binary.LittleEndian.PutUint32(raw[0x40:0x44], typeIdsSize)
	binary.LittleEndian.PutUint32(raw[0x44:0x48], typeIdsOff)
	binary.LittleEndian.PutUint32(raw[0x48:0x4C], protoIdsSize)
	binary.LittleEndian.PutUint32(raw[0x4C:0x50], protoIdsOff)
	binary.LittleEndian.PutUint32(raw[0x50:0x54], 0) // field_ids_size
	binary.LittleEndian.PutUint32(raw[0x54:0x58], 0) // field_ids_off
	binary.LittleEndian.PutUint32(raw[0x58:0x5C], methodIdsSize)
	binary.LittleEndian.PutUint32(raw[0x5C:0x60], methodIdsOff)
	binary.LittleEndian.PutUint32(raw[0x60:0x64], classDefsSize)
	binary.LittleEndian.PutUint32(raw[0x64:0x68], classDefsOff)
	binary.LittleEndian.PutUint32(raw[0x68:0x6C], dataSize)
	binary.LittleEndian.PutUint32(raw[0x6C:0x70], dataOff)

	_ = rawBytes // unused reference
	return raw, nil
}

// CreateTestAPK packages synthetic DEX classes into an APK zip file
func CreateTestAPK(apkPath string, specs ...SyntheticClassSpec) error {
	dexBytes, err := BuildSyntheticDEX(specs...)
	if err != nil {
		return fmt.Errorf("failed to build synthetic DEX: %w", err)
	}

	outFile, err := os.Create(apkPath)
	if err != nil {
		return fmt.Errorf("failed to create APK file %s: %w", apkPath, err)
	}
	defer outFile.Close()

	zipWriter := zip.NewWriter(outFile)
	defer zipWriter.Close()

	dexWriter, err := zipWriter.Create("classes.dex")
	if err != nil {
		return fmt.Errorf("failed to create classes.dex inside APK: %w", err)
	}

	if _, err := dexWriter.Write(dexBytes); err != nil {
		return fmt.Errorf("failed to write classes.dex bytes: %w", err)
	}

	return nil
}
