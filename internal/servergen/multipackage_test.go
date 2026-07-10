package servergen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/parser"
	"go.digitalxero.dev/stdb-go/internal/servergen"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
}

func TestMultiPackageGenerate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.22\n")
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schema"), 0755))
	writeFile(t, filepath.Join(dir, "schema"), "types.go", `package schema

//stdb:table name=player access=public
type Player struct {
	Id   uint64 `+"`"+`stdb:"primarykey,autoinc"`+"`"+`
	Name string
}
`)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "combat"), 0755))
	writeFile(t, filepath.Join(dir, "combat"), "attack.go", `package combat

import "go.digitalxero.dev/spacetimedb-server/reducer"

//stdb:reducer
func MeleeAttack(ctx reducer.ReducerContext, targetId uint64) error {
	_ = targetId
	return nil
}
`)

	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	assert.True(t, parsed.MultiPackage)
	require.Len(t, parsed.Tables, 1)
	require.Len(t, parsed.Reducers, 1)
	assert.Equal(t, "schema", parsed.Tables[0].Package)
	assert.Equal(t, "combat", parsed.Reducers[0].Package)
	assert.True(t, parsed.Reducers[0].Exported)

	analyzed, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, analyzed.Reducers, 1)
	assert.Equal(t, "combat.MeleeAttack", analyzed.Reducers[0].CallName)
	assert.Equal(t, "schema.Player", analyzed.Tables[0].QualifiedStruct)

	files, err := servergen.GenerateAll(analyzed)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(files), 2)

	var moduleSrc, tablesSrc string
	for _, f := range files {
		switch {
		case strings.HasSuffix(f.RelPath, "stdb_module_generated.go"):
			moduleSrc = string(f.Content)
		case strings.Contains(f.RelPath, "stdb_tables_generated.go"):
			tablesSrc = string(f.Content)
		}
	}
	require.NotEmpty(t, moduleSrc, "expected module file")
	require.NotEmpty(t, tablesSrc, "expected tables file")

	assert.Contains(t, moduleSrc, "combat.MeleeAttack")
	assert.Contains(t, moduleSrc, "example.com/m/combat")
	assert.Contains(t, tablesSrc, "PlayerTable")
	assert.Contains(t, tablesSrc, "package schema")
	assert.Contains(t, tablesSrc, "func StdbWritePlayer")
	assert.Contains(t, tablesSrc, "func StdbReadPlayer")
}

func TestMultiPackageCustomStructParam(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.22\n")
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schema"), 0755))
	writeFile(t, filepath.Join(dir, "schema"), "types.go", `package schema

//stdb:table name=player access=public
type Player struct {
	Id   uint64 `+"`"+`stdb:"primarykey,autoinc"`+"`"+`
	Name string
}

// AttackReq is a custom reducer argument owned by schema.
type AttackReq struct {
	TargetId uint64
	SkillId  uint32
}
`)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "combat"), 0755))
	writeFile(t, filepath.Join(dir, "combat"), "attack.go", `package combat

import (
	"example.com/m/schema"
	"go.digitalxero.dev/spacetimedb-server/reducer"
)

//stdb:reducer
func MeleeAttack(ctx reducer.ReducerContext, req schema.AttackReq) error {
	_ = req
	return nil
}
`)

	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	assert.True(t, parsed.MultiPackage)

	// AttackReq must be discoverable as a struct (table-adjacent types).
	require.Contains(t, parsed.Structs, "AttackReq")

	analyzed, err := servergen.Analyze(parsed)
	require.NoError(t, err)
	require.Len(t, analyzed.Reducers, 1)
	require.Len(t, analyzed.Reducers[0].Params, 1)
	assert.Equal(t, "AttackReq", analyzed.Reducers[0].Params[0].AlgType.TypeName)

	// Ensure AttackReq is in the typespace with schema ownership.
	require.Contains(t, analyzed.Types, "AttackReq")
	assert.Equal(t, "schema", analyzed.Types["AttackReq"].Package)
	assert.Equal(t, "schema", analyzed.Types["AttackReq"].RelDir)

	files, err := servergen.GenerateAll(analyzed)
	require.NoError(t, err)

	var moduleSrc, tablesSrc string
	for _, f := range files {
		switch {
		case strings.HasSuffix(f.RelPath, "stdb_module_generated.go"):
			moduleSrc = string(f.Content)
		case strings.Contains(f.RelPath, "schema") && strings.Contains(f.RelPath, "tables"):
			tablesSrc = string(f.Content)
		case f.RelPath == "schema/stdb_tables_generated.go":
			tablesSrc = string(f.Content)
		}
	}
	// Fallback: any tables file
	if tablesSrc == "" {
		for _, f := range files {
			if strings.Contains(f.RelPath, "tables") {
				tablesSrc = string(f.Content)
			}
		}
	}
	require.NotEmpty(t, moduleSrc)
	require.NotEmpty(t, tablesSrc)

	assert.Contains(t, tablesSrc, "func StdbReadAttackReq")
	assert.Contains(t, tablesSrc, "func StdbWriteAttackReq")
	// Root dispatcher must call the exported codec on the schema package.
	assert.Contains(t, moduleSrc, "schema.StdbReadAttackReq")
	assert.Contains(t, moduleSrc, "combat.MeleeAttack")
	assert.Contains(t, moduleSrc, "example.com/m/schema")
}

