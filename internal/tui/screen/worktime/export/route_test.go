package export_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/export"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeAPI struct{ payload []byte }

func (f fakeAPI) Export(_ context.Context, _, _, _, _ string) ([]byte, error) {
	return f.payload, nil
}

func fixedNow() time.Time { return time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC) }

func TestExportRoute_rendersFormAndDefaults(t *testing.T) {
	r := export.NewRoute(fakeAPI{}, fixedNow, theme.Default, wtnav.Registry{})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Range") || !strings.Contains(body, "Format") {
		t.Fatalf("missing form fields:\n%s", body)
	}
	if r.Title() != "Export" {
		t.Fatalf("title = %q, want Export", r.Title())
	}
}

func TestExportRoute_writesFileOnEnter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.md")
	r := export.NewRoute(fakeAPI{payload: []byte("# hi")}, fixedNow, theme.Default, wtnav.Registry{})
	r = export.WithPathForTest(r, path)
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		r2, c := r.Update(msg)
		r, cmd = r2.(*export.Route), c
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "# hi" {
		t.Fatalf("export not written: err=%v content=%q", err, string(b))
	}
}

func TestExportRoute_invalidDate_noExport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "should-not-exist.md")

	r := export.NewRoute(fakeAPI{payload: []byte("x")}, fixedNow, theme.Default, wtnav.Registry{})
	r = export.WithDatesForTest(r, "nope", "also-nope")
	r = export.WithPathForTest(r, path)

	r2, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r = r2.(*export.Route)
	if cmd != nil {
		t.Fatalf("submit returned non-nil cmd for invalid date, want nil")
	}

	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Ungültiges Datum") {
		t.Fatalf("expected 'Ungültiges Datum' in view, got:\n%s", body)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file must not exist, got err=%v", err)
	}
}

func TestExportRoute_toBeforeFrom_noExport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "should-not-exist.md")

	r := export.NewRoute(fakeAPI{payload: []byte("x")}, fixedNow, theme.Default, wtnav.Registry{})
	r = export.WithDatesForTest(r, "2026-06-10", "2026-06-01")
	r = export.WithPathForTest(r, path)

	r2, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r = r2.(*export.Route)
	if cmd != nil {
		t.Fatalf("submit returned non-nil cmd for to<from, want nil")
	}

	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "bis muss >= von sein") {
		t.Fatalf("expected 'bis muss >= von sein' in view, got:\n%s", body)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file must not exist, got err=%v", err)
	}
}
