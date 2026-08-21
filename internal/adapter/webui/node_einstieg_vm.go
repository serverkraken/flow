package webui

import (
	"context"
	"fmt"
	"html/template"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// EinstiegInput is everything the handler loaded for the register entry point,
// owner-scoped and ONE query per sort. The builder does the arithmetic; the
// handler does the I/O. Keeping the seam here makes every Screen-02 figure a
// plain table test instead of an HTTP round trip.
type EinstiegInput struct {
	N         domain.Node
	Ancestors []domain.Node          // leaf→root, self included (NodeStore.Ancestors order)
	AllNodes  []domain.Node          // the owner's nodes — names/kinds/parents beyond the subtree
	Subtree   []domain.Node          // N + descendants (root→leaf)
	Sessions  []domain.WorkSession   // the owner's sessions, oldest first (ListSessionsRange order)
	Docs      []domain.Document      // the owner's documents
	Activity  []domain.ActivityEntry // the owner's activity feed, newest first — capped for the Feed section (Spec C.2.5)
	// AgentsToday is the count of distinct AGENT actors that touched this
	// subtree since midnight. It arrives as a number, not as rows to count:
	// the store answers it directly (DistinctAgentsSince), so no owner-wide
	// feed has to be scanned and no neighbour can distort the tally.
	AgentsToday     int
	Highlights      []domain.NodeHighlight // the owner's newest highlights, newest first
	Rollup          domain.NodeRollup
	Rate            *domain.Money // resolved over N + ancestors, nil = none anywhere
	RateSource      string        // name of the ancestor that carries it, "" when own or none
	Contributors    []string
	RunningNodeID   string // node the owner's running session is booked to, "" if none
	SortByName      bool   // ?sort=name
	DescriptionHTML template.HTML
	// RunningBase is the running session's elapsed seconds. The entry point
	// shows NO clock of its own (Soenne, 21.08.: mockup-true — the clock sits
	// in the rail); this is only the live duration on the running child's row.
	RunningBase int64
	TodayHere   string
	CountsWork  bool
	Today       string // YYYY-MM-DD
	Now         time.Time
}

// NodeEinstieg drives the register entry point (Screen 02). Every field is a
// finished display value — the templates carry no arithmetic and no lookups.
type NodeEinstieg struct {
	N               domain.Node
	Ancestors       []domain.Node
	DescriptionHTML template.HTML
	EbeneColor      string // EbeneAccentColor(N.Kind) — the 3px Ebenenstreifen
	TodayHere       string
	CountsWork      bool
	Today           string

	// Kopf: "Eigene Produkte · ohne Abrechnung · 2 Agenten heute aktiv".
	// Empty segments are already dropped — the template joins with " · ".
	MetaParts []string
	BannerRef string

	// Kennzahlen (Screen 02, drei Zellen)
	WeekDecimal string // "18,5"
	WeekSoll    string // "20" — "" without a weekly target
	MonthStr    string // "55:30"
	MonthName   string // "August"
	YearStr     string // "612:20"
	YearNum     string // "2026"
	YearDelta   string // "+18 %" / "−4 %" — "" without a previous-year figure

	SinceMonth string // "März 2024" — "" when the subtree has no booking at all

	Facts []EinstiegFact

	// Kinder-Liste: Vorhaben (Engagement) bzw. Projekte (Vorhaben); nil beim Repo.
	KinderLabelKey string
	KinderHintKey  string
	Kinder         []EinstiegKindRow
	SortHref       string // "?sort=name" or "" — the TOGGLE target (Sortierkopf link leads HERE)
	SortStateHref  string // "?sort=name" or "" — the CURRENT sort state (Kasten reload hx-get; F1/C1)
	SortLabelKey   string

	LoseRepos []EinstiegRepoRow // "Projekte ohne Vorhaben" — Engagement only

	// CardsTotal is the subtree card count incl. the node itself — the
	// Kopf's "Wissen N ›" way (Spur A4).
	CardsTotal int

	// Wissen vor Zahlen (Soenne, 21.08.): der Kasten zeigt zuerst, was das
	// Register weiß — Typ-Zähler als Wege in den Überblick je Ebene und die
	// frischesten Karten des Teilbaums —, dann erst die Stunden.
	WissenHref   string             // der Überblick je Ebene
	WissenTypes  []WissenTypZaehler // Typen im Teilbaum, häufigster zuerst; Links filtern den Überblick
	WissenRecent []EinstiegWissenRow

	Buchungen     []EinstiegBuchungRow
	BuchungenSpan string // "01.–16.08." — the month window, once in the section head (F14/M7)

	// Lesespalte
	ReadmePath string // "privat/README.md"
	ReadmeWhen string // "vor 6 Tagen" — a RELATIVE age, not the Datumsstaffel:
	// Screen 02's README head answers "how long ago", not
	// "when" (Spec C.2.1 "zuletzt ‹relativ›").
	Highlights []EinstiegHighlightRow
	Feed       []EinstiegFeedRow
}

// EinstiegWissenRow ist eine der frischesten Karten im Kasten: Typ, Titel,
// Herkunft (leer an der Ebene selbst) und Datumsstaffel.
type EinstiegWissenRow struct {
	ID, Title            string
	ChipClass, ChipLabel string
	Where                string // ShortName des Registers, das die Karte trägt; "" = die Ebene selbst
	When                 string
}

// einstiegWissenCap ist die Zahl der Karten, die der Kasten zeigt — ein
// Blick, kein Regal. Das Regal ist der Überblick je Ebene.
const einstiegWissenCap = 5

// EinstiegFact is one Eckdaten row: label, value, optional quiet right column.
type EinstiegFact struct {
	LabelKey string
	Value    string
	Aside    string // "2 J. 5 M." / "seit Beginn"; "" = no right column
	Mono     bool
}

// EinstiegKindRow is one direct child of the entry node.
type EinstiegKindRow struct {
	ID, Name   string
	Kind       domain.NodeKind
	Color      string
	Dimmed     bool   // paused or archived
	RunningDur string // "2:41" when the clock runs inside this child's subtree, else ""
	When       string // Datumsstaffel of the freshest subtree change ("" if never)
	Cards      int
	Repos      []EinstiegRepoRow
	Open       bool // collapsed by default; the running child opens
}

// EinstiegRepoRow is one repo line: slug, upstream, card count. Commits and
// branches do not exist server-side — they arrive with Slice 7 (Screen 15).
type EinstiegRepoRow struct {
	ID, Slug string
	Name     string // display name — the same name the K3Rail shows (Spur A2)
	ParentID string // the Kind row this line hangs under — the collapse script's
	// filter key (data-einstieg-child); "" for LoseRepos, which
	// never collapse.
	Upstream string // gitDisplay(...); "" → the template prints "kein Upstream"
	Cards    int
}

// EinstiegBuchungRow is one line of "Buchungen im ‹Monat›". The month span
// used to repeat on every row (M7) — it now lives once on NodeEinstieg.BuchungenSpan.
type EinstiegBuchungRow struct {
	Title  string // child name, or the i18n'd "direkt hier"
	DurStr string // "12:30 h"
	Amount string // money, "" when no rate is in effect
}

// EinstiegHighlightRow is one marked passage from a daily note.
type EinstiegHighlightRow struct {
	Day   string // Datumsstaffel
	Quote string
	Where string // name of the marked node
	Href  string // "/wissen/{documentID}" — "" when DocumentID no longer resolves (F11/M5)
}

// EinstiegFeedRow is one line of "Zuletzt in ‹Name›".
type EinstiegFeedRow struct {
	When      string // fmtFeedTime
	ChipLabel string // resolved: "PLAN" | "SPEC" | "NOTIZ" | "ERINN." | "KONTEXT" | "ZEIT" | "TAGEB."
	ChipClass string // paletteTypeTextClass(tone)
	Title     string
	Href      string // "" = not linkable
	Note      string
	Vorhaben  string // level-1 descendant the target sits under; "" at the node itself
}

// BuildNodeEinstieg turns the loaded owner data into the Screen-02 view model.
// ctx carries the locale (month names, labels); it makes no I/O call.
func BuildNodeEinstieg(ctx context.Context, in EinstiegInput) NodeEinstieg {
	out := NodeEinstieg{
		N:               in.N,
		Ancestors:       in.Ancestors,
		DescriptionHTML: in.DescriptionHTML,
		EbeneColor:      EbeneAccentColor(in.N.Kind),
		TodayHere:       in.TodayHere,
		CountsWork:      in.CountsWork,
		Today:           in.Today,
		BannerRef:       in.N.BannerRef,
		ReadmePath:      in.N.Slug + "/README.md",
		ReadmeWhen:      EinstiegSince(ctx, in.N.UpdatedAt, in.Now),
	}

	subtreeIDs := make(map[string]bool, len(in.Subtree))
	subtreeParents := make(map[string]string, len(in.Subtree))
	childrenByParent := make(map[string][]domain.Node, len(in.Subtree))
	for _, n := range in.Subtree {
		subtreeIDs[n.ID] = true
		if n.ParentID != nil {
			subtreeParents[n.ID] = *n.ParentID
			childrenByParent[*n.ParentID] = append(childrenByParent[*n.ParentID], n)
		}
	}
	nameByID := make(map[string]string, len(in.AllNodes))
	for _, n := range in.AllNodes {
		nameByID[n.ID] = n.Name
	}
	directChildren := childrenByParent[in.N.ID]
	level1 := make(map[string]bool, len(directChildren))
	for _, c := range directChildren {
		level1[c.ID] = true
	}

	agents := in.AgentsToday

	// --- Kopf: MetaParts ---
	var meta []string
	if excerpt := docKastenExcerpt(in.N.Description); excerpt != "" {
		meta = append(meta, excerpt)
	}
	if in.Rate != nil {
		meta = append(meta, RateLabel(in.Rate))
	} else {
		meta = append(meta, components.T(ctx, "einstieg.noBilling"))
	}
	if agents > 0 {
		meta = append(meta, components.Tn(ctx, "einstieg.agentsToday", agents))
	}
	out.MetaParts = meta

	// --- Kennzahlen ---
	out.WeekDecimal = fmtDurDecimal(in.Rollup.Week)
	if in.N.WeeklyTarget != nil {
		out.WeekSoll = fmtDurDecimal(*in.N.WeeklyTarget)
	}
	out.MonthStr = fmtClock(in.Rollup.Month)
	out.MonthName = monthText(ctx, in.Now.Month())
	out.YearStr = fmtClock(in.Rollup.Year)
	out.YearNum = itoa(in.Now.Year())
	out.YearDelta = PercentDelta(in.Rollup.Year, in.Rollup.PrevYearToDate)

	// --- SinceMonth / Facts.since ---
	lastChange := SubtreeLastChange(in.Subtree, in.Docs, in.Sessions, in.Now)
	docTotals := SubtreeDocTotals(in.Subtree, in.Docs)
	out.CardsTotal = docTotals[in.N.ID]

	// --- Wissen vor Zahlen ---
	out.WissenHref = WissenEbenePageHref(in.N.ID, WissenEbeneQuery{})
	var subtreeDocs []domain.Document
	for _, d := range in.Docs {
		if d.Archived || d.NodeID == nil || !subtreeIDs[*d.NodeID] {
			continue
		}
		subtreeDocs = append(subtreeDocs, d)
	}
	out.WissenTypes = buildTypZaehler(in.N.ID, subtreeDocs, WissenEbeneQuery{}, "")
	for _, d := range SortDocuments(subtreeDocs, SortChanged) {
		row := EinstiegWissenRow{
			ID:        d.ID,
			Title:     d.Title,
			ChipClass: DocTypeChipClass(d.Type),
			ChipLabel: DocTypeLabel(d.Type),
			When:      FmtStaffel(ctx, d.UpdatedAt, in.Now),
		}
		if *d.NodeID != in.N.ID {
			row.Where = ShortName(nameByID[*d.NodeID])
		}
		out.WissenRecent = append(out.WissenRecent, row)
		if len(out.WissenRecent) == einstiegWissenCap {
			break
		}
	}
	since, hasSince := FirstBookingStart(in.Sessions, subtreeIDs)

	var facts []EinstiegFact
	if hasSince {
		out.SinceMonth = monthText(ctx, since.Month()) + " " + itoa(since.Year())
		years, months := AgeYearsMonths(since, in.Now)
		facts = append(facts, EinstiegFact{
			LabelKey: "einstieg.fact.since",
			Value:    out.SinceMonth,
			Aside:    fmtAge(ctx, years, months),
		})
	}
	billing := EinstiegFact{LabelKey: "einstieg.fact.billing"}
	if in.Rate != nil {
		billing.Value = RateLabel(in.Rate)
		billing.Aside = in.RateSource
	} else {
		billing.Value = components.T(ctx, "einstieg.billing.own")
	}
	facts = append(facts, billing)
	facts = append(facts, EinstiegFact{
		LabelKey: "einstieg.fact.total",
		Value:    fmtDurHM(in.Rollup.Total),
		Mono:     true,
		Aside:    components.T(ctx, "einstieg.fact.sinceStart"),
	})
	if in.Rate != nil {
		facts = append(facts, EinstiegFact{
			LabelKey: "cockpit.ov.earnings",
			Value:    in.Rate.Mul(in.Rollup.Total).String(),
		})
	}
	if in.N.Kind == domain.KindRepo && in.N.UpstreamGit != "" {
		facts = append(facts, EinstiegFact{
			LabelKey: "node.upstream",
			Value:    gitDisplay(in.N.UpstreamGit),
		})
	}
	if len(in.Contributors) > 0 {
		facts = append(facts, EinstiegFact{
			LabelKey: "cockpit.rail.contributors",
			Value:    strings.Join(in.Contributors, ", "),
		})
	}
	out.Facts = facts

	// --- Kinder / LoseRepos ---
	var kinderNodes, loseRepoNodes []domain.Node
	switch in.N.Kind {
	case domain.KindEngagement:
		out.KinderLabelKey = "einstieg.vorhaben"
		out.KinderHintKey = "einstieg.vorhaben.hint"
		for _, c := range directChildren {
			switch c.Kind {
			case domain.KindVorhaben:
				kinderNodes = append(kinderNodes, c)
			case domain.KindRepo:
				loseRepoNodes = append(loseRepoNodes, c)
			}
		}
	case domain.KindVorhaben:
		out.KinderLabelKey = "einstieg.projekte"
		for _, c := range directChildren {
			if c.Kind == domain.KindRepo {
				kinderNodes = append(kinderNodes, c)
			}
		}
	}

	if in.SortByName {
		sort.SliceStable(kinderNodes, func(i, j int) bool { return kinderNodes[i].Name < kinderNodes[j].Name })
		out.SortHref = ""                // toggle link leads back to the default
		out.SortStateHref = "?sort=name" // current state — the Kasten reload must keep it (F1/C1)
		out.SortLabelKey = "einstieg.sort.name"
	} else {
		sort.SliceStable(kinderNodes, func(i, j int) bool {
			ti, oki := lastChange[kinderNodes[i].ID]
			tj, okj := lastChange[kinderNodes[j].ID]
			if oki != okj {
				return oki
			}
			if oki && okj && !ti.Equal(tj) {
				return ti.After(tj)
			}
			return kinderNodes[i].Name < kinderNodes[j].Name
		})
		out.SortHref = "?sort=name" // toggle link leads to the name sort
		out.SortStateHref = ""      // current state (default) — the Kasten reload carries no query
		out.SortLabelKey = "einstieg.sort.changed"
	}
	sort.SliceStable(loseRepoNodes, func(i, j int) bool { return loseRepoNodes[i].Name < loseRepoNodes[j].Name })

	kinderSet := make(map[string]bool, len(kinderNodes))
	for _, c := range kinderNodes {
		kinderSet[c.ID] = true
	}
	var runningChild string
	if in.RunningNodeID != "" {
		runningChild, _ = walkToChild(in.RunningNodeID, kinderSet, subtreeParents)
	}

	var kinder []EinstiegKindRow
	for _, c := range kinderNodes {
		row := EinstiegKindRow{
			ID: c.ID, Name: c.Name, Kind: c.Kind, Color: c.Color,
			Dimmed: c.Status == domain.NodePaused || c.Status == domain.NodeArchived,
			Cards:  docTotals[c.ID],
			Open:   c.ID == runningChild,
		}
		if t, ok := lastChange[c.ID]; ok {
			row.When = FmtStaffel(ctx, t, in.Now)
		}
		if c.ID == runningChild {
			row.RunningDur = fmtClock(time.Duration(in.RunningBase) * time.Second)
		}
		if c.Kind == domain.KindVorhaben {
			for _, r := range childrenByParent[c.ID] {
				if r.Kind == domain.KindRepo {
					row.Repos = append(row.Repos, buildRepoRow(r, c.ID, docTotals))
				}
			}
			sort.SliceStable(row.Repos, func(i, j int) bool { return row.Repos[i].Slug < row.Repos[j].Slug })
		}
		kinder = append(kinder, row)
	}
	out.Kinder = kinder

	var loseRepos []EinstiegRepoRow
	for _, r := range loseRepoNodes {
		loseRepos = append(loseRepos, buildRepoRow(r, "", docTotals))
	}
	out.LoseRepos = loseRepos

	// --- Buchungen ---
	monthStart := time.Date(in.Now.Year(), in.Now.Month(), 1, 0, 0, 0, 0, in.Now.Location())
	byChild, own := MonthSumsByChild(in.N.ID, directChildren, subtreeParents, in.Sessions, monthStart, in.Now)
	span := fmt.Sprintf("01.–%02d.%02d.", in.Now.Day(), int(in.Now.Month()))
	type buchungEntry struct {
		title string
		dur   time.Duration
	}
	var entries []buchungEntry
	for _, c := range directChildren {
		if d := byChild[c.ID]; d > 0 {
			entries = append(entries, buchungEntry{c.Name, d})
		}
	}
	if own > 0 {
		entries = append(entries, buchungEntry{components.T(ctx, "einstieg.buchungen.own"), own})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].dur > entries[j].dur })
	var buchungen []EinstiegBuchungRow
	for _, e := range entries {
		row := EinstiegBuchungRow{Title: e.title, DurStr: fmtDurHM(e.dur)}
		if in.Rate != nil {
			row.Amount = in.Rate.Mul(e.dur).String()
		}
		buchungen = append(buchungen, row)
	}
	out.Buchungen = buchungen
	if len(buchungen) > 0 {
		out.BuchungenSpan = span
	}

	// --- Highlights (subtree-filtered, capped at 5 — Spec C.2.4) ---
	docIDs := make(map[string]bool, len(in.Docs))
	for _, d := range in.Docs {
		docIDs[d.ID] = true
	}
	var highlights []EinstiegHighlightRow
	for _, h := range in.Highlights {
		if !subtreeIDs[h.NodeID] {
			continue
		}
		row := EinstiegHighlightRow{
			Day:   FmtStaffel(ctx, h.CreatedAt, in.Now),
			Quote: h.Quote,
			Where: nameByID[h.NodeID],
		}
		if docIDs[h.DocumentID] {
			row.Href = "/wissen/" + h.DocumentID
		}
		highlights = append(highlights, row)
		if len(highlights) == 5 {
			break
		}
	}
	out.Highlights = highlights

	// --- Feed (subtree-filtered, capped at 10 — Spec C.2.5) ---
	docTypes := make(map[string]domain.DocumentType, len(in.Docs))
	for _, d := range in.Docs {
		docTypes[d.ID] = d.Type
	}
	var feed []EinstiegFeedRow
	for _, e := range FilterPulse(in.Activity, subtreeIDs) {
		label, tone := einstiegChip(ctx, e, docTypes)
		row := EinstiegFeedRow{
			When:      fmtFeedTime(e.At, in.Now),
			ChipLabel: label,
			ChipClass: paletteTypeTextClass(tone),
		}
		if e.Label != nil {
			row.Title = *e.Label
		}
		switch {
		case strings.HasPrefix(e.Kind, "document.") && e.TargetRef != nil:
			row.Href = "/wissen/" + *e.TargetRef
		case strings.HasPrefix(e.Kind, "node.") && e.TargetRef != nil:
			row.Href = "/nodes/" + *e.TargetRef
		case strings.HasPrefix(e.Kind, "session.") && e.NodeRef != nil:
			row.Href = "/nodes/" + *e.NodeRef
		}
		if e.NodeRef != nil {
			if vid, ok := walkToChild(*e.NodeRef, level1, subtreeParents); ok {
				row.Vorhaben = nameByID[vid]
			}
		}
		feed = append(feed, row)
		if len(feed) == 10 {
			break
		}
	}
	out.Feed = feed

	return out
}

