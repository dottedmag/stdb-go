package servergen_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dottedmag/stdb-go/internal/parser"
	"github.com/dottedmag/stdb-go/internal/servergen"
)

// helper to build a minimal ParsedModule with initialized maps.
func newParsedModule() *parser.ParsedModule {
	return &parser.ParsedModule{
		PackageName: "main",
		Structs:     make(map[string]*parser.ParsedStruct),
		TypeAliases: make(map[string]string),
	}
}

// TestAnalyzePrimitiveTypes verifies that all Go primitive types resolve to the correct AlgKind.
func TestAnalyzePrimitiveTypes(t *testing.T) {
	cases := []struct {
		goType   string
		expected servergen.AlgKind
	}{
		{"bool", servergen.AlgKindBool},
		{"uint8", servergen.AlgKindU8},
		{"uint16", servergen.AlgKindU16},
		{"uint32", servergen.AlgKindU32},
		{"uint64", servergen.AlgKindU64},
		{"int8", servergen.AlgKindI8},
		{"int16", servergen.AlgKindI16},
		{"int32", servergen.AlgKindI32},
		{"int64", servergen.AlgKindI64},
		{"float32", servergen.AlgKindF32},
		{"float64", servergen.AlgKindF64},
		{"string", servergen.AlgKindString},
	}

	for _, tc := range cases {
		t.Run(tc.goType, func(t *testing.T) {
			parsed := newParsedModule()
			parsed.Tables = []parser.ParsedTable{{
				Name:       "test_table",
				Access:     "public",
				StructName: "TestStruct",
				Fields: []parser.ParsedField{{
					GoName: "Field", GoType: tc.goType, BsatnName: "field",
				}},
			}}
			parsed.Structs["TestStruct"] = &parser.ParsedStruct{
				Name: "TestStruct",
				Fields: []parser.ParsedField{{
					GoName: "Field", GoType: tc.goType, BsatnName: "field",
				}},
			}

			mod, err := servergen.Analyze(parsed)
			require.NoError(t, err)
			require.Len(t, mod.Tables, 1)
			require.Len(t, mod.Tables[0].Fields, 1)
			assert.Equal(t, tc.expected, mod.Tables[0].Fields[0].AlgType.Kind,
				"expected AlgKind for %s", tc.goType)
		})
	}
}

// TestAnalyzeSpecialTypes verifies that special SpacetimeDB types (both qualified and unqualified) resolve correctly.
func TestAnalyzeSpecialTypes(t *testing.T) {
	cases := []struct {
		goType   string
		expected servergen.AlgKind
	}{
		// Qualified forms
		{"types.Identity", servergen.AlgKindIdentity},
		{"types.ConnectionId", servergen.AlgKindConnectionId},
		{"types.Timestamp", servergen.AlgKindTimestamp},
		{"types.TimeDuration", servergen.AlgKindTimeDuration},
		{"types.ScheduleAt", servergen.AlgKindScheduleAt},
		{"types.Uuid", servergen.AlgKindUuid},
		{"types.Uint128", servergen.AlgKindU128},
		{"types.Uint256", servergen.AlgKindU256},
		{"types.Int128", servergen.AlgKindI128},
		{"types.Int256", servergen.AlgKindI256},
		// Unqualified forms
		{"Identity", servergen.AlgKindIdentity},
		{"ConnectionId", servergen.AlgKindConnectionId},
		{"Timestamp", servergen.AlgKindTimestamp},
		{"TimeDuration", servergen.AlgKindTimeDuration},
		{"ScheduleAt", servergen.AlgKindScheduleAt},
		{"Uuid", servergen.AlgKindUuid},
		{"Uint128", servergen.AlgKindU128},
		{"Uint256", servergen.AlgKindU256},
		{"Int128", servergen.AlgKindI128},
		{"Int256", servergen.AlgKindI256},
	}

	for _, tc := range cases {
		t.Run(tc.goType, func(t *testing.T) {
			parsed := newParsedModule()
			parsed.Tables = []parser.ParsedTable{{
				Name:       "test_table",
				Access:     "public",
				StructName: "TestStruct",
				Fields: []parser.ParsedField{{
					GoName: "Field", GoType: tc.goType, BsatnName: "field",
				}},
			}}
			parsed.Structs["TestStruct"] = &parser.ParsedStruct{
				Name: "TestStruct",
				Fields: []parser.ParsedField{{
					GoName: "Field", GoType: tc.goType, BsatnName: "field",
				}},
			}

			mod, err := servergen.Analyze(parsed)
			require.NoError(t, err)
			require.Len(t, mod.Tables, 1)
			require.Len(t, mod.Tables[0].Fields, 1)
			assert.Equal(t, tc.expected, mod.Tables[0].Fields[0].AlgType.Kind,
				"expected AlgKind for %s", tc.goType)
		})
	}
}

