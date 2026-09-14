package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/flesler/scip-cli-go/v2/internal/exclude"
	"github.com/flesler/scip-cli-go/v2/internal/indexing"
	"github.com/flesler/scip-cli-go/v2/internal/metadata"
	"github.com/flesler/scip-cli-go/v2/internal/scope"
)

func TestReindexPreservesPersistedScope(t *testing.T) {
	root := withReindexRoot(t)
	if err := scope.SaveIndexScope(root, []string{"packages/api"}); err != nil {
		t.Fatal(err)
	}

	if err := ReindexMain(reindexArgs()); err != nil {
		t.Fatal(err)
	}

	loaded := scope.LoadIndexScope(root)
	if loaded == nil || len(loaded.Paths) != 1 || loaded.Paths[0] != "packages/api" {
		t.Fatalf("scope=%v", loaded)
	}
}

func TestFreshReindexClearsPersistedMetadata(t *testing.T) {
	root := withReindexRoot(t)
	if err := scope.SaveIndexScope(root, []string{"packages/api"}); err != nil {
		t.Fatal(err)
	}
	if err := exclude.SavePersistedExcludeGlobs(root, []string{"tests/**"}); err != nil {
		t.Fatal(err)
	}

	args := reindexArgs()
	args["fresh"] = true
	if err := ReindexMain(args); err != nil {
		t.Fatal(err)
	}

	if scope.LoadIndexScope(root) != nil {
		t.Fatal("expected scope cleared")
	}
	if exclude.LoadPersistedExcludeGlobs(root) != nil {
		t.Fatal("expected exclude cleared")
	}
}

func TestReindexExcludeClearsPersistedExclude(t *testing.T) {
	root := withReindexRoot(t)
	if err := exclude.SavePersistedExcludeGlobs(root, []string{"tests/**"}); err != nil {
		t.Fatal(err)
	}

	args := reindexArgs()
	args["exclude_set"] = true
	if err := ReindexMain(args); err != nil {
		t.Fatal(err)
	}

	if exclude.LoadPersistedExcludeGlobs(root) != nil {
		t.Fatal("expected exclude cleared")
	}
}

func TestReindexExcludeReplacesPersistedExclude(t *testing.T) {
	root := withReindexRoot(t)
	if err := exclude.SavePersistedExcludeGlobs(root, []string{"tests/**"}); err != nil {
		t.Fatal(err)
	}

	args := reindexArgs()
	args["exclude_set"] = true
	args["exclude"] = []string{"**/*.spec.ts"}
	if err := ReindexMain(args); err != nil {
		t.Fatal(err)
	}

	globs := exclude.LoadPersistedExcludeGlobs(root)
	if len(globs) != 1 || globs[0] != "**/*.spec.ts" {
		t.Fatalf("exclude=%v", globs)
	}
}

func withReindexRoot(t *testing.T) string {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SCIP_CLI_CACHE", filepath.Join(root, ".cache"))
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	origReindex := reindexProject
	reindexProject = func(root string, opts *indexing.ReindexOptions) error {
		if opts == nil {
			return nil
		}
		_, err := metadata.ApplyMetadataUpdates(root, opts.Fresh, opts.ScopeUpdate, opts.ExcludeUpdate)
		return err
	}
	t.Cleanup(func() {
		reindexProject = origReindex
	})
	return root
}

func reindexArgs() map[string]interface{} {
	return map[string]interface{}{
		"path":          []string{},
		"exclude":       []string{},
		"exclude_set":   false,
		"fresh":         false,
		"with_external": false,
	}
}
