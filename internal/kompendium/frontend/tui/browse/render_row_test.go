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
	SetPalette(theme.TokyonightNight)
	stripe, caret := rowStripeAndCaret(true)
	combined := stripe + caret
	if strings.Contains(combined, glyphs.Active) {
		t.Errorf("selected stripe+caret: must not use glyphs.Active (▶); got %q", combined)
	}
	if !strings.Contains(combined, glyphs.AccentBar) {
		t.Errorf("selected: expected glyphs.AccentBar (▎) as selection marker; got %q", combined)
	}
}
