# scip-cli-go

Port of [scip-cli](https://github.com/flesler/scip-cli) to Go.

## Development

Requires Go 1.25+ and Python 3 (for pre-commit only — kept in a repo-local venv).

```bash
make setup          # bin/golangci-lint, bin/goimports, .venv, git hooks
make build          # bin/scip-cli-go
make test           # unit + e2e
make fmt            # gofmt + goimports (whole tree)
make lint           # go build, go vet, golangci-lint
```

Pre-commit runs on every commit (after `make setup`):

| Hook | Tool |
|------|------|
| Format | `gofmt`, `goimports` (auto-fix staged `.go` files) |
| Typecheck | `go build ./...`, `go vet ./...` |
| Lint | `golangci-lint` (config: `.golangci.yaml`) |
| Module hygiene | `go mod tidy` check on `go.mod` / `go.sum` |

All Go tools install to `./bin/` via `make tools` — nothing added to global PATH. Pre-commit lives in `./.venv/`.

Manual hook run: `make pre-commit` or `.venv/bin/pre-commit run --all-files`.

**Do not** install this binary as `scip-cli` on PATH; the Python package keeps that name.
