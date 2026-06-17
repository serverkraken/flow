package worktime

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// TEMPORARY stubs — replaced by dialogs.go in Task 6.
type bookingState struct {
	projects []domain.Project
}
type editState struct{}
type confirmState struct{}

func (r *TodayRoute) handleDialogKey(tea.KeyPressMsg) (shell.Route, tea.Cmd) { return r, nil }
func (r *TodayRoute) startOrStop() (shell.Route, tea.Cmd)                     { return r, nil }
func (r *TodayRoute) openEdit() (shell.Route, tea.Cmd)                        { return r, nil }
func (r *TodayRoute) openDelete() (shell.Route, tea.Cmd)                      { return r, nil }
func (r *TodayRoute) renderDialog(shell.Frame) string                        { return "" }
func (r *TodayRoute) dialogHints() []keyhint.Hint                            { return nil }
