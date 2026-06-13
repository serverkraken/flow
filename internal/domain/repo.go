package domain

import "time"

// Repo is a git or local-path-identified working directory that can hold
// RepoNotes (cf. M4 / Plan C). The CanonicalKey is what makes the Repo
// addressable across devices:
//
//	git:<host>/<owner>/<repo>   — from `git remote get-url origin`, normalised
//	path:<sha256-hex>           — for repos without a git remote
//
// Two devices that clone the same upstream see the same Repo even when
// the local path differs.
type Repo struct {
	ID           string // UUID v4
	UserID       string
	CanonicalKey string
	DisplayName  string
	CreatedAt    time.Time
	Version      int64 // server-incremented Lamport; 0 until first server roundtrip
}
