package main

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/usecase"
)

func TestRenderContext_BodiesAndFooter(t *testing.T) {
	cc := usecase.ComposedContext{
		Instructions:  []usecase.ContextItem{{ScopeLabel: "repo:flow", Body: "RULE A"}},
		ActiveContext: &usecase.ContextItem{ScopeLabel: "repo:flow", Body: "where I was"},
		Memories: map[string][]usecase.ContextItem{
			"leaf": {{ScopeLabel: "repo:flow", Body: "leaf mem"}},
		},
	}
	cc.Budget.Used = 1200
	cc.Budget.Cap = 12000
	cc.Budget.Dropped.Leaf = 65
	cc.Budget.Dropped.Vorhaben = 1
	cc.Budget.Dropped.Engagement = 2
	cc.Budget.Dropped.Global = 3
	cc.Budget.Dropped.Pinned = 1
	out := renderContext(cc, false, "")
	for _, want := range []string{
		"RULE A", "where I was", "leaf mem", "1200/12000",
		"+65 leaf not shown", "+1 vorhaben not shown", "+2 engagement not shown", "+3 global not shown",
		"!! 1 pinned not shown",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q\n---\n%s", want, out)
		}
	}
}

// TestRenderContext_AlwaysMemories: an `immer` memory doc must reach the
// actual agent prompt, not just the server-side Used/JSON accounting
// (agy-Fund #1) — a non-empty AlwaysMemories renders its scope+body; an
// empty AlwaysMemories emits no section at all.
func TestRenderContext_AlwaysMemories(t *testing.T) {
	cc := usecase.ComposedContext{
		AlwaysMemories: []usecase.ContextItem{{ScopeLabel: "global", Body: "ALWAYS BODY"}},
	}
	out := renderContext(cc, false, "")
	if !strings.Contains(out, "ALWAYS BODY") {
		t.Errorf("render must include the AlwaysMemories body:\n%s", out)
	}

	empty := usecase.ComposedContext{}
	out2 := renderContext(empty, false, "")
	if strings.Contains(out2, "Always") {
		t.Errorf("empty AlwaysMemories must emit no Always section:\n%s", out2)
	}
}

func TestRenderContext_UnboundHintAndOffline(t *testing.T) {
	cc := usecase.ComposedContext{}
	cc.Resolution.Unresolved = true
	out := renderContext(cc, true, "2026-06-28T10:00:00Z")
	if !strings.Contains(out, "flow node bind") {
		t.Errorf("unbound render must hint at `flow node bind`:\n%s", out)
	}
	if !strings.Contains(out, "flow_set_active_context") {
		t.Errorf("nil active-context render must hint at `flow_set_active_context`:\n%s", out)
	}
	if !strings.Contains(out, "offline") || !strings.Contains(out, "2026-06-28T10:00:00Z") {
		t.Errorf("offline render must carry the stale marker:\n%s", out)
	}
}
