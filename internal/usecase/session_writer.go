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
	State    ports.LegacyActiveStore
	Lock     ports.Lock
	Reader   *WorktimeReader
	Clock    ports.Clock

	// UserID scopes every Sessions.Load / Sessions.Delete / Sessions.Upsert
	// call so the sqliteclient backend can multiplex multiple users in the
	// same cache.db. Set by the composition root at construction time;
	// SessionWriter itself never mutates it.
	UserID string
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
		return w.State.ClearPause()
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
			return domain.ErrStopBeforeStart
		}
		// Date anchors on the *start* day so a session that crosses
		// midnight returns to the caller with the day it began on,
		// matching the first row written by SplitAtMidnight. Anchoring
		// on stop would silently re-attribute the whole session to the
		// next calendar day in any caller that prints result.Date.
		result = domain.Session{
			Date:    startOfDay(*active),
			Start:   *active,
			Stop:    stop,
			Elapsed: stop.Sub(*active),
		}
		// AppendBatch (review finding B1): a multi-midnight stop split
		// into N parts must persist atomically. The previous loop wrote
		// each part with its own Append; a failure on part N>0 left the
		// earlier parts on disk and the natural retry — same active
		// marker, same SplitAtMidnight output — duplicated them.
		//
		// Round4: also dedupe against existing sessions. If a previous
		// Stop succeeded at AppendBatch but failed at ClearActive (e.g.
		// transient state-store error), the active marker stayed set
		// and the user's retry would otherwise re-AppendBatch the same
		// rows. Filter out any part whose (Date, Start) matches an
		// already-persisted session. After the filter the work is a
		// no-op AppendBatch — ClearActive is the actual retry step.
		parts := domain.SplitAtMidnight(*active, stop)
		existing, err := w.Sessions.Load(w.UserID)
		if err != nil {
			return err
		}
		toAppend := dedupeSessionParts(parts, existing)
		if err := w.upsertParts(toAppend); err != nil {
			return err
		}
		if err := w.State.ClearActive(); err != nil {
			return err
		}
		return w.State.ClearPause()
	})
	if err != nil {
		return domain.Session{}, err
	}
	return result, nil
}

// Pause stops the running session and records a pause marker.
//
// Idempotency contract (review finding Q3): when no session is running,
// returns (zero Session, nil) — NOT ErrNoActiveSession. tmux bindings
// invoke Pause blindly without first checking state, and surfacing
// ErrNoActiveSession there as a red status flash would be wrong (the
// user already has the state they wanted). Callers that need to
// distinguish "paused something" from "nothing was running" should
// check the returned Session's zero-value (Start.IsZero()).
//
// ErrStopBeforeStart (clock-now is at or before the start instant —
// typically an NTP backwards-jump) is treated under the same contract:
// the user wanted to pause, the system can't record a coherent session
// at this instant, so we leave the active state alone and report a
// zero Session. The next Pause/Stop after the clock catches up records
// the session normally. The whole closure runs before any state
// mutation, so the active marker is preserved.
//
// Stop, in contrast, does NOT swallow ErrNoActiveSession — the CLI
// handler at frontend/cli/worktime.go does the errors.Is check and
// translates it to a silent exit-0. The asymmetry is deliberate: Stop
// returns the last session for printing, Pause does not.
//
// Both writes happen under one Lock.With so a concurrent Start can't
// slip between Stop's ClearActive and SetPause and leave both
// worktime.state and worktime.pause populated.
func (w *SessionWriter) Pause() (domain.Session, error) {
	var result domain.Session
	err := w.Lock.With(func() error {
		active, err := w.State.GetActive()
		if err != nil {
			return err
		}
		if active == nil {
			return domain.ErrNoActiveSession
		}
		stop := w.Clock.Now()
		if !stop.After(*active) {
			return domain.ErrStopBeforeStart
		}
		// Date anchors on the start day — see SessionWriter.stopAt for the
		// midnight-cross rationale.
		result = domain.Session{
			Date:    startOfDay(*active),
			Start:   *active,
			Stop:    stop,
			Elapsed: stop.Sub(*active),
		}
		if err := w.upsertParts(domain.SplitAtMidnight(*active, stop)); err != nil {
			return err
		}
		if err := w.State.ClearActive(); err != nil {
			return err
		}
		return w.State.SetPause(stop)
	})
	if err != nil {
		if errors.Is(err, domain.ErrNoActiveSession) || errors.Is(err, domain.ErrStopBeforeStart) {
			return domain.Session{}, nil
		}
		return domain.Session{}, err
	}
	return result, nil
}

