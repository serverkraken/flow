# Worktime: Startzeit eines laufenden Timers via Palette anpassen — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bei laufendem Timer kann die Startzeit der laufenden Session über `:` → „Startzeit anpassen" korrigiert werden.

**Architecture:** Ein neues optionales Route-Interface `shell.PaletteProvider` lässt die aktive Top-Route kontextuelle Aktionen zur `:`-Palette beisteuern; die Shell sammelt sie beim Öffnen frisch. Die Worktime-„Heute"-Route liefert bei laufendem Timer den Eintrag „Startzeit anpassen", der einen HH:MM-Dialog öffnet, dessen Submit `EditSession(..., stop=nil)` aufruft.

**Tech Stack:** Go, charm.land/bubbletea/v2, charm.land/bubbles/v2/textinput, internes `internal/tui/shell`-Framework, `internal/tui/screen/worktime`.

## Global Constraints

- Sprache der UI-Strings: Deutsch (z.B. „Startzeit anpassen", „abbrechen").
- Banned CLI: kein `grep`/`find`/`tree` — `rg`/`fd` verwenden.
- TDD: jeder Task beginnt mit einem fehlschlagenden Test.
- `make ci` muss am Ende grün sein (lint inkl. `gofumpt`/`staticcheck`, templ, build, Tests, Coverage-Gate).
- Keine neuen Felder/Endpunkte im Server: `usecase.EditSession` + `apiclient.EditSession` existieren bereits und akzeptieren `stop == nil`.
- Bestehende Muster spiegeln: optionale Route-Interfaces wie `InputCapturer`/`FullScreener` in `internal/tui/shell/route.go`; Dialog-Muster (`editState`/`openEdit`/`submitEdit`) in `internal/tui/screen/worktime/dialogs.go`.
- Commit-Trailer für jeden Commit: `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

- `internal/tui/shell/route.go` — neues `PaletteProvider`-Interface (+ Doc-Kommentar).
- `internal/tui/shell/shell.go` — `navEntries`-Feld, `buildPalette()`, `Palette()`-Accessor, `:`-Handler.
- `internal/tui/shell/shell_test.go` — Merge-/No-Provider-/Rebuild-Tests + `paletteStub`/`togglePaletteStub`.
- `internal/tui/screen/worktime/route.go` — `dialogEditStart`-Konstante, `adjust`-Feld, `renderDialog`/`dialogHints`-Zweige, `adjustStartMsg`-Zweig in `Update`.
- `internal/tui/screen/worktime/dialogs.go` — `adjustState`, `openAdjustStart`, `handleAdjustStartKey`, `submitAdjustStart`, `renderAdjustStart`, `handleDialogKey`-Zweig.
- `internal/tui/screen/worktime/palette.go` — `adjustStartMsg` + `PaletteEntries` (neue Datei, fokussiert).
- `internal/tui/screen/worktime/route_test.go` — `fakeAPI` um Start-/Stop-Capture erweitern.
- `internal/tui/screen/worktime/dialogs_adjust_test.go` — Dialog-Tests (neue Datei).
- `internal/tui/screen/worktime/palette_test.go` — PaletteEntries-/Update-Tests (neue Datei).

---

## Task 1: Generischer PaletteProvider-Mechanismus in der Shell

**Files:**
- Modify: `internal/tui/shell/route.go` (Interface ergänzen)
- Modify: `internal/tui/shell/shell.go:21-35` (Feld), `:50-70` (`WithTabs`), `:236-239` (`:`-Handler), Accessoren bei `:82-87`
- Test: `internal/tui/shell/shell_test.go`

**Interfaces:**
- Produces: `shell.PaletteProvider interface{ PaletteEntries() []PaletteEntry }`; `func (s Shell) Palette() Palette`; internes `func (s Shell) buildPalette() Palette`.
- Consumes: vorhandene `PaletteEntry`, `NewPalette`, `tabSwitchMsg`.

- [ ] **Step 1: Failing test — Provider-Aktionen erscheinen vor Tab-Einträgen**

In `internal/tui/shell/shell_test.go` ergänzen (am Dateiende; `stubRoute` liegt in `route_test.go`, selbes Package `shell_test`):

```go
type paletteStub struct {
	stubRoute
	actions []shell.PaletteEntry
}

func (p paletteStub) PaletteEntries() []shell.PaletteEntry { return p.actions }

