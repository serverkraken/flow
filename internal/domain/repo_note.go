package domain

import "time"

// RepoNote is a Markdown note tied to a Repo + User. Phase 1 / M4 ships
// one note per (Repo, User) — the canonical "CLAUDE.md for this repo"
// content. The schema allows multiple notes per repo in the future
// (e.g. shared sticky-notes in Phase 2), so RepoNote has its own ID.
type RepoNote struct {
	ID        string
	RepoID    string
	UserID    string
	Content   string
	Version   int64
	UpdatedAt time.Time
}
