package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/serverkraken/flow/internal/state"
)

func TestDefault_IsPalette(t *testing.T) {
	t.Parallel()
	d := state.Default()
	if d.Screen != state.Palette {
		t.Errorf("Default screen = %q, want %q", d.Screen, state.Palette)
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	want := state.State{Screen: state.Worktime, Filter: "foo", Cursor: 3}
	if err := state.Save(want); err != nil {
		t.Fatal(err)
	}
	got := state.Load()
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoad_MissingFile_ReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	got := state.Load()
	if got.Screen != state.Palette {
		t.Errorf("Load() on missing file = %q, want %q", got.Screen, state.Palette)
	}
}

func TestCheckNextScreen_ReturnsAndDeletes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := state.WriteNextScreen(state.Projects); err != nil {
		t.Fatal(err)
	}
	got := state.CheckNextScreen()
	if got != state.Projects {
		t.Errorf("CheckNextScreen() = %q, want %q", got, state.Projects)
	}
	// Second call must return "" (file deleted).
	if second := state.CheckNextScreen(); second != "" {
		t.Errorf("second CheckNextScreen() = %q, want \"\"", second)
	}
}

func TestCheckNextScreen_MissingFile_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if got := state.CheckNextScreen(); got != "" {
		t.Errorf("CheckNextScreen() on missing file = %q, want \"\"", got)
	}
}

func TestWriteNextScreen_UnknownScreen_ReturnsError(t *testing.T) {
	t.Parallel()
	if err := state.WriteNextScreen("unknown"); err == nil {
		t.Error("WriteNextScreen(unknown) expected error, got nil")
	}
}

func TestLoad_InvalidScreenName_FallsBackToPalette(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cacheDir := filepath.Join(dir, ".cache", "flow")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "state.json"),
		[]byte(`{"screen":"invalid"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := state.Load()
	if got.Screen != state.Palette {
		t.Errorf("Load with invalid screen = %q, want %q", got.Screen, state.Palette)
	}
}
