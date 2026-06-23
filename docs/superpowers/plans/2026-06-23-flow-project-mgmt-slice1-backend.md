# Project Management M4 — Slice 1 (Backend-Core) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give `Project` editable rich metadata (description, canonical upstream git), a real three-state lifecycle, full server-side CRUD (`UpdateProject` + `GetProject` + `PATCH`/`GET /projects/{id}`), and auto-sync of the upstream git to the cwd→project remote binding — the backend foundation every M4 surface builds on.

**Architecture:** Hexagonal. Domain gains fields + a `Validate()` + an `ErrInvalidUpstream` sentinel. A migration adds two columns; the single `ProjectStore` impl (pgstore) and the single test fake (`testutil.FakeProjectStore`) gain `Update`. Two new usecases (`UpdateProject` with auto-sync, `GetProject`) + REST handlers + apiclient methods + light MCP enrichment. Composition root wired and curl-smoked at the end.

**Tech Stack:** Go, pgx/v5, goose migrations, net/http (Go 1.22 `ServeMux` with method+wildcard patterns), templ/HTMX (later slices), MCP go-sdk.

## Global Constraints

- Module path: `github.com/serverkraken/flow`. Work on branch `rebuild`.
- Spec: `docs/superpowers/specs/2026-06-23-flow-project-management-design.md`.
- `make ci` must stay green; coverage gate ~80%. Run it before the final commit of each task that changes Go code.
- Migrations are run by **goose** — every `.sql` file MUST carry `-- +goose Up` / `-- +goose Down`. Bare SQL fails at apply (caught only by pgstore Docker tests).
- pgstore tests need the test Postgres; they use the existing `startPG(t)` + `pgstore.NewPool` + `pgstore.Migrate` harness (see `internal/adapter/pgstore/projects_test.go`).
- **Project column order (use verbatim everywhere):** `id, owner_id, name, slug, color, glyph, description, upstream_git, status, created_at, updated_at, rate_amount, rate_currency`.
- `UpdateProject` is a **full replace** of the mutable fields (name, slug, color, glyph, description, upstreamGit, status). It does **not** touch `rate` (that has its own endpoint). Clients send the complete current state with edits applied.
- Every commit message ends with the trailer:
  `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`
- No emoji in code, output, or docs (project convention). Monospace glyphs are fine.
- Do not change the existing `CreateProject.Execute` signature or its callers (`worktime.go`, `webui.go`, `webui_worktime.go`, tests). Description/upstream at creation are applied by the REST handler composing `UpdateProject` (Task 5).

---

### Task 1: Domain — fields, `ProjectPaused`, `Validate()`, `ErrInvalidUpstream`

**Files:**
- Modify: `internal/domain/project.go`
- Modify: `internal/domain/errors.go`
- Test: `internal/domain/project_test.go`

**Interfaces:**
- Produces: `domain.Project` gains string fields `Description`, `UpstreamGit`; new const `domain.ProjectPaused ProjectStatus = "paused"`; new method `func (p Project) Validate() error`; new sentinel `domain.ErrInvalidUpstream`.

- [ ] **Step 1: Write the failing test**

Add to `internal/domain/project_test.go`:

```go
func TestProjectValidate(t *testing.T) {
	base := func() domain.Project {
		return domain.Project{Name: "Flow", Slug: "flow", Status: domain.ProjectActive}
	}
	t.Run("ok active/paused/archived", func(t *testing.T) {
		for _, st := range []domain.ProjectStatus{domain.ProjectActive, domain.ProjectPaused, domain.ProjectArchived} {
			p := base()
			p.Status = st
			if err := p.Validate(); err != nil {
				t.Errorf("status %q: unexpected error %v", st, err)
			}
		}
	})
	t.Run("missing name", func(t *testing.T) {
		p := base()
		p.Name = ""
		if !errors.Is(p.Validate(), domain.ErrInvalidProject) {
			t.Errorf("want ErrInvalidProject for empty name")
		}
	})
	t.Run("missing slug", func(t *testing.T) {
		p := base()
		p.Slug = ""
		if !errors.Is(p.Validate(), domain.ErrInvalidProject) {
			t.Errorf("want ErrInvalidProject for empty slug")
		}
	})
	t.Run("bad status", func(t *testing.T) {
		p := base()
		p.Status = "weird"
		if !errors.Is(p.Validate(), domain.ErrInvalidProject) {
			t.Errorf("want ErrInvalidProject for bad status")
		}
	})
}
```

Ensure the test file imports `errors` and `github.com/serverkraken/flow/internal/domain`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/ -run TestProjectValidate`
Expected: FAIL — `p.Validate undefined` / `ProjectPaused` not declared.

- [ ] **Step 3: Add the sentinel**

In `internal/domain/errors.go`, add to the `var (...)` block:

```go
	ErrInvalidUpstream = errors.New("invalid upstream git url")
```

- [ ] **Step 4: Add fields, status const, and Validate()**

In `internal/domain/project.go`, extend the const block:

```go
const (
	ProjectActive   ProjectStatus = "active"
	ProjectPaused   ProjectStatus = "paused"
	ProjectArchived ProjectStatus = "archived"
)
```

Add the two fields to the `Project` struct (place after `Glyph`):

```go
	Description string `json:"description"`
	UpstreamGit string `json:"upstreamGit"`
```

Add the method at the end of the file:

```go
// Validate checks the invariants enforced on every mutation: a project needs a
// name and slug, and its status must be one of the three known states.
func (p Project) Validate() error {
	switch {
	case p.Name == "":
		return fmt.Errorf("%w: name required", ErrInvalidProject)
	case p.Slug == "":
		return fmt.Errorf("%w: slug required", ErrInvalidProject)
	}
	switch p.Status {
	case ProjectActive, ProjectPaused, ProjectArchived:
		return nil
	default:
		return fmt.Errorf("%w: invalid status %q", ErrInvalidProject, p.Status)
	}
}
```

(`fmt` is already imported in `project.go`.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/domain/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/project.go internal/domain/errors.go internal/domain/project_test.go
git commit -m "feat(project-mgmt): Project gains description/upstream + paused + Validate"
```

