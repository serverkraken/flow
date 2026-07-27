# WebUI Slice 1 — Worktime (Heute · Woche · Historie) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Worktime triad (Heute, Woche, Historie) on the Slice-0 AppShell, with selection-based bulk reassignment of imported sessions, bulk delete, and paginated session listing.

**Architecture:** Hexagonal, server-rendered templ + htmx + Tailwind v4, SSE-live. New usecases (`BulkAssignProject`, `BulkDeleteSessions`, `ListSessionsPage`) + REST endpoints + store method `ListPage`; new reusable templ components in `internal/adapter/webui/components/`; three new pages in package `webui`; ephemeral bulk-selection state in a small embedded vanilla-JS file. WebUI handlers call usecases directly (existing pattern); REST + apiclient are the testable seam and CLI/TUI parity.

**Tech Stack:** Go 1.x, pgx/v5 (pgstore), templ (a-h/templ), htmx 2 + htmx-ext-sse (vendored, offline), Tailwind v4 (build in Docker), net/http ServeMux, in-process EventBus.

## Global Constraints

- **No CDN / offline:** every asset local under `/static/` (`go:embed`). Never reference `unpkg.com`, `googleapis.com`, `gstatic.com`, `fontshare.com`, `cdn.tailwindcss.com`. New pages MUST use `components.Base` (it wires offline assets + SSE + theme boot), NOT a hand-rolled `<head>`.
- **No browser popups:** never emit `window.alert/confirm/prompt`; every destructive action goes through `components.ConfirmDialog`. Validation is inline/in-design.
- **i18n:** no hardcoded display strings in templates — all via `T(ctx, "key")` / `Tn(ctx, "key", n)`; add every new key to BOTH `internal/i18n/catalog_de.go` (full) and `internal/i18n/catalog_en.go` (stub). German is primary.
- **Owner-scoping:** every store/usecase call is scoped to `u.ID`; foreign IDs must never leak or mutate.
- **CI:** Integration/wiring tasks run `make ci` (lint included — `gofumpt`/`staticcheck`; QF1002 has slipped before), not just `go test`. Coverage gate must stay green.
- **Glyph whitelist (semantic, monospace):** `▶ ■ ‖ ✓ ✗ ▲ ▼ ● ○ ★ ☼ ✚ ▎ ▰▱ › · ◆ ▤ ▾ ▴`. No colored emoji.
- **Module path:** `github.com/serverkraken/flow`.
- **Visual source of truth:** `docs/superpowers/specs/assets/2026-06-23-webui/direction-b-studio.html` (Heute), `studio-worktime-week.html` (Woche), `studio-worktime-calendar.html` (Historie). Class lists for dense markup are copied from these verbatim (they already use the Slice-0 token names: `canvas/surface/sunken/line/ink/body/muted/faint/blue/cyan/green/purple/magenta/yellow/orange/red/teal`).
- **Spec:** `docs/superpowers/specs/2026-06-24-flow-webui-slice1-worktime-design.md`.

---

## File Structure

**Create:**
- `internal/usecase/bulk_assign_project.go` + `_test.go`
- `internal/usecase/bulk_delete_sessions.go` + `_test.go`
- `internal/usecase/list_sessions_page.go` + `_test.go`
- `internal/adapter/webui/components/progressbar.templ`, `pacedots.templ`, `sessionrow.templ`, `sessionblock.templ`, `kennzahlen.templ`, `weektotal.templ`, `fuzzypicker.templ`, `selectionbar.templ`, `segtoggle.templ`
- `internal/adapter/webui/components/worktime_components_test.go`
- `internal/adapter/webui/heute.templ`, `woche.templ`, `historie.templ` (+ small viewmodel `.go` siblings if cleaner)
- `internal/adapter/webui/static/js/historie-select.js`
- `internal/adapter/httpserver/webui_heute.go`, `webui_woche.go`, `webui_historie.go` (+ `_test.go` each)

**Modify:**
- `internal/ports/ports.go` — add `ListPage` to `SessionStore`
- `internal/adapter/pgstore/sessions.go` — implement `ListPage`
- `internal/testutil/fakes.go` — implement `FakeSessionStore.ListPage`
- `internal/adapter/httpserver/worktime.go` — add `handleReassignSessions`, `handleBulkDeleteSessions`, extend `handleListSessions` with `?limit&offset` + `X-Total-Count`
- `internal/adapter/httpserver/server.go` — `Server` struct fields + route registration
- `internal/adapter/apiclient/client.go` — `ReassignSessions`, `BulkDeleteSessions`, `ListSessionsPage`
- `cmd/flow-server/main.go` — wire new usecases
- `internal/i18n/catalog_de.go`, `catalog_en.go` — new keys
- **Delete (Task 9, once `/` repoints):** `internal/adapter/webui/worktime.templ` + `worktime_templ.go`

---

## Task 1: SessionStore.ListPage (store + fake)

Paginated, all-time, newest-first session listing with total count.

**Files:**
- Modify: `internal/ports/ports.go` (SessionStore interface)
- Modify: `internal/adapter/pgstore/sessions.go`
- Modify: `internal/testutil/fakes.go`
- Test: `internal/testutil/fakes_session_mutation_test.go` (add a test), `internal/adapter/pgstore/worktime_test.go` (add a pgstore test)

**Interfaces:**
- Produces: `SessionStore.ListPage(ctx, ownerID string, limit, offset int) (items []domain.WorkSession, total int, err error)` — ordered `start_at DESC`; `total` = full owner count ignoring limit/offset.

- [ ] **Step 1: Write the failing fake test**

Add to `internal/testutil/fakes_session_mutation_test.go`:

```go
func TestFakeSessionStore_ListPage(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	mk := func(id string, h int) domain.WorkSession {
		start := time.Date(2026, 6, 15, h, 0, 0, 0, time.UTC)
		stop := start.Add(time.Hour)
		return domain.WorkSession{ID: id, OwnerID: "u1", Start: start, Stop: &stop}
	}
	for _, ws := range []domain.WorkSession{mk("a", 8), mk("b", 10), mk("c", 12)} {
		if _, err := ss.Create(ctx, ws); err != nil {
			t.Fatalf("seed %s: %v", ws.ID, err)
		}
	}
	// foreign owner must not count
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "x", OwnerID: "u2",
		Start: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("seed foreign: %v", err)
	}
	items, total, err := ss.ListPage(ctx, "u1", 2, 0)
	if err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(items) != 2 || items[0].ID != "c" || items[1].ID != "b" {
		t.Fatalf("page1 = %+v, want [c b] newest-first", items)
	}
	page2, _, _ := ss.ListPage(ctx, "u1", 2, 2)
	if len(page2) != 1 || page2[0].ID != "a" {
		t.Fatalf("page2 = %+v, want [a]", page2)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/testutil/ -run TestFakeSessionStore_ListPage`
Expected: FAIL — `ss.ListPage undefined`.

- [ ] **Step 3: Add the interface method**

In `internal/ports/ports.go`, inside `SessionStore`, after `ListRange`:

```go
	// ListPage returns the owner's sessions newest-first (start_at DESC),
	// limited to `limit` rows starting at `offset`, plus the total owner count
	// (ignoring limit/offset) for pagination math. Owner-scoped.
	ListPage(ctx context.Context, ownerID string, limit, offset int) (items []domain.WorkSession, total int, err error)
```

- [ ] **Step 4: Implement in the fake**

In `internal/testutil/fakes.go`, after `FakeSessionStore.ListRange`:

```go
func (s *FakeSessionStore) ListPage(_ context.Context, ownerID string, limit, offset int) ([]domain.WorkSession, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []domain.WorkSession
	for _, e := range s.m {
		if e.OwnerID == ownerID {
			all = append(all, e)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Start.After(all[j].Start) })
	total := len(all)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return all[offset:end], total, nil
}
```

(`sort` is already imported in fakes.go.)

- [ ] **Step 5: Implement in pgstore**

In `internal/adapter/pgstore/sessions.go`, after `ListRange`:

```go
func (s *SessionStore) ListPage(ctx context.Context, ownerID string, limit, offset int) ([]domain.WorkSession, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM work_sessions WHERE owner_id=$1`, ownerID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("pgstore: count sessions: %w", err)
	}
	const q = `
SELECT id, owner_id, project_id, tag, note, start_at, stop_at, created_at
FROM work_sessions WHERE owner_id=$1
ORDER BY start_at DESC
LIMIT $2 OFFSET $3`
	rows, err := s.pool.Query(ctx, q, ownerID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("pgstore: list sessions page: %w", err)
	}
	defer rows.Close()
	var out []domain.WorkSession
	for rows.Next() {
		ws, err := scanSession(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, ws)
	}
	return out, total, rows.Err()
}
```

- [ ] **Step 6: Add the pgstore test**

In `internal/adapter/pgstore/worktime_test.go`, add (follow the existing `TestSessionStore_ListRange` setup for `newSessionStore(t)`/owner seeding — reuse its helpers verbatim):

```go
func TestSessionStore_ListPage(t *testing.T) {
	store, owner, cleanup := newSessionStore(t) // same helper TestSessionStore_ListRange uses
	defer cleanup()
	ctx := context.Background()
	base := time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		st := base.Add(time.Duration(i) * time.Hour)
		sp := st.Add(30 * time.Minute)
		if _, err := store.Create(ctx, domain.WorkSession{
			ID: "p" + itoa(i), OwnerID: owner, Start: st, Stop: &sp, CreatedAt: st,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	items, total, err := store.ListPage(ctx, owner, 2, 0)
	if err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	if total != 3 || len(items) != 2 {
		t.Fatalf("got total=%d len=%d, want 3 and 2", total, len(items))
	}
	if !items[0].Start.After(items[1].Start) {
		t.Fatalf("not newest-first: %+v", items)
	}
}
```

> If `newSessionStore`/`itoa` helpers differ in that file, mirror exactly what `TestSessionStore_ListRange` does for setup and use `fmt.Sprintf("p%d", i)` for ids.

- [ ] **Step 7: Run tests**

Run: `go test ./internal/testutil/ ./internal/adapter/pgstore/ -run 'ListPage'`
Expected: PASS (pgstore test needs the Docker Postgres harness, like the other `TestSessionStore_*`; set `DOCKER_HOST=unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')` if running pgstore tests locally).

- [ ] **Step 8: Commit**

```bash
git add internal/ports/ports.go internal/adapter/pgstore/sessions.go internal/testutil/fakes.go internal/testutil/fakes_session_mutation_test.go internal/adapter/pgstore/worktime_test.go
git commit -m "feat(sessions): SessionStore.ListPage (paginated, newest-first, total count)"
```

---

## Task 2: ListSessionsPage usecase + REST pagination (?limit&offset, X-Total-Count)

**Files:**
- Create: `internal/usecase/list_sessions_page.go`, `internal/usecase/list_sessions_page_test.go`
- Modify: `internal/adapter/httpserver/worktime.go` (extend `handleListSessions`)
- Modify: `internal/adapter/httpserver/server.go` (add `ListSessionsPage` field; route stays the same path)
- Test: `internal/adapter/httpserver/worktime_test.go` (add a handler test)

**Interfaces:**
- Consumes: `ports.SessionStore.ListPage` (Task 1).
- Produces: `usecase.ListSessionsPage{Sessions ports.SessionStore}` with `Execute(ctx, ownerID string, limit, offset int) ([]domain.WorkSession, int, error)`. REST: `GET /api/v1/sessions?limit=N&offset=M` → body `[]WorkSession` (newest-first), header `X-Total-Count: <total>`.

- [ ] **Step 1: Write the failing usecase test**

`internal/usecase/list_sessions_page_test.go`:

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

func TestListSessionsPage(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	for i := 0; i < 5; i++ {
		st := time.Date(2026, 6, 15, 8+i, 0, 0, 0, time.UTC)
		sp := st.Add(time.Hour)
		if _, err := ss.Create(ctx, domain.WorkSession{
			ID: "s" + string(rune('0'+i)), OwnerID: "u1", Start: st, Stop: &sp}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	uc := usecase.ListSessionsPage{Sessions: ss}
	items, total, err := uc.Execute(ctx, "u1", 2, 0)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if total != 5 || len(items) != 2 {
		t.Fatalf("got total=%d len=%d, want 5 and 2", total, len(items))
	}
}
```

- [ ] **Step 2: Run it — expect FAIL**

Run: `go test ./internal/usecase/ -run TestListSessionsPage`
Expected: FAIL — `usecase.ListSessionsPage undefined`.

- [ ] **Step 3: Implement the usecase**

`internal/usecase/list_sessions_page.go`:

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListSessionsPage returns one page of the owner's sessions (newest-first) plus
// the total count, for the WebUI "Alle Sitzungen" list. Owner-scoped via store.
type ListSessionsPage struct {
	Sessions ports.SessionStore
}

func (uc ListSessionsPage) Execute(ctx context.Context, ownerID string, limit, offset int) ([]domain.WorkSession, int, error) {
	return uc.Sessions.ListPage(ctx, ownerID, limit, offset)
}
```

- [ ] **Step 4: Run it — expect PASS**

Run: `go test ./internal/usecase/ -run TestListSessionsPage`
Expected: PASS.

- [ ] **Step 5: Add the Server field**

In `internal/adapter/httpserver/server.go`, in the `Server` struct near the other session usecases:

```go
	ListSessionsPage  usecase.ListSessionsPage
```

- [ ] **Step 6: Extend handleListSessions for pagination**

In `internal/adapter/httpserver/worktime.go`, replace `handleListSessions` body. Pagination applies only when `limit` is present; `since`/`until` behavior is unchanged otherwise:

```go
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())

	// Paginated all-time mode: ?limit (and optional ?offset). Newest-first.
	if q := r.URL.Query().Get("limit"); q != "" {
		limit, err := strconv.Atoi(q)
		if err != nil || limit < 1 || limit > 200 {
			http.Error(w, "bad limit (1..200)", http.StatusBadRequest)
			return
		}
		offset := 0
		if o := r.URL.Query().Get("offset"); o != "" {
			if offset, err = strconv.Atoi(o); err != nil || offset < 0 {
				http.Error(w, "bad offset", http.StatusBadRequest)
				return
			}
		}
		list, total, err := s.ListSessionsPage.Execute(r.Context(), u.ID, limit, offset)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if list == nil {
			list = []domain.WorkSession{}
		}
		w.Header().Set("X-Total-Count", strconv.Itoa(total))
		writeJSON(w, http.StatusOK, list)
		return
	}

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

Add `"strconv"` to the `worktime.go` import block.

- [ ] **Step 7: Add the handler test**

In `internal/adapter/httpserver/worktime_test.go` add (mirror the harness that file's existing tests use — `newTestServer`/seed helpers; the snippet below assumes a `do(t, srv, method, path, body)` style already present, otherwise adapt to the file's existing request helper):

```go
func TestHandleListSessions_Pagination(t *testing.T) {
	// Seed 3 sessions for the authed user via the same fake store the file's
	// other tests construct, then GET with limit/offset.
	srv, ss, token := newSessionTestServer(t) // adapt to this file's existing constructor
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		st := time.Date(2026, 6, 15, 8+i, 0, 0, 0, time.UTC)
		sp := st.Add(time.Hour)
		_, _ = ss.Create(ctx, domain.WorkSession{ID: "s" + string(rune('0'+i)), OwnerID: testUserID, Start: st, Stop: &sp})
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions?limit=2&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("X-Total-Count"); got != "3" {
		t.Fatalf("X-Total-Count = %q, want 3", got)
	}
	var out []domain.WorkSession
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
}
```

> Use the exact server/token/seed helpers already in `worktime_test.go` (look at the top of that file). Wire `ListSessionsPage: usecase.ListSessionsPage{Sessions: ss}` into the test server alongside the other usecases.

- [ ] **Step 8: Run tests**

Run: `go test ./internal/usecase/ ./internal/adapter/httpserver/ -run 'ListSessionsPage|Pagination'`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/usecase/list_sessions_page.go internal/usecase/list_sessions_page_test.go internal/adapter/httpserver/worktime.go internal/adapter/httpserver/server.go internal/adapter/httpserver/worktime_test.go
git commit -m "feat(api): GET /sessions?limit&offset with X-Total-Count header"
```

---

## Task 3: BulkAssignProject usecase + POST /sessions/reassign

Assign one project to many sessions; foreign/stale ids skipped; times untouched.

**Files:**
- Create: `internal/usecase/bulk_assign_project.go`, `_test.go`
- Modify: `internal/adapter/httpserver/worktime.go` (handler `handleReassignSessions`)
- Modify: `internal/adapter/httpserver/server.go` (field + route)
- Test: `internal/adapter/httpserver/worktime_test.go`

**Interfaces:**
- Consumes: `ports.SessionStore.Get`, `.Update`; `ports.ProjectStore.Get`.
- Produces: `usecase.BulkAssignProject{Sessions ports.SessionStore; Projects ports.ProjectStore}` with `Execute(ctx, ownerID string, sessionIDs []string, projectID string) (updated int, err error)`. Errors: `ErrNoSessions` (empty ids), `ports.ErrProjectNotFound` (project missing/foreign). REST: `POST /api/v1/sessions/reassign {ids:[],projectId:""}` → `{updated:N}`; 400 empty ids / 404 bad project; publishes one `session.updated`.

- [ ] **Step 1: Write the failing usecase test**

`internal/usecase/bulk_assign_project_test.go`:

```go
package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func seedSess(t *testing.T, ss *testutil.FakeSessionStore, id, owner string) {
	t.Helper()
	st := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	sp := st.Add(time.Hour)
	if _, err := ss.Create(context.Background(), domain.WorkSession{ID: id, OwnerID: owner, Start: st, Stop: &sp}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func TestBulkAssignProject_AssignsOwnedSkipsForeign(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeProjectStore()
	seedSess(t, ss, "a", "u1")
	seedSess(t, ss, "b", "u1")
	seedSess(t, ss, "c", "u2") // foreign
	if _, err := ps.Create(ctx, domain.Project{ID: "p1", OwnerID: "u1", Name: "flow"}); err != nil {
		t.Fatalf("seed proj: %v", err)
	}
	uc := usecase.BulkAssignProject{Sessions: ss, Projects: ps}
	n, err := uc.Execute(ctx, "u1", []string{"a", "b", "c", "missing"}, "p1")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if n != 2 {
		t.Fatalf("updated = %d, want 2 (a,b; c foreign + missing skipped)", n)
	}
	got, _ := ss.Get(ctx, "u1", "a")
	if got.ProjectID == nil || *got.ProjectID != "p1" {
		t.Fatalf("a not assigned: %+v", got)
	}
	// foreign session untouched
	if c, _ := ss.Get(ctx, "u2", "c"); c.ProjectID != nil {
		t.Fatalf("foreign c was mutated: %+v", c)
	}
}

func TestBulkAssignProject_EmptyIDs(t *testing.T) {
	uc := usecase.BulkAssignProject{Sessions: testutil.NewFakeSessionStore(), Projects: testutil.NewFakeProjectStore()}
	if _, err := uc.Execute(context.Background(), "u1", nil, "p1"); !errors.Is(err, usecase.ErrNoSessions) {
		t.Fatalf("err = %v, want ErrNoSessions", err)
	}
}

func TestBulkAssignProject_ForeignProject(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeProjectStore()
	seedSess(t, ss, "a", "u1")
	if _, err := ps.Create(ctx, domain.Project{ID: "p2", OwnerID: "other", Name: "x"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	uc := usecase.BulkAssignProject{Sessions: ss, Projects: ps}
	if _, err := uc.Execute(ctx, "u1", []string{"a"}, "p2"); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Fatalf("err = %v, want ErrProjectNotFound", err)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./internal/usecase/ -run TestBulkAssignProject`
Expected: FAIL — `usecase.BulkAssignProject` / `usecase.ErrNoSessions` undefined.

- [ ] **Step 3: Implement the usecase**

`internal/usecase/bulk_assign_project.go`:

```go
package usecase

import (
	"context"
	"errors"

	"github.com/serverkraken/flow/internal/ports"
)

// ErrNoSessions is returned by bulk operations when the id list is empty.
var ErrNoSessions = errors.New("no sessions selected")

// BulkAssignProject assigns one project to many sessions (import cleanup).
// Owner-scoped: the project must belong to the owner; sessions that are missing
// or foreign are silently skipped (robust against a stale selection). Start/stop
// are untouched, so no overlap check is needed. Returns the count actually changed.
type BulkAssignProject struct {
	Sessions ports.SessionStore
	Projects ports.ProjectStore
}

func (uc BulkAssignProject) Execute(ctx context.Context, ownerID string, sessionIDs []string, projectID string) (int, error) {
	if len(sessionIDs) == 0 {
		return 0, ErrNoSessions
	}
	// Validate the target project up front (owner-scoped).
	if _, err := uc.Projects.Get(ctx, ownerID, projectID); err != nil {
		return 0, err // ports.ErrProjectNotFound for missing/foreign
	}
	pid := projectID
	updated := 0
	for _, id := range sessionIDs {
		cur, err := uc.Sessions.Get(ctx, ownerID, id)
		if errors.Is(err, ports.ErrSessionNotFound) {
			continue // stale/foreign — skip
		}
		if err != nil {
			return updated, err
		}
		if _, err := uc.Sessions.Update(ctx, ownerID, id, &pid, cur.Tag, cur.Note, cur.Start, cur.Stop); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}
```

- [ ] **Step 4: Run — expect PASS**

Run: `go test ./internal/usecase/ -run TestBulkAssignProject`
Expected: PASS.

- [ ] **Step 5: Add Server field + handler**

In `server.go` `Server` struct: `BulkAssignProject usecase.BulkAssignProject`.

In `worktime.go`, add (the inline-create path mirrors `resolveWebProject`; REST passes only `projectId`):

```go
type reassignReq struct {
	IDs       []string `json:"ids"`
	ProjectID string   `json:"projectId"`
}

func (s *Server) handleReassignSessions(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req reassignReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	n, err := s.BulkAssignProject.Execute(r.Context(), u.ID, req.IDs, req.ProjectID)
	switch {
	case errors.Is(err, usecase.ErrNoSessions):
		http.Error(w, "no sessions selected", http.StatusBadRequest)
		return
	case errors.Is(err, ports.ErrProjectNotFound):
		http.Error(w, "project not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionUpdated, UserID: u.ID})
	writeJSON(w, http.StatusOK, map[string]int{"updated": n})
}
```

- [ ] **Step 6: Register the route**

In `server.go` `Routes()`, with the other session routes (use `authAny` — Bearer or Cookie — so the WebUI can also reach it; static path, no `{id}` clash):

```go
	mux.Handle("POST /api/v1/sessions/reassign", s.authAny(http.HandlerFunc(s.handleReassignSessions)))
```

> Place it BEFORE `POST /api/v1/sessions/{id}/stop` is irrelevant (different path), but keep all `/sessions/*` routes grouped.

- [ ] **Step 7: Handler test**

In `worktime_test.go`:

```go
func TestHandleReassignSessions(t *testing.T) {
	srv, ss, token := newSessionTestServer(t) // adapt to file's constructor; ensure
	// the server also has ProjectStore-backed CreateProject/GetProject + BulkAssignProject wired.
	ctx := context.Background()
	seedSess(t, ss, "a", testUserID) // or inline create as other tests do
	// seed project "p1" for testUserID via the test's project store/usecase
	body := `{"ids":["a"],"projectId":"p1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/reassign", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	got, _ := ss.Get(ctx, testUserID, "a")
	if got.ProjectID == nil || *got.ProjectID != "p1" {
		t.Fatalf("not reassigned: %+v", got)
	}
}
```

> Wire `BulkAssignProject: usecase.BulkAssignProject{Sessions: ss, Projects: ps}` and seed project `p1` for `testUserID` using the project store the test constructs. Match the exact helper names already in `worktime_test.go`.

- [ ] **Step 8: Run + commit**

Run: `go test ./internal/usecase/ ./internal/adapter/httpserver/ -run 'BulkAssignProject|Reassign'`
Expected: PASS.

```bash
git add internal/usecase/bulk_assign_project.go internal/usecase/bulk_assign_project_test.go internal/adapter/httpserver/worktime.go internal/adapter/httpserver/server.go internal/adapter/httpserver/worktime_test.go
git commit -m "feat(sessions): BulkAssignProject usecase + POST /sessions/reassign"
```

---

## Task 4: BulkDeleteSessions usecase + POST /sessions/bulk-delete + apiclient methods

**Files:**
- Create: `internal/usecase/bulk_delete_sessions.go`, `_test.go`
- Modify: `internal/adapter/httpserver/worktime.go` (handler), `server.go` (field + route)
- Modify: `internal/adapter/apiclient/client.go` (3 methods)
- Test: `worktime_test.go`, `internal/adapter/apiclient/worktime_test.go`

**Interfaces:**
- Produces: `usecase.BulkDeleteSessions{Sessions ports.SessionStore}` `Execute(ctx, ownerID string, ids []string) (deleted int, err error)` (empty → `ErrNoSessions`; foreign/missing skipped). REST `POST /api/v1/sessions/bulk-delete {ids:[]}` → `{deleted:N}`, publishes one `session.deleted`. apiclient: `ReassignSessions(ctx, projectID string, ids []string) (int, error)`, `BulkDeleteSessions(ctx, ids []string) (int, error)`, `ListSessionsPage(ctx, limit, offset int) ([]domain.WorkSession, int, error)`.

- [ ] **Step 1: Failing usecase test** — `internal/usecase/bulk_delete_sessions_test.go`:

```go
package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestBulkDeleteSessions(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	seedSess(t, ss, "a", "u1")
	seedSess(t, ss, "b", "u1")
	seedSess(t, ss, "c", "u2")
	uc := usecase.BulkDeleteSessions{Sessions: ss}
	n, err := uc.Execute(ctx, "u1", []string{"a", "b", "c", "missing"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if n != 2 {
		t.Fatalf("deleted = %d, want 2", n)
	}
	if _, err := ss.Get(ctx, "u1", "a"); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("a not deleted")
	}
	if _, err := ss.Get(ctx, "u2", "c"); err != nil {
		t.Fatalf("foreign c was deleted")
	}
	if _, err := uc.Execute(ctx, "u1", nil); !errors.Is(err, usecase.ErrNoSessions) {
		t.Fatalf("empty ids should be ErrNoSessions")
	}
}
```

- [ ] **Step 2: Run — FAIL.** `go test ./internal/usecase/ -run TestBulkDeleteSessions`

- [ ] **Step 3: Implement** — `internal/usecase/bulk_delete_sessions.go`:

```go
package usecase

import (
	"context"
	"errors"

	"github.com/serverkraken/flow/internal/ports"
)

// BulkDeleteSessions deletes many sessions at once (import cleanup). Owner-scoped;
// missing/foreign ids are skipped. Returns the count actually deleted.
type BulkDeleteSessions struct {
	Sessions ports.SessionStore
}

func (uc BulkDeleteSessions) Execute(ctx context.Context, ownerID string, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, ErrNoSessions
	}
	deleted := 0
	for _, id := range ids {
		err := uc.Sessions.Delete(ctx, ownerID, id)
		if errors.Is(err, ports.ErrSessionNotFound) {
			continue
		}
		if err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}
```

- [ ] **Step 4: Run — PASS.**

- [ ] **Step 5: Handler + route + field.** `server.go`: `BulkDeleteSessions usecase.BulkDeleteSessions`. `worktime.go`:

```go
type bulkDeleteReq struct {
	IDs []string `json:"ids"`
}

func (s *Server) handleBulkDeleteSessions(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req bulkDeleteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	n, err := s.BulkDeleteSessions.Execute(r.Context(), u.ID, req.IDs)
	switch {
	case errors.Is(err, usecase.ErrNoSessions):
		http.Error(w, "no sessions selected", http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionDeleted, UserID: u.ID})
	writeJSON(w, http.StatusOK, map[string]int{"deleted": n})
}
```

`server.go` route: `mux.Handle("POST /api/v1/sessions/bulk-delete", s.authAny(http.HandlerFunc(s.handleBulkDeleteSessions)))`.

- [ ] **Step 6: apiclient methods.** In `internal/adapter/apiclient/client.go`, after `ListSessionsRange`:

```go
// ReassignSessions assigns one project to many sessions; returns the count changed.
func (c *Client) ReassignSessions(ctx context.Context, projectID string, ids []string) (int, error) {
	var out struct {
		Updated int `json:"updated"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/sessions/reassign",
		map[string]any{"ids": ids, "projectId": projectID}, &out)
	return out.Updated, err
}

// BulkDeleteSessions deletes many sessions; returns the count deleted.
func (c *Client) BulkDeleteSessions(ctx context.Context, ids []string) (int, error) {
	var out struct {
		Deleted int `json:"deleted"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/sessions/bulk-delete",
		map[string]any{"ids": ids}, &out)
	return out.Deleted, err
}

// ListSessionsPage returns one page (newest-first) plus the total from X-Total-Count.
func (c *Client) ListSessionsPage(ctx context.Context, limit, offset int) ([]domain.WorkSession, int, error) {
	path := fmt.Sprintf("/api/v1/sessions?limit=%d&offset=%d", limit, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, 0, err
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 300 {
		return nil, 0, &APIError{Method: http.MethodGet, Path: path, StatusCode: res.StatusCode}
	}
	var out []domain.WorkSession
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, 0, err
	}
	total, _ := strconv.Atoi(res.Header.Get("X-Total-Count"))
	return out, total, nil
}
```

Add `"strconv"` to `client.go` imports (`fmt` is already imported).

- [ ] **Step 7: apiclient test.** In `internal/adapter/apiclient/worktime_test.go` add a round-trip test against an `httptest.Server` (follow the file's existing fake-server pattern):

```go
func TestClient_ReassignAndPage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/sessions/reassign", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]int{"updated": 3})
	})
	mux.HandleFunc("GET /api/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Total-Count", "7")
		_ = json.NewEncoder(w).Encode([]domain.WorkSession{{ID: "a"}})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")
	if n, err := c.ReassignSessions(context.Background(), "p1", []string{"a", "b", "c"}); err != nil || n != 3 {
		t.Fatalf("reassign n=%d err=%v", n, err)
	}
	items, total, err := c.ListSessionsPage(context.Background(), 5, 0)
	if err != nil || total != 7 || len(items) != 1 {
		t.Fatalf("page items=%d total=%d err=%v", len(items), total, err)
	}
}
```

- [ ] **Step 8: Run + commit.**

Run: `go test ./internal/usecase/ ./internal/adapter/httpserver/ ./internal/adapter/apiclient/ -run 'BulkDelete|Reassign|Page'`
Expected: PASS.

```bash
git add internal/usecase/bulk_delete_sessions.go internal/usecase/bulk_delete_sessions_test.go internal/adapter/httpserver/worktime.go internal/adapter/httpserver/server.go internal/adapter/apiclient/client.go internal/adapter/apiclient/worktime_test.go
git commit -m "feat(sessions): BulkDeleteSessions endpoint + apiclient reassign/delete/page"
```

---

## Task 5: Reusable worktime components

Build the nine new components. Each is small, takes typed VMs, renders into Slice-0 tokens, and has a render-contains test. All strings via `T(ctx,…)`. Add i18n keys as you go (Task 9 verifies completeness, but add them here so render tests pass).

**Files:**
- Create the 9 `.templ` files + `internal/adapter/webui/components/worktime_components_test.go`
- Modify: `internal/i18n/catalog_de.go`, `catalog_en.go`

**Interfaces (Produces — later tasks rely on these exact signatures):**
- `components.ProgressBar(pct int, variant string)` — variant ∈ `hit|over|under|running`.
- `components.PaceDots(dots []components.PaceDot)`; `type PaceDot struct{ State string }` (State ∈ `behind|ontrack|ahead|running|holiday|off`).
- `components.SessionRow(vm components.SessionRowVM)`; VM below.
- `components.SessionBlock(vm components.SessionBlockVM)`; VM below.
- `components.KennzahlenPanel(vm components.KennzahlenVM)`; VM below.
- `components.WeekTotalBanner(vm components.WeekTotalVM)`; VM below.
- `components.ProjectFuzzyPicker(vm components.FuzzyPickerVM)`; VM below.
- `components.SelectionActionBar(vm components.SelectionBarVM)`; VM below.
- `components.SegToggle(options []components.SegOption, active string)`; `type SegOption struct{ Key, LabelKey, Href string }`.

VM types (define in a `worktime_vm.go` in package `components`):

```go
package components

type SessionRowVM struct {
	ID         string
	Title      string // project name or i18n "ohne Projekt"
	Hue        string // project hue; "" → unassigned styling
	Glyph      string // project glyph; "○" for unassigned
	Tag        string // without leading '#'; "" hides chip
	TimeRange  string // "08:30–10:00"
	Duration   string // "1h 30m"
	Unassigned bool
	Running    bool
	Selectable bool // render the row checkbox (bulk mode)
}

type SessionBlockVM struct {
	ID         string
	TopPx      int // (start - windowFloor) minutes / 60 * 48
	HeightPx   int // duration minutes / 60 * 48 (min 24)
	Hue        string
	Glyph      string
	Title      string
	TimeRange  string
	Tag        string
	Unassigned bool
	Running    bool
	Size       string // "" | "sm" | "md" (drives detail reveal; see mockup .block-sm/.block-md)
}

type KennzahlenVM struct {
	AvgPerDay   string // "7h 04m"
	GoalsHit    int    // X
	GoalsTotal  int    // 5
	Balance     string // "+2h 18m" / "−1h 05m"
	BalancePos  bool
	Dots        []PaceDot
	OnTrack     bool   // true → "auf Kurs", false → "Rückstand"
}

type WeekTotalVM struct {
	Total    string // "33h 41m"
	Target   string // "40h 00m"
	Pct      int
	Variant  string // hit|over|under|running (for the bar)
}

type FuzzyPickerVM struct {
	ID       string // dom id for the picker container
	Projects []FuzzyProjectVM
	FormID   string // the form whose hidden projectId/newProject fields this writes
}
type FuzzyProjectVM struct {
	ID    string
	Name  string
	Hue   string
	Glyph string
	Rate  string // "95 €/h" or "—"
}

type SelectionBarVM struct {
	Picker     FuzzyPickerVM
	AssignURL  string // POST target for reassign
	DeleteURL  string // POST target for bulk-delete
}
```

- [ ] **Step 1: Write render tests first** — `internal/adapter/webui/components/worktime_components_test.go`:

```go
package components_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestProgressBarVariants(t *testing.T) {
	for _, v := range []string{"hit", "over", "under", "running"} {
		out := render(t, components.ProgressBar(60, v))
		if !strings.Contains(out, "role=\"progressbar\"") {
			t.Errorf("%s: missing role=progressbar", v)
		}
		if !strings.Contains(out, "width:60%") {
			t.Errorf("%s: missing width style", v)
		}
	}
}

func TestPaceDots(t *testing.T) {
	out := render(t, components.PaceDots([]components.PaceDot{{State: "ahead"}, {State: "behind"}, {State: "off"}}))
	for _, w := range []string{"●", "○"} { // ahead/behind dots filled, off hollow
		if !strings.Contains(out, w) {
			t.Errorf("PaceDots missing %q", w)
		}
	}
}

func TestSessionRow_Unassigned(t *testing.T) {
	out := render(t, components.SessionRow(components.SessionRowVM{
		ID: "s1", Title: "ohne Projekt", Glyph: "○", TimeRange: "10:15–11:05",
		Duration: "50m", Unassigned: true, Selectable: true,
	}))
	for _, w := range []string{"10:15–11:05", "50m", "type=\"checkbox\"", "border-dashed"} {
		if !strings.Contains(out, w) {
			t.Errorf("SessionRow missing %q", w)
		}
	}
}

func TestSessionBlock_PositionAndUnassigned(t *testing.T) {
	out := render(t, components.SessionBlock(components.SessionBlockVM{
		ID: "b1", TopPx: 120, HeightPx: 72, Title: "ohne Projekt", Glyph: "○",
		TimeRange: "10:15–11:05", Unassigned: true, Size: "sm",
	}))
	for _, w := range []string{"top:120px", "height:72px", "block-unassigned", "data-session-id=\"b1\""} {
		if !strings.Contains(out, w) {
			t.Errorf("SessionBlock missing %q", w)
		}
	}
}

func TestKennzahlenPanel(t *testing.T) {
	out := render(t, components.KennzahlenPanel(components.KennzahlenVM{
		AvgPerDay: "7h 04m", GoalsHit: 4, GoalsTotal: 5, Balance: "+2h 18m", BalancePos: true,
		Dots: []components.PaceDot{{State: "ahead"}}, OnTrack: true,
	}))
	for _, w := range []string{"7h 04m", "4", "+2h 18m"} {
		if !strings.Contains(out, w) {
			t.Errorf("Kennzahlen missing %q", w)
		}
	}
}

func TestWeekTotalBanner(t *testing.T) {
	out := render(t, components.WeekTotalBanner(components.WeekTotalVM{Total: "33h 41m", Target: "40h 00m", Pct: 84, Variant: "under"}))
	if !strings.Contains(out, "33h 41m") || !strings.Contains(out, "40h 00m") {
		t.Errorf("WeekTotalBanner missing totals")
	}
}

func TestProjectFuzzyPicker_InlineCreate(t *testing.T) {
	out := render(t, components.ProjectFuzzyPicker(components.FuzzyPickerVM{
		ID: "pick", FormID: "bulkForm",
		Projects: []components.FuzzyProjectVM{{ID: "p1", Name: "flow", Hue: "blue", Glyph: "◆", Rate: "95 €/h"}},
	}))
	for _, w := range []string{"role=\"listbox\"", "flow", "data-new-project", "✚"} {
		if !strings.Contains(out, w) {
			t.Errorf("FuzzyPicker missing %q", w)
		}
	}
}

func TestSelectionActionBar(t *testing.T) {
	out := render(t, components.SelectionActionBar(components.SelectionBarVM{
		AssignURL: "/ui/historie/reassign", DeleteURL: "/ui/historie/bulk-delete",
		Picker: components.FuzzyPickerVM{ID: "pick", FormID: "bulkForm"},
	}))
	for _, w := range []string{"data-sel-count", "/ui/historie/reassign", "/ui/historie/bulk-delete"} {
		if !strings.Contains(out, w) {
			t.Errorf("SelectionActionBar missing %q", w)
		}
	}
}

func TestSegToggle(t *testing.T) {
	out := render(t, components.SegToggle([]components.SegOption{
		{Key: "cal", LabelKey: "historie.calendar", Href: "/historie?view=cal"},
		{Key: "list", LabelKey: "historie.list", Href: "/historie?view=list"},
	}, "cal"))
	if !strings.Contains(out, "aria-pressed=\"true\"") {
		t.Errorf("SegToggle missing active aria-pressed")
	}
}
```

- [ ] **Step 2: Run — expect FAIL (all undefined).**

Run: `go test ./internal/adapter/webui/components/ -run 'ProgressBar|PaceDots|SessionRow|SessionBlock|Kennzahlen|WeekTotal|FuzzyPicker|SelectionActionBar|SegToggle'`

- [ ] **Step 3: Add the VM file** `internal/adapter/webui/components/worktime_vm.go` (paste the VM block from Interfaces above).

- [ ] **Step 4: Implement components.** Write each `.templ`. Use the mockup classes verbatim. Key implementations:

`progressbar.templ`:
```go
package components

// progressFill maps a variant onto its fill color utility.
func progressFill(variant string) string {
	switch variant {
	case "over", "hit":
		return "bg-green"
	case "running":
		return "bg-cyan"
	default:
		return "bg-yellow"
	}
}

// ProgressBar renders a horizontal bar filled to pct% in the variant color.
templ ProgressBar(pct int, variant string) {
	<div class="h-2 w-full overflow-hidden rounded-full bg-sunken" role="progressbar"
		aria-valuenow={ itoa(pct) } aria-valuemin="0" aria-valuemax="100">
		<div class={ "h-full rounded-full transition-[width]", progressFill(variant) } style={ "width:" + itoa(pct) + "%" }></div>
	</div>
}
```
> `itoa` must be a small helper in package components — add to `worktime_vm.go`: `func itoa(n int) string { return strconv.Itoa(n) }` (import `strconv`).

`pacedots.templ`:
```go
package components

func paceGlyph(state string) string {
	if state == "off" || state == "behind" { return "○" }
	return "●"
}
func paceColor(state string) string {
	switch state {
	case "ahead", "ontrack": return "text-green"
	case "behind": return "text-yellow"
	case "running": return "text-cyan"
	case "holiday": return "text-blue"
	default: return "text-faint"
	}
}
templ PaceDots(dots []PaceDot) {
	<span class="inline-flex items-center gap-1" aria-hidden="true">
		for _, d := range dots {
			<span class={ "text-[.7rem]", paceColor(d.State) }>{ paceGlyph(d.State) }</span>
		}
	</span>
}
```

`sessionrow.templ` — port the mobile-agenda `<li>` markup from `studio-worktime-calendar.html:826-862`. Render `data-session-id`, the checkbox (`type="checkbox" class="chk row-chk"` — hidden unless `Selectable`), the glyph tile (`bg-{hue}/10 text-{hue}`, or `bg-orange/10 text-orange` + dashed border when `Unassigned`), title + optional `@Tag(vm.Tag)`, time range, duration. For the bulk-delete confirm path the row itself isn't destructive — deletion happens via the action bar.

`sessionblock.templ` — port the `<button class="block …">` markup from `studio-worktime-calendar.html:600-627`. Set `style={ "top:" + itoa(vm.TopPx) + "px; height:" + itoa(vm.HeightPx) + "px" }`; add `--c:var(--{hue})` via a `style` concatenation when not unassigned; classes `block`, plus `block-unassigned` / `block-running` / `block-sm` / `block-md` per VM; emit `data-session-id={vm.ID}` and `aria-label` from VM fields. Title line uses `vm.Glyph` + `vm.Title`; time line `vm.TimeRange`; extra line the `@Tag`.

`kennzahlen.templ` — port `studio-worktime-week.html` KENNZAHLEN panel (Schnitt/Ziele/Saldo/PaceDots/Status). Use `@StatTile` for the three numbers and `@PaceDots(vm.Dots)`; status line green "auf Kurs" / yellow "Rückstand" via `T(ctx,"kennzahlen.ontrack")` / `"kennzahlen.behind"`.

`weektotal.templ` — port the WOCHE GESAMT banner; show `vm.Total` of `vm.Target` + `@ProgressBar(vm.Pct, vm.Variant)`.

`fuzzypicker.templ` — port the open dropdown from `studio-worktime-calendar.html:1012-1039`: a search `<input data-fuzzy-filter>`, a `<ul role="listbox">` of project options (each `<li role="option" data-project-id={p.ID}>` with `@Chip`-style glyph + name + rate), and a final inline-create `<button data-new-project>✚ neu: …</button>`. It writes the chosen id into the form `vm.FormID` (the selection JS handles wiring; just render the markup + data attributes).

`selectionbar.templ` — port the sticky action bar `studio-worktime-calendar.html:993-1050`. Contains: a `<span data-sel-count>0</span> ausgewählt` badge, the embedded `@ProjectFuzzyPicker(vm.Picker)` behind an "Projekt zuweisen" button (`data-assign-toggle`), a "Löschen" button (`data-bulk-delete` → opens `@ConfirmDialog`), an "Abbrechen" button (`data-bulk-cancel`). The hidden bulk form posts to `vm.AssignURL` / `vm.DeleteURL` with a hidden `name="ids"` field the JS fills (comma-joined). Wrap the destructive delete in a Slice-0 `@ConfirmDialog` (no `window.confirm`).

`segtoggle.templ`:
```go
package components

templ SegToggle(options []SegOption, active string) {
	<div class="seg inline-flex" role="group">
		for _, o := range options {
			if o.Key == active {
				<a href={ templ.SafeURL(o.Href) } aria-pressed="true"
					class="px-3.5 py-1.5 text-[.85rem] font-semibold text-blue rounded-[9px] bg-surface shadow-soft">{ T(ctx, o.LabelKey) }</a>
			} else {
				<a href={ templ.SafeURL(o.Href) } aria-pressed="false"
					class="px-3.5 py-1.5 text-[.85rem] font-medium text-muted hover:text-ink">{ T(ctx, o.LabelKey) }</a>
			}
		}
	</div>
}
```
> The `.seg` class is defined in the Studio token CSS; ensure `web/tailwind.css` carries the `.seg`/`.seg button`/`[aria-pressed]` rules (and `.block*`, `.grid-lines`, `.now-line`, `.chk`, `.pick-row`, `--hour`/`--grid-h`) from the calendar mockup `<style>` block — add them to `web/tailwind.css` in a `@layer components` section if missing, then rebuild app.css (`make web`). This is required for the calendar to render; verify in Task 8.

- [ ] **Step 5: Add i18n keys.** In `internal/i18n/catalog_de.go` add (and English stubs in `catalog_en.go`):

```
"nav.week": "Woche", "nav.history": "Historie",
"historie.title": "Historie", "historie.calendar": "Kalender", "historie.list": "Liste",
"historie.week": "Woche", "historie.month": "Monat",
"historie.unassigned": "ohne Projekt", "historie.assign": "zuweisen",
"historie.select": "Auswählen", "historie.selectDone": "Fertig",
"historie.selectAllUnassigned": "Alle ohne Projekt auswählen",
"historie.selectWeek": "ganze Woche", "historie.selectDay": "Tag wählen",
"historie.assignProject": "Projekt zuweisen", "historie.delete": "Löschen", "historie.cancel": "Abbrechen",
"historie.selectedCount": "ausgewählt",
"historie.unassignedBannerOne": "1 Sitzung ohne Projekt", "historie.unassignedBannerMany": "%d Sitzungen ohne Projekt",
"historie.confirmDelete": "Ausgewählte Sitzungen löschen? Das kann nicht rückgängig gemacht werden.",
"picker.filter": "Projekt filtern…", "picker.new": "neu",
"kennzahlen.title": "Kennzahlen", "kennzahlen.avg": "Schnitt", "kennzahlen.goals": "Ziele",
"kennzahlen.balance": "Saldo", "kennzahlen.ontrack": "auf Kurs", "kennzahlen.behind": "Rückstand",
"week.total": "Woche gesamt", "week.target": "Ziel",
"heute.title": "Heute", "heute.start": "Starten", "heute.stop": "Stopp",
"heute.target": "Tagesziel", "heute.balance": "Saldo", "heute.empty": "Noch keine Sitzung heute — starte oben.",
"woche.title": "Woche", "woche.empty": "Keine Sitzungen diese Woche.",
"historie.empty": "Keine Sitzungen im Zeitraum.", "historie.thisWeek": "Diese Woche",
"sessions.edit": "Sitzung bearbeiten", "sessions.project": "Projekt", "sessions.start": "Start",
"sessions.stop": "Stop", "sessions.tag": "Tag", "sessions.note": "Notiz", "sessions.save": "Speichern",
```
> Plurals (e.g. unassigned banner) use `Tn(ctx, "historie.unassignedBanner", n)` if the catalog supports plural forms; otherwise pick the one/many key in the handler. Check `internal/i18n/i18n.go` `Tn` semantics and follow whatever the existing plural keys do (e.g. how `page.page` was done). Keep it consistent.

- [ ] **Step 6: Generate templ + run tests.**

Run: `templ generate ./internal/adapter/webui/... && go test ./internal/adapter/webui/components/`
Expected: PASS.

- [ ] **Step 7: Commit.**

```bash
git add internal/adapter/webui/components/ internal/i18n/catalog_de.go internal/i18n/catalog_en.go web/tailwind.css internal/adapter/webui/static/app.css
git commit -m "feat(webui): worktime component library (progress/pace/row/block/kennzahlen/picker/selbar/seg)"
```

---

## Task 6: Heute page + handler (port `/` to AppShell)

**Files:**
- Create: `internal/adapter/webui/heute.templ` (+ `heute_vm.go` if cleaner)
- Create: `internal/adapter/httpserver/webui_heute.go`, `webui_heute_test.go`
- Modify: `server.go` (repoint `/{$}` + worktime fragment/action routes to new handlers)

**Interfaces:**
- Consumes: `components.*` (Task 5), existing `s.worktimeDataFor`-style data (ListSessionsRange/ListProjects), existing action usecases (Start/Stop/Add/Edit/Delete).
- Produces: `webui.HeutePage(vm webui.HeuteVM)`, `webui.HeuteFragment(vm webui.HeuteVM)`; handlers `handleHeuteHome`, `handleHeuteFragment`, reuse existing `handleWebStart/Stop/Add/Edit/Delete` (they call `s.renderFragment` → repoint to HeuteFragment).

Define `webui.HeuteVM` (in heute.templ or heute_vm.go), built from the same inputs as `WorktimeData` but shaped for the new components:

```go
type HeuteVM struct {
	User       string
	Date       time.Time
	Running    *domain.WorkSession
	Rows       []components.SessionRowVM
	Projects   []components.FuzzyProjectVM
	LoggedDur  string // "5h 12m"
	TargetDur  string // "8h 00m"
	TargetPct  int
	TargetVar  string // hit|over|under|running
	Balance    string
	BalancePos bool
	WeekDots   []components.PaceDot
	Err        string
}
```

- [ ] **Step 1: Handler test first** — `webui_heute_test.go`: build a server with a `FakeSessionStore` holding one running + one completed session for `testUserID`, GET `/`, assert 200 + body contains the running project, `data-timer` (live timer hook from Base), and the start form. Mirror `webui_worktime_handlers_test.go` server construction (it already wires the worktime usecases) and add nothing new to the struct.

```go
func TestHeuteHome_RendersLiveAndSessions(t *testing.T) {
	srv, ss, token := newWebWorktimeServer(t) // same constructor webui_worktime_handlers_test.go uses
	_ = token
	ctx := context.Background()
	st := time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC)
	_, _ = ss.Create(ctx, domain.WorkSession{ID: "r", OwnerID: testUserID, Start: st}) // running (no stop)
	req := webGet(t, "/") // helper that attaches the web session cookie, as other webui tests do
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	for _, w := range []string{"data-timer", "/static/app.css", "id=\"content\""} {
		if !strings.Contains(rr.Body.String(), w) {
			t.Errorf("Heute missing %q", w)
		}
	}
}
```
> Match the exact cookie/login helper the existing webui tests use (see `webui_worktime_handlers_test.go` / `webauth_test.go`). Do NOT invent a new auth path.

- [ ] **Step 2: Run — FAIL** (handler/route absent).

- [ ] **Step 3: Build the VM + viewmodel builder.** In `webui_heute.go`, write `heuteDataFor(ctx, u, day) (webui.HeuteVM, error)`: call `ListSessionsRange(u.ID, day, day+1d)`, `ListProjects`, derive running pointer, map sessions → `components.SessionRowVM` (compute `TimeRange`/`Duration` with the existing time formatting — reuse `wtfmt`-equivalent or a small local `fmtClock`/`fmtDur`), compute logged vs target (reuse the stats/target logic the old worktime page used — look at how `WorktimeData` / `handleWebStatsFragment` computed target; reuse `s.GetSettings`/`s.Stats` as the old page did). Map projects → `FuzzyProjectVM` (hue=`p.Color`, glyph=`p.Glyph`, rate via `p.Rate`).

- [ ] **Step 4: Build the templ.** `heute.templ` (package webui), port `direction-b-studio.html` content into AppShell:

```go
package webui

import (
	"time"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

templ HeutePage(vm HeuteVM) {
	@components.Base("flow · heute", heuteBody(vm))
}
templ heuteBody(vm HeuteVM) {
	@components.AppShell("today", nil, worktimeSubnav("today"), heuteContent(vm))
}
templ heuteContent(vm HeuteVM) {
	<div id="content" hx-ext="sse" sse-swap="session.started,session.stopped,session.updated,session.deleted,project.created"
		hx-get="/ui/worktime" hx-trigger="sse:session.started,sse:session.stopped,sse:session.updated,sse:session.deleted,sse:project.created" hx-swap="innerHTML">
		@HeuteFragment(vm)
	</div>
}
templ HeuteFragment(vm HeuteVM) {
	// live card, start form (+ @components.ProjectFuzzyPicker), target @components.ProgressBar,
	// balance, today's rows (@components.SessionRow), week strip (@components.PaceDots),
	// edit/delete via @components.Dialog/@components.ConfirmDialog — port markup from
	// direction-b-studio.html. Empty state: @components.EmptyState when len(vm.Rows)==0 && vm.Running==nil.
}
```
> `worktimeSubnav(active string)` is a shared helper (define once, e.g. in `heute.templ`) returning `@components.TabStrip` with the five tabs exactly as `styleguideSubnav` does (`today→/`, `week→/woche`, `history→/historie`, `stats→/stats`, `frei→/frei`). Woche/Historie reuse it.
> The live-card ticking duration uses the Base live-timer (`data-timer` attribute with the running start epoch — see how Base's timer script reads its hook; emit the same attribute the old worktime page used). Verify the timer attribute name against `components/base.templ`.

- [ ] **Step 5: Repoint routes.** In `server.go`:
- `GET /{$}` → `s.handleHeuteHome`
- `GET /ui/worktime` → `s.handleHeuteFragment`
- Keep `POST /ui/worktime/{start,stop,add,edit,delete}` but make their `renderFragment`/`renderDay` render `HeuteFragment` (update `renderFragment` in `webui.go` and `renderDay` in `webui_worktime.go` to call `webui.HeuteFragment(heuteVM)` instead of `webui.WorktimeFragment`). Build the HeuteVM there.

- [ ] **Step 6: Generate + test.**

Run: `templ generate ./... && go test ./internal/adapter/httpserver/ -run Heute`
Expected: PASS.

- [ ] **Step 7: Commit.**

```bash
git add internal/adapter/webui/heute.templ internal/adapter/webui/heute_templ.go internal/adapter/httpserver/webui_heute.go internal/adapter/httpserver/webui_heute_test.go internal/adapter/httpserver/server.go internal/adapter/httpserver/webui.go internal/adapter/httpserver/webui_worktime.go
git commit -m "feat(webui): Heute page on AppShell (port /), live SSE + start/stop/edit"
```

---

## Task 7: Woche page + handler

**Files:**
- Create: `internal/adapter/webui/woche.templ` (+ vm), `internal/adapter/httpserver/webui_woche.go`, `_test.go`
- Modify: `server.go` (routes)

**Interfaces:**
- Consumes: `ListSessionsRange`, `ListDayOffs`, `GetSettings`, `Stats` (for targets/kennzahlen — reuse the same computations the TUI `week/summary.go` and the old `handleWebStatsFragment` use).
- Produces: `webui.WochePage(vm)`, `webui.WocheFragment(vm)`; `handleWocheHome`, `handleWocheFragment`. Routes `GET /woche`, `GET /ui/woche/fragment?week=YYYY-MM-DD`.

`webui.WocheVM`:
```go
type WocheVM struct {
	User       string
	WeekStart  time.Time // Monday
	WeekLabel  string    // "16.–22. Juni 2026"
	PrevWeek   string    // yyyy-mm-dd of prev Monday
	NextWeek   string
	CanForward bool
	Days       []WocheDayVM
	Total      components.WeekTotalVM
	Kennzahlen components.KennzahlenVM
	Empty      bool
}
type WocheDayVM struct {
	Label    string // "Mo 16"
	Dur      string // "7h 36m"
	Pct      int
	Variant  string // hit|over|under|running|weekend
	DayOff   bool
	DayOffLabel string // kind label, e.g. "Urlaub"
	DayOffHue   string
	IsToday  bool
}
```

- [ ] **Step 1: Handler test first** — GET `/woche`, assert 200 + contains "Woche gesamt" label and a day bar; seed two sessions in the current week via the fake store. Use the same web-auth helper.

- [ ] **Step 2: Run — FAIL.**

- [ ] **Step 3: Build week math.** `webui_woche.go`: resolve Monday of the requested week (`?week=` param, default this week via `s.Clock.Now()`), fetch range Mon..Mon+7d, bucket sessions per weekday (group by `start.In(time.Local)` date — heed the tz lesson: group in `now.Location()`), compute per-day duration + target (weekday target from settings), Mon–Fri week total + target (weekends excluded), kennzahlen (avg, goals hit X/5, balance, pace dots, on-track). Reuse helpers from the TUI `internal/tui/screen/worktime/week` where the math already exists (read `week/summary.go` + `week/pacedot.go` and port the pure functions — do NOT import the TUI package; copy the small pure logic into a `webui`/`usecase` helper).

- [ ] **Step 4: Build templ** porting `studio-worktime-week.html` into AppShell with `worktimeSubnav("week")`, `@components.SegToggle` not needed here, `‹ KW ›` nav, `@components.WeekTotalBanner`, `@components.KennzahlenPanel`, day bars via `@components.ProgressBar`. Fragment wrapper with `sse-swap` on `session.*,dayoff.changed,settings.changed`.

- [ ] **Step 5: Routes** `GET /woche` → `handleWocheHome`, `GET /ui/woche/fragment` → `handleWocheFragment`.

- [ ] **Step 6: Generate + test.** `templ generate ./... && go test ./internal/adapter/httpserver/ -run Woche` → PASS.

- [ ] **Step 7: Commit.**

```bash
git add internal/adapter/webui/woche.templ internal/adapter/webui/woche_templ.go internal/adapter/httpserver/webui_woche.go internal/adapter/httpserver/webui_woche_test.go internal/adapter/httpserver/server.go
git commit -m "feat(webui): Woche page (day bars + WOCHE GESAMT + Kennzahlen, KW nav, SSE)"
```

---

## Task 8: Historie page + handler (Kalender/Monat/Agenda/Liste + bulk + selection JS)

The hero. Calendar (hybrid window), month nav, mobile agenda, paginated list, bulk reassign/delete, single-edit.

**Files:**
- Create: `internal/adapter/webui/historie.templ` (+ vm), `internal/adapter/httpserver/webui_historie.go`, `_test.go`
- Create: `internal/adapter/webui/static/js/historie-select.js`
- Modify: `server.go` (routes)

**Interfaces:**
- Consumes: `ListSessionsRange` (week/month window), `ListSessionsPage` (list view), `ListProjects`, `BulkAssignProject` (via handler), `BulkDeleteSessions`, `CreateProject` (inline-create), `EditSession`/`DeleteSession` (single edit). Components from Task 5.
- Produces: `webui.HistoriePage(vm)`, `webui.HistorieCalendarFragment(vm)`, `webui.HistorieListFragment(vm)`, `webui.HistorieAgendaFragment(vm)`; handlers + routes below.

`webui.HistorieVM` (calendar) and `webui.HistorieListVM` (list):
```go
type HistorieVM struct {
	User        string
	View        string // "cal" | "list"
	CalView     string // "week" | "month"
	WeekStart   time.Time
	RangeLabel  string
	PrevHref    string
	NextHref    string
	ThisHref    string
	WindowFloorMin int // minutes since 00:00 of grid top (e.g. 360)
	HourPx      int    // 48
	GridHeightPx int   // (ceil-floor)/60*48
	Days        []HistorieDayVM // 7 columns for week view
	MonthCells  []HistorieMonthCellVM // for month view
	Projects    []components.FuzzyProjectVM
	UnassignedCount int
	Empty       bool
}
type HistorieDayVM struct {
	Label   string // "Mo"
	DayNum  string // "16"
	Dur     string
	IsToday bool
	IsWeekend bool
	NowLineTopPx int // -1 if not today
	Blocks  []components.SessionBlockVM
	Rows    []components.SessionRowVM // mobile agenda
}
type HistorieMonthCellVM struct {
	DayNum string
	Hours  string
	HasUnassigned bool
	IsToday bool
	IsWeekend bool
	WeekHref string // jump to that week
	Empty bool
}
type HistorieListVM struct {
	User string
	Rows []components.SessionRowVM
	Projects []components.FuzzyProjectVM
	Page components.PageNav // Slice-0 pagination VM
}
```

- [ ] **Step 1: Handler test first** — three checks:
  1. `GET /historie` → 200, contains the calendar grid (`grid-lines`) and the subnav.
  2. `GET /historie?view=list` → 200, contains pagination + session rows.
  3. `POST /ui/historie/reassign` with form `ids=a,b&projectId=p1` → assigns (assert via store) and returns the refreshed calendar fragment (200). Add a `newProject` variant asserting inline-create.

Seed sessions (some unassigned) in the current week for `testUserID`. Use the web-auth helper.

- [ ] **Step 2: Run — FAIL.**

- [ ] **Step 3: Hybrid window math.** In `webui_historie.go` write `gridWindow(sessions []domain.WorkSession) (floorMin, ceilMin int)`:

```go
// gridWindow returns the [floor,ceil] minute-of-day band for the week's grid:
// default 06:00–20:00, expanded down/up to the hour to fit any out-of-band
// session start/stop. Sessions with no stop (running) use now via the caller.
func gridWindow(mins []int) (int, int) {
	floor, ceil := 360, 1200 // 06:00, 20:00
	for _, m := range mins {
		if m < floor {
			floor = (m / 60) * 60 // snap down to the hour
		}
		if m > ceil {
			ceil = ((m + 59) / 60) * 60 // snap up to the hour
		}
	}
	if floor < 0 {
		floor = 0
	}
	if ceil > 1440 {
		ceil = 1440
	}
	return floor, ceil
}
```
Collect every session's start-minute and stop-minute (local tz) into `mins`; running session's stop = now. `GridHeightPx = (ceil-floor)/60*HourPx`. Each block `TopPx = (startMin-floor)/60*HourPx`, `HeightPx = max(24,(stopMin-startMin)/60*HourPx)`; `NowLineTopPx` for today = `(nowMin-floor)/60*HourPx`.

- [ ] **Step 4: Add a small unit test for gridWindow** (pure, fast — put in `webui_historie_test.go` or an internal test):

```go
func TestGridWindow(t *testing.T) {
	if f, c := gridWindow(nil); f != 360 || c != 1200 {
		t.Fatalf("default = %d,%d want 360,1200", f, c)
	}
	if f, c := gridWindow([]int{310, 1260}); f != 300 || c != 1260 {
		t.Fatalf("expand = %d,%d want 300,1260", f, c)
	}
}
```

- [ ] **Step 5: Build the handlers.**
- `handleHistorieHome` — parse `?view` (cal|list, default cal), `?cal` (week|month, default week), `?week=` (Monday). For cal/week: fetch range, build `Days` (blocks + agenda rows), `Projects`, `UnassignedCount`, window math. For month: fetch month range, build `MonthCells`. For list: fetch `ListSessionsPage` with `?page` (page size 50), build `Rows` + `components.PageNav`. Render `HistoriePage`.
- `handleHistorieCalendarFragment` (`GET /ui/historie/calendar`) and `handleHistorieListFragment` (`GET /ui/historie/list`) for SSE/pagination swaps.
- `handleHistorieReassign` (`POST /ui/historie/reassign`): `r.ParseForm()`, `ids := strings.Split(r.FormValue("ids"), ",")` (drop empties), resolve project via the existing `resolveWebProject` inline-create pattern (returns `*string`; if nil → error fragment), call `s.BulkAssignProject.Execute(u.ID, ids, *pid)`, publish `session.updated`, re-render the calendar (or list) fragment based on `r.FormValue("view")`.
- `handleHistorieBulkDelete` (`POST /ui/historie/bulk-delete`): same id parsing, `s.BulkDeleteSessions.Execute`, publish `session.deleted`, re-render fragment.

```go
func splitIDs(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
```

- [ ] **Step 6: Build the templ.** `historie.templ` ports `studio-worktime-calendar.html`:
  - `HistoriePage` → `@components.Base` → `@components.AppShell("history", nil, worktimeSubnav("history"), historieContent(vm))`.
  - Toolbar: `@components.SegToggle` for Kalender|Liste and (in cal) Woche|Monat, `‹ › / Diese Woche` nav, project filter `<select>`, "Auswählen" button (`data-select-toggle`). Orange banner when `vm.UnassignedCount>0` using `Tn`.
  - Week grid: time axis + 7 `HistorieDayVM` columns, each `.grid-lines` with `@components.SessionBlock` per block + `.now-line` div when today; day-header buttons with `data-day` for "Tag wählen".
  - Month grid: `MonthCells` → day cells with mini-bars + ○ flag + `WeekHref` link.
  - Mobile agenda: `md:hidden`, per-day `@components.SessionRow` (selectable).
  - List fragment: `HistorieListFragment` = rows (`@components.SessionRow`) + `@components.Pagination(vm.Page)`.
  - `@components.SelectionActionBar` (sticky, hidden until select-mode) wired to `/ui/historie/reassign` + `/ui/historie/bulk-delete`.
  - Single-edit: `@components.Dialog("editSession", "sessions.edit", editSessionForm(...))` posting to `/ui/worktime/edit` (reuse the existing edit handler) — destructive delete inside via `@components.ConfirmDialog`.
  - Load the selection JS: `<script src="/static/js/historie-select.js" defer></script>` at the end of `historieContent`.

- [ ] **Step 7: Write the selection JS** `internal/adapter/webui/static/js/historie-select.js` — dependency-free, ports the mockup `<script>` (lines 1140-1311) but adapted to POST real ids:

```js
(function () {
  "use strict";
  var selectMode = false;
  var selected = new Set();
  function $(s, r) { return (r || document).querySelector(s); }
  function $all(s, r) { return Array.prototype.slice.call((r || document).querySelectorAll(s)); }

  function setMode(on) {
    selectMode = on;
    document.body.classList.toggle("is-selecting", on);
    var bar = $("[data-action-bar]"); if (bar) bar.classList.toggle("hidden", !on);
    $all("[data-select-toggle]").forEach(function (b) {
      b.textContent = on ? b.getAttribute("data-label-done") : b.getAttribute("data-label-select");
    });
    $all(".row-chk").forEach(function (c) { c.classList.toggle("hidden", !on); });
    $all("[data-day-select]").forEach(function (d) { d.classList.toggle("hidden", !on); });
    if (!on) { selected.clear(); paint(); }
    updateCount();
  }
  function toggleId(id, on) {
    if (on === undefined) on = !selected.has(id);
    if (on) selected.add(id); else selected.delete(id);
  }
  function paint() {
    $all("[data-session-id]").forEach(function (el) {
      el.classList.toggle("is-selected", selected.has(el.getAttribute("data-session-id")));
    });
    $all(".row-chk").forEach(function (c) {
      var id = c.getAttribute("data-session-id"); if (id) c.checked = selected.has(id);
    });
  }
  function updateCount() {
    $all("[data-sel-count]").forEach(function (el) { el.textContent = String(selected.size); });
    var hidden = $("[data-ids-field]"); if (hidden) hidden.value = Array.from(selected).join(",");
  }

  document.addEventListener("click", function (e) {
    var t = e.target;
    if (t.closest("[data-select-toggle]")) { e.preventDefault(); setMode(!selectMode); return; }
    if (t.closest("[data-select-unassigned]")) {
      e.preventDefault(); if (!selectMode) setMode(true);
      $all("[data-session-id][data-unassigned='1']").forEach(function (el) { toggleId(el.getAttribute("data-session-id"), true); });
      paint(); updateCount(); return;
    }
    if (t.closest("[data-select-week]")) {
      e.preventDefault(); if (!selectMode) setMode(true);
      $all("[data-session-id]").forEach(function (el) { toggleId(el.getAttribute("data-session-id"), true); });
      paint(); updateCount(); return;
    }
    var dayBtn = t.closest("[data-day-select]");
    if (dayBtn && selectMode) {
      e.preventDefault();
      var day = dayBtn.getAttribute("data-day-select");
      $all("[data-session-id][data-day='" + day + "']").forEach(function (el) { toggleId(el.getAttribute("data-session-id"), true); });
      paint(); updateCount(); return;
    }
    var blk = t.closest("[data-session-id]");
    if (blk && selectMode) {
      e.preventDefault();
      toggleId(blk.getAttribute("data-session-id"));
      paint(); updateCount(); return;
    }
    if (t.closest("[data-bulk-cancel]")) { e.preventDefault(); setMode(false); return; }
  }, true);

  // checkbox change (mobile agenda / list rows)
  document.addEventListener("change", function (e) {
    var c = e.target;
    if (c.classList && c.classList.contains("row-chk")) {
      toggleId(c.getAttribute("data-session-id"), c.checked); updateCount();
    }
  });

  // Esc exits select mode
  document.addEventListener("keydown", function (e) { if (e.key === "Escape" && selectMode) setMode(false); });

  // Re-bind after htmx swaps re-render fragments
  document.body.addEventListener("htmx:afterSwap", function () { paint(); updateCount(); });
})();
```
> The bulk forms (`data-ids-field` hidden input inside the reassign and delete forms) are filled by `updateCount`; htmx submits them. The "neu" inline-create writes the new project name into a `data-new-project`-named field on the reassign form before submit (small handler the picker template wires, or extend the JS to read `data-new-project` clicks). Keep it minimal and dependency-free.

- [ ] **Step 8: Routes.** In `server.go`:
```go
	mux.Handle("GET /historie", s.webAuth(http.HandlerFunc(s.handleHistorieHome)))
	mux.Handle("GET /ui/historie/calendar", s.webAuth(http.HandlerFunc(s.handleHistorieCalendarFragment)))
	mux.Handle("GET /ui/historie/list", s.webAuth(http.HandlerFunc(s.handleHistorieListFragment)))
	mux.Handle("POST /ui/historie/reassign", s.webAuth(http.HandlerFunc(s.handleHistorieReassign)))
	mux.Handle("POST /ui/historie/bulk-delete", s.webAuth(http.HandlerFunc(s.handleHistorieBulkDelete)))
```

- [ ] **Step 9: Generate + test.**

Run: `templ generate ./... && go test ./internal/adapter/httpserver/ -run Historie && go test ./internal/adapter/webui/...`
Expected: PASS.

- [ ] **Step 10: Commit.**

```bash
git add internal/adapter/webui/historie.templ internal/adapter/webui/historie_templ.go internal/adapter/webui/static/js/historie-select.js internal/adapter/httpserver/webui_historie.go internal/adapter/httpserver/webui_historie_test.go internal/adapter/httpserver/server.go
git commit -m "feat(webui): Historie (calendar/month/agenda/list + bulk reassign/delete + selection JS)"
```

---

## Task 9: Main-wiring + verification + cleanup

Wire usecases into `cmd/flow-server/main.go`, delete the dead old worktime page, run the full gate, and live-verify every route.

**Files:**
- Modify: `cmd/flow-server/main.go`
- Delete: `internal/adapter/webui/worktime.templ`, `internal/adapter/webui/worktime_templ.go` (only if nothing else references `WorktimePage`/`WorktimeFragment`/`WorktimeData`)
- Modify: `internal/i18n/*` (only if keys still missing)

- [ ] **Step 1: Wire usecases in main.** In `cmd/flow-server/main.go`, in the `&httpserver.Server{…}` literal, add (next to the other session usecases):

```go
		ListSessionsPage:   usecase.ListSessionsPage{Sessions: sessionStore},
		BulkAssignProject:  usecase.BulkAssignProject{Sessions: sessionStore, Projects: projectStore},
		BulkDeleteSessions: usecase.BulkDeleteSessions{Sessions: sessionStore},
```
(`projectStore` is already constructed in main — confirm its variable name and reuse it.)

- [ ] **Step 2: Build + vet the whole module.**

Run: `templ generate ./... && go build ./... && go vet ./...`
Expected: no errors. Fix any missed wiring.

- [ ] **Step 3: Remove the dead old worktime page.** Confirm nothing references it:

Run: `rg -n "WorktimePage|WorktimeFragment|WorktimeData|worktimeData|worktimeDataFor" internal/`
- If the only references are the now-repointed handlers (Task 6 should have replaced `renderFragment`/`renderDay` to build `HeuteVM`), delete `worktime.templ` + `worktime_templ.go`. If `worktimeDataFor` is still used to build the HeuteVM, keep the Go builder but delete only the templ page/fragment. Re-run the rg until no dead templ symbol remains.

```bash
rm internal/adapter/webui/worktime.templ internal/adapter/webui/worktime_templ.go   # only if rg shows them unused
templ generate ./... && go build ./...
```

- [ ] **Step 4: i18n completeness check.** Ensure every `T(ctx,"…")` / `Tn` key used in the new templates exists in `catalog_de.go`:

Run: `rg -o 'T\(ctx, "[a-z0-9._]+"' internal/adapter/webui | rg -o '"[a-z0-9._]+"' | sort -u` and eyeball each against `catalog_de.go`. Add any missing key to DE (full) + EN (stub).

- [ ] **Step 5: Full CI gate (lint included — NOT just go test).**

Run: `make ci`
Expected: green — lint (gofumpt/staticcheck incl. QF1002), `verify-generate` (templ up to date), `verify-css` (committed app.css matches a fresh Tailwind build — run `make web` and commit `internal/adapter/webui/static/app.css` if it drifted), coverage gate, build. Fix until green.

- [ ] **Step 6: Curl-smoke every route against the dev stack.** Bring up Postgres+Dex and a server token (`make dev-up`, `make dev-run`, `make dev-token` per the dev-env memory; `FLOW_DEV=1`). Then:

```bash
TOKEN=$(make -s dev-token)
BASE=https://localhost:8443   # adjust to dev addr; use -k for self-signed
# REST
curl -ks -o /dev/null -w "sessions paginated: %{http_code} (X-Total-Count via -D)\n" -D - "$BASE/api/v1/sessions?limit=2&offset=0" -H "Authorization: Bearer $TOKEN" | rg -i 'x-total-count|200'
curl -ks -w "reassign: %{http_code}\n" -X POST "$BASE/api/v1/sessions/reassign" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"ids":[],"projectId":"x"}'   # expect 400 (empty ids)
curl -ks -w "bulk-delete: %{http_code}\n" -X POST "$BASE/api/v1/sessions/bulk-delete" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"ids":[]}'   # expect 400
```
For the WebUI routes (cookie-auth), do a scripted Dex login (reuse the dogfood login helper from prior slices — see the M1e/M2a done-gate notes) and curl `/`, `/woche`, `/historie`, `/historie?view=list`, `/historie?cal=month`, `/ui/worktime`, `/ui/woche/fragment`, `/ui/historie/calendar`, `/ui/historie/list` — each must be 200 and the calendar must reference `/static/app.css` + `/static/js/historie-select.js` and contain `grid-lines`. Confirm no external origins in the HTML (`rg -i 'unpkg|googleapis|gstatic|fontshare|cdn.tailwindcss'` over the fetched pages → empty).

- [ ] **Step 7: Browser dogfood (Dark/Light + Mobile + live + bulk).** In a real browser against the dev stack:
  - `/` ticks live; start/stop reflects; SSE refresh works.
  - `/woche` shows day bars + WOCHE GESAMT + Kennzahlen; KW nav works.
  - `/historie` calendar shows blocks; unassigned blocks stand out + banner; hybrid window expands for an early/late session; "Auswählen" → select blocks/day/week/all-unassigned → "Projekt zuweisen" (incl. inline ✚neu) assigns and the calendar refreshes via SSE; bulk-delete asks ConfirmDialog then deletes; single block edit dialog saves.
  - `/historie?view=list` paginates (‹ Seite X/Y ›).
  - Toggle Dark/Light persists; mobile (≤md) shows agenda + bottom-tab + sticky bulk bar.
  - No `window.confirm/alert/prompt` anywhere: `rg -n "window.(confirm|alert|prompt)" internal/adapter/webui` → empty.

- [ ] **Step 8: Final commit.**

```bash
git add cmd/flow-server/main.go internal/adapter/webui internal/i18n internal/adapter/webui/static/app.css
git commit -m "chore(webui): wire Slice 1 worktime usecases, drop old worktime page, gate green"
```

---

## Self-Review (completed during authoring)

- **Spec coverage:** §3.1 Bulk-Reassign → Task 3; §3.2 Bulk-Delete → Task 4; §3.3 Pagination → Tasks 1–2; §3.4 apiclient → Task 4; §4 components → Task 5; §5.1 Heute → Task 6; §5.2 Woche → Task 7; §5.3 Historie (calendar/month/agenda/list/bulk/single-edit) → Task 8; §6 SSE → Tasks 6–8 fragments; §8 states/dialogs/pagination → Tasks 5 (ConfirmDialog/EmptyState/Pagination usage) + 8; §9 testing + done-gate → every task + Task 9; main-wiring → Task 9. All covered.
- **Placeholder scan:** Backend, usecase, apiclient, store, and the small components (ProgressBar/PaceDots/SegToggle) carry complete code. Dense page/templ markup is specified by structure + the exact in-repo mockup file + the components to compose — concrete references, not "TBD". Render/handler tests carry full assertion code. Test harness helpers (`newSessionTestServer`, `newWebWorktimeServer`, `webGet`) are flagged to match the file's existing constructors — adapt names rather than invent.
- **Type consistency:** VM names and component signatures are defined once in Task 5's Interfaces block and reused verbatim in Tasks 6–8. `ErrNoSessions` defined once (Task 3) and reused (Task 4). `ListPage`/`ListSessionsPage`/`ReassignSessions`/`BulkDeleteSessions` signatures consistent across store→usecase→handler→apiclient.
- **Known adaptation points (call out to implementers):** exact test-server constructors and web-auth cookie helpers differ per `*_test.go` file — read the top of each before writing tests. `web/tailwind.css` must carry the calendar `.block*`/`.grid-lines`/`.now-line`/`.seg`/`.chk`/`--hour` rules (Task 5 Step 4 note) or the calendar won't render — verify in Task 8.
