package clientgen

import (
	"fmt"
	"strings"
)

// generateProcedures generates procedure caller functions, their args structs,
// and the per-procedure return-value decode helpers.
func generateProcedures(schema *ModuleSchema, pkgName string) ([]byte, error) {
	if len(schema.Procedures) == 0 {
		return nil, nil
	}

	var w strings.Builder
	w.WriteString(clientFileHeader(pkgName))

	imports := map[string]string{
		"context": "context",
		"bsatn":   "go.digitalxero.dev/spacetimedb-client/bsatn",
		"client":  "go.digitalxero.dev/spacetimedb-client/client",
	}

	var procCode strings.Builder

	for _, proc := range schema.Procedures {
		goName := toGoName(proc.Name)

		hasReturn := procHasReturn(proc.ReturnType)
		var retType string
		if hasReturn {
			retType = goTypeForAlgebraic(proc.ReturnType, schema.Typespace, schema.Types)
			if strings.Contains(retType, "types.") {
				imports["types"] = "go.digitalxero.dev/spacetimedb-client/types"
			}
		}

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

		// Caller function signature
		var paramList []string
		paramList = append(paramList, "ctx context.Context", "conn client.DbConnection")
		for _, param := range proc.Params {
			paramName := toLowerCamel(param.Name)
			goType := goTypeForAlgebraic(param.Type, schema.Typespace, schema.Types)
			paramList = append(paramList, paramName+" "+goType)
		}

		fmt.Fprintf(&procCode, "// Call%s calls the %s procedure on the server.\n", goName, proc.Name)
		if hasReturn {
			fmt.Fprintf(&procCode, "func Call%s(%s) (%s, error) {\n", goName, strings.Join(paramList, ", "), retType)
			fmt.Fprintf(&procCode, "\tvar zero %s\n", retType)
		} else {
			fmt.Fprintf(&procCode, "func Call%s(%s) error {\n", goName, strings.Join(paramList, ", "))
		}

		// Build the args value (or nil when the procedure takes no parameters).
		argsExpr := "nil"
		if len(proc.Params) > 0 {
			argsName := toLowerCamel(proc.Name) + "Args"
			fmt.Fprintf(&procCode, "\targs := &%s{\n", argsName)
			for _, param := range proc.Params {
				fieldName := toGoName(param.Name)
				paramName := toLowerCamel(param.Name)
				fmt.Fprintf(&procCode, "\t\t%s: %s,\n", fieldName, paramName)
			}
			fmt.Fprintf(&procCode, "\t}\n")
			argsExpr = "args"
		}

		if hasReturn {
			fmt.Fprintf(&procCode, "\traw, err := conn.CallProcedure(ctx, %q, %s)\n", proc.Name, argsExpr)
			fmt.Fprintf(&procCode, "\tif err != nil {\n\t\treturn zero, err\n\t}\n")
			fmt.Fprintf(&procCode, "\tresult, err := read%sResult(bsatn.NewReader(raw))\n", goName)
			fmt.Fprintf(&procCode, "\tif err != nil {\n\t\treturn zero, err\n\t}\n")
			fmt.Fprintf(&procCode, "\treturn *result, nil\n")
		} else {
			fmt.Fprintf(&procCode, "\t_, err := conn.CallProcedure(ctx, %q, %s)\n", proc.Name, argsExpr)
			fmt.Fprintf(&procCode, "\treturn err\n")
		}
		fmt.Fprintf(&procCode, "}\n\n")

		// Per-procedure return-value decode helper. It returns a pointer so that
		// the reused writeFieldDecoder error paths (which `return nil, err`) are
		// valid for every return shape, including non-nilable value types.
		if hasReturn {
			fmt.Fprintf(&procCode, "func read%sResult(r bsatn.Reader) (*%s, error) {\n", goName, retType)
			fmt.Fprintf(&procCode, "\tvar result %s\n", retType)
			fmt.Fprintf(&procCode, "\tvar err error\n")
			writeFieldDecoder(&procCode, "result", proc.ReturnType, schema, imports, "\t")
			fmt.Fprintf(&procCode, "\t_ = err\n")
			fmt.Fprintf(&procCode, "\treturn &result, nil\n")
			fmt.Fprintf(&procCode, "}\n\n")
		}
	}

	w.WriteString(clientImportBlock(imports))
	w.WriteString(procCode.String())

	return []byte(w.String()), nil
}

// procHasReturn reports whether a procedure declares a meaningful return value.
// A nil return type, or the unit type (an empty product), means the procedure
// returns nothing and its caller only reports an error.
func procHasReturn(rt *AlgebraicType) bool {
	if rt == nil {
		return false
	}
	if rt.Kind == ATKProduct && (rt.Product == nil || len(rt.Product.Elements) == 0) {
		return false
	}
	return true
}
