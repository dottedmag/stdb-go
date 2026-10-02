package servergen

import (
	"fmt"
	"strings"

	"github.com/dottedmag/stdb-go/internal/parser"
)

// AnalyzedModule is the fully resolved module ready for code generation.
type AnalyzedModule struct {
	PackageName  string // root package name (usually "main")
	ModulePath   string
	RootDir      string
	MultiPackage bool

	Tables     []AnalyzedTable
	Reducers   []AnalyzedReducer
	Lifecycle  []AnalyzedLifecycle
	Procedures []AnalyzedProcedure
	Views      []AnalyzedView
	SumTypes   []AnalyzedSumType
	Enums      []AnalyzedEnum
	Schedules  []parser.ParsedSchedule
	RLS        []string

	// Types maps Go type names to their analyzed type info.
	Types map[string]*AnalyzedType

	// TypeOrder is the order in which types appear in the typespace.
	// This includes all structs, sum types, and enums referenced by tables or other types.
	TypeOrder []string

	// Packages lists packages under the module (from parser).
	Packages []parser.ParsedPackage
}

// CallExpr returns the Go expression used to invoke a function defined in
// Package/ImportPath from the root generated package (bare name or pkg.Func).
func CallExpr(funcName, pkgName, importPath string, isRoot bool, multi bool) string {
	if !multi || isRoot || importPath == "" {
		return funcName
	}
	// Import alias defaults to the last path element, which matches the package
	// clause for conventional layouts (…/combat → combat).
	alias := pkgName
	if alias == "" {
		alias = pathBase(importPath)
	}
	return alias + "." + funcName
}

func pathBase(importPath string) string {
	if i := strings.LastIndex(importPath, "/"); i >= 0 {
		return importPath[i+1:]
	}
	return importPath
}

// AnalyzedType represents a fully resolved Go type.
type AnalyzedType struct {
	Name           string
	Kind           TypeKind
	Fields         []AnalyzedField   // for structs
	Variants       []AnalyzedVariant // for sum types
	EnumVariants   []string          // for simple enums
	Scope          []string          // optional namespace scope
	TypespaceIdx   int               // index in the typespace
	CustomOrdering bool              // whether type needs custom ordering (has btree index)
	Package        string
	ImportPath     string
	RelDir         string
}

// TypeKind classifies a Go type for BSATN encoding.
type TypeKind int

const (
	TypeKindPrimitive TypeKind = iota
	TypeKindSpecial            // Identity, ConnectionId, Timestamp, etc.
	TypeKindStruct
	TypeKindSumType
	TypeKindSimpleEnum
	TypeKindSlice
	TypeKindPointer // *T -> Option<T>
	TypeKindByteSlice
)

// AnalyzedField is a struct field with resolved type info.
type AnalyzedField struct {
	GoName      string
	BsatnName   string
	GoType      string
	AlgType     AlgType // resolved algebraic type
	PrimaryKey  bool
	AutoInc     bool
	Unique      bool
	IndexBTree  bool
	IndexDirect bool
	ColIndex    uint16
	Default     *string // from default=<value>; nil means "no default"
}

// AlgType represents an algebraic type in the typespace.
type AlgType struct {
	Kind     AlgKind
	Ref      int      // typespace index for TypeRef
	ElemType *AlgType // for Array/Option
	TypeName string   // for special types or struct references
}

// AlgKind classifies an algebraic type.
type AlgKind int

const (
	AlgKindBool AlgKind = iota
	AlgKindU8
	AlgKindU16
	AlgKindU32
	AlgKindU64
	AlgKindU128
	AlgKindU256
	AlgKindI8
	AlgKindI16
	AlgKindI32
	AlgKindI64
	AlgKindI128
	AlgKindI256
	AlgKindF32
	AlgKindF64
	AlgKindString
	AlgKindBytes  // []byte
	AlgKindArray  // []T (non-byte)
	AlgKindOption // *T
	AlgKindRef    // TypeRef to typespace
	AlgKindIdentity
	AlgKindConnectionId
	AlgKindTimestamp
	AlgKindTimeDuration
	AlgKindScheduleAt
	AlgKindUuid
)

// AnalyzedVariant is a sum type variant with resolved type info.
type AnalyzedVariant struct {
	Name       string
	StructName string
	Fields     []AnalyzedField
	Tag        uint8
}

