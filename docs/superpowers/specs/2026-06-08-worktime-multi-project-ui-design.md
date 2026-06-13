# Worktime Multi-Project "Heute" Screen — Design

Date: 2026-06-08
Status: Approved (brainstorming) — pending implementation plan
Scope: flow client TUI worktime "Heute" (today) screen + per-project daily
target. Touches `internal/frontend/tui/screen/worktime` (today\_\*), a new local
paused-marker store, `internal/domain/project.go`,
`internal/adapter/sqliteclient/projects`, `internal/adapter/sqliteserver/projects`,
the projects sync payload/handler, `internal/usecase/projects.go`, and the
composition root (`cmd/flow/main.go`). No new server endpoints; one additive
schema column on both sides.

## Problem

The "Heute" screen on `next` is a half-finished migration from a single global
clock to multi-project parallel timers. Only the **start** half of the new path
is wired; everything else still renders the legacy single-clock model. Three
user-visible failures result:

1. **Running timers show the project UUID, not its name.**
   `today_render.go:357` `renderActiveSessionsIndicator` does `name := as.ProjectID`
   (line 366) and truncates it to the last 8 chars. The inline comment defers the
   name join to an unfinished "Task 21". The picker *passes* a `projectName` into
   `activeSessionsStartCmd` but it is only used for the success toast; the
   reloaded `ActiveSession` (from `ListActive`) carries only `ProjectID`.

2. **No way to stop or manage a running timer.**
   `ActiveSessions.Stop(userID, projectID, …)` exists (`active_sessions.go:118`)
   but **no key calls it**. The running timers are a non-interactive header line,
   not a navigable list. The cursor + `D` (delete) operate on the *legacy*
   completed-session list (`h.day.Sessions`), and `p` (pause) calls the legacy
   `SessionWriter.Pause()`.

3. **The headline lies.** Total / status pill / progress bar / session list all
   read `h.day`, populated by `WorktimeReader.Today()` from
   `r.State.GetActive()` — a `ports.LegacyActiveStore` (`worktime_reader.go:31`).
   The new start path never writes that store, so `day.IsRunning()` is
   structurally always false in the new path → "pausiert / 0:00" even while
   project timers run. It *feels* like nothing happened.

### Why this is structural, not a small bug

Running state lives in two **disjoint stores**: `WorktimeReader` reads it from
`ports.LegacyActiveStore`; the new path reads/writes `ports.ActiveSessionStore`.
Completed sessions *do* surface (both `WorktimeReader.Sessions` and
`ActiveSessions.sessions` share `cacheSessions` — `main.go:270` vs `main.go:228`),
they just render without a project name. The new path is live in production
(`main.go:342-344` wires `Projects`/`ActiveSessions`/`UserID`), which is why the
user sees UUIDs and the picker at all.

## Goals

- "Heute" is a coherent **multi-project dashboard**: one row per project showing
  today's total, its own progress against its own daily target, and live
  running / paused state.
- **Start, stop, pause/resume, set-target** are all reachable from the screen and
  operate on the real (`ActiveSessions` + `Projects`) model.
- Running timers always show **project names**, never UUIDs.
- **Local-first**: every action commits locally and syncs async; no server is
  required to start / stop / pause.

## Non-goals (for now)

- CLI parity for per-project target / pause (`flow worktime … --project`).
- Cross-device sync of the pause *hint* (the badge is a local hint; the stop it
  implies does sync).
- Per-project targets in Week / History views — those keep the existing global
  per-weekday `TargetResolver`.
- Per-session edits (tag / note / edit / delete of *completed* sessions) on
  "Heute" — these move to the `Verlauf` tab, which already owns the full editor.
- Removing the legacy single-clock path (kept as a degradation; see below).

## Product decisions (from brainstorming)

- **D1 — Multi-parallel timers.** One timer per project, several concurrently;
  the existing `ActiveSession` Option-2 model. Finish it rather than collapse to
  a single clock.
- **D2 — Per-project daily targets.** Each project carries its own daily target
  and progress bar. The global `Day.Target` stays for Week / History only.
- **D3 — Explicit per-project pause.** `p` stops the focused timer and marks the
  project "pausiert seit HH:MM"; `s` resumes. A thin **local** marker; not synced.
- **D4 — Per-session edits move to `Verlauf`.** "Heute"'s cursor stays purely on
  projects; "Verlauf heute" on "Heute" is a read-only timeline.
