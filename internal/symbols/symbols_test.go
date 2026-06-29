package symbols_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/sourcegraph/scip-cli-go/internal/sqlhelp"
	"github.com/sourcegraph/scip-cli-go/internal/symbols"

	_ "modernc.org/sqlite"
)

func TestExtractLeafName_cases(t *testing.T) {
	cases := map[string]string{
		"scip-typescript npm sample-app 1.0 src/`helper.ts`/greet().":                              "greet",
		"scip-typescript npm sample-app 1.0 src/`helper.ts`/WidgetOptions#":                        "WidgetOptions",
		"scip-typescript npm sample-app 1.0 src/`helper.ts`/someVar.":                              "someVar",
		"scip-typescript npm sample-app 1.0 src/`helper.ts`/WidgetOptions#typeLiteral0:onVerbose.": "onVerbose",
		"scip-typescript npm battler 1.0.0 src/`GameEngine.ts`/GameEngine#config.":                 "config",
		"scip-typescript npm battler 1.0.0 src/`GameEngine.ts`/GameEngine#damageHero().":           "damageHero",
		"scip-typescript npm battler 1.0.0 src/`GameEngine.ts`/GameEngine#`<get>aliveHeroes`().":   "aliveHeroes",
		"scip-typescript npm battler 1.0.0 src/`GameEngine.ts`/GameEngine#`<set>value`().":         "value",
		"scip-typescript npm battler 1.0.0 src/`GameEngine.ts`/GameEngine#`<constructor>`().":      "<constructor>",
	}
	for sym, want := range cases {
		if got := symbols.ExtractLeafName(sym); got != want {
			t.Fatalf("%q: got %q want %q", sym, got, want)
		}
	}
}

func TestIsModuleSymbol(t *testing.T) {
	if !symbols.IsModuleSymbol("scip-typescript npm app 1.0 src/`m.ts`/") {
		t.Fatal("expected module symbol")
	}
}

func TestIsTypeOrInterfaceSymbol(t *testing.T) {
	if !symbols.IsTypeOrInterfaceSymbol("scip-typescript npm app 1.0 src/`t.ts`/Foo#") {
		t.Fatal("Foo#")
	}
	if symbols.IsTypeOrInterfaceSymbol("scip-typescript npm app 1.0 src/`t.ts`/foo().") {
		t.Fatal("function should not be type")
	}
}

func TestInferKind_cases(t *testing.T) {
	cases := []struct {
		sym  string
		want symbols.SymbolKind
	}{
		{"scip-typescript npm sample-app 1.0 src/`helper.ts`/greet().", symbols.KindFunction},
		{"scip-typescript npm battler 1.0.0 src/`GameEngine.ts`/GameEngine#damageHero().", symbols.KindMethod},
		{"scip-typescript npm sample-app 1.0 src/`helper.ts`/WidgetOptions#", symbols.KindClass},
		{"scip-typescript npm sample-app 1.0 src/`helper.ts`/someVar.", symbols.KindUnknown},
		{"scip-typescript npm sample-app 1.0 src/`helper.ts`/WidgetOptions#typeLiteral0:onVerbose.", symbols.KindProperty},
		{"scip-python pip mypkg 1.0 src/module.py/MyClass#method().", symbols.KindMethod},
	}
	for _, c := range cases {
		if got := symbols.InferKind(c.sym); got != c.want {
			t.Fatalf("%q: got %q want %q", c.sym, got, c.want)
		}
	}
}

func TestParseFilterableKind(t *testing.T) {
	_, err := symbols.ParseFilterableKind("bogus")
	if err == nil {
		t.Fatal("expected error for bogus kind")
	}
	k, err := symbols.ParseFilterableKind("class")
	if err != nil || k == nil || *k != symbols.KindClass {
		t.Fatalf("class: k=%v err=%v", k, err)
	}
	if _, err := symbols.ParseFilterableKind(""); err != nil {
		t.Fatal("empty kind should be nil")
	}
}

func TestKindSQLClause(t *testing.T) {
	if !strings.Contains(symbols.KindSQLClause(symbols.KindClass), "LIKE '%#'") {
		t.Fatal("class clause")
	}
	fn := symbols.KindSQLClause(symbols.KindFunction)
	if !strings.Contains(fn, "LIKE '%().'") || !strings.Contains(fn, "NOT LIKE '%#%().'") {
		t.Fatal("function clause")
	}
	if symbols.KindSQLClause(symbols.KindUnknown) != "" {
		t.Fatal("unknown should be empty")
	}
}

func TestEscapeLike(t *testing.T) {
	if sqlhelp.EscapeLike("foo%bar") != `foo\%bar` {
		t.Fatal("percent")
	}
	if sqlhelp.EscapeLike("foo_bar") != `foo\_bar` {
		t.Fatal("underscore")
	}
}

func TestSymbolMatchesQualifier(t *testing.T) {
	optVerbose := "scip-typescript npm test 1.0 src/helper.ts/`helper.ts`/Options#typeLiteral0:verbose."
	if !symbols.SymbolMatchesQualifier("scip-typescript npm test 1.0 src/`widget.ts`/Widget#run().", []string{"Widget"}, "run") {
		t.Fatal("class method")
	}
	if !symbols.SymbolMatchesQualifier(optVerbose, []string{"Options"}, "verbose") {
		t.Fatal("type literal field")
	}
	if symbols.SymbolMatchesQualifier(optVerbose, []string{"Options"}, "quiet") {
		t.Fatal("wrong leaf")
	}
	if symbols.SymbolMatchesQualifier(optVerbose, []string{"Other"}, "verbose") {
		t.Fatal("wrong container")
	}
}

func TestIsVariableSymbol(t *testing.T) {
	if !symbols.IsVariableSymbol("scip-typescript npm t 1.0 src/`a.ts`/message.") {
		t.Fatal("const")
	}
	if symbols.IsVariableSymbol("scip-typescript npm t 1.0 src/`a.ts`/greet().") {
		t.Fatal("function")
	}
}

func TestSQLExcludeVariableSymbols(t *testing.T) {
	clause := symbols.SQLExcludeVariableSymbols("symbol")
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	for sym, want := range map[string]int{
		"scip-typescript npm t 1.0 src/`a.ts`/message.": 0,
		"scip-typescript npm t 1.0 src/`a.ts`/greet().": 1,
	} {
		var ok int
		db.QueryRow("SELECT "+clause+" FROM (SELECT ? AS symbol)", sym).Scan(&ok)
		if ok != want {
			t.Fatalf("%q: got %d want %d", sym, ok, want)
		}
	}
}
