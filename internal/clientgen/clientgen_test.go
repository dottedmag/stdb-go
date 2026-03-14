package clientgen_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/clientgen"
)

var update = flag.Bool("update", false, "update golden files")

func loadTestSchema(t *testing.T, name string) *clientgen.ModuleSchema {
	t.Helper()
	schemaPath := filepath.Join("..", "..", "testdata", "clientgen", name, "schema.json")
	data, err := os.ReadFile(schemaPath)
	require.NoError(t, err, "reading schema file")

	var rawDef clientgen.RawModuleDef
	err = json.Unmarshal(data, &rawDef)
	require.NoError(t, err, "parsing schema JSON")

	// Use the exported resolver
	schema, err := clientgen.ResolveSchemaForTest(&rawDef)
	require.NoError(t, err, "resolving schema")

	return schema
}

func runClientGenGoldenTest(t *testing.T, name string) {
	t.Helper()
	schema := loadTestSchema(t, name)

	gen, err := clientgen.NewClientGen().
		WithSchema(schema).
		WithOutputDir("test_output").
		WithPackageName("module_bindings").
		WithIncludePrivate(true).
		Build()
	require.NoError(t, err, "building client gen")

	files, err := gen.Generate()
	require.NoError(t, err, "generating code")
	require.NotEmpty(t, files, "should generate at least one file")

	goldenDir := filepath.Join("..", "..", "testdata", "clientgen", name)

	for _, f := range files {
		goldenPath := filepath.Join(goldenDir, "golden_"+f.Name)

		if *update {
			err = os.WriteFile(goldenPath, f.Content, 0644)
			require.NoError(t, err, "writing golden file %s", goldenPath)
			continue
		}

		expected, err := os.ReadFile(goldenPath)
		require.NoError(t, err, "reading golden file %s", goldenPath)
		assert.Equal(t, string(expected), string(f.Content),
			"generated %s does not match golden file", f.Name)
	}
}

func TestClientGen_Basic(t *testing.T) {
	runClientGenGoldenTest(t, "basic")
}

func TestSchemaParser_Basic(t *testing.T) {
	schema := loadTestSchema(t, "basic")

	assert.Len(t, schema.Tables, 1, "should have 1 table")
	assert.Len(t, schema.Reducers, 2, "should have 2 reducers")
	assert.Len(t, schema.Types, 2, "should have 2 types")

	// Verify table
	table := schema.Tables[0]
	assert.Equal(t, "Player", table.Name)
	assert.Equal(t, "Public", table.Access)
	require.NotNil(t, table.ProductType)
	assert.Len(t, table.ProductType.Elements, 3)
	assert.Equal(t, "id", table.ProductType.Elements[0].Name)
	assert.Equal(t, "name", table.ProductType.Elements[1].Name)
	assert.Equal(t, "score", table.ProductType.Elements[2].Name)
	assert.Equal(t, []int{0}, table.PrimaryKey)

	// Verify reducers
	assert.Equal(t, "add_player", schema.Reducers[0].Name)
	assert.Equal(t, "ClientCallable", schema.Reducers[0].Visibility)
	assert.Len(t, schema.Reducers[0].Params, 2)

	assert.Equal(t, "update_score", schema.Reducers[1].Name)
	assert.Len(t, schema.Reducers[1].Params, 2)
}

func TestClientGen_Complex(t *testing.T) {
	runClientGenGoldenTest(t, "complex")
}

func TestSchemaParser_Complex(t *testing.T) {
	schema := loadTestSchema(t, "complex")

	assert.Len(t, schema.Tables, 1, "should have 1 table")
	assert.Len(t, schema.Reducers, 1, "should have 1 reducer")
	assert.Len(t, schema.Types, 2, "should have 2 types")

	// Verify table with special types
	table := schema.Tables[0]
	assert.Equal(t, "Item", table.Name)
	require.NotNil(t, table.ProductType)
	assert.Len(t, table.ProductType.Elements, 7)

	// owner field should be a Ref to Identity (typespace[0])
	ownerElem := table.ProductType.Elements[1]
	assert.Equal(t, "owner", ownerElem.Name)
	assert.Equal(t, clientgen.ATKRef, ownerElem.AlgebraicType.Kind)
	assert.Equal(t, 0, ownerElem.AlgebraicType.Ref)

	// description should be Option<String>
	descElem := table.ProductType.Elements[3]
	assert.Equal(t, "description", descElem.Name)
	assert.Equal(t, clientgen.ATKSum, descElem.AlgebraicType.Kind)

	// tags should be Array<String>
	tagsElem := table.ProductType.Elements[4]
	assert.Equal(t, "tags", tagsElem.Name)
	assert.Equal(t, clientgen.ATKArray, tagsElem.AlgebraicType.Kind)

	// data should be Array<U8> (i.e. []byte)
	dataElem := table.ProductType.Elements[6]
	assert.Equal(t, "data", dataElem.Name)
	assert.Equal(t, clientgen.ATKArray, dataElem.AlgebraicType.Kind)

	// Verify sum type
	msgType := schema.Types[1]
	assert.Equal(t, "MessageContent", msgType.Name)
}

func TestGenCommon_ToGoName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"player", "Player"},
		{"add_player", "AddPlayer"},
		{"player_id", "PlayerID"},
		{"http_url", "HTTPURL"},
		{"simple", "Simple"},
		{"my_api_key", "MyAPIKey"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := clientgen.ToGoNameForTest(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenCommon_DetectSpecialType(t *testing.T) {
	tests := []struct {
		name     string
		product  clientgen.ProductType
		expected string
	}{
		{
			name: "Identity",
			product: clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "__identity__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU256}},
				},
			},
			expected: "types.Identity",
		},
		{
			name: "ConnectionId",
			product: clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "__connection_id__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU128}},
				},
			},
			expected: "types.ConnectionId",
		},
		{
			name: "Timestamp",
			product: clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "__timestamp_micros_since_unix_epoch__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinI64}},
				},
			},
			expected: "types.Timestamp",
		},
		{
			name: "Regular product - not special",
			product: clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "id", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clientgen.DetectSpecialTypeForTest(&tt.product)
			assert.Equal(t, tt.expected, result)
		})
	}
}
