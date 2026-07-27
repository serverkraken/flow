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
		{domain.KindFlex, "Gleittag"},
		{domain.KindSpecial, "Sonderurlaub"},
		{domain.KindChildSick, "Kind krank"},
		{domain.KindTraining, "Fortbildung"},
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
		// flex aliases
		{"flex", domain.KindFlex, true},
		{"gleittag", domain.KindFlex, true},
		{"Gleit", domain.KindFlex, true},
		// special aliases
		{"special", domain.KindSpecial, true},
		{"sonderurlaub", domain.KindSpecial, true},
		// child-sick aliases
		{"childsick", domain.KindChildSick, true},
		{"kindkrank", domain.KindChildSick, true},
		{"Kind krank", domain.KindChildSick, true},
		// training aliases
		{"training", domain.KindTraining, true},
		{"fortbildung", domain.KindTraining, true},
		{"schulung", domain.KindTraining, true},
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
		domain.KindHoliday:   false,
		domain.KindVacation:  false,
		domain.KindSick:      false,
		domain.KindFlex:      false,
		domain.KindSpecial:   false,
		domain.KindChildSick: false,
		domain.KindTraining:  false,
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

func TestSelectableKinds_ExcludesHolidayCoversManual(t *testing.T) {
	want := map[domain.Kind]bool{
		domain.KindVacation:  false,
		domain.KindSick:      false,
		domain.KindFlex:      false,
		domain.KindSpecial:   false,
		domain.KindChildSick: false,
		domain.KindTraining:  false,
	}
	for _, k := range domain.SelectableKinds {
		if k == domain.KindHoliday {
			t.Fatal("SelectableKinds must not contain KindHoliday (computed, not manual)")
		}
		if _, ok := want[k]; !ok {
			t.Errorf("SelectableKinds contains unexpected kind %q", k)
		}
		want[k] = true
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("SelectableKinds missing %q", k)
		}
	}
}
