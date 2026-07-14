# Migration Problems & Lessons Learned

## Historical Issues (from initial migration attempts)

### 1. Package Naming Conflict: `sql` vs `sqlhelp`
**Problem**: Directory is `internal/sql` but Go package name is `sqlhelp` to avoid conflict with stdlib `database/sql`.
**Time Lost**: ~30 minutes
**Lesson**: When creating the directory structure, the package name in the file must match what other files import. Files were importing `internal/sqlhelp` (wrong path) when they should import `internal/sql` (correct path) and use `sqlhelp.X` in code.

**Fix Applied**: Changed all imports from `internal/sqlhelp` to `internal/sql` and added alias `sqlhelp "github.com/sourcegraph/scip-cli-go/internal/sql"` where needed.

### 2. SQLite Driver Confusion
**Problem**: `indexing.go` imported `github.com/mattn/go-sqlite3` (CGO-based) while `merge.go` imported `modernc.org/sqlite` (pure Go).
**Time Lost**: ~15 minutes
**Lesson**: The project uses `modernc.org/sqlite` (pure Go, no CGO). All files must use the same driver.

**Fix Applied**: Changed `indexing.go` to use `modernc.org/sqlite`.

### 3. Missing Subpackages in Analyze
**Problem**: `commands/analyze.go` imported non-existent subpackages `internal/analyze/sections` and `internal/analyze/targets`.
**Time Lost**: ~20 minutes
**Lesson**: The analyze functionality was split across multiple Python files (`sections.py`, `targets.py`) but in Go these should be part of the main `analyze` package, not separate subpackages.

**Fix Applied**: Removed subpackage imports, moved all types/functions to main `analyze` package, updated `commands/analyze.go` to use `analyze.Section`, `analyze.ParsePriorities`, etc.

### 4. Subagent Rate Limiting
**Problem**: Got throttled by API rate limits when running multiple subagents in parallel.
**Time Lost**: ~10 minutes waiting
**Lesson**: Don't spawn too many subagents simultaneously. Sequential work is more reliable when rate-limited.

### 5. Duplicate Function Definitions
**Problem**: `extractLeafName` was defined in both `source.go` and needed in `analyze/common.go`.
**Time Lost**: ~10 minutes
**Lesson**: Check for duplicate function names across packages. Use package prefixes or rename to avoid conflicts.

**Fix Applied**: Renamed duplicate to `extractLeafNameFromSymbol` in one location.

### 6. Missing `ResolveDefLocation` Implementation
**Problem**: `source.go` had a stub for `ResolveDefLocation` that didn't match the signature needed by other files.
**Time Lost**: ~15 minutes
**Lesson**: When porting, ensure function signatures match across all call sites.

**Fix Applied**: Updated signature to return `(path string, startLine, endLine int, err error)` and implemented properly.

## Ongoing Issues

### 7. Go Module Dependencies Not in go.mod
**Problem**: `modernc.org/sqlite` not added to `go.mod` yet.
**Status**: Need to run `go get modernc.org/sqlite`

### 8. Missing Functions in Analyze Package
**Problem**: `ParsePriorities`, `NewRowBudget`, `RunProjectSections`, `RunDirSections`, `RunFileSections`, `RunSymbolSections` not yet implemented in `analyze` package.
**Status**: Need to port from Python `sections.py` and add to `project.go` or create new files.

### 9. File and Symbol Analyze Checks Missing
**Problem**: `file.go` and `symbol.go` analyze checks not yet ported.
**Status**: Need to create these files with per-file and per-symbol analysis functions.

### 10. CLI Entry Point Not Created
**Problem**: `cmd/scip-cli/main.go` doesn't exist yet.
**Status**: Need to create main entry point with subcommand dispatch.

### 11. Test Fixtures Not Copied
**Problem**: Test fixtures from Python project not copied to Go project.
**Status**: Need to copy `tests/fixtures/` and adapt test files.

