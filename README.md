# stdb-go

`stdb-go` is a Go code generator for [SpacetimeDB](https://spacetimedb.com) modules. It reads Go source files annotated with `//stdb:` comment directives and generates a `stdb_generated.go` file containing:

- BSATN binary encode/decode functions for all types
- Table accessor types with Insert, Delete, Scan, Count, and index-based query methods
- Reducer, procedure, and view dispatch functions
- Module definition (typespace + schema registration)
- Runtime handler initialization

This replaces reflection-based approaches with compile-time code generation, producing modules that build to WASM with `go build -buildmode=c-shared`.

## Installation

```bash
go install go.digitalxero.dev/stdb-go@latest
```

Or add it as a tool dependency:

```bash
go get go.digitalxero.dev/stdb-go
```

## Quick Start

1. Add a `go:generate` directive to any `.go` file in your SpacetimeDB module package:

```go
//go:generate go run go.digitalxero.dev/stdb-go
```

2. Annotate your types and functions with `//stdb:` directives:

```go
package main

import (
    "github.com/clockworklabs/SpacetimeDB/sdks/go/server/reducer"
    "github.com/clockworklabs/SpacetimeDB/sdks/go/types"
)

//stdb:table name=player access=public
type Player struct {
    Id       uint64         `stdb:"primarykey,autoinc"`
    Name     string         `stdb:"unique"`
    Owner    types.Identity
    Score    *uint64
}

//stdb:init
func Init(ctx reducer.ReducerContext) {
}

//stdb:reducer
func CreatePlayer(ctx reducer.ReducerContext, name string) {
    PlayerTable.Insert(Player{Name: name, Owner: ctx.Sender})
}

//stdb:reducer
func UpdateScore(ctx reducer.ReducerContext, playerId uint64, delta int64) {
    player, found, _ := PlayerTable.FindById(playerId)
    if !found { return }
    newScore := *player.Score + uint64(delta)
    player.Score = &newScore
    PlayerTable.UpdateById(player)
}
```

3. Run the generator:

```bash
go generate ./...
```

This produces `stdb_generated.go` with all the BSATN codecs, table accessors (e.g., `PlayerTable`), reducer dispatch, and module definition.

4. Build the WASM module:

```bash
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o module.wasm .
```

## CLI Usage

```
stdb-go [flags]
stdb-go [command]

Available Commands:
  build       Build a SpacetimeDB WASM module
  init        Initialize a new SpacetimeDB project
  publish     Build and publish a SpacetimeDB WASM module
  upgrade     Upgrade stdb-go to a newer version
  version     Display version information

Flags:
      --dir string      directory containing Go source files to process (default ".")
      --output string   output file name (default "stdb_generated.go")
```

### Init

Scaffold a new SpacetimeDB project:

```bash
# Create a server module
stdb-go init myproject

# Create a client project
stdb-go init myproject --type client

# Create a full-stack project (server + client)
stdb-go init myproject --type fullstack --module github.com/me/myproject

# Create in a specific directory
stdb-go init myproject --dir /path/to/project
```

Project types:
- **server**: SpacetimeDB WASM module with example table, reducer, and init hook
- **client**: Go client that connects to a SpacetimeDB instance
- **fullstack**: Both server and client with an orchestrating Taskfile

### Upgrade

`stdb-go` can self-update from GitLab releases:

```bash
# Upgrade to the latest version
stdb-go upgrade

# List available versions
stdb-go upgrade --list

# Upgrade to a specific version
stdb-go upgrade --version v0.2.0
```

Set `GITLAB_TOKEN` or `GL_TOKEN` for authenticated access to private releases.

## Directives Reference

All directives are Go comments starting with `//stdb:` placed immediately above the target declaration.

### Tables

```go
//stdb:table name=<table_name> access=<public|private> [event=true] [index=<name>:<col0>,<col1>,...]
type MyStruct struct { ... }
```

| Parameter | Required | Default   | Description |
|-----------|----------|-----------|-------------|
| `name`    | yes      | -         | SpacetimeDB table name (snake_case) |
| `access`  | no       | `private` | Table visibility: `public` or `private` |
| `event`   | no       | `false`   | Mark as event table (append-only, no indexes) |
| `index`   | no       | -         | Multi-column BTree index: `name:col0,col1,...` (0-based column indices) |

A single struct can back multiple tables by stacking directives:

```go
//stdb:table name=active_entity access=public
//stdb:table name=inactive_entity access=private
type Entity struct {
    Id   uint64 `stdb:"primarykey"`
    Name string
}
```

This generates `ActiveEntityTable` and `InactiveEntityTable` accessor variables.

### Struct Field Tags

Fields use the `stdb` struct tag to declare constraints and indexes:

```go
type Example struct {
    Id     uint64 `stdb:"primarykey,autoinc"`
    Email  string `stdb:"unique"`
    Name   string `stdb:"index=btree"`
    Status uint8  `stdb:"index=direct"`
}
```

| Tag            | Description |
|----------------|-------------|
| `primarykey`   | Primary key column (implies unique + btree index) |
| `autoinc`      | Auto-incrementing sequence |
| `unique`       | Unique constraint (implies btree index) |
| `index=btree`  | BTree index (for range/equality queries) |
| `index=direct` | Direct hash index (for equality-only queries) |

Tags can be combined: `stdb:"primarykey,autoinc"`.

### Reducers

```go
//stdb:reducer [name=<custom_name>]
func MyReducer(ctx reducer.ReducerContext, param1 string, param2 uint64) [error] {
}
```

- The first parameter must be `reducer.ReducerContext` (skipped in BSATN args).
- If `name` is omitted, the function name is converted to `snake_case` (e.g., `MyReducer` becomes `my_reducer`).
- May optionally return `error`.

### Lifecycle Hooks

```go
//stdb:init
func Init(ctx reducer.ReducerContext) {}

//stdb:connect
func OnConnect(ctx reducer.ReducerContext) {}

//stdb:disconnect
func OnDisconnect(ctx reducer.ReducerContext) {}
```

These map to SpacetimeDB lifecycle events: `__init__`, `__identity_connected__`, `__identity_disconnected__`.

### Procedures

```go
//stdb:procedure [name=<custom_name>]
func GetData(ctx reducer.ProcedureContext, key string) *MyStruct {
    return nil
}
```

- First parameter must be `reducer.ProcedureContext`.
- Return type can be any supported type, a pointer (optional result), a slice (multiple results), or omitted (void).

### Views

```go
// Authenticated view (requires sender identity)
//stdb:view [name=<custom_name>] [public=true]
func GetPlayers(ctx reducer.ViewContext) []Player {
    return nil
}

// Anonymous view (no authentication required)
//stdb:view [name=<custom_name>]
func GetPublicData(ctx reducer.AnonymousViewContext) *Player {
    return nil
}
```

- Authenticated views use `reducer.ViewContext` as the first parameter.
- Anonymous views use `reducer.AnonymousViewContext`.
- Return types: `*T` (optional single row), `[]T` (multiple rows), or `T` (single row).
- `public=true` marks the view as publicly accessible.

### Enums

```go
//stdb:enum variants=<Variant1>,<Variant2>,... [scope=<Namespace>]
type Color uint8

const (
    ColorRed   Color = 0
    ColorGreen Color = 1
    ColorBlue  Color = 2
)
```

Simple enums are encoded as a `uint8` tag. The `scope` parameter places the type under a namespace in the module definition.

### Sum Types

Sum types use Go interfaces with variant structs:

```go
//stdb:sumtype [scope=<Namespace>]
type Shape interface{ isShape() }

//stdb:variant of=Shape name=Circle
type ShapeCircle struct {
    Radius float64
}

//stdb:variant of=Shape name=Rectangle
type ShapeRectangle struct {
    Width  float64
    Height float64
}

//stdb:variant of=Shape name=Point
type ShapePoint struct{}  // unit variant
```

| Directive | Parameters | Description |
|-----------|------------|-------------|
| `//stdb:sumtype` | `scope=` (optional) | Declares the interface as a sum type |
| `//stdb:variant` | `of=` (required), `name=` (optional) | Associates a struct as a variant of the sum type |

If `name` is omitted on `//stdb:variant`, the struct name is used as the variant name.

### Schedules

Link a table to a reducer for scheduled execution:

```go
//stdb:table name=scheduled_task access=private
//stdb:schedule table=scheduled_task function=run_task
type ScheduledTask struct {
    Id          uint64           `stdb:"primarykey,autoinc"`
    ScheduledAt types.ScheduleAt
}

//stdb:reducer name=run_task
func RunTask(ctx reducer.ReducerContext) {}
```

The schedule table must have a `types.ScheduleAt` field.

### Row-Level Security

```go
//stdb:rls SELECT * FROM my_table WHERE owner = @sender
func RlsFilter() {}
```

The SQL expression after `rls` is passed through as-is to the module definition.

### Type Aliases

Type aliases are resolved transparently:

```go
type Score struct {
    Value uint64
    Label string
}
type ScoreAlias = Score

//stdb:reducer
func AddScore(ctx reducer.ReducerContext, score ScoreAlias) {}
```

## Supported Types

### Primitives

| Go Type   | BSATN Type | Size |
|-----------|------------|------|
| `bool`    | Bool       | 1 byte |
| `uint8`   | U8         | 1 byte |
| `uint16`  | U16        | 2 bytes LE |
| `uint32`  | U32        | 4 bytes LE |
| `uint64`  | U64        | 8 bytes LE |
| `int8`    | I8         | 1 byte |
| `int16`   | I16        | 2 bytes LE |
| `int32`   | I32        | 4 bytes LE |
| `int64`   | I64        | 8 bytes LE |
| `float32` | F32        | 4 bytes LE |
| `float64` | F64        | 8 bytes LE |
| `string`  | String     | u32 len + UTF-8 |

### Special Types

Import from `github.com/clockworklabs/SpacetimeDB/sdks/go/types`:

| Go Type              | BSATN Representation | Description |
|----------------------|---------------------|-------------|
| `types.Identity`     | Product{U256}       | 32-byte user identity |
| `types.ConnectionId` | Product{U128}       | 16-byte connection identifier |
| `types.Timestamp`    | Product{I64}        | Microseconds since Unix epoch |
| `types.TimeDuration` | Product{I64}        | Duration in microseconds |
| `types.ScheduleAt`   | Sum{Interval, Time} | Schedule timing (interval or absolute) |
| `types.Uuid`         | Product{U128}       | 16-byte UUID |
| `types.Uint128`      | U128                | 16-byte unsigned integer |
| `types.Uint256`      | U256                | 32-byte unsigned integer |
| `types.Int128`       | I128                | 16-byte signed integer |
| `types.Int256`       | I256                | 32-byte signed integer |

### Composite Types

| Go Type  | BSATN Type | Description |
|----------|------------|-------------|
| `*T`     | Option(T)  | Optional value (nil = None, non-nil = Some) |
| `[]T`    | Array(T)   | Variable-length array |
| `[]byte` | Array(U8)  | Byte array (optimized path) |
| Struct   | Product    | Sequential fields |
| Enum     | Sum        | Tagged union (uint8 tag) |
| Sum type | Sum        | Tagged union with payloads |

Nested composites are fully supported: `*[]*int32` encodes as `Option(Array(Option(I32)))`.

## Generated Code

For each annotated module, `stdb-go` produces a single `stdb_generated.go` file containing:

### BSATN Codecs

For each struct, enum, and sum type:
- `stdbWrite<TypeName>(w bsatn.Writer, v *TypeName)` — encode to BSATN
- `stdbRead<TypeName>(r bsatn.Reader, v *TypeName) error` — decode from BSATN

### Table Accessors

For each `//stdb:table` directive, a global variable `<PascalCaseName>Table` is generated:

| Method | Signature | Description |
|--------|-----------|-------------|
| `Insert` | `(row T) T` | Insert a row, returns row with auto-inc fields populated |
| `Delete` | `(row T)` | Delete a row by exact match |
| `Scan` | `() (TableIterator[T], error)` | Iterate all rows |
| `Count` | `() (uint64, error)` | Count total rows |

For fields with `primarykey` or `unique`:

| Method | Signature | Description |
|--------|-----------|-------------|
| `FindBy<Field>` | `(key K) (T, bool, error)` | Find one row by key |
| `UpdateBy<Field>` | `(row T) T` | Update a row by key |
| `DeleteBy<Field>` | `(key K) uint32` | Delete rows by key, returns count |

For fields with `index=btree`:

| Method | Signature | Description |
|--------|-----------|-------------|
| `FilterBy<Field>` | `(key K) (TableIterator[T], error)` | Filter rows by key |
| `DeleteBy<Field>` | `(key K) uint32` | Delete rows by key, returns count |

For multi-column indexes, methods like `FilterBy<Col1>And<Col2>` are generated for the full column set and all prefix subsets.

### Dispatch Functions

- `stdbCallReducer(id, ctx, args)` — dispatches reducer calls by ID
- `stdbCallProcedure(id, ctx, args)` — dispatches procedure calls (if any)
- `stdbCallView(id, sender, args)` — dispatches authenticated view calls (if any)
- `stdbCallViewAnon(id, args)` — dispatches anonymous view calls (if any)

### Module Definition

`stdbDescribeModule()` builds the complete module schema:
- Typespace with all referenced types
- Table definitions with constraints, indexes, and sequences
- Reducer definitions with parameter types
- Procedure and view definitions
- Schedule and row-level security rules

### Initialization

An `init()` function registers all handlers with the SpacetimeDB runtime.

## Architecture

The generator has a 3-stage pipeline:

```
Source files → parseDirectory() → ParsedModule
                                      ↓
                                  analyze() → AnalyzedModule
                                                   ↓
                                              generate() → stdb_generated.go
```

1. **Parser** (`parser.go`): Uses `go/parser` to walk AST, extracts `//stdb:` directives, struct fields, function signatures, and type aliases. Skips `main.go`, `*_test.go`, `*_generated.go`, and directories.

2. **Analyzer** (`analyzer.go`): Resolves all Go types to algebraic types (BSATN type system). Builds a typespace with sequential indices. Registers enums and sum types first, then resolves struct fields recursively. Assigns reducer/procedure/view IDs.

3. **Generator** (`generator.go`, `gen_*.go`): Emits Go source code from the analyzed module. Runs `gofmt` on the output for consistent formatting.

## File Filtering

The parser processes `.go` files in the target directory with these exclusions:

| Pattern | Reason |
|---------|--------|
| `main.go` | Contains the entry point, not module declarations |
| `*_test.go` | Test files |
| `*_generated.go` | Previously generated files (avoids circular processing) |
| Directories | Not recursed into |

## Development

### Prerequisites

- Go 1.25+
- `gofmt` (included with Go)

### Running Tests

```bash
# Run all tests
go test -v ./...

# Run specific test categories
go test -v -run TestParse     # parser tests
go test -v -run TestAnalyze   # analyzer tests
go test -v -run TestGolden    # golden file integration tests

# Update golden files after intentional changes
go test -run TestGolden -update

# Check coverage
go test -cover ./...
```

### Test Structure

| File | Tests | Description |
|------|-------|-------------|
| `gen_common_test.go` | ~60 | Utility functions (toSnakeCase, toPascalCase, importBlock, etc.) |
| `parser_test.go` | ~40 | Parser: directives, field tags, signatures, file filtering |
| `analyzer_test.go` | ~25 | Analyzer: type resolution, tables, reducers, typespace |
| `error_test.go` | ~18 | Error cases: invalid source, unknown types, bad params |
| `generator_test.go` | 19 | Golden file integration tests (full pipeline on fixture directories) |

Golden file tests run the complete pipeline on `testdata/<name>/input/` directories and compare against committed `testdata/<name>/golden.go` files.

### Build

```bash
# Development build
go build -o stdb-go .

# Release build with version info
go build -ldflags "-X main.version=v0.1.0 -X main.commit=$(git rev-parse --short HEAD)" -o stdb-go .
```

## License

See [LICENSE](LICENSE) for details.