// TestAnalyzeSliceType verifies that slices resolve correctly:
// []byte / []uint8 -> AlgKindBytes, []<other> -> AlgKindArray.
func TestAnalyzeSliceType(t *testing.T) {
	t.Run("[]uint8 is bytes", func(t *testing.T) {
		parsed := newParsedModule()
		parsed.Tables = []parser.ParsedTable{{
			Name: "t", Access: "public", StructName: "S",
			Fields: []parser.ParsedField{{GoName: "Data", GoType: "[]uint8", BsatnName: "data"}},
		}}
		parsed.Structs["S"] = &parser.ParsedStruct{
			Name:   "S",
			Fields: []parser.ParsedField{{GoName: "Data", GoType: "[]uint8", BsatnName: "data"}},
		}

		mod, err := servergen.Analyze(parsed)
		require.NoError(t, err)
		assert.Equal(t, servergen.AlgKindBytes, mod.Tables[0].Fields[0].AlgType.Kind)
	})

	t.Run("[]byte is bytes", func(t *testing.T) {
		parsed := newParsedModule()
		parsed.Tables = []parser.ParsedTable{{
			Name: "t", Access: "public", StructName: "S",
			Fields: []parser.ParsedField{{GoName: "Data", GoType: "[]byte", BsatnName: "data"}},
		}}
		parsed.Structs["S"] = &parser.ParsedStruct{
			Name:   "S",
			Fields: []parser.ParsedField{{GoName: "Data", GoType: "[]byte", BsatnName: "data"}},
		}

		mod, err := servergen.Analyze(parsed)
		require.NoError(t, err)
		assert.Equal(t, servergen.AlgKindBytes, mod.Tables[0].Fields[0].AlgType.Kind)
	})

	t.Run("[]int32 is array of I32", func(t *testing.T) {
		parsed := newParsedModule()
		parsed.Tables = []parser.ParsedTable{{
			Name: "t", Access: "public", StructName: "S",
			Fields: []parser.ParsedField{{GoName: "Nums", GoType: "[]int32", BsatnName: "nums"}},
		}}
		parsed.Structs["S"] = &parser.ParsedStruct{
			Name:   "S",
			Fields: []parser.ParsedField{{GoName: "Nums", GoType: "[]int32", BsatnName: "nums"}},
		}

		mod, err := servergen.Analyze(parsed)
		require.NoError(t, err)
		field := mod.Tables[0].Fields[0]
		assert.Equal(t, servergen.AlgKindArray, field.AlgType.Kind)
		require.NotNil(t, field.AlgType.ElemType)
		assert.Equal(t, servergen.AlgKindI32, field.AlgType.ElemType.Kind)
	})
}

// TestAnalyzePointerType verifies that *T resolves to Option(T).
func TestAnalyzePointerType(t *testing.T) {
	parsed := newParsedModule()
	parsed.Tables = []parser.ParsedTable{{
		Name: "t", Access: "public", StructName: "S",
		Fields: []parser.ParsedField{{GoName: "OptVal", GoType: "*int32", BsatnName: "opt_val"}},
	}}
	parsed.Structs["S"] = &parser.ParsedStruct{
		Name:   "S",
		Fields: []parser.ParsedField{{GoName: "OptVal", GoType: "*int32", BsatnName: "opt_val"}},
	}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	field := mod.Tables[0].Fields[0]
	assert.Equal(t, servergen.AlgKindOption, field.AlgType.Kind)
	require.NotNil(t, field.AlgType.ElemType)
	assert.Equal(t, servergen.AlgKindI32, field.AlgType.ElemType.Kind)
}

