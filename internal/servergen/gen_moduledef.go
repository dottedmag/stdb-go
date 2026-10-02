package servergen

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/dottedmag/stdb-go/internal/parser"
)

// generateModuleDef generates the stdbDescribeModule function that builds the
// module definition statically (no reflection).
func generateModuleDef(module *AnalyzedModule, w *strings.Builder) error {
	fmt.Fprintf(w, "func stdbDescribeModule() []byte {\n")
	fmt.Fprintf(w, "\tts := types.NewTypespace()\n")
	fmt.Fprintf(w, "\tbuilder := moduledef.NewModuleDefBuilder()\n\n")

	// Reserve typespace slots for all types.
	for _, typeName := range module.TypeOrder {
		varName := "ref" + typeName
		fmt.Fprintf(w, "\t%s := ts.Reserve()\n", varName)
	}
	fmt.Fprintf(w, "\n")

	// Fill typespace slots.
	for _, typeName := range module.TypeOrder {
		typeInfo := module.Types[typeName]
		varName := "ref" + typeName
		switch typeInfo.Kind {
		case TypeKindStruct:
			writeTypespaceStruct(w, typeInfo, varName, module)
		case TypeKindSumType:
			writeTypespaceSumType(w, typeInfo, varName, module)
		case TypeKindSimpleEnum:
			writeTypespaceEnum(w, typeInfo, varName)
		}
		fmt.Fprintf(w, "\n")
	}

	// Add TypeDefs.
	for _, typeName := range module.TypeOrder {
		typeInfo := module.Types[typeName]
		varName := "ref" + typeName
		writeTypeDef(w, typeInfo, varName)
	}
	fmt.Fprintf(w, "\n")

	// Add tables.
	for _, table := range module.Tables {
		if err := writeTableDef(w, &table, module); err != nil {
			return err
		}
	}

	// Add reducers.
	for _, r := range module.Reducers {
		writeReducerDef(w, &r, module)
	}

	// Add lifecycle reducers (as both reducer def and lifecycle def).
	for _, lc := range module.Lifecycle {
		writeLifecycleReducerDef(w, &lc)
	}

	// Add procedures.
	for _, p := range module.Procedures {
		writeProcedureDef(w, &p, module)
	}

	// Add views.
	for _, v := range module.Views {
		writeViewDef(w, &v, module)
	}

	// Add schedules.
	for _, sched := range module.Schedules {
		writeScheduleDef(w, &sched, module)
	}

	// Add RLS filters.
	for _, sql := range module.RLS {
		fmt.Fprintf(w, "\tbuilder = builder.AddRowLevelSecurity(%q)\n", sql)
	}

	fmt.Fprintf(w, "\n\tbuilder = builder.SetTypespace(ts)\n")
	fmt.Fprintf(w, "\treturn bsatn.Encode(builder.Build())\n")
	fmt.Fprintf(w, "}\n\n")
	return nil
}

// writeTypespaceStruct writes code to fill a struct type in the typespace.
func writeTypespaceStruct(w *strings.Builder, t *AnalyzedType, varName string, module *AnalyzedModule) {
	fmt.Fprintf(w, "\tts.Set(%s, types.AlgTypeProduct(types.NewProductType(\n", varName)
	for _, f := range t.Fields {
		fmt.Fprintf(w, "\t\ttypes.ProductTypeElement{Name: %q, AlgebraicType: %s},\n", f.BsatnName, algTypeExpr(f.AlgType, module))
	}
	fmt.Fprintf(w, "\t)))\n")
}

