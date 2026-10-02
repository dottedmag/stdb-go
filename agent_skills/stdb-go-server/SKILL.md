---
name: stdb-go-server
description: 'Use whenever writing, reviewing, or debugging SpacetimeDB server modules in Go — any code importing go.digitalxero.dev/spacetimedb-server, any Go file with //stdb: comment directives (//stdb:table, //stdb:reducer, //stdb:column, //stdb:init, //stdb:connect, //stdb:disconnect, //stdb:procedure, //stdb:view, //stdb:enum, //stdb:sumtype, //stdb:schedule, //stdb:rls), or structs with stdb struct tags (primarykey, autoinc, unique, index=btree, index=direct). Trigger on reducers, procedures, or views in SpacetimeDB Go modules; ReducerContext, ProcedureContext, or ViewContext; generated table handles (Insert, Scan, FindBy, FilterBy, UpdateBy, DeleteBy); lifecycle hooks; scheduled reducers; row-level security; SenderAuth/JWT validation; HTTP from procedures; testing with mockhost; or building WASM modules for SpacetimeDB in Go. Covers server-module authoring only — for stdb-go CLI commands (init/build/publish/dev/generate) use the stdb-go-cli skill; for client-side Go code use the stdb-go-client skill.'
---

# Writing SpacetimeDB Server Modules in Go

## How it works

A SpacetimeDB Go module is plain Go annotated with `//stdb:` comment directives and `stdb:"..."` struct tags. The `stdb-go` code generator (`github.com/dottedmag/stdb-go`) parses those annotations and emits `stdb_generated.go` containing BSATN codecs, table handles, reducer/procedure/view dispatch, and the module schema. `stdb-go build` then compiles everything to WASM (`GOOS=wasip1 GOARCH=wasm`). You write types and functions; the generator writes all the glue.

The runtime library is `go.digitalxero.dev/spacetimedb-server` (contexts, tables, sys, auth, http, log). NEVER use `github.com/clockworklabs/...` import paths — they do not exist for this SDK.

## Project setup

```
mymodule/
  spacetime.json       # {"name": "mymodule", "edition": "1.0"}
  go.mod               # requires spacetimedb-server + spacetimedb-client, go 1.25.0
  main.go              # //go:generate line + empty func main() {} — NOTHING ELSE
  types.go             # //stdb:table structs, enums, sum types
  reducers.go          # //stdb:reducer, lifecycle hooks
  stdb_generated.go    # generated — never edit
```

Rules that break silently if violated:

- **Go 1.25+** is required (`wasip1` + `go:wasmexport`). Build with the standard toolchain, never TinyGo.
- The module root is `package main` with an **empty** `func main() {}` (required for `-buildmode=c-shared` WASM).
- `main.go` is **skipped by the directive parser**. A `//stdb:` directive in `main.go` is silently ignored — put directives in other files (`types.go`, `reducers.go`, ...). Also skipped: `*_test.go`, `*_generated.go`, `vendor/`, `testdata/`, dot-dirs.
- Put `//go:generate go run github.com/dottedmag/stdb-go` in main.go so `go generate ./...` works.
- Directives may also live in nested packages (multi-package modules) — see the reference for the extra rules (exported functions, no nested `package main`).

## Directive quick reference

Directives are `//` comments on the line immediately above the declaration, format `//stdb:kind key=value key=value` (space-separated, no parentheses).

| Directive | On | Purpose |
|---|---|---|
| `//stdb:table name=<snake> [access=public\|private] [event=true] [index=<name>:<col0>,<col1>]` | struct | Register a table (access defaults to `private`; stack multiple for multi-table) |
| `//stdb:reducer [name=<snake>]` | func | Client-callable mutation; first param `reducer.ReducerContext`, optional `error` return |
| `//stdb:init` / `//stdb:connect` / `//stdb:disconnect` | func | Lifecycle hooks; `func(ctx reducer.ReducerContext)`, no return |
| `//stdb:procedure [name=<snake>]` | func | First param `reducer.ProcedureContext`; explicit tx, HTTP, sleep, UUIDv7; returns one value or nothing |
| `//stdb:view [name=<snake>] [public=true]` | func | Read-only query; `reducer.ViewContext` (authed) or `reducer.AnonymousViewContext`; must return one value |
| `//stdb:enum variants=<A>,<B>,<C> [scope=<Ns>]` | uint8 type | Unit-variant sum type (tag byte) |
| `//stdb:sumtype [scope=<Ns>]` | interface | Tagged union; interface needs an unexported marker method |
| `//stdb:variant of=<Iface> [name=<Variant>]` | struct | One variant of a sumtype (name defaults to struct name) |
| `//stdb:schedule table=<table> function=<reducer>` | struct or func | Link a scheduled table (needs a `types.ScheduleAt` field) to a reducer |
| `//stdb:rls <SQL>` | func or type | Row-level security filter, e.g. `SELECT * FROM secret WHERE owner = @sender` |

