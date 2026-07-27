# Worktime Nachbuchen — Slice 1 (Backend) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the backend foundation for "Nachbuchen" (creating a worktime session with an explicit past start/stop) and for listing/editing past sessions: a single-source overlap invariant, an `AddSession` usecase, a bounded `ListRange` store method, the REST + apiclient surface, with no overlapping sessions allowed.

**Architecture:** A pure `domain.HasOverlap` helper is the single source of the "no overlapping sessions" rule. It is enforced in the two usecases that accept arbitrary intervals — the new `AddSession` (backfill) and the existing `EditSession`. Creating a complete session reuses the existing `SessionStore.Create` (it already persists a fully-built session with `Stop` set), so no store change is needed for create; only a new bounded `ListRange(since, until)` read is added. The existing `POST /api/v1/sessions` is extended to accept optional `start`+`stop` (present → backfill, absent → live start, unchanged), and `GET /api/v1/sessions` gains an optional `until`.

**Tech Stack:** Go, hexagonal layering (`domain` → `usecase` → `ports` → adapters), Postgres (pgstore via pgx), `net/http` + `httptest`, TDD.

## Global Constraints

- Hexagonal layering: `internal/domain` imports no I/O; usecases depend on `ports` interfaces, never concrete adapters. Read `CLAUDE-hexagonal-plan.md` before touching layer boundaries.
- `make ci` must stay green (golangci-lint `0 issues`, `go tool templ generate` drift check, build, all tests, coverage gate ~80% — currently 84.0%).
- New domain sentinels live in `internal/domain/errors.go`.
- Overlap rule is **single-source** in `domain.HasOverlap`; do not duplicate the interval math anywhere else.
- Interval semantics are half-open `[start, stop)`; touching edges (`a.Stop == b.Start`) do **not** overlap. A running session (`Stop == nil`) occupies `[start, +inf)`.
- The existing live-start path (`POST /sessions` with no `start`/`stop`) and the existing `GET /sessions` with no `until` must behave **exactly** as before (no regression).
- Owner-scoping is enforced by the stores; usecases pass `ownerID` through.
- Adding a method to `ports.SessionStore` requires updating **both** implementations: `internal/adapter/pgstore/sessions.go` and `internal/testutil/fakes.go` (`FakeSessionStore`). These are the only two implementers.

---

## File Structure

- `internal/domain/overlap.go` — new: `HasOverlap` + unexported interval helpers (Task 1)
- `internal/domain/errors.go` — add `ErrOverlap`, `ErrFutureSession` (Task 1)
- `internal/ports/ports.go` — add `ListRange` to `SessionStore` (Task 2)
- `internal/adapter/pgstore/sessions.go` — implement `ListRange` (Task 2)
- `internal/testutil/fakes.go` — implement `FakeSessionStore.ListRange` (Task 2)
- `internal/usecase/add_session.go` — new: `AddSession` usecase (Task 3)
- `internal/usecase/edit_session.go` — add overlap guard (Task 4)
- `internal/usecase/list_sessions_range.go` — new: `ListSessionsRange` usecase (Task 5)
- `internal/adapter/httpserver/server.go` — add `AddSession`, `ListSessionsRange` fields + nothing new in routes (Task 5)
- `internal/adapter/httpserver/worktime.go` — extend `startReq` + `handleStartSession`; extend `handleListSessions` (Task 5)
- `cmd/flow-server/main.go` — wire the two new usecases (Task 5)
- `internal/adapter/apiclient/client.go` — `AddSession`, `ListSessionsRange` (Task 6)

Tests live beside each (`*_test.go`), plus pgstore Docker tests for `ListRange`.

---

## Task 1: domain.HasOverlap + sentinels

**Files:**
- Create: `internal/domain/overlap.go`
- Modify: `internal/domain/errors.go`
- Test: `internal/domain/overlap_test.go`

**Interfaces:**
- Produces: `domain.HasOverlap(existing []WorkSession, start time.Time, stop *time.Time, excludeID string) bool`; sentinels `domain.ErrOverlap`, `domain.ErrFutureSession`.

- [ ] **Step 1: Write the failing test**

Create `internal/domain/overlap_test.go`:

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func at(h, m int) time.Time {
	return time.Date(2026, 6, 15, h, m, 0, 0, time.UTC)
}

func ptr(t time.Time) *time.Time { return &t }

func sess(id string, startH, startM int, stop *time.Time) domain.WorkSession {
	return domain.WorkSession{ID: id, OwnerID: "u1", Start: at(startH, startM), Stop: stop}
}

