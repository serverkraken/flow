# flow-mcp V1 — Design

Date: 2026-06-21 · Branch: `rebuild` · Status: approved for planning

## 1. Context — the 3-part program

flow's Kompendium becomes Claude's **central, cross-device markdown store**: memory,
plans, global + project `CLAUDE.md`, skills — plus Soenne's own project notes as
context. The program has three parts:

- **V0 — project resolution & bindings (DONE).** The `cwd → project` primitive:
  `internal/projectresolve.Resolve(ctx, c, getenv, cwd) (domain.Project, bool, error)`
  with the precedence `FLOW_PROJECT` override → git remote-slug binding → per-device
  path binding → none. Both slices + project-delete shipped on `rebuild`.
- **V1 — flow-mcp (this spec).** A stdio MCP server that lets Claude read/write those
  documents over MCP tools, from any device, project-scoped via the V0 seam.
- **V2 — recall hook + memory migration (later, NOT here).** A SessionStart hook that
  resolves `cwd → project` via V0 and injects project-scoped memory, plus switching
  Claude's memory source to the Kompendium. Lives in dotfiles / Claude config, not flow.

## 2. Goals / non-goals

**Goals**
- A `flow-mcp` stdio binary, a thin REST adapter over the existing `apiclient` document
  methods, exposing documents as MCP **resources** + **tools**.
- Project-scoped by default via the V0 resolution seam, with explicit per-call override
  and a `"global"` escape.
- Full CRUD with an anti-clobber **write guard** protecting human-owned documents.
- Reuse the same auth (`tokenstore` + `clientconfig`) the CLI uses; no interactive flow.

**Non-goals (V1)**
- Populating skills / `CLAUDE.md` / memory into the Kompendium, or the SessionStart
  recall hook — that is V2 and lives outside flow.
- Worktime / day-off / export / stats tools.
- Remote HTTP / SSE MCP transport (the official SDK keeps that door open for later).
- Mutating a document's `type` / `project` / `path` (the update path only touches
  title + body; this matches the server today).

## 3. Resolved decisions (the 4 open questions)

1. **Protocol layer** → official **`github.com/modelcontextprotocol/go-sdk` v1.5.0**
   (stdio transport). Maturity verified: v1.0.0 shipped, v1.5.0 (2026-04-07), protocol
   `2025-11-25`, typed tools via `jsonschema` struct tags, resources + templates,
   maintained with Google. Supersedes the old hand-rolled `mcpstdio` (a pre-1.0 call).
2. **Write safety** → **write-all with guard**. Agent-owned document types are freely
   writable; mutating (update/delete) a **human-owned** type requires an explicit
   per-call `confirm: true`. The guard is a client-side affordance in the MCP tool
   layer (all documents belong to the one OIDC owner — this is anti-clobber, not authz).
3. **Tool surface** → full surface, 11 tools (§7).
4. **cwd → project auto-scope** → **default-on**: tools default `project` to the
   resolved current project, accept an explicit `project` override, and accept the
   sentinel `"global"` to escape scoping.

## 4. Architecture

### 4.1 Generic document adapter (the core principle)

flow-mcp knows **nothing** about "skill" / "CLAUDE.md" / "memory". It reads and writes
*documents* and passes `Type` / `ProjectID` / `Tags` / `Path` / `Role` straight through.
New artifact kinds are new `Type` values + a naming/tagging convention — **never** an
MCP change. This keeps V1 stable while V2 defines how memory/skills/instructions map
onto documents.

### 4.2 Thin over apiclient

All document operations reuse existing `internal/adapter/apiclient` methods verbatim:

```go
CreateDocument(ctx, CreateDocumentInput{Type, ProjectID, Path, Title, Body}) (Document, error)
GetDocument(ctx, id) (Document, error)
ListDocuments(ctx, tags ...string) ([]Document, error)        // + projectID, see §8
UpdateDocument(ctx, id, UpdateDocumentInput{Title, Body}) (Document, error)
DeleteDocument(ctx, id) error
Search(ctx, q, tags ...string) ([]SearchHit, error)           // + projectID, see §8
Tags(ctx) ([]TagCount, error)
Backlinks(ctx, id) ([]BacklinkRef, error)
```

