package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/spf13/cobra"
)

func sessionCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "session", Short: "manage past worktime sessions (Nachbuchen)"}
	cmd.AddCommand(sessionAddCmd(), sessionListCmd(), sessionEditCmd(), sessionDeleteCmd())
	return cmd
}

// parseClock combines a yyyy-mm-dd date with an HH:MM clock in local tz.
func parseClock(dateStr, hhmm string) (time.Time, error) {
	d, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("bad --date %q: %w", dateStr, err)
	}
	c, err := time.ParseInLocation("15:04", hhmm, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("bad time %q (want HH:MM): %w", hhmm, err)
	}
	return time.Date(d.Year(), d.Month(), d.Day(), c.Hour(), c.Minute(), 0, 0, time.Local), nil
}

type sessionAddInput struct {
	Date, From, To, Project, Tag, Note string
}

func runSessionAdd(ctx context.Context, c *apiclient.Client, in sessionAddInput) (string, error) {
	start, err := parseClock(in.Date, in.From)
	if err != nil {
		return "", err
	}
	stop, err := parseClock(in.Date, in.To)
	if err != nil {
		return "", err
	}
	if !stop.After(start) {
		return "", fmt.Errorf("--to (%s) must be after --from (%s)", in.To, in.From)
	}
	pid, err := newProjectResolver(c, false).resolve(ctx, in.Project)
	if err != nil {
		return "", err
	}
	s, err := c.AddSession(ctx, pid, start, stop, in.Tag, in.Note)
	if err != nil {
		return "", fmt.Errorf("add session: %w", err)
	}
	return fmt.Sprintf("added %s  %s–%s", s.ID, in.From, in.To), nil
}

func sessionAddCmd() *cobra.Command {
	var in sessionAddInput
	cmd := &cobra.Command{
		Use:   "add",
		Short: "backfill a past session (--date --from --to)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			out, err := runSessionAdd(cmd.Context(), c, in)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&in.Date, "date", "", "day to book (yyyy-mm-dd, required)")
	cmd.Flags().StringVar(&in.From, "from", "", "start time HH:MM (required)")
	cmd.Flags().StringVar(&in.To, "to", "", "stop time HH:MM (required)")
	cmd.Flags().StringVar(&in.Project, "project", "", "project name (created if new)")
	cmd.Flags().StringVar(&in.Tag, "tag", "", "optional tag")
	cmd.Flags().StringVar(&in.Note, "note", "", "optional note")
	_ = cmd.MarkFlagRequired("date")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

// sessionRange computes [since, until) for a day or date range.
func sessionRange(dateStr, from, to string) (since, until time.Time, err error) {
	if dateStr != "" {
		d, e := time.ParseInLocation("2006-01-02", dateStr, time.Local)
		if e != nil {
			return since, until, fmt.Errorf("bad --date %q: %w", dateStr, e)
		}
		d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.Local)
		return d, d.AddDate(0, 0, 1), nil
	}
	if from == "" || to == "" {
		return since, until, fmt.Errorf("need --date or both --from and --to (yyyy-mm-dd)")
	}
	a, e1 := time.ParseInLocation("2006-01-02", from, time.Local)
	b, e2 := time.ParseInLocation("2006-01-02", to, time.Local)
	if e1 != nil || e2 != nil {
		return since, until, fmt.Errorf("bad --from/--to (want yyyy-mm-dd)")
	}
	return a, b.AddDate(0, 0, 1), nil
}

func fmtHM(t time.Time) string { return t.Local().Format("15:04") }

func runSessionList(ctx context.Context, c *apiclient.Client, dateStr, from, to string) (string, error) {
	since, until, err := sessionRange(dateStr, from, to)
	if err != nil {
		return "", err
	}
	sessions, err := c.ListSessionsRange(ctx, since, until)
	if err != nil {
		return "", err
	}
	projects, err := c.ListProjects(ctx)
	if err != nil {
		return "", err
	}
	name := func(id *string) string {
		if id == nil {
			return "—"
		}
		for _, p := range projects {
			if p.ID == *id {
				return p.Name
			}
		}
		return "—"
	}
	var b strings.Builder
	for _, s := range sessions {
		stop := "…"
		dur := "running"
		if s.Stop != nil {
			stop = fmtHM(*s.Stop)
			d := s.Stop.Sub(s.Start)
			dur = fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
		}
		fmt.Fprintf(&b, "%s  %s–%s  %s  %-16s %s\n",
			s.ID, fmtHM(s.Start), stop, dur, name(s.ProjectID), s.Tag)
	}
	return b.String(), nil
}

func sessionListCmd() *cobra.Command {
	var dateStr, from, to string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list sessions for a day (--date) or range (--from --to)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			out, err := runSessionList(cmd.Context(), c, dateStr, from, to)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&dateStr, "date", "", "single day (yyyy-mm-dd)")
	cmd.Flags().StringVar(&from, "from", "", "range start day (yyyy-mm-dd)")
	cmd.Flags().StringVar(&to, "to", "", "range end day inclusive (yyyy-mm-dd)")
	return cmd
}

