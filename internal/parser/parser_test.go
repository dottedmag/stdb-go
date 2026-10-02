package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/dottedmag/stdb-go/internal/parser"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
}

// ──────────────────────────────────────────────────────────
// Directive extraction
// ──────────────────────────────────────────────────────────

func TestParseTableDirective(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=person access=public
type Person struct {
	Id   uint64 `+"`"+`stdb:"primarykey,autoinc"`+"`"+`
	Name string
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)

	table := parsed.Tables[0]
	assert.Equal(t, "person", table.Name)
	assert.Equal(t, "public", table.Access)
	assert.Equal(t, "Person", table.StructName)
	assert.False(t, table.IsEvent)
	require.Len(t, table.Fields, 2)
	assert.Equal(t, "Id", table.Fields[0].GoName)
	assert.Equal(t, "uint64", table.Fields[0].GoType)
	assert.Equal(t, "id", table.Fields[0].BsatnName)
	assert.True(t, table.Fields[0].PrimaryKey)
	assert.True(t, table.Fields[0].AutoInc)
	assert.Equal(t, "Name", table.Fields[1].GoName)
	assert.Equal(t, "string", table.Fields[1].GoType)
	assert.Equal(t, "name", table.Fields[1].BsatnName)
}

func TestParseTableWithEvent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=log_entry access=public event=true
type LogEntry struct {
	Message string
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)

	table := parsed.Tables[0]
	assert.Equal(t, "log_entry", table.Name)
	assert.Equal(t, "public", table.Access)
	assert.True(t, table.IsEvent)
	assert.Equal(t, "LogEntry", table.StructName)
	require.Len(t, table.Fields, 1)
	assert.Equal(t, "Message", table.Fields[0].GoName)
}

func TestParseTableMultiColIndex(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=item access=public index=myidx:0,1
type Item struct {
	Category string
	Name     string
	Price    float64
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)

	table := parsed.Tables[0]
	assert.Equal(t, "item", table.Name)
	require.Len(t, table.ExtraIndexes, 1)
	assert.Equal(t, "myidx", table.ExtraIndexes[0].Name)
	assert.Equal(t, []uint16{0, 1}, table.ExtraIndexes[0].Columns)
}

func TestParseReducerDirective(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "reducers.go", `package main

import "context"

//stdb:reducer
func AddPlayer(ctx context.Context, name string, score uint32) {
}

//stdb:reducer name=custom_name
func MyCustomReducer(ctx context.Context, value int64) error {
	return nil
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Reducers, 2)

	// Auto-generated name from function name.
	r0 := parsed.Reducers[0]
	assert.Equal(t, "add_player", r0.Name)
	assert.Equal(t, "AddPlayer", r0.FuncName)
	require.Len(t, r0.Params, 2)
	assert.Equal(t, "name", r0.Params[0].Name)
	assert.Equal(t, "string", r0.Params[0].GoType)
	assert.Equal(t, "score", r0.Params[1].Name)
	assert.Equal(t, "uint32", r0.Params[1].GoType)
	assert.False(t, r0.HasError)

	// Explicit name.
	r1 := parsed.Reducers[1]
	assert.Equal(t, "custom_name", r1.Name)
	assert.Equal(t, "MyCustomReducer", r1.FuncName)
	require.Len(t, r1.Params, 1)
	assert.Equal(t, "value", r1.Params[0].Name)
	assert.Equal(t, "int64", r1.Params[0].GoType)
	assert.True(t, r1.HasError)
}

func TestParseLifecycleDirectives(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "lifecycle.go", `package main

import "context"

//stdb:init
func Init(ctx context.Context) {
}

//stdb:connect
func OnConnect(ctx context.Context) {
}

//stdb:disconnect
func OnDisconnect(ctx context.Context) {
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Lifecycle, 3)

	assert.Equal(t, "init", parsed.Lifecycle[0].Kind)
	assert.Equal(t, "Init", parsed.Lifecycle[0].FuncName)

	assert.Equal(t, "connect", parsed.Lifecycle[1].Kind)
	assert.Equal(t, "OnConnect", parsed.Lifecycle[1].FuncName)

	assert.Equal(t, "disconnect", parsed.Lifecycle[2].Kind)
	assert.Equal(t, "OnDisconnect", parsed.Lifecycle[2].FuncName)
}

