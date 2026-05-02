// Package systemclock implements ports.Clock by delegating to
// time.Now. Tests should use testutil.FixedClock instead so behaviour
// under test stays deterministic.
//
// The same Clock value also satisfies the kompendium subtree's
// kompendium/ports.Clock (structurally identical: Now() time.Time), so
// composition wires this single adapter into both hexagons. See
// CLAUDE-kompendium-plan §8 (K0 decision: systemclock dedup).
package systemclock