---

### Task 2: Storage — migration 0014, pgstore columns + `Update`, port method, fake

**Files:**
- Create: `internal/adapter/pgstore/migrations/0014_project_description_upstream.sql`
- Modify: `internal/ports/ports.go` (ProjectStore interface)
- Modify: `internal/adapter/pgstore/projects.go`
- Modify: `internal/testutil/fakes.go`
- Test: `internal/adapter/pgstore/projects_test.go`

**Interfaces:**
- Consumes: `domain.Project.Description/UpstreamGit` (Task 1).
- Produces: `ports.ProjectStore.Update(ctx context.Context, ownerID string, p domain.Project) (domain.Project, error)`; the projects table has `description`/`upstream_git` columns; `testutil.FakeProjectStore.Update`.

- [ ] **Step 1: Write the migration**

Create `internal/adapter/pgstore/migrations/0014_project_description_upstream.sql`:

```sql
-- +goose Up
ALTER TABLE projects ADD COLUMN description  TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN upstream_git TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE projects DROP COLUMN upstream_git;
ALTER TABLE projects DROP COLUMN description;
```

- [ ] **Step 2: Add `Update` to the port + both implementations (keeps compile green)**

In `internal/ports/ports.go`, add to the `ProjectStore` interface (after `Get`):

```go
	// Update overwrites a project's mutable metadata (name, slug, color, glyph,
	// description, upstream_git, status). Rate is NOT touched (see SetRate).
	// Owner-scoped; returns ErrProjectNotFound for a missing or foreign project.
	Update(ctx context.Context, ownerID string, p domain.Project) (domain.Project, error)
```

In `internal/testutil/fakes.go`, add after `FakeProjectStore.Get`:

```go
func (s *FakeProjectStore) Update(_ context.Context, ownerID string, p domain.Project) (domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.m[p.ID]
	if !ok || existing.OwnerID != ownerID {
		return domain.Project{}, ports.ErrProjectNotFound
	}
	// mirror pgstore: rate is not mutated here
	existing.Name = p.Name
	existing.Slug = p.Slug
	existing.Color = p.Color
	existing.Glyph = p.Glyph
	existing.Description = p.Description
	existing.UpstreamGit = p.UpstreamGit
	existing.Status = p.Status
	existing.UpdatedAt = p.UpdatedAt
	s.m[p.ID] = existing
	return existing, nil
}
```

In `internal/adapter/pgstore/projects.go`, update the column lists in `Create`, `List`, `Get`, rewrite `scanProject`, and add `Update`. Replace the existing `Create`, `List`, `Get` query strings + the `Create` arg list and `scanProject` exactly as below:

`Create`:
```go
	const q = `
INSERT INTO projects (id, owner_id, name, slug, color, glyph, description, upstream_git, status, created_at, updated_at, rate_amount, rate_currency)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING id, owner_id, name, slug, color, glyph, description, upstream_git, status, created_at, updated_at, rate_amount, rate_currency`
	ra, rc := rateCols(p.Rate)
	return scanProject(s.pool.QueryRow(ctx, q,
		p.ID, p.OwnerID, p.Name, p.Slug, p.Color, p.Glyph, p.Description, p.UpstreamGit, string(p.Status), p.CreatedAt, p.UpdatedAt, ra, rc))
```

`List` query:
```go
	const q = `
SELECT id, owner_id, name, slug, color, glyph, description, upstream_git, status, created_at, updated_at, rate_amount, rate_currency
FROM projects WHERE owner_id=$1 ORDER BY name`
```

`Get` query:
```go
	const q = `
SELECT id, owner_id, name, slug, color, glyph, description, upstream_git, status, created_at, updated_at, rate_amount, rate_currency
FROM projects WHERE owner_id=$1 AND id=$2`
```

`scanProject`:
```go
func scanProject(r rowScanner) (domain.Project, error) {
	var p domain.Project
	var status string
	var ra *int64
	var rc *string
	if err := r.Scan(&p.ID, &p.OwnerID, &p.Name, &p.Slug, &p.Color, &p.Glyph, &p.Description, &p.UpstreamGit, &status, &p.CreatedAt, &p.UpdatedAt, &ra, &rc); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Project{}, err
		}
		return domain.Project{}, fmt.Errorf("pgstore: scan project: %w", err)
	}
	p.Status = domain.ProjectStatus(status)
	if (ra == nil) != (rc == nil) {
		return domain.Project{}, fmt.Errorf("pgstore: scan project: inconsistent rate columns (amount set=%v currency set=%v)", ra != nil, rc != nil)
	}
	if ra != nil && rc != nil {
		p.Rate = &domain.Money{Amount: *ra, Currency: *rc}
	}
	return p, nil
}
```

Add the `Update` method (place after `Get`):
```go
func (s *ProjectStore) Update(ctx context.Context, ownerID string, p domain.Project) (domain.Project, error) {
	const q = `
UPDATE projects SET name=$1, slug=$2, color=$3, glyph=$4, description=$5, upstream_git=$6, status=$7, updated_at=$8
WHERE owner_id=$9 AND id=$10
RETURNING id, owner_id, name, slug, color, glyph, description, upstream_git, status, created_at, updated_at, rate_amount, rate_currency`
	got, err := scanProject(s.pool.QueryRow(ctx, q,
		p.Name, p.Slug, p.Color, p.Glyph, p.Description, p.UpstreamGit, string(p.Status), p.UpdatedAt, ownerID, p.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Project{}, ports.ErrProjectNotFound
	}
	return got, err
}
```

- [ ] **Step 3: Write the failing pgstore test**

Add to `internal/adapter/pgstore/projects_test.go`:

```go
func TestProjectStore_UpdateRoundTrip(t *testing.T) {
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
	u, _ := domain.NewUser("u-upd", "sub-upd", "upduser", "upd@x.de", "Upd User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}

	st := pgstore.NewProjectStore(pool)
	now := time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)
	proj, _ := domain.NewProject("p-upd", "u-upd", "Acme", "acme", now)
	if _, err := st.Create(ctx, proj); err != nil {
		t.Fatal(err)
	}

	// Set a rate so we can prove Update preserves it.
	if err := st.SetRate(ctx, "u-upd", "p-upd", &domain.Money{Amount: 9000, Currency: "EUR"}); err != nil {
		t.Fatal(err)
	}

	upd := proj
	upd.Name = "Acme Reloaded"
	upd.Description = "# Notes\nhello"
	upd.UpstreamGit = "git@github.com:acme/reloaded.git"
	upd.Status = domain.ProjectPaused
	upd.UpdatedAt = now.Add(time.Hour)
	got, err := st.Update(ctx, "u-upd", upd)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Name != "Acme Reloaded" || got.Description != "# Notes\nhello" ||
		got.UpstreamGit != "git@github.com:acme/reloaded.git" || got.Status != domain.ProjectPaused {
		t.Errorf("Update returned %+v", got)
	}
	if got.Rate == nil || got.Rate.Amount != 9000 {
		t.Errorf("Update must preserve rate, got %+v", got.Rate)
	}

	// Re-read confirms persistence.
	re, err := st.Get(ctx, "u-upd", "p-upd")
	if err != nil || re.Status != domain.ProjectPaused || re.Description != "# Notes\nhello" {
		t.Errorf("Get after Update: %+v err=%v", re, err)
	}

	// Unknown id → ErrProjectNotFound.
	miss := upd
	miss.ID = "nope"
	if _, err := st.Update(ctx, "u-upd", miss); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Errorf("unknown id: want ErrProjectNotFound, got %v", err)
	}
}
```

- [ ] **Step 4: Run the storage test**

Run: `go test ./internal/adapter/pgstore/ -run TestProjectStore_UpdateRoundTrip`
Expected: PASS (spins up / connects to the test Postgres via `startPG`).

- [ ] **Step 5: Confirm the whole tree still compiles**

