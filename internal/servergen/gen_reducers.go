package servergen

import (
	"fmt"
	"strings"
)

// generateReducerDispatch generates the reducer, procedure, and view dispatch functions.
func generateReducerDispatch(module *AnalyzedModule, w *strings.Builder) {
	generateCallReducer(module, w)
	if len(module.Procedures) > 0 {
		generateCallProcedure(module, w)
	}
	if hasAuthViews(module) {
		generateCallView(module, w)
	}
	if hasAnonViews(module) {
		generateCallViewAnon(module, w)
	}
}

// generateCallReducer generates the stdbCallReducer dispatch function.
func generateCallReducer(module *AnalyzedModule, w *strings.Builder) {
	fmt.Fprintf(w, "func stdbCallReducer(id uint32, ctx reducer.ReducerContext, args []byte) error {\n")
	fmt.Fprintf(w, "\tswitch id {\n")

	// Regular reducers.
	for _, r := range module.Reducers {
		call := r.CallName
		if call == "" {
			call = r.FuncName
		}
		fmt.Fprintf(w, "\tcase %d: // %s\n", r.ID, r.Name)
		if len(r.Params) == 0 {
			if r.HasError {
				fmt.Fprintf(w, "\t\treturn %s(ctx)\n", call)
			} else {
				fmt.Fprintf(w, "\t\t%s(ctx)\n", call)
				fmt.Fprintf(w, "\t\treturn nil\n")
			}
		} else {
			fmt.Fprintf(w, "\t\tstdbReader := bsatn.NewZeroCopyReader(args)\n")
			for i, p := range r.Params {
				writeParamDecode(w, "\t\t", module, p, i, "stdbReader", "")
			}
			callArgs := "ctx"
			for _, p := range r.Params {
				callArgs += ", " + p.Name
			}
			if r.HasError {
				fmt.Fprintf(w, "\t\treturn %s(%s)\n", call, callArgs)
			} else {
				fmt.Fprintf(w, "\t\t%s(%s)\n", call, callArgs)
				fmt.Fprintf(w, "\t\treturn nil\n")
			}
		}
	}

	// Lifecycle reducers.
	for _, lc := range module.Lifecycle {
		call := lc.CallName
		if call == "" {
			call = lc.FuncName
		}
		fmt.Fprintf(w, "\tcase %d: // %s (%s)\n", lc.ID, lc.FuncName, lifecycleName(lc.Kind))
		fmt.Fprintf(w, "\t\t%s(ctx)\n", call)
		fmt.Fprintf(w, "\t\treturn nil\n")
	}

	fmt.Fprintf(w, "\tdefault:\n")
	fmt.Fprintf(w, "\t\treturn fmt.Errorf(\"unknown reducer id %%d\", id)\n")
	fmt.Fprintf(w, "\t}\n")
	fmt.Fprintf(w, "}\n\n")
}

// generateCallProcedure generates the stdbCallProcedure dispatch function.
func generateCallProcedure(module *AnalyzedModule, w *strings.Builder) {
	fmt.Fprintf(w, "func stdbCallProcedure(id uint32, ctx reducer.ProcedureContext, args []byte) ([]byte, error) {\n")
	fmt.Fprintf(w, "\tswitch id {\n")

	for _, p := range module.Procedures {
		call := p.CallName
		if call == "" {
			call = p.FuncName
		}
		fmt.Fprintf(w, "\tcase %d: // %s\n", p.ID, p.Name)
		if len(p.Params) == 0 && p.ReturnType == nil {
			fmt.Fprintf(w, "\t\t%s(ctx)\n", call)
			fmt.Fprintf(w, "\t\tw := bsatn.NewWriter(4)\n")
			fmt.Fprintf(w, "\t\treturn w.Bytes(), nil\n")
		} else if len(p.Params) == 0 && p.ReturnType != nil {
			fmt.Fprintf(w, "\t\tresult := %s(ctx)\n", call)
			fmt.Fprintf(w, "\t\tw := bsatn.NewWriter(256)\n")
			writeReturnEncode(w, "\t\t", module, *p.ReturnType, "result", p.ReturnGoType)
			fmt.Fprintf(w, "\t\treturn w.Bytes(), nil\n")
		} else {
			fmt.Fprintf(w, "\t\tstdbReader := bsatn.NewZeroCopyReader(args)\n")
			for i, param := range p.Params {
				writeParamDecode(w, "\t\t", module, param, i, "stdbReader", "nil, ")
			}
			callArgs := "ctx"
			for _, param := range p.Params {
				callArgs += ", " + param.Name
			}
			if p.ReturnType == nil {
				fmt.Fprintf(w, "\t\t%s(%s)\n", call, callArgs)
				fmt.Fprintf(w, "\t\tw := bsatn.NewWriter(4)\n")
				fmt.Fprintf(w, "\t\treturn w.Bytes(), nil\n")
			} else {
				fmt.Fprintf(w, "\t\tresult := %s(%s)\n", call, callArgs)
				fmt.Fprintf(w, "\t\tw := bsatn.NewWriter(256)\n")
				writeReturnEncode(w, "\t\t", module, *p.ReturnType, "result", p.ReturnGoType)
				fmt.Fprintf(w, "\t\treturn w.Bytes(), nil\n")
			}
		}
	}

	fmt.Fprintf(w, "\tdefault:\n")
	fmt.Fprintf(w, "\t\treturn nil, fmt.Errorf(\"unknown procedure id %%d\", id)\n")
	fmt.Fprintf(w, "\t}\n")
	fmt.Fprintf(w, "}\n\n")
}

