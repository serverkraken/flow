package domain_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestValidBundesland(t *testing.T) {
	for _, ok := range []string{"NW", "nw", "BY", "DE", "NRW"} {
		if _, valid := domain.ValidBundesland(ok); !valid {
			t.Fatalf("%q should be valid", ok)
		}
	}
	if _, valid := domain.ValidBundesland("XX"); valid {
		t.Fatal("XX should be invalid")
	}
	if got, _ := domain.ValidBundesland("nrw"); got != "NW" {
		t.Fatalf("nrw should normalize to NW, got %q", got)
	}
}
