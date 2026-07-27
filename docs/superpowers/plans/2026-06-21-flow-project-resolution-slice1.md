# Project Resolution V0 — Slice 1 (override + git-remote) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Resolve "which flow Project is this cwd?" via `FLOW_PROJECT` env override
and the repo's **git remote slug** (worktree- and device-invariant), with a
pick-or-create `flow project bind` CLI and a read-only WebUI panel.

**Architecture:** Hexagonal (domain → usecase → ports → adapters). A discriminated
`project_bindings` table stores `kind='remote'` rows now (the table also has the
`path` columns, unused until Slice 2). Pure cores (`NormalizeRemoteSlug`,
`ResolveBinding`) carry the logic; a client orchestrator (`projectresolve`) runs
the chain `FLOW_PROJECT → git-remote → (server resolve)`.

**Tech Stack:** Go, pgx/pgxpool, goose migrations, net/http mux, cobra,
bubbletea/v2 (`ui/fuzzylist` picker), templ, `github.com/google/uuid`.

## Global Constraints

- Module `github.com/serverkraken/flow`. Hexagonal layering; one responsibility per file (no monoliths).
- pgstore migrations need `-- +goose Up` / `-- +goose Down` annotations.
- Project identified server-side by `{id}`; the CLI resolves slug→id via the existing `resolveSlug` (cmd/flow/projects.go).
- Owner-scoped everywhere (`u, _ := userFrom(r.Context())`, store methods take `ownerID`).
- This slice writes only `kind='remote'` bindings. The path tier (`kind='path'`, machine-id, longest-prefix) is **Slice 2** — do not implement it here, but the table/types must already carry the path columns/fields so Slice 2 is purely additive.
- Ends with `make ci` green (~80% gate). Run the FULL `make ci` (lint incl.), not just `go test`.
- Spec: `docs/superpowers/specs/2026-06-21-flow-project-resolution-design.md`.

## File structure (Slice 1 touches)

- Create `internal/domain/remoteslug.go`, `internal/domain/projectbinding.go`
- Modify `internal/ports/ports.go` (add `ProjectBindingStore` + `ErrBindingNotFound`)
- Modify `internal/testutil/fakes.go` (add `FakeProjectBindingStore`)
- Create `internal/usecase/{bind_project,unbind_project,resolve_project,list_project_bindings}.go`
- Create `internal/adapter/pgstore/projectbindings.go` + `internal/adapter/pgstore/migrations/0011_project_bindings.sql`
- Create `internal/adapter/httpserver/projectbindings.go`; modify `server.go` (Server fields + routes)
- Create `internal/adapter/apiclient/projectbindings.go`
- Create `internal/gitremote/gitremote.go`
- Create `internal/projectresolve/resolve.go`
- Modify `cmd/flow/project.go` (+ `cmd/flow/projectbind.go` for the picker)
- Modify webui worktime/project templ + its handler
- Modify `cmd/flow-server/main.go` (wire store + usecases into Server)

> Confirm the migrations directory path first: `fd 0010 internal/adapter/pgstore` — put `0011_project_bindings.sql` beside it.

---

### Task 1: `domain.NormalizeRemoteSlug` (pure)

**Files:** Create `internal/domain/remoteslug.go`; Test `internal/domain/remoteslug_test.go`

**Interfaces:** Produces `func NormalizeRemoteSlug(url string) (slug string, ok bool)`.

- [ ] **Step 1: Write the failing test**

