// Package mutexlock provides a process-local ports.Lock for the
// server-mode SessionWriter: it serialises local read-modify-upsert
// edits within one flow process. Cross-process / cross-device
// consistency is enforced server-side via ETag/If-Match.
package mutexlock

import (
	"sync"

	"github.com/serverkraken/flow/internal/ports"
)

// compile-time assertion that *Lock satisfies ports.Lock.
var _ ports.Lock = (*Lock)(nil)

// Lock is a process-local mutex-backed implementation of ports.Lock.
type Lock struct{ mu sync.Mutex }

// New returns a ready-to-use Lock.
func New() *Lock { return &Lock{} }

// With acquires the mutex, calls fn, releases the mutex, and returns
// whatever fn returned.
func (l *Lock) With(fn func() error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return fn()
}
