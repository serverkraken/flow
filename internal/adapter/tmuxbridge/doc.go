// Package tmuxbridge implements ports.Tmux by shelling out to the tmux
// binary. The injectable runner makes the adapter unit-testable without
// requiring a live tmux server: production wiring uses os/exec, tests
// pass a fake that records calls and returns canned responses.
//
// Empty stdout from tmux is treated as "no value" so callers can fall
// back to defaults without parsing exit codes (ShowOption,
// CurrentSessionName).
package tmuxbridge
