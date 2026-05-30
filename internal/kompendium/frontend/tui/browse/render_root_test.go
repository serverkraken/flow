package browse

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// noteEntry builds a minimal NoteEntry whose only meaningful field is
// Meta.Type — that's all renderTypeCounts() inspects. Kept local to
// this file so the helper stays scoped to the type-count tests.
func noteEntry(t domain.NoteType) ports.NoteEntry {
	return ports.NoteEntry{Meta: domain.Frontmatter{Type: t}}
}

// TestRenderTypeCounts_GlyphsAreDistinctPerKind: Skill A11y-2 ("glyph
// + colour, never colour alone") demands that the three type buckets
// — täglich, projekt, frei — are distinguishable without colour. The
// previous implementation reused glyphs.Filled (●) for all three and
// only varied the foreground colour, which collapses to an identical
// row under NO_COLOR or colourblind palettes. After the fix each bucket
// carries its own whitelisted single-cell glyph (●, ◆, ○), so the
// shared Filled (●) appears at most once.
func TestRenderTypeCounts_GlyphsAreDistinctPerKind(t *testing.T) {
	t.Parallel()
	m := Model{
		styles:  newBrowseStyles(theme.TokyonightNight),
		all:     []ports.NoteEntry{noteEntry(domain.TypeDaily), noteEntry(domain.TypeProject), noteEntry(domain.TypeFree)},
		visible: []ports.NoteEntry{noteEntry(domain.TypeDaily), noteEntry(domain.TypeProject), noteEntry(domain.TypeFree)},
	}
	out := m.renderTypeCounts()
	gFilled := strings.Count(out, glyphs.Filled)
	if gFilled > 1 {
		t.Errorf("renderTypeCounts: glyphs.Filled used %d times — Skill A11y-2 wants distinct glyphs per kind", gFilled)
	}
}

// UX-Review §4.4 + §1.8: ohne aktiven Type-Filter (FilterAll) ist „Filter:"
// resp. „Typ:" semantisch leer und produziert den Drei-Doppelpunkt-Mismatch
// „Filter:  · Suche: …". Wenn der Type-Filter leer ist (label() == ""),
// muss die Status-Line das Label komplett weglassen.
func TestRenderStatusLine_HidesFilterLabelWhenEmpty(t *testing.T) {
	t.Parallel()
	m := Model{styles: newBrowseStyles(theme.TokyonightNight)} // zero-value Filter == FilterAll → label() ist "".
	out := m.renderStatusLine()
	if strings.Contains(out, "Filter:") {
		t.Errorf("statusLine with empty filter: must not render `Filter:` label, got %q", out)
	}
	if strings.Contains(out, "Typ:") {
		t.Errorf("statusLine with empty filter: must not render `Typ:` label, got %q", out)
	}
}
