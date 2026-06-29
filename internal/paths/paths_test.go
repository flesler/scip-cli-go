package paths_test

import (
	"testing"

	"github.com/flesler/scip-cli-go/internal/paths"
)

func TestPathInScope_exact(t *testing.T) {
	if !paths.PathInScope("src/foo.ts", "src/foo.ts") {
		t.Fatal("exact match should be in scope")
	}
}

func TestPathInScope_prefix(t *testing.T) {
	if !paths.PathInScope("src/foo/bar.ts", "src/foo") {
		t.Fatal("child path should be in scope")
	}
	if paths.PathInScope("src/other.ts", "src/foo") {
		t.Fatal("sibling should not be in scope")
	}
}

func TestPathInScope_empty(t *testing.T) {
	if !paths.PathInScope("anything.ts", "") {
		t.Fatal("empty scope matches all")
	}
}

func TestNormalizePathScope_relative(t *testing.T) {
	root := t.TempDir()
	scope, err := paths.NormalizePathScope("src", root)
	if err != nil {
		t.Fatal(err)
	}
	if scope != "src" {
		t.Fatalf("got %q want src", scope)
	}
}
