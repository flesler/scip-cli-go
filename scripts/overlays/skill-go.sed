# Go-port overlays applied after copying scip_cli/SKILL.md from upstream.
# Keep this small: upstream owns flags, commands, and agent workflows.

s/TS\/JS, Python, Go, or Rust/TS\/JS, Python, or Go/
s/Go (.go), and Rust (.rs)/Go (.go)/
s#\*\*Contributors:\*\* keep `pip install -e .`.*#**Contributors:** `make setup` (pre-commit + lint tools), `make build` → `./bin/scip-cli-go`. Do **not** replace `scip-cli` on PATH.#
s/Go toolchain (for Go via `go install`), or Rust toolchain (for Rust via `rustup`)/Go toolchain (for Go via `go install`)/
s/; `rust-analyzer` installs via `rustup component add`//
s/heap tuning\./heap tuning (`maxHeapMb` applies to Node indexers only, not `scip-go`)./
s/`scip_cli`, `src\/pkg\/`/`internal\/`, `src\/pkg\/`/
s/analyze scip_cli` or `analyze scip_cli\/queries.py`/analyze internal\/queries` or `analyze internal\/commands\/code.go`/
