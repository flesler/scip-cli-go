package targets

import "testing"

func TestLooksLikeFileTarget(t *testing.T) {
	if LooksLikeFileTarget("Transcriber.getMatch") {
		t.Fatal("qualified symbol should not be file")
	}
	if LooksLikeFileTarget("Widget.run") {
		t.Fatal("method should not be file")
	}
	for _, path := range []string{"helper.ts", "index.tsx", "module.py"} {
		if !LooksLikeFileTarget(path) {
			t.Fatalf("%q should be file target", path)
		}
	}
	if !LooksLikeFileTarget("src/util/transcriber/index.ts") {
		t.Fatal("path should be file target")
	}
}
