package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/timefmt"
)

// nodeKindGlyph maps a node kind to a monospace glyph. AGENTS.md bans emoji
// pictograms and sanctions ● ◆ ⬡ ▶ ■. Every line also prints the kind word, so
// the glyph is a scanning aid and never information a model has to decode.
func nodeKindGlyph(k domain.NodeKind) string {
	switch k {
	case domain.KindEngagement:
		return "●"
	case domain.KindVorhaben:
		return "◆"
	case domain.KindRepo:
		return "⬡"
	case domain.KindBranch:
		return "▶"
	default:
		return "·"
	}
}

// nodeTreeEntry is a node plus its name-sorted children.
type nodeTreeEntry struct {
	Node     domain.Node
	Children []*nodeTreeEntry
}

// buildNodeForest groups a flat node list into a parent→children forest. A node
// whose ParentID is nil is a root; a node whose ParentID points at an ID the
// list does not contain becomes a root too, rather than vanishing — hiding a
// node the owner does own would be worse than showing it unindented. Roots and
// every child level are name-sorted case-insensitively, true roots before
// dangling ones. Acyclicity is a server invariant (MoveNode rejects cycles with
// usecase.ErrNodeCycle), so the recursion always terminates.
func buildNodeForest(nodes []domain.Node) []*nodeTreeEntry {
	byID := make(map[string]*nodeTreeEntry, len(nodes))
	for i := range nodes {
		byID[nodes[i].ID] = &nodeTreeEntry{Node: nodes[i]}
	}
	var roots, dangling []*nodeTreeEntry
	// Iterate the slice, not the map, so the pre-sort order is deterministic.
	for i := range nodes {
		entry := byID[nodes[i].ID]
		pid := entry.Node.ParentID
		if pid == nil {
			roots = append(roots, entry)
			continue
		}
		if parent, ok := byID[*pid]; ok {
			parent.Children = append(parent.Children, entry)
			continue
		}
		dangling = append(dangling, entry)
	}
	var sortRec func(ts []*nodeTreeEntry)
	sortRec = func(ts []*nodeTreeEntry) {
		sort.Slice(ts, func(i, j int) bool {
			return strings.ToLower(ts[i].Node.Name) < strings.ToLower(ts[j].Node.Name)
		})
		for _, t := range ts {
			sortRec(t.Children)
		}
	}
	sortRec(roots)
	sortRec(dangling)
	return append(roots, dangling...)
}

// formatNodeTree renders the hierarchy indented two spaces per level, one line
// per node: kind glyph, name, slug, kind, status, id, and upstream when set.
// The flat alphabetical predecessor showed neither kind nor parent — exactly the
// information flow_create_node needs to pick a valid parent (Spec §3).
func formatNodeTree(nodes []domain.Node) string {
	if len(nodes) == 0 {
		return `No nodes yet. Create the first one with flow_create_node (kind="engagement", no parent).`
	}
	var b strings.Builder
	var walk func(ts []*nodeTreeEntry, depth int)
	walk = func(ts []*nodeTreeEntry, depth int) {
		for _, t := range ts {
			n := t.Node
			fmt.Fprintf(&b, "%s%s %s (%s) — %s — %s — %s",
				strings.Repeat("  ", depth), nodeKindGlyph(n.Kind), n.Name, n.Slug, n.Kind, n.Status, n.ID)
			if n.UpstreamGit != "" {
				fmt.Fprintf(&b, " — %s", n.UpstreamGit)
			}
			b.WriteByte('\n')
			walk(t.Children, depth+1)
		}
	}
	walk(buildNodeForest(nodes), 0)
	return strings.TrimRight(b.String(), "\n")
}

// maxDeleteImpactItems caps how many child slugs / document paths the report
// enumerates. A node can legitimately have hundreds of children, and a single
// unbounded comma-joined line is the text-output equivalent of an unbreakable
// string: it drowns the actionable part of the message and burns model context.
const maxDeleteImpactItems = 10

// joinCapped joins at most max items and appends "… and N more" for the rest, so
// the count in the surrounding sentence stays authoritative.
func joinCapped(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s … and %d more", strings.Join(items[:max], ", "), len(items)-max)
}

