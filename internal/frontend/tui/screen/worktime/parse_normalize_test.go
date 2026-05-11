package worktime

import "testing"

// TestNormalizeDurationArg covers the case-insensitive lowercase fix
// that drill-edit and today-edit must apply before calling the strict
// domain parser. The bug originally only existed in today-edit; this
// helper exists so the two sites cannot drift again.
func TestNormalizeDurationArg(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"+8h02m", "+8h02m"},
		{"+8H02M", "+8h02m"},
		{"+8H02m", "+8h02m"},
		{"+1H30M", "+1h30m"},
		// HH:MM passes through — the lowercase step targets the duration
		// form only; clock times are anchored differently downstream.
		{"23:30", "23:30"},
		{"7:05", "7:05"},
		// Edge: bare "+" and empty stay literal so the downstream parser
		// surfaces the canonical "leer" / "format" error.
		{"+", "+"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := normalizeDurationArg(tc.in); got != tc.want {
			t.Errorf("normalizeDurationArg(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
