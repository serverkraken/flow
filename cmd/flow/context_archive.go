package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/spf13/cobra"
)

// archiveClient is the minimal interface runArchive needs.
// Methods are unexported so the interface is local to this package;
// both the real adapter and the test stub live here.
type archiveClient interface {
	resolvePath(path string) (string, error)
	SetArchived(id string, archived bool) error
}

// realArchiveAdapter adapts *apiclient.Client to archiveClient.
// It pre-fetches the active document list for path→id resolution.
type realArchiveAdapter struct {
	c   *apiclient.Client
	ctx context.Context
	all []domain.Document
}

func (a *realArchiveAdapter) resolvePath(path string) (string, error) {
	for _, d := range a.all {
		if d.Path == path {
			return d.ID, nil
		}
	}
	return "", fmt.Errorf("no document with path %q", path)
}

func (a *realArchiveAdapter) SetArchived(id string, archived bool) error {
	return a.c.SetArchived(a.ctx, id, archived)
}

// runArchive is the testable core of `flow context archive --from`.
// It reads a path<TAB>archive TSV from r and calls SetArchived for each row.
// When dryRun is true it prints the count of rows that would be archived.
func runArchive(ctx context.Context, out io.Writer, client archiveClient, r io.Reader, dryRun bool) error {
	_ = ctx // unused: client methods close over ctx in the real adapter
	type row struct {
		path    string
		archive bool
	}
	var rows []row

	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.SplitN(line, "\t", 2)
		if len(f) < 2 {
			continue
		}
		// Skip header
		if f[0] == "path" {
			continue
		}
		rows = append(rows, row{
			path:    strings.TrimSpace(f[0]),
			archive: strings.TrimSpace(f[1]) == "y",
		})
	}
	if err := sc.Err(); err != nil {
		return err
	}

	if dryRun {
		var n int
		for _, rr := range rows {
			if rr.archive {
				n++
			}
		}
		_, _ = fmt.Fprintf(out, "would archive %d\n", n)
		return nil
	}

	for _, rr := range rows {
		id, err := client.resolvePath(rr.path)
		if err != nil {
			return fmt.Errorf("resolve %q: %w", rr.path, err)
		}
		if err := client.SetArchived(id, rr.archive); err != nil {
			return fmt.Errorf("set-archived %q: %w", rr.path, err)
		}
	}
	return nil
}

// contextArchiveCmd returns the `flow context archive` command.
func contextArchiveCmd() *cobra.Command {
	var from string
	var dryRun, candidates bool
	cmd := &cobra.Command{
		Use:   "archive",
		Short: "Bulk archive memory docs, or emit a candidate review TSV",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := clientFromStore(ctx)
			if err != nil {
				return err
			}

			if candidates {
				docs, err := c.ListDocuments(ctx)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "path\ttitle\tarchive")
				for _, d := range docs {
					if string(d.Type) != string(domain.DocMemory) {
						continue
					}
					upper := strings.ToUpper(d.Title + " " + d.Body)
					if strings.Contains(upper, "DONE") || strings.HasSuffix(d.Path, "_done") {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\ty\n", d.Path, d.Title)
					}
				}
				return nil
			}

			if from == "" {
				return fmt.Errorf("use --from <tsv> to apply or --candidates to emit a review TSV")
			}

			f, err := os.Open(from)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()

			all, err := c.ListDocuments(ctx)
			if err != nil {
				return err
			}
			adapter := &realArchiveAdapter{c: c, ctx: ctx, all: all}
			return runArchive(ctx, cmd.OutOrStdout(), adapter, f, dryRun)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "TSV file with path<TAB>archive columns")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print count without writing")
	cmd.Flags().BoolVar(&candidates, "candidates", false, "emit candidate TSV for review")
	return cmd
}

// contextArchivedCmd returns the `flow context archived` command.
func contextArchivedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "archived",
		Short: "List archived documents",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := clientFromStore(ctx)
			if err != nil {
				return err
			}
			docs, err := c.ListArchived(ctx)
			if err != nil {
				return err
			}
			for _, d := range docs {
				archivedAt := ""
				if d.ArchivedAt != nil {
					archivedAt = d.ArchivedAt.Format("2006-01-02")
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s · %s · %s\n", d.Path, d.Title, archivedAt)
			}
			return nil
		},
	}
}
