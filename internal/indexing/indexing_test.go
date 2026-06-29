package indexing

import (
	"bytes"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flesler/scip-cli-go/v2/internal/config"
	"github.com/flesler/scip-cli-go/v2/internal/scope"

	_ "modernc.org/sqlite"
)

func seedIndexDB(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Exec(`
		CREATE TABLE documents (id INTEGER PRIMARY KEY, relative_path TEXT UNIQUE, language TEXT);
		CREATE TABLE global_symbols (id INTEGER PRIMARY KEY, symbol TEXT, display_name TEXT, kind INTEGER);
		CREATE TABLE defn_enclosing_ranges (id INTEGER PRIMARY KEY, document_id INTEGER, symbol_id INTEGER,
			start_line INTEGER, start_char INTEGER, end_line INTEGER, end_char INTEGER);
		CREATE TABLE mentions (chunk_id INTEGER, symbol_id INTEGER, role INTEGER);
	`)
	conn.Exec("INSERT INTO documents VALUES (1, 'src/a.ts', 'ts')")
	conn.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)",
		"scip-typescript npm t 1.0 src/`a.ts`/greet().", "greet")
	conn.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)",
		"scip-typescript npm t 1.0 src/`a.ts`/message.", "message")
	conn.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)",
		"scip-typescript npm t 1.0 src/`a.ts`/Options#typeLiteral0:verbose.", "verbose")
	conn.Exec("INSERT INTO mentions VALUES (1, 2, 0)")
	conn.Exec("INSERT INTO defn_enclosing_ranges VALUES (1, 1, 2, 0, 0, 0, 0)")
}

func TestPostprocessIndex_omitsVariables(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")
	seedIndexDB(t, dbPath)
	if err := postprocessIndex(dbPath); err != nil {
		t.Fatal(err)
	}
	db, _ := sql.Open("sqlite", dbPath)
	defer db.Close()
	rows, _ := db.Query("SELECT symbol FROM global_symbols")
	var syms []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		syms = append(syms, s)
	}
	hasGreet, hasVerbose, hasMessage := false, false, false
	for _, s := range syms {
		if strings.Contains(s, "greet().") {
			hasGreet = true
		}
		if strings.Contains(s, "typeLiteral0:verbose") {
			hasVerbose = true
		}
		if strings.HasSuffix(s, "/message.") {
			hasMessage = true
		}
	}
	if !hasGreet || !hasVerbose || hasMessage {
		t.Fatalf("syms=%v", syms)
	}
	var mentionCount int
	db.QueryRow("SELECT COUNT(*) FROM mentions").Scan(&mentionCount)
	if mentionCount != 0 {
		t.Fatalf("mentions=%d", mentionCount)
	}
}

func TestIndexerEnv_defaults(t *testing.T) {
	os.Unsetenv("SCIP_CLI_MAX_HEAP_MB")
	env, err := indexerEnv(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range env {
		if strings.HasPrefix(e, "NODE_OPTIONS=") && strings.Contains(e, "--max-old-space-size=") {
			found = true
		}
	}
	if !found {
		t.Fatal("missing NODE_OPTIONS heap")
	}
}

func TestIndexerEnv_configOverrides(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, config.ConfigFilename), []byte(`{"maxHeapMb": 12288}`), 0644); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("SCIP_CLI_MAX_HEAP_MB")
	env, err := indexerEnv(root)
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	for _, e := range env {
		if strings.Contains(e, "--max-old-space-size=12288") {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("env=%v", env)
	}
}

func TestIndexerEnv_envOverridesConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, config.ConfigFilename), []byte(`{"maxHeapMb": 12288}`), 0644); err != nil {
		t.Fatal(err)
	}
	os.Setenv("SCIP_CLI_MAX_HEAP_MB", "4096")
	defer os.Unsetenv("SCIP_CLI_MAX_HEAP_MB")
	env, err := indexerEnv(root)
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	for _, e := range env {
		if strings.Contains(e, "--max-old-space-size=4096") {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("env=%v", env)
	}
}

