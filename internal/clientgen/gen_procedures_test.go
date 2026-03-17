package clientgen_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/clientgen"
)

func TestGenerateProcedures_EmptySchema(t *testing.T) {
	schema := &clientgen.ModuleSchema{}
	result, err := clientgen.GenerateProceduresForTest(schema, "test_pkg")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGenerateProcedures_WithParams(t *testing.T) {
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
		Types: []clientgen.TypeSchema{{Name: "User", TypeRef: 0}},
		Procedures: []clientgen.ProcedureSchema{
			{
				Name:       "get_user",
				Visibility: "ClientCallable",
				Params: []clientgen.FieldSchema{
					{Name: "user_id", Type: &clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
				},
				ReturnType: &clientgen.AlgebraicType{Kind: clientgen.ATKRef, Ref: 0},
			},
		},
	}
	result, err := clientgen.GenerateProceduresForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "type getUserArgs struct")
	assert.Contains(t, code, "UserID uint64")
	assert.Contains(t, code, "func (a *getUserArgs) WriteBsatn(w bsatn.Writer)")
	assert.Contains(t, code, "func CallGetUser(conn client.DbConnection, userID uint64) error")
	assert.Contains(t, code, `conn.CallReducer("get_user", args)`)
}

func TestGenerateProcedures_NoParams(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Procedures: []clientgen.ProcedureSchema{
			{
				Name:       "list_all",
				Visibility: "ClientCallable",
				Params:     []clientgen.FieldSchema{},
				ReturnType: &clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString},
			},
		},
	}
	result, err := clientgen.GenerateProceduresForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.NotContains(t, code, "listAllArgs")
	assert.Contains(t, code, "func CallListAll(conn client.DbConnection) error")
	assert.Contains(t, code, `conn.CallReducer("list_all", nil)`)
}

func TestGenerateProcedures_MultipleProcedures(t *testing.T) {
	schema := loadTestSchema(t, "procedures")
	result, err := clientgen.GenerateProceduresForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "CallGetUser")
	assert.Contains(t, code, "CallSearchUsers")
}
