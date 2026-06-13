# tsvsessions Deletion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Delete `internal/adapter/tsvsessions/` and migrate `usecase.SessionWriter` onto `sqliteclient.Sessions` as its sole backend, closing out the Plan-B Task 19 follow-up that was deferred as milestone-scale.

**Architecture:** SessionWriter keeps its public surface (Start/Stop/Toggle/Pause/Resume/Correct/AddManual/Edit/Delete/SetTag/SetNote) so the TUI/CLI call-sites are untouched. Internally every TSV-shaped operation rewrites onto the ID-based ports.SessionStore.Upsert/Delete contract, and the legacy `Append`/`AppendBatch`/`Rewrite` shims on `ports.SessionStore` are deleted. SessionWriter gains a `UserID` field threaded at construction so `Sessions.Load(userID)` and `Sessions.Delete(userID, id)` get the user that previously lived inside the TSV path. The idx→ID translation needed for Edit/Delete/SetTag/SetNote moves into one local helper.

**Tech Stack:** Go 1.24, modernc.org/sqlite via the existing `sqliteclient` adapter, the existing UUID v4 generator from `usecase/active_sessions.go` (`newUUID`), `make ci` as the gate.

---

## File Structure

**Modify (use case):**
- `internal/usecase/session_writer.go` — add `UserID string`; rewrite Stop/Pause/Toggle/AddManual to `Upsert` with newly generated UUIDs; rewrite Edit/Delete/SetTag/SetNote idx-paths to load + filter + Upsert/Delete by ID; add `sessionsByDate` helper.
- `internal/usecase/session_writer_lifecycle_test.go` — add UserID seed to every constructor call; assertions that compare written rows now match by Date/Start instead of by slice ordering.
- `internal/usecase/session_writer_errors_test.go` — same UserID threading as lifecycle tests.
- `internal/usecase/session_writer_manual_test.go` — same.
- `internal/usecase/session_writer_test.go` — top-level shared fakes / fixtures.

**Modify (ports):**
- `internal/ports/sessions.go` — remove the three legacy methods (`Append`, `AppendBatch`, `Rewrite`) from `SessionStore` and the matching doc-comments.

**Modify (adapters):**
- `internal/adapter/sqliteclient/sessions.go` — delete the three legacy shims (`Append`, `AppendBatch`, `Rewrite`) and the comment block above them.
- `internal/adapter/sqliteclient/sessions_test.go` — delete the `TestUnit_Sessions_LegacyShims_DelegateToUpsert` test added in Plan-B follow-up #4.
- `internal/testutil/sessions.go` — delete `Append`, `AppendBatch`, `Rewrite` on `FakeSessionStore`; keep `Upsert`/`UpsertBatch`/`Delete`/`Load`/`LoadFiltered`.
- `internal/testutil/sessions_test.go` — drop any test that exercises the deleted methods.

**Modify (composition root):**
- `cmd/flow/main.go` — replace the `tsvsessions.New(p.WorktimeLog)` constructor with `cacheSessions` (the already-existing `sqliteclient.NewSessions(cacheStore)`); thread `localUser.ID` into the `SessionWriter` construction; drop the `tsvsessions` import.

**Modify (TUI consumer that reads via SessionWriter):**
- `internal/frontend/tui/screen/worktime/history_edit.go:264` — the only direct `sw.Sessions.Load("")` outside the use case; change to `sw.Sessions.Load(sw.UserID)`.

**Delete (whole package):**
- `internal/adapter/tsvsessions/store.go`
- `internal/adapter/tsvsessions/store_test.go`
- `internal/adapter/tsvsessions/append_batch_test.go`
- `internal/adapter/tsvsessions/doc.go`

**No changes needed (verify only):**
- `internal/frontend/tui/screen/worktime/today_actions.go`, `today_dialog_submit.go`, `history_list_add.go`, `menu_correct.go` — call SessionWriter through its public API; user-blind.
- `internal/frontend/cli/worktime.go` — same.

---

## Task 1: Add UserID field to SessionWriter and rewrite the Load("") call sites

**Files:**
- Modify: `internal/usecase/session_writer.go`
- Modify: `internal/usecase/session_writer_lifecycle_test.go`
- Modify: `internal/usecase/session_writer_errors_test.go`
- Modify: `internal/usecase/session_writer_manual_test.go`
- Modify: `internal/usecase/session_writer_test.go`

- [ ] **Step 1: Add the failing assertion**

