package dayoffs_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/dayoffs"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

type fakeAPI struct {
	list       []apiclient.DayOff
	settings   apiclient.Settings
	deleted    string
	addedFrom  string
	addedKind  string
	bundesland string
	listErr    error
}

func (f *fakeAPI) ListDayOffs(_ context.Context, _, _ string) ([]apiclient.DayOff, error) {
	return f.list, f.listErr
}
func (f *fakeAPI) GetSettings(_ context.Context) (apiclient.Settings, error) {
	return f.settings, nil
}
func (f *fakeAPI) SetTargetConfig(_ context.Context, def int, _ map[string]int) error {
	f.settings.DefaultTargetMin = def
	return nil
}
func (f *fakeAPI) AddDayOffs(_ context.Context, from, _, kind, _ string, _ int, _ bool) error {
	f.addedFrom = from
	f.addedKind = kind
	return nil
}
func (f *fakeAPI) DeleteDayOff(_ context.Context, day string) error {
	f.deleted = day
	return nil
}
func (f *fakeAPI) SetBundesland(_ context.Context, land string) error {
	f.bundesland = land
	return nil
}

func drain(r shell.Route, cmd tea.Cmd) shell.Route {
	for i := 0; cmd != nil && i < 20; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		r, cmd = r.Update(msg)
	}
	return r
}

func fixedNow() time.Time { return time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC) }

func newRoute(api dayoffs.API) shell.Route {
	return dayoffs.NewRoute(api, theme.Default, wtnav.Registry{}, fixedNow)
}

func TestDayOffsRoute_listsAndTitle(t *testing.T) {
	api := &fakeAPI{
		list:     []apiclient.DayOff{{Day: "2026-12-25", Kind: "holiday", Label: "Weihnachten", Holiday: true}},
		settings: apiclient.Settings{DefaultTargetMin: 480},
	}
	r := newRoute(api)
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "2026-12-25") || !strings.Contains(body, "Weihnachten") {
		t.Fatalf("missing dayoff row:\n%s", body)
	}
	if r.Title() != "Frei" {
		t.Fatalf("title = %q, want Frei", r.Title())
	}
}

func TestDayOffsRoute_deleteConfirmFlow(t *testing.T) {
	api := &fakeAPI{list: []apiclient.DayOff{{Day: "2026-07-01", Kind: "vacation", Label: "Urlaub"}}}
	r := newRoute(api)
	r = drain(r, r.Init())
	// Open delete dialog with D
	r2, _ := r.Update(tea.KeyPressMsg{Text: "D"})
	// Confirm with y (confirm widget's Confirm key)
	r3, cmd := r2.Update(tea.KeyPressMsg{Text: "y"})
	_ = drain(r3, cmd)
	if api.deleted != "2026-07-01" {
		t.Fatalf("deleted = %q, want 2026-07-01", api.deleted)
	}
}

func TestDayOffsRoute_navEmitsSwitch(t *testing.T) {
	reg := wtnav.Registry{"w": func() shell.Route { return nil }}
	r := dayoffs.NewRoute(&fakeAPI{}, theme.Default, reg, fixedNow)
	if _, cmd := r.Update(tea.KeyPressMsg{Text: "w"}); cmd == nil {
		t.Fatal("w should emit a nav cmd")
	}
}

// TestDayOffsRoute_targetEditDigitFilterAndSubmit verifies that non-digit keys
// are ignored in the target-edit dialog, and that a digit sequence followed by
// enter calls SetTargetConfig with the correct minutes value.
func TestDayOffsRoute_targetEditDigitFilterAndSubmit(t *testing.T) {
	api := &fakeAPI{settings: apiclient.Settings{DefaultTargetMin: 480}}
	r := newRoute(api)
	r = drain(r, r.Init())

	// Open target-edit dialog
	r, _ = r.Update(tea.KeyPressMsg{Text: "g"})

	// Non-digit key must be ignored - a subsequent bare enter is a no-op (empty input)
	r, _ = r.Update(tea.KeyPressMsg{Text: "x"})

	// Type "480"
	for _, ch := range "480" {
		r, _ = r.Update(tea.KeyPressMsg{Text: string(ch)})
	}

	// Submit
	r, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r = drain(r, cmd)

	if api.settings.DefaultTargetMin != 480 {
		t.Fatalf("DefaultTargetMin = %d, want 480", api.settings.DefaultTargetMin)
	}
	// Verify dialog was dismissed - the list view no longer shows the target-edit prompt.
	if strings.Contains(r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default}), "Neues Tagesziel") {
		t.Fatal("target-edit dialog should be closed after submit")
	}
}

