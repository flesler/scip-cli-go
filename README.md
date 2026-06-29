# scip-cli-go

Port of [scip-cli](https://github.com/flesler/scip-cli) to Go.

## Development

Requires Go 1.25+ at `~/.local/go-sdk/go` (install via [go.dev/dl](https://go.dev/dl/); do not rely on `/usr/bin/go`).

```bash
make setup          # bin/golangci-lint, bin/goimports, git hooks
make build          # bin/scip-cli-go
make test           # unit + e2e
make test-cross     # Python scip-cli vs bin/scip-cli-go output parity (needs npx + ../scip-cli)
make fmt            # gofmt + goimports (whole tree)
make lint           # go build, go vet, golangci-lint
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

**Do not** install this binary as `scip-cli` on PATH; the Python package keeps that name.

### Install (users)

```bash
go install github.com/flesler/scip-cli-go/v2/cmd/scip-cli@latest
```

(`@latest` → current release. Pin with `@v2.5.0` if you need an exact version.)

The installed binary is `scip-cli` — rename or symlink to `scip-cli-go` if you want to avoid clashing with Python `scip-cli`.

Or download a release binary from [GitHub Releases](https://github.com/flesler/scip-cli-go/releases).

### Supported projects

Auto-detects language from project markers:

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
