package commands

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/flesler/scip-cli-go/v2/internal/clierr"
	"github.com/flesler/scip-cli-go/v2/internal/output"
	"github.com/flesler/scip-cli-go/v2/internal/queries"
	"github.com/flesler/scip-cli-go/v2/internal/session"
	"github.com/flesler/scip-cli-go/v2/internal/source"
	"github.com/flesler/scip-cli-go/v2/internal/symbols"
)

func resolveSymbolGroups(db *sql.DB, names []string, kind *symbols.SymbolKind, limit int, pathScope string) ([]struct {
	queryName string
	symbols   []queries.SymbolResult
}, error) {
	var groups []struct {
		queryName string
		symbols   []queries.SymbolResult
	}

	for _, queryName := range names {
		limitPlusOne := limit + 1
		syms, err := queries.ResolveSymbol(db, queryName, kind, &limitPlusOne, pathScope)
		if err != nil {
			return nil, err
		}
		if len(syms) == 0 {
			fmt.Fprintf(os.Stderr, "Symbol '%s' not found\n", queryName)
			continue
		}
		syms = output.LimitAndWarn(syms, limit, "symbols")
		groups = append(groups, struct {
			queryName string
			symbols   []queries.SymbolResult
		}{queryName, syms})
	}
	return groups, nil
}

func CodeMain(args map[string]interface{}) error {
	db, projectRoot, err := session.Setup()
	if err != nil {
		return err
	}
	defer db.Close()

	pathScope := args["path_scope"].(string)
	limit := args["limit"].(int)
	symbolNames := args["symbol"].([]string)
	kind := args["kind"].(*symbols.SymbolKind)

	groups, err := resolveSymbolGroups(db, symbolNames, kind, limit, pathScope)
	if err != nil {
		return err
	}

	total := 0
	for _, g := range groups {
		total += len(g.symbols)
	}
	if total == 0 {
		return clierr.Exit(1)
	}

	showHeaders := total > 1
	snippetMode := args["snippet"].(bool)
	fullMode := args["full"].(bool)
	offset := args["offset"].(int)
	lineNumbers := args["line_numbers"].(bool)

	if offset < 0 {
		return fmt.Errorf("--offset must be >= 0, got %d", offset)
	}

	var maxDefLines int
	var maxDefChars int
	if snippetMode {
		maxDefLines = 1
		maxDefChars = 0
	} else if fullMode {
		maxDefLines = 0
		maxDefChars = 0
	} else {
		var maxLinesPtr *int
		if ml, ok := args["max_lines"].(int); ok && ml >= 0 {
			maxLinesPtr = &ml
		}
		resolved, err := output.ResolveMaxDefLines(maxLinesPtr)
		if err != nil {
			return err
		}
		maxDefLines = resolved
		if maxDefLines == 0 {
			maxDefChars = 0
		} else {
			maxDefChars = output.DefaultMaxDefChars
		}
	}

	printed := 0
	for _, group := range groups {
		for _, sym := range group.symbols {
			path, startLine, endLine, err := source.ResolveDefLocation(db, projectRoot, sym.Symbol, sym.ID)
			if err != nil || path == "" {
				fmt.Fprintf(os.Stderr, "Warning: no definition location for '%s'\n", group.queryName)
				continue
			}

			startVal := 0
			endVal := 0
			if startLine != nil {
				startVal = *startLine
			}
			if endLine != nil {
				endVal = *endLine
			}
			defBodyLines := endVal - startVal + 1

			label := output.SymbolOutputLabel(group.queryName, sym.Symbol, len(group.symbols))
			output.MaybePrintSymbolHeader(label, showHeaders)

			if snippetMode {
				lines, _ := source.ReadSourceLines(projectRoot, path, startLine, startLine)
				if lines == nil {
					fmt.Printf("%s:%s [file not found]\n", path, output.FormatLineRange(startLine, endLine, ":"))
					printed++
					continue
				}
				firstLine := strings.TrimRight(lines[0], "\n")
				if lineNumbers {
					firstLine = fmt.Sprintf("%d|%s", startVal+1, firstLine)
				}
				fmt.Printf("%s:%s %s\n", path, output.FormatLineRange(startLine, endLine, ":"), firstLine)
				printed++
				continue
			}

			lines, _ := source.ReadSourceLines(projectRoot, path, startLine, endLine)
			if lines != nil && offset >= len(lines) {
				fmt.Fprintf(os.Stderr, "Warning: offset %d is beyond definition (lines %d-%d)\n", offset, startVal+1, endVal+1)
				fmt.Printf("%s:%s\n", path, output.FormatLineRange(startLine, endLine, ":"))
				printed++
				continue
			}

			result, err := output.FormatDefBody(lines, startVal, endVal, maxDefLines, maxDefChars, offset, lineNumbers)
			if err != nil {
				return err
			}
			fmt.Printf("%s:%s\n", path, output.FormatLineRange(&result.StartLine, &result.EndLine, ":"))
			fmt.Print(result.Body)
			if result.Truncated {
				linesShown := result.EndLine - result.StartLine + 1
				lineLimited := maxDefLines > 0 && linesShown >= maxDefLines
				if lineLimited && offset+linesShown < defBodyLines {
					output.PrintDefTruncationNotice(group.queryName, offset, linesShown, defBodyLines)
				}
			}
			printed++
		}
	}

	if printed == 0 {
		return clierr.Exit(1)
	}
	return nil
}
