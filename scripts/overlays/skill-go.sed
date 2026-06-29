# Go-port overlays applied after copying scip_cli/SKILL.md from upstream.
# Keep this small: upstream owns flags, commands, and agent workflows.

s#\*\*Contributors:\*\* keep `pip install -e .`.*#**Contributors:** `make setup` (pre-commit + lint tools), `make build` → `./bin/scip-cli-go`. Do **not** replace `scip-cli` on PATH.#
s/`scip_cli`, `src\/pkg\/`/`internal\/`, `src\/pkg\/`/
s/analyze scip_cli` or `analyze scip_cli\/queries.py`/analyze internal\/queries` or `analyze internal\/commands\/code.go`/
