// Package project_picker is a full-screen bubbletea modal that lets the
// user choose a worktime Project from a MRU-sorted list or create a new
// one inline. It is decoupled from any specific screen or use-case; the
// caller injects its domain logic through three callbacks:
//
//   - onPick:   called when the user selects an existing project
//   - onCreate: called when the user confirms a new project name
//   - onCancel: emitted (as a static tea.Msg) when the user presses Esc
//
// # Chrome
//
// The picker uses the same rounded-box chrome as markdown_overlay: a
// centered titlebox frame (theme.Palette-driven, BorderStrong), a filter
// input line, a separator rule, a scrollable list of items, a sticky
// "+ Neues Projekt anlegen" entry at the bottom, and a hint footer.
//
// # Filter
//
// Typing any printable character appends to the filter string; Backspace
// removes the last rune. The list is re-scored through
// github.com/sahilm/fuzzy on every keystroke. The "+ Neues Projekt
// anlegen" pseudo-row is never hidden by the filter — it is always the
// last visible entry.
//
// # Keybindings
//
//	↑/↓ or j/k   navigate up/down (wraps at both ends)
//	Tab           jump cursor to the "+ Neues Projekt anlegen" entry
//	Enter         pick selected item, or create new project if on "+ Neu"
//	Esc           emit onCancel and return control to the host
//	<rune>        append character to the filter
//	Backspace     remove last filter character
//
// # Construction sequence
//
//	m := project_picker.New(projects, palette, onPick, onCreate, onCancelMsg)
//	m = m.SetSize(windowWidth, windowHeight)
//	// In Update: route all tea.Msg through m.Update(msg)
//	// In View:   render m.View()
package project_picker
