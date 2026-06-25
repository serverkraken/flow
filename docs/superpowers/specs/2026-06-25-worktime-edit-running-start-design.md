# Worktime: Startzeit eines laufenden Timers via Palette anpassen

**Datum:** 2026-06-25
**Status:** Approved
**Surface:** TUI (`flow ui`) — Worktime „Heute"-Route + Shell-Palette

## Problem

Läuft ein Timer und die Startzeit war falsch (zu spät/zu früh gestartet, vergessen
zu starten), gibt es in der aktuellen TUI **keinen Weg, die Startzeit der laufenden
Session zu korrigieren**:

- `openEdit` (Taste `enter`/`E`) bearbeitet ausschließlich **abgeschlossene** Sessions
  (`r.st.Completed[cursor]`). Die laufende Session (`r.st.Active`, `Stop == nil`) hat
  keine Zeile in `Completed` und ist damit nicht erreichbar.
- Die `:`-Palette ist **statisch**: `Shell.WithTabs` backt pro Tab genau einen Eintrag
  (Tab-Wechsel). Sie trägt keine kontextuellen Verben.

Früher (alte TUI) war genau das über `:` → „Startzeit anpassen" möglich. Das soll
zurückkommen — und zwar als generischer, route-kontextueller Palette-Mechanismus, damit
weitere Routen später eigene Verben beisteuern können.

Das Backend kann das längst: `usecase.EditSession` akzeptiert `Stop == nil`, prüft
Overlap inkl. der laufenden Session, und `apiclient.EditSession` ist verdrahtet.

## Ziel

Bei laufendem Timer auf der „Heute"-Route:
`:` öffnen → Eintrag **„Startzeit anpassen"** wählen → HH:MM-Dialog (vorbefüllt mit der
aktuellen Startzeit) → bestätigen → Session läuft mit korrigierter Startzeit weiter.

Nicht-Ziel (YAGNI): Projekt/Tag/Note der laufenden Session ändern, relative Nudges,
Datepicker-Stepper, Bearbeiten von Startzeiten auf anderen Routen.

## Architektur

Vier Bausteine, stark auf vorhandener Infrastruktur aufbauend.

### 1. `shell.PaletteProvider` — neues optionales Route-Interface

In `internal/tui/shell/route.go`, analog zu den bestehenden optionalen Interfaces
(`InputCapturer`, `FullScreener`, `BreadcrumbHider`, …):

```go
// PaletteProvider lets the active tab's top route contribute contextual action
// entries to the :-palette, gathered fresh each time the palette opens so they
// reflect current route state. Optional — routes that don't implement it only
// expose the static tab-navigation entries.
type PaletteProvider interface{ PaletteEntries() []PaletteEntry }
```

### 2. Shell merged die Einträge beim Öffnen

`internal/tui/shell/shell.go`:

- Die statischen Tab-Navigations-Entries werden in `WithTabs` auf der Shell abgelegt
  (`s.navEntries []PaletteEntry`) statt direkt via `NewPalette(entries)` in die Palette
  gebacken. (Der `s.palette`-Aufbau in `WithTabs` entfällt; die Palette wird erst beim
  Öffnen gebaut.)
- Neuer Helper `buildPalette()`: sammelt zuerst die kontextuellen Aktionen der aktiven
  Top-Route (falls sie `PaletteProvider` implementiert), dann die `navEntries`, und gibt
  `NewPalette(actions ++ navEntries)` zurück.
- Im `:`-Handler (`handleKey`, `case k.Text == ":"`) wird `s.palette = s.buildPalette()`
  aufgerufen (ersetzt das bisherige `s.palette.Reset()`), bevor `paletteOpen = true`.

