package analyze

import (
	"strings"
	"testing"

	"github.com/flesler/scip-cli-go/v2/internal/analyze/testdb"
)

func linesContainAny(lines []string, subs ...string) bool {
	for _, line := range lines {
		for _, sub := range subs {
			if strings.Contains(line, sub) {
				return true
			}
		}
	}
	return false
}

func TestHotspotsFindsFoo(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	lines, err := hotspots(db, 5, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !linesContainAny(lines, "foo") {
		t.Fatalf("expected foo in hotspots: %v", lines)
	}
}

func TestCyclesFindsMutualDependency(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	lines, err := cycles(db, 10, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range lines {
		if strings.Contains(line, "cycle/a.ts") && strings.Contains(line, "cycle/b.ts") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cycle pair: %v", lines)
	}
}

func TestCyclesScopedToDirectory(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	lines, err := cycles(db, 10, CheckOptions{Scope: "src/cycle"})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		if !strings.Contains(line, "cycle/") {
			t.Fatalf("scoped cycle leaked: %q", line)
		}
	}
}

func TestDeadExportsIncludesOrphan(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	lines, err := deadExports(db, 20, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !linesContainAny(lines, "Orphan", "deadFn") {
		t.Fatalf("expected dead export: %v", lines)
	}
}

func TestSameFileOnlyFindsHelper(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	lines, err := sameFileOnly(db, 20, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !linesContainAny(lines, "sameFileHelper") {
		t.Fatalf("expected sameFileHelper: %v", lines)
	}
	if linesContainAny(lines, "Orphan") {
		t.Fatalf("Orphan should not be same-file only: %v", lines)
	}
}

func TestTestOnlyConsumers(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	lines, err := symbolsTestOnlyConsumers(db, 20, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !linesContainAny(lines, "testOnlyFn") {
		t.Fatalf("expected testOnlyFn: %v", lines)
	}
	if linesContainAny(lines, "moduleUsed") {
		t.Fatalf("moduleUsed has production refs: %v", lines)
	}
}

func TestRunProjectSectionsRespectsBudget(t *testing.T) {
	db, _ := testdb.MiniCodebase()
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
		t.Fatalf("budget exceeded: %d rows", total)
	}
	if len(secs) >= 9 {
		t.Fatalf("expected early stop, got %d sections", len(secs))
	}
}

func TestRunProjectSectionsHighPriorityOnly(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	p := map[Priority]bool{PriorityHigh: true}
	secs, err := RunProjectSections(db, 500, false, "", p, NewRowBudget(500))
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) != 4 {
		t.Fatalf("expected 4 high sections, got %d", len(secs))
	}
	for _, s := range secs {
		if !strings.Contains(s.Title, "[high]") {
			t.Fatalf("non-high section: %s", s.Title)
		}
	}
}

func TestChangeSurfaceListsExports(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	lines, err := changeSurface(db, "src/lib.ts", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !linesContainAny(lines, "foo") {
		t.Fatalf("expected foo: %v", lines)
	}
}

func TestUnusedImportsFindsNeverUsed(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	lines, err := unusedImportsForFile(db, "src/importer.ts", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !linesContainAny(lines, "deadFn") {
		t.Fatalf("expected deadFn import: %v", lines)
	}
}

func TestFileConsumersListsConsumer(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	lines, err := fileConsumers(db, "src/lib.ts", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !linesContainAny(lines, "consumer.ts") {
		t.Fatalf("expected consumer.ts: %v", lines)
	}
}

func TestResolveAnalyzeTargetDirectory(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	resolved, err := ResolveAnalyzeTarget(db, "src/cycle", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Kind != KindDir || resolved.Scope != "src/cycle" {
		t.Fatalf("got %+v", resolved)
	}
	files, err := ListDirFiles(db, "src/cycle", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0] != "src/cycle/a.ts" {
		t.Fatalf("files: %v", files)
	}
}

func TestResolveAnalyzeTargetFile(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	resolved, err := ResolveAnalyzeTarget(db, "src/lib.ts", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Kind != KindFile || resolved.Scope != "src/lib.ts" {
		t.Fatalf("got %+v", resolved)
	}
}

func TestResolveAnalyzeTargetSymbol(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	resolved, err := ResolveAnalyzeTarget(db, "foo", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Kind != KindSymbol || resolved.SymbolName != "foo" {
		t.Fatalf("got %+v", resolved)
	}
}

func TestSymbolConsumerFiles(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	fooID, err := testdb.SymbolID(db, "foo")
	if err != nil {
		t.Fatal(err)
	}
	lines, err := consumerFiles(db, fooID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !linesContainAny(lines, "consumer.ts") {
		t.Fatalf("expected consumer: %v", lines)
	}
}

func TestSymbolPressureMetrics(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	fooID, err := testdb.SymbolID(db, "foo")
	if err != nil {
		t.Fatal(err)
	}
	lines, err := symbolPressure(db, fooID)
	if err != nil {
		t.Fatal(err)
	}
	if !linesContainAny(lines, "fan_in=") {
		t.Fatalf("expected metrics: %v", lines)
	}
}

func TestSymbolRunAllFiveSections(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	fooID, err := testdb.SymbolID(db, "foo")
	if err != nil {
		t.Fatal(err)
	}
	secs, err := RunSymbolSections(db, fooID, 500, nil, NewRowBudget(500))
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) != 5 {
		t.Fatalf("expected 5 sections, got %d", len(secs))
	}
}

func TestHotspotsScopedToDirectory(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	allLines, err := hotspots(db, 20, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := hotspots(db, 20, CheckOptions{Scope: "src/cycle"})
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped) == 0 {
		t.Fatal("expected scoped hotspots")
	}
	for _, line := range scoped {
		if !strings.Contains(line, "cycle/") {
			t.Fatalf("scoped hotspot leaked: %q", line)
		}
	}
	if len(scoped) > len(allLines) {
		t.Fatalf("scoped=%d all=%d", len(scoped), len(allLines))
	}
}

func TestUnreferencedFindsOrphan(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	lines, err := unreferencedSymbols(db, 20, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !linesContainAny(lines, "Orphan") {
		t.Fatalf("expected Orphan: %v", lines)
	}
	if linesContainAny(lines, "foo") {
		t.Fatalf("foo has refs: %v", lines)
	}
}

func TestRunProjectSectionsNineSections(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	secs, err := RunProjectSections(db, 500, false, "", nil, NewRowBudget(500))
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) != 9 {
		t.Fatalf("expected 9 sections, got %d", len(secs))
	}
	high, low := 0, 0
	for _, s := range secs {
		if strings.Contains(s.Title, "[high]") {
			high++
		}
		if strings.Contains(s.Title, "[low]") {
			low++
		}
	}
	if high != 4 || low != 4 {
		t.Fatalf("priority counts high=%d low=%d", high, low)
	}
	if !strings.Contains(secs[0].Title, "Cycles") {
		t.Fatalf("first section: %s", secs[0].Title)
	}
}

func TestDeadExportsPrefaceWhenHits(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	secs, err := RunProjectSections(db, 20, false, "", nil, NewRowBudget(20))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range secs {
		if !strings.Contains(s.Title, "Dead exports") {
			continue
		}
		if len(s.Lines) > 0 && s.Lines[0] != "(none)" {
			if s.Preface == "" || !strings.Contains(s.Preface, "rdeps") {
				t.Fatalf("preface=%q lines=%v", s.Preface, s.Lines)
			}
		}
		return
	}
	t.Fatal("dead exports section not found")
}

func TestSameFileHelperNotInDeadExports(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	dead, err := deadExports(db, 20, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if linesContainAny(dead, "sameFileHelper") {
		t.Fatalf("sameFileHelper in dead: %v", dead)
	}
	same, err := sameFileOnly(db, 20, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !linesContainAny(same, "sameFileHelper") {
		t.Fatalf("expected sameFileHelper: %v", same)
	}
}

func TestSameFileOnlySkipsNoImporters(t *testing.T) {
	b, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	handler := b.Define("src/rules/handler.ts", "onEvent", 0, 10)
	b.Reference("src/rules/handler.ts", handler)
	db := b.Finish()
	lines, err := sameFileOnly(db, 20, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if linesContainAny(lines, "onEvent") {
		t.Fatalf("onEvent should be skipped: %v", lines)
	}
}

func TestCyclesTypeOnlyIgnored(t *testing.T) {
	b, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	tA := b.DefineType("src/types/a.ts", "AType", 0, 10)
	tB := b.DefineType("src/types/b.ts", "BType", 0, 10)
	b.Reference("src/types/a.ts", tB)
	b.Reference("src/types/b.ts", tA)
	lines, err := cycles(b.Finish(), 10, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		if strings.Contains(line, "types/a.ts") && strings.Contains(line, "types/b.ts") {
			t.Fatalf("type-only cycle should be ignored: %q", line)
		}
	}
}

func TestCyclesKeepsRuntimeMutualImports(t *testing.T) {
	b, err := testdb.New()
	if err != nil {
		t.Fatal(err)
	}
	symB := b.Define("src/runtime/b.ts", "runB", 0, 10)
	symA := b.Define("src/runtime/a.ts", "runA", 0, 10)
	b.Reference("src/runtime/a.ts", symB)
	b.Reference("src/runtime/b.ts", symA)
	lines, err := cycles(b.Finish(), 10, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range lines {
		if strings.Contains(line, "runtime/a.ts") && strings.Contains(line, "runtime/b.ts") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected runtime cycle: %v", lines)
	}
}

func TestRunFileSectionsIncludesCoupling(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	secs, err := RunFileSections(db, "src/lib.ts", 500, nil, NewRowBudget(500))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range secs {
		if strings.Contains(s.Title, "Coupling partners") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected coupling section")
	}
}

func TestDefContext(t *testing.T) {
	db, _ := testdb.MiniCodebase()
	fooID, err := testdb.SymbolID(db, "foo")
	if err != nil {
		t.Fatal(err)
	}
	lines, err := defContext(db, fooID)
	if err != nil {
		t.Fatal(err)
	}
	if !linesContainAny(lines, "kind=function") {
		t.Fatalf("expected kind=function: %v", lines)
	}
}
