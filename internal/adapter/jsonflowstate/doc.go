// Package jsonflowstate implements ports.FlowStateStore against two
// files in ~/.cache/flow/:
//
//   - state.json — JSON-encoded domain.FlowState restored on each launch.
//   - next-screen — one-line deep-link marker written by goto.sh.
//
// Both paths are configurable per-instance so the composition root can
// point at XDG-specific locations or test directories.
//
// Missing-or-malformed state.json yields DefaultFlowState with no error
// — first launch is normal, not a failure. ConsumeNextScreen removes
// the marker after reading so deep-links fire exactly once.
package jsonflowstate
