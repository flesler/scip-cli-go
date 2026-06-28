package cross_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var (
	pythonBinary string
	goBinary     string
	fixtureDir   string
)

func TestMain(m *testing.M) {
	pythonBinary = "scip-cli"

	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	goBinary = filepath.Join(repoRoot, "bin", "scip-cli-go")

	if out, err := exec.Command("go", "build", "-o", goBinary, filepath.Join(repoRoot, "cmd", "scip-cli")).CombinedOutput(); err != nil {
		os.Stderr.WriteString("build scip-cli-go failed: " + string(out) + "\n")
		os.Exit(1)
	}

	tmp, err := os.MkdirTemp("", "scip-cross-*")
	if err != nil {
		os.Exit(1)
	}
	fixtureDir = tmp
	src := filepath.Join(repoRoot, "testdata", "fixtures", "sample-project")
	if err := copyDir(src, fixtureDir); err != nil {
		os.Stderr.WriteString("copy fixture failed: " + err.Error() + "\n")
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func runPython(args ...string) (string, string, error) {
	cmd := exec.Command(pythonBinary, args...)
	cmd.Dir = fixtureDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func runGo(args ...string) (string, string, error) {
	cmd := exec.Command(goBinary, args...)
	cmd.Dir = fixtureDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func normalizeOutput(s string) string {
	lines := strings.Split(s, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "scip-cli") && strings.Contains(line, "version") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.Join(cleaned, "\n")
}

func compareOutputs(t *testing.T, label string, pyOut, goOut string) {
	t.Helper()
	pyNorm := normalizeOutput(pyOut)
	goNorm := normalizeOutput(goOut)
	if pyNorm != goNorm {
		t.Errorf("%s output mismatch:\nPython:\n%s\n\nGo:\n%s", label, pyNorm, goNorm)
	}
}

func TestCrossCompare_version(t *testing.T) {
	pyOut, _, pyErr := runPython("--version")
	goOut, _, goErr := runGo("--version")
	if pyErr != nil {
		t.Fatalf("Python failed: %v", pyErr)
	}
	if goErr != nil {
		t.Fatalf("Go failed: %v", goErr)
	}
	if !strings.Contains(pyOut, "scip-cli") || !strings.Contains(goOut, "scip-cli-go") {
		t.Fatalf("version mismatch: py=%q go=%q", pyOut, goOut)
	}
}

func TestCrossCompare_search(t *testing.T) {
	pyOut, _, pyErr := runPython("search", "greet", "--limit", "3")
	goOut, _, goErr := runGo("search", "greet", "--limit", "3")
	if pyErr != nil {
		t.Fatalf("Python failed: %v", pyErr)
	}
	if goErr != nil {
		t.Fatalf("Go failed: %v", goErr)
	}
	compareOutputs(t, "search greet", pyOut, goOut)
}

func TestCrossCompare_code(t *testing.T) {
	pyOut, _, pyErr := runPython("code", "greet", "--limit", "1")
	goOut, _, goErr := runGo("code", "greet", "--limit", "1")
	if pyErr != nil {
		t.Fatalf("Python failed: %v", pyErr)
	}
	if goErr != nil {
		t.Fatalf("Go failed: %v", goErr)
	}
	compareOutputs(t, "code greet", pyOut, goOut)
}

func TestCrossCompare_refs(t *testing.T) {
	pyOut, _, pyErr := runPython("refs", "greet", "--limit", "10")
	goOut, _, goErr := runGo("refs", "greet", "--limit", "10")
	if pyErr != nil {
		t.Fatalf("Python failed: %v", pyErr)
	}
	if goErr != nil {
		t.Fatalf("Go failed: %v", goErr)
	}
	compareOutputs(t, "refs greet", pyOut, goOut)
}

func TestCrossCompare_symbols(t *testing.T) {
	pyOut, _, pyErr := runPython("symbols", "src/helper.ts", "--limit", "10")
	goOut, _, goErr := runGo("symbols", "src/helper.ts", "--limit", "10")
	if pyErr != nil {
		t.Fatalf("Python failed: %v", pyErr)
	}
	if goErr != nil {
		t.Fatalf("Go failed: %v", goErr)
	}
	compareOutputs(t, "symbols helper.ts", pyOut, goOut)
}
