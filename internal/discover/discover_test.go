package discover_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/flesler/scip-cli-go/v2/internal/discover"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscover_singlePackage(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), "{}")
	writeFile(t, filepath.Join(root, "tsconfig.json"), `{"include": ["src/**/*.ts"]}`)
	projects, err := discover.DiscoverProjects(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0] != "." {
		t.Fatalf("projects=%v", projects)
	}
}

func TestDiscover_npmWorkspaces(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"workspaces": ["packages/*"]}`)
	writeFile(t, filepath.Join(root, "tsconfig.json"), `{"include": ["*.ts"]}`)
	writeFile(t, filepath.Join(root, "packages", "api", "tsconfig.json"), `{"include": ["src/**/*.ts"]}`)
	writeFile(t, filepath.Join(root, "packages", "web", "tsconfig.json"), `{"include": ["src/**/*.ts"]}`)
	projects, err := discover.DiscoverProjects(root, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"packages/api", "packages/web"}
	if len(projects) != 2 || projects[0] != want[0] || projects[1] != want[1] {
		t.Fatalf("projects=%v", projects)
	}
}

func TestDiscover_nestedPrefersChild(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), "{}")
	writeFile(t, filepath.Join(root, "packages", "nested", "tsconfig.json"), `{"include": ["**/*.ts"]}`)
	writeFile(t, filepath.Join(root, "packages", "nested", "child", "tsconfig.json"), `{"include": ["src/**/*.ts"]}`)
	projects, err := discover.DiscoverProjects(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0] != "packages/nested/child" {
		t.Fatalf("projects=%v", projects)
	}
}

func TestDiscover_skipsNodeModules(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), "{}")
	writeFile(t, filepath.Join(root, "tsconfig.json"), `{"include": ["src/**/*.ts"]}`)
	writeFile(t, filepath.Join(root, "node_modules", "pkg", "tsconfig.json"), `{"include": ["src/**/*.ts"]}`)
	projects, err := discover.DiscoverProjects(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0] != "." {
		t.Fatalf("projects=%v", projects)
	}
}

func TestDiscover_broadRootIncludesDot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), mustJSON(t, map[string]interface{}{"workspaces": []string{"packages/*"}}))
	writeFile(t, filepath.Join(root, "tsconfig.json"), `{"include": ["packages/**/*.ts"]}`)
	writeFile(t, filepath.Join(root, "packages", "api", "tsconfig.json"), `{"include": ["src/**/*.ts"]}`)
	projects, err := discover.DiscoverProjects(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 || projects[0] != "." {
		t.Fatalf("projects=%v", projects)
	}
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDiscover_nestedServiceProject(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), "{}")
	writeFile(t, filepath.Join(root, "services", "api", "tsconfig.json"), `{"include": ["src/**/*.ts"]}`)
	projects, err := discover.DiscoverProjects(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0] != "services/api" {
		t.Fatalf("projects=%v", projects)
	}
}

func TestDiscover_tsconfigVariantWithInclude(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), "{}")
	writeFile(t, filepath.Join(root, "tsconfig.build.json"), `{"include": ["src/**/*.ts"]}`)
	projects, err := discover.DiscoverProjects(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0] != "." {
		t.Fatalf("projects=%v", projects)
	}
}

func TestDiscoverGolangModules_nested(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/root\n\ngo 1.22\n")
	writeFile(t, filepath.Join(root, "services", "api", "go.mod"), "module example.com/api\n\ngo 1.22\n")
	modules, err := discover.DiscoverGolangModules(root)
	if err != nil || len(modules) != 2 {
		t.Fatalf("modules=%v err=%v", modules, err)
	}
}

func TestDiscoverRustCrates_nested(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Cargo.toml"), "[workspace]\nmembers = [\"crates/a\"]\n")
	writeFile(t, filepath.Join(root, "crates", "a", "Cargo.toml"), "[package]\nname=\"a\"\nversion=\"0.1.0\"\n")
	crates, err := discover.DiscoverRustCrates(root)
	if err != nil || len(crates) != 2 || crates[1] != "crates/a" {
		t.Fatalf("crates=%v err=%v", crates, err)
	}
}