Run: `go build ./... && go test ./internal/testutil/ ./internal/domain/`
Expected: builds; tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/pgstore/migrations/0014_project_description_upstream.sql internal/ports/ports.go internal/adapter/pgstore/projects.go internal/testutil/fakes.go internal/adapter/pgstore/projects_test.go
git commit -m "feat(project-mgmt): persist description/upstream + ProjectStore.Update"
```

---

### Task 3: Usecase — `GetProject`

**Files:**
- Create: `internal/usecase/get_project.go`
- Test: `internal/usecase/get_project_test.go`

**Interfaces:**
- Produces: `usecase.GetProject{Projects ports.ProjectStore}` with `Execute(ctx, ownerID, id string) (domain.Project, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/usecase/get_project_test.go`:

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

func TestGetProject(t *testing.T) {
	ctx := context.Background()
	ps := testutil.NewFakeProjectStore()
	now := time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)
	p, _ := domain.NewProject("p1", "u1", "Flow", "flow", now)
	_, _ = ps.Create(ctx, p)

	uc := usecase.GetProject{Projects: ps}

	got, err := uc.Execute(ctx, "u1", "p1")
	if err != nil || got.Slug != "flow" {
		t.Fatalf("Execute: got %+v err=%v", got, err)
	}
	if _, err := uc.Execute(ctx, "u1", "missing"); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Errorf("missing: want ErrProjectNotFound, got %v", err)
	}
	if _, err := uc.Execute(ctx, "other", "p1"); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Errorf("foreign owner: want ErrProjectNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/usecase/ -run TestGetProject`
Expected: FAIL — `usecase.GetProject` undefined.

- [ ] **Step 3: Implement**

Create `internal/usecase/get_project.go`:

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// GetProject fetches a single owner-scoped project by id.
type GetProject struct {
	Projects ports.ProjectStore
}

func (uc GetProject) Execute(ctx context.Context, ownerID, id string) (domain.Project, error) {
	return uc.Projects.Get(ctx, ownerID, id)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/usecase/ -run TestGetProject`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/get_project.go internal/usecase/get_project_test.go
git commit -m "feat(project-mgmt): GetProject usecase"
```

---

### Task 4: Usecase — `UpdateProject` with upstream auto-sync

**Files:**
- Create: `internal/usecase/update_project.go`
- Test: `internal/usecase/update_project_test.go`

**Interfaces:**
- Consumes: `ports.ProjectStore.Update` (Task 2); `ports.ProjectBindingStore.Upsert`/`DeleteRemote` (existing); `domain.NormalizeRemoteSlug`, `domain.ErrInvalidUpstream`, `Project.Validate` (Task 1).
- Produces: `usecase.UpdateProjectInput{Name, Slug, Color, Glyph, Description, UpstreamGit string; Status domain.ProjectStatus}` and `usecase.UpdateProject{Projects, Bindings, IDs, Clock}` with `Execute(ctx, ownerID, id string, in UpdateProjectInput) (domain.Project, error)`.

- [ ] **Step 1: Write the failing tests (auto-sync matrix)**

Create `internal/usecase/update_project_test.go`:

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

func newUpdateUC() (usecase.UpdateProject, *testutil.FakeProjectStore, *testutil.FakeProjectBindingStore) {
	ps := testutil.NewFakeProjectStore()
	bs := testutil.NewFakeProjectBindingStore()
	uc := usecase.UpdateProject{
		Projects: ps, Bindings: bs,
		IDs:   &testutil.FakeIDGen{},
		Clock: testutil.FakeClock{T: time.Date(2026, 6, 23, 10, 0, 0, 0, time.UTC)},
	}
	return uc, ps, bs
}

func seedProj(t *testing.T, ps *testutil.FakeProjectStore, id, upstream string) {
	t.Helper()
	now := time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)
	p, _ := domain.NewProject(id, "u1", "Flow", "flow", now)
	p.UpstreamGit = upstream
	_, _ = ps.Create(context.Background(), p)
}

func baseInput() usecase.UpdateProjectInput {
	return usecase.UpdateProjectInput{Name: "Flow", Slug: "flow", Status: domain.ProjectActive}
}

func remoteSlugs(bs *testutil.FakeProjectBindingStore) []string {
	all, _ := bs.List(context.Background(), "u1")
	var out []string
	for _, b := range all {
		if b.Kind == domain.BindingRemote {
			out = append(out, b.RemoteSlug)
		}
	}
	return out
}

func TestUpdateProject_SetUpstreamCreatesBinding(t *testing.T) {
	uc, ps, bs := newUpdateUC()
	seedProj(t, ps, "p1", "")
	in := baseInput()
	in.UpstreamGit = "git@github.com:serverkraken/flow.git"
	got, err := uc.Execute(context.Background(), "u1", "p1", in)
	if err != nil {
		t.Fatal(err)
	}
	if got.UpstreamGit != in.UpstreamGit {
		t.Errorf("upstream not saved: %q", got.UpstreamGit)
	}
	if slugs := remoteSlugs(bs); len(slugs) != 1 || slugs[0] != "github.com/serverkraken/flow" {
		t.Errorf("want one remote binding github.com/serverkraken/flow, got %v", slugs)
	}
}

func TestUpdateProject_ClearUpstreamRemovesBinding(t *testing.T) {
	uc, ps, bs := newUpdateUC()
	seedProj(t, ps, "p1", "git@github.com:serverkraken/flow.git")
	// pre-create the matching binding
	_, _ = bs.Upsert(context.Background(), domain.ProjectBinding{
		ID: "b1", OwnerID: "u1", ProjectID: "p1",
		Kind: domain.BindingRemote, RemoteSlug: "github.com/serverkraken/flow",
	})
	in := baseInput() // UpstreamGit == ""
	if _, err := uc.Execute(context.Background(), "u1", "p1", in); err != nil {
		t.Fatal(err)
	}
	if slugs := remoteSlugs(bs); len(slugs) != 0 {
		t.Errorf("binding should be gone, got %v", slugs)
	}
}

func TestUpdateProject_ReassignUpstreamRepointsBinding(t *testing.T) {
	uc, ps, bs := newUpdateUC()
	seedProj(t, ps, "p1", "git@github.com:serverkraken/old.git")
	_, _ = bs.Upsert(context.Background(), domain.ProjectBinding{
		ID: "b1", OwnerID: "u1", ProjectID: "p1",
		Kind: domain.BindingRemote, RemoteSlug: "github.com/serverkraken/old",
	})
	in := baseInput()
	in.UpstreamGit = "https://github.com/serverkraken/new.git"
	if _, err := uc.Execute(context.Background(), "u1", "p1", in); err != nil {
		t.Fatal(err)
	}
	slugs := remoteSlugs(bs)
	if len(slugs) != 1 || slugs[0] != "github.com/serverkraken/new" {
		t.Errorf("want only github.com/serverkraken/new, got %v", slugs)
	}
}

func TestUpdateProject_InvalidUpstreamRejected(t *testing.T) {
	uc, ps, bs := newUpdateUC()
	seedProj(t, ps, "p1", "")
	in := baseInput()
	in.UpstreamGit = "not a url"
	if _, err := uc.Execute(context.Background(), "u1", "p1", in); !errors.Is(err, domain.ErrInvalidUpstream) {
		t.Fatalf("want ErrInvalidUpstream, got %v", err)
	}
	// nothing persisted, no binding
	got, _ := ps.Get(context.Background(), "u1", "p1")
	if got.UpstreamGit != "" {
		t.Errorf("upstream must not be persisted on reject, got %q", got.UpstreamGit)
	}
	if len(remoteSlugs(bs)) != 0 {
		t.Errorf("no binding expected on reject")
	}
}

func TestUpdateProject_BadStatusRejected(t *testing.T) {
	uc, ps, _ := newUpdateUC()
	seedProj(t, ps, "p1", "")
	in := baseInput()
	in.Status = "weird"
	if _, err := uc.Execute(context.Background(), "u1", "p1", in); !errors.Is(err, domain.ErrInvalidProject) {
		t.Fatalf("want ErrInvalidProject, got %v", err)
	}
}

func TestUpdateProject_NotFound(t *testing.T) {
	uc, _, _ := newUpdateUC()
	if _, err := uc.Execute(context.Background(), "u1", "missing", baseInput()); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Fatalf("want ErrProjectNotFound, got %v", err)
	}
}

