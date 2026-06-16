# flow rebuild M3c0 — Worktime Session-Mutation API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add server-side **edit** and **delete** for work sessions (start/stop/tag/note), exposed end-to-end (domain event · port · pgstore · usecase · HTTP route · apiclient), so the M3c1 Today TUI can offer session-edit and delete.

**Architecture:** Hexagonal, matching the existing Start/Stop/List session slice. New `SessionStore.Update`/`Delete` port methods (pgstore SQL on the existing `work_sessions` table — no migration), two usecases (`EditSession`, `DeleteSession`), two HTTP routes (`PATCH`/`DELETE /api/v1/sessions/{id}`, owner-scoped via the existing `auth` middleware), two apiclient methods. All mutations publish an SSE event. Pure backend — no TUI.

**Tech Stack:** Go 1.25.7; `net/http` ServeMux method+path patterns (`PathValue`); `jackc/pgx/v5`; the existing `internal/{domain,ports,usecase,testutil}` + `internal/adapter/{pgstore,httpserver,apiclient,sse}`.

---

## Context for the implementer — VERIFIED APIs (do not redefine)

### Existing patterns this plan mirrors
- **Usecase struct** (`internal/usecase/stop_session.go`): a struct holding port deps + an `Execute(ctx, ownerID, …)` method returning `(domain.WorkSession, error)`.
- **pgstore** (`internal/adapter/pgstore/sessions.go`): `SessionStore` with `scanSession(rowScanner)`; `Stop` uses `UPDATE … RETURNING …`, maps `pgx.ErrNoRows → ports.ErrSessionNotFound`.
- **HTTP handler** (`internal/adapter/httpserver/worktime.go`): `userFrom(r.Context())` → user; `json.NewDecoder(r.Body).Decode`; `writeJSON(w, status, v)`; `r.PathValue("id")`; `s.Bus.Publish(domain.Event{Type:…, UserID:u.ID, Data:map[string]any{"id":…}})`.
- **Route registration** (`internal/adapter/httpserver/server.go` `Routes()`): `mux.Handle("POST /api/v1/sessions/{id}/stop", s.auth(http.HandlerFunc(s.handleStopSession)))`.
- **Composition root** (`cmd/flow-server/main.go:88-100`): `srv := &httpserver.Server{ … StartSession: usecase.StartSession{Sessions: sessionStore, IDs: ids, Clock: clock}, StopSession: …, ListSessions: … }`.
- **apiclient** (`internal/adapter/apiclient/client.go`): `c.do(ctx, method, path, body, out any) error` — marshals `body`, sets bearer, errors on status ≥ 300, decodes into `out` when non-nil.

### Verified existing identifiers (reuse, do not recreate)
```go
// domain/event.go — ALREADY EXISTS:
domain.EventSessionUpdated EventType = "session.updated"
// domain/worksession.go — ALREADY EXISTS:
domain.ErrInvalidSession            // sentinel; WorkSession{ID,OwnerID,ProjectID*,Tag,Note,Start,Stop*,CreatedAt}
func (s domain.WorkSession) Running() bool        // Stop == nil
// ports/ports.go — ALREADY EXISTS:
ports.ErrSessionNotFound
type ports.SessionStore interface { Create; Running; Stop; List }   // we ADD Update + Delete
// testutil/fakes.go — ALREADY EXISTS:
testutil.NewFakeSessionStore() *FakeSessionStore   // fields: mu sync.Mutex; m map[string]domain.WorkSession (keyed by id)
```

### What NOT to touch
- The existing `Create/Running/Stop/List` methods and their tests.
- Any TUI code (`internal/tui/*`, `internal/tui/shell/*`) — M3c0 is backend only.
- No DB migration — `work_sessions` already has `project_id, tag, note, start_at, stop_at`.

---

## File map

