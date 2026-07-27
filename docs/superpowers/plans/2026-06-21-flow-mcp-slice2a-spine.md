# flow-mcp Slice 2a (MCP Server Spine) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up a registrable `flow-mcp` stdio binary on the official Go MCP SDK that boots, authenticates against the flow server, resolves the current directory to a flow project, and exposes ONE orientation tool (`flow_project_context`) — proving the whole wiring end-to-end before the read/write/project tools fan out (Slices 2b–2d, just-in-time).

**Architecture:** A thin composition root. `internal/clientauth` (extracted from `cmd/flow`) builds the authenticated `apiclient`. `cmd/flow-mcp` wires config → authed client → boot `Whoami` gate → `projectresolve.Resolve(cwd)` → an `mcp.Server` with the orientation tool → `server.Run(stdio)`. Tool handlers are methods on a small struct holding the client + resolved-project state; they short-circuit "Login required" when unauthenticated. A testable `newServer(...)` constructor lets an in-memory-transport loopback test drive the tool without a subprocess or real server.

**Tech Stack:** Go 1.26.x, `github.com/modelcontextprotocol/go-sdk/mcp` (≥ v1.5.0), existing `internal/adapter/apiclient`, `internal/clientconfig`, `internal/adapter/tokenstore`, `internal/projectresolve`, `golang.org/x/oauth2`.

## Global Constraints

- Go module `github.com/serverkraken/flow`.
- **stdout is reserved for the JSON-RPC stream. ALL logging goes to stderr** (`slog` to `os.Stderr`). A stray `fmt.Println`/default-logger write to stdout corrupts the MCP protocol.
- **No interactive auth.** flow-mcp never runs a device flow. It builds the client from the stored token (or `FLOW_TOKEN`). No/expired token → start anyway in a **degraded** state where every tool returns `IsError: true` with text `"Login required: run 'flow login' in a terminal on this device."` — never crash, never block on stdin for auth.
- **`projectID *string` convention** (from Slice 1, for the doc-count call): `nil` → all; `"none"` → unassigned; else a project ID. This tool passes `&proj.ID`.
- **CI gate is `golangci-lint run`** (+ `verify-generate` + `cover` + `build`), NOT plain gofmt. `make fmt` (gofumpt+goimports) is manual/ungated. Verify with `golangci-lint run`, not `gofmt -l` (local gofmt is go1.26.4 and reports pre-existing committed files as false positives).
- **pgstore/other Docker tests** (unrelated to this slice) need: `export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"` and `export TESTCONTAINERS_RYUK_DISABLED=true`. This slice adds no Docker tests, but `make ci`/`cover` runs the whole suite, so set this env when running the full gate.
- Commit message trailers (every commit):
  ```
  Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01GTHf7tSzKearihWVzL6i4n
  ```

---

### Task 1: Extract `internal/clientauth` (shared authenticated apiclient builder)

`cmd/flow/auth.go` already contains `clientFromStore` + the `lazyDeviceSource` and `persistingSource` token sources — exactly what `cmd/flow-mcp` needs, but it lives in `package main` and can't be imported. Move it to a shared package; `cmd/flow` keeps a one-line delegating wrapper so its ~19 callers are untouched.

**Files:**
- Create: `internal/clientauth/clientauth.go`
- Create: `internal/clientauth/clientauth_test.go`
- Modify: `cmd/flow/auth.go` (becomes a thin wrapper)
- Delete: `cmd/flow/auth_test.go` (its tests move to clientauth_test.go; the helpers it defines — `fakeSource`, `memStore` — are used ONLY there, confirmed)

**Interfaces:**
- Consumes: nothing new.
- Produces: `clientauth.Client(ctx context.Context) (*apiclient.Client, error)` — builds an authenticated apiclient from `FLOW_TOKEN` (static) or the stored token (lazily-refreshing, self-persisting); returns an error (`"not logged in — run \`flow login\`"`) when no token is available.

- [ ] **Step 1: Create `internal/clientauth/clientauth.go`**

Move the bodies of `lazyDeviceSource`, `persistingSource`, and `clientFromStore` from `cmd/flow/auth.go` VERBATIM into this new file, changing only: the package clause to `package clientauth`, and renaming the exported entry point `clientFromStore` → `Client`. The file content is exactly (this is the current `cmd/flow/auth.go` body, re-headed and renamed):

