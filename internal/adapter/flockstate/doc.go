// Package flockstate implements the active-session markers and the
// process-level mutex on top of ~/.tmux/.
//
// State satisfies ports.PauseStore (GetPause/SetPause/ClearPause) and
// ports.LegacyActiveStore (GetActive/SetActive/ClearActive) via two
// single-line files (worktime.state, worktime.pause), each holding a Unix
// epoch. The Active-state methods are in legacy_active.go and will be
// removed in Task 12 once the use cases migrate to the new multi-device
// sqliteclient.ActiveSessions.
//
// Lock satisfies ports.Lock via syscall.Flock on a configurable path
// (typically ~/.tmux/worktime.lock). The lock is process-level (per open
// file description) so concurrent goroutines and concurrent CLI/TUI
// invocations both serialise.
package flockstate
