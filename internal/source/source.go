package source

import (
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/flesler/scip-cli-go/internal/queries"
	"github.com/flesler/scip-cli-go/internal/symbols"
)

type pathCacheKey struct {
	root string
	rel  string
}

var (
	resolvedSourcePaths sync.Map
)

func resolveSourcePath(projectRoot, relativePath string) string {
	key := pathCacheKey{root: projectRoot, rel: relativePath}
	if cached, ok := resolvedSourcePaths.Load(key); ok {
		if cached == nil {
			return ""
		}
		return cached.(string)
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		resolvedSourcePaths.Store(key, nil)
		return ""
	}
	fullPath, err := filepath.Abs(filepath.Join(root, relativePath))
	if err != nil {
		resolvedSourcePaths.Store(key, nil)
		return ""
	}
	if rel, err := filepath.Rel(root, fullPath); err != nil || stringsHasPrefixDotDot(filepath.ToSlash(rel)) {
		resolvedSourcePaths.Store(key, nil)
		return ""
	}
	resolvedSourcePaths.Store(key, fullPath)
	return fullPath
}

func stringsHasPrefixDotDot(s string) bool {
	return s == ".." || len(s) > 2 && (s[:2] == ".." || s[:3] == "../")
}

func readAllSourceLines(fullPath string) ([]string, error) {
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, nil
	}
	if !utf8.Valid(data) {
		return nil, nil
	}
	if len(data) == 0 {
		return []string{}, nil
	}
	parts := strings.SplitAfter(string(data), "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts, nil
}

func ReadSourceLines(projectRoot, relativePath string, startLine, endLine *int) ([]string, error) {
	fullPath := resolveSourcePath(projectRoot, relativePath)
	if fullPath == "" {
		return nil, nil
	}

	lines, err := readAllSourceLines(fullPath)
	if err != nil || lines == nil {
		return nil, err
	}
	if startLine != nil && endLine != nil {
		start := *startLine
		end := *endLine
		if start < 0 {
			start = 0
		}
		if end >= len(lines) {
			end = len(lines) - 1
		}
		if start > end || start >= len(lines) {
			return nil, nil
		}
		return lines[start : end+1], nil
	}
	return lines, nil
}

func ResolveDefLocation(db *sql.DB, projectRoot, symbolStr string, symbolID int) (string, *int, *int, error) {
	path, start, end, err := queries.GetDefLocation(db, symbolID)
	if err != nil {
		return "", nil, nil, err
	}
	if path != "" {
		return path, start, end, nil
	}
	path, start, end, ok := fallbackDefLocation(db, projectRoot, symbolStr)
	if !ok {
		return "", nil, nil, nil
	}
	return path, start, end, nil
}

func fallbackDefLocation(db *sql.DB, projectRoot, symbolStr string) (string, *int, *int, bool) {
	relPath, err := queries.ResolveDocumentPath(db, symbolStr)
	if err != nil || relPath == "" {
		return "", nil, nil, false
	}
	leaf := symbols.ExtractLeafName(symbolStr)
	lines, err := ReadSourceLines(projectRoot, relPath, nil, nil)
	if err != nil || lines == nil {
		return "", nil, nil, false
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`^\s*(?:async\s+)?` + regexp.QuoteMeta(leaf) + `[\(<]`),
		regexp.MustCompile(`^\s*(?:public\s+|private\s+|protected\s+|static\s+|readonly\s+)*` + regexp.QuoteMeta(leaf) + `\??\s*[:=(]`),
		regexp.MustCompile(`^\s*(?:public\s+|private\s+|protected\s+|static\s+|readonly\s+)*` + regexp.QuoteMeta(leaf) + `\(`),
		regexp.MustCompile(`^\s*` + regexp.QuoteMeta(leaf) + `\([^)]*\)\s*:\s*`),
	}
	for i, line := range lines {
		for _, p := range patterns {
			if p.MatchString(line) {
				lineNum := i
				return relPath, &lineNum, &lineNum, true
			}
		}
	}
	return "", nil, nil, false
}
