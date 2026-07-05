package webui

import (
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// TestHomeLogstream_TargetPill verifies that activity rows render the
// form-coded target pill for session activities: a linked, glyph-free
// .targetlink for live nodes (Spec §7 Farb-Gesetz — kinds stay neutral, color
// lives only in the avatar; Task 7 removed activityTargetPill's kind
// glyph/tone), an unlinked neutral pill for deleted nodes, and a connector
// word (activity.on) between the verb and the pill.
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
	if !strings.Contains(body, `class="targetlink"`) {
		t.Errorf("live pill must use the neutral .targetlink class, got: %s", body)
	}
	if strings.Contains(body, "●") || strings.Contains(body, "▲") {
		t.Errorf("target pill must be glyph-free (Farb-Gesetz), got: %s", body)
	}
	if !strings.Contains(body, "auf") {
		t.Errorf("connector word missing, got: %s", body)
	}
	if !strings.Contains(body, "Altprojekt") || strings.Contains(body, `href="/nodes/"`) {
		t.Errorf("deleted-node pill must render unlinked, got: %s", body)
	}
}