// TestAnalyzeNestedOptionSlice verifies *[]*int32 -> Option(Array(Option(I32))).
func TestAnalyzeNestedOptionSlice(t *testing.T) {
	parsed := newParsedModule()
	parsed.Tables = []parser.ParsedTable{{
		Name: "t", Access: "public", StructName: "S",
		Fields: []parser.ParsedField{{GoName: "F", GoType: "*[]*int32", BsatnName: "f"}},
	}}
	parsed.Structs["S"] = &parser.ParsedStruct{
		Name:   "S",
		Fields: []parser.ParsedField{{GoName: "F", GoType: "*[]*int32", BsatnName: "f"}},
	}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	field := mod.Tables[0].Fields[0]

	// *[]*int32 -> Option
	assert.Equal(t, servergen.AlgKindOption, field.AlgType.Kind)
	require.NotNil(t, field.AlgType.ElemType)

	// []*int32 -> Array
	arr := field.AlgType.ElemType
	assert.Equal(t, servergen.AlgKindArray, arr.Kind)
	require.NotNil(t, arr.ElemType)

	// *int32 -> Option
	opt := arr.ElemType
	assert.Equal(t, servergen.AlgKindOption, opt.Kind)
	require.NotNil(t, opt.ElemType)

	// int32 -> I32
	assert.Equal(t, servergen.AlgKindI32, opt.ElemType.Kind)
}

// TestAnalyzeStructRef verifies that a field referencing another struct resolves to AlgKindRef.
func TestAnalyzeStructRef(t *testing.T) {
	parsed := newParsedModule()
	parsed.Structs["Inner"] = &parser.ParsedStruct{
		Name: "Inner",
		Fields: []parser.ParsedField{
			{GoName: "Val", GoType: "uint32", BsatnName: "val"},
		},
	}
	parsed.Structs["Outer"] = &parser.ParsedStruct{
		Name: "Outer",
		Fields: []parser.ParsedField{
			{GoName: "Nested", GoType: "Inner", BsatnName: "nested"},
		},
	}
	parsed.Tables = []parser.ParsedTable{{
		Name: "outer_table", Access: "public", StructName: "Outer",
		Fields: []parser.ParsedField{
			{GoName: "Nested", GoType: "Inner", BsatnName: "nested"},
		},
	}}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, mod.Tables, 1)
	require.Len(t, mod.Tables[0].Fields, 1)

	field := mod.Tables[0].Fields[0]
	assert.Equal(t, servergen.AlgKindRef, field.AlgType.Kind)
	assert.Equal(t, "Inner", field.AlgType.TypeName)

	// The Inner struct should have a typespace entry.
	innerType, ok := mod.Types["Inner"]
	require.True(t, ok)
	assert.Equal(t, field.AlgType.Ref, innerType.TypespaceIdx)
}

// TestAnalyzeTableVarName verifies that snake_case table names produce correct PascalCase+Table accessor names.
func TestAnalyzeTableVarName(t *testing.T) {
	cases := []struct {
		tableName string
		expected  string
	}{
		{"one_u8", "OneU8Table"},
		{"my_players", "MyPlayersTable"},
		{"simple", "SimpleTable"},
	}

	for _, tc := range cases {
		t.Run(tc.tableName, func(t *testing.T) {
			parsed := newParsedModule()
			parsed.Tables = []parser.ParsedTable{{
				Name: tc.tableName, Access: "public", StructName: "S",
				Fields: []parser.ParsedField{{GoName: "Id", GoType: "uint64", BsatnName: "id", PrimaryKey: true}},
			}}
			parsed.Structs["S"] = &parser.ParsedStruct{
				Name:   "S",
				Fields: []parser.ParsedField{{GoName: "Id", GoType: "uint64", BsatnName: "id", PrimaryKey: true}},
			}

			mod, err := servergen.Analyze(parsed)
			require.NoError(t, err)
			require.Len(t, mod.Tables, 1)
			assert.Equal(t, tc.expected, mod.Tables[0].VarName)
		})
	}
}

