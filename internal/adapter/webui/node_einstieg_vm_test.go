package webui

// Pure-helper and builder tests for the Register-Einstieg (Screen 02) view
// model — no server, no I/O. Each helper is exercised directly (hand-computed
// expectations) so BuildNodeEinstieg's arithmetic is pinned independent of the
// httpserver handler that wires it up (Task 7).

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// ---------------------------------------------------------------------------
// SubtreeLastChange
// ---------------------------------------------------------------------------

func TestSubtreeLastChange(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	renamedAt := now.Add(-5 * time.Hour) // repoC's own rename — earliest
	docAt := now.Add(-3 * time.Hour)     // doc on repoA
	stopAt := now.Add(-1 * time.Hour)    // stopped session on vor1 — later than docAt

	nodes := []domain.Node{
		{ID: "eng1", Kind: domain.KindEngagement},
		{ID: "vor1", Kind: domain.KindVorhaben, ParentID: ptr("eng1")},
		{ID: "repoA", Kind: domain.KindRepo, ParentID: ptr("vor1")},
		{ID: "repoC", Kind: domain.KindRepo, ParentID: ptr("eng1"), UpdatedAt: renamedAt},
		{ID: "untouched", Kind: domain.KindRepo, ParentID: ptr("eng1")},
		// separate tree — isolates the running-session assertion from eng1's rollup
		{ID: "eng2", Kind: domain.KindEngagement},
		{ID: "repoB", Kind: domain.KindRepo, ParentID: ptr("eng2")},
	}
	docs := []domain.Document{
		{ID: "d1", NodeID: ptr("repoA"), UpdatedAt: docAt},
		{ID: "d2", NodeID: ptr("ghost"), UpdatedAt: now}, // foreign node, not in nodes — must not panic
	}
	sessions := []domain.WorkSession{
		{ID: "s1", NodeID: ptr("vor1"), Start: stopAt.Add(-time.Hour), Stop: ptrT(stopAt)},
		{ID: "s2", NodeID: ptr("repoB"), Start: now.Add(-10 * time.Minute), Stop: nil}, // running
	}

	got := SubtreeLastChange(nodes, docs, sessions, now)

	if !got["repoA"].Equal(docAt) {
		t.Errorf("repoA = %v, want %v (doc only)", got["repoA"], docAt)
	}
	if !got["vor1"].Equal(stopAt) {
		t.Errorf("vor1 = %v, want %v (later of doc-bubble and its own stopped session)", got["vor1"], stopAt)
	}
	if !got["eng1"].Equal(stopAt) {
		t.Errorf("eng1 = %v, want %v (inherits the later of doc+session in its subtree)", got["eng1"], stopAt)
	}
	if !got["repoC"].Equal(renamedAt) {
		t.Errorf("repoC = %v, want %v (renamed child, own UpdatedAt only)", got["repoC"], renamedAt)
	}
	if !got["repoB"].Equal(now) {
		t.Errorf("repoB = %v, want now=%v (running session)", got["repoB"], now)
	}
	if _, ok := got["untouched"]; ok {
		t.Errorf("untouched node present in map (%v), want absent", got["untouched"])
	}
	if _, ok := got["ghost"]; ok {
		t.Errorf("foreign doc target must not appear in map")
	}
}

// ---------------------------------------------------------------------------
// MonthSumsByChild
// ---------------------------------------------------------------------------