func runSessionDelete(ctx context.Context, c *apiclient.Client, id string) error {
	return c.DeleteSession(ctx, id)
}

func findSession(ctx context.Context, c *apiclient.Client, id string) (domain.WorkSession, error) {
	now := time.Now()
	list, err := c.ListSessionsRange(ctx, now.AddDate(-1, 0, -1), now.AddDate(0, 0, 2))
	if err != nil {
		return domain.WorkSession{}, err
	}
	for _, s := range list {
		if s.ID == id {
			return s, nil
		}
	}
	return domain.WorkSession{}, fmt.Errorf("session %q not found in the last year", id)
}

type sessionEditInput struct {
	From, To, Project, Tag, Note *string
}

func runSessionEdit(ctx context.Context, c *apiclient.Client, id string, in sessionEditInput) (string, error) {
	cur, err := findSession(ctx, c, id)
	if err != nil {
		return "", err
	}
	dateStr := cur.Start.Local().Format("2006-01-02")
	start := cur.Start
	if in.From != nil {
		if start, err = parseClock(dateStr, *in.From); err != nil {
			return "", err
		}
	}
	stop := cur.Stop
	if in.To != nil {
		t, e := parseClock(dateStr, *in.To)
		if e != nil {
			return "", e
		}
		stop = &t
	}
	if stop == nil {
		return "", fmt.Errorf("session %q is still running — pass --to to set its stop time (or stop it first)", id)
	}
	if !stop.After(start) {
		return "", fmt.Errorf("--to must be after --from (start)")
	}
	projectID := cur.ProjectID
	if in.Project != nil {
		if projectID, err = newProjectResolver(c, false).resolve(ctx, *in.Project); err != nil {
			return "", err
		}
	}
	tag := cur.Tag
	if in.Tag != nil {
		tag = *in.Tag
	}
	note := cur.Note
	if in.Note != nil {
		note = *in.Note
	}
	if _, err := c.EditSession(ctx, id, projectID, tag, note, start, stop); err != nil {
		return "", fmt.Errorf("edit session: %w", err)
	}
	return fmt.Sprintf("edited %s", id), nil
}

func sessionDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "delete a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			return runSessionDelete(cmd.Context(), c, args[0])
		},
	}
}

func sessionEditCmd() *cobra.Command {
	var from, to, project, tag, note string
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "edit a session (only provided flags change)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			in := sessionEditInput{}
			f := cmd.Flags()
			if f.Changed("from") {
				in.From = &from
			}
			if f.Changed("to") {
				in.To = &to
			}
			if f.Changed("project") {
				in.Project = &project
			}
			if f.Changed("tag") {
				in.Tag = &tag
			}
			if f.Changed("note") {
				in.Note = &note
			}
			out, err := runSessionEdit(cmd.Context(), c, args[0], in)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "new start HH:MM")
	cmd.Flags().StringVar(&to, "to", "", "new stop HH:MM")
	cmd.Flags().StringVar(&project, "project", "", "new project name")
	cmd.Flags().StringVar(&tag, "tag", "", "new tag")
	cmd.Flags().StringVar(&note, "note", "", "new note")
	return cmd
}
