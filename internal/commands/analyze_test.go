package commands

import (
	"testing"

	"github.com/flesler/scip-cli-go/v2/internal/analyze"
)

func TestProjectIncludeTestsForTestPath(t *testing.T) {
	if !projectIncludeTests(false, "tests/test_foo.py") {
		t.Fatal("test file path should enable include_tests")
	}
	if projectIncludeTests(false, "scip_cli/queries.py") {
		t.Fatal("production path should not auto-include tests")
	}
	if !projectIncludeTests(true, "scip_cli/queries.py") {
		t.Fatal("explicit include_tests should win")
	}
}

func TestProjectIncludeTestsUsesAnalyzeHelper(t *testing.T) {
	if !analyze.IsTestPath("tests/test_foo.py") {
		t.Fatal("IsTestPath mismatch")
	}
}
