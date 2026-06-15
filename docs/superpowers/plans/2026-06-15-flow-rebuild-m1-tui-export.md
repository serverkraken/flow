# flow Rebuild M1 — TUI-Export-Affordance · Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ein interaktives Export-Overlay in der Worktime-TUI (`flow worktime` → Taste `e`) — Range-Presets + freie von/bis, Format csv/json/md, editierbarer Zielpfad → schreibt den vom Server gelieferten Export in eine Datei.

**Architecture:** Neues Overlay als Methoden auf dem bestehenden `tui.Model` (Muster `dayoffs.go`/`stats.go`): pure Helfer (Preset→Range, Default-Pfad, `~`-Expansion, Cycle) in `export.go`, Model-State + Open/Close-Wiring in `worktime.go`, Panel-Key-Handling + View + asynchroner `exportCmd` in `export.go`. Nutzt das bestehende `apiclient.Export(ctx, from, to, format, projectID)`. Keine Server-/REST-/Composition-Root-Änderung.

**Tech Stack:** Go, Bubbletea v2 (`charm.land/bubbletea/v2`), Lipgloss v2, `internal/adapter/apiclient`.

---

## Worktree & Branch

**Alle Code-Tasks im Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild` auf Branch `rebuild`** (HEAD aktuell `f0c1597`, nach M1e + Cleanup). Plan-/Spec-Docs auf `main` — **nicht** ins `rebuild` committen. Modulpfad `github.com/serverkraken/flow`. Kleine fokussierte Commits pro Task; am Ende `make ci` grün inkl. Coverage-Gate **≥80%**.

## Bestehender Kontext (verifiziert)

- `tui.Model` (`internal/tui/worktime.go`): hält `client *apiclient.Client`, `now time.Time` (gesetzt in `New`, fortgeschrieben per `tickMsg`). Overlays per Bool-Flag (`showWeek`/`showStats`/`showDayOffs`) + Tasten (`s`/`x`/`d`/`w`/`t`, Esc schließt, `q`/Ctrl-C quit). `handleKey` dispatcht; `View()` rendert je Flag eine Sub-View.
- Text-Edit-Muster (`dayoffs.go` `handleDayOffKey`): `k.Code == tea.KeyBackspace` → letztes Rune weg; `k.Text != ""` → anhängen; `k.Code == tea.KeyEsc`/`tea.KeyEnter`.
- Async-Cmd-Muster (`reload`/`startCmd`): `func() tea.Msg { … client.X(ctx) … return xMsg{} | errMsg{err} }`; `m.client == nil` → `return nil` (Test-Guard).
- Styles (`styles.go`): `styleHeader`, `styleMuted`, `styleSel`, `styleErr`, `styleOk`, `styleRunning`. Keine farbigen Emoji (Projektregel); Monospace-Glyphen ok.
- `apiclient.Client.Export(ctx, from, to, format, projectID string) ([]byte, error)` liefert die rohen Datei-Bytes (non-200 → error).
- Test-Muster (`worktime_test.go`): Model-zentriert (`New(nil, …)` + `m.Update(...)`), plus `newFakeSrv`-Stil (`httptest` + `apiclient.New(srv.URL, "tok")`) für Cmd-Tests.

## File Structure

**Neu:**
- `internal/tui/export.go` — Export-Overlay: pure Helfer (`exportPresetRange`, `defaultExportPath`, `expandHome`, `cycleFormat`, `cyclePreset`), `handleExportKey`, `exportView`, `exportCmd`, Msg-Typen.
- `internal/tui/export_test.go` — Tests für Helfer + Key-Handling + Cmd.

**Geändert:**
- `internal/tui/worktime.go` — `Model`-Export-Felder; `e`-Open + Esc-Close + Dispatch in `handleKey`; `exportDoneMsg`/`exportErrMsg` in `Update`; `View()`-Dispatch auf `exportView`; Footer-Hinweis `e export`.

---

## Task 1: Pure Helfer — Preset-Range, Default-Pfad, Home-Expansion, Cycle

Reine Funktionen, voll testbar (hoher Coverage-Wert, keine Bubbletea-Abhängigkeit).

**Files:**
- Create: `internal/tui/export.go`
- Test: `internal/tui/export_test.go`

- [ ] **Step 1: Failing test schreiben**

Create `internal/tui/export_test.go`:

```go
package tui

