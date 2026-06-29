package analyze

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/flesler/scip-cli-go/v2/internal/paths"
	"github.com/flesler/scip-cli-go/v2/internal/symbols"
)

type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
)

var priorityOrder = map[Priority]int{
	PriorityHigh:   0,
	PriorityMedium: 1,
	PriorityLow:    2,
}

type CheckFunc func(db *sql.DB, limit int, opts CheckOptions) ([]string, error)

type CheckOptions struct {
	IncludeTests bool
	Scope        string
}

type Check struct {
	Key                  string
	Priority             Priority
	Title                string
	Run                  CheckFunc
	FalsePositivePreface string
}

func (c *Check) LabeledTitle() string {
	return fmt.Sprintf("[%s] %s", c.Priority, c.Title)
}

var falsePositivePrefaces = map[string]string{
	"dead_exports":         "SCIP may miss dynamic loading (loadFiles, GraphQL) and default-export object members — verify with rdeps/rg before deleting.",
	"unreferenced":         "No mentions in the index — symbols may still run via dynamic import or side-effect registration.",
	"same_file_only":       "Referenced only in the defining file — often handlers or private helpers, not dead exports.",
	"stale_types":          "No cross-file refs in the index — may still be used in-file or as a type-only shape.",
	"cycles":               "Remaining cycles may be barrel re-exports; confirm before refactoring.",
	"dead_in_file":         "SCIP may miss dynamic loading and default-export indirection — verify with rdeps/rg before deleting.",
	"unreferenced_in_file": "No mentions in the index — may still be used in-file via handlers or dynamic registration.",
}

func prefaceFor(key string) string {
	return falsePositivePrefaces[key]
}

type RowBudget struct {
	Remaining int
}

func (b *RowBudget) Exhausted() bool {
	return b.Remaining <= 0
}

func RunChecks(checks []Check, db *sql.DB, limit int, priorities map[Priority]bool, opts CheckOptions, budget *RowBudget) ([]SectionResult, error) {
	if budget == nil {
		budget = &RowBudget{Remaining: limit}
	}

	var selected []Check
	for _, c := range checks {
		if len(priorities) == 0 || priorities[c.Priority] {
			selected = append(selected, c)
		}
	}

	sort.Slice(selected, func(i, j int) bool {
		pi := priorityOrder[selected[i].Priority]
		pj := priorityOrder[selected[j].Priority]
		if pi != pj {
			return pi < pj
		}
		return selected[i].Key < selected[j].Key
	})

	var sections []SectionResult
	for _, check := range selected {
		if budget.Exhausted() {
			break
		}

		lines, err := check.Run(db, budget.Remaining, opts)
		if err != nil {
			return nil, err
		}

		isNone := len(lines) == 1 && lines[0] == "(none)"
		if !isNone {
			if len(lines) > budget.Remaining {
				lines = lines[:budget.Remaining]
			}
			budget.Remaining -= len(lines)
		}

		preface := check.FalsePositivePreface
		if preface == "" {
			preface = prefaceFor(check.Key)
		}

		title, finalLines, finalPreface := FormatSection(check.LabeledTitle(), lines, preface)
		sections = append(sections, SectionResult{
			Title:   title,
			Lines:   finalLines,
			Preface: finalPreface,
		})
	}

	return sections, nil
}

