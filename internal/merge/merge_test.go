package merge

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

const mergeSchema = `
CREATE TABLE documents (
    id INTEGER PRIMARY KEY,
    relative_path TEXT NOT NULL UNIQUE
);
CREATE TABLE chunks (
    id INTEGER PRIMARY KEY,
    document_id INTEGER NOT NULL,
    chunk_index INTEGER NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    occurrences BLOB NOT NULL
);
CREATE TABLE global_symbols (
    id INTEGER PRIMARY KEY,
    symbol TEXT NOT NULL UNIQUE,
    display_name TEXT,
    kind INTEGER
);
CREATE TABLE mentions (
    chunk_id INTEGER NOT NULL,
    symbol_id INTEGER NOT NULL,
    role INTEGER NOT NULL,
    PRIMARY KEY (chunk_id, symbol_id, role)
);
CREATE TABLE defn_enclosing_ranges (
    id INTEGER PRIMARY KEY,
    document_id INTEGER NOT NULL,
    symbol_id INTEGER NOT NULL,
    start_line INTEGER NOT NULL,
    start_char INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    end_char INTEGER NOT NULL
);
`

func makePartDB(t *testing.T, dir, name, relPath, symbol string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec(mergeSchema); err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec("INSERT INTO documents (relative_path) VALUES (?)", relPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)",
		symbol, symbol)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(`
		INSERT INTO chunks (document_id, chunk_index, start_line, end_line, occurrences)
		VALUES (1, 0, 0, 0, X'00')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec("INSERT INTO mentions (chunk_id, symbol_id, role) VALUES (1, 1, 0)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(`
		INSERT INTO defn_enclosing_ranges (
			document_id, symbol_id, start_line, start_char, end_line, end_char
		) VALUES (1, 1, 0, 0, 1, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMergeTwoIndexes(t *testing.T) {
	dir := t.TempDir()
	first := makePartDB(t, dir, "first.db", "src/a.ts", "scheme a/symA().")
	second := makePartDB(t, dir, "second.db", "src/b.ts", "scheme b/symB().")
	output := filepath.Join(dir, "merged.db")

	if err := MergeSQLiteIndexes([]string{first, second}, output); err != nil {
		t.Fatal(err)
	}
	conn, err := sql.Open("sqlite", output)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	var docs []string
	rows, err := conn.Query("SELECT relative_path FROM documents ORDER BY 1")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var p string
		rows.Scan(&p)
		docs = append(docs, p)
	}
	rows.Close()

	var symCount int
	conn.QueryRow("SELECT COUNT(*) FROM global_symbols").Scan(&symCount)
	if len(docs) != 2 || docs[0] != "src/a.ts" || docs[1] != "src/b.ts" {
		t.Fatalf("docs=%v", docs)
	}
	if symCount != 2 {
		t.Fatalf("symbols=%d", symCount)
	}
}

func TestMergeSkipsVariableSymbols(t *testing.T) {
	dir := t.TempDir()
	first := makePartDB(t, dir, "first.db", "src/a.ts", "scheme a/foo().")
	second := filepath.Join(dir, "second.db")
	conn, err := sql.Open("sqlite", second)
	if err != nil {
		t.Fatal(err)
	}
	conn.Exec(mergeSchema)
	conn.Exec("INSERT INTO documents (relative_path) VALUES (?)", "src/b.ts")
	conn.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)",
		"scheme b/message.", "message")
	conn.Exec(`INSERT INTO chunks (document_id, chunk_index, start_line, end_line, occurrences) VALUES (1, 0, 0, 0, X'00')`)
	conn.Exec("INSERT INTO mentions (chunk_id, symbol_id, role) VALUES (1, 1, 0)")
	conn.Close()

	output := filepath.Join(dir, "merged.db")
	if err := MergeSQLiteIndexes([]string{first, second}, output); err != nil {
		t.Fatal(err)
	}
	conn, _ = sql.Open("sqlite", output)
	defer conn.Close()
	var sym string
	conn.QueryRow("SELECT symbol FROM global_symbols").Scan(&sym)
	if sym != "scheme a/foo()." {
		t.Fatalf("got %q", sym)
	}
}

