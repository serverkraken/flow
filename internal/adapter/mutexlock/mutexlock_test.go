package mutexlock_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/mutexlock"
)

func TestWith_SerializesConcurrentCalls(t *testing.T) {
	l := mutexlock.New()
	var mu sync.Mutex
	inside, maxInside := 0, 0
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = l.With(func() error {
				mu.Lock()
				inside++
				if inside > maxInside {
					maxInside = inside
				}
				mu.Unlock()
				mu.Lock()
				inside--
				mu.Unlock()
				return nil
			})
		}()
	}
	wg.Wait()
	if maxInside != 1 {
		t.Fatalf("expected max 1 goroutine inside critical section, got %d", maxInside)
	}
}

func TestWith_PropagatesError(t *testing.T) {
	l := mutexlock.New()
	sentinel := errors.New("sentinel error")
	err := l.With(func() error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}