// AnalyzedTable has fully resolved fields and metadata.
type AnalyzedTable struct {
	Name         string
	Access       string
	IsEvent      bool
	StructName   string // bare Go type name
	Fields       []AnalyzedField
	ExtraIndexes []parser.ParsedMultiColIndex
	TypespaceRef int    // typespace index for this table's struct
	VarName      string // Go variable name for the table accessor (e.g., "EntityTable")
	Package      string
	ImportPath   string
	RelDir       string
	// QualifiedStruct is how root code refers to the type ("Character" or "schema.Character").
	QualifiedStruct string
}

// AnalyzedReducer has fully resolved parameters.
type AnalyzedReducer struct {
	Name     string
	FuncName string
	// CallName is the expression the root dispatcher uses (FuncName or pkg.FuncName).
	CallName string
	Params   []AnalyzedParam
	HasError bool
	ID       uint32 // reducer ID (index in combined list)
	Package  string
	ImportPath string
	RelDir   string
}

// AnalyzedParam is a resolved parameter.
type AnalyzedParam struct {
	Name    string
	GoType  string
	AlgType AlgType
}

// AnalyzedLifecycle is a lifecycle reducer.
type AnalyzedLifecycle struct {
	Kind       string // "init", "connect", "disconnect"
	FuncName   string
	CallName   string
	ID         uint32 // reducer ID in combined list
	Package    string
	ImportPath string
	RelDir     string
}

// AnalyzedProcedure has fully resolved parameters and return type.
type AnalyzedProcedure struct {
	Name         string
	FuncName     string
	CallName     string
	Params       []AnalyzedParam
	ReturnType   *AlgType // nil if void
	ReturnGoType string
	ID           uint32
	Package      string
	ImportPath   string
	RelDir       string
}

// AnalyzedView has fully resolved parameters and return type.
type AnalyzedView struct {
	Name         string
	FuncName     string
	CallName     string
	IsPublic     bool
	IsAnonymous  bool
	Params       []AnalyzedParam
	ReturnType   AlgType
	ReturnGoType string
	ID           uint32 // index within auth or anon list
	Package      string
	ImportPath   string
	RelDir       string
}

// AnalyzedSumType has resolved variants.
type AnalyzedSumType struct {
	InterfaceName string
	Scope         []string
	Variants      []AnalyzedVariant
	TypespaceIdx  int
}

// AnalyzedEnum has resolved variant names.
type AnalyzedEnum struct {
	TypeName     string
	Variants     []string
	Scope        []string
	TypespaceIdx int
}

