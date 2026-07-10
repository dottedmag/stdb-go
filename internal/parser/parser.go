package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// ParsedModule is the intermediate representation produced by parsing.
type ParsedModule struct {
	// PackageName is the Go package name of the module root (usually "main").
	PackageName string
	// ModulePath is the go.mod module path (empty if go.mod is missing).
	ModulePath string
	// RootDir is the absolute path to the module root that was parsed.
	RootDir string
	// MultiPackage is true when //stdb: declarations span more than one Go package.
	MultiPackage bool

	Tables     []ParsedTable
	Reducers   []ParsedReducer
	Lifecycle  []ParsedLifecycle
	Procedures []ParsedProcedure
	Views      []ParsedView
	SumTypes   []ParsedSumType
	Enums      []ParsedEnum
	Variants   []ParsedVariant
	Schedules  []ParsedSchedule
	RLS        []string

	// All struct definitions found across packages (for type resolution).
	// Keyed by unqualified Go name; multi-package modules reject name collisions.
	Structs map[string]*ParsedStruct

	// Type aliases: name → underlying type name (e.g., TestAlias → TestA).
	TypeAliases map[string]string

	// Packages lists every Go package discovered under RootDir that contributed
	// source (even if it has no //stdb: directives). Used for import generation.
	Packages []ParsedPackage
}

// ParsedPackage describes one Go package under the module root.
type ParsedPackage struct {
	// Name is the package clause (e.g. "schema", "main").
	Name string
	// ImportPath is the full import path (modulePath + "/" + relDir), or
	// ModulePath alone for the root package.
	ImportPath string
	// RelDir is the path relative to RootDir ("" for root).
	RelDir string
	// IsRoot is true for the package that lives in RootDir.
	IsRoot bool
}

// ParsedStruct represents a struct type definition found in the package.
type ParsedStruct struct {
	Name       string
	Fields     []ParsedField
	Package    string // package clause name
	ImportPath string // full import path of defining package
	RelDir     string
}

// ParsedTable represents a table declared via //stdb:table directive.
type ParsedTable struct {
	Name         string // from name= directive
	Access       string // "public" or "private"
	IsEvent      bool   // event=true
	StructName   string // Go struct name
	Fields       []ParsedField
	ExtraIndexes []ParsedMultiColIndex
	Package      string
	ImportPath   string
	RelDir       string
}

// ParsedMultiColIndex represents a multi-column BTree index.
type ParsedMultiColIndex struct {
	Name    string
	Columns []uint16
}

// ParsedField represents a struct field with metadata.
type ParsedField struct {
	GoName      string // e.g., "EntityId"
	GoType      string // e.g., "uint64", "types.Identity", "DbVector2"
	BsatnName   string // snake_case: "entity_id"
	PrimaryKey  bool
	AutoInc     bool
	Unique      bool
	IndexBTree  bool
	IndexDirect bool
	Default     *string // from default=<value>; nil means "no default" (distinct from default="")
}

// ParsedReducer represents a reducer declared via //stdb:reducer directive.
type ParsedReducer struct {
	Name       string        // from name= or auto snake_case of FuncName
	FuncName   string        // Go function name from AST
	Params     []ParsedParam // after context param, names from Go func signature
	HasError   bool          // returns error
	Package    string
	ImportPath string
	RelDir     string
	// Exported is true when FuncName is exported (required for non-root packages).
	Exported bool
}

// ParsedParam is a reducer/procedure/view parameter.
type ParsedParam struct {
	Name   string // from Go function signature
	GoType string // Go type string
}

// ParsedLifecycle represents a lifecycle reducer declared via //stdb:init, //stdb:connect, or //stdb:disconnect.
type ParsedLifecycle struct {
	Kind       string // "init", "connect", or "disconnect"
	FuncName   string // Go function name from AST
	Package    string
	ImportPath string
	RelDir     string
	Exported   bool
}

// ParsedProcedure represents a procedure declared via //stdb:procedure directive.
type ParsedProcedure struct {
	Name       string
	FuncName   string
	Params     []ParsedParam
	ReturnType string // Go type of return value, empty if void
	Package    string
	ImportPath string
	RelDir     string
	Exported   bool
}

