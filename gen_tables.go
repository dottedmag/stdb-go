package main

import (
	"fmt"
	"strings"
)

// generateTables generates table accessor types and their methods.
func generateTables(module *AnalyzedModule, w *strings.Builder) {
	for _, table := range module.Tables {
		generateTableType(module, &table, w)
	}
}

// generateTableType generates a single table accessor type with all its methods.
func generateTableType(module *AnalyzedModule, table *AnalyzedTable, w *strings.Builder) {
	typeName := "stdb" + toPascalCase(table.Name) + "TableHandle"
	structName := table.StructName

	// Type definition.
	fmt.Fprintf(w, "type %s struct {\n", typeName)
	fmt.Fprintf(w, "\ttableId  uint32\n")
	fmt.Fprintf(w, "\tresolved bool\n")
	fmt.Fprintf(w, "}\n\n")

	// Global variable.
	fmt.Fprintf(w, "var %s = &%s{}\n\n", table.VarName, typeName)

	// resolve method.
	fmt.Fprintf(w, "func (t *%s) resolve() {\n", typeName)
	fmt.Fprintf(w, "\tif t.resolved { return }\n")
	fmt.Fprintf(w, "\tid, err := runtime.GetTableId(%q)\n", table.Name)
	fmt.Fprintf(w, "\tif err != nil { panic(fmt.Sprintf(\"%s: resolve table: %%v\", err)) }\n", table.VarName)
	fmt.Fprintf(w, "\tt.tableId = id\n")
	fmt.Fprintf(w, "\tt.resolved = true\n")
	fmt.Fprintf(w, "}\n\n")

	// Insert method.
	generateInsert(table, typeName, structName, w)

	// Delete method.
	generateDelete(table, typeName, structName, w)

	// Scan method.
	generateScan(table, typeName, structName, w)

	// Count method.
	generateCount(table, typeName, structName, w)

	// Generate index-based methods for fields with indexes.
	for _, f := range table.Fields {
		if f.PrimaryKey || f.Unique {
			idxName := fmt.Sprintf("%s_%s_idx_btree", table.Name, f.BsatnName)
			generateFindBy(module, table, typeName, structName, f, idxName, w)
			generateUpdateBy(table, typeName, structName, f, idxName, w)
			generateDeleteByIndex(module, table, typeName, structName, f, idxName, w)
		}
		if f.IndexBTree && !f.PrimaryKey && !f.Unique {
			idxName := fmt.Sprintf("%s_%s_idx_btree", table.Name, f.BsatnName)
			generateFilterBy(module, table, typeName, structName, f, idxName, w)
			generateDeleteByIndex(module, table, typeName, structName, f, idxName, w)
		}
	}

	// Collect already-generated FilterBy method names to avoid duplicates.
	existingFilterMethods := make(map[string]bool)
	for _, f := range table.Fields {
		if f.PrimaryKey || f.Unique {
			existingFilterMethods["FindBy"+f.GoName] = true
		}
		if f.IndexBTree && !f.PrimaryKey && !f.Unique {
			existingFilterMethods["FilterBy"+f.GoName] = true
		}
	}

	// FilterByMultiColumn for extra indexes (full + partial prefix methods).
	for _, idx := range table.ExtraIndexes {
		generateFilterByMultiColumn(module, table, typeName, structName, idx, w)
		// Generate partial prefix methods for subsets of columns (1..N-1).
		for prefixLen := 1; prefixLen < len(idx.Columns); prefixLen++ {
			partialIdx := ParsedMultiColIndex{
				Name:    idx.Name,
				Columns: idx.Columns[:prefixLen],
			}
			// Build the method name to check for conflicts.
			var colNames []string
			for _, colIdx := range partialIdx.Columns {
				for _, f := range table.Fields {
					if f.ColIndex == colIdx {
						colNames = append(colNames, f.GoName)
						break
					}
				}
			}
			methodName := "FilterBy" + strings.Join(colNames, "And")
			if existingFilterMethods[methodName] {
				continue // Skip — single-column index already generated this method.
			}
			generateFilterByMultiColumn(module, table, typeName, structName, partialIdx, w)
		}
	}
}

