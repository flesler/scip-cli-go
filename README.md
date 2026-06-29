# scip-cli-go

Port of [scip-cli](https://github.com/flesler/scip-cli) to Go.

## Development

Requires Go 1.25+ (`~/.local/go-sdk/go/bin` or similar — `/usr/bin/go` is often too old to read `go.mod`).

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

**Do not** install this binary as `scip-cli` on PATH; the Python package keeps that name.

### Python / `test-cross`

This repo is Go-only for build and hooks. Python appears only for **cross-comparison tests** (`make test-cross`), which diff output against the reference [scip-cli](https://github.com/flesler/scip-cli) install (`scip-cli` on PATH, or `../scip-cli/.venv/bin/scip-cli` from a sibling checkout). That venv lives in the Python repo, not here.
