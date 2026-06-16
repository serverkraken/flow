# flow rebuild M3c — Home-Dashboard + Worktime-Surfaces (Design Spec / Übersicht)

**Date:** 2026-06-16
**Milestone:** M3c — die dritte Slice der M3-Vertikale (nach M3a Design-System-Revival, M3b Sidekick-Shell).
**Status:** approved (Brainstorm abgeschlossen), bereit für Sub-Slice-Pläne.
**Parent-Spec:** `docs/superpowers/specs/2026-06-16-flow-rebuild-m3-tui-design-system-shell.md` (M3-Übersicht).
**Scope-Schnitt:** Diese Spec ist die **Übersicht** für M3c. Ausführung in **Sub-Slices M3c1–M3c4** (je eigener Plan, subagent-driven wie M3b).

## Problem / Motivation

M3a hat das Design-System wiederbelebt (`theme` + `ui`), M3b die Sidekick-Shell (Tabstrip · `:`-Palette · pro-Tab-Nav-Stack · Chrome) mit **Placeholder-Routes** (Home→About-Demo). M3c liefert die **ersten echten Screens** als Routes: ein **Home-Dashboard** und die **vollständige Worktime-Surface**, datengetrieben über `apiclient` + SSE.

Heute existieren zwei parallele, unbefriedigende Worktime-Implementierungen: der **Rebuild-Monolith** `internal/tui/worktime.go` (467 Zeilen, bündelt Today + Booking + DayOffs + Week + Stats + Export über Modal-Flags, mit grobem Text-Render) und das **reife `main`-Design** (`internal/frontend/tui/screen/worktime/today_render.go` u.a. — Status-Pille, `BarColored`, Ziel·noch·ETA, Pause-Trenner, Cursor). M3c **belebt das `main`-Design wieder** (Parent-Spec Locked Decision 6) und ersetzt den Monolithen durch fokussierte Route-Packages (No-Monoliths, [[feedback_no_monoliths]]).

## Locked Decisions (aus dem M3c-Brainstorm)

1. **Today-Scope: voller Port.** Die Worktime-Today-Route portiert die `main`-Surface originalgetreu **inkl.** Interaktivität: j/k-Cursor, start/stop, Projekt-Booking beim Stop, Session-Edit, Note-Attach, Dialoge.
2. **Home-Scope: Anzeige + Drill.** Read-only Dashboard; Enter/Tasten **springen** in den passenden Tab (Drill-Navigation über die Shell). Keine Mutationen von Home aus.
3. **Tabs in M3c: Home · Worktime · Docs.** Drei echte Tabs. Stats-Tab (mit Heatmap/Burndown), Projects-Tab, DayOffs/Export **als eigene Tabs** sowie der custom Markdown-Viewer + Wikilink-Drill bleiben **M3d**.
4. **Standalone: Repoint + Modals.** `flow worktime` wird auf die neue Today-Route umgehängt; die Geschwister-Surfaces (Woche/Stats-Range/DayOffs/Export) sind als **pushbare Routes** erreichbar — derselbe Route-Typ in `flow ui` (Worktime-Tab-Stack) und Standalone. Der Rebuild-Monolith-Pfad `tui.New` wird retired.
5. **Porting-Architektur: Approach A** — pro Surface ein eigenes kleines Route-Package, revived aus dem `main`-Design, Daten auf `apiclient`+SSE umgeklemmt. Kein Adapter-Wrap des Legacy-Monolithen, keine Hybrid-Doppelsprache.
6. **Geschwister-Navigation: push/pop im Worktime-Tab-Stack.** Woche/Stats/DayOffs/Export sind **keine Tabs** und **keine Today-Drill-downs**, sondern **Sibling-Routes**: `w`/`t`/`d`/`e` **pushen** die jeweilige Route auf den Worktime-Stack, `esc` poppt. Breadcrumb zeigt die Tiefe.
7. **Direktsprung-Modi: beides.** Zwei parallele Einstiegswege, beide bleiben/kommen ([[reference_flow_launch_modes]]):
   - **Standalone-Subcommands** `flow worktime` / `flow docs` — chrome-leichter Einstieg (NavHost, **kein** Tabstrip → schmal-Pane-freundlich für die tmux-Sidebar), direkt auf der Surface. Die bestehenden tmux-Bindings (`prefix+a+3/4`, [[reference_soenne_worktime_workflow]]) bleiben unverändert.
   - **Deep-Link in die Voll-App** `flow ui worktime` / `flow ui docs` — `flow ui` nimmt ein optionales Positions-Argument `[tab]` und startet die volle Tab-Shell **direkt auf diesem Tab** (statt Home). Unbekanntes/leeres Argument → Home.

