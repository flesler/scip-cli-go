package exclude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/flesler/scip-cli-go/v2/internal/cache"
)

func withTestCache(t *testing.T, dir string) func() {
	cacheDir := filepath.Join(dir, "cache")
	t.Setenv("SCIP_CLI_CACHE", cacheDir)
	return func() {}
}

func TestPathMatchesGlob(t *testing.T) {
	if !PathMatchesGlob("src/foo.test.ts", "*.test.ts") {
		t.Fatal("expected basename match")
	}
	if PathMatchesGlob("src/foo.ts", "*.test.ts") {
		t.Fatal("unexpected basename match")
	}
	if !PathMatchesGlob("src/__tests__/fixtureOnly.spec.ts", "**/__tests__/**") {
		t.Fatal("expected recursive directory match")
	}
	if PathMatchesGlob("src/helper.ts", "**/__tests__/**") {
		t.Fatal("unexpected recursive directory match")
	}
	if !PathMatchesGlob("tests/unit/a.ts", "tests/**") {
		t.Fatal("expected full path match")
	}
	if PathMatchesGlob("src/tests/unit/a.ts", "tests/**") {
		t.Fatal("unexpected full path match")
	}
}

func TestPathMatchesAnyGlob(t *testing.T) {
	patterns := []string{"**/*.spec.ts", "**/*.test.ts"}
	if !PathMatchesAnyGlob("src/widget.spec.ts", patterns) {
		t.Fatal("expected spec match")
	}
	if PathMatchesAnyGlob("src/widget.ts", patterns) {
		t.Fatal("unexpected match")
	}
}

func TestResolveExcludeGlobsMergesConfigAndPersisted(t *testing.T) {
	dir := t.TempDir()
	withTestCache(t, dir)
	if err := os.WriteFile(filepath.Join(dir, ".scip-cli.json"), []byte(`{"excludeGlobs":["**/*.test.ts"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SavePersistedExcludeGlobs(dir, []string{"**/__tests__/**"}); err != nil {
		t.Fatal(err)
	}
	globs, err := ResolveExcludeGlobs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(globs) != 2 || globs[0] != "**/*.test.ts" || globs[1] != "**/__tests__/**" {
		t.Fatalf("globs=%v", globs)
	}
}

func TestSavePersistedExcludeGlobsClears(t *testing.T) {
	dir := t.TempDir()
	withTestCache(t, dir)
	if err := SavePersistedExcludeGlobs(dir, []string{"tests/**"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cache.GetCacheDir(dir), ExcludeFilename)
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if err := SavePersistedExcludeGlobs(dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file removed: %v", err)
	}
}

func TestLoadPersistedExcludeGlobsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	withTestCache(t, dir)
	path := filepath.Join(cache.GetCacheDir(dir), ExcludeFilename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not-json"), 0644); err != nil {
		t.Fatal(err)
	}
	if globs := LoadPersistedExcludeGlobs(dir); globs != nil {
		t.Fatalf("expected nil, got %v", globs)
	}
}

func TestSavePersistedExcludeGlobsWritesJSON(t *testing.T) {
	dir := t.TempDir()
	withTestCache(t, dir)
	if err := SavePersistedExcludeGlobs(dir, []string{"tests/**"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(cache.GetCacheDir(dir), ExcludeFilename))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string][]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed["globs"]) != 1 || parsed["globs"][0] != "tests/**" {
		t.Fatalf("parsed=%v", parsed)
	}
}