func TestMonthSumsByChild(t *testing.T) {
	t.Parallel()
	monthStart := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	now := monthStart.Add(10 * time.Hour)
	children := []domain.Node{{ID: "vor1"}}
	parents := map[string]string{"repoA": "vor1", "vor1": "eng1"}

	sessions := []domain.WorkSession{
		// grandchild session counts to the child (vor1) via the parent walk
		{NodeID: ptr("repoA"), Start: monthStart.Add(1 * time.Hour), Stop: ptrT(monthStart.Add(3 * time.Hour))},
		// session on the entry node itself counts to "own"
		{NodeID: ptr("eng1"), Start: monthStart.Add(1 * time.Hour), Stop: ptrT(monthStart.Add(2 * time.Hour))},
		// entirely before monthStart — excluded
		{NodeID: ptr("vor1"), Start: monthStart.Add(-2 * time.Hour), Stop: ptrT(monthStart.Add(-1 * time.Hour))},
		// unbooked — skipped
		{NodeID: nil, Start: monthStart.Add(1 * time.Hour), Stop: ptrT(monthStart.Add(2 * time.Hour))},
	}

	byChild, own := MonthSumsByChild("eng1", children, parents, sessions, monthStart, now)
	if byChild["vor1"] != 2*time.Hour {
		t.Errorf("byChild[vor1] = %v, want 2h", byChild["vor1"])
	}
	if own != time.Hour {
		t.Errorf("own = %v, want 1h", own)
	}
}

// ---------------------------------------------------------------------------
// FirstBookingStart
// ---------------------------------------------------------------------------

func TestFirstBookingStart(t *testing.T) {
	t.Parallel()
	if _, ok := FirstBookingStart(nil, map[string]bool{"n1": true}); ok {
		t.Fatalf("empty session list: ok = true, want false")
	}

	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC) // earliest in-set
	sessions := []domain.WorkSession{
		{NodeID: ptr("foreign"), Start: t1}, // outside ids — must not win
		{NodeID: ptr("n1"), Start: t2},
		{NodeID: ptr("n1"), Start: t3},
	}
	ids := map[string]bool{"n1": true}
	got, ok := FirstBookingStart(sessions, ids)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if !got.Equal(t3) {
		t.Errorf("got = %v, want %v (earliest in-set)", got, t3)
	}
}

// ---------------------------------------------------------------------------
// AgentsActiveToday
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// AgeYearsMonths
// ---------------------------------------------------------------------------

