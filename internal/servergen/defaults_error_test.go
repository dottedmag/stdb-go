package servergen_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/parser"
	"go.digitalxero.dev/stdb-go/internal/servergen"
)

// genFromSource writes a single source file, parses, analyzes, and generates it,
// returning the first error encountered along the pipeline.
func genFromSource(t *testing.T, src string) error {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "types.go"), []byte(src), 0644))

	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)

	analyzed, err := servergen.Analyze(parsed)
	if err != nil {
		return err
	}
	_, err = servergen.Generate(analyzed)
	return err
}

func TestDefaultAutoincConflict(t *testing.T) {
	err := genFromSource(t, `package main

//stdb:table name=widget access=public
type Widget struct {
	Id uint64 `+"`"+`stdb:"primarykey,autoinc,default=5"`+"`"+`
}
`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "autoinc")
}

func TestDefaultUnsupportedType(t *testing.T) {
	err := genFromSource(t, `package main

type Point struct {
	X float64
	Y float64
}

//stdb:table name=widget access=public
type Widget struct {
	Id     uint64 `+"`"+`stdb:"primarykey"`+"`"+`
	Origin Point  `+"`"+`stdb:"default=oops"`+"`"+`
}
`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "raw:")
}

func TestDefaultRawEscapeOnStruct(t *testing.T) {
	// The raw: escape makes any type's default expressible.
	err := genFromSource(t, `package main

type Point struct {
	X float64
	Y float64
}

//stdb:table name=widget access=public
type Widget struct {
	Id     uint64 `+"`"+`stdb:"primarykey"`+"`"+`
	Origin Point  `+"`"+`stdb:"default=raw:0x00000000000000000000000000000000"`+"`"+`
}
`)
	require.NoError(t, err)
}