// generateInsert generates the Insert method.
func generateInsert(table *AnalyzedTable, typeName, structName string, w *strings.Builder) {
	fmt.Fprintf(w, "func (t *%s) Insert(row %s) %s {\n", typeName, structName, structName)
	fmt.Fprintf(w, "\tt.resolve()\n")
	fmt.Fprintf(w, "\truntime.GlobalWriter.Reset()\n")
	fmt.Fprintf(w, "\tstdbWrite%s(runtime.GlobalWriter, &row)\n", structName)
	fmt.Fprintf(w, "\tseqBytes, err := sys.DatastoreInsertBSATN(t.tableId, runtime.GlobalWriter.Bytes())\n")
	fmt.Fprintf(w, "\tif err != nil { panic(fmt.Sprintf(\"%s.Insert: %%v\", err)) }\n", table.VarName)

	// Decode auto-increment values.
	autoIncFields := findAutoIncFields(table)
	if len(autoIncFields) > 0 {
		fmt.Fprintf(w, "\tif len(seqBytes) > 0 {\n")
		fmt.Fprintf(w, "\t\tseqR := bsatn.NewZeroCopyReader(seqBytes)\n")
		for _, f := range autoIncFields {
			writeSeqDecode(w, "\t\t", f)
		}
		fmt.Fprintf(w, "\t}\n")
	} else {
		fmt.Fprintf(w, "\t_ = seqBytes\n")
	}

	fmt.Fprintf(w, "\treturn row\n")
	fmt.Fprintf(w, "}\n\n")
}

// generateDelete generates the Delete method.
func generateDelete(table *AnalyzedTable, typeName, structName string, w *strings.Builder) {
	fmt.Fprintf(w, "func (t *%s) Delete(row %s) {\n", typeName, structName)
	fmt.Fprintf(w, "\tt.resolve()\n")
	fmt.Fprintf(w, "\truntime.GlobalWriter.Reset()\n")
	fmt.Fprintf(w, "\truntime.GlobalWriter.PutArrayLen(1)\n")
	fmt.Fprintf(w, "\tstdbWrite%s(runtime.GlobalWriter, &row)\n", structName)
	fmt.Fprintf(w, "\tif _, err := sys.DatastoreDeleteAllByEqBSATN(t.tableId, runtime.GlobalWriter.Bytes()); err != nil {\n")
	fmt.Fprintf(w, "\t\tpanic(fmt.Sprintf(\"%s.Delete: %%v\", err))\n", table.VarName)
	fmt.Fprintf(w, "\t}\n")
	fmt.Fprintf(w, "}\n\n")
}

// generateScan generates the Scan method.
func generateScan(table *AnalyzedTable, typeName, structName string, w *strings.Builder) {
	fmt.Fprintf(w, "func (t *%s) Scan() (runtime.TableIterator[%s], error) {\n", typeName, structName)
	fmt.Fprintf(w, "\tt.resolve()\n")
	fmt.Fprintf(w, "\titer, err := sys.DatastoreTableScanBSATN(t.tableId)\n")
	fmt.Fprintf(w, "\tif err != nil { return nil, err }\n")
	fmt.Fprintf(w, "\treturn runtime.NewTableIterator[%s](iter, func(r bsatn.Reader, v *%s) error {\n", structName, structName)
	fmt.Fprintf(w, "\t\treturn stdbRead%s(r, v)\n", structName)
	fmt.Fprintf(w, "\t}), nil\n")
	fmt.Fprintf(w, "}\n\n")
}

// generateCount generates the Count method.
func generateCount(table *AnalyzedTable, typeName, structName string, w *strings.Builder) {
	fmt.Fprintf(w, "func (t *%s) Count() (uint64, error) {\n", typeName)
	fmt.Fprintf(w, "\tt.resolve()\n")
	fmt.Fprintf(w, "\treturn sys.DatastoreTableRowCount(t.tableId)\n")
	fmt.Fprintf(w, "}\n\n")
}