**Tags are frontmatter-driven, not a write field.** `CreateDocumentInput` has no tags
field and `UpdateDocumentInput` is only `{Title, Body}`; the server derives tags from
the body's YAML frontmatter via `domain.ParseFrontmatter` (M2c single-source). So MCP
write tools set tags by writing frontmatter into `body`; `tags` is a **read/filter**
parameter only.

### 4.3 Shared auth-client builder (`internal/clientauth`, NEW)

The kickoff assumed a `clientFromStore` helper — it does not exist; `cmd/flow` builds
the authed client inline (`cmd/flow/login.go:60` → `apiclient.NewTransport(serverURL,
&oauth2.Transport{Source: ...})`, with the refreshing token source in `cmd/flow/auth.go`).
V1 extracts a shared builder so `flow` and `flow-mcp` construct identical clients:

```go
// internal/clientauth — given config + token store, returns an apiclient whose
// transport refreshes the OIDC token automatically (or an error if no token).
func Client(cfg clientconfig.Config, store ports.TokenStore) (*apiclient.Client, error)
```

It composes: `tokenstore` token → refreshing `oauth2.TokenSource` (issuer/clientID from
`clientconfig`) → `oauth2.Transport` → `apiclient.NewTransport`. `cmd/flow` is refactored
to call it (no behavior change). **Depguard note:** `clientauth` imports adapter
packages; if the layer rules forbid `internal/* → adapter`, it lives under `cmd/` shared
code or gets an explicit allow — resolved in planning.

### 4.4 Auth & boot gate

flow-mcp runs as a stdio subprocess: **stdout is JSON-RPC only; all logs go to stderr.**
It cannot run an interactive device flow, so it requires a prior `flow login` on that
device. The **11 tools are always registered** so `tools/list` is stable regardless of
auth. At boot it calls `apiclient.Whoami(ctx)` (`GET /api/v1/me`):

- success → authed; resolve the project (§4.5); register the project's resources (§7.1).
- no token / expired / refresh fails → start **anyway** in a degraded state; no resources
  are registered, and every tool short-circuits with `IsError: true`, text `"Login
  required: run 'flow login' in a terminal on this device."` Never crash. (Pattern ported
  from the old `MCPTools` login-required short-circuit; the SDK replaces the hand-rolled
  dispatcher.)

### 4.5 The V0 seam (auto-scope)

At boot, after auth, flow-mcp calls `projectresolve.Resolve(ctx, client, os.Getenv,
cwd)` where `cwd = os.Getwd()`. The result is held as process state:
`{project domain.Project, matched bool, how string}` where `how ∈ {override, remote,
path, none}`. Tools default their `project` parameter to `project.ID` when `matched`;
on `none` they default to global and surface a hint to use `flow_bind_project`.

## 5. Domain change — new document types

Add four agent-owned `DocumentType` constants in `internal/domain/document.go` and
extend `valid()`:

```go
DocMemory      DocumentType = "memory"
DocInstruction DocumentType = "instruction"  // CLAUDE.md
DocSkill       DocumentType = "skill"
DocPlan        DocumentType = "plan"
```

`Type` is a `TEXT` column with no DB enum, so **no migration** is needed — extending the
domain `valid()` is sufficient to create/store documents of these types (confirmed:
`NewDocument` is the only gate). Existing types are unchanged.

**Ownership classes** (drives the write guard, §6):
- **Human-owned:** `daily`, `project`, `free`.
- **Agent-owned:** everything else (`agent`, `memory`, `instruction`, `skill`, `plan`,
  and any future type).

The rule is expressed type-agnostically — *"target type ∈ {daily, project, free} ⇒
guarded"* — so future agent types fall on the free side automatically, with no MCP edit.

## 6. Write guard

`flow_create_doc` never clobbers (it only adds), so it is **unguarded** for any type.

`flow_update_doc` and `flow_delete_doc` first fetch the target via `GetDocument(id)`,
then:
- target type agent-owned → proceed.
- target type human-owned and `confirm != true` → return `IsError: true`, text
  `"<id> is a human-owned note (type=<t>). Pass confirm=true to modify it."`
