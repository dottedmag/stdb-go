package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errWriteFile is a test helper that writes content to a file in the given directory.
func errWriteFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
}

// ---------------------------------------------------------------------------
// Parser error tests
// ---------------------------------------------------------------------------

func TestParseError_InvalidGoSource(t *testing.T) {
	dir := t.TempDir()
	errWriteFile(t, dir, "bad.go", "package main\nfunc {broken")

	_, err := parseDirectory(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestParseError_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	parsed, err := parseDirectory(dir)
	require.NoError(t, err)
	assert.Empty(t, parsed.Tables)
	assert.Empty(t, parsed.Reducers)
	assert.Empty(t, parsed.Lifecycle)
	assert.Empty(t, parsed.Procedures)
	assert.Empty(t, parsed.Views)
}

func TestParseError_NonExistentDir(t *testing.T) {
	_, err := parseDirectory("/tmp/nonexistent-stdb-gen-test-dir-that-does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read dir")
}

func TestParseError_InvalidMultiColIndex(t *testing.T) {
	dir := t.TempDir()
	errWriteFile(t, dir, "tables.go", `package test

//stdb:table name=test access=public index=myidx:abc
type Test struct {
	Id   uint64 `+"`"+`stdb:"primarykey"`+"`"+`
	Name string
}
`)

	_, err := parseDirectory(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid column")
}

func TestParseError_InvalidMultiColIndexMissingColon(t *testing.T) {
	dir := t.TempDir()
	errWriteFile(t, dir, "tables.go", `package test

//stdb:table name=test access=public index=badformat
type Test struct {
	Id   uint64
	Name string
}
`)

	_, err := parseDirectory(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid index spec")
}

// ---------------------------------------------------------------------------
// Analyzer error tests
// ---------------------------------------------------------------------------

func TestAnalyzeError_UnknownFieldType(t *testing.T) {
	parsed := &ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*ParsedStruct),
		TypeAliases: make(map[string]string),
	}
	parsed.Structs["BadStruct"] = &ParsedStruct{
		Name: "BadStruct",
		Fields: []ParsedField{
			{GoName: "Data", GoType: "map[string]int", BsatnName: "data"},
		},
	}
	parsed.Tables = []ParsedTable{{
		Name:       "test",
		Access:     "public",
		StructName: "BadStruct",
		Fields: []ParsedField{
			{GoName: "Data", GoType: "map[string]int", BsatnName: "data"},
		},
	}}

	_, err := analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_UnknownStructRef(t *testing.T) {
	parsed := &ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*ParsedStruct),
		TypeAliases: make(map[string]string),
	}
	parsed.Structs["MyStruct"] = &ParsedStruct{
		Name: "MyStruct",
		Fields: []ParsedField{
			{GoName: "Ref", GoType: "NonExistent", BsatnName: "ref"},
		},
	}
	parsed.Tables = []ParsedTable{{
		Name:       "test",
		Access:     "public",
		StructName: "MyStruct",
		Fields: []ParsedField{
			{GoName: "Ref", GoType: "NonExistent", BsatnName: "ref"},
		},
	}}

	_, err := analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")
	assert.Contains(t, err.Error(), "NonExistent")
}

