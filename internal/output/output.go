package output

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/flesler/scip-cli-go/v2/internal/queries"
	"github.com/flesler/scip-cli-go/v2/internal/symbols"
)

const (
	DefaultMaxDefLines = 80
	DefaultMaxDefChars = 32000
)

func ambiguousLabel(match interface{}) string {
	if tuple, ok := match.([]interface{}); ok && len(tuple) > 1 {
		if symbolOrPath, ok := tuple[1].(string); ok {
			if strings.HasPrefix(symbolOrPath, "scip-") {
				leaf := symbols.ExtractLeafName(symbolOrPath)
				relPath := symbols.ExtractFilePathFromSymbol(symbolOrPath)
				if relPath != "" {
					return fmt.Sprintf("%s (%s)", leaf, relPath)
				}
				return leaf
			}
			return symbolOrPath
		}
	}
	if s, ok := match.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", match)
}

func SymbolOutputLabel(queryName, symbolStr string, matchesForQuery int) string {
	if matchesForQuery > 1 {
		return ambiguousLabel([]interface{}{nil, symbolStr})
	}
	return queryName
}

func MaybePrintSymbolHeader(label string, showHeader bool) {
	if showHeader {
		fmt.Println(label)
	}
}

func WarnAmbiguous(name string, matches []interface{}, context string) {
	if len(matches) <= 1 {
		return
	}
	label := ambiguousLabel(matches[0])
	fmt.Fprintf(os.Stderr, "Ambiguous %s '%s' (%d matches). Using first match: %s\n",
		context, name, len(matches), label)
}

func WarnAmbiguousRefs(name string, matches []queries.SymbolResult, db *sql.DB) {
	if len(matches) <= 1 {
		return
	}
	var parts []string
	for _, m := range matches {
		label := ambiguousLabel([]interface{}{nil, m.Symbol})
		count, err := queries.SymbolExternalRefCount(db, m.ID)
		if err != nil {
			count = 0
		}
		parts = append(parts, fmt.Sprintf("%s ext_refs=%d", label, count))
	}
	fmt.Fprintf(os.Stderr, "Ambiguous symbol '%s' (%d matches). Use --path to narrow. %s\n",
		name, len(matches), strings.Join(parts, "; "))
}

func FormatLineRange(startLine, endLine *int, sep string) string {
	if startLine != nil && endLine != nil {
		return fmt.Sprintf("%d%s%d", *startLine+1, sep, *endLine+1)
	}
	if startLine != nil {
		return fmt.Sprintf("%d%s?", *startLine+1, sep)
	}
	return "??"
}

func LimitAndWarn[T any](items []T, limit int, label string) []T {
	if len(items) > limit {
		fmt.Fprintf(os.Stderr, "# Warning: more than %d %s, showing first %d\n", limit, label, limit)
		return items[:limit]
	}
	return items
}

func ResolveMaxDefLines(cliValue *int) (int, error) {
	if cliValue != nil {
		if *cliValue < 0 {
			return 0, fmt.Errorf("--max-lines must be >= 0, got %d", *cliValue)
		}
		return *cliValue, nil
	}
	env := os.Getenv("SCIP_CLI_MAX_DEF_LINES")
	if env != "" {
		value, err := strconv.Atoi(env)
		if err != nil {
			return 0, fmt.Errorf("invalid SCIP_CLI_MAX_DEF_LINES: expected an integer, got %q", env)
		}
		if value < 0 {
			return 0, fmt.Errorf("SCIP_CLI_MAX_DEF_LINES must be >= 0, got %d", value)
		}
		return value, nil
	}
	return DefaultMaxDefLines, nil
}

type FormatDefBodyResult struct {
	Body      string
	Truncated bool
	StartLine int
	EndLine   int
}

func FormatDefBody(lines []string, startLine, endLine int, maxLines, maxChars, offset int, lineNumbers bool) (*FormatDefBodyResult, error) {
	if lines == nil {
		return &FormatDefBodyResult{
			Body:      "(could not read source)",
			Truncated: false,
			StartLine: startLine,
			EndLine:   endLine,
		}, nil
	}

	if maxLines == 0 && maxChars == 0 {
		body := strings.Join(lines, "")
		body = strings.TrimRight(body, "\n")
		if lineNumbers {
			body = addLineNumbers(body, startLine)
		}
		return &FormatDefBodyResult{
			Body:      body,
			Truncated: false,
			StartLine: startLine,
			EndLine:   endLine,
		}, nil
	}

	if maxLines < 0 {
		maxLines = DefaultMaxDefLines
	}

	selected := make([]string, len(lines))
	copy(selected, lines)
	truncated := false

	if offset > 0 {
		if offset >= len(selected) {
			selected = nil
		} else {
			selected = selected[offset:]
		}
		startLine += offset
	}

	if maxLines > 0 && len(selected) > maxLines {
		selected = selected[:maxLines]
		truncated = true
	}

	body := strings.Join(selected, "")
	body = strings.TrimRight(body, "\n")

	if maxChars > 0 && len(body) > maxChars {
		body = body[:maxChars]
		body = strings.TrimRight(body, "\n")
		body += "\n..."
		truncated = true
	}

	if lineNumbers {
		body = addLineNumbers(body, startLine)
	}

	shownEnd := startLine
	if len(selected) > 0 {
		shownEnd = startLine + len(selected) - 1
	}

	return &FormatDefBodyResult{
		Body:      body,
		Truncated: truncated,
		StartLine: startLine,
		EndLine:   shownEnd,
	}, nil
}

func addLineNumbers(body string, startLine int) string {
	lines := strings.Split(body, "\n")
	var numbered []string
	for i, line := range lines {
		lineNum := startLine + i + 1
		numbered = append(numbered, fmt.Sprintf("%d|%s", lineNum, line))
	}
	return strings.Join(numbered, "\n")
}

func PrintDefTruncationNotice(queryName string, bodyOffset, linesShown, defBodyLines int) {
	nextOffset := bodyOffset + linesShown
	if nextOffset >= defBodyLines {
		return
	}
	atLine := bodyOffset + linesShown
	fmt.Fprintf(os.Stderr, "Warning: truncated at line %d/%d of definition. Continue: code --offset %d %s\n",
		atLine, defBodyLines, nextOffset, queryName)
}