// generateFindBy generates a FindBy method for a unique/PK index.
func generateFindBy(module *AnalyzedModule, table *AnalyzedTable, typeName, structName string, field AnalyzedField, idxName string, w *strings.Builder) {
	methodName := "FindBy" + field.GoName
	keyType := field.GoType

	fmt.Fprintf(w, "func (t *%s) %s(key %s) (%s, bool, error) {\n", typeName, methodName, keyType, structName)
	fmt.Fprintf(w, "\tt.resolve()\n")
	fmt.Fprintf(w, "\tindexId, err := runtime.GetIndexId(%q)\n", idxName)
	fmt.Fprintf(w, "\tif err != nil { var zero %s; return zero, false, err }\n", structName)
	fmt.Fprintf(w, "\truntime.GlobalWriter.Reset()\n")
	writeKeyEncode(w, "\t", field.AlgType, "key", module)
	fmt.Fprintf(w, "\titer, err := sys.DatastoreIndexScanPointBSATN(indexId, runtime.GlobalWriter.Bytes())\n")
	fmt.Fprintf(w, "\tif err != nil { var zero %s; return zero, false, err }\n", structName)
	fmt.Fprintf(w, "\tdefer iter.Close()\n")
	fmt.Fprintf(w, "\tdata, ok, err := iter.Next()\n")
	fmt.Fprintf(w, "\tif !ok || err != nil { var zero %s; return zero, false, err }\n", structName)
	fmt.Fprintf(w, "\tvar result %s\n", structName)
	fmt.Fprintf(w, "\tr := bsatn.NewZeroCopyReader(data)\n")
	fmt.Fprintf(w, "\tif err := stdbRead%s(r, &result); err != nil { var zero %s; return zero, false, err }\n", structName, structName)
	fmt.Fprintf(w, "\treturn result, true, nil\n")
	fmt.Fprintf(w, "}\n\n")
}

// generateFilterBy generates a FilterBy method for a btree index.
func generateFilterBy(module *AnalyzedModule, table *AnalyzedTable, typeName, structName string, field AnalyzedField, idxName string, w *strings.Builder) {
	methodName := "FilterBy" + field.GoName
	keyType := field.GoType

	fmt.Fprintf(w, "func (t *%s) %s(key %s) (runtime.TableIterator[%s], error) {\n", typeName, methodName, keyType, structName)
	fmt.Fprintf(w, "\tt.resolve()\n")
	fmt.Fprintf(w, "\tindexId, err := runtime.GetIndexId(%q)\n", idxName)
	fmt.Fprintf(w, "\tif err != nil { return nil, err }\n")
	fmt.Fprintf(w, "\truntime.GlobalWriter.Reset()\n")
	writeKeyEncode(w, "\t", field.AlgType, "key", module)
	fmt.Fprintf(w, "\tkeyBytes := make([]byte, len(runtime.GlobalWriter.Bytes()))\n")
	fmt.Fprintf(w, "\tcopy(keyBytes, runtime.GlobalWriter.Bytes())\n")
	fmt.Fprintf(w, "\titer, err := sys.DatastoreIndexScanPointBSATN(indexId, keyBytes)\n")
	fmt.Fprintf(w, "\tif err != nil { return nil, err }\n")
	fmt.Fprintf(w, "\treturn runtime.NewTableIterator[%s](iter, func(r bsatn.Reader, v *%s) error {\n", structName, structName)
	fmt.Fprintf(w, "\t\treturn stdbRead%s(r, v)\n", structName)
	fmt.Fprintf(w, "\t}), nil\n")
	fmt.Fprintf(w, "}\n\n")
}