## Daten-Modell-Abgleich (zentrales technisches Risiko)

Das `main`-Render lehnte an einem reichen `domain.Day`-Aggregat (`Total(now)`, `Target`, `Active`, `IsRunning()`, `IsPaused()`, `PausedAt`, `Logged`, Pause-Gaps). **Der Rebuild hat das nicht.** Verfügbar:

- `apiclient.Today{ Date, LoggedMin, TargetMin, SaldoMin, Running }` (Tages-Totals, server-berechnet).
- `apiclient.Client.ListSessions(ctx) → []domain.WorkSession` (`ID, ProjectID*, Tag, Note, Start, Stop*`; `Running()`, `Elapsed(now)`).
- **Kein Pause-Konzept** auf dem Rebuild — Sessions starten/stoppen nur.

**Konsequenz fürs Today-Render (M3c1):** Die Präsentations-Felder werden client-seitig aus `Today` + heutigen Sessions rekonstruiert:

| `main`-Konzept | Rebuild-Rekonstruktion |
|---|---|
| `day.Total(now)` | `LoggedMin` (server inkludiert die laufende Session) → `time.Duration`; bei laufender Session sekündlicher Tick fürs Live-Inkrement. |
| `day.Target` | `TargetMin`. |
| `day.IsRunning()` | `Today.Running` bzw. eine Session mit `Stop==nil`. |
| `day.Sessions` | `ListSessions` gefiltert auf `Start` == heute (lokale TZ). |
| ETA | laufende Session `Start` + verbleibende Soll-Dauer; bei `Target<=0` → „kein Tagesziel". |
| Pause-Trenner | aus aufeinanderfolgenden Session-Gaps (`s.Start - prevStop`) — wie `main`, aber **kein** „in Pause seit"-Hinweis (kein Pause-State). |
| `IsPaused()`/`PausedAt` | **entfällt** (Glyph/Label/Logik werden gestrichen). |

