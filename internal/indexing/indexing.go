package indexing

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flesler/scip-cli-go/v2/internal/cache"
	"github.com/flesler/scip-cli-go/v2/internal/config"
	"github.com/flesler/scip-cli-go/v2/internal/discover"
	"github.com/flesler/scip-cli-go/v2/internal/merge"
	"github.com/flesler/scip-cli-go/v2/internal/project"
	"github.com/flesler/scip-cli-go/v2/internal/scip"
	"github.com/flesler/scip-cli-go/v2/internal/scope"
	"github.com/flesler/scip-cli-go/v2/internal/sqlhelp"
	"github.com/flesler/scip-cli-go/v2/internal/symbols"

	_ "modernc.org/sqlite"
)

const (
	IndexTimeout           = 300
	DefaultMaxHeapMb       = 8192
	ProgressLogMinProjects = 10
	MaxTSIndexBatchSize    = 2147483647
)

func defaultIndexWorkers() int {
	n := runtime.NumCPU()
	if n < 4 {
		n = 4
	}
	if n > 8 {
		n = 8
	}
	return n
}

func indexWorkers() int {
	if v := os.Getenv("SCIP_CLI_INDEX_WORKERS"); v != "" {
		n := 0
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 1 {
			return defaultIndexWorkers()
		}
		return n
	}
	return defaultIndexWorkers()
}

func tsIndexBatchSize() (int, error) {
	v := os.Getenv("SCIP_CLI_TS_INDEX_BATCH_SIZE")
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, fmt.Errorf("invalid SCIP_CLI_TS_INDEX_BATCH_SIZE: expected an integer, got %q", v)
	}
	if n < 1 {
		return 0, fmt.Errorf("invalid SCIP_CLI_TS_INDEX_BATCH_SIZE: expected a positive integer, got %d", n)
	}
	if n > MaxTSIndexBatchSize {
		return 0, fmt.Errorf("SCIP_CLI_TS_INDEX_BATCH_SIZE=%d exceeds max (%d)", n, MaxTSIndexBatchSize)
	}
	return n, nil
}

func batchProjects(projects []string, batchSize int) [][]string {
	if len(projects) == 0 {
		return nil
	}
	if batchSize <= 0 {
		return [][]string{projects}
	}
	var batches [][]string
	for i := 0; i < len(projects); i += batchSize {
		end := i + batchSize
		if end > len(projects) {
			end = len(projects)
		}
		batches = append(batches, projects[i:end])
	}
	return batches
}

func projectBatchLabel(projects []string) string {
	if len(projects) == 1 {
		return projectLabel(projects[0])
	}
	return fmt.Sprintf("%s +%d more", projectLabel(projects[0]), len(projects)-1)
}

func tsBatchLimitDisplay(batchSize, total int) string {
	if batchSize <= 0 || batchSize >= total {
		return "all tsconfigs per run"
	}
	return fmt.Sprintf("up to %d tsconfigs per run", batchSize)
}

func typescriptIndexArgs(root, outputScip string, projects []string) []string {
	absRoot, _ := filepath.Abs(root)
	args := []string{"index", "--output", outputScip}
	if _, err := os.Stat(filepath.Join(absRoot, "tsconfig.json")); os.IsNotExist(err) {
		args = []string{"index", "--infer-tsconfig", "--output", outputScip}
	}
	return append(args, projects...)
}

func formatDBSize(dbPath string) string {
	info, err := os.Stat(dbPath)
	if err != nil {
		return "?"
	}
	nbytes := info.Size()
	if nbytes < 1024 {
		return fmt.Sprintf("%d B", nbytes)
	}
	if nbytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(nbytes)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(nbytes)/(1024*1024))
}

func logIndexComplete(dbPath, lang string, projects, skipped int) {
	size := formatDBSize(dbPath)
	suffix := ""
	if projects > 1 {
		suffix = fmt.Sprintf(", %d projects", projects)
		if skipped > 0 {
			suffix += fmt.Sprintf(", %d skipped", skipped)
		}
	}
	fmt.Fprintf(os.Stderr, "Indexed %s (%s, %s%s)\n", dbPath, size, lang, suffix)
}