func TestShell_paletteMergesProviderEntriesFirst(t *testing.T) {
	action := shell.PaletteEntry{Label: "Startzeit anpassen", Action: func() tea.Msg { return nil }}
	provider := paletteStub{stubRoute{title: "Worktime"}, []shell.PaletteEntry{action}}
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{provider, stubRoute{title: "Docs"}})

	next, _ := s.Update(tea.KeyPressMsg{Text: ":"})
	f := next.(shell.Shell).Palette().Filtered()
	if len(f) != 3 {
		t.Fatalf("want 3 entries (1 action + 2 tabs), got %d: %v", len(f), f)
	}
	if f[0].Label != "Startzeit anpassen" {
		t.Fatalf("action should be first, got %q", f[0].Label)
	}
}

func TestShell_paletteWithoutProvider(t *testing.T) {
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{
		stubRoute{title: "Home"}, stubRoute{title: "Docs"},
	})
	next, _ := s.Update(tea.KeyPressMsg{Text: ":"})
	if f := next.(shell.Shell).Palette().Filtered(); len(f) != 2 {
		t.Fatalf("want 2 nav entries, got %d", len(f))
	}
}

type dynPaletteStub struct {
	stubRoute
	mk func() []shell.PaletteEntry
}

func (d dynPaletteStub) PaletteEntries() []shell.PaletteEntry { return d.mk() }