// TestAnalyzeTableFields verifies fields get correct sequential ColIndex values.
func TestAnalyzeTableFields(t *testing.T) {
	parsed := newParsedModule()
	fields := []parser.ParsedField{
		{GoName: "Id", GoType: "uint64", BsatnName: "id", PrimaryKey: true},
		{GoName: "Name", GoType: "string", BsatnName: "name"},
		{GoName: "Score", GoType: "int32", BsatnName: "score"},
	}
	parsed.Tables = []parser.ParsedTable{{
		Name: "players", Access: "public", StructName: "Player",
		Fields: fields,
	}}
	parsed.Structs["Player"] = &parser.ParsedStruct{Name: "Player", Fields: fields}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, mod.Tables, 1)
	require.Len(t, mod.Tables[0].Fields, 3)

	for i, f := range mod.Tables[0].Fields {
		assert.Equal(t, uint16(i), f.ColIndex, "field %s should have ColIndex %d", f.GoName, i)
	}
	assert.Equal(t, servergen.AlgKindU64, mod.Tables[0].Fields[0].AlgType.Kind)
	assert.Equal(t, servergen.AlgKindString, mod.Tables[0].Fields[1].AlgType.Kind)
	assert.Equal(t, servergen.AlgKindI32, mod.Tables[0].Fields[2].AlgType.Kind)
}

// TestAnalyzeTableConstraints verifies PrimaryKey, Unique, AutoInc flags are preserved.
func TestAnalyzeTableConstraints(t *testing.T) {
	parsed := newParsedModule()
	fields := []parser.ParsedField{
		{GoName: "Id", GoType: "uint64", BsatnName: "id", PrimaryKey: true, AutoInc: true},
		{GoName: "Email", GoType: "string", BsatnName: "email", Unique: true},
		{GoName: "Tag", GoType: "string", BsatnName: "tag", IndexBTree: true},
	}
	parsed.Tables = []parser.ParsedTable{{
		Name: "users", Access: "public", StructName: "User",
		Fields: fields,
	}}
	parsed.Structs["User"] = &parser.ParsedStruct{Name: "User", Fields: fields}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, mod.Tables[0].Fields, 3)

	idField := mod.Tables[0].Fields[0]
	assert.True(t, idField.PrimaryKey)
	assert.True(t, idField.AutoInc)
	assert.False(t, idField.Unique)

	emailField := mod.Tables[0].Fields[1]
	assert.True(t, emailField.Unique)
	assert.False(t, emailField.PrimaryKey)

	tagField := mod.Tables[0].Fields[2]
	assert.True(t, tagField.IndexBTree)
	assert.False(t, tagField.PrimaryKey)
	assert.False(t, tagField.Unique)
}

// TestAnalyzeMultiTable verifies that the same struct backing two different table names produces two AnalyzedTable entries.
func TestAnalyzeMultiTable(t *testing.T) {
	parsed := newParsedModule()
	fields := []parser.ParsedField{
		{GoName: "Id", GoType: "uint64", BsatnName: "id", PrimaryKey: true},
		{GoName: "Name", GoType: "string", BsatnName: "name"},
	}
	parsed.Structs["Entity"] = &parser.ParsedStruct{Name: "Entity", Fields: fields}
	parsed.Tables = []parser.ParsedTable{
		{Name: "entity", Access: "public", StructName: "Entity", Fields: fields},
		{Name: "logged_out_entity", Access: "private", StructName: "Entity", Fields: fields},
	}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, mod.Tables, 2)

	assert.Equal(t, "entity", mod.Tables[0].Name)
	assert.Equal(t, "EntityTable", mod.Tables[0].VarName)
	assert.Equal(t, "public", mod.Tables[0].Access)

	assert.Equal(t, "logged_out_entity", mod.Tables[1].Name)
	assert.Equal(t, "LoggedOutEntityTable", mod.Tables[1].VarName)
	assert.Equal(t, "private", mod.Tables[1].Access)

	// Both should reference the same typespace slot.
	assert.Equal(t, mod.Tables[0].TypespaceRef, mod.Tables[1].TypespaceRef)
}

