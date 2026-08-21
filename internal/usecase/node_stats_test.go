package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestNodeStats_UnknownNode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	ss := fakeSessionStore{}
	c := usecase.StatsComputer{
		Sessions: ss,
		Clock:    fixedClock{t: time.Now()},
		Loc:      time.UTC,
		Nodes:    ns,
	}
	_, err := c.NodeStats(ctx, "u1", "ghost")
	if !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("want ErrNodeNotFound, got %v", err)
	}
}

func TestNodeStats_OwnedNodeNoSessions_ZeroRollup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	_, _ = ns.Create(ctx, domain.Node{ID: "eng", OwnerID: "u1", Kind: domain.KindEngagement, Name: "eng", Slug: "eng", Status: domain.NodeActive})
	ss := fakeSessionStore{} // no sessions at all
	c := usecase.StatsComputer{
		Sessions: ss,
		Clock:    fixedClock{t: time.Now()},
		Loc:      time.UTC,
		Nodes:    ns,
	}
	r, err := c.NodeStats(ctx, "u1", "eng")
	if err != nil {
		t.Fatalf("want nil error for owned node with no sessions, got %v", err)
	}
	if r != (domain.NodeRollup{}) {
		t.Errorf("want zero NodeRollup, got %+v", r)
	}
}

func TestNodeStats_RollsUpSubtree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	eng := "eng"
	_, _ = ns.Create(ctx, domain.Node{ID: "eng", OwnerID: "u1", Kind: domain.KindEngagement, Name: "eng", Slug: "eng", Status: domain.NodeActive})
	_, _ = ns.Create(ctx, domain.Node{ID: "repo", OwnerID: "u1", ParentID: &eng, Kind: domain.KindRepo, Name: "repo", Slug: "repo", Status: domain.NodeActive})
	_, _ = ns.Create(ctx, domain.Node{ID: "other", OwnerID: "u1", Kind: domain.KindEngagement, Name: "other", Slug: "other", Status: domain.NodeActive})

	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC) // Wednesday

	repo := "repo"
	other := "other"
	stop1 := time.Date(2026, 6, 17, 11, 0, 0, 0, time.UTC)
	stop2 := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	stop3 := time.Date(2026, 6, 17, 13, 0, 0, 0, time.UTC)

	ss := fakeSessionStore{list: []domain.WorkSession{
		// repo session 2026-06-17 09:00–11:00 (2h, this week+month)
		{ID: "s1", OwnerID: "u1", NodeID: &repo, Start: time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC), Stop: &stop1},
		// repo session 2026-06-03 09:00–10:00 (1h, this month, not this week)
		{ID: "s2", OwnerID: "u1", NodeID: &repo, Start: time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC), Stop: &stop2},
		// other session 2026-06-17 09:00–13:00 (4h, must NOT count)
		{ID: "s3", OwnerID: "u1", NodeID: &other, Start: time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC), Stop: &stop3},
	}}

	c := usecase.StatsComputer{
		Sessions: ss,
		Clock:    fixedClock{t: now},
		Loc:      time.UTC,
		Nodes:    ns,
	}

	r, err := c.NodeStats(ctx, "u1", "eng")
	if err != nil {
		t.Fatalf("nodestats: %v", err)
	}
	if r.Total != 3*time.Hour {
		t.Errorf("Total = %v, want 3h", r.Total)
	}
	if r.Week != 2*time.Hour {
		t.Errorf("Week = %v, want 2h", r.Week)
	}
	if r.Month != 3*time.Hour {
		t.Errorf("Month = %v, want 3h", r.Month)
	}
}

func TestNodeStats_WorkPrivatSplit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	ss := testutil.NewFakeSessionStore()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 1, 12, 0, 0, 0, time.Local)}
	b := func(v bool) *bool { return &v }
	mk := func(id string, parent *string, ctt *bool) {
		n, _ := domain.NewNode(id, "u1", id, id, clk.Now())
		n.ParentID = parent
		n.CountsTowardTarget = ctt
		n.Kind = domain.KindRepo
		_, _ = ns.Create(ctx, n)
	}
	eng := "eng"
	mk(eng, nil, b(false)) // engagement explicitly Privat
	rp := "repo"
	mk(rp, &eng, nil) // repo inherits -> Privat
	rp2 := "repo2"
	mk(rp2, &eng, b(true)) // repo explicit Work (override)
	// 2h on the inherited-privat repo, 1h on the work-override repo. Booked as
	// sequential, non-overlapping windows ending at cursor (AddSession enforces
	// a global no-overlap invariant across all of the owner's nodes, so both
	// legs can't share the same [now-h, now) window), and sharing one IDGen so
	// the two sessions get distinct IDs (the fake session store is ID-keyed).
	ids := &testutil.FakeIDGen{}
	cursor := clk.Now()
	add := func(node string, h int) {
		stop := cursor
		st := stop.Add(time.Duration(-h) * time.Hour)
		_, err := usecase.AddSession{Sessions: ss, Nodes: ns, IDs: ids, Clock: clk}.
			Execute(ctx, "u1", &node, st, stop, nil, "")
		if err != nil {
			t.Fatalf("AddSession(%s, %dh): %v", node, h, err)
		}
		cursor = st
	}
	add(rp, 2)
	add(rp2, 1)

	sc := usecase.StatsComputer{Sessions: ss, Nodes: ns, Clock: clk, Loc: time.Local}
	r, err := sc.NodeStats(ctx, "u1", eng)
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 3*time.Hour {
		t.Errorf("Total=%v want 3h", r.Total)
	}
	if r.WorkTotal != 1*time.Hour {
		t.Errorf("WorkTotal=%v want 1h (only repo2 counts)", r.WorkTotal)
	}
	// Privat = Total - Work = 2h
}

