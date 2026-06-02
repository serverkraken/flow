package cli

// `flow sync` — operational control over the sync worker (Phase 1 M2f).
//
//   flow sync status     — prints WriteQueue length, watermarks per resource,
//                          LastPullAt, and LastPullError.
//   flow sync force-pull — calls SyncController.ForcePull; surfaces the
//                          "not yet wired" advisory until Task 29.

import (
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/spf13/cobra"
)

// SyncDeps is the dependency bundle for `flow sync`.
type SyncDeps struct {
	Controller ports.SyncController
}

// NewSyncCmd constructs the `flow sync` command tree.
func NewSyncCmd(deps SyncDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "sync",
		Short:        "Sync-Worker-Status und manuelle Steuerung",
		SilenceUsage: true,
	}
	cmd.AddCommand(
		newSyncStatusCmd(deps),
		newSyncForcePullCmd(deps),
	)
	return cmd
}

// newSyncStatusCmd implements `flow sync status`.
func newSyncStatusCmd(deps SyncDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "status",
		Short:        "WriteQueue-Laenge, Watermarks, Last-Pull-Zeitstempel anzeigen",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := deps.Controller.Status()
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			lastPullAt := st.LastPullAt
			if lastPullAt == "" {
				lastPullAt = "(sync worker not yet running)"
			}
			lastError := st.LastPullError
			if lastError == "" {
				lastError = "(none)"
			}

			fprintf(out, "QUEUE_LEN     %d\n", st.QueueLen)
			fprintf(out, "LAST_PULL_AT  %s\n", lastPullAt)
			fprintf(out, "LAST_ERROR    %s\n", lastError)
			fprintf(out, "\n")
			fprintf(out, "WATERMARKS:\n")

			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "RESOURCE\tWATERMARK")
			for _, resource := range watermarkOrder(st.Watermarks) {
				_, _ = fmt.Fprintf(w, "%s\t%d\n", resource, st.Watermarks[resource])
			}
			return w.Flush()
		},
	}
}

// newSyncForcePullCmd implements `flow sync force-pull`.
func newSyncForcePullCmd(deps SyncDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "force-pull",
		Short:        "Sofortigen Pull vom Server ausloesen",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := deps.Controller.ForcePull()
			if errors.Is(err, usecase.ErrSyncWorkerNotWired) {
				fprintf(cmd.OutOrStdout(), "%s\n", err.Error())
				return nil
			}
			return err
		},
	}
}

// watermarkOrder returns resource keys in deterministic order for display.
// Keys present in the canonical list come first in spec order; any
// unexpected extra keys are appended sorted.
var canonicalResourceOrder = []string{
	"projects",
	"sessions",
	"active_sessions",
	"users",
	"repos",
	"repo_notes",
}

func watermarkOrder(m map[string]int64) []string {
	seen := make(map[string]bool, len(m))
	var out []string
	for _, r := range canonicalResourceOrder {
		if _, ok := m[r]; ok {
			out = append(out, r)
			seen[r] = true
		}
	}
	// append any unexpected keys in insertion-independent but stable order
	var extra []string
	for k := range m {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	// sort extra for determinism
	for i := 0; i < len(extra)-1; i++ {
		for j := i + 1; j < len(extra); j++ {
			if extra[i] > extra[j] {
				extra[i], extra[j] = extra[j], extra[i]
			}
		}
	}
	return append(out, extra...)
}
