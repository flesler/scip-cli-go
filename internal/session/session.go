package session

import (
	"database/sql"
	"fmt"

	"github.com/flesler/scip-cli-go/internal/clierr"
	"github.com/flesler/scip-cli-go/internal/config"
	"github.com/flesler/scip-cli-go/internal/indexing"
	"github.com/flesler/scip-cli-go/internal/output"
	"github.com/flesler/scip-cli-go/internal/project"
	"github.com/flesler/scip-cli-go/internal/queries"
	"github.com/flesler/scip-cli-go/internal/symbols"
)

// TestSetup, when set, overrides Setup for integration tests.
var TestSetup func() (*sql.DB, string, error)

func Setup() (*sql.DB, string, error) {
	if TestSetup != nil {
		return TestSetup()
	}
	projectRoot, ok := project.FindProjectRoot("")
	if !ok {
		clierr.FatalMsg("Error: Could not find project root")
	}

	if _, err := config.LoadProjectConfig(projectRoot); err != nil {
		clierr.Fatal(err)
	}

	db, err := indexing.GetDB(projectRoot)
	if err != nil {
		clierr.Fatal(err)
	}

	return db, projectRoot, nil
}

func ResolveOneSymbol(db *sql.DB, name string, kindFilter *symbols.SymbolKind, pathScope string) (queries.SymbolResult, error) {
	limit := 2
	symbols, err := queries.ResolveSymbol(db, name, kindFilter, &limit, pathScope)
	if err != nil {
		return queries.SymbolResult{}, err
	}

	if len(symbols) == 0 {
		return queries.SymbolResult{}, fmt.Errorf("symbol '%s' not found", name)
	}

	matches := make([]interface{}, len(symbols))
	for i, s := range symbols {
		matches[i] = []interface{}{s.ID, s.Symbol, s.DisplayName}
	}
	output.WarnAmbiguous(name, matches, "symbol")

	return symbols[0], nil
}

func ResolveOneFile(db *sql.DB, pattern string, pathScope string) (string, error) {
	files, err := queries.ResolveFile(db, pattern, pathScope)
	if err != nil {
		return "", err
	}

	if len(files) == 0 {
		return "", fmt.Errorf("file '%s' not found", pattern)
	}

	matches := make([]interface{}, len(files))
	for i, f := range files {
		matches[i] = f
	}
	output.WarnAmbiguous(pattern, matches, "file")

	return files[0], nil
}
