package webui

// Pure-helper tests for the Übersicht feed's VM builders — no server, no I/O.
// Each helper is exercised directly so its exact rounding/formatting/filter
// behavior is pinned independent of the httpserver builder that wires it up.

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestBuildUebersichtTiles_WeekDelta(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		roll      domain.NodeRollup
		wantDelta string
	}{
		{
			name:      "positive delta",
			roll:      domain.NodeRollup{Week: 12*time.Hour + 5*time.Minute, PrevWeek: 10 * time.Hour},
			wantDelta: "+2h 05m",
		},
		{
			name:      "negative delta",
			roll:      domain.NodeRollup{Week: 1 * time.Hour, PrevWeek: 1*time.Hour + 30*time.Minute},
			wantDelta: "−0h 30m",
		},
		{
			name:      "no prior week: empty delta regardless of Week",
			roll:      domain.NodeRollup{Week: 5 * time.Hour, PrevWeek: 0},
			wantDelta: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := BuildUebersichtTiles(c.roll, nil)
			if got.WeekDelta != c.wantDelta {
				t.Errorf("WeekDelta = %q, want %q", got.WeekDelta, c.wantDelta)
			}
		})
	}
}

func TestBuildUebersichtTiles_TotalsAndEarnings(t *testing.T) {
	t.Parallel()
	roll := domain.NodeRollup{
		Total: 10 * time.Hour,
		Week:  12*time.Hour + 5*time.Minute,
		Month: 40 * time.Hour,
	}
	rate := &domain.Money{Amount: 9500, Currency: "EUR"} // 95.00 EUR/h
	got := BuildUebersichtTiles(roll, rate)
	if got.TotalStr != "10:00 h" {
		t.Errorf("TotalStr = %q, want %q", got.TotalStr, "10:00 h")
	}
	if got.WeekStr != "12:05 h" {
		t.Errorf("WeekStr = %q, want %q", got.WeekStr, "12:05 h")
	}
	if got.MonthStr != "40:00 h" {
		t.Errorf("MonthStr = %q, want %q", got.MonthStr, "40:00 h")
	}
	if got.Earnings != "950.00 EUR" {
		t.Errorf("Earnings = %q, want %q", got.Earnings, "950.00 EUR")
	}

	// No rate anywhere in the chain: Earnings falls back to "—" (rateLabel's
	// established "no value" convention), never a blank tile.
	noRate := BuildUebersichtTiles(roll, nil)
	if noRate.Earnings != "—" {
		t.Errorf("Earnings with nil rate = %q, want %q", noRate.Earnings, "—")
	}
}

func TestBuildSplit_HasSplitCollapse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		roll         domain.NodeRollup
		wantHasSplit bool
		wantWorkPct  int
	}{
		{
			name:         "pure work: privat side is zero, collapses",
			roll:         domain.NodeRollup{Week: 10 * time.Hour, WorkWeek: 10 * time.Hour},
			wantHasSplit: false,
		},
		{
			name:         "pure privat: work side is zero, collapses",
			roll:         domain.NodeRollup{Week: 10 * time.Hour, WorkWeek: 0},
			wantHasSplit: false,
		},
		{
			name:         "both sides present: split shown",
			roll:         domain.NodeRollup{Week: 10 * time.Hour, WorkWeek: 4 * time.Hour},
			wantHasSplit: true,
			wantWorkPct:  40,
		},
		{
			name:         "zero week entirely: collapses",
			roll:         domain.NodeRollup{},
			wantHasSplit: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			workPct, hasSplit, _, _, _ := BuildSplit(c.roll)
			if hasSplit != c.wantHasSplit {
				t.Errorf("hasSplit = %v, want %v", hasSplit, c.wantHasSplit)
			}
			if hasSplit && workPct != c.wantWorkPct {
				t.Errorf("workPct = %d, want %d", workPct, c.wantWorkPct)
			}
		})
	}
}

