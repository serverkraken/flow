# Project ↔ Path Bindings (per-device) — Design

**Status:** approved-pending-review · 2026-06-21 · branch `rebuild`

**Goal:** Give each flow Project a filesystem binding so a client can answer
*"which project is the current working directory?"* — with **per-device paths**
(the same central project lives at different local paths on different machines),
and **one project ↔ many paths per device** (multiple git worktrees of one repo).

## Program context (why this exists)

This is **V0** of a three-part program to make flow's Kompendium the central,
cross-device store for Claude's markdown artifacts (memory, plans, CLAUDE.md,
skills) plus the user's own project notes:

- **V0 — Project↔path bindings (this spec).** The resolution primitive. Also
  independently useful: `flow worktime`/pickers can auto-select the project for
  the current repo.
- **V1 — flow-mcp.** A generic stdio MCP server (thin REST adapter) exposing
  document read/write/search tools; takes an explicit `project` argument.
- **V2 — recall hook + memory migration.** A SessionStart hook that resolves
  cwd→project (via V0) and injects project-scoped memory/instructions; plus the
  switch of Claude's memory source to Kompendium.

**The seam:** V1/V2 never parse filesystem paths. They ask V0 *"resolve this
cwd"* and pass the resulting project to document tools. V0 owns path semantics
entirely.

## Scope (V0)

In: server-side per-device bindings; a stable client machine identity; a
resolve API; bind/unbind/list CLI; a read-only WebUI overview.

Out (later/other vorhaben): auto-creating a binding when picking a project in
worktime; editing/deleting bindings from the WebUI; any MCP or hook work.

## Architecture

Follows the existing hexagonal layering (domain → usecase → ports → adapters).
New units, each one responsibility per file:

- `internal/domain/projectbinding.go` — `ProjectBinding` entity + the **pure**
  resolution function (longest-prefix match). No I/O.
- `internal/ports` — extend with a `ProjectBindingStore` interface.
- `internal/usecase/{bind_project,unbind_project,resolve_project,list_project_bindings}.go`.
- `internal/adapter/pgstore/projectbindings.go` + migration `0011_project_bindings.sql`.
- `internal/adapter/httpserver/projectbindings.go` — REST handlers + routes.
- `internal/adapter/apiclient/projectbindings.go` — client methods.
- `internal/clientmachine/machine.go` — local machine-id (new small package).
- `cmd/flow/project.go` — `bind`/`unbind`/`bindings` subcommands; `bind` (no
  slug) reuses the existing worktime project picker (`ui/fuzzylist` +
  `CreateProject` for inline-create) launched one-shot via `tea.NewProgram`.
- `internal/adapter/webui` — read-only bindings panel on the project/worktime page.

## Data model

```sql
-- migration 0011 (goose Up/Down annotated)
CREATE TABLE project_bindings (
  id            TEXT PRIMARY KEY,
  owner_id      TEXT NOT NULL,
  project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  machine_id    TEXT NOT NULL,
  machine_label TEXT NOT NULL DEFAULT '',
  path          TEXT NOT NULL,             -- canonical absolute local path
  created_at    TIMESTAMPTZ NOT NULL,
  updated_at    TIMESTAMPTZ NOT NULL,
  UNIQUE (owner_id, machine_id, path)      -- one path → exactly one project
);
CREATE INDEX project_bindings_resolve_idx ON project_bindings (owner_id, machine_id);
```

A project may have many rows for one `machine_id` (worktrees). A `path` is bound
to at most one project per machine (the UNIQUE constraint); re-binding the same
path updates the row (upsert).

