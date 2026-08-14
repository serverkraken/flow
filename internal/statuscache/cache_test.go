package statuscache_test

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/statuscache"
)

func TestEntry_FreshnessBoundaries(t *testing.T) {
	base := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	e := statuscache.Entry{FetchedAt: base}
	if !e.Fresh(base.Add(29 * time.Second)) {
		t.Error("29s should be fresh")
	}
	if e.Fresh(base.Add(31 * time.Second)) {
		t.Error("31s should be stale")
	}
	if e.Expired(base.Add(29 * time.Minute)) {
		t.Error("29min should not be expired")
	}
	if !e.Expired(base.Add(31 * time.Minute)) {
		t.Error("31min should be expired")
	}
}

func TestWriteRead_RoundtripAndCorrupt(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "worktime-status.json")
	e := statuscache.Entry{FetchedAt: time.Now().UTC().Truncate(time.Second), Status: apiclient.WorktimeStatus{LoggedMin: 42}}
	if err := statuscache.Write(p, e); err != nil {
		t.Fatal(err)
	}
	got, ok := statuscache.Read(p)
	if !ok || got.Status.LoggedMin != 42 {
		t.Fatalf("roundtrip failed: ok=%v %+v", ok, got)
	}
	_ = os.WriteFile(p, []byte("{not json"), 0o644)
	if _, ok := statuscache.Read(p); ok {
		t.Error("corrupt cache must read as absent")
	}
}

func TestRead_MissingFileIsAbsent(t *testing.T) {
	if _, ok := statuscache.Read(filepath.Join(t.TempDir(), "nope.json")); ok {
		t.Error("missing file must read as absent")
	}
}

// Finding C8: the write is atomic (tmp-in-same-dir + rename) and leaves no
// leftover temp files; a second write overwrites cleanly.
func TestWrite_AtomicNoLeftoverTmp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "worktime-status.json")
	for i := 0; i < 3; i++ {
		if err := statuscache.Write(p, statuscache.Entry{FetchedAt: time.Now(), Status: apiclient.WorktimeStatus{LoggedMin: i}}); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "worktime-status.json" {
			t.Errorf("leftover file in cache dir: %q (temp not renamed/cleaned)", e.Name())
		}
	}
	if got, ok := statuscache.Read(p); !ok || got.Status.LoggedMin != 2 {
		t.Errorf("last write must win, got ok=%v %+v", ok, got)
	}
}

func TestTryWithRefreshLock_DeduplicatesWorkers(t *testing.T) {
	p := filepath.Join(t.TempDir(), "worktime-status.json")
	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := statuscache.TryWithRefreshLock(p, func() error {
			close(held)
			<-release
			return nil
		})
		done <- err
	}()
	<-held

	var entered atomic.Bool
	acquired, err := statuscache.TryWithRefreshLock(p, func() error {
		entered.Store(true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if acquired || entered.Load() {
		t.Fatal("contending worker must return immediately without entering")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
