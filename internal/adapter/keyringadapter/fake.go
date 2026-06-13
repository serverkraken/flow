package keyringadapter

import (
	"sync"

	"github.com/serverkraken/flow/internal/ports"
)

// Fake is an in-memory TokenStore for tests. Goroutine-safe.
type Fake struct {
	mu   sync.Mutex
	data map[string]ports.Tokens
}

// NewFake creates an empty in-memory TokenStore suitable for tests.
func NewFake() *Fake { return &Fake{data: make(map[string]ports.Tokens)} }

// Get returns the tokens stored under slot, or ports.ErrTokenNotFound.
func (f *Fake) Get(slot string) (ports.Tokens, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.data[slot]
	if !ok {
		return ports.Tokens{}, ports.ErrTokenNotFound
	}
	return t, nil
}

// Put stores t under slot, overwriting any existing value.
func (f *Fake) Put(slot string, t ports.Tokens) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[slot] = t
	return nil
}

// Delete removes the tokens stored under slot (no-op if absent).
func (f *Fake) Delete(slot string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, slot)
	return nil
}

// Compile-time assertion.
var _ ports.TokenStore = (*Fake)(nil)