## Key Takeaways

1. **Package naming matters**: Directory name ≠ package name in Go. Be explicit about both.
2. **Use consistent dependencies**: Don't mix CGO and pure-Go SQLite drivers.
3. **Check imports early**: Run `go build` frequently to catch import errors.
4. **Subagent coordination**: When rate-limited, sequential work is more efficient.
5. **Function signatures**: Ensure all call sites agree on signatures before implementing.
6. **Document as you go**: This file should be updated continuously, not at the end.

## Session 2: Continuing compilation fixes (2026-06-28)

### 11. `LimitAndWarn` type mismatch
**Problem**: `output.LimitAndWarn` took `[]interface{}` but callers passed typed slices (`[]queries.SymbolResult`, `[]string`, `[]searchResult`, etc.).
**Time Lost**: ~10 minutes
**Fix**: Changed to generic `LimitAndWarn[T any](items []T, limit int, label string) []T`.

### 12. Missing analyze wrapper functions
**Problem**: `commands/analyze.go` called `analyze.ParsePriorities`, `analyze.NewRowBudget`, `analyze.RunProjectSections`, `analyze.RunDirSections`, `analyze.RunFileSections`, `analyze.RunSymbolSections` — none existed.
**Time Lost**: ~15 minutes
**Fix**: Added all to `project.go`. `RunFileSections` and `RunSymbolSections` use stub check functions (TODO).

### 13. `code.go` type mismatches with output package
**Problem**: `ResolveMaxDefLines` takes `*int` not `int`; `FormatLineRange` takes `*int` not `int`; `endLine - startLine` on `*int` pointers.
**Time Lost**: ~10 minutes
**Status**: Need to fix `code.go` to match output package signatures.

### 14. `deps.go` wrong import alias
**Problem**: Imported `"github.com/sourcegraph/scip-cli-go/internal/sql"` but used `sqlhelp.` prefix without alias.
**Time Lost**: ~5 minutes
**Fix**: Need to add alias `sqlhelp "github.com/sourcegraph/scip-cli-go/internal/sql"`.

### 15. Stub functions for file/symbol analyze checks
**Problem**: `unusedImports`, `deadInFile`, `unreferencedInFile`, `symbolRefs`, `symbolDeps` don't exist yet.
**Time Lost**: ~5 minutes
**Fix**: Added stub implementations returning "(not yet implemented)". These need real SQL later.

### Key pattern: Too many cross-file API mismatches
**Root cause**: Files were ported independently without checking call sites. Each command file assumed slightly different signatures from output/analyze packages.
**Lesson**: When porting, grep for all call sites of a function BEFORE writing the implementation to get the signature right.

### 16. sql vs sqlhelp flip-flop (FIXED)
**Problem**: Package lived in `internal/sql` but was named `sqlhelp` to avoid clashing with stdlib `database/sql`. Some files imported with alias, some without, some used `sql.` when they meant `sqlhelp.`.
**Time Lost**: ~45+ minutes across sessions
**Root cause**: Directory name != package name is a footgun in Go.
**Fix**: Renamed directory to `internal/sqlhelp`. Import is always `"github.com/sourcegraph/scip-cli-go/internal/sqlhelp"` — no alias needed. Never flip again.

### 17. Ported commands assumed wrong APIs
**Problem**: `reindex.go` used `project.PYTHON` (actual: `LanguagePython`), `indexing.IndexProject` (actual: `indexing.Reindex`), ignored `NormalizePathScope` error return. `search.go` shadowed `symbols` package with a param name. `code.go`/`deps.go` did arithmetic on `*int` without deref.
**Time Lost**: ~20 minutes
**Lesson**: `go build ./...` after each package, not after the whole migration.

