package commands

import (
	"fmt"
	"os"

	"github.com/flesler/scip-cli-go/v2/internal/clierr"
	"github.com/flesler/scip-cli-go/v2/internal/indexing"
	"github.com/flesler/scip-cli-go/v2/internal/paths"
	"github.com/flesler/scip-cli-go/v2/internal/project"
	"github.com/flesler/scip-cli-go/v2/internal/scope"
)

func ReindexMain(args map[string]interface{}) error {
	root, lang, ok := project.FindProjectRootAndLanguage("")
	if !ok {
		fmt.Fprintln(os.Stderr, "Error: Could not find project root")
		return clierr.Exit(1)
	}

	pathArgs := args["path"].([]string)
	if len(pathArgs) > 0 && lang != project.LanguageTypeScript {
		fmt.Fprintln(os.Stderr, "Error: reindex --path is only supported for TypeScript projects")
		return clierr.Exit(1)
	}

	if len(pathArgs) > 0 {
		var scopePaths []string
		for _, path := range pathArgs {
			normalized, err := paths.NormalizePathScope(path, root)
			if err != nil {
				return err
			}
			scopePaths = append(scopePaths, normalized)
		}
		if err := scope.SaveIndexScope(root, scopePaths); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Index scope: %v\n", scopePaths)
		fmt.Fprintln(os.Stderr, "Warning: scoped reindex replaces the cache with only these projects; run reindex with no --path to restore the full index")
	} else {
		if err := scope.SaveIndexScope(root, nil); err != nil {
			return err
		}
	}

	if err := indexing.Reindex(root, true); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Index complete\n")
	return nil
}