```go
// Package clientauth builds an authenticated flow apiclient from the stored
// OIDC token (or FLOW_TOKEN), shared by the flow CLI and flow-mcp.
package clientauth

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"

	"golang.org/x/oauth2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/adapter/oidcdevice"
	"github.com/serverkraken/flow/internal/adapter/tokenstore"
	"github.com/serverkraken/flow/internal/clientconfig"
	"github.com/serverkraken/flow/internal/ports"
)

// lazyDeviceSource hands out the stored access token while it is still valid,
// and only builds the refreshing device-flow source — which performs OIDC
// discovery and therefore needs FLOW_OIDC_ISSUER — once the token has actually
// expired.
type lazyDeviceSource struct {
	ctx context.Context
	cfg clientconfig.Config

	mu        sync.Mutex
	last      ports.Token
	refresher oauth2.TokenSource
}

func (s *lazyDeviceSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refresher == nil {
		cached := &oauth2.Token{
			AccessToken:  s.last.AccessToken,
			RefreshToken: s.last.RefreshToken,
			Expiry:       s.last.Expiry,
		}
		if cached.Valid() {
			return cached, nil // fast path: no issuer, no discovery round-trip
		}
		if s.cfg.OIDCIssuer == "" {
			return nil, fmt.Errorf("access token expired and FLOW_OIDC_ISSUER is not set — run `flow login` (or set FLOW_OIDC_ISSUER)")
		}
		flow, err := oidcdevice.New(s.ctx, s.cfg.OIDCIssuer, s.cfg.CliClientID)
		if err != nil {
			return nil, err
		}
		s.refresher = flow.TokenSource(s.ctx, cached)
	}
	return s.refresher.Token()
}

// persistingSource wraps a refreshing oauth2 source and writes refreshed tokens
// back to the store.
type persistingSource struct {
	base  oauth2.TokenSource
	store ports.TokenStore

	mu   sync.Mutex
	last ports.Token
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
	tok, err := p.base.Token()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	refresh := tok.RefreshToken
	if refresh == "" {
		refresh = p.last.RefreshToken
	}
	if tok.AccessToken != p.last.AccessToken {
		next := ports.Token{AccessToken: tok.AccessToken, RefreshToken: refresh, Expiry: tok.Expiry}
		if err := p.store.Save(next); err != nil {
			return nil, fmt.Errorf("clientauth: persist token: %w", err)
		}
		p.last = next
	}
	return tok, nil
}

// Client builds an authenticated apiclient. FLOW_TOKEN (if set) wins as a
// static, non-refreshing bearer (CI). Otherwise it loads the stored token and
// wraps it in a lazily-refreshing, self-persisting source.
func Client(ctx context.Context) (*apiclient.Client, error) {
	cfg := clientconfig.Load(os.Getenv)
	if t := os.Getenv("FLOW_TOKEN"); t != "" {
		if cfg.InsecureTLS {
			return apiclient.NewInsecure(cfg.ServerURL, t), nil
		}
		return apiclient.New(cfg.ServerURL, t), nil
	}
	store := tokenstore.Open()
	loaded, ok, err := store.Load()
	if err != nil {
		return nil, err
	}
	if !ok || loaded.AccessToken == "" {
		return nil, fmt.Errorf("not logged in — run `flow login`")
	}
	base := &lazyDeviceSource{ctx: ctx, cfg: cfg, last: loaded}
	src := &persistingSource{base: base, store: store, last: loaded}
	transport := http.DefaultTransport
	if cfg.InsecureTLS {
		transport = apiclient.InsecureBase()
	}
	rt := &oauth2.Transport{Source: src, Base: transport}
	return apiclient.NewTransport(cfg.ServerURL, rt), nil
}
```

- [ ] **Step 2: Move the tests to `internal/clientauth/clientauth_test.go`**

Move the FULL content of `cmd/flow/auth_test.go` into this new file VERBATIM, changing only the package clause from `package main` to `package clientauth`. (It tests `persistingSource`/`lazyDeviceSource` and defines `fakeSource`/`memStore`; all reference the moved types, so white-box `package clientauth` is correct. Five tests: `TestPersistingSourceSavesOnChange`, `TestPersistingSourcePreservesRefreshWhenEmpty`, `TestLazySourceUsesValidTokenWithoutIssuer`, `TestLazySourceErrorsWhenExpiredWithoutIssuer`, `TestLazySourceRefreshesWhenExpired`.)

- [ ] **Step 3: Replace `cmd/flow/auth.go` with a thin wrapper**

Replace the ENTIRE file `cmd/flow/auth.go` with:

```go
package main

import (
	"context"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientauth"
)

// clientFromStore delegates to the shared clientauth.Client so the CLI and
// flow-mcp build identical authenticated clients. Kept as a local alias so the
// CLI's many call sites stay unchanged.
func clientFromStore(ctx context.Context) (*apiclient.Client, error) {
	return clientauth.Client(ctx)
}
```

