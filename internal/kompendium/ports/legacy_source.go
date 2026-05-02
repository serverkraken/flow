package ports

import "context"

// LegacyDaily is a daily note discovered in the old tmux `notes` plugin
// layout (one YYYY-MM-DD.md per day, flat under $NOTES_DIR).
type LegacyDaily struct {
	Path string
	Date string
	Body []byte
}

// LegacyProject is a project note discovered in the old tmux
// `project-notes` plugin layout (one file per repo under
// ~/.project-notes/, with the canonical-URL hash baked into the filename
// and the original `Remote: <url>` line in a boilerplate header).
type LegacyProject struct {
	Path string
	URL  string
	Body []byte
}

// LegacySource enumerates legacy note files. The adapter owns the on-disk
// naming conventions and boilerplate parsing; the use case stays free of
// filesystem and regex concerns.
type LegacySource interface {
	ListDailyNotes(ctx context.Context, sourceDir string) ([]LegacyDaily, error)
	ListProjectNotes(ctx context.Context, sourceDir string) ([]LegacyProject, error)
}
