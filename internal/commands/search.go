package commands

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/sourcegraph/scip-cli-go/internal/output"
	"github.com/sourcegraph/scip-cli-go/internal/paths"
	"github.com/sourcegraph/scip-cli-go/internal/queries"
	"github.com/sourcegraph/scip-cli-go/internal/session"
	"github.com/sourcegraph/scip-cli-go/internal/source"
	"github.com/sourcegraph/scip-cli-go/internal/sqlhelp"
	"github.com/sourcegraph/scip-cli-go/internal/symbols"
)

func parseSymbol(symbol string) (string, string) {
	backtickRe := regexp.MustCompile("`([^`]+)`")
	if loc := backtickRe.FindStringIndex(symbol); loc != nil {
		filename := symbol[loc[0]+1 : loc[1]-1]
		before := symbol[:loc[0]]
		parts := strings.Fields(before)
		var filePath string
		if len(parts) >= 5 {
			filePath = strings.Join(parts[4:], " ") + filename
		} else {
			filePath = filename
		}
		afterFile := symbol[loc[1]:]
		afterFile = strings.TrimPrefix(afterFile, "/")
		symbolName := strings.TrimRight(afterFile, ".")
		return filePath, symbolName
	}

	pyPathRe := regexp.MustCompile(`(\S+\.py)/(.+)$`)
	if m := pyPathRe.FindStringSubmatch(symbol); m != nil {
		return m[1], m[2]
	}
	return "?", "?"
}

func isNoisySymbol(symbolStr string) bool {
	if strings.HasSuffix(symbolStr, "/") {
		return true
	}
	if strings.HasSuffix(symbolStr, "/__init__:") {
		return true
	}
	if strings.Contains(symbolStr, "typeLiteral") && symbols.InferKind(symbolStr) != symbols.KindProperty {
		return true
	}
	return strings.Contains(symbolStr, ").(")
}

func kindToDisplay(kind symbols.SymbolKind) string {
	return string(kind)
}

func searchRowsWithKind(db *sql.DB, query string, params []interface{}, kind symbols.SymbolKind, limit int) []queries.SymbolResult {
	var results []queries.SymbolResult
	rows, err := sqlhelp.DebugExecute(db, query, params...)
	if err != nil {
		return results
	}
	defer rows.Close()

	for rows.Next() {
		var r queries.SymbolResult
		if err := rows.Scan(&r.ID, &r.Symbol, &r.DisplayName); err != nil {
			continue
		}
		if symbols.InferKind(r.Symbol) != kind {
			continue
		}
		results = append(results, r)
		if len(results) > limit {
			break
		}
	}
	return results
}

func resolveFilePath(db *sql.DB, symbolStr string, docPath string) string {
	if docPath != "" {
		return docPath
	}
	if extracted := symbols.ExtractFilePathFromSymbol(symbolStr); extracted != "" {
		return extracted
	}
	filePath, _ := parseSymbol(symbolStr)
	return filePath
}

type searchResult struct {
	filePath string
	line     interface{} // int or "?"
	kind     string
	name     string
}

func printSearchResults(results []searchResult, namesOnly, pathsOnly bool) {
	if pathsOnly {
		seen := make(map[string]bool)
		var paths []string
		for _, r := range results {
			if r.filePath != "?" && !seen[r.filePath] {
				seen[r.filePath] = true
				paths = append(paths, r.filePath)
			}
		}
		for _, p := range paths {
			fmt.Println(p)
		}
		return
	}

	if namesOnly {
		for _, r := range results {
			fmt.Println(r.name)
		}
		return
	}

	for _, r := range results {
		if lineNum, ok := r.line.(int); ok {
			fmt.Printf("%s:%d %s %s\n", r.filePath, lineNum, r.kind, r.name)
		} else {
			fmt.Printf("%s:%v %s %s\n", r.filePath, r.line, r.kind, r.name)
		}
	}
}

func defLocationForSymbol(db *sql.DB, projectRoot string, sym queries.SymbolResult) (string, interface{}) {
	path, startLine, _, err := source.ResolveDefLocation(db, projectRoot, sym.Symbol, sym.ID)
	if err == nil && path != "" {
		if startLine != nil {
			return path, *startLine + 1
		}
		return path, "?"
	}
	return resolveFilePath(db, sym.Symbol, ""), "?"
}

func searchResultsFromSymbols(db *sql.DB, projectRoot string, syms []queries.SymbolResult, limit int) []searchResult {
	syms = output.LimitAndWarn(syms, limit, "results")
	var results []searchResult
	for _, sym := range syms {
		filePath, line := defLocationForSymbol(db, projectRoot, sym)
		kind := symbols.InferKind(sym.Symbol)
		short := symbols.ExtractLeafName(sym.Symbol)
		results = append(results, searchResult{filePath, line, kindToDisplay(kind), short})
	}
	return results
}

func qualifiedPattern(pattern string) bool {
	return strings.Contains(pattern, ".") && !strings.Contains(pattern, "/") && !strings.Contains(pattern, "*")
}

