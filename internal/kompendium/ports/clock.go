// Package ports declares the interfaces that use cases consume. It depends
// only on the domain layer so it can be implemented by adapters or by
// in-memory fakes from testutil — see CLAUDE.md section 2.1 for the
// dependency rule.
package ports

import "time"

// Clock is the abstraction over wall-clock time. Use cases consume Clock so
// tests can inject a deterministic value via testutil.FixedClock.
//
// Now MUST return a time in the user's local TZ — daily/project notes are
// keyed by the human-perceived calendar date, and an instant near midnight
// rendered in UTC would land in the wrong day for any user east of GMT.
// Adapters wrap time.Now() unchanged.
type Clock interface {
	Now() time.Time
}
