package clientgen_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/clientgen"
)

func TestClientGen_LifecycleAndHTTPHandlers(t *testing.T) {
	// TypeScript registers the empty lifecycle argument product as a named type.
	var raw clientgen.RawModuleDef
	require.NoError(t, json.Unmarshal([]byte(`{"V10":{"sections":[
		{"Typespace":[{"Product":{"elements":[]}}]},
		{"Types":[{"source_name":{"scope":[],"source_name":"OnConnect"},"ty":0}]},
		{"Reducers":[{"source_name":"onConnect","params":{"elements":[]},"visibility":"ClientCallable"}]},
		{"LifeCycleReducers":[{"lifecycle_spec":"OnConnect","function_name":"on_connect"}]},
		{"ExplicitNames":{"entries":[{"Function":{"source_name":"onConnect","canonical_name":"on_connect"}}]}},
		{"HttpHandlers":[{"source_name":"ping"}]},
		{"HttpRoutes":[{"handler_function":"ping","path":"/ping"}]}
	]}}`), &raw))
	schema, err := clientgen.ResolveSchemaForTest(&raw)
	require.NoError(t, err)

	for _, includePrivate := range []bool{false, true} {
		gen, err := clientgen.NewClientGen().WithSchema(schema).
			WithPackageName("bindings").WithIncludePrivate(includePrivate).Build()
		require.NoError(t, err)
		files, err := gen.Generate()
		require.NoError(t, err)
		assert.Empty(t, files, "handlers should not produce client bindings, includePrivate=%v", includePrivate)
	}
	assert.Len(t, schema.Types, 1, "generation must not mutate the input schema")
	assert.Len(t, schema.Reducers, 1)
}

func TestPruneUnusedTypes_TransitiveAndRecursiveDependencies(t *testing.T) {
	ref := func(n int) *clientgen.AlgebraicType {
		return &clientgen.AlgebraicType{Kind: clientgen.ATKRef, Ref: n}
	}
	product := func(fields ...clientgen.AlgebraicType) clientgen.AlgebraicType {
		pt := &clientgen.ProductType{}
		for _, field := range fields {
			pt.Elements = append(pt.Elements, clientgen.ProductTypeElement{AlgebraicType: field})
		}
		return clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: pt}
	}
	typespace := []clientgen.AlgebraicType{
		product(), // unused
		product(clientgen.AlgebraicType{Kind: clientgen.ATKArray, ArrayTy: ref(2)}),
		{Kind: clientgen.ATKSum, Sum: &clientgen.SumType{Variants: []clientgen.SumTypeVariant{
			{AlgebraicType: product(clientgen.AlgebraicType{Kind: clientgen.ATKMap, MapKey: ref(3), MapValue: ref(4)})},
		}}},
		product(),
		product(*ref(1)), // recursive reference
	}
	types := []clientgen.TypeSchema{
		{Name: "Unused", TypeRef: 0},
		{Name: "Root", TypeRef: 1},
		{Name: "Choice", TypeRef: 2},
		{Name: "Key", TypeRef: 3},
		{Name: "Value", TypeRef: 4},
	}
	params := []clientgen.FieldSchema{{Name: "value", Type: ref(1)}}
	for name, schema := range map[string]*clientgen.ModuleSchema{
		"table":             {Tables: []clientgen.TableSchema{{TypeRef: 1, ProductType: typespace[1].Product}}},
		"reducer params":    {Reducers: []clientgen.ReducerSchema{{Params: params}}},
		"procedure params":  {Procedures: []clientgen.ProcedureSchema{{Params: params}}},
		"procedure returns": {Procedures: []clientgen.ProcedureSchema{{ReturnType: ref(1)}}},
		"view params":       {Views: []clientgen.ViewSchema{{Params: params}}},
		"view returns":      {Views: []clientgen.ViewSchema{{ReturnType: ref(1)}}},
	} {
		t.Run(name, func(t *testing.T) {
			schema.Typespace, schema.Types = typespace, types
			pruned := clientgen.PruneUnusedTypesForTest(schema)
			assert.Equal(t, types[1:], pruned.Types)
			assert.Equal(t, typespace, pruned.Typespace, "typespace references must stay stable")
			assert.Equal(t, types, schema.Types, "the input schema must stay intact")
		})
	}
}

func TestClientGen_TypesFollowVisibility(t *testing.T) {
	schema := loadTestSchema(t, "private_filter")
	for _, includePrivate := range []bool{false, true} {
		gen, err := clientgen.NewClientGen().WithSchema(schema).
			WithPackageName("bindings").WithIncludePrivate(includePrivate).Build()
		require.NoError(t, err)
		files, err := gen.Generate()
		require.NoError(t, err)
		var code string
		for _, file := range files {
			code += string(file.Content)
		}
		assert.Contains(t, code, "type PublicItem struct")
		assert.Equal(t, includePrivate, strings.Contains(code, "type PrivateConfig struct"))
	}

	// A private table's row type can still be part of a public function's API.
	schema.Procedures[0].ReturnType = &clientgen.AlgebraicType{Kind: clientgen.ATKRef, Ref: 1}
	pruned := clientgen.PruneUnusedTypesForTest(clientgen.FilteredSchemaForTest(schema, false))
	assert.Len(t, pruned.Tables, 1)
	assert.Equal(t, schema.Types, pruned.Types)
}
