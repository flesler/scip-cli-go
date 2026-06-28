package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectCacheSlugReadable(t *testing.T) {
	dir := t.TempDir()
	cache := GetCacheDir(dir)
	if filepath.Base(filepath.Dir(cache)) != "projects" {
		t.Fatalf("cache path: %s", cache)
	}
	slug := filepath.Base(cache)
	if slug != ProjectCacheSlug(dir) {
		t.Fatalf("slug mismatch: %s vs %s", slug, ProjectCacheSlug(dir))
	}
}

func TestMonorepoSlugUsesBasename(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "Code", "my-monorepo")
	_ = os.MkdirAll(repo, 0755)
	slug := ProjectCacheSlug(repo)
	if len(slug) < len("my-monorepo-") || slug[:len("my-monorepo-")] != "my-monorepo-" {
		t.Fatalf("slug: %s", slug)
	}
}

func TestFindDBReturnsExisting(t *testing.T) {
	dir := t.TempDir()
	cacheDir := GetCacheDir(dir)
	_ = os.MkdirAll(cacheDir, 0755)
	dbPath := filepath.Join(cacheDir, IndexDB)
	_ = os.WriteFile(dbPath, []byte("x"), 0644)
	if FindDB(dir) != dbPath {
		t.Fatalf("FindDB=%q want %q", FindDB(dir), dbPath)
	}
}

func TestPromoteNextIndex(t *testing.T) {
	dir := t.TempDir()
	cacheDir := GetCacheDir(dir)
	_ = os.MkdirAll(cacheDir, 0755)
	live := IndexDBPath(cacheDir, false)
	next := IndexDBPath(cacheDir, true)
	_ = os.WriteFile(live, []byte("live"), 0644)
	_ = os.WriteFile(next, []byte("next"), 0644)
	if err := PromoteNextIndex(cacheDir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(live)
	if string(data) != "next" {
		t.Fatalf("live=%q", data)
	}
	if _, err := os.Stat(next); !os.IsNotExist(err) {
		t.Fatal("next should be gone")
	}
}

func TestCacheDirStableAcrossScopeAndConfig(t *testing.T) {
	dir := t.TempDir()
	defaultCache := GetCacheDir(dir)
	if err := os.WriteFile(filepath.Join(dir, ".scip-cli.json"),
		[]byte(`{"indexRoots": ["packages/api"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if GetCacheDir(dir) != defaultCache {
		t.Fatal("indexRoots should not change cache dir")
	}
}