import (
	"strings"
	"testing"
	"time"
)

func TestExportPresetRange(t *testing.T) {
	// Montag 2026-06-15 als "now".
	now := time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)
	cases := []struct {
		preset, from, to string
	}{
		{"monat", "2026-06-01", "2026-06-15"},
		{"kw", "2026-06-15", "2026-06-15"},      // Montag → from==today
		{"letzter", "2026-05-01", "2026-05-31"}, // ganzer Vormonat
	}
	for _, c := range cases {
		from, to := exportPresetRange(c.preset, now)
		if from != c.from || to != c.to {
			t.Errorf("%s: got %s..%s want %s..%s", c.preset, from, to, c.from, c.to)
		}
	}
}

func TestExportPresetRange_KWMidweek(t *testing.T) {
	// Mittwoch 2026-06-17 → KW-Start Montag 2026-06-15.
	now := time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)
	from, to := exportPresetRange("kw", now)
	if from != "2026-06-15" || to != "2026-06-17" {
		t.Errorf("kw midweek: got %s..%s want 2026-06-15..2026-06-17", from, to)
	}
}

func TestDefaultExportPath(t *testing.T) {
	got := defaultExportPath("2026-06-01", "2026-06-30", "md")
	if got != "~/Downloads/flow-export-2026-06-01_2026-06-30.md" {
		t.Errorf("got %q", got)
	}
	if g := defaultExportPath("2026-06-01", "2026-06-30", "csv"); !strings.HasSuffix(g, ".csv") {
		t.Errorf("csv ext: got %q", g)
	}
}

func TestExpandHome(t *testing.T) {
	if got := expandHome("/tmp/x.csv"); got != "/tmp/x.csv" {
		t.Errorf("absolute path must pass through: %q", got)
	}
	got := expandHome("~/Downloads/x.csv")
	if strings.HasPrefix(got, "~") || !strings.HasSuffix(got, "/Downloads/x.csv") {
		t.Errorf("~ not expanded: %q", got)
	}
}

func TestCycleFormatAndPreset(t *testing.T) {
	if cycleFormat("csv", +1) != "json" || cycleFormat("json", +1) != "md" || cycleFormat("md", +1) != "csv" {
		t.Error("cycleFormat forward wrong")
	}
	if cycleFormat("csv", -1) != "md" {
		t.Error("cycleFormat backward wrong")
	}
	if cyclePreset("kw", +1) != "monat" || cyclePreset("monat", +1) != "letzter" || cyclePreset("letzter", +1) != "custom" || cyclePreset("custom", +1) != "kw" {
		t.Error("cyclePreset forward wrong")
	}
}
```

- [ ] **Step 2: Run — FAIL**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/ -run 'TestExportPresetRange|TestDefaultExportPath|TestExpandHome|TestCycleFormatAndPreset'`
Expected: FAIL (undefined).

- [ ] **Step 3: Helfer implementieren**

Create `internal/tui/export.go`:

```go
package tui

import (
	"os"
	"path/filepath"
	"time"
)

const dayFmtTUI = "2006-01-02"

// exportPresetRange maps a preset name + "now" to an inclusive [from,to] date
// range as yyyy-mm-dd strings (in now's location):
//   - "monat":   first of current month → today
//   - "kw":      Monday of current week → today
//   - "letzter": first → last day of the previous month
//   - anything else (incl. "custom"): today → today (caller overrides for custom)
func exportPresetRange(preset string, now time.Time) (string, string) {
	y, mo, d := now.Date()
	loc := now.Location()
	today := time.Date(y, mo, d, 0, 0, 0, 0, loc)
	switch preset {
	case "monat":
		from := time.Date(y, mo, 1, 0, 0, 0, 0, loc)
		return from.Format(dayFmtTUI), today.Format(dayFmtTUI)
	case "kw":
		// Monday=0 offset: Go Sunday=0..Saturday=6 → days since Monday.
		off := (int(today.Weekday()) + 6) % 7
		from := today.AddDate(0, 0, -off)
		return from.Format(dayFmtTUI), today.Format(dayFmtTUI)
	case "letzter":
		firstThis := time.Date(y, mo, 1, 0, 0, 0, 0, loc)
		lastPrev := firstThis.AddDate(0, 0, -1)
		firstPrev := time.Date(lastPrev.Year(), lastPrev.Month(), 1, 0, 0, 0, 0, loc)
		return firstPrev.Format(dayFmtTUI), lastPrev.Format(dayFmtTUI)
	default:
		return today.Format(dayFmtTUI), today.Format(dayFmtTUI)
	}
}

// defaultExportPath builds the suggested target path. format is also the ext.
func defaultExportPath(from, to, format string) string {
	return "~/Downloads/flow-export-" + from + "_" + to + "." + format
}

// expandHome resolves a leading "~/" against the user's home directory.
func expandHome(path string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// cycleFormat steps through csv → json → md (dir +1) or reverse (dir -1).
func cycleFormat(f string, dir int) string {
	order := []string{"csv", "json", "md"}
	return cycle(order, f, dir)
}

// cyclePreset steps through kw → monat → letzter → custom.
func cyclePreset(p string, dir int) string {
	order := []string{"kw", "monat", "letzter", "custom"}
	return cycle(order, p, dir)
}

func cycle(order []string, cur string, dir int) string {
	idx := 0
	for i, v := range order {
		if v == cur {
			idx = i
			break
		}
	}
	n := (idx + dir + len(order)) % len(order)
	return order[n]
}
```

- [ ] **Step 4: Run — PASS**

Run: `go test ./internal/tui/ -run 'TestExportPresetRange|TestDefaultExportPath|TestExpandHome|TestCycleFormatAndPreset'`
Expected: PASS. Auch `go vet ./internal/tui/` + `gofmt -l internal/tui/` (leer).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/export.go internal/tui/export_test.go
git commit -m "feat(tui-export): pure preset-range/path/cycle helpers"
git rev-parse --short HEAD
```

---

## Task 2: Model-State + Open/Close + View-Dispatch + Footer

**Files:**
- Modify: `internal/tui/worktime.go`
- Modify: `internal/tui/export.go` (add `exportView` + open helper)
- Test: `internal/tui/export_test.go`

- [ ] **Step 1: Failing test schreiben**

Add to `internal/tui/export_test.go` (stelle sicher, dass `tea "charm.land/bubbletea/v2"` in der Importliste der Datei steht — die Helfer-Tests aus Task 1 brauchten es noch nicht):

```go
func TestExportOpenSetsDefaults(t *testing.T) {
	m := New(nil, "tester")
	m.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) // Montag
	next, _ := m.Update(tea.KeyPressMsg{Text: "e"})
	mm := next.(Model)
	if !mm.showExport {
		t.Fatal("e should open the export overlay")
	}
	if mm.expFormat != "md" {
		t.Errorf("default format md, got %q", mm.expFormat)
	}
	if mm.expPreset != "monat" {
		t.Errorf("default preset monat, got %q", mm.expPreset)
	}
	if mm.expFrom != "2026-06-01" || mm.expTo != "2026-06-15" {
		t.Errorf("default range got %s..%s", mm.expFrom, mm.expTo)
	}
	if mm.expPath != "~/Downloads/flow-export-2026-06-01_2026-06-15.md" {
		t.Errorf("default path got %q", mm.expPath)
	}
}

func TestExportEscCloses(t *testing.T) {
	m := New(nil, "tester")
	next, _ := m.Update(tea.KeyPressMsg{Text: "e"})
	m = next.(Model)
	next2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if next2.(Model).showExport {
		t.Fatal("esc should close the export overlay")
	}
}

func TestExportViewRenders(t *testing.T) {
	m := New(nil, "tester")
	m.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	next, _ := m.Update(tea.KeyPressMsg{Text: "e"})
	out := next.(Model).View().Content
	for _, want := range []string{"Export", "Format", "2026-06-01", "md"} {
		if !strings.Contains(out, want) {
			t.Errorf("export view missing %q:\n%s", want, out)
		}
	}
}

