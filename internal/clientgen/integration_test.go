//go:build integration

package clientgen_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/clientgen"
)

func TestIntegration_GeneratedCodeCompiles(t *testing.T) {
	testDirs := []string{
		"basic",
		"complex",
		"procedures",
		"proc_returns",
		"views",
		"private_filter",
		"maps",
		"all_builtins",
		"no_pk",
		"multi_table",
		"no_params_reducer",
	}

	for _, dir := range testDirs {
		t.Run(dir, func(t *testing.T) {
			schema := loadTestSchema(t, dir)

			gen, err := clientgen.NewClientGen().
				WithSchema(schema).
				WithOutputDir("test_output").
				WithPackageName("module_bindings").
				WithIncludePrivate(true).
				Build()
			require.NoError(t, err)

			files, err := gen.Generate()
			require.NoError(t, err)

			fset := token.NewFileSet()
			for _, f := range files {
				_, parseErr := parser.ParseFile(fset, f.Name, f.Content, parser.AllErrors)
				assert.NoError(t, parseErr, "generated file %s should be valid Go syntax", f.Name)
			}
		})
	}
}

func TestIntegration_ExtractFromServer(t *testing.T) {
	ext, err := clientgen.NewSchemaExtractor().
		FromServer(integrationServerURL, integrationDBName, "").
		Build()
	require.NoError(t, err)

	schema, err := ext.Extract(t.Context())
	require.NoError(t, err)
	require.NotNil(t, schema)

	// Basic sanity checks
	assert.NotEmpty(t, schema.Typespace, "schema should have typespace entries")
}

func TestIntegration_GenerateFromServer(t *testing.T) {
	ext, err := clientgen.NewSchemaExtractor().
		FromServer(integrationServerURL, integrationDBName, "").
		Build()
	require.NoError(t, err)

	schema, err := ext.Extract(t.Context())
	require.NoError(t, err)

	gen, err := clientgen.NewClientGen().
		WithSchema(schema).
		WithOutputDir("test_output").
		WithPackageName("module_bindings").
		WithIncludePrivate(true).
		Build()
	require.NoError(t, err)

	files, err := gen.Generate()
	require.NoError(t, err)
	assert.NotEmpty(t, files)

	// Verify all generated files are valid Go syntax
	fset := token.NewFileSet()
	for _, f := range files {
		_, parseErr := parser.ParseFile(fset, f.Name, f.Content, parser.AllErrors)
		assert.NoError(t, parseErr, "generated file %s should be valid Go syntax", f.Name)
	}

	// Verify expected files exist
	fileNames := make([]string, len(files))
	for i, f := range files {
		fileNames[i] = f.Name
	}

	expectedFiles := []string{"types_generated.go", "module_generated.go"}
	for _, expected := range expectedFiles {
		assert.Contains(t, fileNames, expected, "should generate %s", expected)
	}
}

func TestIntegration_GoldenFilesExist(t *testing.T) {
	testDirs := []string{
		"basic",
		"complex",
		"procedures",
		"proc_returns",
		"views",
		"private_filter",
		"maps",
		"all_builtins",
		"no_pk",
		"multi_table",
		"no_params_reducer",
	}

	for _, dir := range testDirs {
		t.Run(dir, func(t *testing.T) {
			goldenDir := filepath.Join("..", "..", "testdata", "clientgen", dir)
			entries, err := os.ReadDir(goldenDir)
			require.NoError(t, err)

			hasGolden := false
			for _, e := range entries {
				if filepath.Ext(e.Name()) == ".go" {
					hasGolden = true
					break
				}
			}
			assert.True(t, hasGolden, "directory %s should have golden .go files", dir)
		})
	}
}