```go
type ProjectBinding struct {
    ID           string
    OwnerID      string
    ProjectID    string
    MachineID    string
    MachineLabel string
    Path         string
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

## Machine identity

A device is identified by a UUID generated **once** by the client and stored at
`<os.UserConfigDir>/flow/machine.json` (next to the token store), with a
human label (`os.Hostname()`):

```json
{ "id": "b1f8…uuid", "label": "soenne-mbp" }
```

`internal/clientmachine` exposes `Load() (Machine, error)` that reads-or-creates
this file. Reused by the CLI now and the MCP/hook later. The server never
generates machine-ids; it only stores what the client reports. Machine-ids and
paths are **owner-scoped** — only the authenticated user ever sees them.

## Resolution semantics (the core invariant)

Pure domain function, fully unit-testable:

```go
// ResolveBinding returns the binding whose Path is the longest path-prefix of
// cwd at a segment boundary, or ok=false. Paths are compared canonically.
func ResolveBinding(bindings []ProjectBinding, cwd string) (ProjectBinding, bool)
```

Rules:
- **Canonical paths.** The client sends `filepath.Clean(absolute cwd)`; binding
  paths are stored cleaned+absolute. (Symlink resolution via `EvalSymlinks` is a
  client-side best-effort before sending; document the macOS `/tmp`→`/private/tmp`
  caveat but do not over-engineer.)
- **Segment boundary.** `cwd` matches binding `p` iff `cwd == p` OR
  `strings.HasPrefix(cwd, p + string(os.PathSeparator))`. So `/a/b` matches
  `/a/b` and `/a/b/c`, never `/a/bc`.
- **Longest wins.** With nested bindings (`/a` and `/a/b`), the longest matching
  path wins — the most specific project.
- **No match → not found.** Resolution returns ok=false; callers treat "no
  project for cwd" as a normal state (CLI prompts to pick/bind; the hook injects
  only global memory).

## REST API (all `s.auth`, owner-scoped)

```
POST   /api/v1/projects/{slug}/bindings   {machineId,machineLabel,path}  -> 200 binding   (upsert)
DELETE /api/v1/projects/bindings          ?machine=&path=                -> 204            (unbind)
GET    /api/v1/projects/resolve           ?machine=&path=                -> 200 project | 404
GET    /api/v1/projects/{slug}/bindings                                  -> 200 [binding…] (one project, all machines — WebUI panel)
GET    /api/v1/bindings                    ?machine=(optional)           -> 200 [binding…] (all of the caller's bindings — CLI overview)
```

`{slug}` accepts the project slug (stable, human-facing). Resolve returns the
full `domain.Project` so the caller needs no second round-trip.

## CLI

```
flow project bind              # interactive: pick an existing project to bind cwd, or create one
flow project bind   <slug>     # non-interactive shortcut: bind cwd ↔ existing <slug> (errors if absent — never auto-creates)
flow project unbind            # remove the binding whose path == cwd
flow project bindings          # list bindings (grouped by machine), '*' marks cwd's resolved project
```

**Bind is pick-or-create, never silent-create.** `flow project bind` with no
slug launches a project picker — the *same* picker the worktime flow uses
(existing projects, fuzzy-filter + MRU sort, with an **inline-create** entry as
the deliberate fallback). The user either selects a matching existing project or
explicitly creates a new one; only then is cwd bound to it. This avoids project
sprawl (no accidental duplicate projects per checkout). The picker is the
existing `ui/fuzzylist`-based component launched one-shot via `tea.NewProgram`
(same pattern as `flow worktime`/`flow ui`).

`<slug>` is the scriptable non-interactive path: it binds to an **existing**
project and errors `slug not found` rather than creating. `bind`/`unbind`/
`bindings` read the local machine-id (auto-creating the machine file) and
`os.Getwd()`. Other errors are clear (`cwd already bound to <other> — re-bind to reassign`).

## WebUI (read-only in V0)

On the project list / worktime page, a small read-only panel per project listing
its bindings as `label: path`. Edit/delete deferred (only the machine itself can
sensibly set its own path).

## Auth / security

All endpoints require the user's token and are owner-scoped (a binding query
returns only the caller's rows). Single-user homelab; machine-ids/paths are
personal data, no cross-user exposure. No new secrets.

## Edge cases

- **Stale path** (repo moved/worktree removed): resolve simply won't match →
  "no project"; `flow project bindings` still lists it so the user can `unbind`.
- **Path bound to a different project**: the UNIQUE constraint makes re-`bind`
  an upsert (reassigns); document that `bind` reassigns rather than errors.
- **Project deleted**: `ON DELETE CASCADE` drops its bindings.
- **Two projects, nested paths**: longest-prefix resolves deterministically.

## Testing strategy (TDD)

- **Domain:** `ResolveBinding` table tests — exact, nested/longest, segment
  boundary (`/a/b` vs `/a/bc`), no-match, empty set.
- **Usecase:** bind upsert, unbind, resolve, list — against a fake store.
- **pgstore:** Docker test for upsert + UNIQUE + cascade + owner-scoping (needs
  goose Up/Down annotations).
- **httpserver:** httptest for each route incl. 404 resolve and slug→project.
- **apiclient + cmd/flow:** client method tests; cmd happy-path + "slug not
  found" / "already bound" error surfacing.
- **webui:** render test that the panel lists `label: path`.
- Ends with `make ci` green (~80% gate) + a live done-gate against the dev stack
  (`flow project bind` in two different paths → `resolve` returns the right
  project; nested longest-prefix verified).

## Done-gate (live)

With the dev stack: `flow project bind flow` in `…/flow-rebuild`, again in a
second worktree path; `GET /resolve` from each path returns project *flow*;
a nested sub-project path resolves to the more specific project; the WebUI panel
shows both paths under the machine label.
