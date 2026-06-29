package analyze

import (
	"database/sql"
	"fmt"

	"github.com/flesler/scip-cli-go/v2/internal/symbols"
)

func affected(db *sql.DB, symbolID int, limit int) ([]string, error) {
	rows, err := fetchAllRows(db, `
		WITH RECURSIVE propagation(symbol_id, depth) AS (
		    SELECT ?, 0
		    UNION ALL
		    SELECT der.symbol_id, p.depth + 1
		    FROM propagation p
		    JOIN mentions m ON m.symbol_id = p.symbol_id AND m.role != 1
		    JOIN chunks c ON m.chunk_id = c.id
		    JOIN documents consumer_doc ON c.document_id = consumer_doc.id
		    JOIN defn_enclosing_ranges der ON der.document_id = consumer_doc.id
		    WHERE p.depth < 5 AND der.symbol_id != p.symbol_id
		)
		SELECT DISTINCT gs.symbol, def_d.relative_path, p.depth
		FROM propagation p
		JOIN global_symbols gs ON gs.id = p.symbol_id
		JOIN defn_enclosing_ranges der ON der.symbol_id = gs.id
		JOIN documents def_d ON der.document_id = def_d.id
		WHERE p.depth > 0
		ORDER BY p.depth, def_d.relative_path, gs.symbol
		LIMIT ?
	`, symbolID, limit)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("depth=%d  %s  (%s)", toInt(r[2]), ShortName(toStr(r[0])), toStr(r[1])))
	}
	return lines, nil
}

func consumerFiles(db *sql.DB, symbolID int, limit int) ([]string, error) {
	rows, err := fetchAllRows(db, `
		SELECT ref_d.relative_path, COUNT(*) AS ref_count
		FROM mentions m
		JOIN chunks c ON m.chunk_id = c.id
		JOIN documents ref_d ON c.document_id = ref_d.id
		JOIN defn_enclosing_ranges der ON m.symbol_id = der.symbol_id
		JOIN documents def_d ON der.document_id = def_d.id
		WHERE m.symbol_id = ? AND m.role != 1 AND ref_d.id != def_d.id
		GROUP BY ref_d.id
		ORDER BY ref_count DESC, ref_d.relative_path
		LIMIT ?
	`, symbolID, limit)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("%s  refs=%d", toStr(r[0]), toInt(r[1])))
	}
	return lines, nil
}

func symbolPressure(db *sql.DB, symbolID int) ([]string, error) {
	row, err := FetchOneRow(db, 4, `
		WITH target AS (
		    SELECT gs.id, gs.symbol, der.document_id, der.start_line, der.end_line
		    FROM global_symbols gs
		    JOIN defn_enclosing_ranges der ON gs.id = der.symbol_id
		    WHERE gs.id = ?
		),
		fan_in AS (
		    SELECT COUNT(DISTINCT ref_d.id) AS n
		    FROM mentions m
		    JOIN chunks c ON m.chunk_id = c.id
		    JOIN documents ref_d ON c.document_id = ref_d.id
		    JOIN target t ON m.symbol_id = t.id
		    WHERE m.role != 1 AND ref_d.id != t.document_id
		),
		fan_out AS (
		    SELECT COUNT(DISTINCT m.symbol_id) AS n
		    FROM target t
		    JOIN chunks c ON c.document_id = t.document_id
		    JOIN mentions m ON m.chunk_id = c.id AND m.role NOT IN (1, 2)
		    JOIN defn_enclosing_ranges callee ON m.symbol_id = callee.symbol_id
		    WHERE callee.document_id != t.document_id
		)
		SELECT t.symbol, t.end_line - t.start_line + 1, fan_in.n, fan_out.n
		FROM target t, fan_in, fan_out
	`, symbolID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return []string{"(symbol not found)"}, nil
	}
	symbol := toStr(row[0])
	loc := toInt(row[1])
	fanIn := toInt(row[2])
	fanOut := toInt(row[3])
	return []string{fmt.Sprintf("%s  loc=%d  fan_in=%d  fan_out=%d  pressure=%d",
		ShortName(symbol), loc, fanIn, fanOut, fanIn*fanOut)}, nil
}