// generateUpdateBy generates an UpdateBy method for a unique/PK index.
func generateUpdateBy(table *AnalyzedTable, typeName, structName string, field AnalyzedField, idxName string, w *strings.Builder) {
	methodName := "UpdateBy" + field.GoName

	fmt.Fprintf(w, "func (t *%s) %s(row %s) %s {\n", typeName, methodName, structName, structName)
	fmt.Fprintf(w, "\tt.resolve()\n")
	fmt.Fprintf(w, "\tindexId, err := runtime.GetIndexId(%q)\n", idxName)
	fmt.Fprintf(w, "\tif err != nil { panic(fmt.Sprintf(\"%s.%s: %%v\", err)) }\n", table.VarName, methodName)
	fmt.Fprintf(w, "\truntime.GlobalWriter.Reset()\n")
	fmt.Fprintf(w, "\tstdbWrite%s(runtime.GlobalWriter, &row)\n", structName)
	fmt.Fprintf(w, "\tseqBytes, err := sys.DatastoreUpdateBSATN(t.tableId, indexId, runtime.GlobalWriter.Bytes())\n")
	fmt.Fprintf(w, "\tif err != nil { panic(fmt.Sprintf(\"%s.%s: %%v\", err)) }\n", table.VarName, methodName)

	autoIncFields := findAutoIncFields(table)
	if len(autoIncFields) > 0 {
		fmt.Fprintf(w, "\tif len(seqBytes) > 0 {\n")
		fmt.Fprintf(w, "\t\tseqR := bsatn.NewZeroCopyReader(seqBytes)\n")
		for _, f := range autoIncFields {
			writeSeqDecode(w, "\t\t", f)
		}
		fmt.Fprintf(w, "\t}\n")
	} else {
		fmt.Fprintf(w, "\t_ = seqBytes\n")
	}

	fmt.Fprintf(w, "\treturn row\n")
	fmt.Fprintf(w, "}\n\n")
}

// generateDeleteByIndex generates a DeleteBy method for an index.
func generateDeleteByIndex(module *AnalyzedModule, table *AnalyzedTable, typeName, structName string, field AnalyzedField, idxName string, w *strings.Builder) {
	methodName := "DeleteBy" + field.GoName
	keyType := field.GoType

	fmt.Fprintf(w, "func (t *%s) %s(key %s) uint32 {\n", typeName, methodName, keyType)
	fmt.Fprintf(w, "\tt.resolve()\n")
	fmt.Fprintf(w, "\tindexId, err := runtime.GetIndexId(%q)\n", idxName)
	fmt.Fprintf(w, "\tif err != nil { panic(fmt.Sprintf(\"%s.%s: %%v\", err)) }\n", table.VarName, methodName)
	fmt.Fprintf(w, "\truntime.GlobalWriter.Reset()\n")
	writeKeyEncode(w, "\t", field.AlgType, "key", module)
	fmt.Fprintf(w, "\tdeleted, err := sys.DatastoreDeleteByIndexScanPointBSATN(indexId, runtime.GlobalWriter.Bytes())\n")
	fmt.Fprintf(w, "\tif err != nil { panic(fmt.Sprintf(\"%s.%s: %%v\", err)) }\n", table.VarName, methodName)
	fmt.Fprintf(w, "\treturn deleted\n")
	fmt.Fprintf(w, "}\n\n")
}

