package analyze

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/flesler/scip-cli-go/v2/internal/paths"
	"github.com/flesler/scip-cli-go/v2/internal/queries"
	"github.com/flesler/scip-cli-go/v2/internal/sqlhelp"
	"github.com/flesler/scip-cli-go/v2/internal/symbols"
)

type AnalyzeKind string

const (
	KindDir    AnalyzeKind = "dir"
	KindFile   AnalyzeKind = "file"
	KindSymbol AnalyzeKind = "symbol"
)

type AnalyzeTarget struct {
	Kind       AnalyzeKind
	Scope      string
	SymbolName string
}

func filesystemDirScope(target, projectRoot string) string {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return ""
	}

	candidates := []string{target}
	if trimmed := strings.TrimRight(target, "/\\"); trimmed != target {
		candidates = append(candidates, trimmed)
	}

	for _, c := range candidates {
		candidate := c
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(root, candidate)
		}
		candidate, err = filepath.Abs(candidate)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, candidate)
		if err != nil {
			continue
		}
		if strings.HasPrefix(rel, "..") {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return filepath.ToSlash(rel)
		}
	}
	return ""
}

func indexedDirScope(db *sql.DB, target string) string {
	norm := strings.ReplaceAll(target, "\\", "/")
	norm = strings.TrimSpace(norm)
	norm = strings.TrimRight(norm, "/")
	if norm == "" {
		return ""
	}

	var exists int
	err := sqlhelp.DebugExecuteOne(db, "SELECT 1 FROM documents WHERE relative_path = ? LIMIT 1", norm).Scan(&exists)
	if err == nil {
		return ""
	}

	escaped := sqlhelp.EscapeLike(norm)
	rows, err := sqlhelp.DebugExecute(db,
		"SELECT relative_path FROM documents WHERE relative_path LIKE ? ESCAPE '\\' ORDER BY relative_path LIMIT 1",
		escaped+"/%")
	if err != nil {
		return ""
	}
	defer rows.Close()

	if rows.Next() {
		return norm
	}
	return ""
}

func ResolveAnalyzeTarget(db *sql.DB, target, projectRoot, pathScope string) (*AnalyzeTarget, error) {
	stripped := strings.TrimSpace(target)
	if stripped == "" {
		return nil, fmt.Errorf("analyze target cannot be empty")
	}

	dirScope := filesystemDirScope(stripped, projectRoot)
	if dirScope == "" {
		dirScope = indexedDirScope(db, stripped)
	}
	if dirScope != "" {
		if pathScope != "" && !paths.PathInScope(dirScope, pathScope) {
			return nil, fmt.Errorf("target %q is outside --path %q", target, pathScope)
		}
		return &AnalyzeTarget{Kind: KindDir, Scope: dirScope}, nil
	}

	files, err := queries.ResolveFile(db, stripped, pathScope)
	if err != nil {
		return nil, err
	}
	if len(files) > 1 {
		lines := strings.Join(files[:min(10, len(files))], "\n  ")
		extra := ""
		if len(files) > 10 {
			extra = fmt.Sprintf("\n  ... and %d more", len(files)-10)
		}
		return nil, fmt.Errorf("ambiguous file %q:\n  %s%s", target, lines, extra)
	}
	if len(files) == 1 {
		return &AnalyzeTarget{Kind: KindFile, Scope: files[0]}, nil
	}

	return &AnalyzeTarget{Kind: KindSymbol, SymbolName: stripped}, nil
}

func ListDirFiles(db *sql.DB, scope string, includeTests bool) ([]string, error) {
	allPaths, err := paths.ListIndexedPathsInScope(db, scope)
	if err != nil {
		return nil, err
	}

	var result []string
	scopeBase := strings.TrimRight(scope, "/")
	for _, p := range allPaths {
		if p == scopeBase {
			continue
		}
		if !includeTests && IsTestPath(p) {
			continue
		}
		result = append(result, p)
	}
	return result, nil
}

var backtickRe = regexp.MustCompile("`([^`]+)`")

func defDocIDFromSymbol(db *sql.DB, symbol string) (int, bool, error) {
	match := backtickRe.FindStringSubmatch(symbol)
	if match == nil {
		return 0, false, nil
	}
	filename := match[1]

	var docID int
	err := sqlhelp.DebugExecuteOne(db,
		"SELECT id FROM documents WHERE relative_path GLOB ? OR relative_path = ? LIMIT 1",
		"*/"+filename, filename).Scan(&docID)
	if err != nil {
		return 0, false, nil
	}
	return docID, true, nil
}

func IsExportAliasSymbol(symbol string) bool {
	parts := strings.Split(symbol, "/")
	tail := parts[len(parts)-1]
	return strings.HasSuffix(tail, "0:") && !strings.Contains(tail, "().")
}

func ExportAliasBase(symbol string) string {
	if !IsExportAliasSymbol(symbol) {
		return ""
	}
	parts := strings.Split(symbol, "/")
	tail := parts[len(parts)-1]
	return tail[:len(tail)-2]
}

