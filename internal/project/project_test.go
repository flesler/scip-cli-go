package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sourcegraph/scip-cli-go/internal/project"
)

func TestDetectLanguage_packageJSON(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)
	lang, ok := project.DetectLanguage(dir)
	if !ok || lang != project.LanguageTypeScript {
		t.Fatalf("got %q ok=%v", lang, ok)
	}
}

func TestDetectLanguage_tsconfigOnly(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte("{}"), 0644)
	lang, ok := project.DetectLanguage(dir)
	if !ok || lang != project.LanguageTypeScript {
		t.Fatalf("got %q ok=%v", lang, ok)
	}
}

func TestDetectLanguage_python(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]"), 0644)
	lang, ok := project.DetectLanguage(dir)
	if !ok || lang != project.LanguagePython {
		t.Fatalf("got %q ok=%v", lang, ok)
	}
}

func TestDetectLanguage_goMod(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/foo\n\ngo 1.25\n"), 0644)
	lang, ok := project.DetectLanguage(dir)
	if !ok || lang != project.LanguageGolang {
		t.Fatalf("got %q ok=%v", lang, ok)
	}
}

func TestDetectLanguage_packageOverGo(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/foo\n\ngo 1.25\n"), 0644)
	lang, ok := project.DetectLanguage(dir)
	if !ok || lang != project.LanguageTypeScript {
		t.Fatalf("got %q ok=%v", lang, ok)
	}
}

func TestDetectLanguage_packageOverPython(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]"), 0644)
	lang, ok := project.DetectLanguage(dir)
	if !ok || lang != project.LanguageTypeScript {
		t.Fatalf("got %q ok=%v", lang, ok)
	}
}
