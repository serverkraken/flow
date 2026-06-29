package domain

import (
	"fmt"
	"time"
)

// NodeStatus is the lifecycle state of a Node.
type NodeStatus string

const (
	NodeActive   NodeStatus = "active"
	NodePaused   NodeStatus = "paused"
	NodeArchived NodeStatus = "archived"
)

// NodeKind is the level of a node in the engagement→vorhaben→repo→branch tree.
type NodeKind string

const (
	KindEngagement NodeKind = "engagement"
	KindVorhaben   NodeKind = "vorhaben"
	KindRepo       NodeKind = "repo"
	KindBranch     NodeKind = "branch" // B1: reserved only; no behavior
)

// Node is the First-Class hub work sessions book against. M1a uses a
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
	ParentID    *string        `json:"parentId,omitempty"`
	Kind        NodeKind       `json:"kind"`
	OriginSlug  string         `json:"originSlug,omitempty"`
	Extra       map[string]any `json:"extra,omitempty"`
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

// Validate checks the invariants enforced on every mutation: a node needs a
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

// ValidParentKind reports whether a node of childKind may hang under parentKind.
// Root placement (nil parent) is handled by the caller (root must be engagement).
func ValidParentKind(child, parent NodeKind) bool {
	switch child {
	case KindVorhaben, KindRepo:
		return parent == KindEngagement || parent == KindVorhaben
	case KindBranch:
		return parent == KindRepo
	default: // engagement (or unknown) may never have a parent
		return false
	}
}

// AllowedChildKind reports whether parentKind may have a child of childKind.
func AllowedChildKind(parent, child NodeKind) bool { return ValidParentKind(child, parent) }

// IsBookable reports whether worktime may be booked to a node of this kind.
// Engagement, Vorhaben and Repo are bookable; Branch is reserved (B1) and not.
func IsBookable(k NodeKind) bool {
	return k == KindEngagement || k == KindVorhaben || k == KindRepo
}

// ResolveEngagement returns the engagement from an ancestor chain ordered
// leaf→root (as NodeStore.Ancestors returns). The engagement is the last
// element (root); ok=false if the chain is empty or its root is not an engagement.
func ResolveEngagement(chain []Node) (Node, bool) {
	if len(chain) == 0 {
		return Node{}, false
	}
	root := chain[len(chain)-1]
	if root.Kind != KindEngagement {
		return Node{}, false
	}
	return root, true
}

// ResolveRate returns the effective rate for a node by walking its ancestor
// chain (leaf→root, as returned by NodeStore.Ancestors): the nearest node that
// carries a rate wins. Returns nil when no node in the chain has a rate.
func ResolveRate(chain []Node) *Money {
	for _, n := range chain {
		if n.Rate != nil {
			return n.Rate
		}
	}
	return nil
}
