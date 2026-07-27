package tokenstore

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/zalando/go-keyring"
)

func TestKeyringStoreRoundTrip(t *testing.T) {
	keyring.MockInit() // in-memory keyring for tests
	s := newKeyringStore()

	if _, ok, err := s.Load(); err != nil || ok {
		t.Fatalf("empty load: ok=%v err=%v", ok, err)
	}

	want := ports.Token{AccessToken: "acc", RefreshToken: "ref", Expiry: time.Unix(2000, 0).UTC()}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Load()
	if err != nil || !ok {
		t.Fatalf("load after save: ok=%v err=%v", ok, err)
	}
	if got != want {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, want)
	}

	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Load(); ok {
		t.Fatal("token present after Clear")
	}
}
