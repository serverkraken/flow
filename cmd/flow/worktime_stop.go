package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"
	"github.com/spf13/cobra"
)

func worktimeStopCmd() *cobra.Command {
	var nodeRef string
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the running timer, booking it to a node (interactive picker)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			fetch := func(ctx context.Context) (apiclient.WorktimeStatus, error) { return c.GetWorktimeStatus(ctx) }
			return runStop(cmd.Context(), c, fetch, nodeRef, isInteractive(), pickBookableNode, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&nodeRef, "node", "", "node ref to book to (slug/path/id) — required in non-TTY")
	return cmd
}

// isInteractive reports whether stdin is a terminal (needed for the picker).
func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && (fi.Mode()&os.ModeCharDevice) != 0
}

// pickBookableNode runs the fuzzy/MRU picker over the owner's bookable nodes and
// returns the chosen node id ("" + nil = cancelled).
func pickBookableNode(ctx context.Context, c *apiclient.Client) (string, error) {
	nodes, err := c.ListNodes(ctx)
	if err != nil {
		return "", err
	}
	var bookable []domain.Node
	for _, n := range nodes {
		if domain.IsBookable(n.Kind) {
			bookable = append(bookable, n)
		}
	}
	prog := newPickBookableProgram(mruOrder(ctx, c, bookable), theme.Load())
	if _, err := tea.NewProgram(prog, tea.WithContext(ctx)).Run(); err != nil {
		return "", err
	}
	picked, _, ok := prog.Selection()
	if !ok {
		return "", nil // Esc → cancel, no stop
	}
	return picked.ID, nil
}

// mruOrder returns picker items MOST-RECENTLY-BOOKED first (Spec §1/§3 "fuzzy/MRU"),
// then the never-booked remainder in ListNodes order. Ranking comes from the
// server's EXACT /nodes/mru (Task 4b) — not a client heuristic; any error
// degrades silently to plain order (the picker must never hard-fail on MRU).
func mruOrder(ctx context.Context, c *apiclient.Client, bookable []domain.Node) []fuzzylist.Item {
	byID := make(map[string]domain.Node, len(bookable))
	for _, n := range bookable {
		byID[n.ID] = n
	}
	var items []fuzzylist.Item
	seen := map[string]bool{}
	if mru, err := c.NodeMRU(ctx); err == nil {
		for _, e := range mru { // server-sorted newest-first
			if n, ok := byID[e.NodeID]; ok && !seen[e.NodeID] {
				seen[e.NodeID] = true
				items = append(items, fuzzylist.Item{ID: n.ID, Label: n.Name})
			}
		}
	}
	for _, n := range bookable {
		if !seen[n.ID] {
			items = append(items, fuzzylist.Item{ID: n.ID, Label: n.Name})
		}
	}
	return items
}

// runStop is the pure cascade (picker + status fetch injected for tests).
func runStop(ctx context.Context, c *apiclient.Client,
	fetch func(context.Context) (apiclient.WorktimeStatus, error),
	nodeRef string, interactive bool,
	pick func(context.Context, *apiclient.Client) (string, error),
	out io.Writer,
) error {
	st, err := fetch(ctx)
	if err != nil {
		return err
	}
	if !st.Running {
		_, _ = fmt.Fprintln(out, "keine laufende Session")
		return nil
	}
	nodeID, err := resolveStopNode(ctx, c, st, nodeRef, interactive, pick)
	if err != nil {
		return err
	}
	if nodeID == "" {
		return nil // cancelled (Esc) — do NOT stop
	}
	sess, err := c.StopSession(ctx, st.ActiveSessionID, nodeID)
	if err != nil {
		return err // stop failed AFTER the node was chosen — cache is NOT invalidated, error surfaced (Finding C12)
	}
	_ = os.Remove(statusCachePath()) // invalidate → next 5s tick refetches
	// Print the booked posten (Spec §3 step 5): node name + duration, not just the id.
	name := nodeID
	if n, gerr := c.GetNode(ctx, nodeID); gerr == nil && n.Name != "" {
		name = n.Name // best-effort; falls back to the id
	}
	_, _ = fmt.Fprintf(out, "gestoppt · gebucht auf %s · %s\n", name, sess.Elapsed(time.Now()).Round(time.Minute))
	return nil
}

func resolveStopNode(ctx context.Context, c *apiclient.Client, st apiclient.WorktimeStatus,
	nodeRef string, interactive bool,
	pick func(context.Context, *apiclient.Client) (string, error),
) (string, error) {
	if nodeRef != "" { // explicit override — wins over st.ActiveNodeID (Finding #4); also the non-TTY path
		nodes, err := c.ListNodes(ctx)
		if err != nil {
			return "", err
		}
		return resolveNodeRef(nodes, nodeRef) // bad ref → clean error, propagated
	}
	if st.ActiveNodeID != nil && *st.ActiveNodeID != "" {
		return *st.ActiveNodeID, nil // already booked at start → stop straight away
	}
	if !interactive {
		return "", fmt.Errorf("no TTY for the picker: pass --node <ref> to book non-interactively")
	}
	return pick(ctx, c)
}