**Kein Server-Change in M3c.** Sollte sich die Rekonstruktion als zu brüchig erweisen (z.B. TZ-Kanten bei der „heute"-Filterung), ist die Eskalation ein dedizierter Server-`/today`-Detail-Endpoint in einer späteren Slice — nicht in M3c.

**Threshold-Color / Status-Badge:** `totalThresholdColor` + `todayStatusBadge` (Parent-Spec: status_adapter) wurden in M3a auf M3d verschoben. M3c1 bringt eine **lokale Kopie im Worktime-Route-Package**; die Promotion zu einem geteilten `internal/tui/ui/status_adapter` passiert in M3d, wenn ein zweiter Konsument (Stats-Heatmap) existiert.

## Architektur

```
internal/tui/
  shell/                 (M3b — Shell, NavStack, Palette, RouteHost, Route-Contract)
    home.go              ← M3c3: HomeRoute ersetzt Placeholder (Dashboard + Drill)
    navhost.go           ← M3c4: chrome-leichter Single-Stack-Host (Footer+Breadcrumb,
                           kein Tabstrip/Palette) — push/pop fürs Standalone
  screen/
    worktime/            ← M3c1: Today-Route (voller Port aus main), + lokale
                           Render-Helfer (threshold/badge/eta), apiclient+SSE
    week/                ← M3c2: Woche-Route (Pace-Strip / Week-Days)
    statsrange/          ← M3c2: Stats-Range-Route (week|month) — KEIN Heatmap (M3d)
    dayoffs/             ← M3c2: DayOffs-Route (Liste + Target-Edit + Bundesland)
    export/              ← M3c2: Export-Route (Preset/Range/Format/Path)
    docs/                ← M3c3: existierende DocsModel als Route gewrappt
                           (Markdown-Viewer-Upgrade bleibt M3d)

cmd/flow/
  ui.go                  Shell mit Tabs: Home · Worktime · Docs;       (M3c3)
                         optionales Arg `flow ui [tab]` → Deep-Link
  worktime.go            → NavHost(Worktime-Today)  statt tui.New      (M3c4)
  docs.go                bleibt Standalone (DocsModel, schon chrome-leicht)

internal/tui/worktime.go (+ worktime-exklusive Monolith-Files)         ← retired (M3c4)
internal/tui/docs.go + styles.go                                       ← BLEIBEN (docs.go
                       wird in M3c3 als Route gewrappt; teilt styles.go)
```

**Route-Contract (M3b, unverändert):** `Title() · Init() tea.Cmd · Update(tea.Msg) (Route, tea.Cmd) · View(Frame) string · KeyHints() []keyhint.Hint`. Navigation per `PushRouteMsg`/`PopRouteMsg`.

**Sibling-Push-Modell:** Die Worktime-Today-Route emittiert bei `w/t/d/e` ein `PushRouteMsg{Route: NewWeekRoute(...)}` usw. In `flow ui` appliziert die Shell das auf den Worktime-Tab-Stack; im Standalone appliziert der **NavHost** es auf seinen einzigen Stack. `esc` → `PopRouteMsg`. Damit ist jede Surface in beiden Launch-Modi identisch erreichbar ([[reference_flow_launch_modes]]).

**NavHost (M3c4):** M3b's `RouteHost` ignoriert Pushes (Single-Route). M3c4 führt `NavHost` ein: ein chrome-leichter Host mit eigenem `NavStack` (push/pop), Footer-Keyhints + Breadcrumb, **ohne** Tabstrip/Palette. `q`/`Ctrl+C` quit; `esc` poppt (Floor = quit oder no-op — im Plan zu fixieren). `RouteHost` bleibt für künftige echte Single-Route-Screens bestehen.

**Home-Dashboard (M3c3):** responsiv — zweispaltig (links Arbeit: laufende Session + Tagesziel `BarColored` + Woche; rechts Wissen: letzte Docs + Projekte) wenn `Frame.Width` breit genug, sonst gestapelt (eine Spalte) fürs schmale tmux-Panel. Read-only; Enter/Tasten emittieren Tab-Switch (Drill) in den passenden Tab. Daten via `apiclient.GetToday`/`GetWeek`/`ListDocuments`/`ListProjects`, Live-Refresh über SSE-`ChangedMsg`-Broadcast.

**Docs-als-Route (M3c3):** die existierende `DocsModel` (`NewDocs(client, ed, op, user)`, `$EDITOR` via `tea.ExecProcess`) wird hinter dem Route-Contract gewrappt — Adapter überbrückt `View() tea.View` → `View(Frame) string`. Funktional unverändert; der **custom Markdown-Viewer + Wikilink-Drill-down** bleibt M3d.

## Sub-Slice-Schnitt (je eigener Plan)

| Sub-Slice | Inhalt | Done-Gate-Kern |
|---|---|---|
| **M3c1** | Worktime-**Today**-Route: voller Port aus `main` (Datumszeile · Headline mit Status-Pille + Total-Threshold + Ziel% · `BarColored` · Ziel·noch·**ETA** prominenter · Sessions-Liste mit Pause-Trennern + Cursor `▎` · Footer max 4) **+ Interaktivität** (j/k, s start/stop, x Booking, Session-Edit, Note-Attach, Dialoge). Daten rekonstruiert aus `Today`+`ListSessions`, Live via SSE. Lokale threshold/badge/eta-Helfer. | Today rendert design-getreu; start/stop/booking/edit/note funktionieren; SSE-Live-Update sichtbar; Unit-Tests (Render-Golden + Reducer) grün. |
| **M3c2** | Worktime-**Sibling-Routes** Woche · Stats(Range week\|month) · DayOffs · Export, revived aufs Design-System, + Push/Pop-Wiring (`w/t/d/e`→Push, `esc`→Pop) im Worktime-Stack. **Kein** Heatmap (M3d). | Jede Sibling-Route pusht/poppt korrekt; Breadcrumb zeigt Tiefe; Daten live; Tests grün. |
| **M3c3** | **Home-Dashboard**-Route (responsiv 2-col→stacked, Drill-Navigation) ersetzt die M3b-Placeholder-Home; **Docs**-Screen als Route gewrappt; `flow ui`-Shell auf **3 Tabs** (Home·Worktime·Docs) verdrahtet; **`flow ui [tab]` Deep-Link** (Positions-Arg wählt Start-Tab, unbekannt→Home). | `flow ui` zeigt echten Tagesstand auf Home; `flow ui worktime`/`flow ui docs` starten direkt auf dem Tab; Drill in Tabs; Tabstrip + Palette über 3 Tabs grün. |
| **M3c4** | **NavHost** + **Repoint Standalone**: `flow worktime` → `NavHost(Worktime-Today)` mit pushbaren Siblings; Legacy `tui.New`-Pfad retiren. **Achtung:** `internal/tui/docs.go` bleibt (M3c3-Route-Wrap) und teilt sich `styles.go` mit dem Monolithen — die exakte Lösch-Menge wird im M3c4-Plan per Import-/Symbol-Analyse bestimmt (nur worktime-exklusive Files; `styles.go` bleibt solange `docs.go` darin lebt; geteilte Styles ggf. in ein eigenes File splitten). | `flow worktime` standalone: Today + w/t/d/e-Push + esc-Pop live; `flow ui` + `flow docs` unberührt; `make ci` grün (≥80 %); kein toter worktime-Monolith-Code mehr. |

## Testing & Done-Gate

- **Unit-Tests** pro Route-Package: Render-Golden fürs Chrome-freie Body (Today-Headline/Bar/Summary, Home-Layout breit vs. gestapelt), Reducer-Tests (Cursor-Bewegung, start/stop-Cmds, Push/Pop-Emission, Dialog-Flow).
- **Daten-Rekonstruktions-Tests:** Today aus synthetischen `Today`+`[]WorkSession` (heute-Filter TZ-Kanten, ETA-Berechnung, Pause-Trenner aus Gaps).
- **Done-Gate (live, M3c-gesamt):** `flow ui` gegen den Dev-Stack ([[reference_flow_dev_env]]) — Home zeigt echten Tagesstand; Tab nach Worktime; Today live (SSE `session.started/stopped`); `w/t/d/e` push + `esc` back; Docs-Tab navigierbar. **Beide Direktsprung-Modi:** Standalone `flow worktime` (NavHost, kein Tabstrip) läuft mit Today + Siblings; `flow docs` standalone unverändert; Deep-Links `flow ui worktime` / `flow ui docs` starten die Voll-Shell auf dem richtigen Tab. `make ci` grün, Coverage ≥ 80 %.

## Referenzen

Parent: `2026-06-16-flow-rebuild-m3-tui-design-system-shell.md` · [[project_flow_rebuild_m3b]] (Shell/Route/NavStack/RouteHost) · [[project_flow_rebuild_m3a]] (theme+ui, `BarColored` ported) · `main`-Referenz `internal/frontend/tui/screen/worktime/today_render.go` · [[feedback_no_monoliths]] · [[feedback_no_icons]] · [[reference_soenne_worktime_workflow]] (TUI-primary, Project-Picker beim Booking) · [[reference_flow_launch_modes]] (sidekick vs. standalone) · [[feedback_navigation_discoverability_over_minimalism]] · [[reference_flow_dev_env]] (Done-Gate-Stack) · [[feedback_subagent_git_commits_isolated]] (Ausführung) · [[feedback_long_lived_integration_branch]] (`rebuild` bleibt Integrations-Branch) · [[feedback_dont_descope_hobby_projects]] (Scope einmal geflaggt, dann gebaut).
```