// einstiegSquareClass is the Ebenen tone of a list marker square (Screen 02's
// 9px steel square in front of a repo line). Spelled out per case — not
// string-concatenated — so the Tailwind scanner finds every literal it needs
// (same convention as ebeneStripeClass / monogramClass).
func einstiegSquareClass(k domain.NodeKind) string {
	switch k {
	case domain.KindEngagement:
		return "bg-amber"
	case domain.KindVorhaben:
		return "bg-violet"
	default:
		return "bg-steel"
	}
}

// buildRepoRow converts a repo node to its EinstiegRepoRow display line.
func buildRepoRow(r domain.Node, parentID string, docTotals map[string]int) EinstiegRepoRow {
	return EinstiegRepoRow{
		ID: r.ID, Name: r.Name, Slug: r.Slug, ParentID: parentID,
		Upstream: gitDisplay(r.UpstreamGit),
		Cards:    docTotals[r.ID],
	}
}

// einstiegRepoName is the repo line's leading label: the display name, or
// the slug when a repo carries no name of its own. One node, one name
// (Karteikasten-Kompass A2).
func einstiegRepoName(row EinstiegRepoRow) string {
	if row.Name != "" {
		return row.Name
	}
	return row.Slug
}

// fmtAge renders the Facts "Besteht seit" right column ("2 J. 5 M." / "5 M."
// / "2 J.") from AgeYearsMonths — the months half is dropped once years
// carries the story on its own, and years is dropped entirely below one year.
func fmtAge(ctx context.Context, years, months int) string {
	y := components.T(ctx, "einstieg.age.years")
	m := components.T(ctx, "einstieg.age.months")
	switch {
	case years > 0 && months > 0:
		return itoa(years) + " " + y + " " + itoa(months) + " " + m
	case years > 0:
		return itoa(years) + " " + y
	default:
		return itoa(months) + " " + m
	}
}