func TestMainViewFooterHasExportHint(t *testing.T) {
	m := New(nil, "tester")
	if !strings.Contains(m.View().Content, "e export") {
		t.Errorf("main footer missing 'e export':\n%s", m.View().Content)
	}
}
```

- [ ] **Step 2: Run — FAIL**

Run: `go test ./internal/tui/ -run 'TestExportOpen|TestExportEsc|TestExportViewRenders|TestMainViewFooter'`
Expected: FAIL (Felder/Methoden undefined).

- [ ] **Step 3: Model-Felder + Open-Helper**

In `internal/tui/worktime.go`, add to the `Model` struct (nach dem `showStats`-Block, vor `today`):

```go
	showExport    bool
	expPreset     string // "kw" | "monat" | "letzter" | "custom"
	expFrom       string // yyyy-mm-dd
	expTo         string // yyyy-mm-dd
	expFormat     string // "csv" | "json" | "md"
	expPath       string
	expPathEdited bool
	expFocus      int // 0=preset 1=from 2=to 3=format 4=path
	expStatus     string
```

In `internal/tui/export.go`, add the open helper + view (append to the file). First add imports `fmt` and `strings` to `export.go`'s import block:

```go
// openExport initialises the export overlay state with sensible defaults
// (current month, markdown) relative to m.now.
func (m Model) openExport() Model {
	m.showExport = true
	m.expPreset = "monat"
	m.expFormat = "md"
	from, to := exportPresetRange("monat", m.now)
	m.expFrom, m.expTo = from, to
	m.expPath = defaultExportPath(from, to, "md")
	m.expPathEdited = false
	m.expFocus = 0
	m.expStatus = ""
	return m
}

// exportView renders the export overlay.
func (m Model) exportView() tea.View {
	var b strings.Builder
	b.WriteString(styleHeader.Render("flow · Export") + "\n\n")
	field := func(idx int, label, val string) {
		cursor := "  "
		render := val
		if m.expFocus == idx {
			cursor = styleSel.Render("▸") + " "
			render = styleSel.Render(val)
		}
		fmt.Fprintf(&b, "%s%-8s %s\n", cursor, label, render)
	}
	field(0, "Range", m.expPreset)
	field(1, "von", m.expFrom)
	field(2, "bis", m.expTo)
	field(3, "Format", m.expFormat)
	field(4, "Pfad", m.expPath)
	b.WriteString("\n")
	if m.expStatus != "" {
		b.WriteString(styleMuted.Render(m.expStatus) + "\n\n")
	}
	b.WriteString(styleMuted.Render("tab Feld · ←/→ wählen · enter export · esc back") + "\n")
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}
```
Add `tea "charm.land/bubbletea/v2"` to `export.go`'s imports (needed for `tea.View` here and key handling in Task 3).

- [ ] **Step 4: Wiring in worktime.go**

In `handleKey`, in the **main** `switch` (the one starting at the `case k.Text == "q"` after the dayoffs block), add a case (place it near the `t` stats case):

```go
	case k.Text == "e":
		return m.openExport(), nil
```

Add Esc-close + dispatch. At the TOP of `handleKey`, after the `if m.booking { … }` block and before the `if m.showWeek || m.showStats` block, add:

```go
	if m.showExport {
		if k.Code == tea.KeyEsc {
			m.showExport = false
			return m, nil
		}
		return m.handleExportKey(k)
	}
```
(`handleExportKey` is defined in Task 3 — for THIS task add a temporary minimal stub at the end of `export.go` so it compiles:)

```go
// handleExportKey is fleshed out in Task 3.
func (m Model) handleExportKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return m, nil
}
```

In `View()`, add a dispatch near the other overlay checks (before the main `var b strings.Builder`):

```go
	if m.showExport {
		return m.exportView()
	}
```

In `View()`'s footer line, extend the hint string to include export:

```go
	b.WriteString(styleMuted.Render("s start · x stop · d dayoffs · w Woche · t Stats · e export · q quit") + "\n")
```

- [ ] **Step 5: Run — PASS**

Run: `go test ./internal/tui/ 2>&1 | tail -8` (alle TUI-Tests), `go build ./...`, `go vet ./internal/tui/`, `gofmt -l internal/tui/` (leer).
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/worktime.go internal/tui/export.go internal/tui/export_test.go
git commit -m "feat(tui-export): overlay open/close + view + footer hint"
git rev-parse --short HEAD
```

