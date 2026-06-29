package queries

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/flesler/scip-cli-go/internal/paths"
	"github.com/flesler/scip-cli-go/internal/sqlhelp"
	"github.com/flesler/scip-cli-go/internal/symbols"
)

type SymbolResult struct {
	ID          int
	Symbol      string
	DisplayName sql.NullString
}

type FileSymbol struct {
	ID          int
	Symbol      string
	DisplayName sql.NullString
	StartLine   int
	EndLine     int
}

type Member struct {
	ID          int
	Symbol      string
	DisplayName sql.NullString
	StartLine   sql.NullInt64
	EndLine     sql.NullInt64
}

func queryDocumentPaths(db *sql.DB, query string, args ...interface{}) ([]string, error) {
	rows, err := sqlhelp.DebugExecute(db, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		result = append(result, path)
	}
	return result, rows.Err()
}

func rankFileMatches(paths []string, pattern string) []string {
	type ranked struct {
		path   string
		exact  bool
		isTest bool
	}

	var ranked_paths []ranked
	for _, p := range paths {
		name := filepath.Base(p)
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		exact := name == pattern || stem == pattern
		isTest := strings.Contains(name, ".test.") || strings.HasSuffix(name, ".spec.ts") || strings.HasSuffix(name, ".spec.tsx")
		ranked_paths = append(ranked_paths, ranked{p, exact, isTest})
	}

	for i := 0; i < len(ranked_paths); i++ {
		for j := i + 1; j < len(ranked_paths); j++ {
			if ranked_paths[i].exact && !ranked_paths[j].exact {
				continue
			} else if !ranked_paths[i].exact && ranked_paths[j].exact {
				ranked_paths[i], ranked_paths[j] = ranked_paths[j], ranked_paths[i]
			} else if ranked_paths[i].isTest && !ranked_paths[j].isTest {
				ranked_paths[i], ranked_paths[j] = ranked_paths[j], ranked_paths[i]
			} else if ranked_paths[i].path > ranked_paths[j].path {
				ranked_paths[i], ranked_paths[j] = ranked_paths[j], ranked_paths[i]
			}
		}
	}

	result := make([]string, len(ranked_paths))
	for i, r := range ranked_paths {
		result[i] = r.path
	}
	return result
}

