# flow-mcp Durable Auth + Reauth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the running flow-mcp process recover authentication in-process after a `flow login` (no MCP reconnect), survive token refreshes, and rarely force re-login (generous rotating Authentik refresh-token validity).

**Architecture:** A new concurrency-safe `authManager` in `cmd/flow-mcp` owns the `*apiclient.Client` and the auth lifecycle. Tools run their backend work through `mgr.Do(ctx, fn)`, which builds the client lazily (re-reading the keyring via the existing `clientauth.Client`), runs a **run-once post-auth init** (project resolution + resource registration) on the first successful auth, and on an **auth error** (HTTP 401 or an OAuth refresh/token-source failure) resets the client, rebuilds it from the keyring, and retries the call once. Non-auth errors are never retried. A separate Authentik blueprint change lengthens the refresh-token window.

**Tech Stack:** Go 1.26.x, `github.com/modelcontextprotocol/go-sdk/mcp` v1.6.1, `golang.org/x/oauth2`, existing `internal/adapter/apiclient`, `internal/clientauth`, `internal/adapter/tokenstore`, `internal/projectresolve`. Authentik blueprint (jinja) lives in the separate `homelab-study` repo.

## Global Constraints

- Go module `github.com/serverkraken/flow`. Code changes are confined to `cmd/flow-mcp/` plus two tiny additive helpers (`apiclient.IsUnauthorized`, `clientauth` sentinel) — **no** server/pgstore/usecase/migration change. Task 4 is a separate repo (`homelab-study`).
- **stdout is the JSON-RPC stream — ALL logs go to stderr.** Never introduce a `fmt.Println`/default-logger write reaching stdout.
- **Never crash.** The server always registers all tools and starts even when unauthenticated; auth failures surface as tool results, never panics/os.Exit.
- The login-required text is exactly: `Login required: run 'flow login' in a terminal on this device.` (unchanged from today).
- **CI gate is `golangci-lint run`** (+ `verify-generate` + `cover` + `build`), NOT gofmt. Verify with `golangci-lint run`; local gofmt (go1.26.4) reports pre-existing files as false positives. **staticcheck ST1005: error strings must not be capitalized or end with punctuation.**
- **pgstore Docker tests** in the full suite need `export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"` and `export TESTCONTAINERS_RYUK_DISABLED=true`. This work adds none, but `make ci`/`cover` runs the whole suite.
- Result text is concise plain text; errors set `IsError: true` with an actionable message — never a silent success.
- Commit message trailer (every commit):
  ```
  Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
  ```

**As-built interfaces this plan consumes (verified — do NOT re-derive):**
- `clientauth.Client(ctx context.Context) (*apiclient.Client, error)` — builds the authed client from the stored token; returns an error when no token (`"not logged in — run \`flow login\`"`) or when expired without `FLOW_OIDC_ISSUER`. Re-reads the keyring on each call (the seam we exploit).
- `apiclient.APIError{Method, Path string; StatusCode int}` returned by `do` for any non-2xx; `apiclient.IsConflict(err) bool` exists (pattern to mirror).
- `apiclient.Client` methods used by tools: `Whoami`, `ListDocumentsScoped`, `SearchScoped`, `GetDocument`, `Tags`, `Backlinks`, `ListProjects`, `CreateDocument`, `UpdateDocument`, `DeleteDocument`.
- `cmd/flow-mcp` 2a/2b/2c spine: `handlers{ client *apiclient.Client; authed bool; proj domain.Project; matched bool; projMu sync.Mutex; projects []domain.Project; projFetched bool; listProjects func(ctx)([]domain.Project,error); srv *mcp.Server }`; `newServer`/`newServerH(client, authed, proj, matched)`; `textResult`/`errorResult`/`(*handlers).loginRequired()`; `(*handlers).resolveScope(ctx, project)`, `projectList`, `projectName`; `(*handlers).registerResources(ctx)`, `addResource`, `removeResource`, `inScope`; the 9 tool handlers `projectContext/searchDocs/listDocs/getDoc/listTags/backlinks/createDoc/updateDoc/deleteDoc`.
- `resolveProject(ctx, client *apiclient.Client, log *slog.Logger) (domain.Project, bool)` (`resolve.go`).
- go oauth2: a failed refresh surfaces as `*oauth2.RetrieveError` (use `errors.As`), possibly wrapped by `*url.Error` from `http.Client.Do` (errors.As unwraps it).

---

### Task 1: Error-classification primitives

Two tiny additive helpers so the manager classifies auth failures via `errors.As`/`errors.Is`, not string-matching.

**Files:**
- Modify: `internal/adapter/apiclient/client.go` (add `IsUnauthorized`)
- Create: `internal/adapter/apiclient/errors_test.go`
- Modify: `internal/clientauth/clientauth.go` (export `ErrNotLoggedIn`, wrap the two not-authable messages with it)
- Create: `internal/clientauth/clientauth_errors_test.go`

**Interfaces:**
- Produces: `apiclient.IsUnauthorized(err error) bool` (true iff err is/wraps an `*APIError` with StatusCode 401); `clientauth.ErrNotLoggedIn` (sentinel; `clientauth.Client` and the lazy source wrap their not-authable errors with it via `%w`).

- [ ] **Step 1: Write the failing test for `IsUnauthorized`**

Create `internal/adapter/apiclient/errors_test.go`:

```go
package apiclient

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestIsUnauthorized(t *testing.T) {
	if !IsUnauthorized(&APIError{Method: "GET", Path: "/x", StatusCode: http.StatusUnauthorized}) {
		t.Error("401 APIError should be unauthorized")
	}
	if !IsUnauthorized(fmt.Errorf("wrapped: %w", &APIError{StatusCode: http.StatusUnauthorized})) {
		t.Error("wrapped 401 should be unauthorized")
	}
	if IsUnauthorized(&APIError{StatusCode: http.StatusConflict}) {
		t.Error("409 is not unauthorized")
	}
	if IsUnauthorized(errors.New("network down")) {
		t.Error("plain error is not unauthorized")
	}
	if IsUnauthorized(nil) {
		t.Error("nil is not unauthorized")
	}
}
```

- [ ] **Step 2: Run it (RED)**

Run: `go test ./internal/adapter/apiclient/ -run TestIsUnauthorized`
Expected: FAIL — `undefined: IsUnauthorized`.

- [ ] **Step 3: Add `IsUnauthorized` (mirror `IsConflict`)**

In `internal/adapter/apiclient/client.go`, after `IsConflict`:

```go
// IsUnauthorized reports whether err is (or wraps) an APIError with HTTP 401.
func IsUnauthorized(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.StatusCode == http.StatusUnauthorized
}
```

- [ ] **Step 4: Run it (GREEN)**

Run: `go test ./internal/adapter/apiclient/ -run TestIsUnauthorized`
Expected: PASS.

- [ ] **Step 5: Write the failing test for the clientauth sentinel**

Create `internal/clientauth/clientauth_errors_test.go`:

```go
package clientauth

import (
	"errors"
	"testing"
)

func TestErrNotLoggedInIsSentinel(t *testing.T) {
	if ErrNotLoggedIn == nil {
		t.Fatal("ErrNotLoggedIn must be a non-nil sentinel")
	}
	wrapped := errors.New("x")
	_ = wrapped
	// The sentinel must be matchable through wrapping.
	err := errors.Join(ErrNotLoggedIn, errors.New("context"))
	if !errors.Is(err, ErrNotLoggedIn) {
		t.Error("ErrNotLoggedIn should be matchable via errors.Is through wrapping")
	}
}
```

- [ ] **Step 6: Run it (RED)**

Run: `go test ./internal/clientauth/ -run TestErrNotLoggedInIsSentinel`
Expected: FAIL — `undefined: ErrNotLoggedIn`.

- [ ] **Step 7: Add the sentinel and wrap the not-authable messages**

In `internal/clientauth/clientauth.go`: add the import `"errors"` (alongside existing imports) and the sentinel near the top of the file (after the imports):

```go
// ErrNotLoggedIn marks "no usable stored credential" — the caller should prompt
// the user to run `flow login`. Both the build-time "no token" case and the
// "access token expired and no issuer to refresh" case wrap it so callers can
// errors.Is them without string-matching.
var ErrNotLoggedIn = errors.New("not logged in — run `flow login`")
```

Then change the two not-authable returns to wrap it.

In `Client`, the no-token branch:

```go
	if !ok || loaded.AccessToken == "" {
		return nil, ErrNotLoggedIn
	}
```

In `lazyDeviceSource.Token`, the expired-without-issuer branch:

```go
		if s.cfg.OIDCIssuer == "" {
			return nil, fmt.Errorf("access token expired and FLOW_OIDC_ISSUER is not set: %w", ErrNotLoggedIn)
		}
```

- [ ] **Step 8: Run clientauth tests (GREEN) + build + lint**

Run:
```
go test ./internal/clientauth/ ./internal/adapter/apiclient/
go build ./...
golangci-lint run ./internal/clientauth/... ./internal/adapter/apiclient/...
```
Expected: PASS; build OK; lint 0 (ST1005: the sentinel text is lowercase, no trailing period — OK).

- [ ] **Step 9: Commit**

```bash
git add internal/adapter/apiclient/client.go internal/adapter/apiclient/errors_test.go internal/clientauth/clientauth.go internal/clientauth/clientauth_errors_test.go
git commit -m "feat(flow-mcp): auth-error classification primitives (IsUnauthorized + ErrNotLoggedIn)" -m "Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: The authManager (pure, unit-tested)

The novel core: lazy client ownership, run-once post-auth init, auth-error classification, and reset+rebuild+retry-once. No MCP wiring yet — fully testable with fakes.

**Files:**
- Create: `cmd/flow-mcp/auth_manager.go`
- Create: `cmd/flow-mcp/auth_manager_test.go`

**Interfaces:**
- Consumes: Task 1 (`apiclient.IsUnauthorized`, `clientauth.ErrNotLoggedIn`); `*apiclient.Client`; `*oauth2.RetrieveError`.
- Produces:
  - `type authManager struct { ... }`
  - `newAuthManager(build func(context.Context) (*apiclient.Client, error), onAuth func(context.Context, *apiclient.Client)) *authManager`
  - `(*authManager).client(ctx) (*apiclient.Client, error)` — current client (builds if absent, fires onAuth once on first success).
  - `(*authManager).reset()` — drops the cached client.
  - `(*authManager).Do(ctx, fn func(*apiclient.Client) error) error` — ensure→fn→(on auth error) reset+rebuild+retry-once.
  - `var errLoginRequired = errors.New("login required")` (sentinel the handlers map to the login-required result).
  - `isAuthError(err error) bool`.

- [ ] **Step 1: Write `auth_manager_test.go` (RED)**

Create `cmd/flow-mcp/auth_manager_test.go`:

```go
package main

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientauth"
)

// a throwaway client value; the manager never dereferences it in these tests
// because fn is a fake that ignores the client.
func dummyClient() *apiclient.Client { return apiclient.New("http://example", "tok") }