// TestAnalyzeReducerParams verifies that reducer parameters are resolved to correct AlgTypes.
func TestAnalyzeReducerParams(t *testing.T) {
	parsed := newParsedModule()
	parsed.Reducers = []parser.ParsedReducer{{
		Name:     "set_name",
		FuncName: "SetName",
		Params: []parser.ParsedParam{
			{Name: "id", GoType: "uint64"},
			{Name: "name", GoType: "string"},
			{Name: "score", GoType: "*float32"},
		},
	}}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, mod.Reducers, 1)
	require.Len(t, mod.Reducers[0].Params, 3)

	assert.Equal(t, servergen.AlgKindU64, mod.Reducers[0].Params[0].AlgType.Kind)
	assert.Equal(t, "id", mod.Reducers[0].Params[0].Name)

	assert.Equal(t, servergen.AlgKindString, mod.Reducers[0].Params[1].AlgType.Kind)
	assert.Equal(t, "name", mod.Reducers[0].Params[1].Name)

	assert.Equal(t, servergen.AlgKindOption, mod.Reducers[0].Params[2].AlgType.Kind)
	require.NotNil(t, mod.Reducers[0].Params[2].AlgType.ElemType)
	assert.Equal(t, servergen.AlgKindF32, mod.Reducers[0].Params[2].AlgType.ElemType.Kind)
}

// TestAnalyzeReducerID verifies reducers get sequential IDs starting from 0.
func TestAnalyzeReducerID(t *testing.T) {
	parsed := newParsedModule()
	parsed.Reducers = []parser.ParsedReducer{
		{Name: "first", FuncName: "First", Params: nil},
		{Name: "second", FuncName: "Second", Params: nil},
		{Name: "third", FuncName: "Third", Params: nil},
	}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, mod.Reducers, 3)

	assert.Equal(t, uint32(0), mod.Reducers[0].ID)
	assert.Equal(t, uint32(1), mod.Reducers[1].ID)
	assert.Equal(t, uint32(2), mod.Reducers[2].ID)
}

// TestAnalyzeLifecycleID verifies lifecycle reducers get IDs continuing after regular reducers.
func TestAnalyzeLifecycleID(t *testing.T) {
	parsed := newParsedModule()
	parsed.Reducers = []parser.ParsedReducer{
		{Name: "action_a", FuncName: "ActionA"},
		{Name: "action_b", FuncName: "ActionB"},
	}
	parsed.Lifecycle = []parser.ParsedLifecycle{
		{Kind: "init", FuncName: "Init"},
		{Kind: "connect", FuncName: "OnConnect"},
	}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)

	// Reducers: 0, 1
	require.Len(t, mod.Reducers, 2)
	assert.Equal(t, uint32(0), mod.Reducers[0].ID)
	assert.Equal(t, uint32(1), mod.Reducers[1].ID)

	// Lifecycle: 2, 3 (continuing from reducers)
	require.Len(t, mod.Lifecycle, 2)
	assert.Equal(t, uint32(2), mod.Lifecycle[0].ID)
	assert.Equal(t, "init", mod.Lifecycle[0].Kind)
	assert.Equal(t, uint32(3), mod.Lifecycle[1].ID)
	assert.Equal(t, "connect", mod.Lifecycle[1].Kind)
}

// TestAnalyzeProcedureReturn verifies that procedure return types are resolved.
func TestAnalyzeProcedureReturn(t *testing.T) {
	parsed := newParsedModule()
	parsed.Procedures = []parser.ParsedProcedure{{
		Name:       "get_value",
		FuncName:   "GetValue",
		Params:     []parser.ParsedParam{{Name: "key", GoType: "string"}},
		ReturnType: "[]uint8",
	}}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, mod.Procedures, 1)
	require.NotNil(t, mod.Procedures[0].ReturnType)
	assert.Equal(t, servergen.AlgKindBytes, mod.Procedures[0].ReturnType.Kind)
	assert.Equal(t, "[]uint8", mod.Procedures[0].ReturnGoType)
}