func typescriptProjects(root string) ([]string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	settings, err := config.LoadProjectConfig(absRoot)
	if err != nil {
		return nil, err
	}

	idxScope := scope.LoadIndexScope(absRoot)
	if idxScope != nil {
		discovered, err := discover.DiscoverProjects(absRoot, "")
		if err != nil {
			return nil, err
		}
		filtered := scope.ProjectsMatchingScope(discovered, idxScope.Paths)
		if len(filtered) == 0 {
			return nil, fmt.Errorf("no TypeScript projects found under index scope: %s", strings.Join(idxScope.Paths, ", "))
		}
		sort.Strings(filtered)
		return filtered, nil
	}

	var configured []string
	if len(settings.IndexRoots) > 0 {
		configured, err = config.ResolveIndexRoots(absRoot, settings)
		if err != nil {
			return nil, err
		}
	}

	if settings.OnlyIndexRoots {
		if len(configured) == 0 {
			return nil, fmt.Errorf("onlyIndexRoots is true but no indexRoots are configured in %s", config.ConfigFilename)
		}
		return configured, nil
	}

	discovered, err := discover.DiscoverProjects(absRoot, "")
	if err != nil {
		return nil, err
	}
	merged := make(map[string]string)
	for _, p := range discovered {
		merged[p] = p
	}
	for _, p := range configured {
		merged[p] = p
	}

	result := make([]string, 0, len(merged))
	for _, p := range merged {
		result = append(result, p)
	}
	sort.Strings(result)
	return result, nil
}

type indexResult struct {
	label  string
	dbPath string
	errMsg string
}

func indexTSProjects(root string, projects []string, workDir string, env []string, outputDB string) indexResult {
	label := projectBatchLabel(projects)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return indexResult{label: label, errMsg: err.Error()}
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return indexResult{label: label, errMsg: err.Error()}
	}
	partScip := filepath.Join(workDir, "index.scip")
	dbPath := outputDB
	if dbPath == "" {
		dbPath = filepath.Join(workDir, "index.db")
	}
	args := typescriptIndexArgs(root, partScip, projects)
	if err := runIndexer("scip-typescript", "@sourcegraph/scip-typescript", scip.ScipTypescriptVersion, "", "", absRoot, args, env); err != nil {
		return indexResult{label: label, errMsg: err.Error()}
	}
	if err := convertScipToDB(partScip, dbPath, ""); err != nil {
		return indexResult{label: label, errMsg: err.Error()}
	}
	return indexResult{label: label, dbPath: dbPath}
}

func projectCwd(root, proj string) string {
	absRoot, _ := filepath.Abs(root)
	if proj == "" || proj == "." {
		return absRoot
	}
	return filepath.Join(absRoot, proj)
}

func projectLabel(proj string) string {
	if proj == "" || proj == "." {
		return "."
	}
	return proj
}

func indexOnePythonProject(root, proj, workDir string, env []string) indexResult {
	label := projectLabel(proj)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return indexResult{label: label, errMsg: err.Error()}
	}
	cwd := projectCwd(root, proj)
	partScip := filepath.Join(workDir, "index.scip")
	partDB := filepath.Join(workDir, "index.db")
	if err := runIndexer("scip-python", "@sourcegraph/scip-python", scip.ScipPythonVersion, "", "", cwd, []string{"index", ".", "--output", partScip}, env); err != nil {
		return indexResult{label: label, errMsg: err.Error()}
	}
	if err := convertScipToDB(partScip, partDB, proj); err != nil {
		return indexResult{label: label, errMsg: err.Error()}
	}
	return indexResult{label: label, dbPath: partDB}
}

func indexOneGolangModule(root, proj, workDir string, env []string) indexResult {
	label := projectLabel(proj)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return indexResult{label: label, errMsg: err.Error()}
	}
	cwd := projectCwd(root, proj)
	partScip := filepath.Join(workDir, "index.scip")
	partDB := filepath.Join(workDir, "index.db")
	goEnv := os.Environ()
	if err := runIndexer("scip-go", "", "", scip.ScipGoPackage, "", cwd, []string{"--output", partScip}, goEnv); err != nil {
		return indexResult{label: label, errMsg: err.Error()}
	}
	if err := convertScipToDB(partScip, partDB, proj); err != nil {
		return indexResult{label: label, errMsg: err.Error()}
	}
	return indexResult{label: label, dbPath: partDB}
}

