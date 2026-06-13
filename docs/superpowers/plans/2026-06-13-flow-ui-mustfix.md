# flow UI Must-fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Read the live code at every cited `file:line` before editing** — line numbers are from `next` @ 2026-06-13 and drift as tasks land. Source of findings: `docs/ui-review-2026-06-13.md`.

**Goal:** Die 13 Must-fix aus dem UI-Review beheben — vor allem die R2a/R2b-Lücken, die den täglichen TUI-Workflow brechen (Crash beim Session-Taggen, kein Stop/Resume, `q` killt den Sidekick).

**Architecture:** Hexagonal. TUI/CLI sprechen via `internal/adapter/httpapi` (REST) mit `flow-server`. Fixes bleiben in den Frontend-Layern (`internal/frontend/tui`, `internal/webui`) + Composition-Root (`cmd/flow/main.go`); ein einziger neuer Adapter (`mutexlock`) für die prozesslokale Lock-Disziplin des server-gestützten `SessionWriter`.

**Tech Stack:** Go, bubbletea v2, lipgloss v2, cobra, templ/HTMX. Tests: `go test` (table/behavior-Tests; bubbletea-Update-Tests in `*_test.go`).

**Entscheidungen (vom Nutzer abgenommen):**
- **#1:** Session-Edit im Server-Modus **voll verdrahten** (server-gestützter `SessionWriter` über `httpapi.Sessions.Upsert`), nicht nur absichern.
- **#2:** `s` als **Toggle** — läuft → stoppen, pausiert → fortsetzen, idle → Picker.

**Worktree:** Ausführung im isolierten Worktree `flow-ui-mustfix` auf Branch `ui-mustfix` (von `next`). Commits dort; Merge nach `next` nach Task F.

**Cluster-Reihenfolge nach Wirkung:** A (Session-Wiring) → B (q-Familie) → C (stumme Fehler) → D (tote Affordanzen) → E (Degraded-Mode-Texte) → F (Wiring-Verifikation).

---

## Cluster A — R2a/R2b Session-Wiring

