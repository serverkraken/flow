package httpsync

import (
	"encoding/json"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Queue wraps ports.WriteQueue with typed helpers for each resource kind.
// It serialises domain values to JSON before handing them to the inner queue,
// keeping the inner queue free from domain imports.
type Queue struct{ inner ports.WriteQueue }

// NewQueue constructs a Queue backed by inner.
func NewQueue(inner ports.WriteQueue) *Queue { return &Queue{inner: inner} }

// EnqueueSession serialises s and enqueues it under resource "sessions".
func (q *Queue) EnqueueSession(s domain.Session, expectedVersion int64) (int64, error) {
	payload, err := json.Marshal(s)
	if err != nil {
		return 0, err
	}
	return q.inner.Enqueue("sessions", s.ID, payload, expectedVersion)
}

// EnqueueProject serialises p and enqueues it under resource "projects".
func (q *Queue) EnqueueProject(p domain.Project, expectedVersion int64) (int64, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return 0, err
	}
	return q.inner.Enqueue("projects", p.ID, payload, expectedVersion)
}

// EnqueueRepo enqueues a repo Upsert push.
func (q *Queue) EnqueueRepo(r domain.Repo, expectedVersion int64) (int64, error) {
	payload, err := json.Marshal(r)
	if err != nil {
		return 0, err
	}
	return q.inner.Enqueue("repos", r.ID, payload, expectedVersion)
}

// EnqueueRepoNote enqueues a repo-note Upsert push.
func (q *Queue) EnqueueRepoNote(n domain.RepoNote, expectedVersion int64) (int64, error) {
	payload, err := json.Marshal(n)
	if err != nil {
		return 0, err
	}
	return q.inner.Enqueue("repo_notes", n.ID, payload, expectedVersion)
}

// DrainAction describes how Queue.Drain should treat the entry handed to a
// DrainCallback.
type DrainAction int

const (
	// DrainAck removes the entry — used on 2xx and on permanent 4xx where
	// retrying would never succeed (the entry is logged and dropped).
	DrainAck DrainAction = iota
	// DrainRetry schedules the entry for a later attempt via SetRetry. The
	// caller supplies the error message; Queue.Drain owns the timestamp
	// computation via its Backoff.
	DrainRetry
	// DrainHalt stops draining without modifying the entry — used on 409
	// conflicts which must be resolved by the user before further drains.
	DrainHalt
)

// DrainCallback receives one queue entry and decides what to do with it.
//
//   - DrainAck, nil               → entry succeeded (or permanently failed); Drain Removes it.
//   - DrainAck, err!=nil          → permanent failure with message; Drain calls SetError then Removes.
//   - DrainRetry, err             → transient failure; Drain calls SetRetry with backoff and continues.
//   - DrainHalt, nil              → conflict/signal; Drain halts immediately.
type DrainCallback func(e ports.WriteQueueEntry) (DrainAction, error)

// Drain fetches up to 50 eligible entries from the inner queue (Peek filters
// out entries whose next_retry_at hasn't elapsed) and processes each via cb.
// The Backoff supplied to NewWorker controls retry timing.
func (q *Queue) Drain(cb DrainCallback, backoff Backoff) error {
	entries, err := q.inner.Peek(50)
	if err != nil {
		return err
	}
	for _, e := range entries {
		action, cbErr := cb(e)
		switch action {
		case DrainAck:
			if cbErr != nil {
				// Permanent failure — record the message for observability,
				// then remove. The Remove must not be skipped on SetError
				// failure: the row is already past the retry decision.
				_ = q.inner.SetError(e.Seq, cbErr.Error())
			}
			if rerr := q.inner.Remove(e.Seq); rerr != nil {
				return rerr
			}
		case DrainRetry:
			msg := ""
			if cbErr != nil {
				msg = cbErr.Error()
			}
			next := time.Now().UTC().Add(backoff.For(e.Attempt)).Format(time.RFC3339)
			if serr := q.inner.SetRetry(e.Seq, msg, next); serr != nil {
				return serr
			}
		case DrainHalt:
			return nil
		}
	}
	return nil
}
