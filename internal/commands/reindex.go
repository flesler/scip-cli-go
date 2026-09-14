package commands

import (
	"fmt"
	"os"

	"github.com/flesler/scip-cli-go/v2/internal/clierr"
	"github.com/flesler/scip-cli-go/v2/internal/exclude"
	"github.com/flesler/scip-cli-go/v2/internal/indexing"
	"github.com/flesler/scip-cli-go/v2/internal/metadata"
	"github.com/flesler/scip-cli-go/v2/internal/paths"
	"github.com/flesler/scip-cli-go/v2/internal/project"
)

var reindexProject = indexing.Reindex

func ReindexMain(args map[string]interface{}) error {
	root, lang, ok := project.FindProjectRootAndLanguage("")
	if !ok {
		fmt.Fprintln(os.Stderr, "Error: Could not find project root")
		return clierr.Exit(1)
	}

	pathArgs := args["path"].([]string)
	excludeSet := false
	if v, ok := args["exclude_set"].(bool); ok {
		excludeSet = v
	}
	excludeArgs := args["exclude"].([]string)
	fresh := false
	if v, ok := args["fresh"].(bool); ok {
		fresh = v
	}
	withExternal := false
	if v, ok := args["with_external"].(bool); ok {
		withExternal = v
	}

	if len(pathArgs) > 0 && lang != project.LanguageTypeScript {
		fmt.Fprintln(os.Stderr, "Error: reindex --path is only supported for TypeScript projects")
		return clierr.Exit(1)
	}

	var scopeUpdate *metadata.OptionalSlice
	if len(pathArgs) > 0 {
		var scopePaths []string
		for _, path := range pathArgs {
			normalized, err := paths.NormalizePathScope(path, root)
			if err != nil {
				return err
			}
			scopePaths = append(scopePaths, normalized)
		}
		scopeUpdate = &metadata.OptionalSlice{Set: true, Value: scopePaths}
		fmt.Fprintf(os.Stderr, "Index scope: %v\n", scopePaths)
		fmt.Fprintln(os.Stderr, "Warning: scoped reindex replaces the cache with only these projects; run reindex --fresh to restore the full index")
	}

	var excludeUpdate *metadata.OptionalSlice
	if excludeSet {
		for _, glob := range excludeArgs {
			if err := exclude.ValidateGlob(glob); err != nil {
				return err
			}
		}
		excludeUpdate = &metadata.OptionalSlice{Set: true, Value: excludeArgs}
		if len(excludeArgs) > 0 {
			fmt.Fprintf(os.Stderr, "Index exclude: %s\n", exclude.FormatExcludeGlobs(excludeArgs))
		}
	}

	if _, err := metadata.ApplyMetadataUpdates(root, fresh, scopeUpdate, excludeUpdate); err != nil {
		return err
	}

	if withExternal {
		os.Setenv("SCIP_CLI_KEEP_EXTERNAL", "1")
		defer os.Unsetenv("SCIP_CLI_KEEP_EXTERNAL")
	}

	if err := reindexProject(root, true); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Index complete\n")
	return nil
}