func indexOneRustCrate(root, proj, workDir string, env []string) indexResult {
	label := projectLabel(proj)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return indexResult{label: label, errMsg: err.Error()}
	}
	cwd := projectCwd(root, proj)
	partScip := filepath.Join(workDir, "index.scip")
	partDB := filepath.Join(workDir, "index.db")
	if err := runIndexer("rust-analyzer", "", "", "", "rust-analyzer", cwd, []string{"scip", cwd, "--output", partScip}, env); err != nil {
		return indexResult{label: label, errMsg: err.Error()}
	}
	if err := convertScipToDB(partScip, partDB, proj); err != nil {
		return indexResult{label: label, errMsg: err.Error()}
	}
	return indexResult{label: label, dbPath: partDB}
}

type indexOneFunc func(root, proj, workDir string, env []string) indexResult

func indexDiscovered(root, cacheDir string, projects []string, env []string, replace bool, indexOne indexOneFunc) (string, int, int, int, error) {
	workers := indexWorkers()
	useParallel := len(projects) > 1 && workers > 1
	outputDB := cache.IndexDBPath(cacheDir, replace)

	tmpDir, err := os.MkdirTemp("", "scip-index-*")
	if err != nil {
		return "", 0, 0, 0, err
	}
	defer os.RemoveAll(tmpDir)

	total := len(projects)
	showProgress := total > ProgressLogMinProjects

	var partDBs []string
	skipped := 0

	if useParallel {
		type indexedResult struct {
			res indexResult
			idx int
		}
		ch := make(chan indexedResult, total)
		var wg sync.WaitGroup

		sem := make(chan struct{}, workers)
		for i, proj := range projects {
			wg.Add(1)
			go func(idx int, p string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				workDir := filepath.Join(tmpDir, fmt.Sprintf("part-%d", idx+1))
				res := indexOne(root, p, workDir, env)
				ch <- indexedResult{res: res, idx: idx}
			}(i, proj)
		}

		go func() {
			wg.Wait()
			close(ch)
		}()

		var results []indexedResult
		completed := 0
		for r := range ch {
			completed++
			results = append(results, r)
			if r.res.dbPath == "" {
				skipped++
				fmt.Fprintf(os.Stderr, "Warning: skipped %s: %s\n", r.res.label, r.res.errMsg)
			} else if showProgress {
				fmt.Fprintf(os.Stderr, "Indexed %d/%d: %s\n", completed, total, r.res.label)
			}
		}

		sort.Slice(results, func(i, j int) bool { return results[i].idx < results[j].idx })
		for _, r := range results {
			if r.res.dbPath != "" {
				partDBs = append(partDBs, r.res.dbPath)
			}
		}
	} else {
		for i, proj := range projects {
			label := projectLabel(proj)
			if showProgress {
				fmt.Fprintf(os.Stderr, "Indexing %d/%d: %s\n", i+1, total, label)
			}
			workDir := filepath.Join(tmpDir, fmt.Sprintf("part-%d", i+1))
			res := indexOne(root, proj, workDir, env)
			if res.dbPath == "" {
				skipped++
				fmt.Fprintf(os.Stderr, "Warning: skipped %s: %s\n", res.label, res.errMsg)
				continue
			}
			partDBs = append(partDBs, res.dbPath)
		}
	}

	if len(partDBs) == 0 {
		return "", 0, skipped, total, fmt.Errorf("failed to index project")
	}

	if len(partDBs) == 1 {
		if err := copyFile(partDBs[0], outputDB); err != nil {
			return "", 0, skipped, total, err
		}
	} else {
		if err := merge.MergeSQLiteIndexes(partDBs, outputDB); err != nil {
			return "", 0, skipped, total, err
		}
	}

	return outputDB, len(partDBs), skipped, total, nil
}

