package servergen

import (
	"fmt"
	"strings"
	"unicode"
)

// generateBsatn generates BSATN encode/decode functions for types in the module.
// When module.MultiPackage is true, only types whose RelDir matches filterRelDir
// are emitted ("" = root package). When single-package, all types are emitted.
func generateBsatn(module *AnalyzedModule, w *strings.Builder, filterRelDir string) {
	for _, typeName := range module.TypeOrder {
		typeInfo := module.Types[typeName]
		if typeInfo == nil {
			continue
		}
		if module.MultiPackage && typeInfo.RelDir != filterRelDir {
			continue
		}
		switch typeInfo.Kind {
		case TypeKindStruct:
			generateStructEncode(typeInfo, w)
			generateStructDecode(typeInfo, w)
		case TypeKindSumType:
			generateSumTypeEncode(typeInfo, w)
			generateSumTypeDecode(typeInfo, w)
		case TypeKindSimpleEnum:
			generateEnumEncode(typeInfo, w)
			generateEnumDecode(typeInfo, w)
		}
	}
}

// generateStructEncode generates a StdbWrite<Name> function.
func generateStructEncode(t *AnalyzedType, w *strings.Builder) {
	fmt.Fprintf(w, "func StdbWrite%s(w bsatn.Writer, v *%s) {\n", t.Name, t.Name)
	for _, f := range t.Fields {
		ptr := fmt.Sprintf("v.%s", f.GoName)
		writeFieldEncode(w, "\t", f.AlgType, ptr, f.GoName)
	}
	fmt.Fprintf(w, "}\n\n")
}

// generateStructDecode generates a StdbRead<Name> function.
func generateStructDecode(t *AnalyzedType, w *strings.Builder) {
	fmt.Fprintf(w, "func StdbRead%s(r bsatn.Reader, v *%s) error {\n", t.Name, t.Name)
	if len(t.Fields) > 0 {
		fmt.Fprintf(w, "\tvar err error\n")
	}
	for _, f := range t.Fields {
		ptr := fmt.Sprintf("v.%s", f.GoName)
		writeFieldDecode(w, "\t", f.AlgType, ptr, f.GoName)
	}
	fmt.Fprintf(w, "\treturn nil\n")
	fmt.Fprintf(w, "}\n\n")
}

// writeFieldEncode writes the encode statement for a single field.
func writeFieldEncode(w *strings.Builder, indent string, algType AlgType, expr string, label string) {
	switch algType.Kind {
	case AlgKindBool:
		fmt.Fprintf(w, "%sw.PutBool(%s)\n", indent, expr)
	case AlgKindU8:
		fmt.Fprintf(w, "%sw.PutU8(%s)\n", indent, expr)
	case AlgKindU16:
		fmt.Fprintf(w, "%sw.PutU16(%s)\n", indent, expr)
	case AlgKindU32:
		fmt.Fprintf(w, "%sw.PutU32(%s)\n", indent, expr)
	case AlgKindU64:
		fmt.Fprintf(w, "%sw.PutU64(%s)\n", indent, expr)
	case AlgKindI8:
		fmt.Fprintf(w, "%sw.PutI8(%s)\n", indent, expr)
	case AlgKindI16:
		fmt.Fprintf(w, "%sw.PutI16(%s)\n", indent, expr)
	case AlgKindI32:
		fmt.Fprintf(w, "%sw.PutI32(%s)\n", indent, expr)
	case AlgKindI64:
		fmt.Fprintf(w, "%sw.PutI64(%s)\n", indent, expr)
	case AlgKindF32:
		fmt.Fprintf(w, "%sw.PutF32(%s)\n", indent, expr)
	case AlgKindF64:
		fmt.Fprintf(w, "%sw.PutF64(%s)\n", indent, expr)
	case AlgKindString:
		fmt.Fprintf(w, "%sw.PutString(%s)\n", indent, expr)
	case AlgKindU128, AlgKindI128:
		// 16-byte types: write raw bytes
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tb := %s.Bytes()\n", indent, expr)
		fmt.Fprintf(w, "%s\tw.PutBytes(b[:])\n", indent)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindU256, AlgKindI256:
		// 32-byte types: write raw bytes
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tb := %s.Bytes()\n", indent, expr)
		fmt.Fprintf(w, "%s\tw.PutBytes(b[:])\n", indent)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindIdentity:
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tb := %s.Bytes()\n", indent, expr)
		fmt.Fprintf(w, "%s\tw.PutBytes(b[:])\n", indent)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindConnectionId:
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tb := %s.Bytes()\n", indent, expr)
		fmt.Fprintf(w, "%s\tw.PutBytes(b[:])\n", indent)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindTimestamp:
		fmt.Fprintf(w, "%sw.PutI64(%s.Microseconds())\n", indent, expr)
	case AlgKindTimeDuration:
		fmt.Fprintf(w, "%sw.PutI64(%s.Microseconds())\n", indent, expr)
	case AlgKindUuid:
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tb := %s.Bytes()\n", indent, expr)
		fmt.Fprintf(w, "%s\tw.PutBytes(b[:])\n", indent)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindScheduleAt:
		fmt.Fprintf(w, "%sStdbWriteScheduleAt(w, %s)\n", indent, expr)
	case AlgKindBytes:
		fmt.Fprintf(w, "%sbsatn.WriteByteArray(w, %s)\n", indent, expr)
	case AlgKindArray:
		elemType := algType.ElemType
		fmt.Fprintf(w, "%sw.PutArrayLen(uint32(len(%s)))\n", indent, expr)
		fmt.Fprintf(w, "%sfor stdbIdx := range %s {\n", indent, expr)
		writeFieldEncode(w, indent+"\t", *elemType, expr+"[stdbIdx]", label+"_elem")
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindOption:
		elemType := algType.ElemType
		fmt.Fprintf(w, "%sif %s != nil {\n", indent, expr)
		fmt.Fprintf(w, "%s\tw.PutSumTag(0) // Some\n", indent)
		writeFieldEncode(w, indent+"\t", *elemType, "(*"+expr+")", label+"_val")
		fmt.Fprintf(w, "%s} else {\n", indent)
		fmt.Fprintf(w, "%s\tw.PutSumTag(1) // None\n", indent)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindRef:
		// Reference to a typespace type (struct, sum type, or enum).
		fmt.Fprintf(w, "%sStdbWrite%s(w, &%s)\n", indent, algType.TypeName, expr)
	}
}

