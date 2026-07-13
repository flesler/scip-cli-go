package commands

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/flesler/scip-cli-go/v2/internal/clierr"
	"github.com/flesler/scip-cli-go/v2/internal/output"
	"github.com/flesler/scip-cli-go/v2/internal/paths"
	"github.com/flesler/scip-cli-go/v2/internal/queries"
	"github.com/flesler/scip-cli-go/v2/internal/session"
	"github.com/flesler/scip-cli-go/v2/internal/source"
	"github.com/flesler/scip-cli-go/v2/internal/sqlhelp"
	"github.com/flesler/scip-cli-go/v2/internal/symbols"
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

func searchRankSQL() string {
	return " ORDER BY CASE WHEN gs.symbol LIKE '%#typeLiteral%' THEN 1 ELSE 0 END, length(gs.symbol)"
}

func searchScanLimit(limit int) int {
	scanLimit := limit * 50
	if scanLimit < 1000 {
		scanLimit = 1000
	}
	return scanLimit
}

func resolveFilePath(db *sql.DB, symbolStr string, docPath string) (string, error) {
	if docPath != "" {
		return docPath, nil
	}
	if resolved, err := queries.ResolveDocumentPath(db, symbolStr); err != nil {
		return "", err
	} else if resolved != "" {
		return resolved, nil
	}
	if extracted := symbols.ExtractFilePathFromSymbol(symbolStr); extracted != "" {
		return extracted, nil
	}
	filePath, _ := parseSymbol(symbolStr)
	return filePath, nil
}

type searchResult struct {
	filePath string
	line     interface{} // int or "?"
	kind     string
	name     string
}

func resultKey(r searchResult) string {
	return fmt.Sprintf("%s\x00%v\x00%s", r.filePath, r.line, r.name)
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
		sort.Strings(paths)
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

func rowToResult(db *sql.DB, projectRoot string, symbolID int, symbolStr string, startLine sql.NullInt64, docPath sql.NullString) (searchResult, error) {
	kind := symbols.InferKind(symbolStr)
	path, resolvedStart, _, err := source.ResolveDefLocation(db, projectRoot, symbolStr, symbolID)
	if err != nil {
		return searchResult{}, err
	}
	var line interface{}
	if path != "" {
		if resolvedStart != nil {
			line = *resolvedStart + 1
		} else {
			line = "?"
		}
	} else {
		doc := ""
		if docPath.Valid {
			doc = docPath.String
		}
		path, err = resolveFilePath(db, symbolStr, doc)
		if err != nil {
			return searchResult{}, err
		}
		if startLine.Valid {
			line = int(startLine.Int64) + 1
		} else {
			line = "?"
		}
	}
	return searchResult{
		filePath: path,
		line:     line,
		kind:     kindToDisplay(kind),
		name:     symbols.ExtractLeafName(symbolStr),
	}, nil
}

func collectUniqueFromRows(
	db *sql.DB,
	projectRoot string,
	rows *sql.Rows,
	limit int,
	kind *symbols.SymbolKind,
) ([]searchResult, error) {
	var results []searchResult
	seen := make(map[string]bool)
	for rows.Next() {
		var symbolID int
		var symbolStr string
		var displayName sql.NullString
		var startLine sql.NullInt64
		var docPath sql.NullString
		if err := rows.Scan(&symbolID, &symbolStr, &displayName, &startLine, &docPath); err != nil {
			return nil, err
		}
		if isNoisySymbol(symbolStr) {
			continue
		}
		if kind != nil && symbols.InferKind(symbolStr) != *kind {
			continue
		}
		result, err := rowToResult(db, projectRoot, symbolID, symbolStr, startLine, docPath)
		if err != nil {
			return nil, err
		}
		key := resultKey(result)
		if seen[key] {
			continue
		}
		seen[key] = true
		results = append(results, result)
		if len(results) > limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return output.LimitAndWarn(results, limit, "results"), nil
}

func searchResultsFromSymbols(db *sql.DB, projectRoot string, syms []queries.SymbolResult, limit int) ([]searchResult, error) {
	var results []searchResult
	seen := make(map[string]bool)
	for _, sym := range syms {
		result, err := rowToResult(db, projectRoot, sym.ID, sym.Symbol, sql.NullInt64{}, sql.NullString{})
		if err != nil {
			return nil, err
		}
		key := resultKey(result)
		if seen[key] {
			continue
		}
		seen[key] = true
		results = append(results, result)
		if len(results) > limit {
			break
		}
	}
	return output.LimitAndWarn(results, limit, "results"), nil
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
		results, err := searchResultsFromSymbols(db, projectRoot, resolvedSymbols, limit)
		if err != nil {
			return err
		}
		printSearchResults(results, namesOnly, pathsOnly)
		return nil
	}

	var prefill []searchResult
	if len(resolvedSymbols) > 0 {
		prefill, err = searchResultsFromSymbols(db, projectRoot, resolvedSymbols, limit)
		if err != nil {
			return err
		}
	}

	if len(likePatterns) == 0 {
		if len(prefill) > 0 {
			printSearchResults(prefill, namesOnly, pathsOnly)
		} else {
			patternStr := strings.Join(patterns, " or ")
			fmt.Fprintf(os.Stderr, "No symbols found matching '%s'\n", patternStr)
			return clierr.Exit(1)
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
	query := fmt.Sprintf(`
		SELECT gs.id, gs.symbol, gs.display_name, der.start_line, d.relative_path
		FROM global_symbols gs
		%s
		WHERE (%s)%s%s
		%s
		LIMIT ?
	`, joinDocs, whereClause, pathClause, kindClause, searchRankSQL())

	params := append(patternParams, pathParams...)
	params = append(params, searchScanLimit(limit))
	rows, err := sqlhelp.DebugExecute(db, query, params...)
	if err != nil {
		return err
	}
	defer rows.Close()

	results, err := collectUniqueFromRows(db, projectRoot, rows, limit, kind)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		patternStr := strings.Join(likePatterns, " or ")
		if kind != nil {
			fmt.Fprintf(os.Stderr, "No %s symbols found matching '%s'\n", string(*kind), patternStr)
		} else {
			fmt.Fprintf(os.Stderr, "No symbols found matching '%s'\n", patternStr)
		}
		return clierr.Exit(1)
	}

	if len(prefill) > 0 {
		seen := make(map[string]bool)
		for _, r := range prefill {
			seen[resultKey(r)] = true
		}
		var filtered []searchResult
		for _, r := range results {
			if !seen[resultKey(r)] {
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
