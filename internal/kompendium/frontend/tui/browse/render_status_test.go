package browse

import (
	"fmt"
	"strings"
	"testing"

	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestRenderFooter_SearchModeUsesStringsHintSearchInput(t *testing.T) {
	t.Parallel()
	m := Model{mode: ModeSearch, styles: newBrowseStyles(theme.TokyonightNight)}
	out := m.renderFooter()
	if !strings.Contains(out, uistrings.HintSearchInput) {
		t.Errorf("renderFooter (Search): expected canonical HintSearchInput, got %q", out)
	}
}

func TestRenderFooter_ConfirmModeUsesStringsHintConfirm(t *testing.T) {
	t.Parallel()
	m := Model{mode: ModeConfirmDelete, styles: newBrowseStyles(theme.TokyonightNight)}
	out := m.renderFooter()
	if !strings.Contains(out, uistrings.HintConfirm) {
		t.Errorf("renderFooter (Confirm): expected canonical HintConfirm, got %q", out)
	}
}

func TestRenderFooter_AllDimNotFooterKeyHighlight(t *testing.T) {
	t.Parallel()
	m := Model{mode: ModeSearch, styles: newBrowseStyles(theme.TokyonightNight)}
	out := m.renderFooter()
	if strings.Contains(out, fmt.Sprintf("%v", theme.TokyonightNight.Sem().Active)) {
		t.Errorf("renderFooter: must not paint keys in Sem.Active (Skill all-dim)")
	}
}
