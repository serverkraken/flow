package main

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/statuscache"
	"github.com/spf13/cobra"
)

const statusRefreshTimeout = 30 * time.Second

func worktimeStatusRefreshCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "status-refresh",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), statusRefreshTimeout)
			defer cancel()
			fetch := func(ctx context.Context) (apiclient.WorktimeStatus, error) {
				client, err := clientFromStore(ctx) // NEVER starts an interactive device flow
				if err != nil {
					return apiclient.WorktimeStatus{}, err
				}
				return client.GetWorktimeStatus(ctx)
			}
			_ = refreshStatusCache(ctx, statusCachePath(), fetch)
			return nil // tmux workers are silent and best-effort
		},
	}
}

func refreshStatusCache(
	ctx context.Context,
	cachePath string,
	fetch func(context.Context) (apiclient.WorktimeStatus, error),
) error {
	_, err := statuscache.TryWithRefreshLock(cachePath, func() error {
		if entry, ok := statuscache.Read(cachePath); ok && entry.Fresh(time.Now()) {
			return nil
		}
		status, err := fetch(ctx)
		if err != nil {
			return err
		}
		return statuscache.Write(cachePath, statuscache.Entry{FetchedAt: time.Now(), Status: status})
	})
	return err
}
