package flockstate_test

import (
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/flockstate"
)

func TestLock_With_RunsFn(t *testing.T) {
	l := flockstate.NewLock(filepath.Join(t.TempDir(), "lock"))
	called := false
	if err := l.With(func() error { called = true; return nil }); err != nil {
		t.Fatalf("With: %v", err)
	}
	if !called {
		t.Error("fn was not called")
	}
}

func TestLock_With_PropagatesFnError(t *testing.T) {
	l := flockstate.NewLock(filepath.Join(t.TempDir(), "lock"))
	want := errors.New("boom")
	got := l.With(func() error { return want })
	if !errors.Is(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLock_With_MkdirError(t *testing.T) {
	l := flockstate.NewLock(pathUnderRegularFile(t, "subdir/lock"))
	if err := l.With(func() error { return nil }); err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestLock_With_Serialises(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	l1 := flockstate.NewLock(path)
	l2 := flockstate.NewLock(path)

	holdEntered := make(chan struct{})
	holdRelease := make(chan struct{})
	holdDone := make(chan error, 1)
	go func() {
		holdDone <- l1.With(func() error {
			close(holdEntered)
			<-holdRelease
			return nil
		})
	}()
	<-holdEntered

	var secondEntered atomic.Bool
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- l2.With(func() error {
			secondEntered.Store(true)
			return nil
		})
	}()

	// While holdDone is still pending, secondEntered must not have flipped.
	time.Sleep(80 * time.Millisecond)
	if secondEntered.Load() {
		t.Fatal("second With entered while first held the lock")
	}

	close(holdRelease)
	if err := <-holdDone; err != nil {
		t.Fatalf("first With: %v", err)
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second With: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second With never finished after first released")
	}
	if !secondEntered.Load() {
		t.Error("second With did not run fn")
	}
}