func TestHasOverlap(t *testing.T) {
	t.Parallel()
	existing := []domain.WorkSession{
		sess("a", 9, 0, ptr(at(11, 0))), // 09:00–11:00
		sess("b", 13, 0, ptr(at(14, 0))), // 13:00–14:00
	}
	cases := []struct {
		name      string
		start     time.Time
		stop      *time.Time
		excludeID string
		want      bool
	}{
		{"disjoint before", at(7, 0), ptr(at(8, 0)), "", false},
		{"disjoint between", at(11, 30), ptr(at(12, 0)), "", false},
		{"touching edge end", at(11, 0), ptr(at(12, 0)), "", false},   // a.Stop == start
		{"touching edge start", at(8, 0), ptr(at(9, 0)), "", false},   // stop == a.Start
		{"partial overlap left", at(8, 30), ptr(at(9, 30)), "", true},
		{"partial overlap right", at(10, 30), ptr(at(11, 30)), "", true},
		{"contained", at(9, 30), ptr(at(10, 0)), "", true},
		{"contains existing", at(8, 0), ptr(at(15, 0)), "", true},
		{"identical to a", at(9, 0), ptr(at(11, 0)), "", true},
		{"identical but excluded", at(9, 0), ptr(at(11, 0)), "a", false},
		{"running candidate over a", at(10, 0), nil, "", true},        // [10:00,+inf) hits a
		{"running candidate after all", at(15, 0), nil, "", false},    // [15:00,+inf) hits nothing
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := domain.HasOverlap(existing, c.start, c.stop, c.excludeID); got != c.want {
				t.Errorf("HasOverlap(%s) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

func TestHasOverlap_RunningExisting(t *testing.T) {
	t.Parallel()
	existing := []domain.WorkSession{sess("run", 9, 0, nil)} // 09:00–+inf
	if !domain.HasOverlap(existing, at(10, 0), ptr(at(10, 30)), "") {
		t.Error("candidate inside a running session must overlap")
	}
	if domain.HasOverlap(existing, at(8, 0), ptr(at(9, 0)), "") {
		t.Error("candidate ending at the running session's start must not overlap")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/ -run HasOverlap -v`
Expected: FAIL (`undefined: domain.HasOverlap`)

- [ ] **Step 3: Implement `internal/domain/overlap.go`**

```go
package domain

import "time"

// HasOverlap reports whether the half-open interval [start, stop) intersects any
// session in existing, skipping the session whose ID == excludeID (pass "" to
// skip nothing). A nil stop means an open-ended running interval [start, +inf).
//
// This is the single source of the "no two sessions of one owner may overlap"
// rule; every path that persists an arbitrary interval (AddSession, EditSession)
// calls it. Touching edges do not overlap (the interval is half-open).
func HasOverlap(existing []WorkSession, start time.Time, stop *time.Time, excludeID string) bool {
	for _, e := range existing {
		if e.ID == excludeID {
			continue
		}
		if intervalsIntersect(start, stop, e.Start, e.Stop) {
			return true
		}
	}
	return false
}

// intervalsIntersect reports whether [aStart, aStop) and [bStart, bStop) overlap,
// where a nil stop denotes +inf. Rule: aStart < bStop && bStart < aStop.
func intervalsIntersect(aStart time.Time, aStop *time.Time, bStart time.Time, bStop *time.Time) bool {
	return beforeEnd(aStart, bStop) && beforeEnd(bStart, aStop)
}

// beforeEnd reports t < end, treating a nil end as +inf (always true).
func beforeEnd(t time.Time, end *time.Time) bool {
	return end == nil || t.Before(*end)
}
```

Add to `internal/domain/errors.go` (inside the `var (...)` block, after `ErrProjectRequired`):

```go
	ErrFutureSession   = errors.New("session times must not be in the future")
	ErrOverlap         = errors.New("session overlaps an existing session")
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/overlap.go internal/domain/overlap_test.go internal/domain/errors.go
git commit -m "feat(domain): HasOverlap single-source interval rule + ErrOverlap/ErrFutureSession"
```

---

## Task 2: SessionStore.ListRange (interface + pgstore + fake)

**Files:**
- Modify: `internal/ports/ports.go` (SessionStore interface, ~line 77-87)
- Modify: `internal/adapter/pgstore/sessions.go`
- Modify: `internal/testutil/fakes.go` (FakeSessionStore, ~line 222)
- Test: `internal/testutil/fakes_session_mutation_test.go` (append)
- Test (Docker): `internal/adapter/pgstore/sessions_test.go` (append, if the file exists; else create)

**Interfaces:**
- Consumes: nothing new.
- Produces: `SessionStore.ListRange(ctx context.Context, ownerID string, since, until time.Time) ([]domain.WorkSession, error)` — sessions with `since <= Start < until`, newest first.

- [ ] **Step 1: Write the failing test (fake store)**

Append to `internal/testutil/fakes_session_mutation_test.go`:

```go
func TestFakeSessionStore_ListRange(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	mk := func(id string, h int) domain.WorkSession {
		start := time.Date(2026, 6, 15, h, 0, 0, 0, time.UTC)
		stop := start.Add(time.Hour)
		return domain.WorkSession{ID: id, OwnerID: "u1", Start: start, Stop: &stop}
	}
	for _, ws := range []domain.WorkSession{mk("a", 8), mk("b", 10), mk("c", 23)} {
		if _, err := ss.Create(ctx, ws); err != nil {
			t.Fatalf("seed %s: %v", ws.ID, err)
		}
	}
	since := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	until := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	got, err := ss.ListRange(ctx, "u1", since, until)
	if err != nil {
		t.Fatalf("ListRange: %v", err)
	}
	if len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("ListRange = %+v, want only b", got)
	}
	// foreign owner sees nothing
	if g, _ := ss.ListRange(ctx, "other", since, until); len(g) != 0 {
		t.Fatalf("foreign ListRange = %+v, want empty", g)
	}
}
```

(`context`, `time`, `domain`, `testutil` are already imported in this test file; add any that is missing.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/testutil/ -run ListRange -v`
Expected: FAIL (`ss.ListRange undefined`)

- [ ] **Step 3a: Add to the `SessionStore` interface in `internal/ports/ports.go`**

Insert after the `List(...)` line (line ~81):

```go
	// ListRange returns sessions with since <= Start < until, newest first.
	// Owner-scoped. Used for past-day views and the overlap check.
	ListRange(ctx context.Context, ownerID string, since, until time.Time) ([]domain.WorkSession, error)
```

- [ ] **Step 3b: Implement in `internal/adapter/pgstore/sessions.go`**

Add after the `List` method:

```go
func (s *SessionStore) ListRange(ctx context.Context, ownerID string, since, until time.Time) ([]domain.WorkSession, error) {
	const q = `
SELECT id, owner_id, project_id, tag, note, start_at, stop_at, created_at
FROM work_sessions WHERE owner_id=$1 AND start_at >= $2 AND start_at < $3
ORDER BY start_at DESC`
	rows, err := s.pool.Query(ctx, q, ownerID, since, until)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list sessions range: %w", err)
	}
	defer rows.Close()
	var out []domain.WorkSession
	for rows.Next() {
		ws, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ws)
	}
	return out, rows.Err()
}
```

- [ ] **Step 3c: Implement in `internal/testutil/fakes.go`**

Add after the `FakeSessionStore.List` method (~line 232):

```go
func (s *FakeSessionStore) ListRange(_ context.Context, ownerID string, since, until time.Time) ([]domain.WorkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.WorkSession
	for _, e := range s.m {
		if e.OwnerID == ownerID && !e.Start.Before(since) && e.Start.Before(until) {
			out = append(out, e)
		}
	}
	return out, nil
}
```

- [ ] **Step 3d: Add a pgstore Docker test**

In `internal/adapter/pgstore/sessions_test.go` (append; if the file does not exist, create it mirroring the package's existing pgstore test setup — find the shared pool/skip helper with `rg -n "func.*testing.T.*pool|t.Skip" internal/adapter/pgstore/*_test.go` and reuse it verbatim). Use the same harness the other pgstore session tests use:

```go
func TestSessionStore_ListRange(t *testing.T) {
	pool := newTestPool(t) // reuse the existing helper name found in this package
	store := pgstore.NewSessionStore(pool)
	ctx := context.Background()
	owner := "u-range-" + t.Name()
	mk := func(id string, h int) domain.WorkSession {
		start := time.Date(2026, 6, 15, h, 0, 0, 0, time.UTC)
		stop := start.Add(time.Hour)
		return domain.WorkSession{ID: id, OwnerID: owner, Start: start, Stop: &stop, CreatedAt: start}
	}
	for _, ws := range []domain.WorkSession{mk("r-a", 8), mk("r-b", 10), mk("r-c", 23)} {
		if _, err := store.Create(ctx, ws); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	got, err := store.ListRange(ctx, owner,
		time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ListRange: %v", err)
	}
	if len(got) != 1 || got[0].ID != "r-b" {
		t.Fatalf("ListRange = %+v, want only r-b", got)
	}
}
```

If the pgstore tests require a running Postgres and the harness `t.Skip`s without one, that skip is acceptable — `make ci` runs them where the DB is available.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/testutil/ ./internal/ports/ -v` then `go build ./...`
Expected: PASS and clean build (pgstore compiles with the new method).

- [ ] **Step 5: Commit**

```bash
git add internal/ports/ports.go internal/adapter/pgstore/sessions.go internal/testutil/fakes.go internal/testutil/fakes_session_mutation_test.go internal/adapter/pgstore/sessions_test.go
git commit -m "feat(store): SessionStore.ListRange(since,until) — pgstore + fake"
```

---

## Task 3: AddSession usecase

**Files:**
- Create: `internal/usecase/add_session.go`
- Test: `internal/usecase/add_session_test.go`

**Interfaces:**
- Consumes: `ports.SessionStore` (`Create`, `ListRange`), `ports.IDGen`, `ports.Clock`; `domain.HasOverlap`; sentinels `domain.ErrStopBeforeStart`, `domain.ErrFutureSession`, `domain.ErrInvalidSession`, `domain.ErrOverlap`.
- Produces: `usecase.AddSession{ Sessions ports.SessionStore; IDs ports.IDGen; Clock ports.Clock }` with `Execute(ctx, ownerID string, projectID *string, start, stop time.Time, tag, note string) (domain.WorkSession, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/usecase/add_session_test.go`:

```go
package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func newAddSession(ss *testutil.FakeSessionStore, now time.Time) usecase.AddSession {
	return usecase.AddSession{Sessions: ss, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: now}}
}

func TestAddSession_HappyPath(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	now := time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)
	uc := newAddSession(ss, now)
	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	pid := "p1"
	got, err := uc.Execute(ctx, "u1", &pid, start, stop, "deep", "n")
	if err != nil {
		t.Fatalf("AddSession: %v", err)
	}
	if got.ID == "" || got.Stop == nil || !got.Stop.Equal(stop) || got.Tag != "deep" {
		t.Fatalf("AddSession result wrong: %+v", got)
	}
	if got.CreatedAt != start {
		t.Errorf("CreatedAt = %v, want start %v", got.CreatedAt, start)
	}
}

func TestAddSession_StopBeforeStart(t *testing.T) {
	ctx := context.Background()
	uc := newAddSession(testutil.NewFakeSessionStore(), time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC))
	start := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	if _, err := uc.Execute(ctx, "u1", nil, start, stop, "", ""); !errors.Is(err, domain.ErrStopBeforeStart) {
		t.Fatalf("want ErrStopBeforeStart, got %v", err)
	}
}

func TestAddSession_Future(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	uc := newAddSession(testutil.NewFakeSessionStore(), now)
	start := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC) // after now
	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	if _, err := uc.Execute(ctx, "u1", nil, start, stop, "", ""); !errors.Is(err, domain.ErrFutureSession) {
		t.Fatalf("want ErrFutureSession, got %v", err)
	}
}

func TestAddSession_CrossMidnight(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	uc := newAddSession(testutil.NewFakeSessionStore(), now)
	start := time.Date(2026, 6, 15, 23, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 16, 1, 0, 0, 0, time.UTC) // next day
	if _, err := uc.Execute(ctx, "u1", nil, start, stop, "", ""); !errors.Is(err, domain.ErrInvalidSession) {
		t.Fatalf("want ErrInvalidSession (cross-midnight), got %v", err)
	}
}

func TestAddSession_Overlap(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	now := time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)
	existingStop := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)
	if _, err := ss.Create(ctx, domain.WorkSession{
		ID: "x", OwnerID: "u1",
		Start: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC), Stop: &existingStop,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	uc := newAddSession(ss, now)
	start := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC) // overlaps 09:00–11:00
	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	if _, err := uc.Execute(ctx, "u1", nil, start, stop, "", ""); !errors.Is(err, domain.ErrOverlap) {
		t.Fatalf("want ErrOverlap, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/usecase/ -run AddSession -v`
Expected: FAIL (`undefined: usecase.AddSession`)

- [ ] **Step 3: Implement `internal/usecase/add_session.go`**

```go
package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// AddSession creates a complete (already-stopped) session for a past interval —
// "Nachbuchen". Unlike StartSession it takes explicit start/stop times. It
// enforces stop>start, no-future, same-day, and the no-overlap invariant.
type AddSession struct {
	Sessions ports.SessionStore
	IDs      ports.IDGen
	Clock    ports.Clock
}

func (uc AddSession) Execute(ctx context.Context, ownerID string, projectID *string, start, stop time.Time, tag, note string) (domain.WorkSession, error) {
	if !stop.After(start) {
		return domain.WorkSession{}, domain.ErrStopBeforeStart
	}
	now := uc.Clock.Now()
	if start.After(now) || stop.After(now) {
		return domain.WorkSession{}, domain.ErrFutureSession
	}
	if !sameLocalDay(start, stop) {
		return domain.WorkSession{}, fmt.Errorf("%w: start and stop must be on the same day", domain.ErrInvalidSession)
	}
	// Overlap check: pull the sessions around the candidate's day (±1 day to also
	// catch a cross-midnight neighbour) and apply the single-source rule.
	dayStart := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	existing, err := uc.Sessions.ListRange(ctx, ownerID, dayStart.Add(-24*time.Hour), dayStart.Add(48*time.Hour))
	if err != nil {
		return domain.WorkSession{}, err
	}
	if domain.HasOverlap(existing, start, &stop, "") {
		return domain.WorkSession{}, domain.ErrOverlap
	}
	s, err := domain.NewWorkSession(uc.IDs.NewID(), ownerID, projectID, start)
	if err != nil {
		return domain.WorkSession{}, err
	}
	s.Stop = &stop
	s.Tag, s.Note = tag, note
	return uc.Sessions.Create(ctx, s)
}

// sameLocalDay reports whether a and b fall on the same calendar day in their
// own locations.
func sameLocalDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/usecase/ -run AddSession -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/add_session.go internal/usecase/add_session_test.go
git commit -m "feat(usecase): AddSession — backfill a past session (stop>start, no-future, same-day, no-overlap)"
```

---

## Task 4: EditSession overlap guard

**Files:**
- Modify: `internal/usecase/edit_session.go`
- Test: `internal/usecase/edit_session_test.go` (append)

**Interfaces:**
- Consumes: `ports.SessionStore.ListRange` (Task 2), `domain.HasOverlap` (Task 1).
- Produces: unchanged `EditSession.Execute` signature; now rejects an edit that would overlap another session (`domain.ErrOverlap`), excluding the edited session itself.

- [ ] **Step 1: Write the failing test**

Append to `internal/usecase/edit_session_test.go`:

```go
func TestEditSession_RejectsOverlap(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	// existing other session 09:00–11:00
	aStop := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "a", OwnerID: "u1",
		Start: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC), Stop: &aStop}); err != nil {
		t.Fatalf("seed a: %v", err)
	}
	// session under edit, currently 13:00–14:00
	bStop := time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "b", OwnerID: "u1",
		Start: time.Date(2026, 6, 15, 13, 0, 0, 0, time.UTC), Stop: &bStop}); err != nil {
		t.Fatalf("seed b: %v", err)
	}
	uc := usecase.EditSession{Sessions: ss}
	// move b onto a → overlap
	newStart := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	newStop := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	if _, err := uc.Execute(ctx, "u1", "b", usecase.EditSessionInput{Start: newStart, Stop: &newStop}); !errors.Is(err, domain.ErrOverlap) {
		t.Fatalf("want ErrOverlap, got %v", err)
	}
}

func TestEditSession_NoSelfOverlap(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	bStop := time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "b", OwnerID: "u1",
		Start: time.Date(2026, 6, 15, 13, 0, 0, 0, time.UTC), Stop: &bStop}); err != nil {
		t.Fatalf("seed b: %v", err)
	}
	uc := usecase.EditSession{Sessions: ss}
	// edit b's note but keep overlapping times — must NOT report self-overlap
	if _, err := uc.Execute(ctx, "u1", "b", usecase.EditSessionInput{
		Note: "updated",
		Start: time.Date(2026, 6, 15, 13, 0, 0, 0, time.UTC), Stop: &bStop,
	}); err != nil {
		t.Fatalf("self-edit should succeed, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/usecase/ -run 'EditSession_RejectsOverlap|EditSession_NoSelfOverlap' -v`
Expected: FAIL (the overlap one returns nil instead of `ErrOverlap`).

- [ ] **Step 3: Implement the guard in `internal/usecase/edit_session.go`**

Replace the body of `Execute` so it checks overlap before delegating to `Update`:

```go
func (uc EditSession) Execute(ctx context.Context, ownerID, id string, in EditSessionInput) (domain.WorkSession, error) {
	if in.Stop != nil && !in.Stop.After(in.Start) {
		return domain.WorkSession{}, domain.ErrStopBeforeStart
	}
	dayStart := time.Date(in.Start.Year(), in.Start.Month(), in.Start.Day(), 0, 0, 0, 0, in.Start.Location())
	existing, err := uc.Sessions.ListRange(ctx, ownerID, dayStart.Add(-24*time.Hour), dayStart.Add(48*time.Hour))
	if err != nil {
		return domain.WorkSession{}, err
	}
	if domain.HasOverlap(existing, in.Start, in.Stop, id) {
		return domain.WorkSession{}, domain.ErrOverlap
	}
	return uc.Sessions.Update(ctx, ownerID, id, in.ProjectID, in.Tag, in.Note, in.Start, in.Stop)
}
```

(`time` and `domain` are already imported in this file.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/usecase/ -run EditSession -v`
Expected: PASS (existing `TestEditSession` stays green — its single session can't overlap itself).

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/edit_session.go internal/usecase/edit_session_test.go
git commit -m "feat(usecase): EditSession rejects overlap (excludes self) via HasOverlap"
```

---

## Task 5: ListSessionsRange usecase + REST surface + wiring

**Files:**
- Create: `internal/usecase/list_sessions_range.go`
- Modify: `internal/adapter/httpserver/server.go` (add two usecase fields)
- Modify: `internal/adapter/httpserver/worktime.go` (`startReq`, `handleStartSession`, `handleListSessions`)
- Modify: `cmd/flow-server/main.go` (wire `AddSession`, `ListSessionsRange`)
- Test: `internal/adapter/httpserver/worktime_test.go` (create)

**Interfaces:**
- Consumes: `usecase.AddSession` (Task 3), `domain` sentinels (Tasks 1/3), `ports.SessionStore.ListRange` (Task 2).
- Produces: `usecase.ListSessionsRange{ Sessions ports.SessionStore }` with `Execute(ctx, ownerID string, since, until time.Time) ([]domain.WorkSession, error)`; `POST /api/v1/sessions` accepts optional `start`+`stop`; `GET /api/v1/sessions` accepts optional `until`.

- [ ] **Step 1: Write the failing test**

Create `internal/adapter/httpserver/worktime_test.go`. It builds a server with the session usecases wired against fakes, then drives the new paths. Mirror the auth pattern from `server_test.go` (Bearer + FakeVerifier + EnsureUser allow-all):

```go
package httpserver_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func newWorktimeServer(t *testing.T) (*httpserver.Server, *testutil.FakeSessionStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)}
	sessions := testutil.NewFakeSessionStore()
	ids := &testutil.FakeIDGen{}
	return &httpserver.Server{
		Verifier:          testutil.FakeVerifier{ID: ports.Identity{Subject: "msoent", Username: "msoent"}},
		Ensure:            usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:               sse.NewBus(),
		Clock:             clk,
		Dev:               true,
		StartSession:      usecase.StartSession{Sessions: sessions, IDs: ids, Clock: clk},
		ListSessions:      usecase.ListSessions{Sessions: sessions, Clock: clk},
		AddSession:        usecase.AddSession{Sessions: sessions, IDs: ids, Clock: clk},
		ListSessionsRange: usecase.ListSessionsRange{Sessions: sessions},
	}, sessions
}

func authPost(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer x")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return res
}

func TestBackfillSession_HappyAndList(t *testing.T) {
	srv, _ := newWorktimeServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// Nachbuchen 09:00–12:00 on 2026-06-15 (clock is 18:00 same day).
	res := authPost(t, ts.URL+"/api/v1/sessions", map[string]any{
		"start": "2026-06-15T09:00:00Z", "stop": "2026-06-15T12:00:00Z", "tag": "deep",
	})
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("backfill status = %d (%s)", res.StatusCode, b)
	}
	var created domain.WorkSession
	_ = json.NewDecoder(res.Body).Decode(&created)
	_ = res.Body.Close()
	if created.Stop == nil || created.Tag != "deep" {
		t.Fatalf("backfill result wrong: %+v", created)
	}

	// GET with since+until brackets the day → returns the session.
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/sessions?since=2026-06-15T00:00:00Z&until=2026-06-16T00:00:00Z", nil)
	req.Header.Set("Authorization", "Bearer x")
	res2, _ := http.DefaultClient.Do(req)
	var list []domain.WorkSession
	_ = json.NewDecoder(res2.Body).Decode(&list)
	_ = res2.Body.Close()
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("range list = %+v, want the backfilled session", list)
	}
}

func TestBackfillSession_FutureRejected(t *testing.T) {
	srv, _ := newWorktimeServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	res := authPost(t, ts.URL+"/api/v1/sessions", map[string]any{
		"start": "2026-06-15T19:00:00Z", "stop": "2026-06-15T20:00:00Z", // after 18:00 clock
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("future backfill status = %d, want 400", res.StatusCode)
	}
}

func TestBackfillSession_OverlapConflict(t *testing.T) {
	srv, _ := newWorktimeServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	first := map[string]any{"start": "2026-06-15T09:00:00Z", "stop": "2026-06-15T11:00:00Z"}
	if r := authPost(t, ts.URL+"/api/v1/sessions", first); r.StatusCode != http.StatusCreated {
		t.Fatalf("seed backfill status = %d", r.StatusCode)
	}
	overlap := map[string]any{"start": "2026-06-15T10:00:00Z", "stop": "2026-06-15T12:00:00Z"}
	res := authPost(t, ts.URL+"/api/v1/sessions", overlap)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("overlap backfill status = %d, want 409", res.StatusCode)
	}
}

func TestBackfillSession_MixedTimestamps400(t *testing.T) {
	srv, _ := newWorktimeServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	res := authPost(t, ts.URL+"/api/v1/sessions", map[string]any{"start": "2026-06-15T09:00:00Z"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("mixed timestamps status = %d, want 400", res.StatusCode)
	}
}

func TestLiveStart_StillWorks(t *testing.T) {
	srv, _ := newWorktimeServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	res := authPost(t, ts.URL+"/api/v1/sessions", map[string]any{"tag": "live"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("live start status = %d, want 201", res.StatusCode)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapter/httpserver/ -run 'Backfill|LiveStart_StillWorks' -v`
Expected: FAIL to compile (`Server` has no `AddSession`/`ListSessionsRange` field; `usecase.ListSessionsRange` undefined).

- [ ] **Step 3a: Create `internal/usecase/list_sessions_range.go`**

```go
package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListSessionsRange returns the owner's sessions with since <= Start < until,
// newest first. Used by past-day views.
type ListSessionsRange struct {
	Sessions ports.SessionStore
}

func (uc ListSessionsRange) Execute(ctx context.Context, ownerID string, since, until time.Time) ([]domain.WorkSession, error) {
	return uc.Sessions.ListRange(ctx, ownerID, since, until)
}
```

- [ ] **Step 3b: Add fields to the `Server` struct in `internal/adapter/httpserver/server.go`**

After the `DeleteSession usecase.DeleteSession` field (~line 26):

```go
	AddSession        usecase.AddSession
	ListSessionsRange usecase.ListSessionsRange
```

- [ ] **Step 3c: Extend `internal/adapter/httpserver/worktime.go`**

Extend `startReq` with optional timestamps:

```go
type startReq struct {
	ProjectID *string    `json:"projectId"`
	Tag       string     `json:"tag"`
	Note      string     `json:"note"`
	Start     *time.Time `json:"start"`
	Stop      *time.Time `json:"stop"`
}
```

Replace `handleStartSession` so a body with both timestamps routes to `AddSession`:

```go
func (s *Server) handleStartSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req startReq
	_ = json.NewDecoder(r.Body).Decode(&req)

	// Nachbuchen: both timestamps present → create a complete past session.
	if req.Start != nil || req.Stop != nil {
		if req.Start == nil || req.Stop == nil {
			http.Error(w, "start and stop are required together", http.StatusBadRequest)
			return
		}
		sess, err := s.AddSession.Execute(r.Context(), u.ID, req.ProjectID, *req.Start, *req.Stop, req.Tag, req.Note)
		switch {
		case errors.Is(err, domain.ErrStopBeforeStart),
			errors.Is(err, domain.ErrFutureSession),
			errors.Is(err, domain.ErrInvalidSession):
			http.Error(w, "invalid session times", http.StatusBadRequest)
			return
		case errors.Is(err, domain.ErrOverlap):
			http.Error(w, "session overlaps an existing session", http.StatusConflict)
			return
		case err != nil:
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		s.Bus.Publish(domain.Event{Type: domain.EventSessionStarted, UserID: u.ID, Data: map[string]any{"id": sess.ID}})
		writeJSON(w, http.StatusCreated, sess)
		return
	}

	// Live start (unchanged).
	sess, err := s.StartSession.Execute(r.Context(), u.ID, req.ProjectID, req.Tag, req.Note)
	if errors.Is(err, domain.ErrAlreadyRunning) {
		http.Error(w, "a session is already running", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionStarted, UserID: u.ID, Data: map[string]any{"id": sess.ID}})
	writeJSON(w, http.StatusCreated, sess)
}
```

Extend `handleListSessions` to honour an optional `until`:

```go
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	since := startOfDay(s.Clock.Now())
	if q := r.URL.Query().Get("since"); q != "" {
		if t, err := time.Parse(time.RFC3339, q); err == nil {
			since = t
		}
	}
	var (
		list []domain.WorkSession
		err  error
	)
	if q := r.URL.Query().Get("until"); q != "" {
		until, perr := time.Parse(time.RFC3339, q)
		if perr != nil {
			http.Error(w, "bad until", http.StatusBadRequest)
			return
		}
		list, err = s.ListSessionsRange.Execute(r.Context(), u.ID, since, until)
	} else {
		list, err = s.ListSessions.Execute(r.Context(), u.ID, since)
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []domain.WorkSession{}
	}
	writeJSON(w, http.StatusOK, list)
}
```

- [ ] **Step 3d: Wire the usecases in `cmd/flow-server/main.go`**

After the existing `DeleteSession: usecase.DeleteSession{Sessions: sessionStore},` line (~line 104), add:

```go
		AddSession:        usecase.AddSession{Sessions: sessionStore, IDs: ids, Clock: clock},
		ListSessionsRange: usecase.ListSessionsRange{Sessions: sessionStore},
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapter/httpserver/ -run 'Backfill|LiveStart_StillWorks' -v` then `go build ./...`
Expected: PASS and clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/list_sessions_range.go internal/adapter/httpserver/server.go internal/adapter/httpserver/worktime.go internal/adapter/httpserver/worktime_test.go cmd/flow-server/main.go
git commit -m "feat(api): POST /sessions backfill (start+stop) + GET /sessions?until + wire AddSession/ListSessionsRange"
```

---

## Task 6: apiclient — AddSession + ListSessionsRange

**Files:**
- Modify: `internal/adapter/apiclient/client.go`
- Test: `internal/adapter/apiclient/worktime_test.go` (append)

**Interfaces:**
- Consumes: the REST surface from Task 5.
- Produces: `Client.AddSession(ctx, projectID *string, start, stop time.Time, tag, note string) (domain.WorkSession, error)`; `Client.ListSessionsRange(ctx, since, until time.Time) ([]domain.WorkSession, error)`.

- [ ] **Step 1: Write the failing test**

Append to `internal/adapter/apiclient/worktime_test.go`:

```go
func TestAddSessionAndListRange(t *testing.T) {
	var gotBody map[string]any
	var gotRangeQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sessions":
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"s9","start":"2026-06-15T09:00:00Z"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sessions":
			gotRangeQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`[{"id":"s9","start":"2026-06-15T09:00:00Z"}]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")

	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	s, err := c.AddSession(context.Background(), nil, start, stop, "deep", "n")
	if err != nil || s.ID != "s9" {
		t.Fatalf("AddSession = %+v err=%v", s, err)
	}
	if gotBody["start"] == nil || gotBody["stop"] == nil || gotBody["tag"] != "deep" {
		t.Fatalf("AddSession body missing fields: %+v", gotBody)
	}

	list, err := c.ListSessionsRange(context.Background(), start, stop)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListSessionsRange = %+v err=%v", list, err)
	}
	if !strings.Contains(gotRangeQuery, "since=") || !strings.Contains(gotRangeQuery, "until=") {
		t.Fatalf("range query missing since/until: %q", gotRangeQuery)
	}
}
```

Add `"encoding/json"` and `"strings"` to this test file's imports if not present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/apiclient/ -run AddSessionAndListRange -v`
Expected: FAIL (`c.AddSession undefined`)

- [ ] **Step 3: Implement in `internal/adapter/apiclient/client.go`**

Add after `ListSessionsSince` (~line 148):

```go
// AddSession backfills a complete past session with explicit start/stop.
func (c *Client) AddSession(ctx context.Context, projectID *string, start, stop time.Time, tag, note string) (domain.WorkSession, error) {
	var s domain.WorkSession
	err := c.do(ctx, http.MethodPost, "/api/v1/sessions",
		map[string]any{"projectId": projectID, "tag": tag, "note": note, "start": start, "stop": stop}, &s)
	return s, err
}

// ListSessionsRange returns sessions with since <= start < until.
func (c *Client) ListSessionsRange(ctx context.Context, since, until time.Time) ([]domain.WorkSession, error) {
	var out []domain.WorkSession
	path := "/api/v1/sessions?since=" + url.QueryEscape(since.Format(time.RFC3339)) +
		"&until=" + url.QueryEscape(until.Format(time.RFC3339))
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
```

(`url`, `time`, `domain`, `http` are already imported in client.go.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapter/apiclient/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/apiclient/client.go internal/adapter/apiclient/worktime_test.go
git commit -m "feat(apiclient): AddSession + ListSessionsRange"
```

---

## Task 7: Full CI + live done-gate

**Files:** none (verification only)

- [ ] **Step 1: Run the full CI**

Run: `make ci`
Expected: green — lint `0 issues`, templ drift OK, build, all tests, coverage gate (~80%, currently ~84%).

- [ ] **Step 2: Live smoke vs the dev stack**

Start the dev stack and mint a token (see `reference_flow_dev_env`):

```bash
make dev-up
make dev-run    # in another shell
TOKEN=$(make -s dev-token)
```

Backfill a past session, list it back, then prove overlap + future are rejected:

```bash
# Nachbuchen yesterday 09:00–12:00 (adjust the date to a recent past day)
curl -s -o /dev/null -w "create=%{http_code}\n" -X POST localhost:8080/api/v1/sessions \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"start":"2026-06-19T09:00:00+02:00","stop":"2026-06-19T12:00:00+02:00","tag":"smoke"}'

# Overlap → expect 409
curl -s -o /dev/null -w "overlap=%{http_code}\n" -X POST localhost:8080/api/v1/sessions \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"start":"2026-06-19T10:00:00+02:00","stop":"2026-06-19T13:00:00+02:00"}'

# Future → expect 400
curl -s -o /dev/null -w "future=%{http_code}\n" -X POST localhost:8080/api/v1/sessions \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"start":"2030-01-01T09:00:00+02:00","stop":"2030-01-01T12:00:00+02:00"}'

# Range list brackets that day → expect the smoke session in the JSON
curl -s "localhost:8080/api/v1/sessions?since=2026-06-19T00:00:00%2B02:00&until=2026-06-20T00:00:00%2B02:00" \
  -H "Authorization: Bearer $TOKEN"

# Live start still works (no start/stop) → expect 201
curl -s -o /dev/null -w "live=%{http_code}\n" -X POST localhost:8080/api/v1/sessions \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"tag":"live"}'
```

Expected: `create=201`, `overlap=409`, `future=400`, the range list contains the smoke session, `live=201`.

- [ ] **Step 3: Final confirmation**

No commit needed. Report `make ci` output + the curl status codes.

---

## Self-Review

**Spec coverage (Slice 1 scope):**
- Overlap invariant single-source (`HasOverlap`) → Task 1 ✓
- `ErrOverlap` / `ErrFutureSession` sentinels → Task 1 ✓
- Overlap enforced in AddSession + EditSession (arbitrary-interval paths) → Tasks 3, 4 ✓
- `AddSession` create-with-times, no store change for create → Task 3 ✓ (reuses `Create`)
- Validation: stop>start, no-future, same-day, overlap → Task 3 ✓
- `SessionStore.ListRange(since, until)` + pgstore + fake → Task 2 ✓
- `POST /sessions` extended (start+stop, mixed→400) + live unchanged → Task 5 ✓
- `GET /sessions?until=` + unchanged default → Task 5 ✓
- main wiring + curl-smoke each new behaviour → Tasks 5, 7 ✓ (per the "plans need a main-wiring task" rule)
- apiclient `AddSession` + `ListSessionsRange` → Task 6 ✓
- Out of scope (TUI/WebUI/CLI, duration-only, cross-midnight nachbuchen) → not in this slice ✓

**Placeholder scan:** none. The one lookup left to the implementer is the pgstore test helper name (Task 2 Step 3d) — the step gives the exact `rg` command to find it and says to reuse it verbatim, because the harness name can't be known without reading the package's test files.

**Type consistency:** `HasOverlap(existing []WorkSession, start time.Time, stop *time.Time, excludeID string) bool` used identically in Tasks 1/3/4. `ListRange(ctx, ownerID, since, until)` identical in the interface, pgstore, fake, `ListSessionsRange`, and the GET handler. `AddSession.Execute(ctx, ownerID, projectID *string, start, stop time.Time, tag, note string)` matches between Task 3, the handler (Task 5), main wiring (Task 5), and apiclient (Task 6). Server fields `AddSession`/`ListSessionsRange` match struct (Task 5b), test harness (Task 5 Step 1), and main (Task 5d).