> **Kopplung:** A2 (#1) macht `Deps.SessionWriter` non-nil. Dessen Lifecycle-Methoden (`Start/Stop/Pause/Resume`) dereferenzieren `State` (= nil im Server-Modus) → Panik, falls aufgerufen. A3 (#2) stellt sicher, dass die TUI Lifecycle ausschließlich über `ActiveSessions` fährt, sodass `sw` nur noch für Edits (`SetTag/SetNote/Edit/Delete`, die `State` nie anfassen — verifiziert: `session_writer.go:306-454`) benutzt wird. **A1 → A2 → A3 in dieser Reihenfolge.**

### Task A1: Prozesslokaler `mutexlock`-Adapter

Server-gestützter `SessionWriter` braucht eine `ports.Lock` (`internal/ports/sessions.go:72` — `With(fn func() error) error`). Der bisherige flock-Adapter ist mit dem flockstate-Pfad gelöscht. Im Server-Modus serialisiert ein prozesslokaler Mutex die lokalen Read-Modify-Upsert-Edits (Server erzwingt zusätzlich ETag/If-Match).

**Files:**
- Create: `internal/adapter/mutexlock/mutexlock.go`
- Test: `internal/adapter/mutexlock/mutexlock_test.go`

- [ ] **Step 1: Failing test**

```go
package mutexlock_test

import (
	"sync"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/mutexlock"
)

func TestWith_SerializesConcurrentCalls(t *testing.T) {
	l := mutexlock.New()
	var mu sync.Mutex
	inside, maxInside := 0, 0
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = l.With(func() error {
				mu.Lock()
				inside++
				if inside > maxInside {
					maxInside = inside
				}
				mu.Unlock()
				mu.Lock()
				inside--
				mu.Unlock()
				return nil
			})
		}()
	}
	wg.Wait()
	if maxInside != 1 {
		t.Fatalf("expected max 1 goroutine inside critical section, got %d", maxInside)
	}
}
```

- [ ] **Step 2: Run → FAIL** `go test ./internal/adapter/mutexlock/ -run TestWith_SerializesConcurrentCalls -v` → "no required module provides package".

- [ ] **Step 3: Implement**

```go
// Package mutexlock provides a process-local ports.Lock for the
// server-mode SessionWriter: it serialises local read-modify-upsert
// edits within one flow process. Cross-process / cross-device
// consistency is enforced server-side via ETag/If-Match.
package mutexlock

import "sync"

type Lock struct{ mu sync.Mutex }

func New() *Lock { return &Lock{} }

func (l *Lock) With(fn func() error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return fn()
}
```

- [ ] **Step 4: Run → PASS** `go test ./internal/adapter/mutexlock/ -v`

- [ ] **Step 5: Commit** — `fix(worktime): mutexlock adapter for server-mode SessionWriter edits`

### Task A2: Server-gestützten `SessionWriter` für Edits verdrahten (#1)

Behebt den nil-Panic bei Tag/Notiz/Edit (`today_dialog_submit.go:25,39,114-120`). `httpapi.Sessions` (`cmd/flow/main.go:140`) implementiert `ports.SessionStore`; `reader` (`main.go:217`) ist bereits da.

**Files:**
- Modify: `cmd/flow/main.go` (Konstruktion + zwei Wiring-Stellen: worktime-Deps `:271`, CLI-WorktimeDeps `:~298`)
- Modify: `internal/frontend/tui/screen/worktime/today_actions.go:138-150` (deleteCmd: irreführenden „nicht eingeloggt"-Text korrigieren)
- Test: `internal/frontend/tui/screen/worktime/today_edit_test.go` (existiert — nutzt `newRig` mit non-nil Writer; ergänzen)

- [ ] **Step 1: Failing test** — Round-Trip-Test, dass Tag-Submit über einen fake `ports.SessionStore` (oder den Test-Rig) tatsächlich `Upsert` mit gesetztem Tag aufruft (kein Panic, korrekter Wert). Nutze das vorhandene `newRig`-Muster aus `today_edit_test.go`; assert, dass nach `t`+Tag+Enter der Store die Session mit `Tag == "x"` enthält.

```bash
go test ./internal/frontend/tui/screen/worktime/ -run TestHeute_SetTag -v
```

- [ ] **Step 2: Run → FAIL** (Test existiert noch nicht / aktueller Code crasht mit nil-Writer im Produktiv-Wiring).

- [ ] **Step 3: Implement** — in `main.go` nach `tagger := …` (`:247`):

```go
sessionWriter := &usecase.SessionWriter{
	Sessions: httpSessions,
	State:    nil, // server mode: lifecycle goes via ActiveSessions; edit-only writer
	Lock:     mutexlock.New(),
	Reader:   reader,
	Clock:    clock,
	UserID:   userID,
}
```

Setze `SessionWriter: sessionWriter` an `main.go:271` (worktime-Deps) **und** `main.go:~298` (CLI-`WorktimeDeps`, ersetzt `nil, // server mode: legacy SessionWriter path disabled`). Import `mutexlock` ergänzen.

In `today_actions.go:141-143` den nil-Guard-Text ersetzen (sw ist jetzt verdrahtet; defensiv behalten, aber ehrlich):

```go
if sw == nil {
	return heuteActionDoneMsg{err: errors.New("Session-Bearbeitung nicht verfügbar")}
}
```

- [ ] **Step 4: Run → PASS** + `go test ./internal/frontend/tui/screen/worktime/ -v`

- [ ] **Step 5: Commit** — `fix(worktime): wire server-backed SessionWriter so tag/note/edit/delete work (was nil-panic)`

### Task A3: `s` als Toggle über `ActiveSessions` (#2)

Heute öffnet `s` immer den Picker (`today_project_picker.go:108`), Stop/Resume aus der TUI unmöglich, Footer lügt. `usecase.ActiveSessions.{Stop,Resume,Start}` ist verdrahtet (`main.go:287/288`, bereits genutzt in `pauseCmd`/`switchProjectCmd`). Muster: `pauseCmd` (`today_actions.go:104-136`) zeigt den ActiveSessions-Pfad.

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/today_project_picker.go` (`handleSKey`)
- Modify: `internal/frontend/tui/screen/worktime/today_actions.go` (neuer `stopActiveCmd` / `resumeActiveCmd` analog `pauseCmd`)
- Test: `internal/frontend/tui/screen/worktime/today_project_picker_switch_test.go` / `today_actions_test.go`

- [ ] **Step 1: Failing tests** — drei Verhaltenstests:
  - `s` bei laufender Session → ruft `ActiveSessions.Stop(userID, projectID, …)` (kein Picker geöffnet, `h.pp == nil`).
  - `s` bei pausierter Session → ruft `ActiveSessions.Resume(userID, projectID)`.
  - `s` bei idle → öffnet den Picker (`h.pp != nil`).

```bash
go test ./internal/frontend/tui/screen/worktime/ -run 'TestSKey_(Stop|Resume|IdleOpensPicker)' -v
```

- [ ] **Step 2: Run → FAIL**

- [ ] **Step 3: Implement** — `handleSKey` von „immer Picker" auf Toggle umstellen:

```go
func (h heute) handleSKey() (tea.Model, tea.Cmd) {
	if h.deps.ActiveSessions != nil {
		switch {
		case h.day.IsRunning():
			return h, h.stopActiveCmd()
		case h.day.IsPaused():
			return h, h.resumeActiveCmd()
		default:
			return h.openProjectPicker()
		}
	}
	return h.legacyToggle() // bestehender SessionWriter-Pfad (nicht im Server-Modus)
}
```

`stopActiveCmd`/`resumeActiveCmd` in `today_actions.go` nach dem Muster von `pauseCmd` bauen (ActiveSessions auf `h.activeSessions[0].ProjectID`, `emitWorktimeChanged`, Success-Toast **ohne** eingebettetes Glyph — siehe Should-fix Doppel-Glyph). Footer/Pause-Hint in `today_render.go:214,293-299` stimmen jetzt; keine Textänderung nötig, aber verifizieren.

- [ ] **Step 4: Run → PASS** + Paket-Tests grün.

- [ ] **Step 5: Commit** — `fix(worktime): s toggles stop/resume via ActiveSessions in server mode (#2)`

---

## Cluster B — Overlay-Exit / q-Familie

### Task B1: Root-`q`-Guard respektiert FullScreen (#3 + #4 gemeinsam)

`model.go:438` quittet bei `q`, wenn kein TextInput aktiv ist. Beide Note-Viewer (`heute.FullScreen()` `today.go:158`, `history.FullScreen()` `history.go:276`) sind dann aber offen, kennen `q` als Close-Key des Overlays — erreichen es nie. Ein Root-Guard auf `fullScreener` (Interface existiert `model.go:666`) fixt beide.

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/model.go` (`handleKeyMsg` `:438`, neuer Helper `subFullScreen`)
- Test: `internal/frontend/tui/screen/worktime/model_test.go` (analog `quit_test.go`)

- [ ] **Step 1: Failing tests** — `q` auf offenem Heute-Note-Viewer (`o`) → **kein** `tea.Quit` (Overlay schließt via ExitMsg); `q` auf offenem Drill-Note-Viewer → kein Quit.

```bash
go test ./internal/frontend/tui/screen/worktime/ -run 'TestQuit_NoteViewerDoesNotQuit' -v
```

- [ ] **Step 2: Run → FAIL** (heute quittet die App).

- [ ] **Step 3: Implement**

```go
func (m Model) subFullScreen() bool {
	if fs, ok := m.subs[m.current].(fullScreener); ok {
		return fs.FullScreen()
	}
	return false
}
```

`model.go:438` ändern:

```go
if msg.String() == "q" && !m.textInputActive() && !m.subFullScreen() {
	return m, tea.Quit
}
```

- [ ] **Step 4: Run → PASS** + Paket-Tests.

- [ ] **Step 5: Commit** — `fix(worktime): q closes note-viewer overlays instead of quitting app/sidekick (#3,#4)`

### Task B2: Cheatsheet quittet nur im Standalone-Modus (#5)

`cheatsheet/model.go:135` quittet bei jeder `markdown_overlay.ExitMsg` — die der Sidekick via `fanOutToAll` (`sidekick/model.go:178`) an alle Screens broadcastet. Folge: Schließen irgendeines Overlays killt den Sidekick. Fix nach dem `palette.WithStandalone`-Muster.

**Files:**
- Modify: `internal/frontend/tui/screen/cheatsheet/model.go` (Option `WithStandalone()` + `standalone bool`; ExitMsg-Case)
- Modify: `cmd/flow/main.go` (Standalone-Cheatsheet-Konstruktion bekommt `WithStandalone()`, embedded nicht)
- Test: `internal/frontend/tui/screen/cheatsheet/model_test.go`

- [ ] **Step 1: Failing tests** — embedded (`standalone=false`) + `ExitMsg` → **kein** `tea.Quit`; standalone + `ExitMsg` → `tea.Quit`.

- [ ] **Step 2: Run → FAIL**

- [ ] **Step 3: Implement** — `standalone` Feld + `WithStandalone()` Functional-Option; ExitMsg-Case:

```go
case markdown_overlay.ExitMsg:
	if m.standalone {
		return m, tea.Quit
	}
	return m, nil // embedded: ignore — sidekick fans this out to all screens
```

In `main.go` die Standalone-Cheatsheet-Factory mit `cheatsheet.WithStandalone()` konstruieren; die sidekick-eingebettete ohne.

- [ ] **Step 4: Run → PASS**

- [ ] **Step 5: Commit** — `fix(cheatsheet): only quit on ExitMsg in standalone mode (was killing whole sidekick) (#5)`

### Task B3: Cheatsheet beansprucht `c` (#11)

`c` ist im Sidekick der Tab-Switch (`sidekick/model.go:306`) und erreicht den Screen nie, obwohl die Hilfe `c → Code kopieren` bewirbt. `keyConsumer` (`sidekick/model.go:66`) ist der Mechanismus.

**Files:**
- Modify: `internal/frontend/tui/screen/cheatsheet/model.go` (`ConsumesKeys`)
- Test: `internal/frontend/tui/sidekick/model_test.go` (screenClaimsKey)

- [ ] **Step 1: Failing test** — sidekick auf Cheatsheet-Tab, `c` wird vom Screen beansprucht (nicht als Tab-Switch konsumiert).
- [ ] **Step 2: Run → FAIL**
- [ ] **Step 3: Implement** — `func (m Model) ConsumesKeys() []string { return []string{"c"} }`
- [ ] **Step 4: Run → PASS**
- [ ] **Step 5: Commit** — `fix(cheatsheet): claim 'c' so code-copy works in sidekick (#11)`

---

## Cluster C — Stumme Fehler / fehlendes Feedback

### Task C1: Frei-Löschen über festgehaltenes Datum (#6)

`dayoffs.go:175` löst das Zieldatum erst beim Bestätigen aus `f.entries[f.cursor]` — Reload/ChangedMsg während des Confirms kann den Index verschieben → falscher Tag gelöscht.

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/dayoffs.go` (`D`-Branch: `f.deleteDate` festhalten; ResultMsg nutzt es)
- Test: `internal/frontend/tui/screen/worktime/dayoffs_test.go`

- [ ] **Step 1: Failing test** — Confirm öffnen, dann `entries` neu sortieren (eine `freiLoadedMsg` einspielen), bestätigen → gelöscht wird das ursprünglich angezeigte Datum, nicht `entries[cursor]`.
- [ ] **Step 2: Run → FAIL**
- [ ] **Step 3: Implement** — Feld `deleteDate time.Time`; im `D`-Handler `f.deleteDate = d.Date` setzen; im `confirm.ResultMsg`-Handler `f.deleteDate` statt `f.entries[f.cursor].Date` verwenden.
- [ ] **Step 4: Run → PASS**
- [ ] **Step 5: Commit** — `fix(worktime): delete day-off by captured date, not live cursor index (#6)`

### Task C2: `wpErrorMsg`-Handler im Projects-Screen (#7)

`worktime_projects.go` liefert `wpErrorMsg` (`:401,417,432`), aber `Update` hat keinen `case` → Create/Rename/Archive-Fehler verschwinden stumm.

**Files:**
- Modify: `internal/frontend/tui/screen/projects/worktime_projects.go` (`Update`: `case wpErrorMsg`)
- Test: `internal/frontend/tui/screen/projects/worktime_projects_test.go`

- [ ] **Step 1: Failing test** — `wpErrorMsg{context:"archivieren", err:…}` an `Update` → `m.toast` ist ein Danger-Toast mit Kontext+Grund.
- [ ] **Step 2: Run → FAIL**
- [ ] **Step 3: Implement**

```go
case wpErrorMsg:
	t := toast.NewDanger(msg.context + ": " + msg.err.Error())
	m.toast = &t
	return m, t.Init()
```

- [ ] **Step 4: Run → PASS**
- [ ] **Step 5: Commit** — `fix(projects): surface create/rename/archive errors as danger toast (#7)`

### Task C3: Notes-Browser lädt bei Server-Änderung nach + `r`-Reload (#8)

`kompendium/browse` lädt nur bei Init/Edit/Delete; SSE-Änderungen anderer Geräte erscheinen nie, kein manueller Reload. Muster: worktime nutzt `httpapi.Status.Changed()` + `listenForChanged`.

**Files:**
- Modify: `internal/kompendium/frontend/tui/browse/model.go` + `update.go` + `commands.go` (Changed-Kanal via Option/Deps, `listenForChanged`-Cmd, `r`-Key → `loadEntriesCmd`, Help-Eintrag)
- Modify: `cmd/flow/main.go` (`buildNotesScreen`: Changed-Kanal verdrahten)
- Test: `internal/kompendium/frontend/tui/browse/update_test.go`

- [ ] **Step 1: Failing tests** — (a) `changedMsg` → `loadEntriesCmd` gefeuert; (b) `r`-Key → `loadEntriesCmd`.
- [ ] **Step 2: Run → FAIL**
- [ ] **Step 3: Implement** — Changed-`<-chan struct{}` als Option in das browse-Model; `listenForChanged` als rekursives `tea.Cmd` (Muster: worktime `listenForChanged`); `r` in `handleNormalKey` → `loadEntriesCmd`; `?`-Overlay um `r → neu laden` ergänzen. In `main.go` `events.Changed()` an `buildNotesScreen` übergeben.
- [ ] **Step 4: Run → PASS** + `go test ./internal/kompendium/frontend/tui/browse/ -v`
- [ ] **Step 5: Commit** — `feat(notes): reload corpus on server change + r key (#8)`

---

## Cluster D — Tote Affordanzen

### Task D1: `project_picker` gibt `j`/`k` an den Filter (#10)

`project_picker/update.go:31` fängt `j`/`k` als Navigation ab, obwohl es ein Live-Filter ist → „kompendium" tippen springt/legt Falsch-Projekt an. Skill nennt den Picker explizit.

**Files:**
- Modify: `internal/frontend/tui/components/project_picker/update.go` (`j`/`k` aus Nav-Cases entfernen; `up`/`down`/`ctrl+p`/`ctrl+n` bleiben)
- Modify: `internal/frontend/tui/components/project_picker/model_test.go:308` (Test umdrehen: `k`/`j` → Filter-Append)
- Optional: `view.go:60` „tab → Neu anlegen" kapitalisieren

- [ ] **Step 1: Failing test** — `k` und `j` an `Update` → werden an den Filter angehängt (Filter == "k"/"kj"), Cursor bewegt sich nicht.
- [ ] **Step 2: Run → FAIL** (aktuell navigiert es).
- [ ] **Step 3: Implement** — `"j"`/`"k"` aus den `case`-Listen streichen; bestehenden Test `model_test.go:308` auf das korrekte Verhalten umschreiben.
- [ ] **Step 4: Run → PASS**
- [ ] **Step 5: Commit** — `fix(project_picker): j/k type into filter, not navigate (#10)`

### Task D2: Standalone-Palette/Projects schließbar (#9)

`flow palette`-Popup: `q` landet im Filter, `esc` ist No-op → nur Ctrl+C beendet. `flow projects` standalone gleich. Help-Text falsch.

**Files:**
- Modify: `internal/frontend/tui/screen/palette/update.go` (`ModeStandalone`: `q` im Normal-Mode + `esc`-auf-leerem-Filter → `tea.Quit`, vor type-to-filter-Fallthrough)
- Modify: `internal/frontend/tui/screen/palette/model.go:207` (Help-Text auf reale Semantik)
- Modify: `internal/frontend/tui/screen/projects/source_dirs.go` (Standalone-Close analog)
- Test: `palette/update_test.go`, `projects/source_dirs_test.go`

- [ ] **Step 1: Failing tests** — standalone Palette: `q` (Normal-Mode) → `tea.Quit`; `esc` bei leerem Filter → `tea.Quit`. Embedded: unverändert (kein Quit). Projects standalone analog.
- [ ] **Step 2: Run → FAIL**
- [ ] **Step 3: Implement** — Mode-Abfrage (`m.mode == ModeStandalone`) vor dem type-to-filter-Pfad; Help-String korrigieren.
- [ ] **Step 4: Run → PASS**
- [ ] **Step 5: Commit** — `fix(palette,projects): standalone popups closable via q/esc (#9)`

---

## Cluster E — Degraded-Mode Fehlertexte

### Task E1: `ErrUnavailable` übersetzen, in beiden CLIs (#12)

`kompendium/cli/output.go:18 wrapAuthErr` deckt nur `ErrLoggedOut`/`ErrNotConfigured`; `httpapi.ErrUnavailable` leakt roh (`dial tcp … connection refused`). Die flow-CLI hat gar kein `wrapAuthErr`.

**Files:**
- Modify: `internal/kompendium/frontend/cli/output.go` (`wrapAuthErr`: `ErrUnavailable`-Case)
- Create/Modify: `internal/frontend/cli/` — `wrapAuthErr`-Pendant + Anwendung auf die server-gestützten `RunE`-Handler (`worktime.go`, `projects*.go`, `repo.go`, `whoami`)
- Test: `internal/frontend/cli/*_test.go`, `internal/kompendium/frontend/cli/output_test.go`

- [ ] **Step 1: Failing tests** — `wrapAuthErr(fmt.Errorf("%w: …", httpapi.ErrUnavailable))` → deutsche, actionable Meldung ohne `httpapi:`-Präfix (z.B. „flow-server nicht erreichbar — läuft er? (FLOW_SERVER_URL prüfen)"); analog für `ErrLoggedOut` → „bitte `flow login`".
- [ ] **Step 2: Run → FAIL**
- [ ] **Step 3: Implement** — `errors.Is(err, httpapi.ErrUnavailable)`-Case in beiden `wrapAuthErr`; flow-CLI-`RunE`-Rückgaben durch `wrapAuthErr(err)` umhüllen.
- [ ] **Step 4: Run → PASS**
- [ ] **Step 5: Commit** — `fix(cli): translate ErrUnavailable/ErrLoggedOut to actionable German in both CLIs (#12)`

### Task E2: WebUI-Footer bewirbt keine toten Tasten mehr (#13)

Keine WebUI-Seite hat Keyboard-Handler, aber jeder Footer rendert TUI-Stil-Hints (`s`,`n`,`j/k`,`/`,`⌘K`). Minimal-Fix für den Must-fix: die nicht-funktionalen Tastenhinweise entfernen (echte Keybindings = separates Feature/Follow-up).

**Files:**
- Modify: `internal/webui/templates/dashboard/index.templ:81`, `…/worktime/today.templ`, `…/notes/*.templ`, `…/repos/*.templ`, `…/settings/index.templ` (Hint-Zeilen entfernen oder durch sichtbare Button-Labels ersetzen)
- Regenerate: `templ generate` (oder `make`-Äquivalent)

- [ ] **Step 1:** Alle Footer-Tastenhinweise per `rg` lokalisieren: `rg -n 'j/k|⌘ ?K|· n |s zum' internal/webui/templates`
- [ ] **Step 2: Implement** — tote Tastenhinweise entfernen; wo eine echte Aktion sichtbar ist (Button), dessen Label nennen statt einer Taste.
- [ ] **Step 3:** `templ generate` (generierte `_templ.go` aktualisieren) und `go build ./internal/webui/...`
- [ ] **Step 4:** Build grün.
- [ ] **Step 5: Commit** — `fix(webui): remove footer hints for keys that do nothing (#13)`

---

## Task F: Wiring-Verifikation & Smoke (PFLICHT)

> Etablierte Regel: Multi-Task-Pläne enden mit einer expliziten Wiring-Verification — per-Task-Reviews fangen „der Composition-Root ruft den neuen Konstruktor nie auf" nicht.

**Files:** read-only Audit von `cmd/flow/main.go` + Smoke.

- [ ] **Step 1:** `make ci` komplett grün (Gate ≥ 71% Coverage). Falls `internal/webui/handlers` wegen podman-bridge fehlschlägt: als Umgebungsproblem dokumentieren, nicht als Regression werten.
- [ ] **Step 2:** Bestätigen, dass jeder neue Konstruktor/Option im Composition-Root verdrahtet ist: `sessionWriter` (worktime-Deps **und** CLI-Deps), `mutexlock.New()`, `cheatsheet.WithStandalone()` (nur Standalone-Pfad), browse-Changed-Kanal. `rg -n 'sessionWriter|mutexlock|WithStandalone|Changed' cmd/flow/main.go`
- [ ] **Step 3:** TUI-Smoke gegen laufenden `flow-server`:
  - Heute: Session läuft → `s` stoppt; `s` startet wieder (Picker); auf abgeschlossener Session `t`+Tag+Enter → kein Crash, Tag erscheint; `E` Zeiten ändern; `D` löschen.
  - `o` Note-Viewer → `q` schließt nur das Overlay (App lebt).
  - Verlauf-Drill `o` → `q` schließt Overlay.
  - Cheatsheet-Tab im Sidekick: `c` kopiert; Brief/Note in anderen Tabs schließen → Sidekick lebt.
  - Notes-Tab: `r` lädt neu; Änderung auf zweitem Gerät erscheint.
  - `flow palette` / `flow projects` Popup: `q` schließt.
- [ ] **Step 4:** Degraded-Smoke: `flow-server` stoppen → `flow worktime status` zeigt deutsche „nicht erreichbar"-Meldung (kein `httpapi:`/`dial tcp`); TUI Heute zeigt verständlichen Offline-Zustand statt Roh-Dump.
- [ ] **Step 5: Commit** (falls Doku/Kommentare angefasst) — `docs(plan): UI must-fix abgeschlossen — Wiring verifiziert + Smoke`

---

## Self-Review-Notiz (Plan-Autor)

- **Spec-Coverage:** Alle 13 Must-fix abgedeckt — #1→A2, #2→A3, #3+#4→B1 (zusammengelegt: ein Root-Guard), #5→B2, #6→C1, #7→C2, #8→C3, #9→D2, #10→D1, #11→B3, #12→E1, #13→E2.
- **Kopplung dokumentiert:** A2 ⇄ A3 (non-nil `sw` erfordert ActiveSessions-Lifecycle).
- **Offene Detailpunkte für Executor:** exakte Test-Rig-Helfer (`newRig`) und Functional-Option-Signaturen am Live-Code verifizieren; `browse`-Changed-Kanal am worktime-`listenForChanged`-Muster spiegeln.
- **Nicht im Scope:** 47 Should-fix + 16 Nice-to-have (`docs/ui-review-2026-06-13.md`); Bug-/Logik-Review (12 Dimensionen) separat.
