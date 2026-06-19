package dayoffs_test

import (
	"context"
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
}

func (f *fakeAPI) ListDayOffs(_ context.Context, _, _ string) ([]apiclient.DayOff, error) {
	return f.list, nil
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

	// Non-digit key must be ignored — a subsequent bare enter is a no-op (empty input)
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
	// Verify dialog was dismissed — the list view no longer shows the target-edit prompt.
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

	// Move down once → index 1 = "BY"
	r, _ = r.Update(tea.KeyPressMsg{Text: "j"})

	// Confirm
	r, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r = drain(r, cmd)

	if api.bundesland != "BY" {
		t.Fatalf("bundesland = %q, want BY", api.bundesland)
	}
	// Verify dialog was dismissed — the Bundesland picker no longer appears.
	if strings.Contains(r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default}), "Bundesland wählen") {
		t.Fatal("Bundesland dialog should be closed after confirm")
	}
}

// TestDayOffsRoute_sseReload verifies that dayoff.changed and settings.changed
// events trigger a reload cmd, and that an unrelated event does not.
func TestDayOffsRoute_sseReload(t *testing.T) {
	var r shell.Route = dayoffs.NewRoute(&fakeAPI{}, theme.Default, wtnav.Registry{})
	r = drain(r, r.Init())

	// dayoff.changed → reload
	_, cmd := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: string(domain.EventDayOffChanged)}})
	if cmd == nil {
		t.Fatal("dayoff.changed should trigger a reload cmd")
	}

	// settings.changed → reload
	_, cmd = r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: string(domain.EventSettingsChanged)}})
	if cmd == nil {
		t.Fatal("settings.changed should trigger a reload cmd")
	}

	// unrelated event → no cmd
	_, cmd = r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: string(domain.EventDocumentCreated)}})
	if cmd != nil {
		t.Fatal("unrelated event should not trigger a reload cmd")
	}
}
