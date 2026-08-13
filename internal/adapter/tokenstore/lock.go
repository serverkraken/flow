package tokenstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/serverkraken/flow/internal/ports"
	"golang.org/x/sys/unix"
)

const lockRetryInterval = 20 * time.Millisecond

type lockedStore struct {
	lockPath string
	session  ports.TokenStoreSession
}

func newLockedStore(lockPath string, session ports.TokenStoreSession) *lockedStore {
	return &lockedStore{lockPath: lockPath, session: session}
}

func (s *lockedStore) WithLock(ctx context.Context, fn func(ports.TokenStoreSession) error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("tokenstore: acquire lock: %w", err)
	}
	if err := ensurePrivateDir(filepath.Dir(s.lockPath)); err != nil {
		return fmt.Errorf("tokenstore: create lock directory: %w", err)
	}
	f, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("tokenstore: open lock: %w", err)
	}
	defer func() { _ = f.Close() }()
	if err := f.Chmod(0o600); err != nil {
		return fmt.Errorf("tokenstore: secure lock: %w", err)
	}

	ticker := time.NewTicker(lockRetryInterval)
	defer ticker.Stop()
	for {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return fmt.Errorf("tokenstore: acquire lock: %w", err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("tokenstore: acquire lock: %w", ctx.Err())
		case <-ticker.C:
		}
	}

	callbackErr := fn(s.session)
	unlockErr := unix.Flock(int(f.Fd()), unix.LOCK_UN)
	if callbackErr != nil {
		return callbackErr
	}
	if unlockErr != nil {
		return fmt.Errorf("tokenstore: release lock: %w", unlockErr)
	}
	return nil
}

func ensurePrivateDir(dir string) error {
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}