// generateFilterByMultiColumn generates a FilterByMultiColumn method.
func generateFilterByMultiColumn(module *AnalyzedModule, table *AnalyzedTable, typeName, structName string, idx ParsedMultiColIndex, w *strings.Builder) {
	// Build method name from column names.
	var colNames []string
	for _, colIdx := range idx.Columns {
		for _, f := range table.Fields {
			if f.ColIndex == colIdx {
				colNames = append(colNames, f.GoName)
				break
			}
		}
	}
	methodName := "FilterBy" + strings.Join(colNames, "And")

	// Build parameter list.
	var params []string
	var paramFields []AnalyzedField
	for _, colIdx := range idx.Columns {
		for _, f := range table.Fields {
			if f.ColIndex == colIdx {
				params = append(params, fmt.Sprintf("%s %s", toParamName(f.GoName), f.GoType))
				paramFields = append(paramFields, f)
				break
			}
		}
	}

	fmt.Fprintf(w, "func (t *%s) %s(%s) (runtime.TableIterator[%s], error) {\n",
		typeName, methodName, strings.Join(params, ", "), structName)
	fmt.Fprintf(w, "\tt.resolve()\n")
	fmt.Fprintf(w, "\tindexId, err := runtime.GetIndexId(%q)\n", idx.Name)
	fmt.Fprintf(w, "\tif err != nil { return nil, err }\n")

	if len(paramFields) == 1 {
		// Single column: use point scan.
		fmt.Fprintf(w, "\truntime.GlobalWriter.Reset()\n")
		writeKeyEncode(w, "\t", paramFields[0].AlgType, toParamName(paramFields[0].GoName), module)
		fmt.Fprintf(w, "\tkeyBytes := make([]byte, len(runtime.GlobalWriter.Bytes()))\n")
		fmt.Fprintf(w, "\tcopy(keyBytes, runtime.GlobalWriter.Bytes())\n")
		fmt.Fprintf(w, "\titer, err := sys.DatastoreIndexScanPointBSATN(indexId, keyBytes)\n")
	} else {
		// Multi-column: encode prefix (N-1) + range bound (last).
		fmt.Fprintf(w, "\tprefixWriter := bsatn.NewWriter(64)\n")
		for _, f := range paramFields[:len(paramFields)-1] {
			writeKeyEncodeOnWriter(w, "\t", f.AlgType, toParamName(f.GoName), "prefixWriter", module)
		}
		fmt.Fprintf(w, "\tprefixBytes := prefixWriter.Bytes()\n")
		fmt.Fprintf(w, "\tnumPrefixCols := uint32(%d)\n", len(paramFields)-1)

		lastField := paramFields[len(paramFields)-1]
		fmt.Fprintf(w, "\tboundWriter := bsatn.NewWriter(16)\n")
		fmt.Fprintf(w, "\tboundWriter.PutU8(0) // Bound::Included\n")
		writeKeyEncodeOnWriter(w, "\t", lastField.AlgType, toParamName(lastField.GoName), "boundWriter", module)
		fmt.Fprintf(w, "\tboundBytes := boundWriter.Bytes()\n")

		fmt.Fprintf(w, "\titer, err := sys.DatastoreIndexScanRangeBSATN(indexId, prefixBytes, numPrefixCols, boundBytes, boundBytes)\n")
	}

	fmt.Fprintf(w, "\tif err != nil { return nil, err }\n")
	fmt.Fprintf(w, "\treturn runtime.NewTableIterator[%s](iter, func(r bsatn.Reader, v *%s) error {\n", structName, structName)
	fmt.Fprintf(w, "\t\treturn stdbRead%s(r, v)\n", structName)
	fmt.Fprintf(w, "\t}), nil\n")
	fmt.Fprintf(w, "}\n\n")
}

// writeKeyEncode writes code to encode a key value into GlobalWriter.
func writeKeyEncode(w *strings.Builder, indent string, algType AlgType, expr string, module *AnalyzedModule) {
	writeKeyEncodeOnWriter(w, indent, algType, expr, "runtime.GlobalWriter", module)
}