- [ ] **Step 4: Delete the old test file**

Run: `git rm cmd/flow/auth_test.go`

- [ ] **Step 5: Build, test, lint**

Run:
```
go build ./...
go test ./internal/clientauth/ ./cmd/flow/
golangci-lint run ./internal/clientauth/... ./cmd/flow/...
```
Expected: build OK; clientauth tests PASS (the 5 moved tests); cmd/flow tests PASS (callers unchanged); golangci-lint 0 issues. If `go build` flags an unused import in `cmd/flow/auth.go`, you left one behind — the wrapper needs only `context`, `apiclient`, `clientauth`.

- [ ] **Step 6: Commit**

```bash
git add internal/clientauth/ cmd/flow/auth.go
git rm cmd/flow/auth_test.go
git commit -m "refactor(clientauth): extract shared authenticated-apiclient builder from cmd/flow" -m "Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01GTHf7tSzKearihWVzL6i4n"
```

---

### Task 2: `cmd/flow-mcp` spine — go-sdk server + auth gate + resolve + `flow_project_context`

**Files:**
- Modify: `go.mod` / `go.sum` (add the MCP SDK)
- Create: `cmd/flow-mcp/main.go` (wiring only)
- Create: `cmd/flow-mcp/auth.go` (boot client + Whoami gate)
- Create: `cmd/flow-mcp/resolve.go` (cwd → project)
- Create: `cmd/flow-mcp/server.go` (`newServer` + result helpers + `handlers` struct)
- Create: `cmd/flow-mcp/tools_project.go` (`flow_project_context` handler)
- Create: `cmd/flow-mcp/loopback_test.go` (in-memory transport integration test)

**Interfaces:**
- Consumes: `clientauth.Client(ctx)` (Task 1); `apiclient.Client.Whoami(ctx) (domain.User, error)`; `apiclient.Client.ListDocumentsScoped(ctx, projectID *string, tags ...string) ([]domain.Document, error)` (Slice 1); `projectresolve.Resolve(ctx, *apiclient.Client, getenv func(string) string, cwd string) (domain.Project, bool, error)` (V0); the MCP SDK (`mcp.NewServer`, `mcp.AddTool`, `mcp.Tool`, `mcp.CallToolResult`, `mcp.TextContent`, `mcp.Content`, `mcp.StdioTransport`, and for tests `mcp.NewInMemoryTransports`, `mcp.NewClient`, `*mcp.ClientSession.{ListTools,CallTool}`, `mcp.CallToolParams`).
- Produces: a `flow-mcp` binary; `newServer(client *apiclient.Client, authed bool, proj domain.Project, matched bool) *mcp.Server` (consumed by 2b–2d to add more tools/resources); the `handlers` struct + `textResult`/`errorResult` helpers.

- [ ] **Step 1: Add the MCP SDK dependency**

Run:
```
go get github.com/modelcontextprotocol/go-sdk@latest
go mod tidy
```
Expected: go.mod gains `github.com/modelcontextprotocol/go-sdk vX.Y.Z` (≥ v1.5.0). Verify: `go list -m github.com/modelcontextprotocol/go-sdk` prints a version ≥ v1.5.0. (If `@latest` resolves below v1.5.0, pin `@v1.5.0`.)

- [ ] **Step 2: Write the failing loopback test**

Create `cmd/flow-mcp/loopback_test.go`. It stands up an httptest flow backend, builds an apiclient against it, constructs the MCP server via `newServer`, connects a client over in-memory transports, and drives the tool. (It fails to compile until `newServer` + the tool exist — that is the RED state.)

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// fakeBackend serves the minimal flow REST endpoints the spine touches.
func fakeBackend(t *testing.T, docs int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("/api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("projectId") != "p1" {
			http.Error(w, "unexpected projectId", http.StatusBadRequest)
			return
		}
		out := make([]domain.Document, docs)
		for i := range out {
			out[i] = domain.Document{ID: "d", OwnerID: "u1", Type: domain.DocFree, Path: "p", Title: "t"}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	return httptest.NewServer(mux)
}

func connect(t *testing.T, srv *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, st) }()
	cl := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	sess, err := cl.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

