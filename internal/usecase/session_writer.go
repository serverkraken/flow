package usecase

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// SessionWriter is the action surface for the worktime data layer.
// Lifecycle (Start/Stop/Toggle/Pause/Resume/Correct), manual edits
// (AddManual/Edit/Delete), and per-session annotations (SetTag/SetNote)
// all go through this single use case so the locking discipline stays
// uniform.
type SessionWriter struct {
	Sessions ports.SessionStore
	State    ports.ActiveSessionStore
	Lock     ports.Lock
	Reader   *WorktimeReader
	Clock    ports.Clock
}

// — lifecycle —

// Start writes a new session start time. Returns ErrAlreadyRunning when
// a session is already active.
func (w *SessionWriter) Start(ts time.Time) error {
	return w.Lock.With(func() error {
		active, err := w.State.GetActive()
		if err != nil {
			return err
		}
		if active != nil {
			return domain.ErrAlreadyRunning
		}
		return w.State.SetActive(ts)
	})
}

// StartForce overwrites the active marker unconditionally and clears any
// pause marker. Used after the user explicitly confirmed "trotzdem starten".
func (w *SessionWriter) StartForce(ts time.Time) error {
	return w.Lock.With(func() error {
		if err := w.State.SetActive(ts); err != nil {
			return err
		}
		_ = w.State.ClearPause()
		return nil
	})
}

// Stop ends the running session at clock-now and logs it. Returns
// ErrNoActiveSession when nothing is running.
func (w *SessionWriter) Stop() (domain.Session, error) {
	return w.stopAt(w.Clock.Now())
}

// StopAt ends the running session at the given time. The stop time must
// be after the active start time.
func (w *SessionWriter) StopAt(stop time.Time) (domain.Session, error) {
	return w.stopAt(stop)
}

func (w *SessionWriter) stopAt(stop time.Time) (domain.Session, error) {
	var result domain.Session
	err := w.Lock.With(func() error {
		active, err := w.State.GetActive()
		if err != nil {
			return err
		}
		if active == nil {
			return domain.ErrNoActiveSession
		}
		if !stop.After(*active) {
			return errors.New("stoppzeit muss nach Startzeit liegen")
		}
		result = domain.Session{
			Date:    startOfDay(stop),
			Start:   *active,
			Stop:    stop,
			Elapsed: stop.Sub(*active),
		}
		for _, part := range domain.SplitAtMidnight(*active, stop) {
			if err := w.Sessions.Append(part); err != nil {
				return err
			}
		}
		if err := w.State.ClearActive(); err != nil {
			return err
		}
		_ = w.State.ClearPause()
		return nil
	})
	if err != nil {
		return domain.Session{}, err
	}
	return result, nil
}

// Pause stops the running session and records a pause marker. No-op (no
// error) when nothing is running.
func (w *SessionWriter) Pause() (domain.Session, error) {
	s, err := w.Stop()
	if err != nil {
		if errors.Is(err, domain.ErrNoActiveSession) {
			return domain.Session{}, nil
		}
		return domain.Session{}, err
	}
	_ = w.State.SetPause(s.Stop)
	return s, nil
}

// Resume starts a session at clock-now and clears the pause marker.
// Equivalent to Start(now) with the marker cleanup, exposed as a
// distinct verb for the CLI/TUI.
func (w *SessionWriter) Resume() error {
	if err := w.Start(w.Clock.Now()); err != nil {
		return err
	}
	_ = w.State.ClearPause()
	return nil
}

// Toggle starts when idle, stops when running. Returns a human-readable
// description of the action taken.
func (w *SessionWriter) Toggle() (string, error) {
	active, err := w.State.GetActive()
	if err != nil {
		return "", err
	}
	if active != nil {
		s, err := w.Stop()
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("gestoppt nach %dh %02dm",
			int(s.Elapsed.Hours()), int(s.Elapsed.Minutes())%60), nil
	}
	if err := w.Start(w.Clock.Now()); err != nil {
		return "", err
	}
	return "gestartet", nil
}

// CorrectStart overwrites the start time of the running session. Returns
// ErrNoActiveSession when nothing is running.
func (w *SessionWriter) CorrectStart(ts time.Time) error {
	return w.Lock.With(func() error {
		active, err := w.State.GetActive()
		if err != nil || active == nil {
			return domain.ErrNoActiveSession
		}
		return w.State.SetActive(ts)
	})
}

// — manual edits —