// writeTypespaceSumType writes code to fill a sum type in the typespace.
func writeTypespaceSumType(w *strings.Builder, t *AnalyzedType, varName string, module *AnalyzedModule) {
	fmt.Fprintf(w, "\tts.Set(%s, types.AlgTypeSum(types.NewSumType(\n", varName)
	for _, v := range t.Variants {
		if len(v.Fields) == 0 {
			// Unit variant: empty product.
			fmt.Fprintf(w, "\t\ttypes.SumTypeVariant{Name: %q, AlgebraicType: types.AlgTypeProduct(types.NewProductType())},\n", v.Name)
		} else if len(v.Fields) == 1 {
			// Single-field variant: the field's type.
			fmt.Fprintf(w, "\t\ttypes.SumTypeVariant{Name: %q, AlgebraicType: %s},\n", v.Name, algTypeExpr(v.Fields[0].AlgType, module))
		} else {
			// Multi-field variant: product type.
			fmt.Fprintf(w, "\t\ttypes.SumTypeVariant{Name: %q, AlgebraicType: types.AlgTypeProduct(types.NewProductType(\n", v.Name)
			for _, f := range v.Fields {
				fmt.Fprintf(w, "\t\t\ttypes.ProductTypeElement{Name: %q, AlgebraicType: %s},\n", f.BsatnName, algTypeExpr(f.AlgType, module))
			}
			fmt.Fprintf(w, "\t\t))},\n")
		}
	}
	fmt.Fprintf(w, "\t)))\n")
}

// writeTypespaceEnum writes code to fill a simple enum in the typespace.
func writeTypespaceEnum(w *strings.Builder, t *AnalyzedType, varName string) {
	fmt.Fprintf(w, "\tts.Set(%s, types.AlgTypeSum(types.NewSumType(\n", varName)
	for _, v := range t.EnumVariants {
		fmt.Fprintf(w, "\t\ttypes.SumTypeVariant{Name: %q, AlgebraicType: types.AlgTypeProduct(types.NewProductType())},\n", v)
	}
	fmt.Fprintf(w, "\t)))\n")
}

// writeTypeDef writes code to add a TypeDef to the module definition.
func writeTypeDef(w *strings.Builder, t *AnalyzedType, varName string) {
	scopeExpr := "nil"
	if len(t.Scope) > 0 {
		parts := make([]string, len(t.Scope))
		for i, s := range t.Scope {
			parts[i] = fmt.Sprintf("%q", s)
		}
		scopeExpr = "[]string{" + strings.Join(parts, ", ") + "}"
	}

	fmt.Fprintf(w, "\tbuilder = builder.AddTypeDef(moduledef.NewTypeDefBuilder(%s, %q, %s)", scopeExpr, t.Name, varName)
	if t.CustomOrdering {
		fmt.Fprintf(w, ".\n\t\tWithCustomOrdering(true)")
	}
	fmt.Fprintf(w, ".Build())\n")
}