// SubtreeLastChange returns, per node, the newest moment anything in its
// SUBTREE changed: a node's own UpdatedAt, a document's UpdatedAt, or a
// session's end (its Start while it is still running — a running session's
// "change" is now, not when it began). ONE pass per source, walking each hit
// up the parent chain (same walk as SubtreeDocTotals). Nodes that never
// changed are absent from the map, so callers can tell "never" from "long ago".
func SubtreeLastChange(nodes []domain.Node, docs []domain.Document, sessions []domain.WorkSession, now time.Time) map[string]time.Time {
	parent := make(map[string]*string, len(nodes))
	for _, n := range nodes {
		parent[n.ID] = n.ParentID
	}
	out := make(map[string]time.Time, len(nodes))
	bubble := func(id string, t time.Time) {
		cur := id
		seen := map[string]bool{}
		for {
			p, ok := parent[cur]
			if !ok || seen[cur] {
				return
			}
			seen[cur] = true
			if t.After(out[cur]) {
				out[cur] = t
			}
			if p == nil {
				return
			}
			cur = *p
		}
	}
	for _, n := range nodes {
		bubble(n.ID, n.UpdatedAt)
	}
	for _, d := range docs {
		if d.NodeID == nil {
			continue
		}
		bubble(*d.NodeID, d.UpdatedAt)
	}
	for _, s := range sessions {
		if s.NodeID == nil {
			continue
		}
		t := now
		if s.Stop != nil {
			t = *s.Stop
		}
		bubble(*s.NodeID, t)
	}
	return out
}

