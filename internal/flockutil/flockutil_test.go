//go:build !windows

package flockutil_test

// Direct test of flockutil.Acquire: a process-1 holder forces process-2
// onto the retry path so the timeout actually fires. Pre-extraction the
// retry logic was duplicated inside flockstate; tests in linkstsv /
// dayoffstsv silently inherited the old "no timeout" behaviour. This
// test pins the shared primitive so the three adapters share one
// verified retry-with-backoff implementation.

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/flockutil"
)

func TestAcquire_SucceedsImmediatelyWhenFree(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck
	if err := flockutil.Acquire(int(f.Fd()), 500*time.Millisecond); err != nil {
		t.Errorf("Acquire on free lock = %v, want nil", err)
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

func TestAcquire_TimesOutWhileHeldByOtherFD(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "lock")
	holder, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close() //nolint:errcheck
	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatalf("seed flock: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Flock(int(holder.Fd()), syscall.LOCK_UN) })

	probe, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close() //nolint:errcheck

	start := time.Now()
	err = flockutil.Acquire(int(probe.Fd()), 100*time.Millisecond)
	dur := time.Since(start)

	if err == nil {
		t.Fatal("Acquire must fail while another FD holds the lock")
	}
	if !errors.Is(err, flockutil.ErrLockTimeout) {
		t.Errorf("got %v, want wrap of ErrLockTimeout", err)
	}
	if dur < 50*time.Millisecond {
		t.Errorf("returned too fast (%v) — retry loop didn't engage", dur)
	}
}

func TestAcquire_AcquiresAfterHolderReleases(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "lock")
	holder, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close() //nolint:errcheck
	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	released := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(80 * time.Millisecond)
		_ = syscall.Flock(int(holder.Fd()), syscall.LOCK_UN)
		close(released)
	}()

	probe, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close() //nolint:errcheck

	// 500 ms ceiling — well above the 80 ms hold so the retry loop wins.
	if err := flockutil.Acquire(int(probe.Fd()), 500*time.Millisecond); err != nil {
		t.Errorf("Acquire after holder release = %v, want nil", err)
	}
	<-released
	wg.Wait()
	_ = syscall.Flock(int(probe.Fd()), syscall.LOCK_UN)
}
