package analyze

import (
	"testing"

	"github.com/flesler/scip-cli-go/v2/internal/analyze/testdb"
)

func TestIsTestPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"tests/test_foo.py", true},
		{"src/foo.test.ts", true},
		{"src/foo.spec.tsx", true},
		{"pkg/__tests__/bar.js", true},
		{"conftest.py", true},
		{"scip_cli/queries.py", false},
	}
	for _, c := range cases {
		if got := IsTestPath(c.path); got != c.want {
			t.Fatalf("%q: got %v want %v", c.path, got, c.want)
		}
	}
}

func TestAnalyzeNoise(t *testing.T) {
	if !AnalyzeNoise("tests/test_foo.py", "scip-python x `t.py`/helper().", false) {
		t.Fatal("expected test path noise")
	}
	if !AnalyzeNoise("scip_cli/foo.py", "scip-python x `t.py`/_helper().", false) {
		t.Fatal("expected private helper noise")
	}
	if AnalyzeNoise("scip_cli/foo.py", "scip-python x `t.py`/helper().", false) {
		t.Fatal("expected production helper to pass")
	}
	if AnalyzeNoise("tests/test_foo.py", "scip-python x `t.py`/helper().", true) {
		t.Fatal("include_tests should keep test paths")
	}
}

func TestComponentPropsStaleNoise(t *testing.T) {
	sym := "scip-typescript npm x 1.0 src/ui/`Button.ts`/ButtonProps#"
	if !isComponentPropsType(sym) {
		t.Fatal("expected ButtonProps to match")
	}
	if isComponentPropsType("scip-typescript npm x 1.0 src/`t.ts`/Options#") {
		t.Fatal("Options should not match component props heuristic")
	}
}

func TestStaleTypeNoiseDataclass(t *testing.T) {
	sym := "scip-python x `config.py`/ProjectSettings#"
	if !StaleTypeNoise("scip_cli/config.py", sym, 0) {
		t.Fatal("expected dataclass stale noise at 0 consumers")
	}
	if StaleTypeNoise("scip_cli/config.py", sym, 1) {
		t.Fatal("should not filter when consumers exist")
	}
}

func TestAnalyzeDashboardRunnerNoise(t *testing.T) {
	sym := "scip-python x `project.py`/bottlenecks()."
	if !AnalyzeNoise("scip_cli/analyze/project.py", sym, false) {
		t.Fatal("expected analyze dashboard runner to be filtered")
	}
}

func TestRowBudgetViaRunChecks(t *testing.T) {
	db, err := testdb.MiniCodebase()
	if err != nil {
		t.Fatal(err)
	}
	budget := NewRowBudget(3)
	secs, err := RunProjectSections(db, 50, false, "", nil, budget)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, s := range secs {
		for _, line := range s.Lines {
			if line != "(none)" {
				total++
			}
		}
	}
	if total > 3 {
		t.Fatalf("budget 3 but got %d rows across %d sections", total, len(secs))
	}
}
