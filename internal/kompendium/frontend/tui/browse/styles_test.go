package browse

import (
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestHeaderSeparator_UsesSemBorderNotBgChip(t *testing.T) {
	SetPalette(theme.TokyonightNight)
	got := headerSeparatorStyle.GetForeground()
	want := theme.TokyonightNight.Sem().Border
	if got != want {
		t.Errorf("headerSeparator fg = %v, want %v (Sem.Border)", got, want)
	}
}

func TestErrorStyle_NotBold(t *testing.T) {
	SetPalette(theme.TokyonightNight)
	// Skill §Builder catalog: Err = Red, "no Bold; not a label".
	// errorStyle wird als Paragraph-Surface verwendet ("Fehler beim
	// Bearbeiten: ..."), nicht als Pille — darf nicht bold sein.
	if errorStyle.GetBold() {
		t.Error("errorStyle: must not be Bold (Skill §Builder catalog: Err is paragraph, not label)")
	}
}
