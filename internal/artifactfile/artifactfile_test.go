package artifactfile

import "testing"

func TestGuessMime(t *testing.T) {
	cases := []struct{ name, path, override, want string }{
		{"override wins over extension", "logo.png", "application/x-thing", "application/x-thing"},
		{"png from extension", "/a/b/logo.png", "", "image/png"},
		{"pdf from extension", "doc.pdf", "", "application/pdf"},
		{"charset parameter stripped", "style.css", "", "text/css"},
		{"unknown extension falls back", "data.zzz", "", "application/octet-stream"},
		{"no extension falls back", "README", "", "application/octet-stream"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := GuessMime(c.path, c.override); got != c.want {
				t.Fatalf("GuessMime(%q, %q) = %q, want %q", c.path, c.override, got, c.want)
			}
		})
	}
}
