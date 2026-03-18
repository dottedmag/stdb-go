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

// --- Golden File Tests ---

func TestClientGen_Basic(t *testing.T) {
	runClientGenGoldenTest(t, "basic")
}

func TestClientGen_Complex(t *testing.T) {
	runClientGenGoldenTest(t, "complex")
}

func TestClientGen_Procedures(t *testing.T) {
	runClientGenGoldenTest(t, "procedures")
}

func TestClientGen_Views(t *testing.T) {
	runClientGenGoldenTest(t, "views")
}

func TestClientGen_PrivateFilter(t *testing.T) {
	runClientGenGoldenTest(t, "private_filter")
}

func TestClientGen_Maps(t *testing.T) {
	runClientGenGoldenTest(t, "maps")
}

func TestClientGen_AllBuiltins(t *testing.T) {
	runClientGenGoldenTest(t, "all_builtins")
}

func TestClientGen_NoPK(t *testing.T) {
	runClientGenGoldenTest(t, "no_pk")
}

func TestClientGen_MultiTable(t *testing.T) {
	runClientGenGoldenTest(t, "multi_table")
}

func TestClientGen_NoParamsReducer(t *testing.T) {
	runClientGenGoldenTest(t, "no_params_reducer")
}

func TestClientGen_TableTypeMismatch(t *testing.T) {
	runClientGenGoldenTest(t, "table_type_mismatch")
}

// --- Schema Parser Tests ---

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

func TestSchemaParser_Procedures(t *testing.T) {
	schema := loadTestSchema(t, "procedures")
	assert.Len(t, schema.Procedures, 2)
	assert.Equal(t, "get_user", schema.Procedures[0].Name)
	assert.Equal(t, "search_users", schema.Procedures[1].Name)
}

func TestSchemaParser_Views(t *testing.T) {
	schema := loadTestSchema(t, "views")
	assert.Len(t, schema.Views, 1)
	assert.Equal(t, "UserSummary", schema.Views[0].Name)
	assert.True(t, schema.Views[0].IsPublic)
}

func TestSchemaParser_Maps(t *testing.T) {
	schema := loadTestSchema(t, "maps")
	require.Len(t, schema.Tables, 1)
	table := schema.Tables[0]
	require.NotNil(t, table.ProductType)
	// metadata field: Map<String, String>
	metaElem := table.ProductType.Elements[1]
	assert.Equal(t, "metadata", metaElem.Name)
	assert.Equal(t, clientgen.ATKMap, metaElem.AlgebraicType.Kind)
}

func TestSchemaParser_AllBuiltins(t *testing.T) {
	schema := loadTestSchema(t, "all_builtins")
	require.Len(t, schema.Tables, 1)
	table := schema.Tables[0]
	require.NotNil(t, table.ProductType)
	assert.Len(t, table.ProductType.Elements, 17)
}

// --- Builder Validation Tests ---

func TestClientGenBuilder_NilSchema(t *testing.T) {
	_, err := clientgen.NewClientGen().
		WithPackageName("test").
		Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema is required")
}

func TestClientGenBuilder_EmptyPackageName(t *testing.T) {
	schema := &clientgen.ModuleSchema{}
	_, err := clientgen.NewClientGen().
		WithSchema(schema).
		Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "package name is required")
}

func TestClientGenBuilder_Success(t *testing.T) {
	schema := &clientgen.ModuleSchema{}
	gen, err := clientgen.NewClientGen().
		WithSchema(schema).
		WithOutputDir("output").
		WithPackageName("test_pkg").
		Build()
	require.NoError(t, err)
	assert.NotNil(t, gen)
}

// --- FilteredSchema Tests ---

