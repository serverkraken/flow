# Project Management (M4) — Design

**Status:** approved-pending-review · 2026-06-23 · branch `rebuild`

**Goal:** Turn `Project` from a thin worktime/docs hub into a fully manageable
first-class entity: rich metadata (description, canonical upstream git), a real
lifecycle (active/paused/archived), full CRUD across every surface, and a
per-PC live view of the project's git checkout + worktrees. cwd→project
resolution becomes automatic the moment a project knows its upstream.

This realises the foundation the original `Project` struct already anticipated:
*"M1a uses a minimal field set; the heavier foundation fields (repos/paths/links/…)
arrive in later migrations."* (`internal/domain/project.go:16-18`).

## Decisions (the forks that shaped this design)

1. **Git role = observe, not act.** flow *detects* and *displays* git state;
   it never runs write-side git (no clone, no `worktree add/rm`). Stays true to
   the server-authoritative / thin-client architecture — the server never
   touches a client filesystem.
2. **Checkouts/worktrees = client-local, live.** No server-side snapshot, no
   cross-device worktree visibility. Each client reads `git worktree list` live
   on the machine it runs on. Consequence: worktrees show in TUI/CLI only; the
   WebUI (no client FS) shows metadata + worktime + docs, never worktrees.
3. **Three lifecycle states:** `active` · `paused` · `archived`.
4. **Upstream↔resolution = auto-sync.** Setting a project's upstream git upserts
   its remote `ProjectBinding`; clearing/changing it removes/re-points the
   binding. One concept ("the project's upstream") instead of two.

**Core invariant:** a project needs only a **name**. `UpstreamGit`,
`Description`, checkout/worktrees, `Rate`, `Color`, `Glyph` are all optional.
Non-code projects ("Orga", "Meetings", "Lernen") stay first-class — they simply
have worktime + docs and no git section. The UI never shows dead git panels:
the git section appears only when an upstream is set *or* a local checkout is
detected.

## Scope

**In:** Project gains `Description` (markdown) + `UpstreamGit` + `paused` status;
`UpdateProject` usecase + `PATCH /projects/{id}`; `ProjectStore.Update`;
apiclient `UpdateProject`; auto-sync of the remote binding; a WebUI project list +
detail cockpit + create/edit form; a TUI projects screen (list + detail with a
live worktree panel) + create/edit form; CLI `create/list/show/edit/pause/resume/
archive/worktrees`; light MCP enrichment (status + upstream in context/list);
closing the TUI session-edit "change project" gap; two new client packages
(`gitworktree`, `clientcheckout`).

**Out (later):**
- **Central tag system** (projects/worktime/docs all taggable + cross-entity
  search). Its own brainstorm→spec — it must reconcile the existing
  *frontmatter-derived* doc tags (M2c) with *structured* tags for projects and
  worktime, plus cross-entity search over the FTS/trgm/semantic stack (M2d/M2e).
  **We deliberately do NOT add an ad-hoc `project.tags` now** — the project model
  stays lean so the future shared tag model doesn't have to migrate it.
- Active git management (clone, worktree add/rm from flow).
- Cross-device worktree/checkout snapshots.
- MCP write tools for project metadata.

## Architecture (hexagonal, one responsibility per file)

**Domain**
- `internal/domain/project.go` — add `Description`, `UpstreamGit` fields; add
  `ProjectPaused` status constant; a `Project.Validate()` / status-enum guard.

**Ports**
- `internal/ports` — add `ProjectStore.Update(ctx, ownerID string, p domain.Project) (domain.Project, error)`.

**Usecases**
- `internal/usecase/update_project.go` — validate, persist, and **auto-sync the
  remote binding** (the one place that ties upstream→resolution). Depends on
  `ProjectStore` + `ProjectBindingStore`.
- (`create_project.go` extended to accept the new optional fields + run the same
  auto-sync when an upstream is supplied at creation.)

**Storage**
- `internal/adapter/pgstore/migrations/0014_project_description_upstream.sql`
  (goose Up/Down annotated).
- `internal/adapter/pgstore/projects.go` — add `Update` (writes all mutable
  columns); `List`/`Get`/`Create` select the new columns.

**Server**
- `internal/adapter/httpserver/projects.go` — `handleUpdateProject` (PATCH).
- `internal/adapter/httpserver/server.go` — register `PATCH /api/v1/projects/{id}`.

