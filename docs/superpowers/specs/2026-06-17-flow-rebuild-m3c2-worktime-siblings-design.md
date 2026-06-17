# flow rebuild M3c2 — Worktime Sibling-Routes (Design Spec)

**Date:** 2026-06-17
**Milestone:** M3c2 — vierte Sub-Slice von M3c (nach M3c0 Session-Mutation-API, M3c1 Today-Route).
**Status:** approved (Brainstorm abgeschlossen), bereit für Implementation-Plan.
**Parent-Spec:** `docs/superpowers/specs/2026-06-16-flow-rebuild-m3c-home-worktime-design.md` (M3c-Übersicht, lockt Architektur + Scope-Schnitt).
**Vorgänger-Plan-Pattern:** `docs/superpowers/plans/2026-06-16-flow-rebuild-m3c1-today-route.md` (Route-Contract-Template, subagent-driven Ausführung).

## Scope (aus Parent-Spec M3c, Tabelle M3c2)

Worktime-**Sibling-Routes** Woche · Stats(Range week|month) · DayOffs · Export, revived aufs Design-System (`theme` + `ui`), Daten auf `apiclient` + SSE umgeklemmt, plus Lateral-Navigation zwischen den Surfaces. **Kein Heatmap** (M3d). Keine Home/Docs-Tabs (M3c3), kein NavHost/Standalone-Repoint (M3c4).

## Brainstorm-Entscheidungen (M3c2-spezifisch, erweitern die Parent-Spec)

1. **Lateral-Navigation statt Leaf-Only.** Die Parent-Spec sprach von „`w/t/d/e` push, `esc` pop". M3c2 verfeinert das: `w/t/d/e` sind **aus jeder Worktime-Surface** aktiv (Today **und** jeder Sibling). Wechsel zwischen Siblings **vertieft den Stack nicht** (replace-top), nur der erste Sprung von Today aus pusht. Damit ist die Tiefe in einem Sibling immer 2 (`Worktime › Woche`), und `esc` kehrt direkt zu Today zurück. Begründung: entspricht dem alten Monolith-Modal-Switching und Soennes „voll bauen"-Haltung ([[feedback_dont_descope_hobby_projects]]).
2. **DayOffs/Frei expandiert.** Der aktuelle Monolith kann in der DayOff-View nur Liste + `g` (globales Default-Target via `SetTargetConfig`). M3c2 **erweitert** das um: `a` Dayoff anlegen (`AddDayOffs`), `D` Dayoff löschen mit Confirm (`DeleteDayOff`), `b` Bundesland-Picker (`SetBundesland`). Die apiclient-Methoden existieren bereits ([[feedback_generic_features]]).

## Architektur

### Paket-Layout — Hub + Leaf-Subpakete (bricht den Import-Zyklus)

```
internal/tui/screen/worktime/            ← Hub (existiert: TodayRoute aus M3c1)
  nav.go            siblingNav-Registry: map[string]func() shell.Route, hier gebaut
                    (einzige Stelle, die alle vier Leaves importiert)
  format.go         existierende Helfer (fmtDur/fmtMin/fmtSaldo/threshold/badge/BarColored-Wrap)
                    — werden EXPORTIERT, damit Leaves sie wiederverwenden
  week/route.go     WeekRoute        (Leaf)
  statsrange/route.go StatsRangeRoute (Leaf)
  dayoffs/route.go  DayOffsRoute     (Leaf)
  export/route.go   ExportRoute      (Leaf)
```

**Zyklus-Bruch:** Leaves importieren **einander nie**. Jede Route bekommt bei Konstruktion einen injizierten `nav` (die `map[key]→factory`, gebaut im Hub). Tastendruck `w` → `nav["w"]()` baut die Ziel-Route → Route emittiert eine Switch-Message mit der gebauten Route. Abhängigkeit ist gerichtet: Hub → Leaves → (nur `shell` + `worktime`-Helfer, keine Geschwister). Leaves dürfen den Hub für `format.go`-Helfer importieren (Hub→Leaf ist die einzige Richtung mit Route-Konstruktion; Leaf→Hub nur für Format-Funktionen — kein Zyklus, weil der Hub die Leaf-Konstruktoren nur **lazy** in `nav.go`-Closures referenziert, nicht auf Paketebene zirkulär).

> Falls Go den Leaf→Hub-Import (für Format-Helfer) zusammen mit Hub→Leaf (für `nav.go`) doch als Zyklus wertet: Format-Helfer in ein **drittes, abhängigkeitsfreies** Paket `internal/tui/screen/worktime/wtfmt/` ziehen, das Hub und Leaves gemeinsam importieren. Im Plan als erste Task verifizieren (`go build`), Eskalation nur bei echtem Zyklus.

### Lateral-Switch-Mechanismus (neue generische Shell-Message)

- **`shell.SwitchRouteMsg{Route}`** — Semantik: *„ist der aktive Stack auf Root (Len==1), dann Push; sonst ReplaceTop"*. Einmal in `Shell.Update` implementiert (nutzt das existierende `NavStack.Push`/`ReplaceTop`). Das ist der Kern des Nicht-Vertiefens.
- **Cross-Slice-Notiz:** M3c4's `NavHost` (eigener Single-Stack-Host fürs Standalone) muss `SwitchRouteMsg` **identisch** honorieren. In M3c2 wird die Semantik in `Shell` gebaut und dort getestet (Done-Gate läuft in `flow ui`); der NavHost-Teil ist M3c4.
- **`esc`** → bestehendes `PopRouteMsg`.
- **Shared `wtnav`-Helfer** (im Hub): mappt `w/t/d/e` → `SwitchRouteMsg{nav[key]()}`, `esc` → `PopRouteMsg`. Eingehängt in **alle fünf** Routen (Today + 4 Siblings). Taste der aktuellen Surface = No-Op (Route kennt ihren eigenen Key).