// TestAnalyzeViewClassification verifies authenticated and anonymous views get separate ID counters.
func TestAnalyzeViewClassification(t *testing.T) {
	parsed := newParsedModule()
	parsed.Views = []parser.ParsedView{
		{Name: "auth_view_1", FuncName: "AuthView1", IsAnonymous: false, ReturnType: "string"},
		{Name: "anon_view_1", FuncName: "AnonView1", IsAnonymous: true, ReturnType: "string"},
		{Name: "auth_view_2", FuncName: "AuthView2", IsAnonymous: false, ReturnType: "uint32"},
		{Name: "anon_view_2", FuncName: "AnonView2", IsAnonymous: true, ReturnType: "uint32"},
	}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, mod.Views, 4)

	// auth_view_1 -> authIdx=0
	assert.Equal(t, uint32(0), mod.Views[0].ID)
	assert.False(t, mod.Views[0].IsAnonymous)

	// anon_view_1 -> anonIdx=0
	assert.Equal(t, uint32(0), mod.Views[1].ID)
	assert.True(t, mod.Views[1].IsAnonymous)

	// auth_view_2 -> authIdx=1
	assert.Equal(t, uint32(1), mod.Views[2].ID)
	assert.False(t, mod.Views[2].IsAnonymous)

	// anon_view_2 -> anonIdx=1
	assert.Equal(t, uint32(1), mod.Views[3].ID)
	assert.True(t, mod.Views[3].IsAnonymous)
}

// TestAnalyzeSimpleEnum verifies that a simple enum is registered with correct variants and TypeKindSimpleEnum.
func TestAnalyzeSimpleEnum(t *testing.T) {
	parsed := newParsedModule()
	parsed.Enums = []parser.ParsedEnum{{
		TypeName: "Color",
		Variants: []string{"Red", "Green", "Blue"},
	}}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, mod.Enums, 1)

	assert.Equal(t, "Color", mod.Enums[0].TypeName)
	assert.Equal(t, []string{"Red", "Green", "Blue"}, mod.Enums[0].Variants)

	typeInfo, ok := mod.Types["Color"]
	require.True(t, ok)
	assert.Equal(t, servergen.TypeKindSimpleEnum, typeInfo.Kind)
	assert.Equal(t, []string{"Red", "Green", "Blue"}, typeInfo.EnumVariants)
}

// TestAnalyzeSumType verifies sum type analysis with variants resolved and tags assigned sequentially.
func TestAnalyzeSumType(t *testing.T) {
	parsed := newParsedModule()
	parsed.SumTypes = []parser.ParsedSumType{{
		InterfaceName: "Shape",
	}}
	parsed.Variants = []parser.ParsedVariant{
		{
			OfInterface: "Shape",
			Name:        "Circle",
			StructName:  "CircleData",
			Fields: []parser.ParsedField{
				{GoName: "Radius", GoType: "float64", BsatnName: "radius"},
			},
		},
		{
			OfInterface: "Shape",
			Name:        "Rectangle",
			StructName:  "RectData",
			Fields: []parser.ParsedField{
				{GoName: "Width", GoType: "float64", BsatnName: "width"},
				{GoName: "Height", GoType: "float64", BsatnName: "height"},
			},
		},
	}
	parsed.Structs["CircleData"] = &parser.ParsedStruct{
		Name: "CircleData",
		Fields: []parser.ParsedField{
			{GoName: "Radius", GoType: "float64", BsatnName: "radius"},
		},
	}
	parsed.Structs["RectData"] = &parser.ParsedStruct{
		Name: "RectData",
		Fields: []parser.ParsedField{
			{GoName: "Width", GoType: "float64", BsatnName: "width"},
			{GoName: "Height", GoType: "float64", BsatnName: "height"},
		},
	}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, mod.SumTypes, 1)

	st := mod.SumTypes[0]
	assert.Equal(t, "Shape", st.InterfaceName)
	require.Len(t, st.Variants, 2)

	assert.Equal(t, "Circle", st.Variants[0].Name)
	assert.Equal(t, uint8(0), st.Variants[0].Tag)
	require.Len(t, st.Variants[0].Fields, 1)
	assert.Equal(t, servergen.AlgKindF64, st.Variants[0].Fields[0].AlgType.Kind)

	assert.Equal(t, "Rectangle", st.Variants[1].Name)
	assert.Equal(t, uint8(1), st.Variants[1].Tag)
	require.Len(t, st.Variants[1].Fields, 2)
}

