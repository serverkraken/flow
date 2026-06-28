package domain

import (
	"strings"
	"time"
)

// TaggableType is the polymorphic kind an assignment (tagging) points at.
type TaggableType string

const (
	TaggableDocument    TaggableType = "document"
	TaggableNode        TaggableType = "node"
	TaggableWorkSession TaggableType = "work_session"
	// TaggableAsset is reserved for Phase 2 (B4); not yet a valid taggable_type.
)

// TagMatch selects AND (all slugs) vs OR (any slug) filtering.
type TagMatch int

const (
	TagMatchAll TagMatch = iota // AND — the entity carries ALL given slugs
	TagMatchAny                 // OR  — the entity carries AT LEAST ONE
)

// TagScope narrows ListTags. Type nil = across all taggable types.
// NodeSubtree is reserved for B3 (hierarchy-scoped tag listing) and is unused in B2.
type TagScope struct {
	Type *TaggableType
}

// Tag is a neutral, owner-scoped label in the registry.
type Tag struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"-"`
	Slug      string    `json:"slug"`
	Display   string    `json:"display"`
	CreatedAt time.Time `json:"createdAt"`
}

// NormalizeTag trims and lowercases a raw tag into its slug identity.
// ok=false for an empty/whitespace-only input.
func NormalizeTag(raw string) (slug string, ok bool) {
	slug = strings.ToLower(strings.TrimSpace(raw))
	return slug, slug != ""
}

// NormalizeTags normalizes a list: trim, lower, drop empties, de-duplicate,
// preserving first-seen order. Returns nil for an empty result.
func NormalizeTags(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range in {
		s, ok := NormalizeTag(t)
		if !ok || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
