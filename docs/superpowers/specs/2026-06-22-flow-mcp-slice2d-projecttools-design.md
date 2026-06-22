# flow-mcp Slice 2d — Project Tools + Runbook + Live Done-Gate (Design)

**Date:** 2026-06-22
**Branch:** `rebuild` (worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`) — NOT merged, per the long-lived-integration-branch convention.
**Status:** approved in brainstorm; this is the just-in-time delta over the v1 design.

## Goal

Complete the flow-mcp V1 tool surface (11 tools) by adding the two project-binding tools, ship a registration runbook, run the deferred live `.mcp.json` done-gate, and close the rolled-up MCP Minors from 2a/2b/2c/durable-auth.

## Context & source of truth

- The two tools are **already specified** in `docs/superpowers/specs/2026-06-21-flow-mcp-v1-design.md` §7.2 (rows `flow_bind_project`, `flow_list_projects`). That table is the authoritative param/behavior reference; this doc records only the 2d-specific decisions and the integration with the code as it evolved since (durable-auth / Säule A, Säule B).
- Slices done on `rebuild`: 2a (spine + `flow_project_context`), 2b (5 read tools), 2c (3 write tools + guard + `flow://doc` resources), Säule A (durable in-process reauth: `authManager`), Säule B (embed hardening). 2d is the final V1 slice.
- **What changed since the v1 spec:** durable-auth introduced `authManager`. Every tool handler now runs its apiclient work inside `h.mgr.Do(ctx, func(c *apiclient.Client) error { … })` (auth-retry/relogin transparent). The two new tools MUST follow this pattern. `mgr.onAuth = h.postAuthInit` runs resolve+registerResources once on first auth.

## 1. Scope

1. `flow_list_projects` tool.
2. `flow_bind_project` tool (+ inline `cmd/flow-mcp/bind.go` orchestration).
3. Registration runbook `docs/runbook/flow-mcp-setup.md`.
4. Live `.mcp.json` done-gate against **PROD** (`flow.thebackend.org`), the deferred 2a item.
5. Close the open MCP Minors (§5).

Out of scope: extracting a shared `internal/projectbind` (decided against — the bind primitives `apiclient.BindRemote/BindPath`, `gitremote`, `clientmachine` are already shared; the CLI orchestration is interactive-UX-entangled, so a shared orchestrator would be a premature abstraction). The MCP bind orchestration lives inline in `cmd/flow-mcp/bind.go`.

## 2. Integration surface (as-built, post-durable-auth)

- `handlers` struct (`server.go`): holds `mgr *authManager`, resolved-project state `proj/matched` (written under `projMu`), `srv *mcp.Server`, and the lazily-fetched project cache (`projectList`/`lookupProject` in `scope.go`, 2b).
- Tool handler signature: `func (h *handlers) X(ctx, _ *mcp.CallToolRequest, in XIn) (*mcp.CallToolResult, any, error)`; apiclient work inside `h.mgr.Do`.
- `h.resolveScope(ctx, project)` (`scope.go`) resolves a `project` ref (id|slug|name, `"global"`→nil, `"none"`→IS NULL, `""`→cwd-default) with refresh-once-on-miss; unknown ref → actionable error.
- `h.postAuthInit(ctx, c)` (`server.go`) = resolve project + `registerResources`, run once via `mgr.onAuth`.
- `h.registerResources(ctx, c)` (`resources.go`) registers `flow://doc/{id}` for the resolved project.
- `clientmachine.Load() (Machine, error)`, `Machine{ID, Label string}`. `gitremote.OriginSlug(dir) (slug string, ok bool, err error)`.
- apiclient: `CreateProject(ctx, name) (Project, error)`, `ListProjects(ctx) ([]Project, error)`, `BindRemote(ctx, projectID, remoteSlug) (ProjectBinding, error)`, `BindPath(ctx, projectID, machineID, machineLabel, path) (ProjectBinding, error)`.

## 3. `flow_list_projects`

- **Params:** none.
- **Behavior:** inside `h.mgr.Do`, fetch the project list via the existing cache helper (`h.projectList(ctx, c)`), format as concise lines `name (slug) — id`. Helps the model pick an existing project before `flow_bind_project` rather than duplicate-creating.
- **Registration:** unconditional `mcp.AddTool` in `newServerH`. Degraded (unauthed) → "Login required" like every tool.

## 4. `flow_bind_project`

- **Params:** `project?` (id|slug|name of an existing project) **xor** `create_name?` (create a new project then bind); optional `kind?` (`"remote"` | `"path"`) overriding auto-detect.
- **Orchestration** (inline `cmd/flow-mcp/bind.go`, called inside `h.mgr.Do`):
  1. `cwd, _ := os.Getwd()`.
  2. Resolve the target project id:
     - exactly one of `project` / `create_name` must be set → else `IsError` ("give either project or create_name").
     - `create_name` → `c.CreateProject(ctx, create_name)`.
     - `project` → resolve the ref to an existing project (reuse the 2b lookup, e.g. `h.lookupProject`); unknown ref → `IsError` listing known slugs (never silent — search-trauma convention).
  3. Determine kind: `kind == "remote"`, or (`kind == ""` and `gitremote.OriginSlug(cwd)` returns `ok`) → **remote** bind; otherwise **path** bind.
  4. Bind:
     - remote → `c.BindRemote(ctx, projectID, originSlug)`.
     - path → `m, _ := clientmachine.Load()`; `c.BindPath(ctx, projectID, m.ID, m.Label, filepath.Clean(cwd))`.
  5. On success **re-resolve + refresh resources**: re-run resolution and `registerResources`, overwriting `h.proj/h.matched` under `projMu`. Extract the resolve+register body of `postAuthInit` into a method (e.g. `h.refreshResolved(ctx, c)`) that both `postAuthInit` (once) and `bindProject` (on demand) call.
  6. Return concise text: bind kind + bound project (name/slug) + "now resolved to <project>" (+ doc count if cheap).
- **Errors:** both/neither of project+create_name; unknown `project` ref; an invalid explicit `kind`. Bind/create/resolve API errors propagate through `mgr.Do` (stay retryable on 401).
- **Edge:** path bind when `clientmachine.Load` fails → degrade to an `IsError` with the cause (do not crash). Remote bind requires a git origin; if `kind:"remote"` forced but no origin → `IsError`.

## 5. Open Minors to close (rolled up from prior slices' ledgers)

Already fixed in `3aa4ba4` (durable-auth final-review wave) — **excluded:** `isAuthError` oauth2.RetrieveError coverage; the unknown-project scope-error rewording (`errGuard`).
Intentional keeps — **leave as-is, do not "fix":** `document_types_test.go` hardcoded `8` (drift guard); `searchDocsIn.Limit` negative→default clamp (sensible for an LLM-facing tool); `format.go` `TrimRight` (only `formatDoc` emits body, which skips it).

To close:
- **Test gaps:** `loopback`: `flow_list_tags` scoped-branch assertion (project=alpha → its tags, not beta); `flow_search_docs` empty-query `IsError` case; `loopback_write` `ListResources`-after-create/delete error checks; `auth_manager_test` add `builds==1` to `TestAuthManager_Do_NonAuthErrorNotRetried`; `scope_test` global/none err-guards (`g,_:=` → `t.Fatal` on err); loopback goroutine cancel path (`context.WithCancel` + `t.Cleanup`).
- **Small cleanups:** `requireType` invalid-type branch shares `typeList()` with `scope.go` `checkType` (DRY); align create/update success-message format; `mcpLog()` → a package-level `*slog.Logger` var (no per-call alloc); `removeResource`-on-absent one-line comment; drop the inert `wrapped:=…;_=wrapped` lines in `clientauth_errors_test.go`; verify whether the `&h.proj.ID` address-of-field note still applies after the durable-auth handler refactor and add a one-line comment if so.

## 6. Runbook — `docs/runbook/flow-mcp-setup.md`

Registration + operations doc. Confirm the exact `.mcp.json` / `claude mcp add` schema via the `claude-docs-consultant` skill (do not hand-wave the format). Cover: build `bin/flow-mcp`; the `.mcp.json` entry (command = built binary, env `FLOW_SERVER_URL`, `FLOW_OIDC_ISSUER`); `flow login` (device-flow) then `/mcp` reconnect; the 11-tool surface; the durable-reauth behavior (Säule A — relogin without reconnect); degraded "Login required" before auth; and the clean-disconnect exit-0 note.

## 7. Done-gate

1. `make ci` green (golangci-lint 0, `verify-generate` clean, coverage ≥ 80%, build; pgstore Docker tests RUN via podman — **gate is golangci-lint, not gofmt**).
2. `go build -o bin/flow-mcp ./cmd/flow-mcp`.
3. **Main-wiring verification** (per `feedback_plan_main_wiring_task`): `main.go` → `newBootManager` → `newServerH` wires all **11** tools + the resource handler; throwaway `CommandTransport` client lists 11 tools.
4. **Live gate vs PROD** (`.mcp.json` → `flow.thebackend.org`; `flow login`; `/mcp` reconnect to load the 2d binary): `flow_list_projects` lists projects; **`flow_bind_project` binds THIS repo** (flow-rebuild) → `flow_project_context` now resolves it → `flow://doc` resources appear; a scoped `flow_search_docs` finds a doc in the bound project; a guarded `flow_update_doc` refused w/o `confirm`, accepted with. Clean up any test docs created.
5. **Assert exit-0 on clean disconnect** (closes the 2a os.Exit Minor): on host-initiated stdin close the process exits 0 (only abnormal termination exits non-zero).

Stdout hygiene (no writes to stdout except the SDK's JSON-RPC) must hold — re-confirm via the loopback/stdio smoke since `bind.go` adds logging.

## 8. Testing

- **Unit:** `bind.go` kind-detect + project-ref-or-create resolution (table tests over a fake/httptest backend); the closed-Minor unit cases (§5).
- **Integration (`loopback_test.go`, real httptest backend + in-memory transport):** assert tool count = **11**; `flow_list_projects` returns the fixture projects; `flow_bind_project` remote-bind (cwd with a git origin), path-bind (no origin), create-then-bind, and both/neither-ref `IsError`; after bind, `flow_project_context`/scoped reads reflect the newly-resolved project; degraded "Login required" for both new tools.
- `make ci` green at the gate.

## 9. File layout (Keine Monolithen)

```
cmd/flow-mcp/bind.go              # NEW: non-interactive bind orchestration (detect-kind + find-or-create + BindRemote/BindPath)
cmd/flow-mcp/tools_project.go     # + flow_list_projects + flow_bind_project handlers (param structs + mgr.Do bodies)
cmd/flow-mcp/server.go            # + 2 AddTool registrations (→ 11); extract refreshResolved from postAuthInit
cmd/flow-mcp/resources.go         # (refreshResolved reuse for bind-time re-register)
cmd/flow-mcp/loopback_test.go     # 11-tool surface + bind/list integration
cmd/flow-mcp/bind_test.go         # NEW: bind orchestration unit tests
docs/runbook/flow-mcp-setup.md    # NEW: registration runbook
# + the §5 Minor touch-ups across scope.go/format/auth_manager_test/clientauth_errors_test/loopback_write_test
```

## 10. Global constraints

- **CI gate:** `make ci` = golangci-lint + verify-generate + cover(≥80%) + build. NOT gofmt (`make fmt` is manual/ungated). Run pgstore Docker tests via podman so coverage counts them.
- **ST1005:** error strings lowercase, no trailing punctuation.
- **Commit trailers:** every commit ends with
  ```
  Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01CicEBhyR4hr3ci2gyoneiP
  ```
- **Execution:** subagent-driven, Sonnet implementer / Opus reviewer per task; final whole-slice review on Opus. Slice BASE = the `rebuild` HEAD at the plan commit.
- **No migration** (no schema change). No new env vars.