// formatDeleteImpact renders the dry run. A node with children or project
// documents is reported as NOT deletable, because the database refuses it anyway
// (nodes.parent_id is ON DELETE RESTRICT, migration 0016; project documents are
// checked explicitly in internal/adapter/pgstore/nodes.go:144-151). Everything
// else is the silent damage made visible: sessions and non-project documents are
// set to NULL (migration 0012), bindings, artifacts and the logo CASCADE.
func formatDeleteImpact(d deleteImpact) string {
	var b strings.Builder
	if d.blocked() {
		fmt.Fprintf(&b, "Cannot delete %s %q (%s) — the server would refuse it:\n", d.Node.Kind, d.Node.Name, d.Node.Slug)
		if len(d.Children) > 0 {
			slugs := make([]string, len(d.Children))
			for i, c := range d.Children {
				slugs[i] = c.Slug
			}
			fmt.Fprintf(&b, "  %d child node(s): %s — move them with flow_move_node first.\n",
				len(d.Children), joinCapped(slugs, maxDeleteImpactItems))
		}
		if len(d.ProjectDocs) > 0 {
			paths := make([]string, len(d.ProjectDocs))
			for i, doc := range d.ProjectDocs {
				paths[i] = doc.Path
			}
			fmt.Fprintf(&b, "  %d project document(s): %s — move or reclassify them with flow_move_doc first.\n",
				len(d.ProjectDocs), joinCapped(paths, maxDeleteImpactItems))
		}
		return strings.TrimRight(b.String(), "\n")
	}
	fmt.Fprintf(&b, "Would delete %s %q (%s).\n", d.Node.Kind, d.Node.Name, d.Node.Slug)
	if d.Rollup.TotalMin > 0 {
		// NodeStats rolls up the SUBTREE, but a node with children is never
		// deletable — so in exactly the deletable case the number is exact.
		fmt.Fprintf(&b, "  %s of booked worktime would lose its node.\n", timefmt.FormatMin(d.Rollup.TotalMin))
	} else {
		b.WriteString("  No booked worktime.\n")
	}
	logo := "no logo"
	if d.HasLogo {
		logo = "1 logo"
	}
	fmt.Fprintf(&b, "  %d artifact(s) and %s would be deleted, along with every binding of this node.\n",
		d.OwnArtifacts, logo)
	b.WriteString("  Other documents in its scope would lose their node but survive.\n")
	b.WriteString("  No children, no project documents — delete is possible.\n")
	b.WriteString("  Pass confirm=true to proceed.")
	return b.String()
}

// nodeDetail is everything flow_get_node shows about one node.
type nodeDetail struct {
	Node     domain.Node
	Chain    []domain.Node // leaf→root, as apiclient.Ancestors returns
	Tags     []domain.Tag
	Bindings []domain.ProjectBinding // already filtered to this node
	Rollup   apiclient.NodeRollup
}

// formatNodeDetail renders one node in full. The breadcrumb is printed root→leaf
// even though Ancestors delivers leaf→root, matching `flow node show`
// (cmd/flow/node_subcommands.go:166-170). Empty tags and empty bindings are
// STATED rather than omitted, so a model never mistakes silence for absence.
func formatNodeDetail(d nodeDetail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %q (%s)\nid: %s\nstatus: %s\n",
		nodeKindGlyph(d.Node.Kind), d.Node.Kind, d.Node.Name, d.Node.Slug, d.Node.ID, d.Node.Status)
	if d.Node.Description != "" {
		fmt.Fprintf(&b, "description: %s\n", d.Node.Description)
	}
	if d.Node.UpstreamGit != "" {
		fmt.Fprintf(&b, "upstream: %s\n", d.Node.UpstreamGit)
	}
	if len(d.Chain) > 0 {
		crumbs := make([]string, 0, len(d.Chain))
		for i := len(d.Chain) - 1; i >= 0; i-- {
			crumbs = append(crumbs, d.Chain[i].Name)
		}
		fmt.Fprintf(&b, "path: %s\n", strings.Join(crumbs, " / "))
	}
	tags := "— (no tool to set node tags exists yet)"
	if len(d.Tags) > 0 {
		names := make([]string, len(d.Tags))
		for i, t := range d.Tags {
			names[i] = t.Slug
		}
		tags = strings.Join(names, ", ")
	}
	fmt.Fprintf(&b, "tags: %s\n", tags)
	fmt.Fprintf(&b, "worktime (subtree): total %s · this week %s · this month %s\n",
		timefmt.FormatMin(d.Rollup.TotalMin), timefmt.FormatMin(d.Rollup.WeekMin), timefmt.FormatMin(d.Rollup.MonthMin))
	if len(d.Bindings) == 0 {
		b.WriteString("bindings: none — bind one with flow_bind_project.\n")
	} else {
		b.WriteString("bindings:\n")
		for _, bd := range d.Bindings {
			if bd.Kind == domain.BindingRemote {
				fmt.Fprintf(&b, "- remote %s\n", bd.RemoteSlug)
				continue
			}
			fmt.Fprintf(&b, "- path %s on machine %s [%s]\n", bd.Path, bd.MachineLabel, bd.MachineID)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