func TestLoopback_ProjectContext_Authed(t *testing.T) {
	ctx := context.Background()
	be := fakeBackend(t, 2)
	defer be.Close()
	client := apiclient.New(be.URL, "tok")
	proj := domain.Project{ID: "p1", Name: "Alpha", Slug: "alpha"}

	sess := connect(t, newServer(client, true, proj, true))

	tools, err := sess.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if !hasTool(tools.Tools, "flow_project_context") {
		t.Fatalf("flow_project_context not advertised; got %v", toolNames(tools.Tools))
	}

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "flow_project_context", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", text(res))
	}
	got := text(res)
	if !strings.Contains(got, "Alpha") || !strings.Contains(got, "2") {
		t.Fatalf("project context = %q, want it to mention Alpha and 2 docs", got)
	}
}

func TestLoopback_ProjectContext_DegradedRequiresLogin(t *testing.T) {
	ctx := context.Background()
	sess := connect(t, newServer(nil, false, domain.Project{}, false))
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "flow_project_context", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError || !strings.Contains(text(res), "Login required") {
		t.Fatalf("degraded call = (IsError=%v, %q), want IsError + 'Login required'", res.IsError, text(res))
	}
}

func hasTool(ts []*mcp.Tool, name string) bool {
	for _, t := range ts {
		if t.Name == name {
			return true
		}
	}
	return false
}

func toolNames(ts []*mcp.Tool) []string {
	var n []string
	for _, t := range ts {
		n = append(n, t.Name)
	}
	return n
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
```

- [ ] **Step 3: Run the test to verify it fails (RED)**

Run: `go test ./cmd/flow-mcp/ -run Loopback`
Expected: FAIL — compile error `undefined: newServer` (and the tool). This confirms the test drives the code you're about to write.

- [ ] **Step 4: Write `cmd/flow-mcp/server.go`**

```go
package main

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

const serverName = "flow-mcp"
const serverVersion = "0.1.0"

// handlers carries the dependencies every tool needs: the authenticated client,
// whether auth succeeded at boot, and the cwd-resolved project.
type handlers struct {
	client  *apiclient.Client
	authed  bool
	proj    domain.Project
	matched bool
}

// newServer builds the MCP server and registers the spine's tools. Kept
// dependency-injected (no global state, no I/O) so loopback tests can drive it.
func newServer(client *apiclient.Client, authed bool, proj domain.Project, matched bool) *mcp.Server {
	h := &handlers{client: client, authed: authed, proj: proj, matched: matched}
	s := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_project_context",
		Description: "Report which flow project the current working directory resolves to, and how many Kompendium documents are in scope. Call this first to orient.",
	}, h.projectContext)
	return s
}

// textResult wraps a plain-text success result.
func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// errorResult wraps an actionable error result (IsError=true).
func errorResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// loginRequired is the standard degraded-mode short-circuit.
func (h *handlers) loginRequired() *mcp.CallToolResult {
	return errorResult("Login required: run 'flow login' in a terminal on this device.")
}
```

- [ ] **Step 5: Write `cmd/flow-mcp/tools_project.go`**

```go
package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// projectContextIn has no parameters.
type projectContextIn struct{}

// projectContext reports the resolved project and its in-scope document count.
// Out is `any` (no output schema) — the result is concise plain text per the
// design spec.
func (h *handlers) projectContext(ctx context.Context, _ *mcp.CallToolRequest, _ projectContextIn) (*mcp.CallToolResult, any, error) {
	if !h.authed {
		return h.loginRequired(), nil, nil
	}
	if !h.matched {
		return textResult("No flow project is bound to this directory. Set FLOW_PROJECT, or bind it (flow_bind_project, coming in a later version)."), nil, nil
	}
	docs, err := h.client.ListDocumentsScoped(ctx, &h.proj.ID)
	if err != nil {
		return errorResult(fmt.Sprintf("flow server error: %v", err)), nil, nil
	}
	msg := fmt.Sprintf("Project: %s (%s) — %d document(s) in scope. Resolved for this working directory.", h.proj.Name, h.proj.Slug, len(docs))
	return textResult(msg), nil, nil
}
```

- [ ] **Step 6: Run the loopback test to verify it passes (GREEN)**

Run: `go test ./cmd/flow-mcp/ -run Loopback -v`
Expected: PASS — both `TestLoopback_ProjectContext_Authed` and `TestLoopback_ProjectContext_DegradedRequiresLogin`.

- [ ] **Step 7: Write `cmd/flow-mcp/auth.go`**

```go
package main

import (
	"context"
	"log/slog"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientauth"
)

