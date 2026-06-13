package domain_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func TestRenderParseRoundtrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		note domain.Note
	}{
		{
			name: "daily note",
			note: mustRoundtripNote(t, domain.ID("daily/2026-04-25"), domain.Frontmatter{
				ID:   "daily/2026-04-25",
				Type: domain.TypeDaily,
				Date: "2026-04-25",
			}, []byte("# today\n\n- bullet\n")),
		},
		{
			name: "free note with tags",
			note: mustRoundtripNote(t, domain.ID("notes/setup"), domain.Frontmatter{
				ID:    "notes/setup",
				Type:  domain.TypeFree,
				Title: "Setup notes",
				Tags:  []string{"infra", "kompendium"},
			}, []byte("some body\n")),
		},
		{
			name: "project note",
			note: mustRoundtripNote(t, domain.ID("projects/github.com/foo/bar/2026-04-25"), domain.Frontmatter{
				ID:      "projects/github.com/foo/bar/2026-04-25",
				Type:    domain.TypeProject,
				Project: "github.com/foo/bar",
				Date:    "2026-04-25",
			}, []byte("## status\n\ndone\n")),
		},
		{
			name: "empty body",
			note: mustRoundtripNote(t, domain.ID("notes/empty"), domain.Frontmatter{
				ID:   "notes/empty",
				Type: domain.TypeFree,
			}, []byte{}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := domain.RenderNote(tc.note)
			if len(raw) == 0 {
				t.Fatal("RenderNote returned empty bytes")
			}
			got, err := domain.ParseNote(tc.note.ID, raw)
			if err != nil {
				t.Fatalf("ParseNote: %v", err)
			}
			if got.ID != tc.note.ID {
				t.Errorf("ID: got %q, want %q", got.ID, tc.note.ID)
			}
			if got.Meta.Type != tc.note.Meta.Type {
				t.Errorf("Type: got %q, want %q", got.Meta.Type, tc.note.Meta.Type)
			}
			if string(got.Body) != string(tc.note.Body) {
				t.Errorf("Body: got %q, want %q", got.Body, tc.note.Body)
			}
		})
	}
}

func mustRoundtripNote(t *testing.T, id domain.ID, meta domain.Frontmatter, body []byte) domain.Note {
	t.Helper()
	n, err := domain.NewNote(id, meta, body)
	if err != nil {
		t.Fatalf("NewNote: %v", err)
	}
	return n
}
