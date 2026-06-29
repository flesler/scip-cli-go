package commands

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/flesler/scip-cli-go/v2/internal/output"
	"github.com/flesler/scip-cli-go/v2/internal/paths"
	"github.com/flesler/scip-cli-go/v2/internal/queries"
	"github.com/flesler/scip-cli-go/v2/internal/session"
	"github.com/flesler/scip-cli-go/v2/internal/source"
	"github.com/flesler/scip-cli-go/v2/internal/symbols"
)

func leafAppearsOnLine(leaf, line string) bool {
	if leaf == "" {
		return false
	}
	isIdent := func(b byte) bool {
		return b == '$' || b == '`' || unicode.IsLetter(rune(b)) || unicode.IsDigit(rune(b)) || b == '_'
	}
	for i := 0; i+len(leaf) <= len(line); i++ {
		if line[i:i+len(leaf)] != leaf {
			continue
		}
		beforeOK := i == 0 || !isIdent(line[i-1])
		after := i + len(leaf)
		afterOK := after >= len(line) || !isIdent(line[after])
		if beforeOK && afterOK {
			return true
		}
	}
	return false
}

func refsFromChunkGroups(byDoc map[int]map[string]interface{}, projectRoot, leaf string) []string {
	var results []string
	type docEntry struct {
		id   int
		path string
	}
	entries := make([]docEntry, 0, len(byDoc))
	for id, info := range byDoc {
		entries = append(entries, docEntry{id, info["path"].(string)})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].path == entries[j].path {
			return entries[i].id < entries[j].id
		}
		return entries[i].path < entries[j].path
	})
	for _, e := range entries {
		info := byDoc[e.id]
		relPath := info["path"].(string)
		chunksList := info["chunks"].([][3]interface{})
		if len(chunksList) == 0 {
			continue
		}

		minLine := chunksList[0][1].(int)
		maxLine := chunksList[0][2].(int)
		for _, c := range chunksList {
			if c[1].(int) < minLine {
				minLine = c[1].(int)
			}
			if c[2].(int) > maxLine {
				maxLine = c[2].(int)
			}
		}

		allSingleLine := true
		for _, c := range chunksList {
			if c[1].(int) != c[2].(int) {
				allSingleLine = false
				break
			}
		}

		if allSingleLine {
			for _, c := range chunksList {
				if c[1] != nil {
					results = append(results, fmt.Sprintf("%s:%d", relPath, c[1].(int)+1))
				}
			}
			continue
		}

		lines, _ := source.ReadSourceLines(projectRoot, relPath, &minLine, &maxLine)
		if lines == nil {
			for _, c := range chunksList {
				if c[1] != nil {
					results = append(results, fmt.Sprintf("%s:%d", relPath, c[1].(int)+1))
				}
			}
			continue
		}

		for _, c := range chunksList {
			startLine := c[1].(int)
			endLine := c[2].(int)
			offset := minLine
			found := false
			for lineIdx := startLine - offset; lineIdx <= endLine-offset && lineIdx < len(lines); lineIdx++ {
				if leafAppearsOnLine(leaf, lines[lineIdx]) {
					results = append(results, fmt.Sprintf("%s:%d", relPath, lineIdx+offset+1))
					found = true
					break
				}
			}
			if !found {
				results = append(results, fmt.Sprintf("%s:%d", relPath, startLine+1))
			}
		}
	}
	return results
}