---

## Task 3: Panel-Key-Handling — Fokus, Choice-Cycling, Text-Edit

**Files:**
- Modify: `internal/tui/export.go` (`handleExportKey` ausbauen)
- Test: `internal/tui/export_test.go`

- [ ] **Step 1: Failing test schreiben**

Add to `internal/tui/export_test.go`:

```go
func openExportM(t *testing.T) Model {
	t.Helper()
	m := New(nil, "tester")
	m.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) // Montag
	next, _ := m.Update(tea.KeyPressMsg{Text: "e"})
	return next.(Model)
}

func TestExportTabMovesFocus(t *testing.T) {
	m := openExportM(t)
	if m.expFocus != 0 {
		t.Fatalf("start focus 0, got %d", m.expFocus)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if next.(Model).expFocus != 1 {
		t.Fatalf("tab → focus 1, got %d", next.(Model).expFocus)
	}
}

func TestExportPresetCycleUpdatesRange(t *testing.T) {
	m := openExportM(t) // focus 0 = preset, preset=monat
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	mm := next.(Model)
	if mm.expPreset != "letzter" {
		t.Fatalf("right on preset: monat → letzter, got %q", mm.expPreset)
	}
	if mm.expFrom != "2026-05-01" || mm.expTo != "2026-05-31" {
		t.Errorf("letzter range got %s..%s", mm.expFrom, mm.expTo)
	}
	// Pfad folgt dem Range (nicht editiert).
	if !strings.Contains(mm.expPath, "2026-05-01_2026-05-31") {
		t.Errorf("path should follow range, got %q", mm.expPath)
	}
}

func TestExportFormatCycleUpdatesPathExt(t *testing.T) {
	m := openExportM(t)
	// Fokus auf Format (Index 3): drei Tabs weiter.
	for i := 0; i < 3; i++ {
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m = next.(Model)
	}
	if m.expFocus != 3 {
		t.Fatalf("focus should be 3 (format), got %d", m.expFocus)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	mm := next.(Model)
	if mm.expFormat != "csv" { // md →(+1) csv
		t.Fatalf("md →+1 csv, got %q", mm.expFormat)
	}
	if !strings.HasSuffix(mm.expPath, ".csv") {
		t.Errorf("path ext should follow format, got %q", mm.expPath)
	}
}

func TestExportCustomDateEditSetsCustomAndPath(t *testing.T) {
	m := openExportM(t)
	// Fokus auf "von" (Index 1).
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(Model)
	// Lösche das von-Feld komplett, tippe neues Datum.
	for i := 0; i < 10; i++ {
		n, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		m = n.(Model)
	}
	for _, ch := range "2026-06-10" {
		n, _ := m.Update(tea.KeyPressMsg{Text: string(ch)})
		m = n.(Model)
	}
	if m.expFrom != "2026-06-10" {
		t.Fatalf("von edit got %q", m.expFrom)
	}
	if m.expPreset != "custom" {
		t.Errorf("editing date should set preset=custom, got %q", m.expPreset)
	}
}

func TestExportManualPathEditSticks(t *testing.T) {
	m := openExportM(t)
	// Fokus auf Pfad (Index 4): vier Tabs.
	for i := 0; i < 4; i++ {
		n, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m = n.(Model)
	}
	// Tippe ein Zeichen an den Pfad.
	n, _ := m.Update(tea.KeyPressMsg{Text: "X"})
	m = n.(Model)
	if !m.expPathEdited {
		t.Fatal("editing path should set expPathEdited")
	}
	editedPath := m.expPath
	// Format wechseln darf den manuell editierten Pfad NICHT überschreiben.
	for i := 4; i%5 != 3; i++ { // zurück auf Format-Fokus via Tab-Wrap
		n2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m = n2.(Model)
	}
	n3, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = n3.(Model)
	if m.expPath != editedPath {
		t.Errorf("manual path must stick after format change: got %q want %q", m.expPath, editedPath)
	}
}
```

- [ ] **Step 2: Run — FAIL**

Run: `go test ./internal/tui/ -run 'TestExportTab|TestExportPresetCycle|TestExportFormatCycle|TestExportCustomDate|TestExportManualPath'`
Expected: FAIL (stub `handleExportKey` no-ops).