// writeFieldDecode writes the decode statement for a single field.
func writeFieldDecode(w *strings.Builder, indent string, algType AlgType, expr string, label string) {
	switch algType.Kind {
	case AlgKindBool:
		fmt.Fprintf(w, "%sif %s, err = r.GetBool(); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, expr, label)
	case AlgKindU8:
		fmt.Fprintf(w, "%sif %s, err = r.GetU8(); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, expr, label)
	case AlgKindU16:
		fmt.Fprintf(w, "%sif %s, err = r.GetU16(); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, expr, label)
	case AlgKindU32:
		fmt.Fprintf(w, "%sif %s, err = r.GetU32(); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, expr, label)
	case AlgKindU64:
		fmt.Fprintf(w, "%sif %s, err = r.GetU64(); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, expr, label)
	case AlgKindI8:
		fmt.Fprintf(w, "%sif %s, err = r.GetI8(); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, expr, label)
	case AlgKindI16:
		fmt.Fprintf(w, "%sif %s, err = r.GetI16(); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, expr, label)
	case AlgKindI32:
		fmt.Fprintf(w, "%sif %s, err = r.GetI32(); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, expr, label)
	case AlgKindI64:
		fmt.Fprintf(w, "%sif %s, err = r.GetI64(); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, expr, label)
	case AlgKindF32:
		fmt.Fprintf(w, "%sif %s, err = r.GetF32(); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, expr, label)
	case AlgKindF64:
		fmt.Fprintf(w, "%sif %s, err = r.GetF64(); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, expr, label)
	case AlgKindString:
		fmt.Fprintf(w, "%sif %s, err = r.GetString(); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, expr, label)
	case AlgKindU128:
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tvar tmp types.Uint128\n", indent)
		fmt.Fprintf(w, "%s\tif tmp, err = types.ReadUint128(r); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, label)
		fmt.Fprintf(w, "%s\t%s = tmp\n", indent, expr)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindI128:
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tvar tmp types.Int128\n", indent)
		fmt.Fprintf(w, "%s\tif tmp, err = types.ReadInt128(r); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, label)
		fmt.Fprintf(w, "%s\t%s = tmp\n", indent, expr)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindU256:
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tvar tmp types.Uint256\n", indent)
		fmt.Fprintf(w, "%s\tif tmp, err = types.ReadUint256(r); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, label)
		fmt.Fprintf(w, "%s\t%s = tmp\n", indent, expr)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindI256:
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tvar tmp types.Int256\n", indent)
		fmt.Fprintf(w, "%s\tif tmp, err = types.ReadInt256(r); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, label)
		fmt.Fprintf(w, "%s\t%s = tmp\n", indent, expr)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindIdentity:
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tvar tmp types.Identity\n", indent)
		fmt.Fprintf(w, "%s\tif tmp, err = types.ReadIdentity(r); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, label)
		fmt.Fprintf(w, "%s\t%s = tmp\n", indent, expr)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindConnectionId:
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tvar tmp types.ConnectionId\n", indent)
		fmt.Fprintf(w, "%s\tif tmp, err = types.ReadConnectionId(r); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, label)
		fmt.Fprintf(w, "%s\t%s = tmp\n", indent, expr)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindTimestamp:
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tvar tmp types.Timestamp\n", indent)
		fmt.Fprintf(w, "%s\tif tmp, err = types.ReadTimestamp(r); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, label)
		fmt.Fprintf(w, "%s\t%s = tmp\n", indent, expr)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindTimeDuration:
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tvar tmp types.TimeDuration\n", indent)
		fmt.Fprintf(w, "%s\tif tmp, err = types.ReadTimeDuration(r); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, label)
		fmt.Fprintf(w, "%s\t%s = tmp\n", indent, expr)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindUuid:
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tvar tmp types.Uuid\n", indent)
		fmt.Fprintf(w, "%s\tif tmp, err = types.ReadUuid(r); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, label)
		fmt.Fprintf(w, "%s\t%s = tmp\n", indent, expr)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindScheduleAt:
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tvar tmp types.ScheduleAt\n", indent)
		fmt.Fprintf(w, "%s\tif tmp, err = types.ReadScheduleAt(r); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, label)
		fmt.Fprintf(w, "%s\t%s = tmp\n", indent, expr)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindBytes:
		fmt.Fprintf(w, "%sif %s, err = bsatn.ReadByteArray(r); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, expr, label)
	case AlgKindArray:
		elemType := algType.ElemType
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tvar arrLen uint32\n", indent)
		fmt.Fprintf(w, "%s\tif arrLen, err = r.GetArrayLen(); err != nil { return fmt.Errorf(\"decode %s len: %%w\", err) }\n", indent, label)
		fmt.Fprintf(w, "%s\t%s = make(%s, arrLen)\n", indent, expr, algType.TypeName)
		fmt.Fprintf(w, "%s\tfor stdbIdx := uint32(0); stdbIdx < arrLen; stdbIdx++ {\n", indent)
		writeFieldDecode(w, indent+"\t\t", *elemType, expr+"[stdbIdx]", label+"_elem")
		fmt.Fprintf(w, "%s\t}\n", indent)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindOption:
		elemType := algType.ElemType
		tmpName := optTmpVar(label)
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tvar tag uint8\n", indent)
		fmt.Fprintf(w, "%s\tif tag, err = r.GetU8(); err != nil { return fmt.Errorf(\"decode %s tag: %%w\", err) }\n", indent, label)
		fmt.Fprintf(w, "%s\tif tag == 0 {\n", indent) // Some
		goElemType := algType.TypeName[1:]            // strip the *
		fmt.Fprintf(w, "%s\t\tvar %s %s\n", indent, tmpName, goElemType)
		writeFieldDecode(w, indent+"\t\t", *elemType, tmpName, label+"_val")
		fmt.Fprintf(w, "%s\t\t%s = &%s\n", indent, expr, tmpName)
		fmt.Fprintf(w, "%s\t} else {\n", indent) // None
		fmt.Fprintf(w, "%s\t\t%s = nil\n", indent, expr)
		fmt.Fprintf(w, "%s\t}\n", indent)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindRef:
		fmt.Fprintf(w, "%sif err = StdbRead%s(r, &%s); err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", indent, algType.TypeName, expr, label)
	}
}

// generateSumTypeEncode generates encode for a sum type (interface-based).
func generateSumTypeEncode(t *AnalyzedType, w *strings.Builder) {
	// The encode function takes a pointer to the interface value.
	fmt.Fprintf(w, "func StdbWrite%s(w bsatn.Writer, v *%s) {\n", t.Name, t.Name)
	fmt.Fprintf(w, "\tswitch val := (*v).(type) {\n")
	for _, variant := range t.Variants {
		fmt.Fprintf(w, "\tcase %s:\n", variant.StructName)
		fmt.Fprintf(w, "\t\tw.PutSumTag(%d)\n", variant.Tag)
		if len(variant.Fields) == 0 {
			// Unit variant: empty product.
		} else if len(variant.Fields) == 1 {
			// Single-field variant: encode the field directly.
			writeFieldEncode(w, "\t\t", variant.Fields[0].AlgType, "val."+variant.Fields[0].GoName, variant.Name)
		} else {
			// Multi-field variant: encode as product.
			for _, f := range variant.Fields {
				writeFieldEncode(w, "\t\t", f.AlgType, "val."+f.GoName, variant.Name+"_"+f.GoName)
			}
		}
	}
	fmt.Fprintf(w, "\tdefault:\n")
	fmt.Fprintf(w, "\t\tpanic(fmt.Sprintf(\"StdbWrite%s: unknown variant %%T\", *v))\n", t.Name)
	fmt.Fprintf(w, "\t}\n")
	fmt.Fprintf(w, "}\n\n")
}

// generateSumTypeDecode generates decode for a sum type.
func generateSumTypeDecode(t *AnalyzedType, w *strings.Builder) {
	fmt.Fprintf(w, "func StdbRead%s(r bsatn.Reader, v *%s) error {\n", t.Name, t.Name)
	fmt.Fprintf(w, "\tvar err error\n")
	fmt.Fprintf(w, "\tvar tag uint8\n")
	fmt.Fprintf(w, "\tif tag, err = r.GetU8(); err != nil { return fmt.Errorf(\"decode %s tag: %%w\", err) }\n", t.Name)
	fmt.Fprintf(w, "\tswitch tag {\n")
	for _, variant := range t.Variants {
		fmt.Fprintf(w, "\tcase %d: // %s\n", variant.Tag, variant.Name)
		fmt.Fprintf(w, "\t\tvar val %s\n", variant.StructName)
		if len(variant.Fields) == 1 {
			writeFieldDecode(w, "\t\t", variant.Fields[0].AlgType, "val."+variant.Fields[0].GoName, variant.Name)
		} else {
			for _, f := range variant.Fields {
				writeFieldDecode(w, "\t\t", f.AlgType, "val."+f.GoName, variant.Name+"_"+f.GoName)
			}
		}
		fmt.Fprintf(w, "\t\t*v = val\n")
	}
	fmt.Fprintf(w, "\tdefault:\n")
	fmt.Fprintf(w, "\t\treturn &bsatn.ErrInvalidTag{Tag: tag, SumName: %q}\n", t.Name)
	fmt.Fprintf(w, "\t}\n")
	fmt.Fprintf(w, "\treturn nil\n")
	fmt.Fprintf(w, "}\n\n")
}

// generateEnumEncode generates encode for a simple enum (uint8 tag).
func generateEnumEncode(t *AnalyzedType, w *strings.Builder) {
	fmt.Fprintf(w, "func StdbWrite%s(w bsatn.Writer, v *%s) {\n", t.Name, t.Name)
	fmt.Fprintf(w, "\tw.PutU8(uint8(*v))\n")
	fmt.Fprintf(w, "}\n\n")
}

// generateEnumDecode generates decode for a simple enum.
func generateEnumDecode(t *AnalyzedType, w *strings.Builder) {
	fmt.Fprintf(w, "func StdbRead%s(r bsatn.Reader, v *%s) error {\n", t.Name, t.Name)
	fmt.Fprintf(w, "\ttag, err := r.GetU8()\n")
	fmt.Fprintf(w, "\tif err != nil { return fmt.Errorf(\"decode %s: %%w\", err) }\n", t.Name)
	fmt.Fprintf(w, "\tif int(tag) >= %d {\n", len(t.EnumVariants))
	fmt.Fprintf(w, "\t\treturn &bsatn.ErrInvalidTag{Tag: tag, SumName: %q}\n", t.Name)
	fmt.Fprintf(w, "\t}\n")
	fmt.Fprintf(w, "\t*v = %s(tag)\n", t.Name)
	fmt.Fprintf(w, "\treturn nil\n")
	fmt.Fprintf(w, "}\n\n")
}

// optTmpVar generates a unique temporary variable name for Option decode based on the field label.
// This prevents variable shadowing in nested Option types (e.g., *[]*int32).
func optTmpVar(label string) string {
	parts := strings.Split(label, "_")
	name := "tmp"
	for _, p := range parts {
		if len(p) > 0 {
			runes := []rune(p)
			runes[0] = unicode.ToUpper(runes[0])
			name += string(runes)
		}
	}
	return name
}

// generateScheduleAtHelpers generates the shared ScheduleAt encode/decode helpers.
func generateScheduleAtHelpers(w *strings.Builder) {
	w.WriteString(`func StdbWriteScheduleAt(w bsatn.Writer, sa types.ScheduleAt) {
	switch v := sa.(type) {
	case types.ScheduleAtInterval:
		w.PutSumTag(0)
		w.PutI64(v.Value.Microseconds())
	case types.ScheduleAtTime:
		w.PutSumTag(1)
		w.PutI64(v.Value.Microseconds())
	default:
		panic(fmt.Sprintf("StdbWriteScheduleAt: unknown variant %T", sa))
	}
}

`)
}