func RefsMain(args map[string]interface{}) error {
	db, projectRoot, err := session.Setup()
	if err != nil {
		return err
	}
	defer db.Close()

	pathScope := args["path_scope"].(string)
	limit := args["limit"].(int)
	symbolNames := args["symbol"].([]string)

	var allRefs []struct {
		label string
		refs  []string
	}

	for _, queryName := range symbolNames {
		limitPlusOne := limit + 1
		syms, err := queries.ResolveSymbol(db, queryName, nil, &limitPlusOne, pathScope)
		if err != nil {
			return err
		}
		if len(syms) == 0 {
			fmt.Fprintf(os.Stderr, "Symbol '%s' not found\n", queryName)
			continue
		}

		syms = output.LimitAndWarn(syms, limit, "symbols")
		output.WarnAmbiguousRefs(queryName, syms, db)

		for _, sym := range syms {
			refs := getExactRefs(db, sym.ID, projectRoot, limit, pathScope)
			if len(refs) > 0 {
				label := output.SymbolOutputLabel(queryName, sym.Symbol, len(syms))
				allRefs = append(allRefs, struct {
					label string
					refs  []string
				}{label, refs})
			}
		}
	}

	if len(allRefs) == 0 {
		names := make([]string, len(symbolNames))
		for i, n := range symbolNames {
			names[i] = "'" + n + "'"
		}
		return fmt.Errorf("no references found for %s", strings.Join(names, ", "))
	}

	pathsOnly := args["paths_only"].(bool)
	if pathsOnly {
		uniquePaths := make(map[string]bool)
		for _, ref := range allRefs {
			for _, r := range ref.refs {
				parts := strings.Split(r, ":")
				if len(parts) > 0 {
					uniquePaths[parts[0]] = true
				}
			}
		}
		var sortedPaths []string
		for path := range uniquePaths {
			if paths.PathInScope(path, pathScope) {
				sortedPaths = append(sortedPaths, path)
			}
		}
		sort.Strings(sortedPaths)
		for _, path := range sortedPaths {
			fmt.Println(path)
		}
		return nil
	}

	showHeaders := len(allRefs) > 1
	for _, ref := range allRefs {
		output.MaybePrintSymbolHeader(ref.label, showHeaders)
		seen := make(map[string]bool)
		for _, r := range ref.refs {
			parts := strings.Split(r, ":")
			path := parts[0]
			if !seen[r] && paths.PathInScope(path, pathScope) {
				seen[r] = true
				fmt.Println(r)
			}
		}
	}

	return nil
}

func getExactRefs(db *sql.DB, symbolID int, projectRoot string, maxRefs int, pathScope string) []string {
	var symStr string
	err := db.QueryRow("SELECT symbol FROM global_symbols WHERE id = ?", symbolID).Scan(&symStr)
	if err != nil {
		return nil
	}

	leaf := symbols.ExtractLeafName(symStr)
	var results []string
	pathClause, pathParams, err := paths.PathFilterSQL(db, pathScope, "d")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: path filter failed: %v\n", err)
		return results
	}

	batchSize := maxRefs * 3
	if batchSize < 20 {
		batchSize = 20
	}
	sqlOffset := 0
	seen := make(map[string]bool)

	for len(results) <= maxRefs {
		query := fmt.Sprintf(`
			SELECT c.id, c.document_id, c.start_line, c.end_line, d.relative_path
			FROM mentions m
			JOIN chunks c ON m.chunk_id = c.id
			JOIN documents d ON c.document_id = d.id
			WHERE m.symbol_id = ? AND m.role != 1%s
			ORDER BY d.relative_path, c.start_line
			LIMIT ? OFFSET ?
		`, pathClause)

		params := append([]interface{}{symbolID}, pathParams...)
		params = append(params, batchSize, sqlOffset)

		rows, err := db.Query(query, params...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: reference query failed: %v\n", err)
			break
		}

		byDoc := make(map[int]map[string]interface{})
		totalChunks := 0

		for rows.Next() {
			var chunkID, docID, startLine, endLine int
			var relPath string
			if err := rows.Scan(&chunkID, &docID, &startLine, &endLine, &relPath); err != nil {
				rows.Close()
				fmt.Fprintf(os.Stderr, "Warning: reference row scan failed: %v\n", err)
				return results
			}

			if _, ok := byDoc[docID]; !ok {
				byDoc[docID] = map[string]interface{}{
					"path":   relPath,
					"chunks": [][3]interface{}{},
				}
			}
			info := byDoc[docID]
			info["chunks"] = append(info["chunks"].([][3]interface{}), [3]interface{}{chunkID, startLine, endLine})
			byDoc[docID] = info
			totalChunks++
		}
		rows.Close()

		if totalChunks == 0 {
			break
		}

		for _, ref := range refsFromChunkGroups(byDoc, projectRoot, leaf) {
			if !seen[ref] {
				seen[ref] = true
				results = append(results, ref)
				if len(results) > maxRefs {
					break
				}
			}
		}

		if len(results) > maxRefs || totalChunks < batchSize {
			break
		}

		sqlOffset += totalChunks
	}

	if len(results) > maxRefs {
		fmt.Fprintf(os.Stderr, "# Warning: more than %d references, showing first %d\n", maxRefs, maxRefs)
		return results[:maxRefs]
	}
	return results
}
