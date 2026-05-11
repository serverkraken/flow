//go:build !windows

// Package flockstate's lock primitive uses syscall.Flock and the
// syscall.LOCK_* / EWOULDBLOCK constants — POSIX-only. Building this
// file under windows fails because those symbols are not declared in
// the windows syscall package. Today the cross-build matrix is
// linux/amd64 + darwin/{amd64,arm64} (CLAUDE.md), all POSIX, so the
// constraint is documentation-as-code rather than a current blocker.
// Review finding T9.

package flockstate

import (
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/serverkraken/flow/internal/flockutil"
)

// Lock implements ports.Lock via syscall.Flock(LOCK_EX) on a configurable
// path. The lock is per open file description, so the second goroutine
// (or process) calling With on the same path blocks until the first
// returns.
type Lock struct {
	path    string
	timeout time.Duration
}

// LockTimeout is the default ceiling for lock acquisition. Re-exported
// from flockutil so existing flockstate.LockTimeout callers still work;
// the canonical definition lives there for sharing with linkstsv /
// dayoffstsv (review finding M3).
const LockTimeout = flockutil.LockTimeout

// ErrLockTimeout is returned by With when the lock could not be
// acquired within LockTimeout. Aliased to the shared sentinel so callers
// can errors.Is(err, flockstate.ErrLockTimeout) regardless of which
// adapter raised it.
var ErrLockTimeout = flockutil.ErrLockTimeout

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
	if err := flockutil.Acquire(int(f.Fd()), l.timeout); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
	return fn()
}
