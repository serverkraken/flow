// Package domain holds the pure value types and rules of flow.
// It must not import any I/O package.
package domain

import "errors"

var (
	ErrInvalidUser     = errors.New("invalid user")
	ErrInvalidProject  = errors.New("invalid project")
	ErrInvalidSession  = errors.New("invalid session")
	ErrAlreadyRunning  = errors.New("a session is already running")
	ErrNoActiveSession = errors.New("no active session")
	ErrStopBeforeStart = errors.New("stop must be after start")
	ErrProjectRequired = errors.New("a project is required to book the session")
	ErrFutureSession   = errors.New("session times must not be in the future")
	ErrOverlap         = errors.New("session overlaps an existing session")
	ErrInvalidDayOff   = errors.New("invalid day-off")
	ErrInvalidRange    = errors.New("invalid range")
	ErrInvalidTarget   = errors.New("invalid target")
	ErrInvalidRate     = errors.New("invalid rate")
	ErrInvalidDocument = errors.New("invalid document")
)