func bottlenecks(db *sql.DB, limit int, opts CheckOptions) ([]string, error) {
	scopeClause, scopeParams, err := paths.PathFilterSQL(db, opts.Scope, "def_d")
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		WITH fan_in AS (
			SELECT gs.id AS symbol_id,
				   COUNT(DISTINCT ref_d.id) AS fan_in
			FROM global_symbols gs
			%s
			JOIN mentions m ON m.symbol_id = gs.id AND m.role != 1
			JOIN chunks c ON m.chunk_id = c.id
			JOIN documents ref_d ON c.document_id = ref_d.id
			WHERE ref_d.id != def_d.id
			GROUP BY gs.id
		),
		fan_out AS (
			SELECT der.symbol_id,
				   COUNT(DISTINCT m.symbol_id) AS fan_out
			FROM defn_enclosing_ranges der
			JOIN documents def_doc ON der.document_id = def_doc.id
			JOIN chunks c ON c.document_id = def_doc.id
			JOIN mentions m ON m.chunk_id = c.id AND m.role NOT IN (1, 2)
			JOIN defn_enclosing_ranges callee_def ON m.symbol_id = callee_def.symbol_id
			JOIN documents callee_doc ON callee_def.document_id = callee_doc.id
			WHERE callee_doc.id != def_doc.id
			GROUP BY der.symbol_id
		)
		SELECT gs.symbol, def_d.relative_path, fi.fan_in, fo.fan_out,
			   sym_def.end_line - sym_def.start_line + 1 AS loc,
			   fi.fan_in * fo.fan_out AS score
		FROM global_symbols gs
		%s
		JOIN fan_in fi ON fi.symbol_id = gs.id
		JOIN fan_out fo ON fo.symbol_id = gs.id
		WHERE fi.fan_in >= 1 AND fo.fan_out >= 1%s
		ORDER BY score DESC, fi.fan_in DESC
		LIMIT ?
	`, SymDefJoin, SymDefJoin, scopeClause)

	rows, err := fetchAllRows(db, query, append(scopeParams, limit*5)...)
	if err != nil {
		return nil, err
	}

	var lines []string
	for _, r := range rows {
		symbol := toStr(r[0])
		path := toStr(r[1])
		fanIn := toInt(r[2])
		fanOut := toInt(r[3])
		loc := toInt(r[4])
		score := toInt(r[5])

		if AnalyzeNoise(path, symbol, opts.IncludeTests) || symbols.IsModuleSymbol(symbol) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s  score=%d  loc=%d  fan_in=%d  fan_out=%d  (%s)",
			ShortName(symbol), score, loc, fanIn, fanOut, path))
		if len(lines) >= limit {
			break
		}
	}
	return lines, nil
}

func hotspots(db *sql.DB, limit int, opts CheckOptions) ([]string, error) {
	scopeClause, scopeParams, err := paths.PathFilterSQL(db, opts.Scope, "def_d")
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT gs.symbol, def_d.relative_path,
			   COUNT(*) AS ref_count,
			   COUNT(DISTINCT ref_d.id) AS file_count
		FROM mentions m
		JOIN chunks c ON m.chunk_id = c.id
		JOIN documents ref_d ON c.document_id = ref_d.id
		JOIN global_symbols gs ON m.symbol_id = gs.id
		%s
		WHERE m.role != 1%s
		GROUP BY gs.id
		ORDER BY ref_count DESC
		LIMIT ?
	`, SymDefJoin, scopeClause)

	rows, err := fetchAllRows(db, query, append(scopeParams, limit*5)...)
	if err != nil {
		return nil, err
	}

	var lines []string
	for _, r := range rows {
		symbol := toStr(r[0])
		path := toStr(r[1])
		refCount := toInt(r[2])
		fileCount := toInt(r[3])

		if AnalyzeNoise(path, symbol, opts.IncludeTests) || symbols.IsModuleSymbol(symbol) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s  refs=%d  files=%d  (%s)",
			ShortName(symbol), refCount, fileCount, path))
		if len(lines) >= limit {
			break
		}
	}
	return lines, nil
}

