package ports

import "github.com/serverkraken/flow/internal/domain"

// FlowStateStore persists the TUI's last screen / filter / cursor across
// sessions, plus the one-shot deep-link file written by goto.sh.
type FlowStateStore interface {
	// Load returns the last persisted state. When the underlying file is
	// missing or malformed, the implementation returns DefaultFlowState
	// with a nil error — first launch is normal, not a failure.
	Load() (domain.FlowState, error)
	// Save writes s to disk.
	Save(s domain.FlowState) error
	// ConsumeNextScreen returns the screen name written by goto.sh and
	// removes the marker so it fires only once. "" when no deep-link is
	// pending.
	ConsumeNextScreen() (string, error)
	// WriteNextScreen writes a one-shot deep-link target, used by goto.sh
	// or any external trigger.
	WriteNextScreen(screen string) error
}