func TestParseProcedureDirective(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "procedures.go", `package main

import "context"

//stdb:procedure
func GetPlayer(ctx context.Context, id uint64) *Player {
	return nil
}

type Player struct {
	Id   uint64
	Name string
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Procedures, 1)

	p := parsed.Procedures[0]
	assert.Equal(t, "get_player", p.Name)
	assert.Equal(t, "GetPlayer", p.FuncName)
	require.Len(t, p.Params, 1)
	assert.Equal(t, "id", p.Params[0].Name)
	assert.Equal(t, "uint64", p.Params[0].GoType)
	assert.Equal(t, "*Player", p.ReturnType)
}

func TestParseViewDirective(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "views.go", `package main

import "context"

type ViewContext struct{}
type AnonymousViewContext struct{}

//stdb:view public=true
func GetPublicView(ctx ViewContext, id uint64) []Player {
	return nil
}

//stdb:view
func GetAnonView(ctx AnonymousViewContext, id uint64) *Player {
	return nil
}

type Player struct {
	Id   uint64
	Name string
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Views, 2)

	v0 := parsed.Views[0]
	assert.Equal(t, "get_public_view", v0.Name)
	assert.Equal(t, "GetPublicView", v0.FuncName)
	assert.True(t, v0.IsPublic)
	assert.False(t, v0.IsAnonymous)
	require.Len(t, v0.Params, 1)
	assert.Equal(t, "id", v0.Params[0].Name)
	assert.Equal(t, "uint64", v0.Params[0].GoType)
	assert.Equal(t, "[]Player", v0.ReturnType)

	v1 := parsed.Views[1]
	assert.Equal(t, "get_anon_view", v1.Name)
	assert.True(t, v1.IsAnonymous)
	assert.False(t, v1.IsPublic)
	assert.Equal(t, "*Player", v1.ReturnType)
}

func TestParseEnumDirective(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:enum variants=Red,Green,Blue
type Color uint8

//stdb:enum variants=Admin,User,Guest scope=Auth
type Role uint8
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Enums, 2)

	e0 := parsed.Enums[0]
	assert.Equal(t, "Color", e0.TypeName)
	assert.Equal(t, []string{"Red", "Green", "Blue"}, e0.Variants)
	assert.Nil(t, e0.Scope)

	e1 := parsed.Enums[1]
	assert.Equal(t, "Role", e1.TypeName)
	assert.Equal(t, []string{"Admin", "User", "Guest"}, e1.Variants)
	assert.Equal(t, []string{"Auth"}, e1.Scope)
}

func TestParseSumtypeDirective(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:sumtype
type Shape interface {
	isShape()
}

//stdb:sumtype scope=Game.Entity
type Action interface {
	isAction()
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.SumTypes, 2)

	s0 := parsed.SumTypes[0]
	assert.Equal(t, "Shape", s0.InterfaceName)
	assert.Nil(t, s0.Scope)

	s1 := parsed.SumTypes[1]
	assert.Equal(t, "Action", s1.InterfaceName)
	assert.Equal(t, []string{"Game", "Entity"}, s1.Scope)
}

func TestParseVariantDirective(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:variant of=Shape name=Circle
type CircleVariant struct {
	Radius float64
}

//stdb:variant of=Shape
type Square struct {
	Side float64
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Variants, 2)

	v0 := parsed.Variants[0]
	assert.Equal(t, "Shape", v0.OfInterface)
	assert.Equal(t, "Circle", v0.Name)
	assert.Equal(t, "CircleVariant", v0.StructName)
	require.Len(t, v0.Fields, 1)
	assert.Equal(t, "Radius", v0.Fields[0].GoName)
	assert.Equal(t, "float64", v0.Fields[0].GoType)

	// When name is not specified, defaults to struct name.
	v1 := parsed.Variants[1]
	assert.Equal(t, "Shape", v1.OfInterface)
	assert.Equal(t, "Square", v1.Name)
	assert.Equal(t, "Square", v1.StructName)
}

func TestParseScheduleDirective(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:schedule table=my_jobs function=process_job
type Job struct {
	ScheduledId uint64
	ScheduledAt uint64
	Data        string
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Schedules, 1)

	s := parsed.Schedules[0]
	assert.Equal(t, "my_jobs", s.TableName)
	assert.Equal(t, "process_job", s.FunctionName)
}