// writeTableDef writes code to add a TableDef to the module definition.
func writeTableDef(w *strings.Builder, table *AnalyzedTable, module *AnalyzedModule) error {
	refVar := "ref" + table.StructName

	// Pre-encode column default values into local bsatn writers, emitted before
	// the builder chain. The byte output must match what the column codec would
	// produce for that value: the migration planner decodes it to backfill rows.
	type defaultEntry struct {
		colIndex uint16
		varName  string
	}
	var defaults []defaultEntry
	for i := range table.Fields {
		f := table.Fields[i]
		if f.Default == nil {
			continue
		}
		varName := fmt.Sprintf("stdbDef_%s_%s", table.Name, f.BsatnName)
		fmt.Fprintf(w, "\t%s := bsatn.NewWriter(16)\n", varName)
		fieldDesc := fmt.Sprintf("table %s field %s", table.Name, f.GoName)
		if err := writeDefaultEncoding(w, varName, f.AlgType, *f.Default, fieldDesc, module); err != nil {
			return err
		}
		defaults = append(defaults, defaultEntry{colIndex: f.ColIndex, varName: varName})
	}

	fmt.Fprintf(w, "\tbuilder = builder.AddTable(moduledef.NewTableDefBuilder(%q).\n", table.Name)
	fmt.Fprintf(w, "\t\tWithProductTypeRef(%s).\n", refVar)

	if table.Access == "public" {
		fmt.Fprintf(w, "\t\tWithTableAccess(moduledef.TableAccessPublic).\n")
	} else {
		fmt.Fprintf(w, "\t\tWithTableAccess(moduledef.TableAccessPrivate).\n")
	}

	if table.IsEvent {
		fmt.Fprintf(w, "\t\tWithIsEvent(true).\n")
	}

	// Primary key, indexes, constraints, sequences.
	var pkCols []uint16
	for _, f := range table.Fields {
		if f.PrimaryKey {
			pkCols = append(pkCols, f.ColIndex)
		}
	}
	if len(pkCols) > 0 {
		colStrs := make([]string, len(pkCols))
		for i, c := range pkCols {
			colStrs[i] = fmt.Sprintf("%d", c)
		}
		fmt.Fprintf(w, "\t\tWithPrimaryKey(%s).\n", strings.Join(colStrs, ", "))
	}

	// Add constraints and indexes for PK columns.
	for _, f := range table.Fields {
		if f.PrimaryKey {
			fmt.Fprintf(w, "\t\tWithConstraint(moduledef.NewUniqueConstraint(nil, %d)).\n", f.ColIndex)
			idxName := fmt.Sprintf("%s_%s_idx_btree", table.Name, f.BsatnName)
			fmt.Fprintf(w, "\t\tWithIndex(moduledef.NewBTreeIndexDef(stdbStrPtr(%q), %d).Build()).\n", idxName, f.ColIndex)
		}
		if f.AutoInc {
			fmt.Fprintf(w, "\t\tWithSequence(moduledef.NewSequenceDefBuilder(nil, %d).Build()).\n", f.ColIndex)
		}
		if f.Unique && !f.PrimaryKey {
			fmt.Fprintf(w, "\t\tWithConstraint(moduledef.NewUniqueConstraint(nil, %d)).\n", f.ColIndex)
			idxName := fmt.Sprintf("%s_%s_idx_btree", table.Name, f.BsatnName)
			fmt.Fprintf(w, "\t\tWithIndex(moduledef.NewBTreeIndexDef(stdbStrPtr(%q), %d).Build()).\n", idxName, f.ColIndex)
		}
		if f.IndexBTree && !f.PrimaryKey && !f.Unique {
			idxName := fmt.Sprintf("%s_%s_idx_btree", table.Name, f.BsatnName)
			fmt.Fprintf(w, "\t\tWithIndex(moduledef.NewBTreeIndexDef(stdbStrPtr(%q), %d).Build()).\n", idxName, f.ColIndex)
		}
		if f.IndexDirect {
			idxName := fmt.Sprintf("%s_%s_idx_direct", table.Name, f.BsatnName)
			fmt.Fprintf(w, "\t\tWithIndex(moduledef.NewDirectIndexDef(stdbStrPtr(%q), %d).Build()).\n", idxName, f.ColIndex)
		}
	}

	// Extra multi-column indexes.
	for _, idx := range table.ExtraIndexes {
		colStrs := make([]string, len(idx.Columns))
		for i, c := range idx.Columns {
			colStrs[i] = fmt.Sprintf("%d", c)
		}
		fmt.Fprintf(w, "\t\tWithIndex(moduledef.NewBTreeIndexDef(stdbStrPtr(%q), %s).Build()).\n", idx.Name, strings.Join(colStrs, ", "))
	}

	// Column default values.
	for _, d := range defaults {
		fmt.Fprintf(w, "\t\tWithDefaultValue(moduledef.NewColumnDefaultValue(%d, %s.Bytes())).\n", d.colIndex, d.varName)
	}

	fmt.Fprintf(w, "\t\tBuild())\n\n")
	return nil
}

