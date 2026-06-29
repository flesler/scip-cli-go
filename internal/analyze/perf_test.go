package analyze

import (
	"sort"
	"testing"
	"time"

	"github.com/flesler/scip-cli-go/internal/analyze/testdb"
)

func medianElapsed(fn func(), runs, warmup int) time.Duration {
	for i := 0; i < warmup; i++ {
		fn()
	}
	var samples []time.Duration
	for i := 0; i < runs; i++ {
		start := time.Now()
		fn()
		samples = append(samples, time.Since(start))
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return samples[len(samples)/2]
}

func TestProjectChecksNotDramaticallySlowerThanPeers(t *testing.T) {
	db, err := testdb.MiniCodebase()
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string]func(){
		"hotspots":     func() { _, _ = hotspots(db, 20, CheckOptions{}) },
		"coupling":     func() { _, _ = topCoupling(db, 20, CheckOptions{}) },
		"stale_types":  func() { _, _ = staleTypes(db, 20, CheckOptions{}) },
		"cycles":       func() { _, _ = cycles(db, 20, CheckOptions{}) },
		"dead_exports": func() { _, _ = deadExports(db, 20, CheckOptions{}) },
	}
	timings := make(map[string]time.Duration, len(checks))
	for name, fn := range checks {
		timings[name] = medianElapsed(fn, 5, 2)
	}
	baseline := []time.Duration{timings["hotspots"], timings["coupling"], timings["stale_types"], timings["dead_exports"]}
	sort.Slice(baseline, func(i, j int) bool { return baseline[i] < baseline[j] })
	peer := baseline[len(baseline)/2]
	const ratio = 25.0
	for name, elapsed := range timings {
		if name == "hotspots" || name == "coupling" || name == "stale_types" || name == "dead_exports" {
			if float64(elapsed) > float64(peer)*ratio {
				t.Fatalf("%s took %v vs peer median %v", name, elapsed, peer)
			}
		}
	}
}
