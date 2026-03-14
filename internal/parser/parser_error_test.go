package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/parser"
)

// errWriteFile is a test helper that writes content to a file in the given directory.
func errWriteFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
}

func TestParseError_InvalidGoSource(t *testing.T) {
	dir := t.TempDir()
	errWriteFile(t, dir, "bad.go", "package main\nfunc {broken")

	_, err := parser.ParseDirectory(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestParseError_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	assert.Empty(t, parsed.Tables)
	assert.Empty(t, parsed.Reducers)
	assert.Empty(t, parsed.Lifecycle)
	assert.Empty(t, parsed.Procedures)
	assert.Empty(t, parsed.Views)
}

func TestParseError_NonExistentDir(t *testing.T) {
	_, err := parser.ParseDirectory("/tmp/nonexistent-stdb-gen-test-dir-that-does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read dir")
}

func TestParseError_InvalidMultiColIndex(t *testing.T) {
	dir := t.TempDir()
	errWriteFile(t, dir, "tables.go", `package test

//stdb:table name=test access=public index=myidx:abc
type Test struct {
	Id   uint64 `+"`"+`stdb:"primarykey"`+"`"+`
	Name string
}
`)

	_, err := parser.ParseDirectory(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid column")
}

func TestParseError_InvalidMultiColIndexMissingColon(t *testing.T) {
	dir := t.TempDir()
	errWriteFile(t, dir, "tables.go", `package test

//stdb:table name=test access=public index=badformat
type Test struct {
	Id   uint64
	Name string
}
`)

	_, err := parser.ParseDirectory(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid index spec")
}
