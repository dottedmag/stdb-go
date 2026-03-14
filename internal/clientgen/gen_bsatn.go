package clientgen

import (
	"fmt"
	"strings"
)

// generateBsatn generates BSATN encode/decode functions for all types.
func generateBsatn(schema *ModuleSchema, pkgName string) ([]byte, error) {
	if len(schema.Types) == 0 && len(schema.Tables) == 0 {
		return nil, nil
	}

	var w strings.Builder
	w.WriteString(clientFileHeader(pkgName))

	imports := map[string]string{
		"bsatn": "go.digitalxero.dev/spacetimedb-client/bsatn",
	}

	var bsatnCode strings.Builder

	// Track which typespace refs have been named
	namedRefs := map[int]string{}
	for _, t := range schema.Types {
		namedRefs[t.TypeRef] = t.Name
	}

	// Generate BSATN for named types
	for _, t := range schema.Types {
		if t.TypeRef < 0 || t.TypeRef >= len(schema.Typespace) {
			continue
		}

		at := &schema.Typespace[t.TypeRef]
		goName := toGoName(t.Name)

		switch at.Kind {
		case ATKProduct:
			if detectSpecialType(at.Product) != "" {
				continue
			}
			generateStructEncode(&bsatnCode, goName, at.Product, schema, imports)
			generateStructDecode(&bsatnCode, goName, at.Product, schema, imports)

		case ATKSum:
			if isOptionType(at.Sum) || isScheduleAtType(at.Sum) {
				continue
			}
			if isSimpleEnum(at.Sum) {
				generateEnumEncode(&bsatnCode, goName, at.Sum)
				generateEnumDecode(&bsatnCode, goName, at.Sum)
			} else {
				generateSumEncode(&bsatnCode, goName, at.Sum, schema, imports)
				generateSumDecode(&bsatnCode, goName, at.Sum, schema, imports)
			}
		}
	}

	// Generate BSATN for table row types without named type defs
	for _, table := range schema.Tables {
		if _, ok := namedRefs[table.TypeRef]; ok {
			continue
		}
		if table.ProductType == nil {
			continue
		}
		if detectSpecialType(table.ProductType) != "" {
			continue
		}

		goName := toGoName(table.Name)
		generateStructEncode(&bsatnCode, goName, table.ProductType, schema, imports)
		generateStructDecode(&bsatnCode, goName, table.ProductType, schema, imports)
	}

	if bsatnCode.Len() == 0 {
		return nil, nil
	}

	w.WriteString(clientImportBlock(imports))
	w.WriteString(bsatnCode.String())

	return []byte(w.String()), nil
}

func generateStructEncode(w *strings.Builder, name string, pt *ProductType, schema *ModuleSchema, imports map[string]string) {
	fmt.Fprintf(w, "func (v *%s) WriteBsatn(w bsatn.Writer) {\n", name)
	for _, elem := range pt.Elements {
		fieldName := toGoName(elem.Name)
		writeFieldEncoder(w, "v."+fieldName, &elem.AlgebraicType, schema, imports, "\t")
	}
	w.WriteString("}\n\n")
}

func generateStructDecode(w *strings.Builder, name string, pt *ProductType, schema *ModuleSchema, imports map[string]string) {
	fmt.Fprintf(w, "func Read%s(r bsatn.Reader) (*%s, error) {\n", name, name)
	fmt.Fprintf(w, "\tv := &%s{}\n", name)
	fmt.Fprintf(w, "\tvar err error\n")

	for _, elem := range pt.Elements {
		fieldName := toGoName(elem.Name)
		writeFieldDecoder(w, "v."+fieldName, &elem.AlgebraicType, schema, imports, "\t")
	}

	w.WriteString("\treturn v, nil\n")
	w.WriteString("}\n\n")
}