func ResolveSymbol(db *sql.DB, name string, kindFilter *symbols.SymbolKind, limit *int, pathScope string) ([]SymbolResult, error) {
	qualifierParts, leaf := symbols.ParseQualifiedName(name)
	searchName := leaf
	if len(qualifierParts) == 0 {
		searchName = name
	}

	escaped := sqlhelp.EscapeLike(searchName)
	pathClause, pathParams, err := paths.PathFilterSQL(db, pathScope, "d")
	if err != nil {
		return nil, err
	}

	kindClause := ""
	if kindFilter != nil {
		kindClause = symbols.KindSQLClause(*kindFilter)
	}

	likePatterns := symbols.SymbolLikePatterns(searchName)
	likeClauses := make([]string, len(likePatterns))
	for i := range likePatterns {
		likeClauses[i] = "gs.symbol LIKE ? ESCAPE '\\'"
	}
	likeClause := strings.Join(likeClauses, " OR ")

	var params []interface{}
	for _, p := range likePatterns {
		params = append(params, p)
	}
	params = append(params, pathParams...)

	limitClause := ""
	limitParam := []interface{}{}
	if limit != nil {
		if len(qualifierParts) == 0 {
			limitClause = " LIMIT ?"
			limitParam = []interface{}{*limit}
		} else {
			limitClause = " LIMIT ?"
			cap := *limit * 50
			if cap > 5000 {
				cap = 5000
			}
			limitParam = []interface{}{cap}
		}
	}

	var results []SymbolResult

	if pathScope != "" {
		query := fmt.Sprintf(`
			SELECT DISTINCT gs.id, gs.symbol, gs.display_name
			FROM global_symbols gs
			LEFT JOIN defn_enclosing_ranges der ON gs.id = der.symbol_id
			LEFT JOIN documents d ON der.document_id = d.id
			WHERE (%s)%s%s
			%s
		`, likeClause, pathClause, kindClause, limitClause)

		allParams := append(params, limitParam...)
		rows, err := sqlhelp.DebugExecute(db, query, allParams...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var r SymbolResult
			if err := rows.Scan(&r.ID, &r.Symbol, &r.DisplayName); err != nil {
				return nil, err
			}
			results = append(results, r)
		}

		if len(results) == 0 {
			query = fmt.Sprintf(`
				SELECT DISTINCT gs.id, gs.symbol, gs.display_name
				FROM global_symbols gs
				LEFT JOIN defn_enclosing_ranges der ON gs.id = der.symbol_id
				LEFT JOIN documents d ON der.document_id = d.id
				WHERE gs.symbol LIKE ? ESCAPE '\'%s%s
				%s
			`, pathClause, kindClause, limitClause)

			allParams = append([]interface{}{"%" + escaped + "%"}, pathParams...)
			allParams = append(allParams, limitParam...)
			rows, err := sqlhelp.DebugExecute(db, query, allParams...)
			if err != nil {
				return nil, err
			}
			defer rows.Close()

			for rows.Next() {
				var r SymbolResult
				if err := rows.Scan(&r.ID, &r.Symbol, &r.DisplayName); err != nil {
					return nil, err
				}
				if strings.Contains(r.Symbol, searchName) || strings.Contains(symbols.ExtractLeafName(r.Symbol), searchName) {
					results = append(results, r)
				}
			}
		}
	} else {
		bareLikeClauses := make([]string, len(likePatterns))
		for i := range likePatterns {
			bareLikeClauses[i] = "symbol LIKE ? ESCAPE '\\'"
		}
		bareLikeClause := strings.Join(bareLikeClauses, " OR ")

		query := fmt.Sprintf(`
			SELECT id, symbol, display_name FROM global_symbols
			WHERE (%s)%s
			%s
		`, bareLikeClause, strings.ReplaceAll(kindClause, "gs.", ""), limitClause)

		allParams := append(params, limitParam...)
		rows, err := sqlhelp.DebugExecute(db, query, allParams...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var r SymbolResult
			if err := rows.Scan(&r.ID, &r.Symbol, &r.DisplayName); err != nil {
				return nil, err
			}
			results = append(results, r)
		}

		if len(results) == 0 {
			query = fmt.Sprintf(`
				SELECT id, symbol, display_name FROM global_symbols
				WHERE symbol LIKE ? ESCAPE '\'%s
				%s
			`, strings.ReplaceAll(kindClause, "gs.", ""), limitClause)

			allParams = append([]interface{}{"%" + escaped + "%"}, limitParam...)
			rows, err := sqlhelp.DebugExecute(db, query, allParams...)
			if err != nil {
				return nil, err
			}
			defer rows.Close()

			for rows.Next() {
				var r SymbolResult
				if err := rows.Scan(&r.ID, &r.Symbol, &r.DisplayName); err != nil {
					return nil, err
				}
				if strings.Contains(r.Symbol, searchName) || strings.Contains(symbols.ExtractLeafName(r.Symbol), searchName) {
					results = append(results, r)
				}
			}
		}
	}

	if len(qualifierParts) > 0 {
		var filtered []SymbolResult
		for _, r := range results {
			if symbols.SymbolMatchesQualifier(r.Symbol, qualifierParts, leaf) && !symbols.IsParameterSymbol(r.Symbol) {
				filtered = append(filtered, r)
			}
		}
		results = filtered
		if limit != nil && len(results) > *limit {
			results = results[:*limit]
		}
	}

	return results, nil
}

