package sqlhelp_test

import (
	"testing"

	"github.com/flesler/scip-cli-go/v2/internal/sqlhelp"
)

func TestEscapeLike(t *testing.T) {
	got := sqlhelp.EscapeLike(`50%_off\`)
	want := `50\%\_off\`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