func writeFieldEncoder(w *strings.Builder, accessor string, at *AlgebraicType, schema *ModuleSchema, imports map[string]string, indent string) {
	switch at.Kind {
	case ATKBuiltin:
		writeBuiltinEncoder(w, accessor, at.Builtin, indent)
	case ATKRef:
		writeRefEncoder(w, accessor, at.Ref, schema, imports, indent)
	case ATKProduct:
		if specialType := detectSpecialType(at.Product); specialType != "" {
			writeSpecialTypeEncoder(w, accessor, specialType, indent, imports)
		} else {
			fmt.Fprintf(w, "%s%s.WriteBsatn(w)\n", indent, accessor)
		}
	case ATKSum:
		if isOptionType(at.Sum) {
			writeOptionEncoder(w, accessor, &at.Sum.Variants[0].AlgebraicType, schema, imports, indent)
		} else {
			fmt.Fprintf(w, "%s%s.WriteBsatn(w)\n", indent, accessor)
		}
	case ATKArray:
		writeArrayEncoder(w, accessor, at.ArrayTy, schema, imports, indent)
	case ATKMap:
		writeMapEncoder(w, accessor, at.MapKey, at.MapValue, schema, imports, indent)
	}
}

func writeBuiltinEncoder(w *strings.Builder, accessor string, bt BuiltinType, indent string) {
	switch bt {
	case BuiltinBool:
		fmt.Fprintf(w, "%sw.PutBool(%s)\n", indent, accessor)
	case BuiltinU8:
		fmt.Fprintf(w, "%sw.PutU8(%s)\n", indent, accessor)
	case BuiltinU16:
		fmt.Fprintf(w, "%sw.PutU16(%s)\n", indent, accessor)
	case BuiltinU32:
		fmt.Fprintf(w, "%sw.PutU32(%s)\n", indent, accessor)
	case BuiltinU64:
		fmt.Fprintf(w, "%sw.PutU64(%s)\n", indent, accessor)
	case BuiltinI8:
		fmt.Fprintf(w, "%sw.PutI8(%s)\n", indent, accessor)
	case BuiltinI16:
		fmt.Fprintf(w, "%sw.PutI16(%s)\n", indent, accessor)
	case BuiltinI32:
		fmt.Fprintf(w, "%sw.PutI32(%s)\n", indent, accessor)
	case BuiltinI64:
		fmt.Fprintf(w, "%sw.PutI64(%s)\n", indent, accessor)
	case BuiltinF32:
		fmt.Fprintf(w, "%sw.PutF32(%s)\n", indent, accessor)
	case BuiltinF64:
		fmt.Fprintf(w, "%sw.PutF64(%s)\n", indent, accessor)
	case BuiltinString:
		fmt.Fprintf(w, "%sw.PutString(%s)\n", indent, accessor)
	case BuiltinU128:
		fmt.Fprintf(w, "%s%s.WriteBsatn(w)\n", indent, accessor)
	case BuiltinU256:
		fmt.Fprintf(w, "%s%s.WriteBsatn(w)\n", indent, accessor)
	case BuiltinI128:
		fmt.Fprintf(w, "%s%s.WriteBsatn(w)\n", indent, accessor)
	case BuiltinI256:
		fmt.Fprintf(w, "%s%s.WriteBsatn(w)\n", indent, accessor)
	case BuiltinBytes:
		fmt.Fprintf(w, "%sbsatn.WriteByteArray(w, %s)\n", indent, accessor)
	}
}

func writeRefEncoder(w *strings.Builder, accessor string, ref int, schema *ModuleSchema, imports map[string]string, indent string) {
	if ref >= 0 && ref < len(schema.Typespace) {
		at := &schema.Typespace[ref]
		if at.Kind == ATKProduct {
			if specialType := detectSpecialType(at.Product); specialType != "" {
				writeSpecialTypeEncoder(w, accessor, specialType, indent, imports)
				return
			}
		}
		if at.Kind == ATKSum && isSimpleEnum(at.Sum) {
			fmt.Fprintf(w, "%sw.PutU8(uint8(%s))\n", indent, accessor)
			return
		}
	}
	fmt.Fprintf(w, "%s%s.WriteBsatn(w)\n", indent, accessor)
}

func writeSpecialTypeEncoder(w *strings.Builder, accessor, specialType, indent string, imports map[string]string) {
	imports["types"] = "go.digitalxero.dev/spacetimedb-client/types"
	fmt.Fprintf(w, "%s%s.WriteBsatn(w)\n", indent, accessor)
}

