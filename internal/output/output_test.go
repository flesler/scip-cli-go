package output_test

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/sourcegraph/scip-cli-go/internal/output"
)

func captureStderr(fn func()) string {
	r, w, _ := os.Pipe()
	old := os.Stderr
	os.Stderr = w
	fn()
	_ = w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestSymbolOutputLabel(t *testing.T) {
	sym := "scip-typescript npm test 1.0 src/`a.ts`/greet()."
	if output.SymbolOutputLabel("greet", sym, 1) != "greet" {
		t.Fatal("single match")
	}
	cls := "scip-typescript npm test 1.0 src/`a.ts`/Foo#"
	if output.SymbolOutputLabel("Foo", cls, 2) != "Foo (src/a.ts)" {
		t.Fatal("ambiguous")
	}
}

func TestFormatLineRange(t *testing.T) {
	start, end := 0, 10
	if output.FormatLineRange(&start, &end, ":") != "1:11" {
		t.Fatal("both defined")
	}
	startOnly := 5
	if output.FormatLineRange(&startOnly, nil, ":") != "6:?" {
		t.Fatal("start only")
	}
	if output.FormatLineRange(nil, nil, ":") != "??" {
		t.Fatal("neither")
	}
}

func TestFormatDefBody_truncates(t *testing.T) {
	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, "line\n")
	}
	res, err := output.FormatDefBody(lines, 10, 209, 80, 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated || strings.Count(res.Body, "\n") != 79 {
		t.Fatalf("truncated=%v body lines=%d", res.Truncated, strings.Count(res.Body, "\n"))
	}
}

func TestPrintDefTruncationNotice(t *testing.T) {
	err := captureStderr(func() {
		output.PrintDefTruncationNotice("bigFn", 0, 80, 200)
	})
	if !strings.Contains(err, "80/200") || !strings.Contains(err, "code --offset 80 bigFn") {
		t.Fatalf("stderr=%q", err)
	}
}

func captureStdout(fn func()) string {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestMaybePrintSymbolHeader(t *testing.T) {
	// Test hidden for single symbol
	captured := captureStdout(func() {
		output.MaybePrintSymbolHeader("greet", false)
	})
	if captured != "" {
		t.Fatalf("expected empty output for single symbol, got %q", captured)
	}

	// Test shown for multiple symbols
	captured = captureStdout(func() {
		output.MaybePrintSymbolHeader("greet", true)
	})
	if !strings.Contains(captured, "greet") {
		t.Fatalf("expected header for multiple symbols, got %q", captured)
	}
}

func TestFormatDefBody_unlimitedWhenMaxLinesZero(t *testing.T) {
	var lines []string
	for i := 0; i < 5; i++ {
		lines = append(lines, "line\n")
	}
	res, err := output.FormatDefBody(lines, 0, 4, 0, 0, 0, false)
	if err != nil || res.Truncated || strings.Count(res.Body, "\n") != 4 {
		t.Fatalf("body=%q truncated=%v err=%v", res.Body, res.Truncated, err)
	}
}

func TestFormatDefBody_charCapTruncates(t *testing.T) {
	lines := []string{"abcdefghij\n"}
	res, err := output.FormatDefBody(lines, 0, 0, 0, 5, 0, false)
	if err != nil || !res.Truncated || !strings.Contains(res.Body, "...") {
		t.Fatalf("body=%q truncated=%v err=%v", res.Body, res.Truncated, err)
	}
}

func TestPrintDefTruncationNotice_noHintWhenFullyShown(t *testing.T) {
	err := captureStderr(func() {
		output.PrintDefTruncationNotice("bigFn", 0, 200, 200)
	})
	if err != "" {
		t.Fatalf("expected empty stderr, got %q", err)
	}
}
