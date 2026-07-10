package servergen

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// GeneratedFile is one output artifact of GenerateAll.
type GeneratedFile struct {
	// RelPath is relative to the module root (e.g. "stdb_generated.go",
	// "schema/stdb_tables_generated.go").
	RelPath string
	Content []byte
}

// Generate produces a single-file stdb_generated.go for single-package modules
// (backward compatible). Prefer GenerateAll for multi-package modules.
func Generate(module *AnalyzedModule) ([]byte, error) {
	files, err := GenerateAll(module)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no output generated")
	}
	// Prefer the root module file.
	for _, f := range files {
		base := filepath.Base(f.RelPath)
		if base == "stdb_generated.go" || base == "stdb_module_generated.go" {
			return f.Content, nil
		}
	}
	return files[0].Content, nil
}

// GenerateAll produces all generated files for the module.
//
// Single-package: one root stdb_generated.go (tables + BSATN + dispatch).
// Multi-package: stdb_tables_generated.go per package that owns tables/types,
// plus root stdb_module_generated.go with dispatch/moduledef/init.
func GenerateAll(module *AnalyzedModule) ([]GeneratedFile, error) {
	if !module.MultiPackage {
		code, err := generateSinglePackage(module)
		if err != nil {
			return nil, err
		}
		return []GeneratedFile{{RelPath: "stdb_generated.go", Content: code}}, nil
	}
	return generateMultiPackage(module)
}

func generateSinglePackage(module *AnalyzedModule) ([]byte, error) {
	var w strings.Builder

	w.WriteString(fileHeader(module.PackageName))
	imports := collectImports(module, true /*includeTableRuntime*/)
	w.WriteString(importBlock(imports))
	writeImportKeepalives(&w, true)

	w.WriteString("func stdbStrPtr(s string) *string { return &s }\n\n")

	if hasScheduleAt(module) {
		generateScheduleAtHelpers(&w)
	}

	generateBsatn(module, &w, "")
	generateTables(module, &w, "")

	generateReducerDispatch(module, &w)
	if err := generateModuleDef(module, &w); err != nil {
		return nil, err
	}
	generateInit(module, &w)

	return gofmtOrRaw([]byte(w.String()))
}

func generateMultiPackage(module *AnalyzedModule) ([]GeneratedFile, error) {
	var out []GeneratedFile

	// Group tables / owned types by RelDir.
	tablesByDir := map[string][]AnalyzedTable{}
	for _, t := range module.Tables {
		tablesByDir[t.RelDir] = append(tablesByDir[t.RelDir], t)
	}
	// Types owned by package (structs with RelDir).
	typesByDir := map[string][]string{}
	for _, name := range module.TypeOrder {
		t := module.Types[name]
		if t == nil || t.Kind != TypeKindStruct {
			// Enums/sumtypes: put in same dir as first table that needs them, or root.
			if t != nil && (t.Kind == TypeKindSimpleEnum || t.Kind == TypeKindSumType) {
				typesByDir[t.RelDir] = append(typesByDir[t.RelDir], name)
			}
			continue
		}
		typesByDir[t.RelDir] = append(typesByDir[t.RelDir], name)
	}

	// Emit tables+bsatn for every package that owns tables or product types.
	dirs := map[string]bool{}
	for d := range tablesByDir {
		dirs[d] = true
	}
	for d := range typesByDir {
		dirs[d] = true
	}

	for relDir := range dirs {
		pkgName := module.PackageName
		if relDir != "" {
			// Find package name from Packages list.
			pkgName = ""
			for _, p := range module.Packages {
				if p.RelDir == relDir {
					pkgName = p.Name
					break
				}
			}
			if pkgName == "" {
				pkgName = pathBase(relDir)
			}
		}

		code, err := generateTablesPackage(module, pkgName, relDir, tablesByDir[relDir], typesByDir[relDir])
		if err != nil {
			return nil, err
		}
		// Skip empty packages (only enums with no local structs/tables).
		if code == nil {
			continue
		}
		// Multi-package: tables file is always stdb_tables_generated.go
		// (module dispatch lives in root stdb_module_generated.go).
		relPath := "stdb_tables_generated.go"
		if relDir != "" {
			relPath = filepath.ToSlash(filepath.Join(relDir, "stdb_tables_generated.go"))
		}
		out = append(out, GeneratedFile{RelPath: relPath, Content: code})
	}

	// Root module file: dispatch + moduledef + init.
	modCode, err := generateModulePackage(module)
	if err != nil {
		return nil, err
	}
	out = append(out, GeneratedFile{RelPath: "stdb_module_generated.go", Content: modCode})

	return out, nil
}

