package analyze

import (
	"database/sql"
	"fmt"
)

func changeSurface(db *sql.DB, relativePath string, limit int) ([]string, error) {
	rows, err := fetchAllRows(db, `
		SELECT gs.symbol, der.start_line, der.end_line,
		       COUNT(DISTINCT CASE WHEN ref_d.id != def_d.id THEN ref_d.id END) AS consumers
		FROM global_symbols gs
		JOIN defn_enclosing_ranges der ON gs.id = der.symbol_id
		JOIN documents def_d ON der.document_id = def_d.id
		LEFT JOIN mentions m ON m.symbol_id = gs.id AND m.role != 1
		LEFT JOIN chunks c ON m.chunk_id = c.id
		LEFT JOIN documents ref_d ON c.document_id = ref_d.id
		WHERE def_d.relative_path = ?
		GROUP BY gs.id
		ORDER BY consumers DESC, der.start_line
		LIMIT ?
	`, relativePath, limit)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, r := range rows {
		symbol := toStr(r[0])
		start := toInt(r[1])
		end := toInt(r[2])
		consumers := toInt(r[3])
		risk := "low"
		if consumers > 10 {
			risk = "high"
		} else if consumers > 0 {
			risk = "medium"
		}
		lines = append(lines, fmt.Sprintf("%s  %d:%d  consumers=%d  risk=%s",
			ShortName(symbol), start+1, end+1, consumers, risk))
	}
	return lines, nil
}

func unusedImportsForFile(db *sql.DB, relativePath string, limit int) ([]string, error) {
	rows, err := fetchAllRows(db, `
		SELECT gs.symbol
		FROM mentions m
		JOIN chunks c ON m.chunk_id = c.id
		JOIN documents imp_d ON c.document_id = imp_d.id
		JOIN global_symbols gs ON m.symbol_id = gs.id
		WHERE imp_d.relative_path = ?
		  AND m.role = 2
		  AND NOT EXISTS (
		      SELECT 1
		      FROM mentions ref_m
		      JOIN chunks ref_c ON ref_m.chunk_id = ref_c.id
		      WHERE ref_m.symbol_id = gs.id
		        AND ref_m.role NOT IN (1, 2)
		        AND ref_c.document_id = imp_d.id
		  )
		ORDER BY gs.symbol
		LIMIT ?
	`, relativePath, limit)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, r := range rows {
		lines = append(lines, ShortName(toStr(r[0])))
	}
	return lines, nil
}

func fileConsumers(db *sql.DB, relativePath string, limit int) ([]string, error) {
	rows, err := fetchAllRows(db, `
		SELECT ref_d.relative_path, COUNT(DISTINCT gs.id) AS symbol_hits
		FROM global_symbols gs
		JOIN defn_enclosing_ranges der ON gs.id = der.symbol_id
		JOIN documents def_d ON der.document_id = def_d.id
		JOIN mentions m ON m.symbol_id = gs.id AND m.role != 1
		JOIN chunks c ON m.chunk_id = c.id
		JOIN documents ref_d ON c.document_id = ref_d.id
		WHERE def_d.relative_path = ? AND ref_d.id != def_d.id
		GROUP BY ref_d.id
		ORDER BY symbol_hits DESC, ref_d.relative_path
		LIMIT ?
	`, relativePath, limit)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("%s  symbols=%d", toStr(r[0]), toInt(r[1])))
	}
	return lines, nil
}

func unreferencedInFile(db *sql.DB, relativePath string, limit int) ([]string, error) {
	live, err := BuildLiveIndex(db)
	if err != nil {
		return nil, err
	}
	rows, err := fetchAllRows(db, `
		SELECT gs.symbol, der.start_line, der.end_line, def_d.id
		FROM global_symbols gs
		JOIN defn_enclosing_ranges der ON gs.id = der.symbol_id
		JOIN documents def_d ON der.document_id = def_d.id
		WHERE def_d.relative_path = ?
		  AND NOT EXISTS (
		      SELECT 1 FROM mentions m
		      WHERE m.symbol_id = gs.id AND m.role = 0
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM mentions m
		      JOIN chunks c ON m.chunk_id = c.id
		      WHERE m.symbol_id = gs.id AND m.role != 1 AND c.document_id != def_d.id
		  )
		ORDER BY der.start_line
		LIMIT ?
	`, relativePath, limit)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, r := range rows {
		symbol := toStr(r[0])
		start := toInt(r[1])
		end := toInt(r[2])
		defDocID := toInt(r[3])
		if AnalyzeNoise(relativePath, symbol, true) {
			continue
		}
		if live.DeadExportNoise(symbol, defDocID) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s  %d:%d", ShortName(symbol), start+1, end+1))
	}
	return lines, nil
}

