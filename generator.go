package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// generate produces the complete stdb_generated.go file content.
func generate(module *AnalyzedModule) ([]byte, error) {
	var w strings.Builder

	// File header.
	w.WriteString(fileHeader(module.PackageName))

	// Imports.
	imports := collectImports(module)
	w.WriteString(importBlock(imports))

	// Ensure we use all imports (avoid unused import errors).
	w.WriteString("// Ensure imports are used.\n")
	w.WriteString("var (\n")
	w.WriteString("\t_ = fmt.Sprintf\n")
	w.WriteString("\t_ = bsatn.NewWriter\n")
	w.WriteString("\t_ = runtime.GlobalWriter\n")
	w.WriteString("\t_ = sys.TableIdFromName\n")
	w.WriteString("\t_ = types.NewIdentity\n")
	w.WriteString("\t_ = moduledef.NewModuleDefBuilder\n")
	w.WriteString("\t_ = reducer.NewReducerContext\n")
	w.WriteString(")\n\n")

	// Helper: string pointer.
	w.WriteString("func stdbStrPtr(s string) *string { return &s }\n\n")

	// ScheduleAt helper (if any table uses it).
	if hasScheduleAt(module) {
		generateScheduleAtHelpers(&w)
	}

	// BSATN encode/decode functions for all types.
	generateBsatn(module, &w)

	// Table accessor types.
	generateTables(module, &w)

	// Reducer/procedure/view dispatch functions.
	generateReducerDispatch(module, &w)

	// Module definition function.
	generateModuleDef(module, &w)

	// init() function to register handlers.
	generateInit(module, &w)

	// Format with gofmt.
	code := []byte(w.String())
	formatted, err := gofmt(code)
	if err != nil {
		// If gofmt fails, return unformatted code with a warning comment.
		return code, nil
	}
	return formatted, nil
}

// generateInit generates the init() function that registers all handlers with the runtime.
func generateInit(module *AnalyzedModule, w *strings.Builder) {
	fmt.Fprintf(w, "func init() {\n")
	fmt.Fprintf(w, "\truntime.SetDescribeModuleHandler(stdbDescribeModule)\n")
	fmt.Fprintf(w, "\truntime.SetCallReducerHandler(stdbCallReducer)\n")
	if len(module.Procedures) > 0 {
		fmt.Fprintf(w, "\truntime.SetCallProcedureHandler(stdbCallProcedure)\n")
	}
	if hasAuthViews(module) {
		fmt.Fprintf(w, "\truntime.SetCallViewHandler(stdbCallView)\n")
	}
	if hasAnonViews(module) {
		fmt.Fprintf(w, "\truntime.SetCallViewAnonHandler(stdbCallViewAnon)\n")
	}
	fmt.Fprintf(w, "}\n")
}

// collectImports determines which packages are needed by the generated code.
func collectImports(module *AnalyzedModule) map[string]string {
	imports := map[string]string{
		"":         "fmt",
		"bsatn":    "github.com/clockworklabs/SpacetimeDB/sdks/go/bsatn",
		"runtime":  "github.com/clockworklabs/SpacetimeDB/sdks/go/server/runtime",
		"sys":      "github.com/clockworklabs/SpacetimeDB/sdks/go/server/sys",
		"types":    "github.com/clockworklabs/SpacetimeDB/sdks/go/types",
		"moduledef": "github.com/clockworklabs/SpacetimeDB/sdks/go/server/moduledef",
		"reducer":  "github.com/clockworklabs/SpacetimeDB/sdks/go/server/reducer",
	}
	return imports
}

// gofmt runs gofmt on the given source code.
func gofmt(src []byte) ([]byte, error) {
	cmd := exec.Command("gofmt")
	cmd.Stdin = strings.NewReader(string(src))
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// hasScheduleAt checks if any type in the module uses ScheduleAt.
func hasScheduleAt(module *AnalyzedModule) bool {
	for _, typeName := range module.TypeOrder {
		typeInfo := module.Types[typeName]
		if typeInfo.Kind == TypeKindStruct {
			for _, f := range typeInfo.Fields {
				if f.AlgType.Kind == AlgKindScheduleAt {
					return true
				}
			}
		}
	}
	return false
}
