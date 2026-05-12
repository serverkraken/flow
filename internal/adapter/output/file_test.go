package output_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/output"
	"github.com/serverkraken/flow/internal/testutil"
)

func TestSaveFile_WritesIntoDownloadsWithTimestampAndPreservesContent(t *testing.T) {
	home := t.TempDir()
	tg := output.New(home, &testutil.FakeTmux{})
	body := []byte("date,hours\n2026-05-08,8\n")
	path, err := tg.SaveFile("worktime-week-csv", "csv", body)
	if err != nil {
		t.Fatalf("SaveFile err: %v", err)
	}
	wantDir := filepath.Join(home, "Downloads")
	if !strings.HasPrefix(path, wantDir+string(filepath.Separator)) {
		t.Errorf("path = %q, want under %q", path, wantDir)
	}
	if !strings.HasSuffix(path, ".csv") {
		t.Errorf("path = %q, want .csv suffix", path)
	}
	base := filepath.Base(path)
	if !strings.HasPrefix(base, "worktime-week-csv-") {
		t.Errorf("path basename = %q, want prefix worktime-week-csv-", base)
	}
	read, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(read) != string(body) {
		t.Errorf("file contents = %q, want %q", read, body)
	}
}

func TestSaveFile_CreatesDownloadsDirWhenMissing(t *testing.T) {
	home := t.TempDir()
	// Sanity: Downloads doesn't exist yet.
	if _, err := os.Stat(filepath.Join(home, "Downloads")); !os.IsNotExist(err) {
		t.Fatalf("precondition: Downloads must not exist; stat err = %v", err)
	}
	tg := output.New(home, &testutil.FakeTmux{})
	if _, err := tg.SaveFile("x", "txt", []byte("hi")); err != nil {
		t.Fatalf("SaveFile err: %v", err)
	}
	info, err := os.Stat(filepath.Join(home, "Downloads"))
	if err != nil {
		t.Fatalf("Downloads must exist after SaveFile: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("Downloads is not a directory")
	}
}

// TestSaveFile_RejectsTraversalBasename pins the defensive
// filepath.IsLocal guard added per review polish. Production callers
// pass hardcoded constants for basename/ext, but the parameter is
// exported — without the guard, a future caller wiring user-supplied
// names could break out of ~/Downloads via `../../`.
func TestSaveFile_RejectsTraversalBasename(t *testing.T) {
	home := t.TempDir()
	tg := output.New(home, &testutil.FakeTmux{})
	cases := []struct{ basename, ext string }{
		{"../etc/passwd", "txt"},
		{"foo", "../sh"},
		{"a/b", "txt"},
		{"/abs", "txt"},
	}
	for _, c := range cases {
		if _, err := tg.SaveFile(c.basename, c.ext, []byte("hi")); err == nil {
			t.Errorf("SaveFile(%q,%q) accepted non-local component", c.basename, c.ext)
		}
	}
}

func TestSaveFile_DefaultsExtensionToTxt(t *testing.T) {
	home := t.TempDir()
	tg := output.New(home, &testutil.FakeTmux{})
	path, err := tg.SaveFile("x", "", []byte("hi"))
	if err != nil {
		t.Fatalf("SaveFile err: %v", err)
	}
	if !strings.HasSuffix(path, ".txt") {
		t.Errorf("path = %q, want .txt fallback suffix", path)
	}
}