func TestFilteredSchema_IncludePrivateTrue(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Tables:     []clientgen.TableSchema{{Name: "pub", Access: "Public"}, {Name: "priv", Access: "Private"}},
		Reducers:   []clientgen.ReducerSchema{{Name: "pub_r", Visibility: "ClientCallable"}, {Name: "priv_r", Visibility: "Private"}},
		Procedures: []clientgen.ProcedureSchema{{Name: "pub_p", Visibility: "ClientCallable"}, {Name: "priv_p", Visibility: "Private"}},
		Views:      []clientgen.ViewSchema{{Name: "pub_v", IsPublic: true}, {Name: "priv_v", IsPublic: false}},
		Typespace:  []clientgen.AlgebraicType{{Kind: clientgen.ATKBuiltin}},
		Types:      []clientgen.TypeSchema{{Name: "T1"}},
	}

	filtered := clientgen.FilteredSchemaForTest(schema, true)
	// When includePrivate is true, filtered returns original schema
	assert.Equal(t, schema, filtered)
}

func TestFilteredSchema_IncludePrivateFalse(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Tables:     []clientgen.TableSchema{{Name: "pub", Access: "Public"}, {Name: "priv", Access: "Private"}},
		Reducers:   []clientgen.ReducerSchema{{Name: "pub_r", Visibility: "ClientCallable"}, {Name: "priv_r", Visibility: "Private"}},
		Procedures: []clientgen.ProcedureSchema{{Name: "pub_p", Visibility: "ClientCallable"}, {Name: "priv_p", Visibility: "Private"}},
		Views:      []clientgen.ViewSchema{{Name: "pub_v", IsPublic: true}, {Name: "priv_v", IsPublic: false}},
		Typespace:  []clientgen.AlgebraicType{{Kind: clientgen.ATKBuiltin}},
		Types:      []clientgen.TypeSchema{{Name: "T1"}},
	}

	filtered := clientgen.FilteredSchemaForTest(schema, false)
	assert.Len(t, filtered.Tables, 1)
	assert.Equal(t, "pub", filtered.Tables[0].Name)
	assert.Len(t, filtered.Reducers, 1)
	assert.Equal(t, "pub_r", filtered.Reducers[0].Name)
	assert.Len(t, filtered.Procedures, 1)
	assert.Equal(t, "pub_p", filtered.Procedures[0].Name)
	assert.Len(t, filtered.Views, 1)
	assert.Equal(t, "pub_v", filtered.Views[0].Name)
}

func TestFilteredSchema_EmptySchema(t *testing.T) {
	schema := &clientgen.ModuleSchema{}
	filtered := clientgen.FilteredSchemaForTest(schema, false)
	assert.Empty(t, filtered.Tables)
	assert.Empty(t, filtered.Reducers)
	assert.Empty(t, filtered.Procedures)
	assert.Empty(t, filtered.Views)
}

func TestFilteredSchema_AllPrivate(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Tables:     []clientgen.TableSchema{{Name: "t1", Access: "Private"}, {Name: "t2", Access: "Private"}},
		Reducers:   []clientgen.ReducerSchema{{Name: "r1", Visibility: "Private"}},
		Procedures: []clientgen.ProcedureSchema{{Name: "p1", Visibility: "Private"}},
		Views:      []clientgen.ViewSchema{{Name: "v1", IsPublic: false}},
	}

	filtered := clientgen.FilteredSchemaForTest(schema, false)
	assert.Empty(t, filtered.Tables)
	assert.Empty(t, filtered.Reducers)
	assert.Empty(t, filtered.Procedures)
	assert.Empty(t, filtered.Views)
}

func TestFilteredSchema_PreservesTypespaceAndTypes(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64},
			{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString},
		},
		Types: []clientgen.TypeSchema{
			{Name: "Type1", TypeRef: 0},
			{Name: "Type2", TypeRef: 1},
		},
		Tables: []clientgen.TableSchema{{Name: "priv", Access: "Private"}},
	}

	filtered := clientgen.FilteredSchemaForTest(schema, false)
	assert.Len(t, filtered.Typespace, 2, "Typespace should be preserved")
	assert.Len(t, filtered.Types, 2, "Types should be preserved")
	assert.Empty(t, filtered.Tables, "Private tables should be filtered")
}