// TestMultiPackageRootTablesNoDuplicateStdbStrPtr ensures package main does not
// redeclare stdbStrPtr when both root stdb_tables_generated.go and
// stdb_module_generated.go are emitted (root tables + nested packages).
func TestMultiPackageRootTablesNoDuplicateStdbStrPtr(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.22\n")
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	writeFile(t, dir, "root_table.go", `package main

import "go.digitalxero.dev/spacetimedb-server/reducer"

//stdb:table name=workspace access=public
type Workspace struct {
	Id uint64 `+"`"+`stdb:"primarykey,autoinc"`+"`"+`
}

//stdb:reducer
func Ping(ctx reducer.ReducerContext) {}
`)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "workos"), 0755))
	writeFile(t, filepath.Join(dir, "workos"), "account.go", `package workos

//stdb:table name=account access=public
type Account struct {
	Id uint64 `+"`"+`stdb:"primarykey,autoinc"`+"`"+`
}
`)

	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.True(t, parsed.MultiPackage)

	analyzed, err := servergen.Analyze(parsed)
	require.NoError(t, err)

	files, err := servergen.GenerateAll(analyzed)
	require.NoError(t, err)

	var rootTables, moduleSrc, nestedTables string
	for _, f := range files {
		switch f.RelPath {
		case "stdb_tables_generated.go":
			rootTables = string(f.Content)
		case "stdb_module_generated.go":
			moduleSrc = string(f.Content)
		case "workos/stdb_tables_generated.go":
			nestedTables = string(f.Content)
		}
	}
	require.NotEmpty(t, rootTables)
	require.NotEmpty(t, moduleSrc)
	require.NotEmpty(t, nestedTables)

	assert.Contains(t, rootTables, "func stdbStrPtr")
	assert.NotContains(t, moduleSrc, "func stdbStrPtr",
		"module file must not redeclare stdbStrPtr when root tables file defines it")
	assert.Contains(t, nestedTables, "func stdbStrPtr",
		"nested package keeps its own stdbStrPtr")
	assert.Contains(t, moduleSrc, "Ping(ctx)")
}

func TestSinglePackageStillOneFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=player access=public
type Player struct {
	Id uint64 `+"`"+`stdb:"primarykey"`+"`"+`
}

//stdb:reducer
func Ping(ctx interface{}) {}
`)
	// Note: Ping won't typecheck for real modules; parser only needs the directive.
	// Use a proper signature:
	writeFile(t, dir, "types.go", `package main

import "go.digitalxero.dev/spacetimedb-server/reducer"

//stdb:table name=player access=public
type Player struct {
	Id uint64 `+"`"+`stdb:"primarykey"`+"`"+`
}

//stdb:reducer
func Ping(ctx reducer.ReducerContext) {}
`)

	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	assert.False(t, parsed.MultiPackage)

	analyzed, err := servergen.Analyze(parsed)
	require.NoError(t, err)

	files, err := servergen.GenerateAll(analyzed)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "stdb_generated.go", files[0].RelPath)
	assert.Contains(t, string(files[0].Content), "Ping(ctx)")
	assert.Contains(t, string(files[0].Content), "PlayerTable")
}
