# flow Rebuild M1a — Worktime Live-Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Start a timer in the TUI and see it appear in the WebUI within ~1s (and the reverse), proving the server-authoritative live-sync loop on a real worktime vertical.

**Architecture:** Hexagonal, server-authoritative. New `Project`/`WorkSession` domain → pgstore (Postgres/goose) → usecases → REST+SSE handlers. Browser authenticates via real OIDC auth-code-flow + a signed session cookie; the TUI keeps the M0 `FLOW_TOKEN` bearer. Every session/project mutation publishes a `domain.Event` on the existing in-process `sse.Bus`; the WebUI re-fetches HTML fragments via HTMX-SSE, the TUI maps events to `tea.Msg`.

**Tech Stack:** Go 1.25, `pgx/v5` + `goose` (migrations), `coreos/go-oidc/v3` + `golang.org/x/oauth2` (auth), `golang-jwt/v5` (session cookie — already vendored), `a-h/templ` + HTMX + Tailwind v4 (WebUI), `charm.land/v2` Bubbletea/Lipgloss (TUI). Tests: testcontainers Postgres (`startPG` helper) + in-memory fakes. Module: `github.com/serverkraken/flow`.

**Branch:** Execute on the orphan `rebuild` branch (worktree `flow-rebuild`). Each task = one commit. `make ci` (lint + 80% coverage gate + build) must stay green.

**Spec:** `docs/superpowers/specs/2026-06-14-flow-rebuild-m1a-worktime-design.md`.

---

## File Structure

**New domain (pure types + rules):**
- `internal/domain/project.go` — `Project`, `ProjectStatus`, `NewProject`
- `internal/domain/worksession.go` — `WorkSession`, `Running()`, `Elapsed()`, `NewWorkSession`
- `internal/domain/errors.go` (modify) — add session/project sentinels
- `internal/domain/event.go` (modify) — add session/project event types, remove `EventPing`

**New ports + Postgres adapters:**
- `internal/ports/ports.go` (modify) — add `ProjectStore`, `SessionStore`, `ErrProjectNotFound`, `ErrSessionNotFound`
- `internal/adapter/pgstore/migrations/0002_project_worksession.sql`
- `internal/adapter/pgstore/projects.go` + `internal/adapter/pgstore/sessions.go`

**New usecases (one file each):**
- `internal/usecase/create_project.go`, `list_projects.go`, `start_session.go`, `stop_session.go`, `list_sessions.go`

**HTTP (REST + SSE + auth-code-flow):**
- `internal/adapter/httpserver/worktime.go` — session/project handlers
- `internal/adapter/httpserver/webauth.go` — login/callback/logout + `webAuth` + `authAny`
- `internal/adapter/httpserver/server.go` + `middleware.go` + `handlers.go` (modify)
- `internal/adapter/oidcauth/auth.go` — auth-code-flow (oauth2 + id_token verify)
- `internal/adapter/websession/cookie.go` — signed session cookie (golang-jwt)
- `internal/config/config.go` (modify) — `OIDCClientSecret`, `OIDCRedirectURL`, `SessionSecret`, `PublicBaseURL`

**Clients:**
- `internal/adapter/apiclient/client.go` (modify) — worktime methods
- `internal/adapter/apiclient/events.go` — SSE client

**TUI:**
- `internal/tui/model.go` — root model + SSE plumbing
- `internal/tui/worktime.go` — worktime screen
- `internal/tui/styles.go` — Tokyonight styles
- `cmd/flow/worktime.go` — `flow worktime` command

**WebUI:**
- `internal/adapter/webui/worktime.templ` + generated `worktime_templ.go`
- `internal/adapter/webui/handlers.go` — page + fragment handlers
- `internal/adapter/webui/static/app.css` (Tailwind build output, embedded)
- `web/tailwind.css` (Tailwind v4 source) + `Makefile` (modify) + CI

**Wiring + verification:**
- `cmd/flow-server/main.go` (modify) — wire all new stores/usecases/auth
- `internal/testutil/fakes.go` (modify) — `FakeProjectStore`, `FakeSessionStore`
- `scripts/smoke-m1a.sh` + `scripts/live-sync-check.sh`

---

## Task 1: Project + WorkSession domain

**Files:**
- Create: `internal/domain/project.go`
- Create: `internal/domain/worksession.go`
- Modify: `internal/domain/errors.go`
- Test: `internal/domain/project_test.go`, `internal/domain/worksession_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/domain/project_test.go`:

```go
package domain

import (
	"testing"
	"time"
)

func TestNewProjectDefaultsActive(t *testing.T) {
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	p, err := NewProject("p1", "u1", "Flow Rebuild", "flow-rebuild", now)
	if err != nil {
		t.Fatalf("NewProject: %v", err)
	}
	if p.Status != ProjectActive {
		t.Fatalf("status = %q, want active", p.Status)
	}
	if !p.CreatedAt.Equal(now) || !p.UpdatedAt.Equal(now) {
		t.Fatal("timestamps not set to now")
	}
}

func TestNewProjectValidates(t *testing.T) {
	now := time.Now()
	cases := map[string]struct{ id, owner, name, slug string }{
		"no id":    {"", "u1", "n", "s"},
		"no owner": {"p1", "", "n", "s"},
		"no name":  {"p1", "u1", "", "s"},
		"no slug":  {"p1", "u1", "n", ""},
	}
	for label, c := range cases {
		if _, err := NewProject(c.id, c.owner, c.name, c.slug, now); err == nil {
			t.Fatalf("%s: expected error", label)
		}
	}
}
```

`internal/domain/worksession_test.go`:

```go
package domain

import (
	"testing"
	"time"
)

func TestWorkSessionRunningWhenStopNil(t *testing.T) {
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	s, err := NewWorkSession("s1", "u1", nil, start)
	if err != nil {
		t.Fatalf("NewWorkSession: %v", err)
	}
	if !s.Running() {
		t.Fatal("a session with nil Stop must be running")
	}
	now := start.Add(90 * time.Minute)
	if got := s.Elapsed(now); got != 90*time.Minute {
		t.Fatalf("running elapsed = %v, want 90m", got)
	}
}

func TestWorkSessionElapsedWhenStopped(t *testing.T) {
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := start.Add(30 * time.Minute)
	s, _ := NewWorkSession("s1", "u1", nil, start)
	s.Stop = &stop
	if s.Running() {
		t.Fatal("stopped session must not be running")
	}
	if got := s.Elapsed(time.Now()); got != 30*time.Minute {
		t.Fatalf("stopped elapsed = %v, want 30m", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/domain/ -run 'Project|WorkSession' -v`
Expected: FAIL — `undefined: NewProject`, `undefined: NewWorkSession`.

- [ ] **Step 3: Implement the domain types**

`internal/domain/project.go`:

```go
package domain

import (
	"fmt"
	"time"
)

// ProjectStatus is the lifecycle state of a Project.
type ProjectStatus string

const (
	ProjectActive   ProjectStatus = "active"
	ProjectArchived ProjectStatus = "archived"
)

// Project is the First-Class hub work sessions book against. M1a uses a
// minimal field set; the heavier foundation fields (repos/paths/links/…)
// arrive in later migrations.
type Project struct {
	ID        string        `json:"id"`
	OwnerID   string        `json:"-"`
	Name      string        `json:"name"`
	Slug      string        `json:"slug"`
	Color     string        `json:"color"`
	Glyph     string        `json:"glyph"`
	Status    ProjectStatus `json:"status"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

// NewProject builds a validated, active Project stamped at now.
func NewProject(id, ownerID, name, slug string, now time.Time) (Project, error) {
	switch {
	case id == "":
		return Project{}, fmt.Errorf("%w: id required", ErrInvalidProject)
	case ownerID == "":
		return Project{}, fmt.Errorf("%w: owner required", ErrInvalidProject)
	case name == "":
		return Project{}, fmt.Errorf("%w: name required", ErrInvalidProject)
	case slug == "":
		return Project{}, fmt.Errorf("%w: slug required", ErrInvalidProject)
	}
	return Project{
		ID: id, OwnerID: ownerID, Name: name, Slug: slug,
		Status: ProjectActive, CreatedAt: now, UpdatedAt: now,
	}, nil
}
```

`internal/domain/worksession.go`:

```go
package domain

import (
	"fmt"
	"time"
)

// WorkSession is one work interval owned by a user. Stop == nil marks the
// single active timer. Elapsed is derived, never stored.
type WorkSession struct {
	ID        string     `json:"id"`
	OwnerID   string     `json:"-"`
	ProjectID *string    `json:"projectId,omitempty"`
	Tag       string     `json:"tag,omitempty"`
	Note      string     `json:"note,omitempty"`
	Start     time.Time  `json:"start"`
	Stop      *time.Time `json:"stop,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// Running reports whether this is the active (unstopped) timer.
func (s WorkSession) Running() bool { return s.Stop == nil }

// Elapsed returns the session duration. For a running session it measures
// against now; for a stopped one, stop-start (now is ignored).
func (s WorkSession) Elapsed(now time.Time) time.Duration {
	if s.Stop != nil {
		return s.Stop.Sub(s.Start)
	}
	return now.Sub(s.Start)
}

// NewWorkSession builds a validated, running session (Stop nil). projectID
// is optional at start and mandatory at stop (enforced in StopSession).
func NewWorkSession(id, ownerID string, projectID *string, start time.Time) (WorkSession, error) {
	switch {
	case id == "":
		return WorkSession{}, fmt.Errorf("%w: id required", ErrInvalidSession)
	case ownerID == "":
		return WorkSession{}, fmt.Errorf("%w: owner required", ErrInvalidSession)
	}
	return WorkSession{ID: id, OwnerID: ownerID, ProjectID: projectID, Start: start, CreatedAt: start}, nil
}
```

Modify `internal/domain/errors.go` to add the sentinels:

```go
// Package domain holds the pure value types and rules of flow.
// It must not import any I/O package.
package domain

import "errors"

var (
	ErrInvalidUser     = errors.New("invalid user")
	ErrInvalidProject  = errors.New("invalid project")
	ErrInvalidSession  = errors.New("invalid session")
	ErrAlreadyRunning  = errors.New("a session is already running")
	ErrNoActiveSession = errors.New("no active session")
	ErrStopBeforeStart = errors.New("stop must be after start")
	ErrProjectRequired = errors.New("a project is required to book the session")
)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/domain/ -v`
Expected: PASS (including the existing user tests).

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/domain/project.go internal/domain/worksession.go internal/domain/project_test.go internal/domain/worksession_test.go internal/domain/errors.go
git commit -m "feat(domain): Project + WorkSession types with running-timer semantics"
```

---

## Task 2: Ports + Postgres stores + migration 0002

**Files:**
- Modify: `internal/ports/ports.go`
- Create: `internal/adapter/pgstore/migrations/0002_project_worksession.sql`
- Create: `internal/adapter/pgstore/projects.go`, `internal/adapter/pgstore/sessions.go`
- Test: `internal/adapter/pgstore/worktime_test.go`

- [ ] **Step 1: Add the port interfaces**

Append to `internal/ports/ports.go` (after the existing `UserStore` block, keeping the existing `ErrUserNotFound`):

```go
var (
	ErrProjectNotFound = errors.New("project not found")
	ErrSessionNotFound = errors.New("session not found")
)

// ProjectStore persists projects. All reads are owner-scoped.
type ProjectStore interface {
	Create(ctx context.Context, p domain.Project) (domain.Project, error)
	List(ctx context.Context, ownerID string) ([]domain.Project, error)
	Get(ctx context.Context, ownerID, id string) (domain.Project, error)
}

// SessionStore persists work sessions. The DB enforces at most one running
// session per owner (partial unique index); Running returns it if present.
type SessionStore interface {
	Create(ctx context.Context, s domain.WorkSession) (domain.WorkSession, error)
	Running(ctx context.Context, ownerID string) (domain.WorkSession, bool, error)
	Stop(ctx context.Context, ownerID, id string, projectID *string, stop time.Time) (domain.WorkSession, error)
	List(ctx context.Context, ownerID string, since time.Time) ([]domain.WorkSession, error)
}
```

(The `errors`, `context`, `time`, and `domain` imports already exist in ports.go.)

- [ ] **Step 2: Write the failing store test**

`internal/adapter/pgstore/worktime_test.go` (same `pgstore_test` package — reuses `startPG`/`TestMain` from `users_test.go`):

```go
package pgstore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestSessionStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	// A session FKs to users(id) and projects(id), so seed a user + project.
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "Martin")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	projects := pgstore.NewProjectStore(pool)
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	p, _ := domain.NewProject("p1", "u1", "Flow", "flow", now)
	if _, err := projects.Create(ctx, p); err != nil {
		t.Fatalf("project create: %v", err)
	}

	sessions := pgstore.NewSessionStore(pool)
	// no running session initially
	if _, ok, err := sessions.Running(ctx, "u1"); err != nil || ok {
		t.Fatalf("expected no running session, got ok=%v err=%v", ok, err)
	}
	// start one
	s, _ := domain.NewWorkSession("s1", "u1", nil, now)
	if _, err := sessions.Create(ctx, s); err != nil {
		t.Fatalf("create: %v", err)
	}
	// a second running session for the same owner is rejected by the partial index
	s2, _ := domain.NewWorkSession("s2", "u1", nil, now.Add(time.Minute))
	if _, err := sessions.Create(ctx, s2); err == nil {
		t.Fatal("expected unique-violation for a second running session")
	}
	// Running returns s1
	got, ok, err := sessions.Running(ctx, "u1")
	if err != nil || !ok || got.ID != "s1" {
		t.Fatalf("Running = %+v ok=%v err=%v", got, ok, err)
	}
	// stop it, booked to p1
	pid := "p1"
	stopAt := now.Add(time.Hour)
	stopped, err := sessions.Stop(ctx, "u1", "s1", &pid, stopAt)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if stopped.Stop == nil || !stopped.Stop.Equal(stopAt) || stopped.ProjectID == nil || *stopped.ProjectID != "p1" {
		t.Fatalf("stop result wrong: %+v", stopped)
	}
	// now nothing running, and List returns the one stopped session
	if _, ok, _ := sessions.Running(ctx, "u1"); ok {
		t.Fatal("nothing should be running after stop")
	}
	list, err := sessions.List(ctx, "u1", now.Add(-24*time.Hour))
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %d sessions err=%v", len(list), err)
	}
	// stopping a missing session → ErrSessionNotFound
	if _, err := sessions.Stop(ctx, "u1", "nope", &pid, stopAt); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("want ErrSessionNotFound, got %v", err)
	}
}