**Client (apiclient)**
- `internal/adapter/apiclient/client.go` — `UpdateProject(ctx, p) (Project, error)`;
  `CreateProject` extended to carry the optional fields; ensure a `GetProject`.

**WebUI**
- `internal/adapter/webui/projects.templ` + handlers — list, detail, create/edit.
- nav entry "Projekte".

**TUI**
- `internal/tui/screen/projects/` — `route.go` (list), `detail.go` (cockpit incl.
  worktree panel), `form.go` (create/edit), plus split files per the
  no-monolith rule. Mounted as a shell tab.

**New client packages**
- `internal/gitworktree/gitworktree.go` — `List(root) ([]Worktree, error)` over
  `git -C root worktree list --porcelain`. Mirrors `internal/gitremote`.
- `internal/clientcheckout/clientcheckout.go` — local registry
  `UserConfigDir/flow/checkouts.json` (`slug→root`), `Get`/`Record`. Mirrors the
  `clientmachine`/tokenstore file pattern.

**CLI**
- `cmd/flow/project.go` — extend with `create/list/show/edit/pause/resume/
  archive/worktrees` (keeps existing `bind/unbind/bindings/rate/rm`).

## Domain model

```go
type ProjectStatus string
const (
    ProjectActive   ProjectStatus = "active"
    ProjectPaused   ProjectStatus = "paused"   // NEW: rests, stays visible, resumable
    ProjectArchived ProjectStatus = "archived"
)

type Project struct {
    ID, OwnerID, Name, Slug, Color, Glyph string
    Rate        *Money
    Description string        // NEW — markdown, optional
    UpstreamGit string        // NEW — canonical repo URL, optional
    Status      ProjectStatus
    CreatedAt, UpdatedAt time.Time
}
```

`Validate()` enforces: name+slug required; status ∈ {active,paused,archived}.
`UpstreamGit` stored as the raw URL the user entered (for display + clone copy);
the resolution slug is derived from it via `NormalizeRemoteSlug` (never stored on
the project — it lives on the binding).

## Auto-sync: upstream → resolution

In `update_project` (and `create_project` when an upstream is given):

```
slug, ok := domain.NormalizeRemoteSlug(p.UpstreamGit)   // pure string parse, no git
switch {
case p.UpstreamGit == "":   // upstream cleared → drop the auto remote binding (if any)
case !ok:                   // unparseable URL → reject the whole update with 400 (no half-bound state)
default:                    // upsert remote ProjectBinding(owner, slug) → this project
}
```

- The server only *parses* the URL string; it never runs git.
- cwd→project (`projectresolve`) is otherwise unchanged: `FLOW_PROJECT` ▸
  git-origin-slug ▸ path-binding. The git-origin tier now hits the auto-synced
  binding, so resolution "just works" on every PC and worktree once the upstream
  is set. Manual path bindings (remote-less repos, bare dirs) remain as today.
- Re-pointing an upstream that was bound to a *different* project upserts
  (reassigns) the remote binding, consistent with the existing bind semantics.

## Checkout & worktrees (client-local, live)

No server state. Two small client pieces:

- **`clientcheckout`** — `UserConfigDir/flow/checkouts.json`: `projectSlug → checkoutRoot`.
  Auto-recorded by `projectresolve` whenever cwd resolves to a project *inside a
  git repo* (records the main-worktree root). The file is inherently per-machine,
  honouring decision #2 (nothing crosses devices).
- **`gitworktree.List(root)`** — parses `git -C root worktree list --porcelain`.
  Per worktree: `Path`, `Branch` (or detached HEAD), `HeadShort`, `Dirty` (cheap
  `git status --porcelain -uno`), `IsMain`. (ahead/behind deferred — optional v2.)

TUI/CLI detail renders: checkout root + the live worktree list, main worktree
marked. If the project has no registry entry on this PC → "nicht ausgecheckt auf
diesem PC" (no empty panel).

## Data model

```sql
-- migration 0014 (goose Up/Down annotated)
-- +goose Up
ALTER TABLE projects ADD COLUMN description  TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN upstream_git TEXT NOT NULL DEFAULT '';
-- status is already TEXT NOT NULL DEFAULT 'active'; 'paused' is a new valid
-- value enforced by the domain, no DB constraint change required.

-- +goose Down
ALTER TABLE projects DROP COLUMN upstream_git;
ALTER TABLE projects DROP COLUMN description;
```