// generateTablesPackage emits BSATN + table accessors for one Go package.
func generateTablesPackage(module *AnalyzedModule, pkgName, relDir string, tables []AnalyzedTable, typeNames []string) ([]byte, error) {
	if len(tables) == 0 && len(typeNames) == 0 {
		return nil, nil
	}
	// Build a filtered view of the module for this package.
	sub := *module
	sub.PackageName = pkgName
	sub.Tables = tables
	// Restrict TypeOrder to this package's types (plus any nested product types
	// they reference that live in the same package — already in typeNames).
	sub.TypeOrder = append([]string(nil), typeNames...)
	// Also include sum/enum types referenced (already filtered by RelDir).

	var w strings.Builder
	w.WriteString(fileHeader(pkgName))
	imports := collectImports(module, true)
	// Table packages do not import feature packages.
	w.WriteString(importBlock(imports))
	writeImportKeepalives(&w, true)
	w.WriteString("func stdbStrPtr(s string) *string { return &s }\n\n")

	if hasScheduleAtForTypes(module, typeNames) {
		generateScheduleAtHelpers(&w)
	}

	generateBsatn(module, &w, relDir)
	generateTables(module, &w, relDir)

	return gofmtOrRaw([]byte(w.String()))
}

// generateModulePackage emits root dispatch + moduledef + init.
func generateModulePackage(module *AnalyzedModule) ([]byte, error) {
	var w strings.Builder
	w.WriteString(fileHeader(module.PackageName))
	imports := collectImports(module, false /*module file needs reducer import + feature pkgs*/)
	// Import packages that host reducers/views OR types decoded/encoded by the
	// root dispatcher (custom struct params via schema.StdbReadX).
	for _, p := range module.Packages {
		if p.IsRoot || p.ImportPath == "" {
			continue
		}
		if packageHasCallables(module, p.RelDir) || packageHasCodecTypes(module, p.RelDir) {
			imports[p.Name] = p.ImportPath
		}
	}
	// Root may still own tables — if so, table runtime imports are needed only
	// when root also emits tables (separate file). Module file always needs
	// moduledef + runtime + bsatn for arg decode.
	imports[""] = "fmt"
	imports["bsatn"] = "go.digitalxero.dev/spacetimedb-client/bsatn"
	imports["runtime"] = "go.digitalxero.dev/spacetimedb-server/runtime"
	imports["types"] = "go.digitalxero.dev/spacetimedb-client/types"
	imports["moduledef"] = "go.digitalxero.dev/spacetimedb-server/moduledef"
	imports["reducer"] = "go.digitalxero.dev/spacetimedb-server/reducer"

	w.WriteString(importBlock(imports))
	writeImportKeepalivesModule(&w)

	// Module file may need stdbStrPtr for defaults in moduledef? moduledef uses it.
	w.WriteString("func stdbStrPtr(s string) *string { return &s }\n\n")

	// Custom struct params are decoded via exported schema.StdbReadX codecs.

	generateReducerDispatch(module, &w)
	if err := generateModuleDef(module, &w); err != nil {
		return nil, err
	}
	generateInit(module, &w)

	return gofmtOrRaw([]byte(w.String()))
}

func packageHasCallables(module *AnalyzedModule, relDir string) bool {
	for _, r := range module.Reducers {
		if r.RelDir == relDir {
			return true
		}
	}
	for _, lc := range module.Lifecycle {
		if lc.RelDir == relDir {
			return true
		}
	}
	for _, p := range module.Procedures {
		if p.RelDir == relDir {
			return true
		}
	}
	for _, v := range module.Views {
		if v.RelDir == relDir {
			return true
		}
	}
	return false
}

