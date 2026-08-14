// Package statuscache is the client-side tmux status snapshot cache: one atomic
// JSON file with a fetch timestamp so the 5s tick can render without a server
// round-trip, and fall back to the last snapshot (dimmed) when offline.
package statuscache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"golang.org/x/sys/unix"
)

const (
	TTL    = 30 * time.Second // fresher than this → render without fetching
	MaxAge = 30 * time.Minute // older than this → segment goes empty (Spec §2)
)

type Entry struct {
	FetchedAt time.Time                `json:"fetchedAt"`
	Status    apiclient.WorktimeStatus `json:"status"`
}

func (e Entry) Fresh(now time.Time) bool   { return now.Sub(e.FetchedAt) < TTL }
func (e Entry) Expired(now time.Time) bool { return now.Sub(e.FetchedAt) > MaxAge }

// Read returns the cached entry; ok=false on a missing OR corrupt file (corrupt
// is treated as "no cache" so a bad write never wedges the segment).
func Read(path string) (Entry, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, false
	}
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil {
		return Entry{}, false
	}
	return e, true
}

// Write atomically persists e (tmp in the same dir + rename); concurrent ticks
// are last-writer-wins, no lock needed.
func Write(path string, e Entry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".worktime-status-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// TryWithRefreshLock runs fn only when this process can immediately acquire
// the cache's cross-process refresh lock. A busy lock is a normal no-op: tmux
// may start several render processes while one detached worker is fetching.
func TryWithRefreshLock(cachePath string, fn func() error) (bool, error) {
	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, fmt.Errorf("statuscache: create lock directory: %w", err)
	}
	lockPath := cachePath + ".refresh.lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false, fmt.Errorf("statuscache: open refresh lock: %w", err)
	}
	defer func() { _ = f.Close() }()
	if err := f.Chmod(0o600); err != nil {
		return false, fmt.Errorf("statuscache: secure refresh lock: %w", err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return false, nil
		}
		return false, fmt.Errorf("statuscache: acquire refresh lock: %w", err)
	}

	callbackErr := fn()
	unlockErr := unix.Flock(int(f.Fd()), unix.LOCK_UN)
	if callbackErr != nil {
		return true, callbackErr
	}
	if unlockErr != nil {
		return true, fmt.Errorf("statuscache: release refresh lock: %w", unlockErr)
	}
	return true, nil
}
