package browse

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/kompendium/domain"
)

// Skill §Glyph whitelist + Skill "one accent per row": die Selection-
// Affordance ist der Stripe (AccentBar in Accent+Bold). Ein zweiter ▶-Caret
// daneben war doppelt — und ▶ heißt semantisch "läuft", nicht "gewählt".
func TestRowStripeAndCaret_SelectedHasNoActiveGlyph(t *testing.T) {
	t.Parallel()
	s := newBrowseStyles(theme.TokyonightNight)
	stripe, caret := s.rowStripeAndCaret(true)
	combined := stripe + caret
	if strings.Contains(combined, glyphs.Active) {
		t.Errorf("selected stripe+caret: must not use glyphs.Active (▶); got %q", combined)
	}
	if !strings.Contains(combined, glyphs.AccentBar) {
		t.Errorf("selected: expected glyphs.AccentBar (▎) as selection marker; got %q", combined)
	}
}

// Skill §Layout-Stabilität: TÄGL./PROJ./FREI sind Type-Pillen in der
// Listen-Row und müssen pixelgleich (cell-gleich) breit sein, damit die
// Title-Spalte nicht beim Wechsel zwischen Daily/Project/Free um eine
// Zelle springt. Die Labels selbst sind alle 5 Zeichen ("TÄGL.", "PROJ.",
// "FREI ") + 2 Cells horizontales Padding = 7 Cells; die Assertion ist
// paarweise gleich (nicht eine feste Konstante), weil sich die Labels
// auch leicht verschieben dürfen, solange sie konsistent bleiben.
func TestBadgeFor_FixedWidthAcrossKinds(t *testing.T) {
	t.Parallel()
	s := newBrowseStyles(theme.TokyonightNight)
	tDaily := s.badgeFor(domain.TypeDaily)
	tProject := s.badgeFor(domain.TypeProject)
	tFree := s.badgeFor(domain.TypeFree)
	wDaily := lipgloss.Width(tDaily)
	wProject := lipgloss.Width(tProject)
	wFree := lipgloss.Width(tFree)
	if wDaily != wProject || wProject != wFree {
		t.Errorf("badges not equal width: Daily=%d Project=%d Free=%d",
			wDaily, wProject, wFree)
	}
}

// UX-Review §4.2: der alte Empty-State war ein 5-zeiliger Hero-Block
// (Glyph + Leerzeile + Titel + 2 Hinweis-Zeilen). Andere Surfaces
// (palette/projects/heute) halten ihre Empty-States auf einer Zeile
// — der 5-Zeiler stach raus. Reduktion auf Titel + eine Hinweiszeile
// macht den Browse-Empty-State konsistent.
func TestRenderEmptyState_AtMostTwoLines(t *testing.T) {
	t.Parallel()
	m := Model{visible: nil, styles: newBrowseStyles(theme.TokyonightNight)}
	out := m.renderEmptyState(60)
	lines := strings.Count(out, "\n") + 1
	if lines > 2 {
		t.Errorf("empty state: at most 2 lines, got %d (lines=%v)", lines, strings.Split(out, "\n"))
	}
}
