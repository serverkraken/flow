package httpsync

import (
	"encoding/json"

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

// EnqueueActiveStart enqueues an active-session start action.
func (q *Queue) EnqueueActiveStart(projectID, device string, expectedVersion int64) (int64, error) {
	payload, err := json.Marshal(struct {
		Action          string `json:"action"`
		ProjectID       string `json:"project_id"`
		StartedOnDevice string `json:"started_on_device"`
	}{"start", projectID, device})
	if err != nil {
		return 0, err
	}
	return q.inner.Enqueue("active_sessions", projectID, payload, expectedVersion)
}

// EnqueueActiveStop enqueues an active-session stop action.
func (q *Queue) EnqueueActiveStop(projectID string, expectedVersion int64, tag, note string) (int64, error) {
	payload, err := json.Marshal(struct {
		Action string `json:"action"`
		Tag    string `json:"tag"`
		Note   string `json:"note"`
	}{"stop", tag, note})
	if err != nil {
		return 0, err
	}
	return q.inner.Enqueue("active_sessions_stop", projectID, payload, expectedVersion)
}

// DrainCallback receives one queue entry and decides what to do with it.
//
//   - ok=true, err=nil   → entry succeeded; Drain removes it.
//   - ok=false, err!=nil → entry failed; Drain records the error via SetError and
//     continues with the next entry.
//   - ok=false, err=nil  → conflict or signal; Drain halts immediately and returns nil.
type DrainCallback func(e ports.WriteQueueEntry) (ok bool, err error)

// Drain fetches up to 50 pending entries from the inner queue and processes
// each via cb. See DrainCallback for the three-way contract.
func (q *Queue) Drain(cb DrainCallback) error {
	entries, err := q.inner.Peek(50)
	if err != nil {
		return err
	}
	for _, e := range entries {
		ok, cbErr := cb(e)
		switch {
		case ok:
			if rerr := q.inner.Remove(e.Seq); rerr != nil {
				return rerr
			}
		case cbErr != nil:
			if serr := q.inner.SetError(e.Seq, cbErr.Error()); serr != nil {
				return serr
			}
		default:
			// conflict / signal: halt drain
			return nil
		}
	}
	return nil
}
