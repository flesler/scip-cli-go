# scip-cli-go

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Go implementation of [scip-cli](https://github.com/flesler/scip-cli) — fast code intelligence via SCIP indexes for TypeScript/JavaScript, Python, Go, and Rust.

Feature and CLI output parity with upstream **scip-cli 2.5.0** (Python reference). Cross-language integration tests compare stdout/stderr/exit codes against `scip-cli` on PATH.

## Why

AI agents waste tokens on grep and file scanning. scip-cli gives precise, type-aware navigation in milliseconds — and `analyze` surfaces dead code, cycles, and coupling from the SCIP index.

## Installation

```bash
go install github.com/flesler/scip-cli-go/v2/cmd/scip-cli@latest
scip-cli --version   # must show 2.5.0
```

(`@latest` → current release. Pin with `@v2.5.0` if you need an exact version.)

The installed binary is `scip-cli` — rename or symlink to `scip-cli-go` so you do **not** shadow Python `scip-cli` on PATH.

Or download a release binary from [GitHub Releases](https://github.com/flesler/scip-cli-go/releases).

**Prerequisites:** Node.js (`npx`) for TypeScript/Python; Go toolchain for Go projects; Rust toolchain for Rust projects. Indexers install on first use; the `scip` converter auto-downloads to `~/.cache/scip-cli/bin/`.

## For AI Agents

```bash
scip-cli skill ~/.claude/skills/scip-cli/
scip-cli skill   # dump SKILL.md to stdout
```

## Usage

```bash
scip-cli <command> [arguments]
```

| Command | Purpose |
|---------|---------|
| `refs` | Find references to a symbol |
| `code` | Definition + source snippet |
| `search` | Search symbols by pattern |
| `symbols` | List symbols in a file |
| `rdeps` | Reverse file dependencies |
| `deps` | Outbound dependencies |
| `members` | Class/interface members |
| `analyze` | SQL health dashboards |
| `reindex` | Force re-index |
| `skill` | Install or dump SKILL.md |

See `scip-cli skill` or upstream [README](https://github.com/flesler/scip-cli) for pipelines, analyze tiers, and `.scip-cli.json` options.

## Development

Requires Go 1.25+ at `~/.local/go-sdk/go` (install via [go.dev/dl](https://go.dev/dl/); do not rely on `/usr/bin/go`).

```bash
make setup          # bin/golangci-lint, bin/goimports, git hooks
make build          # bin/scip-cli-go
make test           # unit + e2e
make test-cross     # Python scip-cli vs bin/scip-cli-go output parity (needs npx + scip-cli)
make fmt            # gofmt + goimports (whole tree)
make lint           # go build, go vet, golangci-lint
make sync-upstream  # refresh fixtures/docs from upstream Python repo
```

Git hooks run on every commit (after `make setup`):

| Hook | Tool |
|------|------|
| Format | `gofmt`, `goimports` (auto-fix staged `.go` files) |
| Typecheck | `go build ./...`, `go vet ./...` |
| Lint | `golangci-lint` (config: `.golangci.yaml`) |
| Module hygiene | `go mod tidy` check on `go.mod` / `go.sum` |

All Go tools install to `./bin/` via `make tools` — nothing added to global PATH.

Manual full check (same as CI): `make pre-commit`.

Release: `scripts/publish.sh patch` (or `minor` / `major`) — test, version bump, tag, push, GitHub release, `go install` smoke test.

**Cross-language parity:** `internal/cross/cross_test.go` runs the same commands against Python `scip-cli` and `bin/scip-cli-go` on the shared `typescript-project` fixture.

### Supported projects

| Language | Markers | Indexer |
|----------|---------|---------|
| TypeScript/JS | `package.json`, `tsconfig.json` | `scip-typescript` (via `npx`) |
| Python | `pyproject.toml`, `setup.py` | `scip-python` (via `npx`) |
| Go | `go.mod` | `scip-go` (via `go install` → `~/go/bin`) |
| Rust | `Cargo.toml` | `rust-analyzer` (via `rustup component add`) |

On first index, missing tools are fetched automatically (`npx` for TS/Python, `go install` for Go, `rustup` for Rust). The `scip` converter auto-downloads from [GitHub releases](https://github.com/scip-code/scip/releases) when absent.

Optional manual install:

```bash
npm install -g @sourcegraph/scip-typescript
npm install -g @sourcegraph/scip-python
go install github.com/scip-code/scip-go/cmd/scip-go@latest
rustup component add rust-analyzer
```

`.scip-cli.json` `maxHeapMb` tunes Node heap for `scip-typescript` / `scip-python` only (not `scip-go` or `rust-analyzer`).

`SCIP_CLI_TS_INDEX_BATCH_SIZE` splits large TS monorepos into multiple `scip-typescript` runs (default: all tsconfigs in one run). `SCIP_CLI_INDEX_WORKERS` controls parallel batch runs (default: up to 8).

### Python / `test-cross`

This repo is Go-only for build and hooks. Python appears only for **cross-comparison tests** (`make test-cross`), which diff output against the reference [scip-cli](https://github.com/flesler/scip-cli) install (`scip-cli` on PATH, or `../scip-cli/.venv/bin/scip-cli` from a sibling checkout). That venv lives in the Python repo, not here.

## License

MIT