No table is added for checkouts/worktrees (decision #2). The existing
`project_bindings` table carries the auto-synced remote binding unchanged.

## REST API

```
POST   /api/v1/projects            {name, slug?, description?, upstreamGit?, color?, glyph?}  -> 201 Project   (auto-sync if upstream)
GET    /api/v1/projects            ?status=active|paused|archived|all                          -> 200 [Project] (default: active+paused)
GET    /api/v1/projects/{id}                                                                    -> 200 Project
PATCH  /api/v1/projects/{id}       {name?, slug?, description?, upstreamGit?, color?, glyph?, status?}  -> 200 Project (auto-sync)
DELETE /api/v1/projects/{id}                                                                    -> 204           (existing; FK SET NULL keeps history)
POST   /api/v1/projects/{id}/rate  …                                                            -> existing
… /projects/.../bindings, /projects/resolve …                                                  -> existing
```

All routes `s.auth`, owner-scoped (single-user homelab; no new secrets).

## Surfaces

### WebUI (templ + HTMX, Kompendium look)

1. **`/projects` — overview.** Cards: glyph+color · name · status badge
   (aktiv/pausiert/archiviert; paused dimmed) · upstream link (if set) · worktime
   Σ (week) · doc count · rate. Status filter (active+paused default; archived
   toggle). "Neues Projekt".
2. **`/projects/{slug}` — detail = the project cockpit** (where "manage the
   lifecycle" happens): header (glyph · name · status badge · actions
   Bearbeiten / Pausieren·Fortsetzen·Archivieren); rendered **description**
   (empty ⇒ subtle "—"); **git section** *(only if upstream set)* upstream URL +
   clone-copy, no worktrees; **worktime panel** Σh total + week/month, `Σh ×
   Rate = €`, recent sessions, link to project-filtered worktime; **documents
   panel** assigned docs (links) + count; **bindings panel** read-only (shows the
   auto-synced remote binding + any path bindings).
3. **Create/Edit form:** name · slug (auto, editable) · description (textarea) ·
   upstream · color (theme palette) · glyph (whitelist) · rate · status. HTMX →
   POST/PATCH.

Visual execution of this slice is done with the **`frontend-design`** skill on
top of the existing design system (theme.Sem, badge/chip/countbar components,
Kompendium look). This is where the original "WebUI durchplanen" intent lands.

### TUI (shell route, tab "Projekte")

- **List:** fuzzy + status filter; glyph+color+name+status+worktime; `enter`→
  detail, `n`→new.
- **Detail (cockpit):** description (M3d markdown renderer) · worktime Σ+€ · docs
  list · **git/worktree live panel** (checkout root from `clientcheckout` +
  `gitworktree.List`; not checked out ⇒ hint) · status actions · `e` edit.
- **Form:** `ui/form` (same fields as WebUI; color/glyph pickers from the
  whitelist).
- Keyboard via `ui/grammar` + `ui/listnav`.

### CLI (`flow project …`)

```
flow project create <name> [--upstream URL --desc … --color … --glyph …]
flow project list   [--status active|paused|archived|all]
flow project show   [<slug>]      # default: cwd-resolved project; incl. live worktrees
flow project edit   <slug> [--name --slug --desc --upstream --color --glyph]
flow project pause|resume|archive <slug>
flow project worktrees [<slug>]   # live git-worktree list (slug→root via clientcheckout, or cwd)
# unchanged: bind · unbind · bindings · rate · rm
```

### MCP (read-only, light)

`flow_project_context` and `flow_list_projects` additionally return `status` +
`upstream`, so an agent working in a repo gets richer context. Write tools
deliberately deferred (they belong with the tag system).

## Session-edit: change the project (cross-cutting requirement)

Gap analysis (verified against the code):

- **Backend/REST/CLI — already done.** `usecase.EditSessionInput.ProjectID`
  (`edit_session.go:13,52`); `handleEditSession` accepts `projectId`
  (`worktime.go:180-199`); `flow session edit --project` resolves+changes it
  (`session.go:224-238,297`).
- **TUI — the actual gap.** The edit dialog in both `screen/worktime/daydetail`
  and Today has focus order Von/Bis/Tag/Notiz (`editFocus` starts at `editVon`,
  `daydetail/dialogs.go:300-304`) — **no project field**, even though the
  Nachbuchen dialog already has a `fuzzylist` project picker. Fix: add a project
  picker (with inline-create, mirroring Nachbuchen/booking) as the first edit
  field, prefilled from the session's current project; pass the resolved id into
  the existing `EditSession` route method.
- **WebUI — verify.** The edit form has a `projectId` select; confirm it
  persists end-to-end (history once had an "edit-wipes-project" bug) and add
  inline-create parity if missing.

## Auth / security

All endpoints require the user's token, owner-scoped. Single-user homelab; no
new secrets. Auto-sync writes only the caller's own bindings.

## Edge cases

- **Name-only project:** valid; no upstream binding, no checkout, no git section.
- **Upstream set to an unparseable URL:** reject with 400 (clear error) — no
  silent half-bound state.
- **Upstream changed to one already bound elsewhere:** upsert/reassign the remote
  binding (existing semantics); surface the reassignment.
- **Upstream cleared:** the auto remote binding is removed; resolution falls back
  to path binding / FLOW_PROJECT / none.
- **Slug rename:** allowed via edit; must keep `(owner_id, slug)` unique. The
  `clientcheckout` registry is a cache keyed by slug, so a stale entry simply
  self-heals — it is re-recorded lazily on the next cwd→project resolve. Worktime/
  docs reference the immutable project **id**, so renames never break links.
- **Archived/paused in pickers:** archived hidden by default (status filter
  reveals); paused shown but dimmed.
- **Delete:** hard delete; `ON DELETE SET NULL` (migration 0012) preserves
  worktime/doc history (they show "kein Projekt").
- **Worktree panel without a checkout on this PC:** explicit hint, never a blank
  panel.

## Testing strategy (TDD)

- **Domain:** new fields + `Validate` (status enum incl. paused; name/slug
  required).
- **Usecase:** `update_project` — persists all fields; **auto-sync** matrix
  (upstream set ⇒ remote binding upserted; cleared ⇒ removed; unparseable ⇒
  rejected/no binding; reassign across projects) with fake stores.
- **gitworktree:** parse `--porcelain` (main vs linked, detached HEAD, dirty)
  against a real temp `git init` + `git worktree add`.
- **clientcheckout:** record-then-get; missing slug; re-record on rename.
- **pgstore (Docker):** `Update` round-trips all columns; migration Up/Down;
  owner-scope. (Recall: goose annotations are mandatory — bare SQL fails apply.)
- **httpserver:** PATCH all fields; status transitions; 404; `?status=` filter;
  the auto-sync binding side-effect.
- **apiclient + cmd/flow:** create/list/show/edit/pause/resume/archive/worktrees
  happy + error paths.
- **webui:** render tests — list (status filter, dimmed paused), detail (with/
  without upstream, with/without docs), create/edit form.
- **TUI:** edit dialog now exposes + persists the project (the session-edit gap);
  projects list/detail render; worktree panel "not checked out" hint.
- Ends with `make ci` green (~80% gate) + a live done-gate.

## Slicing → Milestone M4 (each slice: own plan, subagent-driven, `make ci` +
done-gate, and a final **main-wiring verification task** — main.go/composition
root updated + curl-smoke each new route, per the standing lesson that per-task
reviews miss "the composition root never calls the new constructor").

