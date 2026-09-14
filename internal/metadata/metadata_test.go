package metadata_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/flesler/scip-cli-go/v2/internal/exclude"
	"github.com/flesler/scip-cli-go/v2/internal/metadata"
	"github.com/flesler/scip-cli-go/v2/internal/scope"
)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := metadata.SaveMetadata(root, metadata.IndexMetadata{
		ScopePaths:   []string{"packages/api"},
		ExcludeGlobs: []string{"**/*.test.ts"},
	}); err != nil {
		t.Fatal(err)
	}
	loaded := metadata.LoadMetadata(root)
	if len(loaded.ScopePaths) != 1 || loaded.ScopePaths[0] != "packages/api" {
		t.Fatalf("scope=%v", loaded.ScopePaths)
	}
	if len(loaded.ExcludeGlobs) != 1 || loaded.ExcludeGlobs[0] != "**/*.test.ts" {
		t.Fatalf("exclude=%v", loaded.ExcludeGlobs)
	}
	data, err := os.ReadFile(metadata.MetadataPath(root))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]map[string][]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["scope"]["paths"][0] != "packages/api" || parsed["exclude"]["globs"][0] != "**/*.test.ts" {
		t.Fatalf("parsed=%v", parsed)
	}
}

func TestEmptyMetadataDeletesFile(t *testing.T) {
	root := t.TempDir()
	if err := scope.SaveIndexScope(root, []string{"packages/api"}); err != nil {
		t.Fatal(err)
	}
	if err := metadata.SaveMetadata(root, metadata.IndexMetadata{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(metadata.MetadataPath(root)); !os.IsNotExist(err) {
		t.Fatalf("expected metadata file removed: %v", err)
	}
}

func TestApplyMetadataUpdatesPreservesUnchangedSlices(t *testing.T) {
	root := t.TempDir()
	if err := scope.SaveIndexScope(root, []string{"packages/api"}); err != nil {
		t.Fatal(err)
	}
	if err := exclude.SavePersistedExcludeGlobs(root, []string{"tests/**"}); err != nil {
		t.Fatal(err)
	}

	_, err := metadata.ApplyMetadataUpdates(root, false, nil, &metadata.OptionalSlice{
		Set:   true,
		Value: []string{"**/*.spec.ts"},
	})
	if err != nil {
		t.Fatal(err)
	}

	loadedScope := scope.LoadIndexScope(root)
	if loadedScope == nil || len(loadedScope.Paths) != 1 || loadedScope.Paths[0] != "packages/api" {
		t.Fatalf("scope=%v", loadedScope)
	}
	globs := exclude.LoadPersistedExcludeGlobs(root)
	if len(globs) != 1 || globs[0] != "**/*.spec.ts" {
		t.Fatalf("exclude=%v", globs)
	}
}

func TestFreshClearsMetadata(t *testing.T) {
	root := t.TempDir()
	if err := scope.SaveIndexScope(root, []string{"packages/api"}); err != nil {
		t.Fatal(err)
	}
	if err := exclude.SavePersistedExcludeGlobs(root, []string{"tests/**"}); err != nil {
		t.Fatal(err)
	}

	_, err := metadata.ApplyMetadataUpdates(root, true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if scope.LoadIndexScope(root) != nil {
		t.Fatal("expected scope cleared")
	}
	if exclude.LoadPersistedExcludeGlobs(root) != nil {
		t.Fatal("expected exclude cleared")
	}
	if _, err := os.Stat(metadata.MetadataPath(root)); !os.IsNotExist(err) {
		t.Fatalf("expected metadata file removed: %v", err)
	}
}

func TestFreshWithExcludeOnly(t *testing.T) {
	root := t.TempDir()
	if err := scope.SaveIndexScope(root, []string{"packages/api"}); err != nil {
		t.Fatal(err)
	}

	_, err := metadata.ApplyMetadataUpdates(root, true, nil, &metadata.OptionalSlice{
		Set:   true,
		Value: []string{"tests/**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if scope.LoadIndexScope(root) != nil {
		t.Fatal("expected scope cleared")
	}
	globs := exclude.LoadPersistedExcludeGlobs(root)
	if len(globs) != 1 || globs[0] != "tests/**" {
		t.Fatalf("exclude=%v", globs)
	}
}

func TestBareExcludeClearsSlice(t *testing.T) {
	root := t.TempDir()
	if err := scope.SaveIndexScope(root, []string{"packages/api"}); err != nil {
		t.Fatal(err)
	}
	if err := exclude.SavePersistedExcludeGlobs(root, []string{"tests/**"}); err != nil {
		t.Fatal(err)
	}

	_, err := metadata.ApplyMetadataUpdates(root, false, nil, &metadata.OptionalSlice{
		Set:   true,
		Value: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	loadedScope := scope.LoadIndexScope(root)
	if loadedScope == nil || loadedScope.Paths[0] != "packages/api" {
		t.Fatalf("scope=%v", loadedScope)
	}
	if exclude.LoadPersistedExcludeGlobs(root) != nil {
		t.Fatal("expected exclude cleared")
	}
}
