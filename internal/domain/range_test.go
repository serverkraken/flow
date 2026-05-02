package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestRange_ContainsDate(t *testing.T) {
	d := func(s string) time.Time {
		t.Helper()
		ts, err := time.ParseInLocation("2006-01-02", s, time.Local)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return ts
	}

	tests := []struct {
		name string
		r    domain.Range
		date time.Time
		want bool
	}{
		{"unbounded contains anything", domain.Range{}, d("2026-04-30"), true},
		{"only From, equal to", domain.Range{From: d("2026-04-01")}, d("2026-04-01"), true},
		{"only From, before", domain.Range{From: d("2026-04-01")}, d("2026-03-31"), false},
		{"only From, after", domain.Range{From: d("2026-04-01")}, d("2026-04-15"), true},
		{"only To, equal to (exclusive)", domain.Range{To: d("2026-04-30")}, d("2026-04-30"), false},
		{"only To, before", domain.Range{To: d("2026-04-30")}, d("2026-04-29"), true},
		{"only To, after", domain.Range{To: d("2026-04-30")}, d("2026-05-01"), false},
		{"both bounds, inside", domain.Range{From: d("2026-04-01"), To: d("2026-05-01")}, d("2026-04-15"), true},
		{"both bounds, before From", domain.Range{From: d("2026-04-01"), To: d("2026-05-01")}, d("2026-03-31"), false},
		{"both bounds, on To (exclusive)", domain.Range{From: d("2026-04-01"), To: d("2026-05-01")}, d("2026-05-01"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.r.ContainsDate(tc.date); got != tc.want {
				t.Errorf("ContainsDate(%s) = %v, want %v", tc.date.Format("2006-01-02"), got, tc.want)
			}
		})
	}
}