// ParsedView represents a view declared via //stdb:view directive.
type ParsedView struct {
	Name        string
	FuncName    string
	IsPublic    bool
	IsAnonymous bool
	Params      []ParsedParam
	ReturnType  string // Go type string (e.g., "*Player", "[]Player")
	Package     string
	ImportPath  string
	RelDir      string
	Exported    bool
}

// ParsedSumType represents a sum type declared via //stdb:sumtype directive.
type ParsedSumType struct {
	InterfaceName string   // Go interface name
	Scope         []string // optional scope path
}

// ParsedEnum represents a simple enum declared via //stdb:enum directive.
type ParsedEnum struct {
	TypeName string   // Go type name
	Variants []string // variant names
	Scope    []string // optional scope path
}

// ParsedVariant represents a variant declared via //stdb:variant directive.
type ParsedVariant struct {
	OfInterface string // which sum type interface it belongs to
	Name        string // variant name (BSATN tag name)
	StructName  string // Go struct name
	Fields      []ParsedField
}

// ParsedSchedule represents a schedule declared via //stdb:schedule directive.
type ParsedSchedule struct {
	TableName    string
	FunctionName string
}

// skipDirName reports directories that must not be walked when discovering packages.
func skipDirName(name string) bool {
	switch name {
	case "vendor", "testdata", "node_modules", ".git", ".hg", ".svn":
		return true
	}
	return strings.HasPrefix(name, ".")
}

// isSkippableGoFile reports .go files that are never sources of //stdb: directives.
func isSkippableGoFile(name string) bool {
	if !strings.HasSuffix(name, ".go") {
		return true
	}
	if name == "main.go" {
		return true
	}
	if strings.HasSuffix(name, "_test.go") {
		return true
	}
	// Generated artifacts (legacy and dual-output names).
	if strings.HasSuffix(name, "_generated.go") ||
		name == "stdb_generated.go" ||
		name == "stdb_module_generated.go" ||
		name == "stdb_tables_generated.go" {
		return true
	}
	return false
}