// writeDefaultEncoding emits bsatn writer calls (on varName) that encode the
// default literal as the BSATN AlgebraicValue for the column's type. The output
// must match what the column codec would produce for that value, since the
// SpacetimeDB migration planner decodes it to backfill existing rows.
func writeDefaultEncoding(w *strings.Builder, varName string, at AlgType, literal, fieldDesc string, module *AnalyzedModule) error {
	// Universal escape hatch: raw:<hex> writes pre-encoded BSATN verbatim.
	if raw, ok := strings.CutPrefix(literal, "raw:"); ok {
		b, err := decodeHexLiteral(raw)
		if err != nil {
			return fmt.Errorf("%s: invalid raw default %q: %w", fieldDesc, literal, err)
		}
		fmt.Fprintf(w, "\t%s.PutBytes(%s)\n", varName, goByteSlice(b))
		return nil
	}

	switch at.Kind {
	case AlgKindBool:
		b, err := strconv.ParseBool(literal)
		if err != nil {
			return fmt.Errorf("%s: invalid bool default %q", fieldDesc, literal)
		}
		fmt.Fprintf(w, "\t%s.PutBool(%v)\n", varName, b)
	case AlgKindU8, AlgKindU16, AlgKindU32, AlgKindU64:
		v, err := strconv.ParseUint(literal, 0, uintBits(at.Kind))
		if err != nil {
			return fmt.Errorf("%s: invalid unsigned-int default %q: %w", fieldDesc, literal, err)
		}
		fmt.Fprintf(w, "\t%s.%s(%d)\n", varName, uintPutMethod(at.Kind), v)
	case AlgKindI8, AlgKindI16, AlgKindI32, AlgKindI64:
		v, err := strconv.ParseInt(literal, 0, intBits(at.Kind))
		if err != nil {
			return fmt.Errorf("%s: invalid signed-int default %q: %w", fieldDesc, literal, err)
		}
		fmt.Fprintf(w, "\t%s.%s(%d)\n", varName, intPutMethod(at.Kind), v)
	case AlgKindF32:
		f, err := strconv.ParseFloat(literal, 32)
		if err != nil {
			return fmt.Errorf("%s: invalid float default %q", fieldDesc, literal)
		}
		fmt.Fprintf(w, "\t%s.PutF32(%s)\n", varName, strconv.FormatFloat(f, 'g', -1, 32))
	case AlgKindF64:
		f, err := strconv.ParseFloat(literal, 64)
		if err != nil {
			return fmt.Errorf("%s: invalid float default %q", fieldDesc, literal)
		}
		fmt.Fprintf(w, "\t%s.PutF64(%s)\n", varName, strconv.FormatFloat(f, 'g', -1, 64))
	case AlgKindString:
		fmt.Fprintf(w, "\t%s.PutString(%q)\n", varName, literal)
	case AlgKindU128, AlgKindU256, AlgKindI128, AlgKindI256:
		width, signed := intWidthBytes(at.Kind)
		b, err := bigIntLEBytes(literal, width, signed)
		if err != nil {
			return fmt.Errorf("%s: invalid 128/256-bit default %q: %w", fieldDesc, literal, err)
		}
		fmt.Fprintf(w, "\t%s.PutBytes(%s)\n", varName, goByteSlice(b))
	case AlgKindBytes:
		// []byte column -> array of u8: length prefix followed by the bytes.
		b, err := decodeHexLiteral(literal)
		if err != nil {
			return fmt.Errorf("%s: invalid []byte default %q (expected hex or empty): %w", fieldDesc, literal, err)
		}
		fmt.Fprintf(w, "\t%s.PutArrayLen(%d)\n", varName, len(b))
		if len(b) > 0 {
			fmt.Fprintf(w, "\t%s.PutBytes(%s)\n", varName, goByteSlice(b))
		}
	case AlgKindArray:
		// Only the empty array is expressible as a literal; use raw: for the rest.
		if literal == "" || literal == "[]" {
			fmt.Fprintf(w, "\t%s.PutArrayLen(0)\n", varName)
		} else {
			return fmt.Errorf("%s: non-empty array defaults are not supported; use default=raw:<hex>", fieldDesc)
		}
	case AlgKindOption:
		// Option<T> is Sum(some: T @tag 0, none: () @tag 1).
		if literal == "null" || literal == "none" {
			fmt.Fprintf(w, "\t%s.PutSumTag(1)\n", varName)
		} else {
			fmt.Fprintf(w, "\t%s.PutSumTag(0)\n", varName)
			if at.ElemType == nil {
				return fmt.Errorf("%s: option default has no element type", fieldDesc)
			}
			if err := writeDefaultEncoding(w, varName, *at.ElemType, literal, fieldDesc, module); err != nil {
				return err
			}
		}
	case AlgKindTimestamp, AlgKindTimeDuration:
		// Both wrap a single I64 micros field; the value encoding is just that I64.
		v, err := strconv.ParseInt(literal, 0, 64)
		if err != nil {
			return fmt.Errorf("%s: invalid timestamp/duration micros default %q", fieldDesc, literal)
		}
		fmt.Fprintf(w, "\t%s.PutI64(%d)\n", varName, v)
	case AlgKindRef:
		t := module.Types[at.TypeName]
		if t == nil || t.Kind != TypeKindSimpleEnum {
			return fmt.Errorf("%s: literal defaults for type %q are not supported; use default=raw:<hex>", fieldDesc, at.TypeName)
		}
		tag, err := resolveEnumTag(t, literal)
		if err != nil {
			return fmt.Errorf("%s: %w", fieldDesc, err)
		}
		fmt.Fprintf(w, "\t%s.PutSumTag(%d)\n", varName, tag)
	default:
		return fmt.Errorf("%s: literal defaults for this type are not supported; use default=raw:<hex>", fieldDesc)
	}
	return nil
}

