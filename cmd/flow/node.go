package main

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/spf13/cobra"
)

func nodeCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "node", Short: "manage projects"}
	cmd.AddCommand(nodeRateCmd())
	cmd.AddCommand(nodeBindCmd())
	cmd.AddCommand(nodeUnbindCmd())
	cmd.AddCommand(nodeBindingsCmd())
	cmd.AddCommand(nodeRmCmd())
	cmd.AddCommand(nodeCreateCmd())
	cmd.AddCommand(nodeUpdateCmd())
	cmd.AddCommand(nodeListCmd())
	cmd.AddCommand(nodeShowCmd())
	cmd.AddCommand(nodeMoveCmd())
	cmd.AddCommand(nodePauseCmd())
	cmd.AddCommand(nodeResumeCmd())
	cmd.AddCommand(nodeArchiveCmd())
	return cmd
}

// runNodeRm resolves slug to an ID and deletes the project.
// The confirmation prompt is kept in RunE so this helper remains unit-testable.
func runNodeRm(ctx context.Context, c *apiclient.Client, slug string) error {
	id, err := resolveSlug(ctx, c, slug)
	if err != nil {
		return err
	}
	if err := c.DeleteNode(ctx, id); err != nil {
		if apiclient.IsConflict(err) {
			return fmt.Errorf("cannot delete %s: it has children; move or remove them first", slug)
		}
		return err
	}
	return nil
}

func nodeRmCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rm <slug>",
		Short: "delete a project (sessions and documents are kept but un-assigned)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			if !yes {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(),
					"delete project %s? its sessions and documents will be kept but un-assigned [y/N]: ", slug)
				scanner := bufio.NewScanner(cmd.InOrStdin())
				scanner.Scan()
				answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
				if answer != "y" && answer != "yes" {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "aborted")
					return nil
				}
			}
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			if err := runNodeRm(cmd.Context(), c, slug); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "deleted project %s\n", slug)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt (for scripting)")
	return cmd
}

func nodeRateCmd() *cobra.Command {
	var clear bool
	cmd := &cobra.Command{
		Use:   "rate <slug> [<amount-minor> <currency>]",
		Short: "set or clear a project's per-hour rate (amount in minor units, e.g. 8000 = 80.00)",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 1 || len(args) == 3 {
				return nil
			}
			return fmt.Errorf("accepts 1 arg (slug, with --clear) or 3 args (slug amount-minor currency)")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			id, err := resolveSlug(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if clear && len(args) > 1 {
				return fmt.Errorf("--clear takes only the slug argument; remove the amount and currency")
			}
			if clear {
				return c.SetNodeRate(cmd.Context(), id, nil, "")
			}
			if len(args) != 3 {
				return fmt.Errorf("usage: flow project rate <slug> <amount-minor> <currency>  (or --clear)")
			}
			amount, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("amount must be an integer (minor units): %w", err)
			}
			return c.SetNodeRate(cmd.Context(), id, &amount, args[2])
		},
	}
	cmd.Flags().BoolVar(&clear, "clear", false, "clear the rate instead of setting it")
	return cmd
}