func ResolveFile(db *sql.DB, filePattern string, pathScope string) ([]string, error) {
	pattern := strings.TrimSpace(filePattern)
	if pattern == "" {
		return nil, nil
	}

	escaped := sqlhelp.EscapeLike(pattern)
	basename := filepath.Base(pattern)
	escapedBasename := sqlhelp.EscapeLike(basename)

	var candidates []string

	filePaths, err := queryDocumentPaths(db, "SELECT relative_path FROM documents WHERE relative_path = ?", pattern)
	if err != nil {
		return nil, err
	}
	candidates = append(candidates, filePaths...)

	if strings.Contains(pattern, "/") {
		filePaths, err := queryDocumentPaths(db, `
			SELECT relative_path FROM documents
			WHERE relative_path LIKE ? ESCAPE '\'
			   OR relative_path LIKE ? ESCAPE '\'
			ORDER BY relative_path
		`, "%/"+escaped, "%"+escaped)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, filePaths...)
	} else if strings.Contains(pattern, ".") {
		filePaths, err := queryDocumentPaths(db, `
			SELECT relative_path FROM documents
			WHERE relative_path = ?
			   OR relative_path LIKE ? ESCAPE '\'
			ORDER BY relative_path
		`, pattern, "%/"+escapedBasename)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, filePaths...)
	} else {
		filePaths, err := queryDocumentPaths(db, `
			SELECT relative_path FROM documents
			WHERE relative_path LIKE ? ESCAPE '\'
			   OR relative_path LIKE ? ESCAPE '\'
			   OR relative_path LIKE ? ESCAPE '\'
			   OR relative_path LIKE ? ESCAPE '\'
			   OR relative_path LIKE ? ESCAPE '\'
			   OR relative_path LIKE ? ESCAPE '\'
			   OR relative_path LIKE ? ESCAPE '\'
			ORDER BY relative_path
		`,
			"%/"+escapedBasename+".ts",
			"%/"+escapedBasename+".tsx",
			"%/"+escapedBasename+".js",
			"%/"+escapedBasename+".jsx",
			"%/"+escapedBasename+".mjs",
			"%/"+escapedBasename+".cjs",
			"%/"+escapedBasename+".py")
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, filePaths...)

		filePaths, err = queryDocumentPaths(db,
			"SELECT relative_path FROM documents WHERE relative_path LIKE ? ESCAPE '\\'",
			"%"+escaped+"%")
		if err != nil {
			return nil, err
		}
		for _, p := range filePaths {
			stem := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
			if stem == pattern {
				candidates = append(candidates, p)
			}
		}
	}

	if len(candidates) == 0 {
		filePaths, err := queryDocumentPaths(db,
			"SELECT relative_path FROM documents WHERE relative_path LIKE ? ESCAPE '\\' ORDER BY relative_path",
			"%"+escaped+"%")
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, filePaths...)
	}

	seen := make(map[string]bool)
	var unique []string
	for _, c := range candidates {
		if !seen[c] {
			seen[c] = true
			unique = append(unique, c)
		}
	}

	rankPattern := basename
	if !strings.Contains(pattern, ".") {
		rankPattern = pattern
	}
	ranked := rankFileMatches(unique, rankPattern)

	if pathScope != "" {
		var filtered []string
		for _, p := range ranked {
			if paths.PathInScope(p, pathScope) {
				filtered = append(filtered, p)
			}
		}
		ranked = filtered
	}

	return ranked, nil
}

func GetFileSymbols(db *sql.DB, relativePath string, limit *int) ([]FileSymbol, error) {
	limitClause := ""
	if limit != nil {
		limitClause = fmt.Sprintf(" LIMIT %d", *limit)
	}

	query := fmt.Sprintf(`
		SELECT gs.id, gs.symbol, gs.display_name, der.start_line, der.end_line
		FROM global_symbols gs
		JOIN defn_enclosing_ranges der ON gs.id = der.symbol_id
		JOIN documents d ON der.document_id = d.id
		WHERE d.relative_path = ?
		ORDER BY der.start_line
		%s
	`, limitClause)

	rows, err := sqlhelp.DebugExecute(db, query, relativePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []FileSymbol
	for rows.Next() {
		var f FileSymbol
		if err := rows.Scan(&f.ID, &f.Symbol, &f.DisplayName, &f.StartLine, &f.EndLine); err != nil {
			return nil, err
		}
		results = append(results, f)
	}
	return results, rows.Err()
}

func GetImporterPaths(db *sql.DB, symbolIDs []int, excludePath string, limit *int) ([]string, error) {
	if len(symbolIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(symbolIDs))
	args := make([]interface{}, 0, len(symbolIDs)+2)
	for i, id := range symbolIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, excludePath)

	limitClause := ""
	if limit != nil {
		limitClause = fmt.Sprintf(" LIMIT %d", *limit)
	}

	query := fmt.Sprintf(`
		SELECT DISTINCT d.relative_path
		FROM mentions m
		JOIN chunks c ON m.chunk_id = c.id
		JOIN documents d ON c.document_id = d.id
		WHERE m.symbol_id IN (%s) AND m.role != 1 AND d.relative_path != ?
		ORDER BY d.relative_path
		%s
	`, strings.Join(placeholders, ","), limitClause)

	rows, err := sqlhelp.DebugExecute(db, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		result = append(result, path)
	}
	return result, rows.Err()
}

