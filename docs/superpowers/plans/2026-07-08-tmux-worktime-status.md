# tmux-Worktime-Status — `flow worktime status` + `stop` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Die tmux-Status-Bar-Integration des alten flow wiederherstellen. Die tmux-Seite ist intakt und ruft alle 5 s `flow worktime status` (Segment via `segment-wrap.sh`) sowie auf `prefix+E` `flow worktime stop` — beide Verben fehlen im Rebuild-Binary. Strukturell ist `flow` heute ein REST-Client gegen `flow-server` (PROD: flow.thebackend.org), das alte `status` las lokale Dateien. Deshalb: **ein neuer aggregierter Server-Endpoint** `GET /api/v1/worktime/status` (reine Komposition der Reader today/week/dayoffs/streak/burndown + laufende Session, KEINE neuen Store-Methoden), **ein client-seitiger Cache** (`~/.cache/flow/worktime-status.json`, 30 s TTL, 2 s Fetch-Timeout, Stale+Dim, >30 min → leer), **ein purer Renderer** `internal/statusline` (Port von `domain/status.go` aus dem alten Repo, Inputs aus dem DTO statt aus fünf Readern), und **`stop` als Picker-Popup** (Buchungspflicht des Servers bleibt: laufende Session ohne Node → fuzzy-Picker über bookable Nodes; mit Node → sofort stoppen; `--node <ref>` für non-TTY).

**Architecture:** Hexagonal wie der Rebuild (`adapter → usecase → ports ← domain`). Server: `usecase.WorktimeStatus` (Struct mit den bestehenden Reader-Usecases als Feldern) + Route als Feld an `httpserver.Server` + Konstruktion in `cmd/flow-server/main.go`. Client: neues pures Paket `internal/statusline` (nur `domain.Kind` importiert), Cache-Paket `internal/statuscache`, tmux-Options-Parser `internal/tmuxopts`, zwei Cobra-Subcommands an `worktimeCmd()`. **Kein SSE-Daemon, kein 5-Endpoint-Fanout, keine Migration, kein pgstore, kein templ/webui, kein i18n-Katalog** (CLI-only — die i18n-Regel gilt nur für webui-Anzeige-Strings). Die einzige Mutation (`stop`) läuft über den bestehenden `POST /api/v1/sessions/{id}/stop`, der bereits `session.stopped` emittiert.

**Tech Stack:** Go 1.x · Cobra · charm.land/bubbletea/v2 + `internal/tui/ui/fuzzylist` (Picker) · `net/http` (REST). Keine neuen Abhängigkeiten (TTY-Erkennung über `os.Stdin.Stat()` + `os.ModeCharDevice`, kein `x/term`). Kein `make generate`, kein `make web` (keine `.templ`/CSS-Änderung).

**Spec:** `docs/superpowers/specs/2026-07-08-tmux-worktime-status-design.md` (approved, commit `5fd854e`). Port-Quelle (anderes Repo, read-only Referenz): `/Users/msoent/SourceCode/serverkraken/flow/internal/domain/status.go` + `status_test.go` + `internal/usecase/status_composer.go`. Formatvorbild: `docs/superpowers/plans/2026-07-07-lesesaal-l55-kontext-modus.md`.

**Basis:** Branch `tmux-status` (ab `rebuild` @ `9c37d6e`), Worktree `/Users/msoent/SourceCode/serverkraken/flow-tmux-status`. Slice-Gate-Range = `rebuild..HEAD`.

---

## Global Constraints

