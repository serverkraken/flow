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

// TestExportRoute_toBeforeFrom_noExport verifies that submit rejects to < from.
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

// TestPresetRange_kw verifies the "kw" preset from a Wednesday (2026-06-17).
// monat is default; left once goes to kw.
// The date picker renders segments with spaces: " 2026 - 06 - 15  (Mo)"
// so we check year/month/day components separately.
func TestPresetRange_kw(t *testing.T) {
	now := fixedNow() // 2026-06-17 Wednesday
	r := export.NewRoute(fakeAPI{}, func() time.Time { return now }, theme.Default, wtnav.Registry{})
	r2, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	r = r2.(*export.Route)
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	// kw from = 2026-06-15 (Monday), to = 2026-06-17 (Wednesday)
	if !strings.Contains(body, "2026") {
		t.Errorf("kw year not in view; body:\n%s", body)
	}
	if !strings.Contains(body, "15") {
		t.Errorf("kw from-day (15) not in view; body:\n%s", body)
	}
	if !strings.Contains(body, "17") {
		t.Errorf("kw to-day (17) not in view; body:\n%s", body)
	}
}

// TestPresetRange_monat verifies the "monat" preset (default).
func TestPresetRange_monat(t *testing.T) {
	now := fixedNow() // 2026-06-17
	r := export.NewRoute(fakeAPI{}, func() time.Time { return now }, theme.Default, wtnav.Registry{})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	// monat from = 2026-06-01, to = 2026-06-17
	if !strings.Contains(body, "2026") {
		t.Errorf("monat year not in view; body:\n%s", body)
	}
	if !strings.Contains(body, "01") {
		t.Errorf("monat from-day (01) not in view; body:\n%s", body)
	}
	if !strings.Contains(body, "17") {
		t.Errorf("monat to-day (17) not in view; body:\n%s", body)
	}
}

// TestPresetRange_letzter verifies the "letzter" preset (previous month).
func TestPresetRange_letzter(t *testing.T) {
	now := fixedNow() // 2026-06-17
	r := export.NewRoute(fakeAPI{}, func() time.Time { return now }, theme.Default, wtnav.Registry{})
	// monat -> right once -> letzter
	r2, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	r = r2.(*export.Route)
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	// letzter = May 2026: from 2026-05-01, to 2026-05-31
	if !strings.Contains(body, "05") {
		t.Errorf("letzter month (05) not in view; body:\n%s", body)
	}
	if !strings.Contains(body, "31") {
		t.Errorf("letzter to-day (31) not in view; body:\n%s", body)
	}
}

// TestCycleFormat_forwardAndBackward covers forward and backward format wrapping.
func TestCycleFormat_forwardAndBackward(t *testing.T) {
	now := fixedNow()
	// Navigate to Format field (focus=3) via Tab x3 from default focus=0.
	newRoute := func() *export.Route {
		r := export.NewRoute(fakeAPI{}, func() time.Time { return now }, theme.Default, wtnav.Registry{})
		for i := 0; i < 3; i++ {
			r2, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			r = r2.(*export.Route)
		}
		return r
	}
	frame := shell.Frame{Width: 80, Height: 24, Pal: theme.Default}

	// Default format is "md". Right -> "csv".
	r := newRoute()
	r2, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	r = r2.(*export.Route)
	if !strings.Contains(r.View(frame), "csv") {
		t.Errorf("after right on Format, expected csv; got:\n%s", r.View(frame))
	}

	// csv -> Right -> json
	r3, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	r = r3.(*export.Route)
	if !strings.Contains(r.View(frame), "json") {
		t.Errorf("after two rights on Format, expected json; got:\n%s", r.View(frame))
	}

	// json -> Right -> wraps to md
	r4, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	r = r4.(*export.Route)
	if !strings.Contains(r.View(frame), "md") {
		t.Errorf("after wrap-around right on Format, expected md; got:\n%s", r.View(frame))
	}

	// md -> Left -> wraps backward to json
	r5, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	r = r5.(*export.Route)
	if !strings.Contains(r.View(frame), "json") {
		t.Errorf("after left-wrap on Format, expected json; got:\n%s", r.View(frame))
	}
}

// TestCyclePreset_backward verifies left-cycling from "monat" gives "kw".
func TestCyclePreset_backward(t *testing.T) {
	now := fixedNow()
	r := export.NewRoute(fakeAPI{}, func() time.Time { return now }, theme.Default, wtnav.Registry{})
	r2, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	r = r2.(*export.Route)
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "kw") {
		t.Errorf("after left on preset from monat, expected kw; got:\n%s", body)
	}
}