func TestProjectStoreListOwnerScoped(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	users := pgstore.NewUserStore(pool)
	ua, _ := domain.NewUser("ua", "sa", "a", "a@x", "A")
	ub, _ := domain.NewUser("ub", "sb", "b", "b@x", "B")
	_, _ = users.UpsertBySub(ctx, ua)
	_, _ = users.UpsertBySub(ctx, ub)
	ps := pgstore.NewProjectStore(pool)
	now := time.Now()
	pa, _ := domain.NewProject("pa", "ua", "A proj", "a-proj", now)
	pb, _ := domain.NewProject("pb", "ub", "B proj", "b-proj", now)
	_, _ = ps.Create(ctx, pa)
	_, _ = ps.Create(ctx, pb)
	list, err := ps.List(ctx, "ua")
	if err != nil || len(list) != 1 || list[0].ID != "pa" {
		t.Fatalf("owner-scoped list failed: %+v err=%v", list, err)
	}
	if _, err := ps.Get(ctx, "ua", "pb"); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Fatalf("cross-owner Get must be ErrProjectNotFound, got %v", err)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/pgstore/ -run 'SessionStoreLifecycle|ProjectStoreList' -v`
Expected: FAIL — `undefined: pgstore.NewProjectStore` / `NewSessionStore` (or skip if docker unavailable).

- [ ] **Step 4: Write the migration**

`internal/adapter/pgstore/migrations/0002_project_worksession.sql`:

```sql
-- +goose Up
CREATE TABLE projects (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users(id),
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL,
    color      TEXT NOT NULL DEFAULT '',
    glyph      TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner_id, slug)
);