- Branch **`tmux-status`** (bereits ausgecheckt), Worktree `/Users/msoent/SourceCode/serverkraken/flow-tmux-status`. **Committe NIE als Planner** — der Orchestrator committet nach Soennes Plan-Review; die Implementer-Dispatches committen am Task-Ende mit der im Task genannten exakten Message.
- **LEHREN (in JEDEN Task-Dispatch aufnehmen):** (1) **Tests/`make ci` SYNCHRON foreground, NIEMALS `run_in_background`** (Subagenten warten sonst auf nie kommende Notifications). (2) **Erst `git add -A`, dann `make ci`** (verify-generate/verify-css diffen gegen den Index). (3) **Nie zwei `make ci` parallel** (Podman-VM keilt bei parallelen Testcontainer-Läufen). (4) **`make ci` in Bash mit `timeout: 600000`** annotieren (Lehre L5: Default-Timeout reißt bei pgstore-Testcontainern). (5) Nach jedem Task: `git log --oneline -3` (HEAD vorangegangen?) + `git diff --stat HEAD~1` — Subagent-Commits können den Branch-Ref verfehlen.
- **NIE `make fmt`**. **NIE `git stash`** in Dispatches.
- **`make ci`-Gate-Konvention (Finding C14, eindeutig):** JEDER Task committet nur mit grünem **paket-scoped** `go test ./<neues-paket>/...`. Die neuen PUREN Pakete (`statusline`, `statuscache`, `tmuxopts`) + die Usecase-Reader (`WorktimeStatus`, `CurrentStreak`, `NodeMRU`) brauchen kein Docker. **AUSNAHME Task 1b** (`SessionStore.LastBookedByNode`): der pgstore-Test läuft über **testcontainers/Docker** (`DOCKER_HOST` auf den Podman-Socket, `podman-Gotcha`, `timeout: 600000`) — dieser Slice ändert pgstore (eine neue read-only Query, KEINE Migration). Der VOLLE `make ci` (= `lint verify-generate verify-css verify-no-popups cover build`) ist die **Slice-Gate-Bedingung in Task 9** (und jeder Task-Reviewer DARF ihn zwischendurch laufen). Coverage-Gate 75 % (`*_templ.go` ausgeschlossen); die neuen Pakete/Reader sind gut testbar und dürfen die Quote nicht drücken.
- **owner-scoped überall** (jede Reader-/Mutations-Query trägt `ownerID`; „ist nur ein User" ist keine Begründung, AGENTS.md §Grundsätze). `WorktimeStatus.Execute` reicht `ownerID` an jeden Sub-Reader durch; der Routen-Test hat einen Auth-/Owner-Pfad. `stop` läuft über den owner-scoped Server-Stop.
- **SSE-Regel (Mutation → Event → Konsument benannt):** die EINZIGE Mutation dieses Slices ist `stop` über den **bestehenden** `POST /api/v1/sessions/{id}/stop` → `s.Emitter.Emit(EventSessionStopped)` (`worktime.go:111`). Konsument: WebUI `#timer-pill` / Cockpit (`sse:session.stopped`). Der neue `GET /api/v1/worktime/status` ist **read-only, emittiert NICHT** (Spec §1). **Kein neuer Event-Typ.**
- **Keine Monolithen:** ein File pro Verantwortung. `internal/statusline` in ≥3 Files (palette/segment/pacedots), Usecase eigenes File, Handler eigenes File, jeder CLI-Verb eigenes File.
- **Keine Emojis** (die Segment-Glyphen ⏱ ‖ ▶ ✓ ● ○ ▲ ▼ → sind monospace-Statusbar-Glyphen aus der Port-Quelle, KEINE Emoji-Pictogramme — 1:1 aus `domain/status.go` übernommen). **Keine Browser-Popups** (N/A, kein webui). **Kein `make generate`/`make web`** (kein templ/CSS).
- **Bestandsnamen NUR aus dem Dossier** (unten). Wo das Dossier eine Stelle nicht abdeckt: **Step 0 rg-Verifikation im Task** („vor dem Tippen verifizieren, Bestand gewinnt").
- **Status-Pfad-Härte (Spec §2):** `flow worktime status` ist **NIE interaktiv** (kein Device-Flow bei fehlendem/abgelaufenem Token → Offline-Pfad) und gibt **IMMER Exit 0, KEIN stderr** aus (ein Fehlertext in der Status-Bar wäre schlimmer als gar nichts). Die `RunE` returnt daher **immer `nil`** und schreibt nur das Segment nach stdout.

## Dossier — verifizierte Bestandssignaturen (Bestand gewinnt)

Server (Arbeitsrepo):
- `httpserver/server.go:14` `type Server struct` — u.a. `Clock ports.Clock`, `Emitter ports.Emitter`, `StopSession usecase.StopSession`, `GetRunningSession usecase.GetRunningSession`, `ListDayOffs usecase.ListDayOffs`, `Stats usecase.StatsComputer`. KEIN `worktime/status`-Feld (frei).
- `server.go:130` `Routes()`; Muster `mux.Handle("GET /api/v1/today", s.auth(http.HandlerFunc(s.handleToday)))` (:161).
- `httpserver/stats.go:31` `func minutes(d time.Duration) int`; `:60` `handleToday`; `dayoffs.go:14` `const dayFmt = "2006-01-02"`; `worktime.go:16` `func writeJSON(w, status int, v any)`; `middleware.go:18` `func userFrom(ctx) (domain.User, bool)` → `u,_ := userFrom(r.Context())`, `u.ID`.
- `usecase/stats_computer.go:23` `StatsComputer{Sessions,Settings,DayOffs ListDayOffs,Clock,Loc,Nodes}`; `Today(ctx,ownerID)(TodaySummary,error)` — **`TodaySummary.Logged` INKLUDIERT den live-tail** (`BuildDayRecords` mit `now`); `Week(ctx,ownerID,ref)([]domain.WeekDay,error)`; `RangeStats(ctx,ownerID,rng)(domain.Stats,error)` (rng ∈ `"week"`/`"month"`); `Burndown(ctx,ownerID)(domain.MonthBurndownReport,error)`. `startOfDay(t)` unexported im selben Paket.
- `usecase/get_running_session.go:18` `Execute(ctx,ownerID)(domain.WorkSession,bool,error)`; `usecase/list_dayoffs.go:21` `Execute(ctx,ownerID,from,to)([]domain.DayOff,error)`.
- `usecase/stop_session.go:36` `Execute(ctx,ownerID,sessionID,nodeID *string)(WorkSession,error)` — nil/"" → `domain.ErrProjectRequired`.
- `domain/dayrecord.go:24` `WeekDay{Date,Logged,Active *time.Time,Target,IsToday}`; `:34` `Total(now)` midnight-clamp.
- `domain/stats.go:24` `Stats{… Streak int …}` (current consec hit streak, workdays only).
- `domain/burndown.go:12` `MonthBurndownReport{Total,TargetTotal,Target,Saldo,OnTrack,WorkdaysAll,WorkdaysDue}`.
- `domain/dayoff.go:85` `DayOff{Date,Kind,Label,Target}`; `:14` `KindHoliday/Vacation/Sick/Flex/Special/ChildSick/Training`; `:40` `Kind.LabelDe()`.
- `domain/worksession.go:10` `WorkSession{ID,OwnerID,NodeID *string,…,Start,Stop *time.Time}`; `Running()`; `Elapsed(now)`.
- `domain/node.go:117` `func IsBookable(k NodeKind) bool`; `KindEngagement/Vorhaben/Repo/Branch`.
- `event.go:8` `EventSessionStopped EventType = "session.stopped"`.

Client (Arbeitsrepo):
- `apiclient/client.go:20` `Client{base,hc(15s),rt}`; `do(ctx,method,path,body,out)` (Kontext-Deadline steuert Timeout); `:175` `StopSession(ctx,id,nodeID string)(WorkSession,error)`; `:241` `ListNodes(ctx)([]domain.Node,error)`.
- `apiclient/stats.go:10` `Today{…}`; `:54` `GetToday`. (Muster für neue DTO-Typen + Getter.)
- `cmd/flow/main.go:11` `rootCmd()` → `root.AddCommand(worktimeCmd())`; `worktime.go:15` `worktimeCmd()` → `:40` `cmd.AddCommand(worktimeImportCmd())` (hier `status`/`stop` anhängen).
- `cmd/flow/auth.go:13` `clientFromStore(ctx)(*apiclient.Client,error)`.
- `cmd/flow/context.go` — Cache-Muster (cache-first → offline fallback → exit 0 nil).
- `cmd/flow/projectbind_picker.go:30` `pickProjectProgram{list fuzzylist.Model,title}`; `:51` `newPickParentProgram(items,pal)` (pick-only); `:92` `Selection()(fuzzylist.Item,isCreate,ok bool)`.
- `cmd/flow/projectbind.go:58` Run-Muster: `ListNodes` → `[]fuzzylist.Item{ID:p.ID,Label:p.Name}` → `tea.NewProgram(prog, tea.WithContext(ctx)).Run()` → `prog.Selection()`; `pal := theme.Load()`.
- `fuzzylist/fuzzylist.go:20` `type Item struct{ ID, Label string }`.
- `cmd/flow/noderef.go:15` `resolveNodeRef(nodes []domain.Node, ref string)(string,error)` — für `--node`.
- Test-Muster: `httpserver/stats_test.go:27` `newStatsServerFull()` (Fakes + `FakeVerifier` + `Authorization: Bearer x`); `usecase/stats_computer_test.go` (lokale Fakes / `testutil.FakeClock`); `testutil/fakes.go` (`NewFakeSessionStore/NewFakeNodeStore/NewFakeDayOffStore/NewFakeUserSettingsStore/NewFakeUserStore/FakeClock/FakeIDGen/FakeVerifier`).

---

## Wire-Kontrakt: `worktimeStatusDTO` (verbindlich, Spec §1)

```json
{
  "date": "2026-07-08",
  "loggedMin": 312,
  "targetMin": 480,
  "running": true,
  "activeSessionId": "s-…",
  "activeStart": "2026-07-08T13:05:00+02:00",
  "activeNodeId": null,
  "dayOff": { "kind": "vacation", "label": "Urlaub" },
  "week": [
    { "date": "2026-07-06", "loggedMin": 480, "targetMin": 480,
      "workday": true, "isToday": false, "dayOffKind": null }
  ],
  "streak": 4,
  "burndown": { "saldoMin": 130, "targetMin": 9600 }
}
```

**Semantik (KRITISCH — verhindert Doppelzählung):**
- **`loggedMin` = ABGESCHLOSSENE Zeit heute OHNE laufende Session.** `StatsComputer.Today().Logged` inkludiert bereits den live-tail; das Usecase **subtrahiert** den midnight-geclampten Tail der laufenden Session wieder heraus. Der Client extrapoliert die laufende Session selbst aus `activeStart` — sonst zählt Banner + Client-Extrapolation doppelt.
- **`week[].loggedMin`** = Server-Snapshot **inkl.** live-tail für heute (`minutes(d.Total(now))`, wie `handleWeek`) — nur für die Pace-Dot-Klassifikation (hit/running/missed), zwischen Fetches eingefroren, akzeptiert.
- `activeSessionId`/`activeStart`/`activeNodeId` nur bei `running:true`; `activeNodeId` gesetzt, wenn die Session schon beim Start gebucht wurde (entscheidet Picker vs. Direkt-Stop).
- `dayOff` = heutiger Frei-Eintrag oder `null`; `week[].dayOffKind` = Kind pro Tag (holiday/vacation/sick/…) für Pace-Dot-Farben.
- `burndown.targetMin: 0` = kein Ziel → Saldo-Marker entfällt.
- `streak` = `StatsComputer.CurrentStreak(ctx,ownerID)` — **fensterloser** dedizierter Reader (Semantik wie alter `CurrentStreak`: rückwärts bis zum ersten Miss, KEIN Monatsfenster-Schnitt; Task 1c). NICHT `RangeStats("month")` (Entscheidung Soenne 2026-07-08).

---

## Agent-Besetzung & Dispatch-Protokoll

Dieser Slice ist **nicht** Lesesaal → keine `lesesaal-*`-Agents. Der Orchestrator (Session `/effort high`) fährt superpowers:executing-plans und dispatcht generisch. Dispatches nennen das Modell IMMER explizit (Memory: nie Fable erben).

Bauwreihenfolge: **Server-Reader vor ihren Konsumenten** (`1b`/`1c` vor Task 2; `4b` vor Task 8). Die Buchstaben-Suffixe erhalten die Querverweise der bereits detaillierten Tasks; die Ausführungsreihenfolge ist 1 → 1b → 1c → 2 → 3 → 4 → 4b → 5 → 6 → 7 → 8 → 9.

| Task | Agent (`subagent_type`) | Modell · Effort |
|---|---|---|
| 1 `internal/statusline` (purer Renderer, Port + Dim + Empty + Pace-Dots) | `claude` | Sonnet · high |
| **1b** `SessionStore.LastBookedByNode` (ports + pgstore-Query + Fakes) — **Docker** | `claude` | Sonnet · high |
| **1c** `StatsComputer.CurrentStreak` (fensterloser Streak-Reader) | `claude` | Sonnet · high |
| 2 `usecase.WorktimeStatus` (Komposition + Double-Count-Fix + `CurrentStreak`) | `claude` | Sonnet · high |
| 3 Route + DTO + `handleWorktimeStatus` + **main.go-Wiring** + Routen-Test | `claude` | Sonnet · high |
| 4 `apiclient.WorktimeStatus` + `GetWorktimeStatus` | `claude` | Sonnet · medium |
| **4b** `usecase.NodeMRU` + `GET /api/v1/nodes/mru` + Handler + Wiring + `apiclient.NodeMRU` | `claude` | Sonnet · high |
| 5 `internal/statuscache` (atomarer Cache + Freshness-Prädikate) | `claude` | Sonnet · medium |
| 6 `internal/tmuxopts` (Parser + Palette + MaxStreak) | `claude` | Sonnet · medium |
| 7 `flow worktime status` CLI (fresh/stale/expired, Exit 0) | `claude` | Sonnet · high |
| 8 `flow worktime stop` CLI + Picker (Kaskade, konsumiert `NodeMRU`) | `claude` | Sonnet · high |
| 9 Wiring-Gate (`make ci` + Live-Smoke + Composition-Root-Verify + dotfiles-Notiz) | `claude` | Sonnet · high |
| jedes Task-Review | `code-searcher` | Haiku · high |
| Slice-Ende: Whole-Branch-Review | `codex-second-opinion` + `claude` | Opus · xhigh |

**Protokoll pro Task:** (1) Dispatch Implementer mit wörtlichem Task-Text + Global-Constraints + Dossier + „Branch `tmux-status`, Worktree `/Users/msoent/SourceCode/serverkraken/flow-tmux-status`; Tests/`make ci` SYNCHRON foreground; erst `git add -A`, dann `make ci` mit `timeout: 600000`; nie zwei `make ci` parallel". (2) Orchestrator verifiziert `git log --oneline -3` + `git diff --stat HEAD~1`. (3) Dispatch `code-searcher`-Review mit Task-Text + Commit-Range. Rejected/Critical → Fix-Dispatch an denselben Implementer; Minor darf der Orchestrator selbst fixen. (4) Ledger fortschreiben.

**Protokoll Slice-Ende:** `make ci` grün → Rest-Sweep über `git diff --name-only rebuild..HEAD` (verwaiste Symbole? Double-Count? Exit-0-Garantie im Status-Pfad? SSE beim Stop?) → Whole-Branch-Review → **Soenne-Live-Gate** (tmux-Segment tickt lokal, Offline dim, `stop` bucht korrekt) → Auto-Memory + flow-Mirror.

---

### Task 1: `internal/statusline` — purer Renderer (Port von `domain/status.go`)

**Files:**
- Create: `internal/statusline/palette.go` (`StatusPalette` + `DefaultStatusPalette` + `Dimmed`)
- Create: `internal/statusline/segment.go` (`Snapshot`, `WeekDay`, `DayOffInfo`, `BuildStatusSegment`, `statusBanner`, `activeSessionParts`, `monthBurndownPart`)
- Create: `internal/statusline/pacedots.go` (`buildPaceDots`, `classify`, `glyph`, `KindStatusColor`)
- Test: `internal/statusline/segment_test.go`, `internal/statusline/pacedots_test.go`

**Interfaces / Produces (für Tasks 6/7):**
- `statusline.Snapshot` — der eine Render-Input (aus dem DTO + lokale `Now` + `Palette` + `MaxStreakMin`).
- `statusline.StatusPalette` + `DefaultStatusPalette()` + `(StatusPalette).Dimmed()` (alle Farb-Slots → Dim, für den Stale-Pfad).
- `func BuildStatusSegment(Snapshot) string` — der tmux `status-right`-String; `""` wenn heute nichts, keine Wochenaktivität, kein Frei.

**Zustände (dieser „UI"):** leerer Tag (→ `""`), laufender Timer (▶ + ETA + ▶!-Streak-Warnung), Ziel erreicht (✓ + grün), Frei-Banner, Offline/Dim (via `Dimmed()`-Palette, Task 7). „lang/375px/mobil" ist für ein tmux-Segment N/A; der einzige unbrechbare Pfad ist das Frei-`Label` — Port-Verhalten (nicht truncaten) wird 1:1 übernommen (Entscheidung #3: Labels ungekürzt, tmux kürzt `status-right` rechts).

- [ ] **Step 0: rg-Verifikation** — `rg -n "func BuildStatusSegment|func DefaultStatusPalette|activeSessionParts|monthBurndownPart|KindStatusColor" /Users/msoent/SourceCode/serverkraken/flow/internal/domain/status.go` (Port-Quelle lesen). `rg -n "KindHoliday|KindVacation|KindSick" internal/domain/dayoff.go` (Kind-Konstanten im Arbeitsrepo — Bestand gewinnt).
- [ ] **Step 1: Failing Tests** — die alte Suite `status_test.go` auf die `Snapshot`-Form portieren. Repräsentativ (der Implementer portiert die **volle** Suite: Empty/IdleHit/IdleMissed/RunningColors/WayOverRed/StreakWarning-▶!/ETA/ETACrossesMidnight/DayOffBanner+PerKind/StreakAt3/BurndownArrows/DayOffOnly/NegativeElapsed/PaceDots-Hit-Missed-Running/PaceDots-DayOffPerKind/WeekendsSkipped/AllMissedEmpty + **NEU: Dimmed-Palette-Rendering** + **NEU: RunningAtZeroRendersNonEmpty** — `Running:true`, `LoggedMin:0`, `ActiveStart==Now`, Week leer → NICHT `""`, zeigt `▶ 0:00` (Finding #7)):
```go
package statusline_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/statusline"
)

func pal() statusline.StatusPalette { return statusline.DefaultStatusPalette() }

func TestBuildStatusSegment_EmptyDayReturnsEmpty(t *testing.T) {
	in := statusline.Snapshot{
		Now: time.Date(2026, 4, 29, 14, 30, 0, 0, time.Local), TargetMin: 480, Palette: pal(),
	}
	if got := statusline.BuildStatusSegment(in); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestBuildStatusSegment_IdleHit(t *testing.T) {
	in := statusline.Snapshot{
		Now: time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local),
		LoggedMin: 480, TargetMin: 480, Palette: pal(),
	}
	got := statusline.BuildStatusSegment(in)
	if !strings.Contains(got, "‖ 08:00") || !strings.Contains(got, "✓") {
		t.Errorf("idle hit should show '‖ 08:00' + '✓': %q", got)
	}
}

func TestBuildStatusSegment_RunningExcludesLiveTailFromLoggedButBankerIncludesIt(t *testing.T) {
	// loggedMin = completed only (240). ActiveStart 30 min ago. Banner total = 270 min = 4:30.
	now := time.Date(2026, 4, 29, 14, 30, 0, 0, time.Local)
	in := statusline.Snapshot{
		Now: now, LoggedMin: 240, TargetMin: 480, Running: true,
		ActiveStart: now.Add(-30 * time.Minute), Palette: pal(),
	}
	got := statusline.BuildStatusSegment(in)
	if !strings.Contains(got, "⏱ 04:30") { // banner = logged+tail
		t.Errorf("banner should extrapolate tail to 04:30: %q", got)
	}
	if !strings.Contains(got, "▶ 0:30") { // running session marker
		t.Errorf("running marker should be 0:30: %q", got)
	}
}

func TestBuildStatusSegment_ETACrossesMidnight(t *testing.T) {
	now := time.Date(2026, 4, 29, 6, 30, 0, 0, time.Local)
	in := statusline.Snapshot{
		Now: now, LoggedMin: 0, TargetMin: 480, Running: true,
		ActiveStart: time.Date(2026, 4, 28, 22, 0, 0, 0, time.Local), Palette: pal(),
	}
	got := statusline.BuildStatusSegment(in)
	if !strings.Contains(got, "→08:00") || strings.Contains(got, "→06:00") {
		t.Errorf("ETA must clamp to today midnight (→08:00), not →06:00: %q", got)
	}
}

func TestBuildStatusSegment_DimmedPaletteAllSlotsDim(t *testing.T) {
	now := time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local)
	base := pal()
	in := statusline.Snapshot{
		Now: now, LoggedMin: 480, TargetMin: 480, Streak: 5, Palette: base.Dimmed(),
	}
	got := statusline.BuildStatusSegment(in)
	if strings.Contains(got, base.Green) || strings.Contains(got, base.Cyan) {
		t.Errorf("dimmed render must use no live colours: %q", got)
	}
	if !strings.Contains(got, base.Dim) {
		t.Errorf("dimmed render should carry the Dim colour: %q", got)
	}
}
```
- [ ] **Step 2: Laufen lassen** — `go test ./internal/statusline/...` → FAIL (Paket fehlt).
- [ ] **Step 3: `palette.go`** (Port, Slot-Semantik-Kommentar aus der Quelle übernehmen):
```go
package statusline

// StatusPalette is the colour set used by tmux #[fg=...] markers in the
// status-right segment. Hex codes match the tokyonight defaults flow ships.
// Ported from the old domain/status.go; adapters override slots from tmux @tn_*.
type StatusPalette struct {
	Green, Yellow, Red, Cyan, Blue, Purple, Orange, Dim string
}

// DefaultStatusPalette returns the tokyonight defaults.
func DefaultStatusPalette() StatusPalette {
	return StatusPalette{
		Green: "#9ece6a", Yellow: "#e0af68", Red: "#f7768e", Cyan: "#7dcfff",
		Blue: "#7aa2f7", Purple: "#bb9af7", Orange: "#ff9e64", Dim: "#565f89",
	}
}

// Dimmed returns a copy whose every colour slot is Dim — the stale/offline
// render path feeds this so the whole segment reads as one muted "last known"
// snapshot (Spec §2 Stale+Dim). One-place mapping so a slot never leaks a live
// colour on the offline path.
func (p StatusPalette) Dimmed() StatusPalette {
	return StatusPalette{
		Green: p.Dim, Yellow: p.Dim, Red: p.Dim, Cyan: p.Dim,
		Blue: p.Dim, Purple: p.Dim, Orange: p.Dim, Dim: p.Dim,
	}
}
```
- [ ] **Step 4: `segment.go`** — `Snapshot` + `BuildStatusSegment` + Helfer. Port aus `domain/status.go`, Inputs aus dem DTO statt aus `domain.Day`/`WeekDay`/`MonthBurndownReport`. **Banner-Schwellen −2 h/+4 h und die Midnight-Clamp-Semantik 1:1 übernehmen:**
```go
package statusline

import (
	"fmt"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// Snapshot is everything BuildStatusSegment needs — built by the CLI from the
// worktimeStatusDTO plus the local clock, palette and MaxStreakMin.
type Snapshot struct {
	Now         time.Time
	LoggedMin   int  // COMPLETED today, excl. the running session (client extrapolates)
	TargetMin   int  // 0 = no target → idle-hit is trivially true
	Running     bool
	ActiveStart time.Time // zero when !Running
	DayOff      *DayOffInfo
	Week        []WeekDay
	Streak      int
	SaldoMin    int // monthly burndown saldo
	SaldoTarget int // burndown targetMin; 0 → no saldo marker
	Palette     StatusPalette
	MaxStreakMin int // active-session warning threshold (yellow at N, red at 2N); 0 disables
}

// DayOffInfo is today's day-off (banner). Kind drives the ● colour.
type DayOffInfo struct {
	Kind  domain.Kind
	Label string
}

// WeekDay is one Mon–Fri pace-dot input (from week[]).
type WeekDay struct {
	LoggedMin  int
	TargetMin  int
	Weekday    time.Weekday
	IsToday    bool
	DayOffKind domain.Kind // "" = none
}

const (
	bannerApproachingThreshold   = 2 * time.Hour // yellow "Endspurt" once target in sight
	bannerOvertimeAlertThreshold = 4 * time.Hour // red only on a truly excessive overrun
)

// BuildStatusSegment renders the tmux status-right string. "" when nothing was
// tracked today, no week activity exists and no day-off is set.
func BuildStatusSegment(in Snapshot) string {
	logged := time.Duration(in.LoggedMin) * time.Minute
	target := time.Duration(in.TargetMin) * time.Minute
	var tail time.Duration
	if in.Running && !in.ActiveStart.IsZero() {
		tail = clampedElapsed(in.Now, in.ActiveStart)
	}
	total := logged + tail
	dots := buildPaceDots(in.Week, in.Running, in.Palette)
	// Empty only when NOTHING is happening: no time today, no week activity, no
	// day-off AND no running timer. The !in.Running guard is a deliberate
	// deviation from the old port (Finding #7): a session started on a weekend
	// at the very first tick (elapsed≈0, dots="" because weekends are skipped)
	// would otherwise blank a genuinely running timer. A running timer is
	// "tracked" (Spec §2 empty-criteria), so it must render.
	if total == 0 && dots == "" && in.DayOff == nil && !in.Running {
		return ""
	}
	achieved := target == 0 || total >= target
	icon, mainAttr := statusBanner(in.Running, total, target, achieved, in.Palette)

	var parts []string
	if in.DayOff != nil {
		parts = append(parts, fmt.Sprintf("#[fg=%s]● %s#[default]",
			KindStatusColor(in.DayOff.Kind, in.Palette), in.DayOff.Label))
	}
	parts = append(parts, fmt.Sprintf("#[fg=%s]%s %02d:%02d#[default]",
		mainAttr, icon, int(total.Hours()), int(total.Minutes())%60))
	if in.Running && !in.ActiveStart.IsZero() {
		parts = append(parts, activeSessionParts(in, logged, target, achieved)...)
	}
	if achieved && total > 0 {
		parts = append(parts, fmt.Sprintf("#[fg=%s,bold]✓#[default]", in.Palette.Green))
	}
	if dots != "" {
		parts = append(parts, dots)
	}
	if in.Streak >= 3 {
		parts = append(parts, fmt.Sprintf("#[fg=%s]Streak %d#[default]", in.Palette.Green, in.Streak))
	}
	parts = append(parts, monthBurndownPart(in.SaldoMin, in.SaldoTarget, in.Palette)...)
	return strings.Join(parts, " ")
}

// clampedElapsed is (now - start) with start floored to today's midnight and a
// negative result floored to 0 — a session started yesterday reports only
// today's portion; a start in the future reports 0.
func clampedElapsed(now, start time.Time) time.Duration {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if start.Before(midnight) {
		start = midnight
	}
	if e := now.Sub(start); e > 0 {
		return e
	}
	return 0
}

func statusBanner(running bool, total, target time.Duration, achieved bool, p StatusPalette) (icon, attr string) {
	if running {
		switch {
		case total >= target+bannerOvertimeAlertThreshold:
			return "⏱", p.Red + ",bold"
		case achieved:
			return "⏱", p.Green + ",bold"
		case total >= target-bannerApproachingThreshold:
			return "⏱", p.Yellow + ",bold"
		default:
			return "⏱", p.Cyan + ",bold"
		}
	}
	if achieved && total > 0 {
		return "‖", p.Green
	}
	return "‖", p.Dim
}

func activeSessionParts(in Snapshot, logged, target time.Duration, achieved bool) []string {
	midnight := time.Date(in.Now.Year(), in.Now.Month(), in.Now.Day(), 0, 0, 0, 0, in.Now.Location())
	start := in.ActiveStart
	if start.Before(midnight) {
		start = midnight
	}
	elapsed := in.Now.Sub(start)
	if elapsed < 0 {
		elapsed = 0
	}
	streakColor, glyph := in.Palette.Dim, "▶"
	minutes := int(elapsed.Minutes())
	switch {
	case in.MaxStreakMin > 0 && minutes >= 2*in.MaxStreakMin:
		streakColor, glyph = in.Palette.Red, "▶!"
	case in.MaxStreakMin > 0 && minutes >= in.MaxStreakMin:
		streakColor, glyph = in.Palette.Yellow, "▶!"
	}
	out := []string{fmt.Sprintf("#[fg=%s]%s %d:%02d#[default]",
		streakColor, glyph, int(elapsed.Hours()), int(elapsed.Minutes())%60)}
	if !achieved {
		etaT := start.Add(target - logged) // same clamped start as elapsed
		out = append(out, fmt.Sprintf("#[fg=%s]→%s#[default]", in.Palette.Dim, etaT.Format("15:04")))
	}
	return out
}

// monthBurndownPart renders ▲/▼ monthly saldo. Nothing when |saldo| < 1h or no
// target. Hours are ROUNDED (a 1h59m surplus is "▲ +2h", not "▲ +1h").
func monthBurndownPart(saldoMin, targetMin int, p StatusPalette) []string {
	if targetMin == 0 {
		return nil
	}
	saldo := time.Duration(saldoMin) * time.Minute
	const min = time.Hour
	switch {
	case saldo >= min:
		return []string{fmt.Sprintf("#[fg=%s]▲ +%dh#[default]", p.Green, int(saldo.Round(time.Hour).Hours()))}
	case saldo <= -min:
		return []string{fmt.Sprintf("#[fg=%s]▼ -%dh#[default]", p.Yellow, int((-saldo).Round(time.Hour).Hours()))}
	}
	return nil
}
```
- [ ] **Step 5: `pacedots.go`** — Klassifikation aus dem DTO (kein `domain.Day`); die Empty-Suppression (alle missed → `""`) und die Kind-Farben aus der Quelle. **Der `Running`-Parameter kommt vom Top-Level-Flag** (NICHT blind aus `IsToday`) — sonst zählte ein leerer heutiger Tag als „running" und bräche die Empty-Ausgabe-Garantie:
```go
package statusline

import (
	"fmt"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

type paceDotKind int

const (
	paceMissed paceDotKind = iota
	paceHit
	paceRunning
	paceDayOff
)

// buildPaceDots renders Mon–Fri dots. "" when no weekday has any accounted slot
// (all missed) — avoids a stray dim row at the start of an empty week.
func buildPaceDots(week []WeekDay, running bool, p StatusPalette) string {
	var parts []string
	any := false
	for _, d := range week {
		if d.Weekday == time.Saturday || d.Weekday == time.Sunday {
			continue
		}
		k := classify(d, running)
		if k != paceMissed {
			any = true
		}
		parts = append(parts, fmt.Sprintf("#[fg=%s]%s#[default]", paceColor(k, d.DayOffKind, p), glyph(k)))
	}
	if !any {
		return ""
	}
	return strings.Join(parts, "")
}

func classify(d WeekDay, running bool) paceDotKind {
	if d.DayOffKind != "" {
		return paceDayOff
	}
	if d.TargetMin > 0 && d.LoggedMin >= d.TargetMin {
		return paceHit
	}
	if d.IsToday && running {
		return paceRunning
	}
	return paceMissed
}

func glyph(k paceDotKind) string {
	if k == paceMissed {
		return "○" // ○
	}
	return "●" // ●
}

func paceColor(k paceDotKind, kind domain.Kind, p StatusPalette) string {
	switch k {
	case paceHit:
		return p.Green
	case paceRunning:
		return p.Cyan
	case paceDayOff:
		return KindStatusColor(kind, p)
	}
	return p.Dim
}

// KindStatusColor maps a day-off Kind onto a palette slot: Holiday→Blue
// (info/scheduled), Vacation→Purple (identity), Sick→Orange (pending);
// every other kind → Dim. Ported from the old domain/status.go.
func KindStatusColor(k domain.Kind, p StatusPalette) string {
	switch k {
	case domain.KindHoliday:
		return p.Blue
	case domain.KindVacation:
		return p.Purple
	case domain.KindSick:
		return p.Orange
	}
	return p.Dim
}
```
- [ ] **Step 6: grün + Commit**
```bash
git add -A
go test ./internal/statusline/... -race
git commit -m "feat(statusline): purer tmux-Status-Renderer — Port von domain/status.go (Banner/▶/ETA/Pace-Dots/Dim)"
```
Expected: PASS. Kein `make ci` nötig (isoliertes neues Paket) — der Task-Reviewer/Gate deckt `make ci` ab; wer will, läuft es hier schon.

---

### Task 1b: `SessionStore.LastBookedByNode` — owner-scoped MRU-Query (Port + pgstore + Fakes) · **Docker**

Server-Reader für die exakte Stop-Picker-MRU (Entscheidung Soenne: dedizierter Server-Support statt Client-Heuristik über die letzten 100 Sessions). Konsument: Task 4b (`usecase.NodeMRU`). **Keine Migration** — reine read-only Query gegen `work_sessions`.

**Files:**
- Modify: `internal/ports/ports.go` (`SessionStore`-Interface + Methode)
- Modify: `internal/adapter/pgstore/sessions.go` (Query-Implementierung)
- Modify: `internal/testutil/fakes.go` (`FakeSessionStore.LastBookedByNode`)
- Modify: **jede weitere `SessionStore`-Fake** (Compiler-geführt — u. a. die LOKALE `fakeSessionStore` in `internal/usecase/stats_computer_test.go`)
- Test: `internal/adapter/pgstore/sessions_test.go` (testcontainer)

**Interfaces / Produces (für Task 4b):**
- **`ports.SessionStore.LastBookedByNode(ctx context.Context, ownerID string) (map[string]time.Time, error)`** — owner-scoped; für jeden Node mit ≥1 **gestoppter, gebuchter** Session der neueste `start_at`. Laufende (`stop_at NULL`) und ungebuchte (`node_id NULL`) Sessions zählen NICHT.

- [ ] **Step 0: rg-Verifikation** — `rg -n "SessionStore interface" internal/ports/ports.go`; `rg -n "const sessCols|func \(s \*SessionStore\) List\(|s.pool.Query" internal/adapter/pgstore/sessions.go`; `rg -n "type FakeSessionStore|func \(s \*FakeSessionStore\) Running|m map\[string\]domain.WorkSession" internal/testutil/fakes.go`; `rg -rn "func.*fakeSessionStore\) Running|SessionStore = " internal/usecase/*_test.go` (jede Inline-Fake); `rg -n "func Test.*SessionStore|newSessionStore|testcontainer|StartPostgres|pgtest" internal/adapter/pgstore/sessions_test.go` (Testcontainer-Muster — **Bestandsnamen übernehmen**).
- [ ] **Step 1: Failing Test** (testcontainer; `DOCKER_HOST` Podman-Socket; Muster der Bestand-`SessionStore`-Tests):
```go
func TestSessionStore_LastBookedByNode(t *testing.T) {
	ctx, store := newSessionStore(t) // Bestand-Helper — echten Namen per rg (Step 0)
	n1, n2 := "n1", "n2"
	mk := func(id, owner string, node *string, start time.Time, stopped bool) {
		ws := domain.WorkSession{ID: id, OwnerID: owner, NodeID: node, Start: start}
		if stopped {
			s := start.Add(time.Hour)
			ws.Stop = &s
		}
		if _, err := store.Create(ctx, ws); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	mk("a", "u1", &n1, base, true)                    // n1 older
	mk("b", "u1", &n1, base.AddDate(0, 0, 5), true)   // n1 newest → this start wins
	mk("c", "u1", &n2, base.AddDate(0, 0, 2), true)   // n2
	mk("d", "u1", &n1, base.AddDate(0, 0, 9), false)  // running → ignored
	mk("e", "u1", nil, base.AddDate(0, 0, 9), true)   // unbooked → ignored
	mk("f", "u2", &n1, base.AddDate(0, 0, 9), true)   // other owner → ignored

	got, err := store.LastBookedByNode(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 nodes, got %d (%v)", len(got), got)
	}
	if !got[n1].Equal(base.AddDate(0, 0, 5)) {
		t.Errorf("n1 last-booked = %v, want %v (newest stopped booked start)", got[n1], base.AddDate(0, 0, 5))
	}
	if !got[n2].Equal(base.AddDate(0, 0, 2)) {
		t.Errorf("n2 last-booked = %v", got[n2])
	}
	// Owner-scope: u2 sees only its own.
	if g2, _ := store.LastBookedByNode(ctx, "u2"); len(g2) != 1 || !g2[n1].Equal(base.AddDate(0, 0, 9)) {
		t.Errorf("owner-scope leak: %v", g2)
	}
}
```
- [ ] **Step 2: Laufen lassen** — FAIL (Methode fehlt; Fakes ggf. Compile-Fehler).
- [ ] **Step 3: pgstore-Query**:
```go
// LastBookedByNode returns, per node the owner has ever booked a STOPPED session
// to, the newest such session's start_at. Owner-scoped, read-only. Running
// (stop_at NULL) and unbooked (node_id NULL) sessions are excluded. Used by the
// stop-picker MRU ranking (usecase.NodeMRU) — exact, not a client heuristic.
func (s *SessionStore) LastBookedByNode(ctx context.Context, ownerID string) (map[string]time.Time, error) {
	const q = `SELECT node_id, MAX(start_at) FROM work_sessions
WHERE owner_id=$1 AND node_id IS NOT NULL AND stop_at IS NOT NULL
GROUP BY node_id`
	rows, err := s.pool.Query(ctx, q, ownerID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: last booked by node: %w", err)
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var id string
		var t time.Time
		if err := rows.Scan(&id, &t); err != nil {
			return nil, fmt.Errorf("pgstore: scan last booked: %w", err)
		}
		out[id] = t
	}
	return out, rows.Err()
}
```
- [ ] **Step 4: Port + alle Fakes** — Methode ans `SessionStore`-Interface (Doku wie oben). `internal/testutil/fakes.go`:
```go
func (s *FakeSessionStore) LastBookedByNode(_ context.Context, ownerID string) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	for _, ws := range s.m { // field name per Step 0
		if ws.OwnerID != ownerID || ws.NodeID == nil || ws.Stop == nil {
			continue
		}
		if cur, ok := out[*ws.NodeID]; !ok || ws.Start.After(cur) {
			out[*ws.NodeID] = ws.Start
		}
	}
	return out, nil
}
```
  Dann `go build ./... ./internal/...` — der Compiler listet jede weitere `SessionStore`-Fake (u. a. die Inline-`fakeSessionStore` in `stats_computer_test.go`); überall trivial ergänzen (leere Map genügt, wo Streak/MRU nicht geprüft wird).
- [ ] **Step 5: grün + Commit** (**Docker/Podman-Socket, `timeout: 600000`**):
```bash
git add -A
go build ./... && go test ./internal/adapter/pgstore/... ./internal/usecase/... -race   # DOCKER_HOST=<podman-socket>
git commit -m "feat(pgstore): SessionStore.LastBookedByNode — owner-scoped MRU-Query (kein Migration)"
```

---

### Task 1c: `StatsComputer.CurrentStreak` — fensterloser Streak-Reader

Server-Reader für den Segment-Streak (Entscheidung Soenne: dedizierter fensterloser Reader statt `RangeStats("month")`, das einen über den Monatsanfang reichenden Streak am 1. kürzen würde). Semantik wie der alte `CurrentStreak` (`/Users/msoent/SourceCode/serverkraken/flow/internal/usecase/stats_computer.go` — `c.Aggregate(History()).Streak` über die GESAMTE Historie). Konsument: Task 2 (`usecase.WorktimeStatus`). **Keine neue Store-Methode** (`SessionStore.List(from)` genügt).

**Files:**
- Create: `internal/usecase/current_streak.go` (Methode auf `StatsComputer` — eigenes File, „keine Monolithen")
- Test: `internal/usecase/current_streak_test.go`

**Interfaces / Produces (für Task 2):**
- **`func (c StatsComputer) CurrentStreak(ctx context.Context, ownerID string) (int, error)`** — owner-scoped; aktueller Werktag-Hit-Streak rückwärts vom neuesten relevanten Werktag bis zum ersten Miss; Wochenenden/Frei-Tage werden übersprungen (nicht als Miss gewertet — via `res.IsWorkday`/`listOffs`, wie `RangeStats`). Fensterlos über die geladene Historie, mit 3-Jahres-Sicherheitskappe gegen einen pathologisch alten Ausreißer-Datensatz.

- [ ] **Step 0: rg-Verifikation** — `rg -n "func (c StatsComputer) RangeStats|func (c StatsComputer) resolver|func (c StatsComputer) countsTowardFn|func startOfDay|domain.AggregateRange|domain.BuildDayRecords" internal/usecase/stats_computer.go`; `rg -n "func AggregateRange" internal/domain/*.go` (Signatur EXAKT übernehmen — Bestand gewinnt). Alt-Referenz lesen: `rg -n "CurrentStreak" /Users/msoent/SourceCode/serverkraken/flow/internal/usecase/stats_computer.go`.
- [ ] **Step 1: Failing Test** — der entscheidende Fall gegenüber `RangeStats("month")`: ein Streak, der **über den Monatsanfang** reicht, darf NICHT gekürzt werden:
```go
func TestCurrentStreak_CrossesMonthBoundary(t *testing.T) {
	// "today" = 2026-07-02 (Do). Hits on 06-30 (Di), 07-01 (Mi), 07-02 (Do) →
	// streak 3, spanning the June→July boundary that RangeStats("month") cuts.
	now := time.Date(2026, 7, 2, 18, 0, 0, 0, time.UTC)
	uc, sessions := newStreakStats(t, now) // helper wires StatsComputer over fakes; 8h default target
	for _, d := range []time.Time{
		time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC),
	} {
		stop := d.Add(8 * time.Hour)
		_, _ = sessions.Create(context.Background(), domain.WorkSession{ID: d.Format("id-2006-01-02"), OwnerID: "u1", Start: d, Stop: &stop})
	}
	got, err := uc.CurrentStreak(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got != 3 {
		t.Fatalf("cross-month streak = %d, want 3 (no month-window cut)", got)
	}
}
```
  Der Implementer ergänzt: leere Historie → 0; ein Miss (Werktag unter Ziel) bricht den Streak; ein Frei-Tag/Wochenende dazwischen bricht ihn NICHT.
- [ ] **Step 2: FAIL → Step 3: implementieren** `current_streak.go` (Muster exakt aus `RangeStats`, nur das Fenster ist datengetrieben statt Monat):
```go
package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// CurrentStreak returns the owner's current workday-hit streak — walking back
// from the newest relevant workday to the first miss, weekends/day-offs skipped
// (not counted as misses). Windowless (Entscheidung Soenne): unlike
// RangeStats("month") it never cuts a streak at a month boundary. It loads the
// owner's full history and aggregates from the earliest session's day; a 3-year
// safety cap keeps the AggregateRange day-loop bounded against a stray ancient
// record (a >3-year perfect-attendance streak is not achievable). Semantics
// mirror the old repo's StatsComputer.CurrentStreak. Owner-scoped.
func (c StatsComputer) CurrentStreak(ctx context.Context, ownerID string) (int, error) {
	now := c.Clock.Now().In(c.loc())
	sessions, err := c.Sessions.List(ctx, ownerID, time.Time{}) // full history
	if err != nil {
		return 0, err
	}
	from := startOfDay(now)
	for _, s := range sessions {
		if d := startOfDay(s.Start.In(c.loc())); d.Before(from) {
			from = d
		}
	}
	if floor := startOfDay(now).AddDate(-3, 0, 0); from.Before(floor) {
		from = floor // 3-year safety floor (avoid shadowing builtin cap)
	}
	to := startOfDay(now).AddDate(0, 0, 1)
	res, offs, err := c.resolver(ctx, ownerID, from, to.AddDate(0, 0, -1))
	if err != nil {
		return 0, err
	}
	countsToward, err := c.countsTowardFn(ctx, ownerID)
	if err != nil {
		return 0, err
	}
	recs := domain.BuildDayRecords(sessions, now, res.For, countsToward)
	listOffs := func(f, t time.Time) []domain.DayOff {
		var in []domain.DayOff
		for _, o := range offs {
			if !o.Date.Before(f) && !o.Date.After(t) {
				in = append(in, o)
			}
		}
		return in
	}
	return domain.AggregateRange(recs, from, to, res.IsWorkday, res.For, listOffs).Streak, nil
}
```
  (`resolver`/`countsTowardFn`/`startOfDay`/das `listOffs`-Muster sind Bestand aus `stats_computer.go` — Signaturen per Step 0 verifizieren. Falls `AggregateRange` eine andere Parameterreihenfolge hat: Bestand aus `RangeStats:236` 1:1 übernehmen.)
- [ ] **Step 4: grün + Commit**
```bash
git add -A && go test ./internal/usecase/... -race 2>&1 | tail -20
git commit -m "feat(usecase): StatsComputer.CurrentStreak — fensterloser Streak-Reader (kein Monatsfenster-Schnitt)"
```

---

### Task 2: `usecase.WorktimeStatus` — Komposition + Double-Count-Fix + Result

**Files:**
- Create: `internal/usecase/worktime_status.go`
- Test: `internal/usecase/worktime_status_test.go`

**Interfaces / Produces (für Task 3):**
- `usecase.WorktimeStatus{Stats StatsComputer; Running GetRunningSession; DayOffs ListDayOffs; Clock ports.Clock; Loc *time.Location}` — reine Komposition der bestehenden Reader, **keine neuen Store-Methoden**.
- `func (uc WorktimeStatus) Execute(ctx, ownerID string) (WorktimeStatusResult, error)`.
- `WorktimeStatusResult` + `WorktimeStatusWeekDay` (domain-typisiert; Handler mappt auf DTO).

**Zustände:** running/idle, Frei heute, Wochen-Kinds, Burndown ohne Ziel, laufende Session mit/ohne Node, Session über Mitternacht (Tail-Subtraktion clamped), leerer Tag.

- [ ] **Step 0: rg-Verifikation** — `rg -n "func (c StatsComputer) Today|func (c StatsComputer) Week|func (c StatsComputer) CurrentStreak|func (c StatsComputer) Burndown|func startOfDay" internal/usecase/*.go` (`CurrentStreak` ist aus Task 1c); `rg -n "func (uc GetRunningSession) Execute|func (uc ListDayOffs) Execute" internal/usecase/*.go`; `rg -n "TodaySummary|MonthBurndownReport|WeekDay struct" internal/{usecase,domain}/*.go`. **Bestand gewinnt** (`Loc`, `startOfDay` sind im selben Paket wiederverwendbar).
- [ ] **Step 1: Failing Test** — Fakes wie `stats_computer_test.go` (lokale `fakeSessionStore`/`fakeStatsSettings` oder `testutil.*`), `testutil.FakeClock`:
```go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func newWorktimeStatus(t *testing.T, now time.Time) (usecase.WorktimeStatus, *testutil.FakeSessionStore) {
	t.Helper()
	clk := testutil.FakeClock{T: now}
	sessions := testutil.NewFakeSessionStore()
	settings := testutil.NewFakeUserSettingsStore()
	dayoffs := testutil.NewFakeDayOffStore()
	nodes := testutil.NewFakeNodeStore()
	listDayOffs := usecase.ListDayOffs{Store: dayoffs, Settings: settings, Loc: time.UTC}
	stats := usecase.StatsComputer{Sessions: sessions, Settings: settings, DayOffs: listDayOffs, Clock: clk, Loc: time.UTC, Nodes: nodes}
	return usecase.WorktimeStatus{Stats: stats, Running: usecase.GetRunningSession{Sessions: sessions}, DayOffs: listDayOffs, Clock: clk, Loc: time.UTC}, sessions
}

// The running session's live tail must be subtracted out of loggedMin so the
// client (which re-adds it from activeStart) does not double-count it.
func TestWorktimeStatus_LoggedExcludesRunningTail(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) // Monday noon
	uc, sessions := newWorktimeStatus(t, now)
	start := now.Add(-30 * time.Minute)
	_, _ = sessions.Create(context.Background(), domain.WorkSession{ID: "s1", OwnerID: "u1", Start: start}) // running (Stop nil)
	res, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Running || res.ActiveID != "s1" {
		t.Fatalf("expected running s1, got running=%v id=%q", res.Running, res.ActiveID)
	}
	if res.Logged != 0 { // 30 min tail subtracted from the 30 min Today.Logged
		t.Fatalf("loggedMin should exclude the running tail, got %v", res.Logged)
	}
	if !res.ActiveStart.Equal(start) {
		t.Fatalf("activeStart = %v, want %v", res.ActiveStart, start)
	}
	// Asymmetry (dossier KRITISCH / Finding C3): today's WEEK entry INCLUDES the
	// live tail (server snapshot for pace-dot classification), UNLIKE top-level Logged.
	var todayWeek usecase.WorktimeStatusWeekDay
	for _, d := range res.Week {
		if d.IsToday {
			todayWeek = d
		}
	}
	if todayWeek.Logged < 30*time.Minute {
		t.Fatalf("today's week Logged must INCLUDE the running tail, got %v", todayWeek.Logged)
	}
}

// Finding #1: a session running ACROSS midnight was never counted by Today()
// (List filters start_at >= today-midnight), so its tail must NOT be subtracted
// — otherwise it eats OTHER completed same-day sessions. Here: 2h completed
// today + a session running since yesterday 22:00; loggedMin must stay 2h.
func TestWorktimeStatus_CrossMidnightRunningNotSubtracted(t *testing.T) {
	now := time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC) // Monday 08:00
	uc, sessions := newWorktimeStatus(t, now)
	completedStop := time.Date(2026, 6, 15, 2, 0, 0, 0, time.UTC)
	_, _ = sessions.Create(context.Background(), domain.WorkSession{ID: "done", OwnerID: "u1",
		Start: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Stop: &completedStop}) // 2h completed today
	_, _ = sessions.Create(context.Background(), domain.WorkSession{ID: "s1", OwnerID: "u1",
		Start: time.Date(2026, 6, 14, 22, 0, 0, 0, time.UTC)}) // running since yesterday 22:00
	res, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Logged != 2*time.Hour {
		t.Fatalf("cross-midnight run must not be subtracted; loggedMin want 2h, got %v", res.Logged)
	}
	if !res.ActiveStart.Equal(time.Date(2026, 6, 14, 22, 0, 0, 0, time.UTC)) {
		t.Fatalf("activeStart = %v", res.ActiveStart)
	}
}

// Owner-scoping (AGENTS.md hard rule): u2 must never see u1's running session.
func TestWorktimeStatus_OwnerScoped(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	uc, sessions := newWorktimeStatus(t, now)
	_, _ = sessions.Create(context.Background(), domain.WorkSession{ID: "s1", OwnerID: "u1", Start: now.Add(-time.Hour)})
	res, err := uc.Execute(context.Background(), "u2")
	if err != nil {
		t.Fatal(err)
	}
	if res.Running || res.ActiveID != "" {
		t.Fatalf("u2 must not see u1's session: running=%v id=%q", res.Running, res.ActiveID)
	}
}
```
Der Implementer ergänzt zusätzlich: idle (kein running → `ActiveID==""`, `Logged`=abgeschlossen), **laufende Session MIT Node → `res.ActiveNodeID` propagiert** (Finding C4), Frei heute (`res.DayOff != nil`, korrektes `Kind`/`Label`), Wochen-`DayOffKind` gesetzt, `Burndown.Target==0` durchgereicht, `Streak` aus `CurrentStreak` (Task 1c).
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: Implementieren** `worktime_status.go`:
```go
package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// WorktimeStatus composes the read models the tmux status segment needs into
// ONE owner-scoped snapshot: today (logged/target/running), the running session
// (id/start/node), the ISO week with per-day day-off kinds, the current streak
// and the monthly burndown. Pure composition of existing readers — NO new store
// methods (Spec §1). Mirrors the old usecase.StatusComposer.
type WorktimeStatus struct {
	Stats   StatsComputer
	Running GetRunningSession
	DayOffs ListDayOffs
	Clock   ports.Clock
	Loc     *time.Location
}

// WorktimeStatusResult is the domain-typed snapshot; the handler maps it to the
// wire DTO.
type WorktimeStatusResult struct {
	Date         time.Time
	Logged       time.Duration // COMPLETED today, running tail already subtracted
	Target       time.Duration
	Running      bool
	ActiveID     string
	ActiveStart  time.Time
	ActiveNodeID *string
	DayOff       *domain.DayOff // today's, or nil
	Week         []WorktimeStatusWeekDay
	Streak       int
	Burndown     domain.MonthBurndownReport
}

// WorktimeStatusWeekDay is one Mon..Sun row; Logged INCLUDES today's live tail
// (server snapshot, matches handleWeek), only for pace-dot classification.
type WorktimeStatusWeekDay struct {
	Date       time.Time
	Logged     time.Duration
	Target     time.Duration
	IsToday    bool
	DayOffKind domain.Kind // "" = none
}

func (uc WorktimeStatus) loc() *time.Location {
	if uc.Loc != nil {
		return uc.Loc
	}
	return time.Local
}

func (uc WorktimeStatus) Execute(ctx context.Context, ownerID string) (WorktimeStatusResult, error) {
	now := uc.Clock.Now().In(uc.loc())

	today, err := uc.Stats.Today(ctx, ownerID)
	if err != nil {
		return WorktimeStatusResult{}, err
	}
	week, err := uc.Stats.Week(ctx, ownerID, time.Time{})
	if err != nil {
		return WorktimeStatusResult{}, err
	}
	streak, err := uc.Stats.CurrentStreak(ctx, ownerID) // windowless (Task 1c), NOT RangeStats("month")
	if err != nil {
		return WorktimeStatusResult{}, err
	}
	burndown, err := uc.Stats.Burndown(ctx, ownerID)
	if err != nil {
		return WorktimeStatusResult{}, err
	}
	sess, running, err := uc.Running.Execute(ctx, ownerID)
	if err != nil {
		return WorktimeStatusResult{}, err
	}

	res := WorktimeStatusResult{
		Date: today.Date, Logged: today.Logged, Target: today.Target,
		Running: running, Streak: streak, Burndown: burndown,
	}

	// Subtract the running session's tail from Logged so it is COMPLETED-only —
	// but ONLY when Today() actually counted that tail. StatsComputer.Today()
	// loads sessions via SessionStore.List(from = today-midnight) (verified
	// `WHERE start_at >= $2`, pgstore/sessions.go:105), so a session that
	// STARTED BEFORE today (running across midnight) was never loaded and its
	// tail is NOT in Today().Logged — subtracting it would eat OTHER completed
	// same-day sessions and clamp real time to 0. The client re-adds the
	// midnight-clamped tail from ActiveStart in BOTH cases (Finding #1).
	if running {
		res.ActiveID = sess.ID
		res.ActiveStart = sess.Start
		res.ActiveNodeID = sess.NodeID
		if midnight := startOfDay(now); !sess.Start.Before(midnight) {
			res.Logged -= now.Sub(sess.Start) // start is today → Today() folded this tail in
			if res.Logged < 0 {
				res.Logged = 0
			}
		}
	}

	// One merged day-off read over the week span → today's banner + per-day kinds.
	var offs []domain.DayOff
	if len(week) > 0 {
		offs, err = uc.DayOffs.Execute(ctx, ownerID, week[0].Date, week[len(week)-1].Date)
		if err != nil {
			return WorktimeStatusResult{}, err
		}
	}
	byDay := make(map[string]domain.DayOff, len(offs))
	for _, o := range offs {
		byDay[o.Date.Format("2006-01-02")] = o
	}
	if o, ok := byDay[startOfDay(now).Format("2006-01-02")]; ok {
		res.DayOff = &o
	}
	res.Week = make([]WorktimeStatusWeekDay, 0, len(week))
	for _, d := range week {
		wd := WorktimeStatusWeekDay{Date: d.Date, Logged: d.Total(now), Target: d.Target, IsToday: d.IsToday}
		if o, ok := byDay[d.Date.Format("2006-01-02")]; ok {
			wd.DayOffKind = o.Kind
		}
		res.Week = append(res.Week, wd)
	}
	return res, nil
}
```
(Kein `clampedTail`-Helfer mehr — die Subtraktion ist bewusst inline und konditional auf `sess.Start >= today-midnight`, siehe Finding #1.)
- [ ] **Step 4: grün + Commit**
```bash
git add -A
go test ./internal/usecase/... -race 2>&1 | tail -20
git commit -m "feat(usecase): WorktimeStatus — aggregierter Status-Composer (today/week/streak/burndown/running, live-tail-Subtraktion)"
```

---

### Task 3: Route + DTO + `handleWorktimeStatus` + main.go-Wiring + Routen-Test

**Files:**
- Create: `internal/adapter/httpserver/worktime_status.go` (DTO + Handler)
- Modify: `internal/adapter/httpserver/server.go` (Feld `WorktimeStatus usecase.WorktimeStatus` + Route)
- Modify: `cmd/flow-server/main.go` (Konstruktion des Feldes)
- Test: `internal/adapter/httpserver/worktime_status_test.go`

**Zustände:** unauth (401 über `s.auth`), running/idle DTO-Shape, Frei heute, leerer Tag (DTO mit `week` gefüllt, `running:false`), Server-Fehler (500).

- [ ] **Step 0: rg-Verifikation** — `rg -n "func minutes|const dayFmt|func writeJSON|func userFrom" internal/adapter/httpserver/*.go`; `rg -n "mux.Handle(\"GET /api/v1/burndown\"|GetRunningSession usecase|ListDayOffs usecase|Stats usecase.StatsComputer" internal/adapter/httpserver/server.go`; `rg -n "Stats: usecase.StatsComputer|GetRunningSession:|ListDayOffs:|Clock:" cmd/flow-server/main.go`.
- [ ] **Step 1: Failing Test** — an `stats_test.go`'s `newStatsServerFull` anlehnen, aber mit dem neuen Feld:
```go
func TestHandleWorktimeStatus_ShapeAndAuth(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)}
	bus := sse.NewBus()
	settings := testutil.NewFakeUserSettingsStore()
	sessions := testutil.NewFakeSessionStore()
	dayoffs := testutil.NewFakeDayOffStore()
	nodes := testutil.NewFakeNodeStore()
	ids := &testutil.FakeIDGen{}
	listDayOffs := usecase.ListDayOffs{Store: dayoffs, Settings: settings, Loc: time.UTC}
	stats := usecase.StatsComputer{Sessions: sessions, Settings: settings, DayOffs: listDayOffs, Clock: clk, Loc: time.UTC, Nodes: nodes}
	srv := &httpserver.Server{
		Verifier: testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:   usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:      bus, Emitter: sse.NewEmitter(bus, &fakeActivityStore{}, ids, clk), Clock: clk,
		Stats:            stats,
		GetRunningSession: usecase.GetRunningSession{Sessions: sessions},
		ListDayOffs:      listDayOffs,
		WorktimeStatus: usecase.WorktimeStatus{Stats: stats, Running: usecase.GetRunningSession{Sessions: sessions}, DayOffs: listDayOffs, Clock: clk, Loc: time.UTC},
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// unauth → 401
	res, _ := http.Get(ts.URL + "/api/v1/worktime/status")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-auth want 401, got %d", res.StatusCode)
	}
	_ = res.Body.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/worktime/status", nil)
	req.Header.Set("Authorization", "Bearer x")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(res.Body).Decode(&got)
	for _, k := range []string{"date", "loggedMin", "targetMin", "running", "week", "streak", "burndown"} {
		if _, ok := got[k]; !ok {
			t.Errorf("DTO missing key %q", k)
		}
	}
}
```
  **Der Implementer ergänzt zusätzlich (Findings C5/C6):** (i) **running-Felder** — eine laufende Session vorseeden, dann prüfen, dass `activeSessionId`/`activeStart` gesetzt und `running:true` sind; (ii) **empty-day** — ohne Sessions ist `week` ein nicht-leeres Array (7 Einträge) und `running:false`; (iii) **DayOff-Shape** — einen Frei-Eintrag heute vorseeden, `dayOff.kind`/`dayOff.label` prüfen; (iv) **500** — einen `WorktimeStatus` mit einem Fake-Reader, der einen Fehler liefert (z. B. eine `Sessions`-Fake mit `List`-Fehler), Handler → 500; (v) **Handler-Owner-Scoping** — zwei Owner vorseeden (u1 laufende Session, u2 nichts), mit u2s Identität (`FakeVerifier{ID: …u2}`) requesten und prüfen, dass `running:false` (u2 sieht u1 nie). Owner-Isolation ist damit auf BEIDEN Ebenen (Usecase Task 2 + Handler hier) test-gedeckt.
- [ ] **Step 2: Laufen lassen** — FAIL (Feld/Route/Handler fehlen).
- [ ] **Step 3: DTO + Handler** `worktime_status.go`:
```go
package httpserver

import (
	"net/http"
	"time"
)

type worktimeStatusDTO struct {
	Date            string             `json:"date"`
	LoggedMin       int                `json:"loggedMin"`
	TargetMin       int                `json:"targetMin"`
	Running         bool               `json:"running"`
	ActiveSessionID string             `json:"activeSessionId,omitempty"`
	ActiveStart     string             `json:"activeStart,omitempty"`
	// omitempty: an unbooked running session omits activeNodeId (the wire example
	// shows it as null). Absent ≡ null ≡ unbooked — apiclient.WorktimeStatus
	// decodes both to a nil *string identically (Finding C7, consumer-verified).
	ActiveNodeID    *string            `json:"activeNodeId,omitempty"`
	DayOff          *wsDayOffDTO       `json:"dayOff"`
	Week            []wsWeekDayDTO     `json:"week"`
	Streak          int                `json:"streak"`
	Burndown        wsBurndownDTO      `json:"burndown"`
}

type wsDayOffDTO struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type wsWeekDayDTO struct {
	Date       string  `json:"date"`
	LoggedMin  int     `json:"loggedMin"`
	TargetMin  int     `json:"targetMin"`
	Workday    bool    `json:"workday"`
	IsToday    bool    `json:"isToday"`
	DayOffKind *string `json:"dayOffKind"`
}

type wsBurndownDTO struct {
	SaldoMin  int `json:"saldoMin"`
	TargetMin int `json:"targetMin"`
}

func (s *Server) handleWorktimeStatus(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	res, err := s.WorktimeStatus.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	dto := worktimeStatusDTO{
		Date:      res.Date.Format(dayFmt),
		LoggedMin: minutes(res.Logged),
		TargetMin: minutes(res.Target),
		Running:   res.Running,
		Streak:    res.Streak,
		Burndown:  wsBurndownDTO{SaldoMin: minutes(res.Burndown.Saldo), TargetMin: minutes(res.Burndown.Target)},
	}
	if res.Running {
		dto.ActiveSessionID = res.ActiveID
		dto.ActiveStart = res.ActiveStart.Format(time.RFC3339)
		dto.ActiveNodeID = res.ActiveNodeID
	}
	if res.DayOff != nil {
		dto.DayOff = &wsDayOffDTO{Kind: string(res.DayOff.Kind), Label: res.DayOff.Label}
	}
	dto.Week = make([]wsWeekDayDTO, 0, len(res.Week))
	for _, d := range res.Week {
		row := wsWeekDayDTO{
			Date: d.Date.Format(dayFmt), LoggedMin: minutes(d.Logged), TargetMin: minutes(d.Target),
			Workday: d.Date.Weekday() != time.Saturday && d.Date.Weekday() != time.Sunday,
			IsToday: d.IsToday,
		}
		if d.DayOffKind != "" {
			k := string(d.DayOffKind)
			row.DayOffKind = &k
		}
		dto.Week = append(dto.Week, row)
	}
	writeJSON(w, http.StatusOK, dto)
}
```
- [ ] **Step 4: server.go** — Feld ans Struct (bei den worktime-Usecases, z. B. nach `GetRunningSession`): `WorktimeStatus usecase.WorktimeStatus`. Route in `Routes()` neben den anderen worktime-GETs (nach `:164` `burndown`):
```go
mux.Handle("GET /api/v1/worktime/status", s.auth(http.HandlerFunc(s.handleWorktimeStatus)))
```
- [ ] **Step 5: main.go-Wiring** — nach dem `srv := &httpserver.Server{...}`-Literal (DRY: bestehende Sub-Usecases wiederverwenden statt den StatsComputer-Literal zu duplizieren):
```go
srv.WorktimeStatus = usecase.WorktimeStatus{
	Stats:   srv.Stats,
	Running: srv.GetRunningSession,
	DayOffs: srv.ListDayOffs,
	Clock:   clock,
	Loc:     time.Local,
}
```
- [ ] **Step 6: grün + Commit**
```bash
git add -A
go test ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git commit -m "feat(httpserver): GET /api/v1/worktime/status — aggregierter Status-Endpoint + main.go-Wiring"
```

---

### Task 4: `apiclient.WorktimeStatus` + `GetWorktimeStatus`

**Files:**
- Create: `internal/adapter/apiclient/worktime_status.go`
- Test: `internal/adapter/apiclient/worktime_status_test.go`

**Interfaces / Produces (für Tasks 7/8):**
- `apiclient.WorktimeStatus` (+ genestete `WSDayOff`/`WSWeekDay`/`WSBurndown`) — spiegelt `worktimeStatusDTO`.
- `func (c *Client) GetWorktimeStatus(ctx) (WorktimeStatus, error)` — GET `/api/v1/worktime/status`; der **2 s-Timeout** wird vom Aufrufer per `context.WithTimeout` gesetzt (der `Client.do` respektiert die Kontext-Deadline), NICHT hier hartkodiert.

- [ ] **Step 0: rg-Verifikation** — `rg -n "type Today struct|func (c \*Client) GetToday|func (c \*Client) do" internal/adapter/apiclient/{stats,client}.go`.
- [ ] **Step 1: Failing Test** — `httptest`-Server, der das DTO liefert, dekodiert korrekt:
```go
func TestGetWorktimeStatus_Decodes(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/worktime/status" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"date":"2026-07-08","loggedMin":312,"targetMin":480,"running":true,"activeSessionId":"s1","activeStart":"2026-07-08T13:05:00+02:00","week":[{"date":"2026-07-06","loggedMin":480,"targetMin":480,"workday":true,"isToday":false,"dayOffKind":null}],"streak":4,"burndown":{"saldoMin":130,"targetMin":9600}}`))
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")
	st, err := c.GetWorktimeStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.LoggedMin != 312 || !st.Running || st.ActiveSessionID != "s1" || st.Streak != 4 || len(st.Week) != 1 {
		t.Fatalf("bad decode: %+v", st)
	}
}
```
- [ ] **Step 2: FAIL → Step 3: implementieren** (Feldnamen/JSON-Tags EXAKT wie das DTO in Task 3):
```go
package apiclient

import (
	"context"
	"net/http"
)

// WorktimeStatus mirrors the server's worktimeStatusDTO (tmux status segment).
type WorktimeStatus struct {
	Date            string       `json:"date"`
	LoggedMin       int          `json:"loggedMin"`
	TargetMin       int          `json:"targetMin"`
	Running         bool         `json:"running"`
	ActiveSessionID string       `json:"activeSessionId"`
	ActiveStart     string       `json:"activeStart"`
	ActiveNodeID    *string      `json:"activeNodeId"`
	DayOff          *WSDayOff    `json:"dayOff"`
	Week            []WSWeekDay  `json:"week"`
	Streak          int          `json:"streak"`
	Burndown        WSBurndown   `json:"burndown"`
}

type WSDayOff struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type WSWeekDay struct {
	Date       string  `json:"date"`
	LoggedMin  int     `json:"loggedMin"`
	TargetMin  int     `json:"targetMin"`
	Workday    bool    `json:"workday"`
	IsToday    bool    `json:"isToday"`
	DayOffKind *string `json:"dayOffKind"`
}

type WSBurndown struct {
	SaldoMin  int `json:"saldoMin"`
	TargetMin int `json:"targetMin"`
}

// GetWorktimeStatus fetches the aggregated tmux status snapshot. The caller sets
// a short deadline via context (the status tick uses ~2s); do() honours it.
func (c *Client) GetWorktimeStatus(ctx context.Context) (WorktimeStatus, error) {
	var out WorktimeStatus
	err := c.do(ctx, http.MethodGet, "/api/v1/worktime/status", nil, &out)
	return out, err
}
```
- [ ] **Step 4: grün + Commit**
```bash
git add -A && go test ./internal/adapter/apiclient/... -race 2>&1 | tail -10
git commit -m "feat(apiclient): GetWorktimeStatus + WorktimeStatus-DTO"
```

---

### Task 4b: `usecase.NodeMRU` + `GET /api/v1/nodes/mru` + Handler + main.go-Wiring + `apiclient.NodeMRU`

Der exakte MRU-Server-Support (Entscheidung Soenne). **Form = eigener Endpoint** `GET /api/v1/nodes/mru` (NICHT ein `lastBookedAt`-Feld an `domain.Node` — das würde die Node-DTO durch WebUI/TUI/MCP rippeln; der eigene Endpoint stört **null** Bestands-DTOs). Owner-scoped, read-only. Konsument: Task 8 (Stop-Picker).

**Files:**
- Create: `internal/usecase/node_mru.go`
- Create: `internal/adapter/httpserver/node_mru.go` (DTO + Handler)
- Modify: `internal/adapter/httpserver/server.go` (Feld `NodeMRU usecase.NodeMRU` + Route)
- Modify: `cmd/flow-server/main.go` (Wiring)
- Create: `internal/adapter/apiclient/node_mru.go` (`NodeMRU`-DTO + `NodeMRU`-Methode)
- Test: `internal/usecase/node_mru_test.go` + `internal/adapter/httpserver/node_mru_test.go`

**Interfaces / Produces (für Task 8):**
- `usecase.NodeMRU{Sessions ports.SessionStore}` mit `Execute(ctx, ownerID) ([]NodeMRUEntry, error)` — `LastBookedByNode` (Task 1b) in eine **nach `LastBookedAt` absteigend sortierte** Liste. `NodeMRUEntry{NodeID string; LastBookedAt time.Time}`.
- `func (c *Client) NodeMRU(ctx) ([]NodeMRU, error)` (GET `/api/v1/nodes/mru`); `apiclient.NodeMRU{NodeID string json:"nodeId"; LastBookedAt string json:"lastBookedAt"}` (server-sortiert, newest-first).

**Zustände:** keine Buchungen → leere Liste (200 `[]`); unauth → 401; Server-Fehler → 500.

- [ ] **Step 0: rg-Verifikation** — `rg -n "LastBookedByNode" internal/ports/ports.go` (Task 1b vorhanden); `rg -n "mux.Handle\(\"GET /api/v1/nodes/resolve\"|mux.Handle\(\"GET /api/v1/nodes/bindings\"|GET /api/v1/nodes/\{id\}" internal/adapter/httpserver/server.go` (statische Node-Pfade werden VOR dem `{id}`-Wildcard registriert — `mru` genauso); `rg -n "func writeJSON|func userFrom" internal/adapter/httpserver/*.go`; `rg -n "func (c \*Client) do" internal/adapter/apiclient/client.go`.
- [ ] **Step 1: Failing Tests** — Usecase-Sortierung + Routen-Shape/Auth:
```go
// usecase test
func TestNodeMRU_SortedNewestFirst(t *testing.T) {
	sessions := testutil.NewFakeSessionStore()
	base := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	mk := func(id, node string, start time.Time) {
		stop := start.Add(time.Hour)
		_, _ = sessions.Create(context.Background(), domain.WorkSession{ID: id, OwnerID: "u1", NodeID: &node, Start: start, Stop: &stop})
	}
	mk("a", "n-old", base)
	mk("b", "n-new", base.AddDate(0, 0, 10))
	out, err := usecase.NodeMRU{Sessions: sessions}.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].NodeID != "n-new" || out[1].NodeID != "n-old" {
		t.Fatalf("want newest-first [n-new, n-old], got %+v", out)
	}
}
```
```go
// route test (an newStatsServerFull-Muster; Server bekommt NodeMRU wired)
func TestHandleNodeMRU_ShapeAndAuth(t *testing.T) {
	// … Server mit NodeMRU: usecase.NodeMRU{Sessions: sessions}, eine gebuchte gestoppte Session vorseeden …
	// unauth → 401; authed → 200, JSON-Array mit {"nodeId","lastBookedAt"} newest-first.
}
```
- [ ] **Step 2: FAIL → Step 3: `node_mru.go` (usecase)**:
```go
package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// NodeMRU ranks the owner's nodes by most-recently-booked (Spec §1 MRU support).
// Pure composition over SessionStore.LastBookedByNode — no new store method here.
type NodeMRU struct {
	Sessions ports.SessionStore
}

// NodeMRUEntry is one node's last booking time.
type NodeMRUEntry struct {
	NodeID       string
	LastBookedAt time.Time
}

func (uc NodeMRU) Execute(ctx context.Context, ownerID string) ([]NodeMRUEntry, error) {
	m, err := uc.Sessions.LastBookedByNode(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]NodeMRUEntry, 0, len(m))
	for id, t := range m {
		out = append(out, NodeMRUEntry{NodeID: id, LastBookedAt: t})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastBookedAt.After(out[j].LastBookedAt) })
	return out, nil
}
```
- [ ] **Step 4: `node_mru.go` (handler + DTO)**:
```go
package httpserver

import (
	"net/http"
	"time"
)

type nodeMRUDTO struct {
	NodeID       string `json:"nodeId"`
	LastBookedAt string `json:"lastBookedAt"`
}

func (s *Server) handleNodeMRU(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	entries, err := s.NodeMRU.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	out := make([]nodeMRUDTO, 0, len(entries))
	for _, e := range entries {
		out = append(out, nodeMRUDTO{NodeID: e.NodeID, LastBookedAt: e.LastBookedAt.Format(time.RFC3339)})
	}
	writeJSON(w, http.StatusOK, out)
}
```
- [ ] **Step 5: server.go** — Feld `NodeMRU usecase.NodeMRU` (bei den worktime-Usecases); Route **VOR dem `{id}`-Wildcard** (neben `resolve`/`bindings`):
```go
mux.Handle("GET /api/v1/nodes/mru", s.auth(http.HandlerFunc(s.handleNodeMRU)))
```
- [ ] **Step 6: main.go-Wiring** — im Server-Literal (bei den worktime-Usecases): `NodeMRU: usecase.NodeMRU{Sessions: sessionStore},`.
- [ ] **Step 7: apiclient** `node_mru.go`:
```go
package apiclient

import (
	"context"
	"net/http"
)

// NodeMRU mirrors one row of the server's /nodes/mru response (server-sorted,
// newest-first).
type NodeMRU struct {
	NodeID       string `json:"nodeId"`
	LastBookedAt string `json:"lastBookedAt"`
}

func (c *Client) NodeMRU(ctx context.Context) ([]NodeMRU, error) {
	var out []NodeMRU
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes/mru", nil, &out)
	return out, err
}
```
- [ ] **Step 8: grün + Commit**
```bash
git add -A && go test ./internal/usecase/... ./internal/adapter/httpserver/... ./internal/adapter/apiclient/... -race 2>&1 | tail -20
git commit -m "feat(httpserver): GET /api/v1/nodes/mru — exaktes Stop-Picker-Ranking + apiclient.NodeMRU"
```

---

### Task 5: `internal/statuscache` — atomarer Cache + Freshness-Prädikate

**Files:**
- Create: `internal/statuscache/cache.go`
- Test: `internal/statuscache/cache_test.go`

**Interfaces / Produces (für Task 7):**
- `statuscache.Entry{FetchedAt time.Time; Status apiclient.WorktimeStatus}` (json `fetchedAt`/`status`).
- `func Read(path string) (Entry, bool)` — false bei fehlend ODER korrupt (korrupt = wie „kein Cache", Spec §2).
- `func Write(path string, e Entry) error` — atomar (tmp im selben Verzeichnis + `os.Rename`), erstellt das Verzeichnis.
- `func (e Entry) Fresh(now time.Time) bool` (TTL 30 s) · `func (e Entry) Expired(now time.Time) bool` (MaxAge 30 min). Konstanten `TTL = 30*time.Second`, `MaxAge = 30*time.Minute`.

- [ ] **Step 0: rg-Verifikation** — `rg -n "func writeContextCache|os.Rename|os.MkdirAll|WriteFile" cmd/flow/context.go` (Atomar-Muster als Vorbild).
- [ ] **Step 1: Failing Tests** — Fake-Zeit, Temp-Dir:
```go
func TestEntry_FreshnessBoundaries(t *testing.T) {
	base := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	e := statuscache.Entry{FetchedAt: base}
	if !e.Fresh(base.Add(29 * time.Second)) {
		t.Error("29s should be fresh")
	}
	if e.Fresh(base.Add(31 * time.Second)) {
		t.Error("31s should be stale")
	}
	if e.Expired(base.Add(29 * time.Minute)) {
		t.Error("29min should not be expired")
	}
	if !e.Expired(base.Add(31 * time.Minute)) {
		t.Error("31min should be expired")
	}
}

func TestWriteRead_RoundtripAndCorrupt(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "worktime-status.json")
	e := statuscache.Entry{FetchedAt: time.Now().UTC().Truncate(time.Second), Status: apiclient.WorktimeStatus{LoggedMin: 42}}
	if err := statuscache.Write(p, e); err != nil {
		t.Fatal(err)
	}
	got, ok := statuscache.Read(p)
	if !ok || got.Status.LoggedMin != 42 {
		t.Fatalf("roundtrip failed: ok=%v %+v", ok, got)
	}
	_ = os.WriteFile(p, []byte("{not json"), 0o644)
	if _, ok := statuscache.Read(p); ok {
		t.Error("corrupt cache must read as absent")
	}
}

// Finding C8: the write is atomic (tmp-in-same-dir + rename) and leaves no
// leftover temp files; a second write overwrites cleanly.
func TestWrite_AtomicNoLeftoverTmp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "worktime-status.json")
	for i := 0; i < 3; i++ {
		if err := statuscache.Write(p, statuscache.Entry{FetchedAt: time.Now(), Status: apiclient.WorktimeStatus{LoggedMin: i}}); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "worktime-status.json" {
			t.Errorf("leftover file in cache dir: %q (temp not renamed/cleaned)", e.Name())
		}
	}
	if got, ok := statuscache.Read(p); !ok || got.Status.LoggedMin != 2 {
		t.Errorf("last write must win, got ok=%v %+v", ok, got)
	}
}
```
- [ ] **Step 2: FAIL → Step 3: implementieren**:
```go
// Package statuscache is the client-side tmux status snapshot cache: one atomic
// JSON file with a fetch timestamp so the 5s tick can render without a server
// round-trip, and fall back to the last snapshot (dimmed) when offline.
package statuscache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

const (
	TTL    = 30 * time.Second // fresher than this → render without fetching
	MaxAge = 30 * time.Minute // older than this → segment goes empty (Spec §2)
)

type Entry struct {
	FetchedAt time.Time                 `json:"fetchedAt"`
	Status    apiclient.WorktimeStatus `json:"status"`
}

func (e Entry) Fresh(now time.Time) bool   { return now.Sub(e.FetchedAt) < TTL }
func (e Entry) Expired(now time.Time) bool { return now.Sub(e.FetchedAt) > MaxAge }

// Read returns the cached entry; ok=false on a missing OR corrupt file (corrupt
// is treated as "no cache" so a bad write never wedges the segment).
func Read(path string) (Entry, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, false
	}
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil {
		return Entry{}, false
	}
	return e, true
}

// Write atomically persists e (tmp in the same dir + rename); concurrent ticks
// are last-writer-wins, no lock needed.
func Write(path string, e Entry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".worktime-status-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
```
- [ ] **Step 4: grün + Commit**
```bash
git add -A && go test ./internal/statuscache/... -race
git commit -m "feat(statuscache): atomarer Worktime-Status-Cache + TTL/MaxAge-Prädikate"
```

---

### Task 6: `internal/tmuxopts` — `tmux show-options -g` → Palette + MaxStreak

**Files:**
- Create: `internal/tmuxopts/tmuxopts.go`
- Test: `internal/tmuxopts/tmuxopts_test.go`

**Interfaces / Produces (für Task 7):**
- `func Read() map[string]string` — EIN `tmux show-options -g`-Aufruf; `$TMUX==""` oder Fehler → `nil` (kein Fehler, Defaults).
- `func Parse(raw string) map[string]string` — testbarer Parser (`@key value`/`@key "value"` je Zeile).
- `func Palette(opts map[string]string) statusline.StatusPalette` — `@tn_*` überschreiben `statusline.DefaultStatusPalette()`.
- `func MaxStreak(opts map[string]string) int` — `@flow_max_streak_min` (0/fehlt/kaputt = aus).

- [ ] **Step 0: rg-Verifikation** — `rg -n "func DefaultStatusPalette|type StatusPalette" internal/statusline/palette.go` (Palette-Struct-Slots + Konstruktor aus Task 1); bestätigen, dass die Options-Keys `@`-präfixiert sind (Spec §2: `@tn_green … @tn_dim`, `@flow_max_streak_min`).
- [ ] **Step 1: Failing Test** (inkl. `Read()`-Außer-tmux-Zweig, Finding C9):
```go
func TestRead_OutsideTmuxReturnsNil(t *testing.T) {
	t.Setenv("TMUX", "") // no tmux → no exec, defaults
	if got := tmuxopts.Read(); got != nil {
		t.Errorf("outside tmux Read() must return nil, got %v", got)
	}
}

func TestParseAndPalette(t *testing.T) {
	raw := "@tn_green \"#00ff00\"\n@tn_cyan #11aabb\n@flow_max_streak_min 90\nstatus on\n"
	opts := tmuxopts.Parse(raw)
	p := tmuxopts.Palette(opts)
	if p.Green != "#00ff00" || p.Cyan != "#11aabb" {
		t.Fatalf("override failed: %+v", p)
	}
	if p.Red != statusline.DefaultStatusPalette().Red {
		t.Error("unset slot should keep default")
	}
	if tmuxopts.MaxStreak(opts) != 90 {
		t.Errorf("max streak = %d", tmuxopts.MaxStreak(opts))
	}
	if tmuxopts.MaxStreak(map[string]string{}) != 0 {
		t.Error("missing max streak must be 0")
	}
}
```
- [ ] **Step 2: FAIL → Step 3: implementieren**:
```go
// Package tmuxopts reads flow's tmux status options in ONE `tmux show-options -g`
// call and maps them onto the statusline palette + active-session threshold.
package tmuxopts

import (
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/serverkraken/flow/internal/statusline"
)

// Read runs `tmux show-options -g` once. Outside tmux ($TMUX empty) or on any
// error it returns nil → callers fall back to defaults, never an error.
func Read() map[string]string {
	if os.Getenv("TMUX") == "" {
		return nil
	}
	out, err := exec.Command("tmux", "show-options", "-g").Output()
	if err != nil {
		return nil
	}
	return Parse(string(out))
}

// Parse turns show-options output ("@key value" / `@key "value"` per line) into
// a map. Only @-prefixed user options are kept.
func Parse(raw string) map[string]string {
	opts := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "@") {
			continue
		}
		key, val, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		opts[key] = strings.Trim(strings.TrimSpace(val), `"`)
	}
	return opts
}

// Palette layers @tn_* overrides on top of the tokyonight defaults.
func Palette(opts map[string]string) statusline.StatusPalette {
	def := statusline.DefaultStatusPalette()
	pick := func(key, fallback string) string {
		if v := opts[key]; v != "" {
			return v
		}
		return fallback
	}
	return statusline.StatusPalette{
		Green:  pick("@tn_green", def.Green),
		Yellow: pick("@tn_yellow", def.Yellow),
		Red:    pick("@tn_red", def.Red),
		Cyan:   pick("@tn_cyan", def.Cyan),
		Blue:   pick("@tn_blue", def.Blue),
		Purple: pick("@tn_purple", def.Purple),
		Orange: pick("@tn_orange", def.Orange),
		Dim:    pick("@tn_dim", def.Dim),
	}
}

// MaxStreak reads @flow_max_streak_min (0/missing/garbage → 0 = warning off).
func MaxStreak(opts map[string]string) int {
	n, err := strconv.Atoi(opts["@flow_max_streak_min"])
	if err != nil || n < 0 {
		return 0
	}
	return n
}
```
- [ ] **Step 4: grün + Commit**
```bash
git add -A && go test ./internal/tmuxopts/... -race
git commit -m "feat(tmuxopts): tmux-Options-Parser → Palette + @flow_max_streak_min"
```

---

### Task 7: `flow worktime status` — Cache-Tick (fresh/stale/expired, Exit 0)

**Files:**
- Create: `cmd/flow/worktime_status.go`
- Modify: `cmd/flow/worktime.go` (`cmd.AddCommand(worktimeStatusCmd(), …)`)
- Test: `cmd/flow/worktime_status_test.go`

**Interfaces / Produces:** `func worktimeStatusCmd() *cobra.Command`.

**Zustände (Spec §2 Tick-Ablauf):** (a) Cache jünger als 30 s → nur rendern (läuft lokal weiter); (b) älter → Fetch (2 s) → Erfolg: Cache erneuern + frisch rendern; (c) Fetch-Fehler/kein Token → **Stale-Pfad** (Cache dim rendern); (d) `FetchedAt` > 30 min → **leere Ausgabe**; (e) gar kein Cache + Fetch-Fehler → leere Ausgabe. **IMMER Exit 0, KEIN stderr, kein Device-Flow.**

**Verifiziert (Spec §2 „niemals Device-Flow-Prompt"):** `clientauth.Client(ctx)` (via `clientFromStore`) gibt bei fehlendem/leerem Token **`ErrNotLoggedIn` zurück — es promptet NICHT** (`clientauth.go:112-113`); bei vorhandenem Token liefert es eine lazily-refreshende Quelle, deren Refresh ein Netz-Call unter dem **Request-Kontext** ist (also durch den 2 s-`WithTimeout` gedeckelt). Kein stdin-Read auf dem Status-Pfad. Der `ErrNotLoggedIn`-Fall fällt in `fetch`'s Fehlerzweig → Offline/Stale/Leer.

- [ ] **Step 0: rg-Verifikation** — `rg -n "func clientFromStore" cmd/flow/auth.go`; `rg -n "cmd.AddCommand(worktimeImportCmd" cmd/flow/worktime.go`; Cache-Pfad-Konvention prüfen (`XDG_CACHE_HOME`/`~/.cache`).
- [ ] **Step 1: Failing Test** — die Render-Orchestrierung ist über eine testbare Kernfunktion `renderStatus(ctx, now, cachePath, fetch, opts)` gekapselt (Fetch als Funktions-Parameter, keine echte Auth):
```go
func TestRenderStatus_FreshCacheSkipsFetch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "worktime-status.json")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.Local)
	_ = statuscache.Write(p, statuscache.Entry{FetchedAt: now.Add(-10 * time.Second),
		Status: apiclient.WorktimeStatus{Date: "2026-07-08", LoggedMin: 480, TargetMin: 480}})
	fetched := false
	out := renderStatus(context.Background(), now, p, func(context.Context) (apiclient.WorktimeStatus, error) { fetched = true; return apiclient.WorktimeStatus{}, nil }, statusRenderOpts{Palette: statusline.DefaultStatusPalette()})
	if fetched {
		t.Error("fresh cache must not fetch")
	}
	if !strings.Contains(out, "08:00") {
		t.Errorf("expected rendered segment, got %q", out)
	}
}

func TestRenderStatus_StaleRendersDim(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "worktime-status.json")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.Local)
	base := statusline.DefaultStatusPalette()
	_ = statuscache.Write(p, statuscache.Entry{FetchedAt: now.Add(-2 * time.Minute),
		Status: apiclient.WorktimeStatus{Date: "2026-07-08", LoggedMin: 480, TargetMin: 480}})
	out := renderStatus(context.Background(), now, p, func(context.Context) (apiclient.WorktimeStatus, error) { return apiclient.WorktimeStatus{}, errors.New("offline") }, statusRenderOpts{Palette: base})
	if strings.Contains(out, base.Green) {
		t.Errorf("stale render must be dim, got %q", out)
	}
}

func TestRenderStatus_ExpiredEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "worktime-status.json")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.Local)
	_ = statuscache.Write(p, statuscache.Entry{FetchedAt: now.Add(-40 * time.Minute), Status: apiclient.WorktimeStatus{Date: "2026-07-08", LoggedMin: 480}})
	out := renderStatus(context.Background(), now, p, func(context.Context) (apiclient.WorktimeStatus, error) { return apiclient.WorktimeStatus{}, errors.New("offline") }, statusRenderOpts{Palette: statusline.DefaultStatusPalette()})
	if out != "" {
		t.Errorf("expired cache should render empty, got %q", out)
	}
}

// Finding #8: a corrupt on-disk cache reads as "no cache"; offline → empty.
func TestRenderStatus_CorruptCacheOfflineEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "worktime-status.json")
	_ = os.WriteFile(p, []byte("{garbage"), 0o644)
	out := renderStatus(context.Background(), time.Now(), p, func(context.Context) (apiclient.WorktimeStatus, error) {
		return apiclient.WorktimeStatus{}, errors.New("offline")
	}, statusRenderOpts{Palette: statusline.DefaultStatusPalette()})
	if out != "" {
		t.Errorf("corrupt cache + offline → empty, got %q", out)
	}
}

// Finding C10: cold cache-miss → successful fetch writes the cache AND renders.
func TestRenderStatus_ColdFetchWritesCacheAndRenders(t *testing.T) {
	p := filepath.Join(t.TempDir(), "worktime-status.json")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.Local)
	out := renderStatus(context.Background(), now, p, func(context.Context) (apiclient.WorktimeStatus, error) {
		return apiclient.WorktimeStatus{Date: "2026-07-08", LoggedMin: 480, TargetMin: 480}, nil
	}, statusRenderOpts{Palette: statusline.DefaultStatusPalette()})
	if !strings.Contains(out, "08:00") {
		t.Errorf("cold fetch should render, got %q", out)
	}
	if e, ok := statuscache.Read(p); !ok || e.Status.LoggedMin != 480 {
		t.Errorf("cold fetch must persist the cache, got ok=%v %+v", ok, e)
	}
}

// Finding C10: no cache at all + offline → empty (no panic).
func TestRenderStatus_NoCacheOfflineEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "absent.json")
	out := renderStatus(context.Background(), time.Now(), p, func(context.Context) (apiclient.WorktimeStatus, error) {
		return apiclient.WorktimeStatus{}, errors.New("offline")
	}, statusRenderOpts{Palette: statusline.DefaultStatusPalette()})
	if out != "" {
		t.Errorf("no cache + offline → empty, got %q", out)
	}
}

// Finding #2: exercise the REAL command (clientFromStore + GetWorktimeStatus +
// offline path) end-to-end — deterministically offline via an unreachable
// server. Must exit 0 (RunE returns nil) and render empty; NEVER prompt/hang.
// (Verify the env key names against internal/clientconfig in Step 0 — FLOW_TOKEN
// forces a static bearer so no keychain is touched; FLOW_SERVER_URL points at a
// refused port so the 2s fetch fails fast.)
func TestWorktimeStatusCmd_OfflineExitZeroEmpty(t *testing.T) {
	t.Setenv("FLOW_TOKEN", "dummy")
	t.Setenv("FLOW_SERVER_URL", "http://127.0.0.1:1")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("TMUX", "")
	cmd := worktimeStatusCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status must exit 0 offline, got %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("no cache + offline → empty, got %q", buf.String())
	}
}
```
- [ ] **Step 2: FAIL → Step 3: implementieren** — Kernfunktion + Cobra-Verb. Der Fetch nutzt `context.WithTimeout(ctx, 2*time.Second)`; die `RunE` returnt **immer nil** und schreibt nur nach `cmd.OutOrStdout()`:
```go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/statuscache"
	"github.com/serverkraken/flow/internal/statusline"
	"github.com/serverkraken/flow/internal/tmuxopts"
	"github.com/spf13/cobra"
)

func worktimeStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Render the tmux status-right worktime segment (cached; never interactive)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := tmuxopts.Read()
			ro := statusRenderOpts{Palette: tmuxopts.Palette(opts), MaxStreakMin: tmuxopts.MaxStreak(opts)}
			fetch := func(ctx context.Context) (apiclient.WorktimeStatus, error) {
				c, err := clientFromStore(ctx) // NEVER triggers device flow on a plain read
				if err != nil {
					return apiclient.WorktimeStatus{}, err
				}
				return c.GetWorktimeStatus(ctx)
			}
			seg := renderStatus(cmd.Context(), time.Now(), statusCachePath(), fetch, ro)
			if seg != "" {
				fmt.Fprintln(cmd.OutOrStdout(), seg)
			}
			return nil // ALWAYS exit 0, never stderr (Spec §2)
		},
	}
}

type statusRenderOpts struct {
	Palette      statusline.StatusPalette
	MaxStreakMin int
}

// renderStatus is the pure tick: fresh cache → render; else fetch (2s, derived
// from the command's context so a signal still cancels) → renew + render; on
// fetch error → stale (dim) render, or empty when >30min old / no cache.
func renderStatus(parent context.Context, now time.Time, cachePath string, fetch func(context.Context) (apiclient.WorktimeStatus, error), ro statusRenderOpts) string {
	entry, ok := statuscache.Read(cachePath)
	if ok && entry.Fresh(now) {
		return render(entry.Status, now, ro, false)
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	if st, err := fetch(ctx); err == nil {
		_ = statuscache.Write(cachePath, statuscache.Entry{FetchedAt: now, Status: st})
		return render(st, now, ro, false)
	}
	if !ok || entry.Expired(now) {
		return "" // no usable cache → suppress the segment entirely
	}
	return render(entry.Status, now, ro, true) // stale: dim
}

func render(st apiclient.WorktimeStatus, now time.Time, ro statusRenderOpts, dim bool) string {
	pal := ro.Palette
	if dim {
		pal = pal.Dimmed()
	}
	return statusline.BuildStatusSegment(toSnapshot(st, now, pal, ro.MaxStreakMin))
}

// statusCachePath honours XDG_CACHE_HOME, else ~/.cache, landing at
// flow/worktime-status.json (Spec §2).
func statusCachePath() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "flow", "worktime-status.json")
}
```
  Plus `toSnapshot` (DTO → `statusline.Snapshot`) im selben File — parst `activeStart` (RFC3339), setzt `LoggedMin` (completed), mappt `DayOff`/`Week`/`Burndown`:
```go
func toSnapshot(st apiclient.WorktimeStatus, now time.Time, pal statusline.StatusPalette, maxStreak int) statusline.Snapshot {
	snap := statusline.Snapshot{
		Now: now, LoggedMin: st.LoggedMin, TargetMin: st.TargetMin, Running: st.Running,
		Streak: st.Streak, SaldoMin: st.Burndown.SaldoMin, SaldoTarget: st.Burndown.TargetMin,
		Palette: pal, MaxStreakMin: maxStreak,
	}
	if st.Running && st.ActiveStart != "" {
		if t, err := time.Parse(time.RFC3339, st.ActiveStart); err == nil {
			snap.ActiveStart = t
		}
	}
	if st.DayOff != nil {
		snap.DayOff = &statusline.DayOffInfo{Kind: domain.Kind(st.DayOff.Kind), Label: st.DayOff.Label}
	}
	for _, d := range st.Week {
		wd := statusline.WeekDay{LoggedMin: d.LoggedMin, TargetMin: d.TargetMin, IsToday: d.IsToday}
		if dd, err := time.Parse("2006-01-02", d.Date); err == nil {
			wd.Weekday = dd.Weekday()
		}
		if d.DayOffKind != nil {
			wd.DayOffKind = domain.Kind(*d.DayOffKind)
		}
		snap.Week = append(snap.Week, wd)
	}
	return snap
}
```
  (Import `github.com/serverkraken/flow/internal/domain` fürs `domain.Kind`-Mapping.) In `worktime.go`: `cmd.AddCommand(worktimeImportCmd(), worktimeStatusCmd())`.
- [ ] **Step 4: grün + Commit**
```bash
git add -A && go test ./cmd/flow/... -race 2>&1 | tail -20
git commit -m "feat(cli): flow worktime status — gecachter tmux-Segment-Tick (fresh/stale-dim/expired, Exit 0)"
```

---

### Task 8: `flow worktime stop` — Picker-Popup-Kaskade

**Files:**
- Create: `cmd/flow/worktime_stop.go`
- Modify: `cmd/flow/worktime.go` (`worktimeStopCmd()` anhängen)
- Modify (optional): `cmd/flow/projectbind_picker.go` (pick-only Booking-Variante) ODER neue Datei `cmd/flow/worktime_stop_picker.go`
- Test: `cmd/flow/worktime_stop_test.go`

**Interfaces / Produces:** `func worktimeStopCmd() *cobra.Command` mit Flag `--node <ref>`.

**Zustände (Spec §3 Kaskade):** (1) frischer Status vom Server (Cache umgehen) → `activeSessionId`; (2) keine laufende Session → kurze Meldung, Exit 0; (3) Session hat `activeNodeId` → sofort stoppen, kein Picker; (4) `--node <ref>` → auflösen via `resolveNodeRef`, direkt stoppen; (5) unzugeordnet + TTY → fuzzy-Picker über bookable Nodes (Enter=buchen+stoppen, Esc=Abbruch OHNE Stop); (6) unzugeordnet + **kein TTY** ohne `--node` → Fehlermeldung (Exit 1) statt hängendem Prompt; (7) nach Erfolg: Posten ausgeben + Cache invalidieren (Datei löschen) → Segment aktualisiert beim nächsten Tick; (8) `--node <bad-ref>` → sauberer Fehler aus `resolveNodeRef` (Exit 1).

**Präzedenz (bewusst, Finding #4 — Spec §3 lässt es offen):** `--node` ist ein **expliziter Override und gewinnt über `activeNodeId`** (Reihenfolge in `resolveStopNode`: `--node` zuerst) — so kann Soenne eine falsch gebuchte laufende Session beim Stoppen auf den richtigen Node umbuchen. Empfehlung dokumentiert; falls unerwünscht, im Review zurückmelden.

- [ ] **Step 0: rg-Verifikation** — `rg -n "func newPickParentProgram|func (m \*pickProjectProgram) Selection|type Item struct" cmd/flow/projectbind_picker.go internal/tui/ui/fuzzylist/fuzzylist.go`; `rg -n "func resolveNodeRef" cmd/flow/noderef.go`; `rg -n "func IsBookable" internal/domain/node.go`; `rg -n "func (c \*Client) StopSession|func (c \*Client) ListNodes|func (c \*Client) GetNode|func (c \*Client) NodeMRU" internal/adapter/apiclient/*.go` (`NodeMRU` ist aus Task 4b).
- [ ] **Step 1: Failing Test** — die nicht-interaktiven Zweige über eine testbare Kernfunktion `runStop(ctx, c, statusFetch, nodeRef, interactive, pick, out)` (Picker + Status als Parameter injiziert), gegen einen `httptest`-Stop-Server:
```go
func TestRunStop_NoRunningSession(t *testing.T) {
	var out bytes.Buffer
	st := apiclient.WorktimeStatus{Running: false}
	err := runStop(context.Background(), nil, func(context.Context) (apiclient.WorktimeStatus, error) { return st, nil },
		"", false, nil, &out)
	if err != nil {
		t.Fatalf("no-session must not error: %v", err)
	}
	if !strings.Contains(out.String(), "kein") { // "keine laufende Session"
		t.Errorf("expected no-session note, got %q", out.String())
	}
}

func TestRunStop_NonTTYWithoutNodeErrors(t *testing.T) {
	st := apiclient.WorktimeStatus{Running: true, ActiveSessionID: "s1"} // no ActiveNodeID
	err := runStop(context.Background(), nil, func(context.Context) (apiclient.WorktimeStatus, error) { return st, nil },
		"", false /*interactive*/, nil, io.Discard)
	if err == nil {
		t.Fatal("non-tty unbooked stop without --node must error, not hang")
	}
}

// Finding C12: a stop that fails AFTER a node was chosen must propagate the
// error AND leave the cache untouched (so the segment still shows the timer).
func TestRunStop_StopFailureKeepsCacheAndErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError) // stop fails
	}))
	defer ts.Close()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	_ = statuscache.Write(statusCachePath(), statuscache.Entry{FetchedAt: time.Now()})
	c := apiclient.New(ts.URL, "tok")
	st := apiclient.WorktimeStatus{Running: true, ActiveSessionID: "s1"}
	err := runStop(context.Background(), c,
		func(context.Context) (apiclient.WorktimeStatus, error) { return st, nil },
		"", true, // interactive
		func(context.Context, *apiclient.Client) (string, error) { return "n1", nil }, // picker returns a node
		io.Discard)
	if err == nil {
		t.Fatal("stop failure must propagate")
	}
	if _, ok := statuscache.Read(statusCachePath()); !ok {
		t.Error("cache must NOT be invalidated when stop fails")
	}
}
```
  Der Implementer ergänzt: Session mit `activeNodeId` → StopSession direkt (httptest bestätigt `projectId`); `--node` non-TTY → resolve + stop; `--node` gewinnt über `activeNodeId` (Umbuchung, Finding #4); `--node <bad-ref>` → `resolveNodeRef`-Fehler propagiert (Exit 1; `resolveNodeRef` ist rein — Bestandstest `cmd/flow/noderef_test.go`); Picker-Esc → kein Stop-Call.
- [ ] **Step 2: FAIL → Step 3: implementieren** — die Kaskade + Cobra-Verb. `stop` DARF Fehler returnen (läuft im `display-popup -E` mit TTY):
```go
func worktimeStopCmd() *cobra.Command {
	var nodeRef string
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the running timer, booking it to a node (interactive picker)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			fetch := func(ctx context.Context) (apiclient.WorktimeStatus, error) { return c.GetWorktimeStatus(ctx) }
			return runStop(cmd.Context(), c, fetch, nodeRef, isInteractive(), pickBookableNode, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&nodeRef, "node", "", "node ref to book to (slug/path/id) — required in non-TTY")
	return cmd
}

