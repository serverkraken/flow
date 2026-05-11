package domain

import "errors"

// ErrNoActiveSession signals an attempt to stop, pause, or correct when
// nothing is running. Callers can branch on errors.Is to turn it into a
// no-op (e.g. tmux's blind-stop binding).
var ErrNoActiveSession = errors.New("keine aktive Session")

// ErrAlreadyRunning is returned by Start when a session is already active.
// Prevents silent overwrite of the running state — the caller must Stop
// first or call StartForce explicitly.
var ErrAlreadyRunning = errors.New("session läuft bereits")

// ErrOverlap is returned by AddManual / EditSession when the requested
// span intersects an existing session on the same date. Callers (TUI, CLI)
// can detect this with errors.Is and present a precise hint instead of a
// generic failure.
var ErrOverlap = errors.New("überschneidet eine bestehende Session")

// ErrSessionNotFound is returned when an Edit/SetTag/SetNote/Delete asks
// for a session index that does not exist on the requested date. Without
// this signal the rewrite path silently no-ops, hiding bad CLI input.
var ErrSessionNotFound = errors.New("session nicht gefunden")

// ErrStopBeforeStart is returned by Stop / Pause / Toggle / Edit when
// the requested stop time is not strictly after the start time. Callers
// can branch via errors.Is to surface a localised hint instead of the
// raw error string. Pre-Q5 every call site duplicated the literal
// `errors.New("stoppzeit muss nach Startzeit liegen")`, which made the
// chain undistinguishable.
var ErrStopBeforeStart = errors.New("stoppzeit muss nach Startzeit liegen")