// packageHasCodecTypes reports whether the package owns any product/sum type
// that the root dispatcher may need to StdbRead/StdbWrite (custom struct params
// or procedure/view returns).
func packageHasCodecTypes(module *AnalyzedModule, relDir string) bool {
	need := map[string]bool{}
	var note func(t AlgType)
	note = func(t AlgType) {
		if t.Kind == AlgKindRef && t.TypeName != "" {
			need[t.TypeName] = true
		}
		if t.ElemType != nil {
			note(*t.ElemType)
		}
	}
	for _, r := range module.Reducers {
		for _, p := range r.Params {
			note(p.AlgType)
		}
	}
	for _, p := range module.Procedures {
		for _, param := range p.Params {
			note(param.AlgType)
		}
		if p.ReturnType != nil {
			note(*p.ReturnType)
		}
	}
	for _, v := range module.Views {
		for _, param := range v.Params {
			note(param.AlgType)
		}
		note(v.ReturnType)
	}
	for typeName := range need {
		if t := module.Types[typeName]; t != nil && t.RelDir == relDir {
			return true
		}
	}
	return false
}

func writeImportKeepalives(w *strings.Builder, withSys bool) {
	w.WriteString("// Ensure imports are used.\n")
	w.WriteString("var (\n")
	w.WriteString("\t_ = fmt.Sprintf\n")
	w.WriteString("\t_ = bsatn.NewWriter\n")
	w.WriteString("\t_ = runtime.GlobalWriter\n")
	if withSys {
		w.WriteString("\t_ = sys.TableIdFromName\n")
	}
	w.WriteString("\t_ = types.NewIdentity\n")
	w.WriteString("\t_ = moduledef.NewModuleDefBuilder\n")
	w.WriteString("\t_ = reducer.NewReducerContext\n")
	w.WriteString(")\n\n")
}

func writeImportKeepalivesModule(w *strings.Builder) {
	w.WriteString("// Ensure imports are used.\n")
	w.WriteString("var (\n")
	w.WriteString("\t_ = fmt.Sprintf\n")
	w.WriteString("\t_ = bsatn.NewWriter\n")
	w.WriteString("\t_ = runtime.GlobalWriter\n")
	w.WriteString("\t_ = types.NewIdentity\n")
	w.WriteString("\t_ = moduledef.NewModuleDefBuilder\n")
	w.WriteString("\t_ = reducer.NewReducerContext\n")
	w.WriteString(")\n\n")
}

func gofmtOrRaw(code []byte) ([]byte, error) {
	formatted, err := Gofmt(code)
	if err != nil {
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
// includeTableRuntime adds sys (table id resolution).
func collectImports(module *AnalyzedModule, includeTableRuntime bool) map[string]string {
	imports := map[string]string{
		"":        "fmt",
		"bsatn":    "go.digitalxero.dev/spacetimedb-client/bsatn",
		"runtime":  "go.digitalxero.dev/spacetimedb-server/runtime",
		"types":    "go.digitalxero.dev/spacetimedb-client/types",
		"moduledef": "go.digitalxero.dev/spacetimedb-server/moduledef",
		"reducer":  "go.digitalxero.dev/spacetimedb-server/reducer",
	}
	if includeTableRuntime {
		imports["sys"] = "go.digitalxero.dev/spacetimedb-server/sys"
	}
	_ = module
	return imports
}

// hasScheduleAtForTypes is like hasScheduleAt but limited to named types.
func hasScheduleAtForTypes(module *AnalyzedModule, typeNames []string) bool {
	set := map[string]bool{}
	for _, n := range typeNames {
		set[n] = true
	}
	for _, typeName := range typeNames {
		typeInfo := module.Types[typeName]
		if typeInfo == nil || typeInfo.Kind != TypeKindStruct {
			continue
		}
		for _, f := range typeInfo.Fields {
			if f.AlgType.Kind == AlgKindScheduleAt {
				return true
			}
		}
	}
	return false
}

// Gofmt runs gofmt on the given source code.
func Gofmt(src []byte) ([]byte, error) {
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