func SearchMain(args map[string]interface{}) error {
	db, projectRoot, err := session.Setup()
	if err != nil {
		return err
	}
	defer db.Close()

	pathScope := args["path_scope"].(string)
	limit := args["limit"].(int)
	patterns := args["pattern"].([]string)
	kind := args["kind"].(*symbols.SymbolKind)
	namesOnly := args["names_only"].(bool)
	pathsOnly := args["paths_only"].(bool)

	var resolvedSymbols []queries.SymbolResult
	var likePatterns []string

	for _, pattern := range patterns {
		if qualifiedPattern(pattern) {
			limitPlusOne := limit + 1
			syms, err := queries.ResolveSymbol(db, pattern, kind, &limitPlusOne, pathScope)
			if err != nil {
				return err
			}
			if len(syms) > 0 {
				resolvedSymbols = append(resolvedSymbols, syms...)
				continue
			}
		}
		likePatterns = append(likePatterns, pattern)
	}

	if len(resolvedSymbols) > 0 && len(likePatterns) == 0 {
		results := searchResultsFromSymbols(db, projectRoot, resolvedSymbols, limit)
		printSearchResults(results, namesOnly, pathsOnly)
		return nil
	}

	var prefill []searchResult
	if len(resolvedSymbols) > 0 {
		prefill = searchResultsFromSymbols(db, projectRoot, resolvedSymbols, limit)
	}

	if len(likePatterns) == 0 {
		if len(prefill) > 0 {
			printSearchResults(prefill, namesOnly, pathsOnly)
		} else {
			patternStr := strings.Join(patterns, " or ")
			fmt.Fprintf(os.Stderr, "No symbols found matching '%s'\n", patternStr)
			os.Exit(1)
		}
		return nil
	}

	pathClause, pathParams, err := paths.PathFilterSQL(db, pathScope, "d")
	if err != nil {
		return err
	}

	kindClause := ""
	if kind != nil {
		kindClause = symbols.KindSQLClause(*kind)
	}

	joinDocs := `
		LEFT JOIN defn_enclosing_ranges der ON gs.id = der.symbol_id
		LEFT JOIN documents d ON der.document_id = d.id
	`

	var patternClauses []string
	var patternParams []interface{}
	for _, pattern := range likePatterns {
		escaped := sqlhelp.EscapeLike(pattern)
		patternClauses = append(patternClauses, "gs.symbol LIKE ? ESCAPE '\\'")
		patternParams = append(patternParams, "%"+escaped+"%")
	}

	whereClause := strings.Join(patternClauses, " OR ")

	var results []searchResult
	if kind != nil {
		query := fmt.Sprintf(`
			SELECT gs.id, gs.symbol, gs.display_name
			FROM global_symbols gs
			%s
			WHERE (%s)%s%s
		`, joinDocs, whereClause, pathClause, kindClause)
		params := append(patternParams, pathParams...)
		rows := searchRowsWithKind(db, query, params, *kind, limit)
		for _, sym := range rows {
			filePath, line := defLocationForSymbol(db, projectRoot, sym)
			if isNoisySymbol(sym.Symbol) {
				continue
			}
			symKind := symbols.InferKind(sym.Symbol)
			short := symbols.ExtractLeafName(sym.Symbol)
			results = append(results, searchResult{filePath, line, kindToDisplay(symKind), short})
		}
	} else {
		fetchLimit := limit*5 + 1
		if fetchLimit < limit+1 {
			fetchLimit = limit + 1
		}
		query := fmt.Sprintf(`
			SELECT gs.id, gs.symbol, gs.display_name
			FROM global_symbols gs
			%s
			WHERE (%s)%s
			LIMIT ?
		`, joinDocs, whereClause, pathClause)
		params := append(patternParams, pathParams...)
		params = append(params, fetchLimit)
		rows, err := sqlhelp.DebugExecute(db, query, params...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var sym queries.SymbolResult
			if err := rows.Scan(&sym.ID, &sym.Symbol, &sym.DisplayName); err != nil {
				continue
			}
			if isNoisySymbol(sym.Symbol) {
				continue
			}
			filePath, line := defLocationForSymbol(db, projectRoot, sym)
			symKind := symbols.InferKind(sym.Symbol)
			short := symbols.ExtractLeafName(sym.Symbol)
			results = append(results, searchResult{filePath, line, kindToDisplay(symKind), short})
		}
	}

	if len(results) == 0 {
		patternStr := strings.Join(likePatterns, " or ")
		if kind != nil {
			fmt.Fprintf(os.Stderr, "No %s symbols found matching '%s'\n", string(*kind), patternStr)
		} else {
			fmt.Fprintf(os.Stderr, "No symbols found matching '%s'\n", patternStr)
		}
		os.Exit(1)
	}

	results = output.LimitAndWarn(results, limit, "results")

	if len(prefill) > 0 {
		seen := make(map[string]bool)
		for _, r := range prefill {
			seen[fmt.Sprintf("%s:%v", r.filePath, r.line)] = true
		}
		var filtered []searchResult
		for _, r := range results {
			key := fmt.Sprintf("%s:%v", r.filePath, r.line)
			if !seen[key] {
				filtered = append(filtered, r)
			}
		}
		results = append(prefill, filtered...)
		if len(results) > limit {
			results = results[:limit]
		}
	}

	printSearchResults(results, namesOnly, pathsOnly)
	return nil
}