func sameFileOnlyInFile(db *sql.DB, relativePath string, limit int) ([]string, error) {
	live, err := BuildLiveIndex(db)
	if err != nil {
		return nil, err
	}
	rows, err := fetchAllRows(db, `
		SELECT gs.symbol, der.start_line, der.end_line, def_d.id
		FROM global_symbols gs
		JOIN defn_enclosing_ranges der ON gs.id = der.symbol_id
		JOIN documents def_d ON der.document_id = def_d.id
		WHERE def_d.relative_path = ?
		  AND EXISTS (
		      SELECT 1 FROM mentions m
		      JOIN chunks c ON m.chunk_id = c.id
		      WHERE m.symbol_id = gs.id AND m.role = 0 AND c.document_id = def_d.id
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM mentions m
		      JOIN chunks c ON m.chunk_id = c.id
		      WHERE m.symbol_id = gs.id AND m.role = 0 AND c.document_id != def_d.id
		  )
		ORDER BY der.start_line
		LIMIT ?
	`, relativePath, limit)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, r := range rows {
		symbol := toStr(r[0])
		start := toInt(r[1])
		end := toInt(r[2])
		defDocID := toInt(r[3])
		if AnalyzeNoise(relativePath, symbol, true) {
			continue
		}
		if live.SameFileExportNoise(symbol, defDocID) {
			continue
		}
		if !FileHasSCIPImporters(db, relativePath, live, defDocID) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s  %d:%d", ShortName(symbol), start+1, end+1))
	}
	return lines, nil
}

func deadInFile(db *sql.DB, relativePath string, limit int) ([]string, error) {
	live, err := BuildLiveIndex(db)
	if err != nil {
		return nil, err
	}
	rows, err := fetchAllRows(db, `
		SELECT gs.id, gs.symbol, der.start_line, der.end_line, def_d.id
		FROM global_symbols gs
		JOIN defn_enclosing_ranges der ON gs.id = der.symbol_id
		JOIN documents def_d ON der.document_id = def_d.id
		WHERE def_d.relative_path = ?
		  AND NOT EXISTS (
		      SELECT 1
		      FROM mentions m
		      JOIN chunks c ON m.chunk_id = c.id
		      WHERE m.symbol_id = gs.id
		        AND m.role != 1
		        AND c.document_id != def_d.id
		  )
		ORDER BY der.start_line
		LIMIT ?
	`, relativePath, limit)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, r := range rows {
		symID := toInt(r[0])
		symbol := toStr(r[1])
		start := toInt(r[2])
		end := toInt(r[3])
		defDocID := toInt(r[4])
		if AnalyzeNoise(relativePath, symbol, true) {
			continue
		}
		if HasSameFileReferenceUsage(db, symID, defDocID) {
			continue
		}
		if live.DeadExportNoise(symbol, defDocID) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s  %d:%d", ShortName(symbol), start+1, end+1))
	}
	return lines, nil
}

func importsSummary(db *sql.DB, relativePath string, limit int) ([]string, error) {
	countRows, err := fetchAllRows(db, `
		SELECT COUNT(DISTINCT m.symbol_id)
		FROM mentions m
		JOIN chunks c ON m.chunk_id = c.id
		JOIN documents d ON c.document_id = d.id
		WHERE d.relative_path = ? AND m.role = 2
	`, relativePath)
	if err != nil {
		return nil, err
	}
	total := 0
	if len(countRows) > 0 {
		total = toInt(countRows[0][0])
	}
	rows, err := fetchAllRows(db, `
		SELECT gs.symbol, def_d.relative_path AS from_file
		FROM mentions m
		JOIN chunks c ON m.chunk_id = c.id
		JOIN documents imp_d ON c.document_id = imp_d.id
		JOIN global_symbols gs ON m.symbol_id = gs.id
		LEFT JOIN defn_enclosing_ranges der ON gs.id = der.symbol_id
		LEFT JOIN documents def_d ON der.document_id = def_d.id
		WHERE imp_d.relative_path = ? AND m.role = 2
		ORDER BY gs.symbol
		LIMIT ?
	`, relativePath, limit)
	if err != nil {
		return nil, err
	}
	lines := []string{fmt.Sprintf("total imports: %d", total)}
	for _, r := range rows {
		fromFile := toStr(r[1])
		if fromFile == "" {
			fromFile = "(external)"
		}
		lines = append(lines, fmt.Sprintf("  %s  from %s", ShortName(toStr(r[0])), fromFile))
	}
	return lines, nil
}

