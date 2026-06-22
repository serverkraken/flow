# flow-mcp Durable Auth + Reauth — Design

Date: 2026-06-22 · Branch: `rebuild` · Status: approved for planning

## 1. Context

flow-mcp is a stdio MCP server (`cmd/flow-mcp`) exposing the flow Kompendium to AI
clients, thin over `internal/adapter/apiclient`, authed via the shared
`internal/clientauth` builder (stored OIDC device-flow token in `tokenstore`). It runs
as a **session-scoped** subprocess: Claude Code launches it on connect and kills it on
disconnect; its lifetime is one work session (hours), not 24/7.

**Verified deploy reality (2026-06-22):** PROD `flow.thebackend.org` runs the **`rebuild`
image @ commit `93f9ad4`** (GHCR digest `2563f45d`, tag `:rebuild`; `origin/main` of
homelab-study pins it). So the auth code described here is exactly what is deployed; a fix
on `rebuild` reaches PROD via: CI builds `:rebuild` → bump the digest in homelab-study →
ArgoCD sync.

### 1.1 The problem (observed during the Slice 2c live dogfood)

The MCP dropped authentication and could not recover without a manual `flow login` **and**
an MCP reconnect. Three independent root causes, all confirmed in code:

1. **Boot-once auth latch.** `cmd/flow-mcp/auth.go:bootClient` calls `Whoami` once at
   boot and `main` latches the result into `handlers.authed`. A later token expiry leaves
   the process permanently degraded; auth is never re-checked.
2. **Boot-once keyring read.** `clientauth.Client()` calls `tokenstore.Open().Load()` once
   and holds the token in memory (`lazyDeviceSource.last`). The running process never
   re-reads the keyring, so a fresh `flow login` (which writes a new token to the keyring)
   is invisible to it → a reconnect (process restart) was required to pick it up.
3. **Refresh-token expiry.** The Authentik `flow-cli` provider sets
   `refresh_token_validity: days=30`. With sporadic use (repo work spans >30 days between
   prod logins) the refresh token lapses → `invalid_grant` → only a fresh login recovers.

### 1.2 Goal / non-goals

**Goal:** the running flow-mcp process stays authenticated across token refreshes, and
**recovers in-process after a `flow login` with no reconnect**; and forced re-logins
become rare (generous, sliding refresh-token validity).

