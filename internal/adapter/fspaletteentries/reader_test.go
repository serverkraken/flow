package fspaletteentries_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/fspaletteentries"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.PaletteEntryReader = (*fspaletteentries.Reader)(nil)

// scaffold builds a minimal plugins directory and returns its base
// path. Each plugin gets a directory with the supplied menu.entries
// content.
func scaffold(t *testing.T, plugins map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	pluginsDir := filepath.Join(dir, "plugins")
	for name, body := range plugins {
		pdir := filepath.Join(pluginsDir, name)
		if err := os.MkdirAll(pdir, 0o755); err != nil {
			t.Fatal(err)
		}
		if body != "" {
			if err := os.WriteFile(filepath.Join(pdir, "menu.entries"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return pluginsDir
}

func TestList_NoPluginsAtAll(t *testing.T) {
	dir := t.TempDir()
	r := fspaletteentries.New(filepath.Join(dir, "missing"), "")
	got, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("want nil, got %v", got)
	}
}

func TestList_EnabledFile_RestrictsToListed(t *testing.T) {
	pluginsDir := scaffold(t, map[string]string{
		"sidekick":   "🤖\tToggle Claude\trun-shell foo\tSidekick\n",
		"kompendium": "📓\tAdd Note\trun-shell bar\tKompendium\n",
		"unused":     "X\tNope\trun-shell baz\tMisc\n",
	})
	enabled := filepath.Join(filepath.Dir(pluginsDir), "enabled-plugins")
	if err := os.WriteFile(enabled, []byte("# header\nsidekick\nkompendium\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := fspaletteentries.New(pluginsDir, enabled).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(got), got)
	}
	if got[0].Action != "run-shell foo" || got[1].Action != "run-shell bar" {
		t.Errorf("plugin order not preserved: %+v", got)
	}
	if got[0].Order != 0 || got[1].Order != 1 {
		t.Errorf("Order not assigned cumulatively: %+v", got)
	}
}

func TestList_FallbackToAllSubdirs_WhenEnabledMissing(t *testing.T) {
	pluginsDir := scaffold(t, map[string]string{
		"alpha": "★\tDo A\trun-shell A\tWorktime\n",
		"bravo": "★\tDo B\trun-shell B\tGit\n",
	})

	r := fspaletteentries.New(pluginsDir, filepath.Join(t.TempDir(), "missing"))
	got, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(got), got)
	}

	labels := []string{got[0].Label, got[1].Label}
	wantA := []string{"Do A", "Do B"}
	wantB := []string{"Do B", "Do A"}
	if !reflect.DeepEqual(labels, wantA) && !reflect.DeepEqual(labels, wantB) {
		t.Errorf("got %v — neither alpha-then-bravo nor bravo-then-alpha", labels)
	}
}

func TestList_SkipPluginsWithoutMenuEntries(t *testing.T) {
	pluginsDir := scaffold(t, map[string]string{
		"with":    "★\tHas\trun-shell yes\tMisc\n",
		"without": "", // no menu.entries file
	})
	enabled := filepath.Join(filepath.Dir(pluginsDir), "enabled-plugins")
	if err := os.WriteFile(enabled, []byte("with\nwithout\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := fspaletteentries.New(pluginsDir, enabled).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Label != "Has" {
		t.Errorf("got %+v", got)
	}
}

func TestList_DefaultsSectionToMisc(t *testing.T) {
	pluginsDir := scaffold(t, map[string]string{
		"p": "★\tNo Section\trun-shell x\n", // 3-col only
	})
	r := fspaletteentries.New(pluginsDir, "")
	got, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Section != "Misc" {
		t.Errorf("section default: got %+v", got)
	}
}

func TestList_HonoursKeybind(t *testing.T) {
	pluginsDir := scaffold(t, map[string]string{
		"p": "★\tWith Bind\trun-shell x\tMisc\tprefix+x\n",
	})
	got, err := fspaletteentries.New(pluginsDir, "").List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Keybind != "prefix+x" {
		t.Errorf("keybind: got %+v", got)
	}
}

func TestList_SkipsBlankAndComment(t *testing.T) {
	pluginsDir := scaffold(t, map[string]string{
		"p": "" +
			"# header\n" +
			"\n" +
			"★\tValid\trun-shell ok\tMisc\n" +
			"# trailing comment\n" +
			"   \n" +
			"\t\t\n" + // empty action col → drop
			"only-one-col\n", // too few cols → drop
	})
	got, err := fspaletteentries.New(pluginsDir, "").List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Label != "Valid" {
		t.Errorf("got %+v", got)
	}
}

func TestEnabledPlugins_OpenError(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	r := fspaletteentries.New(filepath.Join(dir, "plugins"), filepath.Join(regular, "child"))
	if _, err := r.List(); err == nil {
		t.Fatal("want error on un-openable enabledFile, got nil")
	}
}
