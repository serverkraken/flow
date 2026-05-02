package jsonpalettestats_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/jsonpalettestats"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.PaletteStatsStore = (*jsonpalettestats.Store)(nil)

func TestLoad_MissingFile(t *testing.T) {
	s := jsonpalettestats.New(filepath.Join(t.TempDir(), "missing.json"))
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Actions == nil {
		t.Error("Actions should be a non-nil empty map for ergonomics")
	}
	if len(got.Actions) != 0 {
		t.Errorf("want empty, got %v", got.Actions)
	}
}

func TestLoad_LegacyDirectMap(t *testing.T) {
	// Legacy writer: entryKey(e) = section + "\x00" + label, json.Encode
	// of the bare map. Reconstruct via json.Marshal so the bytes mirror
	// a real stats file (NUL bytes serialised as the JSON  escape).
	path := filepath.Join(t.TempDir(), "stats.json")
	keyToggle := "Sidekick" + string(rune(0)) + "Toggle Claude"
	keyStart := "Worktime" + string(rune(0)) + "Start"
	legacy := map[string]domain.PaletteActionStat{
		keyToggle: {Count: 5, LastUsed: time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)},
		keyStart:  {Count: 12, Pinned: true},
	}
	body, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := jsonpalettestats.New(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Actions) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got.Actions))
	}
	if a := got.Actions[keyToggle]; a.Count != 5 {
		t.Errorf("Toggle Claude: got %+v", a)
	}
	if a := got.Actions[keyStart]; !a.Pinned || a.Count != 12 {
		t.Errorf("Start: got %+v", a)
	}
}

func TestLoad_NullJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	if err := os.WriteFile(path, []byte("null\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := jsonpalettestats.New(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Actions == nil {
		t.Error("Actions should not be nil after loading 'null'")
	}
}

func TestLoad_MalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	if err := os.WriteFile(path, []byte("{garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := jsonpalettestats.New(path).Load()
	if err == nil {
		t.Fatal("want parse error, got nil")
	}
}

func TestSave_LoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subdir", "stats.json")
	store := jsonpalettestats.New(path)

	now := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)
	keyToggle := "Sidekick" + string(rune(0)) + "Toggle Claude"
	keyStart := "Worktime" + string(rune(0)) + "Start"
	want := domain.PaletteStats{
		Actions: map[string]domain.PaletteActionStat{
			keyToggle: {Count: 3, LastUsed: now, Pinned: true},
			keyStart:  {Count: 1, LastUsed: now},
		},
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Actions) != len(want.Actions) {
		t.Fatalf("len: got %d, want %d", len(got.Actions), len(want.Actions))
	}
	for k, w := range want.Actions {
		g := got.Actions[k]
		if g.Count != w.Count || g.Pinned != w.Pinned || !g.LastUsed.Equal(w.LastUsed) {
			t.Errorf("key %q: got %+v, want %+v", k, g, w)
		}
	}
}

func TestSave_NilActionsMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	store := jsonpalettestats.New(path)
	if err := store.Save(domain.PaletteStats{Actions: nil}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]domain.PaletteActionStat
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Errorf("written file isn't a JSON object: %v", err)
	}
}

func TestSave_PreservesLegacyShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	store := jsonpalettestats.New(path)
	stats := domain.PaletteStats{
		Actions: map[string]domain.PaletteActionStat{
			"Misc" + string(rune(0)) + "Hello": {Count: 1},
		},
	}
	if err := store.Save(stats); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	// On-disk JSON must be the bare map — NOT wrapped in {"actions": …}.
	var asWrapper struct {
		Actions map[string]domain.PaletteActionStat `json:"actions"`
	}
	if err := json.Unmarshal(raw, &asWrapper); err == nil && asWrapper.Actions != nil {
		t.Errorf("file accidentally wrapped in {actions: …}: %s", string(raw))
	}
}

func TestSave_MkdirError(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	store := jsonpalettestats.New(filepath.Join(regular, "subdir", "stats.json"))
	err := store.Save(domain.PaletteStats{Actions: map[string]domain.PaletteActionStat{"x": {}}})
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestLoad_OpenError(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := jsonpalettestats.New(filepath.Join(regular, "child")).Load()
	if err == nil {
		t.Fatal("want error, got nil")
	}
}
