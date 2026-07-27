package webui_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

func TestNodeKindStyle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind      domain.NodeKind
		wantLabel string
		wantGlyph string
		wantTone  string
	}{
		{domain.KindEngagement, "node.kind.engagement", "◆", "accent"},
		{domain.KindVorhaben, "node.kind.vorhaben", "▲", "highlight"},
		{domain.KindRepo, "node.kind.repo", "●", "success"},
		{domain.KindBranch, "node.kind.branch", "·", "muted"},
	}
	for _, c := range cases {
		got := webui.NodeKindStyle(c.kind)
		if got.LabelKey != c.wantLabel || got.Glyph != c.wantGlyph || got.Tone != c.wantTone {
			t.Errorf("NodeKindStyle(%q) = %+v", c.kind, got)
		}
	}
}

func ptrStr(s string) *string { return &s }

// TestMoveTargetsForExported verifies that the exported MoveTargetsFor wrapper
// excludes the node and its subtree from the candidate parents list.
func TestMoveTargetsForExported(t *testing.T) {
	t.Parallel()
	eng1 := domain.Node{ID: "eng1", Kind: domain.KindEngagement, Name: "Privat"}
	eng2 := domain.Node{ID: "eng2", Kind: domain.KindEngagement, Name: "RTL"}
	vor1 := domain.Node{ID: "vor1", Kind: domain.KindVorhaben, Name: "Buch", ParentID: ptrStr("eng1")}
	repo := domain.Node{ID: "repo1", Kind: domain.KindRepo, Name: "flow", ParentID: ptrStr("vor1")}
	all := []domain.Node{eng1, eng2, vor1, repo}

	// Move targets for vor1: engagements only; repo is a descendant and must be excluded.
	targets := webui.MoveTargetsFor(all, vor1)
	ids := map[string]bool{}
	for _, n := range targets {
		ids[n.ID] = true
	}
	if !ids["eng1"] || !ids["eng2"] {
		t.Errorf("expected engagements as targets; got %v", ids)
	}
	if ids["vor1"] || ids["repo1"] {
		t.Errorf("vor1 and its descendant repo must be excluded; got %v", ids)
	}
}
