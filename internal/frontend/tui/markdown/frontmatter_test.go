package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestRender_FrontmatterCard_RendersAllFields: a fully-populated
// Frontmatter produces a card with the type badge, title, date,
// project chip, and tag chips visible in the strip-ANSI output.
func TestRender_FrontmatterCard_RendersAllFields(t *testing.T) {
	t.Parallel()
	fm := &Frontmatter{
		ID:      "projects/foo/_project",
		Type:    TypeProject,
		Project: "github.com/example/foo",
		Date:    "2026-04-25",
		Title:   "Foo",
		Tags:    []string{"go", "infra"},
	}
	out, err := Render("body", 80, WithFrontmatter(fm))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	for _, want := range []string{"PROJECT", "Foo", "2026-04-25", "github.com/example/foo", "go", "infra"} {
		if !strings.Contains(plain, want) {
			t.Errorf("card missing field %q:\n%s", want, plain)
		}
	}
}

// TestRender_FrontmatterCard_NilFrontmatterSkipsCard: passing nil
// frontmatter (or omitting WithFrontmatter) leaves the body alone —
// no separator line, no leading blank that would shift content down.
func TestRender_FrontmatterCard_NilFrontmatterSkipsCard(t *testing.T) {
	t.Parallel()
	out, err := Render("body", 80)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if strings.Contains(plain, "─────") {
		t.Errorf("nil frontmatter should not produce a card separator:\n%s", plain)
	}
}

// TestRender_FrontmatterCard_EmptyFieldsSkipsCard: a Frontmatter
// with all the human-visible fields blank (only e.g. Type set, no
// title/project/tags/id) doesn't render — the card would be a
// useless badge alone.
func TestRender_FrontmatterCard_EmptyFieldsSkipsCard(t *testing.T) {
	t.Parallel()
	fm := &Frontmatter{Type: TypeFree}
	out, err := Render("body", 80, WithFrontmatter(fm))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if strings.Contains(plain, "─────") {
		t.Errorf("empty frontmatter should skip the card separator:\n%s", plain)
	}
}

// TestRender_FrontmatterCard_FallsBackToIDWhenTitleEmpty: when the
// note's Title is blank the card uses the ID as the headline so the
// row isn't anonymously empty.
func TestRender_FrontmatterCard_FallsBackToIDWhenTitleEmpty(t *testing.T) {
	t.Parallel()
	fm := &Frontmatter{ID: "daily/2026-04-25", Type: TypeDaily}
	out, err := Render("body", 60, WithFrontmatter(fm))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "daily/2026-04-25") {
		t.Errorf("ID fallback missing in card:\n%s", plain)
	}
}

// TestRender_FrontmatterCard_DailyAndFreeBadges: TypeDaily and
// TypeFree each get a distinct badge so the user can scan the
// preview pane and distinguish note types at a glance.
func TestRender_FrontmatterCard_DailyAndFreeBadges(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		t    NoteType
		want string
	}{
		{TypeDaily, "DAILY"},
		{TypeFree, "FREE"},
		{TypeProject, "PROJECT"},
	} {
		fm := &Frontmatter{ID: "x", Title: "x", Type: tc.t}
		out, _ := Render("b", 60, WithFrontmatter(fm))
		if !strings.Contains(ansi.Strip(out), tc.want) {
			t.Errorf("type %q: badge %q missing", tc.t, tc.want)
		}
	}
}

// TestTagColorIdx_Deterministic: the same tag string always picks
// the same palette slot — visual consistency between the card and
// the browse-list tag chips.
func TestTagColorIdx_Deterministic(t *testing.T) {
	t.Parallel()
	first := tagColorIdx("go")
	second := tagColorIdx("go")
	if first != second {
		t.Errorf("tagColorIdx not deterministic: %d vs %d", first, second)
	}
	if tagColorIdx("go") == tagColorIdx("infra") {
		t.Skip("two tags happened to hash to the same slot — non-fatal")
	}
}
