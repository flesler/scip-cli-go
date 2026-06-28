package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectConfigDefaults(t *testing.T) {
	settings, err := LoadProjectConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if settings.MaxHeapMb != nil || len(settings.IndexRoots) != 0 || settings.OnlyIndexRoots {
		t.Fatalf("unexpected defaults: %+v", settings)
	}
}

func TestLoadProjectConfigRejectsBadHeap(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFilename), []byte(`{"maxHeapMb": 0}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProjectConfig(dir); err == nil {
		t.Fatal("expected error for zero heap")
	}
	if err := os.WriteFile(filepath.Join(dir, ConfigFilename), []byte(`{"maxHeapMb": true}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProjectConfig(dir); err == nil {
		t.Fatal("expected error for boolean heap")
	}
}

func TestLoadProjectConfigReadsValues(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFilename), []byte(`{
		"maxHeapMb": 12288,
		"indexRoots": ["packages/api"],
		"onlyIndexRoots": true
	}`), 0644); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if settings.MaxHeapMb == nil || *settings.MaxHeapMb != 12288 {
		t.Fatalf("heap: %+v", settings.MaxHeapMb)
	}
	if len(settings.IndexRoots) != 1 || settings.IndexRoots[0] != "packages/api" {
		t.Fatalf("roots: %v", settings.IndexRoots)
	}
	if !settings.OnlyIndexRoots {
		t.Fatal("expected onlyIndexRoots")
	}
}

func TestResolveIndexRoots(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "packages", "api"), 0755)
	settings := &ProjectSettings{IndexRoots: []string{"packages/api"}}
	roots, err := ResolveIndexRoots(dir, settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 {
		t.Fatalf("roots: %v", roots)
	}
	settings.IndexRoots = []string{"missing"}
	if _, err := ResolveIndexRoots(dir, settings); err == nil {
		t.Fatal("expected missing path error")
	}
}