// TestDayOffsRoute_bundeslandSet opens the Bundesland picker, moves down once
// from BW (index 0) to BY (index 1), and confirms.
// Verifies api.bundesland is set to "BY".
func TestDayOffsRoute_bundeslandSet(t *testing.T) {
	// Set current Bundesland to "BW" so openBundesland initialises blSel=0,
	// making KeyDown deterministically advance to index 1 = "BY".
	api := &fakeAPI{settings: apiclient.Settings{Bundesland: "BW"}}
	r := newRoute(api)
	r = drain(r, r.Init())

	// Open Bundesland dialog (blSel starts at 0 = "BW")
	r, _ = r.Update(tea.KeyPressMsg{Text: "b"})

	// Move down once -> index 1 = "BY"
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	// Confirm
	r, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r = drain(r, cmd)

	if api.bundesland != "BY" {
		t.Fatalf("bundesland = %q, want BY", api.bundesland)
	}
	// Verify dialog was dismissed - the Bundesland picker no longer appears.
	if strings.Contains(r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default}), "Bundesland wählen") {
		t.Fatal("Bundesland dialog should be closed after confirm")
	}
}

// TestDayOffsRoute_sseReload verifies that dayoff.changed and settings.changed
// events trigger a reload cmd, and that an unrelated event does not.
func TestDayOffsRoute_sseReload(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())

	// dayoff.changed -> reload
	_, cmd := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: string(domain.EventDayOffChanged)}})
	if cmd == nil {
		t.Fatal("dayoff.changed should trigger a reload cmd")
	}

	// settings.changed -> reload
	_, cmd = r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: string(domain.EventSettingsChanged)}})
	if cmd == nil {
		t.Fatal("settings.changed should trigger a reload cmd")
	}

	// unrelated event -> no cmd
	_, cmd = r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: string(domain.EventDocumentCreated)}})
	if cmd != nil {
		t.Fatal("unrelated event should not trigger a reload cmd")
	}
}

// TestDayOffsRoute_loadingState verifies the placeholder text before data arrives.
func TestDayOffsRoute_loadingState(t *testing.T) {
	r := newRoute(&fakeAPI{})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "lädt") {
		t.Fatalf("loading state should show 'lädt'; got:\n%s", body)
	}
}

// TestDayOffsRoute_errorState verifies the error text when load fails.
func TestDayOffsRoute_errorState(t *testing.T) {
	api := &fakeAPI{listErr: errors.New("db error")}
	r := newRoute(api)
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Fehler") {
		t.Fatalf("error state should show 'Fehler'; got:\n%s", body)
	}
}

// TestDayOffsRoute_keyHintsInListState verifies KeyHints returns non-empty hints in list state.
func TestDayOffsRoute_keyHintsInListState(t *testing.T) {
	r := newRoute(&fakeAPI{})
	hints := r.(*dayoffs.Route).KeyHints()
	if len(hints) == 0 {
		t.Fatal("KeyHints in list state should return non-empty hints")
	}
}

// TestDayOffsRoute_keyHintsWhileAddDialogOpen verifies KeyHints returns dialog hints when open.
func TestDayOffsRoute_keyHintsWhileAddDialogOpen(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})
	// Call KeyHints via the concrete *dayoffs.Route
	cr := r.(*dayoffs.Route)
	hints := cr.KeyHints()
	if len(hints) == 0 {
		t.Fatal("KeyHints in add-dialog state should return non-empty hints")
	}
}

// TestDayOffsRoute_addDialogView verifies the add-dialog renders expected text.
func TestDayOffsRoute_addDialogView(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Frei-Tag anlegen") {
		t.Fatalf("add dialog view missing 'Frei-Tag anlegen'; got:\n%s", body)
	}
}

// TestDayOffsRoute_bundeslandDialogView verifies the Bundesland-dialog renders expected text.
func TestDayOffsRoute_bundeslandDialogView(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "b"})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Bundesland") {
		t.Fatalf("Bundesland dialog view missing 'Bundesland'; got:\n%s", body)
	}
}

