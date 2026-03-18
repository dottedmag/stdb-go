package clientgen_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/clientgen"
)

func TestGenerateModule_EmptySchema(t *testing.T) {
	schema := &clientgen.ModuleSchema{}
	result, err := clientgen.GenerateModuleForTest(schema, "test_pkg")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGenerateModule_TablesOnly(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "id", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{{Name: "Player", TypeRef: 0}},
		Tables: []clientgen.TableSchema{
			{
				Name:       "Player",
				TypeRef:    0,
				PrimaryKey: []int{0},
				ProductType: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "id", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
					},
				},
			},
		},
	}
	result, err := clientgen.GenerateModuleForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "type ModuleBindings struct")
	assert.Contains(t, code, "Player *PlayerTable")
	assert.Contains(t, code, "conn client.DbConnection")
	assert.Contains(t, code, "func NewModuleBindings(conn client.DbConnection) *ModuleBindings")
	assert.Contains(t, code, "cache.RegisterTypedTableWithPK[*Player, uint64]")
	assert.Contains(t, code, "func (m *ModuleBindings) Conn() client.DbConnection")
}

func TestGenerateModule_ViewsOnly(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "value", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
				},
			}},
		},
		Types: []clientgen.TypeSchema{{Name: "Summary", TypeRef: 0}},
		Views: []clientgen.ViewSchema{
			{Name: "Summary", Index: 0, IsPublic: true},
		},
	}
	result, err := clientgen.GenerateModuleForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "Summary *SummaryView")
	assert.Contains(t, code, "cache.RegisterTypedTable[*Summary]")
}

func TestGenerateModule_TablesAndViews(t *testing.T) {
	schema := loadTestSchema(t, "views")
	result, err := clientgen.GenerateModuleForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "User *UserTable")
	assert.Contains(t, code, "UserSummary *UserSummaryView")
}

func TestGenerateModule_TableWithPK_RegisterTypedTableWithPK(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "id", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
						{Name: "value", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{{Name: "Record", TypeRef: 0}},
		Tables: []clientgen.TableSchema{
			{
				Name:       "Record",
				TypeRef:    0,
				PrimaryKey: []int{0},
				ProductType: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "id", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
						{Name: "value", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
					},
				},
			},
		},
	}
	result, err := clientgen.GenerateModuleForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "cache.RegisterTypedTableWithPK[*Record, uint64]")
}

func TestGenerateModule_TableWithoutPK_RegisterTypedTable(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "value", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{{Name: "EventLog", TypeRef: 0}},
		Tables: []clientgen.TableSchema{
			{
				Name:    "EventLog",
				TypeRef: 0,
				ProductType: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "value", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
					},
				},
			},
		},
	}
	result, err := clientgen.GenerateModuleForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "cache.RegisterTypedTable[*EventLog]")
	assert.NotContains(t, code, "RegisterTypedTableWithPK")
}

func TestGenerateModule_TableNameDiffersFromTypeName(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "id", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
						{Name: "name", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{{Name: "User", TypeRef: 0}},
		Tables: []clientgen.TableSchema{
			{
				Name:       "users",
				TypeRef:    0,
				PrimaryKey: []int{0},
				ProductType: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "id", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
						{Name: "name", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
					},
				},
				Access: "Public",
			},
		},
	}
	result, err := clientgen.GenerateModuleForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)

	// Struct field uses table name, type alias uses type name
	assert.Contains(t, code, "Users *UserTable")
	// Registration uses type name for type params
	assert.Contains(t, code, "cache.RegisterTypedTableWithPK[*User, uint64]")
	// Should NOT contain wrong type references
	assert.NotContains(t, code, "*Users,")
	assert.NotContains(t, code, "UsersTable")
}

func TestGenerateModule_ConnAccessor(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Tables: []clientgen.TableSchema{
			{
				Name:    "Dummy",
				TypeRef: 0,
				ProductType: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "x", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU32}},
					},
				},
			},
		},
		Typespace: []clientgen.AlgebraicType{
			{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "x", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU32}},
				},
			}},
		},
	}
	result, err := clientgen.GenerateModuleForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "func (m *ModuleBindings) Conn() client.DbConnection")
	assert.Contains(t, code, "return m.conn")
}
