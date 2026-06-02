package domain

import "time"

// User is the authenticated identity that owns Projects, Sessions, Repos
// and RepoNotes. One-to-one with an OIDC `sub` claim. Phase 1 ships with
// exactly one User per server instance (the allowlisted owner); Phase 2
// adds multi-user via Authentik group claims.
type User struct {
	ID          string // UUID v4
	OIDCSub     string // unique
	Email       string
	DisplayName string
	CreatedAt   time.Time
}