func cycles(db *sql.DB, limit int, opts CheckOptions) ([]string, error) {
	cap := limit * 5

	twoWayQuery := fmt.Sprintf(`
		WITH edges AS (%s)
		SELECT e1.from_file || ' <-> ' || e1.to_file
		FROM edges e1
		JOIN edges e2 ON e1.from_file = e2.to_file AND e1.to_file = e2.from_file
		WHERE e1.from_file < e1.to_file
		ORDER BY 1
		LIMIT ?
	`, FileEdgesSQL)

	twoWayRows, err := fetchAllRows(db, twoWayQuery, cap)
	if err != nil {
		return nil, err
	}

	var lines []string
	for _, r := range twoWayRows {
		cycleLine := toStr(r[0])
		if !CyclePathNoise(cycleLine, opts.IncludeTests) {
			lines = append(lines, cycleLine)
		}
	}

	edges, err := FetchFileEdges(db)
	if err != nil {
		return nil, err
	}
	longer, _ := FindLongerCycles(edges, 8, cap)
	for _, path := range longer {
		found := false
		for _, l := range lines {
			if l == path {
				found = true
				break
			}
		}
		if !found && !CyclePathNoise(path, opts.IncludeTests) {
			lines = append(lines, path)
		}
	}

	if opts.Scope != "" {
		var filtered []string
		for _, line := range lines {
			if cycleTouchesScope(line, opts.Scope) {
				filtered = append(filtered, line)
			}
		}
		lines = filtered
	}

	if len(lines) > limit {
		lines = lines[:limit]
	}
	return lines, nil
}

func cycleTouchesScope(cycleLine, scope string) bool {
	parts := cycleSplitRe.Split(cycleLine, -1)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && paths.PathInScope(part, scope) {
			return true
		}
	}
	return false
}

func formatDeadExportRows(db *sql.DB, rows [][]interface{}, live *LiveIndex, opts CheckOptions, limit int) ([]string, error) {
	var lines []string
	for _, r := range rows {
		symbolID := toInt(r[0])
		symbol := toStr(r[1])
		path := toStr(r[2])
		loc := toInt(r[3])
		defDocID := toInt(r[4])

		if AnalyzeNoise(path, symbol, opts.IncludeTests) {
			continue
		}
		if HasSameFileReferenceUsage(db, symbolID, defDocID) {
			continue
		}
		if live.DeadExportNoise(symbol, defDocID) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s  loc=%d  (%s)", ShortName(symbol), loc, path))
		if len(lines) >= limit {
			break
		}
	}
	return lines, nil
}

func staleTypes(db *sql.DB, limit int, opts CheckOptions) ([]string, error) {
	scopeClause, scopeParams, err := paths.PathFilterSQL(db, opts.Scope, "def_d")
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT gs.id, gs.symbol, def_d.relative_path, def_d.id,
			   COUNT(DISTINCT CASE WHEN ref_d.id != def_d.id THEN ref_d.id END) AS consumers
		FROM global_symbols gs
		%s
		LEFT JOIN mentions m ON m.symbol_id = gs.id AND m.role != 1
		LEFT JOIN chunks c ON m.chunk_id = c.id
		LEFT JOIN documents ref_d ON c.document_id = ref_d.id
		WHERE gs.symbol LIKE '%%#'
		  AND gs.symbol NOT LIKE '%%().'
		  AND gs.symbol NOT LIKE '%%#typeLiteral%%'%s
		GROUP BY gs.id
		HAVING consumers = 0
		ORDER BY consumers ASC, def_d.relative_path
		LIMIT ?
	`, SymDefJoin, scopeClause)

	rows, err := fetchAllRows(db, query, append(scopeParams, limit*5)...)
	if err != nil {
		return nil, err
	}

	live, err := BuildLiveIndex(db)
	if err != nil {
		return nil, err
	}

	var lines []string
	for _, r := range rows {
		symID := toInt(r[0])
		symbol := toStr(r[1])
		path := toStr(r[2])
		defDocID := toInt(r[3])
		consumers := toInt(r[4])

		if AnalyzeNoise(path, symbol, opts.IncludeTests) {
			continue
		}
		if StaleTypeNoise(path, symbol, consumers) {
			continue
		}
		if consumers == 0 && HasSameFileReferenceUsage(db, symID, defDocID) {
			continue
		}
		if live.StaleTypeLiveNoise(symbol, defDocID) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s  consumers=%d  (%s)", ShortName(symbol), consumers, path))
		if len(lines) >= limit {
			break
		}
	}
	return lines, nil
}

func unreferencedSymbols(db *sql.DB, limit int, opts CheckOptions) ([]string, error) {
	scopeClause, scopeParams, err := paths.PathFilterSQL(db, opts.Scope, "def_d")
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT gs.id, gs.symbol, def_d.relative_path,
			   sym_def.end_line - sym_def.start_line + 1 AS loc,
			   def_d.id
		FROM global_symbols gs
		%s
		WHERE NOT EXISTS (
			SELECT 1 FROM mentions m
			WHERE m.symbol_id = gs.id AND m.role = 0
		)
		AND NOT EXISTS (
			SELECT 1 FROM mentions m
			JOIN chunks c ON m.chunk_id = c.id
			WHERE m.symbol_id = gs.id AND m.role != 1 AND c.document_id != def_d.id
		)%s
		ORDER BY loc DESC, def_d.relative_path
		LIMIT ?
	`, SymDefJoin, scopeClause)

	rows, err := fetchAllRows(db, query, append(scopeParams, limit*5)...)
	if err != nil {
		return nil, err
	}

	live, err := BuildLiveIndex(db)
	if err != nil {
		return nil, err
	}
	return formatDeadExportRows(db, rows, live, opts, limit)
}