**Darstellung:** flache Liste, Aktionen oben, dann Tab-Navigation. Die Labels sind
selbsterklärend („Startzeit anpassen" vs. „Heute"/„Wissen"), keine visuelle Gruppierung
nötig. Die vorhandene Substring-Filterung der Palette wirkt unverändert auf alle Einträge.

`PaletteSelectedMsg` läuft bereits über `s.Update(msg.Entry.Action())`, fällt auf
`default` und wird an die aktive Route weitergereicht — kein zusätzlicher Dispatch nötig.

### 3. `TodayRoute` liefert die Aktion

`internal/tui/screen/worktime/`:

```go
// adjustStartMsg is emitted by the "Startzeit anpassen" palette entry and handled
// in TodayRoute.Update to open the start-edit dialog for the running session.
type adjustStartMsg struct{}

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

`TodayRoute.Update` bekommt einen Zweig für `adjustStartMsg{}`, der `openAdjustStart()`
aufruft. (Die Shell reicht die Msg über den `default`-Pfad an `UpdateTop` durch.)

### 4. Neuer Dialog `dialogEditStart`

`internal/tui/screen/worktime/dialogs.go` + `route.go`:

- Neuer `dialogKind`-Wert `dialogEditStart`.
- `adjustState struct { id string; date time.Time; input textinput.Model }` (ein
  einzelnes Feld — kein Slice/`cur` wie bei `editState`).
- `openAdjustStart()`:
  - Guard: `if !r.st.Running || r.st.Active == nil { return r, nil }`.
  - HH:MM-Textfeld via `form.NewTextInput("HH:MM", r.pal)`, vorbefüllt mit
    `r.st.Active.Format("15:04")`.
  - `r.adjust = adjustState{id: r.st.ActiveID, date: *r.st.Active, input: in}`,
    `r.dialog = dialogEditStart`, Feld fokussieren.
- `handleDialogKey` routet `dialogEditStart` → `handleAdjustStartKey`:
  - `Esc` → `dialog = dialogNone`.
  - `Enter` → `submitAdjustStart()`.
  - sonst → Feld-`Update`.
- `submitAdjustStart()`:
  - `wtfmt.ParseHM` auf den Feldwert; Fehler → `toast.NewDanger("Start ungültig (HH:MM)")`,
    Dialog bleibt offen.
  - Startzeit auf das Datum der laufenden Session legen (wie `submitEdit`:
    `base := time.Date(date.Year(), date.Month(), date.Day(), 0,0,0,0, date.Location())`,
    `startTime := base.Add(parsed)`).
  - Zukunfts-Guard: `if startTime.After(r.now()) { toast „Start liegt in der Zukunft" }`.
  - `api.EditSession(ctx, id, nil, "", "", startTime, nil)` — **`stop == nil`** hält die
    Session laufend; Projekt wird (wie bei einer frisch gestarteten Session) nicht gesetzt,
    Tag/Note bleiben leer.
  - Erfolg → `reloadMsg{}`; API-Fehler (inkl. `ErrOverlap`) → `loadedMsg{err}` → Toast.
- `CapturesInput()` (`r.dialog != dialogNone`) greift automatisch — kein Extra-Code.
- `renderDialog` + `dialogHints` bekommen einen `dialogEditStart`-Zweig (Titel
  „Startzeit anpassen", Hints `enter` bestätigen / `esc` abbrechen).

## Datenfluss

```
[Timer läuft] : ──────────────► Shell.buildPalette()
                                   ├─ TodayRoute.PaletteEntries() → ["Startzeit anpassen"]
                                   └─ navEntries → ["Heute", "Wissen", …]
  ▼ Enter auf "Startzeit anpassen"
PaletteSelectedMsg → s.Update(action()) → adjustStartMsg → UpdateTop
  ▼
TodayRoute.openAdjustStart()  → dialogEditStart (HH:MM vorbefüllt)
  ▼ Enter
submitAdjustStart() → api.EditSession(id, nil, "", "", start, nil)
  ▼
reloadMsg → todayState neu → Timer läuft mit korrigierter Startzeit
```

## Fehlerbehandlung

| Situation | Verhalten |
|---|---|
| Kein laufender Timer | Eintrag erscheint nicht; `openAdjustStart` zusätzlich guarded |
| Ungültiges HH:MM | Danger-Toast, Dialog bleibt offen, kein API-Call |
| Startzeit in der Zukunft | Danger-Toast „Start liegt in der Zukunft", kein API-Call |
| Überlappung mit Vorgänger-Session | Backend `EditSession` → `ErrOverlap` → Toast |
| API-/Netzwerkfehler | `loadedMsg{err}` → Toast |

## Tests (TDD)

**Shell (`internal/tui/shell`):**
- Bei geöffneter Palette erscheinen die `PaletteProvider`-Aktionen der aktiven Route
  **vor** den Tab-Navigations-Einträgen.
- Routen ohne `PaletteProvider` zeigen unverändert nur die Tab-Einträge.
- Auswahl eines Aktions-Eintrags reicht dessen Action-Msg an die aktive Route durch
  (Palette schließt).
- `buildPalette()` wird bei jedem `:` neu aufgerufen (Zustandsänderung zwischen zwei
  Öffnungen spiegelt sich).

**Worktime (`internal/tui/screen/worktime`):**
- `PaletteEntries()` liefert den Eintrag **nur** bei laufendem Timer, sonst `nil`.
- `adjustStartMsg` öffnet `dialogEditStart`, Feld vorbefüllt mit aktueller Startzeit.
- `submitAdjustStart` ruft `EditSession` mit `stop == nil` und korrekt komponierter
  Startzeit (Fake-API verifiziert die Argumente).
- Ungültiges HH:MM → kein `EditSession`-Call, Dialog bleibt `dialogEditStart`.
- Startzeit in der Zukunft → kein `EditSession`-Call, Toast.

## Geänderte/neue Dateien

- `internal/tui/shell/route.go` — `PaletteProvider`-Interface (+ Doc).
- `internal/tui/shell/shell.go` — `navEntries`-Feld, `buildPalette()`, `:`-Handler.
- `internal/tui/shell/shell_test.go` (oder `palette_test.go`) — Merge-/Forward-Tests.
- `internal/tui/screen/worktime/route.go` — `PaletteEntries`, `adjustStartMsg`-Zweig,
  `dialogEditStart` in `renderDialog`/`dialogHints`/`handleDialogKey`.
- `internal/tui/screen/worktime/dialogs.go` — `adjustState`, `openAdjustStart`,
  `handleAdjustStartKey`, `submitAdjustStart`.
- `internal/tui/screen/worktime/dialogs_*_test.go` — neue Tests.

## Done-Gate

- `make ci` grün (lint inkl. `gofumpt`/`staticcheck`, templ, build, Tests, Coverage-Gate).
- Manuelles Dogfood gegen das Dev-Stack (Postgres + Dex, `FLOW_DEV=1`,
  `make dev-up`/`dev-run`): Timer starten, `:` → „Startzeit anpassen", Startzeit nach
  vorn/hinten korrigieren, Live-Tick/Logged-Anzeige stimmt; Zukunfts- und
  Overlap-Fehlerfälle zeigen Toasts.