func runIndexer(binary, npxPackage, npxVersion, goPackage, rustupComponent, cwd string, args, env []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), IndexTimeout*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = cwd
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("%s timed out after %ds", binary, IndexTimeout)
	}
	outLower := strings.ToLower(string(out))
	if strings.Contains(outLower, "not found") {
		return runIndexerFallback(binary, npxPackage, npxVersion, goPackage, rustupComponent, cwd, args, env)
	}
	if _, lookErr := exec.LookPath(binary); lookErr != nil {
		return runIndexerFallback(binary, npxPackage, npxVersion, goPackage, rustupComponent, cwd, args, env)
	}
	return fmt.Errorf("%s failed: %s", binary, string(out))
}

func runIndexerFallback(binary, npxPackage, npxVersion, goPackage, rustupComponent, cwd string, args, env []string) error {
	if goPackage != "" {
		return runGoInstallIndexer(goPackage, binary, cwd, args, env)
	}
	if rustupComponent != "" {
		return runRustupIndexer(rustupComponent, binary, cwd, args, env)
	}
	if npxPackage != "" {
		return runNpxIndexer(npxPackage, npxVersion, cwd, args, env)
	}
	return fmt.Errorf("binary %q not found and no fallback available", binary)
}

func goBinPath(binary string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "go", "bin", binary)
}

func runGoInstallIndexer(goPackage, binary, cwd string, args, env []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), IndexTimeout*time.Second)
	defer cancel()

	goBinary := goBinPath(binary)
	if goBinary != "" {
		if _, err := os.Stat(goBinary); err == nil {
			cmd := exec.CommandContext(ctx, goBinary, args...)
			cmd.Dir = cwd
			cmd.Env = env
			out, err := cmd.CombinedOutput()
			if err == nil {
				return nil
			}
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("%s timed out after %ds", binary, IndexTimeout)
			}
			if _, lookErr := exec.LookPath(binary); lookErr == nil {
				return fmt.Errorf("%s failed: %s", binary, string(out))
			}
		}
	}

	install := exec.CommandContext(ctx, "go", "install", goPackage+"@latest")
	install.Dir = cwd
	install.Env = env
	out, err := install.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("go install %s timed out after %ds", goPackage, IndexTimeout)
		}
		return fmt.Errorf("failed to install %s via go install: %s", binary, string(out))
	}

	runBin := binary
	if goBinary != "" {
		if _, statErr := os.Stat(goBinary); statErr == nil {
			runBin = goBinary
		}
	}
	cmd := exec.CommandContext(ctx, runBin, args...)
	cmd.Dir = cwd
	cmd.Env = env
	out, err = cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("%s timed out after %ds", binary, IndexTimeout)
		}
		return fmt.Errorf("%s failed: %s", binary, string(out))
	}
	return nil
}

func runRustupIndexer(component, binary, cwd string, args, env []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), IndexTimeout*time.Second)
	defer cancel()

	install := exec.CommandContext(ctx, "rustup", "component", "add", component)
	install.Dir = cwd
	install.Env = env
	out, err := install.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("rustup component add %s timed out after %ds", component, IndexTimeout)
		}
		return fmt.Errorf("failed to install %s via rustup: %s", component, string(out))
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = cwd
	cmd.Env = env
	out, err = cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("%s timed out after %ds", binary, IndexTimeout)
		}
		return fmt.Errorf("%s failed: %s", binary, string(out))
	}
	return nil
}

func runNpxIndexer(pkg, version, cwd string, args, env []string) error {
	spec := fmt.Sprintf("%s@~%s", pkg, version)
	npxArgs := append([]string{"-y", spec}, args...)
	ctx, cancel := context.WithTimeout(context.Background(), IndexTimeout*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "npx", npxArgs...)
	cmd.Dir = cwd
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("npx %s timed out after %ds", pkg, IndexTimeout)
		}
		return fmt.Errorf("npx %s failed: %s", pkg, string(out))
	}
	return nil
}