// generateCallView generates the stdbCallView dispatch for authenticated views.
func generateCallView(module *AnalyzedModule, w *strings.Builder) {
	fmt.Fprintf(w, "func stdbCallView(id uint32, sender types.Identity, args []byte) ([]byte, error) {\n")
	fmt.Fprintf(w, "\tctx := reducer.NewViewContext(sender)\n")
	fmt.Fprintf(w, "\tswitch id {\n")

	for _, v := range module.Views {
		if v.IsAnonymous {
			continue
		}
		call := v.CallName
		if call == "" {
			call = v.FuncName
		}
		fmt.Fprintf(w, "\tcase %d: // %s\n", v.ID, v.Name)
		if len(v.Params) > 0 {
			fmt.Fprintf(w, "\t\tstdbReader := bsatn.NewZeroCopyReader(args)\n")
			for i, param := range v.Params {
				writeParamDecode(w, "\t\t", module, param, i, "stdbReader", "nil, ")
			}
		}
		callArgs := "ctx"
		for _, param := range v.Params {
			callArgs += ", " + param.Name
		}
		fmt.Fprintf(w, "\t\tresult := %s(%s)\n", call, callArgs)
		fmt.Fprintf(w, "\t\tw := bsatn.NewWriter(256)\n")
		fmt.Fprintf(w, "\t\tw.PutSumTag(0) // ViewResultHeader::RowData\n")
		writeViewResultEncode(w, "\t\t", module, v.ReturnType, v.ReturnGoType, "result")
		fmt.Fprintf(w, "\t\treturn w.Bytes(), nil\n")
	}

	fmt.Fprintf(w, "\tdefault:\n")
	fmt.Fprintf(w, "\t\treturn nil, fmt.Errorf(\"unknown view id %%d\", id)\n")
	fmt.Fprintf(w, "\t}\n")
	fmt.Fprintf(w, "}\n\n")
}

// generateCallViewAnon generates the stdbCallViewAnon dispatch for anonymous views.
func generateCallViewAnon(module *AnalyzedModule, w *strings.Builder) {
	fmt.Fprintf(w, "func stdbCallViewAnon(id uint32, args []byte) ([]byte, error) {\n")
	fmt.Fprintf(w, "\tctx := reducer.NewAnonymousViewContext()\n")
	fmt.Fprintf(w, "\tswitch id {\n")

	for _, v := range module.Views {
		if !v.IsAnonymous {
			continue
		}
		call := v.CallName
		if call == "" {
			call = v.FuncName
		}
		fmt.Fprintf(w, "\tcase %d: // %s\n", v.ID, v.Name)
		if len(v.Params) > 0 {
			fmt.Fprintf(w, "\t\tstdbReader := bsatn.NewZeroCopyReader(args)\n")
			for i, param := range v.Params {
				writeParamDecode(w, "\t\t", module, param, i, "stdbReader", "nil, ")
			}
		}
		callArgs := "ctx"
		for _, param := range v.Params {
			callArgs += ", " + param.Name
		}
		fmt.Fprintf(w, "\t\tresult := %s(%s)\n", call, callArgs)
		fmt.Fprintf(w, "\t\tw := bsatn.NewWriter(256)\n")
		fmt.Fprintf(w, "\t\tw.PutSumTag(0) // ViewResultHeader::RowData\n")
		writeViewResultEncode(w, "\t\t", module, v.ReturnType, v.ReturnGoType, "result")
		fmt.Fprintf(w, "\t\treturn w.Bytes(), nil\n")
	}

	fmt.Fprintf(w, "\tdefault:\n")
	fmt.Fprintf(w, "\t\treturn nil, fmt.Errorf(\"unknown anon view id %%d\", id)\n")
	fmt.Fprintf(w, "\t}\n")
	fmt.Fprintf(w, "}\n\n")
}