// AddManual appends a manual session entry. start..stop crossing midnight
// is split into one row per day. Returns ErrOverlap when any of those
// rows would intersect an existing session.
//
// The first arg (date) is retained for API symmetry with the original
// worktime function. The actual stored row's date is derived from start
// via SplitAtMidnight; the parameter is ignored here.
func (w *SessionWriter) AddManual(_, start, stop time.Time) error {
	if !stop.After(start) {
		return errors.New("stop muss nach Start liegen")
	}
	for _, part := range domain.SplitAtMidnight(start, stop) {
		hit, conflict, err := w.Reader.SessionsOverlap(part.Date, part.Start, part.Stop, -1)
		if err != nil {
			return err
		}
		if hit && conflict != nil {
			return fmt.Errorf("%w (%s, %s → %s)",
				domain.ErrOverlap,
				part.Date.Format("2006-01-02"),
				conflict.Start.Format("15:04"),
				conflict.Stop.Format("15:04"))
		}
	}
	return w.Lock.With(func() error {
		for _, part := range domain.SplitAtMidnight(start, stop) {
			if err := w.Sessions.Append(part); err != nil {
				return err
			}
		}
		return nil
	})
}

// Edit replaces the session at idx (0-based, scoped to date) with the
// given start/stop, preserving its Tag and Note. Returns ErrOverlap when
// the new span intersects another session on the same day.
func (w *SessionWriter) Edit(date time.Time, idx int, newStart, newStop time.Time) error {
	if !newStop.After(newStart) {
		return errors.New("stoppzeit muss nach Startzeit liegen")
	}
	hit, conflict, err := w.Reader.SessionsOverlap(date, newStart, newStop, idx)
	if err != nil {
		return err
	}
	if hit && conflict != nil {
		return fmt.Errorf("%w (%s → %s)",
			domain.ErrOverlap,
			conflict.Start.Format("15:04"), conflict.Stop.Format("15:04"))
	}
	return w.rewriteAtIndex(date, idx, func(s domain.Session) domain.Session {
		return domain.Session{
			Date:    s.Date,
			Start:   newStart,
			Stop:    newStop,
			Elapsed: newStop.Sub(newStart),
			Tag:     s.Tag,
			Note:    s.Note,
		}
	})
}

// Delete removes the session at idx (0-based, scoped to date).
func (w *SessionWriter) Delete(date time.Time, idx int) error {
	return w.Lock.With(func() error {
		all, err := w.Sessions.LoadAll()
		if err != nil {
			return err
		}
		dateStr := date.Format("2006-01-02")
		dayIdx := 0
		out := make([]domain.Session, 0, len(all))
		for _, s := range all {
			if s.Date.Format("2006-01-02") == dateStr {
				if dayIdx != idx {
					out = append(out, s)
				}
				dayIdx++
			} else {
				out = append(out, s)
			}
		}
		return w.Sessions.Rewrite(out)
	})
}

// SetTag sets (or clears, if tag == "") the Tag of the session at idx.
func (w *SessionWriter) SetTag(date time.Time, idx int, tag string) error {
	tag = sanitizeField(tag)
	return w.rewriteAtIndex(date, idx, func(s domain.Session) domain.Session {
		s.Tag = tag
		return s
	})
}

// SetNote sets (or clears, if note == "") the Note of the session at idx.
func (w *SessionWriter) SetNote(date time.Time, idx int, note string) error {
	note = sanitizeField(note)
	return w.rewriteAtIndex(date, idx, func(s domain.Session) domain.Session {
		s.Note = note
		return s
	})
}

// rewriteAtIndex loads the log under lock, applies fn to the session at
// (date, idx), and writes it back. Used by Edit/SetTag/SetNote.
func (w *SessionWriter) rewriteAtIndex(date time.Time, idx int, fn func(domain.Session) domain.Session) error {
	return w.Lock.With(func() error {
		all, err := w.Sessions.LoadAll()
		if err != nil {
			return err
		}
		dateStr := date.Format("2006-01-02")
		dayIdx := 0
		for i, s := range all {
			if s.Date.Format("2006-01-02") == dateStr {
				if dayIdx == idx {
					all[i] = fn(s)
				}
				dayIdx++
			}
		}
		return w.Sessions.Rewrite(all)
	})
}

// startOfDay returns t truncated to 00:00 in t's location.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// sanitizeField strips characters that would break the TSV format.
func sanitizeField(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}
