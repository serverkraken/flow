package tokenstore

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

type noopSession struct{}

func (noopSession) Save(ports.Token) error           { return nil }
func (noopSession) Load() (ports.Token, bool, error) { return ports.Token{}, false, nil }
func (noopSession) Clear() error                     { return nil }

func TestStoresWithSameLockPathSerialize(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "flow", "token.lock")
	first := newLockedStore(lockPath, noopSession{})
	second := newLockedStore(lockPath, noopSession{})
	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- first.WithLock(context.Background(), func(ports.TokenStoreSession) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	var entered atomic.Bool
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := second.WithLock(ctx, func(ports.TokenStoreSession) error {
		entered.Store(true)
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contended lock error = %v, want context deadline", err)
	}
	if entered.Load() {
		t.Fatal("second store entered while first store held the lock")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first lock: %v", err)
	}
	if err := second.WithLock(context.Background(), func(ports.TokenStoreSession) error { return nil }); err != nil {
		t.Fatalf("lock after release: %v", err)
	}
}

func TestLockAcquisitionHonorsCancellation(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "flow", "token.lock")
	first := newLockedStore(lockPath, noopSession{})
	second := newLockedStore(lockPath, noopSession{})
	held := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = first.WithLock(context.Background(), func(ports.TokenStoreSession) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := second.WithLock(ctx, func(ports.TokenStoreSession) error {
		t.Fatal("canceled waiter entered the lock")
		return nil
	})
	close(release)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestLockReleasedAfterHolderProcessTerminates(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "flow", "token.lock")
	cmd := exec.Command(os.Args[0], "-test.run=TestLockHelperProcess")
	cmd.Env = append(os.Environ(), "FLOW_LOCK_HELPER=1", "FLOW_LOCK_PATH="+lockPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "locked\n" {
		_ = cmd.Process.Kill()
		t.Fatalf("helper readiness = %q, err=%v", line, err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	store := newLockedStore(lockPath, noopSession{})
	if err := store.WithLock(ctx, func(ports.TokenStoreSession) error { return nil }); err != nil {
		t.Fatalf("lock was not recovered after process exit: %v", err)
	}
}

func TestLockHelperProcess(t *testing.T) {
	if os.Getenv("FLOW_LOCK_HELPER") != "1" {
		return
	}
	store := newLockedStore(os.Getenv("FLOW_LOCK_PATH"), noopSession{})
	err := store.WithLock(context.Background(), func(ports.TokenStoreSession) error {
		fmt.Println("locked")
		select {}
	})
	if err != nil {
		os.Exit(2)
	}
}

func TestLockAndDirectoryPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "flow")
	lockPath := filepath.Join(dir, "token.lock")
	store := newLockedStore(lockPath, noopSession{})
	if err := store.WithLock(context.Background(), func(ports.TokenStoreSession) error { return nil }); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{dir: 0o700, lockPath: 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("%s mode = %o, want %o", path, got, want)
		}
	}
}

func TestDifferentLockPathsDoNotBlockEachOther(t *testing.T) {
	root := t.TempDir()
	first := newLockedStore(filepath.Join(root, "a", "token.lock"), noopSession{})
	second := newLockedStore(filepath.Join(root, "b", "token.lock"), noopSession{})
	held := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = first.WithLock(context.Background(), func(ports.TokenStoreSession) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	if err := second.WithLock(context.Background(), func(ports.TokenStoreSession) error { return nil }); err != nil {
		t.Fatal(err)
	}
	close(release)
}
