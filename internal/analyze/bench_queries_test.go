//go:build bench

package analyze

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/flesler/scip-cli-go/v2/internal/queries"
)

func benchQuery(name string, runs int, warmup int, fn func()) time.Duration {
	for i := 0; i < warmup; i++ {
		fn()
	}
	times := make([]time.Duration, runs)
	for i := 0; i < runs; i++ {
		start := time.Now()
		fn()
		times[i] = time.Since(start)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	median := times[len(times)/2]
	fmt.Printf("BENCH:%s:%.2f\n", name, float64(median.Microseconds())/1000.0)
	return median
}

func TestQueryBenchmarks(t *testing.T) {
	db, err := ScaledBenchDB(42)
	if err != nil {
		t.Fatal(err)
	}

	benchQuery("resolve_symbol_bare", 3, 1, func() {
		_, _ = queries.ResolveSymbol(db, "func0", nil, intPtr(10), "")
	})
	benchQuery("resolve_symbol_qualified", 3, 1, func() {
		_, _ = queries.ResolveSymbol(db, "Class0.method0", nil, intPtr(10), "")
	})
	benchQuery("resolve_file_exact", 3, 1, func() {
		_, _ = queries.ResolveFile(db, "src/module00/file000.ts", "")
	})
	benchQuery("resolve_file_basename", 3, 1, func() {
		_, _ = queries.ResolveFile(db, "file000.ts", "")
	})
	benchQuery("resolve_file_fuzzy", 3, 1, func() {
		_, _ = queries.ResolveFile(db, "file000", "")
	})
	benchQuery("get_file_symbols", 3, 1, func() {
		_, _ = queries.GetFileSymbols(db, "src/module00/file000.ts", nil)
	})
	benchQuery("get_importer_paths", 3, 1, func() {
		symbols, err := queries.GetFileSymbols(db, "src/module00/file000.ts", nil)
		if err != nil || len(symbols) == 0 {
			return
		}
		_, _ = queries.GetImporterPaths(db, []int{symbols[0].ID}, "src/module00/file000.ts", nil)
	})
	benchQuery("analyze_hotspots", 3, 1, func() {
		_, _ = hotspots(db, 25, CheckOptions{})
	})
	benchQuery("analyze_bottlenecks", 3, 1, func() {
		_, _ = bottlenecks(db, 25, CheckOptions{})
	})
	benchQuery("analyze_cycles", 3, 1, func() {
		_, _ = cycles(db, 25, CheckOptions{})
	})
	benchQuery("analyze_dead_exports", 3, 1, func() {
		_, _ = deadExports(db, 25, CheckOptions{})
	})
	benchQuery("analyze_dead_exports_limit5", 3, 1, func() {
		_, _ = deadExports(db, 5, CheckOptions{})
	})
	benchQuery("analyze_dead_files", 3, 1, func() {
		_, _ = deadFiles(db, 25, CheckOptions{})
	})
	benchQuery("analyze_dead_files_limit5", 3, 1, func() {
		_, _ = deadFiles(db, 5, CheckOptions{})
	})
	benchQuery("analyze_stale_types", 3, 1, func() {
		_, _ = staleTypes(db, 25, CheckOptions{})
	})
	benchQuery("analyze_unreferenced", 3, 1, func() {
		_, _ = unreferencedSymbols(db, 25, CheckOptions{})
	})
	benchQuery("analyze_same_file_only", 3, 1, func() {
		_, _ = sameFileOnly(db, 25, CheckOptions{})
	})
	benchQuery("analyze_test_only_limit5", 3, 1, func() {
		_, _ = symbolsTestOnlyConsumers(db, 5, CheckOptions{})
	})
	benchQuery("analyze_top_coupling", 3, 1, func() {
		_, _ = topCoupling(db, 25, CheckOptions{})
	})
	benchQuery("analyze_run_all", 3, 1, func() {
		_, _ = RunProjectSections(db, 25, false, "", nil, NewRowBudget(25), nil, 0)
	})
	benchQuery("analyze_run_all_per_check_limit", 3, 1, func() {
		_, _ = RunProjectSections(db, 40, false, "", nil, NewRowBudget(40), nil, 5)
	})
	benchQuery("analyze_cycles_limit5", 3, 1, func() {
		_, _ = cycles(db, 5, CheckOptions{})
	})
}

func intPtr(v int) *int {
	return &v
}
