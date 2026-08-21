package domain

import "time"

// NodeHighlight is a human-marked passage of a document (typically a daily
// note, Screen 27 "Stellen markieren und je Register zuordnen") assigned to
// one register. Quote is the plain marked text, matched verbatim against the
// source document's rendered body to place the visual highlight — it carries
// no character offsets, since Markdown source and rendered HTML positions
// diverge.
type NodeHighlight struct {
	ID         string
	OwnerID    string
	DocumentID string
	NodeID     string
	Quote      string
	CreatedAt  time.Time
}
