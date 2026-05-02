package domain_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestKind_LabelDe(t *testing.T) {
	tests := []struct {
		kind domain.Kind
		want string
	}{
		{domain.KindHoliday, "Feiertag"},
		{domain.KindVacation, "Urlaub"},
		{domain.KindSick, "Krank"},
		{domain.Kind("unknown"), "unknown"}, // unknown kinds fall through to the raw string
	}

	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			if got := tc.kind.LabelDe(); got != tc.want {
				t.Errorf("LabelDe() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseKind(t *testing.T) {
	tests := []struct {
		in     string
		want   domain.Kind
		wantOK bool
	}{
		// holiday aliases
		{"h", domain.KindHoliday, true},
		{"H", domain.KindHoliday, true},
		{"holiday", domain.KindHoliday, true},
		{"Feiertag", domain.KindHoliday, true},
		{"  feiertag  ", domain.KindHoliday, true},
		// vacation aliases
		{"v", domain.KindVacation, true},
		{"vacation", domain.KindVacation, true},
		{"urlaub", domain.KindVacation, true},
		// sick aliases
		{"s", domain.KindSick, true},
		{"sick", domain.KindSick, true},
		{"krank", domain.KindSick, true},
		{"krankheit", domain.KindSick, true},
		// unknown / empty
		{"", "", false},
		{"???", "", false},
		{"feier", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := domain.ParseKind(tc.in)
			if ok != tc.wantOK || got != tc.want {
				t.Errorf("ParseKind(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestAllKinds_CoversConstants(t *testing.T) {
	want := map[domain.Kind]bool{
		domain.KindHoliday:  false,
		domain.KindVacation: false,
		domain.KindSick:     false,
	}
	for _, k := range domain.AllKinds {
		if _, ok := want[k]; !ok {
			t.Errorf("AllKinds contains unexpected kind %q", k)
		}
		want[k] = true
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("AllKinds missing %q", k)
		}
	}
}