// TestAnalyzeScopedType verifies that scope is preserved in the AnalyzedType and AnalyzedEnum.
func TestAnalyzeScopedType(t *testing.T) {
	parsed := newParsedModule()
	parsed.Enums = []parser.ParsedEnum{{
		TypeName: "Status",
		Variants: []string{"Active", "Inactive"},
		Scope:    []string{"Game", "Player"},
	}}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, mod.Enums, 1)

	assert.Equal(t, []string{"Game", "Player"}, mod.Enums[0].Scope)

	typeInfo, ok := mod.Types["Status"]
	require.True(t, ok)
	assert.Equal(t, []string{"Game", "Player"}, typeInfo.Scope)
}

// TestAnalyzeTypespaceOrder verifies that enums are registered before structs in the TypeOrder.
func TestAnalyzeTypespaceOrder(t *testing.T) {
	parsed := newParsedModule()
	parsed.Enums = []parser.ParsedEnum{{
		TypeName: "Color",
		Variants: []string{"Red", "Green"},
	}}
	fields := []parser.ParsedField{
		{GoName: "Id", GoType: "uint64", BsatnName: "id", PrimaryKey: true},
	}
	parsed.Tables = []parser.ParsedTable{{
		Name: "items", Access: "public", StructName: "Item",
		Fields: fields,
	}}
	parsed.Structs["Item"] = &parser.ParsedStruct{Name: "Item", Fields: fields}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, mod.TypeOrder, 2)

	// Enums first, then structs.
	assert.Equal(t, "Color", mod.TypeOrder[0])
	assert.Equal(t, "Item", mod.TypeOrder[1])
}

// TestAnalyzeTypespaceIdx verifies that TypespaceIdx is assigned incrementally (0, 1, 2...).
func TestAnalyzeTypespaceIdx(t *testing.T) {
	parsed := newParsedModule()
	parsed.Enums = []parser.ParsedEnum{
		{TypeName: "ColorA", Variants: []string{"Red"}},
		{TypeName: "ColorB", Variants: []string{"Blue"}},
	}
	fields := []parser.ParsedField{
		{GoName: "Id", GoType: "uint64", BsatnName: "id"},
	}
	parsed.Tables = []parser.ParsedTable{{
		Name: "t", Access: "public", StructName: "MyStruct",
		Fields: fields,
	}}
	parsed.Structs["MyStruct"] = &parser.ParsedStruct{Name: "MyStruct", Fields: fields}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)

	assert.Equal(t, 0, mod.Types["ColorA"].TypespaceIdx)
	assert.Equal(t, 1, mod.Types["ColorB"].TypespaceIdx)
	assert.Equal(t, 2, mod.Types["MyStruct"].TypespaceIdx)
}

// TestAnalyzeCustomOrdering verifies that all struct types, enums, and sum types get CustomOrdering=true.
func TestAnalyzeCustomOrdering(t *testing.T) {
	parsed := newParsedModule()
	parsed.Enums = []parser.ParsedEnum{{
		TypeName: "Status",
		Variants: []string{"On", "Off"},
	}}
	parsed.SumTypes = []parser.ParsedSumType{{
		InterfaceName: "Value",
	}}
	parsed.Variants = []parser.ParsedVariant{{
		OfInterface: "Value",
		Name:        "IntVal",
		StructName:  "IntValData",
		Fields:      []parser.ParsedField{{GoName: "V", GoType: "int32", BsatnName: "v"}},
	}}
	parsed.Structs["IntValData"] = &parser.ParsedStruct{
		Name:   "IntValData",
		Fields: []parser.ParsedField{{GoName: "V", GoType: "int32", BsatnName: "v"}},
	}

	// Table without explicit indexes (struct still gets CustomOrdering from resolveStructType).
	plainFields := []parser.ParsedField{{GoName: "X", GoType: "uint32", BsatnName: "x"}}
	parsed.Tables = []parser.ParsedTable{{
		Name: "plain", Access: "public", StructName: "PlainStruct",
		Fields: plainFields,
	}}
	parsed.Structs["PlainStruct"] = &parser.ParsedStruct{Name: "PlainStruct", Fields: plainFields}

	// Table with PK (also gets CustomOrdering from the btree marking).
	pkFields := []parser.ParsedField{{GoName: "Id", GoType: "uint64", BsatnName: "id", PrimaryKey: true}}
	parsed.Tables = append(parsed.Tables, parser.ParsedTable{
		Name: "keyed", Access: "public", StructName: "KeyedStruct",
		Fields: pkFields,
	})
	parsed.Structs["KeyedStruct"] = &parser.ParsedStruct{Name: "KeyedStruct", Fields: pkFields}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)

	// Enum: CustomOrdering=true
	assert.True(t, mod.Types["Status"].CustomOrdering, "enum should have CustomOrdering")

	// Sum type: CustomOrdering=true
	assert.True(t, mod.Types["Value"].CustomOrdering, "sum type should have CustomOrdering")

	// Struct without indexes: CustomOrdering=true (set in resolveStructType)
	assert.True(t, mod.Types["PlainStruct"].CustomOrdering, "plain struct should have CustomOrdering")

	// Struct with PK: CustomOrdering=true
	assert.True(t, mod.Types["KeyedStruct"].CustomOrdering, "keyed struct should have CustomOrdering")
}

