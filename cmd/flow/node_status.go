package main

import (
	"context"
	"fmt"
	"io"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/spf13/cobra"
)

// runNodeSetStatus PATCHes only the status; the partial UpdateNode leaves every
// other field (name, icon, upstream, binding) untouched.
func runNodeSetStatus(ctx context.Context, c *apiclient.Client, w io.Writer, slug, status string) error {
	id, err := resolveSlug(ctx, c, slug)
	if err != nil {
		return err
	}
	st := status
	if _, err := c.UpdateNode(ctx, id, apiclient.UpdateNodeFields{Status: &st}); err != nil {
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
