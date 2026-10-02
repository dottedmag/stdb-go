---
name: stdb-go-cli
description: 'Use whenever running or scripting the stdb-go CLI tool — stdb-go init, stdb-go build, stdb-go publish, stdb-go dev, stdb-go generate (server or client), stdb-go upgrade, stdb-go skills, stdb-go version — or any task involving scaffolding SpacetimeDB Go projects, building SpacetimeDB Go modules to WASM (wasip1), publishing SpacetimeDB databases from Go, generating Go client bindings from a module schema, the spacetime.json project config, //go:generate go run github.com/dottedmag/stdb-go directives, or the dev watch/auto-publish loop. Trigger on any mention of the stdb-go CLI, its flags, its Taskfile tasks, or questions like "how do I build/publish/scaffold a SpacetimeDB Go module". Covers CLI usage only; for writing module code (//stdb: directives, tables, reducers) or client code, use the stdb-go-server and stdb-go-client skills.'
---

# stdb-go CLI

## Overview

`stdb-go` (`github.com/dottedmag/stdb-go`) is the command-line tool for SpacetimeDB Go development. It scaffolds projects, generates server glue code from `//stdb:` comment directives, compiles modules to WASM, publishes them to a SpacetimeDB server, generates typed Go client bindings, and runs a live-reload dev loop.

Running bare `stdb-go` with no subcommand runs server codegen (same as `stdb-go generate server`) for backwards compatibility — this is what the `//go:generate go run github.com/dottedmag/stdb-go` directive relies on.

| Command | Purpose |
|---------|---------|
| `stdb-go init <name>` | Scaffold a new server, client, or fullstack project |
| `stdb-go generate server` | Generate server code from `//stdb:` directives (same as bare `stdb-go`) |
| `stdb-go generate client` | Generate Go client bindings from a running server's schema |
| `stdb-go build` | Codegen + compile module to WASM (wasip1) + shim + optimize |
| `stdb-go publish` | Build and deploy the module to a SpacetimeDB server |
| `stdb-go dev` | Watch Go files; auto-rebuild and republish on change |
| `stdb-go skills --out <dir>` | Install the bundled AI-agent skills into a skills directory |
| `stdb-go upgrade` | Self-update from GitLab releases (alias: `self-update`) |
| `stdb-go version` | Print version and commit |

## Prerequisites

- **Go 1.25+** with the **standard toolchain**. Builds target `GOOS=wasip1 GOARCH=wasm` — do NOT use TinyGo; the tool invokes plain `go build` itself.
- **A running SpacetimeDB server** for `publish`, `dev`, and `generate client` (e.g. local Docker instance on `http://localhost:3000`). `init`, `build`, and `generate server` work offline.
- **wasm-opt** (Binaryen) is optional. `build` uses it when found on PATH and silently skips optimization otherwise, so missing wasm-opt is never an error.

## Command reference

### `stdb-go` (bare) / `stdb-go generate server`

Parses all Go packages under the module root (nested directories included) for `//stdb:` directives and writes generated registration, BSATN codec, table accessor, reducer dispatch, and module definition code.

| Flag | Default | Meaning |
|------|---------|---------|
| `--dir` | `.` | Directory containing Go source files to process |
| `--output` | `stdb_generated.go` | Output file name (honored only for flat single-package modules; multi-package modules always use per-package `stdb_generated.go` files) |

```bash
stdb-go generate server --dir=./mymodule
```

Prefer wiring this into the module via `//go:generate go run github.com/dottedmag/stdb-go` in `main.go` so `go generate ./...` regenerates it — that keeps codegen versioned with the module's `go.mod` rather than a globally installed binary.

### `stdb-go generate client`

Fetches the module schema from a **running SpacetimeDB server** (there is no offline/WASM-file mode) and writes type-safe Go client bindings.

| Flag | Default | Meaning |
|------|---------|---------|
| `-d, --database` | (required) | Database name or identity |
| `--out-dir` | `module_bindings` | Output directory for generated files |
| `-s, --server` | config or `http://localhost:3000` | SpacetimeDB server URL |
| `--token` | env / cli.toml | Auth token |
| `--package` | basename of `--out-dir` | Go package name for the bindings |
| `--schema-version` | `10` | Schema version for the server API |
| `--include-private` | `false` | Include private tables and reducers |

```bash
stdb-go generate client -d my-database --out-dir=./bindings --package=mymodule
```

Publish the module first — the bindings reflect whatever schema the server currently has, so stale deploys produce stale bindings.

### `stdb-go init`

Scaffolds a project. The name comes from the positional argument or `--name`.