- [ ] **Step 3: `handleExportKey` ausbauen**

Replace the stub `handleExportKey` in `export.go` with:

```go
// handleExportKey handles keys within the export overlay (Esc is handled by the
// caller). Tab/Shift-Tab move focus; ←/→ cycle choice fields (preset/format);
// typing edits text fields (von/bis/path); Enter triggers the export.
func (m Model) handleExportKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case k.Code == tea.KeyTab:
		if k.Mod == tea.ModShift {
			m.expFocus = (m.expFocus + 4) % 5
		} else {
			m.expFocus = (m.expFocus + 1) % 5
		}
		return m, nil
	case k.Code == tea.KeyEnter:
		return m.submitExport()
	case k.Code == tea.KeyLeft || k.Code == tea.KeyRight:
		dir := 1
		if k.Code == tea.KeyLeft {
			dir = -1
		}
		return m.cycleField(dir), nil
	case k.Code == tea.KeyBackspace:
		return m.editField(func(s string) string {
			if r := []rune(s); len(r) > 0 {
				return string(r[:len(r)-1])
			}
			return s
		}), nil
	case k.Text != "":
		t := k.Text
		return m.editField(func(s string) string { return s + t }), nil
	}
	return m, nil
}

// cycleField advances the focused choice field (preset/format) and recomputes
// dependent state. No-op on text fields.
func (m Model) cycleField(dir int) Model {
	switch m.expFocus {
	case 0: // preset
		m.expPreset = cyclePreset(m.expPreset, dir)
		if m.expPreset != "custom" {
			m.expFrom, m.expTo = exportPresetRange(m.expPreset, m.now)
		}
		m.refreshDefaultPath()
	case 3: // format
		m.expFormat = cycleFormat(m.expFormat, dir)
		m.refreshDefaultPath()
	}
	return m
}

// editField applies fn to the focused text field (von/bis/path). Editing a date
// switches the preset to "custom"; editing the path marks it user-owned.
func (m Model) editField(fn func(string) string) Model {
	switch m.expFocus {
	case 1:
		m.expFrom = fn(m.expFrom)
		m.expPreset = "custom"
		m.refreshDefaultPath()
	case 2:
		m.expTo = fn(m.expTo)
		m.expPreset = "custom"
		m.refreshDefaultPath()
	case 4:
		m.expPath = fn(m.expPath)
		m.expPathEdited = true
	}
	return m
}

// refreshDefaultPath recomputes the suggested path from from/to/format, unless
// the user has manually edited the path.
func (m *Model) refreshDefaultPath() {
	if !m.expPathEdited {
		m.expPath = defaultExportPath(m.expFrom, m.expTo, m.expFormat)
	}
}
```

`submitExport` is added in Task 4. For THIS task, add a temporary stub so it compiles:

```go
// submitExport is fleshed out in Task 4.
func (m Model) submitExport() (tea.Model, tea.Cmd) {
	return m, nil
}
```

- [ ] **Step 4: Run — PASS**

Run: `go test ./internal/tui/ 2>&1 | tail -8`, `go vet ./internal/tui/`, `gofmt -l internal/tui/` (leer).
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/export.go internal/tui/export_test.go
git commit -m "feat(tui-export): panel focus nav + choice cycling + text edit"
git rev-parse --short HEAD
```

---

## Task 4: Export-Cmd — Validierung, Server-Call, Datei schreiben, Status

**Files:**
- Modify: `internal/tui/export.go` (`submitExport` + `exportCmd` + Msg-Typen)
- Modify: `internal/tui/worktime.go` (`Update` für die neuen Msgs)
- Test: `internal/tui/export_test.go`

- [ ] **Step 1: Failing test schreiben**

Add to `internal/tui/export_test.go` (Imports ggf. ergänzen: `net/http`, `net/http/httptest`, `os`, `path/filepath`):

```go
func TestSubmitExportInvalidDate(t *testing.T) {
	m := openExportM(t)
	m.expPreset = "custom"
	m.expFrom = "not-a-date"
	m.expTo = "2026-06-30"
	next, cmd := m.submitExport()
	if cmd != nil {
		t.Fatal("invalid date should not dispatch a command")
	}
	if next.(Model).expStatus == "" {
		t.Fatal("invalid date should set an inline status")
	}
}