func convertScipToDB(scipPath, dbPath, documentPathPrefix string) error {
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return err
	}

	scipBin, err := scip.EnsureScipBinary()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), IndexTimeout*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, scipBin, "expt-convert", scipPath, "--output", filepath.Base(dbPath))
	cmd.Dir = filepath.Dir(dbPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("scip expt-convert timed out after %ds", IndexTimeout)
		}
		fmt.Fprintln(os.Stderr, string(out))
		return fmt.Errorf("failed to convert index")
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("failed to convert index: output not created")
	}

	if err := postprocessIndex(dbPath); err != nil {
		return err
	}
	prefix := strings.TrimPrefix(filepath.ToSlash(documentPathPrefix), "./")
	if prefix != "" && prefix != "." {
		return prefixDocumentPaths(dbPath, prefix)
	}
	return nil
}

func prefixDocumentPaths(dbPath, prefix string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("UPDATE documents SET relative_path = ? || '/' || relative_path", prefix)
	return err
}

func postprocessIndex(dbPath string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	keepExternal := os.Getenv("SCIP_CLI_KEEP_EXTERNAL") == "1"
	if err := trimUnusedColumns(tx, keepExternal); err != nil {
		return err
	}
	if err := trimMentionsToKnownSymbols(tx); err != nil {
		return err
	}
	if err := trimDefnToKnownSymbols(tx); err != nil {
		return err
	}
	return tx.Commit()
}

type sqlExec interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

func trimUnusedColumns(db sqlExec, keepExternal bool) error {
	if _, err := db.Exec(`CREATE TABLE documents_new (
		id INTEGER PRIMARY KEY,
		relative_path TEXT NOT NULL UNIQUE
	)`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT INTO documents_new SELECT id, relative_path FROM documents`); err != nil {
		return err
	}
	if _, err := db.Exec(`DROP TABLE documents`); err != nil {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE documents_new RENAME TO documents`); err != nil {
		return err
	}

	exclude := symbols.SQLExcludeVariableSymbols("symbol")

	whereClause := exclude
	if !keepExternal {
		// Keep symbols that either:
		// 1. Are defined in the project (have defn_enclosing_ranges entries)
		// 2. Are structural symbols needed for type analysis (type literals, parameters)
		// 3. Are functions/methods (even without defs, they may be project code)
		//
		// SCIP symbol format patterns (language-agnostic):
		// - %typeLiteral% : Type/interface fields (e.g., Options#typeLiteral0:verbose.)
		// - %).(% : Function parameters (e.g., greet().(name))
		// - %().% : Functions and methods (e.g., Widget.run()., greet().)
		whereClause = fmt.Sprintf(`(%s) AND (
			EXISTS (SELECT 1 FROM defn_enclosing_ranges der WHERE der.symbol_id = global_symbols.id)
			OR symbol LIKE '%%typeLiteral%%'
			OR symbol LIKE '%%). (%%'
			OR symbol LIKE '%%().%%'
		)`, exclude)
	}

	if _, err := db.Exec(`CREATE TABLE global_symbols_new (
		id INTEGER PRIMARY KEY,
		symbol TEXT NOT NULL UNIQUE,
		display_name TEXT,
		kind INTEGER
	)`); err != nil {
		return err
	}
	if _, err := db.Exec(fmt.Sprintf(`INSERT INTO global_symbols_new
		SELECT id, symbol, display_name, kind FROM global_symbols
		WHERE %s`, whereClause)); err != nil {
		return err
	}
	if _, err := db.Exec(`DROP TABLE global_symbols`); err != nil {
		return err
	}
	_, err := db.Exec(`ALTER TABLE global_symbols_new RENAME TO global_symbols`)
	return err
}

