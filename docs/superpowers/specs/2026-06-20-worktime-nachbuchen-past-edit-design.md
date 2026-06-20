# Worktime — Nachbuchen + Vergangene Tage editieren — Design

**Date:** 2026-06-20
**Branch:** `rebuild` (unmerged)
**Status:** approved

## Problem

Two gaps surfaced during dogfood:

1. **Nachbuchen unmöglich.** Every session-create path bottoms out at
   `Clock.Now()` server-side — there is no way to create a worktime session for a
   past day. `POST /sessions` accepts only `{projectId, tag, note}`
   (`usecase/start_session.go:24` → `NewWorkSession(..., Clock.Now())`); the CLI
   has no `session add`; the TUI's `s` calls `StartSession(..., nil, "", "")`
   without a timestamp (`dialogs.go:41`).

2. **Vergangene Buchungen nicht editierbar (außer per nacktem REST).** `PATCH`/
   `DELETE /sessions/{id}` already mutate *any* session with no "must be today"
   guard, but no UI surfaces past sessions. The TUI Heute route is double-locked
   to today: the server defaults `since = startOfDay(now)`
   (`httpserver/worktime.go:69`) and the client re-filters `sameLocalDay`
   (`today_state.go:47`). There is no date/week navigation. CLI has nothing.

Additionally, the system has **no overlap invariant**: `NewWorkSession` validates
only id/ownerID, and `EditSession` checks only `stop > start`
(`edit_session.go:28`). Nothing prevents two sessions of the same owner from
overlapping in time.

## Decisions (from brainstorming)

- **Navigation model:** *Woche → Tag aufbohren.* The Woche route becomes
  navigable (`←/→` weeks back, forward only up to the current week); `enter` on a
  week-day pushes a new **Tag-Detail** route that lists that day's sessions and
  supports view / nachbuchen / edit / delete. The **Heute** route stays the
  live-only tracker, unchanged.
- **Nachbuchen input model:** *Von–Bis Uhrzeit* — a real session with explicit
  start/stop times on the context day (+ project, optional tag/note). No
  duration-only shortcut.
