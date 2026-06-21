package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

func projectCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "manage projects"}
	cmd.AddCommand(projectRateCmd())
	cmd.AddCommand(projectBindCmd())
	cmd.AddCommand(projectUnbindCmd())
	cmd.AddCommand(projectBindingsCmd())
	return cmd
}

func projectRateCmd() *cobra.Command {
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
				return c.SetProjectRate(cmd.Context(), id, nil, "")
			}
			if len(args) != 3 {
				return fmt.Errorf("usage: flow project rate <slug> <amount-minor> <currency>  (or --clear)")
			}
			amount, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("amount must be an integer (minor units): %w", err)
			}
			return c.SetProjectRate(cmd.Context(), id, &amount, args[2])
		},
	}
	cmd.Flags().BoolVar(&clear, "clear", false, "clear the rate instead of setting it")
	return cmd
}