### Route-Contract

Unverändert aus M3b/M3c1: `Title() · Init() tea.Cmd · Update(tea.Msg) (shell.Route, tea.Cmd) · View(shell.Frame) string · KeyHints() []keyhint.Hint`. Jede Sibling-Route folgt dem TodayRoute-Aufbau (loadCmd → loadedMsg → reconstruct/state, `shell.EventMsg` triggert reload).

## Die vier Routen

Alle revived aufs Design-System: plain `bar()` → `BarColored`, `styleHeader/styleMuted` → `theme`-Palette + `titlebox`/`keyhint`-Chrome, Footer max 4 Hints. Render-Logik portiert aus dem Monolith (`internal/tui/stats.go`, `dayoffs.go`, `export.go`).

| Route | Title | Body | Interaktivität | Reload-on |
|---|---|---|---|---|
| **week** | Woche | 7 Day-Rows: Marker (heute `▶`), Datum, `BarColored`, logged/target | read-only | `session.*` |
| **statsrange** | Stats | Total/⌀-Tag/Max/Min/Arbeitstage/Treffer/Streak/Saldo + Burndown-Block | `m` Monat · `W` Woche (Range-Toggle, reload) | `session.*` |
| **dayoffs** | Frei | Liste Dayoffs+Feiertage, Default-Target, Bundesland | `g` Default-Target editieren · `a` Dayoff anlegen · `D` löschen (confirm) · `b` Bundesland-Picker | `dayoff.changed`, `settings.changed` |
| **export** | Export | Preset/Range/Format/Path-Form (Port der bestehenden Overlay-Logik) | volle Form, schreibt Datei via `apiclient.Export` | statisch |

- **Burndown** im statsrange-Body ist erlaubt (`GetBurndown`) — nur die **Heatmap** ist M3d (existiert im Rebuild ohnehin nicht).
- **DayOffs-Dialoge** nutzen die `ui`-Komponenten `confirm` (Delete), `form`/`picker` (Add, Bundesland) — dieselben, die Today für Booking/Edit/Delete verwendet.
- **Today** (M3c1) bekommt zusätzlich die Launch-Keys `w/t/d/e` via `wtnav` (Konflikt-frei: `s`/`E`/`D`/`g`/`G` belegt, `w/t/d/e` frei).

## Datenquellen (alle vorhanden)

`GetWeek(ctx, "")` · `GetStats(ctx, rng)` · `GetBurndown(ctx)` · `ListDayOffs(ctx, from, to)` · `AddDayOffs` · `DeleteDayOff` · `GetSettings` · `SetBundesland` · `SetTargetConfig` · `Export(ctx, from, to, format, projectID)`. Keine neue apiclient-Methode, kein Server-Change in M3c2.

## Testing & Done-Gate

- **Unit-Tests pro Route-Paket:** Render-Golden fürs Chrome-freie Body (Week-Rows, Stats-Tabelle+Burndown, DayOffs-Liste, Export-Form); Reducer-Tests (statsrange `m`/`W`-Toggle, dayoffs add/delete/bundesland-Flow, export Form-Transitions).
- **Shell-Test:** `SwitchRouteMsg` → Push bei Root (Len 1→2), ReplaceTop bei Sibling (Len bleibt 2); `wtnav`-Helfer mappt Keys korrekt.
- **Done-Gate (live, `flow ui` gegen Dev-Stack [[reference_flow_dev_env]]):** Aus Today `w/t/d/e` → laterales Switchen ohne Stack-Vertiefung (Breadcrumb bleibt `Worktime › X`); `esc` zurück zu Today; DayOffs `a`/`D`/`b` mutieren live (SSE-Reload sichtbar); statsrange `m`/`W` toggelt; Export schreibt eine Datei. `make ci` grün, Coverage ≥ 80 %.

## YAGNI / explizit ausgeschlossen

- Kein Heatmap, kein Home/Docs (M3c3), kein NavHost/Standalone-Repoint (M3c4).
- Kein neuer Server-Endpoint, keine Migration.
- Kein Note-Attach, kein Pause-State (siehe Parent-Spec).

## Referenzen

Parent: `2026-06-16-flow-rebuild-m3c-home-worktime-design.md` · [[project_flow_rebuild_m3c1_done]] (Today-Route-Template) · [[project_flow_rebuild_m3c0_done]] (Session-Mutation-API) · [[project_flow_rebuild_m3b]] (Shell/NavStack/Route/PushRouteMsg/PopRouteMsg) · [[project_flow_rebuild_m3a]] (`BarColored`, theme, ui) · Monolith-Quellen `internal/tui/stats.go`+`dayoffs.go`+`export.go` · [[feedback_no_monoliths]] · [[feedback_generic_features]] · [[feedback_dont_descope_hobby_projects]] · [[reference_flow_dev_env]] (Done-Gate-Stack) · [[reference_soenne_worktime_workflow]] · [[feedback_subagent_git_commits_isolated]] (Ausführung) · [[feedback_long_lived_integration_branch]] (`rebuild` bleibt Integrations-Branch).
