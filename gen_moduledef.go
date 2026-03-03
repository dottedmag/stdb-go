package main

import (
	"fmt"
	"strings"
)

// generateModuleDef generates the stdbDescribeModule function that builds the
// module definition statically (no reflection).
func generateModuleDef(module *AnalyzedModule, w *strings.Builder) {
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
		writeTableDef(w, &table, module)
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
func writeTableDef(w *strings.Builder, table *AnalyzedTable, module *AnalyzedModule) {
	refVar := "ref" + table.StructName

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

	fmt.Fprintf(w, "\t\tBuild())\n\n")
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
func writeScheduleDef(w *strings.Builder, sched *ParsedSchedule, module *AnalyzedModule) {
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