func TestSubmitExportToBeforeFrom(t *testing.T) {
	m := openExportM(t)
	m.expPreset = "custom"
	m.expFrom = "2026-06-30"
	m.expTo = "2026-06-01"
	_, cmd := m.submitExport()
	if cmd != nil {
		t.Fatal("to<from should not dispatch a command")
	}
}

func TestExportDoneMsgSetsStatus(t *testing.T) {
	m := openExportM(t)
	next, _ := m.Update(exportDoneMsg{path: "/tmp/flow-export.md"})
	if !strings.Contains(next.(Model).expStatus, "/tmp/flow-export.md") {
		t.Errorf("done status should contain path, got %q", next.(Model).expStatus)
	}
}

func TestExportErrMsgSetsStatus(t *testing.T) {
	m := openExportM(t)
	next, _ := m.Update(exportErrMsg{err: errExportTest})
	if !strings.Contains(next.(Model).expStatus, "boom") {
		t.Errorf("err status should contain error, got %q", next.(Model).expStatus)
	}
}

var errExportTest = &exportTestErr{}

type exportTestErr struct{}

func (*exportTestErr) Error() string { return "boom" }

func TestExportCmdWritesFile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/export", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("# Worktime\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "out.md")
	m := New(apiclient.New(srv.URL, "tok"), "tester")
	m.expFrom, m.expTo, m.expFormat, m.expPath = "2026-06-01", "2026-06-30", "md", target

	msg := m.exportCmd()()
	done, ok := msg.(exportDoneMsg)
	if !ok {
		t.Fatalf("want exportDoneMsg, got %T (%v)", msg, msg)
	}
	if done.path != target {
		t.Errorf("done path %q want %q", done.path, target)
	}
	b, err := os.ReadFile(target)
	if err != nil || string(b) != "# Worktime\n" {
		t.Fatalf("file content %q err %v", b, err)
	}
}
```
(`apiclient` ist im Test-Package via `worktime_test.go` bereits importiert — da `export_test.go` dasselbe Package `tui` ist, ist der Import dort vorhanden; falls `go test` über fehlende Imports im neuen File meckert, ergänze `"github.com/serverkraken/flow/internal/adapter/apiclient"` in `export_test.go`.)

- [ ] **Step 2: Run — FAIL**

Run: `go test ./internal/tui/ -run 'TestSubmitExport|TestExportDoneMsg|TestExportErrMsg|TestExportCmdWrites'`
Expected: FAIL (undefined `exportDoneMsg`/`exportErrMsg`/`exportCmd`, stub `submitExport`).

- [ ] **Step 3: Implementieren**

In `internal/tui/export.go`: add imports `context`, `os`, `time` (time already there), and add the msg types + replace the `submitExport` stub + add `exportCmd`:

```go
type exportDoneMsg struct{ path string }
type exportErrMsg struct{ err error }

// submitExport validates the range and, if valid, dispatches exportCmd. Invalid
// dates set an inline status and dispatch nothing.
func (m Model) submitExport() (tea.Model, tea.Cmd) {
	from, errF := time.Parse(dayFmtTUI, m.expFrom)
	to, errT := time.Parse(dayFmtTUI, m.expTo)
	if errF != nil || errT != nil {
		m.expStatus = "Ungültiges Datum (yyyy-mm-dd erwartet)"
		return m, nil
	}
	if to.Before(from) {
		m.expStatus = "bis muss >= von sein"
		return m, nil
	}
	if m.client == nil {
		return m, nil
	}
	m.expStatus = "exportiere…"
	return m, m.exportCmd()
}

// exportCmd fetches the export from the server and writes it to the resolved
// path, returning exportDoneMsg{path} or exportErrMsg{err}.
func (m Model) exportCmd() tea.Cmd {
	from, to, format, path := m.expFrom, m.expTo, m.expFormat, expandHome(m.expPath)
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		b, err := client.Export(ctx, from, to, format, "")
		if err != nil {
			return exportErrMsg{err}
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return exportErrMsg{err}
		}
		return exportDoneMsg{path: path}
	}
}
```

In `internal/tui/worktime.go` `Update`, add cases (near the other msg cases, e.g. after `rangeLoadedMsg`):

```go
	case exportDoneMsg:
		m.expStatus = "✓ geschrieben: " + msg.path
		return m, nil
	case exportErrMsg:
		m.expStatus = "Fehler: " + msg.err.Error()
		return m, nil