```go
package domain

import "testing"

func TestNormalizeRemoteSlug(t *testing.T) {
	cases := map[string]string{
		"git@github.com:serverkraken/flow.git":          "github.com/serverkraken/flow",
		"git@github.com:serverkraken/flow":              "github.com/serverkraken/flow",
		"ssh://git@github.com/serverkraken/flow.git":    "github.com/serverkraken/flow",
		"https://github.com/serverkraken/flow.git":      "github.com/serverkraken/flow",
		"https://user@gitlab.com:8443/a/b/c.git/":       "gitlab.com/a/b/c",
		"https://github.com/Serverkraken/Flow":          "github.com/serverkraken/flow", // case-folded
	}
	for in, want := range cases {
		got, ok := NormalizeRemoteSlug(in)
		if !ok || got != want {
			t.Errorf("NormalizeRemoteSlug(%q) = %q,%v want %q,true", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "   ", "not a url", "https://"} {
		if got, ok := NormalizeRemoteSlug(bad); ok {
			t.Errorf("NormalizeRemoteSlug(%q) = %q,true want ok=false", bad, got)
		}
	}
}
```

- [ ] **Step 2: Run → FAIL** `go test ./internal/domain/ -run TestNormalizeRemoteSlug -v` (undefined).

- [ ] **Step 3: Implement** `internal/domain/remoteslug.go`:

```go
package domain

import (
	"regexp"
	"strings"
)

// scpLike matches git's scp form: [user@]host:path
var scpLike = regexp.MustCompile(`^(?:[^@/]+@)?([^/:]+):(.+)$`)

// NormalizeRemoteSlug turns any git remote URL form into a stable, lowercased
// "host/path" slug, or ok=false when it can't be parsed.
func NormalizeRemoteSlug(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	var host, path string
	switch {
	case strings.Contains(s, "://"): // scheme form
		rest := s[strings.Index(s, "://")+3:]
		if at := strings.LastIndex(rest, "@"); at >= 0 { // strip credentials
			rest = rest[at+1:]
		}
		slash := strings.Index(rest, "/")
		if slash < 0 {
			return "", false
		}
		host, path = rest[:slash], rest[slash+1:]
	case scpLike.MatchString(s): // git@host:path
		m := scpLike.FindStringSubmatch(s)
		host, path = m[1], m[2]
	default:
		return "", false
	}
	if i := strings.Index(host, ":"); i >= 0 { // strip port
		host = host[:i]
	}
	path = strings.TrimSuffix(strings.TrimSuffix(strings.Trim(path, "/"), ".git"), "/")
	if host == "" || path == "" {
		return "", false
	}
	return strings.ToLower(host + "/" + path), true
}
```

- [ ] **Step 4: Run → PASS.** **Step 5: Commit** `feat(domain): NormalizeRemoteSlug — stable git-remote identity`.

---

### Task 2: `domain.ProjectBinding` + `ResolveBinding` (remote tier)

**Files:** Create `internal/domain/projectbinding.go`; Test `internal/domain/projectbinding_test.go`

**Interfaces:**
- Produces `type BindingKind string` (`BindingRemote="remote"`, `BindingPath="path"`).
- Produces `type ProjectBinding struct{ ID,OwnerID,ProjectID string; Kind BindingKind; RemoteSlug,MachineID,MachineLabel,Path string; CreatedAt,UpdatedAt time.Time }`.
- Produces `func ResolveBinding(bs []ProjectBinding, remoteSlug, machineID, cwd string) (ProjectBinding, bool)`. **Slice 1: matches only `kind=remote` by `remoteSlug`.** Slice 2 adds the path branch.

- [ ] **Step 1: Failing test**

```go
package domain

import "testing"

func TestResolveBinding_Remote(t *testing.T) {
	bs := []ProjectBinding{
		{ProjectID: "p1", Kind: BindingRemote, RemoteSlug: "github.com/a/flow"},
		{ProjectID: "p2", Kind: BindingRemote, RemoteSlug: "github.com/a/other"},
	}
	got, ok := ResolveBinding(bs, "github.com/a/flow", "m1", "/whatever")
	if !ok || got.ProjectID != "p1" {
		t.Fatalf("remote match = %+v,%v want p1", got, ok)
	}
	if _, ok := ResolveBinding(bs, "github.com/a/nope", "m1", "/x"); ok {
		t.Fatal("unknown remote must not match")
	}
	if _, ok := ResolveBinding(bs, "", "m1", "/x"); ok {
		t.Fatal("empty remote must not match a remote binding")
	}
}
```

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement** `internal/domain/projectbinding.go`:

```go
package domain

import "time"

type BindingKind string

const (
	BindingRemote BindingKind = "remote"
	BindingPath   BindingKind = "path"
)

type ProjectBinding struct {
	ID, OwnerID, ProjectID string
	Kind                   BindingKind
	RemoteSlug             string // kind=remote
	MachineID, MachineLabel, Path string // kind=path (Slice 2)
	CreatedAt, UpdatedAt   time.Time
}

// ResolveBinding returns the project binding for the given context. A remote
// match (by remoteSlug) wins. The path tier (machineID/cwd longest-prefix) is
// added in Slice 2.
func ResolveBinding(bs []ProjectBinding, remoteSlug, machineID, cwd string) (ProjectBinding, bool) {
	if remoteSlug != "" {
		for _, b := range bs {
			if b.Kind == BindingRemote && b.RemoteSlug == remoteSlug {
				return b, true
			}
		}
	}
	// Slice 2: else longest-prefix path match for machineID over cwd.
	return ProjectBinding{}, false
}
```

- [ ] **Step 4: Run → PASS.** **Step 5: Commit** `feat(domain): ProjectBinding + ResolveBinding (remote tier)`.

---

### Task 3: `ports.ProjectBindingStore` + fake

**Files:** Modify `internal/ports/ports.go`; Modify `internal/testutil/fakes.go`; Test `internal/testutil/fakes_test.go` (extend)

**Interfaces (Produces):**
```go
// in ports.go
var ErrBindingNotFound = errors.New("ports: binding not found")
type ProjectBindingStore interface {
	Upsert(ctx context.Context, b domain.ProjectBinding) (domain.ProjectBinding, error) // by (owner,kind-key)
	DeleteRemote(ctx context.Context, ownerID, remoteSlug string) error
	DeletePath(ctx context.Context, ownerID, machineID, path string) error
	List(ctx context.Context, ownerID string) ([]domain.ProjectBinding, error)
	ListByProject(ctx context.Context, ownerID, projectID string) ([]domain.ProjectBinding, error)
}
```

- [ ] **Step 1: Failing test** (`internal/testutil/fakes_test.go`): upsert a remote binding, `List` returns it; re-`Upsert` same `(owner,remote_slug)` with a different `projectID` reassigns (still one row); `DeleteRemote` removes it.

```go
func TestFakeProjectBindingStore_UpsertReassignDelete(t *testing.T) {
	s := testutil.NewFakeProjectBindingStore()
	ctx := context.Background()
	_, _ = s.Upsert(ctx, domain.ProjectBinding{ID: "b1", OwnerID: "u", ProjectID: "p1", Kind: domain.BindingRemote, RemoteSlug: "r"})
	_, _ = s.Upsert(ctx, domain.ProjectBinding{ID: "b2", OwnerID: "u", ProjectID: "p2", Kind: domain.BindingRemote, RemoteSlug: "r"}) // reassign
	got, _ := s.List(ctx, "u")
	if len(got) != 1 || got[0].ProjectID != "p2" {
		t.Fatalf("reassign: %+v", got)
	}
	if err := s.DeleteRemote(ctx, "u", "r"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.List(ctx, "u"); len(got) != 0 {
		t.Fatalf("after delete: %+v", got)
	}
}
```

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement** the interface in `ports.go` (add `ErrBindingNotFound` + the interface; `errors` is already imported there) and `FakeProjectBindingStore` in `fakes.go` (mirror `FakeProjectStore` shape: a `sync.Mutex` + slice; `Upsert` replaces an existing row with the same kind-key, else appends; `DeleteRemote/DeletePath` filter; `List`/`ListByProject` copy out). Owner-scope every read.

- [ ] **Step 4: Run → PASS.** **Step 5: Commit** `feat(ports): ProjectBindingStore + fake`.

---