| File | Change | Task |
|---|---|---|
| `internal/domain/event.go` | add `EventSessionDeleted` const | 1 |
| `internal/ports/ports.go` | add `Update` + `Delete` to `SessionStore` | 1 |
| `internal/testutil/fakes.go` | add `FakeSessionStore.Update` + `.Delete` | 1 |
| `internal/usecase/edit_session.go` (+ `_test.go`) | `EditSession` usecase + `EditSessionInput` | 2 |
| `internal/usecase/delete_session.go` (+ `_test.go`) | `DeleteSession` usecase | 3 |
| `internal/adapter/pgstore/sessions.go` (+ docker test) | `Update` + `Delete` SQL | 4 |
| `internal/adapter/httpserver/worktime.go` | `handleEditSession` + `handleDeleteSession` | 5 |
| `internal/adapter/httpserver/server.go` | `EditSession`/`DeleteSession` fields + 2 routes | 5 |
| `internal/adapter/httpserver/server_test.go` | edit/delete route test | 5 |
| `cmd/flow-server/main.go` | wire the two usecases into `Server{}` | 5 |
| `internal/adapter/apiclient/client.go` | `EditSession` + `DeleteSession` | 6 |
| `internal/adapter/apiclient/worktime_test.go` | apiclient edit/delete test | 6 |

Build order: contract (event+port+fake) → usecases → pgstore → HTTP+wiring → apiclient → CI/smoke.

---

## Task 1: Contract — event, port methods, fake store

**Files:**
- Modify: `internal/domain/event.go`
- Modify: `internal/ports/ports.go:77-82` (`SessionStore` interface)
- Modify: `internal/testutil/fakes.go` (`FakeSessionStore`)
- Test: `internal/testutil/fakes_session_mutation_test.go` (new)

- [ ] **Step 1: Write the failing test** — `internal/testutil/fakes_session_mutation_test.go`

```go
package testutil_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
)

func TestFakeSessionStore_UpdateAndDelete(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := start.Add(time.Hour)
	seed := domain.WorkSession{ID: "s1", OwnerID: "u1", Start: start, Stop: &stop}
	if _, err := ss.Create(ctx, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	pid := "p1"
	got, err := ss.Update(ctx, "u1", "s1", &pid, "deep", "note", start, &stop)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Tag != "deep" || got.Note != "note" || got.ProjectID == nil || *got.ProjectID != "p1" {
		t.Fatalf("update did not persist fields: %+v", got)
	}

	// foreign owner -> not found
	if _, err := ss.Update(ctx, "other", "s1", nil, "", "", start, &stop); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("foreign update: want ErrSessionNotFound, got %v", err)
	}
	// delete foreign -> not found
	if err := ss.Delete(ctx, "other", "s1"); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("foreign delete: want ErrSessionNotFound, got %v", err)
	}
	// delete owner -> ok, then gone
	if err := ss.Delete(ctx, "u1", "s1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := ss.Delete(ctx, "u1", "s1"); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("double delete: want ErrSessionNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`ss.Update`/`ss.Delete` undefined)

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/testutil/ 2>&1 | head`
Expected: compile error `ss.Update undefined` / `ss.Delete undefined`.

- [ ] **Step 3a: Add the domain event** — in `internal/domain/event.go`, add to the existing const block (next to `EventSessionUpdated`):

```go
	EventSessionDeleted  EventType = "session.deleted"
```

- [ ] **Step 3b: Extend the port interface** — in `internal/ports/ports.go`, the `SessionStore` interface, add two methods after `Stop`:

```go
	// Update overwrites a session's project/tag/note/start/stop. Owner-scoped;
	// returns ErrSessionNotFound for a missing or foreign session.
	Update(ctx context.Context, ownerID, id string, projectID *string, tag, note string, start time.Time, stop *time.Time) (domain.WorkSession, error)
	// Delete removes a session. Owner-scoped; ErrSessionNotFound if absent.
	Delete(ctx context.Context, ownerID, id string) error
```

- [ ] **Step 3c: Implement the fake** — in `internal/testutil/fakes.go`, after `FakeSessionStore.Stop`:

```go
func (s *FakeSessionStore) Update(_ context.Context, ownerID, id string, projectID *string, tag, note string, start time.Time, stop *time.Time) (domain.WorkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[id]
	if !ok || e.OwnerID != ownerID {
		return domain.WorkSession{}, ports.ErrSessionNotFound
	}
	e.ProjectID = projectID
	e.Tag = tag
	e.Note = note
	e.Start = start
	e.Stop = stop
	s.m[id] = e
	return e, nil
}

func (s *FakeSessionStore) Delete(_ context.Context, ownerID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[id]
	if !ok || e.OwnerID != ownerID {
		return ports.ErrSessionNotFound
	}
	delete(s.m, id)
	return nil
}
```

