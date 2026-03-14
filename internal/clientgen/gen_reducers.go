package clientgen

import (
	"fmt"
	"strings"
)

// generateReducers generates reducer caller functions and args structs.
func generateReducers(schema *ModuleSchema, pkgName string) ([]byte, error) {
	if len(schema.Reducers) == 0 {
		return nil, nil
	}

	var w strings.Builder
	w.WriteString(clientFileHeader(pkgName))

	imports := map[string]string{
		"bsatn":  "go.digitalxero.dev/spacetimedb-client/bsatn",
		"client": "go.digitalxero.dev/spacetimedb-client/client",
	}

	var reducerCode strings.Builder

	for _, reducer := range schema.Reducers {
		goName := toGoName(reducer.Name)

		if len(reducer.Params) > 0 {
			// Args struct
			argsName := toLowerCamel(reducer.Name) + "Args"
			fmt.Fprintf(&reducerCode, "type %s struct {\n", argsName)
			for _, param := range reducer.Params {
				fieldName := toGoName(param.Name)
				goType := goTypeForAlgebraic(param.Type, schema.Typespace, schema.Types)
				if needsTypesImport(goType) {
					imports["types"] = "go.digitalxero.dev/spacetimedb-client/types"
				}
				fmt.Fprintf(&reducerCode, "\t%s %s\n", fieldName, goType)
			}
			fmt.Fprintf(&reducerCode, "}\n\n")

			// WriteBsatn for args
			fmt.Fprintf(&reducerCode, "func (a *%s) WriteBsatn(w bsatn.Writer) {\n", argsName)
			for _, param := range reducer.Params {
				fieldName := toGoName(param.Name)
				writeFieldEncoder(&reducerCode, "a."+fieldName, param.Type, schema, imports, "\t")
			}
			fmt.Fprintf(&reducerCode, "}\n\n")
		}

		// Caller function
		fmt.Fprintf(&reducerCode, "// Call%s calls the %s reducer on the server.\n", goName, reducer.Name)

		// Build parameter list
		var paramList []string
		paramList = append(paramList, "conn client.DbConnection")
		for _, param := range reducer.Params {
			paramName := toLowerCamel(param.Name)
			goType := goTypeForAlgebraic(param.Type, schema.Typespace, schema.Types)
			paramList = append(paramList, paramName+" "+goType)
		}

		fmt.Fprintf(&reducerCode, "func Call%s(%s) error {\n", goName, strings.Join(paramList, ", "))

		if len(reducer.Params) > 0 {
			argsName := toLowerCamel(reducer.Name) + "Args"
			fmt.Fprintf(&reducerCode, "\targs := &%s{\n", argsName)
			for _, param := range reducer.Params {
				fieldName := toGoName(param.Name)
				paramName := toLowerCamel(param.Name)
				fmt.Fprintf(&reducerCode, "\t\t%s: %s,\n", fieldName, paramName)
			}
			fmt.Fprintf(&reducerCode, "\t}\n")
			fmt.Fprintf(&reducerCode, "\treturn conn.CallReducer(%q, args)\n", reducer.Name)
		} else {
			fmt.Fprintf(&reducerCode, "\treturn conn.CallReducer(%q, nil)\n", reducer.Name)
		}

		fmt.Fprintf(&reducerCode, "}\n\n")
	}

	w.WriteString(clientImportBlock(imports))
	w.WriteString(reducerCode.String())

	return []byte(w.String()), nil
}