// uintBits / intBits return the bit width for range-validating an integer literal.
func uintBits(k AlgKind) int {
	switch k {
	case AlgKindU8:
		return 8
	case AlgKindU16:
		return 16
	case AlgKindU32:
		return 32
	default:
		return 64
	}
}

func intBits(k AlgKind) int {
	switch k {
	case AlgKindI8:
		return 8
	case AlgKindI16:
		return 16
	case AlgKindI32:
		return 32
	default:
		return 64
	}
}

func uintPutMethod(k AlgKind) string {
	switch k {
	case AlgKindU8:
		return "PutU8"
	case AlgKindU16:
		return "PutU16"
	case AlgKindU32:
		return "PutU32"
	default:
		return "PutU64"
	}
}

func intPutMethod(k AlgKind) string {
	switch k {
	case AlgKindI8:
		return "PutI8"
	case AlgKindI16:
		return "PutI16"
	case AlgKindI32:
		return "PutI32"
	default:
		return "PutI64"
	}
}

// intWidthBytes returns the byte width and signedness for 128/256-bit integers.
func intWidthBytes(k AlgKind) (width int, signed bool) {
	switch k {
	case AlgKindU128:
		return 16, false
	case AlgKindI128:
		return 16, true
	case AlgKindU256:
		return 32, false
	default: // AlgKindI256
		return 32, true
	}
}

// bigIntLEBytes parses an integer literal (decimal, 0x.., 0o.., 0b..) and returns
// its little-endian two's-complement byte representation in exactly width bytes.
func bigIntLEBytes(s string, width int, signed bool) ([]byte, error) {
	n := new(big.Int)
	if _, ok := n.SetString(strings.TrimSpace(s), 0); !ok {
		return nil, fmt.Errorf("not an integer: %q", s)
	}
	out := make([]byte, width)
	if n.Sign() >= 0 {
		b := n.Bytes() // big-endian, minimal
		if len(b) > width {
			return nil, fmt.Errorf("value out of range for %d bytes", width)
		}
		for i := range b {
			out[i] = b[len(b)-1-i] // big-endian -> little-endian
		}
		return out, nil
	}
	if !signed {
		return nil, fmt.Errorf("negative value for unsigned type")
	}
	// Two's complement over width bytes: 2^(width*8) + n (n is negative).
	mod := new(big.Int).Lsh(big.NewInt(1), uint(width*8))
	tc := new(big.Int).Add(mod, n)
	if tc.Sign() < 0 {
		return nil, fmt.Errorf("value out of range for %d bytes", width)
	}
	b := tc.Bytes()
	if len(b) > width {
		return nil, fmt.Errorf("value out of range for %d bytes", width)
	}
	for i := range b {
		out[i] = b[len(b)-1-i]
	}
	return out, nil
}