// Analyze resolves all types and validates the parsed module.
func Analyze(parsed *parser.ParsedModule) (*AnalyzedModule, error) {
	a := &analyzer{
		parsed:    parsed,
		types:     make(map[string]*AnalyzedType),
		typeOrder: nil,
		nextIdx:   0,
	}

	// Pass 1: Register all enums and sum types first (they need to be in the typespace
	// before we can resolve struct fields that reference them).
	for _, enum := range parsed.Enums {
		if err := a.registerEnum(enum); err != nil {
			return nil, err
		}
	}
	for _, st := range parsed.SumTypes {
		if err := a.registerSumType(st); err != nil {
			return nil, err
		}
	}

	// Pass 2: Resolve all table struct types. This recursively resolves nested structs.
	for _, table := range parsed.Tables {
		if _, err := a.resolveStructType(table.StructName); err != nil {
			return nil, fmt.Errorf("table %s: %w", table.Name, err)
		}
	}

	// Pass 3: Resolve reducer/procedure/view parameter and return types.
	// (These may reference struct types not in tables.)
	for _, r := range parsed.Reducers {
		for _, p := range r.Params {
			if _, err := a.resolveGoType(p.GoType); err != nil {
				return nil, fmt.Errorf("reducer %s param %s: %w", r.Name, p.Name, err)
			}
		}
	}
	for _, p := range parsed.Procedures {
		for _, param := range p.Params {
			if _, err := a.resolveGoType(param.GoType); err != nil {
				return nil, fmt.Errorf("procedure %s param %s: %w", p.Name, param.Name, err)
			}
		}
		if p.ReturnType != "" {
			if _, err := a.resolveGoType(p.ReturnType); err != nil {
				return nil, fmt.Errorf("procedure %s return: %w", p.Name, err)
			}
		}
	}
	for _, v := range parsed.Views {
		for _, param := range v.Params {
			if _, err := a.resolveGoType(param.GoType); err != nil {
				return nil, fmt.Errorf("view %s param %s: %w", v.Name, param.Name, err)
			}
		}
		if v.ReturnType != "" {
			if _, err := a.resolveGoType(v.ReturnType); err != nil {
				return nil, fmt.Errorf("view %s return: %w", v.Name, err)
			}
		}
	}

	// Build the analyzed module.
	module := &AnalyzedModule{
		PackageName:  parsed.PackageName,
		ModulePath:   parsed.ModulePath,
		RootDir:      parsed.RootDir,
		MultiPackage: parsed.MultiPackage,
		Types:        a.types,
		TypeOrder:    a.typeOrder,
		Schedules:    parsed.Schedules,
		RLS:          parsed.RLS,
		Packages:     parsed.Packages,
	}

	// Attach package ownership onto AnalyzedType for dual codegen.
	for name, st := range parsed.Structs {
		if at, ok := a.types[name]; ok {
			at.Package = st.Package
			at.ImportPath = st.ImportPath
			at.RelDir = st.RelDir
		}
	}

	// Mark tables that have btree indexes as needing custom ordering.
	for _, table := range parsed.Tables {
		hasBTree := false
		for _, f := range table.Fields {
			if f.PrimaryKey || f.Unique || f.IndexBTree {
				hasBTree = true
				break
			}
		}
		if len(table.ExtraIndexes) > 0 {
			hasBTree = true
		}
		if hasBTree {
			if at, ok := a.types[table.StructName]; ok {
				at.CustomOrdering = true
			}
		}
	}

	// Build analyzed tables.
	for _, table := range parsed.Tables {
		at := AnalyzedTable{
			Name:         table.Name,
			Access:       table.Access,
			IsEvent:      table.IsEvent,
			StructName:   table.StructName,
			ExtraIndexes: table.ExtraIndexes,
			VarName:      ToPascalCase(table.Name) + "Table",
			Package:      table.Package,
			ImportPath:   table.ImportPath,
			RelDir:       table.RelDir,
		}
		at.QualifiedStruct = qualifyTypeName(table.StructName, table.Package, table.ImportPath, table.RelDir == "", parsed.MultiPackage)
		if typeInfo, ok := a.types[table.StructName]; ok {
			at.TypespaceRef = typeInfo.TypespaceIdx
			at.Fields = typeInfo.Fields
		}
		module.Tables = append(module.Tables, at)
	}

	// Build analyzed reducers with sequential IDs.
	var reducerID uint32
	for _, r := range parsed.Reducers {
		ar := AnalyzedReducer{
			Name:       r.Name,
			FuncName:   r.FuncName,
			CallName:   CallExpr(r.FuncName, r.Package, r.ImportPath, r.RelDir == "", parsed.MultiPackage),
			HasError:   r.HasError,
			ID:         reducerID,
			Package:    r.Package,
			ImportPath: r.ImportPath,
			RelDir:     r.RelDir,
		}
		for _, p := range r.Params {
			algType, _ := a.resolveGoType(p.GoType)
			ar.Params = append(ar.Params, AnalyzedParam{
				Name:    p.Name,
				GoType:  p.GoType,
				AlgType: algType,
			})
		}
		module.Reducers = append(module.Reducers, ar)
		reducerID++
	}

	// Build analyzed lifecycle reducers (IDs continue from reducers).
	for _, lc := range parsed.Lifecycle {
		alc := AnalyzedLifecycle{
			Kind:       lc.Kind,
			FuncName:   lc.FuncName,
			CallName:   CallExpr(lc.FuncName, lc.Package, lc.ImportPath, lc.RelDir == "", parsed.MultiPackage),
			ID:         reducerID,
			Package:    lc.Package,
			ImportPath: lc.ImportPath,
			RelDir:     lc.RelDir,
		}
		module.Lifecycle = append(module.Lifecycle, alc)
		reducerID++
	}

	// Build analyzed procedures.
	for i, p := range parsed.Procedures {
		ap := AnalyzedProcedure{
			Name:         p.Name,
			FuncName:     p.FuncName,
			CallName:     CallExpr(p.FuncName, p.Package, p.ImportPath, p.RelDir == "", parsed.MultiPackage),
			ReturnGoType: p.ReturnType,
			ID:           uint32(i),
			Package:      p.Package,
			ImportPath:   p.ImportPath,
			RelDir:       p.RelDir,
		}
		for _, param := range p.Params {
			algType, _ := a.resolveGoType(param.GoType)
			ap.Params = append(ap.Params, AnalyzedParam{
				Name:    param.Name,
				GoType:  param.GoType,
				AlgType: algType,
			})
		}
		if p.ReturnType != "" {
			algType, _ := a.resolveGoType(p.ReturnType)
			ap.ReturnType = &algType
		}
		module.Procedures = append(module.Procedures, ap)
	}

	// Build analyzed views with separate auth/anon indexing.
	var authIdx, anonIdx uint32
	for _, v := range parsed.Views {
		av := AnalyzedView{
			Name:         v.Name,
			FuncName:     v.FuncName,
			CallName:     CallExpr(v.FuncName, v.Package, v.ImportPath, v.RelDir == "", parsed.MultiPackage),
			IsPublic:     v.IsPublic,
			IsAnonymous:  v.IsAnonymous,
			ReturnGoType: v.ReturnType,
			Package:      v.Package,
			ImportPath:   v.ImportPath,
			RelDir:       v.RelDir,
		}
		if v.IsAnonymous {
			av.ID = anonIdx
			anonIdx++
		} else {
			av.ID = authIdx
			authIdx++
		}
		for _, param := range v.Params {
			algType, _ := a.resolveGoType(param.GoType)
			av.Params = append(av.Params, AnalyzedParam{
				Name:    param.Name,
				GoType:  param.GoType,
				AlgType: algType,
			})
		}
		if v.ReturnType != "" {
			algType, _ := a.resolveGoType(v.ReturnType)
			av.ReturnType = algType
		}
		module.Views = append(module.Views, av)
	}

	// Build analyzed sum types.
	for _, st := range parsed.SumTypes {
		typeInfo := a.types[st.InterfaceName]
		ast := AnalyzedSumType{
			InterfaceName: st.InterfaceName,
			Scope:         st.Scope,
			Variants:      typeInfo.Variants,
			TypespaceIdx:  typeInfo.TypespaceIdx,
		}
		module.SumTypes = append(module.SumTypes, ast)
	}

	// Build analyzed enums.
	for _, e := range parsed.Enums {
		typeInfo := a.types[e.TypeName]
		ae := AnalyzedEnum{
			TypeName:     e.TypeName,
			Variants:     e.Variants,
			Scope:        e.Scope,
			TypespaceIdx: typeInfo.TypespaceIdx,
		}
		module.Enums = append(module.Enums, ae)
	}

	return module, nil
}

