package commands

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/flesler/scip-cli-go/internal/clierr"
	"github.com/flesler/scip-cli-go/internal/output"
	"github.com/flesler/scip-cli-go/internal/queries"
	"github.com/flesler/scip-cli-go/internal/session"
	"github.com/flesler/scip-cli-go/internal/source"
	"github.com/flesler/scip-cli-go/internal/symbols"
)

func memberSourcePatterns(memberSymbol, short string, kind symbols.SymbolKind) (string, string) {
	var tsPattern string
	if strings.Contains(memberSymbol, "<constructor>") {
		tsPattern = `^\s*constructor\s*\(`
	} else if strings.Contains(memberSymbol, "<get>") {
		tsPattern = `^\s*(?:public\s+|private\s+|protected\s+|static\s+|readonly\s+)*get\s+` + regexp.QuoteMeta(short) + `\s*\(`
	} else if strings.Contains(memberSymbol, "<set>") {
		tsPattern = `^\s*(?:public\s+|private\s+|protected\s+|static\s+|readonly\s+)*set\s+` + regexp.QuoteMeta(short) + `\s*\(`
	} else {
		prefix := `^\s*(?:public\s+|private\s+|protected\s+|static\s+|readonly\s+)*`
		tsPattern = prefix + regexp.QuoteMeta(short) + `\s*\??\s*[:=(]`
	}

	var pyPattern string
	if kind == symbols.KindMethod {
		pyPattern = `^\s*(?:async\s+)?def\s+` + regexp.QuoteMeta(short) + `\s*\(`
	} else if kind == symbols.KindProperty {
		pyPattern = `^\s*` + regexp.QuoteMeta(short) + `\s*[=:]`
	} else if kind == symbols.KindClass {
		pyPattern = `^\s*class\s+` + regexp.QuoteMeta(short) + `\s*[:\(]`
	}

	return tsPattern, pyPattern
}

func MembersMain(args map[string]interface{}) error {
	db, projectRoot, err := session.Setup()
	if err != nil {
		return err
	}
	defer db.Close()

	pathScope := args["path_scope"].(string)
	limit := args["limit"].(int)
	symbolName := args["symbol"].(string)
	namesOnly := args["names_only"].(bool)

	sym, err := session.ResolveOneSymbol(db, symbolName, nil, pathScope)
	if err != nil {
		return err
	}

	members, truncated, err := queries.GetMembers(db, sym.ID)
	if err != nil {
		return err
	}
	if truncated {
		fmt.Fprintf(os.Stderr, "Warning: member list truncated at 500 (index cap)\n")
	}

	members = output.LimitAndWarn(members, limit, "members")

	if len(members) == 0 {
		fmt.Fprintf(os.Stderr, "No members found for '%s'\n", symbolName)
		return clierr.Exit(1)
	}

	parentPath, parentStart, parentEnd, err := queries.GetDefLocation(db, sym.ID)
	if err != nil {
		return err
	}

	needsLookup := false
	for _, m := range members {
		if !m.StartLine.Valid {
			needsLookup = true
			break
		}
	}

	var sourceLines []string
	if needsLookup && projectRoot != "" && parentPath != "" && parentStart != nil {
		sourceLines, _ = source.ReadSourceLines(projectRoot, parentPath, parentStart, parentEnd)
	}

	for _, member := range members {
		kind := symbols.InferKind(member.Symbol)
		short := symbols.ExtractLeafName(member.Symbol)

		startLine := member.StartLine
		endLine := member.EndLine

		if !startLine.Valid && len(sourceLines) > 0 {
			tsPattern, pyPattern := memberSourcePatterns(member.Symbol, short, kind)
			var patterns []string
			if strings.HasSuffix(parentPath, ".py") {
				if pyPattern != "" {
					patterns = append(patterns, pyPattern)
				}
				patterns = append(patterns, tsPattern)
			} else {
				patterns = append(patterns, tsPattern)
				if pyPattern != "" {
					patterns = append(patterns, pyPattern)
				}
			}

			for i, line := range sourceLines {
				for _, p := range patterns {
					if matched, _ := regexp.MatchString(p, line); matched {
						lineNum := *parentStart + i
						startLine = sql.NullInt64{Int64: int64(lineNum), Valid: true}
						endLine = sql.NullInt64{Int64: int64(lineNum), Valid: true}
						break
					}
				}
				if startLine.Valid {
					break
				}
			}
		}

		if namesOnly {
			fmt.Println(short)
			continue
		}

		var startPtr, endPtr *int
		if startLine.Valid {
			s := int(startLine.Int64)
			startPtr = &s
		}
		if endLine.Valid {
			e := int(endLine.Int64)
			endPtr = &e
		}
		lineInfo := output.FormatLineRange(startPtr, endPtr, "")
		fmt.Printf("%s %s %s\n", lineInfo, string(kind), short)
	}

	return nil
}