// Resume starts a session at clock-now and clears the pause marker.
// Equivalent to Start(now) with the marker cleanup, exposed as a
// distinct verb for the CLI/TUI.
//
// Idempotent: when a session is already running we just clear the
// pause marker. tmux bindings invoke this blindly (CLAUDE.md
// "idempotency in flow worktime <verb>") and surfacing
// ErrAlreadyRunning as a red status flash there is wrong — the user
// already has the state they wanted.
//
// SetActive and ClearPause run under a single Lock.With so a concurrent
// Pause can't slip between the two and end up with both markers set.
func (w *SessionWriter) Resume() error {
	now := w.Clock.Now()
	return w.Lock.With(func() error {
		active, err := w.State.GetActive()
		if err != nil {
			return err
		}
		if active == nil {
			if err := w.State.SetActive(now); err != nil {
				return err
			}
		}
		return w.State.ClearPause()
	})
}

// Toggle starts when idle, stops when running. Returns a human-readable
// description of the action taken.
//
// Read, decide, and write happen under a single Lock.With — without that
// two concurrent toggle calls (e.g. tmux binding double-press, or
// toggle from one pane while another runs `flow worktime stop`) could
// both observe "idle" and both call Start, or one's read could race
// with the other's write.
func (w *SessionWriter) Toggle() (string, error) {
	now := w.Clock.Now()
	var msg string
	err := w.Lock.With(func() error {
		active, err := w.State.GetActive()
		if err != nil {
			return err
		}
		if active != nil {
			if !now.After(*active) {
				return domain.ErrStopBeforeStart
			}
			// Date anchors on the start day — see SessionWriter.stopAt for the
			// midnight-cross rationale.
			s := domain.Session{
				Date:    startOfDay(*active),
				Start:   *active,
				Stop:    now,
				Elapsed: now.Sub(*active),
			}
			if err := w.upsertParts(domain.SplitAtMidnight(*active, now)); err != nil {
				return err
			}
			if err := w.State.ClearActive(); err != nil {
				return err
			}
			if err := w.State.ClearPause(); err != nil {
				return err
			}
			msg = fmt.Sprintf("gestoppt nach %dh %02dm",
				int(s.Elapsed.Hours()), int(s.Elapsed.Minutes())%60)
			return nil
		}
		if err := w.State.SetActive(now); err != nil {
			return err
		}
		msg = "gestartet"
		return nil
	})
	if err != nil {
		return "", err
	}
	return msg, nil
}

