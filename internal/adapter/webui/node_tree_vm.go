package webui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// TreeRow is one rendered line of the node tree: the node plus its depth (0 =
// engagement root) so the template can indent it.
type TreeRow struct {
	Level int
	Node  domain.Node
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

// NodesPageData is the tree view model (node-management home).
type NodesPageData struct {
	User   string
	Status string // "" (active+paused) | "archived" | "all"
	Rows   []TreeRow
}

// NodeFormValues holds raw create/edit form fields (re-rendered on error).
type NodeFormValues struct {
	Name, Slug, Kind, ParentID       string
	Description, UpstreamGit, Status string
	Color, Glyph, Icon               string
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

// nodeIndentStyle returns an inline CSS padding-left for depth-based indentation
// in the tree (1 rem per level).
func nodeIndentStyle(level int) string { return fmt.Sprintf("padding-left:%drem", level) }

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

// gitDisplay normalises a git remote URL to a human-friendly "host/path" form.
// SSH  git@github.com:org/repo.git  → github.com/org/repo
// HTTPS https://github.com/org/repo.git → github.com/org/repo
// Anything else is returned unchanged.
func gitDisplay(raw string) string {
	if after, ok := strings.CutPrefix(raw, "git@"); ok {
		colonIdx := strings.Index(after, ":")
		if colonIdx > 0 {
			host := after[:colonIdx]
			path := strings.TrimSuffix(after[colonIdx+1:], ".git")
			return host + "/" + path
		}
	}
	for _, scheme := range []string{"https://", "http://"} {
		if after, ok := strings.CutPrefix(raw, scheme); ok {
			return strings.TrimSuffix(after, ".git")
		}
	}
	return raw
}

// orDefault returns v if non-empty, otherwise def.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// nodeCrumbs builds breadcrumb segments root→leaf from the leaf→root Ancestors
// chain returned by NodeStore.Ancestors. The current node (last in output) has
// no Href so it renders as "current page".
func nodeCrumbs(d NodeCockpit) []components.Crumb {
	var crumbs []components.Crumb
	for i := len(d.Ancestors) - 1; i >= 0; i-- {
		a := d.Ancestors[i]
		if a.ID == d.N.ID {
			crumbs = append(crumbs, components.Crumb{Label: a.Name})
		} else {
			crumbs = append(crumbs, components.Crumb{Href: "/nodes/" + a.ID, Label: a.Name})
		}
	}
	if len(crumbs) == 0 {
		// Ancestors empty (leaf node with no parent, or defensive fallback).
		crumbs = append(crumbs, components.Crumb{Label: d.N.Name})
	}
	return crumbs
}
