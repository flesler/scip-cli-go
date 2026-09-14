package cross_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/flesler/scip-cli-go/v2/internal/cross"
)

var (
	pySession    *cross.Session
	goSession    *cross.Session
	indexOK      bool
	goIndexOK    bool
	indexSkip    string
	goBinary     string
	pythonBinary string
	fixtureSrc   string
)

func TestMain(m *testing.M) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	goBinary = filepath.Join(repoRoot, "bin", "scip-cli-go")

	if out, err := exec.Command("go", "build", "-o", goBinary, filepath.Join(repoRoot, "cmd", "scip-cli")).CombinedOutput(); err != nil {
		os.Stderr.WriteString("build scip-cli-go failed: " + string(out) + "\n")
		os.Exit(1)
	}

	var err error
	pythonBinary, err = cross.ResolvePythonCLI(repoRoot)
	if err != nil {
		indexSkip = err.Error()
		os.Exit(m.Run())
	}

	if _, err := exec.LookPath("npx"); err != nil {
		indexSkip = "npx not found (Node.js required for fixture indexing)"
		os.Exit(m.Run())
	}

	tmp, err := os.MkdirTemp("", "scip-cross-*")
	if err != nil {
		os.Exit(1)
	}

	fixtureSrc = filepath.Join(repoRoot, "testdata", "fixtures", "typescript-project")

	// Python-indexed fixture (query parity baseline).
	pyFixture := filepath.Join(tmp, "py-fixture")
	pyHome := filepath.Join(tmp, "py-home")
	if err := os.MkdirAll(pyHome, 0755); err != nil {
		os.Exit(1)
	}
	if err := cross.CopyDir(fixtureSrc, pyFixture); err != nil {
		os.Stderr.WriteString("copy fixture failed: " + err.Error() + "\n")
		os.Exit(1)
	}
	pySession = &cross.Session{
		FixtureDir:   pyFixture,
		HomeDir:      pyHome,
		PythonBinary: pythonBinary,
		GoBinary:     goBinary,
	}
	if err := pySession.Reindex(pythonBinary); err != nil {
		indexSkip = err.Error()
	} else {
		indexOK = true
	}

	// Go-indexed fixture (catches Go indexing/merge regressions).
	goFixture := filepath.Join(tmp, "go-fixture")
	goHome := filepath.Join(tmp, "go-home")
	if err := os.MkdirAll(goHome, 0755); err != nil {
		os.Exit(1)
	}
	if err := cross.CopyDir(fixtureSrc, goFixture); err != nil {
		os.Stderr.WriteString("copy go fixture failed: " + err.Error() + "\n")
		os.Exit(1)
	}
	goSession = &cross.Session{
		FixtureDir:   goFixture,
		HomeDir:      goHome,
		PythonBinary: pythonBinary,
		GoBinary:     goBinary,
	}
	if err := goSession.Reindex(goBinary); err != nil {
		if indexSkip == "" {
			indexSkip = "go reindex: " + err.Error()
		}
	} else {
		goIndexOK = true
	}

	os.Exit(m.Run())
}

func requireCrossIndex(t *testing.T) {
	t.Helper()
	if indexSkip != "" {
		t.Skip(indexSkip)
	}
	if !indexOK {
		t.Skip("python-indexed fixture unavailable")
	}
}

func requireGoIndex(t *testing.T) {
	t.Helper()
	if indexSkip != "" {
		t.Skip(indexSkip)
	}
	if !goIndexOK {
		t.Skip("go-indexed fixture unavailable")
	}
}

type compareCase struct {
	name            string
	args            []string
	compareStdout   bool
	compareStderr   bool
	wantExitNonZero bool
}

func compareCLIs(t *testing.T, s *cross.Session, tc compareCase) {
	t.Helper()
	py := s.RunPython(tc.args...)
	goRes := s.RunGo(tc.args...)

	if tc.wantExitNonZero {
		if py.Code == 0 || goRes.Code == 0 {
			t.Fatalf("%s: expected non-zero exit (py=%d go=%d)", tc.name, py.Code, goRes.Code)
		}
		if tc.compareStderr {
			if py.Code != goRes.Code {
				t.Fatalf("%s: exit code mismatch (py=%d go=%d)", tc.name, py.Code, goRes.Code)
			}
			if normalize(py.Stderr) != normalize(goRes.Stderr) {
				t.Errorf("%s stderr mismatch:\nPython:\n%s\n\nGo:\n%s", tc.name, py.Stderr, goRes.Stderr)
			}
		}
		return
	}

	if py.Code != 0 {
		t.Fatalf("%s: Python exit %d stderr=%s", tc.name, py.Code, py.Stderr)
	}
	if goRes.Code != 0 {
		t.Fatalf("%s: Go exit %d stderr=%s", tc.name, goRes.Code, goRes.Stderr)
	}

	if tc.compareStdout && normalize(py.Stdout) != normalize(goRes.Stdout) {
		t.Errorf("%s stdout mismatch:\nPython:\n%s\n\nGo:\n%s", tc.name, py.Stdout, goRes.Stdout)
	}
	if tc.compareStderr && normalize(py.Stderr) != normalize(goRes.Stderr) {
		t.Errorf("%s stderr mismatch:\nPython:\n%s\n\nGo:\n%s", tc.name, py.Stderr, goRes.Stderr)
	}
}