// readModulePath returns the module path from go.mod in dir, or "".
func readModulePath(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

// importPathFor returns the full import path for a package at relDir under root.
func importPathFor(modulePath, relDir string) string {
	relDir = filepath.ToSlash(relDir)
	if modulePath == "" {
		if relDir == "" || relDir == "." {
			return ""
		}
		return relDir
	}
	if relDir == "" || relDir == "." {
		return modulePath
	}
	return modulePath + "/" + relDir
}

// ParseDirectory parses the module rooted at dir. It walks nested packages so
// //stdb: directives may live in subdirectories (each directory is its own Go
// package). Flat single-package modules behave as before.
//
// ParseDirectory is the public entry point used by generate/build.
func ParseDirectory(dir string) (*ParsedModule, error) {
	return ParseModule(dir)
}

// ParseModule walks dir recursively and extracts //stdb: directives from every
// Go package under the module root.
func ParseModule(dir string) (*ParsedModule, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("abs dir: %w", err)
	}

	modulePath := readModulePath(absDir)
	fset := token.NewFileSet()
	module := &ParsedModule{
		ModulePath:  modulePath,
		RootDir:     absDir,
		Structs:     make(map[string]*ParsedStruct),
		TypeAliases: make(map[string]string),
	}

	// Collect packages: relDir → package clause name (first file wins).
	type pkgInfo struct {
		name   string
		relDir string
		files  []string
	}
	packages := map[string]*pkgInfo{} // key = relDir

	err = filepath.WalkDir(absDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path == absDir {
				return nil
			}
			if skipDirName(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if isSkippableGoFile(name) {
			return nil
		}
		rel, err := filepath.Rel(absDir, path)
		if err != nil {
			return err
		}
		relDir := filepath.Dir(rel)
		if relDir == "." {
			relDir = ""
		}
		// Ensure package entry exists (package name filled after parse).
		pi, ok := packages[relDir]
		if !ok {
			pi = &pkgInfo{relDir: relDir}
			packages[relDir] = pi
		}
		pi.files = append(pi.files, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk: %w", err)
	}

	// Ensure root package is processed even if only main.go exists (no stdb files).
	if _, ok := packages[""]; !ok {
		packages[""] = &pkgInfo{relDir: "", name: "main"}
	}

	// Stable package order: root first, then sorted relDirs.
	relDirs := make([]string, 0, len(packages))
	for rel := range packages {
		relDirs = append(relDirs, rel)
	}
	sort.Slice(relDirs, func(i, j int) bool {
		if relDirs[i] == "" {
			return true
		}
		if relDirs[j] == "" {
			return false
		}
		return relDirs[i] < relDirs[j]
	})

	pkgSet := map[string]bool{} // import paths that define //stdb: symbols
	for _, relDir := range relDirs {
		pi := packages[relDir]
		impPath := importPathFor(modulePath, relDir)
		isRoot := relDir == ""

		for _, filePath := range pi.files {
			f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", filePath, err)
			}
			if pi.name == "" {
				pi.name = f.Name.Name
			} else if f.Name.Name != pi.name {
				return nil, fmt.Errorf("package name mismatch in %s: got %q want %q", filePath, f.Name.Name, pi.name)
			}
			if isRoot && module.PackageName == "" {
				module.PackageName = f.Name.Name
			}
			// Nested directory must not be package main (orphan main package).
			if !isRoot && f.Name.Name == "main" {
				return nil, fmt.Errorf("%s: nested package must not be named main (got package main in %s)", filePath, relDir)
			}

			ctx := fileCtx{
				packageName: f.Name.Name,
				importPath:  impPath,
				relDir:      relDir,
				isRoot:      isRoot,
			}
			if err := processFile(f, module, ctx); err != nil {
				return nil, fmt.Errorf("process %s: %w", filePath, err)
			}
		}

		if pi.name == "" {
			// Directory with no parseable files (only skipped) — ignore.
			if len(pi.files) == 0 {
				continue
			}
			pi.name = "main"
		}
		if isRoot && module.PackageName == "" {
			module.PackageName = pi.name
		}

		module.Packages = append(module.Packages, ParsedPackage{
			Name:       pi.name,
			ImportPath: impPath,
			RelDir:     relDir,
			IsRoot:     isRoot,
		})
		pkgSet[impPath] = true
	}

	if module.PackageName == "" {
		module.PackageName = "main"
	}

	// MultiPackage if any //stdb: table/reducer/lifecycle lives outside root.
	for _, t := range module.Tables {
		if t.RelDir != "" {
			module.MultiPackage = true
			break
		}
	}
	if !module.MultiPackage {
		for _, r := range module.Reducers {
			if r.RelDir != "" {
				module.MultiPackage = true
				break
			}
		}
	}
	if !module.MultiPackage {
		for _, lc := range module.Lifecycle {
			if lc.RelDir != "" {
				module.MultiPackage = true
				break
			}
		}
	}
	if !module.MultiPackage {
		for _, p := range module.Procedures {
			if p.RelDir != "" {
				module.MultiPackage = true
				break
			}
		}
	}
	if !module.MultiPackage {
		for _, v := range module.Views {
			if v.RelDir != "" {
				module.MultiPackage = true
				break
			}
		}
	}

	// Validate non-root reducers/lifecycle/procedures/views are exported.
	if err := validateExports(module); err != nil {
		return nil, err
	}

	return module, nil
}

func validateExports(module *ParsedModule) error {
	check := func(kind, name, pkg string, exported bool, relDir string) error {
		if relDir == "" {
			return nil
		}
		if !exported {
			return fmt.Errorf("%s %q in package %q (%s): must be exported (capitalized) so the root module can call it", kind, name, pkg, relDir)
		}
		return nil
	}
	for _, r := range module.Reducers {
		if err := check("reducer", r.FuncName, r.Package, r.Exported, r.RelDir); err != nil {
			return err
		}
	}
	for _, lc := range module.Lifecycle {
		if err := check("lifecycle", lc.FuncName, lc.Package, lc.Exported, lc.RelDir); err != nil {
			return err
		}
	}
	for _, p := range module.Procedures {
		if err := check("procedure", p.FuncName, p.Package, p.Exported, p.RelDir); err != nil {
			return err
		}
	}
	for _, v := range module.Views {
		if err := check("view", v.FuncName, v.Package, v.Exported, v.RelDir); err != nil {
			return err
		}
	}
	return nil
}

// fileCtx is the package context for one source file.
type fileCtx struct {
	packageName string
	importPath  string
	relDir      string
	isRoot      bool
}

// processFile extracts //stdb: directives from a parsed Go file.
func processFile(f *ast.File, module *ParsedModule, ctx fileCtx) error {
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if err := processGenDecl(d, f, module, ctx); err != nil {
				return err
			}
		case *ast.FuncDecl:
			if err := processFuncDecl(d, f, module, ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

// processGenDecl processes type/const declarations for //stdb: directives.
func processGenDecl(d *ast.GenDecl, f *ast.File, module *ParsedModule, ctx fileCtx) error {
	for _, spec := range d.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}

		// Capture type aliases (e.g., type TestAlias = TestA).
		if ts.Assign.IsValid() {
			if ident, ok := ts.Type.(*ast.Ident); ok {
				module.TypeAliases[ts.Name.Name] = ident.Name
			} else if sel, ok := ts.Type.(*ast.SelectorExpr); ok {
				if pkg, ok := sel.X.(*ast.Ident); ok {
					module.TypeAliases[ts.Name.Name] = pkg.Name + "." + sel.Sel.Name
				}
			}
		}

		// Get comments from the decl's doc or the spec's doc.
		comments := collectComments(d.Doc, ts.Doc)
		directives := extractDirectives(comments)

		// If this is a struct type, always parse its fields for type resolution.
		if st, ok := ts.Type.(*ast.StructType); ok {
			fields := parseStructFields(st)
			if prev, exists := module.Structs[ts.Name.Name]; exists {
				// Same package redefinition is last-wins (as before); cross-package is an error.
				if prev.ImportPath != ctx.importPath && prev.RelDir != ctx.relDir {
					return fmt.Errorf("struct %q defined in both package %q and %q (type names must be unique across the module)",
						ts.Name.Name, prev.Package, ctx.packageName)
				}
			}
			module.Structs[ts.Name.Name] = &ParsedStruct{
				Name:       ts.Name.Name,
				Fields:     fields,
				Package:    ctx.packageName,
				ImportPath: ctx.importPath,
				RelDir:     ctx.relDir,
			}

			// Process //stdb:table directives (can have multiple).
			for _, dir := range directives {
				switch dir.Kind {
				case "table":
					table := ParsedTable{
						Name:       dir.Params["name"],
						Access:     dir.Params["access"],
						IsEvent:    dir.Params["event"] == "true",
						StructName: ts.Name.Name,
						Fields:     fields,
						Package:    ctx.packageName,
						ImportPath: ctx.importPath,
						RelDir:     ctx.relDir,
					}
					if table.Access == "" {
						table.Access = "private"
					}
					// Parse index= directive
					if idxStr := dir.Params["index"]; idxStr != "" {
						idx, err := parseMultiColIndex(idxStr)
						if err != nil {
							return fmt.Errorf("table %s: %w", table.Name, err)
						}
						table.ExtraIndexes = append(table.ExtraIndexes, idx)
					}
					module.Tables = append(module.Tables, table)
				case "variant":
					variant := ParsedVariant{
						OfInterface: dir.Params["of"],
						Name:        dir.Params["name"],
						StructName:  ts.Name.Name,
						Fields:      fields,
					}
					if variant.Name == "" {
						variant.Name = ts.Name.Name
					}
					module.Variants = append(module.Variants, variant)
				}
			}
		}

		// Process //stdb:sumtype and //stdb:enum on interface/type declarations.
		for _, dir := range directives {
			switch dir.Kind {
			case "sumtype":
				sumType := ParsedSumType{
					InterfaceName: ts.Name.Name,
				}
				if scope := dir.Params["scope"]; scope != "" {
					sumType.Scope = strings.Split(scope, ".")
				}
				module.SumTypes = append(module.SumTypes, sumType)
			case "enum":
				variants := strings.Split(dir.Params["variants"], ",")
				enum := ParsedEnum{
					TypeName: ts.Name.Name,
					Variants: variants,
				}
				if scope := dir.Params["scope"]; scope != "" {
					enum.Scope = strings.Split(scope, ".")
				}
				module.Enums = append(module.Enums, enum)
			case "schedule":
				sched := ParsedSchedule{
					TableName:    dir.Params["table"],
					FunctionName: dir.Params["function"],
				}
				module.Schedules = append(module.Schedules, sched)
			case "rls":
				module.RLS = append(module.RLS, dir.Params["sql"])
			}
		}
	}
	return nil
}

// processFuncDecl processes function declarations for //stdb: directives.
func processFuncDecl(d *ast.FuncDecl, f *ast.File, module *ParsedModule, ctx fileCtx) error {
	if d.Doc == nil {
		return nil
	}

	directives := extractDirectives(d.Doc)
	if len(directives) == 0 {
		return nil
	}

	// Skip methods (they have a receiver).
	if d.Recv != nil {
		return nil
	}

	exported := d.Name.IsExported()

	for _, dir := range directives {
		switch dir.Kind {
		case "reducer":
			r := ParsedReducer{
				FuncName:   d.Name.Name,
				Name:       dir.Params["name"],
				Package:    ctx.packageName,
				ImportPath: ctx.importPath,
				RelDir:     ctx.relDir,
				Exported:   exported,
			}
			if r.Name == "" {
				r.Name = toSnakeCase(d.Name.Name)
			}
			r.Params = extractFuncParams(d.Type, true)
			r.HasError = funcReturnsError(d.Type)
			module.Reducers = append(module.Reducers, r)

		case "init":
			module.Lifecycle = append(module.Lifecycle, ParsedLifecycle{
				Kind:       "init",
				FuncName:   d.Name.Name,
				Package:    ctx.packageName,
				ImportPath: ctx.importPath,
				RelDir:     ctx.relDir,
				Exported:   exported,
			})

		case "connect":
			module.Lifecycle = append(module.Lifecycle, ParsedLifecycle{
				Kind:       "connect",
				FuncName:   d.Name.Name,
				Package:    ctx.packageName,
				ImportPath: ctx.importPath,
				RelDir:     ctx.relDir,
				Exported:   exported,
			})

		case "disconnect":
			module.Lifecycle = append(module.Lifecycle, ParsedLifecycle{
				Kind:       "disconnect",
				FuncName:   d.Name.Name,
				Package:    ctx.packageName,
				ImportPath: ctx.importPath,
				RelDir:     ctx.relDir,
				Exported:   exported,
			})

		case "procedure":
			p := ParsedProcedure{
				FuncName:   d.Name.Name,
				Name:       dir.Params["name"],
				Package:    ctx.packageName,
				ImportPath: ctx.importPath,
				RelDir:     ctx.relDir,
				Exported:   exported,
			}
			if p.Name == "" {
				p.Name = toSnakeCase(d.Name.Name)
			}
			p.Params = extractFuncParams(d.Type, true)
			p.ReturnType = extractReturnType(d.Type)
			module.Procedures = append(module.Procedures, p)

		case "view":
			v := ParsedView{
				FuncName:   d.Name.Name,
				Name:       dir.Params["name"],
				IsPublic:   dir.Params["public"] == "true",
				Package:    ctx.packageName,
				ImportPath: ctx.importPath,
				RelDir:     ctx.relDir,
				Exported:   exported,
			}
			if v.Name == "" {
				v.Name = toSnakeCase(d.Name.Name)
			}
			v.Params = extractFuncParams(d.Type, true)
			v.ReturnType = extractReturnType(d.Type)
			// Detect anonymous views by first param type.
			if d.Type.Params != nil && d.Type.Params.NumFields() > 0 {
				firstParam := d.Type.Params.List[0]
				firstType := typeExprToString(firstParam.Type)
				if strings.Contains(firstType, "AnonymousViewContext") {
					v.IsAnonymous = true
				}
			}
			module.Views = append(module.Views, v)

		case "schedule":
			sched := ParsedSchedule{
				TableName:    dir.Params["table"],
				FunctionName: dir.Params["function"],
			}
			module.Schedules = append(module.Schedules, sched)

		case "rls":
			sql := dir.RawValue
			module.RLS = append(module.RLS, sql)
		}
	}
	return nil
}

// directive represents a parsed //stdb: directive.
type directive struct {
	Kind     string            // "table", "reducer", "init", "sumtype", "enum", etc.
	Params   map[string]string // key=value pairs
	RawValue string            // raw text after the kind (for RLS SQL)
}

// extractDirectives finds all //stdb: comments in a comment group.
func extractDirectives(groups ...*ast.CommentGroup) []directive {
	var result []directive
	for _, cg := range groups {
		if cg == nil {
			continue
		}
		for _, c := range cg.List {
			text := strings.TrimPrefix(c.Text, "//")
			text = strings.TrimSpace(text)
			if !strings.HasPrefix(text, "stdb:") {
				continue
			}
			text = strings.TrimPrefix(text, "stdb:")
			dir := parseDirective(text)
			result = append(result, dir)
		}
	}
	return result
}

// parseDirective parses "kind key1=val1 key2=val2" or "rls SELECT * FROM ..."
func parseDirective(text string) directive {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return directive{}
	}

	d := directive{
		Kind:   parts[0],
		Params: make(map[string]string),
	}

	// For RLS, the entire rest is the SQL query.
	if d.Kind == "rls" {
		d.RawValue = strings.TrimSpace(strings.TrimPrefix(text, "rls"))
		d.Params["sql"] = d.RawValue
		return d
	}

	for _, part := range parts[1:] {
		if eqIdx := strings.Index(part, "="); eqIdx >= 0 {
			d.Params[part[:eqIdx]] = part[eqIdx+1:]
		}
	}
	return d
}