In `session_writer_lifecycle_test.go`, find the shared `mkWriter` (or equivalent constructor helper used by every Stop/Pause/Toggle test) and add a UserID assertion to one already-green test:

```go
func TestSessionWriter_UserID_FlowsToSessionsLoad(t *testing.T) {
    f := newWriterFixture(t)
    f.writer.UserID = "user-alpha"

    // Drive a Stop so SessionWriter calls Sessions.Load to dedupe.
    f.state.SetActive(f.clock.Now().Add(-time.Hour))
    _, err := f.writer.Stop()
    if err != nil {
        t.Fatalf("Stop: %v", err)
    }

    if got := f.sessions.LastLoadUserID(); got != "user-alpha" {
        t.Errorf("Sessions.Load called with userID=%q, want %q", got, "user-alpha")
    }
}
```

`FakeSessionStore` currently exposes its rows directly via the `Sessions []domain.Session` field but does not track per-call args. Add a `lastLoadUserID string` field, write to it inside the existing `Load` method, and add a `LastLoadUserID() string` accessor. Same pattern applies to the `Upserted()`/`LastDeleteID()` accessors used in Tasks 2-5 — add a `Upserted() []domain.Session { return f.Sessions }` helper (rows already accumulate there) and a `lastDeleteID` field written by `Delete`.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/usecase/... -run TestSessionWriter_UserID_FlowsToSessionsLoad -v
```

Expected: FAIL with `Sessions.Load called with userID="", want "user-alpha"` (the field doesn't exist yet, so the assertion sees the zero-value Load).

- [ ] **Step 3: Add the UserID field and thread it through every Load call**

In `internal/usecase/session_writer.go`:

```go
type SessionWriter struct {
    Sessions ports.SessionStore
    State    ports.LegacyActiveStore
    Lock     ports.Lock
    Reader   *WorktimeReader
    Clock    ports.Clock

    // UserID scopes every Sessions.Load / Sessions.Delete call so the
    // sqliteclient backend can multiplex multiple users in the same
    // cache.db. Set by the composition root at construction time;
    // SessionWriter itself never mutates it.
    UserID string
}
```

Replace every `w.Sessions.Load("")` inside this file with `w.Sessions.Load(w.UserID)`. There are three such call-sites at the moment: inside `stopAt` (dedupe), `Delete`, and `rewriteAtIndexLocked`.

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/usecase/... -run TestSessionWriter_UserID_FlowsToSessionsLoad -v
```

Expected: PASS.

- [ ] **Step 5: Wire UserID in every other writer-test fixture**

In `session_writer_lifecycle_test.go`, `session_writer_errors_test.go`, `session_writer_manual_test.go`, find the constructor (`SessionWriter{Sessions: …, State: …, Lock: …, Reader: …, Clock: …}` — there is one per test file in `newWriterFixture` / `newFixture`) and add `UserID: "test-user"`. Pick a stable literal so subsequent assertions can match on it.

Run the full use-case test suite to make sure nothing regressed:

```bash
go test ./internal/usecase/... -v
```

Expected: PASS for every Test*.

- [ ] **Step 6: Commit**

```bash
git add internal/usecase/session_writer.go \
        internal/usecase/session_writer_lifecycle_test.go \
        internal/usecase/session_writer_errors_test.go \
        internal/usecase/session_writer_manual_test.go \
        internal/usecase/session_writer_test.go \
        internal/testutil/sessions.go
git commit -m "feat(session_writer): add UserID field, thread into Sessions.Load"
```

---

## Task 2: Migrate Stop/Pause/Toggle to Upsert + UUIDs

**Files:**
- Modify: `internal/usecase/session_writer.go`
- Modify: `internal/usecase/session_writer_lifecycle_test.go`

The three lifecycle paths (`stopAt`, `Pause`, `Toggle`) currently call `Sessions.AppendBatch(domain.SplitAtMidnight(...))`. The sqlite path needs each part to have an ID and UserID before it can be Upserted. `AppendBatch` is going away in Task 7.

- [ ] **Step 1: Write the failing test**

```go
func TestStopAt_UpsertsSessionsWithIDAndUserID(t *testing.T) {
    f := newWriterFixture(t)
    f.writer.UserID = "user-beta"
    start := f.clock.Now().Add(-2 * time.Hour)
    f.state.SetActive(start)

    if _, err := f.writer.Stop(); err != nil {
        t.Fatalf("Stop: %v", err)
    }

    upserted := f.sessions.Upserted()
    if len(upserted) != 1 {
        t.Fatalf("upserted len = %d, want 1", len(upserted))
    }
    if upserted[0].ID == "" {
        t.Error("ID is empty on upserted session")
    }
    if upserted[0].UserID != "user-beta" {
        t.Errorf("UserID = %q, want %q", upserted[0].UserID, "user-beta")
    }
}
```