// qualifyTypeName returns how the root package should refer to a type defined
// in another package under multi-package mode.
func qualifyTypeName(typeName, pkgName, importPath string, isRoot, multi bool) string {
	if !multi || isRoot {
		return typeName
	}
	alias := pkgName
	if alias == "" {
		alias = pathBase(importPath)
	}
	return alias + "." + typeName
}

// analyzer tracks type resolution state.
type analyzer struct {
	parsed    *parser.ParsedModule
	types     map[string]*AnalyzedType
	typeOrder []string
	nextIdx   int
}

// reserveTypespaceSlot reserves an index in the typespace for a type.
func (a *analyzer) reserveTypespaceSlot(name string) int {
	idx := a.nextIdx
	a.nextIdx++
	a.typeOrder = append(a.typeOrder, name)
	return idx
}

// registerEnum registers a simple enum type.
func (a *analyzer) registerEnum(enum parser.ParsedEnum) error {
	idx := a.reserveTypespaceSlot(enum.TypeName)
	a.types[enum.TypeName] = &AnalyzedType{
		Name:           enum.TypeName,
		Kind:           TypeKindSimpleEnum,
		EnumVariants:   enum.Variants,
		Scope:          enum.Scope,
		TypespaceIdx:   idx,
		CustomOrdering: true, // All sum types (enums) need custom ordering in SATS
	}
	return nil
}