func TestParseRLSDirective(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "rls.go", `package main

import "context"

//stdb:rls SELECT * FROM users WHERE owner_id = @caller_identity
func rlsCheck(ctx context.Context) {
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.RLS, 1)
	assert.Equal(t, "SELECT * FROM users WHERE owner_id = @caller_identity", parsed.RLS[0])
}

// ──────────────────────────────────────────────────────────
// Struct field parsing
// ──────────────────────────────────────────────────────────

func TestParseFieldTags_PrimaryKey(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=item access=public
type Item struct {
	Id uint64 `+"`"+`stdb:"primarykey"`+"`"+`
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)
	require.Len(t, parsed.Tables[0].Fields, 1)
	f := parsed.Tables[0].Fields[0]
	assert.True(t, f.PrimaryKey)
	assert.False(t, f.AutoInc)
	assert.False(t, f.Unique)
	assert.False(t, f.IndexBTree)
	assert.False(t, f.IndexDirect)
}

func TestParseFieldTags_AutoInc(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=item access=public
type Item struct {
	Id uint64 `+"`"+`stdb:"autoinc"`+"`"+`
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)
	require.Len(t, parsed.Tables[0].Fields, 1)
	f := parsed.Tables[0].Fields[0]
	assert.True(t, f.AutoInc)
	assert.False(t, f.PrimaryKey)
}

func TestParseFieldTags_Unique(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=item access=public
type Item struct {
	Name string `+"`"+`stdb:"unique"`+"`"+`
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)
	require.Len(t, parsed.Tables[0].Fields, 1)
	f := parsed.Tables[0].Fields[0]
	assert.True(t, f.Unique)
	assert.False(t, f.PrimaryKey)
	assert.False(t, f.AutoInc)
}

func TestParseFieldTags_IndexBTree(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=item access=public
type Item struct {
	Name string `+"`"+`stdb:"index=btree"`+"`"+`
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)
	require.Len(t, parsed.Tables[0].Fields, 1)
	f := parsed.Tables[0].Fields[0]
	assert.True(t, f.IndexBTree)
	assert.False(t, f.IndexDirect)
}

func TestParseFieldTags_IndexDirect(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=item access=public
type Item struct {
	Name string `+"`"+`stdb:"index=direct"`+"`"+`
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)
	require.Len(t, parsed.Tables[0].Fields, 1)
	f := parsed.Tables[0].Fields[0]
	assert.True(t, f.IndexDirect)
	assert.False(t, f.IndexBTree)
}

func TestParseFieldTags_Combined(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=item access=public
type Item struct {
	Id uint64 `+"`"+`stdb:"primarykey,autoinc"`+"`"+`
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)
	require.Len(t, parsed.Tables[0].Fields, 1)
	f := parsed.Tables[0].Fields[0]
	assert.True(t, f.PrimaryKey)
	assert.True(t, f.AutoInc)
}

func TestParseBsatnName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=entity access=public
type Entity struct {
	EntityId    uint64
	PlayerName  string
	HTTPCode    int32
	URLPath     string
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)
	fields := parsed.Tables[0].Fields

	require.Len(t, fields, 4)
	assert.Equal(t, "entity_id", fields[0].BsatnName)
	assert.Equal(t, "player_name", fields[1].BsatnName)
	assert.Equal(t, "http_code", fields[2].BsatnName)
	assert.Equal(t, "url_path", fields[3].BsatnName)
}

func TestParseFieldTypes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

import "github.com/clockworklabs/SpacetimeDB/sdks/go/types"

//stdb:table name=mixed access=public
type Mixed struct {
	Id       uint64
	Identity types.Identity
	Tags     []string
	OptName  *string
	Data     []byte
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)
	fields := parsed.Tables[0].Fields

	require.Len(t, fields, 5)
	assert.Equal(t, "uint64", fields[0].GoType)
	assert.Equal(t, "types.Identity", fields[1].GoType)
	assert.Equal(t, "[]string", fields[2].GoType)
	assert.Equal(t, "*string", fields[3].GoType)
	assert.Equal(t, "[]byte", fields[4].GoType)
}

// ──────────────────────────────────────────────────────────
// Function signature parsing
// ──────────────────────────────────────────────────────────

func TestExtractFuncParams(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "reducers.go", `package main

import "context"

//stdb:reducer
func DoWork(ctx context.Context, name string, count uint32, active bool) {
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Reducers, 1)

	params := parsed.Reducers[0].Params
	require.Len(t, params, 3)
	assert.Equal(t, "name", params[0].Name)
	assert.Equal(t, "string", params[0].GoType)
	assert.Equal(t, "count", params[1].Name)
	assert.Equal(t, "uint32", params[1].GoType)
	assert.Equal(t, "active", params[2].Name)
	assert.Equal(t, "bool", params[2].GoType)
}