func writeOptionEncoder(w *strings.Builder, accessor string, someType *AlgebraicType, schema *ModuleSchema, imports map[string]string, indent string) {
	goType := goTypeForAlgebraic(someType, schema.Typespace, schema.Types)
	_ = goType // used for context

	fmt.Fprintf(w, "%sif %s != nil {\n", indent, accessor)
	fmt.Fprintf(w, "%s\tw.PutSumTag(0) // Some\n", indent)
	writeFieldEncoder(w, "*"+accessor, someType, schema, imports, indent+"\t")
	fmt.Fprintf(w, "%s} else {\n", indent)
	fmt.Fprintf(w, "%s\tw.PutSumTag(1) // None\n", indent)
	fmt.Fprintf(w, "%s}\n", indent)
}

func writeArrayEncoder(w *strings.Builder, accessor string, elemType *AlgebraicType, schema *ModuleSchema, imports map[string]string, indent string) {
	if elemType.Kind == ATKBuiltin && elemType.Builtin == BuiltinU8 {
		fmt.Fprintf(w, "%sbsatn.WriteByteArray(w, %s)\n", indent, accessor)
		return
	}

	fmt.Fprintf(w, "%sw.PutArrayLen(uint32(len(%s)))\n", indent, accessor)
	fmt.Fprintf(w, "%sfor _, elem := range %s {\n", indent, accessor)
	writeFieldEncoder(w, "elem", elemType, schema, imports, indent+"\t")
	fmt.Fprintf(w, "%s}\n", indent)
}

func writeMapEncoder(w *strings.Builder, accessor string, keyType, valType *AlgebraicType, schema *ModuleSchema, imports map[string]string, indent string) {
	fmt.Fprintf(w, "%sw.PutMapLen(uint32(len(%s)))\n", indent, accessor)
	fmt.Fprintf(w, "%sfor k, v := range %s {\n", indent, accessor)
	writeFieldEncoder(w, "k", keyType, schema, imports, indent+"\t")
	writeFieldEncoder(w, "v", valType, schema, imports, indent+"\t")
	fmt.Fprintf(w, "%s}\n", indent)
}

func writeFieldDecoder(w *strings.Builder, accessor string, at *AlgebraicType, schema *ModuleSchema, imports map[string]string, indent string) {
	switch at.Kind {
	case ATKBuiltin:
		writeBuiltinDecoder(w, accessor, at.Builtin, indent)
	case ATKRef:
		writeRefDecoder(w, accessor, at.Ref, schema, imports, indent)
	case ATKProduct:
		if specialType := detectSpecialType(at.Product); specialType != "" {
			writeSpecialTypeDecoder(w, accessor, specialType, indent, imports)
		} else {
			fmt.Fprintf(w, "%s// inline product decode\n", indent)
		}
	case ATKSum:
		if isOptionType(at.Sum) {
			writeOptionDecoder(w, accessor, &at.Sum.Variants[0].AlgebraicType, schema, imports, indent)
		} else {
			fmt.Fprintf(w, "%s// sum type decode\n", indent)
		}
	case ATKArray:
		writeArrayDecoder(w, accessor, at.ArrayTy, schema, imports, indent)
	case ATKMap:
		writeMapDecoder(w, accessor, at.MapKey, at.MapValue, schema, imports, indent)
	}
}