func normalize(s string) string {
	lines := strings.Split(s, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimRight(line, " \t\r")
		if line == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.Join(cleaned, "\n")
}

var (
	skillContributorsRe = regexp.MustCompile(`(?m)^\*\*Contributors:\*\*.*$`)
	skillDogfoodRe      = regexp.MustCompile(`(?m)^\*\*Dogfood loop:\*\*.*$`)
)

// normalizeSkill strips Go-port overlay diffs that are intentional vs Python scip-cli.
func normalizeSkill(s string) string {
	s = normalize(s)
	s = skillContributorsRe.ReplaceAllString(s, "**Contributors:** <port-specific>")
	s = strings.ReplaceAll(s, "(`scip_cli`, `src/pkg/`)", "(`<pkg>`, `src/pkg/`)")
	s = strings.ReplaceAll(s, "(`internal/`, `src/pkg/`)", "(`<pkg>`, `src/pkg/`)")
	s = skillDogfoodRe.ReplaceAllString(s, "**Dogfood loop:** <port-specific>")
	return s
}

var parityCases = []compareCase{
	{name: "search_greet", args: []string{"search", cross.FnGreet, "--limit", "3"}, compareStdout: true},
	{name: "search_qualified_field", args: []string{"search", cross.TypeOptions + "." + cross.FieldVerbose, "--limit", "3"}, compareStdout: true},
	{name: "search_multi_pattern", args: []string{"search", cross.MethodRun, cross.FnGreet, "--limit", "5"}, compareStdout: true},
	{name: "search_paths_only", args: []string{"search", cross.FnGreet, "--paths-only", "--limit", "3"}, compareStdout: true},
	{name: "search_kind_class", args: []string{"search", "Widget", "--kind", "class", "--limit", "5"}, compareStdout: true},
	{name: "search_names_only", args: []string{"search", cross.FnGreet, "--names-only", "--limit", "3"}, compareStdout: true},

	{name: "symbols_by_path", args: []string{"symbols", cross.HelperFile, "--limit", "10"}, compareStdout: true},
	{name: "symbols_bare_filename", args: []string{"symbols", "helper.ts", "--limit", "10"}, compareStdout: true},

	{name: "code_single", args: []string{"code", cross.FnGreet, "--limit", "1"}, compareStdout: true},
	{name: "code_multi_snippet", args: []string{"code", cross.FnGreet, cross.MethodRun, "--snippet", "--limit", "1"}, compareStdout: true},
	{name: "code_ambiguous_handler", args: []string{"code", cross.ClassHandler, "--snippet", "--limit", "2"}, compareStdout: true},
	{name: "code_qualified_field", args: []string{"code", cross.TypeOptions + "." + cross.FieldVerbose, "--snippet", "--limit", "1"}, compareStdout: true},
	{name: "code_line_numbers", args: []string{"code", cross.FnGreet, "--limit", "1", "-n"}, compareStdout: true},
	{name: "code_full", args: []string{"code", cross.ClassWidget, "--full", "--limit", "1"}, compareStdout: true},

	{name: "refs_paths_only", args: []string{"refs", cross.FnGreet, "--paths-only", "--limit", "10"}, compareStdout: true},
	{name: "refs_multi", args: []string{"refs", cross.FnGreet, cross.MethodRun, "--limit", "10"}, compareStdout: true},
	{name: "refs_multi_paths_only", args: []string{"refs", cross.FnGreet, cross.MethodRun, "--paths-only", "--limit", "10"}, compareStdout: true},
	{name: "refs_ambiguous_stderr", args: []string{"refs", cross.TypeOpts, "--limit", "5"}, compareStdout: true, compareStderr: true},

	{name: "members_widget", args: []string{"members", cross.ClassWidget, "--names-only"}, compareStdout: true},
	{name: "members_options", args: []string{"members", cross.TypeOptions, "--names-only"}, compareStdout: true},

	{name: "rdeps_helper", args: []string{"rdeps", cross.HelperFile, "--limit", "10"}, compareStdout: true},
	{name: "deps_symbol", args: []string{"deps", "useWidget", "--limit", "10"}, compareStdout: true},
	{name: "deps_short_symbol", args: []string{"deps", "io", "--limit", "5"}, wantExitNonZero: true},
	{name: "deps_file", args: []string{"deps", cross.UserFile, "--limit", "10"}, compareStdout: true},
	{name: "deps_paths_only", args: []string{"deps", cross.UserFile, "--paths-only", "--limit", "10"}, compareStdout: true},

	{name: "analyze_project", args: []string{"analyze", "--limit", "5"}, compareStdout: true},
	{name: "analyze_priority_high", args: []string{"analyze", "--priority", "high", "--limit", "15"}, compareStdout: true},
	{name: "analyze_check_cycles", args: []string{"analyze", "--check", "cycles", "--limit", "5"}, compareStdout: true},
	{name: "analyze_check_dead_files", args: []string{"analyze", "--check", "dead_files", "--limit", "5"}, compareStdout: true},
	{name: "analyze_file", args: []string{"analyze", cross.HelperFile, "--limit", "10"}, compareStdout: true},
	{name: "analyze_symbol", args: []string{"analyze", cross.ClassWidget, "--limit", "10"}, compareStdout: true},
	{name: "analyze_directory", args: []string{"analyze", "src", "--limit", "10"}, compareStdout: true},
}

func TestCrossCompare_version(t *testing.T) {
	requireCrossIndex(t)
	py := pySession.RunPython("--version")
	goRes := pySession.RunGo("--version")
	if py.Code != 0 || goRes.Code != 0 {
		t.Fatalf("version failed py=%d go=%d", py.Code, goRes.Code)
	}
	if !strings.Contains(py.Stdout, "scip-cli") {
		t.Fatalf("unexpected Python version: %q", py.Stdout)
	}
	if !strings.Contains(goRes.Stdout, "scip-cli-go") {
		t.Fatalf("unexpected Go version: %q", goRes.Stdout)
	}
}

func TestCrossCompare_skill(t *testing.T) {
	requireCrossIndex(t)
	py := pySession.RunPython("skill")
	goRes := pySession.RunGo("skill")
	if py.Code != 0 || goRes.Code != 0 {
		t.Fatalf("skill failed py=%d go=%d", py.Code, goRes.Code)
	}
	if normalizeSkill(py.Stdout) != normalizeSkill(goRes.Stdout) {
		t.Errorf("skill stdout mismatch:\nPython:\n%s\n\nGo:\n%s", py.Stdout, goRes.Stdout)
	}
}

func TestCrossCompare_pythonIndexed(t *testing.T) {
	requireCrossIndex(t)
	for _, tc := range parityCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			compareCLIs(t, pySession, tc)
		})
	}
}