func TestUpdateProject_DescriptionOnlyNoBindingChurn(t *testing.T) {
	uc, ps, bs := newUpdateUC()
	seedProj(t, ps, "p1", "")
	in := baseInput()
	in.Description = "just notes"
	if _, err := uc.Execute(context.Background(), "u1", "p1", in); err != nil {
		t.Fatal(err)
	}
	if len(remoteSlugs(bs)) != 0 {
		t.Errorf("description-only edit must not create a binding")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/usecase/ -run TestUpdateProject`
Expected: FAIL — `usecase.UpdateProject` undefined.

- [ ] **Step 3: Implement**

Create `internal/usecase/update_project.go`:

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// UpdateProjectInput is the full mutable field set of a project (rate excluded —
// see SetProjectRate). Update is a full replace: callers send current values.
type UpdateProjectInput struct {
	Name        string
	Slug        string
	Color       string
	Glyph       string
	Description string
	UpstreamGit string
	Status      domain.ProjectStatus
}

// UpdateProject overwrites a project's metadata and keeps the auto-managed
// remote binding in sync with its upstream git (set/clear/repoint).
type UpdateProject struct {
	Projects ports.ProjectStore
	Bindings ports.ProjectBindingStore
	IDs      ports.IDGen
	Clock    ports.Clock
}

func (uc UpdateProject) Execute(ctx context.Context, ownerID, id string, in UpdateProjectInput) (domain.Project, error) {
	cur, err := uc.Projects.Get(ctx, ownerID, id)
	if err != nil {
		return domain.Project{}, err
	}
	p := cur
	p.Name, p.Slug, p.Color, p.Glyph = in.Name, in.Slug, in.Color, in.Glyph
	p.Description, p.UpstreamGit, p.Status = in.Description, in.UpstreamGit, in.Status
	p.UpdatedAt = uc.Clock.Now()
	if err := p.Validate(); err != nil {
		return domain.Project{}, err
	}
	// Pre-validate the upstream so a bad URL rejects the whole update before any
	// write or binding mutation.
	var newSlug string
	if p.UpstreamGit != "" {
		s, ok := domain.NormalizeRemoteSlug(p.UpstreamGit)
		if !ok {
			return domain.Project{}, domain.ErrInvalidUpstream
		}
		newSlug = s
	}
	saved, err := uc.Projects.Update(ctx, ownerID, p)
	if err != nil {
		return domain.Project{}, err
	}
	if cur.UpstreamGit != p.UpstreamGit {
		if err := uc.syncRemoteBinding(ctx, ownerID, id, cur.UpstreamGit, newSlug); err != nil {
			return domain.Project{}, err
		}
	}
	return saved, nil
}

// syncRemoteBinding drops the previous upstream's remote binding (when it
// changed) and upserts the new one. newSlug == "" means the upstream was cleared.
func (uc UpdateProject) syncRemoteBinding(ctx context.Context, ownerID, projectID, oldURL, newSlug string) error {
	if oldSlug, ok := domain.NormalizeRemoteSlug(oldURL); ok && oldSlug != newSlug {
		if err := uc.Bindings.DeleteRemote(ctx, ownerID, oldSlug); err != nil {
			return err
		}
	}
	if newSlug == "" {
		return nil
	}
	now := uc.Clock.Now()
	_, err := uc.Bindings.Upsert(ctx, domain.ProjectBinding{
		ID:         uc.IDs.NewID(),
		OwnerID:    ownerID,
		ProjectID:  projectID,
		Kind:       domain.BindingRemote,
		RemoteSlug: newSlug,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/usecase/ -run TestUpdateProject`
Expected: PASS (all matrix cases).

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/update_project.go internal/usecase/update_project_test.go
git commit -m "feat(project-mgmt): UpdateProject usecase with upstream auto-sync"
```

---

### Task 5: REST — handlers, routes, event, status filter, compose-on-create, wiring

**Files:**
- Modify: `internal/domain/event.go` (add `EventProjectUpdated`)
- Modify: `internal/adapter/httpserver/server.go` (Server fields + routes)
- Modify: `internal/adapter/httpserver/worktime.go` (req structs, handlers, status filter)
- Modify: `cmd/flow-server/main.go` (wire the two new usecases)
- Test: `internal/adapter/httpserver/projects_test.go`

**Interfaces:**
- Consumes: `usecase.UpdateProject`, `usecase.GetProject` (Tasks 3–4); `domain.EventProjectUpdated`.
- Produces: routes `PATCH /api/v1/projects/{id}`, `GET /api/v1/projects/{id}`; `GET /api/v1/projects?status=` filter; POST accepts `description`/`upstreamGit`.

- [ ] **Step 1: Add the event type**

In `internal/domain/event.go`, add to the const block (after `EventProjectDeleted`):

```go
	EventProjectUpdated  EventType = "project.updated"
```

- [ ] **Step 2: Write failing handler tests**

Add to `internal/adapter/httpserver/projects_test.go`. Match the existing construction style there (`testutil.NewFakeProjectStore()`, `usecase.CreateProject{Projects: ps, IDs: ids, Clock: clk}`, an `httptest` server or direct handler call — follow whatever the file already does). The new test must wire `UpdateProject`, `GetProject`, and a `FakeProjectBindingStore`:

```go
func TestUpdateAndGetProjectRoutes(t *testing.T) {
	ps := testutil.NewFakeProjectStore()
	bs := testutil.NewFakeProjectBindingStore()
	ids := &testutil.FakeIDGen{}
	clk := testutil.FakeClock{T: time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)}
	srv := newTestServer(t, func(s *httpserver.Server) { // adapt to this file's helper
		s.CreateProject = usecase.CreateProject{Projects: ps, IDs: ids, Clock: clk}
		s.ListProjects = usecase.ListProjects{Projects: ps}
		s.GetProject = usecase.GetProject{Projects: ps}
		s.UpdateProject = usecase.UpdateProject{Projects: ps, Bindings: bs, IDs: ids, Clock: clk}
	})

	// create with an upstream → auto-synced remote binding
	created := doJSON(t, srv, "POST", "/api/v1/projects",
		`{"name":"Flow","upstreamGit":"git@github.com:serverkraken/flow.git"}`, 201)
	id := created["id"].(string)
	if got := remoteSlugs(bs); len(got) != 1 || got[0] != "github.com/serverkraken/flow" {
		t.Fatalf("create-with-upstream should auto-bind, got %v", got)
	}

	// GET one
	one := doJSON(t, srv, "GET", "/api/v1/projects/"+id, "", 200)
	if one["upstreamGit"] != "git@github.com:serverkraken/flow.git" {
		t.Errorf("GET returned %v", one)
	}

	// PATCH → pause + change description
	upd := doJSON(t, srv, "PATCH", "/api/v1/projects/"+id,
		`{"name":"Flow","slug":"flow","description":"hi","upstreamGit":"git@github.com:serverkraken/flow.git","status":"paused"}`, 200)
	if upd["status"] != "paused" || upd["description"] != "hi" {
		t.Errorf("PATCH returned %v", upd)
	}

	// PATCH bad upstream → 400
	doStatus(t, srv, "PATCH", "/api/v1/projects/"+id,
		`{"name":"Flow","slug":"flow","status":"active","upstreamGit":"garbage"}`, 400)

	// PATCH unknown id → 404
	doStatus(t, srv, "PATCH", "/api/v1/projects/missing",
		`{"name":"X","slug":"x","status":"active"}`, 404)
}

func TestListProjectsStatusFilter(t *testing.T) {
	ps := testutil.NewFakeProjectStore()
	ids := &testutil.FakeIDGen{}
	clk := testutil.FakeClock{T: time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)}
	bs := testutil.NewFakeProjectBindingStore()
	srv := newTestServer(t, func(s *httpserver.Server) {
		s.CreateProject = usecase.CreateProject{Projects: ps, IDs: ids, Clock: clk}
		s.ListProjects = usecase.ListProjects{Projects: ps}
		s.GetProject = usecase.GetProject{Projects: ps}
		s.UpdateProject = usecase.UpdateProject{Projects: ps, Bindings: bs, IDs: ids, Clock: clk}
	})
	a := doJSON(t, srv, "POST", "/api/v1/projects", `{"name":"Aaa"}`, 201)
	_ = doJSON(t, srv, "POST", "/api/v1/projects", `{"name":"Bbb"}`, 201)
	// archive Aaa
	doStatus(t, srv, "PATCH", "/api/v1/projects/"+a["id"].(string),
		`{"name":"Aaa","slug":"aaa","status":"archived"}`, 200)

	all := doJSONArray(t, srv, "GET", "/api/v1/projects", 200)
	if len(all) != 2 {
		t.Errorf("no filter → all, got %d", len(all))
	}
	arch := doJSONArray(t, srv, "GET", "/api/v1/projects?status=archived", 200)
	if len(arch) != 1 {
		t.Errorf("status=archived → 1, got %d", len(arch))
	}
	act := doJSONArray(t, srv, "GET", "/api/v1/projects?status=active,paused", 200)
	if len(act) != 1 {
		t.Errorf("status=active,paused → 1, got %d", len(act))
	}
}
```

NOTE for the implementer: `newTestServer`, `doJSON`, `doStatus`, `doJSONArray`, `remoteSlugs` are illustrative helper names — reuse the equivalents already present in the `httpserver` test package (e.g. the existing tests construct an `httptest.Server` and decode bodies). If a helper is missing, add a tiny local one; do not invent a framework.

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/adapter/httpserver/ -run 'TestUpdateAndGetProjectRoutes|TestListProjectsStatusFilter'`
Expected: FAIL — `s.UpdateProject`/`s.GetProject` undefined, routes 404.

