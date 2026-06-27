package main

import (
	"context"
	"fmt"
	"io"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/spf13/cobra"
)

// runNodeSetStatus reads the node, then PATCHes it with the new status (full
// replace; rate is untouched by UpdateNode).
func runNodeSetStatus(ctx context.Context, c *apiclient.Client, w io.Writer, slug, status string) error {
	id, err := resolveSlug(ctx, c, slug)
	if err != nil {
		return err
	}
	n, err := c.GetNode(ctx, id)
	if err != nil {
		return err
	}
	if _, err := c.UpdateNode(ctx, id, apiclient.UpdateNodeFields{
		Name: n.Name, Slug: n.Slug, Color: n.Color, Glyph: n.Glyph,
		Description: n.Description, UpstreamGit: n.UpstreamGit, Status: status,
	}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "%s is now %s\n", slug, status)
	return nil
}

func nodeStatusCmd(use, short, status string) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <slug>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			return runNodeSetStatus(cmd.Context(), c, cmd.OutOrStdout(), args[0], status)
		},
	}
}

func nodePauseCmd() *cobra.Command   { return nodeStatusCmd("pause", "pause a node", "paused") }
func nodeResumeCmd() *cobra.Command  { return nodeStatusCmd("resume", "resume a paused node", "active") }
func nodeArchiveCmd() *cobra.Command { return nodeStatusCmd("archive", "archive a node", "archived") }