func trimMentionsToKnownSymbols(db sqlExec) error {
	var exists int
	err := db.QueryRow(`SELECT 1 FROM sqlite_master WHERE type='table' AND name='mentions' LIMIT 1`).Scan(&exists)
	if err != nil {
		return nil
	}

	if _, err := db.Exec(`CREATE TABLE mentions_new (
		chunk_id INTEGER NOT NULL,
		symbol_id INTEGER NOT NULL,
		role INTEGER NOT NULL,
		PRIMARY KEY (chunk_id, symbol_id, role)
	)`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT INTO mentions_new (chunk_id, symbol_id, role)
		SELECT m.chunk_id, m.symbol_id, m.role
		FROM mentions m
		JOIN global_symbols g ON g.id = m.symbol_id`); err != nil {
		return err
	}
	if _, err := db.Exec(`DROP TABLE mentions`); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE mentions_new RENAME TO mentions`)
	return err
}

func trimDefnToKnownSymbols(db sqlExec) error {
	var exists int
	err := db.QueryRow(`SELECT 1 FROM sqlite_master WHERE type='table' AND name='defn_enclosing_ranges' LIMIT 1`).Scan(&exists)
	if err != nil {
		return nil
	}

	if _, err := db.Exec(`CREATE TABLE defn_enclosing_ranges_new (
		id INTEGER PRIMARY KEY,
		document_id INTEGER NOT NULL,
		symbol_id INTEGER NOT NULL,
		start_line INTEGER NOT NULL,
		start_char INTEGER NOT NULL,
		end_line INTEGER NOT NULL,
		end_char INTEGER NOT NULL
	)`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT INTO defn_enclosing_ranges_new (
		id, document_id, symbol_id, start_line, start_char, end_line, end_char
	)
		SELECT d.id, d.document_id, d.symbol_id, d.start_line, d.start_char, d.end_line, d.end_char
		FROM defn_enclosing_ranges d
		JOIN global_symbols g ON g.id = d.symbol_id`); err != nil {
		return err
	}
	if _, err := db.Exec(`DROP TABLE defn_enclosing_ranges`); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE defn_enclosing_ranges_new RENAME TO defn_enclosing_ranges`)
	return err
}

func parseHeapMb(value, source string) (string, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n <= 0 {
		return "", fmt.Errorf("invalid %s: expected a positive integer, got %q", source, value)
	}
	return strconv.Itoa(n), nil
}

func indexerEnv(projectRoot string) ([]string, error) {
	env := os.Environ()
	heapMb := os.Getenv("SCIP_CLI_MAX_HEAP_MB")
	if heapMb != "" {
		parsed, err := parseHeapMb(heapMb, "SCIP_CLI_MAX_HEAP_MB")
		if err != nil {
			return nil, err
		}
		heapMb = parsed
	} else if settings, err := config.LoadProjectConfig(projectRoot); err == nil && settings.MaxHeapMb != nil {
		heapMb = fmt.Sprintf("%d", *settings.MaxHeapMb)
	} else {
		heapMb = fmt.Sprintf("%d", DefaultMaxHeapMb)
	}

	flag := fmt.Sprintf("--max-old-space-size=%s", heapMb)
	for i, e := range env {
		if strings.HasPrefix(e, "NODE_OPTIONS=") {
			val := e[len("NODE_OPTIONS="):]
			if !strings.Contains(val, flag) {
				env[i] = "NODE_OPTIONS=" + strings.TrimSpace(val+" "+flag)
			}
			return env, nil
		}
	}
	env = append(env, "NODE_OPTIONS="+flag)
	return env, nil
}