// MonthSumsByChild splits the entry node's current-month subtree time across
// its direct children, plus "own" for sessions booked to the entry node
// itself. ONE pass over the sessions; each session walks up to the child that
// contains it (walkToChild). Sessions outside [monthStart, now] are skipped.
func MonthSumsByChild(entryID string, children []domain.Node, parents map[string]string, sessions []domain.WorkSession, monthStart, now time.Time) (byChild map[string]time.Duration, own time.Duration) {
	childSet := make(map[string]bool, len(children))
	for _, c := range children {
		childSet[c.ID] = true
	}
	byChild = make(map[string]time.Duration, len(children))
	for _, s := range sessions {
		if s.NodeID == nil {
			continue
		}
		if s.Start.Before(monthStart) || s.Start.After(now) {
			continue
		}
		id := *s.NodeID
		el := s.Elapsed(now)
		if id == entryID {
			own += el
			continue
		}
		if cid, ok := walkToChild(id, childSet, parents); ok {
			byChild[cid] += el
		}
	}
	return byChild, own
}

// FirstBookingStart returns the earliest session start inside ids, and false
// when the subtree has never been booked.
func FirstBookingStart(sessions []domain.WorkSession, ids map[string]bool) (time.Time, bool) {
	var earliest time.Time
	found := false
	for _, s := range sessions {
		if s.NodeID == nil || !ids[*s.NodeID] {
			continue
		}
		if !found || s.Start.Before(earliest) {
			earliest = s.Start
			found = true
		}
	}
	return earliest, found
}

