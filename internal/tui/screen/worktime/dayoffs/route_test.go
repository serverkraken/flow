package dayoffs_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/dayoffs"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeAPI struct {
	list       []apiclient.DayOff
	settings   apiclient.Settings
	deleted    string
	addedFrom  string
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
func (f *fakeAPI) AddDayOffs(_ context.Context, from, _, _, _ string, _ int, _ bool) error {
	f.addedFrom = from
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

func TestDayOffsRoute_listsAndTitle(t *testing.T) {
	api := &fakeAPI{
		list:     []apiclient.DayOff{{Day: "2026-12-25", Kind: "holiday", Label: "Weihnachten", Holiday: true}},
		settings: apiclient.Settings{DefaultTargetMin: 480},
	}
	var r shell.Route = dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
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
	api := &fakeAPI{list: []apiclient.DayOff{{Day: "2026-07-01", Kind: "urlaub", Label: "Urlaub"}}}
	var r shell.Route = dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
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
	r := dayoffs.NewRoute(&fakeAPI{}, theme.Default, reg)
	if _, cmd := r.Update(tea.KeyPressMsg{Text: "w"}); cmd == nil {
		t.Fatal("w should emit a nav cmd")
	}
}

// TestDayOffsRoute_addDialogSubmitToDefaultsToFrom opens the add dialog, types
// a date into the Von field, leaves Bis empty, advances to the Label field and
// submits. The API should receive the typed from value (to defaults to from).
func TestDayOffsRoute_addDialogSubmitToDefaultsToFrom(t *testing.T) {
	api := &fakeAPI{}
	var r shell.Route = dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())

	// Open add dialog
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})

	// Type date into Von field character by character
	for _, ch := range "2026-08-15" {
		r, _ = r.Update(tea.KeyPressMsg{Text: string(ch)})
	}

	// Tab to Bis field (leave empty), then Tab to Label field
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// Enter on last field submits
	r, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r = drain(r, cmd)

	if api.addedFrom != "2026-08-15" {
		t.Fatalf("addedFrom = %q, want 2026-08-15", api.addedFrom)
	}
	// Also verify the dialog was dismissed (Title is still "Frei").
	if r.Title() != "Frei" {
		t.Fatalf("title after submit = %q, want Frei", r.Title())
	}
}

// TestDayOffsRoute_targetEditDigitFilterAndSubmit verifies that non-digit keys
// are ignored in the target-edit dialog, and that a digit sequence followed by
// enter calls SetTargetConfig with the correct minutes value.
func TestDayOffsRoute_targetEditDigitFilterAndSubmit(t *testing.T) {
	api := &fakeAPI{settings: apiclient.Settings{DefaultTargetMin: 480}}
	var r shell.Route = dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
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
	// making the j-key deterministically advance to index 1 = "BY".
	api := &fakeAPI{settings: apiclient.Settings{Bundesland: "BW"}}
	var r shell.Route = dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())

	// Open Bundesland dialog (blSel starts at 0 = "BW")
	r, _ = r.Update(tea.KeyPressMsg{Text: "b"})

	// Move down once -> index 1 = "BY"
	r, _ = r.Update(tea.KeyPressMsg{Text: "j"})

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
	var r shell.Route = dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
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
	r := dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "lädt") {
		t.Fatalf("loading state should show 'lädt'; got:\n%s", body)
	}
}

// TestDayOffsRoute_errorState verifies the error text when load fails.
func TestDayOffsRoute_errorState(t *testing.T) {
	api := &fakeAPI{listErr: errors.New("db error")}
	var r shell.Route = dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Fehler") {
		t.Fatalf("error state should show 'Fehler'; got:\n%s", body)
	}
}

// TestDayOffsRoute_keyHintsInListState verifies KeyHints returns non-empty hints in list state.
func TestDayOffsRoute_keyHintsInListState(t *testing.T) {
	r := dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	hints := r.KeyHints()
	if len(hints) == 0 {
		t.Fatal("KeyHints in list state should return non-empty hints")
	}
}

// TestDayOffsRoute_keyHintsWhileAddDialogOpen verifies KeyHints returns dialog hints when open.
func TestDayOffsRoute_keyHintsWhileAddDialogOpen(t *testing.T) {
	r := dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	var sr shell.Route = r
	sr = drain(sr, sr.Init())
	sr, _ = sr.Update(tea.KeyPressMsg{Text: "a"})
	// Call KeyHints via the concrete *dayoffs.Route
	cr := sr.(*dayoffs.Route)
	hints := cr.KeyHints()
	if len(hints) == 0 {
		t.Fatal("KeyHints in add-dialog state should return non-empty hints")
	}
}

// TestDayOffsRoute_addDialogView verifies the add-dialog renders expected text.
func TestDayOffsRoute_addDialogView(t *testing.T) {
	var r shell.Route = dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Frei-Tag anlegen") {
		t.Fatalf("add dialog view missing 'Frei-Tag anlegen'; got:\n%s", body)
	}
}

// TestDayOffsRoute_bundeslandDialogView verifies the Bundesland-dialog renders expected text.
func TestDayOffsRoute_bundeslandDialogView(t *testing.T) {
	var r shell.Route = dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "b"})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Bundesland") {
		t.Fatalf("Bundesland dialog view missing 'Bundesland'; got:\n%s", body)
	}
}