func TestAnalyzeError_ReducerBadParam(t *testing.T) {
	parsed := &ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*ParsedStruct),
		TypeAliases: make(map[string]string),
		Reducers: []ParsedReducer{{
			Name:     "bad_reducer",
			FuncName: "BadReducer",
			Params: []ParsedParam{
				{Name: "ch", GoType: "chan int"},
			},
		}},
	}

	_, err := analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reducer bad_reducer")
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_ReducerBadParamMap(t *testing.T) {
	parsed := &ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*ParsedStruct),
		TypeAliases: make(map[string]string),
		Reducers: []ParsedReducer{{
			Name:     "map_reducer",
			FuncName: "MapReducer",
			Params: []ParsedParam{
				{Name: "data", GoType: "map[string]string"},
			},
		}},
	}

	_, err := analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reducer map_reducer")
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_ProcedureBadReturn(t *testing.T) {
	parsed := &ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*ParsedStruct),
		TypeAliases: make(map[string]string),
		Procedures: []ParsedProcedure{{
			Name:       "bad_proc",
			FuncName:   "BadProc",
			ReturnType: "chan int",
		}},
	}

	_, err := analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "procedure bad_proc return")
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_ProcedureParamBad(t *testing.T) {
	parsed := &ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*ParsedStruct),
		TypeAliases: make(map[string]string),
		Procedures: []ParsedProcedure{{
			Name:     "bad_proc",
			FuncName: "BadProc",
			Params: []ParsedParam{
				{Name: "arg", GoType: "func()"},
			},
		}},
	}

	_, err := analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "procedure bad_proc param arg")
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_ViewBadReturn(t *testing.T) {
	parsed := &ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*ParsedStruct),
		TypeAliases: make(map[string]string),
		Views: []ParsedView{{
			Name:       "bad_view",
			FuncName:   "BadView",
			ReturnType: "map[int]string",
		}},
	}

	_, err := analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "view bad_view return")
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_ViewParamBad(t *testing.T) {
	parsed := &ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*ParsedStruct),
		TypeAliases: make(map[string]string),
		Views: []ParsedView{{
			Name:     "bad_view",
			FuncName: "BadView",
			Params: []ParsedParam{
				{Name: "arg", GoType: "interface{}"},
			},
		}},
	}

	_, err := analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "view bad_view param arg")
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_SumTypeVariantBadField(t *testing.T) {
	parsed := &ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*ParsedStruct),
		TypeAliases: make(map[string]string),
		Variants: []ParsedVariant{{
			OfInterface: "BadSum",
			Name:        "BadVariant",
			StructName:  "BadVariantStruct",
			Fields: []ParsedField{
				{GoName: "Data", GoType: "map[string]any", BsatnName: "data"},
			},
		}},
		SumTypes: []ParsedSumType{{
			InterfaceName: "BadSum",
		}},
	}

	_, err := analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sum type BadSum")
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_TableStructMissing(t *testing.T) {
	parsed := &ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*ParsedStruct),
		TypeAliases: make(map[string]string),
		Tables: []ParsedTable{{
			Name:       "ghost",
			Access:     "public",
			StructName: "GhostStruct",
			Fields: []ParsedField{
				{GoName: "Id", GoType: "uint64", BsatnName: "id"},
			},
		}},
	}

	_, err := analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown struct type")
	assert.Contains(t, err.Error(), "GhostStruct")
}

func TestAnalyzeError_NestedUnknownType(t *testing.T) {
	parsed := &ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*ParsedStruct),
		TypeAliases: make(map[string]string),
	}
	parsed.Structs["Inner"] = &ParsedStruct{
		Name: "Inner",
		Fields: []ParsedField{
			{GoName: "Bad", GoType: "complex128", BsatnName: "bad"},
		},
	}
	parsed.Structs["Outer"] = &ParsedStruct{
		Name: "Outer",
		Fields: []ParsedField{
			{GoName: "Nested", GoType: "Inner", BsatnName: "nested"},
		},
	}
	parsed.Tables = []ParsedTable{{
		Name:       "test",
		Access:     "public",
		StructName: "Outer",
		Fields: []ParsedField{
			{GoName: "Nested", GoType: "Inner", BsatnName: "nested"},
		},
	}}

	_, err := analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")
	assert.Contains(t, err.Error(), "complex128")
}

func TestAnalyzeError_SliceOfUnsupportedType(t *testing.T) {
	parsed := &ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*ParsedStruct),
		TypeAliases: make(map[string]string),
	}
	parsed.Structs["BadSlice"] = &ParsedStruct{
		Name: "BadSlice",
		Fields: []ParsedField{
			{GoName: "Items", GoType: "[]map[string]int", BsatnName: "items"},
		},
	}
	parsed.Tables = []ParsedTable{{
		Name:       "test",
		Access:     "public",
		StructName: "BadSlice",
		Fields: []ParsedField{
			{GoName: "Items", GoType: "[]map[string]int", BsatnName: "items"},
		},
	}}

	_, err := analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_PointerToUnsupportedType(t *testing.T) {
	parsed := &ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*ParsedStruct),
		TypeAliases: make(map[string]string),
	}
	parsed.Structs["BadPtr"] = &ParsedStruct{
		Name: "BadPtr",
		Fields: []ParsedField{
			{GoName: "Ptr", GoType: "*chan int", BsatnName: "ptr"},
		},
	}
	parsed.Tables = []ParsedTable{{
		Name:       "test",
		Access:     "public",
		StructName: "BadPtr",
		Fields: []ParsedField{
			{GoName: "Ptr", GoType: "*chan int", BsatnName: "ptr"},
		},
	}}

	_, err := analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")
}
