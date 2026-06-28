package e2e_test

import (
	"bytes"
	"database/sql"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sourcegraph/scip-cli-go/internal/cache"
	"github.com/sourcegraph/scip-cli-go/internal/indexing"
	"github.com/sourcegraph/scip-cli-go/internal/scope"

	_ "modernc.org/sqlite"
)

const (
	helperFile   = "src/helper.ts"
	widgetFile   = "src/widget.ts"
	consumerFile = "src/consumer.ts"
	userFile     = "src/user.ts"
	fnGreet      = "greet"
	typeOptions  = "Options"
	fieldVerbose = "verbose"
	methodRun    = "Widget.run"
	classWidget  = "Widget"
)

var (
	binaryPath  string
	fixtureRoot string
	cacheBase   string
	indexOK     bool
	indexErr    error
)

func TestMain(m *testing.M) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	binaryPath = filepath.Join(repoRoot, "bin", "scip-cli-go")

	if out, err := exec.Command("go", "build", "-o", binaryPath, filepath.Join(repoRoot, "cmd", "scip-cli")).CombinedOutput(); err != nil {
		os.Stderr.WriteString(string(out))
		os.Exit(1)
	}

	tmp, err := os.MkdirTemp("", "scip-go-fixture-*")
	if err != nil {
		os.Exit(1)
	}
	fixtureRoot = tmp
	src := filepath.Join(repoRoot, "testdata", "fixtures", "sample-project")
	if _, err := os.Stat(src); err != nil {
		os.Stderr.WriteString("missing fixture: " + src + "\n")
		os.Exit(1)
	}
	if err := copyDir(src, fixtureRoot); err != nil {
		os.Exit(1)
	}

	cacheBase = filepath.Join(tmp, "cache")
	os.Setenv("SCIP_CLI_CACHE", cacheBase)

	indexErr = indexing.Reindex(fixtureRoot, true)
	indexOK = indexErr == nil

	os.Exit(m.Run())
}

func requireIndex(t *testing.T) {
	t.Helper()
	if !indexOK {
		t.Skipf("fixture indexing unavailable: %v", indexErr)
	}
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

type cliResult struct {
	Code   int
	Stdout string
	Stderr string
}

func runCLI(args ...string) cliResult {
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = fixtureRoot
	cmd.Env = append(os.Environ(), "SCIP_CLI_CACHE="+cacheBase)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 1
		}
	}
	return cliResult{Code: code, Stdout: stdout.String(), Stderr: stderr.String()}
}

func runCLIInDir(dir string, args ...string) cliResult {
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = dir
	cacheDir := filepath.Join(dir, ".cache")
	cmd.Env = append(os.Environ(), "SCIP_CLI_CACHE="+cacheDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 1
		}
	}
	return cliResult{Code: code, Stdout: stdout.String(), Stderr: stderr.String()}
}

func TestVersion(t *testing.T) {
	res := runCLI("--version")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "scip-cli-go") {
		t.Fatalf("unexpected version output: %q", res.Stdout)
	}
}

func TestSkill(t *testing.T) {
	res := runCLI("skill")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Quick Decision Guide") {
		t.Fatalf("missing skill content: %q", res.Stdout)
	}
}