func sameFileOnly(db *sql.DB, limit int, opts CheckOptions) ([]string, error) {
	scopeClause, scopeParams, err := paths.PathFilterSQL(db, opts.Scope, "def_d")
	if err != nil {
		return nil, err
	}

	live, err := BuildLiveIndex(db)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT gs.symbol, def_d.relative_path,
			   sym_def.end_line - sym_def.start_line + 1 AS loc,
			   def_d.id
		FROM global_symbols gs
		%s
		WHERE EXISTS (
			SELECT 1 FROM mentions m
			JOIN chunks c ON m.chunk_id = c.id
			WHERE m.symbol_id = gs.id AND m.role = 0 AND c.document_id = def_d.id
		)
		AND NOT EXISTS (
			SELECT 1 FROM mentions m
			JOIN chunks c ON m.chunk_id = c.id
			WHERE m.symbol_id = gs.id AND m.role = 0 AND c.document_id != def_d.id
		)%s
		ORDER BY loc DESC, def_d.relative_path
		LIMIT ?
	`, SymDefJoin, scopeClause)

	rows, err := fetchAllRows(db, query, append(scopeParams, limit*5)...)
	if err != nil {
		return nil, err
	}

	var lines []string
	for _, r := range rows {
		symbol := toStr(r[0])
		path := toStr(r[1])
		loc := toInt(r[2])
		defDocID := toInt(r[3])

		if AnalyzeNoise(path, symbol, opts.IncludeTests) {
			continue
		}
		if live.SameFileExportNoise(symbol, defDocID) {
			continue
		}
		if !FileHasSCIPImporters(db, path, live, defDocID) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s  loc=%d  (%s)", ShortName(symbol), loc, path))
		if len(lines) >= limit {
			break
		}
	}
	return lines, nil
}

func symbolsTestOnlyConsumers(db *sql.DB, limit int, opts CheckOptions) ([]string, error) {
	if opts.IncludeTests {
		return nil, nil
	}

	scopeClause, scopeParams, err := paths.PathFilterSQL(db, opts.Scope, "def_d")
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT gs.symbol, def_d.relative_path,
			   GROUP_CONCAT(DISTINCT ref_d.relative_path) AS consumer_paths
		FROM global_symbols gs
		%s
		JOIN mentions m ON m.symbol_id = gs.id AND m.role != 1
		JOIN chunks c ON m.chunk_id = c.id
		JOIN documents ref_d ON c.document_id = ref_d.id
		WHERE ref_d.id != def_d.id%s
		  AND NOT EXISTS (
			  SELECT 1 FROM mentions m2
			  JOIN chunks c2 ON m2.chunk_id = c2.id
			  WHERE m2.symbol_id = gs.id AND m2.role = 0 AND c2.document_id = def_d.id
		  )
		GROUP BY gs.id
		HAVING COUNT(DISTINCT ref_d.id) > 0
		ORDER BY def_d.relative_path, gs.symbol
		LIMIT ?
	`, SymDefJoin, scopeClause)

	rows, err := fetchAllRows(db, query, append(scopeParams, limit*10)...)
	if err != nil {
		return nil, err
	}

	var lines []string
	for _, r := range rows {
		symbol := toStr(r[0])
		path := toStr(r[1])
		consumerPaths := toStr(r[2])

		if AnalyzeNoise(path, symbol, opts.IncludeTests) {
			continue
		}
		pathList := []string{}
		for _, p := range strings.Split(consumerPaths, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				pathList = append(pathList, p)
			}
		}
		if len(pathList) > 0 {
			allTest := true
			for _, p := range pathList {
				if !IsTestPath(p) {
					allTest = false
					break
				}
			}
			if allTest {
				lines = append(lines, fmt.Sprintf("%s  test_consumers=%d  (%s)", ShortName(symbol), len(pathList), path))
			}
		}
	}
	if len(lines) > limit {
		lines = lines[:limit]
	}
	return lines, nil
}

