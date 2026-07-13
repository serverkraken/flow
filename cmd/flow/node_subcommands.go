package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/projectresolve"
	"github.com/spf13/cobra"
)

// --- testable cores ---

func runNodeCreate(ctx context.Context, c *apiclient.Client, w io.Writer, name, kind, parentSlug, color, glyph, desc, upstream string) error {
	var parentID *string
	if parentSlug != "" {
		id, err := resolveSlug(ctx, c, parentSlug)
		if err != nil {
			return err
		}
		parentID = &id
	}
	n, err := c.CreateNode(ctx, apiclient.CreateNodeFields{
		Name: name, Kind: kind, ParentID: parentID,
		Color: color, Glyph: glyph, Description: desc, UpstreamGit: upstream,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "created %s %s (%s)\n", n.Kind, n.Name, n.Slug)
	return nil
}

// rateChange carries an optional rate mutation for `node update`: clear=true
// clears the rate, otherwise amount (minor units) + currency set it.
type rateChange struct {
	clear    bool
	amount   *int64
	currency string
}

// updateHasField reports whether at least one metadata field is set (so an
// invocation with only --rate skips the empty PATCH).
func updateHasField(f apiclient.UpdateNodeFields) bool {
	return f.Name != nil || f.Slug != nil || f.Color != nil || f.Glyph != nil ||
		f.Icon != nil || f.Description != nil || f.UpstreamGit != nil || f.Status != nil
}

// runNodeUpdate applies a partial metadata PATCH (only the set fields) and,
// when rc != nil, a separate rate mutation via the rate endpoint.
func runNodeUpdate(ctx context.Context, c *apiclient.Client, w io.Writer, slug string, f apiclient.UpdateNodeFields, rc *rateChange) error {
	id, err := resolveSlug(ctx, c, slug)
	if err != nil {
		return err
	}
	if updateHasField(f) {
		if _, err := c.UpdateNode(ctx, id, f); err != nil {
			return err
		}
	}
	if rc != nil {
		if rc.clear {
			if err := c.SetNodeRate(ctx, id, nil, ""); err != nil {
				return err
			}
		} else {
			if err := c.SetNodeRate(ctx, id, rc.amount, rc.currency); err != nil {
				return err
			}
		}
	}
	_, _ = fmt.Fprintf(w, "updated %s\n", slug)
	return nil
}

// statusWanted reports whether n passes the --status filter (""/"all" → any).
func statusWanted(n domain.Node, status string) bool {
	if status == "" || status == "all" {
		return true
	}
	return string(n.Status) == status
}

func runNodeList(ctx context.Context, c *apiclient.Client, w io.Writer, tree bool, kind, status string) error {
	nodes, err := c.ListNodes(ctx)
	if err != nil {
		return err
	}
	filtered := make([]domain.Node, 0, len(nodes))
	for _, n := range nodes {
		if kind != "" && string(n.Kind) != kind {
			continue
		}
		if !statusWanted(n, status) {
			continue
		}
		filtered = append(filtered, n)
	}
	if tree {
		renderTree(buildTree(filtered), w)
		return nil
	}
	for _, n := range filtered {
		_, _ = fmt.Fprintf(w, "%-11s %-24s %s\n", n.Kind, n.Slug, n.Name)
	}
	return nil
}

func runNodeMove(ctx context.Context, c *apiclient.Client, w io.Writer, slug, parentSlug string) error {
	id, err := resolveSlug(ctx, c, slug)
	if err != nil {
		return err
	}
	var parentID *string
	if parentSlug != "" {
		pid, err := resolveSlug(ctx, c, parentSlug)
		if err != nil {
			return err
		}
		parentID = &pid
	}
	if _, err := c.MoveNode(ctx, id, parentID); err != nil {
		if apiclient.IsConflict(err) {
			return fmt.Errorf("cannot move %s: it would create a cycle", slug)
		}
		return fmt.Errorf("move %s: %w", slug, err)
	}
	dest := "root"
	if parentSlug != "" {
		dest = parentSlug
	}
	_, _ = fmt.Fprintf(w, "moved %s → %s\n", slug, dest)
	return nil
}

// runNodeShow renders a node's detail + leaf→root breadcrumb. If slug is empty,
// the node is resolved from cwd (git origin → repo).
func runNodeShow(ctx context.Context, c *apiclient.Client, w io.Writer, getenv func(string) string, cwd, slug string) error {
	var node domain.Node
	if slug != "" {
		id, err := resolveSlug(ctx, c, slug)
		if err != nil {
			return err
		}
		if node, err = c.GetNode(ctx, id); err != nil {
			return err
		}
	} else {
		n, ok, err := projectresolve.Resolve(ctx, c, getenv, cwd)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("no node bound to %s (pass a slug or bind this directory)", cwd)
		}
		node = n
	}
	chain, err := c.Ancestors(ctx, node.ID)
	if err != nil {
		return err
	}
	// chain is leaf→root; print breadcrumb root→leaf.
	crumbs := make([]string, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		crumbs = append(crumbs, chain[i].Name)
	}
	_, _ = fmt.Fprintf(w, "%s %s (%s)\n", node.Kind, node.Name, node.Slug)
	_, _ = fmt.Fprintf(w, "status: %s\n", node.Status)
	if node.UpstreamGit != "" {
		_, _ = fmt.Fprintf(w, "upstream: %s\n", node.UpstreamGit)
	}
	if len(crumbs) > 0 {
		_, _ = fmt.Fprintf(w, "path: %s\n", strings.Join(crumbs, " / "))
	}
	return nil
}

