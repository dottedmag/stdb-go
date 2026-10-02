package clientgen_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dottedmag/stdb-go/internal/clientgen"
)

func TestResolveSchema_NilV10(t *testing.T) {
	raw := &clientgen.RawModuleDef{V10: nil}
	_, err := clientgen.ResolveSchemaForTest(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "V10")
}

func TestResolveSchema_EmptySections(t *testing.T) {
	raw := &clientgen.RawModuleDef{
		V10: &clientgen.RawModuleDefV10{
			Sections: []clientgen.RawModuleDefV10Section{},
		},
	}
	schema, err := clientgen.ResolveSchemaForTest(raw)
	require.NoError(t, err)
	assert.Empty(t, schema.Tables)
	assert.Empty(t, schema.Reducers)
	assert.Empty(t, schema.Procedures)
	assert.Empty(t, schema.Views)
	assert.Empty(t, schema.Types)
	assert.Empty(t, schema.Typespace)
}

func TestResolveSchema_TypespaceExtracted(t *testing.T) {
	schema := loadTestSchema(t, "basic")

	require.Len(t, schema.Typespace, 2)
	assert.Equal(t, clientgen.ATKProduct, schema.Typespace[0].Kind)
	assert.Equal(t, clientgen.ATKSum, schema.Typespace[1].Kind)
}

func TestResolveSchema_TypesResolved(t *testing.T) {
	schema := loadTestSchema(t, "basic")

	require.Len(t, schema.Types, 2)

	playerType := schema.Types[0]
	assert.Equal(t, "Player", playerType.Name)
	assert.Empty(t, playerType.Scope)
	assert.Equal(t, 0, playerType.TypeRef)
	assert.False(t, playerType.CustomOrdering)

	statusType := schema.Types[1]
	assert.Equal(t, "PlayerStatus", statusType.Name)
	assert.Equal(t, 1, statusType.TypeRef)
}

func TestResolveSchema_TablesResolved(t *testing.T) {
	schema := loadTestSchema(t, "basic")

	require.Len(t, schema.Tables, 1)
	table := schema.Tables[0]
	assert.Equal(t, "Player", table.Name)
	require.NotNil(t, table.ProductType)
	assert.Len(t, table.ProductType.Elements, 3)
	assert.Equal(t, []int{0}, table.PrimaryKey)
	assert.Equal(t, "Public", table.Access)
	assert.Equal(t, "User", table.TableType)
	assert.False(t, table.IsEvent)
	assert.Equal(t, 0, table.TypeRef)
}

func TestResolveSchema_TableWithoutPK(t *testing.T) {
	schema := loadTestSchema(t, "no_pk")

	require.Len(t, schema.Tables, 1)
	table := schema.Tables[0]
	assert.Equal(t, "EventLog", table.Name)
	assert.Nil(t, table.PrimaryKey)
	assert.True(t, table.IsEvent)
}

func TestResolveSchema_MultipleTables(t *testing.T) {
	schema := loadTestSchema(t, "multi_table")

	require.Len(t, schema.Tables, 3)
	assert.Equal(t, "Author", schema.Tables[0].Name)
	assert.Equal(t, "Post", schema.Tables[1].Name)
	assert.Equal(t, "Comment", schema.Tables[2].Name)

	// Comment doesn't have a named type ref, but still has product type from typespace
	require.NotNil(t, schema.Tables[2].ProductType)
	assert.Len(t, schema.Tables[2].ProductType.Elements, 4)
}

func TestResolveSchema_ReducersResolved(t *testing.T) {
	schema := loadTestSchema(t, "basic")

	require.Len(t, schema.Reducers, 2)

	r0 := schema.Reducers[0]
	assert.Equal(t, "add_player", r0.Name)
	assert.Equal(t, "ClientCallable", r0.Visibility)
	require.Len(t, r0.Params, 2)
	assert.Equal(t, "name", r0.Params[0].Name)
	assert.Equal(t, "score", r0.Params[1].Name)
}

func TestResolveSchema_ProceduresResolved(t *testing.T) {
	schema := loadTestSchema(t, "procedures")

	require.Len(t, schema.Procedures, 2)

	p0 := schema.Procedures[0]
	assert.Equal(t, "get_user", p0.Name)
	assert.Equal(t, "ClientCallable", p0.Visibility)
	require.Len(t, p0.Params, 1)
	assert.Equal(t, "user_id", p0.Params[0].Name)
	require.NotNil(t, p0.ReturnType)

	p1 := schema.Procedures[1]
	assert.Equal(t, "search_users", p1.Name)
	assert.Len(t, p1.Params, 2)
}

func TestResolveSchema_ViewsResolved(t *testing.T) {
	schema := loadTestSchema(t, "views")

	require.Len(t, schema.Views, 1)

	v := schema.Views[0]
	assert.Equal(t, "UserSummary", v.Name)
	assert.Equal(t, 1, v.Index)
	assert.True(t, v.IsPublic)
	assert.False(t, v.IsAnonymous)
	assert.Empty(t, v.Params)
	require.NotNil(t, v.ReturnType)
}

func TestResolveSchema_ProductTypeFromTypespace(t *testing.T) {
	schema := loadTestSchema(t, "basic")

	require.Len(t, schema.Tables, 1)
	table := schema.Tables[0]

	// The table's ProductType should be the same as typespace[0]
	require.NotNil(t, table.ProductType)
	assert.Len(t, table.ProductType.Elements, 3)
	assert.Equal(t, "id", table.ProductType.Elements[0].Name)

	// Verify it matches typespace entry
	require.Equal(t, clientgen.ATKProduct, schema.Typespace[0].Kind)
	assert.Equal(t, table.ProductType, schema.Typespace[0].Product)
}

func TestResolveParams(t *testing.T) {
	pt := clientgen.ProductType{
		Elements: []clientgen.ProductTypeElement{
			{Name: "name", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
			{Name: "age", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU32}},
		},
	}

	fields := clientgen.ResolveParamsForTest(pt)
	require.Len(t, fields, 2)
	assert.Equal(t, "name", fields[0].Name)
	require.NotNil(t, fields[0].Type)
	assert.Equal(t, clientgen.ATKBuiltin, fields[0].Type.Kind)
	assert.Equal(t, clientgen.BuiltinString, fields[0].Type.Builtin)

	assert.Equal(t, "age", fields[1].Name)
	assert.Equal(t, clientgen.BuiltinU32, fields[1].Type.Builtin)
}

func TestResolveParams_Empty(t *testing.T) {
	pt := clientgen.ProductType{
		Elements: []clientgen.ProductTypeElement{},
	}

	fields := clientgen.ResolveParamsForTest(pt)
	assert.Empty(t, fields)
}
