package queries_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/flesler/scip-cli-go/internal/analyze/testdb"
	"github.com/flesler/scip-cli-go/internal/queries"
	"github.com/flesler/scip-cli-go/internal/symbols"

	_ "modernc.org/sqlite"
)

func seedSymbols(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE global_symbols (id INTEGER PRIMARY KEY, symbol TEXT, display_name TEXT)`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestResolveSymbol_exactFunction(t *testing.T) {
	db := seedSymbols(t)
	defer db.Close()
	db.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)",
		"scip-typescript npm test 1.0 src/`test.ts`/myFunc().", "myFunc")
	res, err := queries.ResolveSymbol(db, "myFunc", nil, nil, "")
	if err != nil || len(res) != 1 || res[0].DisplayName.String != "myFunc" {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestResolveSymbol_qualifiedClassMethod(t *testing.T) {
	db := seedSymbols(t)
	defer db.Close()
	db.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)",
		"scip-typescript npm test 1.0 src/`widget.ts`/Widget#run().", "run")
	db.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)",
		"scip-typescript npm test 1.0 src/`other.module.ts`/OtherModule#onModuleInit().", "onModuleInit")
	res, err := queries.ResolveSymbol(db, "Widget.run", nil, nil, "")
	if err != nil || len(res) != 1 || !strings.Contains(res[0].Symbol, "Widget#run") {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestResolveSymbol_qualifiedTypeLiteralField(t *testing.T) {
	db := seedSymbols(t)
	defer db.Close()
	db.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)",
		"scip-typescript npm test 1.0 src/`helper.ts`/Options#typeLiteral0:verbose.", "verbose")
	db.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)",
		"scip-typescript npm test 1.0 src/`other.ts`/Other#typeLiteral0:verbose.", "verbose")
	res, err := queries.ResolveSymbol(db, "Options.verbose", nil, nil, "")
	if err != nil || len(res) != 1 || !strings.Contains(res[0].Symbol, "Options#typeLiteral0:verbose") {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestResolveSymbol_kindFilter(t *testing.T) {
	db := seedSymbols(t)
	defer db.Close()
	db.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)",
		"scip-typescript npm test 1.0 src/`test.ts`/myFunc().", "myFunc")
	db.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)",
		"scip-typescript npm test 1.0 src/`test.ts`/MyClass#", "MyClass")
	kind := symbols.KindFunction
	res, err := queries.ResolveSymbol(db, "my", &kind, nil, "")
	if err != nil || len(res) != 1 || res[0].DisplayName.String != "myFunc" {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestResolveFile_bareFilenamePrefersNonTest(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.Exec(`CREATE TABLE documents (id INTEGER PRIMARY KEY, relative_path TEXT)`)
	db.Exec("INSERT INTO documents (relative_path) VALUES (?), (?)",
		"pkg/src/helper.ts", "pkg/src/helper.test.ts")
	res, err := queries.ResolveFile(db, "helper.ts", "")
	if err != nil || len(res) == 0 || res[0] != "pkg/src/helper.ts" {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestGetMembers_classMethods(t *testing.T) {
	b, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	widgetID := b.Define("src/widget.ts", "Widget", 0, 10)
	b.Method("src/widget.ts", "Widget", "run", 0, 10)
	b.Method("src/widget.ts", "Widget", "stop", 0, 10)
	db := b.Finish()
	members, _, err := queries.GetMembers(db, widgetID)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, m := range members {
		names[symbols.ExtractLeafName(m.Symbol)] = true
	}
	if !names["run"] || !names["stop"] {
		t.Fatalf("members=%v", names)
	}
}

func TestGetMembers_typeLiteralFields(t *testing.T) {
	b, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	optsID := b.Define("src/helper.ts", "Options", 0, 10)
	b.TypeLiteralField("src/helper.ts", "Options", "verbose", 0)
	b.TypeLiteralField("src/helper.ts", "Options", "debug", 0)
	db := b.Finish()
	members, _, err := queries.GetMembers(db, optsID)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, m := range members {
		names[symbols.ExtractLeafName(m.Symbol)] = true
	}
	if !names["verbose"] || !names["debug"] {
		t.Fatalf("members=%v", names)
	}
}

func TestGetMembers_excludesParameters(t *testing.T) {
	b, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	fooID := b.Define("src/x.ts", "Foo", 0, 10)
	methodID := b.Method("src/x.ts", "Foo", "bar", 0, 10)
	db := b.Finish()
	paramSym := "scip-typescript npm test 1.0 src/x.ts/`x.ts`/Foo#bar().(eventIds)"
	db.Exec("INSERT INTO global_symbols (id, symbol, display_name) VALUES (?, ?, ?)", 99, paramSym, "eventIds")

	members, _, err := queries.GetMembers(db, fooID)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[int]bool{}
	for _, m := range members {
		ids[m.ID] = true
	}
	if !ids[methodID] || ids[99] {
		t.Fatalf("ids=%v method=%d", ids, methodID)
	}
}

func TestResolveSymbol_exactClass(t *testing.T) {
	db := seedSymbols(t)
	defer db.Close()
	db.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)",
		"scip-typescript npm test 1.0 src/`test.ts`/MyClass#", "MyClass")
	res, err := queries.ResolveSymbol(db, "MyClass", nil, nil, "")
	if err != nil || len(res) != 1 || res[0].DisplayName.String != "MyClass" {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestResolveSymbol_noMatch(t *testing.T) {
	db := seedSymbols(t)
	defer db.Close()
	res, err := queries.ResolveSymbol(db, "missing", nil, nil, "")
	if err != nil || len(res) != 0 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestResolveSymbol_kindFilterNoMatch(t *testing.T) {
	db := seedSymbols(t)
	defer db.Close()
	db.Exec("INSERT INTO global_symbols (symbol, display_name) VALUES (?, ?)",
		"scip-typescript npm test 1.0 src/`test.ts`/myFunc().", "myFunc")
	kind := symbols.KindClass
	res, err := queries.ResolveSymbol(db, "myFunc", &kind, nil, "")
	if err != nil || len(res) != 0 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestResolveFile_exactMatch(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.Exec(`CREATE TABLE documents (id INTEGER PRIMARY KEY, relative_path TEXT)`)
	db.Exec("INSERT INTO documents (relative_path) VALUES (?)", "pkg/src/helper.ts")
	res, err := queries.ResolveFile(db, "pkg/src/helper.ts", "")
	if err != nil || len(res) != 1 || res[0] != "pkg/src/helper.ts" {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestResolveFile_patternMatch(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.Exec(`CREATE TABLE documents (id INTEGER PRIMARY KEY, relative_path TEXT)`)
	db.Exec("INSERT INTO documents (relative_path) VALUES (?), (?)",
		"pkg/src/helper.ts", "pkg/src/other.ts")
	res, err := queries.ResolveFile(db, "helper", "")
	if err != nil || len(res) == 0 || res[0] != "pkg/src/helper.ts" {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestResolveFile_noMatch(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.Exec(`CREATE TABLE documents (id INTEGER PRIMARY KEY, relative_path TEXT)`)
	res, err := queries.ResolveFile(db, "missing.ts", "")
	if err != nil || len(res) != 0 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}