### 18. ESCAPE in SQL: backticks vs double-quotes (modernc.org/sqlite)
**Problem**: Python uses `ESCAPE '\\'` in SQL strings. In Go **backtick** raw strings, `\\` is two literal backslashes → SQLite error "ESCAPE expression must be a single character".
**Time Lost**: ~40 minutes
**Fix**: Backtick SQL → `ESCAPE '\'`; double-quoted Go strings → `ESCAPE '\\'` (one backslash in SQL).

### 19. CLI flag parser stopped at first positional
**Problem**: `search greet --limit 3` treated `--limit` as a search pattern because Parse() did `append(argv[i:]...); break` on first non-flag.
**Time Lost**: ~15 minutes
**Fix**: Append one positional at a time and continue parsing flags.

### 20. Binary name
**Decision**: Build to `bin/scip-cli-go`, version string `scip-cli-go`. Do NOT `go install` as `scip-cli` — Python keeps that name on PATH.

## Session 3: Finish missing migration pieces (2026-06-28)

### 21. Duplicate analyze stubs in project.go
**Problem**: `file.go` / `symbol.go` added real implementations but `project.go` still had stub checks and duplicate `RunFileSections` / `RunSymbolSections`.
**Fix**: Removed stubs; `RunDirSections` runs project checks + per-file batch (`MaxDirFiles=30`) with stderr notes.

### 22. source.go overwrote ReadSourceLines
**Problem**: Porting `ResolveDefLocation` replaced the whole file, breaking `code`/`refs`/`members`.
**Fix**: Restored `ReadSourceLines`, path cache, traversal guard; added `source_test.go`.

### 23. live.go filesystemDirScope
**Problem**: Used `filepath.Glob` on an absolute path instead of `os.Stat` + `IsDir`.
**Fix**: Stat candidate directory under project root.

### 24. ParsePriorities missing aliases
**Problem**: Python accepts `1/2/3`, `h/m/l`; Go only accepted full names.
**Fix**: Added alias map in `ParsePriorities`.

### 25. Test helper name collision
**Problem**: `patterns_e2e_test.go` defined `contains` — clashes with `graph.go`.
**Fix**: Renamed to `linesContain`.

### 26. Fixtures + tests added
- `testdata/fixtures/sample-project/`
- `internal/analyze/graph_test.go` + `testdb/builder.go`
- `internal/analyze/patterns_e2e_test.go`
- Extended `internal/e2e` (analyze, deps, members)
- `.golangci.yaml`, `.github/workflows/ci.yml`

## Session 4: Tests + search parity (2026-06-28)

### 27. Ported pytest coverage to Go
- `internal/analyze/dashboard_test.go`, `common_test.go`, `perf_test.go`
- `internal/analyze/testdb/mini.go` (mini_codebase_db)
- `internal/targets`, `config`, `cache`, `merge`, `scip` unit tests
- Extended e2e: qualified search, multi-symbol code/refs, deps paths-only, index counts

### 28. symbolPressure Scan bug
**Problem**: `fetchOneRow` aliases `FetchOne` (1 column) but `symbolPressure`/`defContext` SELECT 4–5 columns.
**Fix**: Use `FetchOneRow(db, ncols, ...)` in `symbol.go`.

### 29. search qualified patterns
**Problem**: `Options.verbose` went to SQL LIKE instead of `resolve_symbol` (Python `_qualified_pattern`).
**Fix**: `qualifiedPattern()` + `ResolveDefLocation` in search output for line numbers.

## Session 5: Tooling + test parity (2026-06-28)

### 30. Malformed JSON in tool calls
**Problem**: Previous tool call had broken JSON structure — missing closing braces and incomplete parameters.
**Time Lost**: ~5 minutes debugging
**Lesson**: Always validate JSON structure before submitting tool calls. Use proper escaping for multi-line strings.

### 31. Cross-comparison test harness incomplete
**Problem**: Created `internal/cross/cross_test.go` but didn't finish integrating it into Makefile or verify it runs.
**Status**: Need to add `make test-cross` target and verify both Python and Go binaries are indexed before comparison.