- [ ] **Step 4: Add Server fields**

In `internal/adapter/httpserver/server.go`, add to the `Server` struct near the other project usecases:

```go
	UpdateProject usecase.UpdateProject
	GetProject    usecase.GetProject
```

- [ ] **Step 5: Register routes**

In `internal/adapter/httpserver/server.go`, after the existing `DELETE /api/v1/projects/{id}` line, add:

```go
	mux.Handle("GET /api/v1/projects/{id}", s.auth(http.HandlerFunc(s.handleGetProject)))
	mux.Handle("PATCH /api/v1/projects/{id}", s.auth(http.HandlerFunc(s.handleUpdateProject)))
```

NOTE: Go 1.22 `ServeMux` resolves overlap by specificity, so the literal routes `GET /api/v1/projects/resolve` and `GET /api/v1/projects/bindings` still win over `GET /api/v1/projects/{id}` — no conflict, no panic.

- [ ] **Step 6: Extend req struct + handlers + filter**

In `internal/adapter/httpserver/worktime.go`:

Add `"strings"` to the import block.

Extend `createProjReq` (add two fields):

```go
	Description string `json:"description"`
	UpstreamGit string `json:"upstreamGit"`
```

Replace `handleCreateProject` body with the compose-on-create version:

```go
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req createProjReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	// Reject a bad upstream up front so we never create a half-configured project.
	if req.UpstreamGit != "" {
		if _, ok := domain.NormalizeRemoteSlug(req.UpstreamGit); !ok {
			http.Error(w, "invalid upstream git url", http.StatusBadRequest)
			return
		}
	}
	p, err := s.CreateProject.Execute(r.Context(), u.ID, req.Name, req.Slug, req.Color, req.Glyph)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	// Apply optional description/upstream (auto-syncs the remote binding).
	if req.Description != "" || req.UpstreamGit != "" {
		p, err = s.UpdateProject.Execute(r.Context(), u.ID, p.ID, usecase.UpdateProjectInput{
			Name: p.Name, Slug: p.Slug, Color: p.Color, Glyph: p.Glyph,
			Description: req.Description, UpstreamGit: req.UpstreamGit, Status: p.Status,
		})
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
	}
	s.Bus.Publish(domain.Event{Type: domain.EventProjectCreated, UserID: u.ID, Data: map[string]any{"id": p.ID}})
	writeJSON(w, http.StatusCreated, p)
}
```

Replace `handleListProjects` to apply the filter and add the helper:

```go
func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	list, err := s.ListProjects.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	list = filterProjectsByStatus(list, r.URL.Query().Get("status"))
	if list == nil {
		list = []domain.Project{}
	}
	writeJSON(w, http.StatusOK, list)
}

// filterProjectsByStatus keeps projects whose status is in the comma-separated
// `status` query (e.g. "active,paused"). Empty query → all (backward compatible).
func filterProjectsByStatus(in []domain.Project, status string) []domain.Project {
	status = strings.TrimSpace(status)
	if status == "" {
		return in
	}
	want := map[string]bool{}
	for _, s := range strings.Split(status, ",") {
		if s = strings.TrimSpace(s); s != "" {
			want[s] = true
		}
	}
	var out []domain.Project
	for _, p := range in {
		if want[string(p.Status)] {
			out = append(out, p)
		}
	}
	return out
}
```

Add the two new handlers + the update request struct (place near the other project handlers):

```go
func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	p, err := s.GetProject.Execute(r.Context(), u.ID, r.PathValue("id"))
	switch {
	case errors.Is(err, ports.ErrProjectNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type updateProjReq struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Color       string `json:"color"`
	Glyph       string `json:"glyph"`
	Description string `json:"description"`
	UpstreamGit string `json:"upstreamGit"`
	Status      string `json:"status"`
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req updateProjReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	p, err := s.UpdateProject.Execute(r.Context(), u.ID, r.PathValue("id"), usecase.UpdateProjectInput{
		Name: req.Name, Slug: req.Slug, Color: req.Color, Glyph: req.Glyph,
		Description: req.Description, UpstreamGit: req.UpstreamGit,
		Status: domain.ProjectStatus(req.Status),
	})
	switch {
	case errors.Is(err, ports.ErrProjectNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case errors.Is(err, domain.ErrInvalidProject) || errors.Is(err, domain.ErrInvalidUpstream):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventProjectUpdated, UserID: u.ID, Data: map[string]any{"id": p.ID}})
	writeJSON(w, http.StatusOK, p)
}
```

