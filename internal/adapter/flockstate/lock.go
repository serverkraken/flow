package flockstate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Lock implements ports.Lock via syscall.Flock(LOCK_EX) on a configurable
// path. The lock is per open file description, so the second goroutine
// (or process) calling With on the same path blocks until the first
// returns.
type Lock struct {
	path    string
	timeout time.Duration
}

// LockTimeout is the default ceiling for lock acquisition. Without a
// ceiling a stuck other process (crashed flow that didn't release, a
// mounted volume gone unresponsive) wedges every UI keystroke
// indefinitely with no diagnostic; with the ceiling we fail fast and
// the caller surfaces a tmux display-message.
const LockTimeout = 5 * time.Second

// ErrLockTimeout is returned by With when the lock could not be
// acquired within LockTimeout. Callers may type-assert via errors.Is
// to surface a "another flow is busy" hint instead of a raw error.
var ErrLockTimeout = errors.New("worktime lock acquisition timed out")

// NewLock constructs a Lock that flocks on path. The lockfile and its
// parent are created on demand. Lock acquisition will retry until
// LockTimeout elapses.
func NewLock(path string) *Lock {
	return &Lock{path: path, timeout: LockTimeout}
}

// With acquires an exclusive lock (subject to the configured timeout),
// runs fn, and releases the lock. Lock acquisition errors short-circuit
// fn; fn's error is returned unmodified.
func (l *Lock) With(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	if err := acquireFlock(int(f.Fd()), l.timeout); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
	return fn()
}

// acquireFlock tries LOCK_EX|LOCK_NB and retries with a small backoff
// until timeout elapses. EWOULDBLOCK is the signal "someone else
// holds it"; any other error short-circuits with the syscall error.
func acquireFlock(fd int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	delay := 20 * time.Millisecond
	for {
		err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w after %s", ErrLockTimeout, timeout)
		}
		time.Sleep(delay)
		if delay < 200*time.Millisecond {
			delay *= 2
		}
	}
}