func deadExports(db *sql.DB, limit int, opts CheckOptions) ([]string, error) {
	scopeClause, scopeParams, err := paths.PathFilterSQL(db, opts.Scope, "def_d")
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT gs.id, gs.symbol, def_d.relative_path,
			   sym_def.end_line - sym_def.start_line + 1 AS loc,
			   def_d.id
		FROM global_symbols gs
		%s
		WHERE NOT EXISTS (
			SELECT 1
			FROM mentions m
			JOIN chunks c ON m.chunk_id = c.id
			WHERE m.symbol_id = gs.id
			  AND m.role != 1
			  AND c.document_id != def_d.id
		)%s
		ORDER BY loc DESC, def_d.relative_path
		LIMIT ?
	`, SymDefJoin, scopeClause)

	rows, err := fetchAllRows(db, query, append(scopeParams, limit*5)...)
	if err != nil {
		return nil, err
	}

	live, err := BuildLiveIndex(db)
	if err != nil {
		return nil, err
	}
	return formatDeadExportRows(db, rows, live, opts, limit)
}

func topCoupling(db *sql.DB, limit int, opts CheckOptions) ([]string, error) {
	scopeClause, scopeParams, err := paths.PathFilterSQLAny(db, opts.Scope, "def_d", "ref_d")
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT def_d.relative_path AS file1,
			   ref_d.relative_path AS file2,
			   COUNT(DISTINCT gs.id) AS shared
		FROM mentions m
		JOIN chunks c ON m.chunk_id = c.id
		JOIN documents ref_d ON c.document_id = ref_d.id
		JOIN global_symbols gs ON m.symbol_id = gs.id
		%s
		WHERE m.role != 1 AND def_d.id != ref_d.id%s
		GROUP BY def_d.id, ref_d.id
		ORDER BY shared DESC
		LIMIT ?
	`, SymDefJoin, scopeClause)

	rows, err := fetchAllRows(db, query, append(scopeParams, limit*5)...)
	if err != nil {
		return nil, err
	}

	var lines []string
	for _, r := range rows {
		file1 := toStr(r[0])
		file2 := toStr(r[1])
		shared := toInt(r[2])

		if FilePairNoise(file1, file2, opts.IncludeTests) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s  <->  %s  shared=%d", file1, file2, shared))
		if len(lines) >= limit {
			break
		}
	}
	return lines, nil
}