- **D5 — Silent stop** (no tag / note prompt). Tag/note are added later via
  `Verlauf`.

## The screen

```
  Mi · 08.06.2026                              gesamt  3h 12m / 8h 30m

  Projekte ──────────────────────────────────────────────────────────
  ▶ flow              1h 30m   ██████░░░░░░   50%  / 3h      seit 14:05
  ‖ homelab-study       42m    ███░░░░░░░░░   23%  / 3h      pausiert 14:40
    kompendium        1h 00m   ████░░░░░░░░   40%  / 2h 30m
    ＋ Timer starten …

  Verlauf heute ─────────────────────────────────────────────────────
    09:00 → 10:30   flow            1h 30m
    11:00 → 11:42   homelab-study     42m   [meeting]
    13:00 → 14:00   kompendium      1h 00m

  ▶ flow läuft   ·   [s] stoppen  [p] pause  [Z] ziel  j/k  ? hilfe
```

**Which projects appear as rows** (a project shows if *any* holds):
running now · paused today · has logged time today · has `DailyTarget > 0`.
Order: running → paused → today-total desc → MRU (`LastUsedAt`). A project with
a target but no activity shows at 0 %, so unmet targets are visible. A trailing
`＋ Timer starten …` row is always present.

**Per-project total** = sum of today's completed `Session.Elapsed` for that
`ProjectID` + (running ? `now - StartedAt` : 0). The header `gesamt` = sum across
projects / sum of targets.

## Interaction / keymap (new-path "Heute")

- `j`/`k` move, `g`/`G` top/bottom (cursor over project rows + the `＋` row).
- `s` — act on focused row: idle → **start**, running → **stop**, paused →
  **resume**. On the `＋` row → open the existing `project_picker`
  (MRU list + create-new) to start any other project.
- `p` — focused **running** project → **pause** (stop + set local marker). No-op
  on idle/paused rows (consumed so it doesn't fall through to global `p`).
- `Z` — set / edit the focused project's `DailyTarget` (single-input dialog
  "Ziel für <name>: 3h30m", parsed by the existing duration parser).
- `?` help; `q` exits per the worktime-root rules.
- Footer hints adapt to the focused row's state (start vs stoppen vs fortsetzen).

The legacy per-session keys on "Heute" (`t`/`N`/`E`/`D` on completed sessions,
`o`/`O`/`R` Kompendium-note ops) are **removed from the new-path "Heute"**; the
session editor lives in `Verlauf`. (Kompendium attach stays reachable where it
belongs after a follow-up; not in scope here.)

## Data flow

On load / refresh (idle only), "Heute" fans out:
- `Projects.ListAll(userID)` → `map[ID]Project` (name + `DailyTarget`; `ListAll`
  so archived-but-running projects still resolve a name; `GetByID` as fallback).
- `ActiveSessions.ListActive(userID)` → running set (`ProjectID`, `StartedAt`).
- `Paused.List(userID)` → `map[ProjectID]pausedAt` (local).
- `WorktimeReader.Today()` → completed sessions (grouped by `ProjectID`) + date.

These merge into `[]projectRow`; render is a pure `(rows, now) → string`.

Actions (each emits `heuteActionDoneMsg` + `emitWorktimeChanged`):
- **start** `ActiveSessions.Start(userID, pid, "", "")`, then `Paused.Clear(pid)`.
- **stop** `ActiveSessions.Stop(userID, pid, "", "")`.
- **pause** `ActiveSessions.Stop(userID, pid, "", "")` + `Paused.Set(userID, pid, now)`.
- **resume** = start on a paused row (`Start` + `Paused.Clear`).
- **setTarget** `Projects.SetDailyTarget(userID, pid, d)`.

## Domain & store changes

- `domain.Project` += `DailyTarget time.Duration` (0 = no target).
- `sqliteclient` + `sqliteserver` projects: additive column
  `daily_target_seconds INTEGER NOT NULL DEFAULT 0`; included in row read/write
  and `Upsert`. Existing rows migrate to 0 via the column default.
- Projects sync: add `daily_target_seconds` (snake_case) to the projects push/pull
  body and `projects_handlers` in/out mapping, so a target set on one device
  reaches the others.
- `usecase.Projects.SetDailyTarget(userID, id string, d time.Duration) error`:
  `GetByID` → set field → `Upsert` → `enqueueProject(pr, pr.Version)` → `signalPush`
  (mirrors `Rename`/`Archive`).