| Flag | Default | Meaning |
|------|---------|---------|
| `--name` | — | Project name (alternative to positional arg) |
| `--module` | project name | Go module path for `go.mod` |
| `--type` | `server` | `server`, `client`, or `fullstack` |
| `--dir` | `./<name>` | Output directory |

```bash
stdb-go init myproject --type fullstack --module github.com/me/myproject
```

What each type generates:

- **server**: `go.mod` (requires `spacetimedb-server` and `spacetimedb-client` at the latest published versions), `main.go` (only the `//go:generate` directive and an empty `func main() {}` — the directive parser skips main.go), `types.go` + `reducers.go` starter files (a `User` table example plus the `//stdb:init` hook), `spacetime.json` (`{"name": ..., "edition": "1.0"}`), and a `Taskfile.yml` with `generate`, `build`, and `publish` tasks.
- **client**: `go.mod` (requires `spacetimedb-client`) and a `main.go` that connects to `ws://localhost:3000` and subscribes to the example table.
- **fullstack**: the server scaffold in `server/`, the client scaffold in `client/` (module path gets a `/client` suffix), plus a root `Taskfile.yml` orchestrating `server:generate`, `server:build`, `server:publish`, and `client:run`.

### `stdb-go build`

Compiles a module to WASM in four steps: run server codegen, `go mod tidy` + `go build` with `GOOS=wasip1 GOARCH=wasm -buildmode=c-shared -trimpath -tags=netgo,osusergo`, rewrite WASI Preview 1 imports with local stubs, then `wasm-opt -all -g -O2` if available.

| Flag | Default | Meaning |
|------|---------|---------|
| `--dir` | `.` | Module directory |
| `--output` | `module.wasm` | Output WASM path (relative paths resolve inside `--dir`) |
| `--optimize` | `true` | Run wasm-opt if available |
| `--release` | `true` | Strip debug info (`-ldflags="-s -w"`) |
| `--wasi-shim` | `true` | Rewrite WASI imports with local stubs |

```bash
stdb-go build --dir=./mymodule --output=mymodule.wasm
```

Keep `--wasi-shim=true` unless the target server provides host-side WASI stubs — SpacetimeDB does not implement WASI Preview 1, so an unshimmed standard-toolchain binary will fail to load. The build deliberately deletes any existing output first so `go build` cannot skip the write; re-shimming a previously shimmed binary is not idempotent and eventually corrupts it, so never re-run the shim over an already-built file yourself.

### `stdb-go publish`

Builds (unless `--skip-build`) and deploys the module. Reads defaults from `spacetime.json` in the module dir and `~/.config/spacetime/cli.toml`.

| Flag | Default | Meaning |
|------|---------|---------|
| `--dir` | `.` | Module directory |
| `-d, --database` | spacetime.json | Database name or identity (required from flag or config) |
| `-s, --server` | config or `http://localhost:3000` | Server URL |
| `--token` | env / cli.toml | Auth token |
| `--wasm-file` | — | Use a pre-built WASM file instead of building |
| `--skip-build` | `false` | Skip the build step (requires `--wasm-file`) |
| `--clear-database` | `false` | Destroy all data before publishing (alias for `--delete-data=always`) |
| `--delete-data` | `never` | When to destroy data on migration: `always`, `on-conflict`, `never` |
| `--break-clients` | `false` | Allow a migration that breaks existing clients (e.g. a new `default=` column) |
| `-y, --yes` | `false` | Auto-confirm breaking changes and major version upgrades |
| `--num-replicas` | `0` | Replica count (0 = server default) |
| `--parent` | — | Parent database (only applied when creating) |
| `--organization` / `--org` | — | Organization (only applied when creating) |
| `--wasi-shim` | `true` | Passed through to the build step |

```bash
stdb-go publish -d my-database -s https://spacetimedb.example.com
```

Migration behavior: before publishing to an existing database, a pre-publish check runs. Schema changes that break clients abort unless `--break-clients` or `--yes` is given; changes requiring manual migration abort unless `--delete-data=on-conflict` (or `--clear-database`) permits wiping data; major version upgrades require `--yes`. This is why a "failed" publish after a schema change usually just needs one of these flags — read the error message, it names the flag.

Auth is zero-setup: with no token from `--token`, `SPACETIMEDB_TOKEN`, or `cli.toml`, publish creates a new identity on the target server and saves its token to `~/.config/spacetime/cli.toml` so subsequent publishes reuse the same identity (important — the identity owns the database).

### `stdb-go dev`

Live-reload loop: watches Go files in the module dir, rebuilds and republishes on change. Start SpacetimeDB separately first (e.g. via Docker).