func writeBuiltinDecoder(w *strings.Builder, accessor string, bt BuiltinType, indent string) {
	switch bt {
	case BuiltinBool:
		fmt.Fprintf(w, "%s%s, err = r.GetBool()\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	case BuiltinU8:
		fmt.Fprintf(w, "%s%s, err = r.GetU8()\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	case BuiltinU16:
		fmt.Fprintf(w, "%s%s, err = r.GetU16()\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	case BuiltinU32:
		fmt.Fprintf(w, "%s%s, err = r.GetU32()\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	case BuiltinU64:
		fmt.Fprintf(w, "%s%s, err = r.GetU64()\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	case BuiltinI8:
		fmt.Fprintf(w, "%s%s, err = r.GetI8()\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	case BuiltinI16:
		fmt.Fprintf(w, "%s%s, err = r.GetI16()\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	case BuiltinI32:
		fmt.Fprintf(w, "%s%s, err = r.GetI32()\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	case BuiltinI64:
		fmt.Fprintf(w, "%s%s, err = r.GetI64()\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	case BuiltinF32:
		fmt.Fprintf(w, "%s%s, err = r.GetF32()\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	case BuiltinF64:
		fmt.Fprintf(w, "%s%s, err = r.GetF64()\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	case BuiltinString:
		fmt.Fprintf(w, "%s%s, err = r.GetString()\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	case BuiltinU128:
		fmt.Fprintf(w, "%s%s, err = types.ReadU128(r)\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	case BuiltinU256:
		fmt.Fprintf(w, "%s%s, err = types.ReadU256(r)\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	case BuiltinI128:
		fmt.Fprintf(w, "%s%s, err = types.ReadI128(r)\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	case BuiltinI256:
		fmt.Fprintf(w, "%s%s, err = types.ReadI256(r)\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	case BuiltinBytes:
		fmt.Fprintf(w, "%s%s, err = bsatn.ReadByteArray(r)\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
	}
}

func writeRefDecoder(w *strings.Builder, accessor string, ref int, schema *ModuleSchema, imports map[string]string, indent string) {
	if ref >= 0 && ref < len(schema.Typespace) {
		at := &schema.Typespace[ref]
		if at.Kind == ATKProduct {
			if specialType := detectSpecialType(at.Product); specialType != "" {
				writeSpecialTypeDecoder(w, accessor, specialType, indent, imports)
				return
			}
		}
		if at.Kind == ATKSum && isSimpleEnum(at.Sum) {
			// Find the go type name
			goName := goTypeForRef(ref, schema.Typespace, schema.Types)
			fmt.Fprintf(w, "%s{\n", indent)
			fmt.Fprintf(w, "%s\tvar tag uint8\n", indent)
			fmt.Fprintf(w, "%s\ttag, err = r.GetU8()\n", indent)
			writeDecodeErrCheck(w, indent+"\t")
			fmt.Fprintf(w, "%s\t%s = %s(tag)\n", indent, accessor, goName)
			fmt.Fprintf(w, "%s}\n", indent)
			return
		}
	}

	// Named struct reference
	goName := goTypeForRef(ref, schema.Typespace, schema.Types)
	fmt.Fprintf(w, "%s{\n", indent)
	fmt.Fprintf(w, "%s\tvar decoded *%s\n", indent, goName)
	fmt.Fprintf(w, "%s\tdecoded, err = Read%s(r)\n", indent, goName)
	writeDecodeErrCheck(w, indent+"\t")
	fmt.Fprintf(w, "%s\t%s = *decoded\n", indent, accessor)
	fmt.Fprintf(w, "%s}\n", indent)
}

func writeSpecialTypeDecoder(w *strings.Builder, accessor, specialType, indent string, imports map[string]string) {
	imports["types"] = "go.digitalxero.dev/spacetimedb-client/types"

	switch specialType {
	case "types.Identity":
		fmt.Fprintf(w, "%s%s, err = types.ReadIdentity(r)\n", indent, accessor)
	case "types.ConnectionId":
		fmt.Fprintf(w, "%s%s, err = types.ReadConnectionId(r)\n", indent, accessor)
	case "types.Timestamp":
		fmt.Fprintf(w, "%s%s, err = types.ReadTimestamp(r)\n", indent, accessor)
	case "types.TimeDuration":
		fmt.Fprintf(w, "%s%s, err = types.ReadTimeDuration(r)\n", indent, accessor)
	case "types.UUID":
		fmt.Fprintf(w, "%s%s, err = types.ReadUUID(r)\n", indent, accessor)
	default:
		fmt.Fprintf(w, "%s// unknown special type: %s\n", indent, specialType)
		return
	}
	writeDecodeErrCheck(w, indent)
}

func writeOptionDecoder(w *strings.Builder, accessor string, someType *AlgebraicType, schema *ModuleSchema, imports map[string]string, indent string) {
	fmt.Fprintf(w, "%s{\n", indent)
	fmt.Fprintf(w, "%s\tvar tag uint8\n", indent)
	fmt.Fprintf(w, "%s\ttag, err = r.GetSumTag()\n", indent)
	writeDecodeErrCheck(w, indent+"\t")
	fmt.Fprintf(w, "%s\tif tag == 0 { // Some\n", indent)

	goType := goTypeForAlgebraic(someType, schema.Typespace, schema.Types)
	fmt.Fprintf(w, "%s\t\tvar val %s\n", indent, goType)
	writeFieldDecoder(w, "val", someType, schema, imports, indent+"\t\t")
	fmt.Fprintf(w, "%s\t\t%s = &val\n", indent, accessor)
	fmt.Fprintf(w, "%s\t}\n", indent)
	fmt.Fprintf(w, "%s}\n", indent)
}

func writeArrayDecoder(w *strings.Builder, accessor string, elemType *AlgebraicType, schema *ModuleSchema, imports map[string]string, indent string) {
	if elemType.Kind == ATKBuiltin && elemType.Builtin == BuiltinU8 {
		fmt.Fprintf(w, "%s%s, err = bsatn.ReadByteArray(r)\n", indent, accessor)
		writeDecodeErrCheck(w, indent)
		return
	}

	goElemType := goTypeForAlgebraic(elemType, schema.Typespace, schema.Types)
	fmt.Fprintf(w, "%s{\n", indent)
	fmt.Fprintf(w, "%s\tvar arrLen uint32\n", indent)
	fmt.Fprintf(w, "%s\tarrLen, err = r.GetArrayLen()\n", indent)
	writeDecodeErrCheck(w, indent+"\t")
	fmt.Fprintf(w, "%s\t%s = make([]%s, arrLen)\n", indent, accessor, goElemType)
	fmt.Fprintf(w, "%s\tfor i := uint32(0); i < arrLen; i++ {\n", indent)
	writeFieldDecoder(w, accessor+"[i]", elemType, schema, imports, indent+"\t\t")
	fmt.Fprintf(w, "%s\t}\n", indent)
	fmt.Fprintf(w, "%s}\n", indent)
}

func writeMapDecoder(w *strings.Builder, accessor string, keyType, valType *AlgebraicType, schema *ModuleSchema, imports map[string]string, indent string) {
	goKeyType := goTypeForAlgebraic(keyType, schema.Typespace, schema.Types)
	goValType := goTypeForAlgebraic(valType, schema.Typespace, schema.Types)
	fmt.Fprintf(w, "%s{\n", indent)
	fmt.Fprintf(w, "%s\tvar mapLen uint32\n", indent)
	fmt.Fprintf(w, "%s\tmapLen, err = r.GetMapLen()\n", indent)
	writeDecodeErrCheck(w, indent+"\t")
	fmt.Fprintf(w, "%s\t%s = make(map[%s]%s, mapLen)\n", indent, accessor, goKeyType, goValType)
	fmt.Fprintf(w, "%s\tfor i := uint32(0); i < mapLen; i++ {\n", indent)
	fmt.Fprintf(w, "%s\t\tvar k %s\n", indent, goKeyType)
	fmt.Fprintf(w, "%s\t\tvar val %s\n", indent, goValType)
	writeFieldDecoder(w, "k", keyType, schema, imports, indent+"\t\t")
	writeFieldDecoder(w, "val", valType, schema, imports, indent+"\t\t")
	fmt.Fprintf(w, "%s\t\t%s[k] = val\n", indent, accessor)
	fmt.Fprintf(w, "%s\t}\n", indent)
	fmt.Fprintf(w, "%s}\n", indent)
}

func generateEnumEncode(w *strings.Builder, name string, st *SumType) {
	fmt.Fprintf(w, "func (v %s) WriteBsatn(w bsatn.Writer) {\n", name)
	fmt.Fprintf(w, "\tw.PutU8(uint8(v))\n")
	w.WriteString("}\n\n")
}

func generateEnumDecode(w *strings.Builder, name string, st *SumType) {
	fmt.Fprintf(w, "func Read%s(r bsatn.Reader) (%s, error) {\n", name, name)
	fmt.Fprintf(w, "\ttag, err := r.GetU8()\n")
	fmt.Fprintf(w, "\tif err != nil {\n")
	fmt.Fprintf(w, "\t\treturn 0, err\n")
	fmt.Fprintf(w, "\t}\n")
	fmt.Fprintf(w, "\treturn %s(tag), nil\n", name)
	w.WriteString("}\n\n")
}

func generateSumEncode(w *strings.Builder, name string, st *SumType, schema *ModuleSchema, imports map[string]string) {
	fmt.Fprintf(w, "func Write%s(w bsatn.Writer, v %s) {\n", name, name)
	fmt.Fprintf(w, "\tswitch val := v.(type) {\n")
	for i, variant := range st.Variants {
		variantGoName := name + toGoName(variant.Name)
		fmt.Fprintf(w, "\tcase %s:\n", variantGoName)
		if variant.AlgebraicType.Kind == ATKProduct && variant.AlgebraicType.Product != nil && len(variant.AlgebraicType.Product.Elements) > 0 {
			fmt.Fprintf(w, "\t\tw.PutSumTag(%d)\n", i)
			fmt.Fprintf(w, "\t\tval.WriteBsatn(w)\n")
		} else {
			fmt.Fprintf(w, "\t\t_ = val\n")
			fmt.Fprintf(w, "\t\tbsatn.WriteSumUnit(w, %d)\n", i)
		}
	}
	fmt.Fprintf(w, "\t}\n")
	w.WriteString("}\n\n")

	// WriteBsatn for each variant with fields
	for _, variant := range st.Variants {
		if variant.AlgebraicType.Kind == ATKProduct && variant.AlgebraicType.Product != nil && len(variant.AlgebraicType.Product.Elements) > 0 {
			variantGoName := name + toGoName(variant.Name)
			generateStructEncode(w, variantGoName, variant.AlgebraicType.Product, schema, imports)
		}
	}
}

func generateSumDecode(w *strings.Builder, name string, st *SumType, schema *ModuleSchema, imports map[string]string) {
	fmt.Fprintf(w, "func Read%s(r bsatn.Reader) (%s, error) {\n", name, name)
	fmt.Fprintf(w, "\ttag, err := r.GetSumTag()\n")
	fmt.Fprintf(w, "\tif err != nil {\n")
	fmt.Fprintf(w, "\t\treturn nil, err\n")
	fmt.Fprintf(w, "\t}\n")
	fmt.Fprintf(w, "\tswitch tag {\n")
	for i, variant := range st.Variants {
		variantGoName := name + toGoName(variant.Name)
		fmt.Fprintf(w, "\tcase %d:\n", i)
		if variant.AlgebraicType.Kind == ATKProduct && variant.AlgebraicType.Product != nil && len(variant.AlgebraicType.Product.Elements) > 0 {
			fmt.Fprintf(w, "\t\tv, err := Read%s(r)\n", variantGoName)
			fmt.Fprintf(w, "\t\tif err != nil {\n")
			fmt.Fprintf(w, "\t\t\treturn nil, err\n")
			fmt.Fprintf(w, "\t\t}\n")
			fmt.Fprintf(w, "\t\treturn *v, nil\n")
		} else {
			fmt.Fprintf(w, "\t\treturn %s{}, nil\n", variantGoName)
		}
	}
	fmt.Fprintf(w, "\tdefault:\n")
	fmt.Fprintf(w, "\t\treturn nil, &bsatn.ErrInvalidTag{Tag: tag, SumName: %q}\n", name)
	fmt.Fprintf(w, "\t}\n")
	w.WriteString("}\n\n")

	// ReadXxx for each variant with fields
	for _, variant := range st.Variants {
		if variant.AlgebraicType.Kind == ATKProduct && variant.AlgebraicType.Product != nil && len(variant.AlgebraicType.Product.Elements) > 0 {
			variantGoName := name + toGoName(variant.Name)
			generateStructDecode(w, variantGoName, variant.AlgebraicType.Product, schema, imports)
		}
	}
}

func writeDecodeErrCheck(w *strings.Builder, indent string) {
	fmt.Fprintf(w, "%sif err != nil {\n", indent)
	fmt.Fprintf(w, "%s\treturn nil, err\n", indent)
	fmt.Fprintf(w, "%s}\n", indent)
}
