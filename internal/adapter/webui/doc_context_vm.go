package webui

import (
	"fmt"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// DocContextVM is the docrail "Im Agenten-Kontext" block (Task 6, Mockup
// Z.794-798): the answer to "is this document in the composed agent context,
// and where", PLUS the mode switcher (Task 4). Built by BuildDocContext from
// usecase.StandingOf's output and the doc's ContextMode — the caller only
// builds a DocContextVM at all for context-eligible doc types (buildDocumentVM's
// isContextType gate); once built, it is never nil, so the mode switcher stays
// reachable even when the doc currently composes to nothing ("absent") or is
// explicitly hidden ("nie" — StandingOf itself still reports "absent" for a
// nie-mode doc, since Compose never adds it to Instructions/ActiveContext/
// AlwaysMemories/Ranked; the template branches on Mode == nie BEFORE looking
// at State to render "ausgeblendet (nie)" instead of nothing).
type DocContextVM struct {
	State    string // "included" | "dropped" | "always" | "absent"
	NodeName string
	RankStr  string // "04 / 24", only set for "included"
	Included bool
	Mode     domain.ContextMode
}

// BuildDocContext reduces a usecase.ContextStanding (StandingOf's output),
// the doc's owning-node display name, and its ContextMode into the docrail
// block VM. Pure. Always returns a non-nil VM — the switcher must stay
// reachable in every state, including "absent".
func BuildDocContext(st usecase.ContextStanding, nodeName string, mode domain.ContextMode) *DocContextVM {
	vm := &DocContextVM{
		State:    st.State,
		NodeName: nodeName,
		Included: st.State == "included",
		Mode:     mode,
	}
	if st.State == "included" {
		vm.RankStr = fmt.Sprintf("%02d / %02d", st.Rank, st.Total)
	}
	return vm
}