### 32. Missing Python test ports
**Problem**: Still haven't ported all Python tests:
- `test_analyze.py` (most sections covered but some gaps)
- `test_multi_symbol.py` (partially in e2e but not all cases)
- `test_pure_functions.py` (many cases scattered across unit tests)
- `test_index_prune.py` (partially covered in indexing_test.go)
- `test_reindex.py` (scope clearing covered, Python path rejection covered)
**Status**: Need systematic pass through each Python test file and ensure 1:1 coverage.

### 33. Pre-commit hook not tested
**Problem**: Added `.pre-commit-config.yaml` but didn't verify it actually runs or what errors it catches.
**Status**: Replaced Python `pre-commit` + `.venv` with `scripts/hooks/pre-commit` (bash). Root cause of failed commits: hook fell back to broken `~/.local/bin/pre-commit` when `.venv` missing. Go repo should not require Python for hooks.

### 34. golangci-lint not verified
**Problem**: Updated `.golangci.yaml` and Makefile but didn't run `make lint` to see if there are existing violations.
**Status**: Need to run lint and fix any violations before declaring tooling complete.

### 35. Go toolchain version mismatch (2026-06-28)
**Problem**: System had Go 1.18.1; project needs Go 1.25+. Separate from §46: `go` directive must be `go 1.25` (no patch), not `go 1.25.0`.
**Time Lost**: ~15 minutes
**Fix**: Installed Go 1.25.0 to `/home/flesler/.local/go-sdk/go/`. Use `export PATH="/home/flesler/.local/go-sdk/go/bin:$PATH"` before running Go commands.

### 36. Cross-comparison test harness build failure (2026-06-28)
**Problem**: `internal/cross/cross_test.go` expected `bin/scip-cli-go` to exist but tests don't build it first.
**Time Lost**: ~5 minutes
**Fix**: Added `go build` step in `TestMain` to build the binary before running cross tests.

### 37. Output format mismatch between Python and Go (2026-06-28)
**Problem**:
- `code` command: Python uses `:line:column` format, Go used `:line-column`
- `symbols` command: Python uses `-` separator, Go used `:`
- `refs` command: Ordering mismatch (Go used database docID order, Python uses alphabetical path order)
**Time Lost**: ~20 minutes
**Fix**:
- Changed `code.go` to use `:` separator (matching Python)
- Changed `symbols.go` to use `-` separator (matching Python)
- Fixed `refs.go` to sort by path alphabetically instead of database docID
- Fixed chunk grouping bug where all chunks were assigned to every document

### 38. Stale variable references after refactoring (2026-06-28)
**Problem**: After refactoring `getExactRefs` to fix chunk grouping, left references to undefined `chunks` variable.
**Time Lost**: ~5 minutes
**Fix**: Changed references from `chunks` to `totalChunks` variable.

## Session 6: Complete test port + WarnAmbiguousRefs (2026-06-28)

### 39. Missing `WarnAmbiguousRefs` in Go refs command
**Problem**: Python `refs` prints `Ambiguous symbol 'Opts' ... ext_refs=N` when a name resolves to multiple definitions; Go only had generic `WarnAmbiguous` (used by members/analyze), so `test_e2e_analyze_patterns.TestRefsAmbiguityE2E` had no Go equivalent.
**Time Lost**: ~15 minutes discovering during test port
**Fix**: Added `output.WarnAmbiguousRefs` using `queries.SymbolExternalRefCount`; call from `RefsMain` after symbol resolution.

### 41. `code` command skipped `ResolveDefLocation` fallback
**Problem**: Type literal fields like `Options.verbose` have no `defn_enclosing_ranges` row; Python uses `resolve_def_location` with source-file scan fallback. Go `code` called `queries.GetDefLocation` only → `Warning: no definition location`.
**Time Lost**: ~10 minutes during e2e test port
**Fix**: Use `source.ResolveDefLocation` in `code.go`.