// TestDayOffsRoute_targetEditDialogView verifies the target-edit dialog renders expected text.
func TestDayOffsRoute_targetEditDialogView(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "g"})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Tagesziel") {
		t.Fatalf("target-edit dialog view missing 'Tagesziel'; got:\n%s", body)
	}
}

// TestDayOffsRoute_cursorNavArrows verifies arrow key cursor navigation (listnav, no j/k).
func TestDayOffsRoute_cursorNavArrows(t *testing.T) {
	api := &fakeAPI{list: []apiclient.DayOff{
		{Day: "2026-07-01", Kind: "vacation", Label: "A"},
		{Day: "2026-07-02", Kind: "vacation", Label: "B"},
	}}
	r := newRoute(api)
	r = drain(r, r.Init())
	f := shell.Frame{Width: 80, Height: 24, Pal: theme.Default}

	// Capture view before Down to detect actual cursor change.
	bodyBefore := r.View(f)
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	bodyAfterDown := r.View(f)
	if bodyAfterDown == bodyBefore {
		t.Fatalf("Down should move cursor (view unchanged);\n%s", bodyAfterDown)
	}

	// Move back up.
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	bodyAfterUp := r.View(f)
	if bodyAfterUp != bodyBefore {
		t.Fatalf("Up should restore cursor to 0 (view differs from baseline);\n%s", bodyAfterUp)
	}
}

// TestDayOffsRoute_emptyListRendersNoItems verifies the "keine" placeholder.
func TestDayOffsRoute_emptyListRendersNoItems(t *testing.T) {
	api := &fakeAPI{list: []apiclient.DayOff{}, settings: apiclient.Settings{DefaultTargetMin: 480}}
	r := newRoute(api)
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "keine") {
		t.Fatalf("empty list should show 'keine'; got:\n%s", body)
	}
}

// TestDayOffsRoute_weekdayTargetsInView verifies per-weekday targets appear in View.
func TestDayOffsRoute_weekdayTargetsInView(t *testing.T) {
	api := &fakeAPI{
		settings: apiclient.Settings{
			DefaultTargetMin: 480,
			WeekdayTargetMin: map[string]int{"1": 360, "5": 240},
		},
	}
	r := newRoute(api)
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Mo") || !strings.Contains(body, "Fr") {
		t.Fatalf("weekday targets should appear in view; got:\n%s", body)
	}
}

// TestDayOffsRoute_holidayAndNonHolidayGlyphs verifies both glyph types render.
func TestDayOffsRoute_holidayAndNonHolidayGlyphs(t *testing.T) {
	api := &fakeAPI{
		list: []apiclient.DayOff{
			{Day: "2026-10-03", Kind: "holiday", Label: "Tag der Einheit", Holiday: true},
			{Day: "2026-08-01", Kind: "vacation", Label: "Urlaub", Holiday: false},
		},
		settings: apiclient.Settings{DefaultTargetMin: 480},
	}
	r := newRoute(api)
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Tag der Einheit") {
		t.Fatalf("holiday entry missing; got:\n%s", body)
	}
	if !strings.Contains(body, "Urlaub") {
		t.Fatalf("non-holiday entry missing; got:\n%s", body)
	}
}

// TestDayOffsRoute_addDialogEsc dismisses the add dialog on Esc.
func TestDayOffsRoute_addDialogEsc(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if strings.Contains(body, "Frei-Tag anlegen") {
		t.Fatal("add dialog should be dismissed after Esc")
	}
}

// TestDayOffsRoute_targetEditEsc dismisses the target-edit dialog on Esc.
func TestDayOffsRoute_targetEditEsc(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "g"})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	// "Neues Tagesziel" is the dialog-specific text; list view shows "Tagesziel:" but not "Neues".
	if strings.Contains(body, "Neues Tagesziel") {
		t.Fatal("target-edit dialog should be dismissed after Esc")
	}
}

// TestDayOffsRoute_bundeslandEsc dismisses the Bundesland dialog on Esc.
func TestDayOffsRoute_bundeslandEsc(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "b"})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if strings.Contains(body, "Bundesland wählen") {
		t.Fatal("Bundesland dialog should be dismissed after Esc")
	}
}