func GetMembers(db *sql.DB, symbolID int) ([]Member, bool, error) {
	var symbolStr string
	err := sqlhelp.DebugExecuteOne(db, "SELECT symbol FROM global_symbols WHERE id = ?", symbolID).Scan(&symbolStr)
	if err != nil {
		return nil, false, nil
	}

	escapedParent := sqlhelp.EscapeLike(symbolStr)
	const membersCap = 500
	rows, err := sqlhelp.DebugExecute(db, `
		SELECT gs.id, gs.symbol, gs.display_name, der.start_line, der.end_line
		FROM global_symbols gs
		LEFT JOIN defn_enclosing_ranges der ON gs.id = der.symbol_id
		WHERE gs.symbol LIKE ? ESCAPE '\' AND gs.symbol != ?
		ORDER BY der.start_line, gs.symbol
		LIMIT ?
	`, escapedParent+"%", symbolStr, membersCap+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var results []Member
	nFetched := 0
	for rows.Next() {
		nFetched++
		var m Member
		if err := rows.Scan(&m.ID, &m.Symbol, &m.DisplayName, &m.StartLine, &m.EndLine); err != nil {
			return nil, false, err
		}
		if !strings.Contains(m.Symbol, ").(") && isDirectMember(symbolStr, m.Symbol) {
			results = append(results, m)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := nFetched > membersCap
	if len(results) > membersCap {
		results = results[:membersCap]
	}
	return results, truncated, nil
}

func isDirectMember(parentSymbol, memberSymbol string) bool {
	if !strings.HasPrefix(memberSymbol, parentSymbol) {
		return false
	}
	suffix := memberSymbol[len(parentSymbol):]
	if suffix == "" {
		return false
	}
	if strings.HasPrefix(suffix, "typeLiteral") {
		matched, _ := regexp.MatchString(`typeLiteral\d+:[^#]+\.`, suffix)
		return matched
	}
	if matched, _ := regexp.MatchString(`^[^#]+#$`, suffix); matched {
		return true
	}
	head := strings.Split(suffix, "().")[0]
	return !strings.Contains(head, "#")
}

func GetDefLocation(db *sql.DB, symbolID int) (string, *int, *int, error) {
	var path string
	var startLine, endLine int
	err := sqlhelp.DebugExecuteOne(db, `
		SELECT d.relative_path, der.start_line, der.end_line
		FROM defn_enclosing_ranges der
		JOIN documents d ON der.document_id = d.id
		WHERE der.symbol_id = ?
	`, symbolID).Scan(&path, &startLine, &endLine)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil, nil, nil
		}
		return "", nil, nil, err
	}
	return path, &startLine, &endLine, nil
}

func SymbolExternalRefCount(db *sql.DB, symbolID int) (int, error) {
	var excludeDoc sql.NullInt64
	err := sqlhelp.DebugExecuteOne(db,
		"SELECT document_id FROM defn_enclosing_ranges WHERE symbol_id = ? LIMIT 1",
		symbolID).Scan(&excludeDoc)
	if err != nil {
		excludeDoc.Int64 = -1
	}

	var count int
	err = sqlhelp.DebugExecuteOne(db, `
		SELECT COUNT(DISTINCT c.document_id)
		FROM mentions m
		JOIN chunks c ON m.chunk_id = c.id
		WHERE m.symbol_id = ? AND m.role != 1 AND c.document_id != ?
	`, symbolID, excludeDoc.Int64).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func ResolveDocumentPath(db *sql.DB, symbolStr string) (string, error) {
	extracted := symbols.ExtractFilePathFromSymbol(symbolStr)
	if extracted == "" {
		return "", nil
	}

	rows, err := sqlhelp.DebugExecute(db,
		"SELECT relative_path FROM documents WHERE relative_path = ?",
		extracted)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return "", err
		}
		paths = append(paths, path)
	}

	if len(paths) > 0 {
		return paths[0], nil
	}

	basename := filepath.Base(extracted)
	escaped := sqlhelp.EscapeLike(basename)
	rows, err = sqlhelp.DebugExecute(db,
		"SELECT relative_path FROM documents WHERE relative_path LIKE ? ESCAPE '\\'",
		"%"+escaped)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	paths = nil
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return "", err
		}
		paths = append(paths, path)
	}

	if len(paths) == 0 {
		return extracted, nil
	}

	var suffixMatches []string
	for _, path := range paths {
		if strings.HasSuffix(path, extracted) {
			suffixMatches = append(suffixMatches, path)
		}
	}

	if len(suffixMatches) == 1 {
		return suffixMatches[0], nil
	}
	if len(paths) == 1 {
		return paths[0], nil
	}
	if len(suffixMatches) > 0 {
		return suffixMatches[0], nil
	}
	return paths[0], nil
}
