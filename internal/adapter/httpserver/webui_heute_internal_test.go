package httpserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// newHeuteVMTestServer builds a minimal *Server sufficient to call
// heuteDataFor directly (no HTTP round-trip) — only the ports the builder
// touches (Clock, ListSessionsRange, ListNodes, GetRunningSession) are wired;
// Stats/ListSessions/ListDayOffs stay zero so the Wochenskala/Σ-line blocks
// degrade to empty, which the ledger tests below don't assert on.
func newHeuteVMTestServer(t *testing.T) (*Server, *testutil.FakeSessionStore, *testutil.FakeNodeStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 21, 12, 0, 0, 0, time.Local)}
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	s := &Server{
		Clock:             clk,
		ListSessionsRange: usecase.ListSessionsRange{Sessions: ss},
		ListNodes:         usecase.ListNodes{Nodes: ps},
		GetRunningSession: usecase.GetRunningSession{Sessions: ss},
	}
	return s, ss, ps
}

// seedHeuteNode inserts a bookable node directly so tests can reference a
// fixed id like "n1" (mirrors worktimeTestServer.seedNode in the sibling
// httpserver_test package).
func seedHeuteNode(t *testing.T, ps *testutil.FakeNodeStore, id, name string) {
	t.Helper()
	if _, err := ps.Create(context.Background(), domain.Node{
		ID: id, OwnerID: "u1", Name: name, Slug: id, Kind: domain.KindEngagement,
	}); err != nil {
		t.Fatalf("seedHeuteNode: %v", err)
	}
}

// TestHeuteDataFor_LedgerCarriesEditPrefill is the RED→GREEN guard for the
// Heute ledger: each completed session's row must carry a fully pre-filled
// edit-mode SessionDialogVM (Task 4 opens it on block click).
func TestHeuteDataFor_LedgerCarriesEditPrefill(t *testing.T) {
	s, ss, ps := newHeuteVMTestServer(t)
	seedHeuteNode(t, ps, "n1", "flow")

	day := time.Date(2026, 6, 21, 0, 0, 0, 0, time.Local)
	from := day.Add(9 * time.Hour)
	to := day.Add(11 * time.Hour)
	nodeID := "n1"
	if _, err := ss.Create(context.Background(), domain.WorkSession{
		ID: "sess-1", OwnerID: "u1", NodeID: &nodeID, Start: from, Stop: &to,
		Tags: []string{"deep"}, Note: "note-x",
	}); err != nil {
		t.Fatalf("seed completed session: %v", err)
	}

	u := domain.User{ID: "u1", Username: "u1"}
	vm, err := s.heuteDataFor(context.Background(), u, "")
	if err != nil {
		t.Fatalf("heuteDataFor: %v", err)
	}
	if len(vm.Ledger) == 0 {
		t.Fatal("no ledger rows")
	}
	e := vm.Ledger[0].Edit
	if e.Mode != "edit" || e.SessionID == "" || e.From != "09:00" || e.To != "11:00" || e.NodeID != "n1" {
		t.Errorf("edit prefill wrong: %+v", e)
	}
	if e.Action != "/ui/worktime/edit" {
		t.Errorf("edit action = %q", e.Action)
	}
	if e.Target != "#content" {
		t.Errorf("edit target = %q", e.Target)
	}
	if e.DialogID != "edit-sess-1" {
		t.Errorf("edit dialog id = %q", e.DialogID)
	}
	if e.Tag != "deep" || e.Note != "note-x" {
		t.Errorf("edit tag/note wrong: tag=%q note=%q", e.Tag, e.Note)
	}
	if len(e.Nodes) != 1 || e.Nodes[0].ID != "n1" {
		t.Errorf("edit picker nodes wrong: %+v", e.Nodes)
	}
}