### Task 4: usecases (bind / unbind / resolve / list)

**Files:** Create `internal/usecase/{bind_project,unbind_project,resolve_project,list_project_bindings}.go`; Test each `_test.go`

**Interfaces (Produces):**
```go
type BindKey struct { Kind domain.BindingKind; RemoteSlug, MachineID, MachineLabel, Path string }
type BindProject struct { Bindings ports.ProjectBindingStore; Projects ports.ProjectStore; IDs ports.IDGen; Clock ports.Clock }
func (uc BindProject) Execute(ctx, ownerID, projectID string, k BindKey) (domain.ProjectBinding, error) // validates project exists (Projects.Get), builds binding, Upsert
type ResolveProject struct { Bindings ports.ProjectBindingStore; Projects ports.ProjectStore }
func (uc ResolveProject) Execute(ctx, ownerID, remoteSlug, machineID, cwd string) (domain.Project, bool, error) // List → domain.ResolveBinding → Projects.Get
type UnbindProject struct { Bindings ports.ProjectBindingStore }
func (uc UnbindProject) Execute(ctx, ownerID string, k BindKey) error // DeleteRemote or DeletePath by kind
type ListProjectBindings struct { Bindings ports.ProjectBindingStore }
func (uc ListProjectBindings) Execute(ctx, ownerID string) ([]domain.ProjectBinding, error)
```

- [ ] **Step 1: Failing tests** (fake stores). Key cases:
  - `BindProject` with an unknown `projectID` → error (Projects.Get returns `ErrProjectNotFound`); with a known project → Upsert called, returns the binding.
  - `ResolveProject` with a matching remote → returns the project; no match → `(Project{}, false, nil)`.
  - `UnbindProject` remote → `DeleteRemote` called.

```go
func TestBindProject_RemoteHappyAndUnknownProject(t *testing.T) {
	ps := testutil.NewFakeProjectStore(); bs := testutil.NewFakeProjectBindingStore()
	clk := testutil.FakeClock{T: time.Unix(0,0)}; ids := &testutil.FakeIDGen{}
	p, _ := ps.Create(context.Background(), domain.Project{ID:"p1",OwnerID:"u",Slug:"flow"})
	uc := usecase.BindProject{Bindings: bs, Projects: ps, IDs: ids, Clock: clk}
	b, err := uc.Execute(context.Background(), "u", p.ID, usecase.BindKey{Kind: domain.BindingRemote, RemoteSlug: "github.com/a/flow"})
	if err != nil || b.RemoteSlug != "github.com/a/flow" { t.Fatalf("happy: %+v %v", b, err) }
	if _, err := uc.Execute(context.Background(), "u", "nope", usecase.BindKey{Kind: domain.BindingRemote, RemoteSlug: "x"}); err == nil {
		t.Fatal("unknown project must error")
	}
}
```

- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the four usecase files (one per file). `BindProject.Execute` calls `uc.Projects.Get(ctx, ownerID, projectID)` first (propagating `ErrProjectNotFound`), then builds `domain.ProjectBinding{ID: uc.IDs.NewID(), OwnerID: ownerID, ProjectID: projectID, Kind: k.Kind, RemoteSlug: k.RemoteSlug, ...Path fields..., CreatedAt/UpdatedAt: uc.Clock.Now()}` and `uc.Bindings.Upsert`. `ResolveProject.Execute` lists bindings, calls `domain.ResolveBinding`, on hit returns `uc.Projects.Get(ctx, ownerID, b.ProjectID)`.
- [ ] **Step 4: Run → PASS.** **Step 5: Commit** `feat(usecase): project binding bind/unbind/resolve/list`.

---

### Task 5: pgstore + migration 0011

**Files:** Create `internal/adapter/pgstore/migrations/0011_project_bindings.sql`; Create `internal/adapter/pgstore/projectbindings.go`; Test `internal/adapter/pgstore/projectbindings_test.go` (Docker pgstore test, like the existing ones)