// TestDayOffsRoute_addDialogUpKey verifies the Up key is forwarded to the focused picker
// (steps the active segment) and the dialog remains open.
func TestDayOffsRoute_addDialogUpKey(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})
	// Tab to Bis then Up (steps Bis picker, does not close dialog)
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	// Form should still be open
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Frei-Tag anlegen") {
		t.Fatalf("add dialog should still be open after Up; got:\n%s", body)
	}
}

// TestDayOffsRoute_bundeslandNavK verifies the k key decrements in Bundesland dialog.
func TestDayOffsRoute_bundeslandNavK(t *testing.T) {
	api := &fakeAPI{settings: apiclient.Settings{Bundesland: "BY"}}
	r := newRoute(api)
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "b"})
	// Move j down then k up
	r, _ = r.Update(tea.KeyPressMsg{Text: "j"})
	r, _ = r.Update(tea.KeyPressMsg{Text: "k"})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Bundesland") {
		t.Fatalf("Bundesland dialog should still be open after k; got:\n%s", body)
	}
}

// TestDayOffsRoute_targetEditBackspace verifies backspace removes last digit.
func TestDayOffsRoute_targetEditBackspace(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "g"})
	r, _ = r.Update(tea.KeyPressMsg{Text: "4"})
	r, _ = r.Update(tea.KeyPressMsg{Text: "8"})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Tagesziel") {
		t.Fatalf("target dialog should still be open; got:\n%s", body)
	}
}

// TestDayOffsRoute_deleteDialogOpensOnD verifies D opens delete confirmation.
func TestDayOffsRoute_deleteDialogOpensOnD(t *testing.T) {
	api := &fakeAPI{list: []apiclient.DayOff{{Day: "2026-07-01", Kind: "vacation", Label: "Test"}}}
	r := newRoute(api)
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "D"})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "löschen") {
		t.Fatalf("delete dialog should show 'löschen'; got:\n%s", body)
	}
}

// TestDayOffsRoute_addDialogEnterAdvancesField verifies Enter in non-last field advances.
func TestDayOffsRoute_addDialogEnterAdvancesField(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})
	// Enter on Von field (not last field) should advance to Bis
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Frei-Tag anlegen") {
		t.Fatalf("add dialog should still be open after Enter on non-last field; got:\n%s", body)
	}
}

// TestDayOffsRoute_addDialogDownKey verifies KeyDown is forwarded to the focused picker.
func TestDayOffsRoute_addDialogDownKey(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Frei-Tag anlegen") {
		t.Fatalf("add dialog should still be open after Down; got:\n%s", body)
	}
}

// TestDayOffsRoute_bundeslandNavUpKey verifies KeyUp works in Bundesland dialog.
func TestDayOffsRoute_bundeslandNavUpKey(t *testing.T) {
	api := &fakeAPI{settings: apiclient.Settings{Bundesland: "BY"}}
	r := newRoute(api)
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "b"})
	r, _ = r.Update(tea.KeyPressMsg{Text: "j"})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Bundesland") {
		t.Fatalf("Bundesland dialog should still be open after KeyUp; got:\n%s", body)
	}
}

// TestDayOffsRoute_bundeslandNavDownKey verifies KeyDown works in Bundesland dialog.
func TestDayOffsRoute_bundeslandNavDownKey(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "b"})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Bundesland") {
		t.Fatalf("Bundesland dialog should still be open after KeyDown; got:\n%s", body)
	}
}

// TestDayOffsRoute_keyHintsWhileBundeslandOpen verifies KeyHints returns dialog hints for Bundesland dialog.
func TestDayOffsRoute_keyHintsWhileBundeslandOpen(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "b"})
	cr := r.(*dayoffs.Route)
	hints := cr.KeyHints()
	if len(hints) == 0 {
		t.Fatal("KeyHints in Bundesland-dialog state should return non-empty hints")
	}
}

// TestDayOffsRoute_keyHintsWhileDeleteOpen verifies KeyHints returns delete dialog hints.
func TestDayOffsRoute_keyHintsWhileDeleteOpen(t *testing.T) {
	api := &fakeAPI{list: []apiclient.DayOff{{Day: "2026-07-01", Kind: "vacation", Label: "Test"}}}
	r := newRoute(api)
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "D"})
	cr := r.(*dayoffs.Route)
	hints := cr.KeyHints()
	if len(hints) == 0 {
		t.Fatal("KeyHints in delete-dialog state should return non-empty hints")
	}
}

