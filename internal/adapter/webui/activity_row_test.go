package webui

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestBuildActivityRows_TargetPill(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.Local)
	ref := func(s string) *string { return &s }
	entries := []domain.ActivityEntry{
		// 1: session booked to a live node → live name+kind win, label cleared, href set
		{ActorKind: "human", ActorRef: "msoent", Kind: "session.started",
			NodeRef: ref("n1"), Label: ref("old-snapshot"), At: now.Add(-5 * time.Minute)},
		// 2: session booked to a since-deleted node → label snapshot, no kind, no href
		{ActorKind: "human", ActorRef: "msoent", Kind: "session.stopped",
			NodeRef: ref("gone"), Label: ref("Altprojekt"), At: now.Add(-10 * time.Minute)},
		// 3: unbooked session → no pill
		{ActorKind: "human", ActorRef: "msoent", Kind: "session.started", At: now.Add(-1 * time.Minute)},
		// 4: document event with NodeRef must NOT grow a pill (documents keep their label link)
		{ActorKind: "agent", ActorRef: "claude", Kind: "document.updated",
			TargetRef: ref("d1"), Label: ref("Spec"), NodeRef: ref("n1"), At: now.Add(-2 * time.Minute)},
	}
	names := map[string]string{"n1": "flow"}
	kinds := map[string]domain.NodeKind{"n1": domain.KindRepo}

	rows := BuildActivityRows(entries, names, kinds, now)

	if r := rows[0]; r.TargetName != "flow" || r.TargetKind != domain.KindRepo || r.TargetHref != "/nodes/n1" || r.Label != "" {
		t.Errorf("live target row = %+v", r)
	}
	if r := rows[1]; r.TargetName != "Altprojekt" || r.TargetKind != domain.NodeKind("") || r.TargetHref != "" || r.Label != "" {
		t.Errorf("deleted-node row = %+v", r)
	}
	if r := rows[2]; r.TargetName != "" {
		t.Errorf("unbooked row must have no pill, got %+v", r)
	}
	if r := rows[3]; r.TargetName != "" || r.Label != "Spec" || r.Href != "/wissen/d1" {
		t.Errorf("document row must keep label link and grow no pill, got %+v", r)
	}
}
