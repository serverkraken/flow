package flockstate

import (
	"os"
	"path/filepath"
	"syscall"
)

// Lock implements ports.Lock via syscall.Flock(LOCK_EX) on a configurable
// path. The lock is per open file description, so the second goroutine
// (or process) calling With on the same path blocks until the first
// returns.
type Lock struct {
	path string
}

// NewLock constructs a Lock that flocks on path. The lockfile and its
// parent are created on demand.
func NewLock(path string) *Lock {
	return &Lock{path: path}
}

// With acquires an exclusive lock, runs fn, and releases the lock.
// Lock acquisition errors short-circuit fn; fn's error is returned
// unmodified.
func (l *Lock) With(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
	return fn()
}
