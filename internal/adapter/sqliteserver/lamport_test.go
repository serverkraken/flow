package sqliteserver

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestUnit_NextLamport_Monotonic(t *testing.T) {
	t.Parallel()
	s := mustOpenServer(t)

	prev := int64(0)
	for i := 0; i < 10; i++ {
		tx, _ := s.DB().Begin()
		v, err := NextLamport(tx)
		if err != nil {
			t.Fatalf("NextLamport: %v", err)
		}
		_ = tx.Commit()
		if v <= prev {
			t.Errorf("v = %d, want > %d", v, prev)
		}
		prev = v
	}
}

func TestUnit_NextLamport_ConcurrentCallers_NoSkip(t *testing.T) {
	t.Parallel()
	s := mustOpenServer(t)
	var wg sync.WaitGroup
	got := make(chan int64, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, _ := s.DB().Begin()
			v, _ := NextLamport(tx)
			_ = tx.Commit()
			got <- v
		}()
	}
	wg.Wait()
	close(got)
	seen := map[int64]bool{}
	for v := range got {
		if seen[v] {
			t.Errorf("duplicate lamport %d", v)
		}
		seen[v] = true
	}
}

func mustOpenServer(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "server.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
