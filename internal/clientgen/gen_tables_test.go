package clientgen_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/clientgen"
)

func TestGenerateTables_EmptySchema(t *testing.T) {
	schema := &clientgen.ModuleSchema{}
	result, err := clientgen.GenerateTablesForTest(schema, "test_pkg")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGenerateTables_SingleTableWithPK(t *testing.T) {
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
		Types: []clientgen.TypeSchema{{Name: "Player", TypeRef: 0}},
		Tables: []clientgen.TableSchema{
			{
				Name:       "Player",
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
	result, err := clientgen.GenerateTablesForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "type playerTableDef struct{}")
	assert.Contains(t, code, `func (playerTableDef) TableName() string { return "Player" }`)
	assert.Contains(t, code, "func (playerTableDef) DecodeRow(r bsatn.Reader) (*Player, error)")
	assert.Contains(t, code, "return ReadPlayer(r)")
	assert.Contains(t, code, "func (playerTableDef) EncodeRow(row *Player) []byte")
	assert.Contains(t, code, "func (playerTableDef) PrimaryKey(row *Player) uint64")
	assert.Contains(t, code, "return row.ID")
	assert.Contains(t, code, "type PlayerTable = cache.TypedTableCache[*Player]")
}

func TestGenerateTables_TableWithoutPK(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "event_type", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
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
						{Name: "event_type", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
					},
				},
				Access: "Public",
			},
		},
	}
	result, err := clientgen.GenerateTablesForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "type eventLogTableDef struct{}")
	assert.NotContains(t, code, "PrimaryKey(")
}

func TestGenerateTables_NilProductTypeSkipped(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Tables: []clientgen.TableSchema{
			{Name: "broken", ProductType: nil},
		},
	}
	result, err := clientgen.GenerateTablesForTest(schema, "test_pkg")
	require.NoError(t, err)
	// The table should be skipped since ProductType is nil, but we still have
	// the header code generated since we had at least one table
	// Actually let me check: generateTables writes header + imports first,
	// then iterates tables. If all skip, we still get output.
	// But the table code is empty so the formatted output will just be imports.
	if result != nil {
		code := string(result)
		assert.NotContains(t, code, "brokenTableDef")
	}
}

func TestGenerateTables_MultipleTables(t *testing.T) {
	schema := loadTestSchema(t, "multi_table")
	result, err := clientgen.GenerateTablesForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "authorTableDef")
	assert.Contains(t, code, "postTableDef")
	assert.Contains(t, code, "commentTableDef")
	assert.Contains(t, code, "AuthorTable")
	assert.Contains(t, code, "PostTable")
	assert.Contains(t, code, "CommentTable")
}

func TestGenerateTables_TableNameDiffersFromTypeName(t *testing.T) {
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
	result, err := clientgen.GenerateTablesForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)

	// Table def name should use the table name
	assert.Contains(t, code, "type usersTableDef struct{}")
	assert.Contains(t, code, `func (usersTableDef) TableName() string { return "users" }`)

	// Go type references should use the type name (User), not the table name (Users)
	assert.Contains(t, code, "func (usersTableDef) DecodeRow(r bsatn.Reader) (*User, error)")
	assert.Contains(t, code, "return ReadUser(r)")
	assert.Contains(t, code, "func (usersTableDef) EncodeRow(row *User) []byte")
	assert.Contains(t, code, "func (usersTableDef) PrimaryKey(row *User) uint64")
	assert.Contains(t, code, "type UserTable = cache.TypedTableCache[*User]")

	// Should NOT contain the wrong table-name-derived type references
	assert.NotContains(t, code, "*Users")
	assert.NotContains(t, code, "ReadUsers")
	assert.NotContains(t, code, "UsersTable")
}

func TestGenerateTables_TypesImportForPK(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			// 0: Identity special type
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "__identity__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU256}},
					},
				},
			},
			// 1: struct with identity ref
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "owner", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKRef, Ref: 0}},
						{Name: "name", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{{Name: "OwnedEntity", TypeRef: 1}},
		Tables: []clientgen.TableSchema{
			{
				Name:       "OwnedEntity",
				TypeRef:    1,
				PrimaryKey: []int{0}, // PK is the owner (Identity)
				ProductType: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "owner", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKRef, Ref: 0}},
						{Name: "name", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
					},
				},
			},
		},
	}
	result, err := clientgen.GenerateTablesForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "types.Identity")
	assert.Contains(t, code, `"go.digitalxero.dev/spacetimedb-client/types"`)
}