### 40. Remaining Python test gaps ported
**Status**: Added Go tests mirroring `test_analyze.py` (hotspots scope, unreferenced, 9 sections, preface, same-file helper, dynamic-load skip, cycles type/runtime, coupling, def_context, dashboard runner noise), `test_merge.py` (duplicate chunks, preserve mentions), `test_index_prune.py` (log_index_complete), `test_scope.py` / `test_typescript_projects.py` (scope-filtered projects), `test_composability.py` (refs paths dedup), `test_pure_functions.py` gaps (FormatDefBody, ResolveFile/Symbol), `test_discover.py` (nested service, tsconfig variant), `test_cache.py` (stable cache dir), `test_e2e.py` / `test_multi_symbol.py` / `test_e2e_analyze_patterns.py` e2e cases.

## Session 7: Audit noise — why so many "very bad" findings? (2026-06-28)

### 42. Full-codebase audit inflated severity — root cause analysis
**What happened**: A readonly subagent audit returned ~15 items labeled "fix now", including several that were latent API gaps, intentional Python parity, or test-coverage holes rather than production bugs.

**Why it happened (mix of all three factors)**:

1. **Methodology (primary)**
   - Prompt asked for "really bad" issues with a 15-item cap → reviewer ranked *relative* severity inside the repo, not absolute user impact.
   - No "ignore list" on round 1, so fixed items from the prior session could be re-discovered conceptually.
   - Subagent compared Go to Python *spec* without checking whether Python shared the behavior (e.g. partial monorepo index promotion).
   - `/iterate` default rewards breadth (15 findings) over precision.

2. **Go / porting mechanics (secondary)**
   - Real bugs from porting style: string suffix checks without length guards (`deps` panic), `rows.Scan` ignored, map iteration order (`refs`/`deps --paths-only`), unused constants (`IndexTimeout`).
   - Go has no argparse `choices=` — invalid `--kind` slipped through.
   - Duplicate helpers (`symbols.EscapeLike` vs `sqlhelp.EscapeLike`) diverged silently.
   - `copyFile` via `ReadFile` is a common Go footgun for large binaries.

3. **Skill / process (secondary)**
   - Large batch port without per-package `go test` let API mismatches accumulate (documented in §17).
   - Cross tests initially indexed only via Python → real gap, but reads as "broken" rather than "missing test".
   - Agent skill says strive for parity but doesn't say "audit subagent findings are hypotheses — verify before fix".

**What was actually fix-now vs overstated**:

| Finding | Verdict |
|---------|---------|
| `deps` panic on short names | **Real** — fixed |
| Missing index timeouts | **Real** — fixed (`indexing` + `scip`) |
| Invalid `--kind` silent pass | **Real** — fixed |
| `refs` Scan ignored | **Real** — fixed |
| Cross tests Python-only index | **Real test gap** — fixed (dual `pySession` + `goSession`) |
| `copyFile` OOM | **Real at scale** — fixed (`io.Copy`) |
| `GetImporterPaths` extra arg | **Real bug** — fixed |
| `FindDB` / `GetDB` root resolution | **Real latent bug** — fixed |
| `EscapeLike` backslash drift | **Real parity** — fixed (match Python) |
| SQLite error message at CLI | **UX parity** — fixed |
| Partial index on skipped tsconfigs | **Intentional parity with Python** — not changed |
| "Cross missing flag X" | **Test gap** — expanded cases, not product bug |

**Lessons**:
- Treat subagent audits as a **backlog to verify**, not a punch list.
- Run `make test-cross` with **both** index builders before declaring indexing done.
- Prefer shared helpers (`targets.LooksLikeFileTarget`, `sqlhelp.EscapeLike`) over reimplemented Python one-liners.
- For `/iterate`, pass an ignore list on round 2+ and cap "fix now" at verified failures.