func TestExtractReturnType(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "procedures.go", `package main

import "context"

//stdb:procedure
func GetItems(ctx context.Context) []Item {
	return nil
}

type Item struct {
	Id uint64
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Procedures, 1)
	assert.Equal(t, "[]Item", parsed.Procedures[0].ReturnType)
}

func TestFuncReturnsError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "reducers.go", `package main

import "context"

//stdb:reducer
func NoError(ctx context.Context) {
}

//stdb:reducer
func WithError(ctx context.Context) error {
	return nil
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Reducers, 2)

	assert.False(t, parsed.Reducers[0].HasError)
	assert.Equal(t, "NoError", parsed.Reducers[0].FuncName)

	assert.True(t, parsed.Reducers[1].HasError)
	assert.Equal(t, "WithError", parsed.Reducers[1].FuncName)
}

// ──────────────────────────────────────────────────────────
// File filtering
// ──────────────────────────────────────────────────────────

func TestParseSkipsTestFiles(t *testing.T) {
	dir := t.TempDir()
	// Write a valid source file so we get no errors.
	writeFile(t, dir, "types.go", `package main

//stdb:table name=ok access=public
type Ok struct {
	Id uint64
}
`)
	// Write a test file with a table directive — should be ignored.
	writeFile(t, dir, "types_test.go", `package main

//stdb:table name=should_not_appear access=public
type ShouldNotAppear struct {
	Id uint64
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)
	assert.Equal(t, "ok", parsed.Tables[0].Name)
}

func TestParseSkipsGeneratedFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=ok access=public
type Ok struct {
	Id uint64
}
`)
	writeFile(t, dir, "stdb_generated.go", `package main

//stdb:table name=generated access=public
type Generated struct {
	Id uint64
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)
	assert.Equal(t, "ok", parsed.Tables[0].Name)
}

func TestParseSkipsMainGo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=ok access=public
type Ok struct {
	Id uint64
}
`)
	writeFile(t, dir, "main.go", `package main

//stdb:table name=from_main access=public
type FromMain struct {
	Id uint64
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)
	assert.Equal(t, "ok", parsed.Tables[0].Name)
}

func TestParseIncludesNestedPackages(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=ok access=public
type Ok struct {
	Id uint64
}
`)
	// Nested package contributes //stdb: tables (multi-package modules).
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0755))
	writeFile(t, filepath.Join(dir, "subdir"), "nested.go", `package subdir

//stdb:table name=nested access=public
type Nested struct {
	Id uint64
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 2)
	assert.True(t, parsed.MultiPackage)
	names := map[string]bool{}
	for _, tb := range parsed.Tables {
		names[tb.Name] = true
	}
	assert.True(t, names["ok"])
	assert.True(t, names["nested"])
}

func TestParseNestedUnexportedReducerErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=ok access=public
type Ok struct {
	Id uint64
}
`)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "combat"), 0755))
	writeFile(t, filepath.Join(dir, "combat"), "attack.go", `package combat

import "go.digitalxero.dev/spacetimedb-server/reducer"

//stdb:reducer
func meleeAttack(ctx reducer.ReducerContext) error {
	return nil
}
`)
	_, err := parser.ParseDirectory(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be exported")
}

// ──────────────────────────────────────────────────────────
// Edge cases
// ──────────────────────────────────────────────────────────

func TestParseMultipleTableDirectivesOnOneStruct(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=entity access=public
//stdb:table name=logged_out_entity access=public
type Entity struct {
	Id   uint64 `+"`"+`stdb:"primarykey"`+"`"+`
	Name string
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 2)

	assert.Equal(t, "entity", parsed.Tables[0].Name)
	assert.Equal(t, "Entity", parsed.Tables[0].StructName)

	assert.Equal(t, "logged_out_entity", parsed.Tables[1].Name)
	assert.Equal(t, "Entity", parsed.Tables[1].StructName)

	// Both tables share the same fields.
	assert.Equal(t, parsed.Tables[0].Fields, parsed.Tables[1].Fields)
}

func TestParseTypeAlias(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

type Original struct {
	Id uint64
}

type Alias = Original
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	assert.Contains(t, parsed.TypeAliases, "Alias")
	assert.Equal(t, "Original", parsed.TypeAliases["Alias"])
}

func TestParseStructMap(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

type Position struct {
	X float64
	Y float64
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Contains(t, parsed.Structs, "Position")

	s := parsed.Structs["Position"]
	assert.Equal(t, "Position", s.Name)
	require.Len(t, s.Fields, 2)
	assert.Equal(t, "X", s.Fields[0].GoName)
	assert.Equal(t, "float64", s.Fields[0].GoType)
	assert.Equal(t, "Y", s.Fields[1].GoName)
	assert.Equal(t, "float64", s.Fields[1].GoType)
}

func TestParseEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Empty(t, parsed.Tables)
	assert.Empty(t, parsed.Reducers)
	assert.Empty(t, parsed.Lifecycle)
	assert.Empty(t, parsed.Procedures)
	assert.Empty(t, parsed.Views)
	assert.Empty(t, parsed.SumTypes)
	assert.Empty(t, parsed.Enums)
	assert.Empty(t, parsed.Variants)
	assert.Empty(t, parsed.Schedules)
	assert.Empty(t, parsed.RLS)
	assert.NotNil(t, parsed.Structs)
	assert.NotNil(t, parsed.TypeAliases)
	// Empty modules default to package main (WASM entry package).
	assert.Equal(t, "main", parsed.PackageName)
}

