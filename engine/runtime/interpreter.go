package runtime

import (
	"errors"
	"fmt"
	"sync"
)

// DalvikOpcode represents a Dalvik instruction opcode byte
type DalvikOpcode byte

const (
	OpNop             DalvikOpcode = 0x00
	OpMove            DalvikOpcode = 0x01
	OpMoveFrom16      DalvikOpcode = 0x02
	OpMove16          DalvikOpcode = 0x03
	OpMoveWide        DalvikOpcode = 0x04
	OpMoveObject      DalvikOpcode = 0x07
	OpMoveResult      DalvikOpcode = 0x0a
	OpMoveResultWide  DalvikOpcode = 0x0b
	OpMoveResultObj   DalvikOpcode = 0x0c
	OpReturnVoid      DalvikOpcode = 0x0e
	OpReturn          DalvikOpcode = 0x0f
	OpReturnWide      DalvikOpcode = 0x10
	OpReturnObj       DalvikOpcode = 0x11
	OpConst4          DalvikOpcode = 0x12
	OpConst16         DalvikOpcode = 0x13
	OpConst           DalvikOpcode = 0x14
	OpConstHigh16     DalvikOpcode = 0x15
	OpConstString     DalvikOpcode = 0x1a
	OpConstStringJumbo DalvikOpcode = 0x1b
	OpConstClass      DalvikOpcode = 0x1c
	OpGoto            DalvikOpcode = 0x28
	OpGoto16          DalvikOpcode = 0x29
	OpGoto32          DalvikOpcode = 0x2a
	OpIfEq            DalvikOpcode = 0x32
	OpIfNe            DalvikOpcode = 0x33
	OpIfLt            DalvikOpcode = 0x34
	OpIfGe            DalvikOpcode = 0x35
	OpIfGt            DalvikOpcode = 0x36
	OpIfLe            DalvikOpcode = 0x37
	OpIfEqz           DalvikOpcode = 0x38
	OpIfNez           DalvikOpcode = 0x39
	OpIfLtz           DalvikOpcode = 0x3a
	OpIfGez           DalvikOpcode = 0x3b
	OpIfGtz           DalvikOpcode = 0x3c
	OpIfLez           DalvikOpcode = 0x3d
	OpIget            DalvikOpcode = 0x52
	OpIgetWide        DalvikOpcode = 0x53
	OpIgetObject      DalvikOpcode = 0x54
	OpIgetBoolean     DalvikOpcode = 0x55
	OpIput            DalvikOpcode = 0x59
	OpIputWide        DalvikOpcode = 0x5a
	OpIputObject      DalvikOpcode = 0x5b
	OpIputBoolean     DalvikOpcode = 0x5c
	OpSget            DalvikOpcode = 0x60
	OpSgetObject      DalvikOpcode = 0x62
	OpSgetBoolean     DalvikOpcode = 0x63
	OpSput            DalvikOpcode = 0x67
	OpSputObject      DalvikOpcode = 0x69
	OpSputBoolean     DalvikOpcode = 0x6a
	OpInvokeVirtual   DalvikOpcode = 0x6e
	OpInvokeSuper     DalvikOpcode = 0x6f
	OpInvokeDirect    DalvikOpcode = 0x70
	OpInvokeStatic    DalvikOpcode = 0x71
	OpInvokeInterface DalvikOpcode = 0x72
	OpInvokeVirtualRange DalvikOpcode = 0x74
	OpInvokeSuperRange   DalvikOpcode = 0x75
	OpInvokeDirectRange  DalvikOpcode = 0x76
	OpInvokeStaticRange  DalvikOpcode = 0x77
	OpAddInt          DalvikOpcode = 0x90
	OpSubInt          DalvikOpcode = 0x91
	OpMulInt          DalvikOpcode = 0x92
	OpDivInt          DalvikOpcode = 0x93
	OpRemInt          DalvikOpcode = 0x94
	OpAndInt          DalvikOpcode = 0x95
	OpOrInt           DalvikOpcode = 0x96
	OpXorInt          DalvikOpcode = 0x97
	OpShlInt          DalvikOpcode = 0x98
	OpShrInt          DalvikOpcode = 0x99
	OpUshrInt         DalvikOpcode = 0x9a
	OpAddIntLit8      DalvikOpcode = 0xd8
	OpRsubIntLit8     DalvikOpcode = 0xd9
	OpMulIntLit8      DalvikOpcode = 0xda
	OpDivIntLit8      DalvikOpcode = 0xdb
	OpRemIntLit8      DalvikOpcode = 0xdc
	OpAndIntLit8      DalvikOpcode = 0xdd
	OpOrIntLit8       DalvikOpcode = 0xde
	OpXorIntLit8      DalvikOpcode = 0xdf
	OpShlIntLit8      DalvikOpcode = 0xe0
	OpShrIntLit8      DalvikOpcode = 0xe1
	OpUshrIntLit8     DalvikOpcode = 0xe2
)

