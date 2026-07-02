package webui

import (
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// TestHomeLogstream_TargetPill verifies that activity rows render the form-coded
// target pill for session activities: a linked pill with kind glyph for live nodes,
// an unlinked neutral pill for deleted nodes (without glyph), and a connector word
// (activity.on) between the verb and the pill.
func TestHomeLogstream_TargetPill(t *testing.T) {
	ctx := context.Background()
	vm := HomeVM{LogEntries: []ActivityRowVM{
		{ActorKind: "human", ActorRef: "msoent", VerbKey: "activity.verb.session.started",
			TargetName: "flow", TargetKind: domain.KindRepo, TargetHref: "/nodes/n1", RelTime: "vor 5 Min"},
		{ActorKind: "human", ActorRef: "msoent", VerbKey: "activity.verb.session.stopped",
			TargetName: "Altprojekt", RelTime: "vor 10 Min"}, // deleted node: pill without link
	}}
	body := renderToBuf(t, ctx, homeLogstreamInner(vm))

	if !strings.Contains(body, `href="/nodes/n1"`) || !strings.Contains(body, "flow") {
		t.Errorf("live pill must link the node, got: %s", body)
	}
	if !strings.Contains(body, "●") {
		t.Errorf("live pill must carry the repo form-glyph, got: %s", body)
	}
	if !strings.Contains(body, "auf") {
		t.Errorf("connector word missing, got: %s", body)
	}
	if !strings.Contains(body, "Altprojekt") || strings.Contains(body, `href="/nodes/"`) {
		t.Errorf("deleted-node pill must render unlinked, got: %s", body)
	}
}
