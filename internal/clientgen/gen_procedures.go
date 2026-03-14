package clientgen

import (
	"fmt"
	"strings"
)

// generateProcedures generates procedure caller functions and args structs.
func generateProcedures(schema *ModuleSchema, pkgName string) ([]byte, error) {
	if len(schema.Procedures) == 0 {
		return nil, nil
	}

	var w strings.Builder
	w.WriteString(clientFileHeader(pkgName))

	imports := map[string]string{
		"bsatn":  "go.digitalxero.dev/spacetimedb-client/bsatn",
		"client": "go.digitalxero.dev/spacetimedb-client/client",
	}

	var procCode strings.Builder

	for _, proc := range schema.Procedures {
		goName := toGoName(proc.Name)

		if len(proc.Params) > 0 {
			// Args struct
			argsName := toLowerCamel(proc.Name) + "Args"
			fmt.Fprintf(&procCode, "type %s struct {\n", argsName)
			for _, param := range proc.Params {
				fieldName := toGoName(param.Name)
				goType := goTypeForAlgebraic(param.Type, schema.Typespace, schema.Types)
				if needsTypesImport(goType) {
					imports["types"] = "go.digitalxero.dev/spacetimedb-client/types"
				}
				fmt.Fprintf(&procCode, "\t%s %s\n", fieldName, goType)
			}
			fmt.Fprintf(&procCode, "}\n\n")

			// WriteBsatn for args
			fmt.Fprintf(&procCode, "func (a *%s) WriteBsatn(w bsatn.Writer) {\n", argsName)
			for _, param := range proc.Params {
				fieldName := toGoName(param.Name)
				writeFieldEncoder(&procCode, "a."+fieldName, param.Type, schema, imports, "\t")
			}
			fmt.Fprintf(&procCode, "}\n\n")
		}

		// Caller function
		fmt.Fprintf(&procCode, "// Call%s calls the %s procedure on the server.\n", goName, proc.Name)

		var paramList []string
		paramList = append(paramList, "conn client.DbConnection")
		for _, param := range proc.Params {
			paramName := toLowerCamel(param.Name)
			goType := goTypeForAlgebraic(param.Type, schema.Typespace, schema.Types)
			paramList = append(paramList, paramName+" "+goType)
		}

		fmt.Fprintf(&procCode, "func Call%s(%s) error {\n", goName, strings.Join(paramList, ", "))

		if len(proc.Params) > 0 {
			argsName := toLowerCamel(proc.Name) + "Args"
			fmt.Fprintf(&procCode, "\targs := &%s{\n", argsName)
			for _, param := range proc.Params {
				fieldName := toGoName(param.Name)
				paramName := toLowerCamel(param.Name)
				fmt.Fprintf(&procCode, "\t\t%s: %s,\n", fieldName, paramName)
			}
			fmt.Fprintf(&procCode, "\t}\n")
			fmt.Fprintf(&procCode, "\treturn conn.CallReducer(%q, args)\n", proc.Name)
		} else {
			fmt.Fprintf(&procCode, "\treturn conn.CallReducer(%q, nil)\n", proc.Name)
		}

		fmt.Fprintf(&procCode, "}\n\n")
	}

	w.WriteString(clientImportBlock(imports))
	w.WriteString(procCode.String())

	return []byte(w.String()), nil
}