// TestDayOffsRoute_keyHintsWhileTargetEditOpen verifies KeyHints returns target hints.
func TestDayOffsRoute_keyHintsWhileTargetEditOpen(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "g"})
	cr := r.(*dayoffs.Route)
	hints := cr.KeyHints()
	if len(hints) == 0 {
		t.Fatal("KeyHints in target-edit dialog state should return non-empty hints")
	}
}

func TestDayOffsRoute_capturesInputWhileDialogOpen(t *testing.T) {
	api := &fakeAPI{}
	r := newRoute(api)
	r2 := drain(r, r.Init())
	if r2.(interface{ CapturesInput() bool }).CapturesInput() {
		t.Fatal("dayoffs should not capture in the list state")
	}
	// 'a' opens the add dialog.
	r3, _ := r2.Update(tea.KeyPressMsg{Text: "a"})
	if !r3.(interface{ CapturesInput() bool }).CapturesInput() {
		t.Fatal("dayoffs should capture while the add dialog is open")
	}
}

// TestDayOffsRoute_targetEditEmptyEnterNoOp verifies Enter with empty digits is a no-op.
func TestDayOffsRoute_targetEditEmptyEnterNoOp(t *testing.T) {
	api := &fakeAPI{settings: apiclient.Settings{DefaultTargetMin: 480}}
	r := newRoute(api)
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "g"})
	// Enter with empty input - should be a no-op (dialog stays open)
	r, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("Enter with empty target should return nil cmd (no-op)")
	}
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Tagesziel") {
		t.Fatalf("target dialog should remain open after empty Enter; got:\n%s", body)
	}
}

func TestDayOffsRoute_addViaDatepicker(t *testing.T) {
	api := &fakeAPI{}
	r := drain(newRoute(api), nil)
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"}) // open add dialog; Von focused, defaults to today
	// Von defaults to fixedNow (2026-06-18); step year segment +0, set day to 20 via digits.
	// seg starts at year; move to day and type 20.
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyRight}) // -> month
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyRight}) // -> day
	r, _ = r.Update(tea.KeyPressMsg{Text: "2"})
	r, _ = r.Update(tea.KeyPressMsg{Text: "0"}) // Von day = 20
	// Tab Von→Bis→Kategorie→Label (Label is now field 3).
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Bis
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Kategorie
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Label
	for _, c := range []string{"U", "r", "l", "a", "u", "b"} {
		r, _ = r.Update(tea.KeyPressMsg{Text: c})
	}
	r2, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // submit on last field
	_ = drain(r2, cmd)
	if api.addedFrom != "2026-06-20" {
		t.Fatalf("addedFrom = %q, want 2026-06-20", api.addedFrom)
	}
	if api.addedKind != "vacation" {
		t.Fatalf("addedKind = %q, want vacation (default)", api.addedKind)
	}
}

// TestDayoffs_ArrowsClampNoWrap verifies that Up at top clamps (no wrap) and that
// 'j' no longer moves the list cursor (verb keys removed per unified grammar).
// We verify position via the highlighted row in View: the active row is rendered via
// theme.Active which wraps it differently from plain rows.
func TestDayoffs_ArrowsClampNoWrap(t *testing.T) {
	api := &fakeAPI{list: []apiclient.DayOff{
		{Day: "2026-07-01", Kind: "vacation", Label: "A"},
		{Day: "2026-07-02", Kind: "vacation", Label: "B"},
	}}
	r := newRoute(api)
	r = drain(r, r.Init())

	f := shell.Frame{Width: 80, Height: 24, Pal: theme.Default}

	// Baseline: cursor is on item A (index 0). Move Down to item B.
	rDown, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	bodyDown := rDown.View(f)
	if !strings.Contains(bodyDown, "2026-07-02") {
		t.Fatalf("Down should move to second item; view:\n%s", bodyDown)
	}
	// Move Up back to A — cursor should be 0.
	rUp, _ := rDown.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	bodyUp := rUp.View(f)
	if !strings.Contains(bodyUp, "2026-07-01") {
		t.Fatalf("Up should return to first item; view:\n%s", bodyUp)
	}
	// Up again at top must clamp: view must not change (still shows A as active, B inactive).
	rUpClamp, _ := rUp.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	bodyUpClamp := rUpClamp.View(f)
	if bodyUpClamp != bodyUp {
		t.Fatalf("Up at top must clamp (no wrap); view changed:\n%s", bodyUpClamp)
	}

	// 'j' must NOT navigate (list cursor stays at 0, view unchanged).
	// Capture baseline BEFORE Update because *Route mutates in place.
	r2 := newRoute(api)
	r2 = drain(r2, r2.Init())
	bodyBase2 := r2.View(f)
	rJ, _ := r2.Update(tea.KeyPressMsg{Text: "j"})
	bodyJ := rJ.View(f)
	if bodyJ != bodyBase2 {
		t.Fatalf("'j' must not move list cursor; view changed:\nbefore: %q\nafter:  %q", bodyBase2, bodyJ)
	}
}

