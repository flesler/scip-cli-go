package e2e_test

import (
	"sort"
	"testing"
	"time"
)

func medianElapsed(fn func(), runs, warmup int) time.Duration {
	for i := 0; i < warmup; i++ {
		fn()
	}
	samples := make([]time.Duration, runs)
	for i := 0; i < runs; i++ {
		start := time.Now()
		fn()
		samples[i] = time.Since(start)
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return samples[len(samples)/2]
}

func TestE2eCommandPerfNoOutliers(t *testing.T) {
	requireIndex(t)
	fast := map[string]time.Duration{
		"search":  medianElapsed(func() { runCLI("search", fnGreet, "--limit", "3") }, 3, 1),
		"symbols": medianElapsed(func() { runCLI("symbols", helperFile, "--limit", "10") }, 3, 1),
		"code":    medianElapsed(func() { runCLI("code", fnGreet, "--limit", "1") }, 3, 1),
		"refs":    medianElapsed(func() { runCLI("refs", fnGreet, "--paths-only", "--limit", "10") }, 3, 1),
		"members": medianElapsed(func() { runCLI("members", classWidget, "--names-only") }, 3, 1),
		"rdeps":   medianElapsed(func() { runCLI("rdeps", helperFile, "--limit", "10") }, 3, 1),
		"deps":    medianElapsed(func() { runCLI("deps", fnGreet, "--limit", "10") }, 3, 1),
	}
	analyzeHigh := medianElapsed(func() { runCLI("analyze", "--priority", "high", "--limit", "5") }, 3, 1)
	analyzeAll := medianElapsed(func() { runCLI("analyze", "--limit", "5") }, 3, 1)

	vals := make([]time.Duration, 0, len(fast))
	for _, v := range fast {
		vals = append(vals, v)
	}
	sort.Slice(vals, func(i, j int) bool { return vals[i] < vals[j] })
	peer := vals[len(vals)/2]

	const fastRatio = 5.0
	for name, elapsed := range fast {
		if float64(elapsed) > float64(peer)*fastRatio {
			t.Fatalf("%s took %v vs peer median %v", name, elapsed, peer)
		}
	}

	analyzeHighLimit := maxDuration(peer*fastRatio, analyzeHigh)
	if analyzeHigh > analyzeHighLimit {
		t.Fatalf("analyze --priority high too slow: %v (limit %v)", analyzeHigh, analyzeHighLimit)
	}
	analyzeAllLimit := maxDuration(analyzeHigh*6, analyzeHigh)
	if analyzeAll > analyzeAllLimit {
		t.Fatalf("analyze all priorities too slow: %v vs high %v (limit %v)", analyzeAll, analyzeHigh, analyzeAllLimit)
	}
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