func TestBuildSplit_Strings(t *testing.T) {
	t.Parallel()
	roll := domain.NodeRollup{Week: 10 * time.Hour, WorkWeek: 4 * time.Hour, WorkMonth: 30 * time.Hour}
	_, _, workWeekStr, privatWeekStr, workMonthStr := BuildSplit(roll)
	if workWeekStr != "4:00 h" {
		t.Errorf("workWeekStr = %q, want %q", workWeekStr, "4:00 h")
	}
	if privatWeekStr != "6:00 h" {
		t.Errorf("privatWeekStr = %q, want %q", privatWeekStr, "6:00 h")
	}
	if workMonthStr != "30:00 h" {
		t.Errorf("workMonthStr = %q, want %q", workMonthStr, "30:00 h")
	}
}

func TestBuildComp_PctRounding(t *testing.T) {
	t.Parallel()
	children := []domain.Node{
		{ID: "c1", Name: "One", Kind: domain.KindRepo, Color: "cyan"},
		{ID: "c2", Name: "Two", Kind: domain.KindRepo, Color: "purple"},
	}
	statsByID := map[string]domain.NodeRollup{
		"c1": {Total: 1 * time.Hour},
		"c2": {Total: 2 * time.Hour},
	}
	nodeTotal := 3 * time.Hour
	rows := BuildComp(children, statsByID, "", nil, nil, nodeTotal, time.Now())
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].Pct != 33 {
		t.Errorf("rows[0].Pct = %d, want 33 (1/3 rounded)", rows[0].Pct)
	}
	if rows[1].Pct != 67 {
		t.Errorf("rows[1].Pct = %d, want 67 (2/3 rounded)", rows[1].Pct)
	}
}

func TestBuildComp_ZeroNodeTotal_NoDivByZero(t *testing.T) {
	t.Parallel()
	children := []domain.Node{{ID: "c1", Name: "One"}}
	rows := BuildComp(children, map[string]domain.NodeRollup{}, "", nil, nil, 0, time.Now())
	if len(rows) != 1 || rows[0].Pct != 0 {
		t.Errorf("rows = %+v, want single row with Pct=0", rows)
	}
}

// TestBuildComp_LiveDetection pins the parent-walk: a running session two
// levels below a direct child must still light up that child's live dot,
// using only the subtree's parent map (no extra per-child query).
func TestBuildComp_LiveDetection(t *testing.T) {
	t.Parallel()
	children := []domain.Node{
		{ID: "child-a", Name: "A"},
		{ID: "child-b", Name: "B"},
	}
	// grandchild-of-a is TWO levels under child-a.
	subtreeParents := map[string]string{
		"child-a":         "cockpit",
		"child-b":         "cockpit",
		"grandchild-of-a": "child-a",
	}
	rows := BuildComp(children, map[string]domain.NodeRollup{}, "grandchild-of-a", subtreeParents, nil, time.Hour, time.Now())
	var gotA, gotB bool
	for _, r := range rows {
		if r.ID == "child-a" {
			gotA = r.Live
		}
		if r.ID == "child-b" {
			gotB = r.Live
		}
	}
	if !gotA {
		t.Errorf("child-a should be Live (running session is 2 levels under it)")
	}
	if gotB {
		t.Errorf("child-b should NOT be Live")
	}
}

// TestBuildComp_LastActFromPulse pins that LastAct attributes an activity
// entry to the child whose subtree the entry's NodeRef falls under, and
// picks the freshest one when multiple entries match the same child.
func TestBuildComp_LastActFromPulse(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	children := []domain.Node{{ID: "child-a", Name: "A"}}
	subtreeParents := map[string]string{"child-a": "cockpit"}
	older := "child-a"
	newer := "child-a"
	pulse := []domain.ActivityEntry{
		{NodeRef: &older, At: now.Add(-2 * time.Hour)},
		{NodeRef: &newer, At: now.Add(-10 * time.Minute)},
	}
	rows := BuildComp(children, map[string]domain.NodeRollup{}, "", subtreeParents, pulse, time.Hour, now)
	if rows[0].LastAct == "" {
		t.Fatalf("LastAct must be set from the matching pulse entry")
	}
	if rows[0].LastAct != "vor 10 Min" {
		t.Errorf("LastAct = %q, want the freshest entry's rel time %q", rows[0].LastAct, "vor 10 Min")
	}
}

