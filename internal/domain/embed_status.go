package domain

import "time"

// EmbedState is a document's embedding state, derived from chunk freshness and
// any recorded embed-failure.
type EmbedState string

const (
	EmbedOK       EmbedState = "ok"       // chunks current
	EmbedPending  EmbedState = "pending"  // stale, queued, no failure recorded
	EmbedRetrying EmbedState = "retrying" // failed, within backoff, will retry
	EmbedFailed   EmbedState = "failed"   // dead-lettered; needs a manual retry
)

// EmbedStatus is the read model describing a document's embedding state.
type EmbedStatus struct {
	State     EmbedState
	Attempts  int
	LastError string
	NextRetry *time.Time
}