func TestShell_paletteRebuiltOnEachOpen(t *testing.T) {
	on := false
	pp := dynPaletteStub{stubRoute{title: "Worktime"}, func() []shell.PaletteEntry {
		if !on {
			return nil
		}
		return []shell.PaletteEntry{{Label: "Aktion", Action: func() tea.Msg { return nil }}}
	}}
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{pp})

	// First open while idle: only the single nav entry.
	n1, _ := s.Update(tea.KeyPressMsg{Text: ":"})
	if got := len(n1.(shell.Shell).Palette().Filtered()); got != 1 {
		t.Fatalf("idle: want 1 nav entry, got %d", got)
	}
	// Dismiss, flip the flag, reopen: the action now appears -> 2 entries.
	sh := n1.(shell.Shell)
	_, cmd := sh.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	closed, _ := sh.Update(cmd())
	on = true
	n2, _ := closed.(shell.Shell).Update(tea.KeyPressMsg{Text: ":"})
	if got := len(n2.(shell.Shell).Palette().Filtered()); got != 2 {
		t.Fatalf("after toggle: want 2 entries, got %d", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/shell/ -run TestShell_palette -v`
Expected: Compile-Fehler — `Palette` (Methode) und `PaletteProvider` existieren noch nicht.

- [ ] **Step 3: PaletteProvider-Interface ergänzen**

In `internal/tui/shell/route.go` nach `BreadcrumbHider` (um Zeile 84):

```go
// PaletteProvider lets the active tab's top route contribute contextual action
// entries to the :-palette, gathered fresh each time the palette opens so they
// reflect current route state (e.g. "Startzeit anpassen" only while a timer
// runs). Optional — routes that don't implement it expose only the static
// tab-navigation entries.
type PaletteProvider interface{ PaletteEntries() []PaletteEntry }
```

- [ ] **Step 4: Shell-Feld + buildPalette + Accessor + Handler**

In `internal/tui/shell/shell.go`, Struct `Shell` (um Zeile 25) das Feld ergänzen:

```go
	palette     Palette
	navEntries  []PaletteEntry
	paletteOpen bool
	helpOpen    bool
```

In `WithTabs` (Zeile 57-70) die Zeile `s.palette = NewPalette(entries)` ersetzen durch `s.navEntries = entries`:

```go
func (s Shell) WithTabs(routes []Route) Shell {
	s.tabs = make([]*NavStack, len(routes))
	entries := make([]PaletteEntry, len(routes))
	for i, r := range routes {
		s.tabs[i] = NewNavStack(r)
		idx := i
		entries[i] = PaletteEntry{Label: r.Title(), Action: func() tea.Msg { return tabSwitchMsg(idx) }}
	}
	s.navEntries = entries
	if s.activeTab >= len(s.tabs) {
		s.activeTab = 0
	}
	return s
}
```

Bei den Accessoren (nach `PaletteOpen`, um Zeile 86) ergänzen:

```go
// Palette returns the current palette model (used by tests to inspect merged
// entries after the palette is opened).
func (s Shell) Palette() Palette { return s.palette }

// buildPalette gathers the active route's contextual actions (if it implements
// PaletteProvider) ahead of the static tab-navigation entries, fresh on each
// open so the action set reflects current route state.
func (s Shell) buildPalette() Palette {
	var entries []PaletteEntry
	if pp, ok := s.tabs[s.activeTab].Top().(PaletteProvider); ok {
		entries = append(entries, pp.PaletteEntries()...)
	}
	entries = append(entries, s.navEntries...)
	return NewPalette(entries)
}
```

Im `:`-Handler (`handleKey`, Zeile 236-239) `Reset()` durch `buildPalette()` ersetzen:

```go
	case k.Text == ":":
		s.paletteOpen = true
		s.palette = s.buildPalette()
		return s, nil
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/shell/ -run TestShell_palette -v`
Expected: PASS (alle drei).

- [ ] **Step 6: Full package + lint**

Run: `go test ./internal/tui/shell/... && gofumpt -l internal/tui/shell/`
Expected: Tests PASS, `gofumpt -l` gibt keine Datei aus.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/shell/route.go internal/tui/shell/shell.go internal/tui/shell/shell_test.go
git commit -m "feat(tui): PaletteProvider lets routes add contextual palette actions

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: dialogEditStart-Dialog in der Worktime-Route

**Files:**
- Modify: `internal/tui/screen/worktime/route.go:32-39` (dialogKind), `:50-67` (Feld), `renderDialog`/`dialogHints` via dialogs.go
- Modify: `internal/tui/screen/worktime/dialogs.go` (Dialog-Funktionen, `handleDialogKey`, `renderDialog`, `dialogHints`)
- Modify: `internal/tui/screen/worktime/route_test.go:22-49` (fakeAPI um Capture erweitern)
- Test: `internal/tui/screen/worktime/dialogs_adjust_test.go` (neu)

**Interfaces:**
- Consumes: vorhandene `r.st.Running`, `r.st.Active *time.Time`, `r.st.ActiveID`, `r.api.EditSession`, `wtfmt.ParseHM`, `form.NewTextInput`, `toast.NewDanger`, `r.now`.
- Produces: `dialogEditStart dialogKind`; `adjustState{id string; date time.Time; input textinput.Model}`; `func (r *TodayRoute) openAdjustStart() (shell.Route, tea.Cmd)`; `func (r *TodayRoute) submitAdjustStart() tea.Cmd`; `func (r *TodayRoute) handleAdjustStartKey(tea.KeyPressMsg) (shell.Route, tea.Cmd)`.

- [ ] **Step 1: fakeAPI um Start-/Stop-Capture erweitern**

In `internal/tui/screen/worktime/route_test.go` die Struct (Zeile 22-30) und `EditSession` (Zeile 46-49) ändern:

```go
type fakeAPI struct {
	today     apiclient.Today
	sessions  []domain.WorkSession
	projects  []domain.Project
	started   bool
	stopped   [2]string
	edited    string
	editStart time.Time
	editStop  *time.Time
	deleted   string
}
```

```go
func (f *fakeAPI) EditSession(_ context.Context, id string, _ *string, _, _ string, start time.Time, stop *time.Time) (domain.WorkSession, error) {
	f.edited = id
	f.editStart = start
	f.editStop = stop
	return domain.WorkSession{ID: id}, nil
}
```

- [ ] **Step 2: Failing test — Submit ruft EditSession mit stop=nil**

Neue Datei `internal/tui/screen/worktime/dialogs_adjust_test.go`:

```go
package worktime

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// makeRunningRoute builds a TodayRoute with a running session started at
// 09:00 UTC on 2026-06-14 (fixedNow is 12:00 UTC that day) and the
// adjust-start dialog already open.
func makeRunningRoute(t *testing.T, f *fakeAPI) *TodayRoute {
	t.Helper()
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	r := NewTodayRoute(f, fixedNow, theme.Default, nil)
	r.st.Running = true
	r.st.Active = &start
	r.st.ActiveID = "run1"
	res, _ := r.openAdjustStart()
	dr := res.(*TodayRoute)
	if dr.dialog != dialogEditStart {
		t.Fatal("could not open adjust-start dialog")
	}
	return dr
}

func TestOpenAdjustStart_prefillsCurrentStart(t *testing.T) {
	r := makeRunningRoute(t, &fakeAPI{})
	if got := r.adjust.input.Value(); got != "09:00" {
		t.Fatalf("prefill = %q want 09:00", got)
	}
}

func TestSubmitAdjustStart_validCallsEditWithNilStop(t *testing.T) {
	f := &fakeAPI{}
	r := makeRunningRoute(t, f)
	r.adjust.input.SetValue("08:30")

	cmd := r.submitAdjustStart()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	if _, ok := cmd().(reloadMsg); !ok {
		t.Fatal("valid submit should yield reloadMsg")
	}
	if f.edited != "run1" {
		t.Fatalf("EditSession not called for run1: %q", f.edited)
	}
	if f.editStop != nil {
		t.Fatal("stop must be nil so the session keeps running")
	}
	want := time.Date(2026, 6, 14, 8, 30, 0, 0, time.UTC)
	if !f.editStart.Equal(want) {
		t.Fatalf("start = %v want %v", f.editStart, want)
	}
	if r.dialog != dialogNone {
		t.Fatal("dialog should close after a valid submit")
	}
}

func TestSubmitAdjustStart_invalidKeepsDialogNoCall(t *testing.T) {
	f := &fakeAPI{}
	r := makeRunningRoute(t, f)
	r.adjust.input.SetValue("99:99")

	r.submitAdjustStart()
	if f.edited != "" {
		t.Fatal("invalid HH:MM must not call EditSession")
	}
	if r.dialog != dialogEditStart {
		t.Fatal("dialog should stay open on invalid input")
	}
}

func TestSubmitAdjustStart_futureNoCall(t *testing.T) {
	f := &fakeAPI{}
	r := makeRunningRoute(t, f)
	r.adjust.input.SetValue("13:00") // fixedNow is 12:00 UTC

	r.submitAdjustStart()
	if f.edited != "" {
		t.Fatal("future start must not call EditSession")
	}
}

func TestHandleAdjustStartKey_EscCancels(t *testing.T) {
	r := makeRunningRoute(t, &fakeAPI{})
	res, _ := r.handleAdjustStartKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if res.(*TodayRoute).dialog != dialogNone {
		t.Fatal("esc should close the dialog")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/tui/screen/worktime/ -run AdjustStart -v`
Expected: Compile-Fehler — `dialogEditStart`, `openAdjustStart`, `adjust`, `submitAdjustStart`, `handleAdjustStartKey` existieren noch nicht.

- [ ] **Step 4: dialogEditStart-Konstante + adjust-Feld**

In `internal/tui/screen/worktime/route.go`, `dialogKind`-Block (Zeile 34-39):

```go
const (
	dialogNone dialogKind = iota
	dialogBooking
	dialogEdit
	dialogDelete
	dialogEditStart
)
```

In Struct `TodayRoute` (Zeile 63-66) das Feld ergänzen:

```go
	dialog  dialogKind
	booking bookingState
	edit    editState
	adjust  adjustState
	confirm confirmState
```

- [ ] **Step 5: Dialog-Funktionen in dialogs.go**

In `internal/tui/screen/worktime/dialogs.go` ergänzen (nach `submitEdit`, vor `confirmState`):

```go
type adjustState struct {
	id    string
	date  time.Time
	input textinput.Model
}

// openAdjustStart opens the start-edit dialog for the *running* session,
// prefilled with its current start time. Reached via the ":" palette entry.
func (r *TodayRoute) openAdjustStart() (shell.Route, tea.Cmd) {
	if !r.st.Running || r.st.Active == nil {
		return r, nil
	}
	in := form.NewTextInput("HH:MM", r.pal)
	in.SetValue(r.st.Active.Format("15:04"))
	cmd := in.Focus()
	r.adjust = adjustState{id: r.st.ActiveID, date: *r.st.Active, input: in}
	r.dialog = dialogEditStart
	return r, cmd
}

func (r *TodayRoute) handleAdjustStartKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch k.Code {
	case tea.KeyEsc:
		r.dialog = dialogNone
		return r, nil
	case tea.KeyEnter:
		return r, r.submitAdjustStart()
	}
	var cmd tea.Cmd
	r.adjust.input, cmd = r.adjust.input.Update(k)
	return r, cmd
}

// submitAdjustStart validates the HH:MM field and, on success, edits the
// running session's start time with stop=nil so it keeps running.
func (r *TodayRoute) submitAdjustStart() tea.Cmd {
	startD, err := wtfmt.ParseHM(strings.TrimSpace(r.adjust.input.Value()))
	if err != nil {
		r.toast = toast.NewDanger("Start ungültig (HH:MM)", r.pal)
		return r.toast.Init()
	}
	d := r.adjust.date
	base := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
	startTime := base.Add(startD)
	if startTime.After(r.now()) {
		r.toast = toast.NewDanger("Start liegt in der Zukunft", r.pal)
		return r.toast.Init()
	}
	id := r.adjust.id
	api := r.api
	r.dialog = dialogNone
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := api.EditSession(ctx, id, nil, "", "", startTime, nil); err != nil {
			return loadedMsg{err: err}
		}
		return reloadMsg{}
	}
}

func (r *TodayRoute) renderAdjustStart(f shell.Frame) string {
	var b strings.Builder
	b.WriteString("\n  Startzeit anpassen (enter speichert · esc bricht ab)\n\n")
	fmt.Fprintf(&b, "  %-6s %s\n", "Start", r.adjust.input.View())
	return b.String()
}
```

- [ ] **Step 6: handleDialogKey / renderDialog / dialogHints verdrahten**

In `handleDialogKey` (Zeile 61-73) den Zweig ergänzen:

```go
	case dialogEdit:
		return r.handleEditKey(k)
	case dialogEditStart:
		return r.handleAdjustStartKey(k)
	case dialogDelete:
```

In `renderDialog` (Zeile 221-231):

```go
	case dialogEdit:
		return r.renderEdit(f)
	case dialogEditStart:
		return r.renderAdjustStart(f)
	case dialogDelete:
```

In `dialogHints` (Zeile 252-262):

```go
	case dialogEdit:
		return []keyhint.Hint{{Key: "tab", Desc: "Feld"}, {Key: "enter", Desc: "speichern"}, {Key: "esc", Desc: "abbrechen"}}
	case dialogEditStart:
		return []keyhint.Hint{{Key: "enter", Desc: "speichern"}, {Key: "esc", Desc: "abbrechen"}}
	case dialogDelete:
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/tui/screen/worktime/ -run AdjustStart -v`
Expected: PASS (alle Adjust-Tests).

- [ ] **Step 8: Full package**

Run: `go test ./internal/tui/screen/worktime/... && gofumpt -l internal/tui/screen/worktime/`
Expected: PASS, keine gofumpt-Ausgabe.

- [ ] **Step 9: Commit**

```bash
git add internal/tui/screen/worktime/route.go internal/tui/screen/worktime/dialogs.go internal/tui/screen/worktime/route_test.go internal/tui/screen/worktime/dialogs_adjust_test.go
git commit -m "feat(worktime): dialogEditStart edits a running session's start time

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: PaletteEntries + adjustStartMsg verdrahten

**Files:**
- Create: `internal/tui/screen/worktime/palette.go`
- Modify: `internal/tui/screen/worktime/route.go:110-164` (Update-Switch)
- Test: `internal/tui/screen/worktime/palette_test.go` (neu)

**Interfaces:**
- Consumes: `openAdjustStart` (Task 2), `r.st.Running`, `r.st.Active`, `shell.PaletteEntry`.
- Produces: `adjustStartMsg struct{}`; `func (r *TodayRoute) PaletteEntries() []shell.PaletteEntry` (erfüllt `shell.PaletteProvider`).

- [ ] **Step 1: Failing test — PaletteEntries nur bei laufendem Timer + öffnet Dialog**

Neue Datei `internal/tui/screen/worktime/palette_test.go`:

```go
package worktime

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestPaletteEntries_onlyWhenRunning(t *testing.T) {
	r := NewTodayRoute(&fakeAPI{}, fixedNow, theme.Default, nil)
	if r.PaletteEntries() != nil {
		t.Fatal("no palette entry while idle")
	}
	start := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	r.st.Running = true
	r.st.Active = &start
	r.st.ActiveID = "run1"

	e := r.PaletteEntries()
	if len(e) != 1 || e[0].Label != "Startzeit anpassen" {
		t.Fatalf("want 1 entry 'Startzeit anpassen', got %v", e)
	}
	if _, ok := e[0].Action().(adjustStartMsg); !ok {
		t.Fatal("entry action should yield adjustStartMsg")
	}
}

func TestUpdate_adjustStartMsgOpensDialog(t *testing.T) {
	start := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	r := NewTodayRoute(&fakeAPI{}, fixedNow, theme.Default, nil)
	r.st.Running = true
	r.st.Active = &start
	r.st.ActiveID = "run1"

	res, _ := r.Update(adjustStartMsg{})
	dr := res.(*TodayRoute)
	if dr.dialog != dialogEditStart {
		t.Fatalf("dialog = %v want dialogEditStart", dr.dialog)
	}
	if got := dr.adjust.input.Value(); got != "10:00" {
		t.Fatalf("prefill = %q want 10:00", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/screen/worktime/ -run "PaletteEntries|adjustStartMsg" -v`
Expected: Compile-Fehler — `PaletteEntries` und `adjustStartMsg` existieren noch nicht.

- [ ] **Step 3: palette.go anlegen**

Neue Datei `internal/tui/screen/worktime/palette.go`:

```go
package worktime

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
)

// adjustStartMsg is emitted by the "Startzeit anpassen" palette entry and
// handled in TodayRoute.Update to open the start-edit dialog for the running
// session.
type adjustStartMsg struct{}

// PaletteEntries implements shell.PaletteProvider: while a timer runs, the
// ":"-palette offers "Startzeit anpassen" to correct the running session's
// start time.
func (r *TodayRoute) PaletteEntries() []shell.PaletteEntry {
	if !r.st.Running || r.st.Active == nil {
		return nil
	}
	return []shell.PaletteEntry{{
		Label:  "Startzeit anpassen",
		Action: func() tea.Msg { return adjustStartMsg{} },
	}}
}
```

- [ ] **Step 4: adjustStartMsg im Update-Switch behandeln**

In `internal/tui/screen/worktime/route.go`, `Update` (im `switch m := msg.(type)`, z.B. vor `case tea.KeyPressMsg:` bei Zeile 161):

```go
	case adjustStartMsg:
		return r.openAdjustStart()
	case tea.KeyPressMsg:
		return r.handleKey(m)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/screen/worktime/ -run "PaletteEntries|adjustStartMsg" -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/screen/worktime/palette.go internal/tui/screen/worktime/route.go internal/tui/screen/worktime/palette_test.go
git commit -m "feat(worktime): expose 'Startzeit anpassen' as a running-timer palette action

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 4: Verifikation (make ci + Dogfood)

**Files:** keine — reine Verifikation.

- [ ] **Step 1: Volles CI**

Run: `make ci`
Expected: grün — lint (`gofumpt`/`staticcheck`), templ, build, alle Tests, Coverage-Gate erfüllt.

- [ ] **Step 2: Manuelles Dogfood gegen Dev-Stack**

```bash
make dev-up          # Postgres + Dex
FLOW_DEV=1 make dev-run   # bzw. wie in reference_flow_dev_env beschrieben
```

Im TUI (`flow ui`):
- Timer starten (`s`).
- `:` öffnen → „Startzeit anpassen" ist gelistet (oben, vor den Tabs).
- Eintrag wählen → HH:MM-Dialog vorbefüllt mit aktueller Startzeit.
- Startzeit nach vorn korrigieren (frühere Uhrzeit) → bestätigen → Live-Tick/„Logged"-Anzeige steigt entsprechend, Timer läuft weiter.
- `:` ohne laufenden Timer → „Startzeit anpassen" fehlt.
- Ungültiges HH:MM und Zukunfts-Uhrzeit → jeweils Danger-Toast, kein Speichern.
- Startzeit vor das Ende einer früheren Session legen → Overlap-Toast vom Server.

Expected: alle Punkte erfüllt.

- [ ] **Step 3: Memory-Update (optional)**

Nach erfolgreichem Dogfood `project_flow_rebuild_*`-Memory bzw. `feedback_tui_palette_contextual_commands` aktualisieren (Status: kontextuelle Palette umgesetzt).

---

## Self-Review

**Spec-Abdeckung:** PaletteProvider-Mechanismus → Task 1. „Startzeit anpassen"-Eintrag bei laufendem Timer → Task 3. HH:MM-Dialog + EditSession(stop=nil) → Task 2. Edge Cases (kein Timer / ungültig / Zukunft / Overlap) → Task 2/3-Tests + Task 4-Dogfood. Done-Gate → Task 4. Keine Spec-Anforderung ohne Task.

**Platzhalter:** keine — jeder Code-Step zeigt vollständigen Code, jeder Run-Step ein erwartetes Ergebnis.

**Typ-Konsistenz:** `adjustState{id,date,input}` einheitlich in Task 2 definiert und in Task 2/3 verwendet; `adjustStartMsg` in Task 3 erzeugt (PaletteEntries) und konsumiert (Update); `PaletteProvider.PaletteEntries()` Signatur identisch in route.go (Task 1) und palette.go (Task 3); `fakeAPI.editStop`/`editStart` in Task 2 eingeführt und im Test geprüft.
