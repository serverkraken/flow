package webui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// ProjectsVM is the view model for the Projekte page (/nodes): the node tree
// rendered as Lesesaal content rather than a sidebar — engagement headers,
// vorhaben sub-heads, repo rows with short name + full mono path (Mockup
// Z.442–570).
type ProjectsVM struct {
	CountEng      int
	CountVor      int
	CountRepo     int
	TotalHoursStr string
	Engagements   []EngagementSection
}

// EngagementSection is one .eng block: the engagement header plus its
// vorhaben groups and any repos parented directly on the engagement.
type EngagementSection struct {
	N           domain.Node
	Initials    string
	Tone        string
	HoursStr    string
	RateNote    string
	Groups      []VorhabenGroup
	DirectRepos []ProjRow
}

// VorhabenGroup is one .vh block: a vorhaben's own repo (and nested
// sub-vorhaben) rows.
type VorhabenGroup struct {
	N         domain.Node
	Short     string
	CountNote string
	Rows      []ProjRow
}

// ProjRow is one .projrow line: a repo, or — when IsVorhaben — a nested
// sub-vorhaben shown inline in its parent vorhaben's group rather than as its
// own .vh head (Mockup Z.474–484 "infra" → "base-infra"/"k8s-infra"). For an
// IsVorhaben row, Full carries the parent vorhaben's short name (rendered as
// "unter <Full>" via projRowUnderNote) instead of a git path.
type ProjRow struct {
	ID, Short, Full, Initials, Tone, KindLabel string
	LogoRef                                    string // domain.Node.LogoRef, "" = no logo (NodeAvatar)
	Desc                                        string // domain.Node.Description, "" = no subtitle line (Task 5, OE #7)
	RightV, RightK                              string
	IsVorhaben                                  bool
	PathWarn                                    bool
}

