package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSourceLines(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "proj")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	content := "line0\nline1\nline2\n"
	if err := os.WriteFile(filepath.Join(root, "test.ts"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	lines, err := ReadSourceLines(root, "test.ts", nil, nil)
	if err != nil || len(lines) != 3 {
		t.Fatalf("got %v err=%v", lines, err)
	}
	start, end := 1, 2
	slice, err := ReadSourceLines(root, "test.ts", &start, &end)
	if err != nil || len(slice) != 2 {
		t.Fatalf("slice got %v err=%v", slice, err)
	}
}

func TestReadSourceLinesRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	lines, _ := ReadSourceLines(dir, "../outside.ts", nil, nil)
	if lines != nil {
		t.Fatal("expected nil for path traversal")
	}
}

func TestReadSourceLines_nonexistentFile(t *testing.T) {
	dir := t.TempDir()
	lines, err := ReadSourceLines(dir, "missing.ts", nil, nil)
	if err != nil || lines != nil {
		t.Fatalf("got %v err=%v", lines, err)
	}
}
