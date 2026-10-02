package servergen_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dottedmag/stdb-go/internal/parser"
	"github.com/dottedmag/stdb-go/internal/servergen"
)

func TestAnalyzeError_UnknownFieldType(t *testing.T) {
	parsed := &parser.ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*parser.ParsedStruct),
		TypeAliases: make(map[string]string),
	}
	parsed.Structs["BadStruct"] = &parser.ParsedStruct{
		Name: "BadStruct",
		Fields: []parser.ParsedField{
			{GoName: "Data", GoType: "map[string]int", BsatnName: "data"},
		},
	}
	parsed.Tables = []parser.ParsedTable{{
		Name:       "test",
		Access:     "public",
		StructName: "BadStruct",
		Fields: []parser.ParsedField{
			{GoName: "Data", GoType: "map[string]int", BsatnName: "data"},
		},
	}}

	_, err := servergen.Analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_UnknownStructRef(t *testing.T) {
	parsed := &parser.ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*parser.ParsedStruct),
		TypeAliases: make(map[string]string),
	}
	parsed.Structs["MyStruct"] = &parser.ParsedStruct{
		Name: "MyStruct",
		Fields: []parser.ParsedField{
			{GoName: "Ref", GoType: "NonExistent", BsatnName: "ref"},
		},
	}
	parsed.Tables = []parser.ParsedTable{{
		Name:       "test",
		Access:     "public",
		StructName: "MyStruct",
		Fields: []parser.ParsedField{
			{GoName: "Ref", GoType: "NonExistent", BsatnName: "ref"},
		},
	}}

	_, err := servergen.Analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")
	assert.Contains(t, err.Error(), "NonExistent")
}

func TestAnalyzeError_ReducerBadParam(t *testing.T) {
	parsed := &parser.ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*parser.ParsedStruct),
		TypeAliases: make(map[string]string),
		Reducers: []parser.ParsedReducer{{
			Name:     "bad_reducer",
			FuncName: "BadReducer",
			Params: []parser.ParsedParam{
				{Name: "ch", GoType: "chan int"},
			},
		}},
	}

	_, err := servergen.Analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reducer bad_reducer")
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_ReducerBadParamMap(t *testing.T) {
	parsed := &parser.ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*parser.ParsedStruct),
		TypeAliases: make(map[string]string),
		Reducers: []parser.ParsedReducer{{
			Name:     "map_reducer",
			FuncName: "MapReducer",
			Params: []parser.ParsedParam{
				{Name: "data", GoType: "map[string]string"},
			},
		}},
	}

	_, err := servergen.Analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reducer map_reducer")
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_ProcedureBadReturn(t *testing.T) {
	parsed := &parser.ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*parser.ParsedStruct),
		TypeAliases: make(map[string]string),
		Procedures: []parser.ParsedProcedure{{
			Name:       "bad_proc",
			FuncName:   "BadProc",
			ReturnType: "chan int",
		}},
	}

	_, err := servergen.Analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "procedure bad_proc return")
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_ProcedureParamBad(t *testing.T) {
	parsed := &parser.ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*parser.ParsedStruct),
		TypeAliases: make(map[string]string),
		Procedures: []parser.ParsedProcedure{{
			Name:     "bad_proc",
			FuncName: "BadProc",
			Params: []parser.ParsedParam{
				{Name: "arg", GoType: "func()"},
			},
		}},
	}

	_, err := servergen.Analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "procedure bad_proc param arg")
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_ViewBadReturn(t *testing.T) {
	parsed := &parser.ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*parser.ParsedStruct),
		TypeAliases: make(map[string]string),
		Views: []parser.ParsedView{{
			Name:       "bad_view",
			FuncName:   "BadView",
			ReturnType: "map[int]string",
		}},
	}

	_, err := servergen.Analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "view bad_view return")
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_ViewParamBad(t *testing.T) {
	parsed := &parser.ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*parser.ParsedStruct),
		TypeAliases: make(map[string]string),
		Views: []parser.ParsedView{{
			Name:     "bad_view",
			FuncName: "BadView",
			Params: []parser.ParsedParam{
				{Name: "arg", GoType: "interface{}"},
			},
		}},
	}

	_, err := servergen.Analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "view bad_view param arg")
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_SumTypeVariantBadField(t *testing.T) {
	parsed := &parser.ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*parser.ParsedStruct),
		TypeAliases: make(map[string]string),
		Variants: []parser.ParsedVariant{{
			OfInterface: "BadSum",
			Name:        "BadVariant",
			StructName:  "BadVariantStruct",
			Fields: []parser.ParsedField{
				{GoName: "Data", GoType: "map[string]any", BsatnName: "data"},
			},
		}},
		SumTypes: []parser.ParsedSumType{{
			InterfaceName: "BadSum",
		}},
	}

	_, err := servergen.Analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sum type BadSum")
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_TableStructMissing(t *testing.T) {
	parsed := &parser.ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*parser.ParsedStruct),
		TypeAliases: make(map[string]string),
		Tables: []parser.ParsedTable{{
			Name:       "ghost",
			Access:     "public",
			StructName: "GhostStruct",
			Fields: []parser.ParsedField{
				{GoName: "Id", GoType: "uint64", BsatnName: "id"},
			},
		}},
	}

	_, err := servergen.Analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown struct type")
	assert.Contains(t, err.Error(), "GhostStruct")
}