**Non-goals:** a background keep-alive goroutine (YAGNI — the process is session-scoped;
on-demand refresh on each session's first use plus a sliding Authentik window suffices);
changes to the flow CLI's auth (it is short-lived, one command per process); changing the
web auth-code flow's behavior beyond a consistent validity bump.

## 2. Resolved decisions

1. **Recovery mechanism → option A: lazy per-call with transparent retry**, localized in
   `cmd/flow-mcp`. `clientauth` stays unchanged (no risk to the CLI; fully covers the
   reconnect pain).
2. **Idle durability → on-demand refresh + generous, rotating Authentik validity.** No
   background-refresh goroutine.
3. **Late-auth recovery → option (i): full coherent recovery.** A run-once post-auth init
   (project resolution + resource registration) fires on the first successful auth,
   whether at boot or later — no half-initialized state after a mid-session login.
4. **Refresh-token validity → `days=90`** for `flow-cli` (and consistently `flow-web`),
   pending verification (planning phase, `authentik-expert` skill) that Authentik slides
   the window on rotation rather than enforcing an absolute-from-initial-login expiry.

## 3. Architecture

### 3.1 `authManager` (NEW — `cmd/flow-mcp/auth_manager.go`)

A concurrency-safe owner of the authenticated client and the auth lifecycle. Replaces the
latched `handlers.client`/`handlers.authed` fields; `handlers` gains `mgr *authManager`.

```go
type authManager struct {
    build   func(ctx context.Context) (*apiclient.Client, error) // = clientauth.Client (seam for tests)
    onAuth  func(ctx context.Context, c *apiclient.Client)        // run-once post-auth init (§3.3)

    mu      sync.Mutex
    client  *apiclient.Client
    inited  bool // post-auth init has run
}
```

- `client(ctx) (*apiclient.Client, error)` — returns the cached client, building it via
  `build` (which re-reads the keyring) if absent. On a successful build that is the first
  successful auth, triggers `onAuth` exactly once. Returns a sentinel `ErrLoginRequired`
  (wrapping the underlying "not logged in"/"expired-without-issuer" reason) when no usable
  token exists.
- `reset()` — drops the cached client so the next `client(ctx)` rebuilds (re-reads the
  keyring). Called after an auth-error response.

`build` defaults to `clientauth.Client`; tests inject a fake. `onAuth` defaults to the
post-auth init closure (§3.3); tests inject a spy to assert run-once.

### 3.2 Per-call recovery flow

A single helper wraps every tool's backend interaction:

```go
func (h *handlers) withClient(ctx, fn func(c *apiclient.Client) error) error
```

1. `c, err := h.mgr.client(ctx)`; if `err` (ErrLoginRequired) → return it (handler maps to
   the "Login required" result).
2. `err = fn(c)`; if `err` is an **auth error** (§3.2.1): `h.mgr.reset()`, rebuild via
   `h.mgr.client(ctx)`, and **retry `fn` exactly once**. If the retry also auth-fails (or
   rebuild yields ErrLoginRequired) → return ErrLoginRequired.
3. Any non-auth error (network, 5xx, 404, validation) is returned as-is — **no reset, no
   retry** (avoids needless keyring reads / double-writes on transient failures).

Handlers keep their existing shape; the `if !h.authed { return h.loginRequired() }`
prelude is replaced by routing the backend call through `withClient`, and an
`ErrLoginRequired` result is rendered with the same text as today: `"Login required: run
'flow login' in a terminal on this device."`

**Dynamic client, not a boot-bound seam.** Today `newServer` binds `h.listProjects =
client.ListProjects` once (the 2b scope-cache fetch seam) and tools read `h.client`
directly. With a rebuildable client both become stale after a `reset()`. The refactor
removes the captured `h.client`/`h.listProjects`: every backend access — the read/write
tools, `resolveScope`'s project-list fetch (`scope.go`), `projectName`, and
`registerResources` — obtains the **current** client from the manager (via `withClient`,
or for the scope cache a `listProjects` that calls `h.mgr.client(ctx)` then
`c.ListProjects(ctx)`). The 2b project-ref cache (`projects`/`projFetched`) stays as-is;
only its fetch goes through the manager.

#### 3.2.1 Auth-error classification

An error is an auth error iff:
- the apiclient surfaced an **HTTP 401**, or
- the call failed at the **oauth2 transport / token-source** layer with an auth-meaning
  cause: the lazy source's "not logged in" / "access token expired and FLOW_OIDC_ISSUER
  is not set", or an oauth2 `*oauth2.RetrieveError` / `invalid_grant` from the refresh.

Everything else (DNS, connection refused, timeouts, 5xx, 404, 400) is **not** an auth
error. The exact apiclient error shape is confirmed in planning; if 401 is not already
distinguishable, the apiclient gains a small typed/sentinel error (additive, no behavior
change) so the manager can classify reliably rather than string-matching.

### 3.3 Post-auth init (run-once, coherent recovery)

The work today done eagerly in `main` only when boot is authed — `resolveProject(cwd)`
(sets `proj`/`matched`, feeding `flow_project_context` and scope defaults) and
`registerResources` (the `flow://doc` set) — moves into an idempotent `onAuth(ctx, c)`
that the `authManager` invokes exactly once on the first successful auth. So after a
mid-session `flow login`, the next tool call both authenticates **and** resolves the
project + registers resources — no reconnect, no half-initialized state.

`registerResources` already no-ops when unmatched and is safe to call once here.

### 3.4 Boot behavior

`main` builds the `authManager` and attempts an eager `client(ctx)` + `Whoami`:
- success → `onAuth` fires now (resources ready at startup, as today);
- failure → start degraded; the first authed tool call later triggers `onAuth`.

The server **always** registers all tools and **never crashes**; all logs go to stderr
(stdout is the JSON-RPC stream).

## 4. Authentik change (homelab-study)

Blueprint `bootstrap/templates/kubernetes/apps/identity/identity/config/blueprints/52-app-flow.yaml.j2`:

- `provider-flow-cli`: `refresh_token_validity: days=30` → **`days=90`**; add the
  **`offline_access`** scope mapping to `property_mappings` (the device-flow client
  requests it; today a refresh token is still issued, but the grant should be explicit).
- `provider-flow-web`: bump `refresh_token_validity` to `days=90` for consistency
  (web sessions transparently refresh; this only lengthens the outer bound).
- **Verify with the `authentik-expert` skill (planning phase):** that Authentik issues a
  refresh token for the device-code grant with these mappings, and that **refresh-token
  rotation slides the validity window** (each refresh resets the 90-day clock) rather than
  enforcing an absolute expiry from initial login. If it is absolute, revisit the value /
  rotation settings. Confirm `offline_access` is the correct managed scope-mapping key.

Apply path: blueprint → ArgoCD sync → a fresh `flow login` → confirm the stored token's
refresh works after the access token's 5-minute TTL and that validity is ~90 days.

## 5. File layout (Keine Monolithen)

```
cmd/flow-mcp/auth_manager.go        # NEW authManager (build/onAuth seams, client/reset, run-once)
cmd/flow-mcp/auth_manager_test.go   # NEW unit tests (fake build + fake client)
cmd/flow-mcp/auth.go                # bootClient → returns the manager (eager attempt, no latch)
cmd/flow-mcp/server.go              # handlers: drop client/authed+matched/proj latches → mgr; withClient helper; newServerH wires onAuth
cmd/flow-mcp/main.go                # build manager, eager attempt, Run
cmd/flow-mcp/tools_docs.go          # route backend calls through withClient (read tools)
cmd/flow-mcp/tools_write.go         # route backend calls through withClient (write tools)
cmd/flow-mcp/tools_project.go       # project_context via manager/post-auth state
cmd/flow-mcp/resources.go           # registerResources invoked from onAuth (run-once)
internal/adapter/apiclient/…        # (only if needed) typed 401 sentinel for classification
homelab-study …/blueprints/52-app-flow.yaml.j2   # refresh_token_validity=90d + offline_access (separate repo)
```

`handlers` currently carries `proj`/`matched` (set at boot). These become manager-owned
state set by `onAuth`; tools read them through the manager (or handlers fields the manager
writes under lock) so a late post-auth init is visible to subsequent calls.

## 6. Testing

- **Unit (`auth_manager_test.go`):** fake `build` + fake client.
  - no token → `client` returns `ErrLoginRequired`; `onAuth` never runs.
  - token present → `client` builds once, `onAuth` runs exactly once (second `client` call
    does not re-run it).
  - `withClient`: a fake `fn` returning 401 once then success → exactly one `reset`+rebuild
    +retry, final success; `onAuth` still once.
  - `withClient`: `fn` returning a network error → **no** reset/retry, error propagated.
  - `withClient`: 401 on both attempts → `ErrLoginRequired`.
  - recovery: first `build` errors (no token), a later `build` succeeds (post-login) →
    `client` recovers and `onAuth` fires on that first success.
- **Loopback (existing `cmd/flow-mcp` tests):** keep all green. The authed-server helpers
  build a manager whose `build` returns the httptest-backed client; degraded-mode tests
  assert the unchanged "Login required" text. Add one loopback test: a backend that 401s
  the first documents call then 200s → the tool transparently recovers (single retry).
- `make ci` green at/above the current coverage gate (lint = `golangci-lint run`, ST1005:
  no trailing punctuation on error strings).

## 7. Done-gate

1. `make ci` green; `go build ./cmd/flow-mcp` OK; stdout-hygiene smoke (JSON-RPC only) via
   a real MCP `CommandTransport` client (the raw-pipe check does not complete the SDK
   handshake — use a client), asserting all 9 tools list.
2. **Main-wiring verification** (per `feedback_plan_main_wiring_task`): `main.go` builds
   the manager, every tool routes its backend call through `withClient`, and `onAuth`
   wires `resolveProject` + `registerResources`.
3. **Live reauth gate (the whole point), against PROD after deploy:** in a Claude Code
   session with flow-mcp registered, let the stored token expire (or `flow logout`), call
   a tool → "Login required"; run `flow login` in a terminal; **without reconnecting**,
   call a tool again → it succeeds, and `flow_project_context` + resources are coherent.
4. **Durability:** after a fresh login, confirm (Authentik admin / token inspection) the
   refresh-token validity is ~90 days and that an access-token refresh after the 5-minute
   TTL succeeds in-process.

## 8. Out of scope / sequencing

Säule B (the embed/Ollama poison-doc retry-storm fix) is a separate spec/plan after this.
No background keep-alive. No flow-CLI auth change. Deploy (image digest bump + ArgoCD) and
the Authentik blueprint apply are part of this work's done-gate but tracked as the wiring/
deploy step, not new features.