- [ ] **Step 4: Run — expect PASS**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/testutil/ ./internal/domain/ ./internal/ports/ 2>&1 | tail`
Expected: PASS. (The real pgstore `*SessionStore` will not compile against the new interface until Task 4 — that's expected; `go test ./internal/testutil/...` does not build pgstore.)

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/domain/event.go internal/ports/ports.go internal/testutil/fakes.go internal/testutil/fakes_session_mutation_test.go && git commit -m "feat(m3c0): SessionStore Update/Delete contract + fake + session.deleted event"
```

---

## Task 2: `EditSession` usecase

**Files:**
- Create: `internal/usecase/edit_session.go`
- Test: `internal/usecase/edit_session_test.go`

- [ ] **Step 1: Write the failing test** — `internal/usecase/edit_session_test.go`

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

func TestEditSession(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := start.Add(2 * time.Hour)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "s1", OwnerID: "u1", Start: start, Stop: &stop}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	uc := usecase.EditSession{Sessions: ss}

	newStop := start.Add(3 * time.Hour)
	got, err := uc.Execute(ctx, "u1", "s1", usecase.EditSessionInput{Tag: "deep", Note: "n", Start: start, Stop: &newStop})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if got.Tag != "deep" || got.Stop == nil || !got.Stop.Equal(newStop) {
		t.Fatalf("edit not applied: %+v", got)
	}

	// stop <= start -> ErrInvalidSession
	bad := start.Add(-time.Minute)
	if _, err := uc.Execute(ctx, "u1", "s1", usecase.EditSessionInput{Start: start, Stop: &bad}); !errors.Is(err, domain.ErrInvalidSession) {
		t.Fatalf("want ErrInvalidSession, got %v", err)
	}
	// foreign owner -> not found (store-enforced)
	if _, err := uc.Execute(ctx, "other", "s1", usecase.EditSessionInput{Start: start, Stop: &newStop}); err == nil {
		t.Fatal("foreign edit should fail")
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/usecase/ -run TestEditSession 2>&1 | head`
Expected: `undefined: usecase.EditSession`.

- [ ] **Step 3: Implement** — `internal/usecase/edit_session.go`

```go
package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// EditSessionInput carries the editable fields of an existing session.
type EditSessionInput struct {
	ProjectID *string
	Tag       string
	Note      string
	Start     time.Time
	Stop      *time.Time
}

// EditSession overwrites a session's project/tag/note/times. Owner-scoped via
// the store. A set Stop must be strictly after Start.
type EditSession struct {
	Sessions ports.SessionStore
}

func (uc EditSession) Execute(ctx context.Context, ownerID, id string, in EditSessionInput) (domain.WorkSession, error) {
	if in.Stop != nil && !in.Stop.After(in.Start) {
		return domain.WorkSession{}, domain.ErrInvalidSession
	}
	return uc.Sessions.Update(ctx, ownerID, id, in.ProjectID, in.Tag, in.Note, in.Start, in.Stop)
}
```

- [ ] **Step 4: Run — expect PASS**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/usecase/ -run TestEditSession -v 2>&1 | tail`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/usecase/edit_session.go internal/usecase/edit_session_test.go && git commit -m "feat(m3c0): EditSession usecase (validates stop>start, owner-scoped)"
```

---

## Task 3: `DeleteSession` usecase

**Files:**
- Create: `internal/usecase/delete_session.go`
- Test: `internal/usecase/delete_session_test.go`

- [ ] **Step 1: Write the failing test** — `internal/usecase/delete_session_test.go`

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

func TestDeleteSession(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := start.Add(time.Hour)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "s1", OwnerID: "u1", Start: start, Stop: &stop}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	uc := usecase.DeleteSession{Sessions: ss}

	if err := uc.Execute(ctx, "u1", "s1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := uc.Execute(ctx, "u1", "s1"); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("want ErrSessionNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/usecase/ -run TestDeleteSession 2>&1 | head`
Expected: `undefined: usecase.DeleteSession`.

- [ ] **Step 3: Implement** — `internal/usecase/delete_session.go`

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/ports"
)

