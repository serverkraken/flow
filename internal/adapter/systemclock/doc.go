// Package systemclock implements ports.Clock by delegating to
// time.Now. Tests should use testutil.FixedClock instead so behaviour
// under test stays deterministic.
package systemclock