func ExportValueBase(symbol string) string {
	if strings.HasSuffix(symbol, "().") {
		leaf := symbols.ExtractLeafName(symbol)
		if leaf != "" {
			return leaf
		}
		return ""
	}
	if strings.HasSuffix(symbol, "#") && !strings.Contains(symbol, "#typeLiteral") {
		parts := strings.Split(symbol, "/")
		tail := parts[len(parts)-1]
		if !strings.Contains(tail, "().") {
			leaf := symbols.ExtractLeafName(symbol)
			if leaf != "" {
				return leaf
			}
		}
	}
	return ""
}

const externalMentionSQL = `
    SELECT 1 FROM mentions m
    JOIN chunks c ON m.chunk_id = c.id
    WHERE m.symbol_id = ? AND m.role != 1 AND c.document_id != ?
    LIMIT 1
`

type LiveIndex struct {
	LiveModuleDocs  map[int]bool
	LiveAliasBases  map[string]bool
	ModuleImporters map[int]int
}

func BuildLiveIndex(db *sql.DB) (*LiveIndex, error) {
	idx := &LiveIndex{
		LiveModuleDocs:  make(map[int]bool),
		LiveAliasBases:  make(map[string]bool),
		ModuleImporters: make(map[int]int),
	}

	rows, err := fetchAllRows(db, `
		SELECT der.document_id, COUNT(DISTINCT c.document_id)
		FROM global_symbols gs
		JOIN defn_enclosing_ranges der ON der.symbol_id = gs.id
		JOIN mentions m ON m.symbol_id = gs.id AND m.role != 1
		JOIN chunks c ON m.chunk_id = c.id
		WHERE gs.symbol LIKE '%/' AND gs.symbol NOT LIKE '%().'
		  AND c.document_id != der.document_id
		GROUP BY der.document_id
	`)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		docID := toInt(r[0])
		count := toInt(r[1])
		idx.LiveModuleDocs[docID] = true
		idx.ModuleImporters[docID] = count
	}

	symRows, err := fetchAllRows(db, `
		SELECT id, symbol FROM global_symbols
		WHERE symbol LIKE '%0:' AND symbol NOT LIKE '%().'
	`)
	if err != nil {
		return nil, err
	}
	for _, r := range symRows {
		symID := toInt(r[0])
		symbol := toStr(r[1])
		base := ExportAliasBase(symbol)
		if base == "" {
			continue
		}
		defDoc, ok, err := defDocIDFromSymbol(db, symbol)
		if err != nil || !ok {
			continue
		}
		row, err := fetchOneRow(db, externalMentionSQL, symID, defDoc)
		if err != nil {
			continue
		}
		if row != nil {
			idx.LiveAliasBases[base] = true
		}
	}

	return idx, nil
}

func (li *LiveIndex) PossiblyLiveLabel(symbol string, defDocID int) string {
	if symbols.IsModuleSymbol(symbol) {
		if !li.LiveModuleDocs[defDocID] {
			return ""
		}
		count := li.ModuleImporters[defDocID]
		return fmt.Sprintf("module_import:%d", count)
	}
	base := ExportValueBase(symbol)
	if base == "" {
		return ""
	}
	if li.LiveAliasBases[base] {
		return "export_alias"
	}
	if li.LiveModuleDocs[defDocID] {
		count := li.ModuleImporters[defDocID]
		return fmt.Sprintf("default_export:%d", count)
	}
	return ""
}

func (li *LiveIndex) DeadExportNoise(symbol string, defDocID int) bool {
	return li.PossiblyLiveLabel(symbol, defDocID) != "" || symbols.IsModuleSymbol(symbol)
}

func (li *LiveIndex) SameFileExportNoise(symbol string, defDocID int) bool {
	base := ExportValueBase(symbol)
	if base == "" {
		return false
	}
	if li.LiveAliasBases[base] {
		return true
	}
	return li.LiveModuleDocs[defDocID]
}

func (li *LiveIndex) StaleTypeLiveNoise(symbol string, defDocID int) bool {
	base := ExportValueBase(symbol)
	if base == "" {
		return false
	}
	if li.LiveAliasBases[base] {
		return true
	}
	return li.LiveModuleDocs[defDocID]
}

func HasSameFileReferenceUsage(db *sql.DB, symbolID, defDocID int) bool {
	row, err := fetchOneRow(db, `
		SELECT 1 FROM mentions m
		JOIN chunks c ON m.chunk_id = c.id
		WHERE m.symbol_id = ? AND m.role = 0 AND c.document_id = ?
		LIMIT 1
	`, symbolID, defDocID)
	if err != nil {
		return false
	}
	return row != nil
}

func FileHasSCIPImporters(db *sql.DB, relativePath string, live *LiveIndex, defDocID int) bool {
	if live.LiveModuleDocs[defDocID] {
		return true
	}

	fileSymbols, err := queries.GetFileSymbols(db, relativePath, nil)
	if err != nil || len(fileSymbols) == 0 {
		return false
	}

	symbolIDs := make([]int, len(fileSymbols))
	for i, fs := range fileSymbols {
		symbolIDs[i] = fs.ID
	}

	importers, err := queries.GetImporterPaths(db, symbolIDs, relativePath, nil)
	if err != nil {
		return false
	}
	return len(importers) > 0
}