func symbolDependencies(db *sql.DB, symbolID int, limit int) ([]string, error) {
	rows, err := fetchAllRows(db, `
		SELECT DISTINCT gs.symbol, callee_d.relative_path
		FROM global_symbols target_gs
		JOIN defn_enclosing_ranges target_der ON target_gs.id = target_der.symbol_id
		JOIN chunks c ON c.document_id = target_der.document_id
		JOIN mentions m ON m.chunk_id = c.id AND m.role NOT IN (1, 2)
		JOIN global_symbols gs ON m.symbol_id = gs.id
		JOIN defn_enclosing_ranges callee_def ON callee_def.symbol_id = gs.id
		JOIN documents callee_d ON callee_def.document_id = callee_d.id
		WHERE target_gs.id = ? AND callee_d.id != target_der.document_id
		ORDER BY callee_d.relative_path, gs.symbol
		LIMIT ?
	`, symbolID, limit)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("%s  (%s)", ShortName(toStr(r[0])), toStr(r[1])))
	}
	return lines, nil
}

func defContext(db *sql.DB, symbolID int) ([]string, error) {
	row, err := FetchOneRow(db, 5, `
		SELECT gs.symbol, gs.display_name, def_d.relative_path,
		       der.start_line, der.end_line
		FROM global_symbols gs
		JOIN defn_enclosing_ranges der ON gs.id = der.symbol_id
		JOIN documents def_d ON der.document_id = def_d.id
		WHERE gs.id = ?
	`, symbolID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return []string{"(symbol not found)"}, nil
	}
	symbol := toStr(row[0])
	displayName := toStr(row[1])
	path := toStr(row[2])
	start := toInt(row[3])
	end := toInt(row[4])
	kind := symbols.InferKind(symbol)
	memberRow, _ := fetchOneRow(db, `
		SELECT COUNT(*) FROM global_symbols gs
		WHERE gs.symbol LIKE ? AND gs.symbol != ?
	`, symbol+"%", symbol)
	memberCount := 0
	if memberRow != nil {
		memberCount = toInt(memberRow[0])
	}
	return []string{
		fmt.Sprintf("name=%s  kind=%s", ShortName(symbol), kind),
		fmt.Sprintf("file=%s  lines=%d:%d", path, start+1, end+1),
		fmt.Sprintf("display_name=%s  members≈%d", displayNameOrDash(displayName), memberCount),
	}, nil
}

func displayNameOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

type symbolCheckFn func(*sql.DB, int, int) ([]string, error)
type symbolCheckFn0 func(*sql.DB, int) ([]string, error)

func bindSymbol(fn symbolCheckFn, symbolID int) CheckFunc {
	return func(db *sql.DB, limit int, opts CheckOptions) ([]string, error) {
		return fn(db, symbolID, limit)
	}
}

func bindSymbol0(fn symbolCheckFn0, symbolID int) CheckFunc {
	return func(db *sql.DB, limit int, opts CheckOptions) ([]string, error) {
		return fn(db, symbolID)
	}
}

func RunSymbolSections(db *sql.DB, symbolID int, limit int, priorities map[Priority]bool, budget *RowBudget) ([]SectionResult, error) {
	checks := []Check{
		{"consumer_files", PriorityHigh, "Consumer files (direct)", bindSymbol(consumerFiles, symbolID), ""},
		{"dependencies", PriorityHigh, "Dependencies (cross-file)", bindSymbol(symbolDependencies, symbolID), ""},
		{"affected", PriorityLow, "Affected (transitive, coarse)", bindSymbol(affected, symbolID), ""},
		{"symbol_pressure", PriorityLow, "Symbol pressure (loc x fan metrics)", bindSymbol0(symbolPressure, symbolID), ""},
		{"def_context", PriorityLow, "Definition context", bindSymbol0(defContext, symbolID), ""},
	}
	return RunChecks(checks, db, limit, priorities, CheckOptions{}, budget)
}