func TestAgeYearsMonths(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		from       time.Time
		wantYears  int
		wantMonths int
	}{
		{"2y5m", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC), 2, 5},
		{"same day", now, 0, 0},
		{"future", now.Add(24 * time.Hour), 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			y, m := AgeYearsMonths(c.from, now)
			if y != c.wantYears || m != c.wantMonths {
				t.Errorf("AgeYearsMonths(%v, %v) = (%d,%d), want (%d,%d)", c.from, now, y, m, c.wantYears, c.wantMonths)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// EinstiegSince
// ---------------------------------------------------------------------------

func TestEinstiegSince(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{"12 minutes", now.Add(-12 * time.Minute), "vor 12 Minuten"},
		{"5 hours", now.Add(-5 * time.Hour), "vor 5 Stunden"},
		{"6 days", now.Add(-6 * 24 * time.Hour), "vor 6 Tagen"},
		{"1 day singular", now.Add(-24 * time.Hour), "vor 1 Tag"},
		{"95 days", now.Add(-95 * 24 * time.Hour), "vor 3 Monaten"},
		{"future clamped", now.Add(24 * time.Hour), "vor 1 Minute"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EinstiegSince(ctx, c.at, now); got != c.want {
				t.Errorf("EinstiegSince(%v) = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// PercentDelta
// ---------------------------------------------------------------------------

func TestPercentDelta(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		cur, prev time.Duration
		want      string
	}{
		{"positive", 118 * time.Hour, 100 * time.Hour, "+18 %"},
		{"negative", 96 * time.Hour, 100 * time.Hour, "−4 %"},
		{"zero", 100 * time.Hour, 100 * time.Hour, "±0 %"},
		{"prev zero", 100 * time.Hour, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PercentDelta(c.cur, c.prev); got != c.want {
				t.Errorf("PercentDelta(%v,%v) = %q, want %q", c.cur, c.prev, got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// fmtDurDecimal
// ---------------------------------------------------------------------------

func TestFmtDurDecimal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		d    time.Duration
		want string
	}{
		{18*time.Hour + 30*time.Minute, "18,5"},
		{20 * time.Hour, "20"},
		{0, "0"},
		{-time.Hour, "0"},
	}
	for _, c := range cases {
		if got := fmtDurDecimal(c.d); got != c.want {
			t.Errorf("fmtDurDecimal(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// einstiegChip
// ---------------------------------------------------------------------------

func TestEinstiegChip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docTypes := map[string]domain.DocumentType{
		"dPlan": domain.DocPlan, "dSpec": domain.DocSpec, "dMem": domain.DocMemory,
		"dCtx": domain.DocActiveContext, "dDaily": domain.DocDaily, "dOther": domain.DocProject,
	}
	cases := []struct {
		name      string
		e         domain.ActivityEntry
		wantLabel string
		wantTone  string
	}{
		{"session started", domain.ActivityEntry{Kind: "session.started"}, "ZEIT", "live"},
		{"doc plan", domain.ActivityEntry{Kind: "document.updated", TargetRef: ptr("dPlan")}, "PLAN", "purple"},
		{"doc spec", domain.ActivityEntry{Kind: "document.updated", TargetRef: ptr("dSpec")}, "SPEC", "teal"},
		{"doc memory", domain.ActivityEntry{Kind: "document.updated", TargetRef: ptr("dMem")}, "ERINN.", "red"},
		{"doc active context", domain.ActivityEntry{Kind: "document.updated", TargetRef: ptr("dCtx")}, "KONTEXT", "accent"},
		{"doc daily", domain.ActivityEntry{Kind: "document.updated", TargetRef: ptr("dDaily")}, "TAGEB.", "green"},
		{"doc other", domain.ActivityEntry{Kind: "document.updated", TargetRef: ptr("dOther")}, "NOTIZ", "blue"},
		{"unknown kind widget", domain.ActivityEntry{Kind: "widget.frobnicated"}, "WIDGET", "accent"},
		{"node created", domain.ActivityEntry{Kind: "node.created"}, "NODE", "accent"},
		{"no dot", domain.ActivityEntry{Kind: "ping"}, "PING", "accent"},
		{"long kind capped", domain.ActivityEntry{Kind: "instrumentation.x"}, "INSTRUM", "accent"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotLabel, gotTone := einstiegChip(ctx, c.e, docTypes)
			if gotLabel != c.wantLabel || gotTone != c.wantTone {
				t.Errorf("einstiegChip(%q) = (%q,%q), want (%q,%q)", c.e.Kind, gotLabel, gotTone, c.wantLabel, c.wantTone)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FilterPulse / pctStyle (moved here from cockpit_uebersicht_vm_test.go —
// reines Verschieben, kein Verhalten)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// BuildNodeEinstieg — integration test over the pure layer
// ---------------------------------------------------------------------------

func einstiegFixture() (domain.Node, []domain.Node, []domain.Node) {
	eng1 := domain.Node{
		ID: "eng1", Kind: domain.KindEngagement, Name: "Kunde A", Color: "amber", Slug: "kunde-a",
		Description:  "Eigene Produkte",
		WeeklyTarget: ptrDur(20 * time.Hour),
		UpdatedAt:    time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC), // 6 days before the fixture's now
	}
	vor1 := domain.Node{ID: "vor1", Kind: domain.KindVorhaben, Name: "Buch", Color: "violet", ParentID: ptr("eng1"), Status: domain.NodeActive}
	vor2 := domain.Node{ID: "vor2", Kind: domain.KindVorhaben, Name: "App", Color: "violet", ParentID: ptr("eng1"), Status: domain.NodePaused}
	repoA := domain.Node{ID: "repoA", Kind: domain.KindRepo, Name: "buch-satz", Slug: "buch-satz", ParentID: ptr("vor1"), UpstreamGit: "git@github.com:acme/buch-satz.git"}
	repoB := domain.Node{ID: "repoB", Kind: domain.KindRepo, Name: "buch-cover", Slug: "buch-cover", ParentID: ptr("vor1")}
	repoC := domain.Node{ID: "repoC", Kind: domain.KindRepo, Name: "landingpage", Slug: "landingpage", ParentID: ptr("eng1")}
	subtree := []domain.Node{eng1, vor1, vor2, repoA, repoB, repoC}
	return eng1, subtree, subtree
}

func TestBuildNodeEinstieg(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	monthStart := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	sinceStart := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)

	eng1, subtree, allNodes := einstiegFixture()

	sessions := []domain.WorkSession{
		// running on repoA (under vor1) — drives Open/RunningDur on vor1
		{ID: "srun", NodeID: ptr("repoA"), Start: now.Add(-161 * time.Minute), Stop: nil},
		// booking this month on repoA (grandchild of vor1)
		{ID: "s2", NodeID: ptr("repoA"), Start: monthStart.Add(2 * time.Hour), Stop: ptrT(monthStart.Add(5 * time.Hour))},
		// booking this month directly on eng1 itself
		{ID: "s3", NodeID: ptr("eng1"), Start: monthStart.Add(1 * time.Hour), Stop: ptrT(monthStart.Add(2 * time.Hour))},
		// first-ever booking, drives SinceMonth/AgeYearsMonths
		{ID: "sfirst", NodeID: ptr("vor1"), Start: sinceStart, Stop: ptrT(sinceStart.Add(time.Hour))},
	}
	docA := domain.Document{ID: "docA", NodeID: ptr("repoA"), Type: domain.DocSpec, Title: "Satzspiegel", UpdatedAt: now.Add(-2 * time.Hour)}
	docs := []domain.Document{docA}

	activity := []domain.ActivityEntry{
		// newest first (EinstiegInput.Activity contract) — the builder trusts this order.
		{Kind: "document.updated", ActorKind: "human", NodeRef: ptr("foreignNode"), At: now.Add(-10 * time.Minute)}, // outside subtree — dropped
		{Kind: "document.updated", ActorKind: "agent", ActorRef: "botA", NodeRef: ptr("eng1"), Label: ptr("eng1 own doc"), At: now.Add(-30 * time.Minute)},
		{Kind: "document.updated", ActorKind: "human", NodeRef: ptr("repoA"), TargetRef: ptr("docA"), Label: ptr("Satzspiegel"), At: now.Add(-1 * time.Hour)},
		{Kind: "session.started", ActorKind: "human", NodeRef: ptr("repoA"), At: now.Add(-161 * time.Minute)},
	}

	highlights := []domain.NodeHighlight{
		{ID: "h1", DocumentID: "docA", NodeID: "repoA", Quote: "Q1", CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "h2", DocumentID: "docA", NodeID: "repoA", Quote: "Q2", CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "h3", DocumentID: "docA", NodeID: "repoA", Quote: "Q3", CreatedAt: now.Add(-3 * time.Hour)},
		{ID: "h4", DocumentID: "docA", NodeID: "repoA", Quote: "Q4", CreatedAt: now.Add(-4 * time.Hour)},
		{ID: "h5", DocumentID: "docA", NodeID: "repoA", Quote: "Q5", CreatedAt: now.Add(-5 * time.Hour)},
		{ID: "h6", DocumentID: "docA", NodeID: "repoA", Quote: "Q6", CreatedAt: now.Add(-6 * time.Hour)},
		{ID: "hforeign", DocumentID: "docX", NodeID: "foreignNode", Quote: "QForeign", CreatedAt: now.Add(-30 * time.Minute)},
	}

	in := EinstiegInput{
		N:             eng1,
		Ancestors:     []domain.Node{eng1},
		AllNodes:      allNodes,
		Subtree:       subtree,
		Sessions:      sessions,
		Docs:          docs,
		Activity:      activity,
		AgentsToday:   1, // der Store zählt das jetzt selbst (DistinctAgentsSince)
		Highlights:    highlights,
		Rollup:        domain.NodeRollup{Week: 18*time.Hour + 30*time.Minute, Month: 55*time.Hour + 30*time.Minute, Year: 118 * time.Hour, PrevYearToDate: 100 * time.Hour, Total: 200 * time.Hour},
		Rate:          nil,
		RunningNodeID: "repoA",
		RunningBase:   161 * 60,
		Now:           now,
	}

	vm := BuildNodeEinstieg(ctx, in)

	// MetaParts: description excerpt, "ohne Abrechnung", one agent segment.
	if len(vm.MetaParts) != 3 {
		t.Fatalf("MetaParts = %v, want 3 segments", vm.MetaParts)
	}
	if vm.MetaParts[0] != "Eigene Produkte" {
		t.Errorf("MetaParts[0] = %q, want description excerpt", vm.MetaParts[0])
	}
	if vm.MetaParts[1] != "ohne Abrechnung" {
		t.Errorf("MetaParts[1] = %q, want noBilling text", vm.MetaParts[1])
	}
	if vm.MetaParts[2] != "1 Agent heute aktiv" {
		t.Errorf("MetaParts[2] = %q, want agents segment", vm.MetaParts[2])
	}

	if vm.WeekDecimal != "18,5" {
		t.Errorf("WeekDecimal = %q, want 18,5", vm.WeekDecimal)
	}
	if vm.WeekSoll != "20" {
		t.Errorf("WeekSoll = %q, want 20", vm.WeekSoll)
	}
	if vm.MonthName != "August" {
		t.Errorf("MonthName = %q, want August", vm.MonthName)
	}
	if vm.YearDelta != "+18 %" {
		t.Errorf("YearDelta = %q, want +18 %%", vm.YearDelta)
	}
	if vm.SinceMonth != "März 2024" {
		t.Errorf("SinceMonth = %q, want März 2024", vm.SinceMonth)
	}

	// No readme doc in the fixture: the head claims neither path nor age, and
	// the single way in is the create editor (the cockpit's address).
	if vm.HasReadme || vm.ReadmePath != "" || vm.ReadmeWhen != "" {
		t.Errorf("without a README: HasReadme=%v path=%q when=%q, want false/empty", vm.HasReadme, vm.ReadmePath, vm.ReadmeWhen)
	}
	if vm.ReadmeHref != "/wissen/neu?node=eng1&type=project&path=readme" {
		t.Errorf("ReadmeHref = %q, want the create editor", vm.ReadmeHref)
	}

	// Sortierkopf: default (SortByName==false) shows the ACTIVE mode ("geändert"),
	// the link leads to the other one ("?sort=name").
	if vm.SortLabelKey != "einstieg.sort.changed" {
		t.Errorf("SortLabelKey = %q, want einstieg.sort.changed (default is the active mode)", vm.SortLabelKey)
	}
	if vm.SortHref != "?sort=name" {
		t.Errorf("SortHref = %q, want ?sort=name", vm.SortHref)
	}
	// F1/C1: SortStateHref is the CURRENT state (Kasten reload URL) — the
	// default mode carries no query, the opposite of the toggle-target SortHref.
	if vm.SortStateHref != "" {
		t.Errorf("SortStateHref (default) = %q, want empty (Kasten reload must not jump to name-sort)", vm.SortStateHref)
	}

	// Kinder: default sort = SubtreeLastChange desc. vor1 has fresher activity
	// (doc+session on repoA, session on itself) than vor2 (never touched).
	if len(vm.Kinder) != 2 {
		t.Fatalf("Kinder = %v, want 2 rows", vm.Kinder)
	}
	if vm.Kinder[0].ID != "vor1" || vm.Kinder[1].ID != "vor2" {
		t.Fatalf("Kinder order = [%s,%s], want [vor1,vor2]", vm.Kinder[0].ID, vm.Kinder[1].ID)
	}
	if !vm.Kinder[0].Open {
		t.Errorf("Kinder[0] (running child) Open = false, want true")
	}
	if vm.Kinder[0].RunningDur != "2:41" {
		t.Errorf("Kinder[0].RunningDur = %q, want 2:41", vm.Kinder[0].RunningDur)
	}
	if vm.Kinder[1].Open {
		t.Errorf("Kinder[1].Open = true, want false")
	}
	if !vm.Kinder[1].Dimmed {
		t.Errorf("Kinder[1] (paused) Dimmed = false, want true")
	}
	if vm.Kinder[1].RunningDur != "" {
		t.Errorf("Kinder[1].RunningDur = %q, want empty (paused, not running)", vm.Kinder[1].RunningDur)
	}
	if len(vm.Kinder[0].Repos) != 2 {
		t.Fatalf("Kinder[0].Repos = %v, want 2", vm.Kinder[0].Repos)
	}

	// SortByName reorders alphabetically, and swaps the Sortierkopf: now the
	// active mode is "Name" (SortLabelKey), the link goes back to the default
	// (SortHref == "").
	in.SortByName = true
	vmSorted := BuildNodeEinstieg(ctx, in)
	if vmSorted.Kinder[0].ID != "vor2" || vmSorted.Kinder[1].ID != "vor1" {
		t.Fatalf("name-sorted Kinder = [%s,%s], want [vor2,vor1] (App < Buch)", vmSorted.Kinder[0].ID, vmSorted.Kinder[1].ID)
	}
	if vmSorted.SortLabelKey != "einstieg.sort.name" {
		t.Errorf("SortLabelKey (SortByName) = %q, want einstieg.sort.name (active mode)", vmSorted.SortLabelKey)
	}
	if vmSorted.SortHref != "" {
		t.Errorf("SortHref (SortByName) = %q, want empty (back to default)", vmSorted.SortHref)
	}
	if vmSorted.SortStateHref != "?sort=name" {
		t.Errorf("SortStateHref (SortByName) = %q, want ?sort=name (Kasten reload must keep the viewer's sort)", vmSorted.SortStateHref)
	}

	// LoseRepos: only the Engagement's direct repo child (repoC), not vor1's repos.
	if len(vm.LoseRepos) != 1 || vm.LoseRepos[0].ID != "repoC" {
		t.Fatalf("LoseRepos = %v, want [repoC]", vm.LoseRepos)
	}

	// Buchungen: descending by duration; "direkt hier" only because own > 0.
	// vor1 gets the 3h stopped session PLUS the still-running session's
	// elapsed-so-far (2h41m) — both fall inside [monthStart, now].
	if len(vm.Buchungen) != 2 {
		t.Fatalf("Buchungen = %v, want 2 rows", vm.Buchungen)
	}
	if vm.Buchungen[0].Title != "Buch" || vm.Buchungen[0].DurStr != "5:41 h" {
		t.Errorf("Buchungen[0] = %+v, want Buch/5:41 h (3h stopped + 2:41 running on repoA > 1h own)", vm.Buchungen[0])
	}
	if vm.Buchungen[1].Title != "direkt hier" || vm.Buchungen[1].DurStr != "1:00 h" {
		t.Errorf("Buchungen[1] = %+v, want direkt hier/1:00 h", vm.Buchungen[1])
	}

	// Feed: subtree-filtered (foreign entry dropped), Vorhaben resolved.
	if len(vm.Feed) != 3 {
		t.Fatalf("Feed = %v, want 3 rows (foreign entry dropped)", vm.Feed)
	}
	// newest first: eng1-own doc (Vorhaben=""), then session.started, then doc on repoA (both under vor1).
	if vm.Feed[0].Vorhaben != "" {
		t.Errorf("Feed[0].Vorhaben = %q, want empty (activity at the node itself)", vm.Feed[0].Vorhaben)
	}
	if vm.Feed[1].Vorhaben != "Buch" || vm.Feed[2].Vorhaben != "Buch" {
		t.Errorf("Feed[1/2].Vorhaben = %q/%q, want Buch/Buch", vm.Feed[1].Vorhaben, vm.Feed[2].Vorhaben)
	}

	// Highlights: subtree-filtered (hforeign dropped) and capped at 5.
	if len(vm.Highlights) != 5 {
		t.Fatalf("Highlights = %v, want 5 (capped, foreign dropped)", vm.Highlights)
	}
	for _, h := range vm.Highlights {
		if h.Quote == "QForeign" {
			t.Errorf("foreign highlight leaked into subtree-filtered Highlights")
		}
	}
}

func TestBuildNodeEinstieg_EmptyState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	fresh := domain.Node{ID: "eng1", Kind: domain.KindEngagement, Name: "Frisch"}

	in := EinstiegInput{
		N:         fresh,
		Ancestors: []domain.Node{fresh},
		AllNodes:  []domain.Node{fresh},
		Subtree:   []domain.Node{fresh},
		Now:       now,
	}

	var vm NodeEinstieg
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("BuildNodeEinstieg panicked: %v", r)
			}
		}()
		vm = BuildNodeEinstieg(ctx, in)
	}()

	if vm.SinceMonth != "" {
		t.Errorf("SinceMonth = %q, want empty", vm.SinceMonth)
	}
	if vm.Kinder != nil {
		t.Errorf("Kinder = %v, want nil", vm.Kinder)
	}
	if vm.Buchungen != nil {
		t.Errorf("Buchungen = %v, want nil", vm.Buchungen)
	}
	if vm.Highlights != nil {
		t.Errorf("Highlights = %v, want nil", vm.Highlights)
	}
	if vm.Feed != nil {
		t.Errorf("Feed = %v, want nil", vm.Feed)
	}
	if vm.YearDelta != "" {
		t.Errorf("YearDelta = %q, want empty", vm.YearDelta)
	}
	for _, p := range vm.MetaParts {
		if p == "1 Agent heute aktiv" {
			t.Errorf("MetaParts contains an agents segment with zero agents active")
		}
	}
}

func TestBuildNodeEinstieg_FeedCap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	n := domain.Node{ID: "eng1", Kind: domain.KindEngagement, Name: "Viel los"}

	var activity []domain.ActivityEntry
	for i := 0; i < 12; i++ {
		activity = append(activity, domain.ActivityEntry{
			Kind: "node.updated", NodeRef: ptr("eng1"), At: now.Add(-time.Duration(i) * time.Minute),
		})
	}

	in := EinstiegInput{
		N:         n,
		Ancestors: []domain.Node{n},
		AllNodes:  []domain.Node{n},
		Subtree:   []domain.Node{n},
		Activity:  activity,
		Now:       now,
	}
	vm := BuildNodeEinstieg(ctx, in)
	if len(vm.Feed) != 10 {
		t.Fatalf("Feed = %d rows, want capped at 10", len(vm.Feed))
	}
}

// TestBuildNodeEinstieg_HighlightHref pins F11/M5: EinstiegHighlightRow.Href
// must only be set when the highlight's DocumentID still resolves against
// in.Docs — BuildNodeEinstieg used to set it UNCONDITIONALLY, so a highlight
// marked on a since-deleted card linked straight into a 404. Both branches:
// one highlight whose document survives, one whose document is gone.
func TestBuildNodeEinstieg_HighlightHref(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	n := domain.Node{ID: "repoA", Kind: domain.KindRepo, Name: "flow"}

	in := EinstiegInput{
		N:         n,
		Ancestors: []domain.Node{n},
		AllNodes:  []domain.Node{n},
		Subtree:   []domain.Node{n},
		Docs:      []domain.Document{{ID: "docLives", NodeID: ptr("repoA")}},
		Highlights: []domain.NodeHighlight{
			{ID: "h1", DocumentID: "docLives", NodeID: "repoA", Quote: "lebendig", CreatedAt: now.Add(-1 * time.Hour)},
			{ID: "h2", DocumentID: "docGone", NodeID: "repoA", Quote: "verwaist", CreatedAt: now.Add(-2 * time.Hour)},
		},
		Now: now,
	}
	vm := BuildNodeEinstieg(ctx, in)
	if len(vm.Highlights) != 2 {
		t.Fatalf("Highlights = %d rows, want 2", len(vm.Highlights))
	}
	live := vm.Highlights[0]
	if live.Quote != "lebendig" || live.Href != "/wissen/docLives" {
		t.Errorf("live highlight = %+v, want Href /wissen/docLives", live)
	}
	gone := vm.Highlights[1]
	if gone.Quote != "verwaist" || gone.Href != "" {
		t.Errorf("orphaned highlight = %+v, want Href empty (DocumentID not in in.Docs)", gone)
	}
}

func ptrDur(d time.Duration) *time.Duration { return &d }
func ptrT(t time.Time) *time.Time           { return &t }

// TestEinstiegPage_MountsDocRenderScripts pins what DocRenderScripts' own
// comment demands: every surface that renders a Kompendium document body
// mounts the same set. The entry point shows the register's README in its
// reading column, so mermaid diagrams and the image lightbox must work there
// exactly as they do on /wissen/{id}. The flat cockpit page mounted them and
// had a test for it; that page is gone, and its test went with it — this one
// takes over.
func TestEinstiegPage_MountsDocRenderScripts(t *testing.T) {
	d := NodeEinstieg{N: domain.Node{ID: "n1", Name: "flow", Kind: domain.KindRepo}}
	out := renderToBuf(t, context.Background(), NodeEinstiegPage(d))
	for _, want := range []string{"js/mermaid-init.js", "js/lightbox.js", `id="doc-lightbox"`} {
		if !strings.Contains(out, want) {
			t.Errorf("the entry point renders a README but does not mount %s", want)
		}
	}
	// Once per page, and OUTSIDE the SSE-swapped columns — a fragment swap
	// must never re-add the tag.
	if n := strings.Count(out, "js/mermaid-init.js"); n != 1 {
		t.Errorf("mermaid-init.js mounted %d times, want exactly once", n)
	}
}

// TestBuildNodeEinstieg_Readme: with the register's own readme document the
// Lesespalte shows ITS path and age (not the node's), and "Bearbeiten" leads
// to that document's editor — not to an address that does not exist.
func TestBuildNodeEinstieg_Readme(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	eng1, subtree, allNodes := einstiegFixture()
	readme := domain.Document{ID: "rd1", NodeID: ptr("eng1"), Type: domain.DocProject, Path: "readme", Title: "Kunde A", UpdatedAt: now.AddDate(0, 0, -6)}

	vm := BuildNodeEinstieg(ctx, EinstiegInput{
		N: eng1, Ancestors: []domain.Node{eng1}, AllNodes: allNodes, Subtree: subtree,
		Docs: []domain.Document{readme}, Readme: &readme, ReadmeHTML: "<p>Hallo README</p>",
		Now: now,
	})

	if !vm.HasReadme {
		t.Fatal("HasReadme = false, want true")
	}
	if vm.ReadmeHTML != "<p>Hallo README</p>" {
		t.Errorf("ReadmeHTML = %q, want the rendered body passed through", vm.ReadmeHTML)
	}
	if vm.ReadmeHref != "/wissen/rd1/bearbeiten" {
		t.Errorf("ReadmeHref = %q, want /wissen/rd1/bearbeiten", vm.ReadmeHref)
	}
	if vm.ReadmePath != "kunde-a/readme" {
		t.Errorf("ReadmePath = %q, want kunde-a/readme", vm.ReadmePath)
	}
	if vm.ReadmeWhen != "vor 6 Tagen" {
		t.Errorf("ReadmeWhen = %q, want vor 6 Tagen (the DOCUMENT's age)", vm.ReadmeWhen)
	}
}
