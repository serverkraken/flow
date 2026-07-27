package tokenstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

func TestFileStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "token.json")
	s := newFileStore(path)

	if _, ok, err := s.Load(); err != nil || ok {
		t.Fatalf("empty load: ok=%v err=%v", ok, err)
	}

	want := ports.Token{AccessToken: "a", RefreshToken: "r", Expiry: time.Unix(1000, 0).UTC()}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perm = %o, want 600", perm)
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
