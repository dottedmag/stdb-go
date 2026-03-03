package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// ParsedModule is the intermediate representation produced by parsing.
type ParsedModule struct {
	PackageName string
	Tables      []ParsedTable
	Reducers    []ParsedReducer
	Lifecycle   []ParsedLifecycle
	Procedures  []ParsedProcedure
	Views       []ParsedView
	SumTypes    []ParsedSumType
	Enums       []ParsedEnum
	Variants    []ParsedVariant
	Schedules   []ParsedSchedule
	RLS         []string

	// All struct definitions found in the package (for type resolution).
	Structs map[string]*ParsedStruct

	// Type aliases: name → underlying type name (e.g., TestAlias → TestA).
	TypeAliases map[string]string
}

// ParsedStruct represents a struct type definition found in the package.
type ParsedStruct struct {
	Name   string
	Fields []ParsedField
}

// ParsedTable represents a table declared via //stdb:table directive.
type ParsedTable struct {
	Name       string // from name= directive
	Access     string // "public" or "private"
	IsEvent    bool   // event=true
	StructName string // Go struct name
	Fields     []ParsedField
	ExtraIndexes []ParsedMultiColIndex
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
}

// ParsedReducer represents a reducer declared via //stdb:reducer directive.
type ParsedReducer struct {
	Name     string        // from name= or auto snake_case of FuncName
	FuncName string        // Go function name from AST
	Params   []ParsedParam // after context param, names from Go func signature
	HasError bool          // returns error
}

// ParsedParam is a reducer/procedure/view parameter.
type ParsedParam struct {
	Name   string // from Go function signature
	GoType string // Go type string
}

// ParsedLifecycle represents a lifecycle reducer declared via //stdb:init, //stdb:connect, or //stdb:disconnect.
type ParsedLifecycle struct {
	Kind     string // "init", "connect", or "disconnect"
	FuncName string // Go function name from AST
}

// ParsedProcedure represents a procedure declared via //stdb:procedure directive.
type ParsedProcedure struct {
	Name       string
	FuncName   string
	Params     []ParsedParam
	ReturnType string // Go type of return value, empty if void
}

// ParsedView represents a view declared via //stdb:view directive.
type ParsedView struct {
	Name        string
	FuncName    string
	IsPublic    bool
	IsAnonymous bool
	Params      []ParsedParam
	ReturnType  string // Go type string (e.g., "*Player", "[]Player")
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

// parseDirectory parses all .go files in the directory and extracts //stdb: directives.
func parseDirectory(dir string) (*ParsedModule, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	fset := token.NewFileSet()
	module := &ParsedModule{
		Structs:     make(map[string]*ParsedStruct),
		TypeAliases: make(map[string]string),
	}

	for _, entry := range entries {
		name := entry.Name()
		// Skip generated files, test files, directories
		if entry.IsDir() ||
			!strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_generated.go") ||
			strings.HasSuffix(name, "_test.go") ||
			name == "main.go" {
			continue
		}

		filePath := filepath.Join(dir, name)
		f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}

		if module.PackageName == "" {
			module.PackageName = f.Name.Name
		}

		if err := processFile(f, module); err != nil {
			return nil, fmt.Errorf("process %s: %w", name, err)
		}
	}

	return module, nil
}

// processFile extracts //stdb: directives from a parsed Go file.
func processFile(f *ast.File, module *ParsedModule) error {
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if err := processGenDecl(d, f, module); err != nil {
				return err
			}
		case *ast.FuncDecl:
			if err := processFuncDecl(d, f, module); err != nil {
				return err
			}
		}
	}
	return nil
}

// processGenDecl processes type/const declarations for //stdb: directives.
func processGenDecl(d *ast.GenDecl, f *ast.File, module *ParsedModule) error {
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
			module.Structs[ts.Name.Name] = &ParsedStruct{
				Name:   ts.Name.Name,
				Fields: fields,
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
func processFuncDecl(d *ast.FuncDecl, f *ast.File, module *ParsedModule) error {
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

	for _, dir := range directives {
		switch dir.Kind {
		case "reducer":
			r := ParsedReducer{
				FuncName: d.Name.Name,
				Name:     dir.Params["name"],
			}
			if r.Name == "" {
				r.Name = toSnakeCase(d.Name.Name)
			}
			r.Params = extractFuncParams(d.Type, true)
			r.HasError = funcReturnsError(d.Type)
			module.Reducers = append(module.Reducers, r)

		case "init":
			module.Lifecycle = append(module.Lifecycle, ParsedLifecycle{
				Kind:     "init",
				FuncName: d.Name.Name,
			})

		case "connect":
			module.Lifecycle = append(module.Lifecycle, ParsedLifecycle{
				Kind:     "connect",
				FuncName: d.Name.Name,
			})

		case "disconnect":
			module.Lifecycle = append(module.Lifecycle, ParsedLifecycle{
				Kind:     "disconnect",
				FuncName: d.Name.Name,
			})

		case "procedure":
			p := ParsedProcedure{
				FuncName: d.Name.Name,
				Name:     dir.Params["name"],
			}
			if p.Name == "" {
				p.Name = toSnakeCase(d.Name.Name)
			}
			p.Params = extractFuncParams(d.Type, true)
			p.ReturnType = extractReturnType(d.Type)
			module.Procedures = append(module.Procedures, p)

		case "view":
			v := ParsedView{
				FuncName: d.Name.Name,
				Name:     dir.Params["name"],
				IsPublic: dir.Params["public"] == "true",
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

	for _, part := range strings.Split(stdbTag, ",") {
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
		}
	}
	return field
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

// parseMultiColIndex parses an index specification like "name:col0,col1,col2".
func parseMultiColIndex(spec string) (ParsedMultiColIndex, error) {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) != 2 {
		return ParsedMultiColIndex{}, fmt.Errorf("invalid index spec %q: expected name:col0,col1,...", spec)
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