// collectComments merges multiple comment groups.
func collectComments(groups ...*ast.CommentGroup) *ast.CommentGroup {
	var allComments []*ast.Comment
	for _, g := range groups {
		if g != nil {
			allComments = append(allComments, g.List...)
		}
	}
	if len(allComments) == 0 {
		return nil
	}
	return &ast.CommentGroup{List: allComments}
}

// parseStructFields extracts fields from a struct type.
func parseStructFields(st *ast.StructType) []ParsedField {
	var fields []ParsedField
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			continue // embedded field, skip
		}
		for _, name := range f.Names {
			if !name.IsExported() {
				continue // skip unexported fields
			}
			field := ParsedField{
				GoName:    name.Name,
				GoType:    typeExprToString(f.Type),
				BsatnName: toSnakeCase(name.Name),
			}
			// Parse struct tags.
			if f.Tag != nil {
				tag := strings.Trim(f.Tag.Value, "`")
				field = parseFieldTag(field, tag)
			}
			fields = append(fields, field)
		}
	}
	return fields
}

// parseFieldTag parses stdb:"..." struct tags.
func parseFieldTag(field ParsedField, tag string) ParsedField {
	// Extract stdb:"..." value from the full tag string.
	const prefix = `stdb:"`
	idx := strings.Index(tag, prefix)
	if idx < 0 {
		return field
	}
	rest := tag[idx+len(prefix):]
	endIdx := strings.Index(rest, `"`)
	if endIdx < 0 {
		return field
	}
	stdbTag := rest[:endIdx]

	for _, part := range splitTagParts(stdbTag) {
		part = strings.TrimSpace(part)
		switch {
		case part == "primarykey":
			field.PrimaryKey = true
		case part == "autoinc":
			field.AutoInc = true
		case part == "unique":
			field.Unique = true
		case part == "index=btree":
			field.IndexBTree = true
		case part == "index=direct":
			field.IndexDirect = true
		case strings.HasPrefix(part, "default="):
			v := unquoteTagValue(strings.TrimPrefix(part, "default="))
			field.Default = &v
		}
	}
	return field
}