There is no `//stdb:column` directive in the current grammar — column options are **struct tags**. (Older README examples showing `//stdb:table(name=x,public)`, `//stdb:column(...)`, or `//stdb:reducer(lifecycle=init)` paren-syntax are stale; the parser ignores them.)

## Struct tags

```go
//stdb:table name=player access=public
type Player struct {
    Id    uint64 `stdb:"primarykey,autoinc"`
    Name  string `stdb:"unique"`
    Level uint32 `stdb:"index=btree"`
    Kind  uint8  `stdb:"index=direct"`
    Score uint32 `stdb:"default=0"`   // enables additive migration backfill
}
```

| Tag | Effect | Generated methods |
|---|---|---|
| `primarykey` | PK + unique constraint + btree index | `FindBy<F>`, `UpdateBy<F>`, `DeleteBy<F>` |
| `unique` | Unique constraint + btree index | `FindBy<F>`, `UpdateBy<F>`, `DeleteBy<F>` |
| `autoinc` | Sequence (Insert/UpdateBy return the filled row) | — (combine with primarykey) |
| `index=btree` | Range-capable index | `FilterBy<F>`, `DeleteBy<F>` |
| `index=direct` | Direct index in the schema | none (schema-only, no accessor) |
| `default=<v>` | Column default for in-place migrations | — (see reference for value syntax) |

Only exported fields are serialized; field names become snake_case columns. `default=` cannot combine with `autoinc`.

## Contexts — pick the right function kind

| | Reducer | Procedure | View |
|---|---|---|---|
| Transaction | automatic, whole call atomic | explicit `WithTx` / `TryWithTx` | read-only |
| HTTP (`http.Get`/`Send`, `ctx.HttpGet`) | no | yes | no |
| `SleepUntil`, `NewUuidV7` | no | yes | no |
| `Sender()` / `ConnectionId()` / `Timestamp()` | yes / yes / yes | yes / yes / yes | Sender only (authed); nothing (anonymous) |
| `SenderAuth()` (validated JWT claims) | yes | yes | no (no connection id in the ABI) |
| Mutates state | yes | via tx | never |

## Table handle essentials

Each `//stdb:table name=player ...` generates `var PlayerTable = ...` (PascalCase of the **table name** + `Table`):

```go
row := PlayerTable.Insert(Player{Name: "zeke"})    // returns row with autoinc filled; panics on error
p, found, err := PlayerTable.FindByName("zeke")    // pk/unique fields
PlayerTable.UpdateByName(p)                        // update via unique index; panics on error
n := PlayerTable.DeleteById(p.Id)                  // returns delete count
iter, err := PlayerTable.FilterByLevel(3)          // btree fields; defer iter.Close()
iter, err := PlayerTable.Scan()                    // all rows; Next() (T, bool); defer Close()
count, err := PlayerTable.Count()
removed, err := PlayerTable.Clear()                // truncate
```

Multi-column `index=name:0,1` also generates `FilterBy<Col0>And<Col1>` plus prefix variants.

## Common mistakes

- Putting `//stdb:` directives in `main.go` — silently ignored (the parser skips that file).
- Using the stale paren directive syntax (`//stdb:table(name=x,public)`, `//stdb:column`) — parsed as an unknown kind and dropped.
- Editing `stdb_generated.go` / `stdb_module_generated.go` / `stdb_tables_generated.go` — overwritten on every generate.
- Old `github.com/clockworklabs/...` import paths — always `go.digitalxero.dev/spacetimedb-server` (and `spacetimedb-client/types` for Identity, Timestamp, ScheduleAt, ...).
- Making HTTP calls or sleeping in a reducer — only procedures can; reducers fail because the host interface isn't available.
- Procedures/views returning `(T, error)` — they must return exactly one value (or nothing, procedures only); only reducers may return `error`.
- Expecting `FindBy`/`UpdateBy` on `index=btree` fields — those get `FilterBy`; point lookups need `primarykey` or `unique`.
- Forgetting `defer iter.Close()` on Scan/FilterBy iterators.
- Unexported reducers/views in nested packages — non-root `//stdb:` functions must be exported.

## When to read the reference

Read [references/server-reference.md](references/server-reference.md) before doing any of these: writing enums/sum types, `default=` column values, scheduled reducers, RLS, multi-package module layout, procedures with transactions/HTTP, view result shapes, JWT auth (`SenderAuth`, `auth.NewTrustedIssuers`), unit testing with `sys.SetHost` + `mockhost`, or when you need the full Go↔SATS type mapping table, the exact generated-code surface, or a complete worked example module.