// TestAnalyzeTypeAlias verifies type aliases are resolved through to the underlying type.
func TestAnalyzeTypeAlias(t *testing.T) {
	parsed := newParsedModule()
	parsed.TypeAliases["MyAlias"] = "Inner"
	parsed.Structs["Inner"] = &parser.ParsedStruct{
		Name: "Inner",
		Fields: []parser.ParsedField{
			{GoName: "Val", GoType: "uint32", BsatnName: "val"},
		},
	}
	parsed.Structs["Outer"] = &parser.ParsedStruct{
		Name: "Outer",
		Fields: []parser.ParsedField{
			{GoName: "Ref", GoType: "MyAlias", BsatnName: "ref"},
		},
	}
	parsed.Tables = []parser.ParsedTable{{
		Name: "outer", Access: "public", StructName: "Outer",
		Fields: []parser.ParsedField{
			{GoName: "Ref", GoType: "MyAlias", BsatnName: "ref"},
		},
	}}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, mod.Tables, 1)
	require.Len(t, mod.Tables[0].Fields, 1)

	// MyAlias resolves to Inner, which is a struct ref.
	field := mod.Tables[0].Fields[0]
	assert.Equal(t, servergen.AlgKindRef, field.AlgType.Kind)
	assert.Equal(t, "Inner", field.AlgType.TypeName)
}

// TestAnalyzeEmptyModule verifies an empty module produces a valid AnalyzedModule without error.
func TestAnalyzeEmptyModule(t *testing.T) {
	parsed := newParsedModule()

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.NotNil(t, mod)

	assert.Equal(t, "main", mod.PackageName)
	assert.Empty(t, mod.Tables)
	assert.Empty(t, mod.Reducers)
	assert.Empty(t, mod.Lifecycle)
	assert.Empty(t, mod.Procedures)
	assert.Empty(t, mod.Views)
	assert.Empty(t, mod.SumTypes)
	assert.Empty(t, mod.Enums)
	assert.Empty(t, mod.TypeOrder)
	assert.NotNil(t, mod.Types)
	assert.Empty(t, mod.Types)
}

// TestAnalyzeReducerWithError verifies the HasError flag is preserved on analyzed reducers.
func TestAnalyzeReducerWithError(t *testing.T) {
	parsed := newParsedModule()
	parsed.Reducers = []parser.ParsedReducer{
		{Name: "safe_action", FuncName: "SafeAction", HasError: false},
		{Name: "risky_action", FuncName: "RiskyAction", HasError: true},
	}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, mod.Reducers, 2)

	assert.False(t, mod.Reducers[0].HasError)
	assert.True(t, mod.Reducers[1].HasError)
}

// TestAnalyzeProcedureVoidReturn verifies that procedures with no return type have nil ReturnType.
func TestAnalyzeProcedureVoidReturn(t *testing.T) {
	parsed := newParsedModule()
	parsed.Procedures = []parser.ParsedProcedure{{
		Name:       "do_thing",
		FuncName:   "DoThing",
		Params:     nil,
		ReturnType: "", // void
	}}

	mod, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, mod.Procedures, 1)
	assert.Nil(t, mod.Procedures[0].ReturnType)
	assert.Equal(t, "", mod.Procedures[0].ReturnGoType)
}