// TestExpandHome_tilde verifies the "~/..." path is expanded to the home dir.
func TestExpandHome_tilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	outPath := filepath.Join(home, "flow-test-expand-home-tui.md")
	t.Cleanup(func() { _ = os.Remove(outPath) })

	now := fixedNow()
	r := export.NewRoute(fakeAPI{payload: []byte("hi")}, func() time.Time { return now }, theme.Default, wtnav.Registry{})
	r = export.WithPathForTest(r, "~/flow-test-expand-home-tui.md")
	r = export.WithDatesForTest(r, "2026-06-01", "2026-06-17")

	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("submit should return a cmd")
	}
	msg := cmd()
	r2, _ := r.Update(msg)
	r = r2.(*export.Route)
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "geschrieben") {
		t.Errorf("expected success status; got:\n%s", body)
	}
}

// TestExpandHome_absolute verifies an absolute path passes through unchanged.
func TestExpandHome_absolute(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "abs-export.md")
	now := fixedNow()

	r := export.NewRoute(fakeAPI{payload: []byte("data")}, func() time.Time { return now }, theme.Default, wtnav.Registry{})
	r = export.WithPathForTest(r, outPath)
	r = export.WithDatesForTest(r, "2026-06-01", "2026-06-17")

	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	msg := cmd()
	r2, _ := r.Update(msg)
	r = r2.(*export.Route)
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "geschrieben") {
		t.Errorf("expected success status for absolute path; got:\n%s", body)
	}
}

// TestExportRoute_tabFocus verifies Tab advances focus and Shift-Tab retreats.
func TestExportRoute_tabFocus(t *testing.T) {
	now := fixedNow()
	r := export.NewRoute(fakeAPI{}, func() time.Time { return now }, theme.Default, wtnav.Registry{})
	frame := shell.Frame{Width: 80, Height: 24, Pal: theme.Default}

	// Default focus=0 (Range). View should show the active marker.
	if !strings.Contains(r.View(frame), "▸") {
		t.Fatalf("initial view missing active marker:\n%s", r.View(frame))
	}

	// Tab x5 -> wraps back to focus=0
	for i := 0; i < 5; i++ {
		r2, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		r = r2.(*export.Route)
	}
	if !strings.Contains(r.View(frame), "▸") {
		t.Fatalf("after 5 tabs, active marker missing:\n%s", r.View(frame))
	}

	// Shift-Tab from focus=0 -> focus=4
	r3, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	r = r3.(*export.Route)
	if !strings.Contains(r.View(frame), "▸") {
		t.Fatalf("after shift-tab, active marker missing:\n%s", r.View(frame))
	}
}

// TestExportRoute_keyHints checks that KeyHints returns non-empty hints.
func TestExportRoute_keyHints(t *testing.T) {
	r := export.NewRoute(fakeAPI{}, fixedNow, theme.Default, wtnav.Registry{})
	hints := r.KeyHints()
	if len(hints) == 0 {
		t.Fatal("KeyHints should return non-empty hints")
	}
}

// TestExportRoute_init checks that Init returns nil (no startup cmd).
func TestExportRoute_init(t *testing.T) {
	r := export.NewRoute(fakeAPI{}, fixedNow, theme.Default, wtnav.Registry{})
	if r.Init() != nil {
		t.Fatal("Init should return nil")
	}
}

// TestExportRoute_editVonFieldSetsCustomPreset types into the von field
// and asserts that the preset switches to "custom".
func TestExportRoute_editVonFieldSetsCustomPreset(t *testing.T) {
	now := fixedNow()
	r := export.NewRoute(fakeAPI{}, func() time.Time { return now }, theme.Default, wtnav.Registry{})

	// Tab to von field (focus=1)
	r2, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	r = r2.(*export.Route)

	// Type a digit -> this should set preset to "custom"
	r3, _ := r.Update(tea.KeyPressMsg{Text: "2"})
	r = r3.(*export.Route)

	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "custom") {
		t.Fatalf("after typing in von field, preset should be 'custom'; got:\n%s", body)
	}
}