func TestBuildChain_Pct(t *testing.T) {
	t.Parallel()
	node := domain.Node{ID: "repo1", Name: "flow", Kind: domain.KindRepo}
	ancestors := []domain.Node{
		{ID: "vor1", Name: "Plattform", Kind: domain.KindVorhaben},
		{ID: "eng1", Name: "Kunde", Kind: domain.KindEngagement},
	}
	statsByID := map[string]domain.NodeRollup{
		"repo1": {Total: 4 * time.Hour},
		"vor1":  {Total: 20 * time.Hour},
		"eng1":  {Total: 50 * time.Hour},
	}
	ownerTotal := 50 * time.Hour
	rows := BuildChain(node, ancestors, statsByID, ownerTotal)
	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4 (this + 2 ancestors + sum)", len(rows))
	}
	if !rows[0].This {
		t.Errorf("rows[0] must be the This row")
	}
	if rows[0].Pct != 8 {
		t.Errorf("this row Pct = %d, want 8 (4h/50h)", rows[0].Pct)
	}
	sum := rows[len(rows)-1]
	if !sum.Sum {
		t.Errorf("last row must be the Sum row")
	}
	if sum.Pct != 100 {
		t.Errorf("sum row Pct = %d, want 100 (hardcoded)", sum.Pct)
	}
	if sum.DurStr != fmtDurHM(ownerTotal) {
		t.Errorf("sum row DurStr = %q, want %q", sum.DurStr, fmtDurHM(ownerTotal))
	}
}

func TestBuildChain_ZeroOwnerTotal_NoDivByZero(t *testing.T) {
	t.Parallel()
	node := domain.Node{ID: "repo1", Name: "flow"}
	rows := BuildChain(node, nil, map[string]domain.NodeRollup{"repo1": {Total: time.Hour}}, 0)
	if rows[0].Pct != 0 {
		t.Errorf("this row Pct = %d, want 0 when ownerTotal is 0", rows[0].Pct)
	}
}

func TestFilterPulse_DropsForeignNodeRef(t *testing.T) {
	t.Parallel()
	inSubtree := "n1"
	foreign := "other"
	entries := []domain.ActivityEntry{
		{Kind: "session.started", NodeRef: &inSubtree},
		{Kind: "session.started", NodeRef: &foreign},
		{Kind: "document.created", NodeRef: nil}, // no node association at all
	}
	subtreeIDs := map[string]bool{"n1": true}
	got := FilterPulse(entries, subtreeIDs)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (only the in-subtree entry survives)", len(got))
	}
	if got[0].NodeRef == nil || *got[0].NodeRef != "n1" {
		t.Errorf("surviving entry NodeRef = %v, want n1", got[0].NodeRef)
	}
}

func TestTopDocs_SortAndTop3(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	inNode := "n1"
	outNode := "other"
	mk := func(id, title string, ago time.Duration, nodeID *string) domain.Document {
		return domain.Document{ID: id, Title: title, NodeID: nodeID, UpdatedAt: now.Add(-ago)}
	}
	docs := []domain.Document{
		mk("d1", "Oldest in-subtree", 5*time.Hour, &inNode),
		mk("d2", "Newest in-subtree", 1*time.Hour, &inNode),
		mk("d3", "Middle in-subtree", 3*time.Hour, &inNode),
		mk("d4", "Fourth in-subtree", 4*time.Hour, &inNode),
		mk("d5", "Foreign subtree", 30*time.Minute, &outNode),
	}
	subtreeIDs := map[string]bool{"n1": true}
	top, total := TopDocs(docs, subtreeIDs, now)
	if total != 4 {
		t.Fatalf("total = %d, want 4 (foreign doc excluded)", total)
	}
	if len(top) != 3 {
		t.Fatalf("len(top) = %d, want 3", len(top))
	}
	wantOrder := []string{"Newest in-subtree", "Middle in-subtree", "Fourth in-subtree"}
	for i, w := range wantOrder {
		if top[i].Title != w {
			t.Errorf("top[%d].Title = %q, want %q", i, top[i].Title, w)
		}
	}
}

func TestPctStyle_Clamps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		pct  int
		want string
	}{
		{50, "width:50%"},
		{-5, "width:0%"},
		{150, "width:100%"},
	}
	for _, c := range cases {
		if got := pctStyle(c.pct); got != c.want {
			t.Errorf("pctStyle(%d) = %q, want %q", c.pct, got, c.want)
		}
	}
}