CREATE TABLE work_sessions (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users(id),
    project_id TEXT REFERENCES projects(id),
    tag        TEXT NOT NULL DEFAULT '',
    note       TEXT NOT NULL DEFAULT '',
    start_at   TIMESTAMPTZ NOT NULL,
    stop_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- at most one running (unstopped) session per owner
CREATE UNIQUE INDEX one_running_session_per_user
    ON work_sessions (owner_id) WHERE stop_at IS NULL;
CREATE INDEX work_sessions_owner_start
    ON work_sessions (owner_id, start_at DESC);

-- +goose Down
DROP TABLE work_sessions;
DROP TABLE projects;
```

- [ ] **Step 5: Implement the stores**

`internal/adapter/pgstore/projects.go`:

```go
package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type ProjectStore struct{ pool *pgxpool.Pool }

func NewProjectStore(pool *pgxpool.Pool) *ProjectStore { return &ProjectStore{pool: pool} }

func (s *ProjectStore) Create(ctx context.Context, p domain.Project) (domain.Project, error) {
	const q = `
INSERT INTO projects (id, owner_id, name, slug, color, glyph, status, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
RETURNING id, owner_id, name, slug, color, glyph, status, created_at, updated_at`
	return scanProject(s.pool.QueryRow(ctx, q,
		p.ID, p.OwnerID, p.Name, p.Slug, p.Color, p.Glyph, string(p.Status), p.CreatedAt, p.UpdatedAt))
}

func (s *ProjectStore) List(ctx context.Context, ownerID string) ([]domain.Project, error) {
	const q = `
SELECT id, owner_id, name, slug, color, glyph, status, created_at, updated_at
FROM projects WHERE owner_id=$1 ORDER BY name`
	rows, err := s.pool.Query(ctx, q, ownerID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list projects: %w", err)
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *ProjectStore) Get(ctx context.Context, ownerID, id string) (domain.Project, error) {
	const q = `
SELECT id, owner_id, name, slug, color, glyph, status, created_at, updated_at
FROM projects WHERE owner_id=$1 AND id=$2`
	p, err := scanProject(s.pool.QueryRow(ctx, q, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Project{}, ports.ErrProjectNotFound
	}
	return p, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanProject(r rowScanner) (domain.Project, error) {
	var p domain.Project
	var status string
	if err := r.Scan(&p.ID, &p.OwnerID, &p.Name, &p.Slug, &p.Color, &p.Glyph, &status, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Project{}, err
		}
		return domain.Project{}, fmt.Errorf("pgstore: scan project: %w", err)
	}
	p.Status = domain.ProjectStatus(status)
	return p, nil
}
```

`internal/adapter/pgstore/sessions.go`:

```go
package pgstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type SessionStore struct{ pool *pgxpool.Pool }

func NewSessionStore(pool *pgxpool.Pool) *SessionStore { return &SessionStore{pool: pool} }

func (s *SessionStore) Create(ctx context.Context, ws domain.WorkSession) (domain.WorkSession, error) {
	const q = `
INSERT INTO work_sessions (id, owner_id, project_id, tag, note, start_at, stop_at, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id, owner_id, project_id, tag, note, start_at, stop_at, created_at`
	return scanSession(s.pool.QueryRow(ctx, q,
		ws.ID, ws.OwnerID, ws.ProjectID, ws.Tag, ws.Note, ws.Start, ws.Stop, ws.CreatedAt))
}

func (s *SessionStore) Running(ctx context.Context, ownerID string) (domain.WorkSession, bool, error) {
	const q = `
SELECT id, owner_id, project_id, tag, note, start_at, stop_at, created_at
FROM work_sessions WHERE owner_id=$1 AND stop_at IS NULL`
	ws, err := scanSession(s.pool.QueryRow(ctx, q, ownerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WorkSession{}, false, nil
	}
	if err != nil {
		return domain.WorkSession{}, false, err
	}
	return ws, true, nil
}

func (s *SessionStore) Stop(ctx context.Context, ownerID, id string, projectID *string, stop time.Time) (domain.WorkSession, error) {
	const q = `
UPDATE work_sessions SET stop_at=$1, project_id=$2
WHERE owner_id=$3 AND id=$4 AND stop_at IS NULL
RETURNING id, owner_id, project_id, tag, note, start_at, stop_at, created_at`
	ws, err := scanSession(s.pool.QueryRow(ctx, q, stop, projectID, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WorkSession{}, ports.ErrSessionNotFound
	}
	return ws, err
}

func (s *SessionStore) List(ctx context.Context, ownerID string, since time.Time) ([]domain.WorkSession, error) {
	const q = `
SELECT id, owner_id, project_id, tag, note, start_at, stop_at, created_at
FROM work_sessions WHERE owner_id=$1 AND start_at >= $2
ORDER BY start_at DESC`
	rows, err := s.pool.Query(ctx, q, ownerID, since)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list sessions: %w", err)
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

func scanSession(r rowScanner) (domain.WorkSession, error) {
	var ws domain.WorkSession
	if err := r.Scan(&ws.ID, &ws.OwnerID, &ws.ProjectID, &ws.Tag, &ws.Note, &ws.Start, &ws.Stop, &ws.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WorkSession{}, err
		}
		return domain.WorkSession{}, fmt.Errorf("pgstore: scan session: %w", err)
	}
	return ws, nil
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/pgstore/ -v`
Expected: PASS (or `SKIP` lines if docker is unavailable — the suite skips gracefully, which is acceptable locally but must run green in CI where docker exists).

- [ ] **Step 7: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/ports/ports.go internal/adapter/pgstore/projects.go internal/adapter/pgstore/sessions.go internal/adapter/pgstore/migrations/0002_project_worksession.sql internal/adapter/pgstore/worktime_test.go
git commit -m "feat(pgstore): Project + WorkSession stores + migration 0002 (one-running-timer index)"
```

---

## Task 3: Worktime usecases

**Files:**
- Create: `internal/usecase/create_project.go`, `list_projects.go`, `start_session.go`, `stop_session.go`, `list_sessions.go`
- Modify: `internal/testutil/fakes.go` (add `FakeProjectStore`, `FakeSessionStore`)
- Test: `internal/usecase/worktime_test.go`

- [ ] **Step 1: Add fakes for the new stores**

Append to `internal/testutil/fakes.go`:

```go
// FakeProjectStore is an in-memory ports.ProjectStore.
type FakeProjectStore struct {
	mu sync.Mutex
	m  map[string]domain.Project // keyed by id
}

func NewFakeProjectStore() *FakeProjectStore { return &FakeProjectStore{m: map[string]domain.Project{}} }

func (s *FakeProjectStore) Create(_ context.Context, p domain.Project) (domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[p.ID] = p
	return p, nil
}

func (s *FakeProjectStore) List(_ context.Context, ownerID string) ([]domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Project
	for _, p := range s.m {
		if p.OwnerID == ownerID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *FakeProjectStore) Get(_ context.Context, ownerID, id string) (domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.m[id]
	if !ok || p.OwnerID != ownerID {
		return domain.Project{}, ports.ErrProjectNotFound
	}
	return p, nil
}

// FakeSessionStore is an in-memory ports.SessionStore enforcing one running
// session per owner, like the Postgres partial index.
type FakeSessionStore struct {
	mu sync.Mutex
	m  map[string]domain.WorkSession // keyed by id
}

func NewFakeSessionStore() *FakeSessionStore { return &FakeSessionStore{m: map[string]domain.WorkSession{}} }

func (s *FakeSessionStore) Create(_ context.Context, ws domain.WorkSession) (domain.WorkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ws.Stop == nil {
		for _, e := range s.m {
			if e.OwnerID == ws.OwnerID && e.Stop == nil {
				return domain.WorkSession{}, errors.New("fake: running session exists")
			}
		}
	}
	s.m[ws.ID] = ws
	return ws, nil
}

func (s *FakeSessionStore) Running(_ context.Context, ownerID string) (domain.WorkSession, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.m {
		if e.OwnerID == ownerID && e.Stop == nil {
			return e, true, nil
		}
	}
	return domain.WorkSession{}, false, nil
}

func (s *FakeSessionStore) Stop(_ context.Context, ownerID, id string, projectID *string, stop time.Time) (domain.WorkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[id]
	if !ok || e.OwnerID != ownerID || e.Stop != nil {
		return domain.WorkSession{}, ports.ErrSessionNotFound
	}
	e.Stop = &stop
	e.ProjectID = projectID
	s.m[id] = e
	return e, nil
}

func (s *FakeSessionStore) List(_ context.Context, ownerID string, since time.Time) ([]domain.WorkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.WorkSession
	for _, e := range s.m {
		if e.OwnerID == ownerID && !e.Start.Before(since) {
			out = append(out, e)
		}
	}
	return out, nil
}
```

Add `"errors"` and `"time"` to the import block of `fakes.go` if not present (`time` already is; add `errors`).

- [ ] **Step 2: Write the failing usecase test**

`internal/usecase/worktime_test.go`:

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

func TestStartStopBookingFlow(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	sessions := testutil.NewFakeSessionStore()
	projects := testutil.NewFakeProjectStore()

	start := usecase.StartSession{Sessions: sessions, IDs: ids, Clock: clk}
	stop := usecase.StopSession{Sessions: sessions, Projects: projects, Clock: clk}
	createProj := usecase.CreateProject{Projects: projects, IDs: ids, Clock: clk}

	// start with no project
	s, err := start.Execute(ctx, "u1", nil, "", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if !s.Running() {
		t.Fatal("started session must be running")
	}
	// a second start is rejected
	if _, err := start.Execute(ctx, "u1", nil, "", ""); !errors.Is(err, domain.ErrAlreadyRunning) {
		t.Fatalf("want ErrAlreadyRunning, got %v", err)
	}
	// stop without a project is rejected
	if _, err := stop.Execute(ctx, "u1", s.ID, nil); !errors.Is(err, domain.ErrProjectRequired) {
		t.Fatalf("want ErrProjectRequired, got %v", err)
	}
	// create a project, then stop booked to it
	p, err := createProj.Execute(ctx, "u1", "Flow", "", "", "")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if p.Slug != "flow" {
		t.Fatalf("slug not derived from name: %q", p.Slug)
	}
	clk.T = clk.T.Add(time.Hour)
	stop.Clock = clk
	stopped, err := stop.Execute(ctx, "u1", s.ID, &p.ID)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if stopped.Stop == nil || stopped.ProjectID == nil || *stopped.ProjectID != p.ID {
		t.Fatalf("stop result wrong: %+v", stopped)
	}
	// stop booked to an unknown project → not found
	s2, _ := start.Execute(ctx, "u1", nil, "", "")
	bad := "ghost"
	if _, err := stop.Execute(ctx, "u1", s2.ID, &bad); err == nil {
		t.Fatal("stop with unknown project must error")
	}
}

func TestListSessionsSince(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)}
	sessions := testutil.NewFakeSessionStore()
	list := usecase.ListSessions{Sessions: sessions, Clock: clk}
	old, _ := domain.NewWorkSession("old", "u1", nil, clk.T.Add(-48*time.Hour))
	oldStop := old.Start.Add(time.Hour)
	old.Stop = &oldStop
	_, _ = sessions.Create(ctx, old)
	got, err := list.Execute(ctx, "u1", clk.T.Add(-24*time.Hour))
	if err != nil || len(got) != 0 {
		t.Fatalf("since-filter failed: %d err=%v", len(got), err)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/usecase/ -run 'StartStop|ListSessionsSince' -v`
Expected: FAIL — `undefined: usecase.StartSession` etc.

- [ ] **Step 4: Implement the usecases**

`internal/usecase/start_session.go`:

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// StartSession begins the user's single running timer. projectID is optional
// at start; tag/note are optional annotations.
type StartSession struct {
	Sessions ports.SessionStore
	IDs      ports.IDGen
	Clock    ports.Clock
}

func (uc StartSession) Execute(ctx context.Context, ownerID string, projectID *string, tag, note string) (domain.WorkSession, error) {
	if _, running, err := uc.Sessions.Running(ctx, ownerID); err != nil {
		return domain.WorkSession{}, err
	} else if running {
		return domain.WorkSession{}, domain.ErrAlreadyRunning
	}
	s, err := domain.NewWorkSession(uc.IDs.NewID(), ownerID, projectID, uc.Clock.Now())
	if err != nil {
		return domain.WorkSession{}, err
	}
	s.Tag, s.Note = tag, note
	return uc.Sessions.Create(ctx, s)
}
```

`internal/usecase/stop_session.go`:

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// StopSession ends a running session and books it to a project. Booking is
// mandatory; the project must already exist (clients inline-create via
// CreateProject first, then pass the new id here).
type StopSession struct {
	Sessions ports.SessionStore
	Projects ports.ProjectStore
	Clock    ports.Clock
}

func (uc StopSession) Execute(ctx context.Context, ownerID, sessionID string, projectID *string) (domain.WorkSession, error) {
	if projectID == nil || *projectID == "" {
		return domain.WorkSession{}, domain.ErrProjectRequired
	}
	if _, err := uc.Projects.Get(ctx, ownerID, *projectID); err != nil {
		return domain.WorkSession{}, err // ErrProjectNotFound bubbles to a 404
	}
	return uc.Sessions.Stop(ctx, ownerID, sessionID, projectID, uc.Clock.Now())
}
```

`internal/usecase/list_sessions.go`:

```go
package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListSessions returns the user's sessions started at or after since,
// newest first.
type ListSessions struct {
	Sessions ports.SessionStore
	Clock    ports.Clock
}

func (uc ListSessions) Execute(ctx context.Context, ownerID string, since time.Time) ([]domain.WorkSession, error) {
	return uc.Sessions.List(ctx, ownerID, since)
}
```

`internal/usecase/create_project.go`:

```go
package usecase

import (
	"context"
	"regexp"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// CreateProject creates an owner-scoped project. When slug is empty it is
// derived from name.
type CreateProject struct {
	Projects ports.ProjectStore
	IDs      ports.IDGen
	Clock    ports.Clock
}

func (uc CreateProject) Execute(ctx context.Context, ownerID, name, slug, color, glyph string) (domain.Project, error) {
	if slug == "" {
		slug = Slugify(name)
	}
	p, err := domain.NewProject(uc.IDs.NewID(), ownerID, name, slug, uc.Clock.Now())
	if err != nil {
		return domain.Project{}, err
	}
	p.Color, p.Glyph = color, glyph
	return uc.Projects.Create(ctx, p)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify lowercases name and collapses non-alphanumerics to single hyphens.
func Slugify(name string) string {
	s := nonSlug.ReplaceAllString(strings.ToLower(name), "-")
	return strings.Trim(s, "-")
}
```

`internal/usecase/list_projects.go`:

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListProjects returns the user's projects, ordered by name.
type ListProjects struct {
	Projects ports.ProjectStore
}

func (uc ListProjects) Execute(ctx context.Context, ownerID string) ([]domain.Project, error) {
	return uc.Projects.List(ctx, ownerID)
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/usecase/ ./internal/testutil/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/usecase/start_session.go internal/usecase/stop_session.go internal/usecase/list_sessions.go internal/usecase/create_project.go internal/usecase/list_projects.go internal/testutil/fakes.go internal/usecase/worktime_test.go
git commit -m "feat(usecase): start/stop/list sessions + create/list projects (booking + slugify)"
```

---

## Task 4: REST handlers + SSE event types + wiring (API)

**Files:**
- Modify: `internal/domain/event.go` (add types, remove `EventPing`)
- Create: `internal/adapter/httpserver/worktime.go`
- Modify: `internal/adapter/httpserver/server.go`, `handlers.go`, `server_test.go`
- Modify: `cmd/flow-server/main.go` (wire the new usecases)

- [ ] **Step 1: Update event types**

Replace `internal/domain/event.go`'s const block (drop `EventPing`):

```go
const (
	EventSessionStarted EventType = "session.started"
	EventSessionStopped EventType = "session.stopped"
	EventSessionUpdated EventType = "session.updated"
	EventProjectCreated EventType = "project.created"
)
```

- [ ] **Step 2: Remove the dev ping handler + route**

In `internal/adapter/httpserver/handlers.go` delete `handleDebugPing` entirely. In `internal/adapter/httpserver/server.go` delete the `if s.Dev { mux.Handle("POST /api/v1/debug/ping", ...) }` block. In `internal/adapter/httpserver/server_test.go` delete the `debug/ping` sub-test. (The `Dev` field stays on `Server` for other dev-gating.)

- [ ] **Step 3: Write the failing handler test**

Append to `internal/adapter/httpserver/server_test.go` a worktime round-trip. It uses the existing `FakeVerifier` to inject an identity and the real fakes/usecases:

```go
func TestSessionStartStopRoutes(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeProjectStore()
	users := testutil.NewFakeUserStore()
	srv := &httpserver.Server{
		Verifier: testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:   usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:      sse.NewBus(),
		Clock:    clk,
		StartSession:  usecase.StartSession{Sessions: ss, IDs: ids, Clock: clk},
		StopSession:   usecase.StopSession{Sessions: ss, Projects: ps, Clock: clk},
		ListSessions:  usecase.ListSessions{Sessions: ss, Clock: clk},
		CreateProject: usecase.CreateProject{Projects: ps, IDs: ids, Clock: clk},
		ListProjects:  usecase.ListProjects{Projects: ps},
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

	// create a project
	res := do("POST", "/api/v1/projects", `{"name":"Flow"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create project status %d", res.StatusCode)
	}
	var proj domain.Project
	_ = json.NewDecoder(res.Body).Decode(&proj)
	res.Body.Close()

	// start a session
	res = do("POST", "/api/v1/sessions", `{}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("start status %d", res.StatusCode)
	}
	var s domain.WorkSession
	_ = json.NewDecoder(res.Body).Decode(&s)
	res.Body.Close()

	// a second start → 409
	res = do("POST", "/api/v1/sessions", `{}`)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("double start status %d, want 409", res.StatusCode)
	}
	res.Body.Close()

	// stop without a project → 400
	res = do("POST", "/api/v1/sessions/"+s.ID+"/stop", `{}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("stop-no-project status %d, want 400", res.StatusCode)
	}
	res.Body.Close()

	// stop booked to the project → 200
	res = do("POST", "/api/v1/sessions/"+s.ID+"/stop", `{"projectId":"`+proj.ID+`"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("stop status %d, want 200", res.StatusCode)
	}
	res.Body.Close()
}
```

Add imports to the test file as needed: `encoding/json`, `net/http`, `net/http/httptest`, `strings`, `time`, and the `domain`, `ports`, `sse`, `testutil`, `usecase` packages.

- [ ] **Step 4: Run the test to verify it fails**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/httpserver/ -run Session -v`
Expected: FAIL — `Server` has no field `StartSession`, no route registered.

- [ ] **Step 5: Extend the Server struct + routes**

Replace `internal/adapter/httpserver/server.go`:

```go
// Package httpserver exposes the REST + SSE API and the WebUI auth flow.
package httpserver

import (
	"net/http"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

type Server struct {
	Verifier ports.TokenVerifier
	Ensure   usecase.EnsureUser
	Bus      ports.EventBus
	Clock    ports.Clock
	Dev      bool

	// worktime usecases
	StartSession  usecase.StartSession
	StopSession   usecase.StopSession
	ListSessions  usecase.ListSessions
	CreateProject usecase.CreateProject
	ListProjects  usecase.ListProjects

	// WebUI auth (wired in Task 5)
	OIDCAuth Authenticator
	Session  SessionCodec
	Users    ports.UserStore
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.Handle("GET /api/v1/me", s.auth(http.HandlerFunc(s.handleMe)))
	mux.Handle("GET /api/v1/events", s.authAny(http.HandlerFunc(s.handleEvents)))

	mux.Handle("POST /api/v1/sessions", s.auth(http.HandlerFunc(s.handleStartSession)))
	mux.Handle("POST /api/v1/sessions/{id}/stop", s.auth(http.HandlerFunc(s.handleStopSession)))
	mux.Handle("GET /api/v1/sessions", s.auth(http.HandlerFunc(s.handleListSessions)))
	mux.Handle("POST /api/v1/projects", s.auth(http.HandlerFunc(s.handleCreateProject)))
	mux.Handle("GET /api/v1/projects", s.auth(http.HandlerFunc(s.handleListProjects)))

	// WebUI auth routes (handlers in webauth.go, Task 5)
	mux.HandleFunc("GET /auth/login", s.handleLogin)
	mux.HandleFunc("GET /auth/callback", s.handleCallback)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)
	return mux
}
```

Note: `authAny`, `Authenticator`, `SessionCodec`, and the `/auth/*` handlers are added in Task 5. To keep Task 4 compiling on its own, temporarily alias `authAny` to `auth` and stub the auth handlers — **OR** sequence Task 5 immediately after and only run the full suite once. Recommended: define the stubs now so each task builds:

In `internal/adapter/httpserver/webauth.go` (created now, fleshed out in Task 5):

```go
package httpserver

import (
	"context"
	"net/http"

	"github.com/serverkraken/flow/internal/ports"
)

// Authenticator is the OIDC auth-code-flow port (oidcauth.Authenticator).
type Authenticator interface {
	AuthCodeURL(state string) string
	Exchange(ctx context.Context, code string) (ports.Identity, error)
}

// SessionCodec issues/parses the signed browser session cookie value.
type SessionCodec interface {
	Issue(userID string) (string, error)
	Parse(token string) (string, error)
}

// authAny accepts either a bearer token (TUI) or a session cookie (browser).
// Fully implemented in Task 5; for now it falls back to bearer-only.
func (s *Server) authAny(next http.Handler) http.Handler { return s.auth(next) }

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request)    { http.Error(w, "not implemented", http.StatusNotImplemented) }
func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) { http.Error(w, "not implemented", http.StatusNotImplemented) }
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request)   { http.Error(w, "not implemented", http.StatusNotImplemented) }
```

- [ ] **Step 6: Implement the worktime handlers**

`internal/adapter/httpserver/worktime.go`:

```go
package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type startReq struct {
	ProjectID *string `json:"projectId"`
	Tag       string  `json:"tag"`
	Note      string  `json:"note"`
}

func (s *Server) handleStartSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req startReq
	_ = json.NewDecoder(r.Body).Decode(&req) // empty body is valid (start w/o project)
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

type stopReq struct {
	ProjectID *string `json:"projectId"`
}

func (s *Server) handleStopSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req stopReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	sess, err := s.StopSession.Execute(r.Context(), u.ID, r.PathValue("id"), req.ProjectID)
	switch {
	case errors.Is(err, domain.ErrProjectRequired):
		http.Error(w, "a project is required", http.StatusBadRequest)
		return
	case errors.Is(err, ports.ErrProjectNotFound) || errors.Is(err, ports.ErrSessionNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionStopped, UserID: u.ID, Data: map[string]any{"id": sess.ID}})
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	since := startOfDay(s.Clock.Now())
	if q := r.URL.Query().Get("since"); q != "" {
		if t, err := time.Parse(time.RFC3339, q); err == nil {
			since = t
		}
	}
	list, err := s.ListSessions.Execute(r.Context(), u.ID, since)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []domain.WorkSession{}
	}
	writeJSON(w, http.StatusOK, list)
}

type createProjReq struct {
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Color string `json:"color"`
	Glyph string `json:"glyph"`
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req createProjReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	p, err := s.CreateProject.Execute(r.Context(), u.ID, req.Name, req.Slug, req.Color, req.Glyph)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventProjectCreated, UserID: u.ID, Data: map[string]any{"id": p.ID}})
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	list, err := s.ListProjects.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []domain.Project{}
	}
	writeJSON(w, http.StatusOK, list)
}

// startOfDay truncates t to local midnight.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
```

- [ ] **Step 7: Wire the usecases in main.go**

In `cmd/flow-server/main.go`, after building `pool`, construct the stores once and wire the server. Replace the `srv := &httpserver.Server{...}` block:

```go
	userStore := pgstore.NewUserStore(pool)
	projectStore := pgstore.NewProjectStore(pool)
	sessionStore := pgstore.NewSessionStore(pool)
	clock := systemclock.Clock{}
	ids := uuidgen.Gen{}

	srv := &httpserver.Server{
		Verifier: verifier,
		Ensure: usecase.EnsureUser{
			Users: userStore,
			IDs:   ids,
			Allow: func(id ports.Identity) bool { return cfg.AllowedSubs[id.Username] || cfg.AllowedSubs[id.Subject] },
		},
		Bus:           sse.NewBus(),
		Clock:         clock,
		Dev:           cfg.Dev,
		StartSession:  usecase.StartSession{Sessions: sessionStore, IDs: ids, Clock: clock},
		StopSession:   usecase.StopSession{Sessions: sessionStore, Projects: projectStore, Clock: clock},
		ListSessions:  usecase.ListSessions{Sessions: sessionStore, Clock: clock},
		CreateProject: usecase.CreateProject{Projects: projectStore, IDs: ids, Clock: clock},
		ListProjects:  usecase.ListProjects{Projects: projectStore},
		Users:         userStore,
		// OIDCAuth + Session wired in Task 5
	}
```

Add `"github.com/serverkraken/flow/internal/adapter/systemclock"` to the imports. (`systemclock.Clock{}` is the M0 clock adapter — confirm its type name with `cat internal/adapter/systemclock/*.go`; adjust if the exported type differs.)

- [ ] **Step 8: Run the suite**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go build ./... && go test ./internal/adapter/httpserver/ ./internal/domain/ -v`
Expected: PASS; `go build ./...` clean.

- [ ] **Step 9: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/domain/event.go internal/adapter/httpserver/ cmd/flow-server/main.go
git commit -m "feat(api): session/project REST + SSE events; drop dev debug/ping; wire usecases"
```

---

## Task 5: OIDC auth-code-flow + session cookie + cookie-or-bearer SSE

**Files:**
- Modify: `internal/config/config.go`, `config_test.go`
- Modify: `internal/ports/ports.go` (add `UserStore.GetByID`), `internal/adapter/pgstore/users.go`, `internal/testutil/fakes.go`
- Create: `internal/adapter/oidcauth/auth.go` (+ `auth_test.go`)
- Create: `internal/adapter/websession/cookie.go` (+ `cookie_test.go`)
- Modify: `internal/adapter/httpserver/webauth.go` (flesh out the Task-4 stub), `middleware.go`
- Create: `internal/adapter/httpserver/webauth_test.go`
- Modify: `cmd/flow-server/main.go`

- [ ] **Step 1: Add the new config fields (failing test first)**

Update `internal/config/config_test.go` — every existing test now must supply the new required vars. Replace the three test bodies' env maps to include them, e.g. `TestLoadFromEnv`:

```go
	env := map[string]string{
		"DATABASE_URL":            "postgres://flow:flow@localhost:5432/flow?sslmode=disable",
		"FLOW_OIDC_ISSUER":        "https://id.thebackend.org/application/o/flow/",
		"FLOW_OIDC_CLIENT_ID":     "flow",
		"FLOW_OIDC_CLIENT_SECRET": "shh",
		"FLOW_PUBLIC_BASE_URL":    "https://flow.thebackend.org",
		"FLOW_SESSION_SECRET":     "0123456789abcdef0123456789abcdef",
		"FLOW_ALLOWED_SUBS":       "msoent, alice",
		"FLOW_LISTEN_ADDR":        ":8080",
		"FLOW_DEV":                "1",
	}
```

Add an assertion after the existing ones:

```go
	if c.OIDCClientSecret != "shh" || c.SessionSecret == "" {
		t.Fatal("auth-code config not parsed")
	}
	if got := c.RedirectURL(); got != "https://flow.thebackend.org/auth/callback" {
		t.Fatalf("RedirectURL = %q", got)
	}
```

In `TestLoadDefaultsListenAddr` and `TestLoadMissingRequired`'s success path, also add `FLOW_OIDC_CLIENT_SECRET`, `FLOW_PUBLIC_BASE_URL`, `FLOW_SESSION_SECRET` so they still load. (`TestLoadMissingRequired` keeps passing — it sends all-empty and still expects an error.)

- [ ] **Step 2: Run config test to verify it fails**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/config/ -v`
Expected: FAIL — `c.OIDCClientSecret` undefined, `RedirectURL` undefined.

- [ ] **Step 3: Extend config**

In `internal/config/config.go`, add fields to `Config`:

```go
	OIDCClientSecret string
	PublicBaseURL    string
	SessionSecret    string
```

In `Load`, read them:

```go
		OIDCClientSecret: getenv("FLOW_OIDC_CLIENT_SECRET"),
		PublicBaseURL:    getenv("FLOW_PUBLIC_BASE_URL"),
		SessionSecret:    getenv("FLOW_SESSION_SECRET"),
```

Add them to the required-fields loop:

```go
		{"FLOW_OIDC_CLIENT_SECRET", c.OIDCClientSecret},
		{"FLOW_PUBLIC_BASE_URL", c.PublicBaseURL},
		{"FLOW_SESSION_SECRET", c.SessionSecret},
```

Add the derived redirect URL (uses `strings`, already imported):

```go
// RedirectURL is the OIDC auth-code callback, derived from the public base URL.
func (c Config) RedirectURL() string {
	return strings.TrimRight(c.PublicBaseURL, "/") + "/auth/callback"
}
```

Run: `go test ./internal/config/ -v` → PASS.

- [ ] **Step 4: Add UserStore.GetByID**

In `internal/ports/ports.go`, add to the `UserStore` interface:

```go
	GetByID(ctx context.Context, id string) (domain.User, error)
```

In `internal/adapter/pgstore/users.go`, add:

```go
func (s *UserStore) GetByID(ctx context.Context, id string) (domain.User, error) {
	const q = `SELECT id, oidc_sub, username, email, display_name FROM users WHERE id=$1`
	var out domain.User
	err := s.pool.QueryRow(ctx, q, id).Scan(&out.ID, &out.OIDCSub, &out.Username, &out.Email, &out.DisplayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ports.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("pgstore: get user by id: %w", err)
	}
	return out, nil
}
```

In `internal/testutil/fakes.go`, add to `FakeUserStore`:

```go
func (s *FakeUserStore) GetByID(_ context.Context, id string) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.bySub {
		if u.ID == id {
			return u, nil
		}
	}
	return domain.User{}, ports.ErrUserNotFound
}
```

- [ ] **Step 5: Session cookie codec (TDD)**

`internal/adapter/websession/cookie_test.go`:

```go
package websession

import (
	"testing"
	"time"
)

func TestIssueParseRoundTrip(t *testing.T) {
	c := NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	tok, err := c.Issue("user-42")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	got, err := c.Parse(tok)
	if err != nil || got != "user-42" {
		t.Fatalf("parse = %q err=%v", got, err)
	}
}

func TestParseRejectsForeignSecret(t *testing.T) {
	a := NewCodec("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", time.Hour)
	b := NewCodec("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", time.Hour)
	tok, _ := a.Issue("u")
	if _, err := b.Parse(tok); err == nil {
		t.Fatal("a token signed by A must not verify under B")
	}
}
```

`internal/adapter/websession/cookie.go`:

```go
// Package websession issues and parses the signed browser session cookie.
package websession

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Codec signs/verifies a session value (an HS256 JWT carrying the user id).
type Codec struct {
	secret []byte
	ttl    time.Duration
}

func NewCodec(secret string, ttl time.Duration) *Codec {
	return &Codec{secret: []byte(secret), ttl: ttl}
}

// Issue returns a signed cookie value for userID, valid for the codec's TTL.
func (c *Codec) Issue(userID string) (string, error) {
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(c.ttl)),
	})
	s, err := tok.SignedString(c.secret)
	if err != nil {
		return "", fmt.Errorf("websession: sign: %w", err)
	}
	return s, nil
}

// Parse verifies the value and returns the user id, or an error when the
// signature/expiry is invalid.
func (c *Codec) Parse(raw string) (string, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return c.secret, nil
	})
	if err != nil {
		return "", fmt.Errorf("websession: parse: %w", err)
	}
	return claims.Subject, nil
}
```

Run: `go test ./internal/adapter/websession/ -v` → PASS.

- [ ] **Step 6: OIDC auth-code adapter**

`internal/adapter/oidcauth/auth.go`:

```go
// Package oidcauth implements the OIDC authorization-code flow for the WebUI.
package oidcauth

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/serverkraken/flow/internal/ports"
)

// Authenticator drives the auth-code flow against Authentik and verifies the
// returned id_token.
type Authenticator struct {
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
}

func New(ctx context.Context, issuer, clientID, clientSecret, redirectURL string) (*Authenticator, error) {
	p, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidcauth: provider: %w", err)
	}
	return &Authenticator{
		oauth: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     p.Endpoint(),
			RedirectURL:  redirectURL,
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
		verifier: p.Verifier(&oidc.Config{ClientID: clientID}),
	}, nil
}

func (a *Authenticator) AuthCodeURL(state string) string { return a.oauth.AuthCodeURL(state) }

func (a *Authenticator) Exchange(ctx context.Context, code string) (ports.Identity, error) {
	tok, err := a.oauth.Exchange(ctx, code)
	if err != nil {
		return ports.Identity{}, fmt.Errorf("oidcauth: exchange: %w", err)
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok {
		return ports.Identity{}, fmt.Errorf("oidcauth: no id_token in token response")
	}
	idt, err := a.verifier.Verify(ctx, raw)
	if err != nil {
		return ports.Identity{}, fmt.Errorf("oidcauth: verify id_token: %w", err)
	}
	var c struct {
		Sub               string   `json:"sub"`
		PreferredUsername string   `json:"preferred_username"`
		Email             string   `json:"email"`
		Name              string   `json:"name"`
		Groups            []string `json:"groups"`
	}
	if err := idt.Claims(&c); err != nil {
		return ports.Identity{}, fmt.Errorf("oidcauth: claims: %w", err)
	}
	return ports.Identity{Subject: c.Sub, Username: c.PreferredUsername, Email: c.Email, Name: c.Name, Groups: c.Groups}, nil
}
```

`internal/adapter/oidcauth/auth_test.go` — covers `New` + `AuthCodeURL` against a mock discovery document (mirror the pattern in `internal/adapter/oidcverify/verifier_test.go`):

```go
package oidcauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewAndAuthCodeURL(t *testing.T) {
	mux := http.NewServeMux()
	var issuer string
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 issuer,
			"authorization_endpoint": issuer + "/authorize",
			"token_endpoint":         issuer + "/token",
			"jwks_uri":               issuer + "/jwks",
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	issuer = ts.URL

	a, err := New(context.Background(), issuer, "flow", "secret", "https://app/auth/callback")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	url := a.AuthCodeURL("xyz")
	if !strings.Contains(url, "/authorize") || !strings.Contains(url, "state=xyz") || !strings.Contains(url, "client_id=flow") {
		t.Fatalf("AuthCodeURL malformed: %s", url)
	}
}
```

> Coverage note: `Exchange` is exercised end-to-end by the live smoke in Task 9. If `make cover` dips below the 80% gate after this task, extend `auth_test.go` with a `/token` endpoint returning a signed `id_token` plus a `/jwks` stub — copy the RSA-key + JWKS scaffolding from `oidcverify/verifier_test.go`.

- [ ] **Step 7: Flesh out webauth.go + middleware**

Replace `internal/adapter/httpserver/webauth.go` (the Task-4 stub) with the real flow:

```go
package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

const (
	sessionCookie = "flow_session"
	stateCookie   = "flow_oidc_state"
)

// Authenticator is the OIDC auth-code-flow port (oidcauth.Authenticator).
type Authenticator interface {
	AuthCodeURL(state string) string
	Exchange(ctx context.Context, code string) (ports.Identity, error)
}

// SessionCodec issues/parses the signed browser session cookie value.
type SessionCodec interface {
	Issue(userID string) (string, error)
	Parse(token string) (string, error)
}

func randToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	state := randToken()
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: state, Path: "/", HttpOnly: true,
		Secure: !s.Dev, SameSite: http.SameSiteLaxMode, MaxAge: 600,
	})
	http.Redirect(w, r, s.OIDCAuth.AuthCodeURL(state), http.StatusFound)
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	st, err := r.Cookie(stateCookie)
	if err != nil || st.Value == "" || st.Value != r.URL.Query().Get("state") {
		http.Error(w, "bad state", http.StatusBadRequest)
		return
	}
	id, err := s.OIDCAuth.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "auth failed", http.StatusUnauthorized)
		return
	}
	u, err := s.Ensure.Execute(r.Context(), id)
	if errors.Is(err, usecase.ErrNotAllowed) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	val, err := s.Session.Issue(u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: val, Path: "/", HttpOnly: true,
		Secure: !s.Dev, SameSite: http.SameSiteLaxMode, MaxAge: int((7 * 24 * time.Hour).Seconds()),
	})
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusFound)
}

// resolveCookie loads the user from a valid session cookie.
func (s *Server) resolveCookie(r *http.Request) (domain.User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return domain.User{}, false
	}
	uid, err := s.Session.Parse(c.Value)
	if err != nil {
		return domain.User{}, false
	}
	u, err := s.Users.GetByID(r.Context(), uid)
	if err != nil {
		return domain.User{}, false
	}
	return u, true
}

// webAuth gates WebUI pages on a session cookie, redirecting to login.
func (s *Server) webAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.resolveCookie(r)
		if !ok {
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

// authAny accepts a bearer token (TUI) OR a session cookie (browser). Used by
// the SSE endpoint, which the browser EventSource reaches without an
// Authorization header.
func (s *Server) authAny(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := s.resolveBearer(r); ok {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
			return
		}
		if u, ok := s.resolveCookie(r); ok {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}
```

Add `resolveBearer` to `internal/adapter/httpserver/middleware.go` (a soft, ok-style bearer resolver used by `authAny`; the existing `auth` stays unchanged):

```go
// resolveBearer verifies a bearer token and ensures the user. Returns
// ok=false on any failure (used by authAny, which then tries the cookie).
func (s *Server) resolveBearer(r *http.Request) (domain.User, bool) {
	raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if raw == "" || raw == r.Header.Get("Authorization") {
		return domain.User{}, false
	}
	id, err := s.Verifier.Verify(r.Context(), raw)
	if err != nil {
		return domain.User{}, false
	}
	u, err := s.Ensure.Execute(r.Context(), id)
	if err != nil {
		return domain.User{}, false
	}
	return u, true
}
```

- [ ] **Step 8: Write the webauth flow test**

`internal/adapter/httpserver/webauth_test.go`:

```go
package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

type fakeAuth struct {
	url string
	id  ports.Identity
}

func (f fakeAuth) AuthCodeURL(string) string                                { return f.url }
func (f fakeAuth) Exchange(context.Context, string) (ports.Identity, error) { return f.id, nil }

func TestAuthCodeFlowSetsSessionCookie(t *testing.T) {
	users := testutil.NewFakeUserStore()
	srv := &httpserver.Server{
		Ensure:   usecase.EnsureUser{Users: users, IDs: &testutil.FakeIDGen{}, Allow: func(ports.Identity) bool { return true }},
		Bus:      sse.NewBus(),
		Users:    users,
		OIDCAuth: fakeAuth{url: "https://id/authorize?state=", id: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Session:  websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour),
		Dev:      true,
	}
	// no-redirect client so we can inspect Set-Cookie + Location
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// login → 302 + state cookie
	res, err := client.Get(ts.URL + "/auth/login")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusFound {
		t.Fatalf("login status %d", res.StatusCode)
	}
	var state *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "flow_oidc_state" {
			state = c
		}
	}
	if state == nil {
		t.Fatal("no state cookie set on login")
	}

	// callback with matching state → 302 to / + session cookie
	req, _ := http.NewRequest("GET", ts.URL+"/auth/callback?code=abc&state="+state.Value, nil)
	req.AddCookie(state)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/" {
		t.Fatalf("callback status %d loc %q", res.StatusCode, res.Header.Get("Location"))
	}
	var session *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "flow_session" {
			session = c
		}
	}
	if session == nil || session.Value == "" {
		t.Fatal("no session cookie set on callback")
	}

	// the session cookie authenticates the SSE endpoint (authAny cookie path)
	req, _ = http.NewRequest("GET", ts.URL+"/api/v1/events", nil)
	req.AddCookie(session)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	res, err = http.DefaultClient.Do(req.WithContext(ctx))
	if err == nil && res.StatusCode != http.StatusOK {
		t.Fatalf("SSE with session cookie status %d, want 200", res.StatusCode)
	}
}
```

- [ ] **Step 9: Wire auth into main.go**

In `cmd/flow-server/main.go` `run()`, after the `verifier` is built, add:

```go
	authn, err := oidcauth.New(ctx, cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCClientSecret, cfg.RedirectURL())
	if err != nil {
		return err
	}
```

and set the two fields on `srv`:

```go
		OIDCAuth: authn,
		Session:  websession.NewCodec(cfg.SessionSecret, 7*24*time.Hour),
```

Add imports: `"time"` (already present), `"github.com/serverkraken/flow/internal/adapter/oidcauth"`, `"github.com/serverkraken/flow/internal/adapter/websession"`.

- [ ] **Step 10: Run the suite**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go build ./... && go test ./internal/config/ ./internal/adapter/websession/ ./internal/adapter/oidcauth/ ./internal/adapter/httpserver/ -v`
Expected: PASS.

- [ ] **Step 11: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/config/ internal/ports/ports.go internal/adapter/pgstore/users.go internal/testutil/fakes.go internal/adapter/oidcauth/ internal/adapter/websession/ internal/adapter/httpserver/ cmd/flow-server/main.go
git commit -m "feat(auth): OIDC auth-code-flow + signed session cookie + cookie-or-bearer SSE"
```

---

## Task 6: apiclient worktime methods + SSE client

**Files:**
- Modify: `internal/adapter/apiclient/client.go`
- Create: `internal/adapter/apiclient/events.go`
- Test: `internal/adapter/apiclient/worktime_test.go`

- [ ] **Step 1: Write the failing client tests**

`internal/adapter/apiclient/worktime_test.go`:

```go
package apiclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

func TestStartSessionAndListProjects(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing bearer")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/sessions":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"s1","start":"2026-06-14T09:00:00Z"}`))
		case "/api/v1/projects":
			_, _ = w.Write([]byte(`[{"id":"p1","name":"Flow","slug":"flow","status":"active"}]`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")

	s, err := c.StartSession(context.Background(), nil, "", "")
	if err != nil || s.ID != "s1" {
		t.Fatalf("StartSession = %+v err=%v", s, err)
	}
	ps, err := c.ListProjects(context.Background())
	if err != nil || len(ps) != 1 || ps[0].Name != "Flow" {
		t.Fatalf("ListProjects = %+v err=%v", ps, err)
	}
}

func TestEventsStream(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("event: session.started\ndata: {\"type\":\"session.started\",\"data\":{\"id\":\"s1\"}}\n\n"))
		w.(http.Flusher).Flush()
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := c.Events(ctx)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	ev := <-ch
	if ev.Type != "session.started" || ev.Data["id"] != "s1" {
		t.Fatalf("event = %+v", ev)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/apiclient/ -v`
Expected: FAIL — `c.StartSession` / `c.Events` undefined.

- [ ] **Step 3: Add worktime methods**

Append to `internal/adapter/apiclient/client.go` (and add `bytes`, `io` to its imports):

```go
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 300 {
		return fmt.Errorf("apiclient: %s %s: status %d", method, path, res.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}

func (c *Client) StartSession(ctx context.Context, projectID *string, tag, note string) (domain.WorkSession, error) {
	var s domain.WorkSession
	err := c.do(ctx, http.MethodPost, "/api/v1/sessions",
		map[string]any{"projectId": projectID, "tag": tag, "note": note}, &s)
	return s, err
}

func (c *Client) StopSession(ctx context.Context, id, projectID string) (domain.WorkSession, error) {
	var s domain.WorkSession
	err := c.do(ctx, http.MethodPost, "/api/v1/sessions/"+id+"/stop",
		map[string]any{"projectId": projectID}, &s)
	return s, err
}

func (c *Client) ListSessions(ctx context.Context) ([]domain.WorkSession, error) {
	var out []domain.WorkSession
	err := c.do(ctx, http.MethodGet, "/api/v1/sessions", nil, &out)
	return out, err
}

func (c *Client) CreateProject(ctx context.Context, name string) (domain.Project, error) {
	var p domain.Project
	err := c.do(ctx, http.MethodPost, "/api/v1/projects", map[string]any{"name": name}, &p)
	return p, err
}

func (c *Client) ListProjects(ctx context.Context) ([]domain.Project, error) {
	var out []domain.Project
	err := c.do(ctx, http.MethodGet, "/api/v1/projects", nil, &out)
	return out, err
}
```

- [ ] **Step 4: Add the SSE client**

`internal/adapter/apiclient/events.go`:

```go
package apiclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// streamClient has no timeout — the events stream is long-lived and ends only
// when the context is cancelled or the server closes.
var streamClient = &http.Client{}

// ClientEvent is a decoded SSE frame: the event name and the small payload.
type ClientEvent struct {
	Type string
	Data map[string]any
}

// Events subscribes to the server SSE stream. The returned channel is closed
// when ctx is cancelled or the connection drops; callers reconnect by calling
// Events again (and should full-refresh their state on reconnect).
func (c *Client) Events(ctx context.Context) (<-chan ClientEvent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/events", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := streamClient.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return nil, fmt.Errorf("apiclient: events status %d", res.StatusCode)
	}
	ch := make(chan ClientEvent)
	go func() {
		defer close(ch)
		defer func() { _ = res.Body.Close() }()
		sc := bufio.NewScanner(res.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var evType, data string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event:"):
				evType = strings.TrimSpace(line[len("event:"):])
			case strings.HasPrefix(line, "data:"):
				data = strings.TrimSpace(line[len("data:"):])
			case line == "":
				if evType == "" {
					continue
				}
				var payload struct {
					Data map[string]any `json:"data"`
				}
				_ = json.Unmarshal([]byte(data), &payload)
				select {
				case ch <- ClientEvent{Type: evType, Data: payload.Data}:
				case <-ctx.Done():
					return
				}
				evType, data = "", ""
			}
		}
	}()
	return ch, nil
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/apiclient/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/adapter/apiclient/
git commit -m "feat(apiclient): worktime REST methods + SSE event stream client"
```

---

## Task 7: TUI worktime screen + live SSE

> Skills: consult **bubbletea** (framework idioms) and **tui-usability** (Tokyonight colors, keybind grammar, glyph whitelist) while implementing. The code below is a working baseline; refine visuals per tui-usability.

**Files:**
- Create: `internal/tui/styles.go`, `internal/tui/worktime.go`
- Create: `cmd/flow/worktime.go`
- Test: `internal/tui/worktime_test.go`

- [ ] **Step 1: Add Charm v2 deps**

Run:
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
go get charm.land/bubbletea/v2@v2.0.6 charm.land/lipgloss/v2@v2.0.3
```
Expected: `go.mod` gains `charm.land/bubbletea/v2` and `charm.land/lipgloss/v2`.

- [ ] **Step 2: Styles**

`internal/tui/styles.go`:

```go
package tui

import "charm.land/lipgloss/v2"

// Tokyonight-night palette (subset). tui-usability governs the full semantics.
var (
	colBg     = lipgloss.Color("#1a1b26")
	colMuted  = lipgloss.Color("#565f89")
	colAccent = lipgloss.Color("#7aa2f7")
	colGreen  = lipgloss.Color("#9ece6a")
	colRed    = lipgloss.Color("#f7768e")

	styleHeader  = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	styleRunning = lipgloss.NewStyle().Foreground(colBg).Background(colGreen).Bold(true).Padding(0, 1)
	styleMuted   = lipgloss.NewStyle().Foreground(colMuted)
	styleSel     = lipgloss.NewStyle().Foreground(colBg).Background(colAccent)
	styleErr     = lipgloss.NewStyle().Foreground(colRed)
)
```

- [ ] **Step 3: Write the failing model test**

`internal/tui/worktime_test.go`:

```go
package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
)

func TestLoadedPopulatesAndViewRenders(t *testing.T) {
	m := New(nil, "tester")
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	run, _ := domain.NewWorkSession("s1", "u1", nil, start)
	next, _ := m.Update(loadedMsg{
		sessions: []domain.WorkSession{run},
		projects: []domain.Project{{ID: "p1", Name: "Flow"}},
		now:      start.Add(25 * time.Minute),
	})
	m = next.(Model)
	if m.running == nil || m.running.ID != "s1" {
		t.Fatal("running session not detected from loaded sessions")
	}
	view := m.View()
	if !strings.Contains(view, "00:25") {
		t.Fatalf("running elapsed not rendered:\n%s", view)
	}
}

func TestQuitKey(t *testing.T) {
	m := New(nil, "tester")
	_, cmd := m.Update(tea.KeyPressMsg{Text: "q"})
	if cmd == nil {
		t.Fatal("q should return a quit command")
	}
}

func TestTickAdvancesNow(t *testing.T) {
	m := New(nil, "tester")
	t0 := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	next, _ := m.Update(tickMsg(t0))
	if got := next.(Model).now; !got.Equal(t0) {
		t.Fatalf("tick now = %v", got)
	}
}
```

- [ ] **Step 4: Run to verify it fails**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/ -v`
Expected: FAIL — `undefined: New`, `loadedMsg`, `tickMsg`, `Model`.

- [ ] **Step 5: Implement the model**

`internal/tui/worktime.go`:

```go
// Package tui is the flow terminal UI (Bubbletea v2). M1a ships one screen:
// the worktime timer, live-synced via the server SSE stream.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// Model is the worktime screen (and, for M1a, the whole app shell).
type Model struct {
	client *apiclient.Client
	user   string

	sessions []domain.WorkSession
	projects []domain.Project
	running  *domain.WorkSession
	now      time.Time

	booking bool   // stop pressed → choosing a project to book
	sel     int    // selected project index in the picker
	newName string // inline new-project name buffer

	status string
	err    error
	events <-chan apiclient.ClientEvent
}

// New builds the model. client may be nil in tests that only drive Update.
func New(client *apiclient.Client, user string) Model {
	return Model{client: client, user: user, now: time.Now()}
}

// — messages —

type loadedMsg struct {
	sessions []domain.WorkSession
	projects []domain.Project
	now      time.Time
}
type eventMsg struct{ ev apiclient.ClientEvent }
type eventsReadyMsg struct{ ch <-chan apiclient.ClientEvent }
type tickMsg time.Time
type errMsg struct{ err error }

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.reload(), m.subscribe(), tick())
}

// — commands —

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) reload() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		sessions, err := m.client.ListSessions(ctx)
		if err != nil {
			return errMsg{err}
		}
		projects, err := m.client.ListProjects(ctx)
		if err != nil {
			return errMsg{err}
		}
		return loadedMsg{sessions: sessions, projects: projects, now: time.Now()}
	}
}

func (m Model) subscribe() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ch, err := m.client.Events(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return eventsReadyMsg{ch}
	}
}

func waitForEvent(ch <-chan apiclient.ClientEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return eventMsg{ev}
	}
}

func (m Model) startCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := m.client.StartSession(ctx, nil, "", ""); err != nil {
			return errMsg{err}
		}
		return nil // the SSE event triggers reload
	}
}

func (m Model) stopCmd(sessionID, projectID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := m.client.StopSession(ctx, sessionID, projectID); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m Model) createAndStopCmd(sessionID, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		p, err := m.client.CreateProject(ctx, name)
		if err != nil {
			return errMsg{err}
		}
		if _, err := m.client.StopSession(ctx, sessionID, p.ID); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

// — update —

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.now = time.Time(msg)
		return m, tick()
	case loadedMsg:
		m.sessions = msg.sessions
		m.projects = msg.projects
		m.now = msg.now
		m.running = nil
		for i := range m.sessions {
			if m.sessions[i].Running() {
				s := m.sessions[i]
				m.running = &s
			}
		}
		return m, nil
	case eventsReadyMsg:
		m.events = msg.ch
		return m, waitForEvent(msg.ch)
	case eventMsg:
		// any worktime/project event → refresh, keep listening
		return m, tea.Batch(m.reload(), waitForEvent(m.events))
	case errMsg:
		m.err = msg.err
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.booking {
		return m.handleBookingKey(k)
	}
	switch {
	case k.Text == "q" || (k.Code == 'c' && k.Mod == tea.ModCtrl):
		return m, tea.Quit
	case k.Text == "s":
		if m.running != nil || m.client == nil {
			return m, nil
		}
		m.status = "starting…"
		return m, m.startCmd()
	case k.Text == "x":
		if m.running == nil {
			return m, nil
		}
		m.booking = true
		m.sel = 0
		m.newName = ""
		return m, nil
	}
	return m, nil
}

func (m Model) handleBookingKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		m.booking = false
		return m, nil
	case k.Code == tea.KeyEnter:
		id := m.running.ID
		m.booking = false
		if strings.TrimSpace(m.newName) != "" {
			return m, m.createAndStopCmd(id, strings.TrimSpace(m.newName))
		}
		if len(m.projects) == 0 {
			m.booking = true // nothing to book to yet; keep typing
			return m, nil
		}
		return m, m.stopCmd(id, m.projects[m.sel].ID)
	case k.Code == tea.KeyBackspace:
		if m.newName != "" {
			m.newName = m.newName[:len(m.newName)-1]
		}
		return m, nil
	case k.Text == "j" && m.newName == "":
		if m.sel < len(m.projects)-1 {
			m.sel++
		}
		return m, nil
	case k.Text == "k" && m.newName == "":
		if m.sel > 0 {
			m.sel--
		}
		return m, nil
	case k.Text != "":
		m.newName += k.Text
		return m, nil
	}
	return m, nil
}

// — view —

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render("flow · worktime") + styleMuted.Render("  "+m.user) + "\n\n")

	if m.running != nil {
		el := m.running.Elapsed(m.now)
		b.WriteString(styleRunning.Render("▶ "+fmtDur(el)) + styleMuted.Render("  running") + "\n")
	} else {
		b.WriteString(styleMuted.Render("○ idle — press s to start") + "\n")
	}
	b.WriteString("\n")

	if m.booking {
		b.WriteString(styleHeader.Render("Book session to a project") + "\n")
		for i, p := range m.projects {
			line := "  " + glyphOr(p.Glyph) + " " + p.Name
			if i == m.sel && m.newName == "" {
				line = styleSel.Render("▸ " + glyphOr(p.Glyph) + " " + p.Name)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString(styleMuted.Render("  new: ") + m.newName + "▏\n")
		b.WriteString(styleMuted.Render("  j/k pick · type a name to create · enter confirm · esc cancel") + "\n")
	} else {
		b.WriteString(styleHeader.Render("Today") + "\n")
		if len(m.sessions) == 0 {
			b.WriteString(styleMuted.Render("  no sessions yet") + "\n")
		}
		for _, s := range m.sessions {
			mark := "·"
			if s.Running() {
				mark = "▶"
			}
			b.WriteString(fmt.Sprintf("  %s %s  %s\n", mark, s.Start.Local().Format("15:04"), fmtDur(s.Elapsed(m.now))))
		}
	}

	b.WriteString("\n")
	if m.err != nil {
		b.WriteString(styleErr.Render("error: "+m.err.Error()) + "\n")
	}
	b.WriteString(styleMuted.Render("s start · x stop · q quit") + "\n")
	return b.String()
}

func fmtDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

func glyphOr(g string) string {
	if g == "" {
		return "●"
	}
	return g
}
```

- [ ] **Step 6: Run the model tests**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/tui/ -v`
Expected: PASS.

- [ ] **Step 7: Add the `flow worktime` command**

`cmd/flow/worktime.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui"
	"github.com/spf13/cobra"
)

func worktimeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "worktime",
		Short: "Worktime timer (TUI)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			base := envOr("FLOW_SERVER_URL", "http://localhost:8080")
			token := os.Getenv("FLOW_TOKEN") // device-flow login lands in M1b
			if token == "" {
				return fmt.Errorf("set FLOW_TOKEN (device-flow login comes in M1b)")
			}
			// slog/stderr must never corrupt the TUI: send logs to a file.
			logf, err := os.OpenFile(filepath.Join(os.TempDir(), "flow-tui.log"),
				os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err == nil {
				defer func() { _ = logf.Close() }()
				os.Stderr = logf
			}
			client := apiclient.New(base, token)
			m := tui.New(client, os.Getenv("USER"))
			_, err = tea.NewProgram(m, tea.WithContext(cmd.Context()), tea.WithAltScreen()).Run()
			return err
		},
	}
}
```

Register it in `cmd/flow/main.go`:

```go
	root.AddCommand(whoamiCmd())
	root.AddCommand(worktimeCmd())
```

- [ ] **Step 8: Build + run the suite**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go build ./... && go test ./internal/tui/ -v`
Expected: PASS; `go build ./...` clean.

(`tea.WithAltScreen` / `tea.WithContext` exist in bubbletea v2 v2.0.6 — confirm exact option names against the vendored version if the build complains, and adjust.)

- [ ] **Step 9: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/tui/ cmd/flow/ go.mod go.sum
git commit -m "feat(tui): worktime screen with live SSE + book-on-stop project picker"
```

---

## Task 8: WebUI — templ + HTMX-SSE + Tailwind v4

> Design language per the spec: light canvas, gradient only on the focus element (the running timer), mobile-first single column. Pixel polish is deferred to the `frontend-design` skill — this task delivers functional-correct.

**Architecture note:** the WebUI is **hypermedia**. The browser holds a session cookie, not a bearer, so its action endpoints live under `webAuth` and return HTML fragments (not the bearer-only JSON REST API the TUI uses). templ components are pure view (`internal/adapter/webui`); the controller methods stay on `httpserver.Server` (where `userFrom` and the usecases already live).

**Files:**
- Create: `internal/adapter/webui/worktime.templ` (+ generated `worktime_templ.go`)
- Create: `internal/adapter/webui/format.go`, `internal/adapter/webui/static.go`, `internal/adapter/webui/static/app.css`
- Create: `web/tailwind.css`
- Create: `internal/adapter/httpserver/webui.go`
- Modify: `internal/adapter/httpserver/server.go` (routes), `Makefile`, `.github/workflows/*` (CI), `go.mod`
- Test: `internal/adapter/webui/render_test.go`, append to `internal/adapter/httpserver/webauth_test.go`

- [ ] **Step 1: Add templ deps + tool**

Run:
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
go get github.com/a-h/templ@latest
go get -tool github.com/a-h/templ/cmd/templ@latest
```
Expected: `go.mod` gains `github.com/a-h/templ` and a `tool` directive for the `templ` CLI. (`go tool templ` now works without a global install — keeps CI hermetic.)

- [ ] **Step 2: Write the templ components**

`internal/adapter/webui/worktime.templ`:

```templ
package webui

import "github.com/serverkraken/flow/internal/domain"
import "time"

// WorktimeData is the view model for the worktime screen.
type WorktimeData struct {
	User     string
	Running  *domain.WorkSession
	Now      time.Time
	Sessions []domain.WorkSession
	Projects []domain.Project
}

templ WorktimePage(d WorktimeData) {
	<!DOCTYPE html>
	<html lang="en">
		<head>
			<meta charset="utf-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1"/>
			<title>flow · worktime</title>
			<link rel="stylesheet" href="/static/app.css"/>
			<script src="https://unpkg.com/htmx.org@2.0.4"></script>
			<script src="https://unpkg.com/htmx-ext-sse@2.2.3"></script>
		</head>
		<body class="bg-slate-50 text-slate-800" hx-ext="sse" sse-connect="/api/v1/events">
			<main class="mx-auto max-w-md p-4">
				<div id="wt" hx-get="/ui/worktime" hx-trigger="sse:session.started, sse:session.stopped, sse:session.updated, sse:project.created" hx-swap="innerHTML">
					@WorktimeFragment(d)
				</div>
			</main>
		</body>
	</html>
}

templ WorktimeFragment(d WorktimeData) {
	<header class="mb-4 flex items-center justify-between">
		<h1 class="text-lg font-semibold text-slate-900">flow · worktime</h1>
		<form action="/auth/logout" method="post" hx-boost="false">
			<button class="text-sm text-slate-500">logout { d.User }</button>
		</form>
	</header>
	if d.Running != nil {
		<div class="rounded-2xl bg-gradient-to-r from-emerald-400 to-teal-500 p-5 text-white shadow-lg">
			<div class="text-4xl font-bold tabular-nums">{ fmtDur(d.Running.Elapsed(d.Now)) }</div>
			<div class="text-sm text-white/80">running</div>
			<form hx-post="/ui/worktime/stop" hx-target="#wt" hx-swap="innerHTML" class="mt-4 flex flex-wrap gap-2">
				<input type="hidden" name="sessionId" value={ d.Running.ID }/>
				<select name="projectId" class="rounded bg-white/20 px-2 py-1 text-sm">
					for _, p := range d.Projects {
						<option value={ p.ID }>{ p.Name }</option>
					}
				</select>
				<input name="newProject" placeholder="or new project…" class="rounded bg-white/20 px-2 py-1 text-sm placeholder-white/60"/>
				<button class="rounded bg-white/90 px-3 py-1 text-sm font-medium text-slate-900">stop</button>
			</form>
		</div>
	} else {
		<form hx-post="/ui/worktime/start" hx-target="#wt" hx-swap="innerHTML">
			<button class="w-full rounded-2xl bg-slate-900 p-5 text-left text-lg text-white">▶ start timer</button>
		</form>
	}
	<section class="mt-6">
		<h2 class="mb-2 text-sm font-semibold uppercase tracking-wide text-slate-500">Today</h2>
		if len(d.Sessions) == 0 {
			<p class="text-sm text-slate-400">no sessions yet</p>
		}
		<ul class="divide-y divide-slate-100">
			for _, s := range d.Sessions {
				<li class="flex justify-between py-2 text-sm">
					<span>{ s.Start.Local().Format("15:04") }</span>
					<span class="tabular-nums text-slate-500">{ fmtDur(s.Elapsed(d.Now)) }</span>
				</li>
			}
		</ul>
	</section>
}
```

`internal/adapter/webui/format.go`:

```go
// Package webui holds the templ view components for the server-rendered UI.
package webui

import (
	"fmt"
	"time"
)

// fmtDur renders a duration as HH:MM (clamped at zero).
func fmtDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}
```

- [ ] **Step 3: Generate the templ Go**

Run:
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
go tool templ generate
```
Expected: creates `internal/adapter/webui/worktime_templ.go`. Commit this generated file alongside the source.

- [ ] **Step 4: Tailwind source + build the CSS**

`web/tailwind.css`:

```css
@import "tailwindcss";
@source "../internal/adapter/webui/**/*.templ";
```

Build the stylesheet (Tailwind v4 standalone CLI — install via `mise use -g tailwindcss@4` or download the release binary):

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
tailwindcss -i web/tailwind.css -o internal/adapter/webui/static/app.css --minify
```

Commit the generated `internal/adapter/webui/static/app.css` so `go build`/`go:embed` never depends on the Tailwind toolchain being present.

`internal/adapter/webui/static.go`:

```go
package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/app.css
var staticFS embed.FS

// StaticHandler serves the embedded static assets (mount under /static/).
func StaticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err) // embedded path is a compile-time constant; cannot fail at runtime
	}
	return http.FileServerFS(sub)
}
```

- [ ] **Step 5: Controller methods + routes (failing handler test first)**

Append to `internal/adapter/httpserver/webauth_test.go`:

```go
func TestWebHomeRendersWithSessionCookie(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeProjectStore()
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	srv := &httpserver.Server{
		Ensure:        usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:           sse.NewBus(),
		Clock:         clk,
		Users:         users,
		Session:       codec,
		StartSession:  usecase.StartSession{Sessions: ss, IDs: ids, Clock: clk},
		StopSession:   usecase.StopSession{Sessions: ss, Projects: ps, Clock: clk},
		ListSessions:  usecase.ListSessions{Sessions: ss, Clock: clk},
		CreateProject: usecase.CreateProject{Projects: ps, IDs: ids, Clock: clk},
		ListProjects:  usecase.ListProjects{Projects: ps},
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	cookieVal, _ := codec.Issue("u1")
	req, _ := http.NewRequest("GET", ts.URL+"/", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("home status %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "start timer") {
		t.Fatalf("home did not render the worktime screen:\n%s", string(body))
	}
}
```

Add imports `io`, `strings` to the test file as needed.

`internal/adapter/httpserver/webui.go`:

```go
package httpserver

import (
	"context"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

func (s *Server) worktimeData(ctx context.Context, u domain.User) (webui.WorktimeData, error) {
	since := startOfDay(s.Clock.Now())
	sessions, err := s.ListSessions.Execute(ctx, u.ID, since)
	if err != nil {
		return webui.WorktimeData{}, err
	}
	projects, err := s.ListProjects.Execute(ctx, u.ID)
	if err != nil {
		return webui.WorktimeData{}, err
	}
	var running *domain.WorkSession
	for i := range sessions {
		if sessions[i].Running() {
			r := sessions[i]
			running = &r
		}
	}
	return webui.WorktimeData{
		User: u.Username, Running: running, Now: s.Clock.Now(),
		Sessions: sessions, Projects: projects,
	}, nil
}

func (s *Server) renderFragment(w http.ResponseWriter, r *http.Request, u domain.User) {
	d, err := s.worktimeData(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WorktimeFragment(d).Render(r.Context(), w)
}

func (s *Server) handleWebHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.worktimeData(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WorktimePage(d).Render(r.Context(), w)
}

func (s *Server) handleWebFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	s.renderFragment(w, r, u)
}

func (s *Server) handleWebStart(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	if _, err := s.StartSession.Execute(r.Context(), u.ID, nil, "", ""); err == nil {
		s.Bus.Publish(domain.Event{Type: domain.EventSessionStarted, UserID: u.ID})
	}
	s.renderFragment(w, r, u)
}

func (s *Server) handleWebStop(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	sessionID := r.FormValue("sessionId")
	projectID := r.FormValue("projectId")
	if name := r.FormValue("newProject"); name != "" {
		if p, err := s.CreateProject.Execute(r.Context(), u.ID, name, "", "", ""); err == nil {
			projectID = p.ID
			s.Bus.Publish(domain.Event{Type: domain.EventProjectCreated, UserID: u.ID})
		}
	}
	if _, err := s.StopSession.Execute(r.Context(), u.ID, sessionID, &projectID); err == nil {
		s.Bus.Publish(domain.Event{Type: domain.EventSessionStopped, UserID: u.ID})
	}
	s.renderFragment(w, r, u)
}
```

Add the routes in `internal/adapter/httpserver/server.go` `Routes()` (before `return mux`):

```go
	// WebUI (cookie-authenticated, returns HTML fragments)
	mux.Handle("GET /{$}", s.webAuth(http.HandlerFunc(s.handleWebHome)))
	mux.Handle("GET /ui/worktime", s.webAuth(http.HandlerFunc(s.handleWebFragment)))
	mux.Handle("POST /ui/worktime/start", s.webAuth(http.HandlerFunc(s.handleWebStart)))
	mux.Handle("POST /ui/worktime/stop", s.webAuth(http.HandlerFunc(s.handleWebStop)))
	mux.Handle("GET /static/", http.StripPrefix("/static/", webui.StaticHandler()))
```

Add `"github.com/serverkraken/flow/internal/adapter/webui"` to server.go imports.

- [ ] **Step 6: templ render unit test**

`internal/adapter/webui/render_test.go`:

```go
package webui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestFragmentShowsRunningTimer(t *testing.T) {
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	run, _ := domain.NewWorkSession("s1", "u1", nil, start)
	d := WorktimeData{User: "msoent", Running: &run, Now: start.Add(90 * time.Minute)}
	var b bytes.Buffer
	if err := WorktimeFragment(d).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "01:30") {
		t.Fatalf("running elapsed not rendered:\n%s", b.String())
	}
}

func TestFragmentIdleShowsStart(t *testing.T) {
	var b bytes.Buffer
	if err := WorktimeFragment(WorktimeData{User: "x"}).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "start timer") {
		t.Fatal("idle fragment missing start button")
	}
}
```

- [ ] **Step 7: Wire codegen + CSS into Makefile and CI**

Add to `Makefile`:

```makefile
WEB_CSS := internal/adapter/webui/static/app.css

.PHONY: generate web verify-generate
generate:
	go tool templ generate
web:
	tailwindcss -i web/tailwind.css -o $(WEB_CSS) --minify
verify-generate:
	go tool templ generate
	git diff --exit-code -- ':*_templ.go'
```

Change the `ci` target to gate on fresh codegen:

```makefile
ci: lint verify-generate cover build
```

There is no GitHub Actions workflow on the `rebuild` branch yet — CI is `make ci` run locally. The `verify-generate` target (added above) is therefore the codegen gate. `go tool templ` resolves from the `tool` directive, so no global install is needed. (Tailwind is **not** part of `make ci` — the committed `app.css` is the source of truth; regenerate locally with `make web` when classes change.) If a CI workflow is added later, it only needs `go mod download` + `make ci`.

- [ ] **Step 8: Build + full suite**

Run:
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
go tool templ generate && go build ./... && go test ./internal/adapter/webui/ ./internal/adapter/httpserver/ -v
```
Expected: PASS; build clean.

- [ ] **Step 9: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/adapter/webui/ internal/adapter/httpserver/ web/ Makefile go.mod go.sum
git commit -m "feat(webui): templ + HTMX-SSE worktime page (cookie auth, live fragment swaps)"
```

---

## Task 9: Wiring verification + two-surface live-sync check

> Lesson baked in ("plans need a main-wiring task"): per-task tests don't catch "the composition root never calls the new constructor." This task asserts every route is registered and that a real start→SSE loop fires end-to-end.

**Files:**
- Create: `internal/adapter/httpserver/routes_test.go`
- Create: `scripts/smoke-m1a.sh`, `scripts/live-sync-check.sh`
- Delete: `scripts/smoke-m0.sh`
- Modify: `Makefile` (`smoke` target)

- [ ] **Step 1: Route-registration test (guards main wiring)**

`internal/adapter/httpserver/routes_test.go`:

```go
package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestAllRoutesRegistered(t *testing.T) {
	clk := testutil.FakeClock{T: time.Now()}
	ids := &testutil.FakeIDGen{}
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeProjectStore()
	users := testutil.NewFakeUserStore()
	srv := &httpserver.Server{
		Verifier:      testutil.FakeVerifier{ID: ports.Identity{Subject: "s"}},
		Ensure:        usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:           sse.NewBus(),
		Clock:         clk,
		Users:         users,
		Session:       websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour),
		OIDCAuth:      fakeAuth{url: "https://id/authorize?state="},
		StartSession:  usecase.StartSession{Sessions: ss, IDs: ids, Clock: clk},
		StopSession:   usecase.StopSession{Sessions: ss, Projects: ps, Clock: clk},
		ListSessions:  usecase.ListSessions{Sessions: ss, Clock: clk},
		CreateProject: usecase.CreateProject{Projects: ps, IDs: ids, Clock: clk},
		ListProjects:  usecase.ListProjects{Projects: ps},
	}
	h := srv.Routes()
	cases := []struct{ method, path string }{
		{"GET", "/healthz"},
		{"GET", "/api/v1/me"},
		{"GET", "/api/v1/events"},
		{"POST", "/api/v1/sessions"},
		{"POST", "/api/v1/sessions/x/stop"},
		{"GET", "/api/v1/sessions"},
		{"POST", "/api/v1/projects"},
		{"GET", "/api/v1/projects"},
		{"GET", "/auth/login"},
		{"GET", "/auth/callback"},
		{"POST", "/auth/logout"},
		{"GET", "/"},
		{"GET", "/ui/worktime"},
		{"POST", "/ui/worktime/start"},
		{"POST", "/ui/worktime/stop"},
		{"GET", "/static/app.css"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Errorf("%s %s is not registered (404)", tc.method, tc.path)
		}
	}
}
```

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/httpserver/ -run AllRoutes -v`
Expected: PASS (auth-gated routes return 401/302, not 404 — which proves registration).

- [ ] **Step 2: Replace the smoke script**

Delete `scripts/smoke-m0.sh`. Create `scripts/smoke-m1a.sh`:

```bash
#!/usr/bin/env bash
# Smoke the unauthenticated surface + print manual steps for authed routes.
# Run `make db-up` and start flow-server first.
set -euo pipefail
BASE="${BASE:-http://localhost:8080}"

echo "== /healthz =="
curl -fsS "$BASE/healthz" && echo

echo "== /api/v1/me without token (expect 401) =="
code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/me")
[ "$code" = "401" ] && echo "OK 401" || { echo "FAIL: $code"; exit 1; }

echo "== / without session cookie (expect 302 -> /auth/login) =="
loc=$(curl -s -o /dev/null -w '%{redirect_url}' "$BASE/")
case "$loc" in */auth/login) echo "OK redirect: $loc";; *) echo "FAIL: $loc"; exit 1;; esac

cat <<'EOF'
== authed live-sync (manual / scripted) ==
  TOKEN=<allowlisted Authentik access token>  ./scripts/live-sync-check.sh
  WebUI:  open http://localhost:8080/  -> login via Authentik -> start a timer
          in the browser, then run a `flow worktime` TUI (FLOW_TOKEN=$TOKEN) and
          watch the timer appear/disappear on both within ~1s.
EOF
```

`scripts/live-sync-check.sh` — the scripted half of the acceptance gate:

```bash
#!/usr/bin/env bash
# Asserts a start fired over REST shows up on the SSE stream (the M1a Done-gate,
# server side). Requires a running flow-server + DB and a real token.
set -euo pipefail
BASE="${BASE:-http://localhost:8080}"
: "${TOKEN:?set TOKEN to an allowlisted Authentik access token}"
auth=(-H "Authorization: Bearer $TOKEN")

tmp=$(mktemp)
curl -N "${auth[@]}" "$BASE/api/v1/events" >"$tmp" 2>/dev/null &
spid=$!
trap 'kill "$spid" 2>/dev/null || true; rm -f "$tmp"' EXIT
sleep 1

curl -fsS -X POST "${auth[@]}" -H 'Content-Type: application/json' -d '{}' \
  "$BASE/api/v1/sessions" >/dev/null

for _ in $(seq 1 20); do
  if grep -q 'event: session.started' "$tmp"; then
    echo "OK: session.started observed on the SSE stream"
    echo "(cleanup: stop the running session via the WebUI or 'flow worktime')"
    exit 0
  fi
  sleep 0.25
done
echo "FAIL: no session.started event within 5s"
exit 1
```

Make both executable: `chmod +x scripts/smoke-m1a.sh scripts/live-sync-check.sh`.

In `Makefile`, point `smoke` at the new script:

```makefile
smoke:
	./scripts/smoke-m1a.sh
```

- [ ] **Step 3: Full gate**

Run:
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
make ci
```
Expected: lint clean, `verify-generate` clean (templ up to date), coverage ≥ 80%, build OK.

> If coverage dips below 80% (templ-generated code enlarges the denominator), the cheapest lever is more `WorktimeData` variations in `internal/adapter/webui/render_test.go` (e.g. a fragment with several `Projects` and `Sessions`) — that exercises the generated `for`/`if` branches.

- [ ] **Step 4: Manual two-surface confirmation (the actual Done-gate)**

Document the result in the commit body. Steps:
1. `make db-up` and run `FLOW_DEV=1 ... ./bin/flow-server` (with real Authentik env for auth-code-flow + a real `FLOW_TOKEN` for the TUI).
2. `./scripts/live-sync-check.sh` → `OK: session.started observed`.
3. Browser: open `http://localhost:8080/`, log in via Authentik, start a timer. In a terminal: `FLOW_SERVER_URL=http://localhost:8080 FLOW_TOKEN=$TOKEN ./bin/flow worktime`. Confirm the running timer shows in **both**, and stopping in one clears it in the other within ~1s.

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/adapter/httpserver/routes_test.go scripts/ Makefile
git rm scripts/smoke-m0.sh
git commit -m "test(m1a): route-registration guard + live-sync smoke; replace m0 smoke"
```

---

## Self-Review

**Spec coverage** (each spec section → task):
- Data model deltas (Project-min, WorkSession, one-running invariant) → Tasks 1, 2.
- Carry-over domain semantics (elapsed, running) → Task 1.
- Stores + migration 0002 + partial unique index → Task 2.
- Usecases (start/stop/list/create/listProjects, booking, slugify) → Task 3.
- REST + SSE event types + drop debug/ping → Task 4.
- Auth-code-flow + session cookie + cookie-or-bearer SSE + config → Task 5.
- apiclient methods + SSE client → Task 6.
- TUI small shell + worktime screen + live SSE → Task 7.
- WebUI templ + HTMX-SSE + Tailwind + gradient focus element → Task 8.
- Verification gate (per-route smoke, wiring guard, two-surface live-sync) → Task 9.
- Non-goals (device-flow=M1b, DayOff/Stats/ICS=M1c, offline) → intentionally absent. ✓

**Type consistency** (names used across tasks):
- `ports.SessionStore`: `Create / Running / Stop / List` — Task 2 defs match Task 3 usecase calls and Task 3 fakes.
- `ports.ProjectStore`: `Create / List / Get` — consistent across pgstore, fakes, usecases.
- `httpserver.Server` fields (`StartSession`, `StopSession`, `ListSessions`, `CreateProject`, `ListProjects`, `Clock`, `OIDCAuth`, `Session`, `Users`) — defined in Task 4 struct, wired in Tasks 4/5 main.go, used in Tasks 4/5/8 handlers.
- `Authenticator` / `SessionCodec` interfaces — declared in Task 4 stub, kept identical when fleshed out in Task 5; `oidcauth.Authenticator` and `websession.Codec` satisfy them.
- `apiclient.Client` methods (`StartSession/StopSession/ListSessions/CreateProject/ListProjects/Events`) — Task 6 defs match Task 7 TUI calls.
- `webui.WorktimeData` fields (`User/Running/Now/Sessions/Projects`) — Task 8 templ + Task 7-independent; populated identically by `httpserver.worktimeData`.
- Event types (`session.started/stopped/updated`, `project.created`) — Task 4 domain consts; published in Tasks 4 + 8; subscribed in TUI (Task 7) + WebUI `hx-trigger` (Task 8). ✓

**Known soft spots (flagged, not placeholders):**
- `oidcauth.Exchange` is covered by the live smoke, not a unit test (Task 5 note explains how to add token-endpoint coverage if the gate dips).
- Charm v2 option names (`tea.WithAltScreen`/`tea.WithContext`) and lipgloss color API are pinned to the versions in `main`'s go.mod; Task 7 says to adjust if the build complains.
- Tailwind/templ generated artifacts are committed so `go build` is toolchain-free; `make ci`'s `verify-generate` keeps templ output honest.

---

## Execution Handoff

Execute on the `rebuild` branch (worktree `flow-rebuild`), one commit per task, `make ci` green before moving on. Recommended: **subagent-driven** (fresh subagent per task + review between), matching how M0 and the earlier rebuild work were run. Tasks 1→9 are mostly sequential (4 depends on 1–3; 5 on 4; 8 on 5+4; 9 on all); within Task boundaries the steps are TDD-ordered.
