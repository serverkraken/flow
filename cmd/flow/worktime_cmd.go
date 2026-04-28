package main

import (
	"fmt"
	"os"

	"github.com/serverkraken/flow/internal/worktime"
	"github.com/spf13/cobra"
)

var worktimeCmd = &cobra.Command{
	Use:          "worktime",
	Short:        "Worktime subcommands",
	SilenceUsage: true,
}

var wtStatusCmd = &cobra.Command{
	Use:          "status",
	Short:        "Print tmux status-right segment",
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		fmt.Print(worktime.StatusSegment())
		return nil
	},
}

var wtStartCmd = &cobra.Command{
	Use:          "start [zeit]",
	Short:        "Start worktime session (jetzt, HH:MM, -Nm, -NhMMm)",
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		arg := ""
		if len(args) > 0 {
			arg = args[0]
		}
		ts, err := worktime.ParseStartArg(arg)
		if err != nil {
			return err
		}
		if err := worktime.Start(ts); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Worktime läuft seit %s\n", ts.Format("15:04"))
		return nil
	},
}

var wtStopCmd = &cobra.Command{
	Use:          "stop [HH:MM]",
	Short:        "Stop current worktime session (optional: custom stop time)",
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		var s worktime.Session
		var err error
		if len(args) > 0 {
			ts, parseErr := worktime.ParseStartArg(args[0])
			if parseErr != nil {
				return parseErr
			}
			s, err = worktime.StopAt(ts)
		} else {
			s, err = worktime.Stop()
		}
		if err != nil {
			return err
		}
		h := int(s.Elapsed.Hours())
		m := int(s.Elapsed.Minutes()) % 60
		fmt.Fprintf(os.Stderr, "Gestoppt nach %dh %02dm\n", h, m)
		return nil
	},
}

var wtToggleCmd = &cobra.Command{
	Use:          "toggle",
	Short:        "Start wenn idle, stopp wenn läuft",
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		msg, err := worktime.Toggle()
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, msg)
		return nil
	},
}

var wtCorrectCmd = &cobra.Command{
	Use:          "correct [HH:MM]",
	Short:        "Startzeit der laufenden Session korrigieren",
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		arg := ""
		if len(args) > 0 {
			arg = args[0]
		}
		ts, err := worktime.ParseStartArg(arg)
		if err != nil {
			return err
		}
		if err := worktime.CorrectStart(ts); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Startzeit korrigiert auf %s\n", ts.Format("15:04"))
		return nil
	},
}
