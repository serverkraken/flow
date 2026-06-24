package projects

import (
	"testing"
)

// TestParseRateCents covers all branches of the internal parseRateCents
// function: blank → nil, integer, one-decimal, two-decimal, invalid input.
func TestParseRateCents(t *testing.T) {
	ptr := func(v int64) *int64 { return &v }

	cases := []struct {
		input   string
		want    *int64
		wantErr bool
	}{
		{"", nil, false},        // blank → nil, nil
		{"  ", nil, false},      // whitespace only → nil, nil
		{"90", ptr(9000), false},   // integer euros → 9000 cents
		{"90.5", ptr(9050), false}, // one decimal digit
		{"90.50", ptr(9050), false}, // two decimal digits
		{"90.509", ptr(9050), false}, // truncated after 2 decimal digits
		{"-5", nil, true},        // negative → error
		{"abc", nil, true},       // non-numeric → error
		{"9.ab", nil, true},      // invalid fractional → error
	}

	for _, tc := range cases {
		got, err := parseRateCents(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseRateCents(%q): expected error, got nil (result=%v)", tc.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseRateCents(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if tc.want == nil {
			if got != nil {
				t.Errorf("parseRateCents(%q) = %v, want nil", tc.input, *got)
			}
		} else {
			if got == nil {
				t.Errorf("parseRateCents(%q) = nil, want %d", tc.input, *tc.want)
			} else if *got != *tc.want {
				t.Errorf("parseRateCents(%q) = %d, want %d", tc.input, *got, *tc.want)
			}
		}
	}
}
