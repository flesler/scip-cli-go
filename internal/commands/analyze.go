package commands

import (
	"fmt"
	"os"

	"github.com/flesler/scip-cli-go/v2/internal/analyze"
	"github.com/flesler/scip-cli-go/v2/internal/clierr"
	"github.com/flesler/scip-cli-go/v2/internal/session"
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

	resetLive, err := analyze.BindLive(db)
	if err != nil {
		return err
	}
	defer resetLive()

	pathScope := args["path_scope"].(string)
	limit := args["limit"].(int)
	perCheckLimit := 0
	if v, ok := args["per_check_limit"]; ok && v != nil {
		perCheckLimit = v.(int)
	}
	includeTests := args["include_tests"].(bool)
	priorityStr := args["priority"].(string)
	checkArgs := args["check"].([]string)
	targetName := args["target"].(string)

	priorities, err := analyze.ParsePriorities(priorityStr)
	if err != nil {
		return err
	}
	selectedChecks, err := analyze.ParseChecks(checkArgs)
	if err != nil {
		return err
	}
	budget := analyze.NewRowBudget(limit)

	var secs []analyze.SectionResult

	if targetName == "" {
		if pathScope != "" {
			fmt.Fprintf(os.Stderr, "Error: use analyze %q for directory scope (not analyze --path)\n", pathScope)
			return clierr.Exit(1)
		}
		secs, err = analyze.RunProjectSections(db, limit, includeTests, "", priorities, budget, selectedChecks, perCheckLimit)
		if err != nil {
			return err
		}
	} else {
		resolved, err := analyze.ResolveAnalyzeTarget(db, targetName, projectRoot, pathScope)
		if err != nil {
			return err
		}

		switch resolved.Kind {
		case "dir":
			secs, err = analyze.RunDirSections(db, resolved.Scope, limit, includeTests, priorities, budget, selectedChecks, perCheckLimit)
			if err != nil {
				return err
			}
		case "file":
			fileInclude := projectIncludeTests(includeTests, resolved.Scope)
			secs, err = analyze.RunProjectSections(db, limit, fileInclude, resolved.Scope, priorities, budget, selectedChecks, perCheckLimit)
			if err != nil {
				return err
			}
			if !budget.Exhausted() {
				fileSecs, err := analyze.RunFileSections(db, resolved.Scope, limit, priorities, budget, selectedChecks, perCheckLimit)
				if err != nil {
					return err
				}
				secs = append(secs, fileSecs...)
			}
		default:
			sym, err := session.ResolveOneSymbol(db, resolved.SymbolName, nil, pathScope)
			if err != nil {
				return err
			}
			secs, err = analyze.RunSymbolSections(db, sym.ID, limit, priorities, budget, selectedChecks, perCheckLimit)
			if err != nil {
				return err
			}
		}
	}

	printSections(secs)
	return nil
}
