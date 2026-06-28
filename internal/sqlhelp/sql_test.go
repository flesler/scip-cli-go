package sqlhelp_test

import (
	"testing"

	"github.com/sourcegraph/scip-cli-go/internal/sqlhelp"
)

func TestEscapeLike(t *testing.T) {
	got := sqlhelp.EscapeLike(`50%_off\`)
	want := `50\%\_off\\`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
