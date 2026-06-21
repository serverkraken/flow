# Project Resolution & Bindings — Design

**Status:** approved-pending-review · 2026-06-21 · branch `rebuild`
**Consolidates** the earlier path-only and remote-only drafts into one hybrid
resolution chain.

**Goal:** Let any client answer *"which flow Project is this working context?"*
through a precedence chain that covers every way Soenne works: git repos (incl.
worktrees, cross-device), local-only repos, bare directories, and remote systems
worked via another MCP (e.g. Home Assistant) where the cwd is unrelated.

## Resolution chain (the primitive everyone uses)

```
resolve(env, cwd):
  1. FLOW_PROJECT set        → that project           # explicit override — ANY context, cwd-independent
  2. git origin in cwd       → remote-slug binding     # device/worktree-invariant (one bind, resolves everywhere)
  3. else                    → longest-prefix path     # per-device: local-only repo / bare dir
  4. nothing                 → none (caller prompts pick-or-create)
```

- **Tier 1 (override):** runtime only — set `FLOW_PROJECT=<slug>` (e.g. in a
  direnv `.envrc` for the HA working dir, or a shell). Nothing stored.
- **Tier 2 (remote):** a *git remote slug* is identical across all worktrees and
  all machines, so one stored binding resolves everywhere. The common case.
- **Tier 3 (path):** the only stable identity for a remote-less repo or a bare
  directory is its absolute path — inherently per-device.

This is intuitive: *"bind this place to a project; if it's a remote repo the
binding follows the repo everywhere, otherwise it's this folder on this machine —
and you can always force it with FLOW_PROJECT."*

## Program context

V0 of a three-part program making Kompendium Claude's central cross-device
markdown store:
- **V0 — project resolution & bindings (this spec).** The cwd→project primitive.
  Also upgrades `flow worktime`/pickers to auto-select the current project.
- **V1 — flow-mcp.** Generic stdio MCP server (REST adapter) with document
  read/write/search tools; takes an explicit `project`.
- **V2 — recall hook + memory migration.** SessionStart hook resolves cwd→project
  (via V0) and injects project-scoped memory/instructions.

**The seam:** V1/V2 never touch git, env, or paths — they call V0's resolver and
pass the resulting project onward. V0 owns context-identity entirely.

## Scope (V0)

In: the resolution chain (override + remote + path); a pure remote-slug
normalizer; a client git-origin reader; a client machine-id; a server-side
binding store (discriminated remote/path) + REST; bind/unbind/list CLI with
**pick-or-create**; a read-only WebUI overview.

Out (later): monorepo sub-projects (one remote, many projects → optional
subpath); a persistent `.flow` marker file (env override covers the need);
auto-binding from the worktime picker; WebUI editing; any MCP/hook work.

## Architecture (hexagonal, one responsibility per file)

- `internal/domain/remoteslug.go` — pure `NormalizeRemoteSlug(url)`.
- `internal/domain/projectbinding.go` — `ProjectBinding` entity + pure
  `ResolveBinding(bindings, remoteSlug, machineID, cwd)` (the precedence between
  remote and path tiers, longest-prefix path match).
- `internal/ports` — `ProjectBindingStore`.
- `internal/usecase/{bind_project,unbind_project,resolve_project,list_project_bindings}.go`.
- `internal/adapter/pgstore/projectbindings.go` + migration `0011_project_bindings.sql`.
- `internal/adapter/httpserver/projectbindings.go` — handlers + routes.
- `internal/adapter/apiclient/projectbindings.go` — client methods.
- `internal/gitremote/gitremote.go` — `OriginSlug(dir)` (runs git, normalizes).
- `internal/clientmachine/machine.go` — machine-id (uuid + hostname) in
  `UserConfigDir/flow/machine.json`.
- `internal/projectresolve/resolve.go` — the **client orchestrator** that runs
  the chain (reads FLOW_PROJECT, git origin, machine-id, calls the server).
- `cmd/flow/project.go` — `bind`/`unbind`/`bindings` subcommands.
- `internal/adapter/webui` — read-only bindings panel.

## Core invariants (pure, unit-tested)

**Remote-slug normalization** — every git URL form → one stable lowercased slug:

| input | slug |
|---|---|
| `git@github.com:serverkraken/flow.git` | `github.com/serverkraken/flow` |
| `ssh://git@github.com/serverkraken/flow.git` | `github.com/serverkraken/flow` |
| `https://user@gitlab.com:8443/a/b/c.git/` | `gitlab.com/a/b/c` |
| `` / `garbage` | ok=false |

Strip scheme/credentials/port, accept scp-form, strip trailing `.git` and `/`,
lowercase. `func NormalizeRemoteSlug(url string) (slug string, ok bool)`.

**Binding resolution** — precedence + segment-boundary longest-prefix:
```go
// remote match wins; else the longest path binding (for machineID) that is a
// segment-boundary prefix of cwd. Pure.
func ResolveBinding(bindings []ProjectBinding, remoteSlug, machineID, cwd string) (ProjectBinding, bool)
```
Path match: `cwd == p` OR `HasPrefix(cwd, p + "/")` — so `/a/b` matches `/a/b/c`,
never `/a/bc`; longest wins.

## Client building blocks

- `gitremote.OriginSlug(dir) (slug string, ok bool, err error)` — runs
  `git -C dir remote get-url origin`, normalizes; ok=false (not error) when no
  repo/origin. Worktrees share origin → worktree-invariant.
- `clientmachine.Load() (Machine{ID,Label}, error)` — read-or-create
  `UserConfigDir/flow/machine.json` (uuid + `os.Hostname()`). Mirrors the
  tokenstore file pattern. Used by the path tier only.
- `projectresolve.Resolve(ctx, c, getenv, cwd) (domain.Project, bool, error)`:
  1. `getenv("FLOW_PROJECT")` set → `ListProjects` + match slug → return (error
     if slug unknown).
  2. `gitremote.OriginSlug(cwd)` → if ok pass as `slug`.
  3. `clientmachine.Load()` → machine id.
  4. call `apiclient.ResolveProject(slug, machineID, cwd)` → project | not-found.

## Data model

```sql
-- migration 0011 (goose Up/Down annotated)
CREATE TABLE project_bindings (
  id            TEXT PRIMARY KEY,
  owner_id      TEXT NOT NULL,
  project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  kind          TEXT NOT NULL CHECK (kind IN ('remote','path')),
  remote_slug   TEXT,                       -- kind=remote
  machine_id    TEXT,                       -- kind=path
  machine_label TEXT,                        -- kind=path (display)
  path          TEXT,                        -- kind=path
  created_at    TIMESTAMPTZ NOT NULL,
  updated_at    TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX project_bindings_remote ON project_bindings (owner_id, remote_slug) WHERE kind='remote';
CREATE UNIQUE INDEX project_bindings_path   ON project_bindings (owner_id, machine_id, path) WHERE kind='path';
CREATE INDEX project_bindings_owner ON project_bindings (owner_id);
```

One remote → one project; one (machine,path) → one project; a project may own
many of either. Re-binding an already-owned key **upserts/reassigns**.

```go
type BindingKind string // "remote" | "path"
type ProjectBinding struct {
    ID, OwnerID, ProjectID string
    Kind                   BindingKind
    RemoteSlug             string  // kind=remote
    MachineID, MachineLabel, Path string // kind=path
    CreatedAt, UpdatedAt   time.Time
}
```

## REST API (all `s.auth`, owner-scoped; server never runs git)

```
GET    /api/v1/projects/resolve   ?slug=&machine=&path=   -> 200 Project | 404   (remote-then-path precedence)
PUT    /api/v1/projects/{id}/bindings   {kind, remoteSlug | machineId, machineLabel, path}  -> 200 ProjectBinding  (upsert)
DELETE /api/v1/projects/bindings  ?kind=remote&slug=  |  ?kind=path&machine=&path=   -> 204
GET    /api/v1/projects/{id}/bindings    -> 200 [ProjectBinding…]   (one project — WebUI panel)
GET    /api/v1/projects/bindings         -> 200 [ProjectBinding…]   (all of caller's — CLI overview)
```

`resolve` loads the owner's bindings and applies `domain.ResolveBinding`.
Project identified by `{id}`; CLI resolves slug→id via the existing
`resolveSlug` (the `/projects/{id}/rate` convention). FLOW_PROJECT resolution
needs no endpoint — the client reuses `ListProjects`.

