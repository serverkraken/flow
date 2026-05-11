//go:build !windows

package flockstate

// White-box test for ErrLockTimeout: needs to reach the unexported
// `timeout` field so the test doesn't have to wait the production
// 5-second ceiling. Review finding T1 — the timeout path was never
// exercised before this test was added.

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestLock_With_TimeoutSurfacesErrLockTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	holder := &Lock{path: path, timeout: 5 * time.Second}

	holdEntered := make(chan struct{})
	holdRelease := make(chan struct{})
	var holderDone sync.WaitGroup
	holderDone.Add(1)
	go func() {
		defer holderDone.Done()
		_ = holder.With(func() error {
			close(holdEntered)
			<-holdRelease
			return nil
		})
	}()
	<-holdEntered

	// Probe lock has a sub-second timeout so the test runs fast. Pre-fix
	// no test exercised this codepath at all, leaving the
	// "worktime lock acquisition timed out" surface entirely dark.
	probe := &Lock{path: path, timeout: 100 * time.Millisecond}
	start := time.Now()
	err := probe.With(func() error { return nil })
	dur := time.Since(start)

	close(holdRelease)
	holderDone.Wait()

	if err == nil {
		t.Fatal("probe With must fail while holder holds the lock")
	}
	if !errors.Is(err, ErrLockTimeout) {
		t.Errorf("got %v, want wrap of ErrLockTimeout", err)
	}
	// Sanity: the wait must have at least approached the configured
	// timeout (otherwise the timeout signal didn't actually fire). A
	// generous 50 ms floor accommodates CI scheduler jitter.
	if dur < 50*time.Millisecond {
		t.Errorf("returned too fast (%v) — the retry loop didn't engage", dur)
	}
}
