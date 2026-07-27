package webui

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestBuildRecentNodes_RunningFirstDistinctBookable(t *testing.T) {
	now := time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC)
	nodeA := domain.Node{ID: "a", Name: "github.com/x/alpha", Kind: domain.KindRepo}
	nodeB := domain.Node{ID: "b", Name: "github.com/x/beta", Kind: domain.KindRepo, LogoRef: "hash123"}
	nodes := []domain.Node{nodeA, nodeB}

	running := domain.WorkSession{ID: "s3", NodeID: ptr("a"), Start: now.Add(-41 * time.Minute)}
	stop1 := now.Add(-2 * time.Hour)
	older := domain.WorkSession{ID: "s2", NodeID: ptr("b"), Start: now.Add(-3 * time.Hour), Stop: &stop1}
	stop2 := now.Add(-25 * time.Hour)
	// An older, since-superseded session on the SAME node as the running one —
	// must be deduped (only the newest touch per node counts).
	dup := domain.WorkSession{ID: "s1", NodeID: ptr("a"), Start: now.Add(-26 * time.Hour), Stop: &stop2}

	rows := BuildRecentNodes([]domain.WorkSession{older, dup, running}, nodes, now, 5)

	if len(rows) != 2 {
		t.Fatalf("want 2 distinct bookable nodes, got %d: %+v", len(rows), rows)
	}
	if rows[0].ID != "a" || rows[0].LabelKey != "home.runningNow" {
		t.Errorf("running node must sort first with runningNow label, got %+v", rows[0])
	}
	if rows[0].Name != "alpha" || rows[0].FullPath != "github.com/x/alpha" {
		t.Errorf("short/full name mismatch: %+v", rows[0])
	}
	if rows[0].Initials == "" || rows[0].Tone == "" {
		t.Errorf("initials/tone must be derived, got %+v", rows[0])
	}
	if rows[1].ID != "b" || rows[1].LabelKey != "home.lastActive" {
		t.Errorf("stopped node must carry lastActive label, got %+v", rows[1])
	}
	if rows[0].LogoRef != "" {
		t.Errorf("node without a logo must carry empty LogoRef, got %+v", rows[0])
	}
	if rows[1].LogoRef != "hash123" {
		t.Errorf("LogoRef must flow through from domain.Node, got %+v", rows[1])
	}
}

func TestBuildRecentNodes_CapsAtN(t *testing.T) {
	now := time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC)
	var nodes []domain.Node
	var sessions []domain.WorkSession
	for i := 0; i < 8; i++ {
		id := string(rune('a' + i))
		nodes = append(nodes, domain.Node{ID: id, Name: "repo-" + id, Kind: domain.KindRepo})
		stop := now.Add(-time.Duration(i) * time.Hour)
		start := stop.Add(-time.Hour)
		sessions = append(sessions, domain.WorkSession{ID: "s" + id, NodeID: ptr(id), Start: start, Stop: &stop})
	}
	rows := BuildRecentNodes(sessions, nodes, now, 5)
	if len(rows) != 5 {
		t.Fatalf("want cap 5, got %d", len(rows))
	}
}

func TestBuildRecentNodes_SkipsUnbookableAndUnbound(t *testing.T) {
	now := time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC)
	branch := domain.Node{ID: "br", Name: "feature/x", Kind: domain.KindBranch}
	nodes := []domain.Node{branch}
	unbound := domain.WorkSession{ID: "s1", NodeID: nil, Start: now.Add(-time.Hour)}
	onBranch := domain.WorkSession{ID: "s2", NodeID: ptr("br"), Start: now.Add(-2 * time.Hour)}

	rows := BuildRecentNodes([]domain.WorkSession{unbound, onBranch}, nodes, now, 5)
	if len(rows) != 0 {
		t.Fatalf("want no rows (unbound + unbookable-kind), got %+v", rows)
	}
}