func TestAnalyzeError_NestedUnknownType(t *testing.T) {
	parsed := &parser.ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*parser.ParsedStruct),
		TypeAliases: make(map[string]string),
	}
	parsed.Structs["Inner"] = &parser.ParsedStruct{
		Name: "Inner",
		Fields: []parser.ParsedField{
			{GoName: "Bad", GoType: "complex128", BsatnName: "bad"},
		},
	}
	parsed.Structs["Outer"] = &parser.ParsedStruct{
		Name: "Outer",
		Fields: []parser.ParsedField{
			{GoName: "Nested", GoType: "Inner", BsatnName: "nested"},
		},
	}
	parsed.Tables = []parser.ParsedTable{{
		Name:       "test",
		Access:     "public",
		StructName: "Outer",
		Fields: []parser.ParsedField{
			{GoName: "Nested", GoType: "Inner", BsatnName: "nested"},
		},
	}}

	_, err := servergen.Analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")
	assert.Contains(t, err.Error(), "complex128")
}

func TestAnalyzeError_SliceOfUnsupportedType(t *testing.T) {
	parsed := &parser.ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*parser.ParsedStruct),
		TypeAliases: make(map[string]string),
	}
	parsed.Structs["BadSlice"] = &parser.ParsedStruct{
		Name: "BadSlice",
		Fields: []parser.ParsedField{
			{GoName: "Items", GoType: "[]map[string]int", BsatnName: "items"},
		},
	}
	parsed.Tables = []parser.ParsedTable{{
		Name:       "test",
		Access:     "public",
		StructName: "BadSlice",
		Fields: []parser.ParsedField{
			{GoName: "Items", GoType: "[]map[string]int", BsatnName: "items"},
		},
	}}

	_, err := servergen.Analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestAnalyzeError_PointerToUnsupportedType(t *testing.T) {
	parsed := &parser.ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*parser.ParsedStruct),
		TypeAliases: make(map[string]string),
	}
	parsed.Structs["BadPtr"] = &parser.ParsedStruct{
		Name: "BadPtr",
		Fields: []parser.ParsedField{
			{GoName: "Ptr", GoType: "*chan int", BsatnName: "ptr"},
		},
	}
	parsed.Tables = []parser.ParsedTable{{
		Name:       "test",
		Access:     "public",
		StructName: "BadPtr",
		Fields: []parser.ParsedField{
			{GoName: "Ptr", GoType: "*chan int", BsatnName: "ptr"},
		},
	}}

	_, err := servergen.Analyze(parsed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")
}
