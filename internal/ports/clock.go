package ports

import "time"

// Clock provides the current time. Injected so use cases stay testable
// without monkey-patching time.Now.
type Clock interface {
	Now() time.Time
}