// AgeYearsMonths returns whole years and remaining whole months between from
// and now, clamped at zero — the quiet right column of "Besteht seit".
func AgeYearsMonths(from, now time.Time) (years, months int) {
	if from.After(now) {
		return 0, 0
	}
	y1, m1, d1 := from.Date()
	y2, m2, d2 := now.Date()
	years = y2 - y1
	months = int(m2) - int(m1)
	if d2 < d1 {
		months--
	}
	if months < 0 {
		years--
		months += 12
	}
	if years < 0 {
		return 0, 0
	}
	return years, months
}

// EinstiegSince renders a coarse relative age for the README head: "vor 12
// Minuten" / "vor 5 Stunden" / "vor 6 Tagen" / "vor 3 Monaten". The Bestand's
// fmtRelTime stops at hours and then falls back to a date, which is exactly
// the "when" the head does NOT want — so this is its own helper rather than a
// change to the Home feed's formatter. Plural forms via Tn, both catalogues.
func EinstiegSince(ctx context.Context, at, now time.Time) string {
	diff := now.Sub(at)
	if diff < 0 {
		diff = 0
	}
	switch {
	case diff < time.Hour:
		n := int(diff.Minutes())
		if n < 1 {
			n = 1
		}
		return components.Tn(ctx, "einstieg.since.minutes", n)
	case diff < 24*time.Hour:
		n := int(diff.Hours())
		if n < 1 {
			n = 1
		}
		return components.Tn(ctx, "einstieg.since.hours", n)
	case diff < 30*24*time.Hour:
		n := int(diff.Hours() / 24)
		if n < 1 {
			n = 1
		}
		return components.Tn(ctx, "einstieg.since.days", n)
	default:
		n := int(diff.Hours() / 24 / 30)
		if n < 1 {
			n = 1
		}
		return components.Tn(ctx, "einstieg.since.months", n)
	}
}

