// Package flockstate implements the active-session markers and the
// process-level mutex on top of ~/.tmux/.
//
// State satisfies ports.LegacyActiveStore via two single-line files
// (worktime.state, worktime.pause), each holding a Unix epoch.
//
// Lock satisfies ports.Lock via syscall.Flock on a configurable path
// (typically ~/.tmux/worktime.lock). The lock is process-level (per open
// file description) so concurrent goroutines and concurrent CLI/TUI
// invocations both serialise.
package flockstate
