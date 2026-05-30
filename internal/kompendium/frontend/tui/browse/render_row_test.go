package browse

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
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
