package scope_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/flesler/scip-cli-go/v2/internal/cache"
	"github.com/flesler/scip-cli-go/v2/internal/scope"
)

func TestSaveAndLoadScope(t *testing.T) {
	root := t.TempDir()
	if scope.LoadIndexScope(root) != nil {
		t.Fatal("expected nil")
	}
	if err := scope.SaveIndexScope(root, []string{"packages/server", "packages/api"}); err != nil {
		t.Fatal(err)
	}
	loaded := scope.LoadIndexScope(root)
	if loaded == nil || len(loaded.Paths) != 2 {
		t.Fatalf("loaded=%v", loaded)
	}
	if err := scope.SaveIndexScope(root, nil); err != nil {
		t.Fatal(err)
	}
	if scope.LoadIndexScope(root) != nil {
		t.Fatal("cleared scope should be nil")
	}
}

func TestProjectInScope(t *testing.T) {
	if !scope.ProjectInScope("packages/server", []string{"packages/server"}) {
		t.Fatal("exact")
	}
	if !scope.ProjectInScope("packages/server/domains/foo", []string{"packages/server"}) {
		t.Fatal("nested")
	}
	if scope.ProjectInScope("packages/dashboard", []string{"packages/server"}) {
		t.Fatal("outside")
	}
}

func TestScopeUsesSameCacheDir(t *testing.T) {
	root := t.TempDir()
	before := cache.GetCacheDir(root)
	if err := scope.SaveIndexScope(root, []string{"packages/api"}); err != nil {
		t.Fatal(err)
	}
	if cache.GetCacheDir(root) != before {
		t.Fatal("cache dir changed")
	}
}

func TestScopeSurvivesCacheClear(t *testing.T) {
	root := t.TempDir()
	if err := scope.SaveIndexScope(root, []string{"packages/api"}); err != nil {
		t.Fatal(err)
	}
	cacheDir := cache.GetCacheDir(root)
	_ = os.MkdirAll(cacheDir, 0755)
	_ = os.WriteFile(filepath.Join(cacheDir, "index.db"), []byte("old"), 0644)
	_ = os.RemoveAll(cacheDir)
	if err := scope.SaveIndexScope(root, []string{"packages/api"}); err != nil {
		t.Fatal(err)
	}
	loaded := scope.LoadIndexScope(root)
	if loaded == nil || loaded.Paths[0] != "packages/api" {
		t.Fatalf("loaded=%v", loaded)
	}
}