// ExecutionFrame holds local registers and invocation state
type ExecutionFrame struct {
	Registers  []any
	LastResult any
	PC         int
}

// DalvikInterpreter executes Dalvik bytecode on host without emulator overhead
type DalvikInterpreter struct {
	mu            sync.RWMutex
	registry      *InMemoryClassRegistry
	maxCallDepth  int
	currentDepth  int
	stepListener  func(pc int, op DalvikOpcode, regs []any)
}

// NewDalvikInterpreter creates an interpreter bound to a class registry
func NewDalvikInterpreter(registry *InMemoryClassRegistry) *DalvikInterpreter {
	return &DalvikInterpreter{
		registry:     registry,
		maxCallDepth: 256,
	}
}

// SetStepListener installs an optional debugger/profiler callback for each instruction
func (interp *DalvikInterpreter) SetStepListener(listener func(pc int, op DalvikOpcode, regs []any)) {
	interp.mu.Lock()
	defer interp.mu.Unlock()
	interp.stepListener = listener
}

// Execute runs the Dalvik bytecode in method's CodeItem
func (interp *DalvikInterpreter) Execute(method *DexMethod, instance any, args ...any) (any, error) {
	if method == nil {
		return nil, errors.New("cannot execute nil method")
	}

	// 1. If method has a native or registered hook, invoke hook directly
	if method.Hook != nil {
		return method.Hook(instance, args...)
	}

	code := method.CodeItem
	if code == nil || len(code.Insns) == 0 {
		// Method without code body (abstract/interface or stub), return nil gracefully
		return nil, nil
	}

	interp.mu.Lock()
	if interp.currentDepth >= interp.maxCallDepth {
		interp.mu.Unlock()
		return nil, fmt.Errorf("dalvik stack overflow: maximum call depth %d exceeded", interp.maxCallDepth)
	}
	interp.currentDepth++
	interp.mu.Unlock()

	defer func() {
		interp.mu.Lock()
		interp.currentDepth--
		interp.mu.Unlock()
	}()

	numRegs := int(code.RegistersSize)
	if numRegs < 256 {
		numRegs = 256
	}
	frame := &ExecutionFrame{
		Registers: make([]any, numRegs),
		PC:        0,
	}

	getReg := func(idx int) any {
		if idx >= 0 && idx < len(frame.Registers) {
			return frame.Registers[idx]
		}
		return nil
	}

	setReg := func(idx int, val any) {
		if idx >= 0 {
			if idx >= len(frame.Registers) {
				newRegs := make([]any, idx+64)
				copy(newRegs, frame.Registers)
				frame.Registers = newRegs
			}
			frame.Registers[idx] = val
		}
	}

	// Pass incoming arguments to the top registers (Dalvik calling convention)
	// For instance methods, first argument is 'this' pointer
	allArgs := make([]any, 0, len(args)+1)
	if instance != nil {
		allArgs = append(allArgs, instance)
	}
	allArgs = append(allArgs, args...)

	argCount := len(allArgs)
	if argCount > 0 && numRegs >= argCount {
		startReg := numRegs - argCount
		for i := 0; i < argCount; i++ {
			frame.Registers[startReg+i] = allArgs[i]
		}
	}

	insns := code.Insns
	insnsLen := len(insns)

	for frame.PC < insnsLen {
		inst := insns[frame.PC]
		op := DalvikOpcode(inst & 0xff)

		if interp.stepListener != nil {
			interp.stepListener(frame.PC, op, frame.Registers)
		}

		switch op {
		case OpNop:
			frame.PC++

		case OpMove, OpMoveObject:
			// 12x: move vA, vB (vA = (inst>>8)&0xf, vB = (inst>>12)&0xf)
			vA := int((inst >> 8) & 0x0f)
			vB := int((inst >> 12) & 0x0f)
			if vA < numRegs && vB < numRegs {
				frame.Registers[vA] = frame.Registers[vB]
			}
			frame.PC++

		case OpMoveFrom16:
			// 22x: move/from16 vAA, vBBBB
			vA := int((inst >> 8) & 0xff)
			if frame.PC+1 < insnsLen {
				vB := int(insns[frame.PC+1])
				if vA < numRegs && vB < numRegs {
					frame.Registers[vA] = frame.Registers[vB]
				}
			}
			frame.PC += 2

		case OpMove16:
			// 32x: move/16 vAAAA, vBBBB
			if frame.PC+2 < insnsLen {
				vA := int(insns[frame.PC+1])
				vB := int(insns[frame.PC+2])
				if vA < numRegs && vB < numRegs {
					frame.Registers[vA] = frame.Registers[vB]
				}
			}
			frame.PC += 3

		case OpMoveResult, OpMoveResultWide, OpMoveResultObj:
			// 11x: move-result vAA
			vAA := int((inst >> 8) & 0xff)
			if vAA < numRegs {
				frame.Registers[vAA] = frame.LastResult
			}
			frame.PC++

		case OpReturnVoid:
			return nil, nil

		case OpReturn, OpReturnObj, OpReturnWide:
			vAA := int((inst >> 8) & 0xff)
			if vAA < numRegs {
				return frame.Registers[vAA], nil
			}
			return nil, nil

		case OpConst4:
			// 11n: const/4 vA, #+B
			vA := int((inst >> 8) & 0x0f)
			rawB := int((inst >> 12) & 0x0f)
			if (rawB & 0x08) != 0 {
				rawB |= -16 // sign-extend 4 bits to 32 bits
			}
			if vA < numRegs {
				frame.Registers[vA] = rawB
			}
			frame.PC++

		case OpConst16:
			// 21s: const/16 vAA, #+BBBB
			vAA := int((inst >> 8) & 0xff)
			if frame.PC+1 < insnsLen {
				val := int16(insns[frame.PC+1])
				if vAA < numRegs {
					frame.Registers[vAA] = int(val)
				}
			}
			frame.PC += 2

		case OpConst:
			// 31i: const vAA, #+BBBBBBBB
			vAA := int((inst >> 8) & 0xff)
			if frame.PC+2 < insnsLen {
				low := uint32(insns[frame.PC+1])
				high := uint32(insns[frame.PC+2])
				val := int32((high << 16) | low)
				if vAA < numRegs {
					frame.Registers[vAA] = int(val)
				}
			}
			frame.PC += 3

		case OpConstString:
			// 21c: const-string vAA, string@BBBB
			vAA := int((inst >> 8) & 0xff)
			if frame.PC+1 < insnsLen {
				strIdx := int(insns[frame.PC+1])
				s := interp.registry.GetString(strIdx)
				if vAA < numRegs {
					frame.Registers[vAA] = s
				}
			}
			frame.PC += 2

		case OpConstStringJumbo:
			vAA := int((inst >> 8) & 0xff)
			if frame.PC+2 < insnsLen {
				strIdx := int(uint32(insns[frame.PC+1]) | (uint32(insns[frame.PC+2]) << 16))
				s := interp.registry.GetString(strIdx)
				if vAA < numRegs {
					frame.Registers[vAA] = s
				}
			}
			frame.PC += 3

		case OpGoto:
			// 10t: goto +AA (signed 8-bit offset)
			offset := int8((inst >> 8) & 0xff)
			frame.PC += int(offset)

		case OpGoto16:
			// 20t: goto/16 +AAAA
			if frame.PC+1 < insnsLen {
				offset := int16(insns[frame.PC+1])
				frame.PC += int(offset)
			} else {
				frame.PC += 2
			}

		case OpIfEq, OpIfNe, OpIfLt, OpIfGe, OpIfGt, OpIfLe:
			vA := int((inst >> 8) & 0x0f)
			vB := int((inst >> 12) & 0x0f)
			offset := int16(0)
			if frame.PC+1 < insnsLen {
				offset = int16(insns[frame.PC+1])
			}
			valA := toInt(getReg(vA))
			valB := toInt(getReg(vB))
			branch := false
			switch op {
			case OpIfEq:
				branch = (valA == valB)
			case OpIfNe:
				branch = (valA != valB)
			case OpIfLt:
				branch = (valA < valB)
			case OpIfGe:
				branch = (valA >= valB)
			case OpIfGt:
				branch = (valA > valB)
			case OpIfLe:
				branch = (valA <= valB)
			}
			if branch {
				frame.PC += int(offset)
			} else {
				frame.PC += 2
			}

		case OpIfEqz, OpIfNez, OpIfLtz, OpIfGez, OpIfGtz, OpIfLez:
			vAA := int((inst >> 8) & 0xff)
			offset := int16(0)
			if frame.PC+1 < insnsLen {
				offset = int16(insns[frame.PC+1])
			}
			val := toInt(getReg(vAA))
			branch := false
			switch op {
			case OpIfEqz:
				branch = (val == 0)
			case OpIfNez:
				branch = (val != 0)
			case OpIfLtz:
				branch = (val < 0)
			case OpIfGez:
				branch = (val >= 0)
			case OpIfGtz:
				branch = (val > 0)
			case OpIfLez:
				branch = (val <= 0)
			}
			if branch {
				frame.PC += int(offset)
			} else {
				frame.PC += 2
			}

		case OpAddInt, OpSubInt, OpMulInt, OpDivInt, OpRemInt, OpAndInt, OpOrInt, OpXorInt:
			vAA := int((inst >> 8) & 0xff)
			if frame.PC+1 < insnsLen {
				next := insns[frame.PC+1]
				vBB := int(next & 0xff)
				vCC := int((next >> 8) & 0xff)
				a := toInt(getReg(vBB))
				b := toInt(getReg(vCC))
				var res int
				switch op {
				case OpAddInt:
					res = a + b
				case OpSubInt:
					res = a - b
				case OpMulInt:
					res = a * b
				case OpDivInt:
					if b != 0 {
						res = a / b
					}
				case OpRemInt:
					if b != 0 {
						res = a % b
					}
				case OpAndInt:
					res = a & b
				case OpOrInt:
					res = a | b
				case OpXorInt:
					res = a ^ b
				}
				setReg(vAA, res)
			}
			frame.PC += 2

		case OpAddIntLit8, OpRsubIntLit8, OpMulIntLit8, OpDivIntLit8, OpRemIntLit8, OpAndIntLit8, OpOrIntLit8, OpXorIntLit8:
			vAA := int((inst >> 8) & 0xff)
			if frame.PC+1 < insnsLen {
				next := insns[frame.PC+1]
				vBB := int(next & 0xff)
				lit := int(int8((next >> 8) & 0xff))
				a := toInt(getReg(vBB))
				var res int
				switch op {
				case OpAddIntLit8:
					res = a + lit
				case OpRsubIntLit8:
					res = lit - a
				case OpMulIntLit8:
					res = a * lit
				case OpDivIntLit8:
					if lit != 0 {
						res = a / lit
					}
				case OpRemIntLit8:
					if lit != 0 {
						res = a % lit
					}
				case OpAndIntLit8:
					res = a & lit
				case OpOrIntLit8:
					res = a | lit
				case OpXorIntLit8:
					res = a ^ lit
				}
				setReg(vAA, res)
			}
			frame.PC += 2

		case OpIget, OpIgetObject, OpIgetBoolean:
			vA := int((inst >> 8) & 0x0f)
			vB := int((inst >> 12) & 0x0f)
			if frame.PC+1 < insnsLen {
				fieldIdx := int(insns[frame.PC+1])
				fieldName := interp.registry.GetFieldName(fieldIdx)
				targetInst := getReg(vB)
				val, _ := interp.registry.GetField(targetInst, fieldName)
				setReg(vA, val)
			}
			frame.PC += 2

		case OpIput, OpIputObject, OpIputBoolean:
			vA := int((inst >> 8) & 0x0f)
			vB := int((inst >> 12) & 0x0f)
			if frame.PC+1 < insnsLen {
				fieldIdx := int(insns[frame.PC+1])
				fieldName := interp.registry.GetFieldName(fieldIdx)
				targetInst := getReg(vB)
				val := getReg(vA)
				_ = interp.registry.SetField(targetInst, fieldName, val)
			}
			frame.PC += 2

		case OpSget, OpSgetObject, OpSgetBoolean:
			vAA := int((inst >> 8) & 0xff)
			if frame.PC+1 < insnsLen {
				fieldIdx := int(insns[frame.PC+1])
				className, fieldName := interp.registry.GetFieldClassAndName(fieldIdx)
				if class, err := interp.registry.ResolveClass(className); err == nil {
					val, _ := interp.registry.GetField(class, fieldName)
					setReg(vAA, val)
				}
			}
			frame.PC += 2

		case OpSput, OpSputObject, OpSputBoolean:
			vAA := int((inst >> 8) & 0xff)
			if frame.PC+1 < insnsLen {
				fieldIdx := int(insns[frame.PC+1])
				className, fieldName := interp.registry.GetFieldClassAndName(fieldIdx)
				if class, err := interp.registry.ResolveClass(className); err == nil {
					val := getReg(vAA)
					_ = interp.registry.SetField(class, fieldName, val)
				}
			}
			frame.PC += 2

		case OpInvokeVirtual, OpInvokeDirect, OpInvokeStatic, OpInvokeSuper, OpInvokeInterface:
			// 35c format
			argCount := int((inst >> 12) & 0x0f)
			if frame.PC+2 < insnsLen {
				methodIdx := int(insns[frame.PC+1])
				regWord := insns[frame.PC+2]
				var callArgs []any
				regIndices := []int{
					int(regWord & 0x0f),
					int((regWord >> 4) & 0x0f),
					int((regWord >> 8) & 0x0f),
					int((regWord >> 12) & 0x0f),
					int((inst >> 8) & 0x0f),
				}
				for i := 0; i < argCount && i < len(regIndices); i++ {
					rIdx := regIndices[i]
					if rIdx < numRegs {
						callArgs = append(callArgs, frame.Registers[rIdx])
					} else {
						callArgs = append(callArgs, nil)
					}
				}

				result, err := interp.dispatchMethodCall(methodIdx, op == OpInvokeStatic, callArgs...)
				if err != nil {
					return nil, err
				}
				frame.LastResult = result
			}
			frame.PC += 3

		case OpInvokeVirtualRange, OpInvokeDirectRange, OpInvokeStaticRange, OpInvokeSuperRange:
			// 3rc format
			argCount := int((inst >> 8) & 0xff)
			if frame.PC+2 < insnsLen {
				methodIdx := int(insns[frame.PC+1])
				startReg := int(insns[frame.PC+2])
				var callArgs []any
				for i := 0; i < argCount; i++ {
					rIdx := startReg + i
					if rIdx < numRegs {
						callArgs = append(callArgs, frame.Registers[rIdx])
					} else {
						callArgs = append(callArgs, nil)
					}
				}
				result, err := interp.dispatchMethodCall(methodIdx, op == OpInvokeStaticRange, callArgs...)
				if err != nil {
					return nil, err
				}
				frame.LastResult = result
			}
			frame.PC += 3

		default:
			// Unhandled or advance 1 code unit
			frame.PC++
		}
	}

	return frame.LastResult, nil
}

