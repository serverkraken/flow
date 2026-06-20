package worktime

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestMruProjects_RecentFirstUnusedTrail(t *testing.T) {
	t.Parallel()
	projects := []domain.Project{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	t1 := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	pa, pc := "a", "c"
	stop2 := t2.Add(time.Hour)
	sessions := []domain.WorkSession{
		{ProjectID: &pa, Start: t1, Stop: ptr(t1.Add(time.Hour))}, // a used at ~t1
		{ProjectID: &pc, Start: t2, Stop: &stop2},                 // c used at ~t2 (more recent)
	}
	got := mruProjects(projects, sessions)
	ids := []string{got[0].ID, got[1].ID, got[2].ID}
	// c (most recent) > a (older) > b (unused, original order)
	if ids[0] != "c" || ids[1] != "a" || ids[2] != "b" {
		t.Errorf("mru order = %v, want [c a b]", ids)
	}
}

func TestMruProjects_RunningSessionUsesStart(t *testing.T) {
	t.Parallel()
	projects := []domain.Project{{ID: "a"}, {ID: "b"}}
	pb := "b"
	recent := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	sessions := []domain.WorkSession{{ProjectID: &pb, Start: recent}} // running, no Stop
	got := mruProjects(projects, sessions)
	if got[0].ID != "b" {
		t.Errorf("running session should rank b first, got %v", got[0].ID)
	}
}