// registerSumType registers a sum type with its variants.
func (a *analyzer) registerSumType(st parser.ParsedSumType) error {
	idx := a.reserveTypespaceSlot(st.InterfaceName)

	// Find all variants for this sum type.
	var variants []AnalyzedVariant
	for i, v := range a.parsed.Variants {
		if v.OfInterface != st.InterfaceName {
			continue
		}
		av := AnalyzedVariant{
			Name:       v.Name,
			StructName: v.StructName,
			Tag:        uint8(len(variants)),
		}
		// Resolve variant fields.
		for j, f := range v.Fields {
			algType, err := a.resolveGoType(f.GoType)
			if err != nil {
				return fmt.Errorf("sum type %s variant %s field %s: %w", st.InterfaceName, v.Name, f.GoName, err)
			}
			av.Fields = append(av.Fields, AnalyzedField{
				GoName:    f.GoName,
				BsatnName: f.BsatnName,
				GoType:    f.GoType,
				AlgType:   algType,
				ColIndex:  uint16(j),
			})
		}
		variants = append(variants, av)
		_ = i
	}

	a.types[st.InterfaceName] = &AnalyzedType{
		Name:           st.InterfaceName,
		Kind:           TypeKindSumType,
		Variants:       variants,
		Scope:          st.Scope,
		TypespaceIdx:   idx,
		CustomOrdering: true, // All sum types need custom ordering in SATS
	}
	return nil
}

// resolveStructType resolves a struct type, adding it to the typespace if needed.
func (a *analyzer) resolveStructType(name string) (*AnalyzedType, error) {
	// Already resolved?
	if t, ok := a.types[name]; ok {
		return t, nil
	}

	// Find the struct definition.
	s, ok := a.parsed.Structs[name]
	if !ok {
		return nil, fmt.Errorf("unknown struct type %q", name)
	}

	// Reserve a slot first (handles recursive types).
	idx := a.reserveTypespaceSlot(name)
	at := &AnalyzedType{
		Name:           name,
		Kind:           TypeKindStruct,
		TypespaceIdx:   idx,
		CustomOrdering: true, // All product types need custom ordering (field order is declaration order, not alphabetical)
	}
	a.types[name] = at

	// Resolve fields.
	for i, f := range s.Fields {
		algType, err := a.resolveGoType(f.GoType)
		if err != nil {
			return nil, fmt.Errorf("struct %s field %s: %w", name, f.GoName, err)
		}
		if f.Default != nil && f.AutoInc {
			return nil, fmt.Errorf("struct %s field %s: a default value cannot be combined with autoinc (the sequence supplies values)", name, f.GoName)
		}
		at.Fields = append(at.Fields, AnalyzedField{
			GoName:      f.GoName,
			BsatnName:   f.BsatnName,
			GoType:      f.GoType,
			AlgType:     algType,
			PrimaryKey:  f.PrimaryKey,
			AutoInc:     f.AutoInc,
			Unique:      f.Unique,
			IndexBTree:  f.IndexBTree,
			IndexDirect: f.IndexDirect,
			ColIndex:    uint16(i),
			Default:     f.Default,
		})
	}

	return at, nil
}