// --- cobra wrappers ---

func nodeCreateCmd() *cobra.Command {
	var kind, parent, color, glyph, desc, upstream string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "create a node (engagement, vorhaben or repo)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			return runNodeCreate(cmd.Context(), c, cmd.OutOrStdout(), args[0], kind, parent, color, glyph, desc, upstream)
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "engagement|vorhaben|repo (required)")
	cmd.Flags().StringVar(&parent, "parent", "", "parent node slug (omit for an engagement root)")
	cmd.Flags().StringVar(&color, "color", "", "identity color name")
	cmd.Flags().StringVar(&glyph, "glyph", "", "identity glyph")
	cmd.Flags().StringVar(&desc, "desc", "", "description")
	cmd.Flags().StringVar(&upstream, "upstream", "", "git clone URL (repo only)")
	_ = cmd.MarkFlagRequired("kind")
	return cmd
}

func nodeUpdateCmd() *cobra.Command {
	var name, slug, color, glyph, icon, desc, upstream, status, currency string
	var rate int64
	var clearRate bool
	cmd := &cobra.Command{
		Use:   "update <slug>",
		Short: "update a node's fields (only the flags you pass change)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fl := cmd.Flags()
			if clearRate && fl.Changed("rate") {
				return fmt.Errorf("--clear-rate and --rate are mutually exclusive")
			}
			if fl.Changed("status") && status != "active" && status != "paused" && status != "archived" {
				return fmt.Errorf("--status must be active, paused or archived")
			}
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			var f apiclient.UpdateNodeFields
			if fl.Changed("name") {
				f.Name = &name
			}
			if fl.Changed("slug") {
				f.Slug = &slug
			}
			if fl.Changed("color") {
				f.Color = &color
			}
			if fl.Changed("glyph") {
				f.Glyph = &glyph
			}
			if fl.Changed("icon") {
				f.Icon = &icon
			}
			if fl.Changed("desc") {
				f.Description = &desc
			}
			if fl.Changed("upstream") {
				f.UpstreamGit = &upstream
			}
			if fl.Changed("status") {
				f.Status = &status
			}
			var rc *rateChange
			switch {
			case clearRate:
				rc = &rateChange{clear: true}
			case fl.Changed("rate"):
				cur := currency
				if cur == "" {
					cur = "EUR"
				}
				amt := rate
				rc = &rateChange{amount: &amt, currency: cur}
			}
			return runNodeUpdate(cmd.Context(), c, cmd.OutOrStdout(), args[0], f, rc)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new name")
	cmd.Flags().StringVar(&slug, "slug", "", "new slug (identity change)")
	cmd.Flags().StringVar(&color, "color", "", "identity color name")
	cmd.Flags().StringVar(&glyph, "glyph", "", "identity glyph")
	cmd.Flags().StringVar(&icon, "icon", "", "identity icon")
	cmd.Flags().StringVar(&desc, "desc", "", "description")
	cmd.Flags().StringVar(&upstream, "upstream", "", "git clone URL (repo only)")
	cmd.Flags().StringVar(&status, "status", "", "active|paused|archived")
	cmd.Flags().Int64Var(&rate, "rate", 0, "per-hour rate in minor units (e.g. 8000 = 80.00)")
	cmd.Flags().StringVar(&currency, "currency", "", "rate currency (default EUR)")
	cmd.Flags().BoolVar(&clearRate, "clear-rate", false, "clear the rate")
	return cmd
}

func nodeListCmd() *cobra.Command {
	var tree bool
	var kind, status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list nodes (flat, or --tree for the hierarchy)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			return runNodeList(cmd.Context(), c, cmd.OutOrStdout(), tree, kind, status)
		},
	}
	cmd.Flags().BoolVar(&tree, "tree", false, "render the hierarchy indented")
	cmd.Flags().StringVar(&kind, "kind", "", "filter by kind (engagement|vorhaben|repo)")
	cmd.Flags().StringVar(&status, "status", "all", "active|paused|archived|all")
	return cmd
}

func nodeShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show [<slug>]",
		Short: "show a node and its ancestor path (default: cwd-resolved repo)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			cwd, _ := os.Getwd()
			slug := ""
			if len(args) == 1 {
				slug = args[0]
			}
			return runNodeShow(cmd.Context(), c, cmd.OutOrStdout(), os.Getenv, cwd, slug)
		},
	}
	return cmd
}

func nodeMoveCmd() *cobra.Command {
	var parent string
	cmd := &cobra.Command{
		Use:   "move <slug>",
		Short: "reparent a node (cycle-free); --parent \"\" moves it to a root",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			return runNodeMove(cmd.Context(), c, cmd.OutOrStdout(), args[0], parent)
		},
	}
	cmd.Flags().StringVar(&parent, "parent", "", "new parent node slug (empty = root)")
	return cmd
}