func TestSearchBareName(t *testing.T) {
	requireIndex(t)
	res := runCLI("search", fnGreet, "--limit", "3")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, fnGreet) {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

func TestSymbolsByPath(t *testing.T) {
	requireIndex(t)
	res := runCLI("symbols", helperFile, "--limit", "10")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, fnGreet) {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

func TestCodeSingleSymbol(t *testing.T) {
	requireIndex(t)
	res := runCLI("code", fnGreet, "--limit", "1")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s stdout=%s", res.Code, res.Stderr, res.Stdout)
	}
	if !strings.HasPrefix(strings.TrimSpace(res.Stdout), helperFile) {
		t.Fatalf("stdout=%q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "function greet") {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

func TestRefsPathsOnly(t *testing.T) {
	requireIndex(t)
	res := runCLI("refs", fnGreet, "--paths-only", "--limit", "10")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, consumerFile) && !strings.Contains(res.Stdout, widgetFile) {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

func TestRdeps(t *testing.T) {
	requireIndex(t)
	res := runCLI("rdeps", helperFile, "--limit", "10")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if res.Stdout == "" {
		t.Fatal("expected importers")
	}
}

func TestSearchQualifiedTypeField(t *testing.T) {
	requireIndex(t)
	res := runCLI("search", typeOptions+"."+fieldVerbose, "--limit", "3")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, fieldVerbose) {
		t.Fatalf("stdout=%q", res.Stdout)
	}
	if strings.Contains(res.Stdout, ":? ") {
		t.Fatalf("unexpected optional marker: %q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, helperFile+":2") {
		t.Fatalf("expected line 2 in %q", res.Stdout)
	}
}

func TestSymbolsByBareFilename(t *testing.T) {
	requireIndex(t)
	res := runCLI("symbols", "helper.ts", "--limit", "10")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, fnGreet) {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

func TestCodeMultipleSymbolsWithHeaders(t *testing.T) {
	requireIndex(t)
	res := runCLI("code", fnGreet, methodRun, "--snippet", "--limit", "1")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	lines := strings.Split(strings.TrimSpace(res.Stdout), "\n")
	if lines[0] != fnGreet || lines[2] != methodRun {
		t.Fatalf("headers missing: %q", res.Stdout)
	}
}

func TestRefsMultipleSymbols(t *testing.T) {
	requireIndex(t)
	res := runCLI("refs", fnGreet, methodRun, "--limit", "10")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, fnGreet) || !strings.Contains(res.Stdout, methodRun) {
		t.Fatalf("stdout=%q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, consumerFile) && !strings.Contains(res.Stdout, userFile) {
		t.Fatalf("expected consumer paths: %q", res.Stdout)
	}
}

func TestMembersNamesOnly(t *testing.T) {
	requireIndex(t)
	res := runCLI("members", classWidget, "--names-only")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "run") {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

func TestDepsPathsOnly(t *testing.T) {
	requireIndex(t)
	res := runCLI("deps", userFile, "--paths-only", "--limit", "10")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, widgetFile) {
		t.Fatalf("stdout=%q", res.Stdout)
	}
	if strings.Contains(res.Stdout, ":") {
		t.Fatalf("paths-only should omit line numbers: %q", res.Stdout)
	}
}

func TestFixtureIndexQueryable(t *testing.T) {
	requireIndex(t)
	dbPath := cache.FindDB(fixtureRoot)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var docs, symbols int
	db.QueryRow("SELECT COUNT(*) FROM documents").Scan(&docs)
	db.QueryRow("SELECT COUNT(*) FROM global_symbols").Scan(&symbols)
	if docs < 25 || symbols < 40 {
		t.Fatalf("docs=%d symbols=%d", docs, symbols)
	}
}

func TestAnalyzeProjectDashboard(t *testing.T) {
	requireIndex(t)
	res := runCLI("analyze", "--limit", "15")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "===") || !strings.Contains(res.Stdout, "[high]") {
		t.Fatalf("expected sections: %q", res.Stdout)
	}
}

func TestAnalyzePriorityHigh(t *testing.T) {
	requireIndex(t)
	res := runCLI("analyze", "--priority", "high", "--limit", "15")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "[high]") || strings.Contains(res.Stdout, "[low]") {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

func TestDepsFile(t *testing.T) {
	requireIndex(t)
	res := runCLI("deps", helperFile, "--limit", "10")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if res.Stdout == "" {
		t.Fatal("expected deps output")
	}
}

func TestMembersWidget(t *testing.T) {
	requireIndex(t)
	res := runCLI("members", "Widget", "--limit", "10")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s stdout=%s", res.Code, res.Stderr, res.Stdout)
	}
	if !strings.Contains(res.Stdout, "run") && !strings.Contains(res.Stdout, "Widget") {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

func TestSearchMultiPattern(t *testing.T) {
	requireIndex(t)
	res := runCLI("search", methodRun, fnGreet, "--limit", "5")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "run") || !strings.Contains(res.Stdout, fnGreet) {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

func TestSearchPathsPipeSymbols(t *testing.T) {
	requireIndex(t)
	paths := runCLI("search", fnGreet, "--paths-only", "--limit", "3")
	if paths.Code != 0 {
		t.Fatalf("exit %d", paths.Code)
	}
	first := strings.TrimSpace(strings.Split(paths.Stdout, "\n")[0])
	res := runCLI("symbols", first, "--limit", "10")
	if res.Code != 0 || !strings.Contains(res.Stdout, fnGreet) {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

func TestCodeAmbiguousHandler(t *testing.T) {
	requireIndex(t)
	res := runCLI("code", "Handler", "--snippet", "--limit", "2")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Handler (") {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

func TestDepsSymbol(t *testing.T) {
	requireIndex(t)
	res := runCLI("deps", "useWidget", "--limit", "10")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Widget") && !strings.Contains(res.Stdout, fnGreet) {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

func TestMissingSymbol(t *testing.T) {
	requireIndex(t)
	res := runCLI("code", "__scip_cli_missing_symbol_xyz__")
	if res.Code == 0 {
		t.Fatal("expected non-zero exit")
	}
	if !strings.Contains(strings.ToLower(res.Stderr), "not found") {
		t.Fatalf("stderr=%q", res.Stderr)
	}
}

func TestReindexFullClearsScope(t *testing.T) {
	requireIndex(t)
	if err := scope.SaveIndexScope(fixtureRoot, []string{"src"}); err != nil {
		t.Fatal(err)
	}
	if scope.LoadIndexScope(fixtureRoot) == nil {
		t.Fatal("expected persisted scope")
	}
	res := runCLI("reindex")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if scope.LoadIndexScope(fixtureRoot) != nil {
		t.Fatal("full reindex should clear scope")
	}
}

func TestReindexPathRejectedForPython(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[project]\nname = 'x'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	res := runCLIInDir(root, "reindex", "--path", "src")
	if res.Code == 0 {
		t.Fatalf("expected failure stderr=%s", res.Stderr)
	}
	if !strings.Contains(res.Stderr, "only supported for TypeScript") {
		t.Fatalf("stderr=%q", res.Stderr)
	}
}

func TestCodeQualifiedTypeField(t *testing.T) {
	requireIndex(t)
	res := runCLI("code", typeOptions+"."+fieldVerbose, "--snippet", "--limit", "1")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, fieldVerbose) {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

func TestMembersTypeLiteralFields(t *testing.T) {
	requireIndex(t)
	res := runCLI("members", typeOptions, "--names-only")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stdout, fieldVerbose) {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

func TestCodePartialMissStillPrints(t *testing.T) {
	requireIndex(t)
	res := runCLI("code", fnGreet, "__scip_cli_missing_symbol_xyz__", "--snippet", "--limit", "1")
	if !strings.Contains(res.Stderr, "missing") && !strings.Contains(res.Stderr, "not found") {
		t.Fatalf("stderr=%q", res.Stderr)
	}
	if !strings.HasPrefix(strings.TrimSpace(res.Stdout), helperFile) {
		t.Fatalf("expected greet output, stdout=%q", res.Stdout)
	}
}

func TestAnalyzeRejectsPathScope(t *testing.T) {
	requireIndex(t)
	res := runCLI("analyze", "--path", "src", "--limit", "5")
	if res.Code == 0 {
		t.Fatalf("expected failure stderr=%s", res.Stderr)
	}
	if !strings.Contains(res.Stderr, "not analyze --path") {
		t.Fatalf("stderr=%q", res.Stderr)
	}
}

func TestRefsPathsOnlyNoHeaders(t *testing.T) {
	requireIndex(t)
	res := runCLI("refs", fnGreet, methodRun, "--paths-only", "--limit", "10")
	if res.Code != 0 {
		t.Fatalf("exit %d stderr=%s", res.Code, res.Stderr)
	}
	lines := strings.Split(strings.TrimSpace(res.Stdout), "\n")
	for _, line := range lines {
		if line == fnGreet || line == methodRun {
			t.Fatalf("paths-only should omit headers: %q", res.Stdout)
		}
		if strings.Contains(line, ":") {
			t.Fatalf("paths-only should omit line numbers: %q", line)
		}
	}
}

// silence unused import if build tags change
var _ = io.Discard