// bootClient builds the authenticated client and verifies the token against the
// server. On any failure it returns authed=false so the server still starts and
// every tool surfaces a clean "run flow login" message instead of crashing.
func bootClient(ctx context.Context, log *slog.Logger) (*apiclient.Client, bool) {
	client, err := clientauth.Client(ctx)
	if err != nil {
		log.Warn("not authenticated; tools will require login", "err", err)
		return nil, false
	}
	if _, err := client.Whoami(ctx); err != nil {
		log.Warn("token rejected by server; tools will require login", "err", err)
		return client, false
	}
	return client, true
}
```

- [ ] **Step 8: Write `cmd/flow-mcp/resolve.go`**

```go
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/projectresolve"
)

// resolveProject answers "which flow project is this directory?" via the V0
// resolution chain (FLOW_PROJECT override → git remote → per-device path).
// Any failure degrades to "no project" (matched=false) rather than erroring.
func resolveProject(ctx context.Context, client *apiclient.Client, log *slog.Logger) (domain.Project, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		log.Warn("cannot determine working directory; no project scope", "err", err)
		return domain.Project{}, false
	}
	proj, matched, err := projectresolve.Resolve(ctx, client, os.Getenv, cwd)
	if err != nil {
		log.Warn("project resolution failed; no project scope", "err", err)
		return domain.Project{}, false
	}
	return proj, matched
}
```

- [ ] **Step 9: Write `cmd/flow-mcp/main.go`**

```go
// Command flow-mcp is a stdio Model Context Protocol server exposing the flow
// Kompendium to AI clients. stdout carries the JSON-RPC stream; all logs go to
// stderr.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/domain"
)

func main() {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	client, authed := bootClient(ctx, log)
	var proj domain.Project
	var matched bool
	if authed {
		proj, matched = resolveProject(ctx, client, log)
	}

	srv := newServer(client, authed, proj, matched)
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Error("flow-mcp exited", "err", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 10: Build, full gate, and a manual stdio sanity check**

Run:
```
go build -o bin/flow-mcp ./cmd/flow-mcp
go test ./cmd/flow-mcp/ -v
golangci-lint run ./cmd/flow-mcp/... ./internal/clientauth/...
```
Expected: binary builds; loopback tests PASS; golangci-lint 0 issues.

Then a stdout-hygiene smoke (no flow login needed — degraded mode is fine; this proves stdout carries only JSON-RPC and the initialize handshake works):
```
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' | ./bin/flow-mcp 2>/dev/null
```
Expected: a single JSON-RPC line on stdout with `"result"` containing `serverInfo.name":"flow-mcp"` and a `protocolVersion` — and NOTHING else on stdout (logs went to stderr, which `2>/dev/null` dropped). If you see any non-JSON text on stdout, a log or print leaked to stdout — fix it before committing.

- [ ] **Step 11: Commit**

```bash
git add go.mod go.sum cmd/flow-mcp/
git commit -m "feat(flow-mcp): stdio MCP server spine (go-sdk) with flow_project_context" -m "Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01GTHf7tSzKearihWVzL6i4n"
```

---

## Final verification

- [ ] Run the full gate with the podman env (so unrelated pgstore Docker tests in the suite run, not skip):
  ```
  export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
  export TESTCONTAINERS_RYUK_DISABLED=true
  make ci
  ```
  Expected: golangci-lint 0, coverage ≥ 80%, build OK.
- [ ] Confirm `cmd/flow` still works end-to-end after the clientauth extraction: `go test ./cmd/flow/` green (its ~19 `clientFromStore` callers are unchanged; the wrapper delegates).

## Self-review notes (spec coverage for 2a)

- Spec §4.3 (shared clientauth) → Task 1.
- Spec §4.1/§4.2/§4.4 (generic adapter, thin-over-apiclient, stdio + stderr-logging + boot Whoami gate + degraded "run flow login") → Task 2.
- Spec §4.5 + §7.2 `flow_project_context` (the orientation entry point; auto-scope via projectresolve) → Task 2. NOTE: the spec's `flow_project_context` also reports the resolution *tier* (override/remote/path/none); `projectresolve.Resolve` returns only `(Project, matched, err)`, not the tier, so 2a reports resolved/not-resolved + project + doc count. Reporting the exact tier is a deferred refinement (would need a small `projectresolve` change) — out of scope for the spine; revisit in 2d with the bind tool if wanted.
- DEFERRED to later sub-slices (each its own just-in-time plan): 2b read tools (search_docs/list_docs/get_doc/list_tags/backlinks) + resources; 2c write tools (create/update/delete) + write-guard; 2d project tools (bind/list_projects) + runbook + the `.mcp.json` live registration done-gate. CARRY-FORWARD from Slice 1: the MCP `project` arg's `"global"` sentinel must be mapped to `nil` before calling apiclient (backend knows only nil/"none"/id).
