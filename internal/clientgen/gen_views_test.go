package clientgen_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dottedmag/stdb-go/internal/clientgen"
)

func TestGenerateViews_EmptySchema(t *testing.T) {
	schema := &clientgen.ModuleSchema{}
	result, err := clientgen.GenerateViewsForTest(schema, "test_pkg")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGenerateViews_SingleView(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "user_id", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
						{Name: "display_name", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{{Name: "UserSummary", TypeRef: 0}},
		Views: []clientgen.ViewSchema{
			{
				Name:       "UserSummary",
				Index:      0,
				IsPublic:   true,
				Params:     []clientgen.FieldSchema{},
				ReturnType: &clientgen.AlgebraicType{Kind: clientgen.ATKRef, Ref: 0},
			},
		},
	}
	result, err := clientgen.GenerateViewsForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "type userSummaryViewDef struct{}")
	assert.Contains(t, code, `func (userSummaryViewDef) TableName() string { return "UserSummary" }`)
	assert.Contains(t, code, "func (userSummaryViewDef) DecodeRow(r bsatn.Reader) (*UserSummary, error)")
	assert.Contains(t, code, "return ReadUserSummary(r)")
	assert.Contains(t, code, "func (userSummaryViewDef) EncodeRow(row *UserSummary) []byte")
	assert.Contains(t, code, "type UserSummaryView = cache.TypedTableCache[*UserSummary]")
}

func TestGenerateViews_MultipleViews(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{Elements: []clientgen.ProductTypeElement{
				{Name: "a", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
			}}},
			{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{Elements: []clientgen.ProductTypeElement{
				{Name: "b", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU32}},
			}}},
		},
		Types: []clientgen.TypeSchema{
			{Name: "ViewA", TypeRef: 0},
			{Name: "ViewB", TypeRef: 1},
		},
		Views: []clientgen.ViewSchema{
			{Name: "ViewA", Index: 0, IsPublic: true, ReturnType: &clientgen.AlgebraicType{Kind: clientgen.ATKRef, Ref: 0}},
			{Name: "ViewB", Index: 1, IsPublic: true, ReturnType: &clientgen.AlgebraicType{Kind: clientgen.ATKRef, Ref: 1}},
		},
	}
	result, err := clientgen.GenerateViewsForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "viewAViewDef")
	assert.Contains(t, code, "viewBViewDef")
	assert.Contains(t, code, "ViewAView")
	assert.Contains(t, code, "ViewBView")
}

func TestClientGen_ViewReturnRowTypes(t *testing.T) {
	product := clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{
		Elements: []clientgen.ProductTypeElement{
			{Name: "id", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU32}},
		},
	}}
	for _, named := range []bool{false, true} {
		for _, shape := range []string{"product", "ref", "array", "option", "named_option"} {
			t.Run(fmt.Sprintf("named=%v/%s", named, shape), func(t *testing.T) {
				schema := &clientgen.ModuleSchema{Typespace: []clientgen.AlgebraicType{product}}
				if named {
					schema.Types = []clientgen.TypeSchema{{Name: "Row", TypeRef: 0}}
				}
				ret := clientgen.AlgebraicType{Kind: clientgen.ATKRef, Ref: 0}
				switch shape {
				case "product":
					ret = product
				case "array":
					elem := ret
					ret = clientgen.AlgebraicType{Kind: clientgen.ATKArray, ArrayTy: &elem}
				case "option", "named_option":
					ret = clientgen.AlgebraicType{Kind: clientgen.ATKSum, Sum: &clientgen.SumType{Variants: []clientgen.SumTypeVariant{
						{Name: "some", AlgebraicType: ret},
						{Name: "none", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{}}},
					}}}
					if shape == "named_option" {
						schema.Typespace = append(schema.Typespace, ret)
						ret = clientgen.AlgebraicType{Kind: clientgen.ATKRef, Ref: 1}
					}
				}
				schema.Views = []clientgen.ViewSchema{{Name: "visible", IsPublic: true, ReturnType: &ret}}
				gen, err := clientgen.NewClientGen().WithSchema(schema).WithPackageName("bindings").Build()
				require.NoError(t, err)
				files, err := gen.Generate()
				require.NoError(t, err)
				var code string
				for _, file := range files {
					code += string(file.Content)
				}
				rowName := "Visible"
				if named && shape != "product" {
					rowName = "Row"
				}
				assert.Contains(t, code, "type "+rowName+" struct")
				assert.Contains(t, code, "return Read"+rowName+"(r)")
				assert.Contains(t, code, "cache.RegisterTypedTable[*"+rowName+"]")
			})
		}
	}
}
