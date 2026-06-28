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
	cc.Budget.Cap = 6000
	cc.Budget.Dropped.Engagement = 2
	out := renderContext(cc, false, "")
	for _, want := range []string{"RULE A", "where I was", "leaf mem", "1200/6000", "+2 engagement"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderContext_UnboundHintAndOffline(t *testing.T) {
	cc := usecase.ComposedContext{}
	cc.Resolution.Unresolved = true
	out := renderContext(cc, true, "2026-06-28T10:00:00Z")
	if !strings.Contains(out, "flow node bind") {
		t.Errorf("unbound render must hint at `flow node bind`:\n%s", out)
	}
	if !strings.Contains(out, "offline") || !strings.Contains(out, "2026-06-28T10:00:00Z") {
		t.Errorf("offline render must carry the stale marker:\n%s", out)
	}
}