// CorrectStart overwrites the start time of the running session. Returns
// ErrNoActiveSession when nothing is running. Real I/O errors from the
// state read surface verbatim — masking them as "no active session"
// would point a user with a permission-denied state file at the wrong
// problem and let the next Start overwrite it.
func (w *SessionWriter) CorrectStart(ts time.Time) error {
	return w.Lock.With(func() error {
		active, err := w.State.GetActive()
		if err != nil {
			return err
		}
		if active == nil {
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
//
// Overlap check and append run under one Lock.With so a concurrent
// writer can't slip a colliding session in between.
func (w *SessionWriter) AddManual(_, start, stop time.Time) error {
	if !stop.After(start) {
		return domain.ErrStopBeforeStart
	}
	return w.Lock.With(func() error {
		parts := domain.SplitAtMidnight(start, stop)
		for _, part := range parts {
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
		// AppendBatch (review finding B1): same partial-failure
		// reasoning as stopAt — a manual entry crossing midnight must
		// either land entirely or not at all.
		return w.Sessions.AppendBatch(parts)
	})
}

// Edit replaces the session at idx (0-based, scoped to date) with the
// given start/stop, preserving its Tag and Note. Returns ErrOverlap when
// the new span intersects another session on the same day, or
// ErrSessionNotFound when idx is out of range for that day.
//
// Overlap check, lookup and rewrite all happen under one Lock.With.
func (w *SessionWriter) Edit(date time.Time, idx int, newStart, newStop time.Time) error {
	if !newStop.After(newStart) {
		return domain.ErrStopBeforeStart
	}
	return w.Lock.With(func() error {
		hit, conflict, err := w.Reader.SessionsOverlap(date, newStart, newStop, idx)
		if err != nil {
			return err
		}
		if hit && conflict != nil {
			return fmt.Errorf("%w (%s → %s)",
				domain.ErrOverlap,
				conflict.Start.Format("15:04"), conflict.Stop.Format("15:04"))
		}
		return w.rewriteAtIndexLocked(date, idx, func(s domain.Session) domain.Session {
			return domain.Session{
				Date:    s.Date,
				Start:   newStart,
				Stop:    newStop,
				Elapsed: newStop.Sub(newStart),
				Tag:     s.Tag,
				Note:    s.Note,
			}
		})
	})
}

// Delete removes the session at idx (0-based, scoped to date). Returns
// ErrSessionNotFound when idx is out of range so stale CLI input like
// `flow worktime delete 99` against a day with 3 sessions surfaces an
// error instead of silently rewriting the unchanged log and reporting
// success — same contract Edit / SetTag / SetNote already enforce.
func (w *SessionWriter) Delete(date time.Time, idx int) error {
	return w.Lock.With(func() error {
		all, err := w.Sessions.Load(w.UserID)
		if err != nil {
			return err
		}
		dateStr := date.Format("2006-01-02")
		dayIdx := 0
		found := false
		out := make([]domain.Session, 0, len(all))
		for _, s := range all {
			if s.Date.Format("2006-01-02") == dateStr {
				if dayIdx == idx {
					found = true
				} else {
					out = append(out, s)
				}
				dayIdx++
			} else {
				out = append(out, s)
			}
		}
		if !found {
			return domain.ErrSessionNotFound
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

// rewriteAtIndex acquires the lock and delegates to the locked variant.
// Used by SetTag/SetNote which have no extra pre-checks.
func (w *SessionWriter) rewriteAtIndex(date time.Time, idx int, fn func(domain.Session) domain.Session) error {
	return w.Lock.With(func() error {
		return w.rewriteAtIndexLocked(date, idx, fn)
	})
}

// rewriteAtIndexLocked loads the log, applies fn to the session at
// (date, idx), and writes it back. Caller must hold the Lock. Returns
// ErrSessionNotFound when no session exists at the requested index for
// that day — without this signal the rewrite was a silent no-op for
// stale CLI input like `flow worktime tag 99 deep`.
func (w *SessionWriter) rewriteAtIndexLocked(date time.Time, idx int, fn func(domain.Session) domain.Session) error {
	all, err := w.Sessions.Load(w.UserID)
	if err != nil {
		return err
	}
	dateStr := date.Format("2006-01-02")
	dayIdx := 0
	found := false
	for i, s := range all {
		if s.Date.Format("2006-01-02") != dateStr {
			continue
		}
		if dayIdx == idx {
			all[i] = fn(s)
			found = true
		}
		dayIdx++
	}
	if !found {
		return domain.ErrSessionNotFound
	}
	return w.Sessions.Rewrite(all)
}

// startOfDay returns t truncated to 00:00 in t's location.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// upsertParts assigns a fresh UUID + the writer's UserID + an UpdatedAt
// stamp to each row before writing it through Sessions.Upsert one-by-one.
// Replaces the legacy AppendBatch call that lifecycle paths used while
// the TSV adapter still satisfied ports.SessionStore.
//
// Empty slice is a no-op — keeps stopAt's dedupe-yielded-empty branch
// quiet without a wrapping len() check at the call site.
func (w *SessionWriter) upsertParts(parts []domain.Session) error {
	if len(parts) == 0 {
		return nil
	}
	now := w.Clock.Now().UTC()
	for i := range parts {
		if parts[i].ID == "" {
			parts[i].ID = newUUID()
		}
		parts[i].UserID = w.UserID
		parts[i].UpdatedAt = now
		if err := w.Sessions.Upsert(parts[i]); err != nil {
			return err
		}
	}
	return nil
}

// sanitizeField strips characters that would break the TSV format.
func sanitizeField(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

// dedupeSessionParts returns the subset of parts that don't yet exist
// in existing. A part is considered "already persisted" when an
// existing session shares its Date AND Start instant — SplitAtMidnight
// is deterministic per (start, stop) input, so equality on those two
// fields uniquely identifies a slice.
//
// Used by Stop to make the AppendBatch step idempotent: if a prior
// Stop succeeded at AppendBatch but failed at ClearActive, a retry
// sees the same active marker, computes the same parts, and would
// otherwise duplicate them. With the dedupe, the retry's AppendBatch
// becomes a no-op and ClearActive is the only step that actually
// happens.
func dedupeSessionParts(parts, existing []domain.Session) []domain.Session {
	if len(existing) == 0 {
		return parts
	}
	keep := make([]domain.Session, 0, len(parts))
	for _, p := range parts {
		duplicate := false
		for _, e := range existing {
			if e.Date.Equal(p.Date) && e.Start.Equal(p.Start) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			keep = append(keep, p)
		}
	}
	return keep
}
