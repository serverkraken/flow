package webui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/i18n"
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

func renderRow(t *testing.T, row ActivityRowVM, agentChip, onPrefix bool) string {
	t.Helper()
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	var b bytes.Buffer
	if err := activityFeedRow(row, agentChip, onPrefix).Render(ctx, &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

func TestActivityFeedRow_StacksActorAndTime(t *testing.T) {
	row := ActivityRowVM{ActorKind: "human", ActorRef: "msoent", VerbKey: "activity.verb.session.stopped", RelTime: "vor 1 Std", TargetName: "RTL Extern", TargetHref: "/nodes/x"}
	html := renderRow(t, row, false, true)
	// two-line structure: actor+time in the first flex row, detail in the second
	if strings.Count(html, "<div") < 2 {
		t.Fatalf("expected two stacked divs, got: %s", html)
	}
	// timestamp present, and NOT wrapped in a nowrap single-line container
	if !strings.Contains(html, "vor 1 Std") {
		t.Fatalf("reltime missing: %s", html)
	}
	// target pill contained (border pill), not a bare overflowing span
	if !strings.Contains(html, "RTL Extern") {
		t.Fatalf("target pill missing: %s", html)
	}
	// the detail row wraps (flex-wrap) so the pill can drop to a new line inside the card
	if !strings.Contains(html, "flex-wrap") {
		t.Fatalf("detail row must be flex-wrap: %s", html)
	}
}

// TestActivityFeedRow_TargetPill verifies that activityFeedRow renders the
// form-coded target pill for session activities: a linked, glyph-free
// .targetlink for live nodes (Spec §7 Farb-Gesetz — kinds stay neutral,
// color lives only in the avatar), an unlinked neutral pill for deleted
// nodes, and a connector word (activity.on) between the verb and the pill.
// Moved from home_render_test.go (L4 Task 2, which retired the Home
// logstream fragment activityFeedRow was only reachable through) onto the
// still-alive activityFeedRow/activityTargetPill directly, so the behavior
// stays pinned instead of being test-deleted with its dead caller.
func TestActivityFeedRow_TargetPill(t *testing.T) {
	live := ActivityRowVM{ActorKind: "human", ActorRef: "msoent", VerbKey: "activity.verb.session.started",
		TargetName: "flow", TargetKind: domain.KindRepo, TargetHref: "/nodes/n1", RelTime: "vor 5 Min"}
	deleted := ActivityRowVM{ActorKind: "human", ActorRef: "msoent", VerbKey: "activity.verb.session.stopped",
		TargetName: "Altprojekt", RelTime: "vor 10 Min"} // deleted node: pill without link

	liveHTML := renderRow(t, live, false, true)
	if !strings.Contains(liveHTML, `href="/nodes/n1"`) || !strings.Contains(liveHTML, "flow") {
		t.Errorf("live pill must link the node, got: %s", liveHTML)
	}
	if !strings.Contains(liveHTML, `class="targetlink"`) {
		t.Errorf("live pill must use the neutral .targetlink class, got: %s", liveHTML)
	}
	if strings.Contains(liveHTML, "●") || strings.Contains(liveHTML, "▲") {
		t.Errorf("target pill must be glyph-free (Farb-Gesetz), got: %s", liveHTML)
	}
	if !strings.Contains(liveHTML, "auf") {
		t.Errorf("connector word missing, got: %s", liveHTML)
	}

	deletedHTML := renderRow(t, deleted, false, true)
	if !strings.Contains(deletedHTML, "Altprojekt") || strings.Contains(deletedHTML, `href="/nodes/"`) {
		t.Errorf("deleted-node pill must render unlinked, got: %s", deletedHTML)
	}
}

func TestActivityFeedRow_AgentChipOnlyWhenAsked(t *testing.T) {
	agent := ActivityRowVM{ActorKind: "agent", ActorRef: "claude", VerbKey: "activity.verb.document.created", RelTime: "vor 2 Min"}
	// The AGENT chip is the only element carrying text-purple; assert on that
	// class rather than the localized chip text.
	withChip := renderRow(t, agent, true, false)
	without := renderRow(t, agent, false, false)
	if !strings.Contains(withChip, "text-purple") {
		t.Fatalf("agent chip expected when showAgentChip=true: %s", withChip)
	}
	if strings.Contains(without, "text-purple") {
		t.Fatalf("agent chip must be absent when showAgentChip=false: %s", without)
	}
}