1. **Backend-core** — domain fields + `paused`; `UpdateProject` usecase + PATCH
   route + `ProjectStore.Update` + apiclient + **auto-sync remote binding** +
   migration 0014 + `?status=` filter + MCP enrichment. *(The heart; unblocks
   every surface.)*
2. **WebUI** — `/projects` list + `/projects/{slug}` cockpit + create/edit form +
   nav. **Invoke `frontend-design` here.**
3. **TUI** — `screen/projects` (list + detail) + create/edit form + git/worktree
   live panel + new `gitworktree` + `clientcheckout` + checkout auto-record in
   `projectresolve`.
4. **CLI** — `create/list/show/edit/pause/resume/archive/worktrees`.
5. **Session-edit project** — add the project picker to the TUI edit dialog
   (Today + daydetail) + verify WebUI persistence. *(Small — backend/CLI already
   done.)*

## Done-gate (live, against the dev stack)

- Create a project with an upstream via WebUI; from a *different worktree* of that
  repo, `flow project show` (or cwd-resolution) returns it **without any manual
  bind** (auto-sync proven).
- Edit the project: change description + status active→paused→archived; confirm
  it dims in pickers and hides from the default list, reappears with the archived
  filter.
- Create a name-only project; confirm no git section anywhere and it is bookable.
- TUI detail of a checked-out project shows its live worktrees (after a
  `git worktree add`); a non-checked-out project shows the hint.
- Edit a worktime session in the TUI and change its project (the closed gap).