// TestExportRoute_pathEditedLatch confirms that once the path is manually
// edited, cycling the preset no longer auto-updates the path.
func TestExportRoute_pathEditedLatch(t *testing.T) {
	now := fixedNow()
	r := export.NewRoute(fakeAPI{}, func() time.Time { return now }, theme.Default, wtnav.Registry{})
	frame := shell.Frame{Width: 80, Height: 24, Pal: theme.Default}

	// Navigate to Pfad field (focus=4) via Tab x4
	for i := 0; i < 4; i++ {
		r2, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		r = r2.(*export.Route)
	}

	// Type "x" to latch pathEdited=true
	r3, _ := r.Update(tea.KeyPressMsg{Text: "x"})
	r = r3.(*export.Route)

	// Navigate back to Range field (focus=0) via Shift-Tab x4
	for i := 0; i < 4; i++ {
		r4, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
		r = r4.(*export.Route)
	}

	// Cycle the preset (right -> letzter)
	r5, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	r = r5.(*export.Route)

	// Path should still contain the "x" we typed
	if !strings.Contains(r.View(frame), "x") {
		t.Fatalf("after cycling preset with pathEdited=true, edited path should remain; got:\n%s", r.View(frame))
	}
}

// TestExportRoute_arrowEditsPicker verifies arrow keys change the picker display
// when the von field is focused.
func TestExportRoute_arrowEditsPicker(t *testing.T) {
	now := fixedNow()
	r := export.NewRoute(fakeAPI{}, func() time.Time { return now }, theme.Default, wtnav.Registry{})

	// Tab to von field (focus=1)
	r2, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	r = r2.(*export.Route)

	body1 := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})

	// Arrow down steps the active segment (year) down by one
	r3, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	r = r3.(*export.Route)
	body2 := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})

	if body1 == body2 {
		t.Fatal("view should change after arrow key in von picker field")
	}
}

func TestExportRoute_capturesOnTextFieldsNotChoiceFields(t *testing.T) {
	r := export.NewRoute(fakeAPI{}, fixedNow, theme.Default, wtnav.Registry{})
	// focus 0 = Range (choice) → not capturing
	if r.CapturesInput() {
		t.Fatal("Range field must not capture (globals stay reachable)")
	}
	// Tab to focus 1 = von (text/picker) → capturing
	r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !r.CapturesInput() {
		t.Fatal("von field must capture input")
	}
}

func TestExportRoute_escEmitsPop(t *testing.T) {
	r := export.NewRoute(fakeAPI{}, fixedNow, theme.Default, wtnav.Registry{})
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Esc should emit a command")
	}
	if _, ok := cmd().(shell.PopRouteMsg); !ok {
		t.Fatalf("Esc should emit shell.PopRouteMsg, got %T", cmd())
	}
}

// TestExportRoute_errMsgUpdatesStatus feeds a failed write back to the route.
func TestExportRoute_errMsgUpdatesStatus(t *testing.T) {
	// Use a path in a nonexistent directory to force a write error.
	badPath := "/nonexistent-dir-xyz/export.md"
	now := fixedNow()
	r := export.NewRoute(fakeAPI{payload: []byte("data")}, func() time.Time { return now }, theme.Default, wtnav.Registry{})
	r = export.WithPathForTest(r, badPath)
	r = export.WithDatesForTest(r, "2026-06-01", "2026-06-17")

	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("submit should return a cmd")
	}
	msg := cmd()
	r2, _ := r.Update(msg)
	r = r2.(*export.Route)
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Fehler") {
		t.Fatalf("expected 'Fehler' in view after write error; got:\n%s", body)
	}
}

func TestExportRoute_presetFillsPickersAndExports(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.md")
	r := export.NewRoute(fakeAPI{payload: []byte("# hi")}, fixedNow, theme.Default, wtnav.Registry{})
	// Default preset "monat" with fixedNow (2026-06-17) → from 2026-06-01.
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "2026") || !strings.Contains(body, "06") {
		t.Fatalf("view should show the preset-filled date pickers:\n%s", body)
	}
	r = export.WithPathForTest(r, path)
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		var c tea.Cmd
		var nr shell.Route
		nr, c = r.Update(msg)
		r, cmd = nr.(*export.Route), c
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "# hi" {
		t.Fatalf("export not written: err=%v content=%q", err, string(b))
	}
}

func TestExportRoute_calendarShownWhenDateFocused(t *testing.T) {
	r := export.NewRoute(fakeAPI{}, fixedNow, theme.Default, wtnav.Registry{})
	r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // focus von
	body := r.View(shell.Frame{Width: 80, Height: 30, Pal: theme.Default})
	if !strings.Contains(body, "Mo Di Mi Do Fr Sa So") {
		t.Fatalf("calendar grid should show when a date field is focused:\n%s", body)
	}
}