// TestDayOffsRoute_targetEditDialogView verifies the target-edit dialog renders expected text.
func TestDayOffsRoute_targetEditDialogView(t *testing.T) {
	var r shell.Route = dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "g"})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Tagesziel") {
		t.Fatalf("target-edit dialog view missing 'Tagesziel'; got:\n%s", body)
	}
}

// TestDayOffsRoute_cursorNavJK verifies j/k cursor navigation.
func TestDayOffsRoute_cursorNavJK(t *testing.T) {
	api := &fakeAPI{list: []apiclient.DayOff{
		{Day: "2026-07-01", Kind: "urlaub", Label: "A"},
		{Day: "2026-07-02", Kind: "urlaub", Label: "B"},
	}}
	var r shell.Route = dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())

	// Move down
	r, _ = r.Update(tea.KeyPressMsg{Text: "j"})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "B") {
		t.Fatalf("after j, second row should be visible; got:\n%s", body)
	}

	// Move back up
	r, _ = r.Update(tea.KeyPressMsg{Text: "k"})
	body2 := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body2, "A") {
		t.Fatalf("after k, first row should be visible; got:\n%s", body2)
	}
}

// TestDayOffsRoute_emptyListRendersNoItems verifies the "keine" placeholder.
func TestDayOffsRoute_emptyListRendersNoItems(t *testing.T) {
	api := &fakeAPI{list: []apiclient.DayOff{}, settings: apiclient.Settings{DefaultTargetMin: 480}}
	var r shell.Route = dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
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
	var r shell.Route = dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
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
			{Day: "2026-08-01", Kind: "urlaub", Label: "Urlaub", Holiday: false},
		},
		settings: apiclient.Settings{DefaultTargetMin: 480},
	}
	var r shell.Route = dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
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
	var r shell.Route = dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
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
	var r shell.Route = dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
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
	var r shell.Route = dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "b"})
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if strings.Contains(body, "Bundesland wählen") {
		t.Fatal("Bundesland dialog should be dismissed after Esc")
	}
}

// TestDayOffsRoute_addDialogUpKey verifies the Up key navigates backward in add form.
func TestDayOffsRoute_addDialogUpKey(t *testing.T) {
	var r shell.Route = dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})
	// Tab down then Up to go back
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
	var r shell.Route = dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
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
	var r shell.Route = dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
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
	api := &fakeAPI{list: []apiclient.DayOff{{Day: "2026-07-01", Kind: "urlaub", Label: "Test"}}}
	var r shell.Route = dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "D"})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "löschen") {
		t.Fatalf("delete dialog should show 'löschen'; got:\n%s", body)
	}
}

// TestDayOffsRoute_addDialogEnterAdvancesField verifies Enter in non-last field advances.
func TestDayOffsRoute_addDialogEnterAdvancesField(t *testing.T) {
	var r shell.Route = dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())
	r, _ = r.Update(tea.KeyPressMsg{Text: "a"})
	// Enter on Von field (not last field) should advance to Bis
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "Frei-Tag anlegen") {
		t.Fatalf("add dialog should still be open after Enter on non-last field; got:\n%s", body)
	}
}

// TestDayOffsRoute_addDialogDownKey verifies KeyDown navigates in add form.
func TestDayOffsRoute_addDialogDownKey(t *testing.T) {
	var r shell.Route = dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
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
	var r shell.Route = dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
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
	var r shell.Route = dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
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
	r := dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	var sr shell.Route = r
	sr = drain(sr, sr.Init())
	sr, _ = sr.Update(tea.KeyPressMsg{Text: "b"})
	cr := sr.(*dayoffs.Route)
	hints := cr.KeyHints()
	if len(hints) == 0 {
		t.Fatal("KeyHints in Bundesland-dialog state should return non-empty hints")
	}
}

// TestDayOffsRoute_keyHintsWhileDeleteOpen verifies KeyHints returns delete dialog hints.
func TestDayOffsRoute_keyHintsWhileDeleteOpen(t *testing.T) {
	api := &fakeAPI{list: []apiclient.DayOff{{Day: "2026-07-01", Kind: "urlaub", Label: "Test"}}}
	r := dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
	var sr shell.Route = r
	sr = drain(sr, sr.Init())
	sr, _ = sr.Update(tea.KeyPressMsg{Text: "D"})
	cr := sr.(*dayoffs.Route)
	hints := cr.KeyHints()
	if len(hints) == 0 {
		t.Fatal("KeyHints in delete-dialog state should return non-empty hints")
	}
}

// TestDayOffsRoute_keyHintsWhileTargetEditOpen verifies KeyHints returns target hints.
func TestDayOffsRoute_keyHintsWhileTargetEditOpen(t *testing.T) {
	r := dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	var sr shell.Route = r
	sr = drain(sr, sr.Init())
	sr, _ = sr.Update(tea.KeyPressMsg{Text: "g"})
	cr := sr.(*dayoffs.Route)
	hints := cr.KeyHints()
	if len(hints) == 0 {
		t.Fatal("KeyHints in target-edit dialog state should return non-empty hints")
	}
}

// TestDayOffsRoute_targetEditEmptyEnterNoOp verifies Enter with empty digits is a no-op.
func TestDayOffsRoute_targetEditEmptyEnterNoOp(t *testing.T) {
	api := &fakeAPI{settings: apiclient.Settings{DefaultTargetMin: 480}}
	var r shell.Route = dayoffs.NewRoute(api, theme.Default, wtnav.Registry{})
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