// isInteractive reports whether stdin is a terminal (needed for the picker).
func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && (fi.Mode()&os.ModeCharDevice) != 0
}

// pickBookableNode runs the fuzzy/MRU picker over the owner's bookable nodes and
// returns the chosen node id ("" + nil = cancelled).
func pickBookableNode(ctx context.Context, c *apiclient.Client) (string, error) {
	nodes, err := c.ListNodes(ctx)
	if err != nil {
		return "", err
	}
	var bookable []domain.Node
	for _, n := range nodes {
		if domain.IsBookable(n.Kind) {
			bookable = append(bookable, n)
		}
	}
	prog := newPickBookableProgram(mruOrder(ctx, c, bookable), theme.Load())
	if _, err := tea.NewProgram(prog, tea.WithContext(ctx)).Run(); err != nil {
		return "", err
	}
	picked, _, ok := prog.Selection()
	if !ok {
		return "", nil // Esc → cancel, no stop
	}
	return picked.ID, nil
}

// mruOrder returns picker items MOST-RECENTLY-BOOKED first (Spec §1/§3 "fuzzy/MRU"),
// then the never-booked remainder in ListNodes order. Ranking comes from the
// server's EXACT /nodes/mru (Task 4b) — not a client heuristic; any error
// degrades silently to plain order (the picker must never hard-fail on MRU).
func mruOrder(ctx context.Context, c *apiclient.Client, bookable []domain.Node) []fuzzylist.Item {
	byID := make(map[string]domain.Node, len(bookable))
	for _, n := range bookable {
		byID[n.ID] = n
	}
	var items []fuzzylist.Item
	seen := map[string]bool{}
	if mru, err := c.NodeMRU(ctx); err == nil {
		for _, e := range mru { // server-sorted newest-first
			if n, ok := byID[e.NodeID]; ok && !seen[e.NodeID] {
				seen[e.NodeID] = true
				items = append(items, fuzzylist.Item{ID: n.ID, Label: n.Name})
			}
		}
	}
	for _, n := range bookable {
		if !seen[n.ID] {
			items = append(items, fuzzylist.Item{ID: n.ID, Label: n.Name})
		}
	}
	return items
}