// codecCall returns the Go expression for StdbReadX / StdbWriteX for a named
// product/sum type. In multi-package modules, types owned by another package are
// reached as schema.StdbReadPlayer (exported codecs live next to the type).
func codecCall(module *AnalyzedModule, typeName, which string) string {
	fn := "Stdb" + which + typeName
	if module == nil || !module.MultiPackage || typeName == "" {
		return fn
	}
	t := module.Types[typeName]
	if t == nil || t.RelDir == "" {
		// Root-owned type: codec is in the root package (tables file or module package).
		return fn
	}
	alias := t.Package
	if alias == "" {
		alias = pathBase(t.ImportPath)
	}
	return alias + "." + fn
}

// writeParamDecode writes code to decode a function parameter from BSATN.
// readerName is the variable name of the bsatn.Reader (e.g., "stdbReader").
// errRetPrefix is prepended before fmt.Errorf in return statements (e.g., "" for reducers, "nil, " for procedures/views).
func writeParamDecode(w *strings.Builder, indent string, module *AnalyzedModule, param AnalyzedParam, idx int, readerName string, errRetPrefix string) {
	goType := param.GoType
	// Qualify product types for the root package when needed.
	if module != nil && module.MultiPackage && param.AlgType.Kind == AlgKindRef {
		if t := module.Types[param.AlgType.TypeName]; t != nil && t.RelDir != "" {
			// Prefer the type's package-qualified form when the param was written
			// with a bare name (same-package reducer) but we're decoding in root.
			if !strings.Contains(goType, ".") {
				goType = qualifyTypeName(param.AlgType.TypeName, t.Package, t.ImportPath, false, true)
			}
		}
	}
	fmt.Fprintf(w, "%svar %s %s\n", indent, param.Name, goType)

	switch param.AlgType.Kind {
	case AlgKindBool:
		fmt.Fprintf(w, "%s{ v, err := %s.GetBool(); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindU8:
		fmt.Fprintf(w, "%s{ v, err := %s.GetU8(); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindU16:
		fmt.Fprintf(w, "%s{ v, err := %s.GetU16(); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindU32:
		fmt.Fprintf(w, "%s{ v, err := %s.GetU32(); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindU64:
		fmt.Fprintf(w, "%s{ v, err := %s.GetU64(); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindI8:
		fmt.Fprintf(w, "%s{ v, err := %s.GetI8(); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindI16:
		fmt.Fprintf(w, "%s{ v, err := %s.GetI16(); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindI32:
		fmt.Fprintf(w, "%s{ v, err := %s.GetI32(); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindI64:
		fmt.Fprintf(w, "%s{ v, err := %s.GetI64(); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindF32:
		fmt.Fprintf(w, "%s{ v, err := %s.GetF32(); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindF64:
		fmt.Fprintf(w, "%s{ v, err := %s.GetF64(); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindString:
		fmt.Fprintf(w, "%s{ v, err := %s.GetString(); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindU128:
		fmt.Fprintf(w, "%s{ v, err := types.ReadUint128(%s); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindI128:
		fmt.Fprintf(w, "%s{ v, err := types.ReadInt128(%s); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindU256:
		fmt.Fprintf(w, "%s{ v, err := types.ReadUint256(%s); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindI256:
		fmt.Fprintf(w, "%s{ v, err := types.ReadInt256(%s); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindIdentity:
		fmt.Fprintf(w, "%s{ v, err := types.ReadIdentity(%s); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindConnectionId:
		fmt.Fprintf(w, "%s{ v, err := types.ReadConnectionId(%s); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindUuid:
		fmt.Fprintf(w, "%s{ v, err := types.ReadUuid(%s); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindTimestamp:
		fmt.Fprintf(w, "%s{ v, err := types.ReadTimestamp(%s); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindTimeDuration:
		fmt.Fprintf(w, "%s{ v, err := types.ReadTimeDuration(%s); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindScheduleAt:
		fmt.Fprintf(w, "%s{ v, err := types.ReadScheduleAt(%s); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindBytes:
		fmt.Fprintf(w, "%s{ v, err := bsatn.ReadByteArray(%s); if err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }; %s = v }\n", indent, readerName, errRetPrefix, idx, param.Name)
	case AlgKindRef:
		readFn := codecCall(module, param.AlgType.TypeName, "Read")
		fmt.Fprintf(w, "%sif err := %s(%s, &%s); err != nil { return %sfmt.Errorf(\"decode arg %d: %%w\", err) }\n", indent, readFn, readerName, param.Name, errRetPrefix, idx)
	case AlgKindArray, AlgKindOption:
		// For complex composite types, wrap writeFieldDecode in an anonymous function.
		// writeFieldDecode generates code that uses 'r' as the reader and 'err' as the error variable,
		// and returns 'error' on failure. The anonymous function provides these bindings and
		// catches any returned error.
		fmt.Fprintf(w, "%sif stdbDecErr := func() error {\n", indent)
		fmt.Fprintf(w, "%s\tr := %s\n", indent, readerName)
		fmt.Fprintf(w, "%s\tvar err error\n", indent)
		writeFieldDecode(w, indent+"\t", param.AlgType, param.Name, fmt.Sprintf("arg_%d", idx))
		fmt.Fprintf(w, "%s\treturn nil\n", indent)
		fmt.Fprintf(w, "%s}(); stdbDecErr != nil {\n", indent)
		fmt.Fprintf(w, "%s\treturn %sfmt.Errorf(\"decode arg %d: %%w\", stdbDecErr)\n", indent, errRetPrefix, idx)
		fmt.Fprintf(w, "%s}\n", indent)
	default:
		fmt.Fprintf(w, "%s// TODO: decode %s of type %s\n", indent, param.Name, goType)
	}
}

// writeReturnEncode writes code to encode a procedure return value.
func writeReturnEncode(w *strings.Builder, indent string, module *AnalyzedModule, algType AlgType, expr string, goType string) {
	switch algType.Kind {
	case AlgKindRef:
		writeFn := codecCall(module, algType.TypeName, "Write")
		fmt.Fprintf(w, "%s%s(w, &%s)\n", indent, writeFn, expr)
	default:
		writeFieldEncode(w, indent, algType, expr, "result")
	}
}

// writeViewResultEncode writes code to encode a view result as Vec<RowType>.
func writeViewResultEncode(w *strings.Builder, indent string, module *AnalyzedModule, algType AlgType, goType string, expr string) {
	// Determine if the return type is a pointer (*T -> Option), slice ([]T -> Vec), or single (T).
	if strings.HasPrefix(goType, "*") {
		// Option<T>: encode as array of 0 or 1 elements.
		fmt.Fprintf(w, "%sif %s != nil {\n", indent, expr)
		fmt.Fprintf(w, "%s\tw.PutArrayLen(1)\n", indent)
		innerType := goType[1:]
		if algType.ElemType != nil {
			writeReturnEncode(w, indent+"\t", module, *algType.ElemType, "(*"+expr+")", innerType)
		}
		fmt.Fprintf(w, "%s} else {\n", indent)
		fmt.Fprintf(w, "%s\tw.PutArrayLen(0)\n", indent)
		fmt.Fprintf(w, "%s}\n", indent)
	} else if strings.HasPrefix(goType, "[]") {
		// Vec<T>: encode as array of elements.
		fmt.Fprintf(w, "%sif %s == nil {\n", indent, expr)
		fmt.Fprintf(w, "%s\tw.PutArrayLen(0)\n", indent)
		fmt.Fprintf(w, "%s} else {\n", indent)
		fmt.Fprintf(w, "%s\tw.PutArrayLen(uint32(len(%s)))\n", indent, expr)
		fmt.Fprintf(w, "%s\tfor i := range %s {\n", indent, expr)
		if algType.ElemType != nil {
			writeReturnEncode(w, indent+"\t\t", module, *algType.ElemType, expr+"[i]", goType[2:])
		}
		fmt.Fprintf(w, "%s\t}\n", indent)
		fmt.Fprintf(w, "%s}\n", indent)
	} else {
		// Single value: encode as array of 1 element.
		fmt.Fprintf(w, "%sw.PutArrayLen(1)\n", indent)
		writeReturnEncode(w, indent, module, algType, expr, goType)
	}
}

// lifecycleName returns the BSATN lifecycle name.
func lifecycleName(kind string) string {
	switch kind {
	case "init":
		return "__init__"
	case "connect":
		return "__identity_connected__"
	case "disconnect":
		return "__identity_disconnected__"
	default:
		return kind
	}
}

func hasAuthViews(module *AnalyzedModule) bool {
	for _, v := range module.Views {
		if !v.IsAnonymous {
			return true
		}
	}
	return false
}

func hasAnonViews(module *AnalyzedModule) bool {
	for _, v := range module.Views {
		if v.IsAnonymous {
			return true
		}
	}
	return false
}
