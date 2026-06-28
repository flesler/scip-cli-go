package scip

import (
	"strings"
	"testing"
)

func TestParseVersionTagged(t *testing.T) {
	v, ok := parseVersion("scip version v0.8.1")
	if !ok || v.major != 0 || v.minor != 8 || v.patch != 1 {
		t.Fatalf("got %+v ok=%v", v, ok)
	}
}

func TestParseVersionRejectsUnrelated(t *testing.T) {
	_, ok := parseVersion("SCIP Optimization Suite 8.0")
	if ok {
		t.Fatal("expected no match")
	}
}

func TestPlatformArchiveName(t *testing.T) {
	name, err := platformArchive()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "scip-") || !strings.HasSuffix(name, ".tar.gz") {
		t.Fatalf("unexpected archive name: %q", name)
	}
}
