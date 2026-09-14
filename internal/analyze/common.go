package analyze

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/flesler/scip-cli-go/v2/internal/sqlhelp"
	"github.com/flesler/scip-cli-go/v2/internal/symbols"
)

const DefaultLimit = 20

// Cap rows scanned when post-filtering SQL (filters drop many SCIP false hits).
const AnalyzeMaxScanRows = 10_000

// CollectUntilLimit fetches SQL pages until limit rows pass accept, or data is exhausted.
// Use only when a check post-filters SQL rows; checks that format every row as-is should use LIMIT ? only.
func CollectUntilLimit(
	limit int,
	fetchPage func(pageSize, offset int) ([][]interface{}, error),
	accept func(row []interface{}) string,
	pageSize int,
	maxScan int,
) ([]string, error) {
	if pageSize <= 0 {
		if limit > 50 {
			pageSize = limit
		} else {
			pageSize = 50
		}
	}
	if maxScan <= 0 {
		maxScan = AnalyzeMaxScanRows
	}

	var lines []string
	offset := 0
	scanned := 0
	for len(lines) < limit {
		rows, err := fetchPage(pageSize, offset)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			scanned++
			if scanned > maxScan {
				return lines, nil
			}
			line := accept(row)
			if line == "" {
				continue
			}
			lines = append(lines, line)
			if len(lines) >= limit {
				return lines, nil
			}
		}
		if len(rows) < pageSize {
			break
		}
		offset += len(rows)
	}
	return lines, nil
}

// CollectFromProducer fills limit rows from a capped producer that can return more candidates on retry.
func CollectFromProducer(
	limit int,
	produce func(cap int) ([]string, error),
	accept func(candidate string) string,
	maxScan int,
) ([]string, error) {
	if maxScan <= 0 {
		maxScan = AnalyzeMaxScanRows
	}

	var lines []string
	seen := make(map[string]bool)
	scanned := 0
	fetchCap := limit
	for len(lines) < limit && fetchCap <= maxScan {
		candidates, err := produce(fetchCap)
		if err != nil {
			return nil, err
		}
		if len(candidates) == 0 {
			break
		}
		for _, candidate := range candidates {
			if seen[candidate] {
				continue
			}
			seen[candidate] = true
			scanned++
			if scanned > maxScan {
				return lines, nil
			}
			line := accept(candidate)
			if line == "" {
				continue
			}
			lines = append(lines, line)
			if len(lines) >= limit {
				return lines, nil
			}
		}
		if len(candidates) < fetchCap {
			break
		}
		fetchCap += limit
	}
	return lines, nil
}

// CollectFromIterable filters an in-memory candidate list until limit rows pass accept.
func CollectFromIterable(
	limit int,
	candidates []string,
	accept func(candidate string) string,
	maxScan int,
) []string {
	if maxScan <= 0 {
		maxScan = AnalyzeMaxScanRows
	}

	var lines []string
	scanned := 0
	for _, candidate := range candidates {
		scanned++
		if scanned > maxScan {
			break
		}
		line := accept(candidate)
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) >= limit {
			break
		}
	}
	return lines
}

const SymDefJoin = `
    JOIN defn_enclosing_ranges sym_def ON sym_def.symbol_id = gs.id
    JOIN documents def_d ON sym_def.document_id = def_d.id
`

func FetchAll(db *sql.DB, query string, args ...interface{}) ([][]interface{}, error) {
	rows, err := sqlhelp.DebugExecute(db, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result [][]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		result = append(result, vals)
	}
	return result, rows.Err()
}

func FetchAllStrings(db *sql.DB, query string, args ...interface{}) ([]string, error) {
	rows, err := sqlhelp.DebugExecute(db, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func FetchOne(db *sql.DB, query string, args ...interface{}) ([]interface{}, error) {
	row := sqlhelp.DebugExecuteOne(db, query, args...)
	cols := 1
	vals := make([]interface{}, cols)
	ptrs := make([]interface{}, cols)
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := row.Scan(ptrs...); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return vals, nil
}

func FetchOneRow(db *sql.DB, ncols int, query string, args ...interface{}) ([]interface{}, error) {
	row := sqlhelp.DebugExecuteOne(db, query, args...)
	vals := make([]interface{}, ncols)
	ptrs := make([]interface{}, ncols)
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := row.Scan(ptrs...); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return vals, nil
}

func ShortName(symbol string) string {
	if symbols.IsModuleSymbol(symbol) {
		return "(module)"
	}
	leaf := symbols.ExtractLeafName(symbol)
	if leaf != "" {
		return leaf
	}
	if strings.HasSuffix(symbol, "/") {
		return "(module)"
	}
	parts := strings.Split(symbol, "/")
	last := parts[len(parts)-1]
	if len(last) > 60 {
		last = last[:60]
	}
	return last
}

func IsTestPath(relativePath string) bool {
	p := strings.ReplaceAll(relativePath, "\\", "/")
	lower := strings.ToLower(p)
	if strings.HasPrefix(lower, "tests/") || strings.HasPrefix(lower, "test/") {
		return true
	}
	if strings.Contains(lower, "/tests/") || strings.Contains(lower, "/test/") || strings.Contains(lower, "/__tests__/") {
		return true
	}
	name := lower
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	if strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") || strings.Contains(name, ".mocha.") {
		return true
	}
	stem := name
	if dot := strings.LastIndex(name, "."); dot >= 0 {
		stem = name[:dot]
	}
	if strings.HasSuffix(stem, "_test") || strings.HasSuffix(stem, "_spec") {
		return true
	}
	if strings.Contains(name, "test-fixture") || strings.Contains(name, "test_fixture") || strings.Contains(name, "test-mocks") {
		return true
	}
	if strings.Contains(lower, "/__fixtures__/") || strings.Contains(lower, "/__mocks__/") {
		return true
	}
	if strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py") {
		return true
	}
	return name == "conftest.py"
}

