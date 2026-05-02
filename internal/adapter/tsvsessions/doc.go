// Package tsvsessions implements ports.SessionStore as a TSV file at the
// configured path (typically ~/.tmux/worktime.log).
//
// File format: each session is one row of
// `date<TAB>HH:MM<TAB>HH:MM<TAB>elapsedSec[<TAB>tag[<TAB>note]]`.
// Trailing tag/note columns are omitted when empty so historical
// 4-column readers still parse the file.
//
// Multi-day spans must be split into one Session per day by the caller —
// see domain.SplitAtMidnight.
//
// Locking: the adapter does not lock. Callers wrap mutations in a
// ports.Lock callback (typically a flock on worktime.lock) so concurrent
// writers serialise. Readers (LoadAll, LoadFiltered) are unlocked by
// design — the TUI's per-second tick would amplify contention and the
// log shape tolerates a stale read between writes.
package tsvsessions
