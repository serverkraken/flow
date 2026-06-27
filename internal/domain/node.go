package domain

import (
	"fmt"
	"time"
)

// NodeStatus is the lifecycle state of a Project.
type NodeStatus string

const (
	NodeActive   NodeStatus = "active"
	NodePaused   NodeStatus = "paused"
	NodeArchived NodeStatus = "archived"
)

// Project is the First-Class hub work sessions book against. M1a uses a
// minimal field set; the heavier foundation fields (repos/paths/links/…)
// arrive in later migrations.
type Node struct {
	ID        string        `json:"id"`
	OwnerID   string        `json:"-"`
	Name      string        `json:"name"`
	Slug      string        `json:"slug"`
	Color     string        `json:"color"`
	Glyph     string        `json:"glyph"`
	Description string      `json:"description"`
	UpstreamGit string      `json:"upstreamGit"`
	Rate      *Money        `json:"rate,omitempty"` // optional per-hour rate (nil = unset)
	Status    NodeStatus `json:"status"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

// NewNode builds a validated, active Node stamped at now.
func NewNode(id, ownerID, name, slug string, now time.Time) (Node, error) {
	switch {
	case id == "":
		return Node{}, fmt.Errorf("%w: id required", ErrInvalidNode)
	case ownerID == "":
		return Node{}, fmt.Errorf("%w: owner required", ErrInvalidNode)
	case name == "":
		return Node{}, fmt.Errorf("%w: name required", ErrInvalidNode)
	case slug == "":
		return Node{}, fmt.Errorf("%w: slug required", ErrInvalidNode)
	}
	return Node{
		ID: id, OwnerID: ownerID, Name: name, Slug: slug,
		Status: NodeActive, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Validate checks the invariants enforced on every mutation: a project needs a
// name and slug, and its status must be one of the three known states.
func (p Node) Validate() error {
	switch {
	case p.Name == "":
		return fmt.Errorf("%w: name required", ErrInvalidNode)
	case p.Slug == "":
		return fmt.Errorf("%w: slug required", ErrInvalidNode)
	case !ValidNodeColor(p.Color):
		return fmt.Errorf("%w: invalid color %q", ErrInvalidNode, p.Color)
	case !ValidNodeGlyph(p.Glyph):
		return fmt.Errorf("%w: invalid glyph %q", ErrInvalidNode, p.Glyph)
	}
	switch p.Status {
	case NodeActive, NodePaused, NodeArchived:
		return nil
	default:
		return fmt.Errorf("%w: invalid status %q", ErrInvalidNode, p.Status)
	}
}
