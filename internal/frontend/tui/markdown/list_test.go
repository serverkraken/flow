package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestRender_BulletListL1: a top-level bullet item gets the L1
// glyph (●). Asserts on the strip-ANSI form so the colour SGR is
// allowed to wrap the glyph.
func TestRender_BulletListL1(t *testing.T) {
	t.Parallel()
	out, err := Render("- alpha\n- beta\n", 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "● alpha") {
		t.Errorf("L1 bullet missing for first item:\n%s", plain)
	}
	if !strings.Contains(plain, "● beta") {
		t.Errorf("L1 bullet missing for second item:\n%s", plain)
	}
}

// TestRender_NestedListsUseDistinctGlyphs: L2 uses ○, L3 uses ◆.
// The hierarchy must be visually distinct so the reader can read
// nesting at a glance.
func TestRender_NestedListsUseDistinctGlyphs(t *testing.T) {
	t.Parallel()
	src := "- one\n  - two\n    - three\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	for _, want := range []string{"● one", "○ two", "◆ three"} {
		if !strings.Contains(plain, want) {
			t.Errorf("nested list missing glyph %q:\n%s", want, plain)
		}
	}
}

// TestRender_NestedListsIndentMonotonic: each deeper level indents
// further than its parent so the visual hierarchy is unambiguous.
func TestRender_NestedListsIndentMonotonic(t *testing.T) {
	t.Parallel()
	src := "- one\n  - two\n    - three\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	idxL1 := strings.Index(plain, "● one")
	idxL2 := strings.Index(plain, "○ two")
	idxL3 := strings.Index(plain, "◆ three")
	if min(min(idxL1, idxL2), idxL3) < 0 {
		t.Fatalf("missing glyphs in output:\n%s", plain)
	}
	col := func(idx int) int {
		// columns from start of line containing idx
		nl := strings.LastIndexByte(plain[:idx], '\n')
		return idx - nl - 1
	}
	c1, c2, c3 := col(idxL1), col(idxL2), col(idxL3)
	if c1 >= c2 || c2 >= c3 {
		t.Errorf("expected indent strictly increasing: L1=%d L2=%d L3=%d", c1, c2, c3)
	}
}

// TestRender_OrderedListShowsNumbers: 1., 2., 3. markers.
func TestRender_OrderedListShowsNumbers(t *testing.T) {
	t.Parallel()
	src := "1. first\n2. second\n3. third\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	for _, want := range []string{"1. first", "2. second", "3. third"} {
		if !strings.Contains(plain, want) {
			t.Errorf("ordered marker missing %q:\n%s", want, plain)
		}
	}
}

// TestRender_OrderedListHonoursStart: when the markdown starts at
// `5.`, subsequent items count up from there. CommonMark spec.
func TestRender_OrderedListHonoursStart(t *testing.T) {
	t.Parallel()
	src := "5. five\n6. six\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "5. five") || !strings.Contains(plain, "6. six") {
		t.Errorf("ordered start not honoured:\n%s", plain)
	}
}

// TestRender_TaskList_OpenAndDone: `[ ]` and `[x]` items render with
// ☐/☑ glyphs. Done items get a strike-through dim style on the body
// so the reader can scan completed work fast.
func TestRender_TaskList_OpenAndDone(t *testing.T) {
	t.Parallel()
	src := "- [ ] open task\n- [x] done task\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "☐ open task") {
		t.Errorf("open task glyph missing:\n%s", plain)
	}
	if !strings.Contains(plain, "☑") {
		t.Errorf("done task glyph missing:\n%s", plain)
	}
	if !strings.Contains(plain, "done task") {
		t.Errorf("done task body missing:\n%s", plain)
	}
}

// TestRender_TaskList_DoneTextHasStrikethroughSGR: completed tasks
// must carry the strikethrough SGR (9) so the visual cue is more
// than just a glyph.
func TestRender_TaskList_DoneTextHasStrikethroughSGR(t *testing.T) {
	t.Parallel()
	out, err := Render("- [x] finished\n", 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "\x1b[9m") && !strings.Contains(out, ";9m") {
		t.Errorf("done task body missing strikethrough SGR (9):\n%q", out)
	}
}