// runStop is the pure cascade (picker + status fetch injected for tests).
func runStop(ctx context.Context, c *apiclient.Client,
	fetch func(context.Context) (apiclient.WorktimeStatus, error),
	nodeRef string, interactive bool,
	pick func(context.Context, *apiclient.Client) (string, error),
	out io.Writer,
) error {
	st, err := fetch(ctx)
	if err != nil {
		return err
	}
	if !st.Running {
		fmt.Fprintln(out, "keine laufende Session")
		return nil
	}
	nodeID, err := resolveStopNode(ctx, c, st, nodeRef, interactive, pick)
	if err != nil {
		return err
	}
	if nodeID == "" {
		return nil // cancelled (Esc) — do NOT stop
	}
	sess, err := c.StopSession(ctx, st.ActiveSessionID, nodeID)
	if err != nil {
		return err // stop failed AFTER the node was chosen — cache is NOT invalidated, error surfaced (Finding C12)
	}
	_ = os.Remove(statusCachePath()) // invalidate → next 5s tick refetches
	// Print the booked posten (Spec §3 step 5): node name + duration, not just the id.
	name := nodeID
	if n, gerr := c.GetNode(ctx, nodeID); gerr == nil && n.Name != "" {
		name = n.Name // best-effort; falls back to the id
	}
	fmt.Fprintf(out, "gestoppt · gebucht auf %s · %s\n", name, sess.Elapsed(time.Now()).Round(time.Minute))
	return nil
}