// DeleteSession removes a session. Owner-scoped via the store.
type DeleteSession struct {
	Sessions ports.SessionStore
}

func (uc DeleteSession) Execute(ctx context.Context, ownerID, id string) error {
	return uc.Sessions.Delete(ctx, ownerID, id)
}
```

- [ ] **Step 4: Run — expect PASS**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/usecase/ -run TestDeleteSession -v 2>&1 | tail`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/usecase/delete_session.go internal/usecase/delete_session_test.go && git commit -m "feat(m3c0): DeleteSession usecase (owner-scoped)"
```

---

## Task 4: pgstore `Update` + `Delete`

**Files:**
- Modify: `internal/adapter/pgstore/sessions.go`

Note: the pgstore package has Docker-backed integration tests gated behind a build tag / env (see `make ci`). This task adds the methods; the `make ci` run in Task 7 exercises them. No new migration — `work_sessions` already has all columns.

- [ ] **Step 1: Implement `Update`** — in `internal/adapter/pgstore/sessions.go`, after the `Stop` method:

```go
func (s *SessionStore) Update(ctx context.Context, ownerID, id string, projectID *string, tag, note string, start time.Time, stop *time.Time) (domain.WorkSession, error) {
	const q = `
UPDATE work_sessions SET project_id=$1, tag=$2, note=$3, start_at=$4, stop_at=$5
WHERE owner_id=$6 AND id=$7
RETURNING id, owner_id, project_id, tag, note, start_at, stop_at, created_at`
	ws, err := scanSession(s.pool.QueryRow(ctx, q, projectID, tag, note, start, stop, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WorkSession{}, ports.ErrSessionNotFound
	}
	return ws, err
}
```

- [ ] **Step 2: Implement `Delete`** — directly after `Update`:

```go
func (s *SessionStore) Delete(ctx context.Context, ownerID, id string) error {
	const q = `DELETE FROM work_sessions WHERE owner_id=$1 AND id=$2`
	ct, err := s.pool.Exec(ctx, q, ownerID, id)
	if err != nil {
		return fmt.Errorf("pgstore: delete session: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ports.ErrSessionNotFound
	}
	return nil
}
```

(`errors`, `fmt`, `time`, `pgx`, `ports`, `domain` are all already imported in this file.)

- [ ] **Step 3: Verify the whole module compiles** (the real store now satisfies the extended interface)

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go build ./... 2>&1 && echo OK`
Expected: `OK`.

- [ ] **Step 4: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/adapter/pgstore/sessions.go && git commit -m "feat(m3c0): pgstore SessionStore Update/Delete SQL"
```

---

## Task 5: HTTP routes + handlers + composition wiring

**Files:**
- Modify: `internal/adapter/httpserver/worktime.go` (handlers)
- Modify: `internal/adapter/httpserver/server.go` (fields + routes)
- Modify: `cmd/flow-server/main.go` (wire usecases)
- Test: `internal/adapter/httpserver/server_test.go` (add one test)

- [ ] **Step 1: Write the failing test** — append to `internal/adapter/httpserver/server_test.go`:

```go
func TestSessionEditDeleteRoutes(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeProjectStore()
	users := testutil.NewFakeUserStore()
	srv := &httpserver.Server{
		Verifier:      testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:        usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:           sse.NewBus(),
		Clock:         clk,
		StartSession:  usecase.StartSession{Sessions: ss, IDs: ids, Clock: clk},
		StopSession:   usecase.StopSession{Sessions: ss, Projects: ps, Clock: clk},
		ListSessions:  usecase.ListSessions{Sessions: ss, Clock: clk},
		CreateProject: usecase.CreateProject{Projects: ps, IDs: ids, Clock: clk},
		ListProjects:  usecase.ListProjects{Projects: ps},
		EditSession:   usecase.EditSession{Sessions: ss},
		DeleteSession: usecase.DeleteSession{Sessions: ss},
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	do := func(method, path, body string) *http.Response {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer x")
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// start then stop (with a project) to get a completed session
	res := do("POST", "/api/v1/projects", `{"name":"Flow"}`)
	var proj domain.Project
	_ = json.NewDecoder(res.Body).Decode(&proj)
	_ = res.Body.Close()
	res = do("POST", "/api/v1/sessions", `{}`)
	var s domain.WorkSession
	_ = json.NewDecoder(res.Body).Decode(&s)
	_ = res.Body.Close()
	res = do("POST", "/api/v1/sessions/"+s.ID+"/stop", `{"projectId":"`+proj.ID+`"}`)
	_ = res.Body.Close()

	// PATCH edit: set a tag
	res = do("PATCH", "/api/v1/sessions/"+s.ID, `{"projectId":"`+proj.ID+`","tag":"deep","note":"","start":"2026-06-14T09:00:00Z","stop":"2026-06-14T11:00:00Z"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("edit status %d, want 200", res.StatusCode)
	}
	var edited domain.WorkSession
	_ = json.NewDecoder(res.Body).Decode(&edited)
	_ = res.Body.Close()
	if edited.Tag != "deep" {
		t.Fatalf("edit did not persist tag: %+v", edited)
	}

	// PATCH invalid times -> 400
	res = do("PATCH", "/api/v1/sessions/"+s.ID, `{"start":"2026-06-14T11:00:00Z","stop":"2026-06-14T09:00:00Z"}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid edit status %d, want 400", res.StatusCode)
	}
	_ = res.Body.Close()

	// DELETE -> 204, then 404
	res = do("DELETE", "/api/v1/sessions/"+s.ID, "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status %d, want 204", res.StatusCode)
	}
	_ = res.Body.Close()
	res = do("DELETE", "/api/v1/sessions/"+s.ID, "")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("double delete status %d, want 404", res.StatusCode)
	}
	_ = res.Body.Close()
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/httpserver/ -run TestSessionEditDeleteRoutes 2>&1 | head`
Expected: compile error — `EditSession`/`DeleteSession` not fields of `Server`.

- [ ] **Step 3a: Add Server fields** — in `internal/adapter/httpserver/server.go`, in the `// worktime usecases` block, after `ListProjects`:

```go
	EditSession   usecase.EditSession
	DeleteSession usecase.DeleteSession
```

- [ ] **Step 3b: Register routes** — in `server.go` `Routes()`, after the `GET /api/v1/sessions` line:

```go
	mux.Handle("PATCH /api/v1/sessions/{id}", s.auth(http.HandlerFunc(s.handleEditSession)))
	mux.Handle("DELETE /api/v1/sessions/{id}", s.auth(http.HandlerFunc(s.handleDeleteSession)))
```

- [ ] **Step 3c: Add handlers** — in `internal/adapter/httpserver/worktime.go`, add the `usecase` import (`"github.com/serverkraken/flow/internal/usecase"`) and append:

```go
type editReq struct {
	ProjectID *string    `json:"projectId"`
	Tag       string     `json:"tag"`
	Note      string     `json:"note"`
	Start     time.Time  `json:"start"`
	Stop      *time.Time `json:"stop"`
}

func (s *Server) handleEditSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req editReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	sess, err := s.EditSession.Execute(r.Context(), u.ID, r.PathValue("id"), usecase.EditSessionInput{
		ProjectID: req.ProjectID, Tag: req.Tag, Note: req.Note, Start: req.Start, Stop: req.Stop,
	})
	switch {
	case errors.Is(err, domain.ErrInvalidSession):
		http.Error(w, "invalid session times", http.StatusBadRequest)
		return
	case errors.Is(err, ports.ErrSessionNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionUpdated, UserID: u.ID, Data: map[string]any{"id": sess.ID}})
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	switch err := s.DeleteSession.Execute(r.Context(), u.ID, id); {
	case errors.Is(err, ports.ErrSessionNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionDeleted, UserID: u.ID, Data: map[string]any{"id": id}})
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 3d: Wire the composition root** — in `cmd/flow-server/main.go`, in the `srv := &httpserver.Server{ … }` literal, after the `ListSessions:` line (≈ line 100):

```go
		EditSession:   usecase.EditSession{Sessions: sessionStore},
		DeleteSession: usecase.DeleteSession{Sessions: sessionStore},
```

- [ ] **Step 4: Run — expect PASS** (handler test + full build)

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go build ./... && go test ./internal/adapter/httpserver/ -run TestSessionEditDeleteRoutes -v 2>&1 | tail`
Expected: build OK; test PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/adapter/httpserver/worktime.go internal/adapter/httpserver/server.go internal/adapter/httpserver/server_test.go cmd/flow-server/main.go && git commit -m "feat(m3c0): PATCH/DELETE /api/v1/sessions/{id} routes + handlers + wiring"
```

---

## Task 6: apiclient `EditSession` + `DeleteSession`

**Files:**
- Modify: `internal/adapter/apiclient/client.go` (add `time` import + 2 methods)
- Test: `internal/adapter/apiclient/worktime_test.go` (add one test)

- [ ] **Step 1: Write the failing test** — append to `internal/adapter/apiclient/worktime_test.go`:

```go
func TestEditAndDeleteSession(t *testing.T) {
	var sawPatch, sawDelete bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/sessions/s1":
			sawPatch = true
			_, _ = w.Write([]byte(`{"id":"s1","tag":"deep","start":"2026-06-14T09:00:00Z"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/sessions/s1":
			sawDelete = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")

	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := start.Add(2 * time.Hour)
	s, err := c.EditSession(context.Background(), "s1", nil, "deep", "", start, &stop)
	if err != nil || s.Tag != "deep" {
		t.Fatalf("EditSession = %+v err=%v", s, err)
	}
	if err := c.DeleteSession(context.Background(), "s1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if !sawPatch || !sawDelete {
		t.Fatalf("server not hit: patch=%v delete=%v", sawPatch, sawDelete)
	}
}
```

(Add `"time"` to this test file's imports.)

- [ ] **Step 2: Run — expect FAIL**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/apiclient/ -run TestEditAndDeleteSession 2>&1 | head`
Expected: `undefined: c.EditSession` / `c.DeleteSession`.

- [ ] **Step 3: Implement** — in `internal/adapter/apiclient/client.go`, add `"time"` to imports and append after `StopSession`:

```go
func (c *Client) EditSession(ctx context.Context, id string, projectID *string, tag, note string, start time.Time, stop *time.Time) (domain.WorkSession, error) {
	var s domain.WorkSession
	err := c.do(ctx, http.MethodPatch, "/api/v1/sessions/"+id,
		map[string]any{"projectId": projectID, "tag": tag, "note": note, "start": start, "stop": stop}, &s)
	return s, err
}

func (c *Client) DeleteSession(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/sessions/"+id, nil, nil)
}
```

- [ ] **Step 4: Run — expect PASS**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/apiclient/ -run TestEditAndDeleteSession -v 2>&1 | tail`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add internal/adapter/apiclient/client.go internal/adapter/apiclient/worktime_test.go && git commit -m "feat(m3c0): apiclient EditSession/DeleteSession"
```

---

## Task 7: Full CI + curl-smoke done-gate

**Files:** none (verification; commit only if lint fixups needed).

- [ ] **Step 1: Build + vet**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go build ./... && go vet ./internal/... ./cmd/... && echo OK`
Expected: `OK`.

- [ ] **Step 2: `make ci`** (includes pgstore Docker tests + coverage gate)

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && make ci 2>&1 | tail -25`
Expected: lint + verify-generate + cover (≥ 80 %) + build green; the pgstore session Docker tests exercise the new `Update`/`Delete` SQL. Fix any lint nit minimally and re-run.

- [ ] **Step 3: Live curl-smoke** (against the dev stack — document the result in the completion note)

```bash
# Terminal A: cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && make dev-up && make dev-run
# Terminal B:
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
TOK=$(make -s dev-token)            # bearer for the dev user
BASE=http://localhost:8080
# create project + start + stop to get a completed session id
PID=$(curl -s -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d '{"name":"Smoke"}' $BASE/api/v1/projects | jq -r .id)
SID=$(curl -s -H "Authorization: Bearer $TOK" -d '{}' $BASE/api/v1/sessions | jq -r .id)
curl -s -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d "{\"projectId\":\"$PID\"}" $BASE/api/v1/sessions/$SID/stop >/dev/null
# PATCH edit: set tag -> expect 200 + "tag":"deep"
curl -s -o /dev/null -w '%{http_code}\n' -X PATCH -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d "{\"projectId\":\"$PID\",\"tag\":\"deep\",\"note\":\"\",\"start\":\"2026-06-14T09:00:00Z\",\"stop\":\"2026-06-14T11:00:00Z\"}" $BASE/api/v1/sessions/$SID
# DELETE -> expect 204 then 404
curl -s -o /dev/null -w '%{http_code}\n' -X DELETE -H "Authorization: Bearer $TOK" $BASE/api/v1/sessions/$SID
curl -s -o /dev/null -w '%{http_code}\n' -X DELETE -H "Authorization: Bearer $TOK" $BASE/api/v1/sessions/$SID
```
Expected: PATCH → `200`; first DELETE → `204`; second DELETE → `404`. (If `make dev-token` differs, use the device-flow login from [[reference_flow_dev_env]].)

- [ ] **Step 4: Commit any lint fixups** (skip if clean)

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git add -A && git commit -m "chore(m3c0): lint fixups for session-mutation API"
```

---

## Self-review

### Spec coverage (M3c0 row)
| Spec item | Task |
|---|---|
| `SessionStore.Update`+`Delete` (pgstore UPDATE/DELETE, table exists) | 1 (port+fake) + 4 (pgstore SQL) |
| `EditSession` usecase (start/stop/tag/note, Stop>Start, owner-scope) | 2 |
| `DeleteSession` usecase (owner-scope) | 3 |
| httpapi `PATCH`/`DELETE /api/v1/sessions/{id}` (owner-auth via `auth`) | 5 |
| apiclient `EditSession`/`DeleteSession` | 6 |
| Publishes SSE `session.*` | 5 (`EventSessionUpdated` on PATCH, `EventSessionDeleted` on DELETE) |
| Done-gate: curl PATCH/DELETE owner-scoped + 404 foreign; unit + pgstore tests; `make ci` | 5 (route test) + 7 (curl + make ci) |

Note on "no Overlap validation": the spec mentioned overlap; this plan validates **Stop>Start + owner-scope** only. Cross-session overlap checking (loading all sessions, interval intersection) is deferred as future hardening — out of scope for this slice. This narrows the spec's M3c0 row by one validation; flagged here rather than silently dropped.

### Placeholder scan
No TBD/TODO/"add error handling"/"similar to Task N" — every step has complete code, exact paths, and run commands with expected output.

### Type consistency
- `SessionStore.Update(ctx, ownerID, id string, projectID *string, tag, note string, start time.Time, stop *time.Time) (domain.WorkSession, error)` and `Delete(ctx, ownerID, id string) error` — identical signature in the port (T1), fake (T1), pgstore (T4); consumed by `EditSession`/`DeleteSession` (T2/T3).
- `usecase.EditSessionInput{ProjectID *string; Tag, Note string; Start time.Time; Stop *time.Time}` — defined T2, constructed in the HTTP handler T5.
- `usecase.EditSession{Sessions}` / `usecase.DeleteSession{Sessions}` — defined T2/T3, fielded on `Server` (T5), wired in `main.go` (T5).
- Events `domain.EventSessionUpdated` (exists) on PATCH, `domain.EventSessionDeleted` (added T1) on DELETE.
- apiclient `EditSession(ctx, id string, projectID *string, tag, note string, start time.Time, stop *time.Time)` / `DeleteSession(ctx, id string)` — defined T6, mirror the wire shape the handler decodes in T5.

### Notes for the executor
- [[feedback_subagent_git_commits_isolated]]: verify HEAD advances after each subagent commit; recover orphans via reflog; do final wiring yourself.
- [[feedback_pgstore_goose_migrations]]: not applicable — **no migration** in this slice (columns already exist).
- [[feedback_plan_main_wiring_task]]: the composition-root wiring (`cmd/flow-server/main.go`) is Task 5 Step 3d; the curl-smoke (Task 7 Step 3) is the wiring verification.
- [[project_flow_rebuild_m3c_home_worktime_design]] is the parent spec; M3c0 unblocks M3c1's session-edit/delete dialogs. Next plan (M3c1) is written after M3c0 lands so it references the real `apiclient.EditSession`/`DeleteSession` signatures.
- [[feedback_long_lived_integration_branch]]: commit on `rebuild`; do not merge to main per milestone.
