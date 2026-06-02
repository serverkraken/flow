package flockstate_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/flockstate"
	"github.com/serverkraken/flow/internal/ports"
)

// Compile-time checks.
var (
	_ ports.LegacyActiveStore = (*flockstate.State)(nil)
	_ ports.Lock              = (*flockstate.Lock)(nil)
)

func newState(t *testing.T) (*flockstate.State, string) {
	t.Helper()
	dir := t.TempDir()
	active := filepath.Join(dir, "worktime.state")
	pause := filepath.Join(dir, "worktime.pause")
	return flockstate.NewState(active, pause), dir
}

func TestState_GetActive_MissingReturnsNil(t *testing.T) {
	s, _ := newState(t)
	got, err := s.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if got != nil {
		t.Errorf("want nil, got %v", got)
	}
}

func TestState_SetActive_RoundTrip(t *testing.T) {
	s, _ := newState(t)
	want := time.Unix(1714500000, 0)

	if err := s.SetActive(want); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	got, err := s.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if got == nil || !got.Equal(want) {
		t.Errorf("round-trip: got %v, want %v", got, want)
	}
}

func TestState_ClearActive_Idempotent(t *testing.T) {
	s, _ := newState(t)

	// On a missing file, ClearActive is a no-op.
	if err := s.ClearActive(); err != nil {
		t.Errorf("ClearActive on missing file: %v", err)
	}

	// After Set + Clear, the marker is gone.
	if err := s.SetActive(time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearActive(); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetActive()
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("want nil after clear, got %v", got)
	}
}

func TestState_PauseRoundTrip(t *testing.T) {
	s, _ := newState(t)
	want := time.Unix(1714500123, 0)

	if got, err := s.GetPause(); err != nil || got != nil {
		t.Fatalf("GetPause initial: got=%v err=%v", got, err)
	}

	if err := s.SetPause(want); err != nil {
		t.Fatalf("SetPause: %v", err)
	}
	got, err := s.GetPause()
	if err != nil || got == nil || !got.Equal(want) {
		t.Errorf("round-trip: got=%v err=%v want=%v", got, err, want)
	}

	if err := s.ClearPause(); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetPause(); err != nil || got != nil {
		t.Errorf("after clear: got=%v err=%v", got, err)
	}
}

func TestState_GetActive_GarbageContent(t *testing.T) {
	s, dir := newState(t)
	if err := os.WriteFile(filepath.Join(dir, "worktime.state"), []byte("not-a-number"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := s.GetActive()
	if err == nil {
		t.Fatal("want parse error, got nil")
	}
}

func TestState_GetActive_TolerantOfWhitespace(t *testing.T) {
	s, dir := newState(t)
	if err := os.WriteFile(filepath.Join(dir, "worktime.state"), []byte("  1714500000  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetActive()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Unix() != 1714500000 {
		t.Errorf("want epoch 1714500000, got %v", got)
	}
}

// pathUnderRegularFile constructs a path whose parent directory is actually
// a regular file. Operations that need to create or open under that path
// fail with ENOTDIR.
func pathUnderRegularFile(t *testing.T, leaf string) string {
	t.Helper()
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(regular, leaf)
}

func TestState_SetActive_MkdirError(t *testing.T) {
	s := flockstate.NewState(
		pathUnderRegularFile(t, "subdir/state"),
		filepath.Join(t.TempDir(), "pause"),
	)
	if err := s.SetActive(time.Unix(1, 0)); err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestState_GetActive_OpenError(t *testing.T) {
	s := flockstate.NewState(
		pathUnderRegularFile(t, "child"),
		filepath.Join(t.TempDir(), "pause"),
	)
	_, err := s.GetActive()
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("want non-NotExist error, got %v", err)
	}
}

func TestState_ClearActive_PropagatesNonNotExist(t *testing.T) {
	// Pointing at a path that is itself a directory makes os.Remove return
	// EISDIR (or EPERM on macOS) — neither is ErrNotExist, so the adapter
	// must propagate.
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state-as-dir")
	if err := os.Mkdir(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Make the directory non-empty so os.Remove fails (rmdir refuses
	// non-empty directories cross-platform).
	if err := os.WriteFile(filepath.Join(stateDir, "blocker"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	s := flockstate.NewState(stateDir, filepath.Join(dir, "pause"))
	if err := s.ClearActive(); err == nil {
		t.Fatal("want error, got nil")
	}
}