## Session 8: Audit round 2 — verify-first fixes (2026-06-28)

### 43. Round 2 slowdowns (language, tooling, process)
**What slowed the second pass**:

| Area | Issue |
|------|--------|
| **Error handling split** | `session.Setup()` and `reindex` call `os.Exit` with plain `Error:`; only `main.exitWithError` maps SQLite to `Database error:`. Easy to miss when auditing CLI UX parity. |
| **Go-only footgun** | `bufio.Scanner` default 64KB max line → `ReadSourceLines` returned `(nil, nil)` on long lines; Python `readlines()` has no such cap. |
| **SQLite DDL** | Index `postprocessIndex` ran trim DDL outside a transaction — crash mid-trim can leave `index.db.next` half-migrated. |
| **Ignored `Scan`** | `deps` still `continue` on `Scan` error after round 1 fixed `refs` — same porting pattern, different file. |
| **Map iteration** | `search --paths-only` collected paths in map iteration order; `refs`/`deps` already sorted — inconsistent within one codebase. |
| **No argparse `choices`** | `analyze --priority` silently dropped unknown tokens; Python `parse_priorities` raises. |
| **Silent SQL partials** | `getExactRefs` broke on `Query` error without stderr warning; partial ref lists looked like success. |
| **Dead code drift** | `RunScipIndexer` unused while `indexerEnv` is live — NODE_OPTIONS merge diverged unnoticed. |
| **Subagent round 2** | 10 items after ignore list; still needed file-by-file verify (e.g. members 500 cap matches Python SQL, warning is enhancement). |

**Lessons**: Extract shared `clierr.Fatal` before adding more `os.Exit` call sites; prefer `os.ReadFile` + split for source parity with Python; wrap index postprocess in `BEGIN`/`COMMIT`; extend cross-test ignore list each audit round.

## Session 9: Dev tooling — pre-commit, git identity, go.mod (2026-06-28)

### 44. Python `.venv` in a Go repo (pre-commit)
**Problem**: `make setup` created `scip-cli-go/.venv` solely to run the Python `pre-commit` package. README said "Python 3 for pre-commit only." Confusing next to `./bin/` Go tools and unrelated to `../scip-cli/.venv` (sibling Python CLI for `test-cross`).
**Root cause of failed commits**: Generated `.git/hooks/pre-commit` pointed at `.venv/bin/python3`; when `.venv` missing, hook fell back to `~/.local/bin/pre-commit` → `ModuleNotFoundError: No module named 'pre_commit'`.
**Fix**: Removed `.pre-commit-config.yaml`, `requirements-dev.txt`, venv from `make setup`. Added `scripts/hooks/pre-commit` (bash); `make setup` sets `core.hooksPath=scripts/hooks`. `make pre-commit` → `make fmt-check lint` + mod tidy check.
**Lesson**: Go repos don't need a local venv for hooks when checks are already shell scripts + `./bin` tools.

### 45. Cursor agent shell has no `~/.gitconfig`
**Problem**: Agent `git commit` failed with "Author identity unknown" despite developer's real `~/.gitconfig` at `/home/flesler/.gitconfig`. Agent `$HOME` is isolated (`/tmp/.../home`) with no `.gitconfig`.
**Mistake**: Agent invented `flesler@users.noreply.github.com` instead of reading real config (`Ariel Flesler` / `aflesler@gmail.com`).
**Fix**: `scripts/dev-env.sh` sets `GIT_CONFIG_GLOBAL=/home/$USER/.gitconfig` when `~/.gitconfig` is absent. Hooks and `make` targets source `dev-env.sh`.
**Lesson**: Agent commits must source `dev-env.sh` or set `GIT_CONFIG_GLOBAL`; never guess git identity.