// resolveGoType resolves a Go type string to an AlgType.
func (a *analyzer) resolveGoType(goType string) (AlgType, error) {
	// Pointer -> Option
	if strings.HasPrefix(goType, "*") {
		inner, err := a.resolveGoType(goType[1:])
		if err != nil {
			return AlgType{}, err
		}
		return AlgType{Kind: AlgKindOption, ElemType: &inner, TypeName: goType}, nil
	}

	// Slice
	if strings.HasPrefix(goType, "[]") {
		elemType := goType[2:]
		if elemType == "byte" || elemType == "uint8" {
			return AlgType{Kind: AlgKindBytes, TypeName: goType}, nil
		}
		inner, err := a.resolveGoType(elemType)
		if err != nil {
			return AlgType{}, err
		}
		return AlgType{Kind: AlgKindArray, ElemType: &inner, TypeName: goType}, nil
	}

	// Primitives
	switch goType {
	case "bool":
		return AlgType{Kind: AlgKindBool}, nil
	case "uint8", "byte":
		return AlgType{Kind: AlgKindU8}, nil
	case "uint16":
		return AlgType{Kind: AlgKindU16}, nil
	case "uint32":
		return AlgType{Kind: AlgKindU32}, nil
	case "uint64":
		return AlgType{Kind: AlgKindU64}, nil
	case "int8":
		return AlgType{Kind: AlgKindI8}, nil
	case "int16":
		return AlgType{Kind: AlgKindI16}, nil
	case "int32":
		return AlgType{Kind: AlgKindI32}, nil
	case "int64":
		return AlgType{Kind: AlgKindI64}, nil
	case "float32":
		return AlgType{Kind: AlgKindF32}, nil
	case "float64":
		return AlgType{Kind: AlgKindF64}, nil
	case "string":
		return AlgType{Kind: AlgKindString}, nil
	}

	// Special types (with or without package prefix)
	switch goType {
	case "types.Identity", "Identity":
		return AlgType{Kind: AlgKindIdentity, TypeName: "types.Identity"}, nil
	case "types.ConnectionId", "ConnectionId":
		return AlgType{Kind: AlgKindConnectionId, TypeName: "types.ConnectionId"}, nil
	case "types.Timestamp", "Timestamp":
		return AlgType{Kind: AlgKindTimestamp, TypeName: "types.Timestamp"}, nil
	case "types.TimeDuration", "TimeDuration":
		return AlgType{Kind: AlgKindTimeDuration, TypeName: "types.TimeDuration"}, nil
	case "types.ScheduleAt", "ScheduleAt":
		return AlgType{Kind: AlgKindScheduleAt, TypeName: "types.ScheduleAt"}, nil
	case "types.Uint128", "Uint128":
		return AlgType{Kind: AlgKindU128, TypeName: "types.Uint128"}, nil
	case "types.Uint256", "Uint256":
		return AlgType{Kind: AlgKindU256, TypeName: "types.Uint256"}, nil
	case "types.Int128", "Int128":
		return AlgType{Kind: AlgKindI128, TypeName: "types.Int128"}, nil
	case "types.Int256", "Int256":
		return AlgType{Kind: AlgKindI256, TypeName: "types.Int256"}, nil
	case "types.Uuid", "Uuid":
		return AlgType{Kind: AlgKindUuid, TypeName: "types.Uuid"}, nil
	}

	// Registered type (enum, sum type, or struct) — bare name.
	if t, ok := a.types[goType]; ok {
		return AlgType{Kind: AlgKindRef, Ref: t.TypespaceIdx, TypeName: goType}, nil
	}

	// Try to resolve as a struct from the parsed module.
	if _, ok := a.parsed.Structs[goType]; ok {
		t, err := a.resolveStructType(goType)
		if err != nil {
			return AlgType{}, err
		}
		return AlgType{Kind: AlgKindRef, Ref: t.TypespaceIdx, TypeName: goType}, nil
	}

	// package.Type selector (multi-package: combat uses schema.AttackReq).
	// TypeName is the bare product name so StdbReadAttackReq / schema.StdbReadAttackReq
	// resolve correctly; the Go param type keeps the qualified form from the signature.
	if i := strings.LastIndex(goType, "."); i >= 0 {
		bare := goType[i+1:]
		if t, ok := a.types[bare]; ok {
			return AlgType{Kind: AlgKindRef, Ref: t.TypespaceIdx, TypeName: bare}, nil
		}
		if _, ok := a.parsed.Structs[bare]; ok {
			t, err := a.resolveStructType(bare)
			if err != nil {
				return AlgType{}, err
			}
			return AlgType{Kind: AlgKindRef, Ref: t.TypespaceIdx, TypeName: bare}, nil
		}
		if underlying, ok := a.parsed.TypeAliases[bare]; ok {
			return a.resolveGoType(underlying)
		}
	}

	// Resolve type aliases (e.g., TestAlias = TestA).
	if underlying, ok := a.parsed.TypeAliases[goType]; ok {
		return a.resolveGoType(underlying)
	}

	return AlgType{}, fmt.Errorf("unsupported type %q", goType)
}

// ToPascalCase converts a snake_case string to PascalCase.
func ToPascalCase(s string) string {
	parts := strings.Split(s, "_")
	var b strings.Builder
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		runes := []rune(part)
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		b.WriteString(string(runes))
	}
	return b.String()
}