func TestIndexerEnv_invalidHeap(t *testing.T) {
	os.Setenv("SCIP_CLI_MAX_HEAP_MB", "not-a-number")
	defer os.Unsetenv("SCIP_CLI_MAX_HEAP_MB")
	_, err := indexerEnv(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "SCIP_CLI_MAX_HEAP_MB") {
		t.Fatalf("err=%v", err)
	}
}

func TestFormatDbSize(t *testing.T) {
	dir := t.TempDir()
	tiny := filepath.Join(dir, "tiny.db")
	if err := os.WriteFile(tiny, bytesRepeat('x', 512), 0644); err != nil {
		t.Fatal(err)
	}
	if formatDBSize(tiny) != "512 B" {
		t.Fatal(formatDBSize(tiny))
	}
	kb := filepath.Join(dir, "kb.db")
	if err := os.WriteFile(kb, bytesRepeat('x', 2048), 0644); err != nil {
		t.Fatal(err)
	}
	if formatDBSize(kb) != "2.0 KB" {
		t.Fatal(formatDBSize(kb))
	}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func TestTypescriptProjects_mergesConfigRoots(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "package.json"), `{"workspaces": ["packages/api"]}`)
	writeJSON(t, filepath.Join(root, "tsconfig.json"), `{"include": ["*.ts"]}`)
	writeJSON(t, filepath.Join(root, "packages", "api", "tsconfig.json"), `{"include": ["src/**/*.ts"]}`)
	writeJSON(t, filepath.Join(root, "packages", "worker", "tsconfig.json"), `{"include": ["src/**/*.ts"]}`)
	writeJSON(t, filepath.Join(root, config.ConfigFilename), `{"indexRoots": ["packages/worker"]}`)
	projects, err := typescriptProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	hasAPI, hasWorker := false, false
	for _, p := range projects {
		if p == "packages/api" {
			hasAPI = true
		}
		if p == "packages/worker" {
			hasWorker = true
		}
	}
	if !hasAPI || !hasWorker {
		t.Fatalf("projects=%v", projects)
	}
}

func TestTypescriptProjects_onlyIndexRoots(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "package.json"), `{"workspaces": ["packages/api"]}`)
	writeJSON(t, filepath.Join(root, "packages", "worker", "tsconfig.json"), `{"include": ["src/**/*.ts"]}`)
	writeJSON(t, filepath.Join(root, config.ConfigFilename), `{"indexRoots": ["packages/worker"], "onlyIndexRoots": true}`)
	projects, err := typescriptProjects(root)
	if err != nil || len(projects) != 1 || projects[0] != "packages/worker" {
		t.Fatalf("projects=%v err=%v", projects, err)
	}
}

func TestTypescriptProjects_filteredByScope(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "package.json"), `{"workspaces": ["packages/api"]}`)
	writeJSON(t, filepath.Join(root, "packages", "api", "tsconfig.json"), `{"include": ["src/**/*.ts"]}`)
	writeJSON(t, filepath.Join(root, "packages", "worker", "tsconfig.json"), `{"include": ["src/**/*.ts"]}`)
	if err := scope.SaveIndexScope(root, []string{"packages/worker"}); err != nil {
		t.Fatal(err)
	}
	projects, err := typescriptProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0] != "packages/worker" {
		t.Fatalf("projects=%v", projects)
	}
}

func TestLogIndexComplete(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")
	if err := os.WriteFile(dbPath, make([]byte, 1024), 0644); err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	logIndexComplete(dbPath, "typescript", 3, 1)
	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	err := buf.String()
	if !strings.Contains(err, "Indexed") || !strings.Contains(err, "1.0 KB") ||
		!strings.Contains(err, "typescript") || !strings.Contains(err, "3 tsconfigs") ||
		!strings.Contains(err, "1 skipped") {
		t.Fatalf("stderr=%q", err)
	}
}

func writeJSON(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
