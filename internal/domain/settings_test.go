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

func TestValidBundesland_Aliases(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"NRW", "NW"},
		{"nrw", "NW"},
		{"BAYERN", "BY"},
		{"Bayern", "BY"},
		{"BAWÜ", "BW"},
		{"BAWUE", "BW"},
		{"BADEN-WÜRTTEMBERG", "BW"},
		{"BADEN-WUERTTEMBERG", "BW"},
	}
	for _, tc := range cases {
		got, ok := domain.ValidBundesland(tc.input)
		if !ok {
			t.Errorf("ValidBundesland(%q): want valid=true, got false", tc.input)
		}
		if got != tc.want {
			t.Errorf("ValidBundesland(%q): want %q, got %q", tc.input, tc.want, got)
		}
	}
}
