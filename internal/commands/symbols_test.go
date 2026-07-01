package commands

import (
	"testing"

	"github.com/flesler/scip-cli-go/v2/internal/queries"
)

func TestSortByFrequency_basic(t *testing.T) {
	syms := []queries.FileSymbol{
		{Symbol: "scip-typescript npm app 1.0 src/a.ts/foo()."},
		{Symbol: "scip-typescript npm app 1.0 src/a.ts/bar()."},
		{Symbol: "scip-typescript npm app 1.0 src/a.ts/foo()."},
		{Symbol: "scip-typescript npm app 1.0 src/a.ts/baz()."},
		{Symbol: "scip-typescript npm app 1.0 src/a.ts/bar()."},
		{Symbol: "scip-typescript npm app 1.0 src/a.ts/bar()."},
	}

	sorted := sortByFrequency(syms)

	// bar appears 3 times, foo appears 2 times, baz appears 1 time
	if sorted[0].Symbol != "scip-typescript npm app 1.0 src/a.ts/bar()." {
		t.Fatalf("expected bar first, got %s", sorted[0].Symbol)
	}
	if sorted[3].Symbol != "scip-typescript npm app 1.0 src/a.ts/foo()." {
		t.Fatalf("expected foo at position 3-4, got %s", sorted[3].Symbol)
	}
	if sorted[5].Symbol != "scip-typescript npm app 1.0 src/a.ts/baz()." {
		t.Fatalf("expected baz last, got %s", sorted[5].Symbol)
	}
}

func TestSortByFrequency_tiesAlphabetical(t *testing.T) {
	syms := []queries.FileSymbol{
		{Symbol: "scip-typescript npm app 1.0 src/a.ts/zebra()."},
		{Symbol: "scip-typescript npm app 1.0 src/a.ts/alpha()."},
		{Symbol: "scip-typescript npm app 1.0 src/a.ts/middle()."},
	}

	sorted := sortByFrequency(syms)

	// All appear once, so should be alphabetical
	if sorted[0].Symbol != "scip-typescript npm app 1.0 src/a.ts/alpha()." {
		t.Fatalf("expected alpha first, got %s", sorted[0].Symbol)
	}
	if sorted[1].Symbol != "scip-typescript npm app 1.0 src/a.ts/middle()." {
		t.Fatalf("expected middle second, got %s", sorted[1].Symbol)
	}
	if sorted[2].Symbol != "scip-typescript npm app 1.0 src/a.ts/zebra()." {
		t.Fatalf("expected zebra last, got %s", sorted[2].Symbol)
	}
}

func TestSortByFrequency_empty(t *testing.T) {
	syms := []queries.FileSymbol{}
	sorted := sortByFrequency(syms)
	if len(sorted) != 0 {
		t.Fatalf("expected empty slice, got %d elements", len(sorted))
	}
}
