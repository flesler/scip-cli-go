package commands

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flesler/scip-cli-go/v2/internal/symbols"

	_ "modernc.org/sqlite"
)

func TestParseSymbol_function(t *testing.T) {
	s := "scip-typescript npm sample-app 1.0 src/`helper.ts`/greet()."
	path, name := parseSymbol(s)
	if path != "src/helper.ts" || name != "greet()" {
		t.Fatalf("path=%q name=%q", path, name)
	}
}

func TestParseSymbol_nestedPath(t *testing.T) {
	s := "scip-typescript npm sample-app 1.0 src/components/ui/`btn.tsx`/Btn#"
	path, name := parseSymbol(s)
	if path != "src/components/ui/btn.tsx" || name != "Btn#" {
		t.Fatalf("path=%q name=%q", path, name)
	}
}

func TestIsNoisySymbol(t *testing.T) {
	fileLevel := "scip-typescript npm sample-app 1.0 src/`helper.ts`/"
	if !isNoisySymbol(fileLevel) {
		t.Fatal("file level")
	}
	param := "scip-typescript npm sample-app 1.0 src/`helper.ts`/greet().(err)"
	if !isNoisySymbol(param) {
		t.Fatal("parameter")
	}
	fn := "scip-typescript npm sample-app 1.0 src/`helper.ts`/greet()."
	if isNoisySymbol(fn) {
		t.Fatal("normal function")
	}
}

func TestKindToDisplay(t *testing.T) {
	if kindToDisplay(symbols.KindClass) != "class" {
		t.Fatal(kindToDisplay(symbols.KindClass))
	}
}

func TestLeafAppearsOnLine(t *testing.T) {
	if leafAppearsOnLine("id", "const valid = 1") {
		t.Fatal("substring id")
	}
	if !leafAppearsOnLine("id", "const id = 1") {
		t.Fatal("word id")
	}
	if !leafAppearsOnLine("run", "foo.run()") {
		t.Fatal("qualified run")
	}
	if leafAppearsOnLine("run", "truncate()") {
		t.Fatal("substring run")
	}
}

func TestGetExactRefs_singleReference(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "test.py"), []byte("def foo():\n    pass\n\nfoo()\n"), 0644)
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.Exec(`CREATE TABLE global_symbols (id INTEGER PRIMARY KEY, symbol TEXT, display_name TEXT)`)
	db.Exec(`CREATE TABLE documents (id INTEGER PRIMARY KEY, relative_path TEXT)`)
	db.Exec(`CREATE TABLE chunks (id INTEGER PRIMARY KEY, document_id INTEGER, start_line INTEGER, end_line INTEGER)`)
	db.Exec(`CREATE TABLE mentions (chunk_id INTEGER, symbol_id INTEGER, role INTEGER)`)
	db.Exec("INSERT INTO global_symbols VALUES (1, 'scip-python test/test `test.py`/foo().', 'foo')")
	db.Exec("INSERT INTO documents VALUES (1, 'test.py')")
	db.Exec("INSERT INTO chunks VALUES (1, 1, 3, 3)")
	db.Exec("INSERT INTO mentions VALUES (1, 1, 0)")
	refs := getExactRefs(db, 1, root, 10, "")
	if len(refs) != 1 || refs[0] != "test.py:4" {
		t.Fatalf("refs=%v", refs)
	}
}

func TestQualifiedPattern(t *testing.T) {
	if !qualifiedPattern("Options.verbose") {
		t.Fatal("qualified")
	}
	if qualifiedPattern("src/foo.ts") {
		t.Fatal("path not qualified")
	}
	if strings.Contains("greet", ".") && qualifiedPattern("greet") {
		t.Fatal("bare name")
	}
}