**Interfaces:** Produces `pgstore.ProjectBindingStore` implementing `ports.ProjectBindingStore`; `NewProjectBindingStore(pool)`.

- [ ] **Step 1: Migration** (verbatim from the spec's data-model SQL, with goose annotations):

```sql
-- +goose Up
CREATE TABLE project_bindings (
  id            TEXT PRIMARY KEY,
  owner_id      TEXT NOT NULL,
  project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  kind          TEXT NOT NULL CHECK (kind IN ('remote','path')),
  remote_slug   TEXT,
  machine_id    TEXT,
  machine_label TEXT,
  path          TEXT,
  created_at    TIMESTAMPTZ NOT NULL,
  updated_at    TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX project_bindings_remote ON project_bindings (owner_id, remote_slug) WHERE kind='remote';
CREATE UNIQUE INDEX project_bindings_path   ON project_bindings (owner_id, machine_id, path) WHERE kind='path';
CREATE INDEX project_bindings_owner ON project_bindings (owner_id);

-- +goose Down
DROP TABLE IF EXISTS project_bindings;
```

- [ ] **Step 2: Failing Docker test** (mirror an existing `*_test.go` in pgstore for the harness/skip-guard): upsert a remote binding; re-upsert same `(owner,remote_slug)` other project → still one row, reassigned (via `ON CONFLICT` on the partial unique); `List` owner-scoped; `DeleteRemote` removes; cascade when the project is deleted.

- [ ] **Step 3: Implement** `projectbindings.go` mirroring `projects.go` (struct{pool}, `New…`, scan helper). `Upsert` uses `INSERT … ON CONFLICT (owner_id, remote_slug) WHERE kind='remote' DO UPDATE SET project_id=EXCLUDED.project_id, updated_at=now()` for remote; for path use the path partial index. (Slice 1 only exercises the remote conflict target; include both for completeness.) `DeleteRemote`/`DeletePath` are `DELETE … WHERE owner_id=$1 AND …`. `List`/`ListByProject` select owner-scoped.

- [ ] **Step 4: Run** `go test ./internal/adapter/pgstore/ -run Binding` (Docker). **Step 5: Commit** `feat(pgstore): project_bindings store + migration 0011`.

---

### Task 6: REST handlers + routes

**Files:** Create `internal/adapter/httpserver/projectbindings.go`; Modify `internal/adapter/httpserver/server.go` (add usecase fields + routes); Test `internal/adapter/httpserver/projectbindings_test.go`

**Interfaces:**
- Consumes the usecases (Task 4) as `Server` fields: `BindProject`, `UnbindProject`, `ResolveProject`, `ListProjectBindings`.
- Produces routes (spec §REST): `GET /api/v1/projects/resolve`, `PUT /api/v1/projects/{id}/bindings`, `DELETE /api/v1/projects/bindings`, `GET /api/v1/projects/{id}/bindings`, `GET /api/v1/projects/bindings`.

- [ ] **Step 1: Failing httptest** mirroring `webui_worktime_handlers_test.go`/existing httpserver tests: build a `Server` with fake stores + usecases; PUT a remote binding for an existing project → 200; `GET /resolve?slug=<remote>` → 200 with that project; unknown slug → 404; `GET /projects/bindings` lists it. (Reuse the existing test-server helper that seeds an authed user.)

- [ ] **Step 2: Run → FAIL** (routes 404).

- [ ] **Step 3: Implement** handlers (`u, _ := userFrom(r.Context())`; decode body via `json.NewDecoder`; `r.PathValue("id")`; `r.URL.Query().Get(...)`; encode via `json.NewEncoder`; map `ports.ErrProjectNotFound`/no-resolve → 404, decode errors → 400). The bind body is `{kind, remoteSlug, machineId, machineLabel, path}` → `usecase.BindKey`. Register the 5 routes in `server.go` with `s.auth(...)` beside the other `/api/v1/projects/...` routes; add the 4 usecase fields to the `Server` struct.

- [ ] **Step 4: Run → PASS** + `go test ./internal/adapter/httpserver/...`. **Step 5: Commit** `feat(httpserver): project binding resolve/bind/unbind/list routes`.

---

### Task 7: apiclient methods

**Files:** Create `internal/adapter/apiclient/projectbindings.go`; Test `internal/adapter/apiclient/projectbindings_test.go`

**Interfaces (Produces):**
```go
func (c *Client) ResolveProject(ctx, remoteSlug, machineID, cwd string) (domain.Project, bool, error) // GET /resolve, 404 → ok=false
func (c *Client) BindRemote(ctx, projectID, remoteSlug string) (domain.ProjectBinding, error)         // PUT {id}/bindings {kind:remote,...}
func (c *Client) UnbindRemote(ctx, remoteSlug string) error                                            // DELETE bindings?kind=remote&slug=
func (c *Client) ListBindings(ctx) ([]domain.ProjectBinding, error)                                    // GET projects/bindings
```
(Slice 2 adds `BindPath`/`UnbindPath`.)

- [ ] **Step 1: Failing test** with an `httptest` server asserting method+path+body and decoding responses (mirror `apiclient/worktime_test.go`). Cover `ResolveProject` 200 and 404→ok=false.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** using the existing `c.do(ctx, method, path, body, &out)` helper and `url.QueryEscape` for query params; for the 404→ok=false case, check the returned `*APIError`/status (see how `IsConflict`/status is exposed in `client.go`).
- [ ] **Step 4: Run → PASS.** **Step 5: Commit** `feat(apiclient): project binding methods`.

---

### Task 8: `internal/gitremote.OriginSlug`

**Files:** Create `internal/gitremote/gitremote.go`; Test `internal/gitremote/gitremote_test.go`

**Interfaces:** Produces `func OriginSlug(dir string) (slug string, ok bool, err error)`.

- [ ] **Step 1: Failing test** against a real temp repo:

```go
func TestOriginSlug(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string){ c := exec.Command("git", args...); c.Dir = dir; if out,err := c.CombinedOutput(); err != nil { t.Fatalf("git %v: %v %s", args, err, out) } }
	run("init"); run("remote","add","origin","git@github.com:serverkraken/flow.git")
	slug, ok, err := gitremote.OriginSlug(dir)
	if err != nil || !ok || slug != "github.com/serverkraken/flow" { t.Fatalf("%q %v %v", slug, ok, err) }
	// non-repo dir → ok=false, no error
	if _, ok, err := gitremote.OriginSlug(t.TempDir()); ok || err != nil { t.Fatalf("non-repo: ok=%v err=%v", ok, err) }
}
```

- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement**: `exec.CommandContext`-free is fine here (small); run `git -C dir remote get-url origin`; on non-zero exit / empty → `ok=false, nil` (treat "not a repo / no origin" as not-found, not error); else `domain.NormalizeRemoteSlug(strings.TrimSpace(out))`. Return a real error only for unexpected exec failures (git missing).
- [ ] **Step 4: Run → PASS.** **Step 5: Commit** `feat(gitremote): OriginSlug — cwd → normalized remote slug`.

---

### Task 9: `internal/projectresolve.Resolve` (client orchestrator)

**Files:** Create `internal/projectresolve/resolve.go`; Test `internal/projectresolve/resolve_test.go`

**Interfaces:** Produces `func Resolve(ctx, c *apiclient.Client, getenv func(string)string, cwd string) (domain.Project, bool, error)`. Slice 1 chain: FLOW_PROJECT → git-remote → server resolve. (Machine-id/path added in Slice 2.)

- [ ] **Step 1: Failing test** with a fake/httptest-backed apiclient + injected `getenv`/`cwd`:
  - `FLOW_PROJECT=flow` set → returns the project named by that slug (via `ListProjects`), without calling resolve.
  - unset + cwd is a git repo (use a temp repo) → calls `ResolveProject(remoteSlug,…)`.
  - `FLOW_PROJECT` set to unknown slug → error.

- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement**: if `getenv("FLOW_PROJECT") != ""` → `c.ListProjects` + match Slug (error `unknown FLOW_PROJECT slug %q` if none). Else `slug,_,_ := gitremote.OriginSlug(cwd)`; `c.ResolveProject(ctx, slug, "", cwd)` (machineID empty in Slice 1). Return its `(project, ok)`.
- [ ] **Step 4: Run → PASS.** **Step 5: Commit** `feat(projectresolve): client resolution chain (override + remote)`.

---

### Task 10: CLI `bind <slug>` / `unbind` / `bindings` (non-interactive)

**Files:** Modify `cmd/flow/project.go`; Test `cmd/flow/project_test.go`

**Interfaces:** Adds subcommands. `bind <slug>` (git repo): `OriginSlug(cwd)` → `resolveSlug`→id → `BindRemote`. `unbind`: `OriginSlug` → `UnbindRemote`. `bindings`: `ListBindings` + `projectresolve.Resolve` to mark `*`.

- [ ] **Step 1: Failing tests** (httptest-backed apiclient, like `session_test.go`): a `runBind(ctx,c,getwd,slug)` helper binds the cwd's origin to the project; "not a git repo" → clear error; `runBindings(ctx,c,getwd)` lists with the resolved project starred. Make the git-cwd injectable (`getOrigin func() (string,bool,error)`) so tests don't need a real repo, OR point `getwd` at a temp git repo.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the subcommands + helpers, wiring `gitremote.OriginSlug(cwd)`, `resolveSlug`, and the apiclient methods. Clear errors: `not in a git repo with an 'origin' remote`, `no project with slug …`. Register in `projectCmd()`.
- [ ] **Step 4: Run → PASS.** **Step 5: Commit** `feat(cli): flow project bind/unbind/bindings (remote, non-interactive)`.

---

### Task 11: CLI `bind` interactive picker (pick-or-create)

**Files:** Create `cmd/flow/projectbind.go`; Test `cmd/flow/projectbind_test.go`

**Interfaces:** `flow project bind` with no slug → one-shot `tea.NewProgram` over a `ui/fuzzylist`-based model seeded with existing projects (fuzzy + MRU), inline-create entry pre-filled from the remote repo name; returns the chosen/created project id → `BindRemote`.

- [ ] **Step 1:** Read `internal/tui/ui/fuzzylist` (`New(items,pal)`, `WithCreateHint`, `Update(KeyPressMsg)`, `Selection() (Item,isCreate,ok)`) and how a worktime screen drives it, to mirror the wiring.
- [ ] **Step 2: Failing test** of the *non-TUI* core: a `pickProjectModel` whose `Selection()` after simulated keys yields either an existing item or a create-with-name; assert the create name defaults to the repo name. (Drive `Update` with `tea.KeyPressMsg` like the fuzzylist tests do; do not launch a real terminal.)
- [ ] **Step 3: Implement** a small bubbletea model wrapping `fuzzylist.Model` (Init/Update/View delegating; Enter resolves `Selection()`), and the cobra glue: build items from `ListProjects`, set the create hint/default to the repo name from `OriginSlug`, run via `tea.NewProgram(... , tea.WithContext(ctx))`, then `CreateProject` if create else use the picked id, then `BindRemote`. Reuse the `tea.NewProgram` pattern from `cmd/flow/worktime.go`.
- [ ] **Step 4: Run → PASS** (`go test ./cmd/flow/`). **Step 5: Commit** `feat(cli): flow project bind picker (pick-or-create)`.

---

### Task 12: WebUI read-only bindings panel

**Files:** Modify the worktime/project templ + its handler (e.g. add to `internal/adapter/webui` + the handler that renders the project context); regenerate `*_templ.go`. Test: `internal/adapter/webui/render_test.go` (extend).

- [ ] **Step 1: Failing render test:** given `WorktimeData`/a project view-model carrying `[]domain.ProjectBinding`, the fragment lists a remote binding's slug. (Add a `Bindings []domain.ProjectBinding` field to whichever view-model renders the project area; populate it in the handler via `ListProjectBindings`.)
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the templ panel (`for _, b := range d.Bindings { … b.RemoteSlug / label:path … }`), wire the handler to load bindings, `go tool templ generate ./internal/adapter/webui/`, commit the regenerated `_templ.go`.
- [ ] **Step 4: Run → PASS.** **Step 5: Commit** `feat(webui): read-only project bindings panel`.

---

### Task 13: Wire into composition root + live verification

**Files:** Modify `cmd/flow-server/main.go`. No new logic — wiring + smoke only.

- [ ] **Step 1: Wire** `bindingStore := pgstore.NewProjectBindingStore(pool)` (beside the other stores ~line 66) and set the 4 `Server` usecase fields: `BindProject: usecase.BindProject{Bindings: bindingStore, Projects: projectStore, IDs: ids, Clock: clock}`, `ResolveProject: usecase.ResolveProject{Bindings: bindingStore, Projects: projectStore}`, `UnbindProject: usecase.UnbindProject{Bindings: bindingStore}`, `ListProjectBindings: usecase.ListProjectBindings{Bindings: bindingStore}`.
- [ ] **Step 2: `make ci`** green (lint + templ-no-diff + tests + coverage ≥ gate).
- [ ] **Step 3: Live done-gate** (dev stack up; `set -a; . deploy/dev/flow-cli.env; set +a`):
  - `flow project bind flow` in `…/flow-rebuild` → "bound repo github.com/serverkraken/flow → flow".
  - from `…/flow` (other worktree): `flow project bindings` shows `*` on flow; `curl -k "https://localhost:8080/api/v1/projects/resolve?slug=github.com/serverkraken/flow" -H "Authorization: Bearer $(make -s dev-token)"` → project flow.
  - `FLOW_PROJECT=<other> flow project bindings` → override wins.
  - WebUI project page shows the bound remote.
- [ ] **Step 4: Commit** `feat(flow-server): wire project bindings; V0 slice 1 done`.

---

## Self-review (done)

- **Spec coverage (Slice 1 scope):** override (Task 9), remote-slug normalize (1), binding model+resolve (2), store+migration (3,5), REST (6), apiclient (7), git reader (8), CLI bind/unbind/list + pick-or-create (10,11), WebUI (12), wiring+gate (13). Path tier explicitly Slice 2.
- **Placeholders:** the only forward-references ("Slice 2 adds …") are scoped-out-by-design, not gaps; the path columns/fields exist now so Slice 2 is additive.
- **Type consistency:** `BindingKind`/`ProjectBinding`/`BindKey`/`ProjectBindingStore` names are used identically across tasks; `ResolveProject.Execute` returns `(Project,bool,error)` consistent with apiclient `ResolveProject` and `projectresolve.Resolve`.

## Slice 2 (path tier) — outline, to detail just-in-time

Purely additive once Slice 1 is green:
1. `internal/clientmachine` — `Load() (Machine{ID,Label})` read-or-create `UserConfigDir/flow/machine.json` (uuid + hostname) + test.
2. Extend `domain.ResolveBinding` — add the longest-prefix path branch (segment boundary) + tests (`/a/b` vs `/a/bc`, longest wins, machine isolation).
3. `apiclient.BindPath/UnbindPath`; `usecase`/`httpserver` already accept `kind=path` via `BindKey` — add path conflict-target to pgstore `Upsert` + a Docker test for the path partial-unique.
4. `projectresolve.Resolve` — add machine-id + pass to `ResolveProject`; CLI `bind` auto-detects (no git origin → `BindPath` with `clientmachine` + cwd), reports "bound path … on this machine".
5. WebUI panel already lists `label: path` rows.
6. Live gate: `flow project bind` in a bare `/tmp/x` → path binding; `resolve` from a subdir returns it.
