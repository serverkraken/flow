package ports

import (
	"context"
	"errors"
)

// ErrNoRemoteConfigured is returned when a sync or get-remote operation
// runs against a notebook whose git repository has no "origin" remote
// configured. The CLI maps this to a hand-written hint pointing the
// user at `kompendium remote set <url>`.
var ErrNoRemoteConfigured = errors.New("no remote configured")

// SyncStats reports what happened during a Sync. Both flags can be
// false on a no-op (working tree clean, remote already in sync) and
// both can be true on a real round-trip.
type SyncStats struct {
	Pulled bool
	Pushed bool
}

// NotebookRemote manages the notebook's git remote ("origin") and runs
// the actual pull/push round-trip. Implementations shell out to the
// system git binary.
type NotebookRemote interface {
	// GetRemote returns the URL of the notebook's "origin" remote, or
	// ErrNoRemoteConfigured when none is set.
	GetRemote(ctx context.Context, root string) (string, error)
	// SetRemote sets (or replaces) the notebook's "origin" remote URL.
	SetRemote(ctx context.Context, root, url string) error
	// Sync runs `git pull --rebase --autostash origin && git push
	// origin HEAD`. It is the one operation that moves notebook state
	// between machines.
	Sync(ctx context.Context, root string) (SyncStats, error)
}
