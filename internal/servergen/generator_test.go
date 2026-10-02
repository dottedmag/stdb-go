package servergen_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dottedmag/stdb-go/internal/parser"
	"github.com/dottedmag/stdb-go/internal/servergen"
)

var update = flag.Bool("update", false, "update golden files")

func runGoldenTest(t *testing.T, name string) {
	t.Helper()
	inputDir := filepath.Join("..", "..", "testdata", name, "input")
	goldenFile := filepath.Join("..", "..", "testdata", name, "golden.go")

	parsed, err := parser.ParseDirectory(inputDir)
	require.NoError(t, err)

	analyzed, err := servergen.Analyze(parsed)
	require.NoError(t, err)

	output, err := servergen.Generate(analyzed)
	require.NoError(t, err)

	if *update {
		err = os.WriteFile(goldenFile, output, 0644)
		require.NoError(t, err)
		return
	}

	expected, err := os.ReadFile(goldenFile)
	require.NoError(t, err)
	assert.Equal(t, string(expected), string(output))
}

func TestGolden_BasicTable(t *testing.T)    { runGoldenTest(t, "basic_table") }
func TestGolden_AllPrimitives(t *testing.T) { runGoldenTest(t, "all_primitives") }
func TestGolden_Indexes(t *testing.T)       { runGoldenTest(t, "indexes") }
func TestGolden_Enums(t *testing.T)         { runGoldenTest(t, "enums") }
func TestGolden_SumTypes(t *testing.T)      { runGoldenTest(t, "sumtypes") }
func TestGolden_Reducers(t *testing.T)      { runGoldenTest(t, "reducers") }
func TestGolden_Lifecycle(t *testing.T)     { runGoldenTest(t, "lifecycle") }
func TestGolden_Procedures(t *testing.T)    { runGoldenTest(t, "procedures") }
func TestGolden_Views(t *testing.T)         { runGoldenTest(t, "views") }
func TestGolden_EventTables(t *testing.T)   { runGoldenTest(t, "event_tables") }
func TestGolden_Schedules(t *testing.T)     { runGoldenTest(t, "schedules") }
func TestGolden_OptionsSlices(t *testing.T) { runGoldenTest(t, "options_slices") }
func TestGolden_MultiTable(t *testing.T)    { runGoldenTest(t, "multi_table") }
func TestGolden_ScopedTypes(t *testing.T)   { runGoldenTest(t, "scoped_types") }
func TestGolden_SpecialTypes(t *testing.T)  { runGoldenTest(t, "special_types") }
func TestGolden_RLS(t *testing.T)           { runGoldenTest(t, "rls") }
func TestGolden_TypeAliases(t *testing.T)   { runGoldenTest(t, "type_aliases") }
func TestGolden_NestedStructs(t *testing.T) { runGoldenTest(t, "nested_structs") }
func TestGolden_Comprehensive(t *testing.T) { runGoldenTest(t, "comprehensive") }
func TestGolden_Defaults(t *testing.T)      { runGoldenTest(t, "defaults") }