func indexTypescript(root, cacheDir string, projects []string, env []string, replace bool) (string, int, int, int, error) {
	batchSize, err := tsIndexBatchSize()
	if err != nil {
		return "", 0, 0, 0, err
	}
	batches := batchProjects(projects, batchSize)
	workers := indexWorkers()
	useParallel := len(batches) > 1 && workers > 1
	outputDB := cache.IndexDBPath(cacheDir, replace)

	tmpDir, err := os.MkdirTemp("", "scip-index-*")
	if err != nil {
		return "", 0, 0, 0, err
	}
	defer os.RemoveAll(tmpDir)

	total := len(projects)
	showProgress := total > ProgressLogMinProjects

	var partDBs []string
	skipped := 0

	if showProgress && useParallel {
		fmt.Fprintf(os.Stderr, "Indexing %d TypeScript projects (%d workers, %s; merge is serial)...\n",
			total, workers, tsBatchLimitDisplay(batchSize, total))
	}

	if useParallel {
		type indexedResult struct {
			res   indexResult
			idx   int
			batch []string
		}
		ch := make(chan indexedResult, len(batches))
		var wg sync.WaitGroup

		sem := make(chan struct{}, workers)
		for i, batch := range batches {
			wg.Add(1)
			go func(idx int, b []string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				workDir := filepath.Join(tmpDir, fmt.Sprintf("part-%d", idx+1))
				res := indexTSProjects(root, b, workDir, env, "")
				ch <- indexedResult{res: res, idx: idx, batch: b}
			}(i, batch)
		}

		go func() {
			wg.Wait()
			close(ch)
		}()

		var results []indexedResult
		completed := 0
		for r := range ch {
			completed += len(r.batch)
			results = append(results, r)
			if r.res.dbPath == "" {
				skipped += len(r.batch)
				fmt.Fprintf(os.Stderr, "Warning: skipped %s: %s\n", r.res.label, r.res.errMsg)
			} else if showProgress {
				fmt.Fprintf(os.Stderr, "Indexed %d/%d: %s\n", completed, total, r.res.label)
			}
		}

		sort.Slice(results, func(i, j int) bool { return results[i].idx < results[j].idx })
		for _, r := range results {
			if r.res.dbPath != "" {
				partDBs = append(partDBs, r.res.dbPath)
			}
		}
	} else {
		indexed := 0
		for i, batch := range batches {
			label := projectBatchLabel(batch)
			if showProgress {
				end := indexed + len(batch)
				fmt.Fprintf(os.Stderr, "Indexing %d-%d/%d: %s\n", indexed+1, end, total, label)
			}
			var directOutput string
			if len(batches) == 1 {
				directOutput = outputDB
			}
			workDir := cacheDir
			if directOutput == "" {
				workDir = filepath.Join(tmpDir, fmt.Sprintf("part-%d", i+1))
			}
			res := indexTSProjects(root, batch, workDir, env, directOutput)
			indexed += len(batch)
			if res.dbPath == "" {
				skipped += len(batch)
				fmt.Fprintf(os.Stderr, "Warning: skipped %s: %s\n", res.label, res.errMsg)
				continue
			}
			partDBs = append(partDBs, res.dbPath)
		}
	}

	if len(partDBs) == 0 {
		return "", 0, skipped, total, fmt.Errorf("failed to index project")
	}

	if len(partDBs) == 1 {
		if partDBs[0] != outputDB {
			if err := copyFile(partDBs[0], outputDB); err != nil {
				return "", 0, skipped, total, err
			}
		}
	} else {
		if err := merge.MergeSQLiteIndexes(partDBs, outputDB); err != nil {
			return "", 0, skipped, total, err
		}
	}

	return outputDB, len(partDBs), skipped, total, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func indexProject(root, lang, cacheDir string, replace, doLog bool) (string, int, int, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", 0, 0, err
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", 0, 0, err
	}

	env, err := indexerEnv(absRoot)
	if err != nil {
		return "", 0, 0, err
	}

	switch project.Language(lang) {
	case project.LanguageTypeScript:
		projects, err := typescriptProjects(absRoot)
		if err != nil {
			return "", 0, 0, err
		}
		outputDB, _, skipped, total, err := indexTypescript(absRoot, cacheDir, projects, env, replace)
		if err != nil {
			return "", 0, 0, err
		}
		if doLog {
			projCount := 0
			if total > 1 {
				projCount = total
			}
			logIndexComplete(outputDB, lang, projCount, skipped)
		}
		return outputDB, skipped, total, nil

	case project.LanguagePython:
		projects, err := discover.DiscoverPythonProjects(absRoot)
		if err != nil {
			return "", 0, 0, err
		}
		outputDB, _, skipped, total, err := indexDiscovered(absRoot, cacheDir, projects, env, replace, indexOnePythonProject)
		if err != nil {
			return "", 0, 0, err
		}
		if doLog {
			projCount := 0
			if total > 1 {
				projCount = total
			}
			logIndexComplete(outputDB, lang, projCount, skipped)
		}
		return outputDB, skipped, total, nil

	case project.LanguageGolang:
		modules, err := discover.DiscoverGolangModules(absRoot)
		if err != nil {
			return "", 0, 0, err
		}
		outputDB, _, skipped, total, err := indexDiscovered(absRoot, cacheDir, modules, os.Environ(), replace, indexOneGolangModule)
		if err != nil {
			return "", 0, 0, err
		}
		if doLog {
			projCount := 0
			if total > 1 {
				projCount = total
			}
			logIndexComplete(outputDB, lang, projCount, skipped)
		}
		return outputDB, skipped, total, nil

	case project.LanguageRust:
		crates, err := discover.DiscoverRustCrates(absRoot)
		if err != nil {
			return "", 0, 0, err
		}
		outputDB, _, skipped, total, err := indexDiscovered(absRoot, cacheDir, crates, os.Environ(), replace, indexOneRustCrate)
		if err != nil {
			return "", 0, 0, err
		}
		if doLog {
			projCount := 0
			if total > 1 {
				projCount = total
			}
			logIndexComplete(outputDB, lang, projCount, skipped)
		}
		return outputDB, skipped, total, nil

	default:
		return "", 0, 0, fmt.Errorf("unsupported language '%s'", lang)
	}
}

