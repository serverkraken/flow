package jsonflowstate_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/jsonflowstate"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.FlowStateStore = (*jsonflowstate.Store)(nil)

func newStore(t *testing.T) (*jsonflowstate.Store, string) {
	t.Helper()
	dir := t.TempDir()
	state := filepath.Join(dir, "state.json")
	next := filepath.Join(dir, "next-screen")
	return jsonflowstate.New(state, next), dir
}

func TestLoad_MissingFile_ReturnsDefault(t *testing.T) {
	s, _ := newStore(t)
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	want := domain.DefaultFlowState()
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLoad_MalformedFile_ReturnsDefault(t *testing.T) {
	s, dir := newStore(t)
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Errorf("malformed → want nil error, got %v", err)
	}
	if got != domain.DefaultFlowState() {
		t.Errorf("malformed → want default, got %+v", got)
	}
}

func TestLoad_InvalidScreen_FallsBackToPalette(t *testing.T) {
	s, dir := newStore(t)
	if err := os.WriteFile(filepath.Join(dir, "state.json"),
		[]byte(`{"screen":"unknown","filter":"foo","cursor":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Screen != domain.ScreenPalette {
		t.Errorf("screen: got %q, want palette", got.Screen)
	}
	// Filter and cursor preserved — only the screen falls back.
	if got.Filter != "foo" || got.Cursor != 3 {
		t.Errorf("filter/cursor lost: %+v", got)
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	s, _ := newStore(t)
	want := domain.FlowState{Screen: domain.ScreenWorktime, Filter: "x", Cursor: 7}
	if err := s.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSave_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "subdir", "state.json")
	next := filepath.Join(dir, "subdir", "next-screen")
	s := jsonflowstate.New(state, next)

	if err := s.Save(domain.FlowState{Screen: domain.ScreenPalette}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(state); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestSave_MkdirError(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	s := jsonflowstate.New(filepath.Join(regular, "subdir", "state.json"), filepath.Join(dir, "next"))
	err := s.Save(domain.DefaultFlowState())
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestConsumeNextScreen_Missing(t *testing.T) {
	s, _ := newStore(t)
	got, err := s.ConsumeNextScreen()
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("want empty, got %q", got)
	}
}

func TestConsumeNextScreen_ConsumesAndRemoves(t *testing.T) {
	s, dir := newStore(t)
	path := filepath.Join(dir, "next-screen")
	if err := os.WriteFile(path, []byte(domain.ScreenWorktime+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := s.ConsumeNextScreen()
	if err != nil {
		t.Fatal(err)
	}
	if got != domain.ScreenWorktime {
		t.Errorf("got %q, want %q", got, domain.ScreenWorktime)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file should be removed after consume, stat err = %v", err)
	}
}

func TestConsumeNextScreen_InvalidScreen(t *testing.T) {
	s, dir := newStore(t)
	if err := os.WriteFile(filepath.Join(dir, "next-screen"), []byte("bogus\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := s.ConsumeNextScreen()
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("invalid screen: want empty, got %q", got)
	}
	// Marker is still consumed (removed) so it can't fire again with bogus content.
	if _, err := os.Stat(filepath.Join(dir, "next-screen")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("marker should be removed even when invalid: %v", err)
	}
}

func TestConsumeNextScreen_OpenError(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	s := jsonflowstate.New(filepath.Join(dir, "state.json"), filepath.Join(regular, "child"))
	_, err := s.ConsumeNextScreen()
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("want non-NotExist error, got %v", err)
	}
}

func TestWriteNextScreen_Valid(t *testing.T) {
	s, dir := newStore(t)
	if err := s.WriteNextScreen(domain.ScreenWorktime); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "next-screen"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != domain.ScreenWorktime {
		t.Errorf("got %q, want %q", string(raw), domain.ScreenWorktime)
	}
}

func TestWriteNextScreen_RejectsUnknown(t *testing.T) {
	s, _ := newStore(t)
	if err := s.WriteNextScreen("garbage"); err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestWriteNextScreen_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	s := jsonflowstate.New(
		filepath.Join(dir, "state.json"),
		filepath.Join(dir, "subdir", "next-screen"),
	)
	if err := s.WriteNextScreen(domain.ScreenPalette); err != nil {
		t.Fatal(err)
	}
}

func TestWriteNextScreen_MkdirError(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	s := jsonflowstate.New(
		filepath.Join(dir, "state.json"),
		filepath.Join(regular, "subdir", "next-screen"),
	)
	if err := s.WriteNextScreen(domain.ScreenPalette); err == nil {
		t.Fatal("want error, got nil")
	}
}