func ParsePriorities(priorityStr string) (map[Priority]bool, error) {
	if priorityStr == "" {
		return nil, nil
	}
	aliases := map[string]Priority{
		"1": PriorityHigh, "h": PriorityHigh, "high": PriorityHigh,
		"2": PriorityMedium, "m": PriorityMedium, "medium": PriorityMedium,
		"3": PriorityLow, "l": PriorityLow, "low": PriorityLow,
	}
	result := make(map[Priority]bool)
	for _, p := range strings.Split(strings.ReplaceAll(priorityStr, " ", ""), ",") {
		if p == "" {
			continue
		}
		key := strings.ToLower(p)
		if pr, ok := aliases[key]; ok {
			result[pr] = true
			continue
		}
		return nil, fmt.Errorf("unknown analyze priority %q (use high, medium, low or 1/2/3)", p)
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func NewRowBudget(limit int) *RowBudget {
	return &RowBudget{Remaining: limit}
}

func RunProjectSections(db *sql.DB, limit int, includeTests bool, scope string, priorities map[Priority]bool, budget *RowBudget) ([]SectionResult, error) {
	scopeSuffix := ""
	if scope != "" {
		scopeSuffix = fmt.Sprintf(" [%s]", scope)
	}

	checks := []Check{
		{"cycles", PriorityHigh, fmt.Sprintf("Cycles (file dependencies)%s", scopeSuffix), cycles, ""},
		{"unreferenced", PriorityHigh, fmt.Sprintf("Unreferenced symbols (no refs)%s", scopeSuffix), unreferencedSymbols, ""},
		{"dead_exports", PriorityHigh, fmt.Sprintf("Dead exports (no in-file or external use)%s", scopeSuffix), deadExports, ""},
		{"stale_types", PriorityHigh, fmt.Sprintf("Stale types (no external consumers)%s", scopeSuffix), staleTypes, ""},
		{"same_file_only", PriorityMedium, fmt.Sprintf("Same-file only (in-file use, not exported)%s", scopeSuffix), sameFileOnly, ""},
		{"test_only", PriorityLow, fmt.Sprintf("Test-only consumers (index may miss same-file calls)%s", scopeSuffix), symbolsTestOnlyConsumers, ""},
		{"top_coupling", PriorityLow, fmt.Sprintf("Top coupling (file pairs)%s", scopeSuffix), topCoupling, ""},
		{"bottlenecks", PriorityLow, fmt.Sprintf("Bottlenecks (fan-in x fan-out)%s", scopeSuffix), bottlenecks, ""},
		{"hotspots", PriorityLow, fmt.Sprintf("Hotspots (most referenced)%s", scopeSuffix), hotspots, ""},
	}

	opts := CheckOptions{IncludeTests: includeTests, Scope: scope}
	return RunChecks(checks, db, limit, priorities, opts, budget)
}

const MaxDirFiles = 30

func RunDirSections(db *sql.DB, scope string, limit int, includeTests bool, priorities map[Priority]bool, budget *RowBudget) ([]SectionResult, error) {
	secs, err := RunProjectSections(db, limit, includeTests, scope, priorities, budget)
	if err != nil {
		return nil, err
	}
	files, err := ListDirFiles(db, scope, includeTests)
	if err != nil {
		return nil, err
	}
	total := len(files)
	if total == 0 {
		fmt.Fprintf(os.Stderr, "Note: no indexed files under %s (check path or run scip-cli reindex)\n", scope)
	} else if total > MaxDirFiles {
		fmt.Fprintf(os.Stderr, "Note: %d indexed files under %s; showing first %d (analyze one file for full detail)\n",
			total, scope, MaxDirFiles)
		files = files[:MaxDirFiles]
	}
	if len(files) > 0 {
		title, lines, preface := FormatSection(fmt.Sprintf("Files in %s", scope),
			[]string{fmt.Sprintf("%d shown of %d indexed", len(files), total)}, "")
		secs = append(secs, SectionResult{Title: title, Lines: lines, Preface: preface})
	}
	for _, path := range files {
		if budget.Exhausted() {
			break
		}
		fileSecs, err := RunFileSectionsOnly(db, path, limit, priorities, budget)
		if err != nil {
			return nil, err
		}
		secs = append(secs, fileSecs...)
	}
	return secs, nil
}