func TestCrossCompare_goIndexed(t *testing.T) {
	requireGoIndex(t)
	for _, tc := range parityCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			compareCLIs(t, goSession, tc)
		})
	}
}

func TestCrossCompare_search_paths_pipe_symbols(t *testing.T) {
	requireCrossIndex(t)
	pyPaths := pySession.RunPython("search", cross.FnGreet, "--paths-only", "--limit", "3")
	goPaths := pySession.RunGo("search", cross.FnGreet, "--paths-only", "--limit", "3")
	if pyPaths.Code != 0 || goPaths.Code != 0 {
		t.Fatalf("paths-only search failed")
	}
	if normalize(pyPaths.Stdout) != normalize(goPaths.Stdout) {
		t.Fatalf("search paths mismatch")
	}
	pyFirst := strings.TrimSpace(strings.Split(pyPaths.Stdout, "\n")[0])
	compareCLIs(t, pySession, compareCase{
		name:          "symbols_from_search_path",
		args:          []string{"symbols", pyFirst, "--limit", "10"},
		compareStdout: true,
	})
}

func TestCrossCompare_code_partial_miss(t *testing.T) {
	requireCrossIndex(t)
	args := []string{"code", cross.FnGreet, "__scip_cli_missing_symbol_xyz__", "--snippet", "--limit", "1"}
	py := pySession.RunPython(args...)
	goRes := pySession.RunGo(args...)
	if py.Code != 0 || goRes.Code != 0 {
		t.Fatalf("expected success with partial miss py=%d go=%d", py.Code, goRes.Code)
	}
	if normalize(py.Stdout) != normalize(goRes.Stdout) {
		t.Errorf("stdout mismatch:\nPython:\n%s\n\nGo:\n%s", py.Stdout, goRes.Stdout)
	}
	lowPy := strings.ToLower(py.Stderr)
	lowGo := strings.ToLower(goRes.Stderr)
	if !strings.Contains(lowPy, "not found") && !strings.Contains(lowPy, "missing") {
		t.Fatalf("Python stderr=%q", py.Stderr)
	}
	if !strings.Contains(lowGo, "not found") && !strings.Contains(lowGo, "missing") {
		t.Fatalf("Go stderr=%q", goRes.Stderr)
	}
}

