package browse

import (
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestHeaderSeparator_UsesSemBorderNotBgChip(t *testing.T) {
	t.Parallel()
	s := newBrowseStyles(theme.TokyonightNight)
	got := s.headerSeparator.GetForeground()
	want := theme.TokyonightNight.Sem().Border
	if got != want {
		t.Errorf("headerSeparator fg = %v, want %v (Sem.Border)", got, want)
	}
}

func TestErrorStyle_NotBold(t *testing.T) {
	t.Parallel()
	// Skill §Builder catalog: Err = Red, "no Bold; not a label".
	// errorPara wird als Paragraph-Surface verwendet ("Fehler beim
	// Bearbeiten: ..."), nicht als Pille — darf nicht bold sein.
	s := newBrowseStyles(theme.TokyonightNight)
	if s.errorPara.GetBold() {
		t.Error("errorPara: must not be Bold (Skill §Builder catalog: Err is paragraph, not label)")
	}
}
