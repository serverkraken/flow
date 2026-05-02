package domain_test

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func TestParseID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		want    domain.ID
		wantErr bool
	}{
		{"daily plain", "daily/2026-04-25", domain.ID("daily/2026-04-25"), false},
		{"daily with .md", "daily/2026-04-25.md", domain.ID("daily/2026-04-25"), false},
		{"deep project", "projects/serverkraken/dotfiles/2026-04-25", domain.ID("projects/serverkraken/dotfiles/2026-04-25"), false},
		{"free note", "notes/setup", domain.ID("notes/setup"), false},
		{"surrounding whitespace trimmed", "  daily/2026-04-25  ", domain.ID("daily/2026-04-25"), false},

		{"empty", "", "", true},
		{"whitespace only", "   ", "", true},
		{"only .md suffix", ".md", "", true},
		{"absolute path", "/daily/2026", "", true},
		{"parent traversal", "../escape", "", true},
		{"current dir reference prefix", "./relative", "", true},
		{"current dir only", ".", "", true},
		{"parent dir only", "..", "", true},
		{"double slash", "daily//foo", "", true},
		{"trailing slash", "daily/", "", true},
		{"embedded dot-segment", "daily/./foo", "", true},
		{"intra-path traversal", "x/y/../z", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.ParseID(tc.input)
			if tc.wantErr {
				if !errors.Is(err, domain.ErrInvalidID) {
					t.Fatalf("expected ErrInvalidID, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestID_Path(t *testing.T) {
	t.Parallel()
	id := domain.ID("daily/2026-04-25")
	if got, want := id.Path(), "daily/2026-04-25.md"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestID_String(t *testing.T) {
	t.Parallel()
	id := domain.ID("daily/2026-04-25")
	if got, want := id.String(), "daily/2026-04-25"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