func TestCrossCompare_missing_symbol_exit(t *testing.T) {
	requireCrossIndex(t)
	compareCLIs(t, pySession, compareCase{
		name:            "code_missing_symbol",
		args:            []string{"code", "__scip_cli_missing_symbol_xyz__"},
		wantExitNonZero: true,
	})
}

func TestCrossCompare_invalid_kind_exit(t *testing.T) {
	requireCrossIndex(t)
	compareCLIs(t, pySession, compareCase{
		name:            "search_invalid_kind",
		args:            []string{"search", "greet", "--kind", "bogus"},
		wantExitNonZero: true,
	})
}

func TestCrossCompare_invalid_priority_exit(t *testing.T) {
	requireCrossIndex(t)
	compareCLIs(t, pySession, compareCase{
		name:            "analyze_invalid_priority",
		args:            []string{"analyze", "--priority", "bogus", "--limit", "5"},
		wantExitNonZero: true,
	})
}

func TestCrossCompare_analyze_rejects_path_scope(t *testing.T) {
	requireCrossIndex(t)
	compareCLIs(t, pySession, compareCase{
		name:            "analyze_rejects_path",
		args:            []string{"analyze", "--path", "src", "--limit", "5"},
		wantExitNonZero: true,
	})
}

func TestCrossCompare_analyze_unknown_check(t *testing.T) {
	requireCrossIndex(t)
	compareCLIs(t, pySession, compareCase{
		name:            "analyze_unknown_check",
		args:            []string{"analyze", "--check", "not_a_check"},
		wantExitNonZero: true,
		compareStderr:   true,
	})
}

func TestCrossCompare_reindex_exclude_globs(t *testing.T) {
	requireCrossIndex(t)

	excludeArgs := []string{
		"reindex",
		"--exclude", cross.DefaultExcludeGlob1,
		"--exclude", cross.DefaultExcludeGlob2,
		"--exclude", cross.DefaultExcludeGlob3,
	}

	pyExcludeFixture := filepath.Join(filepath.Dir(pySession.FixtureDir), "exclude-py")
	pyExcludeHome := filepath.Join(filepath.Dir(pySession.HomeDir), "exclude-py-home")
	goExcludeFixture := filepath.Join(filepath.Dir(goSession.FixtureDir), "exclude-go")
	goExcludeHome := filepath.Join(filepath.Dir(goSession.HomeDir), "exclude-go-home")

	setupExcludeSession := func(fixtureDir, homeDir string) (*cross.Session, error) {
		if err := os.RemoveAll(fixtureDir); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(homeDir, 0755); err != nil {
			return nil, err
		}
		if err := cross.CopyDir(fixtureSrc, fixtureDir); err != nil {
			return nil, err
		}
		return &cross.Session{
			FixtureDir:   fixtureDir,
			HomeDir:      homeDir,
			PythonBinary: pythonBinary,
			GoBinary:     goBinary,
		}, nil
	}

	pySess, err := setupExcludeSession(pyExcludeFixture, pyExcludeHome)
	if err != nil {
		t.Fatal(err)
	}
	goSess, err := setupExcludeSession(goExcludeFixture, goExcludeHome)
	if err != nil {
		t.Fatal(err)
	}

	pyReindex := pySess.RunPython(excludeArgs...)
	if pyReindex.Code != 0 {
		t.Fatalf("Python exclude reindex: exit %d stderr=%s", pyReindex.Code, pyReindex.Stderr)
	}
	goReindex := goSess.RunGo(excludeArgs...)
	if goReindex.Code != 0 {
		t.Fatalf("Go exclude reindex: exit %d stderr=%s", goReindex.Code, goReindex.Stderr)
	}

	compareCLIs(t, pySess, compareCase{
		name:          "symbols_after_exclude",
		args:          []string{"symbols", cross.HelperFile, "--limit", "10"},
		compareStdout: true,
	})

	pySearch := pySess.RunPython("search", cross.FnFixtureOnlyHelper, "--limit", "5")
	goSearch := goSess.RunGo("search", cross.FnFixtureOnlyHelper, "--limit", "5")
	if pySearch.Code == 0 || goSearch.Code == 0 {
		t.Fatalf("expected search miss after exclude (py=%d go=%d)", pySearch.Code, goSearch.Code)
	}
	if pySearch.Code != goSearch.Code {
		t.Fatalf("search exit mismatch after exclude (py=%d go=%d)", pySearch.Code, goSearch.Code)
	}

	compareCLIs(t, goSess, compareCase{
		name:          "symbols_after_exclude_go_index",
		args:          []string{"symbols", cross.HelperFile, "--limit", "10"},
		compareStdout: true,
	})
}