// TestHeuteDataFor_LedgerSkipsEditForRunningSession verifies a RUNNING
// session (Stop == nil) is not editable via the ledger dialog — its Edit VM
// stays the zero value (Mode "") so the template can skip rendering it.
func TestHeuteDataFor_LedgerSkipsEditForRunningSession(t *testing.T) {
	s, ss, ps := newHeuteVMTestServer(t)
	seedHeuteNode(t, ps, "n1", "flow")

	nodeID := "n1"
	start := time.Date(2026, 6, 21, 10, 0, 0, 0, time.Local)
	if _, err := ss.Create(context.Background(), domain.WorkSession{
		ID: "running-1", OwnerID: "u1", NodeID: &nodeID, Start: start,
	}); err != nil {
		t.Fatalf("seed running session: %v", err)
	}

	u := domain.User{ID: "u1", Username: "u1"}
	vm, err := s.heuteDataFor(context.Background(), u, "")
	if err != nil {
		t.Fatalf("heuteDataFor: %v", err)
	}
	if len(vm.Ledger) != 1 {
		t.Fatalf("want 1 ledger row, got %d", len(vm.Ledger))
	}
	if got := vm.Ledger[0].Edit.Mode; got != "" {
		t.Errorf("running session should have zero Edit VM, got Mode=%q", got)
	}
}

// TestZeitWeekDays_ScaleAndLabels is the unit-level RED→GREEN guard for the
// pure builder feeding the vertical Wochenskala: bar height is proportional
// to the week's own max logged (never a NaN/divide-by-zero on a zero-target
// weekend day), a zero-logged workday shows "—", a zero-logged day covered
// by a day-off (or a weekend with nothing logged) shows "frei", and today's
// label carries the "· heute" suffix.
func TestZeitWeekDays_ScaleAndLabels(t *testing.T) {
	now := time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local) // Thursday
	mon := time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local)
	week := make([]domain.WeekDay, 7)
	for i := range week {
		d := mon.AddDate(0, 0, i)
		week[i] = domain.WeekDay{Date: d, Target: 8 * time.Hour, IsToday: d.Equal(startOfDay(now))}
	}
	// Monday: 4h logged (half of the 8h target/scale) → has, ~50%.
	week[0].Logged = 4 * time.Hour
	// Tuesday: nothing logged, no day-off, a workday → "—".
	week[1].Logged = 0
	// Wednesday: nothing logged but covered by a day-off → "frei".
	week[2].Logged = 0
	// Thursday (today): 8h exactly on target/scale → 100%.
	week[3].Logged = 8 * time.Hour
	// Sat/Sun: zero target (weekend), nothing logged → "frei" via isWeekendTime.
	week[5].Target, week[6].Target = 0, 0

	off := map[string]domain.DayOff{
		week[2].Date.Format("2006-01-02"): {Date: week[2].Date, Kind: domain.KindVacation},
	}

	days := zeitWeekDays(context.Background(), week, now, off)
	if len(days) != 7 {
		t.Fatalf("want 7 days, got %d", len(days))
	}
	if !days[0].Has || days[0].Pct <= 0 || days[0].Pct >= 100 {
		t.Errorf("Monday (4h/8h): want Has + partial bar, got %+v", days[0])
	}
	if days[1].ValueStr != "—" {
		t.Errorf("Tuesday (0 logged, no day-off): want %q, got %q", "—", days[1].ValueStr)
	}
	if days[2].ValueStr != "frei" {
		t.Errorf("Wednesday (0 logged, day-off): want %q, got %q", "frei", days[2].ValueStr)
	}
	if !days[3].Today || !days[3].Has || days[3].Pct != 100 {
		t.Errorf("Thursday (today, 8h/8h): want Today+Has+100%%, got %+v", days[3])
	}
	if !strings.Contains(days[3].Label, "· heute") {
		t.Errorf("today's label missing '· heute' suffix, got %q", days[3].Label)
	}
	if days[5].ValueStr != "frei" || days[6].ValueStr != "frei" {
		t.Errorf("weekend with zero target/logged: want %q, got Sa=%q So=%q", "frei", days[5].ValueStr, days[6].ValueStr)
	}
}
