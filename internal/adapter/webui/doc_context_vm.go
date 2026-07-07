package webui

import (
	"fmt"

	"github.com/serverkraken/flow/internal/usecase"
)

// DocContextVM is the docrail "Im Agenten-Kontext" block (Task 6, Mockup
// Z.794-798): the answer to "is this document in the composed agent context,
// and where". Built by BuildDocContext from usecase.StandingOf's output —
// nil means no block at all (non-context-type doc, or a context-type doc not
// present in the composed chain).
type DocContextVM struct {
	State    string // "included" | "dropped" | "always"
	NodeName string
	RankStr  string // "04 / 24", only set for "included"
	Included bool
}

// BuildDocContext reduces a usecase.ContextStanding (StandingOf's output) plus
// the doc's owning-node display name into the docrail block VM. Pure. absent
// -> nil (no block rendered at all).
func BuildDocContext(st usecase.ContextStanding, nodeName string) *DocContextVM {
	if st.State == "absent" {
		return nil
	}
	vm := &DocContextVM{
		State:    st.State,
		NodeName: nodeName,
		Included: st.State == "included",
	}
	if st.State == "included" {
		vm.RankStr = fmt.Sprintf("%02d / %02d", st.Rank, st.Total)
	}
	return vm
}
