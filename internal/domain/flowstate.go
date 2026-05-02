package domain

// Screen identifiers — must match what goto.sh writes into ~/.cache/flow/next-screen.
const (
	ScreenPalette    = "palette"
	ScreenProjects   = "projects"
	ScreenWorktime   = "worktime"
	ScreenCheatsheet = "cheatsheet"
)

// FlowState holds the persisted UI state restored on next launch.
type FlowState struct {
	Screen string `json:"screen"`
	Filter string `json:"filter"`
	Cursor int    `json:"cursor"`
}

// IsValidScreen reports whether s is one of the four screen identifiers.
func IsValidScreen(s string) bool {
	switch s {
	case ScreenPalette, ScreenProjects, ScreenWorktime, ScreenCheatsheet:
		return true
	}
	return false
}

// DefaultFlowState returns a fresh state with the palette as the active screen.
func DefaultFlowState() FlowState { return FlowState{Screen: ScreenPalette} }
