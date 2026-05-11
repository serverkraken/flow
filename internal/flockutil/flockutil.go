//go:build !windows

// Package flockutil holds the canonical advisory-lock-with-timeout
// primitive shared by every cross-process writer in flow:
//   - flockstate.Lock (the central worktime mutex)
//   - linkstsv.Store (note attachments)
//   - dayoffstsv.Store (day-off entries)
//
// Originally each adapter rolled its own variant. The variants diverged:
// flockstate retried with backoff until a 5s ceiling, the two TSV stores
// called LOCK_EX without LOCK_NB and would block indefinitely if another
// process held the lock (review finding M3). Centralising means one
// definition of "how long should we wait" and one place to tune it.
//
// POSIX-only (syscall.Flock / EWOULDBLOCK). The cross-build matrix is
// linux + darwin today; Windows builds skip this file via the build tag.
package flockutil

import (
	"errors"
	"fmt"
	"syscall"
	"time"
)

// LockTimeout is the default ceiling for lock acquisition. Without a
// ceiling a stuck other process (crashed flow that didn't release, an
// unresponsive mounted volume) wedges every UI keystroke indefinitely
// with no diagnostic; with the ceiling we fail fast and the caller
// surfaces an error.
const LockTimeout = 5 * time.Second

// ErrLockTimeout is returned by Acquire when the lock could not be
// acquired within the supplied timeout. Callers can errors.Is against
// it to surface a "another flow is busy" hint instead of a raw error.
var ErrLockTimeout = errors.New("flow lock acquisition timed out")

// Acquire tries LOCK_EX|LOCK_NB on fd and retries with an exponential
// backoff (capped at 200 ms) until timeout elapses. EWOULDBLOCK is the
// signal "someone else holds it"; any other Flock error short-circuits
// with the syscall error unwrapped.
//
// The caller owns fd and is responsible for LOCK_UN on success.
func Acquire(fd int, timeout time.Duration) error {
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
