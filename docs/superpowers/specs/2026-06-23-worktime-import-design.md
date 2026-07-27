# `flow worktime import` — Design

**Date:** 2026-06-23
**Status:** Approved (brainstorming)

## Problem

An old flow installation (the `main` branch) wrote worktime data into three
flat files under `~/worktime`. Soenne wants to import that historical data into
the server-authoritative rebuild so the booked times, days off, and the one
day→doc link are preserved.

Source files (verbatim formats observed):

| File | Lines | Format |
|---|---|---|
| `worktime.log` | 31 | `date<TAB>start(HH:MM)<TAB>end(HH:MM)<TAB>elapsed_seconds` |
| `worktime-dayoffs.tsv` | 52 (incl. 2 `#` comment lines) | `date<TAB>kind<TAB>label[<TAB>hours]`, kinds `holiday\|vacation\|sick` |
| `worktime-links.tsv` | 1 | `date<TAB>doc-path` (e.g. `2026-05-11<TAB>daily/2026-05-11`) |

Observed quirks:
- The `elapsed_seconds` column usually equals the clock duration
  (`08:16→16:18` = `28920`s exactly) but diverges on at least one line
  (`2026-04-24 07:34 07:42` records `259703`s ≈ 72h — a forgotten running
  timer). The clock times are the source of truth.
- `worktime-dayoffs.tsv` carries standard German public holidays
  (Neujahr, Karfreitag, …) which the rebuild computes from the user's
  Bundesland at read time and refuses to store manually.
- No row in the sample data uses the optional `hours` column (all full days).

## Key constraints (verified against the rebuild)

- `usecase.AddSession` accepts `projectID *string` (may be nil) and enforces
  `stop>start`, no-future, **same-local-day**, and the **no-overlap**
  invariant. → Build start/stop from the clock columns; **ignore
  `elapsed_seconds`** (using it would push the 72h anomaly's stop 3 days
  out and break same-day).
- `usecase.AddDayOffs` **rejects `KindHoliday`** (`ErrHolidayNotManual`) and
  **upserts** every other kind by date. → Skip `holiday` rows; non-holiday
  rows are naturally idempotent.
- The old day-off kinds `holiday`/`vacation`/`sick` are the exact rebuild
  `domain.Kind` string literals (`KindHoliday`/`KindVacation`/`KindSick`) — a
  1:1 mapping. `domain.ParseKind` additionally tolerates these.
- There is **no day→doc link table** in the rebuild; the day↔daily-doc
  relationship is encoded purely by the `daily/<date>` path convention, which
  `flow docs import` already preserves. → The single link row needs no separate
  storage.
- The apiclient already exposes every method needed —
  `AddSession`, `AddDayOffs`, `CreateProject`, `ListProjects`. **No server,
  usecase, or migration change is required.** This is a pure CLI feature,
  mirroring `flow docs import`.

## Architecture

A new cobra subcommand `flow worktime import [dir]` (default `dir = ~/worktime`),
added to the existing `worktimeCmd()`. `flow worktime` (no args) keeps launching
the TUI; `flow worktime import …` runs the importer. The importer is a thin
client over existing apiclient methods with per-row error isolation, a summary
line, and a non-zero exit on failures — the exact shape of `runImport` in
`cmd/flow/docs_import.go`.

**Files (focused, no monolith):**
- `cmd/flow/worktime_import.go` — the command, orchestration, and pure parser
  functions (`parseLogLine`, `parseDayOffLine`, time construction).
- `cmd/flow/worktime_import_test.go` — parser + planning (dry-run) tests.

## Data flow

1. **Placeholder project** — find-or-create once. `ListProjects` → match by
   `Name` or `Slug`; if absent, `CreateProject(name)`. Default name `"Import"`,
   overridable via `--project <name>`. All imported sessions hang here.
2. **`worktime.log`** → for each non-empty line: parse `date`, `start`, `end`;
   build `start`/`stop` as `time.Time` in **Europe/Berlin** from `date+HH:MM`;
   ignore `elapsed_seconds`. Call
   `AddSession(ctx, &placeholderID, start, stop, "", "")`. If
   `|elapsed_seconds − (stop−start)| > 5min`, append a warning line to the
   summary (surfaces the corrupt row without dropping it).
3. **`worktime-dayoffs.tsv`** → skip `#` comment lines and blank lines; skip
   rows whose kind is `holiday`; map the kind via `domain.ParseKind`; parse the
   optional `hours` column into `targetMin` (else 0 = full day). Call
   `AddDayOffs(ctx, date, date, kind, label, targetMin, false)` (single day,
   `from==to`, `skipWeekends=false`).
4. **`worktime-links.tsv`** → parse and count only; log each as
   "covered by daily/<date> convention". No write.

## Idempotency

- Sessions: a re-run makes `AddSession` return overlap-409; the importer treats
  a conflict as "skipped" (already imported). Uses `apiclient.IsConflict`.
- Day-offs: `AddDayOffs` upserts by date → re-running is safe.
- `--dry-run`: parse and plan (counting imported/skipped) without any write,
  including a simulated find-or-create of the placeholder project.

## Flags

- `--dry-run` — parse and plan, no writes.
- `--project <name>` — placeholder project name (default `"Import"`).

`[dir]` is an optional positional arg; default `~/worktime`.

## Error handling & summary

Per-row failures are isolated (collected, the walk continues). Final summary,
German, mirroring docs import:

```
gebucht N · freie Tage N · übersprungen N · Projekt: "Import" · Fehler N
  <warnings: e.g. "worktime.log:1: Sekunden 259703 ≠ Uhrzeit-Dauer 8m (importiere Uhrzeit)">
  <failures: "worktime-dayoffs.tsv:7: …">
```

Exit non-zero if any row failed (warnings do not fail the run).

## Testing (TDD)

- `parseLogLine`: valid line, blank line, malformed (wrong column count /
  unparseable time), and that a multi-session day yields contiguous
  non-overlapping intervals.
- `parseDayOffLine`: `#` comment skipped, `holiday` skipped, `vacation`/`sick`
  mapped, optional `hours` → `targetMin`, malformed line reported.
- Time construction: `date+HH:MM` resolves in Europe/Berlin.
- Link line: parsed and counted, never written.
- Dry-run planning against a `testdata/` fixture copy of the three files
  produces the expected counts.
- Idempotency: a second pass over the same fixture reports all rows skipped.

## Defaults (agreed)

Placeholder project name **"Import"** · default dir **`~/worktime`** ·
timezone **Europe/Berlin** · imported sessions carry no tag/note.

## Risk to verify during implementation

Multi-session days are contiguous (e.g. `…14:28` stop then `14:28…` start). The
implementation must confirm `domain.HasOverlap` treats half-open intervals
(touching boundary = no overlap) so the second session is not wrongly skipped as
an overlap. The existing rebuild TUI produces such contiguous sessions, so this
is expected to hold — but it is verified explicitly via the multi-session
parser/integration test and a live re-run check.
