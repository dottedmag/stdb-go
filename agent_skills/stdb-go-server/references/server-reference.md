# SpacetimeDB Go Server Module Reference

All import paths use `go.digitalxero.dev/spacetimedb-server` (runtime library) and `go.digitalxero.dev/spacetimedb-client` (shared types/BSATN). Never use `github.com/clockworklabs/...` paths.

---

## Table of Contents

1. [Project Layout and Prerequisites](#project-layout-and-prerequisites)
2. [Directive Grammar and File Rules](#directive-grammar-and-file-rules)
3. [Directive Reference](#directive-reference)
4. [Struct Tag Reference](#struct-tag-reference)
5. [Column Default Values](#column-default-values)
6. [Go Type Mappings](#go-type-mappings)
7. [Special Types](#special-types)
8. [Contexts: Reducer vs Procedure vs View](#contexts-reducer-vs-procedure-vs-view)
9. [Generated Code Surface](#generated-code-surface)
10. [Table Handle API](#table-handle-api)
11. [The table Package Interfaces](#the-table-package-interfaces)
12. [Enums and Sum Types](#enums-and-sum-types)
13. [Scheduled Reducers](#scheduled-reducers)
14. [Row-Level Security](#row-level-security)
15. [Multi-Package Modules](#multi-package-modules)
16. [Authentication (auth package)](#authentication-auth-package)
17. [HTTP from Procedures (http package)](#http-from-procedures-http-package)
18. [Logging (log package)](#logging-log-package)
19. [Testing with mockhost](#testing-with-mockhost)
20. [Complete Worked Example](#complete-worked-example)
21. [Build and Publish](#build-and-publish)
22. [Gotchas and Constraints](#gotchas-and-constraints)

---

## Project Layout and Prerequisites

- **Go 1.25+** — needed for `wasip1` WASM support and `go:wasmexport`. Use the standard Go toolchain; do NOT use TinyGo (stdb-go invokes plain `go build` itself).
- **stdb-go CLI** (`go.digitalxero.dev/stdb-go`) — generator, builder, publisher. See the `stdb-go-cli` skill for command/flag details.

```
mymodule/
  spacetime.json           # {"name": "mymodule", "edition": "1.0"}
  go.mod                   # module mymodule; go 1.25.0; requires spacetimedb-server + spacetimedb-client
  main.go                  # //go:generate + empty func main() {} — no //stdb: directives here
  types.go                 # //stdb:table structs, enums, sum types
  reducers.go              # //stdb:reducer, lifecycle hooks, procedures, views
  stdb_generated.go        # generated (single-package) — DO NOT EDIT
  Taskfile.yml             # optional task runner
  module.wasm              # build artifact (gitignore)
```

### main.go (exactly this shape)

```go
//go:generate go run go.digitalxero.dev/stdb-go
package main

func main() {}
```

The empty `func main() {}` is required for the `-buildmode=c-shared` WASM build. Do not put `//stdb:` directives or lifecycle hooks in main.go — the parser skips that file entirely, so they would be silently ignored. Put lifecycle hooks in `reducers.go` (or any non-main.go file).

### spacetime.json

```json
{
  "name": "mymodule",
  "edition": "1.0"
}
```

`name` is the default database name used by `stdb-go publish` / `stdb-go dev`.

---

## Directive Grammar and File Rules

A directive is a `//` comment placed on the line(s) immediately above the declaration it annotates:

```
//stdb:<kind> key1=value1 key2=value2
```

- Parameters are **space-separated `key=value` pairs** — no parentheses, no commas between params. `//stdb:table(name=x,public)` is NOT valid syntax; the parser drops it silently. There is no `//stdb:column` directive — column options are struct tags.
- Exception: `//stdb:rls` treats everything after `rls` as raw SQL.
- Multiple directives can be stacked on one declaration (multi-table structs).
- Directives attach to type declarations (doc comment of the `type` block or the individual TypeSpec) and to top-level functions (doc comment). Methods (functions with receivers) are ignored.

Files/dirs the parser skips when scanning for directives: `main.go`, `*_test.go`, `*_generated.go` (including `stdb_generated.go`, `stdb_module_generated.go`, `stdb_tables_generated.go`), and `vendor/`, `testdata/`, `node_modules/`, VCS and dot-directories. The parser walks nested directories recursively — see [Multi-Package Modules](#multi-package-modules).

---

## Directive Reference

### `//stdb:table`

```
//stdb:table name=<table_name> [access=<public|private>] [event=true] [index=<name>:<col0>,<col1>,...]
```

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `name` | Yes | — | Table name in the database (snake_case). Also determines the handle var: `name=player` → `PlayerTable` |
| `access` | No | `private` | `public` rows are readable by clients; `private` only by the module owner |
| `event` | No | `false` | Event table (append-only semantics, `WithIsEvent(true)` in the schema) |
| `index` | No | — | Multi-column BTree index: `index=idx_name:0,1` using 0-based **column indices** |

```go
//stdb:table name=player access=public
type Player struct {
    Id    uint64         `stdb:"primarykey,autoinc"`
    Name  string         `stdb:"unique"`
    Owner types.Identity
}
```

Event table (no indexes needed):

```go
//stdb:table name=game_log access=public event=true
type GameLog struct {
    Message string
    Level   uint8
}
```

Multi-table — one struct backing several tables (stack directives):

```go
//stdb:table name=active_entity access=public
//stdb:table name=inactive_entity access=private
type Entity struct {
    Id   uint64 `stdb:"primarykey"`
    Name string
}
```

Generates separate handles (`ActiveEntityTable`, `InactiveEntityTable`) sharing one codec.

Notes:
- Only **exported** struct fields become columns; unexported and embedded fields are skipped.
- Column names are the snake_case of the Go field name (`EntityId` → `entity_id`).
- A primary key is not enforced by the generator, but without `primarykey`/`unique` you get no `FindBy`/`UpdateBy`/`DeleteBy` accessors.

### `//stdb:reducer`

```
//stdb:reducer [name=<custom_name>]
```

- First parameter must be `reducer.ReducerContext`; remaining parameters are the client-visible args (names come from the Go signature; `_` becomes `arg_N`).
- May optionally return `error` (and nothing else). No data returns.
- `name` defaults to snake_case of the function name (`CreatePlayer` → `create_player`).

```go
//stdb:reducer
func CreatePlayer(ctx reducer.ReducerContext, name string) {
    PlayerTable.Insert(Player{Name: name, Owner: ctx.Sender()})
}

//stdb:reducer name=custom_remove
func RemovePlayer(ctx reducer.ReducerContext, id uint64) error {
    if n := PlayerTable.DeleteById(id); n == 0 {
        return fmt.Errorf("player %d not found", id)
    }
    return nil
}
```

### `//stdb:init`, `//stdb:connect`, `//stdb:disconnect`

Lifecycle hooks. Signature is exactly `func(ctx reducer.ReducerContext)` — no extra params, no return value.

```go
//stdb:init
func Init(ctx reducer.ReducerContext) {}        // module published/updated

//stdb:connect
func OnConnect(ctx reducer.ReducerContext) {}   // client connected

//stdb:disconnect
func OnDisconnect(ctx reducer.ReducerContext) {} // client disconnected
```

These map to the reducer names `__init__`, `__identity_connected__`, `__identity_disconnected__` (registered with private visibility plus a lifecycle def). Remember: not in main.go.

### `//stdb:procedure`

```
//stdb:procedure [name=<custom_name>]
```

- First parameter must be `reducer.ProcedureContext`.
- Return **exactly one value** (any supported type, `*T` for optional, `[]T` for lists) or nothing (void). Do NOT return `error` — the generated dispatch assigns a single result.

```go
//stdb:procedure
func GetPlayerCount(ctx reducer.ProcedureContext) uint64 {
    var count uint64
    ctx.WithTx(func() {
        count, _ = PlayerTable.Count()
    })
    return count
}
```

### `//stdb:view`

```
//stdb:view [name=<custom_name>] [public=true]
```

Two flavors, chosen by the first parameter type:

| First param | Kind | Caller identity |
|-------------|------|-----------------|
| `reducer.ViewContext` | Authenticated | `ctx.Sender()` available |
| `reducer.AnonymousViewContext` | Anonymous | none |

- `public=true` marks the view publicly accessible in the schema (defaults to false).
- Views must return exactly one value: `T` (single row), `*T` (optional), or `[]T` (list). The result is delivered to clients as a row array (single value → array of 1, `*T` → 0 or 1, `[]T` → n). No `error` return.
- Views are read-only; they have no connection id, so `SenderAuth` is not available.

```go
//stdb:view public=true
func GetPlayers(ctx reducer.ViewContext) []Player {
    iter, err := PlayerTable.Scan()
    if err != nil { return nil }
    defer iter.Close()
    var out []Player
    for p, ok := iter.Next(); ok; p, ok = iter.Next() {
        out = append(out, p)
    }
    return out
}

//stdb:view
func GetPlayerByName(ctx reducer.AnonymousViewContext, name string) *Player {
    p, found, err := PlayerTable.FindByName(name)
    if err != nil || !found { return nil }
    return &p
}
```

### `//stdb:enum`

```
//stdb:enum variants=<V1>,<V2>,<V3> [scope=<Ns.SubNs>]
```

Declares a `uint8`-backed Go type as a unit-variant sum type. Define constants matching the variant order (tags 0..n-1). See [Enums and Sum Types](#enums-and-sum-types).

### `//stdb:sumtype` and `//stdb:variant`

```
//stdb:sumtype [scope=<Ns>]           // on an interface with an unexported marker method
//stdb:variant of=<Iface> [name=<V>]  // on each variant struct (name defaults to struct name)
```

See [Enums and Sum Types](#enums-and-sum-types).

### `//stdb:schedule`

```
//stdb:schedule table=<table_name> function=<reducer_name>
```

Stack it with the `//stdb:table` directive on the schedule table struct (it may also annotate a function). See [Scheduled Reducers](#scheduled-reducers).

### `//stdb:rls`

```
//stdb:rls <SQL filter expression>
```

Everything after `rls` is passed through verbatim. Attach it to any func (the body is ignored). See [Row-Level Security](#row-level-security).

---

## Struct Tag Reference

Struct tags use the `stdb` key; parts are comma-separated (commas inside single quotes are literal, for `default=` values).

| Tag | Description | Generated behavior |
|-----|-------------|--------------------|
| `primarykey` | Primary key column | Unique constraint + btree index; `FindBy`/`UpdateBy`/`DeleteBy<Field>` methods |
| `autoinc` | Auto-increment sequence | `Insert`/`UpdateBy` return the row with the sequence value filled (integer columns only) |
| `unique` | Unique constraint | Same accessors as primarykey |
| `index=btree` | BTree index (range/equality) | `FilterBy<Field>` + `DeleteBy<Field>` methods |
| `index=direct` | Direct index (equality-only) | Index registered in the schema; **no accessor methods generated** |
| `default=<value>` | Column default | Enables additive in-place migrations (backfills existing rows) |

```go
//stdb:table name=indexed access=public index=multi_idx:0,2
type Indexed struct {
    Id     uint64 `stdb:"primarykey,autoinc"`
    Name   string `stdb:"unique"`
    Age    uint32 `stdb:"index=btree"`
    Status uint8  `stdb:"index=direct"`
}
```

Single-column btree indexes are named `<table>_<column>_idx_btree`, direct indexes `<table>_<column>_idx_direct` in the schema.

---

## Column Default Values

`default=` attaches a default to a column so SpacetimeDB can migrate an already-published table **in place** when you add the column (instead of requiring `--delete-data`). Existing rows are backfilled with the default.

```go
//stdb:table name=player access=public
type Player struct {
    Id     uint64  `stdb:"primarykey,autoinc"`
    Score  uint32  `stdb:"default=0"`
    Name   string  `stdb:"index=btree,default='Unknown'"`
    Note   string  `stdb:"default=''"`             // empty string must be quoted
    Tags   string  `stdb:"default='a, b, c'"`      // commas survive inside quotes
    Status Status  `stdb:"default=Offline"`        // enum: variant name or numeric tag
    Bonus  *uint64 `stdb:"default=null"`           // Option -> None
    Buff   *uint32 `stdb:"default=7"`              // Option -> Some(7)
    Blob   []byte  `stdb:"default=0x01020304"`     // []byte from hex
    Origin Point   `stdb:"default=raw:0x0000...."` // escape hatch: verbatim BSATN
}
```

Value syntax (validated at generate time):

- **Bare value** — read to the next top-level comma: `default=5`, `default=true`.
- **Single-quoted** — preserves commas/spaces, allows `''`; escape a quote with `\'`. Double quotes can't be used (they close the Go tag).
- **bool / ints / floats / string** — parsed and range-checked; 128/256-bit integers accept decimal or `0x`/`0o`/`0b` literals.
- **Enums** — variant name (`default=Offline`) or numeric tag index.
- **Option (`*T`)** — `null` or `none` → None; anything else → `Some(value)`.
- **`[]byte`** — hex (`0x...`) or empty. **Arrays (`[]T`)** — only empty (`` or `[]`) as a literal.
- **Timestamp/TimeDuration** — an int64 microseconds literal.
- **`raw:<hex>`** — universal escape hatch: the bytes are written verbatim as the column's BSATN value, so any type (nested structs, sum types, exotic values) can carry a default. You must supply correctly-encoded BSATN.
- `default` **cannot be combined with `autoinc`** — generate-time error (the sequence supplies values).

Generated as `WithDefaultValue(moduledef.NewColumnDefaultValue(colIdx, bytes))` in the table def. Publishing a module that adds a defaulted column may require `stdb-go publish --break-clients` (adding a column breaks existing client codegen).

---

## Go Type Mappings

| Go type | SATS / BSATN | Notes |
|---------|--------------|-------|
| `bool` | Bool | 1 byte |
| `uint8`/`byte`, `uint16`, `uint32`, `uint64` | U8..U64 | little-endian |
| `int8`, `int16`, `int32`, `int64` | I8..I64 | little-endian |
| `float32`, `float64` | F32, F64 | |
| `string` | String | u32 length + UTF-8 |
| `[]byte` | Array(U8) | optimized path |
| `[]T` | Array(T) | u32 length prefix + elements |
| `*T` | Option(T) | Sum: tag 0 = some(value), tag 1 = none. `nil` ⇔ None |
| struct | Product | fields in declaration order (via typespace ref) |
| `//stdb:enum` type | Sum of unit variants | single tag byte |
| `//stdb:sumtype` interface | Sum | tag byte + variant payload |
| type alias (`type A = B`) | resolved to underlying | transparent |
| `int`, `uint`, `map[K]V`, channels, funcs | **unsupported** | generate-time error: use sized ints; model maps as row tables |

Nested composites compose freely: `*[]*int32` ⇔ `Option(Array(Option(I32)))`.

---

## Special Types

Import from `go.digitalxero.dev/spacetimedb-client/types`:

| Go type | SATS representation | Description |
|---------|--------------------|-------------|
| `types.Identity` | Product{`__identity__`: U256} | 32-byte caller/module identity |
| `types.ConnectionId` | Product{`__connection_id__`: U128} | 16-byte connection id |
| `types.Timestamp` | Product{`__timestamp_micros_since_unix_epoch__`: I64} | `types.NewTimestamp(micros)`; `.Microseconds()`, `.Time()` |
| `types.TimeDuration` | Product{`__time_duration_micros__`: I64} | `types.NewTimeDuration(micros)`; `.Duration()` |
| `types.ScheduleAt` | Sum{Interval: TimeDuration, Time: Timestamp} | `types.ScheduleAtInterval{Value: d}` / `types.ScheduleAtTime{Value: t}` |
| `types.Uuid` | Product{`__uuid__`: U128} | UUID; `ProcedureContext.NewUuidV7()` mints them |
| `types.Uint128` / `types.Int128` | U128 / I128 | 16-byte integers |
| `types.Uint256` / `types.Int256` | U256 / I256 | 32-byte integers |

The analyzer accepts these with or without the `types.` qualifier.

---

## Contexts: Reducer vs Procedure vs View

The root package re-exports these as `server.ReducerContext` etc. (`import server "go.digitalxero.dev/spacetimedb-server"`), but generated code and most modules import `go.digitalxero.dev/spacetimedb-server/reducer` directly.

### `reducer.ReducerContext`

Reducers run inside an **automatic transaction** — every table op in one call is atomic; returning an error aborts it. No HTTP, no sleeping, no UUID minting.

```go
type ReducerContext interface {
    Sender() types.Identity          // caller identity
    ConnectionId() types.ConnectionId
    Timestamp() types.Timestamp      // host-assigned call time
    Identity() types.Identity        // module owner identity
    Db() any                         // reserved; currently nil
    SenderAuth() auth.SenderAuth     // validated JWT claims, if any
}
```

### `reducer.ProcedureContext`

Procedures do **not** run in a transaction. Wrap table access in `WithTx`/`TryWithTx`. They may perform external I/O between transactions.

```go
type ProcedureContext interface {
    Sender() types.Identity
    ConnectionId() types.ConnectionId
    Timestamp() types.Timestamp
    Identity() types.Identity
    SenderAuth() auth.SenderAuth
    WithTx(fn func())                      // run fn in a transaction
    TryWithTx(fn func() error) error       // transaction with error propagation
    SleepUntil(target types.Timestamp)     // suspend execution
    HttpGet(uri string) (statusCode uint16, body []byte, err error)
    NewUuidV7() (types.Uuid, error)
}
```

Table operations **outside** `WithTx` fail with `sys.ErrNotInTransaction`.

### `reducer.ViewContext` / `reducer.AnonymousViewContext`

```go
type ViewContext interface { Sender() types.Identity }
type AnonymousViewContext interface { /* marker only — no data */ }
```

Views are read-only and carry no connection id, so no `SenderAuth`.

For tests, construct contexts with `reducer.NewReducerContext(sender, connId, ts, moduleIdentity)`, `reducer.NewViewContext(sender)`, `reducer.NewAnonymousViewContext()`.

---

## Generated Code Surface

Single-package modules get one `stdb_generated.go`; multi-package modules get per-package `stdb_tables_generated.go` files plus a root `stdb_module_generated.go`. Contents:

### BSATN codecs (exported)

```go
func StdbWritePlayer(w bsatn.Writer, v *Player)
func StdbReadPlayer(r bsatn.Reader, v *Player) error
```

One pair per struct/enum/sumtype (plus `StdbWriteScheduleAt` helpers when needed). They are **exported** (`Stdb...`, capital S) so other packages in a multi-package module can call them.

### Dispatch functions

- `stdbCallReducer(id uint32, ctx reducer.ReducerContext, args []byte) error`
- `stdbCallProcedure(id, ctx, args) ([]byte, error)` — only if procedures exist
- `stdbCallView(id uint32, sender types.Identity, args []byte) ([]byte, error)` — authed views
- `stdbCallViewAnon(id uint32, args []byte) ([]byte, error)` — anonymous views

### Module definition + registration

`stdbDescribeModule() []byte` builds the full schema (typespace, tables with constraints/indexes/sequences/defaults, reducers, lifecycle defs, procedures, views, schedules, RLS) with the `moduledef` builders and BSATN-encodes it. A Go `init()` registers everything:

```go
func init() {
    runtime.SetDescribeModuleHandler(stdbDescribeModule)
    runtime.SetCallReducerHandler(stdbCallReducer)
    // + SetCallProcedureHandler / SetCallViewHandler / SetCallViewAnonHandler as needed
}
```

---

## Table Handle API

For `//stdb:table name=player ...` the generator emits `var PlayerTable = &stdbPlayerTableHandle{}` (PascalCase of the **table name**, not the struct name). The table id is lazily resolved from the host on first use.

Always generated:

| Method | Signature | Notes |
|--------|-----------|-------|
| `Insert` | `(row T) T` | Returns row with autoinc fields populated. **Panics** on error (e.g. unique violation) |
| `Delete` | `(row T)` | Delete by full row equality. Panics on error |
| `Scan` | `() (runtime.TableIterator[T], error)` | All rows. `defer iter.Close()` |
| `Count` | `() (uint64, error)` | Row count |
| `Clear` | `() (uint64, error)` | Truncate; returns rows removed |

Per `primarykey` / `unique` field:

| Method | Signature | Notes |
|--------|-----------|-------|
| `FindBy<Field>` | `(key K) (T, bool, error)` | Point lookup |
| `UpdateBy<Field>` | `(row T) T` | Update the row matched by that field. Panics on error |
| `DeleteBy<Field>` | `(key K) uint32` | Returns delete count. Panics on error |

Per `index=btree` field (that is not pk/unique):

| Method | Signature |
|--------|-----------|
| `FilterBy<Field>` | `(key K) (runtime.TableIterator[T], error)` |
| `DeleteBy<Field>` | `(key K) uint32` |

Per multi-column `index=<name>:<c0>,<c1>,...`: `FilterBy<Col0>And<Col1>...` for the full column set **and** every prefix (`FilterBy<Col0>`, `FilterBy<Col0>And<Col1>`, ...), skipping names already taken by single-column methods.

`index=direct` fields get **no accessor methods** — the direct index exists only in the schema (host-side lookups).

Iterator usage:

```go
iter, err := PlayerTable.FilterByLevel(3)
if err != nil { return }
defer iter.Close()
for row, ok := iter.Next(); ok; row, ok = iter.Next() {
    // use row
}
```

`runtime.TableIterator[T]` has `Next() (T, bool)` and `Close()`.

---

## The table Package Interfaces

`go.digitalxero.dev/spacetimedb-server/table` provides generic, error-returning abstractions used by hand-rolled (non-generated) table code:

```go
table.NewTable[R](name string, encode EncodeFn[R], decode DecodeFn[R]) Table[R]

type Table[R any] interface {
    TableId() TableId
    Insert(row R) (R, error)
    Delete(row R) error
    Scan() (Iterator[R], error)
    Count() (uint64, error)
    Clear() (uint64, error)
}
type UniqueIndex[R, K any] interface {
    FindBy(key K) (R, bool, error)
    DeleteBy(key K) (bool, error)
    UpdateBy(key K, row R) (R, error)
}
type BTreeIndex[R, K any] interface {
    Scan() (Iterator[R], error)
    ScanRange(start, end K) (Iterator[R], error)
}
```

Prefer the generated handles for normal module authoring; use this package when you need explicit error handling or write registration code manually.

---

## Enums and Sum Types

### Enum (unit variants)

```go
//stdb:enum variants=Online,Offline,Away
type Status uint8

const (
    StatusOnline  Status = 0
    StatusOffline Status = 1
    StatusAway    Status = 2
)
```

- The Go type must be `uint8`-backed; constants must match the variant order (tag = index in `variants=`).
- Encoded as a single tag byte; decoding rejects tags ≥ variant count.
- `scope=Combat` (or `scope=A.B`) namespaces the type in the module definition.

### Sum type (data-carrying variants)

```go
//stdb:sumtype
type Shape interface{ isShape() }

//stdb:variant of=Shape name=Circle
type ShapeCircle struct{ Radius float64 }

//stdb:variant of=Shape name=Rectangle
type ShapeRectangle struct{ Width, Height float64 }

//stdb:variant of=Shape
type ShapePoint struct{}          // name defaults to "ShapePoint"
```

Each variant struct must implement the interface (`func (ShapeCircle) isShape() {}`). Tags are assigned in variant declaration order. Encoding rules:

- **Single-field variant** — payload written directly after the tag (not wrapped in a product).
- **Multi-field variant** — fields written as a product.
- **Empty variant** — tag only (unit product in the schema).

Use pointer-free value structs for variants; the generated writer type-switches on the concrete value type.

---

## Scheduled Reducers

A schedule table links rows to a reducer the host invokes:

```go
import "go.digitalxero.dev/spacetimedb-client/types"

//stdb:table name=cleanup_schedule access=private
//stdb:schedule table=cleanup_schedule function=run_cleanup
type CleanupSchedule struct {
    ScheduledId uint64           `stdb:"primarykey,autoinc"`
    ScheduledAt types.ScheduleAt
}

//stdb:reducer name=run_cleanup
func RunCleanup(ctx reducer.ReducerContext) {
    // runs at each scheduled time / interval
}
```

- The struct **must** have a `types.ScheduleAt` field — the generator locates its column index for the schedule definition.
- `function=` must match the reducer's registered (snake_case or `name=`) name.
- Insert rows to schedule work: `CleanupScheduleTable.Insert(CleanupSchedule{ScheduledAt: types.ScheduleAtInterval{Value: types.NewTimeDuration(5_000_000)}})` (every 5s) or `types.ScheduleAtTime{Value: ts}` (one-shot).

---

## Row-Level Security

RLS filters which rows of a table each client can see, using `@sender` for the caller's identity:

```go
//stdb:rls SELECT * FROM secret WHERE owner = @sender
func RlsSecretFilter() {}
```

The function body is ignored — only the directive matters. The SQL is passed through verbatim (`builder.AddRowLevelSecurity(...)`).

---

## Multi-Package Modules

`//stdb:` declarations may live in nested directories; each directory is its own Go package. Rules enforced by the parser/generator:

1. The root package is `main` (the WASM entry). Nested directories must **not** be `package main` (error).
2. Reducers, lifecycle hooks, procedures, and views in non-root packages must be **exported** (`MeleeAttack`, not `meleeAttack`) so the root dispatcher can call them (error otherwise).
3. Type (struct) names must be **unique across the whole module** — cross-package name collisions are an error.
4. Put table structs in an importable package (e.g. `schema/`) so feature packages can use the generated `*Table` handles without importing `main`.

Generated output changes in multi-package mode:

- `<pkg>/stdb_tables_generated.go` per package owning tables/types — exported `StdbReadX`/`StdbWriteX` codecs + table handles.
- Root `stdb_module_generated.go` — dispatch, moduledef, `init()`, calling e.g. `combat.MeleeAttack` and decoding struct args via `schema.StdbReadAttackReq`.

Flat single-package modules emit one `stdb_generated.go` as before.

```
mymodule/
  main.go                        # package main, empty main()
  stdb_module_generated.go       # generated root dispatch
  schema/
    types.go                     # //stdb:table structs
    stdb_tables_generated.go     # generated codecs + handles
  combat/
    reducers.go                  # exported //stdb:reducer funcs using schema.*Table
```

---

## Authentication (auth package)

`go.digitalxero.dev/spacetimedb-server/auth` exposes the caller's **host-validated** JWT to module code. The host already verified the signature; modules only inspect claims.

Available via `ctx.SenderAuth()` on **reducers and procedures** (both carry a connection id). Not available on views. Reducers can use it directly (implicit transaction); procedures manage the required short transaction internally — behavior is identical from the author's perspective.

```go
type SenderAuth interface {
    IsInternal() bool              // scheduled/lifecycle calls — never carry a JWT
    HasJWT() bool
    JWT() (claims JWTClaims, ok bool)
}
```

`JWTClaims`: `Subject()`, `Issuer()`, `Audience() []string`, `ExpiresAt()/IssuedAt()/NotBefore() int64`, `RawPayload() []byte`, `Claim(name) (any, bool)`, `StringClaim(name) (string, bool)`, `Into(v any) error` (unmarshal custom claims).

### Trusted-issuer checks

Build a reusable `Authorizer` once (package-level) and call it per request:

```go
var authz, _ = auth.NewTrustedIssuers().
    WithIssuer("https://issuer.example.com").
    WithAudience("my-module").   // optional; omit to skip audience checks
    WithLeeway(60).              // clock-skew seconds (default 60)
    Build()                      // errors if no issuer configured

//stdb:reducer
func Privileged(ctx reducer.ReducerContext) error {
    claims, err := authz.Authorize(ctx.SenderAuth(), ctx.Timestamp())
    if err != nil {
        return err // auth.ErrNoJWT / ErrIssuerMismatch / ErrExpired / ErrNotYetValid / ErrAudienceMismatch
    }
    _ = claims.Subject()
    return nil
}
```

---

## HTTP from Procedures (http package)

`go.digitalxero.dev/spacetimedb-server/http` works **only inside procedures** — reducers do not have the HTTP host interface and calls will fail.

```go
import stdbhttp "go.digitalxero.dev/spacetimedb-server/http"

//stdb:procedure
func FetchPrice(ctx reducer.ProcedureContext, symbol string) float64 {
    status, body, err := stdbhttp.Get("https://api.example.com/price/" + symbol)
    if err != nil || status != 200 { return 0 }

    // POST with headers/body:
    status, body, err = stdbhttp.Send(
        stdbhttp.MethodPost,
        "https://api.example.com/data",
        map[string]string{"Content-Type": "application/json"},
        []byte(`{"key":"value"}`),
    )
    _ = body
    // write results inside a transaction:
    ctx.WithTx(func() { /* table ops */ })
    return 0
}
```

Method constants: `MethodGet/Head/Post/Put/Delete/Connect/Options/Trace/Patch`. `ProcedureContext.HttpGet` is a shortcut for GET.

---

## Logging (log package)

```go
import (
    "go.digitalxero.dev/spacetimedb-server/log"
    // or: server "go.digitalxero.dev/spacetimedb-server" and server.NewLogger
)

logger := log.NewLogger("my_module")   // target name shown in host logs
logger.Error("...") ; logger.Warn("...") ; logger.Info("...")
logger.Debug("...") ; logger.Trace("...")
```

Writes via the host's `console_log`. Level constants live in `sys` (`LogLevelError`..`LogLevelTrace`) — prefer the Logger interface.

---

## Testing with mockhost

Module code runs against the `sys.Host` interface. Outside WASM the default host panics, so tests swap in `mockhost.MockHost` via `sys.SetHost` (returns a restore func). Set only the `Fn` fields the test needs — any unconfigured method panics with a descriptive message.

```go
package mymodule_test

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "go.digitalxero.dev/spacetimedb-server/log"
    "go.digitalxero.dev/spacetimedb-server/sys"
    "go.digitalxero.dev/spacetimedb-server/sys/mockhost"
)

func TestLogger(t *testing.T) {
    var captured string
    mock := &mockhost.MockHost{
        ConsoleLogFn: func(level uint8, target, filename string, line uint32, message string) {
            captured = message
        },
    }
    restore := sys.SetHost(mock)
    defer restore()

    log.NewLogger("test").Info("hello")
    assert.Equal(t, "hello", captured)
}

func TestCreatePlayer(t *testing.T) {
    var inserted []byte
    mock := &mockhost.MockHost{
        TableIdFromNameFn: func(name string) (uint32, error) {
            require.Equal(t, "player", name)
            return 1, nil
        },
        DatastoreInsertBSATNFn: func(tableId uint32, row []byte) ([]byte, error) {
            inserted = row
            return nil, nil // return sequence bytes to simulate autoinc
        },
    }
    restore := sys.SetHost(mock)
    defer restore()

    // Call the reducer directly with a test context.
    // CreatePlayer(reducer.NewReducerContext(sender, connId, ts, modId), "zeke")
    _ = inserted
}
```

Useful helpers:

- `mock.NewRowIteratorFromData(data)` — a `sys.RowIterator` yielding `data` once, then exhausted (simulates a point lookup for `FindBy`).
- `mock.NewEmptyRowIterator()` — immediately exhausted (not found). Both overwrite `RowIterBSATNAdvanceFn`/`RowIterBSATNCloseFn` on the mock.
- `sys.Errno` sentinels for host errors: `sys.ErrNoSuchTable`, `sys.ErrNoSuchIndex`, `sys.ErrUniqueAlreadyExists`, `sys.ErrNotInTransaction`, `sys.ErrHTTPError`, ...
- Test contexts: `reducer.NewReducerContext(...)`, `reducer.NewViewContext(sender)`, `reducer.NewAnonymousViewContext()`.

Remember (user Go style): test files use `package <pkg>_test` and testify's `assert`/`require`.

---

## Complete Worked Example

### main.go

```go
//go:generate go run go.digitalxero.dev/stdb-go
package main

func main() {}
```

### types.go

```go
package main

import "go.digitalxero.dev/spacetimedb-client/types"

//stdb:enum variants=Online,Offline,Away
type Status uint8

const (
    StatusOnline  Status = 0
    StatusOffline Status = 1
    StatusAway    Status = 2
)

type Position struct {
    X float64
    Y float64
}

//stdb:table name=player access=public
type Player struct {
    Id       uint64         `stdb:"primarykey,autoinc"`
    Name     string         `stdb:"unique"`
    Owner    types.Identity
    Position Position
    Status   Status
    Score    *uint64
}

//stdb:table name=game_log access=public event=true
type GameLog struct {
    Message string
    Level   uint8
}

//stdb:table name=tick_schedule access=private
//stdb:schedule table=tick_schedule function=tick
type TickSchedule struct {
    ScheduledId uint64           `stdb:"primarykey,autoinc"`
    ScheduledAt types.ScheduleAt
}

//stdb:rls SELECT * FROM player WHERE owner = @sender
func RlsPlayerFilter() {}
```

### reducers.go

```go
package main

import (
    "fmt"

    "go.digitalxero.dev/spacetimedb-client/types"
    "go.digitalxero.dev/spacetimedb-server/log"
    "go.digitalxero.dev/spacetimedb-server/reducer"
)

var logger = log.NewLogger("mymodule")

//stdb:init
func Init(ctx reducer.ReducerContext) {
    // schedule a tick every 10 seconds
    TickScheduleTable.Insert(TickSchedule{
        ScheduledAt: types.ScheduleAtInterval{Value: types.NewTimeDuration(10_000_000)},
    })
    logger.Info("module initialized")
}

//stdb:connect
func OnConnect(ctx reducer.ReducerContext) {}

//stdb:disconnect
func OnDisconnect(ctx reducer.ReducerContext) {}

//stdb:reducer
func CreatePlayer(ctx reducer.ReducerContext, name string) error {
    if _, found, _ := PlayerTable.FindByName(name); found {
        return fmt.Errorf("player %q already exists", name)
    }
    row := PlayerTable.Insert(Player{Name: name, Owner: ctx.Sender(), Status: StatusOnline})
    GameLogTable.Insert(GameLog{Message: fmt.Sprintf("player %d joined", row.Id), Level: 1})
    return nil
}

//stdb:reducer
func UpdateScore(ctx reducer.ReducerContext, playerId uint64, score uint64) error {
    p, found, err := PlayerTable.FindById(playerId)
    if err != nil || !found {
        return fmt.Errorf("player %d not found", playerId)
    }
    p.Score = &score
    PlayerTable.UpdateById(p)
    return nil
}

//stdb:reducer name=tick
func Tick(ctx reducer.ReducerContext) {
    GameLogTable.Insert(GameLog{Message: "tick", Level: 0})
}

//stdb:view public=true
func GetPlayers(ctx reducer.ViewContext) []Player {
    iter, err := PlayerTable.Scan()
    if err != nil { return nil }
    defer iter.Close()
    var out []Player
    for p, ok := iter.Next(); ok; p, ok = iter.Next() {
        out = append(out, p)
    }
    return out
}

//stdb:procedure
func PlayerCount(ctx reducer.ProcedureContext) uint64 {
    var n uint64
    ctx.WithTx(func() { n, _ = PlayerTable.Count() })
    return n
}
```

### spacetime.json / go.mod

```json
{ "name": "mymodule", "edition": "1.0" }
```

```
module mymodule

go 1.25.0

require (
    go.digitalxero.dev/spacetimedb-client vX.Y.Z
    go.digitalxero.dev/spacetimedb-server vX.Y.Z
)
```

---

## Build and Publish

Full CLI details are in the **stdb-go-cli** skill; the essentials:

```bash
go generate ./...                 # or: stdb-go generate server / bare stdb-go
stdb-go build                     # codegen + GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared
                                  #   + WASI-import shim + optional wasm-opt → module.wasm
stdb-go publish -d mydb           # build + deploy via the SpacetimeDB HTTP API
stdb-go dev -d mydb               # watch, rebuild, republish on change
```

Publish flags worth knowing when schemas change: `--delete-data always|on-conflict|never`, `--clear-database`, `--break-clients` (required for client-breaking migrations such as new `default=` columns), `--yes` to auto-confirm.

---

## Gotchas and Constraints

- **main.go is invisible to the generator.** Any `//stdb:` directive there (including `//stdb:init`) is silently ignored. Keep main.go to the `go:generate` line and `func main() {}`.
- **No paren syntax.** `//stdb:table(name=x,public)` / `//stdb:column(...)` / `//stdb:reducer(lifecycle=init)` come from an older doc style and are not parsed. Use `//stdb:table name=x access=public`, struct tags, and `//stdb:init`.
- Generated codecs are **exported** (`StdbWritePlayer`/`StdbReadPlayer`); dispatch functions (`stdbCallReducer`, `stdbDescribeModule`) are unexported.
- `Insert`, `Delete`, `UpdateBy*`, `DeleteBy*` **panic** on host errors (unique violations included); design reducers to check with `FindBy` first or accept the abort.
- Reducers: optional `error` return only. Procedures: one return value or none. Views: exactly one return value. None of them may return `(T, error)`.
- Table ops from a procedure must be inside `WithTx`/`TryWithTx` (`sys.ErrNotInTransaction` otherwise). Reducers must not use HTTP/sleep.
- `index=direct` produces no Go accessor; `FilterBy` needs `index=btree`; `FindBy`/`UpdateBy`/`DeleteBy` need `primarykey` or `unique`.
- Enum types must be `uint8`-backed with tags 0..n-1; sum-type variants implement the marker interface.
- Structs used in tables/params must have unique names module-wide (multi-package rule) and resolvable field types; `int`/`uint`/maps are unsupported.
- `default=` + `autoinc` is a generate-time error; non-empty array literals need `default=raw:<hex>`.
- WASM execution is single-threaded — no goroutines/locks needed for table state; package-level vars are safe within a call.
