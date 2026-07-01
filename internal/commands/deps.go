package commands

import (
	"database/sql"
	"fmt"
	"os"
	"sort"

	"github.com/flesler/scip-cli-go/v2/internal/clierr"
	"github.com/flesler/scip-cli-go/v2/internal/output"
	"github.com/flesler/scip-cli-go/v2/internal/paths"
	"github.com/flesler/scip-cli-go/v2/internal/queries"
	"github.com/flesler/scip-cli-go/v2/internal/session"
	"github.com/flesler/scip-cli-go/v2/internal/sqlhelp"
	"github.com/flesler/scip-cli-go/v2/internal/symbols"
	"github.com/flesler/scip-cli-go/v2/internal/targets"
)

func depsFromSymbol(db *sql.DB, symbolID int, limit int) ([]queries.SymbolResult, error) {
	var docID int
	var startLine, endLine int
	err := sqlhelp.DebugExecuteOne(db, `
		SELECT document_id, start_line, end_line
		FROM defn_enclosing_ranges
		WHERE symbol_id = ?
		LIMIT 1
	`, symbolID).Scan(&docID, &startLine, &endLine)
	if err != nil {
		return nil, nil
	}

	limitPlusOne := limit + 1
	rows, err := sqlhelp.DebugExecute(db, `
		SELECT DISTINCT gs.id, gs.symbol, gs.display_name
		FROM mentions m
		JOIN chunks c ON m.chunk_id = c.id
		JOIN global_symbols gs ON m.symbol_id = gs.id
		JOIN defn_enclosing_ranges def_der ON def_der.symbol_id = gs.id
		JOIN documents def_d ON def_der.document_id = def_d.id
		WHERE c.document_id = ?
		  AND c.start_line <= ?
		  AND c.end_line >= ?
		  AND m.role != 1
		  AND gs.id != ?
		ORDER BY gs.symbol
		LIMIT ?
	`, docID, endLine, startLine, symbolID, limitPlusOne)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []queries.SymbolResult
	for rows.Next() {
		var r queries.SymbolResult
		if err := rows.Scan(&r.ID, &r.Symbol, &r.DisplayName); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func depsFromFile(db *sql.DB, filePath string, limit int) ([]queries.SymbolResult, error) {
	var docID int
	err := sqlhelp.DebugExecuteOne(db, `
		SELECT id FROM documents WHERE relative_path = ?
	`, filePath).Scan(&docID)
	if err != nil {
		return nil, nil
	}

	limitPlusOne := limit + 1
	rows, err := sqlhelp.DebugExecute(db, `
		SELECT DISTINCT gs.id, gs.symbol, gs.display_name
		FROM mentions m
		JOIN chunks c ON m.chunk_id = c.id
		JOIN global_symbols gs ON m.symbol_id = gs.id
		JOIN defn_enclosing_ranges def_der ON def_der.symbol_id = gs.id
		JOIN documents def_d ON def_der.document_id = def_d.id
		WHERE c.document_id = ?
		  AND m.role != 1
		  AND gs.id NOT IN (
		      SELECT der2.symbol_id FROM defn_enclosing_ranges der2
		      WHERE der2.document_id = ?
		  )
		ORDER BY gs.symbol
		LIMIT ?
	`, docID, docID, limitPlusOne)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []queries.SymbolResult
	for rows.Next() {
		var r queries.SymbolResult
		if err := rows.Scan(&r.ID, &r.Symbol, &r.DisplayName); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func DepsMain(args map[string]interface{}) error {
	db, _, err := session.Setup()
	if err != nil {
		return err
	}
	defer db.Close()

	pathScope := args["path_scope"].(string)
	limit := args["limit"].(int)
	target := args["target"].(string)
	pathsOnly := args["paths_only"].(bool)

	var deps []queries.SymbolResult
	targetLabel := target

	if targets.LooksLikeFileTarget(target) {
		filePath, err := session.ResolveOneFile(db, target, pathScope)
		if err != nil {
			return err
		}
		deps, err = depsFromFile(db, filePath, limit)
		if err != nil {
			return err
		}
		targetLabel = filePath
	} else {
		sym, err := session.ResolveOneSymbol(db, target, nil, pathScope)
		if err != nil {
			return err
		}
		deps, err = depsFromSymbol(db, sym.ID, limit)
		if err != nil {
			return err
		}
	}

	if len(deps) == 0 {
		fmt.Fprintf(os.Stderr, "No dependencies found for '%s'\n", targetLabel)
		return clierr.Exit(1)
	}

	deps = output.LimitAndWarn(deps, limit, "dependencies")

	if pathsOnly {
		seenPaths := make(map[string]bool)
		for _, dep := range deps {
			path, _, _, err := queries.GetDefLocation(db, dep.ID)
			if err != nil {
				return err
			}
			if path != "" && !seenPaths[path] && paths.PathInScope(path, pathScope) {
				seenPaths[path] = true
			}
		}

		if len(seenPaths) == 0 {
			fmt.Fprintf(os.Stderr, "No dependency files found for '%s'\n", targetLabel)
			return clierr.Exit(1)
		}

		var sortedPaths []string
		for path := range seenPaths {
			sortedPaths = append(sortedPaths, path)
		}
		sort.Strings(sortedPaths)
		for _, path := range sortedPaths {
			fmt.Println(path)
		}
		return nil
	}

	for _, dep := range deps {
		name := dep.DisplayName.String
		if name == "" {
			name = symbols.ExtractLeafName(dep.Symbol)
		}

		path, line, _, err := queries.GetDefLocation(db, dep.ID)
		if err != nil {
			return err
		}
		if path != "" && paths.PathInScope(path, pathScope) {
			lineNum := "?"
			if line != nil {
				lineNum = fmt.Sprintf("%d", *line+1)
			}
			fmt.Printf("%s:%s  %s\n", path, lineNum, name)
		} else {
			fmt.Println(name)
		}
	}

	return nil
}
