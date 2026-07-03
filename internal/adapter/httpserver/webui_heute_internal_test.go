package httpserver

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// TestHeuteTargetVariant covers all four branches of heuteTargetVariant:
// running, over (Saldo > 0), hit (Saldo==0 && Target>0), and under (default).
func TestHeuteTargetVariant(t *testing.T) {
	cases := []struct {
		name    string
		today   usecase.TodaySummary
		running bool
		want    string
	}{
		{
			name:    "running session",
			today:   usecase.TodaySummary{Saldo: 0, Target: 8 * time.Hour},
			running: true,
			want:    "running",
		},
		{
			name:    "over target",
			today:   usecase.TodaySummary{Saldo: 30 * time.Minute, Target: 8 * time.Hour},
			running: false,
			want:    "over",
		},
		{
			name:    "hit exactly",
			today:   usecase.TodaySummary{Saldo: 0, Target: 8 * time.Hour},
			running: false,
			want:    "hit",
		},
		{
			name:    "under target",
			today:   usecase.TodaySummary{Saldo: -30 * time.Minute, Target: 8 * time.Hour},
			running: false,
			want:    "under",
		},
	}
	for _, tc := range cases {
		got := heuteTargetVariant(tc.today, tc.running)
		if got != tc.want {
			t.Errorf("%s: heuteTargetVariant = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// newHeuteVMTestServer builds a minimal *Server sufficient to call
// heuteDataFor directly (no HTTP round-trip) — only the ports the builder
// touches (Clock, ListSessionsRange, ListNodes, GetRunningSession) are wired;
// Stats stays zero so the target/balance block degrades to empty, which the
// ledger tests below don't assert on.
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