func resolveStopNode(ctx context.Context, c *apiclient.Client, st apiclient.WorktimeStatus,
	nodeRef string, interactive bool,
	pick func(context.Context, *apiclient.Client) (string, error),
) (string, error) {
	if nodeRef != "" { // explicit override — wins over st.ActiveNodeID (Finding #4); also the non-TTY path
		nodes, err := c.ListNodes(ctx)
		if err != nil {
			return "", err
		}
		return resolveNodeRef(nodes, nodeRef) // bad ref → clean error, propagated
	}
	if st.ActiveNodeID != nil && *st.ActiveNodeID != "" {
		return *st.ActiveNodeID, nil // already booked at start → stop straight away
	}
	if !interactive {
		return "", fmt.Errorf("no TTY for the picker: pass --node <ref> to book non-interactively")
	}
	return pick(ctx, c)
}
```
  `newPickBookableProgram` (neben `newPickParentProgram`, pick-only, Booking-Titel):
```go
func newPickBookableProgram(items []fuzzylist.Item, pal theme.Palette) *pickProjectProgram {
	return &pickProjectProgram{list: fuzzylist.New(items, pal), title: "Zeit buchen auf …"}
}
```
  In `worktime.go`: `cmd.AddCommand(worktimeImportCmd(), worktimeStatusCmd(), worktimeStopCmd())`.
- [ ] **Step 4: grün + Commit**
```bash
git add -A && go test ./cmd/flow/... -race 2>&1 | tail -20
git commit -m "feat(cli): flow worktime stop — Picker-Popup-Kaskade (direkt/--node/fuzzy/non-TTY)"
```

---

### Task 9: Wiring-Gate — `make ci` + Live-Smoke + Composition-Root-Verify + dotfiles-Notiz

**Files:** keine Code-Änderung außer evtl. Fixes aus dem Smoke. Dieser Task ist der Pflicht-Wiring-Task ([[feedback_plan_main_wiring_task]]) und bewusst ein **Verifikations-/Gate-Task OHNE TDD-failing-Step** (Finding C13) — die Test-Deckung liegt in Tasks 1–8; hier zählt der Repo-weite `make ci`-Grün-Beweis + Live-Smoke.

- [ ] **Step 1: Composition-Root-Verify** — bestätigen, dass kein Usecase verwaist ist:
```bash
rg -n "WorktimeStatus|NodeMRU" cmd/flow-server/main.go internal/adapter/httpserver/server.go  # beide Felder + Wiring
rg -n "worktime/status|nodes/mru" internal/adapter/httpserver/server.go                        # beide Routen registriert
rg -n "LastBookedByNode" internal/ports/ports.go internal/adapter/pgstore/sessions.go          # Store-Methode vorhanden
rg -n "worktimeStatusCmd|worktimeStopCmd" cmd/flow/worktime.go                                 # beide Verben angehängt
go build ./...                                                                                  # alle drei Binaries bauen
```
- [ ] **Step 2: Rest-Sweep** über `git diff --name-only rebuild..HEAD` — Fokus: Double-Count (Logged excl. Tail?), Exit-0-Garantie im Status-Pfad (kein `return err` in `worktimeStatusCmd`, kein stderr), Stale→Dim vollständig (alle Slots), Expired→leer, `stop`-Kaskade (Esc kein Stop, non-TTY Fehler), SSE beim Stop (bestehender `EventSessionStopped`-Pfad unangetastet), keine verwaisten Symbole (`statusline`/`statuscache`/`tmuxopts`/`WorktimeStatus`).
- [ ] **Step 3: `make ci`** — grün. **In Bash mit `timeout: 600000` aufrufen** (pgstore-Testcontainer; `DOCKER_HOST` auf Podman-Socket). Nach dem Task-Ende `git status` sauber.
```bash
git add -A && make ci   # timeout 600000; DOCKER_HOST=<podman-socket>
```
- [ ] **Step 4: Live-Smoke gegen den dev-Stack** (nicht delegierbar an CI):
```bash
# dev-Stack hochfahren + Token
make dev-up && make dev-run   # eigener Terminal/Hintergrund
TOKEN=$(make dev-token)
# 1) Endpoints direkt
curl -sk -H "Authorization: Bearer $TOKEN" https://localhost:8080/api/v1/worktime/status | jq .
curl -sk -H "Authorization: Bearer $TOKEN" https://localhost:8080/api/v1/nodes/mru | jq .   # [] wenn noch nichts gebucht; sonst newest-first
# 2) CLI-Segment (FLOW_INSECURE_TLS=1 gegen self-signed dev)
./bin/flow worktime status          # leere Ausgabe wenn nichts getrackt — erwartet
#    → eine Session starten (WebUI/TUI/`flow session start`), dann erneut:
./bin/flow worktime status          # ⏱ HH:MM ▶ … erscheint; zweiter Aufruf < 30s → aus Cache (kein Server-Hit)
# 3) Stop mit Picker
./bin/flow worktime stop            # Picker → Node wählen → gestoppt; danach `status` zeigt ‖
```
  Verifizieren: (a) DTO enthält `loggedMin`/`running`/`activeStart`/`week`/`streak`/`burndown`; (b) `loggedMin` bei laufender Session ist die ABGESCHLOSSENE Zeit (nicht inkl. Tail — Banner tickt lokal, ein zweiter `status`-Aufruf 3 s später zeigt eine höhere Bannerzahl OHNE Server-Hit); (c) Offline-Pfad: `flow-server` stoppen → `status` rendert dim; nach >30 min-Simulation (Cache-`fetchedAt` manuell zurückdatieren) → leer.
- [ ] **Step 5: dotfiles-Notiz (manueller Abschluss, NICHT dieses Repo)** — im tmux-`worktime`-Plugin die eine Zeile ändern und im dotfiles-Repo committen (Picker braucht TTY):
```
bind E run-shell -b 'flow worktime stop'   →   bind E display-popup -E 'flow worktime stop'
```
  Diese Änderung ist NICHT Teil dieses Repos — nur als Abschluss-Schritt im Ledger/Handoff vermerken. `segment-wrap.sh raw … 'flow worktime status'` bleibt unverändert (die leere Ausgabe unterdrückt den Delimiter bereits).
- [ ] **Step 6: kein Commit nötig**, wenn `make ci` grün und der Smoke sauber war (der letzte Code-Commit war Task 8). Fixes aus dem Smoke → eigener `fix(...)`-Commit.

---

## Non-Goals & Risiken (aus der Spec, im Plan gespiegelt)

**Non-Goals (Spec §Non-Goals — bewusst NICHT in diesem Slice):** Sidekick-Deep-Links (`prefix+W`, `~/.cache/flow/next-screen`-Protokoll, `flow sidekick`-Alias) = eigener Slice 2. Kein Start-/Pause-/Resume-Verb (TUI/WebUI decken das ab). **Keine** Änderung an der Buchungspflicht, an DTOs bestehender Endpoints oder am WebUI. Kein neuer SSE-Event-Typ.

**Risiken/Ränder (Spec §Risiken — wo behandelt):** (1) **Mitternacht** — Session gestern gestartet: der Renderer (`activeSessionParts`/`clampedElapsed`) clampt auf heutige Mitternacht (Banner und ▶ zeigen dieselbe Zahl); das Usecase subtrahiert den Tail NUR bei Same-Day-Start (Finding G1); Tests `ETACrossesMidnight` (Task 1) + `CrossMidnightRunningNotSubtracted` (Task 2). (2) **Multi-Maschine** — Cache ist pro Maschine; 30 s Sichtverzug auf Mutationen anderer Geräte akzeptiert (Cache-Design Task 5/7). (3) **Uhrzeit-Skew Client/Server** — die Client-Extrapolation nutzt die **lokale** Uhr gegen den Server-`activeStart` (RFC 3339 mit Offset, in `toSnapshot` geparst); Minuten-Genauigkeit reicht fürs Segment; die Tail-Subtraktion im Usecase nutzt die Server-Uhr, die Re-Addition im Client die lokale — bei synchronen Uhren deckungsgleich, bei Skew im Minutenbereich unkritisch. (4) **Cache-Snapshot-Lag** — `week[].loggedMin` (inkl. Tail) friert zwischen Fetches ein → ein heutiger Pace-Dot kann bis zu 30 s hinter dem lokal tickenden Banner zurückliegen (Hit/Running-Wechsel); rein kosmetisch, akzeptiert.

## Getroffene Entscheidungen (Soenne, 2026-07-08 — alle fünf entschieden)

1. **MRU im Stop-Picker → dedizierter Server-Support (ENTSCHIEDEN).** Nicht die Client-Heuristik über `ListSessionsPage(100)`, sondern ein exakter Server-Endpoint. **Form (Planner-Wahl, begründet): eigener Endpoint `GET /api/v1/nodes/mru`** statt ein `lastBookedAt`-Feld an `domain.Node` — das Feld würde die Node-DTO durch WebUI/TUI/MCP rippeln; der Endpoint stört **null** Bestands-DTOs. Neue Store-Methode `SessionStore.LastBookedByNode` (`MAX(start_at) … GROUP BY node_id` über gestoppte, gebuchte Sessions, owner-scoped, keine Migration) → Task **1b**; Endpoint/Usecase/apiclient → Task **4b**; Konsum im Picker → Task 8.
2. **Streak → dedizierter fensterloser Reader (ENTSCHIEDEN).** `StatsComputer.CurrentStreak` (Task **1c**) statt `RangeStats("month")`: Semantik wie der alte `CurrentStreak` (rückwärts bis zum ersten Miss, kein Monatsfenster-Schnitt), Wochenenden/Frei-Tage übersprungen. Fensterlos über die geladene Historie (3-Jahres-Sicherheitskappe gegen Ausreißer). Keine neue Store-Methode (`List(from)` genügt). Konsum → Task 2.
3. **Kosmetik: Port 1:1 (ENTSCHIEDEN, unverändert).** 3 Frei-Kinds farbig (Holiday→Blue, Vacation→Purple, Sick→Orange), flex/special/childsick/training → Dim; Labels ungekürzt. Wie in Task 1 (`KindStatusColor`) und Global Constraints umgesetzt.
4. **Cache-Pfad `~/.cache/flow/` (ENTSCHIEDEN, unverändert).** XDG-konform (`XDG_CACHE_HOME`), spec-literal; wie in Task 7 (`statusCachePath`) umgesetzt.

**Residual (kein Blocker, nur Notiz):** die MRU-Tiefe ist jetzt exakt (kein 100er-Cut mehr); die Streak-3-Jahres-Kappe ist eine reine Ausreißer-Sicherung, kein funktionales Fenster.

---

## Self-Review-Appendix

**Update 2026-07-08 (Soenne-Entscheidungen, Spec-Rev `6bf2cb0`).** Nach dem ersten Berater-Durchlauf hat Soenne alle fünf offenen Punkte entschieden (siehe „Getroffene Entscheidungen"). Zwei davon fügen **dedizierten Server-Support** hinzu: **Streak** = fensterloser Reader `StatsComputer.CurrentStreak` (Task **1c**, ersetzt `RangeStats("month")`); **MRU** = eigener Endpoint `GET /api/v1/nodes/mru` auf Basis einer neuen read-only Query `SessionStore.LastBookedByNode` (Tasks **1b** + **4b**, ersetzt die Client-Heuristik `ListSessionsPage(100)`). Server-Reader-Tasks stehen VOR ihren Konsumenten (Reihenfolge 1 → 1b → 1c → 2 → 3 → 4 → 4b → 5 … → 8). Global Constraints angepasst (pgstore wird jetzt geändert → Task 1b braucht Docker/testcontainers, keine Migration). Kosmetik/Cache-Pfad blieben wie im Entwurf. **Konsistenz-Check über die geänderten Tasks durchgeführt** (Store-Methode → Fake-Ripple inkl. Inline-`fakeSessionStore`; Route vor `{id}`-Wildcard; Streak-Reader reust `resolver`/`AggregateRange`-Bestand; Picker konsumiert `NodeMRU`; Wiring-Gate deckt beide neuen Routen + die Store-Methode). Kein voller Berater-Rerun (Soenne: nur bei Unsicherheit) — die Streak-Fensterung wurde datengetrieben gelöst (Fenster ab frühester Session, 3-Jahres-Kappe), die MRU-Form über die DTO-Rippel-Analyse entschieden.

**Grounding-Quelle:** Selbst-Grounding (Erstverifikation aller Signaturen per Read/rg im Arbeitsrepo + Port-Quelle) **plus** `agy`/gemini-bigcontext-Dossier (background) — **beide stimmen überein**, kein Degradations-Modus. Das agy-Dossier bestätigte alle load-bearing Signaturen (Server-Struct-Felder, StatsComputer-Reader, apiclient `StopSession(ctx,id,nodeID string)`, `IsBookable`, `resolveNodeRef`, fuzzylist-Item, Test-Fakes) und ergänzte: `TargetResolver`-Shape, `Stats`-Zusatzfelder (nur `Streak` genutzt), höchste Migration 0030 (irrelevant — keine Migration).

**Berater-Findings — Verbleib.** Beide Berater (codex-second-opinion + gemini-bigcontext/agy) bekamen Spec + Plan-Entwurf + Dossier mit dem wörtlichen Lücken-Raster. Jede Lücke einzeln verbucht:

_Gemini/agy (unabhängige Code-Verifikation gegen das echte Repo):_
- **G1 [HIGH, echter Bug] Cross-Midnight-Double-Count** — `StatsComputer.Today()` lädt Sessions via `List(from=today-midnight)` (`WHERE start_at >= $2`, `pgstore/sessions.go:105` selbst verifiziert), also ist eine über Mitternacht laufende Session NICHT in `Today().Logged`; die unbedingte Tail-Subtraktion fräße andere abgeschlossene Tages-Sessions. → **EINGEARBEITET (Task 2):** Subtraktion ist jetzt konditional auf `sess.Start >= today-midnight`; `clampedTail`-Helfer entfernt; neuer Test `TestWorktimeStatus_CrossMidnightRunningNotSubtracted`.
- **G2 [moderat] Kein Test gegen den echten `clientauth`-Pfad** (alle Task-7-Tests injizieren fake fetch). → **EINGEARBEITET (Task 7):** neuer Test `TestWorktimeStatusCmd_OfflineExitZeroEmpty` fährt das echte Command (clientFromStore + GetWorktimeStatus) deterministisch offline (unreachable `FLOW_SERVER_URL`), prüft Exit 0 + leere Ausgabe.
- **G3 [low-mod] Owner-Scope-Test nur prosaisch in Task 2.** → **EINGEARBEITET (Task 2):** expliziter `TestWorktimeStatus_OwnerScoped` als Code.
- **G4 [b] `--node` überschreibt gebuchte Session ohne dokumentierte Präzedenz; kein bad-ref-Test.** → **EINGEARBEITET (Task 8):** Präzedenz (`--node` gewinnt = Umbuchung) explizit dokumentiert (Code-Kommentar + Zustand #8 + Risiko); bad-ref-Fehlerpfad als Zustand + Implementer-Test ergänzt.
- **G5 [sekundär, kosmetisch] Pace-Dot vs. Banner-Staleness** (Dot lagt bis 30 s hinter dem lokal tickenden Banner). → **BEGRÜNDET ABGELEHNT (bereits abgedeckt):** exakt Risiken-Punkt #4 im Plan; rein kosmetisch, Spec §Risiken „30 s Sichtverzug akzeptiert".
- **G6 [a] Spec §3 verlangt „fuzzy/MRU"; Plan liefert fuzzy-only.** → **EINGEARBEITET (Tasks 1b+4b+8):** nach Soenne-Entscheidung als exakte Server-MRU umgesetzt (siehe C1) — nicht mehr vertagt.
- **G7 [b, selbst gefunden, moderat] Empty-Suppression ohne `Running`-Guard** → Wochenend-Start-Timer im ersten Tick rendert leer. → **EINGEARBEITET (Task 1):** Guard `&& !in.Running` (bewusste Abweichung vom Port, spec-konformer: laufender Timer ist „getrackt"); neuer Test `RunningAtZeroRendersNonEmpty`.
- **G8 [low] Kein End-to-End-Test für korrupte On-Disk-Cache in `renderStatus`.** → **EINGEARBEITET (Task 7):** `TestRenderStatus_CorruptCacheOfflineEmpty`.
- Sauber (kein Finding): SSE (stop nutzt Bestand-`EventSessionStopped`), main.go-Wiring (Task 3/9), i18n/Responsive echt N/A (CLI/tmux), alle Bestandsnamen matchen den echten Code.

_Codex-second-opinion (14 Findings):_
- **C1 [a] „fuzzy/MRU" wörtlich in Spec §3, Plan war fuzzy-only.** → **EINGEARBEITET (Tasks 1b+4b+8):** nach Soenne-Entscheidung **exakte Server-MRU** (`GET /api/v1/nodes/mru` ← `SessionStore.LastBookedByNode`), im Picker via `mruOrder` konsumiert; degradiert lautlos auf ListNodes-Reihenfolge. (Deckt Gemini G6 mit ab.)
- **C2 [a/d] Stop-Erfolg druckt nur `sess.ID`, nicht den „gebuchten Posten" (Spec §3 Schritt 5).** → **EINGEARBEITET (Task 8):** Ausgabe jetzt `gestoppt · gebucht auf <Node-Name> · <Dauer>` (Name best-effort via `GetNode`, Fallback id).
- **C3 [d] `week[].loggedMin` (heute, inkl. Tail) untested — die dossier-„KRITISCH"-Asymmetrie.** → **EINGEARBEITET (Task 2):** Assertion in `TestWorktimeStatus_LoggedExcludesRunningTail`, dass `todayWeek.Logged >= 30min` (inkl. Tail) während top-level `Logged==0`.
- **C4 [b/d] Task-2-Zustände (mit/ohne Node, Mitternacht) nicht in expliziten Tests.** → **EINGEARBEITET (Task 2):** Cross-Midnight-Test (aus G1) deckt Mitternacht; „laufende Session MIT Node → ActiveNodeID propagiert" in die Implementer-Fälle aufgenommen.
- **C5 [b/d] Task-3-Test nur 401 + Key-Presence.** → **EINGEARBEITET (Task 3):** geforderte Zusatztests aufgezählt (running-Felder, empty-day-week, DayOff-Shape, Usecase-Fehler→500).
- **C6 [c/d] Handler-Ebene Owner-Scoping ungetestet.** → **EINGEARBEITET (Task 3):** expliziter Zwei-Owner-Handler-Test (u2 sieht u1s Session nie) gefordert; Owner-Isolation nun Usecase- UND Handler-Ebene gedeckt.
- **C7 [b] `activeNodeId,omitempty` omittet statt `null`.** → **BEGRÜNDET ABGELEHNT (dokumentiert):** Codex selbst bestätigt: null-consumer-identisch — `apiclient.WorktimeStatus.ActiveNodeID` dekodiert fehlend≡null≡nil gleich. DTO-Kommentar ergänzt (absent≡null≡unbooked); `omitempty` behalten vermeidet ein Lone-`null`-Feld im not-running-Fall.
- **C8 [d] Atomic-Write-Mechanik (tmp+rename) ungetestet.** → **EINGEARBEITET (Task 5):** `TestWrite_AtomicNoLeftoverTmp` (keine Leftover-`.tmp`, last-writer-wins).
- **C9 [b/d] `tmuxopts.Read()` selbst ungetestet; Step 0 kein konkretes rg.** → **EINGEARBEITET (Task 6):** `TestRead_OutsideTmuxReturnsNil`; Step 0 auf konkretes rg gesetzt.
- **C10 [b/d] Task-7-Cluster: cold-fetch-writes-cache, no-cache+offline, RunE-Exit-0.** → **EINGEARBEITET (Task 7):** `TestRenderStatus_ColdFetchWritesCacheAndRenders`, `…_NoCacheOfflineEmpty`, `TestWorktimeStatusCmd_OfflineExitZeroEmpty` (echte RunE, deterministisch offline). (Deckt Gemini G2/G8 mit.)
- **C11 [b, interner Widerspruch] Prosa `ctx`, Code `context.Background()`.** → **EINGEARBEITET (Task 7):** `renderStatus(parent ctx, …)` + `WithTimeout(parent, 2s)`; `RunE` gibt `cmd.Context()` durch; alle Test-Call-Sites angepasst — Cobra-Cancellation jetzt live.
- **C12 [b/d] Stop-Fehler NACH Picker: Cache-Invalidierung + Fehlerpfad ungetestet.** → **EINGEARBEITET (Task 8):** `TestRunStop_StopFailureKeepsCacheAndErrors` (Fehler propagiert, Cache bleibt).
- **C13 [d] Task 9 ohne failing-Test-Step.** → **BEGRÜNDET ABGELEHNT (Gate-Task):** Codex stimmt zu — Task 9 ist per Design ein Verifikations-/Gate-Task; Note in Task 9 ergänzt.
- **C14 [d] „make ci am Task-Ende" mehrdeutig.** → **EINGEARBEITET (Global Constraints):** Konvention präzisiert — paket-scoped `go test` je Task, voller `make ci` als Slice-Gate in Task 9.

**Dissens/Downgrade-Entscheidungen des Planners:** C7 (null-vs-omitempty) und C13 (Gate-Task) als begründet abgelehnt verbucht — beide von Codex selbst als „quick edit / note, not structural" eingestuft; C7 zusätzlich consumer-verifiziert. Alle übrigen 12 Findings eingearbeitet. Kein echter Dissens zwischen den Beratern; die überlappenden Findings (MRU, Owner-Scope, Task-7-Cluster, Double-Count-Deckung) wurden konvergent behandelt.