### 46. `go.mod` patch version in `go` directive
**Problem**: `go 1.25.0` in `go.mod` → `invalid go version '1.25.0': must match format 1.23`. Broke `go build` inside pre-commit hook; surfaced when user committed from their shell.
**Confusion**: §35 blamed "requires 1.25.0" vs system Go 1.18 — that was a different issue (toolchain install). The commit failure was **format**: `go` directive is major.minor only (`go 1.25`), not semver.
**Fix**: `go 1.25` in `go.mod`. README still says "Go 1.25+" for the toolchain.
**Lesson**: After bumping toolchain, use `go 1.25` not `go 1.25.0`; run `go build` before declaring hooks green.

## Session 10: Multiple Go installs — PATH confusion (2026-06-28)

### 47. Three Go toolchains on one machine
**Symptom**: `go version` → `go1.18.1` in user terminal despite installing Go "today".

**Inventory (before cleanup)**:

| Location | Version | Role |
|----------|---------|------|
| `/usr/bin/go` | 1.18.1 | Ubuntu `golang-go` / `golang-1.18-go` apt packages (2022) |
| `~/local/go` | 1.26.4 | Duplicate SDK tree (identical binary to go-sdk); **wrong bashrc path** `$HOME/local/go/bin` |
| `~/.local/go-sdk/go` | 1.26.4 | **Canonical** user SDK (installed from go.dev tarball) |

**Why 1.18 won**: Login `PATH` had no `~/.local/go-sdk/go/bin`. `~/.bashrc` appended dead `$HOME/local/go/bin` at the **end** of PATH. `/usr/bin/go` was first usable `go`.

**Cleanup done**:
- Installed **Go 1.26.4** to `~/.local/go-sdk/go` (replaced 1.25.0).
- Fixed `~/.bashrc`: `export PATH="$HOME/.local/go-sdk/go/bin:$PATH:..."` (removed `local/go`).
- **Deleted** `~/local/go` (167MB duplicate; same md5 as go-sdk).
- `scripts/dev-env.sh`: single canonical `GOSDK` default, dropped hardcoded `/home/flesler/...`.

**Still on disk (needs manual sudo)**:
```bash
sudo apt remove golang-go golang-1.18-go golang-1.18-src
```
Harmless if `~/.bashrc` keeps go-sdk first; remove apt packages to stop `type -a go` listing `/usr/bin/go`.

**Lesson**: One SDK dir (`~/.local/go-sdk/go`), prepend to PATH in bashrc, never `$HOME/local/go`. Project `dev-env.sh` is a safety net, not a substitute for shell config.

## 48. `go install` @v2.3.1 fails — module path must include `/v2`

**Symptom** (after renaming module to `github.com/flesler/scip-cli-go` and tagging v2.3.1):

```text
go install github.com/flesler/scip-cli-go/cmd/scip-cli@v2.3.1
invalid version: module contains a go.mod file, so module path must match major version ("github.com/flesler/scip-cli-go/v2")
```

**Cause**: Go **semantic import versioning** (SIV). Any git tag with major version ≥ 2 (`v2.3.0`, `v2.3.1`, …) requires the `go.mod` `module` line and every internal import to use the **`/v2` suffix**. Tags alone are not enough; the module path must match the tag’s major version.

**Why you cannot “just override” v2.3.1**: There is no `go.mod` flag or release tweak to bypass this. Options are only:

| Approach | `go.mod` module | Install path | Git tags |
|----------|-----------------|--------------|----------|
| **A (correct for v2.x)** | `github.com/flesler/scip-cli-go/v2` | `.../v2/cmd/scip-cli@v2.3.1` | Keep `v2.3.x` |
| **B** | `github.com/flesler/scip-cli-go` (no suffix) | `.../cmd/scip-cli@v1.3.1` | Tags must stay **v0.x or v1.x** |

CLI `--version` string (e.g. `2.3.0`) is independent of module/tag semver; only the **git tag major** drives `/v2`.

**Fix applied**: Option A — module + imports → `/v2`, delete broken GitHub release/tag `v2.3.1`, re-tag same version after fix.