```

- [ ] **Step 4: Run — PASS**

Run: `go test ./internal/tui/ 2>&1 | tail -10`, `go build ./...`, `go vet ./internal/tui/`, `gofmt -l internal/tui/` (leer).
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/export.go internal/tui/worktime.go internal/tui/export_test.go
git commit -m "feat(tui-export): submit validation + async export-to-file cmd"
git rev-parse --short HEAD
```

---

## Task 5: Verifikation — make ci + manuelles Done-Gate

**Files:** keine Code-Änderung (außer evtl. Coverage-Lücken schließen).

- [ ] **Step 1: Voller Lauf**

Run (from `/Users/msoent/SourceCode/serverkraken/flow-rebuild`):
```bash
gofmt -l internal/tui
go build ./... && go vet ./...
make ci
```
Expected: `make ci` grün inkl. Coverage-Gate **≥80%**. Falls die Gate-Zahl unter 80% rutscht: Happy-Path-Tests in `export_test.go` ergänzen (Preset-Branches `kw`/`monat`/`letzter`, `exportView`-Render mit gesetztem `expStatus`) — **nicht** die Schwelle senken.

- [ ] **Step 2: Manuelles Done-Gate (Dev-Stack)**

Vorbedingung: Dev-Stack läuft (`make dev-up && make dev-run &`), Token vorhanden.
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
export FLOW_TOKEN=$(make -s dev-token); export FLOW_SERVER_URL=http://localhost:8080
flow worktime
```
Im TUI: `e` öffnet das Export-Panel → mit `tab` zwischen Feldern wechseln, `←/→` für Range-Preset + Format, ggf. von/bis tippen, Enter. Erwartet: Statuszeile „✓ geschrieben: <pfad>"; Datei liegt unter `~/Downloads/flow-export-<von>_<bis>.<ext>` und enthält den Export (MD-Summary mit Projekt-Zeile, bzw. CSV-Detailzeilen). `esc` schließt das Panel, `q` beendet.

Prüfe die Datei z.B.:
```bash
ls -la ~/Downloads/flow-export-*.md && head -12 ~/Downloads/flow-export-*.md
```

- [ ] **Step 3: HEAD verifizieren**

```bash
git log --oneline -6 && git status --short
```
Expected: 4 TUI-Export-Commits auf `rebuild`, Worktree clean. (Lesson [[feedback_subagent_git_commits_isolated]] — HEAD nach jedem Subagent prüfen.)

---

## Self-Review-Notiz (vom Planautor)

**Spec-Coverage:** Overlay per `e` (T2) ✓; Presets KW/Monat/letzter + freie von/bis (T1 Range-Helfer, T3 Cycling+Edit) ✓; Format csv/json/md (T1 cycle, T3 Format-Fokus) ✓; editierbarer Pfad vorbelegt + `expPathEdited`-Sticky (T1 defaultPath, T3 editField, T4 expandHome+write) ✓; Datei schreiben + Pfad/Fehler-Status (T4) ✓; Fokus-Nav Tab/Shift-Tab (T3) ✓; Inline-Validierung Datum/`to<from` (T4) ✓; Footer-Hinweis (T2) ✓; kein Per-Projekt-Filter / kein Rate-Setzen (Scope eingehalten, nichts gebaut) ✓; Tests + make-ci-Coverage (T5) ✓.

**Bewusste Defaults:** Fokus-Nav nur Tab/Shift-Tab (kein j/k — kollidiert mit Text-Edit in von/bis/Pfad). Choice-Cycling nur ←/→ (Space weggelassen — YAGNI; falls gewünscht trivial nachrüstbar). `kw`-Preset = Montag→heute, `monat` = Monatserster→heute, `letzter` = ganzer Vormonat (konsistent zur WebUI-`exportDefaultRange`). Panel bleibt nach Export offen (Esc schließt). Kein Auto-Öffnen der Datei.
