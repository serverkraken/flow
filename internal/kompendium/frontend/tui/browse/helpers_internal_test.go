package browse

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

func TestSmartTitle_FallbacksByType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		e    ports.NoteEntry
		want string
	}{
		{
			name: "explicit title wins",
			e:    ports.NoteEntry{Meta: domain.Frontmatter{Title: "Explicit"}},
			want: "Explicit",
		},
		{
			name: "project with empty title shows last 2 url segments",
			e: ports.NoteEntry{
				ID:   "projects/github.com/serverkraken/dotfiles/2026-04-26",
				Meta: domain.Frontmatter{Type: domain.TypeProject, Project: "github.com/serverkraken/dotfiles"},
			},
			want: "serverkraken/dotfiles",
		},
		{
			name: "project with single-segment URL falls through",
			e: ports.NoteEntry{
				ID:   "projects/local/2026-04-26",
				Meta: domain.Frontmatter{Type: domain.TypeProject, Project: "local"},
			},
			want: "local",
		},
		{
			name: "project without project field falls back to ID",
			e: ports.NoteEntry{
				ID:   "projects/github.com/foo/bar/2026-04-26",
				Meta: domain.Frontmatter{Type: domain.TypeProject},
			},
			want: "projects/github.com/foo/bar/2026-04-26",
		},
		{
			name: "daily falls back to ID",
			e: ports.NoteEntry{
				ID:   "daily/2026-04-25",
				Meta: domain.Frontmatter{Type: domain.TypeDaily},
			},
			want: "daily/2026-04-25",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := smartTitle(tc.e); got != tc.want {
				t.Errorf("smartTitle got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTruncateText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"short", 10, "short"},
		{"exactly20charsworthhh", 20, "exactly20charsworth…"},
		{"longer than allowed", 5, "long…"},
		{"x", 1, "…"},
		{"", 5, ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := truncateText(tc.in, tc.max); got != tc.want {
				t.Errorf("truncateText(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
			}
		})
	}
}

func TestIsDateString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"2026-04-25", true},
		{"1999-12-31", true},
		{"abcd-ef-gh", false},
		{"2026/04/25", false},
		{"2026-04-2", false},   // 9 chars
		{"2026-04-255", false}, // 11 chars
		{"", false},
		{"hello", false},
		{"2026:04-25", false}, // wrong separator
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := isDateString(tc.in); got != tc.want {
				t.Errorf("isDateString(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestExcerptFor_SkipsRedundant covers the visual cleanup: a project
// note whose body is just the date (the on-disk template artifact)
// must produce no excerpt, otherwise the row renders the same date twice.
func TestExcerptFor_SkipsRedundant(t *testing.T) {
	t.Parallel()
	m := Model{
		bodies: map[domain.ID][]byte{
			"projects/foo/bar/2026-04-25": []byte("2026-04-25\n"),
			"daily/2026-04-25":            []byte("Real content here\n"),
			"projects/foo/2026-04-25":     []byte("github.com/foo\n"),
		},
	}
	cases := []struct {
		id   domain.ID
		meta domain.Frontmatter
		want string
	}{
		{
			id:   "projects/foo/bar/2026-04-25",
			meta: domain.Frontmatter{Type: domain.TypeProject, Date: "2026-04-25"},
			want: "",
		},
		{
			id:   "daily/2026-04-25",
			meta: domain.Frontmatter{Type: domain.TypeDaily, Date: "2026-04-25"},
			want: "Real content here",
		},
		{
			id:   "projects/foo/2026-04-25",
			meta: domain.Frontmatter{Type: domain.TypeProject, Project: "github.com/foo"},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.id), func(t *testing.T) {
			t.Parallel()
			e := ports.NoteEntry{ID: tc.id, Meta: tc.meta, Mtime: time.Unix(1, 0)}
			if got := m.excerptFor(e); got != tc.want {
				t.Errorf("excerptFor got %q, want %q", got, tc.want)
			}
		})
	}
}