**Lesson**: Before first `v2.0.0` tag, decide: either add `/v2` to module path, or keep tags at v1.x until you do.

## 49. `source scripts/dev-env.sh` hangs for minutes in agent shells [L385-394]
## 50. Pre-commit hook missed golangci-lint v2 incompatibility (2026-07-14) [L394-425]
 ### Root cause: Version mismatch between local and CI tooling [L396-402]
 ### Why it happened [L402-415]
 ### Fix applied [L415-425]

**Symptom**: One-liners like `source dev-env.sh && sed … && make test` sit with no output for 3+ minutes; file edits never run.

**Cause**: `dev-env.sh` used to run `make tools` when `bin/golangci-lint` or `bin/goimports` was missing. `make tools` downloads and builds both — fine for `make setup`, catastrophic when sourced before every quick command.

**Fix**: Removed auto-`make tools` from `dev-env.sh`. Run `make setup` once for hooks and `./bin` tools.

**Lesson**: `dev-env.sh` sets PATH only (Go SDK + `./bin`). No network, no builds.

## 50. Pre-commit hook missed golangci-lint v2 incompatibility (2026-07-14) [L396-427]
 ### Root cause: Version mismatch between local and CI tooling [L398-404]
 The pre-commit hooks call `golangci-lint run` directly from PATH without version validation. Local developers may have:
 - An older/newer version installed globally
 - No golangci-lint installed at all
 - A version that doesn't match what CI pins in `.github/workflows/ci.yml`

 The Makefile's `GOLANGCI_LINT_VERSION` variable only gates the `make tools` target, not every commit. There's no enforcement that the local binary matches CI's expectation.
 ### Why it happened [L404-417]
 Go 1.25 was a "minor" version bump (1.24 → 1.25), but golangci-lint v1.64.8 was built with Go 1.24's toolchain. When golangci-lint loads its config, it validates against the Go runtime version it was compiled with. Since our project targets Go 1.25, the linter refused to run, citing an incompatibility.

 This forced us to upgrade from golangci-lint v1.x to v2.x — a full major version jump triggered by what seemed like a harmless minor Go update. The v2 upgrade also required migrating the entire `.golangci.yaml` schema (linters → formatters split, exclusion rules restructuring, array vs string type changes).
 ### Fix applied [L417-427]
 **Symptom**: CI fails with `can't load config: the Go language version (go1.24) used to build golangci-lint is lower than the targeted Go version (1.25.0)`.

 **Immediate fix**:
 1. Updated `.github/workflows/ci.yml`: Changed `golangci-lint-action@v6` → `@v7` (v2 requires action v7)
 2. Updated version from `v1.64.8` → `v2.12.2` in both CI workflow and Makefile
 3. Migrated `.golangci.yaml` to v2 schema:
    - Added `version: "2"` at top
    - Split `linters-settings` → `linters.settings` + `formatters.settings`
    - Moved `gofmt`/`goimports` from linters to formatters section
    - Converted `issues.*` → `linters.exclusions.*`
    - Fixed `local-prefixes` from string to array format
 4. Fixed newly surfaced lint errors:
    - Removed duplicate `database/sql` import
    - Converted if-else chains to tagged switches (QF1003)
    - Lowercased error message capitalization (ST1005)

 **Why pre-commit missed it**: Local hooks run `golangci-lint run` directly from PATH, which may be a different version (or absent). The Makefile's `GOLANGCI_LINT_VERSION` variable only gates `make tools`, not every commit. CI explicitly pins its own version, so the mismatch surfaced only in the workflow.

 **Lesson**: When Go does a minor bump, tooling that embeds or validates the Go runtime often requires a major version upgrade. Pin lint tool versions in both CI and local dev, and treat Go runtime bumps as breaking changes for the toolchain. Consider adding a version check to pre-commit hooks that validates the local binary matches CI's pinned version.