// writeKeyEncodeOnWriter writes code to encode a key value into a named writer.
func writeKeyEncodeOnWriter(w *strings.Builder, indent string, algType AlgType, expr string, writerName string, module *AnalyzedModule) {
	switch algType.Kind {
	case AlgKindBool:
		fmt.Fprintf(w, "%s%s.PutBool(%s)\n", indent, writerName, expr)
	case AlgKindU8:
		fmt.Fprintf(w, "%s%s.PutU8(%s)\n", indent, writerName, expr)
	case AlgKindU16:
		fmt.Fprintf(w, "%s%s.PutU16(%s)\n", indent, writerName, expr)
	case AlgKindU32:
		fmt.Fprintf(w, "%s%s.PutU32(%s)\n", indent, writerName, expr)
	case AlgKindU64:
		fmt.Fprintf(w, "%s%s.PutU64(%s)\n", indent, writerName, expr)
	case AlgKindI8:
		fmt.Fprintf(w, "%s%s.PutI8(%s)\n", indent, writerName, expr)
	case AlgKindI16:
		fmt.Fprintf(w, "%s%s.PutI16(%s)\n", indent, writerName, expr)
	case AlgKindI32:
		fmt.Fprintf(w, "%s%s.PutI32(%s)\n", indent, writerName, expr)
	case AlgKindI64:
		fmt.Fprintf(w, "%s%s.PutI64(%s)\n", indent, writerName, expr)
	case AlgKindF32:
		fmt.Fprintf(w, "%s%s.PutF32(%s)\n", indent, writerName, expr)
	case AlgKindF64:
		fmt.Fprintf(w, "%s%s.PutF64(%s)\n", indent, writerName, expr)
	case AlgKindString:
		fmt.Fprintf(w, "%s%s.PutString(%s)\n", indent, writerName, expr)
	case AlgKindIdentity:
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tb := %s.Bytes()\n", indent, expr)
		fmt.Fprintf(w, "%s\t%s.PutBytes(b[:])\n", indent, writerName)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindConnectionId:
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tb := %s.Bytes()\n", indent, expr)
		fmt.Fprintf(w, "%s\t%s.PutBytes(b[:])\n", indent, writerName)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindUuid:
		fmt.Fprintf(w, "%s{\n", indent)
		fmt.Fprintf(w, "%s\tb := %s.Bytes()\n", indent, expr)
		fmt.Fprintf(w, "%s\t%s.PutBytes(b[:])\n", indent, writerName)
		fmt.Fprintf(w, "%s}\n", indent)
	case AlgKindRef:
		// Resolve the referenced type to determine encoding.
		if module != nil {
			if t, ok := module.Types[algType.TypeName]; ok {
				switch t.Kind {
				case TypeKindSimpleEnum:
					// Simple enums are uint8 underneath — cast to uint8 for encoding.
					fmt.Fprintf(w, "%s%s.PutU8(uint8(%s))\n", indent, writerName, expr)
					return
				case TypeKindStruct:
					// Struct keys: use the generated BSATN writer.
					fmt.Fprintf(w, "%sstdbWrite%s(%s, &%s)\n", indent, algType.TypeName, writerName, expr)
					return
				}
			}
		}
		// Fall back to runtime encoder.
		fmt.Fprintf(w, "%sruntime.EncodeKeyInto(%s, %s)\n", indent, writerName, expr)
	default:
		// For complex types, fall back to the runtime encoder.
		fmt.Fprintf(w, "%sruntime.EncodeKeyInto(%s, %s)\n", indent, writerName, expr)
	}
}

// findAutoIncFields returns fields with AutoInc set.
func findAutoIncFields(table *AnalyzedTable) []AnalyzedField {
	var fields []AnalyzedField
	for _, f := range table.Fields {
		if f.AutoInc {
			fields = append(fields, f)
		}
	}
	return fields
}

// writeSeqDecode writes code to decode an auto-increment field from seqBytes.
func writeSeqDecode(w *strings.Builder, indent string, f AnalyzedField) {
	switch f.AlgType.Kind {
	case AlgKindU8:
		fmt.Fprintf(w, "%s{ v, _ := seqR.GetU8(); row.%s = v }\n", indent, f.GoName)
	case AlgKindU16:
		fmt.Fprintf(w, "%s{ v, _ := seqR.GetU16(); row.%s = v }\n", indent, f.GoName)
	case AlgKindU32:
		fmt.Fprintf(w, "%s{ v, _ := seqR.GetU32(); row.%s = v }\n", indent, f.GoName)
	case AlgKindU64:
		fmt.Fprintf(w, "%s{ v, _ := seqR.GetU64(); row.%s = v }\n", indent, f.GoName)
	case AlgKindI8:
		fmt.Fprintf(w, "%s{ v, _ := seqR.GetI8(); row.%s = v }\n", indent, f.GoName)
	case AlgKindI16:
		fmt.Fprintf(w, "%s{ v, _ := seqR.GetI16(); row.%s = v }\n", indent, f.GoName)
	case AlgKindI32:
		fmt.Fprintf(w, "%s{ v, _ := seqR.GetI32(); row.%s = v }\n", indent, f.GoName)
	case AlgKindI64:
		fmt.Fprintf(w, "%s{ v, _ := seqR.GetI64(); row.%s = v }\n", indent, f.GoName)
	default:
		fmt.Fprintf(w, "%s// TODO: auto-inc decode for %s (%v)\n", indent, f.GoName, f.AlgType.Kind)
	}
}

// toParamName converts a Go field name to a parameter name (lowercased first char).
func toParamName(name string) string {
	if len(name) == 0 {
		return name
	}
	runes := []rune(name)
	runes[0] = []rune(strings.ToLower(string(runes[0])))[0]
	return string(runes)
}
