package clientgen

import (
	"fmt"
	"strings"
)

// generateTables generates table definition types and registration code.
func generateTables(schema *ModuleSchema, pkgName string) ([]byte, error) {
	if len(schema.Tables) == 0 {
		return nil, nil
	}

	var w strings.Builder
	w.WriteString(clientFileHeader(pkgName))

	imports := map[string]string{
		"bsatn": "go.digitalxero.dev/spacetimedb-client/bsatn",
		"cache": "go.digitalxero.dev/spacetimedb-client/client/cache",
	}

	var tableCode strings.Builder

	for _, table := range schema.Tables {
		if table.ProductType == nil {
			continue
		}

		goName := toGoName(table.Name)
		defName := toLowerCamel(table.Name) + "TableDef"

		// Table definition struct
		fmt.Fprintf(&tableCode, "// %s implements cache.TypedTableDef for the %s table.\n", defName, table.Name)
		fmt.Fprintf(&tableCode, "type %s struct{}\n\n", defName)

		// TableName
		fmt.Fprintf(&tableCode, "func (%s) TableName() string { return %q }\n\n", defName, table.Name)

		// DecodeRow
		fmt.Fprintf(&tableCode, "func (%s) DecodeRow(r bsatn.Reader) (*%s, error) {\n", defName, goName)
		fmt.Fprintf(&tableCode, "\treturn Read%s(r)\n", goName)
		fmt.Fprintf(&tableCode, "}\n\n")

		// EncodeRow
		fmt.Fprintf(&tableCode, "func (%s) EncodeRow(row *%s) []byte {\n", defName, goName)
		fmt.Fprintf(&tableCode, "\tw := bsatn.NewWriter(64)\n")
		fmt.Fprintf(&tableCode, "\trow.WriteBsatn(w)\n")
		fmt.Fprintf(&tableCode, "\treturn w.Bytes()\n")
		fmt.Fprintf(&tableCode, "}\n\n")

		// PrimaryKey (if table has one)
		if len(table.PrimaryKey) > 0 && table.ProductType != nil && len(table.PrimaryKey) == 1 {
			pkIdx := table.PrimaryKey[0]
			if pkIdx < len(table.ProductType.Elements) {
				pkElem := table.ProductType.Elements[pkIdx]
				pkFieldName := toGoName(pkElem.Name)
				pkGoType := goTypeForAlgebraic(&pkElem.AlgebraicType, schema.Typespace, schema.Types)

				if needsTypesImport(pkGoType) {
					imports["types"] = "go.digitalxero.dev/spacetimedb-client/types"
				}

				fmt.Fprintf(&tableCode, "func (%s) PrimaryKey(row *%s) %s {\n", defName, goName, pkGoType)
				fmt.Fprintf(&tableCode, "\treturn row.%s\n", pkFieldName)
				fmt.Fprintf(&tableCode, "}\n\n")
			}
		}

		// Type alias for typed table cache
		fmt.Fprintf(&tableCode, "// %sTable is a type-safe table cache for %s rows.\n", goName, table.Name)
		fmt.Fprintf(&tableCode, "type %sTable = cache.TypedTableCache[*%s]\n\n", goName, goName)
	}

	w.WriteString(clientImportBlock(imports))
	w.WriteString(tableCode.String())

	return []byte(w.String()), nil
}
