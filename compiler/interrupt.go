package compiler

import (
	"strconv"
	"strings"

	"golang.org/x/tools/go/ssa"
	"tinygo.org/x/go-llvm"
)

// createInterruptGlobal creates a new runtime/interrupt.Interrupt struct that
// will be lowered to a real interrupt during interrupt lowering.
//
// This two-stage approach allows unused interrupts to be optimized away if
// necessary.
func (b *builder) createInterruptGlobal(instr *ssa.CallCommon) (llvm.Value, error) {
	id, err := b.getInterruptID(instr)
	if err != nil {
		return llvm.Value{}, err
	}

	funcValue := b.getValue(instr.Args[1], getPos(instr))
	if funcValue.IsAConstant().IsNil() {
		switch instr.Args[1].(type) {
		case *ssa.MakeClosure:
			return llvm.Value{}, b.makeError(instr.Pos(), "closures are not supported in interrupt.New")
		}
		return llvm.Value{}, b.makeError(instr.Pos(), "interrupt function must be constant")
	}
	funcRawPtr, funcContext := b.decodeFuncValue(funcValue)
	funcPtr := llvm.ConstPtrToInt(funcRawPtr, b.uintptrType)

	globalType := b.program.ImportedPackage("runtime/interrupt").Type("handle").Type()
	globalLLVMType := b.getLLVMType(globalType)
	globalName := b.fn.Package().Pkg.Path() + "$interrupt" + strconv.FormatInt(id, 10)
	global := llvm.AddGlobal(b.mod, globalLLVMType, globalName)
	global.SetVisibility(llvm.HiddenVisibility)
	global.SetGlobalConstant(true)
	global.SetUnnamedAddr(true)
	global.SetAlignment(1)
	initializer := llvm.ConstNull(globalLLVMType)
	initializer = b.CreateInsertValue(initializer, funcContext, 0, "")
	initializer = b.CreateInsertValue(initializer, funcPtr, 1, "")
	initializer = b.CreateInsertValue(initializer, llvm.ConstNamedStruct(globalLLVMType.StructElementTypes()[2], []llvm.Value{
		llvm.ConstInt(b.intType, uint64(id), true),
	}), 2, "")
	global.SetInitializer(initializer)

	if b.Debug {
		pos := b.program.Fset.Position(instr.Pos())
		diglobal := b.dibuilder.CreateGlobalVariableExpression(b.getDIFile(pos.Filename), llvm.DIGlobalVariableExpression{
			Name:        "interrupt" + strconv.FormatInt(id, 10),
			LinkageName: globalName,
			File:        b.getDIFile(pos.Filename),
			Line:        pos.Line,
			Type:        b.getDIType(globalType),
			Expr:        b.dibuilder.CreateExpression(nil),
			LocalToUnit: false,
		})
		global.AddMetadata(0, diglobal)
	}

	num := llvm.ConstPtrToInt(global, b.intType)
	interrupt := llvm.ConstNamedStruct(b.mod.GetTypeByName("runtime/interrupt.Interrupt"), []llvm.Value{num})

	if strings.HasPrefix(b.Triple, "avr") {
		useFn := b.mod.NamedFunction("runtime/interrupt.use")
		if useFn.IsNil() {
			useFnType := llvm.FunctionType(b.ctx.VoidType(), []llvm.Type{interrupt.Type()}, false)
			useFn = llvm.AddFunction(b.mod, "runtime/interrupt.use", useFnType)
		}
		b.CreateCall(useFn.GlobalValueType(), useFn, []llvm.Value{interrupt}, "")
	}

	return interrupt, nil
}

func (b *builder) getInterruptID(instr *ssa.CallCommon) (int64, error) {
	firstArg := instr.Args[0]

	switch arg := firstArg.(type) {
	case *ssa.Const:
		if arg.Type().Underlying().String() == "string" {
			name := arg.String()
			name = strings.Trim(name, "\"")
			return b.lookupInterruptID(name, instr)
		}
		return arg.Int64(), nil
	default:
		return 0, b.makeError(instr.Pos(), "interrupt ID is not a constant")
	}
}

func (b *builder) lookupInterruptID(name string, instr *ssa.CallCommon) (int64, error) {
	if b.Interrupts == nil || len(b.Interrupts) == 0 {
		return 0, b.makeError(instr.Pos(), "interrupt name lookup not available for this target")
	}

	searchName := strings.ToUpper(name)
	for key, id := range b.Interrupts {
		keyName := strings.ToUpper(key)
		if keyName == searchName ||
			keyName == "IRQ_"+searchName ||
			keyName == searchName+"_IRQ" ||
			keyName == "_"+searchName ||
			keyName == searchName+"_" ||
			"IRQ_"+keyName == searchName ||
			keyName+"_IRQ" == searchName ||
			"_"+keyName == searchName ||
			keyName+"_" == searchName {
			return int64(id), nil
		}
	}

	return 0, b.makeError(instr.Pos(), "unknown interrupt name: "+name)
}