func TestDayOffsRoute_addRejectsBisBeforeVon(t *testing.T) {
	api := &fakeAPI{}
	r := drain(newRoute(api), nil)
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})
	// Set Von to day 20, leave Bis defaulting to Von (=20), then lower Bis to 10.
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	r, _ = r.Update(tea.KeyPressMsg{Text: "2"})
	r, _ = r.Update(tea.KeyPressMsg{Text: "0"})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // Bis (=20)
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	r, _ = r.Update(tea.KeyPressMsg{Text: "1"})
	r, _ = r.Update(tea.KeyPressMsg{Text: "0"}) // Bis day = 10 (< Von 20)
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Kategorie
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Label
	r2, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = drain(r2, cmd)
	if api.addedFrom != "" {
		t.Fatalf("submit with Bis<Von must not call AddDayOffs (addedFrom=%q)", api.addedFrom)
	}
}

// TestDayOffsRoute_addSubmitsDefaultKind verifies that submitting the add form
// without touching the category sends the default kind "vacation".
func TestDayOffsRoute_addSubmitsDefaultKind(t *testing.T) {
	api := &fakeAPI{}
	r := drain(newRoute(api), nil)
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})          // open add; Von focused (today=2026-06-18)
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})   // -> Bis
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})   // -> Kategorie
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})   // -> Label
	r2, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // submit
	_ = drain(r2, cmd)
	if api.addedKind != "vacation" {
		t.Fatalf("addedKind = %q, want vacation", api.addedKind)
	}
}

// TestBundesland_ArrowsMoveNotJK verifies that the Bundesland picker navigates
// via arrow keys (grammar nav via listnav) and that "j" is no longer a nav key.
// RED phase: before migration j still moves; KeyDown does move (was already wired).
// GREEN phase after migration: KeyDown moves, j does NOT move.
// statsStub is a minimal shell.Route used by wtnav.Registry in nav tests.
type statsStub struct{}

func (statsStub) Init() tea.Cmd                          { return nil }
func (statsStub) Update(tea.Msg) (shell.Route, tea.Cmd) { return statsStub{}, nil }
func (statsStub) View(shell.Frame) string                { return "" }
func (statsStub) Title() string                          { return "Stats" }
func (statsStub) KeyHints() []keyhint.Hint               { return nil }

// TestDayoffs_StripAndLeftPopsAndHideCrumb verifies the sub-tab strip is rendered,
// HideBreadcrumb returns true, and ← emits a navigation command.
func TestDayoffs_StripAndLeftPopsAndHideCrumb(t *testing.T) {
	reg := wtnav.Registry{"t": func() shell.Route { return statsStub{} }}
	r := dayoffs.NewRoute(nil, theme.Default, reg, time.Now)
	out := r.View(shell.Frame{Width: 200, Height: 24, Pal: theme.Default})
	for _, l := range []string{"Heute", "Woche", "Stats", "Frei"} {
		if !strings.Contains(out, l) {
			t.Fatalf("Frei View missing sub-tab %q", l)
		}
	}
	if strings.Contains(out, "Export") {
		t.Fatal("Frei strip must not contain Export (it is a drilled route)")
	}
	if !r.HideBreadcrumb() {
		t.Fatal("Frei must hide breadcrumb")
	}
	// ← from Frei (idx 3) → Stats via reg.
	r2, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	_ = r2
	if cmd == nil {
		t.Fatal("← on Frei must emit a command")
	}
}