func TestMergeRequiresInputs(t *testing.T) {
	err := MergeSQLiteIndexes(nil, filepath.Join(t.TempDir(), "out.db"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMergeReusesDuplicateSymbols(t *testing.T) {
	dir := t.TempDir()
	shared := "scheme shared/sym()."
	first := makePartDB(t, dir, "first.db", "src/a.ts", shared)
	second := makePartDB(t, dir, "second.db", "src/b.ts", shared)
	output := filepath.Join(dir, "merged.db")
	if err := MergeSQLiteIndexes([]string{first, second}, output); err != nil {
		t.Fatal(err)
	}
	conn, _ := sql.Open("sqlite", output)
	defer conn.Close()
	var n int
	conn.QueryRow("SELECT COUNT(*) FROM global_symbols").Scan(&n)
	if n != 1 {
		t.Fatalf("expected 1 symbol, got %d", n)
	}
}

func TestMergeReusesDuplicateChunks(t *testing.T) {
	dir := t.TempDir()
	shared := "scheme shared/sym()."
	first := makePartDB(t, dir, "first.db", "src/shared.ts", shared)
	second := makePartDB(t, dir, "second.db", "src/shared.ts", shared)
	output := filepath.Join(dir, "merged.db")
	if err := MergeSQLiteIndexes([]string{first, second}, output); err != nil {
		t.Fatal(err)
	}
	conn, _ := sql.Open("sqlite", output)
	defer conn.Close()
	var n int
	conn.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&n)
	if n != 1 {
		t.Fatalf("expected 1 chunk, got %d", n)
	}
}

func TestMergePreservesMentionsOnDuplicateChunks(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.db")
	second := filepath.Join(dir, "second.db")
	output := filepath.Join(dir, "merged.db")

	conn, err := sql.Open("sqlite", first)
	if err != nil {
		t.Fatal(err)
	}
	conn.Exec(mergeSchema)
	conn.Exec("INSERT INTO documents (relative_path) VALUES (?)", "src/shared.ts")
	conn.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)", "scheme a/symA().", "symA")
	conn.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)", "scheme a/symB().", "symB")
	conn.Exec(`INSERT INTO chunks (document_id, chunk_index, start_line, end_line, occurrences) VALUES (1, 0, 0, 0, X'00')`)
	conn.Exec("INSERT INTO mentions (chunk_id, symbol_id, role) VALUES (1, 1, 0)")
	conn.Close()

	conn, err = sql.Open("sqlite", second)
	if err != nil {
		t.Fatal(err)
	}
	conn.Exec(mergeSchema)
	conn.Exec("INSERT INTO documents (relative_path) VALUES (?)", "src/shared.ts")
	conn.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)", "scheme b/symB().", "symB")
	conn.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)", "scheme c/symC().", "symC")
	conn.Exec(`INSERT INTO chunks (document_id, chunk_index, start_line, end_line, occurrences) VALUES (1, 0, 0, 0, X'00')`)
	conn.Exec("INSERT INTO mentions (chunk_id, symbol_id, role) VALUES (1, 2, 0)")
	conn.Close()

	if err := MergeSQLiteIndexes([]string{first, second}, output); err != nil {
		t.Fatal(err)
	}
	conn, _ = sql.Open("sqlite", output)
	defer conn.Close()
	var mentionCount int
	conn.QueryRow("SELECT COUNT(*) FROM mentions").Scan(&mentionCount)
	rows, err := conn.Query(`
		SELECT gs.symbol FROM mentions m
		JOIN global_symbols gs ON m.symbol_id = gs.id`)
	if err != nil {
		t.Fatal(err)
	}
	symbols := map[string]bool{}
	for rows.Next() {
		var sym string
		rows.Scan(&sym)
		symbols[sym] = true
	}
	rows.Close()
	if mentionCount != 2 {
		t.Fatalf("mentions=%d", mentionCount)
	}
	if !symbols["scheme a/symA()."] || !symbols["scheme c/symC()."] {
		t.Fatalf("symbols=%v", symbols)
	}
}

// silence unused import
var _ = os.IsNotExist
