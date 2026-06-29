package commands

import (
	"fmt"
	"os"

	"github.com/sourcegraph/scip-cli-go/internal/clierr"
	"github.com/sourcegraph/scip-cli-go/internal/output"
	"github.com/sourcegraph/scip-cli-go/internal/paths"
	"github.com/sourcegraph/scip-cli-go/internal/queries"
	"github.com/sourcegraph/scip-cli-go/internal/session"
)

func RdepsMain(args map[string]interface{}) error {
	db, _, err := session.Setup()
	if err != nil {
		return err
	}
	defer db.Close()

	pathScope := args["path_scope"].(string)
	limit := args["limit"].(int)
	filePattern := args["file"].(string)

	filePath, err := session.ResolveOneFile(db, filePattern, pathScope)
	if err != nil {
		return err
	}

	syms, err := queries.GetFileSymbols(db, filePath, nil)
	if err != nil {
		return err
	}

	if len(syms) == 0 {
		fmt.Fprintf(os.Stderr, "No symbols found in '%s'\n", filePath)
		return clierr.Exit(1)
	}

	symbolIDs := make([]int, len(syms))
	for i, sym := range syms {
		symbolIDs[i] = sym.ID
	}

	limitPlusOne := limit + 1
	importers, err := queries.GetImporterPaths(db, symbolIDs, filePath, &limitPlusOne)
	if err != nil {
		return err
	}

	var rdeps []string
	for _, path := range importers {
		if paths.PathInScope(path, pathScope) {
			rdeps = append(rdeps, path)
		}
	}

	if len(rdeps) == 0 {
		fmt.Fprintf(os.Stderr, "No reverse dependencies found for '%s'\n", filePath)
		return clierr.Exit(1)
	}

	rdeps = output.LimitAndWarn(rdeps, limit, "reverse dependencies")

	for _, depPath := range rdeps {
		fmt.Println(depPath)
	}

	return nil
}