// decodeHexLiteral decodes an optional-0x-prefixed hex string; "" yields no bytes.
func decodeHexLiteral(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	if s == "" {
		return nil, nil
	}
	return hex.DecodeString(s)
}

// goByteSlice renders bytes as a Go []byte literal (or nil when empty).
func goByteSlice(b []byte) string {
	if len(b) == 0 {
		return "nil"
	}
	parts := make([]string, len(b))
	for i, by := range b {
		parts[i] = fmt.Sprintf("0x%02x", by)
	}
	return "[]byte{" + strings.Join(parts, ", ") + "}"
}

// resolveEnumTag maps an enum default literal (variant name or numeric index)
// to its sum-type tag.
func resolveEnumTag(t *AnalyzedType, literal string) (int, error) {
	for i, name := range t.EnumVariants {
		if name == literal {
			return i, nil
		}
	}
	if idx, err := strconv.Atoi(literal); err == nil {
		if idx >= 0 && idx < len(t.EnumVariants) {
			return idx, nil
		}
		return 0, fmt.Errorf("enum index %d out of range for %s (0..%d)", idx, t.Name, len(t.EnumVariants)-1)
	}
	return 0, fmt.Errorf("unknown enum variant %q for %s", literal, t.Name)
}

// writeReducerDef writes code to add a ReducerDef.
func writeReducerDef(w *strings.Builder, r *AnalyzedReducer, module *AnalyzedModule) {
	fmt.Fprintf(w, "\tbuilder = builder.AddReducer(moduledef.NewReducerDefBuilder(%q).\n", r.Name)
	fmt.Fprintf(w, "\t\tWithParams(types.NewProductType(\n")
	for _, p := range r.Params {
		fmt.Fprintf(w, "\t\t\ttypes.ProductTypeElement{Name: %q, AlgebraicType: %s},\n", p.Name, algTypeExpr(p.AlgType, module))
	}
	fmt.Fprintf(w, "\t\t)).\n")
	fmt.Fprintf(w, "\t\tWithVisibility(moduledef.FunctionVisibilityClientCallable).\n")
	fmt.Fprintf(w, "\t\tWithErrReturnType(types.AlgTypeString()).\n")
	fmt.Fprintf(w, "\t\tBuild())\n\n")
}

// writeLifecycleReducerDef writes code to add lifecycle reducer defs.
func writeLifecycleReducerDef(w *strings.Builder, lc *AnalyzedLifecycle) {
	lcName := lifecycleName(lc.Kind)

	// Add as regular reducer with private visibility.
	fmt.Fprintf(w, "\tbuilder = builder.AddReducer(moduledef.NewReducerDefBuilder(%q).\n", lcName)
	fmt.Fprintf(w, "\t\tWithParams(types.NewProductType()).\n")
	fmt.Fprintf(w, "\t\tWithVisibility(moduledef.FunctionVisibilityPrivate).\n")
	fmt.Fprintf(w, "\t\tWithErrReturnType(types.AlgTypeString()).\n")
	fmt.Fprintf(w, "\t\tBuild())\n")

	// Add as lifecycle reducer.
	var mdLifecycle string
	switch lc.Kind {
	case "init":
		mdLifecycle = "moduledef.LifecycleInit"
	case "connect":
		mdLifecycle = "moduledef.LifecycleOnConnect"
	case "disconnect":
		mdLifecycle = "moduledef.LifecycleOnDisconnect"
	}
	fmt.Fprintf(w, "\tbuilder = builder.AddLifecycleReducer(moduledef.NewLifecycleReducerDef(%s, %q))\n\n", mdLifecycle, lcName)
}

