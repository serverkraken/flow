package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func dayoffCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "dayoff", Short: "manage days off (vacation/sick) + holidays"}
	cmd.AddCommand(dayoffListCmd(), dayoffAddCmd(), dayoffRmCmd())
	return cmd
}

func dayoffListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list day-offs for the current year (manual + holidays)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			year := time.Now().Year()
			list, err := c.ListDayOffs(cmd.Context(),
				fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
			if err != nil {
				return err
			}
			for _, d := range list {
				tag := d.Kind
				if d.Holiday {
					tag = "holiday"
				}
				fmt.Printf("%s  %-8s %s\n", d.Day, tag, d.Label)
			}
			return nil
		},
	}
}

func dayoffAddCmd() *cobra.Command {
	var kind, label string
	var targetMin int
	var skipWeekends bool
	cmd := &cobra.Command{
		Use:   "add <from> <to>",
		Short: "add a day-off range (yyyy-mm-dd yyyy-mm-dd)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			return c.AddDayOffs(cmd.Context(), args[0], args[1], kind, label, targetMin, skipWeekends)
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "vacation", "vacation|sick|flex|special|childsick|training")
	cmd.Flags().StringVar(&label, "label", "", "optional label")
	cmd.Flags().IntVar(&targetMin, "target-min", 0, "half-day target in minutes (0 = full day off)")
	cmd.Flags().BoolVar(&skipWeekends, "skip-weekends", true, "skip Sat/Sun")
	return cmd
}

func dayoffRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <day>",
		Short: "remove a day-off (yyyy-mm-dd)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			return c.DeleteDayOff(cmd.Context(), args[0])
		},
	}
}
