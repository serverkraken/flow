package domain

// SourceDir is one entry in the SourceDir / Projects screen — a directory
// under $SOURCECODE_ROOT, optionally annotated with whether a tmux session
// of the same name is already running. Distinct from `domain.Project`
// (worktime category, M2).
//
// Path is the absolute filesystem path the adapter resolved Name to —
// the use case forwards it to Tmux.NewSessionAt without ever doing its
// own path arithmetic.
type SourceDir struct {
	Name           string
	Path           string
	HasTmuxSession bool
}