func couplingForFile(db *sql.DB, relativePath string, limit int) ([]string, error) {
	rows, err := fetchAllRows(db, fmt.Sprintf(`
		SELECT other_file, shared FROM (
		    SELECT ref_d.relative_path AS other_file,
		           COUNT(DISTINCT gs.id) AS shared
		    FROM mentions m
		    JOIN chunks c ON m.chunk_id = c.id
		    JOIN documents ref_d ON c.document_id = ref_d.id
		    JOIN global_symbols gs ON m.symbol_id = gs.id
		    %s
		    WHERE m.role != 1 AND def_d.relative_path = ? AND ref_d.relative_path != ?
		    GROUP BY ref_d.id
		    UNION ALL
		    SELECT def_d.relative_path AS other_file,
		           COUNT(DISTINCT gs.id) AS shared
		    FROM mentions m
		    JOIN chunks c ON m.chunk_id = c.id
		    JOIN documents ref_d ON c.document_id = ref_d.id
		    JOIN global_symbols gs ON m.symbol_id = gs.id
		    %s
		    WHERE m.role != 1 AND ref_d.relative_path = ? AND def_d.relative_path != ?
		    GROUP BY def_d.id
		)
		ORDER BY shared DESC, other_file
		LIMIT ?
	`, SymDefJoin, SymDefJoin), relativePath, relativePath, relativePath, relativePath, limit)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("%s  shared=%d", toStr(r[0]), toInt(r[1])))
	}
	return lines, nil
}

func topSymbolPressure(db *sql.DB, relativePath string, limit int) ([]string, error) {
	cap := limit
	if cap > 5 {
		cap = 5
	}
	rows, err := fetchAllRows(db, `
		SELECT gs.id,
		       COUNT(DISTINCT CASE WHEN ref_d.id != def_d.id THEN ref_d.id END) AS consumers
		FROM global_symbols gs
		JOIN defn_enclosing_ranges der ON gs.id = der.symbol_id
		JOIN documents def_d ON der.document_id = def_d.id
		LEFT JOIN mentions m ON m.symbol_id = gs.id AND m.role != 1
		LEFT JOIN chunks c ON m.chunk_id = c.id
		LEFT JOIN documents ref_d ON c.document_id = ref_d.id
		WHERE def_d.relative_path = ?
		GROUP BY gs.id
		HAVING consumers > 0
		ORDER BY consumers DESC, der.start_line
		LIMIT ?
	`, relativePath, cap)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, r := range rows {
		symID := toInt(r[0])
		consumers := toInt(r[1])
		pressure, err := symbolPressure(db, symID)
		if err != nil || len(pressure) == 0 || pressure[0] == "(symbol not found)" {
			continue
		}
		lines = append(lines, fmt.Sprintf("consumers=%d  %s", consumers, pressure[0]))
	}
	return lines, nil
}

type fileCheckFn func(*sql.DB, string, int) ([]string, error)

func bindPath(fn fileCheckFn, path string) CheckFunc {
	return func(db *sql.DB, limit int, opts CheckOptions) ([]string, error) {
		return fn(db, path, limit)
	}
}

func fileChecks(relativePath string, includeTopSymbols bool) []Check {
	title := fmt.Sprintf("(%s)", relativePath)
	checks := []Check{
		{"unreferenced_in_file", PriorityHigh, fmt.Sprintf("Unreferenced in file %s", title), bindPath(unreferencedInFile, relativePath), ""},
		{"dead_in_file", PriorityHigh, fmt.Sprintf("Dead exports in file %s", title), bindPath(deadInFile, relativePath), ""},
		{"unused_imports", PriorityHigh, fmt.Sprintf("Unused imports %s", title), bindPath(unusedImportsForFile, relativePath), ""},
		{"same_file_only", PriorityMedium, fmt.Sprintf("Same-file only %s", title), bindPath(sameFileOnlyInFile, relativePath), ""},
		{"change_surface", PriorityMedium, fmt.Sprintf("Change surface %s", title), bindPath(changeSurface, relativePath), ""},
		{"file_consumers", PriorityMedium, fmt.Sprintf("File consumers %s", title), bindPath(fileConsumers, relativePath), ""},
		{"coupling", PriorityLow, fmt.Sprintf("Coupling partners %s", title), bindPath(couplingForFile, relativePath), ""},
		{"imports_summary", PriorityLow, fmt.Sprintf("Imports summary %s", title), bindPath(importsSummary, relativePath), ""},
	}
	if includeTopSymbols {
		checks = append(checks, Check{
			"top_symbols", PriorityLow, fmt.Sprintf("Top symbols (by external consumers) %s", title),
			bindPath(topSymbolPressure, relativePath), "",
		})
	}
	return checks
}

func RunFileSections(db *sql.DB, relativePath string, limit int, priorities map[Priority]bool, budget *RowBudget) ([]SectionResult, error) {
	return RunChecks(fileChecks(relativePath, true), db, limit, priorities, CheckOptions{}, budget)
}

func RunFileSectionsOnly(db *sql.DB, relativePath string, limit int, priorities map[Priority]bool, budget *RowBudget) ([]SectionResult, error) {
	return RunChecks(fileChecks(relativePath, false), db, limit, priorities, CheckOptions{}, budget)
}