// writeProcedureDef writes code to add a ProcedureDef.
func writeProcedureDef(w *strings.Builder, p *AnalyzedProcedure, module *AnalyzedModule) {
	fmt.Fprintf(w, "\tbuilder = builder.AddProcedure(moduledef.NewProcedureDefBuilder(%q).\n", p.Name)
	fmt.Fprintf(w, "\t\tWithParams(types.NewProductType(\n")
	for _, param := range p.Params {
		fmt.Fprintf(w, "\t\t\ttypes.ProductTypeElement{Name: %q, AlgebraicType: %s},\n", param.Name, algTypeExpr(param.AlgType, module))
	}
	fmt.Fprintf(w, "\t\t)).\n")

	if p.ReturnType != nil {
		fmt.Fprintf(w, "\t\tWithReturnType(%s).\n", algTypeExpr(*p.ReturnType, module))
	} else {
		fmt.Fprintf(w, "\t\tWithReturnType(types.AlgTypeProduct(types.NewProductType())).\n")
	}
	fmt.Fprintf(w, "\t\tWithVisibility(moduledef.FunctionVisibilityClientCallable).\n")
	fmt.Fprintf(w, "\t\tBuild())\n\n")
}

// writeViewDef writes code to add a ViewDef.
func writeViewDef(w *strings.Builder, v *AnalyzedView, module *AnalyzedModule) {
	fmt.Fprintf(w, "\tbuilder = builder.AddView(moduledef.NewViewDefBuilder(%q).\n", v.Name)
	fmt.Fprintf(w, "\t\tWithIndex(%d).\n", v.ID)
	fmt.Fprintf(w, "\t\tWithIsPublic(%v).\n", v.IsPublic)
	fmt.Fprintf(w, "\t\tWithIsAnonymous(%v).\n", v.IsAnonymous)
	fmt.Fprintf(w, "\t\tWithParams(types.NewProductType(\n")
	for _, param := range v.Params {
		fmt.Fprintf(w, "\t\t\ttypes.ProductTypeElement{Name: %q, AlgebraicType: %s},\n", param.Name, algTypeExpr(param.AlgType, module))
	}
	fmt.Fprintf(w, "\t\t)).\n")
	fmt.Fprintf(w, "\t\tWithReturnType(%s).\n", algTypeExpr(v.ReturnType, module))
	fmt.Fprintf(w, "\t\tBuild())\n\n")
}

// writeScheduleDef writes code to add a ScheduleDef.
func writeScheduleDef(w *strings.Builder, sched *parser.ParsedSchedule, module *AnalyzedModule) {
	// Find the ScheduleAt column index.
	var schedAtCol uint16
	for _, table := range module.Tables {
		if table.Name != sched.TableName {
			continue
		}
		for _, f := range table.Fields {
			if f.AlgType.Kind == AlgKindScheduleAt {
				schedAtCol = f.ColIndex
				break
			}
		}
		break
	}
	fmt.Fprintf(w, "\tbuilder = builder.AddSchedule(moduledef.NewScheduleDef(nil, %q, %d, %q))\n\n", sched.TableName, schedAtCol, sched.FunctionName)
}