func IsDynamicLoaderPath(relativePath string) bool {
	p := strings.ReplaceAll(relativePath, "\\", "/")
	return strings.Contains(strings.ToLower(p), "/migrations/")
}

func isPythonMainEntrypoint(relativePath, symbol string) bool {
	if ShortName(symbol) != "main" {
		return false
	}
	return strings.HasSuffix(strings.ReplaceAll(relativePath, "\\", "/"), "__main__.py")
}

func AnalyzeNoise(relativePath, symbol string, includeTests bool) bool {
	if !includeTests && IsTestPath(relativePath) {
		return true
	}
	if strings.HasPrefix(ShortName(symbol), "_") {
		return true
	}
	if isPythonMainEntrypoint(relativePath, symbol) {
		return true
	}
	if IsDynamicLoaderPath(relativePath) {
		return true
	}
	name := ShortName(symbol)
	if name == "<constructor>" || name == "constructor" {
		return true
	}
	return isAnalyzeDashboardExport(relativePath, symbol)
}

var analyzeDashboardSuffixes = []string{
	"analyze/project.py",
	"analyze/file.py",
	"analyze/symbol.py",
}

func isAnalyzeDashboardExport(relativePath, symbol string) bool {
	path := strings.ReplaceAll(relativePath, "\\", "/")
	matched := false
	for _, suffix := range analyzeDashboardSuffixes {
		if strings.HasSuffix(path, suffix) {
			matched = true
			break
		}
	}
	if !matched {
		return false
	}
	name := ShortName(symbol)
	return strings.Contains(symbol, "().") || strings.HasSuffix(name, ")")
}

func isComponentPropsType(symbol string) bool {
	name := ShortName(symbol)
	if name == "" || !strings.HasSuffix(name, "Props") {
		return false
	}
	tail := symbol[strings.LastIndex(symbol, "/")+1:]
	return strings.Contains(tail, "#")
}

func StaleTypeNoise(relativePath, symbol string, consumers int) bool {
	if isComponentPropsType(symbol) {
		return true
	}
	if consumers > 0 {
		return false
	}
	name := ShortName(symbol)
	if name == "" || name == "(module)" || !isUpper(name[0]) {
		return false
	}
	path := strings.ReplaceAll(relativePath, "\\", "/")
	return strings.HasSuffix(path, "config.py") ||
		strings.HasSuffix(path, "scope.py") ||
		strings.HasSuffix(path, "analyze/targets.py")
}

func isUpper(c byte) bool {
	return c >= 'A' && c <= 'Z'
}

func FilePairNoise(file1, file2 string, includeTests bool) bool {
	if includeTests {
		return false
	}
	return IsTestPath(file1) || IsTestPath(file2)
}

var cycleSplitRe = regexp.MustCompile(`\s<->\s|\s->\s`)

func CyclePathNoise(cycleLine string, includeTests bool) bool {
	if includeTests {
		return false
	}
	parts := cycleSplitRe.Split(cycleLine, -1)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && IsTestPath(part) {
			return true
		}
	}
	return false
}

type SectionResult struct {
	Title   string
	Lines   []string
	Preface string
}

func Section(title string, lines []string, preface string) SectionResult {
	if len(lines) == 0 {
		return SectionResult{Title: title, Lines: []string{"(none)"}}
	}
	return SectionResult{Title: title, Lines: lines, Preface: preface}
}

func FormatSection(title string, lines []string, preface string) (string, []string, string) {
	if len(lines) == 0 {
		return title, []string{"(none)"}, preface
	}
	return title, lines, preface
}

func fetchAllRows(db *sql.DB, query string, args ...interface{}) ([][]interface{}, error) {
	return FetchAll(db, query, args...)
}

func fetchOneRow(db *sql.DB, query string, args ...interface{}) ([]interface{}, error) {
	return FetchOne(db, query, args...)
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	default:
		return 0
	}
}

func toStr(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		return fmt.Sprintf("%v", v)
	}
}
