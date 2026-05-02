package domain

// Project is one entry in the projects screen — a directory under
// $SOURCECODE_ROOT, optionally annotated with whether a tmux session of
// the same name is already running.
//
// Path is the absolute filesystem path the adapter resolved Name to —
// the use case forwards it to Tmux.NewSessionAt without ever doing its
// own path arithmetic.
type Project struct {
	Name           string
	Path           string
	HasTmuxSession bool
}
