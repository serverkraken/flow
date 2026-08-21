package webui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// TreeRow is one rendered line of the node tree: the node plus its depth (0 =
// engagement root) so the template can indent it. Hours is the subtree hour
// badge ("41h"; empty under 1h).
type TreeRow struct {
	Level int
	Node  domain.Node
	Hours string
}

// buildNodeTree turns a flat node slice into a depth-first, indented row list:
// engagement roots first, each followed by its subtree; siblings are ordered by
// name. It is cycle- and orphan-safe — any node whose parent is absent (or that
// would re-enter a visited subtree) is surfaced at level 0 so nothing is hidden.
func buildNodeTree(nodes []domain.Node) []TreeRow {
	children := map[string][]domain.Node{}
	for _, n := range nodes {
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	for k := range children {
		sort.SliceStable(children[k], func(i, j int) bool {
			return children[k][i].Name < children[k][j].Name
		})
	}
	var out []TreeRow
	seen := map[string]bool{}
	var walk func(parentKey string, level int)
	walk = func(parentKey string, level int) {
		for _, n := range children[parentKey] {
			if seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			out = append(out, TreeRow{Level: level, Node: n})
			walk(n.ID, level+1)
		}
	}
	walk("", 0)
	// Orphans (parent not in the set) — defensive: never drop a node.
	for _, n := range nodes {
		if !seen[n.ID] {
			seen[n.ID] = true
			out = append(out, TreeRow{Level: 0, Node: n})
		}
	}
	return out
}

// ValidParentsFor returns the nodes that may host a child of the given kind,
// name-ordered. Engagement is always a root → empty result.
func ValidParentsFor(kind domain.NodeKind, all []domain.Node) []domain.Node {
	if kind == domain.KindEngagement {
		return nil
	}
	var out []domain.Node
	for _, n := range all {
		if domain.AllowedChildKind(n.Kind, kind) {
			out = append(out, n)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// descendantIDs returns id plus every node in its subtree. Move targets must be
// excluded from this set to keep reparenting acyclic.
func descendantIDs(all []domain.Node, id string) map[string]bool {
	children := map[string][]string{}
	for _, n := range all {
		if n.ParentID != nil {
			children[*n.ParentID] = append(children[*n.ParentID], n.ID)
		}
	}
	out := map[string]bool{id: true}
	var walk func(string)
	walk = func(cur string) {
		for _, c := range children[cur] {
			if !out[c] {
				out[c] = true
				walk(c)
			}
		}
	}
	walk(id)
	return out
}

// ---- page/form/cockpit view models (rendered by D3–D6) ----

// NodesPageData is the Projekte page's view model (node-management home):
// the tree-as-content ProjectsVM (Task 3) plus the active status filter.
type NodesPageData struct {
	User   string
	Status string // "" (active+paused) | "archived" | "all"
	VM     ProjectsVM
}

// NodeFormValues holds raw create/edit form fields (re-rendered on error).
type NodeFormValues struct {
	Name, Slug, Kind, ParentID       string
	Description, UpstreamGit, Status string
	Color                            string
	RateAmount, RateCurrency         string
	TagsCSV                          string // space-separated tag slugs for the tags input
	CountsMode                       string // Work/Privat tri-state: ""/"inherit" | "work" | "privat"
}

// NodeFormData drives the create (editing==nil) / edit form.
type NodeFormData struct {
	User    string
	Error   string
	Vals    NodeFormValues
	Parents []domain.Node // candidate parents (engagements + vorhaben)
	// MoveTargets is only filled in edit mode (handleWebNodeEdit) — valid new
	// parents for the inline Move form (Task 7: moved here from the cockpit
	// page's old cockpitMoveForm). "" (root) is always offered separately by
	// the template.
	MoveTargets []domain.Node
}

// NodeMoveData drives the inline move form on the cockpit page.
type NodeMoveData struct {
	User    string
	N       domain.Node
	Targets []domain.Node // valid new parents (descendant IDs excluded)
}

// moveTargetsFor returns valid new parents for n: parents allowed by kind,
// excluding n and its subtree (keeps reparenting acyclic).
func moveTargetsFor(all []domain.Node, n domain.Node) []domain.Node {
	sub := descendantIDs(all, n.ID)
	var out []domain.Node
	for _, p := range ValidParentsFor(n.Kind, all) {
		if !sub[p.ID] {
			out = append(out, p)
		}
	}
	return out
}

// MoveTargetsFor is the exported entry point for the httpserver adapter.
func MoveTargetsFor(all []domain.Node, n domain.Node) []domain.Node { return moveTargetsFor(all, n) }

// BuildTree is the exported entry point used by the httpserver adapter.
func BuildTree(nodes []domain.Node) []TreeRow { return buildNodeTree(nodes) }

// SubtreeHourTotals aggregates each node's SUBTREE worktime in ONE pass over
// the owner's sessions (owner-scoped by construction — callers pass one
// owner's nodes+sessions): every session adds its elapsed time to its node
// and all ancestors. Unbooked sessions are skipped.
func SubtreeHourTotals(nodes []domain.Node, sessions []domain.WorkSession, now time.Time) map[string]time.Duration {
	parent := make(map[string]*string, len(nodes))
	for _, n := range nodes {
		parent[n.ID] = n.ParentID
	}
	totals := make(map[string]time.Duration, len(nodes))
	for _, s := range sessions {
		if s.NodeID == nil {
			continue
		}
		el := s.Elapsed(now)
		if el <= 0 {
			continue
		}
		id := *s.NodeID
		seen := make(map[string]bool, len(parent))
		for !seen[id] { // defensively stop at corrupt parent cycles
			seen[id] = true
			p, ok := parent[id]
			if !ok {
				break // node not visible (archived/foreign) — stop the walk
			}
			totals[id] += el
			if p == nil {
				break
			}
			id = *p
		}
	}
	return totals
}

// FillTreeHours stamps rows with a compact subtree-hours badge ("41h");
// under one hour the badge stays empty to keep the tree calm.
func FillTreeHours(rows []TreeRow, totals map[string]time.Duration) {
	for i := range rows {
		if h := int(totals[rows[i].Node.ID].Hours()); h >= 1 {
			rows[i].Hours = fmt.Sprintf("%dh", h)
		}
	}
}

// NodeSelectOptions builds hierarchy-ordered <select> options for a node picker:
// each option is depth-indented (engagement → vorhaben → repo) and carries the
// node's kind glyph + localized kind label, so a flat dropdown no longer hides
// the type and parent/child structure. Used by the document editor's "Projekt"
// picker (and reusable by any node <select>).
func NodeSelectOptions(ctx context.Context, nodes []domain.Node) []EditorOption {
	rows := buildNodeTree(nodes)
	out := make([]EditorOption, 0, len(rows))
	for _, row := range rows {
		b := NodeKindStyle(row.Node.Kind)
		indent := strings.Repeat("  ", row.Level) // non-breaking spaces survive in <option>
		label := indent + b.Glyph + " " + row.Node.Name + " · " + components.T(ctx, b.LabelKey)
		out = append(out, EditorOption{Value: row.Node.ID, Label: label})
	}
	return out
}

// nodeFilterChip returns Tailwind chip classes for the filter bar; active = ink
// background, inactive = muted text with hover accent.
func nodeFilterChip(active bool) string {
	if active {
		return "rounded-full bg-ink px-3 py-1 text-xs font-medium text-canvas"
	}
	return "rounded-full border border-line bg-surface px-3 py-1 text-xs font-medium text-muted hover:border-blue/40 hover:text-blue"
}

// nodeFormAction returns the form POST target for create (/nodes) or edit (/nodes/{id}).
func nodeFormAction(editing *domain.Node) templ.SafeURL {
	if editing != nil {
		return templ.SafeURL("/nodes/" + editing.ID)
	}
	return templ.SafeURL("/nodes")
}

// nodeParentLabel returns the display label for a parent candidate.
func nodeParentLabel(p domain.Node) string { return p.Name }

// ---------------------------------------------------------------------------
// Helpers shared by nodes.templ templates.
// ---------------------------------------------------------------------------

// orDefault returns v if non-empty, otherwise def.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// nodeCrumbs baut die K3Crumb-Spur aus der Ahnenkette: einen "‹ zurück"-Chip
// für den unmittelbaren Elternteil (nil bei einem Wurzel-Engagement), die
// übrigen Ahnen plus den aktuellen Knoten als Segmente — jedes mit seiner
// Ebene als Punktfarbe —, und die Ebene des aktuellen Knotens als
// rechtsbündige Marke. Der Elternteil steht nie doppelt: er ist schon der
// Zurück-Chip.
func nodeCrumbs(d NodeCockpit) (*components.Crumb, []components.Crumb, string) {
	n := len(d.Ancestors)
	chain := make([]domain.Node, 0, n)
	for i := n - 1; i >= 0; i-- { // Ancestors ist Blatt→Wurzel; wir laufen Wurzel→Blatt.
		chain = append(chain, d.Ancestors[i])
	}
	if len(chain) == 0 {
		chain = []domain.Node{d.N} // ohne geladene Kette: wenigstens der Knoten selbst
	}
	var back *components.Crumb
	items := make([]components.Crumb, 0, len(chain))
	for i, a := range chain {
		isCurrent := a.ID == d.N.ID
		if !isCurrent && i == len(chain)-2 {
			back = &components.Crumb{Href: "/nodes/" + a.ID, Label: a.Name}
			continue
		}
		c := components.Crumb{Label: a.Name, Level: string(a.Kind)}
		if !isCurrent {
			c.Href = "/nodes/" + a.ID
		}
		items = append(items, c)
	}
	return back, items, string(d.N.Kind)
}

// SubtreeDocTotals zählt Dokumente je Knoten EINSCHLIESSLICH seines Teilbaums
// — die Kartenzähler der Schiene (eine Karte am Repo zählt über Vorhaben und
// Engagement mit hoch). Dokumente an abwesenden oder fremden Knoten brechen
// den Aufstieg gefahrlos ab.
func SubtreeDocTotals(nodes []domain.Node, docs []domain.Document) map[string]int {
	parent := make(map[string]*string, len(nodes))
	for _, n := range nodes {
		parent[n.ID] = n.ParentID
	}
	totals := make(map[string]int, len(nodes))
	for _, d := range docs {
		if d.NodeID == nil {
			continue
		}
		id := *d.NodeID
		seen := map[string]bool{}
		for {
			p, ok := parent[id]
			if !ok || seen[id] {
				break
			}
			seen[id] = true
			totals[id]++
			if p == nil {
				break
			}
			id = *p
		}
	}
	return totals
}