- **New local paused-marker store.** `ports.PausedProjectStore` with
  `Set(userID, projectID string, at time.Time) error`,
  `Clear(userID, projectID string) error`,
  `List(userID string) (map[string]time.Time, error)`. Backed by a local
  `paused_projects(user_id, project_id, paused_at)` table in the client cache DB
  (consistent with the other `sqliteclient` stores). **Not synced.**
  - Alternative considered: a JSON/TSV file like the legacy pause marker —
    rejected for store consistency.

## Wiring (composition root)

- `cmd/flow/main.go`: construct the `PausedProjectStore` against the cache DB;
  add it to `worktime.Deps`. `Projects`/`ActiveSessions`/`UserID` are already
  wired (`main.go:342-344`).
- `worktime.Deps` += `Paused ports.PausedProjectStore`. "Heute" uses it only when
  `ActiveSessions != nil && UserID != ""`; the legacy path ignores it.

## Legacy path

The new project-dashboard render + keymap is gated on
`deps.ActiveSessions != nil && deps.UserID != "" && deps.Projects != nil`. When
any is unset (existing `newRig` tests, standalone-without-sync) the screen keeps
the current legacy single-clock render and `toggleStartStopCmd`. The gate lives
at the top of `renderBody` and the key handler, so the two paths don't interleave.
The legacy path is **not deleted** in this change — it guards existing tests and
the no-sync standalone. A later cleanup can retire it once the CLI and tests move
to the new model.

## Error handling

- `Projects.ListAll` fails → danger toast; fall back to rendering running timers
  by name-if-cached else short id; never crash.
- Start race (`ErrActiveSessionExists`) → info toast "<name> läuft bereits"
  (existing behavior) + reload.
- Stop / pause where the `ActiveSession` is already gone (stopped on another
  device) → treat as a no-op info toast + reload.
- `SetDailyTarget` parse error → keep the dialog open, surface via `errMsg`.
- `Paused` store failures are non-fatal (the pause hint is best-effort); the
  underlying stop still commits.

## Testing

- `domain`: `Project.DailyTarget` zero value + struct round-trip.
- `sqliteclient` / `sqliteserver` projects: target column read/write; existing
  rows default to 0 (migration).
- `usecase.Projects.SetDailyTarget`: persists + enqueues a project sync row.
- `usecase` (or screen helper): per-project aggregation (group today's sessions
  by project, add running elapsed, merge paused) — pure, table-driven.
- `screen/worktime`:
  - **Regression for the uid bug**: running rows render the project *name*, never
    the UUID.
  - `s` starts / stops / resumes the focused project (fake
    `ActiveSessions`/`Projects`/`Paused`).
  - `p` pauses (stop + marker); the "pausiert" badge renders; `s` resumes and
    clears the marker.
  - `Z` sets a target; the per-project bar + "/ Ziel" reflect it.
  - the `＋` row opens the picker.
  - **Legacy path unchanged**: `newRig` (no `ActiveSessions`/`UserID`) still
    exercises `toggleStartStopCmd` with the old render.

## Build order — two phases on a worktree off `next`

| Phase | Content | Effect |
|---|---|---|
| **A — Un-break** (no schema change) | name join (uid → name), drive state from `ActiveSessions`, project-grouped dashboard, `s` = start/stop/resume per project, `＋` picker, read-only "Verlauf heute" timeline, adaptive footer, new-path gate | screen **works** and is shippable |
| **B — Ambition** (schema + sync) | `Project.DailyTarget` (+ store column, sync field, `SetDailyTarget`, migration), per-project bars + "/ Ziel" + `Z`; `PausedProjectStore` + `p` + "pausiert" badge | full per-project dashboard |

This is build *order*, not descoping — both phases ship. Phase A restores a
usable screen without any schema/sync change; Phase B layers the per-project
target + pause on top.

## Risks / open items

- **Render duality** (legacy + new in one `heute` model) — contained by a single
  top-level gate; flagged for later removal once CLI/tests migrate.
- **Paused-marker staleness** (day rollover, archived project) — cleared on
  start/resume; markers for non-today dates are ignored and may be pruned by
  `List`.
- **Picker overlap** — `openProjectPicker` already filters out running projects
  (`today_project_picker.go:106`); keep that. The `＋` row is the only entry to
  the picker in the new path.