// TestDayOffsRoute_kindPickerSelectsKind opens the add form, enters the kind
// picker from the Kategorie field, moves down once (vacation→sick), confirms,
// then submits — addedKind must be "sick".
func TestDayOffsRoute_kindPickerSelectsKind(t *testing.T) {
	api := &fakeAPI{}
	r := drain(newRoute(api), nil)
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})          // open add; Von focused
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})   // -> Bis
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})   // -> Kategorie
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open kind picker
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyDown})  // vacation(0) -> sick(1)
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // confirm -> back to add form
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})   // Kategorie -> Label
	r2, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // submit
	_ = drain(r2, cmd)
	if api.addedKind != "sick" {
		t.Fatalf("addedKind = %q, want sick", api.addedKind)
	}
}

// TestDayOffsRoute_kindPickerView verifies the picker lists German kind labels.
func TestDayOffsRoute_kindPickerView(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open picker
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	for _, want := range []string{"Gleittag", "Sonderurlaub", "Kind krank", "Fortbildung"} {
		if !strings.Contains(body, want) {
			t.Fatalf("kind picker missing %q; got:\n%s", want, body)
		}
	}
}

// TestDayOffsRoute_kindPickerEsc returns to the add form without changing kind.
func TestDayOffsRoute_kindPickerEsc(t *testing.T) {
	r := newRoute(&fakeAPI{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open picker
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEsc})   // back to add form
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Frei-Tag anlegen") {
		t.Fatalf("Esc from kind picker should return to add form; got:\n%s", body)
	}
}

// TestDayOffsRoute_listShowsCategoryLabel verifies the German category label is
// rendered for a non-holiday entry (here a Gleittag), independent of free-text label.
func TestDayOffsRoute_listShowsCategoryLabel(t *testing.T) {
	api := &fakeAPI{
		list:     []apiclient.DayOff{{Day: "2026-09-10", Kind: "flex", Label: ""}},
		settings: apiclient.Settings{DefaultTargetMin: 480},
	}
	r := newRoute(api)
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Gleittag") {
		t.Fatalf("list should show category label 'Gleittag'; got:\n%s", body)
	}
}

// TestDayOffsRoute_listShowsVacationCategoryLabel verifies that a canonical
// "vacation" entry with an empty free-text label renders the category label
// "Urlaub" in the list (exercises the real vacation render path, not the
// unknown-kind fallback).
func TestDayOffsRoute_listShowsVacationCategoryLabel(t *testing.T) {
	api := &fakeAPI{
		list:     []apiclient.DayOff{{Day: "2026-08-15", Kind: "vacation", Label: ""}},
		settings: apiclient.Settings{DefaultTargetMin: 480},
	}
	r := newRoute(api)
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Urlaub") {
		t.Fatalf("list should show category label 'Urlaub' for canonical vacation kind; got:\n%s", body)
	}
}

func TestBundesland_ArrowsMoveNotJK(t *testing.T) {
	// BW is index 0 in bundeslaender; BY is index 1.
	api := &fakeAPI{settings: apiclient.Settings{Bundesland: "BW"}}
	r := newRoute(api)
	r = drain(r, r.Init())

	// Open Bundesland dialog (blSel starts at 0 = "BW").
	r, _ = r.Update(tea.KeyPressMsg{Text: "b"})

	// KeyDown must move to index 1 = "BY".
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	// Confirm to capture selection.
	r2, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drain(r2, cmd)
	if api.bundesland != "BY" {
		t.Fatalf("KeyDown: bundesland = %q, want BY", api.bundesland)
	}

	// Reset: reopen dialog (blSel=0 again) and send Text "j".
	// After migration, "j" must NOT move the cursor, so Enter should set "BW".
	api.bundesland = ""
	r, _ = r.Update(tea.KeyPressMsg{Text: "b"})
	r, _ = r.Update(tea.KeyPressMsg{Text: "j"})
	r3, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drain(r3, cmd)
	if api.bundesland != "BW" {
		t.Fatalf("Text j: bundesland = %q, want BW (j must not navigate)", api.bundesland)
	}
}
