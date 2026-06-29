package analyze

import (
	"strings"
	"testing"

	"github.com/flesler/scip-cli-go/internal/analyze/testdb"
)

func TestFindLongerCyclesThreeNode(t *testing.T) {
	edges := []FileEdge{
		{"a.ts", "b.ts"},
		{"b.ts", "c.ts"},
		{"c.ts", "a.ts"},
	}
	lines, err := FindLongerCycles(edges, 8, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 cycle, got %d: %v", len(lines), lines)
	}
	for _, f := range []string{"a.ts", "b.ts", "c.ts"} {
		if !strings.Contains(lines[0], f) {
			t.Fatalf("missing %s in %q", f, lines[0])
		}
	}
}

func TestFindLongerCyclesTwoNodeSkipped(t *testing.T) {
	edges := []FileEdge{{"a.ts", "b.ts"}, {"b.ts", "a.ts"}}
	lines, err := FindLongerCycles(edges, 8, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected no 3+ cycles, got %v", lines)
	}
}

func TestFindLongerCyclesMiniCodebase(t *testing.T) {
	b, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	symX := b.Define("src/cycle/a.ts", "alpha", 0, 10)
	symY := b.Define("src/cycle/b.ts", "beta", 0, 10)
	b.Reference("src/cycle/a.ts", symY)
	b.Reference("src/cycle/b.ts", symX)
	edges, err := FetchFileEdges(b.Finish())
	if err != nil {
		t.Fatal(err)
	}
	lines, err := FindLongerCycles(edges, 8, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected no long cycles, got %v", lines)
	}
}

func TestFetchEdgesSkipsTypeSymbols(t *testing.T) {
	b, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	tA := b.DefineType("src/types/a.ts", "AType", 0, 5)
	tB := b.DefineType("src/types/b.ts", "BType", 0, 5)
	b.Reference("src/types/a.ts", tB)
	b.Reference("src/types/b.ts", tA)
	edges, err := FetchFileEdges(b.Finish())
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 0 {
		t.Fatalf("expected no type-only edges, got %v", edges)
	}
}

func TestFetchEdgesKeepsRuntimeSymbols(t *testing.T) {
	b, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	symB := b.Define("src/runtime/b.ts", "runB", 0, 10)
	symA := b.Define("src/runtime/a.ts", "runA", 0, 10)
	b.Reference("src/runtime/a.ts", symB)
	b.Reference("src/runtime/b.ts", symA)
	edges, err := FetchFileEdges(b.Finish())
	if err != nil {
		t.Fatal(err)
	}
	has := func(from, to string) bool {
		for _, e := range edges {
			if e.From == from && e.To == to {
				return true
			}
		}
		return false
	}
	if !has("src/runtime/a.ts", "src/runtime/b.ts") || !has("src/runtime/b.ts", "src/runtime/a.ts") {
		t.Fatalf("missing runtime edges: %v", edges)
	}
}

func TestFetchEdgesSkipsModuleBarrelEdges(t *testing.T) {
	b, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	modA := b.DefineModule("src/barrel/a.ts")
	modB := b.DefineModule("src/barrel/b.ts")
	b.Reference("src/barrel/a.ts", modB)
	b.Reference("src/barrel/b.ts", modA)
	edges, err := FetchFileEdges(b.Finish())
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 0 {
		t.Fatalf("expected no module barrel edges, got %v", edges)
	}
}
