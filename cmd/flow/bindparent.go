package main

import (
	"context"
	"fmt"
	"sort"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"
)

// repoParentItems returns the nodes a new repo may hang under (engagements and
// vorhaben), each path-labeled (so duplicate names stay distinguishable) and
// sorted, for the bind-create parent picker.
func repoParentItems(nodes []domain.Node) []fuzzylist.Item {
	byID := indexByID(nodes)
	var items []fuzzylist.Item
	for _, n := range nodes {
		if n.Kind == domain.KindEngagement || n.Kind == domain.KindVorhaben {
			items = append(items, fuzzylist.Item{ID: n.ID, Label: nodePathOf(byID, n)})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return items
}

// pickRepoParent runs the parent picker over the valid repo-parents in nodes and
// returns the chosen parent's node ID. ok is false if the user cancelled. It errors
// when no engagement/vorhaben exists yet — there is nothing to hang a repo under.
func pickRepoParent(ctx context.Context, nodes []domain.Node, pal theme.Palette) (parentID string, ok bool, err error) {
	items := repoParentItems(nodes)
	if len(items) == 0 {
		return "", false, fmt.Errorf("kein Engagement/Vorhaben vorhanden — erst eines anlegen: flow node create <name> --kind engagement")
	}
	prog := newPickParentProgram(items, pal)
	if _, err := tea.NewProgram(prog, tea.WithContext(ctx)).Run(); err != nil {
		return "", false, fmt.Errorf("parent picker: %w", err)
	}
	picked, _, sel := prog.Selection()
	if !sel {
		return "", false, nil // cancelled
	}
	return picked.ID, true, nil
}

// resolveOrCreateBindTarget returns the node id + display name to bind to. When
// isCreate it creates a repo named picked.Label under parentID; otherwise it uses
// the picked existing node. Shared by the remote- and path-binding flows.
func resolveOrCreateBindTarget(ctx context.Context, c *apiclient.Client, picked fuzzylist.Item, isCreate bool, parentID *string) (nodeID, name string, err error) {
	if !isCreate {
		return picked.ID, picked.Label, nil
	}
	p, err := c.CreateNode(ctx, apiclient.CreateNodeFields{
		Name: picked.Label, Kind: string(domain.KindRepo), ParentID: parentID,
	})
	if err != nil {
		return "", "", fmt.Errorf("create project: %w", err)
	}
	return p.ID, p.Name, nil
}
