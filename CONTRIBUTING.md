# Contributing

## Setup

```bash
make build
make test
```

Requires Go 1.25+, Node.js for e2e tests.

## Pull requests

1. Fork and branch from `main`
2. `make fmt lint test`
3. If upstream [scip-cli](https://github.com/flesler/scip-cli) changed CLI flags or agent docs, run `make sync-upstream` (or edit `scripts/overlays/` for Go-only SKILL lines)
4. Open a PR with a clear description and test plan

## Code style

- `gofmt` / `goimports` via `make fmt`
- `go vet` + `golangci-lint` via `make lint`
- Match existing patterns in `internal/` — thin commands, SQL in `queries` / `analyze`

## Reporting issues

Include OS, `scip-cli --version`, repro steps, and expected vs actual output.
