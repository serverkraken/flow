package ports

import "time"

// PauseStore is per-device (never synced) — a tiny marker for the
// worktime pause flow. Lives in flockstate; not part of the multi-
// device sync model.
type PauseStore interface {
	GetPause() (*time.Time, error)
	SetPause(t time.Time) error
	ClearPause() error
}
