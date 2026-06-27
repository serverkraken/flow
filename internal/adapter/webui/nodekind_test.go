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
