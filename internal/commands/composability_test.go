package commands

import (
	"bytes"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestPrintSearchResults_namesOnly(t *testing.T) {
	results := []searchResult{
		{filePath: "pkg/a.ts", line: 11, kind: "function", name: "foo"},
		{filePath: "pkg/b.ts", line: 4, kind: "function", name: "bar"},
	}
	out := captureStdout(func() {
		printSearchResults(results, true, false)
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || lines[0] != "foo" || lines[1] != "bar" {
		t.Fatalf("out=%q", out)
	}
}

func TestPrintSearchResults_pathsOnly(t *testing.T) {
	results := []searchResult{
		{filePath: "pkg/a.ts", name: "foo"},
		{filePath: "pkg/a.ts", name: "bar"},
		{filePath: "pkg/b.ts", name: "baz"},
	}
	out := captureStdout(func() {
		printSearchResults(results, false, true)
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || lines[0] != "pkg/a.ts" || lines[1] != "pkg/b.ts" {
		t.Fatalf("out=%q", out)
	}
}

func TestRefsPathsOnly_dedupesPaths(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.Exec(`CREATE TABLE global_symbols (id INTEGER PRIMARY KEY, symbol TEXT, display_name TEXT)`)
	db.Exec(`CREATE TABLE documents (id INTEGER PRIMARY KEY, relative_path TEXT)`)
	db.Exec(`CREATE TABLE chunks (id INTEGER PRIMARY KEY, document_id INTEGER, start_line INTEGER, end_line INTEGER)`)
	db.Exec(`CREATE TABLE mentions (chunk_id INTEGER, symbol_id INTEGER, role INTEGER)`)
	db.Exec("INSERT INTO documents (relative_path) VALUES ('pkg/a.ts'), ('pkg/b.ts')")
	db.Exec("INSERT INTO global_symbols (symbol) VALUES ('scip-typescript npm app 1.0 pkg/`a.ts`/foo().')")
	db.Exec("INSERT INTO chunks VALUES (1, 1, 20, 20), (2, 2, 30, 30)")
	db.Exec("INSERT INTO mentions VALUES (1, 1, 0), (2, 1, 0)")

	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "pkg"), 0755)
	_ = os.WriteFile(filepath.Join(root, "pkg", "a.ts"), []byte("foo()\n"), 0644)
	_ = os.WriteFile(filepath.Join(root, "pkg", "b.ts"), []byte("foo()\n"), 0644)

	refs := getExactRefs(db, 1, root, 10, "")
	if len(refs) != 2 {
		t.Fatalf("refs=%v", refs)
	}
	unique := map[string]bool{}
	for _, r := range refs {
		parts := strings.Split(r, ":")
		unique[parts[0]] = true
	}
	if len(unique) != 2 || !unique["pkg/a.ts"] || !unique["pkg/b.ts"] {
		t.Fatalf("unique=%v refs=%v", unique, refs)
	}
}