// GetDB returns a SQLite connection to the index, triggering indexing if needed.
func GetDB(projectRoot string) (*sql.DB, error) {
	root, lang, ok := project.FindProjectRootAndLanguage(projectRoot)
	if !ok {
		return nil, fmt.Errorf("could not find project root")
	}
	if lang == "" {
		return nil, fmt.Errorf("no supported project markers found in %s", root)
	}

	dbPath := cache.FindDB(root)
	if dbPath == "" {
		cacheDir := cache.GetCacheDir(root)
		lock, err := cache.IndexBuildLock(cacheDir)
		if err != nil {
			return nil, err
		}
		defer cache.UnlockIndex(lock)

		dbPath = cache.FindDB(root)
		if dbPath == "" {
			cache.CleanupInProgressIndex(cacheDir)
			outputDB, skipped, total, idxErr := indexProject(root, string(lang), cacheDir, true, false)
			if idxErr != nil {
				cache.CleanupInProgressIndex(cacheDir)
				return nil, idxErr
			}
			if err := cache.PromoteNextIndex(cacheDir); err != nil {
				return nil, err
			}
			projCount := 0
			if total > 1 {
				projCount = total
			}
			logIndexComplete(outputDB, string(lang), projCount, skipped)
		}

		dbPath = cache.FindDB(root)
		if dbPath == "" {
			return nil, fmt.Errorf("no index.db found after indexing")
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if err := sqlhelp.ConfigureReadConnection(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// EnsureIndex runs the indexer if no index exists.
func EnsureIndex(projectRoot string) error {
	if cache.FindDB(projectRoot) != "" {
		return nil
	}
	_, err := GetDB(projectRoot)
	return err
}

// Reindex rebuilds the index from scratch.
func Reindex(projectRoot string, force bool) error {
	if projectRoot == "" {
		projectRoot, _ = os.Getwd()
	}
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return err
	}

	root, lang, ok := project.FindProjectRootAndLanguage(absRoot)
	if !ok {
		return fmt.Errorf("could not find project root")
	}
	if lang == "" {
		return fmt.Errorf("no supported project markers found in %s", root)
	}

	cacheDir := cache.GetCacheDir(root)
	lock, err := cache.IndexBuildLock(cacheDir)
	if err != nil {
		return err
	}
	defer cache.UnlockIndex(lock)

	cache.CleanupInProgressIndex(cacheDir)
	outputDB, skipped, total, err := indexProject(root, string(lang), cacheDir, true, false)
	if err != nil {
		cache.CleanupInProgressIndex(cacheDir)
		return err
	}
	if err := cache.PromoteNextIndex(cacheDir); err != nil {
		return err
	}
	projCount := 0
	if total > 1 {
		projCount = total
	}
	logIndexComplete(outputDB, string(lang), projCount, skipped)
	return nil
}