// BuildProjectsVM turns the owner's flat node list into the Projekte page's
// tree-as-content view model. sessions/docCounts/running feed the per-row
// hours / doc-count / "Timer läuft" note; a nil/empty value there degrades
// those notes to "—"/quiet rather than failing — the handler is expected to
// degrade silently on a failed side-source (brief §Zustände "Request-Fehler")
// and still call this with whatever it has.
//
// RightK carries an i18n KEY (not translated text) since this function has no
// ctx to resolve one — the template resolves it via components.T at render
// time. RightV / CountNote / rate notes stay locale-neutral compact tokens
// ("41h", "6 Docs", "95.00 EUR/h") in the same spirit as the existing Hours
// badge — no new i18n key was requested for these in the plan.
func BuildProjectsVM(nodes []domain.Node, sessions []domain.WorkSession, docCounts map[string]int, running *domain.WorkSession, now time.Time) ProjectsVM {
	totals := SubtreeHourTotals(nodes, sessions, now)
	runningID := ""
	if running != nil && running.NodeID != nil {
		runningID = *running.NodeID
	}

	byID := make(map[string]domain.Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	childrenOf := map[string][]domain.Node{}
	for _, n := range nodes {
		if n.ParentID == nil {
			continue
		}
		if _, ok := byID[*n.ParentID]; !ok {
			continue // orphan/foreign parent — surfaced as its own section below
		}
		childrenOf[*n.ParentID] = append(childrenOf[*n.ParentID], n)
	}
	for k := range childrenOf {
		sort.SliceStable(childrenOf[k], func(i, j int) bool { return childrenOf[k][i].Name < childrenOf[k][j].Name })
	}

	var engs []domain.Node
	for _, n := range nodes {
		if n.Kind == domain.KindEngagement {
			engs = append(engs, n)
		}
	}
	sort.SliceStable(engs, func(i, j int) bool { return engs[i].Name < engs[j].Name })

	seen := map[string]bool{}
	var sections []EngagementSection
	var totalVorhaben, totalRepos int
	var sumHours time.Duration

	for _, eng := range engs {
		seen[eng.ID] = true
		sec, vc, rc := buildEngagementSection(eng, childrenOf, totals, docCounts, runningID)
		sections = append(sections, sec)
		totalVorhaben += vc
		totalRepos += rc
		sumHours += totals[eng.ID]
		markSubtreeSeen(seen, eng.ID, childrenOf)
	}

	// Orphan fallback: a node whose ancestor chain never reaches a visible
	// engagement (foreign/absent parent, or a bare Vorhaben/Repo root) is
	// still surfaced — as its own pseudo-engagement section — so nothing is
	// silently dropped (buildNodeTree's orphan rule, applied here).
	var orphans []domain.Node
	for _, n := range nodes {
		if !seen[n.ID] {
			orphans = append(orphans, n)
		}
	}
	sort.SliceStable(orphans, func(i, j int) bool { return orphans[i].Name < orphans[j].Name })
	for _, n := range orphans {
		if seen[n.ID] {
			continue // may have been swept in while walking an earlier orphan's subtree
		}
		seen[n.ID] = true
		sec, vc, rc := buildEngagementSection(n, childrenOf, totals, docCounts, runningID)
		sections = append(sections, sec)
		totalVorhaben += vc
		totalRepos += rc
		sumHours += totals[n.ID]
		markSubtreeSeen(seen, n.ID, childrenOf)
	}

	return ProjectsVM{
		CountEng:      len(engs),
		CountVor:      totalVorhaben,
		CountRepo:     totalRepos,
		TotalHoursStr: FmtDurHMExport(sumHours),
		Engagements:   sections,
	}
}

// buildEngagementSection builds one .eng section rooted at root (a real
// engagement, or an orphan node promoted to section level) plus the
// vorhaben/repo counts it contributes to the page Summary.
func buildEngagementSection(root domain.Node, childrenOf map[string][]domain.Node, totals map[string]time.Duration, docCounts map[string]int, runningID string) (EngagementSection, int, int) {
	var vorhabenKids, repoKids []domain.Node
	for _, k := range childrenOf[root.ID] {
		if k.Kind == domain.KindVorhaben {
			vorhabenKids = append(vorhabenKids, k)
		} else {
			repoKids = append(repoKids, k)
		}
	}

	// DisplayNames is called ONCE per engagement's visible scope (Spec
	// §5.5): every row-name that will actually render under this engagement
	// (each group's direct children + the direct repos), not per-group —
	// otherwise a "gitlab/group" collision between two different vorhaben
	// groups would go undetected.
	var scopeNames []string
	for _, v := range vorhabenKids {
		for _, k := range childrenOf[v.ID] {
			scopeNames = append(scopeNames, k.Name)
		}
	}
	for _, r := range repoKids {
		scopeNames = append(scopeNames, r.Name)
	}
	shortOf := DisplayNames(scopeNames)

	vorhabenCount, repoCount := 0, 0
	var groups []VorhabenGroup
	for _, v := range vorhabenKids {
		vorhabenCount++
		group, gv, gr := buildVorhabenGroup(v, childrenOf, totals, docCounts, runningID, shortOf)
		vorhabenCount += gv
		repoCount += gr
		groups = append(groups, group)
	}

	var directRepos []ProjRow
	for _, r := range repoKids {
		repoCount++
		directRepos = append(directRepos, buildProjRow(r, totals, docCounts, runningID, shortOf))
	}

	sec := EngagementSection{
		N:           root,
		Initials:    Initials(ShortName(root.Name)),
		Tone:        AvatarTone(root.Name),
		HoursStr:    FmtDurHMExport(totals[root.ID]),
		RateNote:    rateNote(root),
		Groups:      groups,
		DirectRepos: directRepos,
	}
	return sec, vorhabenCount, repoCount
}

// buildVorhabenGroup builds one .vh group for vorhaben v: its repo children
// become plain rows, its vorhaben children (sub-vorhaben, recursive per
// domain.ValidParentKind) become inline IsVorhaben rows — deliberately NOT
// expanded further on this page (drill via the cockpit).
func buildVorhabenGroup(v domain.Node, childrenOf map[string][]domain.Node, totals map[string]time.Duration, docCounts map[string]int, runningID string, shortOf map[string]string) (VorhabenGroup, int, int) {
	var rows []ProjRow
	nestedVorhabenCount, repoCount := 0, 0
	for _, k := range childrenOf[v.ID] {
		if k.Kind == domain.KindVorhaben {
			nestedVorhabenCount++
			row := buildProjRow(k, totals, docCounts, runningID, shortOf)
			row.IsVorhaben = true
			row.KindLabel = string(domain.KindVorhaben)
			row.Full = ShortName(v.Name)
			row.PathWarn = false // no path is rendered for a nested-vorhaben row
			rows = append(rows, row)
			continue
		}
		repoCount++
		rows = append(rows, buildProjRow(k, totals, docCounts, runningID, shortOf))
	}
	countNote := fmt.Sprintf("%d Repos", repoCount)
	if nestedVorhabenCount > 0 {
		countNote = fmt.Sprintf("%d Vorhaben", nestedVorhabenCount)
	}
	return VorhabenGroup{N: v, Short: ShortName(v.Name), CountNote: countNote, Rows: rows}, nestedVorhabenCount, repoCount
}

// buildProjRow builds one repo row: dedup'd short name (from the
// engagement-scoped shortOf map), the doc-count/hours right-value, and the
// timer/pathWarn/quiet right-key note (an i18n key resolved by the template).
func buildProjRow(n domain.Node, totals map[string]time.Duration, docCounts map[string]int, runningID string, shortOf map[string]string) ProjRow {
	short := shortOf[n.Name]
	if short == "" {
		short = ShortName(n.Name)
	}
	warn := pathWarn(n.Name)
	row := ProjRow{
		ID:       n.ID,
		Short:    short,
		Full:     n.Name,
		Initials: Initials(short),
		Tone:     AvatarTone(n.Name),
		LogoRef:  n.LogoRef,
		Desc:     n.Description,
		PathWarn: warn,
	}
	switch {
	case docCounts[n.ID] > 0:
		row.RightV = fmt.Sprintf("%d Docs", docCounts[n.ID])
	case totals[n.ID] > 0:
		row.RightV = FmtDurHMExport(totals[n.ID])
	default:
		row.RightV = "—"
	}
	switch {
	case runningID != "" && n.ID == runningID:
		row.RightK = "nodes.row.timerRunning"
	case warn:
		row.RightK = "nodes.row.pathWarn"
	default:
		row.RightK = "nodes.row.quiet"
	}
	return row
}

// markSubtreeSeen marks id's whole subtree (however deep) as visited so
// deeper, deliberately-unexpanded descendants (e.g. a repo two levels under a
// sub-vorhaben) are never mistaken for a dropped/orphan node.
func markSubtreeSeen(seen map[string]bool, id string, childrenOf map[string][]domain.Node) {
	for _, k := range childrenOf[id] {
		if seen[k.ID] {
			continue
		}
		seen[k.ID] = true
		markSubtreeSeen(seen, k.ID, childrenOf)
	}
}

// pathWarn flags a name that looks like corrupted remote-path data (Spec §9):
// a stray "> " (redirect/copy-paste artifact) or a doubled "//" segment.
func pathWarn(name string) bool {
	return strings.ContainsAny(name, "> ") || strings.Contains(name, "//")
}

// rateNote renders an engagement's own resolved rate as a compact "<amount>
// <currency>/h" note ("" when unset) — engagements are tree roots, so their
// own Rate (if any) is always the effective one.
func rateNote(n domain.Node) string {
	if n.Rate == nil {
		return ""
	}
	return n.Rate.String() + "/h"
}

// repoCountNote renders the "Direkt am Engagement" group's repo count.
func repoCountNote(n int) string { return fmt.Sprintf("%d Repos", n) }

// projRowUnderNote composes a nested-vorhaben row's "unter <parent>" secondary
// line. i18n key nodes.row.under is an addition beyond the plan's literal
// Step-7 list (not spelled out there) but required for de/en parity on this
// user-facing text — see task completion report.
func projRowUnderNote(ctx context.Context, parentShort string) string {
	return fmt.Sprintf(components.T(ctx, "nodes.row.under"), parentShort)
}

// ProjectsSummary composes the Projekte page's headline count line via i18n
// (nodes.summary) from the VM's raw counts — the VM itself stays ctx-free
// (same convention as RightK/projRowUnderNote), so the templ resolves the
// locale-specific format string at render time instead of the VM baking in
// hardcoded German (Final-Review Finding 1).
func ProjectsSummary(ctx context.Context, vm ProjectsVM) string {
	return fmt.Sprintf(components.T(ctx, "nodes.summary"), vm.CountEng, vm.CountVor, vm.CountRepo, vm.TotalHoursStr)
}
