package clientgen

import (
	"fmt"
	"strings"
)

// generateModule generates the top-level module bindings struct.
func generateModule(schema *ModuleSchema, pkgName string) ([]byte, error) {
	if len(schema.Tables) == 0 && len(schema.Views) == 0 {
		return nil, nil
	}

	var w strings.Builder
	w.WriteString(clientFileHeader(pkgName))

	imports := map[string]string{
		"client": "go.digitalxero.dev/spacetimedb-client/client",
		"cache":  "go.digitalxero.dev/spacetimedb-client/client/cache",
	}

	var moduleCode strings.Builder

	namedRefs := map[int]string{}
	for _, t := range schema.Types {
		namedRefs[t.TypeRef] = t.Name
	}

	// ModuleBindings struct
	fmt.Fprintf(&moduleCode, "// ModuleBindings ties together all table and view caches for the module.\n")
	fmt.Fprintf(&moduleCode, "type ModuleBindings struct {\n")
	fmt.Fprintf(&moduleCode, "\tconn client.DbConnection\n")

	for _, table := range schema.Tables {
		fieldName := toGoName(table.Name)
		typeName := fieldName
		if tn, ok := namedRefs[table.TypeRef]; ok {
			typeName = toGoName(tn)
		}
		fmt.Fprintf(&moduleCode, "\t%s *%sTable\n", fieldName, typeName)
	}

	for _, view := range schema.Views {
		goName := toGoName(view.Name)
		fmt.Fprintf(&moduleCode, "\t%s *%sView\n", goName, goName)
	}

	fmt.Fprintf(&moduleCode, "}\n\n")

	// NewModuleBindings constructor
	fmt.Fprintf(&moduleCode, "// NewModuleBindings registers all tables and views with the connection and\n")
	fmt.Fprintf(&moduleCode, "// returns a ModuleBindings with typed cache handles.\n")
	fmt.Fprintf(&moduleCode, "func NewModuleBindings(conn client.DbConnection) *ModuleBindings {\n")
	fmt.Fprintf(&moduleCode, "\tc := conn.Cache()\n")
	fmt.Fprintf(&moduleCode, "\tm := &ModuleBindings{conn: conn}\n")

	for _, table := range schema.Tables {
		fieldName := toGoName(table.Name)
		typeName := fieldName
		if tn, ok := namedRefs[table.TypeRef]; ok {
			typeName = toGoName(tn)
		}
		defName := toLowerCamel(table.Name) + "TableDef"

		// Determine if table has a single PK for RegisterTypedTableWithPK
		hasSinglePK := len(table.PrimaryKey) == 1 && table.ProductType != nil && table.PrimaryKey[0] < len(table.ProductType.Elements)

		if hasSinglePK {
			pkIdx := table.PrimaryKey[0]
			pkElem := table.ProductType.Elements[pkIdx]
			pkGoType := goTypeForAlgebraic(&pkElem.AlgebraicType, schema.Typespace, schema.Types)
			if needsTypesImport(pkGoType) {
				imports["types"] = "go.digitalxero.dev/spacetimedb-client/types"
			}
			fmt.Fprintf(&moduleCode, "\tm.%s = cache.RegisterTypedTableWithPK[*%s, %s](c, %s{})\n",
				fieldName, typeName, pkGoType, defName)
		} else {
			fmt.Fprintf(&moduleCode, "\tm.%s = cache.RegisterTypedTable[*%s](c, %s{})\n",
				fieldName, typeName, defName)
		}
	}

	for _, view := range schema.Views {
		goName := toGoName(view.Name)
		defName := toLowerCamel(view.Name) + "ViewDef"
		fmt.Fprintf(&moduleCode, "\tm.%s = cache.RegisterTypedTable[*%s](c, %s{})\n",
			goName, goName, defName)
	}

	fmt.Fprintf(&moduleCode, "\treturn m\n")
	fmt.Fprintf(&moduleCode, "}\n\n")

	// Conn() accessor
	fmt.Fprintf(&moduleCode, "// Conn returns the underlying database connection.\n")
	fmt.Fprintf(&moduleCode, "func (m *ModuleBindings) Conn() client.DbConnection {\n")
	fmt.Fprintf(&moduleCode, "\treturn m.conn\n")
	fmt.Fprintf(&moduleCode, "}\n")

	w.WriteString(clientImportBlock(imports))
	w.WriteString(moduleCode.String())

	return []byte(w.String()), nil
}
