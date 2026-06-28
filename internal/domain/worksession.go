package domain

import (
	"fmt"
	"time"
)

// WorkSession is one work interval owned by a user. Stop == nil marks the
// single active timer. Elapsed is derived, never stored.
type WorkSession struct {
	ID        string     `json:"id"`
	OwnerID   string     `json:"-"`
	NodeID *string    `json:"projectId,omitempty"`
	Tags      []string   `json:"tags,omitempty"`
	Note      string     `json:"note,omitempty"`
	Start     time.Time  `json:"start"`
	Stop      *time.Time `json:"stop,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// Running reports whether this is the active (unstopped) timer.
func (s WorkSession) Running() bool { return s.Stop == nil }

// Elapsed returns the session duration. For a running session it measures
// against now; for a stopped one, stop-start (now is ignored).
func (s WorkSession) Elapsed(now time.Time) time.Duration {
	if s.Stop != nil {
		return s.Stop.Sub(s.Start)
	}
	return now.Sub(s.Start)
}

// NewWorkSession builds a validated, running session (Stop nil). nodeID
// is optional at start and mandatory at stop (enforced in StopSession).
func NewWorkSession(id, ownerID string, nodeID *string, start time.Time) (WorkSession, error) {
	switch {
	case id == "":
		return WorkSession{}, fmt.Errorf("%w: id required", ErrInvalidSession)
	case ownerID == "":
		return WorkSession{}, fmt.Errorf("%w: owner required", ErrInvalidSession)
	}
	return WorkSession{ID: id, OwnerID: ownerID, NodeID: nodeID, Start: start, CreatedAt: start}, nil
}
