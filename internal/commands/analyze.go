package commands

import (
	"fmt"
	"os"

	"github.com/sourcegraph/scip-cli-go/internal/analyze"
	"github.com/sourcegraph/scip-cli-go/internal/session"
)

func projectIncludeTests(includeTests bool, scope string) bool {
	if scope != "" && analyze.IsTestPath(scope) {
		return true
	}
	return includeTests
}

func printSections(secs []analyze.SectionResult) {
	for _, sec := range secs {
		fmt.Printf("=== %s ===\n", sec.Title)
		if sec.Preface != "" && len(sec.Lines) > 0 && sec.Lines[0] != "(none)" {
			fmt.Println(sec.Preface)
		}
		for _, line := range sec.Lines {
			fmt.Println(line)
		}
		fmt.Println()
	}
}

func AnalyzeMain(args map[string]interface{}) error {
	db, projectRoot, err := session.Setup()
	if err != nil {
		return err
	}
	defer db.Close()

	pathScope := args["path_scope"].(string)
	limit := args["limit"].(int)
	includeTests := args["include_tests"].(bool)
	priorityStr := args["priority"].(string)
	targetName := args["target"].(string)

	priorities := analyze.ParsePriorities(priorityStr)
	budget := analyze.NewRowBudget(limit)

	var secs []analyze.SectionResult

	if targetName == "" {
		if pathScope != "" {
			fmt.Fprintf(os.Stderr, "Error: use analyze %q for directory scope (not analyze --path)\n", pathScope)
			os.Exit(1)
		}
		secs, err = analyze.RunProjectSections(db, limit, includeTests, "", priorities, budget)
		if err != nil {
			return err
		}
	} else {
		resolved, err := analyze.ResolveAnalyzeTarget(db, targetName, projectRoot, pathScope)
		if err != nil {
			return err
		}

		if resolved.Kind == "dir" {
			secs, err = analyze.RunDirSections(db, resolved.Scope, limit, includeTests, priorities, budget)
			if err != nil {
				return err
			}
		} else if resolved.Kind == "file" {
			fileInclude := projectIncludeTests(includeTests, resolved.Scope)
			secs, err = analyze.RunProjectSections(db, limit, fileInclude, resolved.Scope, priorities, budget)
			if err != nil {
				return err
			}
			if !budget.Exhausted() {
				fileSecs, err := analyze.RunFileSections(db, resolved.Scope, limit, priorities, budget)
				if err != nil {
					return err
				}
				secs = append(secs, fileSecs...)
			}
		} else {
			sym, err := session.ResolveOneSymbol(db, resolved.SymbolName, nil, pathScope)
			if err != nil {
				return err
			}
			secs, err = analyze.RunSymbolSections(db, sym.ID, limit, priorities, budget)
			if err != nil {
				return err
			}
		}
	}

	printSections(secs)
	return nil
}
