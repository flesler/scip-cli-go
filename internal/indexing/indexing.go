package indexing

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/sourcegraph/scip-cli-go/internal/cache"
	"github.com/sourcegraph/scip-cli-go/internal/config"
	"github.com/sourcegraph/scip-cli-go/internal/discover"
	"github.com/sourcegraph/scip-cli-go/internal/merge"
	"github.com/sourcegraph/scip-cli-go/internal/project"
	"github.com/sourcegraph/scip-cli-go/internal/scip"
	"github.com/sourcegraph/scip-cli-go/internal/scope"
	"github.com/sourcegraph/scip-cli-go/internal/sqlhelp"
	"github.com/sourcegraph/scip-cli-go/internal/symbols"

	_ "modernc.org/sqlite"
)

const (
	IndexTimeout           = 300
	DefaultMaxHeapMb       = 8192
	ProgressLogMinProjects = 10
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
		fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			return n
		}
	}
	return defaultIndexWorkers()
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
		suffix = fmt.Sprintf(", %d tsconfigs", projects)
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

func indexOneTSProject(root, proj, workDir string, env []string) indexResult {
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return indexResult{label: proj, errMsg: err.Error()}
	}

	absRoot, _ := filepath.Abs(root)
	partScip := filepath.Join(workDir, "index.scip")
	args := []string{"index", "--output", partScip, proj}

	if _, err := os.Stat(filepath.Join(absRoot, "tsconfig.json")); os.IsNotExist(err) {
		args = []string{"index", "--infer-tsconfig", "--output", partScip, proj}
	}

	if err := runIndexer("scip-typescript", "@sourcegraph/scip-typescript", scip.ScipTypescriptVersion, absRoot, args, env); err != nil {
		return indexResult{label: proj, errMsg: err.Error()}
	}

	partDB := filepath.Join(workDir, "index.db")
	if err := convertScipToDB(partScip, partDB); err != nil {
		return indexResult{label: proj, errMsg: err.Error()}
	}

	return indexResult{label: proj, dbPath: partDB}
}

func runIndexer(binary, npxPackage, npxVersion, cwd string, args, env []string) error {
	cmd := exec.Command(binary, args...)
	cmd.Dir = cwd
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(string(out)), "not found") {
		return runNpxIndexer(npxPackage, npxVersion, cwd, args, env)
	}
	if _, lookErr := exec.LookPath(binary); lookErr != nil {
		return runNpxIndexer(npxPackage, npxVersion, cwd, args, env)
	}
	return fmt.Errorf("%s failed: %s", binary, string(out))
}

func runNpxIndexer(pkg, version, cwd string, args, env []string) error {
	spec := fmt.Sprintf("%s@~%s", pkg, version)
	npxArgs := append([]string{"-y", spec}, args...)
	cmd := exec.Command("npx", npxArgs...)
	cmd.Dir = cwd
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("npx %s failed: %s", pkg, string(out))
	}
	return nil
}

func convertScipToDB(scipPath, dbPath string) error {
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

	cmd := exec.Command(scipBin, "expt-convert", scipPath, "--output", filepath.Base(dbPath))
	cmd.Dir = filepath.Dir(dbPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintln(os.Stderr, string(out))
		return fmt.Errorf("failed to convert index")
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("failed to convert index: output not created")
	}

	return postprocessIndex(dbPath)
}

func postprocessIndex(dbPath string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := trimUnusedColumns(db); err != nil {
		return err
	}
	if err := trimMentionsToKnownSymbols(db); err != nil {
		return err
	}
	if err := trimDefnToKnownSymbols(db); err != nil {
		return err
	}
	return nil
}

func trimUnusedColumns(db *sql.DB) error {
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
		WHERE %s`, exclude)); err != nil {
		return err
	}
	if _, err := db.Exec(`DROP TABLE global_symbols`); err != nil {
		return err
	}
	_, err := db.Exec(`ALTER TABLE global_symbols_new RENAME TO global_symbols`)
	return err
}

func trimMentionsToKnownSymbols(db *sql.DB) error {
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

func trimDefnToKnownSymbols(db *sql.DB) error {
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
				res := indexOneTSProject(root, p, workDir, env)
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
			if showProgress {
				fmt.Fprintf(os.Stderr, "Indexing %d/%d: %s\n", i+1, total, proj)
			}
			workDir := filepath.Join(tmpDir, fmt.Sprintf("part-%d", i+1))
			res := indexOneTSProject(root, proj, workDir, env)
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

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
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
		tmpDir, err := os.MkdirTemp("", "scip-index-*")
		if err != nil {
			return "", 0, 0, err
		}
		defer os.RemoveAll(tmpDir)

		indexScip := filepath.Join(tmpDir, "index.scip")
		if err := runIndexer("scip-python", "@sourcegraph/scip-python", scip.ScipPythonVersion, absRoot, []string{"index", ".", "--output", indexScip}, env); err != nil {
			return "", 0, 0, err
		}

		out := cache.IndexDBPath(cacheDir, replace)
		if err := convertScipToDB(indexScip, out); err != nil {
			return "", 0, 0, err
		}
		if doLog {
			logIndexComplete(out, lang, 0, 0)
		}
		return out, 0, 1, nil

	default:
		return "", 0, 0, fmt.Errorf("unsupported language '%s'", lang)
	}
}

// GetDB returns a SQLite connection to the index, triggering indexing if needed.
func GetDB(projectRoot string) (*sql.DB, error) {
	if projectRoot == "" {
		projectRoot, _ = os.Getwd()
	}

	dbPath := cache.FindDB(projectRoot)
	if dbPath == "" {
		root, lang, ok := project.FindProjectRootAndLanguage(projectRoot)
		if !ok {
			return nil, fmt.Errorf("could not find project root")
		}
		if lang == "" {
			return nil, fmt.Errorf("no supported project markers found in %s", root)
		}

		cacheDir := cache.GetCacheDir(root)
		lock, err := cache.IndexBuildLock(cacheDir)
		if err != nil {
			return nil, err
		}
		defer cache.UnlockIndex(lock)

		dbPath = cache.FindDB(projectRoot)
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

		dbPath = cache.FindDB(projectRoot)
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