- target type human-owned and `confirm == true` → proceed.

Fetch-then-act also enables partial update: when only `body` (or only `title`) is given,
the missing field is carried over from the current document (`UpdateDocumentInput`
requires both).

## 7. MCP surface

### 7.1 Resources

One resource per document of the resolved project (skipped when unresolved):
`flow://doc/{id}`, `Name = title`, `Description = "<path> · <type> · <tags>"`,
`MIMEType = text/markdown`. The read handler returns `Body`. The set is registered at
boot from the project's documents and kept live on create/delete via `server.AddResource`
/ `server.RemoveResources` (the SDK emits `listChanged`). Resources are the bonus path
for capable clients; tools are the reliable path (§7.2).

### 7.2 Tools (11)

Default `project` = resolved project; explicit `project` (slug/id) overrides; `"global"`
escapes scoping. All handlers short-circuit "Login required" when unauthed.

| Tool | Params | Backing call | Notes |
|---|---|---|---|
| `flow_project_context` | — | resolve state + scoped `ListDocuments` count | Orientation entry point: resolved project (name/slug/id), `how` (override/remote/path/none), doc count, next-step hint. |
| `flow_search_docs` | `query`, `project?`, `tags?`, `type?`, `limit?` | `Search` (+projectID, §8) | Hybrid FTS+semantic. `type` filtered MCP-side. Returns id/title/path/type/snippet. |
| `flow_list_docs` | `project?`, `tags?`, `type?` | `ListDocuments` (+projectID, §8) | Metadata only (id/title/path/type/tags). `type` filtered MCP-side. |
| `flow_get_doc` | `id` \| `path` | `GetDocument` | `path` resolved to id MCP-side (list/search lookup). Returns full doc + project + role. |
| `flow_list_tags` | `project?` | `Tags`, or `CollectTags` over scoped docs | Tag counts for filtering. |
| `flow_backlinks` | `id` \| `path` | `Backlinks` | Inbound wikilink refs — memory-graph navigation. |
| `flow_create_doc` | `path`, `title`, `body`, `type`, `project?` | `CreateDocument` | Tags via body frontmatter. `type` must be valid. Unguarded. |
| `flow_update_doc` | `id`, `title?`, `body?`, `confirm?` | `GetDocument`+`UpdateDocument` | Partial via fetch-then-merge. Guarded (§6). type/project/path immutable. |
| `flow_delete_doc` | `id`, `confirm?` | `GetDocument`+`DeleteDocument` | Guarded (§6). |
| `flow_bind_project` | `project` \| `create_name`, `kind?` | project-create + `BindRemote`/`BindPath` | Binds cwd; auto-detect kind (git origin → remote, else path), mirroring the CLI. `create_name` → create then bind. Returns binding + newly resolved project. |
| `flow_list_projects` | — | `GET /api/v1/projects` | id/name/slug — avoids duplicate-project creation when binding. |

Result text is concise plain text (the model reads it); errors set `IsError: true` with
an actionable message.

## 8. Backend slice — project-scoped list + search

Auto-scope requires server-side project filtering, which does not exist today (V0
deferred it as the "bigger piece"). Semantic search **must** scope server-side
(post-filtering top-K semantic hits client-side is broken), so this is in V1.