`FakeSessionStore.Upserted()` was added in Task 1 Step 1 — reuse it.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/usecase/... -run TestStopAt_UpsertsSessionsWithIDAndUserID -v
```

Expected: FAIL — `len(upserted) = 0` (the path still uses `AppendBatch`).

- [ ] **Step 3: Replace AppendBatch with per-row Upsert in stopAt/Pause/Toggle**

Inside `stopAt` rewrite the persistence block:

```go
toAppend := dedupeSessionParts(parts, existing)
for i := range toAppend {
    if toAppend[i].ID == "" {
        toAppend[i].ID = newUUID()
    }
    toAppend[i].UserID = w.UserID
    toAppend[i].UpdatedAt = w.Clock.Now().UTC()
    if err := w.Sessions.Upsert(toAppend[i]); err != nil {
        return err
    }
}
```

Apply the same shape inside `Pause` (drop `dedupeSessionParts` there — Pause does not retry, so no dedupe is needed) and `Toggle`.

`newUUID` already exists in `internal/usecase/active_sessions.go`. Calling it from `session_writer.go` is fine; both files are in the `usecase` package.

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/usecase/... -v
```

Expected: PASS for every Test*. The dedupe test in lifecycle (`TestStop_DedupesAgainstExistingSessions` if present) still passes because the loop runs Upsert per surviving part.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/session_writer.go internal/usecase/session_writer_lifecycle_test.go
git commit -m "refactor(session_writer): persist lifecycle parts via per-row Upsert"
```

---

## Task 3: Migrate AddManual to per-row Upsert

**Files:**
- Modify: `internal/usecase/session_writer.go`
- Modify: `internal/usecase/session_writer_manual_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestAddManual_UpsertsEachPartWithUUID(t *testing.T) {
    f := newWriterFixture(t)
    f.writer.UserID = "user-gamma"

    start := time.Date(2026, 6, 3, 23, 30, 0, 0, time.Local)
    stop := time.Date(2026, 6, 4, 1, 0, 0, 0, time.Local)

    if err := f.writer.AddManual(start, start, stop); err != nil {
        t.Fatalf("AddManual: %v", err)
    }
    rows := f.sessions.Upserted()
    if len(rows) != 2 {
        t.Fatalf("Upserted len = %d, want 2 (midnight split)", len(rows))
    }
    seen := map[string]bool{}
    for _, r := range rows {
        if r.ID == "" {
            t.Error("manual session ID is empty")
        }
        if seen[r.ID] {
            t.Errorf("duplicate manual ID %q", r.ID)
        }
        seen[r.ID] = true
    }
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/usecase/... -run TestAddManual_UpsertsEachPartWithUUID -v
```

Expected: FAIL.

- [ ] **Step 3: Replace the AddManual AppendBatch with the same per-row Upsert loop**

In `AddManual`, replace the trailing `return w.Sessions.AppendBatch(parts)` with the loop from Task 2 Step 3.

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/usecase/... -run TestAddManual -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/session_writer.go internal/usecase/session_writer_manual_test.go
git commit -m "refactor(session_writer): persist AddManual parts via per-row Upsert"
```

---

## Task 4: Convert Delete from index+Rewrite to ID-based Sessions.Delete

**Files:**
- Modify: `internal/usecase/session_writer.go`
- Modify: `internal/usecase/session_writer_lifecycle_test.go`

The current `Delete(date, idx)` resolves the row at idx, then calls `Sessions.Rewrite(filtered)`. With the legacy API gone we need the row's ID and `Sessions.Delete(userID, id)`.

- [ ] **Step 1: Write the failing test**

```go
func TestDelete_RemovesByIDViaSessionStore(t *testing.T) {
    f := newWriterFixture(t)
    f.writer.UserID = "user-delta"

    date := time.Date(2026, 6, 3, 0, 0, 0, 0, time.Local)
    seedID := "session-to-delete"
    if err := f.sessions.Upsert(domain.Session{
        ID: seedID, UserID: "user-delta",
        Date: date, Start: date.Add(9 * time.Hour), Stop: date.Add(10 * time.Hour),
        Elapsed: time.Hour, UpdatedAt: date,
    }); err != nil {
        t.Fatalf("seed: %v", err)
    }

    if err := f.writer.Delete(date, 0); err != nil {
        t.Fatalf("Delete: %v", err)
    }

    if got := f.sessions.LastDeleteID(); got != seedID {
        t.Errorf("Delete called with id=%q, want %q", got, seedID)
    }
}
```

`FakeSessionStore.LastDeleteID()` was added in Task 1 Step 1 — reuse it.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/usecase/... -run TestDelete_RemovesByIDViaSessionStore -v
```

Expected: FAIL — the implementation still calls `Sessions.Rewrite`.

- [ ] **Step 3: Rewrite Delete**

```go
func (w *SessionWriter) Delete(date time.Time, idx int) error {
    return w.Lock.With(func() error {
        target, ok, err := w.sessionByDayIndex(date, idx)
        if err != nil {
            return err
        }
        if !ok {
            return domain.ErrSessionNotFound
        }
        return w.Sessions.Delete(w.UserID, target.ID)
    })
}

// sessionByDayIndex loads the user's sessions, filters by date, sorts by
// Start, and returns the (idx-th, true) row or (zero, false) when idx is
// out of range. Date comparison uses YYYY-MM-DD string equality so a row
// stored without a UTC normalisation still matches.
func (w *SessionWriter) sessionByDayIndex(date time.Time, idx int) (domain.Session, bool, error) {
    all, err := w.Sessions.Load(w.UserID)
    if err != nil {
        return domain.Session{}, false, err
    }
    dateStr := date.Format("2006-01-02")
    var day []domain.Session
    for _, s := range all {
        if s.Date.Format("2006-01-02") == dateStr {
            day = append(day, s)
        }
    }
    sort.Slice(day, func(i, j int) bool { return day[i].Start.Before(day[j].Start) })
    if idx < 0 || idx >= len(day) {
        return domain.Session{}, false, nil
    }
    return day[idx], true, nil
}
```

Add `"sort"` to the imports.

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/usecase/... -run TestDelete -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/session_writer.go internal/usecase/session_writer_lifecycle_test.go internal/testutil/sessions.go
git commit -m "refactor(session_writer): Delete resolves idx→ID and calls Sessions.Delete"
```

---

## Task 5: Convert Edit/SetTag/SetNote from Rewrite to Upsert-by-ID

**Files:**
- Modify: `internal/usecase/session_writer.go`
- Modify: `internal/usecase/session_writer_errors_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestSetTag_UpsertsByIDPreservingOtherFields(t *testing.T) {
    f := newWriterFixture(t)
    f.writer.UserID = "user-epsilon"

    date := time.Date(2026, 6, 3, 0, 0, 0, 0, time.Local)
    seed := domain.Session{
        ID: "seed-1", UserID: "user-epsilon",
        Date: date, Start: date.Add(9 * time.Hour), Stop: date.Add(10 * time.Hour),
        Elapsed: time.Hour, Tag: "old", Note: "preserved", UpdatedAt: date,
    }
    if err := f.sessions.Upsert(seed); err != nil {
        t.Fatalf("seed: %v", err)
    }

    if err := f.writer.SetTag(date, 0, "new"); err != nil {
        t.Fatalf("SetTag: %v", err)
    }

    after, _ := f.sessions.Load("user-epsilon")
    if len(after) != 1 {
        t.Fatalf("session count = %d, want 1", len(after))
    }
    if after[0].ID != "seed-1" {
        t.Errorf("ID changed: %q, want seed-1", after[0].ID)
    }
    if after[0].Tag != "new" {
        t.Errorf("Tag = %q, want new", after[0].Tag)
    }
    if after[0].Note != "preserved" {
        t.Errorf("Note clobbered: %q", after[0].Note)
    }
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/usecase/... -run TestSetTag_UpsertsByIDPreservingOtherFields -v
```

Expected: FAIL.

- [ ] **Step 3: Rewrite `rewriteAtIndexLocked` to Upsert by ID**

```go
func (w *SessionWriter) rewriteAtIndexLocked(date time.Time, idx int, fn func(domain.Session) domain.Session) error {
    target, ok, err := w.sessionByDayIndex(date, idx)
    if err != nil {
        return err
    }
    if !ok {
        return domain.ErrSessionNotFound
    }
    updated := fn(target)
    updated.ID = target.ID         // never let fn override identity
    updated.UserID = w.UserID      // ensure user scope persists
    updated.UpdatedAt = w.Clock.Now().UTC()
    return w.Sessions.Upsert(updated)
}
```

`Edit` already calls this through `rewriteAtIndexLocked` after the overlap check; no change needed at the Edit call site itself.

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/usecase/... -run "TestSetTag|TestSetNote|TestEdit" -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/session_writer.go internal/usecase/session_writer_errors_test.go
git commit -m "refactor(session_writer): Edit/SetTag/SetNote upsert by ID, preserve identity"
```

---

## Task 6: Wire SessionWriter onto sqliteclient.Sessions in the composition root

**Files:**
- Modify: `cmd/flow/main.go`

The TUI/CLI test suites are already green at this point because they exercise SessionWriter against the in-memory fake. This task switches the production wiring.

- [ ] **Step 1: Locate the existing tsvsessions wiring**

```bash
rg -n 'tsvsessions\.' cmd/flow/main.go
```

Expected output:
```
cmd/flow/main.go:118:    sessionStore := tsvsessions.New(p.WorktimeLog)
... + an import line for github.com/serverkraken/flow/internal/adapter/tsvsessions
```

- [ ] **Step 2: Swap the constructor and drop the import**

In `cmd/flow/main.go`:

```go
// Sessions persist exclusively through the sqlite cache after Plan-B
// follow-up #1. The legacy TSV adapter is gone — see Task 19 in the
// M2-M3 plan for the migration entry point users run once.
sessionStore := cacheSessions
```

`cacheSessions` is the already-built `*sqliteclient.Sessions` value (search upward in `main.go` for `cacheSessions := sqliteclient.NewSessions(cacheStore)` — it is constructed before the SessionWriter block as part of the Plan-B wiring).

In the same block, where `SessionWriter` is built, add `UserID: localUser.ID`:

```go
sessionWriter := &usecase.SessionWriter{
    Sessions: sessionStore,
    State:    activeStore,
    Lock:     fileLock,
    Reader:   reader,
    Clock:    clock,
    UserID:   localUser.ID,
}
```

Remove the `github.com/serverkraken/flow/internal/adapter/tsvsessions` import. Run `go build ./...` to check for an unused-import lint error and remove it if the linter flags it.

- [ ] **Step 3: Build and smoke**

```bash
go build ./...
```

Expected: success, no unused-import warnings.

```bash
make ci
```

Expected: PASS — every test that was green before still green, coverage gate ≥ 85.

- [ ] **Step 4: Commit**

```bash
git add cmd/flow/main.go
git commit -m "feat(wiring): SessionWriter persists to sqliteclient.Sessions, drop tsv adapter wiring"
```

---

## Task 7: Remove the legacy methods from `ports.SessionStore` and the matching shims

**Files:**
- Modify: `internal/ports/sessions.go`
- Modify: `internal/adapter/sqliteclient/sessions.go`
- Modify: `internal/adapter/sqliteclient/sessions_test.go`
- Modify: `internal/testutil/sessions.go`

All production callers of `Append`/`AppendBatch`/`Rewrite` are gone after Tasks 2-5. This task strips the interface so the legacy shape can never sneak back in.

- [ ] **Step 1: Confirm no remaining callers**

```bash
rg -n '\.Append\b|\.AppendBatch\b|\.Rewrite\b' internal/ cmd/
```

Expected: only matches inside test files asserting against the now-removed shims (e.g. `TestUnit_Sessions_LegacyShims_DelegateToUpsert` from Plan-B follow-up #4), plus the shim implementations themselves. Anything outside those files means a Task 2-5 caller was missed and must be migrated first.

- [ ] **Step 2: Delete the three methods from the port**

In `internal/ports/sessions.go`, remove the `Append`, `AppendBatch`, `Rewrite` method declarations from the `SessionStore` interface, and drop the doc-comment block that references "Task 19" / "tsvsessions adapter" above the section.

- [ ] **Step 3: Delete the three shims from sqliteclient.Sessions**

In `internal/adapter/sqliteclient/sessions.go`, delete the trailing comment block and the three functions `Append`, `AppendBatch`, `Rewrite`. The interface assertion (`var _ ports.SessionStore = (*Sessions)(nil)`) right above must still compile after the change.

In `internal/adapter/sqliteclient/sessions_test.go`, delete `TestUnit_Sessions_LegacyShims_DelegateToUpsert`.

- [ ] **Step 4: Delete the three legacy methods on the test fake**

In `internal/testutil/sessions.go`, remove `Append`, `AppendBatch`, `Rewrite` on `FakeSessionStore` (the existing `Upsert`/`UpsertBatch`/`Delete` paths cover all production needs). Drop any matching tests in `internal/testutil/sessions_test.go` if they exist.

- [ ] **Step 5: Run the build + tests**

```bash
go build ./...
go test ./...
```

Expected: PASS — no compile errors, no failing tests.

- [ ] **Step 6: Commit**

```bash
git add internal/ports/sessions.go \
        internal/adapter/sqliteclient/sessions.go \
        internal/adapter/sqliteclient/sessions_test.go \
        internal/testutil/sessions.go \
        internal/testutil/sessions_test.go
git commit -m "refactor(ports): drop Append/AppendBatch/Rewrite from SessionStore"
```

---

## Task 8: Fix the one TUI consumer that calls `sw.Sessions.Load("")` directly

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/history_edit.go`

`history_edit.go:264` reaches past the SessionWriter facade to call `sw.Sessions.Load("")` directly. Now that SessionWriter knows its UserID, the caller should use it too.

- [ ] **Step 1: Inspect the call site**

```bash
rg -n 'sw\.Sessions\.' internal/frontend/tui/screen/worktime/history_edit.go
```

Expected output: a single line at ~`history_edit.go:264` showing `sw.Sessions.Load("")`.

- [ ] **Step 2: Change the call**

Replace `sw.Sessions.Load("")` with `sw.Sessions.Load(sw.UserID)`. The local `sw` is the SessionWriter handed in via `h.deps.SessionWriter`, which already has `UserID` set after Task 6.

- [ ] **Step 3: Build + targeted test**

```bash
go build ./...
go test ./internal/frontend/tui/screen/worktime/... -run TestHistory -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/frontend/tui/screen/worktime/history_edit.go
git commit -m "refactor(tui): history_edit loads sessions for sw.UserID, not all users"
```

---

## Task 9: Delete the tsvsessions package

**Files:**
- Delete: `internal/adapter/tsvsessions/store.go`
- Delete: `internal/adapter/tsvsessions/store_test.go`
- Delete: `internal/adapter/tsvsessions/append_batch_test.go`
- Delete: `internal/adapter/tsvsessions/doc.go`

- [ ] **Step 1: Verify nothing imports the package**

```bash
rg -n 'serverkraken/flow/internal/adapter/tsvsessions' .
```

Expected: zero matches. Any hit must be fixed first — a leftover import indicates a Task 6 gap.

- [ ] **Step 2: Remove the package directory**

```bash
git rm -r internal/adapter/tsvsessions/
```

- [ ] **Step 3: Build + full test run**

```bash
make ci
```

Expected: PASS. Coverage may shift downward by ≤ 0.3 percentage points because the deleted package contributed a few well-tested statements; this is acceptable inside the existing 85% gate. If the gate trips, bump `COVER_THRESHOLD` in the Makefile by one point with a one-line justification.

- [ ] **Step 4: Commit**

```bash
git add internal/adapter/tsvsessions/
git commit -m "chore(repo): delete tsvsessions adapter — sqliteclient is the sole backend"
```

---

## Task 10: Verification + memory note

**Files:**
- No code changes.

- [ ] **Step 1: Full CI**

```bash
make ci
```

Expected: PASS, coverage ≥ 85%.

- [ ] **Step 2: TSV round-trip smoke**

The migration verb (`flow worktime migrate-from-tsv --tsv=<path>`) reads the TSV and writes via the same `SessionWriter`/`sqliteclient.Sessions` path. Run it against the smoke fixture used by Plan-B Task 19 to make sure ingestion still works:

```bash
go build -o /tmp/flow ./cmd/flow
/tmp/flow worktime migrate-from-tsv --tsv=docs/runbook/fixtures/worktime-smoke.tsv \
                                    --user=test-user \
                                    --db=/tmp/flow-tsv-smoke.db
```

Expected: idempotent — running twice yields the same row count. Spot-check with:

```bash
sqlite3 /tmp/flow-tsv-smoke.db 'SELECT COUNT(*) FROM sessions;'
```

If the runbook fixture lives at a different path on disk, substitute it; otherwise create one minimal TSV (two rows is enough) before running the verb.

- [ ] **Step 3: Update the Plan-B memory note**

Open `~/.claude/projects/-Users-msoent-SourceCode-serverkraken-flow/memory/project_plan_b_progress.md` and mark deferred item #1 (tsvsessions-Adapter-Deletion) as resolved with the squash-commit SHA produced by merging this branch.

- [ ] **Step 4: Final commit (only if any test fixtures were touched)**

```bash
git status
```

If clean, no commit needed — Tasks 1-9 already covered everything substantive.