func TestNodeStats_PrevWeek(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	ss := testutil.NewFakeSessionStore()
	// Wednesday, 2026-07-01 at 12:00 UTC
	// This week's Monday = 2026-06-29
	// Previous week's Monday = 2026-06-22
	// Two weeks ago Monday = 2026-06-15
	clk := testutil.FakeClock{T: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)}

	b := func(v bool) *bool { return &v }
	eng := "eng"
	n, _ := domain.NewNode(eng, "u1", eng, eng, clk.Now())
	n.Kind = domain.KindEngagement
	n.CountsTowardTarget = b(true) // Work
	_, _ = ns.Create(ctx, n)

	ids := &testutil.FakeIDGen{}

	// Session 1: 2026-06-29 09:00–11:00 (2h, this week)
	s1Stop := time.Date(2026, 6, 29, 11, 0, 0, 0, time.UTC)
	_, err := usecase.AddSession{Sessions: ss, Nodes: ns, IDs: ids, Clock: clk}.
		Execute(ctx, "u1", &eng, time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC), s1Stop, nil, "")
	if err != nil {
		t.Fatalf("AddSession (this week): %v", err)
	}

	// Session 2: 2026-06-22 09:00–12:00 (3h, previous week)
	s2Stop := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	_, err = usecase.AddSession{Sessions: ss, Nodes: ns, IDs: ids, Clock: clk}.
		Execute(ctx, "u1", &eng, time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC), s2Stop, nil, "")
	if err != nil {
		t.Fatalf("AddSession (prev week): %v", err)
	}

	// Session 3: 2026-06-15 09:00–10:00 (1h, two weeks ago)
	s3Stop := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	_, err = usecase.AddSession{Sessions: ss, Nodes: ns, IDs: ids, Clock: clk}.
		Execute(ctx, "u1", &eng, time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC), s3Stop, nil, "")
	if err != nil {
		t.Fatalf("AddSession (two weeks ago): %v", err)
	}

	sc := usecase.StatsComputer{Sessions: ss, Nodes: ns, Clock: clk, Loc: time.UTC}
	r, err := sc.NodeStats(ctx, "u1", eng)
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 6*time.Hour {
		t.Errorf("Total=%v want 6h", r.Total)
	}
	if r.Week != 2*time.Hour {
		t.Errorf("Week=%v want 2h", r.Week)
	}
	if r.PrevWeek != 3*time.Hour {
		t.Errorf("PrevWeek=%v want 3h", r.PrevWeek)
	}
}

// TestNodeStats_YearAndPrevYearToDate pins the Screen-02 year tile: Year is the
// running calendar year, PrevYearToDate is the SAME span one year back — Jan 1
// up to today-minus-one-year, NOT the whole previous year. The November session
// is the discriminator: it lies in the previous calendar year but AFTER the
// cutoff, so an implementation that simply summed "last year" would report 4h.
func TestNodeStats_YearAndPrevYearToDate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	ss := testutil.NewFakeSessionStore()
	// Wednesday, 2026-07-01 12:00 UTC → year starts 2026-01-01,
	// the previous-year window is 2025-01-01 .. 2025-07-01 12:00.
	clk := testutil.FakeClock{T: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)}

	eng := "eng"
	n, _ := domain.NewNode(eng, "u1", eng, eng, clk.Now())
	n.Kind = domain.KindEngagement
	_, _ = ns.Create(ctx, n)

	ids := &testutil.FakeIDGen{}
	add := func(label string, start, stop time.Time) {
		t.Helper()
		if _, err := (usecase.AddSession{Sessions: ss, Nodes: ns, IDs: ids, Clock: clk}).
			Execute(ctx, "u1", &eng, start, stop, nil, ""); err != nil {
			t.Fatalf("AddSession (%s): %v", label, err)
		}
	}
	// 2h this year
	add("this year", time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC), time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC))
	// 1h last year, inside the same-span window
	add("prev year to date", time.Date(2025, 3, 10, 9, 0, 0, 0, time.UTC), time.Date(2025, 3, 10, 10, 0, 0, 0, time.UTC))
	// 3h last year, but past the cutoff — belongs to neither window
	add("prev year after cutoff", time.Date(2025, 11, 5, 9, 0, 0, 0, time.UTC), time.Date(2025, 11, 5, 12, 0, 0, 0, time.UTC))

	sc := usecase.StatsComputer{Sessions: ss, Nodes: ns, Clock: clk, Loc: time.UTC}
	r, err := sc.NodeStats(ctx, "u1", eng)
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 6*time.Hour {
		t.Errorf("Total=%v want 6h", r.Total)
	}
	if r.Year != 2*time.Hour {
		t.Errorf("Year=%v want 2h", r.Year)
	}
	if r.PrevYearToDate != 1*time.Hour {
		t.Errorf("PrevYearToDate=%v want 1h (the November session is past the cutoff)", r.PrevYearToDate)
	}
}
