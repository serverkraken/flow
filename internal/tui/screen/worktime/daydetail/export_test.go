package daydetail

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
)

// DialogOpen reports whether any modal (Nachbuchen add, edit, or delete confirm)
// is currently open. It exists for the external _test package to assert that a
// save error keeps the dialog open (and populated) so the user never loses typed
// input, and that a successful save / cancel closes it.
func (r *Route) DialogOpen() bool { return r.nachb != nil || r.edit != nil || r.del != nil }

// NachbuchenOpenForTest reports whether the Nachbuchen (add) dialog specifically
// is open. Used by the external _test package to assert the async project-load
// race guard does not open Nachbuchen over an already-open dialog.
func (r *Route) NachbuchenOpenForTest() bool { return r.nachb != nil }

// LateProjectsMsgForTest builds the (unexported) project-load message the async
// loadProjectsCmd produces, so the external _test package can deliver a "late"
// project load and assert it is ignored while another dialog is open.
func LateProjectsMsgForTest(ps []domain.Node) tea.Msg {
	return nachbuchenLoadProjectsMsg{projects: ps}
}
