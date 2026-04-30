package palette_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/serverkraken/flow/internal/screen/palette"
)

func writeMenuEntries(t *testing.T, pluginsDir, plugin, content string) {
	t.Helper()
	dir := filepath.Join(pluginsDir, plugin)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "menu.entries"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseEntriesFile_BasicParsing(t *testing.T) {
	dir := t.TempDir()
	pluginsDir := filepath.Join(dir, ".tmux", "plugins")
	t.Setenv("HOME", dir)
	t.Setenv("ENABLED_FILE", filepath.Join(dir, "does-not-exist")) // use fallback scan

	writeMenuEntries(t, pluginsDir, "myplugin",
		"🤖\tToggle Claude\trun-shell '~/.tmux/plugins/sidekick/sidekick.sh toggle claude'\tSidekick\n"+
			"📝\tNew Note\trun-shell 'kompendium new'\tKompendium\n",
	)

	// We can't call LoadEntries() here without mocking the FS layout,
	// so test parseEntriesFile via the exported surface (LoadEntries with temp home).
	entries, _, err := palette.LoadEntries()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	// Sidekick comes before Kompendium in section priority.
	if entries[0].Section != "Sidekick" {
		t.Errorf("entries[0].Section = %q, want %q", entries[0].Section, "Sidekick")
	}
	if entries[1].Section != "Kompendium" {
		t.Errorf("entries[1].Section = %q, want %q", entries[1].Section, "Kompendium")
	}
}

func TestParseEntriesFile_SkipsCommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	pluginsDir := filepath.Join(dir, ".tmux", "plugins")
	t.Setenv("HOME", dir)
	t.Setenv("ENABLED_FILE", filepath.Join(dir, "does-not-exist"))

	writeMenuEntries(t, pluginsDir, "myplugin",
		"# this is a comment\n\n🤖\tClaude\trun-shell 'foo'\tSidekick\n# another\n",
	)

	entries, _, err := palette.LoadEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1", len(entries))
	}
}

func TestParseEntriesFile_MissingSectionDefaultsMisc(t *testing.T) {
	dir := t.TempDir()
	pluginsDir := filepath.Join(dir, ".tmux", "plugins")
	t.Setenv("HOME", dir)
	t.Setenv("ENABLED_FILE", filepath.Join(dir, "does-not-exist"))

	writeMenuEntries(t, pluginsDir, "myplugin", "🔄\tReload\trun-shell 'tmux source'\n")

	entries, _, err := palette.LoadEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatal("expected 1 entry")
	}
	if entries[0].Section != "Misc" {
		t.Errorf("Section = %q, want %q", entries[0].Section, "Misc")
	}
}

func TestLoadEntries_SectionOrder(t *testing.T) {
	dir := t.TempDir()
	pluginsDir := filepath.Join(dir, ".tmux", "plugins")
	t.Setenv("HOME", dir)
	t.Setenv("ENABLED_FILE", filepath.Join(dir, "does-not-exist"))

	writeMenuEntries(t, pluginsDir, "plugin1",
		"🗂\tMisc Action\trun-shell 'x'\tMisc\n"+
			"📓\tNotes Action\trun-shell 'y'\tKompendium\n"+
			"🤖\tSidekick Action\trun-shell 'z'\tSidekick\n",
	)

	entries, _, err := palette.LoadEntries()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Sidekick", "Kompendium", "Misc"}
	for i, w := range want {
		if entries[i].Section != w {
			t.Errorf("entries[%d].Section = %q, want %q", i, entries[i].Section, w)
		}
	}
}