func TestParseUnexportedFieldsSkipped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=secret access=public
type Secret struct {
	Id       uint64
	internal string
	hidden   bool
	Public   string
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)

	fields := parsed.Tables[0].Fields
	require.Len(t, fields, 2)
	assert.Equal(t, "Id", fields[0].GoName)
	assert.Equal(t, "Public", fields[1].GoName)
}

// ──────────────────────────────────────────────────────────
// Default access level
// ──────────────────────────────────────────────────────────

func TestParseTableDefaultAccess(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=privtable
type PrivTable struct {
	Id uint64
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)
	assert.Equal(t, "private", parsed.Tables[0].Access)
}

// ──────────────────────────────────────────────────────────
// Package name extraction
// ──────────────────────────────────────────────────────────

func TestParsePackageName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package mymodule

type Foo struct {
	X int
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	assert.Equal(t, "mymodule", parsed.PackageName)
}

// ──────────────────────────────────────────────────────────
// Schedule on function declaration
// ──────────────────────────────────────────────────────────

func TestParseScheduleOnFunction(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "sched.go", `package main

import "context"

//stdb:schedule table=job_table function=run_job
func RunJob(ctx context.Context) {
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Schedules, 1)
	assert.Equal(t, "job_table", parsed.Schedules[0].TableName)
	assert.Equal(t, "run_job", parsed.Schedules[0].FunctionName)
}

// ──────────────────────────────────────────────────────────
// RLS on type declaration
// ──────────────────────────────────────────────────────────

func TestParseRLSOnTypeDecl(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:rls SELECT id FROM things WHERE owner = @caller
type RlsMarker uint8
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.RLS, 1)
	assert.Equal(t, "SELECT id FROM things WHERE owner = @caller", parsed.RLS[0])
}

// ──────────────────────────────────────────────────────────
// Type alias with selector (pkg.Type)
// ──────────────────────────────────────────────────────────

func TestParseTypeAliasSelector(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

import "github.com/clockworklabs/SpacetimeDB/sdks/go/types"

type MyIdentity = types.Identity
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	assert.Contains(t, parsed.TypeAliases, "MyIdentity")
	assert.Equal(t, "types.Identity", parsed.TypeAliases["MyIdentity"])
}

// ──────────────────────────────────────────────────────────
// Multiple files in one directory
// ──────────────────────────────────────────────────────────

func TestParseMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=player access=public
type Player struct {
	Id   uint64 `+"`"+`stdb:"primarykey"`+"`"+`
	Name string
}
`)
	writeFile(t, dir, "reducers.go", `package main

import "context"

//stdb:reducer
func CreatePlayer(ctx context.Context, name string) {
}
`)
	writeFile(t, dir, "lifecycle.go", `package main

import "context"

//stdb:init
func Setup(ctx context.Context) {
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)

	assert.Len(t, parsed.Tables, 1)
	assert.Equal(t, "player", parsed.Tables[0].Name)

	assert.Len(t, parsed.Reducers, 1)
	assert.Equal(t, "create_player", parsed.Reducers[0].Name)

	assert.Len(t, parsed.Lifecycle, 1)
	assert.Equal(t, "init", parsed.Lifecycle[0].Kind)
}

// ──────────────────────────────────────────────────────────
// Procedure with no return type
// ──────────────────────────────────────────────────────────

func TestParseProcedureNoReturnType(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "procedures.go", `package main

import "context"

//stdb:procedure
func DoSomething(ctx context.Context, id uint64) {
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Procedures, 1)
	assert.Equal(t, "", parsed.Procedures[0].ReturnType)
}

// ──────────────────────────────────────────────────────────
// Method with directive is skipped
// ──────────────────────────────────────────────────────────

func TestParseMethodWithDirectiveSkipped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

import "context"

type MyStruct struct{}

//stdb:reducer
func (m *MyStruct) ShouldBeSkipped(ctx context.Context) {
}

//stdb:reducer
func ShouldBeIncluded(ctx context.Context) {
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Reducers, 1)
	assert.Equal(t, "ShouldBeIncluded", parsed.Reducers[0].FuncName)
}