// algTypeExpr returns a Go expression for constructing an AlgebraicType.
// These must match the exact SATS representations used by the Rust codegen.
func algTypeExpr(at AlgType, module *AnalyzedModule) string {
	switch at.Kind {
	case AlgKindBool:
		return "types.AlgTypeBool()"
	case AlgKindU8:
		return "types.AlgTypeU8()"
	case AlgKindU16:
		return "types.AlgTypeU16()"
	case AlgKindU32:
		return "types.AlgTypeU32()"
	case AlgKindU64:
		return "types.AlgTypeU64()"
	case AlgKindI8:
		return "types.AlgTypeI8()"
	case AlgKindI16:
		return "types.AlgTypeI16()"
	case AlgKindI32:
		return "types.AlgTypeI32()"
	case AlgKindI64:
		return "types.AlgTypeI64()"
	case AlgKindF32:
		return "types.AlgTypeF32()"
	case AlgKindF64:
		return "types.AlgTypeF64()"
	case AlgKindString:
		return "types.AlgTypeString()"
	case AlgKindU128:
		return "types.AlgTypeU128()"
	case AlgKindU256:
		return "types.AlgTypeU256()"
	case AlgKindI128:
		return "types.AlgTypeI128()"
	case AlgKindI256:
		return "types.AlgTypeI256()"
	case AlgKindIdentity:
		// Identity is Product{__identity__: U256}
		return `types.AlgTypeProduct(types.NewProductType(types.ProductTypeElement{Name: "__identity__", AlgebraicType: types.AlgTypeU256()}))`
	case AlgKindConnectionId:
		// ConnectionId is Product{__connection_id__: U128}
		return `types.AlgTypeProduct(types.NewProductType(types.ProductTypeElement{Name: "__connection_id__", AlgebraicType: types.AlgTypeU128()}))`
	case AlgKindTimestamp:
		// Timestamp is Product{__timestamp_micros_since_unix_epoch__: I64}
		return `types.AlgTypeProduct(types.NewProductType(types.ProductTypeElement{Name: "__timestamp_micros_since_unix_epoch__", AlgebraicType: types.AlgTypeI64()}))`
	case AlgKindTimeDuration:
		// TimeDuration is Product{__time_duration_micros__: I64}
		return `types.AlgTypeProduct(types.NewProductType(types.ProductTypeElement{Name: "__time_duration_micros__", AlgebraicType: types.AlgTypeI64()}))`
	case AlgKindUuid:
		// Uuid is Product{__uuid__: U128}
		return `types.AlgTypeProduct(types.NewProductType(types.ProductTypeElement{Name: "__uuid__", AlgebraicType: types.AlgTypeU128()}))`
	case AlgKindScheduleAt:
		// ScheduleAt is Sum(Interval: TimeDuration, Time: Timestamp)
		return `types.AlgTypeSum(types.NewSumType(
			types.SumTypeVariant{Name: "Interval", AlgebraicType: types.AlgTypeProduct(types.NewProductType(types.ProductTypeElement{Name: "__time_duration_micros__", AlgebraicType: types.AlgTypeI64()}))},
			types.SumTypeVariant{Name: "Time", AlgebraicType: types.AlgTypeProduct(types.NewProductType(types.ProductTypeElement{Name: "__timestamp_micros_since_unix_epoch__", AlgebraicType: types.AlgTypeI64()}))},
		))`
	case AlgKindBytes:
		return "types.AlgTypeArray(types.AlgTypeU8())"
	case AlgKindArray:
		if at.ElemType != nil {
			return "types.AlgTypeArray(" + algTypeExpr(*at.ElemType, module) + ")"
		}
		return "types.AlgTypeArray(types.AlgTypeU8())"
	case AlgKindOption:
		// Option<T> is Sum(some: T, none: Product())
		if at.ElemType != nil {
			return fmt.Sprintf("types.AlgTypeSum(types.NewSumType(\n\t\t\ttypes.SumTypeVariant{Name: \"some\", AlgebraicType: %s},\n\t\t\ttypes.SumTypeVariant{Name: \"none\", AlgebraicType: types.AlgTypeProduct(types.NewProductType())},\n\t\t))", algTypeExpr(*at.ElemType, module))
		}
		return "types.AlgTypeSum(types.NewSumType(types.SumTypeVariant{Name: \"some\", AlgebraicType: types.AlgTypeU8()}, types.SumTypeVariant{Name: \"none\", AlgebraicType: types.AlgTypeProduct(types.NewProductType())}))"
	case AlgKindRef:
		return fmt.Sprintf("types.AlgTypeRef(ref%s)", at.TypeName)
	default:
		return "types.AlgTypeU8() /* TODO */"
	}
}
