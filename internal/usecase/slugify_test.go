package usecase

import "testing"

func TestSlugify(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain ascii unchanged", "Flow Rebuild", "flow-rebuild"},
		{"eszett transliterates to ss", "straßenfuchs", "strassenfuchs"},
		{"umlauts transliterate", "Müller Änderung Öl", "mueller-aenderung-oel"},
		{"uppercase umlaut lowercased then mapped", "ÜBER", "ueber"},
		{"collapses and trims separators", "  hi -- there!  ", "hi-there"},
		{"digits kept", "v2 Plan", "v2-plan"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Slugify(c.in); got != c.want {
				t.Errorf("Slugify(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