// PercentDelta renders cur against prev as a signed whole percentage
// ("+18 %", "−4 %", "±0 %"). prev <= 0 yields "" — there is nothing to
// compare against, and "+100 %" against zero would be a lie.
func PercentDelta(cur, prev time.Duration) string {
	if prev <= 0 {
		return ""
	}
	pct := int(math.Round(float64(cur-prev) / float64(prev) * 100))
	switch {
	case pct > 0:
		return fmt.Sprintf("+%d %%", pct)
	case pct < 0:
		return fmt.Sprintf("−%d %%", -pct)
	default:
		return "±0 %"
	}
}

// fmtDurDecimal renders a duration as decimal hours with a German comma and at
// most one decimal ("18,5", "20"). Screen 02's week cell is the only place in
// the box that is NOT clock time — it stands next to a decimal weekly target.
func fmtDurDecimal(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := strconv.FormatFloat(d.Hours(), 'f', 1, 64)
	s = strings.TrimSuffix(s, ".0")
	return strings.ReplaceAll(s, ".", ",")
}

// einstiegChip maps an activity entry to its Screen-02 type chip: the resolved
// label and the Karteikasten tone (TOKENS.md Kartentypen). docTypes resolves a
// document.* entry's target to its Regal. An UNKNOWN kind gets a generic chip
// carrying the kind's own abbreviation (Spec C.2.5: "unbekannte Kinds →
// generischer Chip mit Kind-Kürzel") — uppercased first path segment, capped
// at 7 runes — never a translated catch-all word, which would hide what
// actually happened.
func einstiegChip(ctx context.Context, e domain.ActivityEntry, docTypes map[string]domain.DocumentType) (label, tone string) {
	switch {
	case strings.HasPrefix(e.Kind, "session."):
		return components.T(ctx, "einstieg.chip.zeit"), "live"
	case strings.HasPrefix(e.Kind, "document."):
		var dt domain.DocumentType
		if e.TargetRef != nil {
			dt = docTypes[*e.TargetRef]
		}
		switch dt {
		case domain.DocPlan:
			return components.T(ctx, "einstieg.chip.plan"), "purple"
		case domain.DocSpec:
			return components.T(ctx, "einstieg.chip.spec"), "teal"
		case domain.DocMemory:
			return components.T(ctx, "einstieg.chip.erinnerung"), "red"
		case domain.DocActiveContext:
			return components.T(ctx, "einstieg.chip.kontext"), "accent"
		case domain.DocDaily:
			return components.T(ctx, "einstieg.chip.tagebuch"), "green"
		default:
			return components.T(ctx, "einstieg.chip.notiz"), "blue"
		}
	default:
		seg := e.Kind
		if i := strings.Index(seg, "."); i >= 0 {
			seg = seg[:i]
		}
		seg = strings.ToUpper(seg)
		if r := []rune(seg); len(r) > 7 {
			seg = string(r[:7])
		}
		return seg, "accent"
	}
}