Thread an optional project filter through, additively (nil ⇒ all; a `"none"` sentinel ⇒
`project_id IS NULL`, which also delivers V0's deferred "(no project)" filter):

- **ports** (`internal/ports`): extend `DocumentStore.List` and `DocumentStore.Search`
  with a `projectID *string` argument (or a small filter struct — chosen in planning to
  minimise caller churn).
- **pgstore** (`internal/adapter/pgstore/documents.go`): add
  `AND ($N::text IS NULL OR project_id = $N)` (and the `IS NULL` sentinel branch) to the
  `List` SQL (~:54) and the `Search` SQL (~:152).
- **usecases**: `ListDocuments.Execute` and `SearchDocuments.Execute` gain the
  `projectID` argument; update all callers (`ListTags` uses `List`, the TUI, etc.).
- **httpserver** (`documents.go`): parse `?projectId=` in `handleListDocuments` (applies
  to both the search and list branches) and pass it through.
- **apiclient**: extend `ListDocuments` and `Search` to carry an optional `projectID`
  (additive method variant or options struct, to avoid breaking existing call sites).

The TUI's current client-side `p` project filter can later adopt this; out of scope here.

## 9. File layout (Keine Monolithen)

```
internal/clientauth/clientauth.go      # NEW shared authed-apiclient builder (§4.3)
internal/domain/document.go            # +4 DocumentType constants, valid() (§5)
internal/ports/…                       # +projectID on List/Search (§8)
internal/adapter/pgstore/documents.go  # +projectId SQL (§8)
internal/usecase/{list,search}_document*.go  # +projectID param (§8)
internal/adapter/httpserver/documents.go     # parse ?projectId= (§8)
internal/adapter/apiclient/documents.go      # +projectID on list/search (§8)
cmd/flow/…                             # refactor to use internal/clientauth (no behavior change)
cmd/flow-mcp/main.go                   # wiring only: config → clientauth → resolve → SDK server → Run(stdio)
cmd/flow-mcp/auth.go                   # boot Whoami gate + login-required short-circuit
cmd/flow-mcp/resolve.go                # projectresolve → resolved-project state
cmd/flow-mcp/resources.go              # document resources (register/refresh)
cmd/flow-mcp/tools_docs.go             # search/list/get/create/update/delete + guard
cmd/flow-mcp/tools_project.go          # project_context/bind/list_projects/list_tags/backlinks
cmd/flow-mcp/loopback_test.go          # stdio integration test
docs/runbook/flow-mcp-setup.md         # NEW: registration runbook (§11)
```

Pure helpers (guard classification, scope resolution, `path → id`, result formatting)
are package-level funcs in `cmd/flow-mcp`, unit-tested via `package main` tests. If the
handler logic grows, extract `internal/adapter/mcpserver`.

## 10. Testing

- **Unit:** guard classification (type → guarded?); scope resolution
  (resolved/override/`"global"`/`none`); `path → id`; partial-update merge; result
  formatting — table tests.
- **Integration:** `loopback_test.go` drives the SDK server over in-memory pipes against
  an `httptest`-backed apiclient: assert tool count = 11, `initialize`/`tools/list`/
  `resources/list`, a `create → get` round-trip, a guarded `update`/`delete` rejected
  without `confirm` and accepted with it, and project-scoped vs `"global"` search.
- **Backend slice:** pgstore Docker tests for the `projectId` filter (and `"none"`
  sentinel) on `List` + `Search`; usecase tests for the new argument. No migration ⇒ no
  goose annotations needed.
- `make ci` green at/above the current coverage gate.

## 11. Done-gate

1. `make ci` green.
2. `go build ./cmd/flow-mcp` produces the binary.
3. Register with Claude Code via `.mcp.json` / `claude mcp add` pointing at the built
   binary with env (`FLOW_SERVER_URL`, `FLOW_OIDC_ISSUER`). Exact config format confirmed
   via the `claude-docs-consultant` skill.
4. Live gate against the dev server (controller pattern: `FLOW_DEV=1
   FLOW_LISTEN_ADDR=:8443 FLOW_PUBLIC_BASE_URL=https://localhost:8443`, auto-TLS+migrate;
   see `reference_flow_dev_env`). After `flow login`: from a real Claude Code session
   with the MCP registered —
   - `flow_project_context` resolves this repo → its project;
   - `flow_create_doc` writes a `memory` doc to the project;
   - `flow_search_docs` (project-scoped) finds it; `"global"` widens it;
   - `flow_update_doc` on a human-owned doc is refused without `confirm`, accepted with;
   - `curl` smoke of `GET /api/v1/documents?projectId=…` (list + `q=` search).
5. main-wiring verification per `feedback_plan_main_wiring_task`: `cmd/flow-mcp/main.go`
   actually wires clientauth + auth gate + resolve + every one of the 11 tools and the
   resource handler; each tool exercised at least once.

## 12. Out of scope / future

V2 memory population + SessionStart recall hook (dotfiles); worktime/day-off/export/stats
tools; remote HTTP transport; `listChanged` subscriptions beyond add/remove; mutating
`type`/`project`/`path` on update.
