package clientauth

import (
	"errors"
	"testing"
)

func TestErrNotLoggedInIsSentinel(t *testing.T) {
	if ErrNotLoggedIn == nil {
		t.Fatal("ErrNotLoggedIn must be a non-nil sentinel")
	}
	// The sentinel must be matchable through wrapping.
	err := errors.Join(ErrNotLoggedIn, errors.New("context"))
	if !errors.Is(err, ErrNotLoggedIn) {
		t.Error("ErrNotLoggedIn should be matchable via errors.Is through wrapping")
	}
}