## CLI (pick-or-create; auto-detects the tier)

```
flow project bind              # interactive: bind cwd to a picked/created project
flow project bind   <slug>     # non-interactive: bind cwd to existing <slug> (no auto-create)
flow project unbind            # remove the binding for cwd (remote or path, whichever applies)
flow project bindings          # list all bindings ('*' = cwd's resolved project)
```

`bind` auto-detects the tier from cwd: **git origin present → bind the
remote-slug** (machine-independent, covers all worktrees); **else → bind the
path on this machine**. The CLI reports which (`bound repo github.com/… → flow`
vs `bound path /… on this machine → flow`).

**Pick-or-create, never silent-create.** `flow project bind` (no slug) launches
the existing worktime picker (`ui/fuzzylist`: existing projects, fuzzy + MRU,
**inline-create** as the deliberate fallback), one-shot via `tea.NewProgram`. For
a git repo the new-project name pre-fills from the remote's repo name
(`serverkraken/flow` → `flow`). `<slug>` is the scriptable path: binds to an
existing project, errors `no project with slug …`.

`FLOW_PROJECT` is documented as the override; it is not "bound" (just exported).

## WebUI (read-only in V0)

A small per-project panel listing the project's bindings — remotes as
`remote-slug`, paths as `label: path`. Edit/delete deferred.

## Auth / security

All endpoints require the user's token, owner-scoped. Single-user homelab;
slugs/paths/machine-ids are personal, no cross-user exposure. No new secrets.

## Edge cases

- **No FLOW_PROJECT, no git origin, no path match:** resolve returns not-found →
  callers treat as "no project" (CLI prompts pick-or-create; the hook injects
  only global memory).
- **FLOW_PROJECT set to an unknown slug:** clear error, no silent fallthrough.
- **Same repo, ssh vs https on two machines:** normalizer maps both to one slug.
- **Key already bound to another project:** `PUT` upserts (reassign); CLI notes.
- **Project deleted:** `ON DELETE CASCADE` drops its bindings.
- **Multiple remotes on a repo:** V0 uses `origin`.

## Testing strategy (TDD)

- **Domain:** `NormalizeRemoteSlug` table (every row + scp/url equivalence +
  strip + lowercase + ok=false); `ResolveBinding` (remote-beats-path, longest
  path prefix, segment boundary `/a/b` vs `/a/bc`, machine isolation, empty).
- **gitremote:** `OriginSlug` against a real temp `git init` + `remote add`; and
  a non-repo dir → ok=false.
- **clientmachine:** create-then-reload returns the same id.
- **projectresolve:** chain order — FLOW_PROJECT wins; remote next; path last;
  none — with a fake apiclient + injected getenv/cwd.
- **usecase:** bind upsert/reassign (both kinds), unbind, resolve, list — fake store.
- **pgstore:** Docker test: both partial uniques, reassign, cascade, owner-scope
  (goose Up/Down annotations).
- **httpserver:** httptest each route incl. 404 resolve, both bind kinds, slug→id.
- **apiclient + cmd/flow:** client methods; cmd happy paths + "no project with
  slug" + "not in a git repo" (path-tier) errors.
- **webui:** render test the panel lists a remote and a path binding.
- Ends with `make ci` green (~80% gate) + a live done-gate.

## Plan sequencing (recommended)

Design the whole chain up front; build in two slices so value ships early:
- **Slice 1 (override + remote):** chain skeleton, `FLOW_PROJECT`, remote-slug
  tier end-to-end (domain→pgstore→REST→apiclient→CLI→WebUI). Covers git repos
  (incl. worktrees, cross-device) + HA-via-override — the bulk of real use.
- **Slice 2 (path tier):** `clientmachine`, path bindings + longest-prefix in
  `ResolveBinding`, the `kind=path` bind/resolve paths. Covers local-only repos
  and bare directories. Purely additive.

## Done-gate (live)

Against the dev stack: `flow project bind flow` in `…/flow-rebuild`; from a
*different worktree* `…/flow`, `flow project bindings` marks `*` on `flow` and
`resolve` returns *flow* (one bind, both worktrees). `FLOW_PROJECT=other flow
project bindings` shows the override winning. (Slice 2) `flow project bind` in a
bare `/tmp/x` dir binds a path; `resolve` from a subdir returns it.
