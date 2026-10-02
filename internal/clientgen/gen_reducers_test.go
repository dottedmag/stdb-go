package clientgen_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dottedmag/stdb-go/internal/clientgen"
)

func TestGenerateReducers_EmptySchema(t *testing.T) {
	schema := &clientgen.ModuleSchema{}
	result, err := clientgen.GenerateReducersForTest(schema, "test_pkg")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGenerateReducers_WithParams(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Reducers: []clientgen.ReducerSchema{
			{
				Name:       "add_player",
				Visibility: "ClientCallable",
				Params: []clientgen.FieldSchema{
					{Name: "name", Type: &clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
					{Name: "score", Type: &clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU32}},
				},
			},
		},
	}
	result, err := clientgen.GenerateReducersForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	// Args struct
	assert.Contains(t, code, "type addPlayerArgs struct")
	assert.Contains(t, code, "Name string")
	assert.Contains(t, code, "Score uint32")
	// WriteBsatn
	assert.Contains(t, code, "func (a *addPlayerArgs) WriteBsatn(w bsatn.Writer)")
	assert.Contains(t, code, "w.PutString(a.Name)")
	assert.Contains(t, code, "w.PutU32(a.Score)")
	// Caller function
	assert.Contains(t, code, "func CallAddPlayer(conn client.DbConnection, name string, score uint32) error")
	assert.Contains(t, code, `conn.CallReducer("add_player", args)`)
}

func TestGenerateReducers_NoParams(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Reducers: []clientgen.ReducerSchema{
			{
				Name:       "reset_all",
				Visibility: "ClientCallable",
				Params:     []clientgen.FieldSchema{},
			},
		},
	}
	result, err := clientgen.GenerateReducersForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	// No args struct
	assert.NotContains(t, code, "resetAllArgs")
	// Caller function with nil args
	assert.Contains(t, code, "func CallResetAll(conn client.DbConnection) error")
	assert.Contains(t, code, `conn.CallReducer("reset_all", nil)`)
}

func TestGenerateReducers_MultipleReducers(t *testing.T) {
	schema := loadTestSchema(t, "no_params_reducer")
	result, err := clientgen.GenerateReducersForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "CallResetAll")
	assert.Contains(t, code, "CallAddRecord")
}

func TestGenerateReducers_TypesImport(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "__identity__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU256}},
					},
				},
			},
		},
		Reducers: []clientgen.ReducerSchema{
			{
				Name:       "set_owner",
				Visibility: "ClientCallable",
				Params: []clientgen.FieldSchema{
					{Name: "owner", Type: &clientgen.AlgebraicType{Kind: clientgen.ATKRef, Ref: 0}},
				},
			},
		},
	}
	result, err := clientgen.GenerateReducersForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "types.Identity")
	assert.Contains(t, code, `"go.digitalxero.dev/spacetimedb-client/types"`)
}