// splitTagParts splits a comma-separated stdb tag value into parts, treating
// commas inside single quotes as literal. This lets a default= value contain
// commas and spaces, e.g. stdb:"default='a, b, c'".
func splitTagParts(tag string) []string {
	var parts []string
	var sb strings.Builder
	inQuote := false
	for i := 0; i < len(tag); i++ {
		c := tag[i]
		switch {
		case c == '\\' && i+1 < len(tag):
			// Preserve the escape sequence verbatim; unquoteTagValue handles it.
			sb.WriteByte(c)
			sb.WriteByte(tag[i+1])
			i++
		case c == '\'':
			inQuote = !inQuote
			sb.WriteByte(c)
		case c == ',' && !inQuote:
			parts = append(parts, sb.String())
			sb.Reset()
		default:
			sb.WriteByte(c)
		}
	}
	parts = append(parts, sb.String())
	return parts
}

// unquoteTagValue strips a single pair of surrounding single quotes (if present)
// from a tag value and unescapes \' sequences. A bare value is returned as-is.
func unquoteTagValue(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		inner := v[1 : len(v)-1]
		return strings.ReplaceAll(inner, `\'`, `'`)
	}
	return v
}

// typeExprToString converts an AST type expression to a string representation.
func typeExprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return typeExprToString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + typeExprToString(t.X)
	case *ast.ArrayType:
		if t.Len != nil {
			return fmt.Sprintf("[%s]%s", typeExprToString(t.Len), typeExprToString(t.Elt))
		}
		return "[]" + typeExprToString(t.Elt)
	case *ast.MapType:
		return "map[" + typeExprToString(t.Key) + "]" + typeExprToString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.ParenExpr:
		return typeExprToString(t.X)
	case *ast.BasicLit:
		return t.Value
	default:
		return fmt.Sprintf("%T", expr)
	}
}

