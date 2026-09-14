package analyze

import (
	"testing"

	"github.com/flesler/scip-cli-go/v2/internal/analyze/testdb"
)

func TestIsLowSignalDeadFile(t *testing.T) {
	b, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	docID, _ := b.AddFile("src/ui/widget.ts")
	b.DefineModule("src/ui/widget.ts")
	if !IsLowSignalDeadFile(b.Finish(), docID) {
		t.Fatal("module-only widget file should be low-signal")
	}

	typed, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	typed.DefineModule("src/ui/button.ts")
	typed.DefineType("src/ui/button.ts", "ButtonProps", 0, 10)
	typedDB := typed.Finish()
	typedDoc, err := testdb.DocumentID(typedDB, "src/ui/button.ts")
	if err != nil {
		t.Fatal(err)
	}
	if !IsLowSignalDeadFile(typedDB, typedDoc) {
		t.Fatal("module+type-only file should be low-signal")
	}

	mixed, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	mixed.DefineModule("src/lib/api.ts")
	mixed.Define("src/lib/api.ts", "register", 0, 10)
	mixedDB := mixed.Finish()
	mixedDoc, err := testdb.DocumentID(mixedDB, "src/lib/api.ts")
	if err != nil {
		t.Fatal(err)
	}
	if IsLowSignalDeadFile(mixedDB, mixedDoc) {
		t.Fatal("mixed module+value file should not be low-signal")
	}
}

func TestLiveForReusesOneIndexDuringPass(t *testing.T) {
	b, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	db := b.Finish()
	reset, err := BindLive(db)
	if err != nil {
		t.Fatal(err)
	}
	defer reset()

	first, err := LiveFor(db)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LiveFor(db)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("expected same LiveIndex instance during bound pass")
	}
}

func TestNestedBindLiveReusesSameIndex(t *testing.T) {
	b, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	db := b.Finish()
	outer, err := BindLive(db)
	if err != nil {
		t.Fatal(err)
	}
	defer outer()

	first, err := LiveFor(db)
	if err != nil {
		t.Fatal(err)
	}
	inner, err := BindLive(db)
	if err != nil {
		t.Fatal(err)
	}
	innerLive, err := LiveFor(db)
	if err != nil {
		t.Fatal(err)
	}
	if innerLive != first {
		t.Fatal("nested bind should reuse outer LiveIndex")
	}
	inner()
	after, err := LiveFor(db)
	if err != nil {
		t.Fatal(err)
	}
	if after != first {
		t.Fatal("after inner reset, outer binding should remain")
	}
}

func TestLiveForDoesNotReuseIndexForDifferentDB(t *testing.T) {
	b1, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	b2, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	db := b1.Finish()
	other := b2.Finish()
	reset, err := BindLive(db)
	if err != nil {
		t.Fatal(err)
	}
	defer reset()

	bound, err := LiveFor(db)
	if err != nil {
		t.Fatal(err)
	}
	otherIdx, err := LiveFor(other)
	if err != nil {
		t.Fatal(err)
	}
	if otherIdx == bound {
		t.Fatal("different DBs should not share bound LiveIndex")
	}
}