- [ ] **Step 7: Wire the composition root**

In `cmd/flow-server/main.go`, add the two usecases to the `Server` construction near the other project usecases (use the existing `projectStore`, `bindingStore`, `ids`, `clock` locals):

```go
		UpdateProject: usecase.UpdateProject{Projects: projectStore, Bindings: bindingStore, IDs: ids, Clock: clock},
		GetProject:    usecase.GetProject{Projects: projectStore},
```

- [ ] **Step 8: Run handler tests + full build**

Run: `go test ./internal/adapter/httpserver/ -run 'TestUpdateAndGetProjectRoutes|TestListProjectsStatusFilter' && go build ./...`
Expected: PASS; builds.

- [ ] **Step 9: Commit**

```bash
git add internal/domain/event.go internal/adapter/httpserver/server.go internal/adapter/httpserver/worktime.go cmd/flow-server/main.go internal/adapter/httpserver/projects_test.go
git commit -m "feat(project-mgmt): REST update/get + status filter + create autosync + wiring"
```

---

### Task 6: apiclient — `GetProject` + `UpdateProject`

**Files:**
- Modify: `internal/adapter/apiclient/client.go`
- Test: `internal/adapter/apiclient/client_test.go` (or the existing apiclient test file — match what's there)

**Interfaces:**
- Produces: `Client.GetProject(ctx, id string) (domain.Project, error)`; `apiclient.UpdateProjectFields` struct; `Client.UpdateProject(ctx, id string, in UpdateProjectFields) (domain.Project, error)`.

- [ ] **Step 1: Write the failing test**

Add a test that stands up an `httptest.Server` asserting method+path+body and returning a project. Follow the existing apiclient test style in this package (the package already tests `do`-based methods). Minimal example:

```go
func TestClientUpdateAndGetProject(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_ = json.NewEncoder(w).Encode(domain.Project{ID: "p1", Name: "Flow", Slug: "flow", Status: domain.ProjectPaused, UpstreamGit: "git@github.com:serverkraken/flow.git"})
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")

	got, err := c.UpdateProject(context.Background(), "p1", apiclient.UpdateProjectFields{
		Name: "Flow", Slug: "flow", Status: "paused", UpstreamGit: "git@github.com:serverkraken/flow.git",
	})
	if err != nil || got.Status != domain.ProjectPaused {
		t.Fatalf("UpdateProject: %+v err=%v", got, err)
	}
	if gotMethod != "PATCH" || gotPath != "/api/v1/projects/p1" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if !strings.Contains(gotBody, `"status":"paused"`) {
		t.Errorf("body missing status: %s", gotBody)
	}

	one, err := c.GetProject(context.Background(), "p1")
	if err != nil || one.Slug != "flow" {
		t.Fatalf("GetProject: %+v err=%v", one, err)
	}
	if gotMethod != "GET" || gotPath != "/api/v1/projects/p1" {
		t.Errorf("GET method/path = %s %s", gotMethod, gotPath)
	}
}
```

Ensure imports: `context`, `encoding/json`, `io`, `net/http`, `net/http/httptest`, `strings`, `testing`, the `apiclient` and `domain` packages.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/adapter/apiclient/ -run TestClientUpdateAndGetProject`
Expected: FAIL — methods undefined.

- [ ] **Step 3: Implement**

In `internal/adapter/apiclient/client.go`, add after `DeleteProject`:

```go
func (c *Client) GetProject(ctx context.Context, id string) (domain.Project, error) {
	var p domain.Project
	err := c.do(ctx, http.MethodGet, "/api/v1/projects/"+id, nil, &p)
	return p, err
}

// UpdateProjectFields are the mutable project fields (full replace; rate has its
// own endpoint). JSON tags match the server's updateProjReq.
type UpdateProjectFields struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Color       string `json:"color"`
	Glyph       string `json:"glyph"`
	Description string `json:"description"`
	UpstreamGit string `json:"upstreamGit"`
	Status      string `json:"status"`
}

