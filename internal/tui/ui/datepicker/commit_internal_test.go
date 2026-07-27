package datepicker

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/tui/theme"
)

// TestCommit_ClampBranches covers the unreached clamping branches in commit():
// y<1, mo<1, mo>12, d<1, d>daysInMonth.
func TestCommit_ClampBranches(t *testing.T) {
	pal := theme.Default
	// Use a fixed valid seed date for New; we overwrite y/mo/d directly after.
	seed := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name                  string
		y, mo, d              int
		wantY, wantMo, wantD  int
	}{
		{"year zero clamped to 1", 0, 6, 15, 1, 6, 15},
		{"month zero clamped to 1", 2026, 0, 15, 2026, 1, 15},
		{"month 13 clamped to 12", 2026, 13, 15, 2026, 12, 15},
		{"day zero clamped to 1", 2026, 6, 0, 2026, 6, 1},
		{"day 32 in June clamped to 30", 2026, 6, 32, 2026, 6, 30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(seed, pal)
			// Directly set invalid raw values to exercise the clamping in commit().
			m.y, m.mo, m.d = tc.y, tc.mo, tc.d
			m.commit()
			if m.y != tc.wantY || m.mo != tc.wantMo || m.d != tc.wantD {
				t.Errorf("commit() with y=%d mo=%d d=%d → got y=%d mo=%d d=%d, want y=%d mo=%d d=%d",
					tc.y, tc.mo, tc.d,
					m.y, m.mo, m.d,
					tc.wantY, tc.wantMo, tc.wantD)
			}
		})
	}
}
