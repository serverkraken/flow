package daydetail

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestBuildRows(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	mk := func(id string, sh, sm, eh, em int) domain.WorkSession {
		s := day.Add(time.Duration(sh)*time.Hour + time.Duration(sm)*time.Minute)
		e := day.Add(time.Duration(eh)*time.Hour + time.Duration(em)*time.Minute)
		return domain.WorkSession{ID: id, Start: s, Stop: &e, Tags: []string{"t-" + id}}
	}
	rows := buildRows([]domain.WorkSession{mk("b", 13, 0, 14, 0), mk("a", 9, 0, 11, 0)}, day)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].ID != "a" {
		t.Fatalf("rows not sorted ascending by start: %+v", rows)
	}
	if rows[0].Dur != 2*time.Hour {
		t.Fatalf("row a duration = %v, want 2h", rows[0].Dur)
	}
}

func TestBuildRows_RunningSession(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	start := day.Add(9 * time.Hour)
	sessions := []domain.WorkSession{
		{ID: "running", Start: start, Stop: nil, Tags: []string{"work"}},
	}
	rows := buildRows(sessions, day)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if !rows[0].Running {
		t.Fatal("row should be marked Running when Stop is nil")
	}
	if rows[0].Dur != 0 {
		t.Fatalf("running row Dur = %v, want 0", rows[0].Dur)
	}
}

func TestBuildRows_TagPropagated(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	s := day.Add(9 * time.Hour)
	e := day.Add(10 * time.Hour)
	sessions := []domain.WorkSession{
		{ID: "s1", Start: s, Stop: &e, Tags: []string{"deep"}},
	}
	rows := buildRows(sessions, day)
	if len(rows) != 1 || len(rows[0].Tags) != 1 || rows[0].Tags[0] != "deep" {
		t.Fatalf("tags not propagated: %+v", rows)
	}
}

func TestBuildRows_ProjectIDPropagated(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	s := day.Add(9 * time.Hour)
	e := day.Add(10 * time.Hour)
	pid := "proj-1"
	sessions := []domain.WorkSession{
		{ID: "s1", Start: s, Stop: &e, NodeID: &pid},
	}
	rows := buildRows(sessions, day)
	if len(rows) != 1 || rows[0].Project != "proj-1" {
		t.Fatalf("project id not propagated: %+v", rows)
	}
}

func TestBuildRows_NotePropagated(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	s := day.Add(9 * time.Hour)
	e := day.Add(10 * time.Hour)
	sessions := []domain.WorkSession{
		{ID: "s1", Start: s, Stop: &e, Note: "meeting notes"},
	}
	rows := buildRows(sessions, day)
	if len(rows) != 1 || rows[0].Note != "meeting notes" {
		t.Fatalf("note not propagated: %+v", rows)
	}
}

// TestResolveProjectName covers all 3 branches of resolveProjectName.
func TestResolveProjectName(t *testing.T) {
	m := map[string]string{"p1": "Acme"}

	// Empty id → "".
	if got := resolveProjectName("", m); got != "" {
		t.Errorf("empty id: got %q, want %q", got, "")
	}
	// Found in map → display name.
	if got := resolveProjectName("p1", m); got != "Acme" {
		t.Errorf("found id: got %q, want %q", got, "Acme")
	}
	// Not found in map → fallback to raw id.
	if got := resolveProjectName("unknown-id", m); got != "unknown-id" {
		t.Errorf("unknown id: got %q, want %q", got, "unknown-id")
	}
}