- **Surfaces:** TUI **and** WebUI **and** CLI (matches the "generic features in
  every host" principle). A shared backend foundation serves all three.
- **Overlap is a general domain invariant** (not feature-scoped): no two sessions
  of one owner may overlap in time, enforced single-source across every
  session-mutating path.
- **REST shape:** extend `POST /api/v1/sessions` to accept optional `start`+`stop`
  (both present → Nachbuchen; both absent → live start, unchanged).
- **Validation rules:** `Bis > Von`; no future (`Von`,`Bis` ≤ now); same-day
  (no midnight crossing) for Nachbuchen; **overlap rejected**.

## Architecture overview

```
Backend (Slice 1) ── domain.HasOverlap (single-source) + ErrOverlap/ErrFutureSession
                  ── AddSession usecase (Create with explicit start/stop)
                  ── SessionStore.ListRange(since, until) + pgstore SQL
                  ── POST /sessions (extended) + GET /sessions?until=
                  ── apiclient.AddSession / ListSessionsRange
   │
   ├─ TUI (Slice 2)   Woche ←/→ nav + screen/worktime/daydetail route
   │                  (list day sessions, n=nachbuchen form, e/d=edit/delete)
   ├─ WebUI (Slice 3) navigable day view under /ui/worktime (list + form + edit/delete)
   └─ CLI (Slice 4)   flow session add/edit/delete/list
```

Each slice gets its own implementation plan and subagent-driven run + dogfood
gate. Backend first, because 2–4 build on it.

---

## Slice 1 — Backend foundation

### Overlap invariant (single source)

New domain-pure helper (no I/O, fully unit-testable):

```go
// HasOverlap reports whether [start, stop) intersects any session in existing,
// excluding the session whose ID == excludeID (pass "" to exclude nothing).
// A running session (Stop == nil) is treated as occupying [Start, +inf).
// Standard interval-intersection rule: a.Start < b.Stop && b.Start < a.Stop.
func HasOverlap(existing []WorkSession, start time.Time, stop *time.Time, excludeID string) bool
```

- `stop == nil` (the candidate is a live start) is treated as `[start, +inf)`.
- Lives in `internal/domain/` next to `WorkSession`. New sentinel
  `ErrOverlap` in `internal/domain/errors.go`.

Wired into **every** session-mutating usecase as the shared rule:
- `AddSession` (Nachbuchen) — primary.
- `EditSession` — gains an overlap guard (today it has none), excluding self.
- `StartSession` / `StopSession` — call the same helper for completeness; given
  the no-future rule these rarely trip, but the invariant is uniform and any
  future mutation path inherits it.

Each usecase fetches the relevant same-day sessions via `ListRange(owner,
startOfDay, endOfDay)` and calls `HasOverlap`. No extra store query type needed.

### AddSession usecase

New `internal/usecase/worktime/add_session.go` (deps: `ports.SessionStore` +
`ports.IDGen`, identical to `StartSession`):

```go
func (uc AddSession) Execute(ctx, ownerID string, projectID *string,
    start, stop time.Time, tag, note string) (domain.WorkSession, error)
```

Logic: validate `stop.After(start)` (`ErrStopBeforeStart`); validate not-future
(`start,stop ≤ Clock.Now()` → `ErrFutureSession`, new sentinel); validate
same-day (`start` and `stop` on the same local date); fetch day sessions, run
`HasOverlap` → `ErrOverlap`; build `NewWorkSession(id, owner, projectID, start)`,
set `s.Stop = &stop`, `s.Tag/Note`; `Store.Create(ctx, s)`. **No store change**
for create (`Create` already accepts a fully-built session with Stop set).

### Range listing

- New port method `SessionStore.ListRange(ctx, ownerID string, since, until time.Time) ([]WorkSession, error)`; pgstore SQL `start_at >= $2 AND start_at < $3 ORDER BY start_at`. Existing `List(since)` stays for the Heute route.
- `GET /api/v1/sessions` parses an optional `until` (RFC3339) in addition to
  `since`; default `until` = now. When both absent, behavior is unchanged
  (today's sessions).

### REST + apiclient

- Extend `handleStartSession` body to `{projectId, tag, note, start?, stop?}`:
  both timestamps present → `AddSession.Execute`; both absent → `StartSession`
  (unchanged). Mixed (only one present) → 400. `ErrOverlap`/`ErrFutureSession`
  map to 409/400 with a clear message.
- apiclient: `AddSession(ctx, projectID *string, start, stop time.Time, tag, note string)` and `ListSessionsRange(ctx, since, until time.Time)`.

### Tests (Slice 1)

- `HasOverlap`: table — disjoint, touching-edge (no overlap), partial, contained,
  identical, exclude-self, running-session-open-end.
- `AddSession`: happy path; `stop ≤ start`; future; cross-midnight; overlap reject.
- `EditSession`: overlap guard excludes self; rejects a colliding edit.
- pgstore `ListRange`: bounded window returns only in-range rows.
- REST: `POST /sessions` backfill path; mixed-timestamp 400; `until` filter on GET.

---

## Slice 2 — TUI (Woche → Tag-Detail)

### Woche navigation

- Add `weekOffset int` to `week.Route` (0 = current week). `←/→` (grammar's
  lateral/`h`/`l` already feed the strip — use distinct keys, e.g. `[`/`]` or
  `<`/`>`, to avoid clashing with sub-tab `←/→`; final binding chosen in the
  plan from `ui/grammar`). Forward clamped to offset 0 (no future weeks).
- `loadCmd()` passes a computed `ref` (`yyyy-mm-dd` of the target week) to the
  already-existing `GetWeek(ref)` — server `?ref=` plumbing exists. Header shows
  the week range / `‹ KW.. ›`.

### Tag-Detail route

- New `internal/tui/screen/worktime/daydetail/` (`shell.Route`), pushed via the
  established NavStack pattern when `enter` is pressed on a week-day row. Carries
  the selected date.
- Loads sessions via `ListSessionsRange(startOfDay, endOfDay)`; renders a list of
  `Von–Bis · Projekt · Dauer` rows.
- `n` → Nachbuchen form: project picker (reuse `ui/fuzzylist` + `mruProjects`,
  zero new infra), Von/Bis time fields, optional Tag/Notiz → `AddSession`. Date is
  the context day.
- `e` / `d` → edit / delete the selected session, reusing the existing
  edit/delete dialogs + REST, targeting the chosen day instead of "today".
- SSE `session.*` events reload the day (existing `EventMsg` pattern).
- Heute route unchanged.

### Tests (Slice 2)

Route-level (fake apiclient): week prev/next changes ref + forward-clamp;
enter→daydetail push; daydetail lists ranged sessions; nachbuchen form submits
`AddSession` with the context date; edit/delete target the selected session;
overlap/future errors surface as a toast/message.

---

## Slice 3 — WebUI

- Extend `/ui/worktime` (today only today) with a navigable day view: prev/next
  day controls, list the day's sessions, a Nachbuchen form, and per-row
  edit/delete controls wired to the extended `POST` + existing `PATCH`/`DELETE`.
  New templ fragment; regenerate `*_templ.go`.
- Tests: rendered day page lists ranged sessions; backfill form round-trips;
  overlap rejection shows an error.

## Slice 4 — CLI

- New `flow session` cobra subcommand:
  - `add --date --from --to --project [--tag --note]` → `AddSession`.
  - `edit <id> [--from --to --project --tag --note]` → `EditSession`.
  - `delete <id>` → delete.
  - `list [--date | --from --to]` → `ListSessionsRange`.
- Tests: cmd-level happy paths + validation error surfacing.

---

## Out of scope (deferred)

- Duration-only ("8h on project X") quick entry — explicitly rejected in favor of
  Von–Bis.
- Cross-midnight Nachbuchen (live sessions may still cross midnight).
- Bulk import / CSV backfill.
- Overlap *warnings* (we hard-reject) and overlap auto-merge.
- A standalone navigable "Tag" tab independent of Woche (drill-from-Woche only).

## Testing strategy

TDD throughout. Domain `HasOverlap` and the usecases are pure/fakeable; pgstore
`ListRange` covered by the Docker pgstore tests; REST via httptest; TUI via the
fake-apiclient route tests; WebUI via httptest + templ; CLI via cmd tests. Each
slice ends with `make ci` green (~83% gate) + a live done-gate vs the dev stack
(Postgres + Dex) and Soenne dogfood.