func (c *Client) UpdateProject(ctx context.Context, id string, in UpdateProjectFields) (domain.Project, error) {
	var p domain.Project
	err := c.do(ctx, http.MethodPatch, "/api/v1/projects/"+id, in, &p)
	return p, err
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/adapter/apiclient/ -run TestClientUpdateAndGetProject`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/apiclient/client.go internal/adapter/apiclient/client_test.go
git commit -m "feat(project-mgmt): apiclient GetProject + UpdateProject"
```

---

### Task 7: MCP enrichment — status + upstream in context and list

**Files:**
- Modify: `cmd/flow-mcp/tools_project.go` (`projectContext` message)
- Modify: `cmd/flow-mcp/format.go` (`formatProjects` line)
- Test: `cmd/flow-mcp/format_test.go` (or wherever `formatProjects` is tested; otherwise add a small test)

**Interfaces:**
- Consumes: `domain.Project.Status`, `domain.Project.UpstreamGit` (Task 1).

- [ ] **Step 1: Write the failing test**

Add to the MCP test package:

```go
func TestFormatProjectsIncludesStatus(t *testing.T) {
	out := formatProjects([]domain.Project{
		{ID: "p1", Name: "Flow", Slug: "flow", Status: domain.ProjectPaused, UpstreamGit: "git@github.com:serverkraken/flow.git"},
	})
	if !strings.Contains(out, "paused") {
		t.Errorf("formatProjects must include status, got %q", out)
	}
	if !strings.Contains(out, "github.com") {
		t.Errorf("formatProjects must include upstream, got %q", out)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./cmd/flow-mcp/ -run TestFormatProjectsIncludesStatus`
Expected: FAIL.

- [ ] **Step 3: Implement the formatProjects line**

In `cmd/flow-mcp/format.go`, replace the loop body of `formatProjects`:

```go
	for _, p := range sorted {
		line := fmt.Sprintf("%s (%s) — %s — %s", p.Name, p.Slug, p.Status, p.ID)
		if p.UpstreamGit != "" {
			line += " — " + p.UpstreamGit
		}
		b.WriteString(line + "\n")
	}
```

- [ ] **Step 4: Enrich projectContext**

In `cmd/flow-mcp/tools_project.go`, replace the `msg :=` line in `projectContext`:

```go
	msg := fmt.Sprintf("Project: %s (%s) — status %s — %d document(s) in scope.", proj.Name, proj.Slug, proj.Status, count)
	if proj.UpstreamGit != "" {
		msg += " Upstream: " + proj.UpstreamGit + "."
	}
	msg += " Resolved for this working directory."
```

- [ ] **Step 5: Run to verify pass**

Run: `go test ./cmd/flow-mcp/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/flow-mcp/format.go cmd/flow-mcp/tools_project.go cmd/flow-mcp/format_test.go
git commit -m "feat(project-mgmt): MCP reports project status + upstream"
```

---

### Task 8: Wiring verification + live done-gate

**Files:** none (verification only). If a gap is found, fix it in the relevant task's files and re-commit.

- [ ] **Step 1: Full CI gate**

Run: `make ci`
Expected: lint + templ + build + tests green, coverage ≥ gate (~80%). Fix anything red before proceeding.

- [ ] **Step 2: Confirm the composition root calls the new constructors**

Run: `rg -n "UpdateProject: usecase.UpdateProject|GetProject:\s+usecase.GetProject" cmd/flow-server/main.go`
Expected: both lines present (catches the "routes registered but usecase never wired" class of bug).

- [ ] **Step 3: Bring up the dev stack**

Run (per `reference_flow_dev_env`): `make dev-up` then `make dev-run` (in a second shell), and `make dev-token` to mint a token. Export the base URL and token, e.g.:

```bash
export TOKEN="$(make -s dev-token)"
export BASE="https://localhost:8443"   # adjust to the dev stack's address
alias fcurl='curl -sk -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json"'
```

- [ ] **Step 4: Smoke each new route + prove auto-sync**

```bash
# create with an upstream
fcurl -X POST "$BASE/api/v1/projects" \
  -d '{"name":"PM Smoke","upstreamGit":"git@github.com:serverkraken/pmsmoke.git"}'
# capture the id from the response, then:
PID=<id-from-above>

# GET one
fcurl "$BASE/api/v1/projects/$PID"

# the auto-synced remote binding exists
fcurl "$BASE/api/v1/projects/$PID/bindings"      # expect remote_slug github.com/serverkraken/pmsmoke

# resolution finds it by the upstream slug WITHOUT any manual bind
fcurl "$BASE/api/v1/projects/resolve?slug=github.com/serverkraken/pmsmoke"   # expect the project, 200

# edit: pause + description (full replace)
fcurl -X PATCH "$BASE/api/v1/projects/$PID" \
  -d '{"name":"PM Smoke","slug":"pm-smoke","description":"hello","upstreamGit":"git@github.com:serverkraken/pmsmoke.git","status":"paused"}'

# status filter
fcurl "$BASE/api/v1/projects?status=archived"     # expect [] (none archived yet)
fcurl "$BASE/api/v1/projects?status=active,paused" # expect the project listed

# clear upstream → binding removed
fcurl -X PATCH "$BASE/api/v1/projects/$PID" \
  -d '{"name":"PM Smoke","slug":"pm-smoke","status":"paused"}'
fcurl "$BASE/api/v1/projects/$PID/bindings"        # expect the remote binding gone

# bad upstream → 400, nothing created
fcurl -o /dev/null -w "%{http_code}\n" -X POST "$BASE/api/v1/projects" -d '{"name":"Bad","upstreamGit":"garbage"}'  # expect 400
```

Expected: every call behaves as annotated; the resolve call returning the project after only setting the upstream is the headline proof of auto-sync.

- [ ] **Step 5: Final commit (only if Step 1–4 required fixes)**

```bash
git add -A
git commit -m "chore(project-mgmt): slice-1 done-gate fixes"
```

---

## Self-Review

**1. Spec coverage (Slice 1 scope):**
- description + upstream fields → Task 1, 2. ✓
- paused status → Task 1 (const + Validate). ✓
- UpdateProject usecase + PATCH route → Task 4, 5. ✓
- GetProject + GET /projects/{id} → Task 3, 5. ✓
- ProjectStore.Update + migration 0014 → Task 2. ✓
- apiclient UpdateProject + GetProject → Task 6. ✓
- auto-sync remote binding (set/clear/repoint/invalid) → Task 4 (+ create compose Task 5). ✓
- `?status=` filter → Task 5. ✓
- MCP enrichment → Task 7. ✓
- main wiring + done-gate → Task 5 (wire) + Task 8 (verify). ✓
- EventProjectUpdated → Task 5. ✓

**2. Placeholder scan:** All code steps carry real code. The only named-but-not-shown items are the existing `httpserver`/`apiclient` test helpers (`newTestServer`, `doJSON`, `startPG`, etc.) — flagged explicitly as "reuse the file's existing helpers," not invented APIs.

**3. Type consistency:** `UpdateProjectInput` (usecase, `Status domain.ProjectStatus`) vs `updateProjReq`/`UpdateProjectFields` (REST/apiclient, `Status string`) — the boundary converts via `domain.ProjectStatus(req.Status)` in the handler (Task 5). Column order identical across Create/List/Get/Update/scanProject (Global Constraints). `ProjectStore.Update(ctx, ownerID, p)` signature identical in port, pgstore, and fake. Auto-sync uses the existing `ProjectBindingStore.Upsert`/`DeleteRemote` exactly.

**Deviation from spec (deliberate):** the REST list default is **all** (not active+paused) so session/document name-resolution never loses archived projects; the "active+paused default view" is a UI concern handled by the WebUI/TUI requesting `?status=active,paused`. Recorded here so a reviewer doesn't flag it as a miss.
