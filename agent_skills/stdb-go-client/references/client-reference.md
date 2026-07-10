# SpacetimeDB Go Client Reference

Complete reference for `go.digitalxero.dev/spacetimedb-client` and the bindings
produced by `stdb-go generate client`. All import paths use
`go.digitalxero.dev/spacetimedb-client` — never `github.com/clockworklabs/...`.
Requires Go 1.25+.

---

## Table of Contents

1. [Packages and Imports](#packages-and-imports)
2. [Connection Builder](#connection-builder)
3. [DbConnection Interface](#dbconnection-interface)
4. [Event Loop Model](#event-loop-model)
5. [Table Definitions and the Row Cache](#table-definitions-and-the-row-cache)
6. [Subscriptions](#subscriptions)
7. [Calling Reducers](#calling-reducers)
8. [Calling Procedures](#calling-procedures)
9. [One-Off Queries](#one-off-queries)
10. [Error Types](#error-types)
11. [Event Contexts](#event-contexts)
12. [BSATN Serialization](#bsatn-serialization)
13. [The types Package](#the-types-package)
14. [Generated Client Bindings](#generated-client-bindings)
15. [Complete Worked Example](#complete-worked-example)
16. [Project Structure](#project-structure)

---

## Packages and Imports

| Package | Import path | Use |
|---------|-------------|-----|
| client | `go.digitalxero.dev/spacetimedb-client/client` | `DbConnection`, `SubscriptionBuilder`, error types, event contexts |
| cache | `go.digitalxero.dev/spacetimedb-client/client/cache` | `ClientCache`, `TableCache`, `TableDef`, `TypedTableCache`, `RegisterTypedTable[WithPK]` |
| bsatn | `go.digitalxero.dev/spacetimedb-client/bsatn` | `Writer`, `Reader`, `Serializable`, generic encode/decode helpers |
| types | `go.digitalxero.dev/spacetimedb-client/types` | `Identity`, `ConnectionId`, `Uuid`, `Timestamp`, `Uint128/256`, `ScheduleAt`, SATS descriptors |
| protocol | `go.digitalxero.dev/spacetimedb-client/client/protocol` | Internal wire types; users only need the `Compression` constants |
| ws | `go.digitalxero.dev/spacetimedb-client/client/ws` | Internal low-level WebSocket; used indirectly through `client` |

---

## Connection Builder

`client.NewDbConnection()` returns a `DbConnectionBuilder`:

```go
type DbConnectionBuilder interface {
    WithUri(uri string) DbConnectionBuilder
    WithDatabaseName(nameOrAddress string) DbConnectionBuilder
    WithToken(token string) DbConnectionBuilder
    WithCompression(c protocol.Compression) DbConnectionBuilder
    OnConnect(fn func(conn DbConnection, identity types.Identity, token string)) DbConnectionBuilder
    OnConnectError(fn func(err error)) DbConnectionBuilder
    OnDisconnect(fn func(conn DbConnection, err error)) DbConnectionBuilder
    OnReducerError(fn func(err error)) DbConnectionBuilder
    Build(ctx context.Context) (DbConnection, error)
}
```

- `WithUri` / `WithDatabaseName` are required before `Build`.
- `WithToken` supplies an auth token for reconnecting as the same identity
  (obtain it from `conn.Token()` or the `OnConnect` callback).
- `WithCompression` selects wire compression: `protocol.CompressionNone` (default),
  `protocol.CompressionBrotli`, or `protocol.CompressionGzip`.
- `OnConnect` fires from inside the event loop when the server delivers the
  `InitialConnection` message (identity + token) — i.e. after `Run` has started,
  not during `Build`. It is the idiomatic place to `Subscribe` and `CallReducer`.
- `OnConnectError` fires when `Build` fails to dial and when incoming messages
  fail to decompress/decode (`*ProtocolError`).
- `OnDisconnect` fires on clean `Disconnect()`, context cancellation, and
  connection loss (with a `*ConnectionError` in the lost case, `nil`/`ctx.Err()` otherwise).
- `OnReducerError` fires when a fire-and-forget reducer call fails on the server
  (structured error return or host error such as "no such reducer"). The error is
  a `*ReducerError`. A reducer failure does NOT affect the connection — do not
  treat it as fatal.
- `Build(ctx)` dials the WebSocket and returns the connection; on failure it
  invokes `OnConnectError` and returns a `*ConnectionError`.

---

## DbConnection Interface

```go
type DbConnection interface {
    Identity() types.Identity          // zero until OnConnect fires
    ConnectionId() types.ConnectionId
    Token() string                     // auth token, valid after OnConnect
    IsActive() bool                    // true while Run is executing
    Subscribe(queries ...string) SubscriptionBuilder
    CallReducer(reducer string, args bsatn.Serializable) error
    CallProcedure(ctx context.Context, procedure string, args bsatn.Serializable) ([]byte, error)
    OneOffQuery(query string) ([][]byte, error)
    Disconnect() error
    Run(ctx context.Context) error
    RegisterTable(def cache.TableDef)  // must be called BEFORE Run
    Cache() cache.ClientCache
}
```

---

## Event Loop Model

`Run(ctx)` blocks on the calling goroutine and processes **all** WebSocket
messages and commands sequentially in that single goroutine:

- Server messages (subscription updates, reducer results, identity tokens) are
  dispatched one at a time. Cache mutations and every callback (`OnConnect`,
  `OnApplied`, `OnInsert`/`OnDelete`/`OnUpdate`, `OnReducerError`, ...) run here.
- Commands (`CallReducer`, `Subscribe(...).Build()`, `OneOffQuery`,
  `CallProcedure`, `Disconnect`) send requests through an internal buffered
  channel that `Run` consumes — they are safe to call from any goroutine.
- Because there is exactly one writer goroutine, **no locks are needed and you
  must not add your own** around connection or cache state.

Rules:

- `RegisterTable` must happen before `Run`; unregistered tables are ignored when
  subscription rows arrive.
- Call `Run` exactly once. Calling it again after it returns is undefined behavior.
- `Run` returns `ctx.Err()` on cancellation, `nil` after a clean `Disconnect()`,
  and a `*ConnectionError` ("connection lost") if the socket drops.
- There is **no automatic reconnect**: when `Run` returns you must build a new
  connection (pass `WithToken` to keep the same identity) and re-subscribe.
- Callbacks execute on the event-loop goroutine — keep them fast. Never call
  `CallProcedure` or `OneOffQuery` from inside a callback: both block waiting for
  the event loop to process the response, which deadlocks.

Typical structure: either run `conn.Run(ctx)` as the final blocking call of
`main`, or start it in a goroutine and coordinate through callbacks/channels.

---

## Table Definitions and the Row Cache

### TableDef (untyped, hand-written clients)

```go
type TableDef interface {
    TableName() string                       // must match the server-side name exactly
    DecodeRow(r bsatn.Reader) (any, error)   // typically returns *YourStruct
    EncodeRow(row any) []byte                // BSATN bytes; used as the cache key
}

type TableDefWithPK interface {
    TableDef
    PrimaryKey(row any) any // must return a comparable value (uint64, string, ...)
}
```

Implement `TableDefWithPK` whenever the table has a primary key: the cache then
matches delete+insert pairs sharing a PK within one transaction and fires
`OnUpdate(old, new)` instead of separate `OnDelete` + `OnInsert`.

Example:

```go
type Player struct {
    ID   uint64
    Name string
    HP   uint32
}

type playerDef struct{}

func (playerDef) TableName() string { return "Player" }

func (playerDef) DecodeRow(r bsatn.Reader) (any, error) {
    id, _ := r.GetU64()
    name, _ := r.GetString()
    hp, err := r.GetU32()
    if err != nil {
        return nil, err
    }
    return &Player{ID: id, Name: name, HP: hp}, nil
}

func (playerDef) EncodeRow(row any) []byte {
    p := row.(*Player)
    w := bsatn.NewWriter(32)
    w.PutU64(p.ID)
    w.PutString(p.Name)
    w.PutU32(p.HP)
    return w.Bytes()
}

func (playerDef) PrimaryKey(row any) any { return row.(*Player).ID }
```

Register with `conn.RegisterTable(playerDef{})` (or `cache.RegisterTable` on a
`ClientCache`) before `Run`.

### ClientCache and TableCache

```go
type ClientCache interface {
    GetTable(name string) TableCache           // nil if not registered
    RegisterTable(def TableDef)
    DecodeRow(tableName string, r bsatn.Reader) (any, error)
    GetTableDef(tableName string) (TableDefWithPK, bool)
}

type TableCache interface {
    Count() int
    Iter(fn func(row any) bool)                // return false to stop iteration
    OnInsert(cb InsertCallback) CallbackID     // func(row any)
    OnDelete(cb DeleteCallback) CallbackID     // func(row any)
    OnUpdate(cb UpdateCallback) CallbackID     // func(oldRow, newRow any); needs PK
    OnEvent(cb EventCallback) CallbackID       // func(row any); ephemeral event tables
    RemoveCallback(id CallbackID)
    // ApplyInsert/ApplyDelete/ApplyUpdate/ApplyEvent are internal — never call them.
}
```

- Rows are keyed internally by their raw BSATN bytes and decoded through the
  registered `TableDef`.
- `OnEvent` fires for ephemeral event-table rows: they invoke only this callback
  and are never retained (they do not appear in `Count`/`Iter`).
- Every `On*` returns a `cache.CallbackID` for later `RemoveCallback`.
- There is no `All()` method — use `Iter` (or `Count`) to read cached rows.

### Typed table caches (used by generated code)

```go
type TypedTableDef[T any] interface {
    TableName() string
    DecodeRow(r bsatn.Reader) (T, error)
    EncodeRow(row T) []byte
}
type TypedTableDefWithPK[T any, K comparable] interface {
    TypedTableDef[T]
    PrimaryKey(row T) K
}

func RegisterTypedTable[T any](cc ClientCache, def TypedTableDef[T]) *TypedTableCache[T]
func RegisterTypedTableWithPK[T any, K comparable](cc ClientCache, def TypedTableDefWithPK[T, K]) *TypedTableCache[T]
```

`TypedTableCache[T]` mirrors `TableCache` with compile-time types:
`Count()`, `Iter(func(row T) bool)`, `OnInsert(func(T))`, `OnDelete(func(T))`,
`OnUpdate(func(old, new T))`, `OnEvent(func(T))`, `RemoveCallback(id)`.

---

## Subscriptions

```go
type SubscriptionBuilder interface {
    OnApplied(fn func()) SubscriptionBuilder   // initial rows are in the cache
    OnError(fn func(error)) SubscriptionBuilder // server rejected: *SubscriptionError
    Build() (SubscriptionHandle, error)
}

type SubscriptionHandle interface {
    Unsubscribe() error
    IsActive() bool
    QueryID() uint32 // server-side query_set_id
}
```

- `conn.Subscribe("SELECT * FROM Player", "SELECT * FROM Item")` accepts one or
  more SQL queries per subscription.
- Each `Build()` registers an **independent** subscription — it adds to the set
  of active queries, never replaces earlier ones. Remove one with
  `handle.Unsubscribe()`; the server confirms and drops its rows from the cache.
- `OnApplied` fires (in the event loop) after the initial matching rows have been
  applied to the cache as inserts — read `Count()`/`Iter` safely from there.
- Subscribing from inside `OnConnect` is the standard pattern.

---

## Calling Reducers

```go
err := conn.CallReducer("create_player", &createPlayerArgs{Name: "Alice"})
```

- Args must implement `bsatn.Serializable` (a single `WriteBsatn(w bsatn.Writer)`
  method writing fields in schema order). Pass `nil` for reducers with no args.
- `CallReducer` is **fire-and-forget**: it enqueues the command and returns
  immediately. The returned error does not reflect server-side failure.
- On a committed reducer the server's transaction update is applied to the cache
  (your `OnInsert`/`OnUpdate`/`OnDelete` callbacks observe the effects).
- On failure the connection's `OnReducerError` handler receives a
  `*ReducerError{ReducerName, Message}`.

Hand-written args struct:

```go
type createPlayerArgs struct{ Name string }

func (a *createPlayerArgs) WriteBsatn(w bsatn.Writer) {
    w.PutString(a.Name)
}
```

---

## Calling Procedures

Unlike reducers, procedures return a value:

```go
raw, err := conn.CallProcedure(ctx, "get_stats", args) // raw = BSATN-encoded return value
```

- Blocks until the server sends the matching result, `ctx` is cancelled, or the
  connection drops. Decode `raw` with a `bsatn.Reader` per the procedure's
  declared return type (generated bindings do this for you).
- Non-success statuses (host/internal error, out-of-energy) return a
  `*ProcedureError{ProcedureName, Message}`.
- **Never call from within the event loop** (i.e. from any callback) — it blocks
  waiting for that same goroutine and deadlocks.

---

## One-Off Queries

```go
rows, err := conn.OneOffQuery("SELECT * FROM Player") // [][]byte of raw BSATN rows
```

- Executes a single SQL query and returns each result row as raw BSATN bytes.
- Does **not** update the local cache and does not require a subscription.
- Decode rows yourself:

```go
for _, rowBytes := range rows {
    row, err := playerDef{}.DecodeRow(bsatn.NewReader(rowBytes))
    ...
}
```

- Server-side query errors come back as a `*ProtocolError` carrying the server's
  error string. Like `CallProcedure`, it blocks on the event loop — do not call
  it from a callback.

---

## Error Types

All five implement `error`; `ConnectionError` and `ProtocolError` also implement
`Unwrap()` for `errors.Is`/`errors.As` chains. Match with `errors.As`:

| Type | Fields | Produced by |
|------|--------|-------------|
| `*client.ConnectionError` | `Message string`, `Err error` | `Build` dial failure; `Run` when the socket is unexpectedly lost |
| `*client.ReducerError` | `ReducerName string`, `Message string` | Server-side reducer failure, delivered via `OnReducerError` |
| `*client.ProcedureError` | `ProcedureName string`, `Message string` | `CallProcedure` non-success status (host error, out-of-energy) |
| `*client.SubscriptionError` | `QuerySetID uint32`, `Message string` | Server rejects a subscription; delivered via the subscription's `OnError` |
| `*client.ProtocolError` | `Message string`, `Err error` | Decompress/decode failures; one-off query server errors |

---

## Event Contexts

Metadata carriers defined in the `client` package:

```go
type EventContext struct {
    Identity     types.Identity     // who caused the change
    ConnectionID types.ConnectionId
    Timestamp    types.Timestamp    // when it occurred
    Conn         DbConnection
}

type ReducerEventContext struct {
    EventContext
    ReducerName string
    Status      string // "committed" or "failed"
    ErrMessage  string // set when Status == "failed"
}

type ErrorContext struct {
    Err error
}
```

`client.CallbackID` (an alias concept of `cache.CallbackID`) identifies
registered callbacks for removal via `TableCache.RemoveCallback`.

---

## BSATN Serialization

BSATN (Binary SpacetimeDB Algebraic Type Notation) is a deterministic,
little-endian binary format. The bytes carry no type tags or field names —
**field order must match the schema exactly**; that ordering is the contract.

### Encoding rules

- Primitives: natural width, little-endian (`bool` = 1 byte, 0x00/0x01).
- Strings: u32 LE byte-length prefix + raw UTF-8 bytes.
- Arrays: u32 LE element count + elements sequentially ([]byte = count + raw bytes).
- Maps: u32 LE entry count + key-value pairs sequentially.
- Products (structs): fields written sequentially, no prefix or separator.
- Sums (tagged unions): u8 tag byte + variant payload (unit variants: no payload).
- Options: special sum — **tag 0 = Some(value), tag 1 = None**. Go models
  `Option<T>` as `*T` where `nil` means None.

### Go-to-BSATN type mapping

| Go type | BSATN type | Wire size |
|---------|-----------|-----------|
| `bool` | bool | 1 byte |
| `uint8` / `int8` | u8 / i8 | 1 byte |
| `uint16` / `int16` | u16 / i16 | 2 bytes LE |
| `uint32` / `int32` | u32 / i32 | 4 bytes LE |
| `uint64` / `int64` | u64 / i64 | 8 bytes LE |
| `float32` / `float64` | f32 / f64 | 4 / 8 bytes LE (IEEE 754) |
| `string` | String | 4-byte len + UTF-8 |
| `[]byte` | Array\<u8\> | 4-byte len + raw bytes |
| `[]T` | Array\<T\> | 4-byte count + elements |
| `*T` | Option\<T\> | 1-byte tag [+ value] |
| `map[K]V` | Map\<K,V\> | 4-byte count + pairs |
| struct | Product | sequential fields |
| tagged union | Sum | 1-byte tag + payload |

### Core interfaces

```go
w := bsatn.NewWriter(64)     // streaming encoder; grows as needed
w.PutU32(42); w.PutString("hello")
data := w.Bytes()

r := bsatn.NewReader(data)   // streaming decoder
id, err := r.GetU32()
name, err := r.GetString()

type Serializable interface { WriteBsatn(w Writer) }
```

`bsatn.NewZeroCopyReader(data)` returns a `Reader` whose `GetString` shares the
underlying buffer (no copy) — the buffer must outlive the decoded strings.

Writer methods: `PutBool/PutU8/PutU16/PutU32/PutU64/PutI8/.../PutF32/PutF64/PutString`.
Reader methods: matching `GetBool/GetU8/.../GetString`, returning typed errors:
`bsatn.ErrBufferTooShort`, `bsatn.ErrInvalidBool`, `bsatn.ErrInvalidTag`.

### Generic helpers

- `Encode(s Serializable) []byte` / `Decode(data, readFn)`
- `WriteArray` / `ReadArray` — slices of `Serializable` elements
- `WriteByteArray` / `ReadByteArray` — `[]byte`
- `WriteOption` / `ReadOption` — `*T` as Option
- `WriteMap` / `ReadMap` — typed key-value maps
- `WriteSum(w, tag, payload)` / `WriteSumUnit(w, tag)` — sum variants
- One-shot primitive helpers: `EncodeBool`/`DecodeBool`, `EncodeU32`/`DecodeU32`,
  `EncodeString`/`DecodeString`, etc.

Sum type encoding example:

```go
func (m *chatMessage) WriteBsatn(w bsatn.Writer) {
    switch v := m.variant.(type) {
    case *textMessage:
        bsatn.WriteSum(w, 0, v)
    case *systemMessage:
        bsatn.WriteSum(w, 1, v)
    }
}
```

---

## The types Package

Every type implements `bsatn.Serializable` and follows a constructor pair:
`NewXxx(...)` from Go-native values and `ReadXxx(r bsatn.Reader)` for decoding
(e.g. `types.NewIdentity`, `types.ReadIdentity`, `types.ReadTimestamp`).

### Identifiers

- `Identity` — 32 bytes, identifies a user (product-wrapped u256). `IsZero()`
  reports the unset value.
- `ConnectionId` — 16 bytes, identifies a client connection (product-wrapped u128).
- `Uuid` — 16 bytes, standard UUID as product-wrapped u128.
  `NewUuidV7(counter *uint32, timestampMicros int64, randomBytes [4]byte)` generates
  a monotonic UUID v7.

All three store bytes little-endian internally but `String()` renders big-endian
hex to match the SpacetimeDB CLI/dashboard display.

### Extended integers

- `Uint128` / `Int128` — 16 bytes LE; `Lo()`/`Hi()` accessors.
- `Uint256` / `Int256` — 32 bytes LE; `Bytes()` accessor.
- Signed variants are two's complement; `String()` goes through `math/big.Int`.

### Time

- `Timestamp` — i64 **microseconds** since the Unix epoch; `Time()` converts to `time.Time`.
- `TimeDuration` — i64 microseconds; `Duration()` converts to `time.Duration`.

### Scheduling and energy

- `ScheduleAt` — sum type: `ScheduleAtInterval` (tag 0, holds a `TimeDuration`,
  recurring) and `ScheduleAtTime` (tag 1, holds a `Timestamp`, one-shot).
- `EnergyQuanta` — `Uint128` energy budget for a reducer call.

### Algebraic type system (SATS)

For schema introspection: `AlgebraicType` (constructors `AlgTypeBool`,
`AlgTypeU8`, `AlgTypeString`, ..., `AlgTypeProduct`, `AlgTypeSum`, `AlgTypeRef`),
`ProductType`/`ProductTypeElement`, `SumType`/`SumTypeVariant`, `TypeRef`, and
`Typespace` (with `Reserve`/`Set` for recursive definitions). Most client apps
never touch these directly.

---

## Generated Client Bindings

### Generating

```bash
# From a running SpacetimeDB server (database is required)
stdb-go generate client -d my-database

# Flags
#   --out-dir          output directory (default "module_bindings")
#   -s/--server        server URL (default from config or http://localhost:3000)
#   -d/--database      database name or identity (required; or from spacetime.json)
#   --token            auth token (default from env or cli.toml)
#   --package          Go package name (default: basename of --out-dir)
#   --schema-version   schema API version, string (default "10")
#   --include-private  include private tables/reducers/procedures/views
```

The schema is fetched over HTTP from the running server. Without
`--include-private`, private tables, reducers, procedures, and non-public views
are filtered out.

### Generated files

Each file is only written when the schema has matching items; all carry the
header `// Code generated by stdb-go generate client; DO NOT EDIT.`

| File | Contents |
|------|----------|
| `types_generated.go` | Struct definitions, enum types with `String()`, sum-type interfaces + variant structs |
| `bsatn_generated.go` | `WriteBsatn()` methods and `Read<Type>()` functions for every type |
| `tables_generated.go` | `<name>TableDef` structs (implementing `cache.TypedTableDef[*T]`, plus `PrimaryKey` when the table has a single PK) and `<Type>Table = cache.TypedTableCache[*T]` aliases |
| `reducers_generated.go` | `Call<Reducer>(conn, args...) error` functions + private args structs |
| `procedures_generated.go` | `Call<Procedure>(ctx, conn, args...) (T, error)` functions with result decoding |
| `views_generated.go` | `<name>ViewDef` structs and `<Name>View = cache.TypedTableCache[*T]` aliases |
| `module_generated.go` | `ModuleBindings` aggregator |

### ModuleBindings pattern

```go
// module_generated.go (shape)
type ModuleBindings struct {
    conn   client.DbConnection
    Player *PlayerTable // *cache.TypedTableCache[*Player]
    Item   *ItemTable
    // ... one field per table and per view
}

func NewModuleBindings(conn client.DbConnection) *ModuleBindings {
    c := conn.Cache()
    m := &ModuleBindings{conn: conn}
    // single-column PK -> RegisterTypedTableWithPK (enables OnUpdate)
    m.Player = cache.RegisterTypedTableWithPK[*Player, uint64](c, playerTableDef{})
    // no PK (or composite PK) and views -> RegisterTypedTable
    m.Item = cache.RegisterTypedTable[*Item](c, itemTableDef{})
    return m
}

func (m *ModuleBindings) Conn() client.DbConnection { return m.conn }
```

Usage — `NewModuleBindings` registers every table, so call it **before**
`conn.Run`:

```go
bindings := module_bindings.NewModuleBindings(conn)

bindings.Player.OnInsert(func(p *module_bindings.Player) {
    fmt.Println("new player:", p.Name)
})
bindings.Player.Iter(func(p *module_bindings.Player) bool {
    fmt.Println(p.Name)
    return true
})
n := bindings.Player.Count()

module_bindings.CallCreatePlayer(bindings.Conn(), "Alice", 100)
```

### Generated table defs

```go
type playerTableDef struct{}

func (playerTableDef) TableName() string { return "player" } // exact server name
func (playerTableDef) DecodeRow(r bsatn.Reader) (*Player, error) { return ReadPlayer(r) }
func (playerTableDef) EncodeRow(row *Player) []byte {
    w := bsatn.NewWriter(64)
    row.WriteBsatn(w)
    return w.Bytes()
}
func (playerTableDef) PrimaryKey(row *Player) uint64 { return row.ID } // single-PK tables only

type PlayerTable = cache.TypedTableCache[*Player]
```

### Generated reducer callers

```go
type createPlayerArgs struct {
    Name  string
    Score uint32
}

func (a *createPlayerArgs) WriteBsatn(w bsatn.Writer) {
    w.PutString(a.Name)
    w.PutU32(a.Score)
}

// CallCreatePlayer calls the create_player reducer on the server.
func CallCreatePlayer(conn client.DbConnection, name string, score uint32) error {
    args := &createPlayerArgs{Name: name, Score: score}
    return conn.CallReducer("create_player", args)
}

// No-arg reducers pass nil:
func CallResetGame(conn client.DbConnection) error {
    return conn.CallReducer("reset_game", nil)
}
```

### Generated procedure callers

```go
// With a return value:
func CallGetStats(ctx context.Context, conn client.DbConnection, playerID uint64) (Stats, error) {
    var zero Stats
    args := &getStatsArgs{PlayerID: playerID}
    raw, err := conn.CallProcedure(ctx, "get_stats", args)
    if err != nil {
        return zero, err
    }
    result, err := readGetStatsResult(bsatn.NewReader(raw))
    if err != nil {
        return zero, err
    }
    return *result, nil
}
```

### Generated type patterns

Naming: snake_case server names become Go PascalCase with common acronyms
uppercased (`player_id` -> `PlayerID`, `http_url` -> `HTTPURL`).

Plain enums (all-unit sum types) become a `uint8` with constants and `String()`:

```go
type PlayerStatus uint8

const (
    PlayerStatusOnline PlayerStatus = iota
    PlayerStatusOffline
)

func (e PlayerStatus) String() string { /* returns the variant name */ }
```

Sum types with payloads become an interface with a private marker method plus one
struct per variant:

```go
type MessageContent interface{ messageContentVariant() }

type MessageContentText struct{ Content string }
func (MessageContentText) messageContentVariant() {}

type MessageContentDeleted struct{}
func (MessageContentDeleted) messageContentVariant() {}
```

Optional fields become pointers (`*string` for `Option<String>`), arrays become
slices, and special SpacetimeDB types map to the `types` package
(`types.Identity`, `types.ConnectionId`, `types.Timestamp`, `types.Uint128`, ...).

---

## Complete Worked Example

Connect, subscribe, react to cache events, and call a reducer — using a
hand-written `TableDef` (swap in `NewModuleBindings` + generated callers for
generated projects):

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "os/signal"

    "go.digitalxero.dev/spacetimedb-client/bsatn"
    "go.digitalxero.dev/spacetimedb-client/client"
    "go.digitalxero.dev/spacetimedb-client/types"
)

type Player struct {
    ID   uint64
    Name string
    HP   uint32
}

type playerDef struct{}

func (playerDef) TableName() string { return "Player" }
func (playerDef) DecodeRow(r bsatn.Reader) (any, error) {
    id, _ := r.GetU64()
    name, _ := r.GetString()
    hp, err := r.GetU32()
    if err != nil {
        return nil, err
    }
    return &Player{ID: id, Name: name, HP: hp}, nil
}
func (playerDef) EncodeRow(row any) []byte {
    p := row.(*Player)
    w := bsatn.NewWriter(32)
    w.PutU64(p.ID)
    w.PutString(p.Name)
    w.PutU32(p.HP)
    return w.Bytes()
}
func (playerDef) PrimaryKey(row any) any { return row.(*Player).ID }

type createPlayerArgs struct{ Name string }

func (a *createPlayerArgs) WriteBsatn(w bsatn.Writer) { w.PutString(a.Name) }

func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()

    conn, err := client.NewDbConnection().
        WithUri("ws://localhost:3000").
        WithDatabaseName("my_game").
        OnConnect(func(c client.DbConnection, id types.Identity, token string) {
            fmt.Println("connected as", id)

            // Subscribe once connected; OnApplied fires after initial rows land.
            c.Subscribe("SELECT * FROM Player").
                OnApplied(func() {
                    tc := c.Cache().GetTable("Player")
                    fmt.Println("cached players:", tc.Count())
                    // Call a reducer (fire-and-forget).
                    _ = c.CallReducer("create_player", &createPlayerArgs{Name: "Alice"})
                }).
                OnError(func(err error) { log.Println("subscribe failed:", err) }).
                Build()
        }).
        OnReducerError(func(err error) { log.Println("reducer failed:", err) }).
        OnDisconnect(func(_ client.DbConnection, err error) {
            log.Println("disconnected:", err)
        }).
        Build(ctx)
    if err != nil {
        log.Fatal(err)
    }

    // Register tables BEFORE Run.
    conn.RegisterTable(playerDef{})

    // React to cache changes (callbacks run on the event-loop goroutine).
    tc := conn.Cache().GetTable("Player")
    tc.OnInsert(func(row any) {
        p := row.(*Player)
        fmt.Printf("new player: %s (HP=%d)\n", p.Name, p.HP)
    })
    tc.OnUpdate(func(oldRow, newRow any) {
        fmt.Println("player updated:", newRow.(*Player).Name)
    })

    // Blocks until ctx is cancelled or Disconnect is called.
    if err := conn.Run(ctx); err != nil {
        log.Println("run ended:", err)
    }
}
```

---

## Project Structure

```
myclient/
  go.mod                       # requires go.digitalxero.dev/spacetimedb-client (Go 1.25+)
  main.go                      # connection setup + event loop
  module_bindings/             # stdb-go generate client -d <db>
    types_generated.go         # type definitions
    bsatn_generated.go         # BSATN codecs
    tables_generated.go        # table defs + typed cache aliases
    reducers_generated.go      # Call<Reducer> functions
    procedures_generated.go    # Call<Procedure> functions (if module has procedures)
    views_generated.go         # view defs + typed cache aliases (if module has views)
    module_generated.go        # ModuleBindings aggregator
```