| Flag | Default | Meaning |
|------|---------|---------|
| `--dir` | `.` | Module directory |
| `-d, --database` | spacetime.json | Database name |
| `-s, --server` | config or `http://localhost:3000` | Server URL |
| `--token` | env / cli.toml | Auth token |
| `--debounce` | `500ms` | Debounce for file-change events |
| `--clear-database` | `true` | Clear database on the initial publish (note: opposite default from `publish`) |
| `--wasi-shim` | `true` | Rewrite WASI imports |
| `--client-cmd` | — | Command to run alongside (e.g. `"go run ./cmd/client"`), restarted with each publish |

```bash
stdb-go dev -d my-database --client-cmd="go run ./cmd/client"
```

`dev` clears the database by default because iterating on schema during development constantly hits migration conflicts; pass `--clear-database=false` to preserve data across reloads.

### `stdb-go skills`

Installs the AI-agent skills bundled with the CLI (including this one) into a skills directory.

```bash
stdb-go skills --out ~/.claude/skills
```

Existing files are overwritten, so re-run it after `stdb-go upgrade` to refresh the skills to match the installed CLI version.

### `stdb-go upgrade`

Self-updates from GitLab releases. Alias: `stdb-go self-update`. Uses `GITLAB_TOKEN` or `GL_TOKEN` from the environment if set (needed only for rate limits / private access).

| Flag | Default | Meaning |
|------|---------|---------|
| `-l, --list` | `false` | List available versions instead of upgrading |
| `-v, --version` | latest | Target version (e.g. `v0.2.0`) |

### `stdb-go version`

Prints `stdb-go version <version> (commit: <hash>)`.

## Configuration

Two config sources feed `publish`, `dev`, and `generate client`:

**`spacetime.json`** (in the module directory) — fields the CLI reads:

```json
{
  "database": "my-database",
  "server": "local",
  "module-path": ""
}
```

- `database`: default for `-d` (flag wins).
- `server`: a **nickname** looked up in `cli.toml`'s `server_configs` (not a URL). `-s` takes a full URL and wins.
- Note: `stdb-go init` writes `{"name": ..., "edition": "1.0"}` — the `name` field is NOT used as the database name. Add a `"database"` field yourself to skip passing `-d` every time.

**`~/.config/spacetime/cli.toml`** — shared with the official `spacetime` CLI: `spacetimedb_token`, `default_server`, and `server_configs` (nickname/host/protocol entries). stdb-go writes tokens here atomically and seeds `maincloud` + `local` entries when creating the file fresh.

Resolution order — server: flag > `spacetime.json` nickname > `cli.toml` `default_server` > `http://localhost:3000`. Token: flag > `SPACETIMEDB_TOKEN` env > `cli.toml`. Database: flag > `spacetime.json`.

## Typical workflows

**New server module, first deploy:**

```bash
stdb-go init myproject            # scaffold
cd myproject
# ... write tables in types.go, reducers in reducers.go ...
stdb-go publish -d myproject      # codegen + build + deploy in one step
```

`publish` runs codegen and build internally, so the explicit `generate`/`build` steps are only needed when you want the artifacts without deploying.

**Iterating (watch loop):**

```bash
docker run -p 3000:3000 clockworklabs/spacetime start   # server, separate terminal
stdb-go dev -d myproject --client-cmd="go run ./client"
```

**Client bindings after a deploy:**

```bash
stdb-go publish -d myproject
stdb-go generate client -d myproject --out-dir=./client/module_bindings
```

**Via go:generate** (module already has the directive): `go generate ./...` regenerates `stdb_generated.go` using the stdb-go version pinned in `go.mod`.

## Common mistakes

- Building with TinyGo or plain `go build` by hand — use `stdb-go build`; it sets `wasip1/wasm`, `-buildmode=c-shared`, and the WASI shim, all of which SpacetimeDB requires.
- Running the WASI shim (or `stdb-go build` pointing at an existing shimmed file with codegen skipped) twice over one binary — shimming is not idempotent; always let `build` produce a fresh binary.
- Expecting `generate client` to work offline or from a `.wasm` file — it only fetches the schema from a running server, so publish first.
- Expecting the scaffolded `spacetime.json` `name` field to supply the database name — the CLI reads `database`, so add that key or pass `-d`.
- Passing a URL in `spacetime.json`'s `server` field — it must be a `cli.toml` nickname; use `-s` for raw URLs.
- Treating a publish abort about breaking clients / manual migration as a hard failure — re-run with `--break-clients`, `--delete-data=on-conflict`, or `--yes` as the error suggests (deliberately, so data is never wiped silently).
- Using `--skip-build` without `--wasm-file` — publish rejects it because there would be nothing to deploy.
- Forgetting the server must already be running for `publish`/`dev` — stdb-go never starts SpacetimeDB itself.
