// Package output implements ports.Output: the worktime menu's three
// output targets (clipboard / tmux-Split with pager / save to a file
// in the user's Downloads folder).
//
// Construction lives in output.go; each method has its own file
// (copy.go, pager.go, file.go) so the adapter stays small and a future
// reader sees one concern per file. ports.Tmux is consumed at
// construction so Pager can dispatch a tmux split-window without the
// caller having to thread tmux through the menu layer.
package output