// dispatchMethodCall routes an invoke instruction to framework stubs, hooks, or Dalvik bytecode
func (interp *DalvikInterpreter) dispatchMethodCall(methodIdx int, isStatic bool, callArgs ...any) (any, error) {
	className, methodName := interp.registry.GetMethodClassAndName(methodIdx)
	if className == "" || methodName == "" {
		return nil, nil
	}

	var instance any
	args := callArgs
	if !isStatic && len(callArgs) > 0 {
		instance = callArgs[0]
		args = callArgs[1:]
	}

	// 1. Resolve class in registry
	class, err := interp.registry.ResolveClass(className)
	if err == nil && class != nil {
		// Check if target method has bytecode or registered hook
		if targetMethod, found := class.FindMethod(methodName); found {
			if targetMethod.Hook != nil {
				return targetMethod.Hook(instance, args...)
			}
			if targetMethod.CodeItem != nil && len(targetMethod.CodeItem.Insns) > 0 {
				return interp.Execute(targetMethod, instance, args...)
			}
		}
	}

	// 2. Fallback to framework dispatch or reflection
	return interp.registry.InvokeMethod(class, methodName, instance, args...)
}

func toInt(v any) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int32:
		return int(val)
	case int64:
		return int(val)
	case int16:
		return int(val)
	case int8:
		return int(val)
	case uint:
		return int(val)
	case uint32:
		return int(val)
	case uint64:
		return int(val)
	case bool:
		if val {
			return 1
		}
		return 0
	}
	return 0
}