func TestAuthManager_BuildsOnceAndFiresOnAuthOnce(t *testing.T) {
	builds, auths := 0, 0
	m := newAuthManager(
		func(context.Context) (*apiclient.Client, error) { builds++; return dummyClient(), nil },
		func(context.Context, *apiclient.Client) { auths++ },
	)
	for i := 0; i < 3; i++ {
		if _, err := m.client(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if builds != 1 {
		t.Fatalf("builds = %d, want 1 (cached)", builds)
	}
	if auths != 1 {
		t.Fatalf("onAuth fired %d times, want exactly 1", auths)
	}
}

func TestAuthManager_NoTokenReturnsLoginRequired_NoOnAuth(t *testing.T) {
	auths := 0
	m := newAuthManager(
		func(context.Context) (*apiclient.Client, error) { return nil, clientauth.ErrNotLoggedIn },
		func(context.Context, *apiclient.Client) { auths++ },
	)
	_, err := m.client(context.Background())
	if !errors.Is(err, errLoginRequired) {
		t.Fatalf("err = %v, want errLoginRequired", err)
	}
	if auths != 0 {
		t.Fatal("onAuth must not fire when auth never succeeded")
	}
}

func TestAuthManager_Do_RetriesOnceOn401ThenSucceeds(t *testing.T) {
	builds := 0
	m := newAuthManager(
		func(context.Context) (*apiclient.Client, error) { builds++; return dummyClient(), nil },
		func(context.Context, *apiclient.Client) {},
	)
	calls := 0
	err := m.Do(context.Background(), func(*apiclient.Client) error {
		calls++
		if calls == 1 {
			return &apiclient.APIError{StatusCode: http.StatusUnauthorized}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do = %v, want nil after one retry", err)
	}
	if calls != 2 {
		t.Fatalf("fn called %d times, want 2 (call + one retry)", calls)
	}
	if builds != 2 {
		t.Fatalf("builds = %d, want 2 (initial + rebuild after reset)", builds)
	}
}

func TestAuthManager_Do_NonAuthErrorNotRetried(t *testing.T) {
	m := newAuthManager(
		func(context.Context) (*apiclient.Client, error) { return dummyClient(), nil },
		func(context.Context, *apiclient.Client) {},
	)
	calls := 0
	sentinel := errors.New("network down")
	err := m.Do(context.Background(), func(*apiclient.Client) error { calls++; return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do = %v, want the network error propagated", err)
	}
	if calls != 1 {
		t.Fatalf("fn called %d times, want 1 (no retry on non-auth error)", calls)
	}
}

func TestAuthManager_Do_PersistentAuthErrorBecomesLoginRequired(t *testing.T) {
	m := newAuthManager(
		func(context.Context) (*apiclient.Client, error) { return dummyClient(), nil },
		func(context.Context, *apiclient.Client) {},
	)
	calls := 0
	err := m.Do(context.Background(), func(*apiclient.Client) error {
		calls++
		return &apiclient.APIError{StatusCode: http.StatusUnauthorized}
	})
	if !errors.Is(err, errLoginRequired) {
		t.Fatalf("Do = %v, want errLoginRequired after retry also 401", err)
	}
	if calls != 2 {
		t.Fatalf("fn called %d times, want 2 (call + one retry)", calls)
	}
}

func TestAuthManager_Do_RecoversAfterReLogin(t *testing.T) {
	// First build has no token (logged out); a later build succeeds (post flow login).
	var mu sync.Mutex
	loggedIn := false
	auths := 0
	m := newAuthManager(
		func(context.Context) (*apiclient.Client, error) {
			mu.Lock()
			defer mu.Unlock()
			if !loggedIn {
				return nil, clientauth.ErrNotLoggedIn
			}
			return dummyClient(), nil
		},
		func(context.Context, *apiclient.Client) { auths++ },
	)
	// logged out → login required, onAuth never ran
	if err := m.Do(context.Background(), func(*apiclient.Client) error { return nil }); !errors.Is(err, errLoginRequired) {
		t.Fatalf("logged-out Do = %v, want errLoginRequired", err)
	}
	// user runs `flow login`
	mu.Lock()
	loggedIn = true
	mu.Unlock()
	// next call recovers WITHOUT any reconnect; onAuth fires now (first success)
	if err := m.Do(context.Background(), func(*apiclient.Client) error { return nil }); err != nil {
		t.Fatalf("post-login Do = %v, want nil", err)
	}
	if auths != 1 {
		t.Fatalf("onAuth fired %d times, want 1 (on first success after re-login)", auths)
	}
}

func TestIsAuthError(t *testing.T) {
	if !isAuthError(&apiclient.APIError{StatusCode: http.StatusUnauthorized}) {
		t.Error("401 is an auth error")
	}
	if !isAuthError(clientauth.ErrNotLoggedIn) {
		t.Error("ErrNotLoggedIn is an auth error")
	}
	if isAuthError(errors.New("dial tcp: connection refused")) {
		t.Error("network error is NOT an auth error")
	}
	if isAuthError(&apiclient.APIError{StatusCode: http.StatusInternalServerError}) {
		t.Error("500 is NOT an auth error")
	}
}
```

- [ ] **Step 2: Run it (RED)**

Run: `go test ./cmd/flow-mcp/ -run 'AuthManager|IsAuthError'`
Expected: FAIL — `undefined: newAuthManager` (compile error).

- [ ] **Step 3: Write `cmd/flow-mcp/auth_manager.go`**

```go
package main

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/oauth2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientauth"
)

// errLoginRequired is the sentinel a tool maps to the "run flow login" result.
var errLoginRequired = errors.New("login required")

// authManager owns the authenticated apiclient and the auth lifecycle for the
// long-running MCP process. It builds the client lazily (re-reading the keyring
// via build), runs onAuth exactly once on the first successful auth, and on an
// auth error drops and rebuilds the client so a fresh `flow login` is picked up
// without an MCP reconnect.
type authManager struct {
	build  func(ctx context.Context) (*apiclient.Client, error)
	onAuth func(ctx context.Context, c *apiclient.Client)

	mu     sync.Mutex
	cur    *apiclient.Client
	inited bool // onAuth has run
}

func newAuthManager(build func(context.Context) (*apiclient.Client, error), onAuth func(context.Context, *apiclient.Client)) *authManager {
	return &authManager{build: build, onAuth: onAuth}
}

// client returns the current authenticated client, building it (which re-reads
// the stored token) when absent. On the first successful build it fires onAuth
// exactly once, outside the lock (onAuth must not call back into client). A
// build failure is normalized to errLoginRequired.
func (m *authManager) client(ctx context.Context) (*apiclient.Client, error) {
	m.mu.Lock()
	if m.cur != nil {
		c := m.cur
		m.mu.Unlock()
		return c, nil
	}
	c, err := m.build(ctx)
	if err != nil {
		m.mu.Unlock()
		return nil, errLoginRequired
	}
	m.cur = c
	fire := !m.inited
	m.inited = true
	m.mu.Unlock()

	if fire && m.onAuth != nil {
		m.onAuth(ctx, c)
	}
	return c, nil
}

// reset drops the cached client so the next client() call rebuilds from the
// store. inited is left set: onAuth is a once-per-process post-auth init, not
// re-run on every recovery.
func (m *authManager) reset() {
	m.mu.Lock()
	m.cur = nil
	m.mu.Unlock()
}

// Do runs fn with the current client. On an auth error it resets, rebuilds, and
// retries fn exactly once; a persistent auth failure (or no usable token) is
// returned as errLoginRequired. Non-auth errors are returned unchanged and never
// retried.
func (m *authManager) Do(ctx context.Context, fn func(c *apiclient.Client) error) error {
	c, err := m.client(ctx)
	if err != nil {
		return err // already errLoginRequired
	}
	if err := fn(c); err == nil {
		return nil
	} else if !isAuthError(err) {
		return err
	}
	// Auth error: drop the (stale) client, rebuild from the store, retry once.
	m.reset()
	c, err = m.client(ctx)
	if err != nil {
		return err // errLoginRequired
	}
	if err := fn(c); err != nil {
		if isAuthError(err) {
			return errLoginRequired
		}
		return err
	}
	return nil
}

// isAuthError reports whether err means "the credential is bad" (so a rebuild
// from the store might help) rather than a transport/server failure. It matches
// an HTTP 401, the not-logged-in sentinel, and an OAuth refresh failure.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	if apiclient.IsUnauthorized(err) {
		return true
	}
	if errors.Is(err, clientauth.ErrNotLoggedIn) {
		return true
	}
	var re *oauth2.RetrieveError
	return errors.As(err, &re)
}
```

Note: the cached-client field is `cur` (not `client`) to avoid colliding with the `client` method. Keep it unexported; only this file touches it.

- [ ] **Step 4: Run it (GREEN) + lint**

Run:
```
go test ./cmd/flow-mcp/ -run 'AuthManager|IsAuthError' -v
golangci-lint run ./cmd/flow-mcp/...
```
Expected: all manager tests PASS; lint 0.

- [ ] **Step 5: Commit**

```bash
git add cmd/flow-mcp/auth_manager.go cmd/flow-mcp/auth_manager_test.go
git commit -m "feat(flow-mcp): authManager (lazy client, run-once post-auth init, reset+retry-once)" -m "Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Integrate the manager across cmd/flow-mcp

Replace the boot-once latch (`handlers.client`/`authed`) with the manager; route every tool's backend work through `mgr.Do`; move project-resolution + resource-registration into a run-once `onAuth`; keep all loopback tests green and add a transparent-recovery loopback test. This is one atomic refactor (removing `h.client` forces every call-site to migrate together).

**Files:**
- Modify: `cmd/flow-mcp/server.go` (handlers struct → `mgr`; `newServerH` builds the manager + wires `onAuth`; `loginRequired`/result mapping; `withClient` removed in favor of `mgr.Do`; `listProjects` seam via manager)
- Modify: `cmd/flow-mcp/auth.go` (`bootClient` → build the manager, eager attempt)
- Modify: `cmd/flow-mcp/main.go` (build manager, eager boot, Run)
- Modify: `cmd/flow-mcp/tools_project.go` (`projectContext` via manager state)
- Modify: `cmd/flow-mcp/tools_docs.go` (5 read tools via `mgr.Do`)
- Modify: `cmd/flow-mcp/tools_write.go` (3 write tools via `mgr.Do`; resource sync uses manager client)
- Modify: `cmd/flow-mcp/resources.go` (`registerResources(ctx, c)` takes the client; read closures fetch the current client)
- Modify: `cmd/flow-mcp/loopback_test.go`, `cmd/flow-mcp/loopback_write_test.go` (build servers via the manager; add a recovery test)

**Interfaces:**
- Consumes: Task 2 (`newAuthManager`, `mgr.Do`, `mgr.client`, `errLoginRequired`); Task 1 helpers; `clientauth.Client`; `resolveProject`.
- Produces: `handlers` with `mgr *authManager` (no `client`/`authed` fields); `(*handlers).resultErr(err) *mcp.CallToolResult` (maps `errLoginRequired`→login text, else `errorResult`); `newServerH(mgr *authManager) (*mcp.Server, *handlers)` and `newServer(mgr *authManager) *mcp.Server`.

- [ ] **Step 1: Rewrite the `handlers` struct + `newServerH`/`newServer` + result mapping (server.go)**

Replace the `handlers` struct and constructors in `cmd/flow-mcp/server.go`. New struct (drops `client`, `authed`; keeps the project-ref cache; adds `mgr`; `proj`/`matched`/`srv` now written by `onAuth`):

```go
type handlers struct {
	mgr *authManager
	srv *mcp.Server

	// resolved-project state, written once by onAuth under projMu.
	proj    domain.Project
	matched bool

	// project-ref cache (2b), guarded by projMu. listProjects fetches via the
	// manager's current client so a rebuild is always reflected.
	projMu       sync.Mutex
	projects     []domain.Project
	projFetched  bool
	listProjects func(ctx context.Context) ([]domain.Project, error)
}
```

Constructors:

```go
// newServer is the production entry point: it wires the tools to a handlers
// backed by mgr. onAuth (project resolution + resource registration) is attached
// to mgr by the caller before first use (see main / newServerH).
func newServer(mgr *authManager) *mcp.Server {
	s, _ := newServerH(mgr)
	return s
}

// newServerH builds the server + handlers and returns both. It also sets
// mgr.onAuth to this handlers' run-once post-auth init and h.srv to the server,
// and wires the project-ref fetch seam through the manager.
func newServerH(mgr *authManager) (*mcp.Server, *handlers) {
	h := &handlers{mgr: mgr}
	h.listProjects = func(ctx context.Context) ([]domain.Project, error) {
		c, err := mgr.client(ctx)
		if err != nil {
			return nil, err
		}
		return c.ListProjects(ctx)
	}
	mgr.onAuth = h.postAuthInit
	s := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, nil)
	h.srv = s
	// ... all existing mcp.AddTool calls (project_context + 5 read + 3 write) unchanged ...
	return s, h
}
```

(Keep every existing `mcp.AddTool(...)` line exactly as in the current `newServerH`.)

Add the result-mapping helper + the post-auth init (also in server.go, or `auth_manager.go` — keep with handlers in server.go):

```go
// resultErr maps a backend error to a tool result: errLoginRequired → the
// standard login-required message; anything else → a generic actionable error.
func (h *handlers) resultErr(err error) *mcp.CallToolResult {
	if errors.Is(err, errLoginRequired) {
		return h.loginRequired()
	}
	return errorResult("flow server error: " + err.Error())
}

// postAuthInit runs once on the first successful auth (mgr.onAuth): resolve the
// cwd→project, then register the project's documents as resources.
func (h *handlers) postAuthInit(ctx context.Context, c *apiclient.Client) {
	proj, matched := resolveProject(ctx, c, mcpLog())
	h.projMu.Lock()
	h.proj, h.matched = proj, matched
	h.projMu.Unlock()
	if err := h.registerResources(ctx, c); err != nil {
		mcpLog().Warn("could not register document resources", "err", err)
	}
}
```

Add a tiny stderr logger accessor used by postAuthInit (stdout stays JSON-RPC):

```go
func mcpLog() *slog.Logger { return slog.New(slog.NewTextHandler(os.Stderr, nil)) }
```

Update server.go imports: add `errors`, `log/slog`, `os` (remove `apiclient` only if now unused — it is still used by the seam closure signature via `domain.Project`; keep `domain`). Verify imports compile.

`loginRequired`, `textResult`, `errorResult` stay unchanged.

- [ ] **Step 2: Make `proj`/`matched` reads lock-safe (server.go helper)**

`projectContext` and `resolveScope` read `h.proj`/`h.matched`, which `postAuthInit` may write mid-session. Add an accessor and use it:

```go
func (h *handlers) resolved() (domain.Project, bool) {
	h.projMu.Lock()
	defer h.projMu.Unlock()
	return h.proj, h.matched
}
```

In `scope.go`, change `resolveScope`'s default-branch read from `h.matched`/`h.proj` to `h.resolved()`:

```go
	case "":
		if proj, matched := h.resolved(); matched {
			id := proj.ID
			return scope{projectID: &id, label: "in project " + proj.Name}, nil
		}
		return scope{projectID: nil, label: "across all projects (no project is bound to this directory — use flow_bind_project)"}, nil
```

(Leave the rest of `resolveScope` unchanged; it already uses `h.projectList`/`lookupProject`.)

- [ ] **Step 3: Rewrite `bootClient` + `main` (auth.go, main.go)**

`cmd/flow-mcp/auth.go` — replace `bootClient` with a one-line manager constructor. `clientauth.Client` has signature `func(context.Context) (*apiclient.Client, error)`, which matches the manager's `build` type exactly, so it is passed directly. `onAuth` is nil here and wired by `newServerH`:

```go
package main

import "github.com/serverkraken/flow/internal/clientauth"

// newBootManager builds the authManager whose client is the shared clientauth
// builder (it re-reads the stored token on each build, so a fresh `flow login`
// is picked up without a reconnect). onAuth is wired by newServerH; auth is
// driven lazily by the first tool call (and an eager warm in main).
func newBootManager() *authManager {
	return newAuthManager(clientauth.Client, nil)
}
```

(Delete the old `bootClient` function entirely; nothing else references it after main.go is updated below.)

`cmd/flow-mcp/main.go`:

```go
func main() {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	mgr := newBootManager()
	srv, h := newServerH(mgr) // wires mgr.onAuth = h.postAuthInit
	_ = h

	// Eager warm: if a valid token is stored, this builds the client, fires the
	// run-once post-auth init (resolve project + register resources), and logs
	// who we are. Failures are expected when logged out — the server still starts
	// and recovers on the first authed tool call.
	if c, err := mgr.client(ctx); err != nil {
		log.Warn("not authenticated at boot; tools will require login until `flow login`", "err", err)
	} else if u, err := c.Whoami(ctx); err != nil {
		log.Warn("token present but server rejected it; will retry lazily", "err", err)
	} else {
		log.Info("flow-mcp authenticated", "user", u.Email)
	}

	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Error("flow-mcp exited", "err", err)
		os.Exit(1)
	}
}
```

Imports for main.go: `context`, `log/slog`, `os`, the mcp pkg. `domain` no longer needed in main.go (proj/matched are internal now) — remove it if unused.

Subtlety to honor: the eager `mgr.client(ctx)` fires `postAuthInit` once. If Whoami then fails (token rejected), the client is cached but the server will 401 on real calls → `mgr.Do` resets+rebuilds. That is correct. Do not gate `postAuthInit` on Whoami; it is cheap and idempotent and re-resolution on later recovery is not needed (inited stays set).

- [ ] **Step 4: Route `projectContext` through the manager (tools_project.go)**

```go
func (h *handlers) projectContext(ctx context.Context, _ *mcp.CallToolRequest, _ projectContextIn) (*mcp.CallToolResult, any, error) {
	proj, matched := h.resolved()
	if !matched {
		// Either unauthed (no project resolved yet) or genuinely unbound. Probe
		// auth so a logged-out caller gets the actionable login message.
		if _, err := h.mgr.client(ctx); err != nil {
			return h.resultErr(err), nil, nil
		}
		return textResult("No flow project is bound to this directory. Set FLOW_PROJECT, or bind it (flow_bind_project, coming in a later version)."), nil, nil
	}
	var count int
	err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		docs, err := c.ListDocumentsScoped(ctx, &proj.ID)
		if err != nil {
			return err
		}
		count = len(docs)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	msg := fmt.Sprintf("Project: %s (%s) — %d document(s) in scope. Resolved for this working directory.", proj.Name, proj.Slug, count)
	return textResult(msg), nil, nil
}
```

- [ ] **Step 5: Route the 5 read tools through `mgr.Do` (tools_docs.go)**

Each handler: validate inputs first (no client needed), then do the backend work inside `mgr.Do`, capturing the result text in an outer variable. Replace the bodies (keep the param structs + `resolveDocRef`/filters unchanged):

```go
func (h *handlers) searchDocs(ctx context.Context, _ *mcp.CallToolRequest, in searchDocsIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Query) == "" {
		return errorResult("query is required"), nil, nil
	}
	typ, err := checkType(in.Type)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	var out string
	err = h.mgr.Do(ctx, func(c *apiclient.Client) error {
		sc, err := h.resolveScope(ctx, in.Project)
		if err != nil {
			return err
		}
		hits, err := c.SearchScoped(ctx, in.Query, sc.projectID, in.Tags...)
		if err != nil {
			return err
		}
		if typ != "" {
			hits = filterHitsByType(hits, typ)
		}
		limit := in.Limit
		if limit <= 0 {
			limit = defaultSearchLimit
		}
		if len(hits) > limit {
			hits = hits[:limit]
		}
		out = formatSearchHits(hits, in.Query, sc)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}

func (h *handlers) listDocs(ctx context.Context, _ *mcp.CallToolRequest, in listDocsIn) (*mcp.CallToolResult, any, error) {
	typ, err := checkType(in.Type)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	var out string
	err = h.mgr.Do(ctx, func(c *apiclient.Client) error {
		sc, err := h.resolveScope(ctx, in.Project)
		if err != nil {
			return err
		}
		docs, err := c.ListDocumentsScoped(ctx, sc.projectID, in.Tags...)
		if err != nil {
			return err
		}
		if typ != "" {
			docs = filterDocsByType(docs, typ)
		}
		out = formatDocList(docs, sc)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}

func (h *handlers) getDoc(ctx context.Context, _ *mcp.CallToolRequest, in getDocIn) (*mcp.CallToolResult, any, error) {
	var out string
	err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		sc, _ := h.resolveScope(ctx, "")
		id, err := h.resolveDocRef(ctx, c, in.ID, in.Path, sc)
		if err != nil {
			return err
		}
		d, err := c.GetDocument(ctx, id)
		if err != nil {
			return err
		}
		out = formatDoc(d, h.projectName(ctx, d.ProjectID))
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}

func (h *handlers) listTags(ctx context.Context, _ *mcp.CallToolRequest, in listTagsIn) (*mcp.CallToolResult, any, error) {
	var out string
	err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		sc, err := h.resolveScope(ctx, in.Project)
		if err != nil {
			return err
		}
		var tags []domain.TagCount
		if sc.projectID == nil {
			tags, err = c.Tags(ctx)
		} else {
			var docs []domain.Document
			docs, err = c.ListDocumentsScoped(ctx, sc.projectID)
			if err == nil {
				tags = domain.CollectTags(docs)
			}
		}
		if err != nil {
			return err
		}
		out = formatTags(tags, sc)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}

func (h *handlers) backlinks(ctx context.Context, _ *mcp.CallToolRequest, in backlinksIn) (*mcp.CallToolResult, any, error) {
	ref := strings.TrimSpace(in.ID)
	if ref == "" {
		ref = strings.TrimSpace(in.Path)
	}
	var out string
	err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		sc, _ := h.resolveScope(ctx, "")
		id, err := h.resolveDocRef(ctx, c, in.ID, in.Path, sc)
		if err != nil {
			return err
		}
		refs, err := c.Backlinks(ctx, id)
		if err != nil {
			return err
		}
		out = formatBacklinks(refs, ref)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
```

`resolveDocRef` gains a client param (it calls `ListDocumentsScoped`). In `tools_docs.go` change its signature + the one internal call:

```go
func (h *handlers) resolveDocRef(ctx context.Context, c *apiclient.Client, id, path string, sc scope) (string, error) {
	id, path = strings.TrimSpace(id), strings.TrimSpace(path)
	switch {
	case id != "" && path != "":
		return "", fmt.Errorf("pass either id or path, not both")
	case id != "":
		return id, nil
	case path == "":
		return "", fmt.Errorf("pass either id or path")
	}
	docs, err := c.ListDocumentsScoped(ctx, sc.projectID)
	if err != nil {
		return "", err
	}
	// ... unchanged match logic ...
}
```

Note: `resolveScope`/`projectName` internally use `h.listProjects` (now manager-backed) — they need no client param. Only `resolveDocRef` takes `c` (it calls `ListDocumentsScoped` directly).

- [ ] **Step 6: Route the 3 write tools through `mgr.Do` (tools_write.go)**

```go
func (h *handlers) createDoc(ctx context.Context, _ *mcp.CallToolRequest, in createDocIn) (*mcp.CallToolResult, any, error) {
	typ, err := requireType(in.Type)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	if strings.TrimSpace(in.Path) == "" || strings.TrimSpace(in.Title) == "" {
		return errorResult("path and title are required"), nil, nil
	}
	var out string
	err = h.mgr.Do(ctx, func(c *apiclient.Client) error {
		sc, err := h.resolveScope(ctx, in.Project)
		if err != nil {
			return err
		}
		pid := sc.projectID
		if pid != nil && *pid == "none" {
			pid = nil
		}
		d, err := c.CreateDocument(ctx, apiclient.CreateDocumentInput{
			Type: string(typ), ProjectID: pid, Path: in.Path, Title: in.Title, Body: in.Body,
		})
		if err != nil {
			return err
		}
		h.addResource(ctx, d)
		out = fmt.Sprintf("Created %s [%s] %s · %s.", d.Type, d.ID, d.Title, d.Path)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}

func (h *handlers) updateDoc(ctx context.Context, _ *mcp.CallToolRequest, in updateDocIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errorResult("id is required"), nil, nil
	}
	var out string
	err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		cur, err := c.GetDocument(ctx, in.ID)
		if err != nil {
			return err
		}
		if err := guardMutation(cur, in.Confirm); err != nil {
			return errGuard{err}
		}
		payload, err := mergeUpdate(cur, in.Title, in.Body)
		if err != nil {
			return errGuard{err}
		}
		d, err := c.UpdateDocument(ctx, in.ID, payload)
		if err != nil {
			return err
		}
		h.removeResource(d.ID)
		h.addResource(ctx, d)
		out = fmt.Sprintf("Updated [%s] %s · %s.", d.ID, d.Title, d.Path)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}

func (h *handlers) deleteDoc(ctx context.Context, _ *mcp.CallToolRequest, in deleteDocIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errorResult("id is required"), nil, nil
	}
	var out string
	err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
		cur, err := c.GetDocument(ctx, in.ID)
		if err != nil {
			return err
		}
		if err := guardMutation(cur, in.Confirm); err != nil {
			return errGuard{err}
		}
		if err := c.DeleteDocument(ctx, in.ID); err != nil {
			return err
		}
		h.removeResource(cur.ID)
		out = fmt.Sprintf("Deleted [%s] %s.", cur.ID, cur.Title)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
```

**Guard-error wrapping:** the write guard (`guardMutation`) and `mergeUpdate` produce *user-facing* errors that must surface verbatim (e.g. `"… is a human-owned note … Pass confirm=true …"`), not be reworded by `resultErr` and never trigger a login retry. Wrap them in a marker type so `resultErr` renders them as-is. Add to `tools_write.go`:

```go
// errGuard marks a guard/validation error whose message is meant for the user
// verbatim (not an auth or transport failure).
type errGuard struct{ err error }

func (e errGuard) Error() string { return e.err.Error() }
```

And update `resultErr` (server.go) to render guard errors directly and never treat them as auth errors:

```go
func (h *handlers) resultErr(err error) *mcp.CallToolResult {
	var g errGuard
	if errors.As(err, &g) {
		return errorResult(g.Error())
	}
	if errors.Is(err, errLoginRequired) {
		return h.loginRequired()
	}
	return errorResult("flow server error: " + err.Error())
}
```

(`errGuard` is a non-auth error, so `isAuthError` returns false for it → no retry. Good.)

- [ ] **Step 7: `registerResources(ctx, c)` + resource read via current client (resources.go)**

Change `registerResources` to take the client (called from `postAuthInit`), and make `addResource` take ctx + capture nothing client-ish (the read closure fetches the current client from the manager so reads survive a rebuild):

```go
func (h *handlers) registerResources(ctx context.Context, c *apiclient.Client) error {
	proj, matched := h.resolved()
	if !matched {
		return nil
	}
	docs, err := c.ListDocumentsScoped(ctx, &proj.ID)
	if err != nil {
		return err
	}
	for _, d := range docs {
		h.addResource(ctx, d)
	}
	return nil
}

func (h *handlers) inScope(d domain.Document) bool {
	proj, matched := h.resolved()
	return matched && d.ProjectID != nil && *d.ProjectID == proj.ID
}

func (h *handlers) addResource(ctx context.Context, d domain.Document) {
	if h.srv == nil || !h.inScope(d) {
		return
	}
	id := d.ID
	h.srv.AddResource(resourceFor(d), func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		c, err := h.mgr.client(ctx)
		if err != nil {
			return nil, err
		}
		doc, err := c.GetDocument(ctx, id)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI: docURI(id), MIMEType: "text/markdown", Text: doc.Body,
		}}}, nil
	})
}
```

`removeResource`, `docURI`, `resourceFor` unchanged. `addResource` now takes `ctx` — update the three call-sites in tools_write.go (already shown in Step 6 passing `ctx`).

- [ ] **Step 8: Update loopback test helpers + add the recovery test (loopback_test.go, loopback_write_test.go)**

The helpers currently call `newServer(client, true, proj, true)` / `newServerH(client, true, proj, true)`. They must now build a manager whose `build` returns the fixture client, and **seed the resolved project directly** — the fixture backends do not implement the V0 resolution endpoints, so the real `postAuthInit`→`resolveProject` would resolve `matched=false` and clobber the seed. So the helper sets `mgr.onAuth = nil` (the run-once init is unit-tested in Task 2 and exercised live in the done-gate; loopback seeds state directly) and connects to **`h.srv`** (the server `newServerH` already built — do not build a second one).

Add a shared helper to `loopback_test.go`:

```go
// managerFor builds an authManager that always returns the given client, with
// the resolved project seeded directly (fixtures lack the V0 resolution
// endpoints, so onAuth is disabled here). Returns the manager and the handlers
// whose h.srv the caller connects to.
func managerFor(t *testing.T, client *apiclient.Client, proj domain.Project) (*authManager, *handlers) {
	t.Helper()
	mgr := newAuthManager(func(context.Context) (*apiclient.Client, error) { return client, nil }, nil)
	_, h := newServerH(mgr) // newServerH sets mgr.onAuth = h.postAuthInit …
	mgr.onAuth = nil        // … which we disable: loopback fixtures can't drive V0 resolution.
	h.projMu.Lock()
	h.proj, h.matched = proj, true
	h.projMu.Unlock()
	return mgr, h
}

// degradedSession builds a logged-out server (build always fails).
func degradedSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	mgr := newAuthManager(func(context.Context) (*apiclient.Client, error) { return nil, clientauth.ErrNotLoggedIn }, nil)
	_, h := newServerH(mgr)
	return connect(t, h.srv)
}
```

Then update the existing authed helpers to the same pattern:

- `loopback_test.go` `TestLoopback_ProjectContext_Authed`:
  ```go
	mgr, h := managerFor(t, client, proj)
	_ = mgr
	sess := connect(t, h.srv)
  ```
- `loopback_test.go` `authedReadServer` and `loopback_write_test.go` `authedWriteServer`:
  ```go
	mgr, h := managerFor(t, client, proj) // proj = {ID:"p1",Name:"Alpha",Slug:"alpha"}
	_ = mgr
	return connect(t, h.srv)
  ```
- `loopback_write_test.go` `authedWriteServerWithResources` — seed via `managerFor`, then register resources explicitly with the fixture client (onAuth is nil here):
  ```go
	mgr, h := managerFor(t, client, proj)
	if err := h.registerResources(context.Background(), client); err != nil {
		t.Fatalf("registerResources: %v", err)
	}
	return connect(t, h.srv), h
  ```
- The degraded-mode tests (`TestLoopback_ProjectContext_DegradedRequiresLogin`, `TestLoopback_ReadTools_DegradedRequireLogin`, `TestLoopback_WriteTools_DegradedRequireLogin`): replace the old `newServer(nil, false, domain.Project{}, false)` construction with `degradedSession(t)`. Assertions (`IsError` + "Login required") stay identical.

Add the transparent-recovery test to `loopback_write_test.go`:

```go
// fakeReauthBackend 401s the first documents request, then serves normally —
// proving mgr.Do resets+rebuilds+retries within a single tool call.
func TestLoopback_Reauth_TransparentRetryOn401(t *testing.T) {
	var mu sync.Mutex
	first := true
	p1 := "p1"
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		f := first
		first = false
		mu.Unlock()
		if f {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode([]domain.Document{{ID: "d1", OwnerID: "u1", ProjectID: &p1, Type: domain.DocMemory, Path: "p", Title: "t"}})
	})
	be := httptest.NewServer(mux)
	t.Cleanup(be.Close)

	builds := 0
	mgr := newAuthManager(func(context.Context) (*apiclient.Client, error) {
		builds++
		return apiclient.New(be.URL, "tok"), nil
	}, nil)
	srv, h := newServerH(mgr)
	mgr.onAuth = nil // seed resolution directly; fixture lacks V0 endpoints
	h.projMu.Lock()
	h.proj, h.matched = domain.Project{ID: "p1", Name: "Alpha", Slug: "alpha"}, true
	h.projMu.Unlock()
	sess := connect(t, srv)

	res, out := callText(t, sess, "flow_list_docs", map[string]any{})
	if res.IsError {
		t.Fatalf("list after transparent reauth = error %q, want success", out)
	}
	if !strings.Contains(out, "d1") {
		t.Fatalf("list = %q, want d1 after retry", out)
	}
	if builds < 2 {
		t.Fatalf("builds = %d, want >= 2 (initial + rebuild on 401)", builds)
	}
}
```

(Add imports `sync`, `net/http`, `net/http/httptest`, `encoding/json` to the test file if not already present — `loopback_write_test.go` already imports them.)

- [ ] **Step 9: Build, full flow-mcp test, lint (GREEN)**

Run:
```
go build ./...
go test ./cmd/flow-mcp/ -v
golangci-lint run ./cmd/flow-mcp/...
```
Expected: build OK; ALL flow-mcp tests pass (2a/2b/2c loopback + manager unit + the new recovery test); lint 0. Fix any compile fallout from the struct change (every `h.client`/`h.authed`/`h.matched` direct reference must now go through `mgr`/`resolved()`); there should be none left outside the migrated files.

- [ ] **Step 10: Commit**

```bash
git add cmd/flow-mcp/
git commit -m "feat(flow-mcp): durable in-process reauth via authManager (no reconnect after flow login)" -m "Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: Authentik refresh-token validity (homelab-study)

Lengthen the `flow-cli` (and `flow-web`) refresh-token window and make the `offline_access` grant explicit, after verifying Authentik's rotation semantics. This is a **separate repo** (`/Users/msoent/SourceCode/serverkraken/homelab-study`) and a **separate commit/PR**; it does not touch the flow module.

**Files:**
- Modify: `bootstrap/templates/kubernetes/apps/identity/identity/config/blueprints/52-app-flow.yaml.j2`
- (rendered) `kubernetes/apps/identity/identity/config/blueprints/52-app-flow.yaml` — regenerate per the repo's render flow (`reference_homelab_study_gitops_quirks`: render-then-commit, `mise exec` for makejinja).

- [ ] **Step 1: Verify Authentik semantics with the `authentik-expert` skill**

Invoke the `authentik-expert` skill and confirm, for an OAuth2 provider used via the RFC-8628 device-code grant + refresh_token grant:
1. Whether `refresh_token_validity` is **sliding** (reset on each rotation/refresh) or **absolute** from initial issuance. (Determines whether 90d gives a rolling 90-day-since-last-use window — the goal — or a hard 90-day cap.)
2. Whether a refresh token is issued for a **public** client on the device-code grant given the current scope mappings, and whether adding the managed `offline_access` scope mapping is the correct/necessary way to guarantee it. Confirm the exact managed key (e.g. `goauthentik.io/providers/oauth2/scope-offline_access`).
3. Whether refresh-token **rotation** is on by default (a new refresh token per refresh) and that flow's `persistingSource` (which saves rotated tokens) keeps it alive.

Record the answers as a comment block in the blueprint. If validity is absolute (not sliding), note it in the plan ledger and keep 90d (still a 3× improvement) — do not block.

- [ ] **Step 2: Edit the blueprint**

In `52-app-flow.yaml.j2`, for **`provider-flow-cli`**: change `refresh_token_validity: days=30` → `refresh_token_validity: days=90`; and (if Step 1 confirms it is the right mechanism) add to `property_mappings`:

```yaml
        - !Find [authentik_providers_oauth2.scopemapping, [managed, goauthentik.io/providers/oauth2/scope-offline_access]]
```

For **`provider-flow-web`**: change `refresh_token_validity: days=30` → `days=90` (consistency; web refresh is transparent server-side).

Add a short comment above each `refresh_token_validity` recording the Step-1 finding (sliding vs absolute).

- [ ] **Step 3: Render + validate**

Per `reference_homelab_study_gitops_quirks`, regenerate the rendered manifest (`mise exec -- ...` makejinja flow the repo uses) and confirm the rendered `52-app-flow.yaml` shows `days=90` for both providers + the offline_access mapping on flow-cli. Validate YAML (`yq` parses it).

- [ ] **Step 4: Commit (homelab-study)**

```bash
cd /Users/msoent/SourceCode/serverkraken/homelab-study
git add bootstrap/templates/kubernetes/apps/identity/identity/config/blueprints/52-app-flow.yaml.j2 kubernetes/apps/identity/identity/config/blueprints/52-app-flow.yaml
git commit -m "feat(identity): flow-cli/flow-web refresh_token_validity 30d→90d + explicit offline_access"
```

(Apply = ArgoCD sync on merge to its main; live verification is in the done-gate. Do NOT push/PR without Soenne's go-ahead — surface the commit for review.)

---

## Final verification

- [ ] Full gate with podman env:
  ```
  export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
  export TESTCONTAINERS_RYUK_DISABLED=true
  make ci
  ```
  Expected: golangci-lint 0, coverage ≥ 80%, build OK.
- [ ] **Main-wiring verification** (per `feedback_plan_main_wiring_task`): `cmd/flow-mcp/main.go` builds the manager via `newBootManager`, `newServerH` wires `mgr.onAuth = h.postAuthInit` and registers all 9 tools, every tool routes its backend work through `h.mgr.Do`, and `postAuthInit` calls `resolveProject` + `registerResources`. stdout-hygiene + 9-tool surface via a real MCP `CommandTransport` client (not a raw pipe):
  ```
  go build -o bin/flow-mcp ./cmd/flow-mcp
  ```
  then a throwaway `CommandTransport` client lists 9 tools and gets the degraded "Login required" when logged out (pattern from the Slice 2c dogfood). Remove the throwaway after.
- [ ] **Live reauth done-gate (the whole point), against PROD after the rebuild image is built + the homelab digest bumped + ArgoCD synced:** in a Claude Code session with flow-mcp registered — call a tool while logged out (`flow logout` or an expired token) → "Login required"; run `flow login` in a terminal; **without reconnecting the MCP**, call a tool again → it succeeds, `flow_project_context` resolves, and a created doc appears as a resource. (This is the behavior that failed in the Slice 2c dogfood.)
- [ ] **Durability:** after a fresh login, confirm via Authentik admin (or token introspection) the refresh-token validity is ~90 days, and that an in-process access-token refresh after the 5-minute TTL succeeds (leave the MCP idle >5 min, then call a tool → still works, no re-login).

## Self-review notes (spec coverage)

- Spec §3.1 authManager → Task 2 (full, unit-tested).
- Spec §3.2 per-call recovery + §3.2.1 classification → Task 2 (`Do`, `isAuthError`) + Task 1 (`IsUnauthorized`, `ErrNotLoggedIn`); oauth2.RetrieveError via `errors.As`.
- Spec §3.2 "dynamic client, not a boot-bound seam" → Task 3 Step 1 (`listProjects` via `mgr.client`), Step 5 (`resolveDocRef(c)`), Step 7 (resource read via `mgr.client`).
- Spec §3.3 run-once post-auth init → Task 2 (`onAuth` fires once) + Task 3 (`postAuthInit` = resolveProject + registerResources), wired in `newServerH`.
- Spec §3.4 boot behavior → Task 3 Step 3 (eager warm, non-fatal, lazy recovery).
- Spec §4 Authentik → Task 4 (validity 90d + offline_access + rotation verification via authentik-expert).
- Spec §6 testing → Task 2 unit tests + Task 3 Step 8 recovery loopback + preserved 2a/2b/2c loopbacks.
- Spec §7 done-gate → Final verification (make ci, main-wiring, live reauth, durability).
- Guard/validation errors preserved verbatim (Task 3 Step 6 `errGuard`) — the write guard's user-facing messages are not reworded by `resultErr` and never trigger an auth retry.
- **DEFERRED:** Säule B (embed/Ollama poison-doc storm) is its own spec/plan (next). Deploy (rebuild image build + homelab digest bump + ArgoCD) is the controller's post-merge wiring step, shared with Säule B if landed together.
```
