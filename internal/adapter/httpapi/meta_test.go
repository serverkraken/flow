package httpapi

import "testing"

func TestVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.2.3", "1.10.0", true},  // 2 < 10 in middle segment
		{"1.10.0", "1.2.3", false}, // 10 > 2
		{"1.0.0", "1.0.0", false},  // equal is not less
		{"1.0.0", "1.0.1", true},   // patch version
		{"1.0.1", "1.0.0", false},
		{"2.0.0", "1.99.99", false}, // major wins
		{"dev", "1.0.0", false},     // dev is never outdated
		{"dev", "99.99.99", false},  // dev is never outdated
		{"1.0", "1.0.0", false},     // short vs long: equal
		{"1.0", "1.0.1", true},      // short vs long: less
	}
	for _, tc := range cases {
		got := versionLess(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("versionLess(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
