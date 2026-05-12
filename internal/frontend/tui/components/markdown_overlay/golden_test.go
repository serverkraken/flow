package markdown_overlay_test

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/serverkraken/flow/internal/frontend/tui/components/markdown_overlay"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files")

// TestGolden pins View() output for representative chrome scenarios.
// Run with -update to refresh golden files when chrome intentionally
// changes; commit the diff alongside the change.
func TestGolden(t *testing.T) {
	cases := []struct {
		name  string
		build func() markdown_overlay.Model
	}{
		{
			name: "small_body_no_scroll",
			build: func() markdown_overlay.Model {
				return markdown_overlay.New(
					func(_ string, _ int) string { return "single line" },
					markdown_overlay.WithTitle("Title"),
					markdown_overlay.WithSource("ignored"),
				).SetSize(40, 12)
			},
		},
		{
			name: "narrow_width",
			build: func() markdown_overlay.Model {
				return markdown_overlay.New(
					func(_ string, _ int) string { return "narrow" },
					markdown_overlay.WithTitle("X"),
					markdown_overlay.WithSource("x"),
				).SetSize(20, 8)
			},
		},
		{
			name: "search_active_with_matches",
			build: func() markdown_overlay.Model {
				render := func(_ string, _ int) string {
					return "alpha foo bar\nbeta foo qux\ngamma"
				}
				m := markdown_overlay.New(render,
					markdown_overlay.WithTitle("S"),
					markdown_overlay.WithSource("x"),
					markdown_overlay.WithSearch(),
				).SetSize(50, 12)
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
				for _, r := range "foo" {
					m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				}
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				return m
			},
		},
		{
			name: "search_no_matches",
			build: func() markdown_overlay.Model {
				render := func(_ string, _ int) string {
					return "alpha\nbeta\ngamma"
				}
				m := markdown_overlay.New(render,
					markdown_overlay.WithTitle("S"),
					markdown_overlay.WithSource("x"),
					markdown_overlay.WithSearch(),
				).SetSize(50, 12)
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
				for _, r := range "xyz" {
					m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				}
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				return m
			},
		},
		{
			name: "error_display",
			build: func() markdown_overlay.Model {
				return markdown_overlay.New(
					func(_ string, _ int) string { return "would-be body" },
					markdown_overlay.WithTitle("E"),
					markdown_overlay.WithSource("ignored"),
				).SetSize(50, 12).SetError(errors.New("could not load source"))
			},
		},
		{
			name: "long_title_truncates",
			build: func() markdown_overlay.Model {
				return markdown_overlay.New(
					func(_ string, _ int) string { return "body" },
					markdown_overlay.WithTitle("eine-sehr-lange-kompendium-note-die-in-einem-schmalen-pane-nicht-passt"),
					markdown_overlay.WithSource("x"),
				).SetSize(40, 10)
			},
		},
		{
			name: "code_copy_status",
			build: func() markdown_overlay.Model {
				body := "intro\n```sh\necho a\n```\n"
				m := markdown_overlay.New(
					func(s string, _ int) string { return s },
					markdown_overlay.WithTitle("C"),
					markdown_overlay.WithSource(body),
					markdown_overlay.WithCodeCopy(),
				).SetSize(50, 12)
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
				return m
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.build().View()
			path := filepath.Join("testdata", tc.name+".golden")
			if *updateGolden {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if got != string(want) {
				t.Errorf("golden mismatch for %s\nwant:\n%s\ngot:\n%s",
					tc.name, want, got)
			}
		})
	}
}