// extractFuncParams extracts parameters from a function type, skipping the first (context) param.
func extractFuncParams(ft *ast.FuncType, skipFirst bool) []ParsedParam {
	if ft.Params == nil {
		return nil
	}
	var params []ParsedParam
	fieldIdx := 0
	for _, field := range ft.Params.List {
		goType := typeExprToString(field.Type)
		if len(field.Names) == 0 {
			// Unnamed parameter.
			if skipFirst && fieldIdx == 0 {
				fieldIdx++
				continue
			}
			params = append(params, ParsedParam{
				Name:   fmt.Sprintf("arg_%d", len(params)),
				GoType: goType,
			})
			fieldIdx++
		} else {
			for _, name := range field.Names {
				if skipFirst && fieldIdx == 0 {
					fieldIdx++
					continue
				}
				paramName := name.Name
				if paramName == "_" {
					paramName = fmt.Sprintf("arg_%d", len(params))
				}
				params = append(params, ParsedParam{
					Name:   paramName,
					GoType: goType,
				})
				fieldIdx++
			}
		}
	}
	return params
}

// extractReturnType extracts the return type from a function type as a string.
func extractReturnType(ft *ast.FuncType) string {
	if ft.Results == nil || ft.Results.NumFields() == 0 {
		return ""
	}
	// If first return is error, there's no data return type.
	first := ft.Results.List[0]
	rt := typeExprToString(first.Type)
	if rt == "error" {
		return ""
	}
	return rt
}

// funcReturnsError checks if a function returns error as its last result.
func funcReturnsError(ft *ast.FuncType) bool {
	if ft.Results == nil || ft.Results.NumFields() == 0 {
		return false
	}
	// Check the last return value.
	lastField := ft.Results.List[ft.Results.NumFields()-1]
	return typeExprToString(lastField.Type) == "error"
}

// toSnakeCase converts a PascalCase or camelCase string to snake_case.
func toSnakeCase(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				if unicode.IsLower(prev) || unicode.IsDigit(prev) {
					b.WriteRune('_')
				} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
					b.WriteRune('_')
				}
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// parseMultiColIndex parses an index specification like "name:col0,col1,col2".
func parseMultiColIndex(spec string) (ParsedMultiColIndex, error) {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) != 2 {
		return ParsedMultiColIndex{}, fmt.Errorf("invalid index spec %q: expected name:col0,col1", spec)
	}
	idx := ParsedMultiColIndex{Name: parts[0]}
	for _, colStr := range strings.Split(parts[1], ",") {
		var col uint16
		if _, err := fmt.Sscanf(colStr, "%d", &col); err != nil {
			return ParsedMultiColIndex{}, fmt.Errorf("invalid column %q in index spec", colStr)
		}
		idx.Columns = append(idx.Columns, col)
	}
	return idx, nil
}
