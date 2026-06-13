# flow Phase 1 — M1: Server Skeleton + OIDC Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deployable `flow-server` skeleton with end-to-end OIDC authentication. Browser users can log in via Authorization-Code-Flow; CLI users via Device-Authorization-Flow with tokens persisted in the OS-Keychain. No data operations yet — only auth machinery.

**Architecture:** New `cmd/flow-server` binary using Chi router. Server-side OIDC verifier with JWKS-cache + allowlist middleware. Browser flow with HttpOnly session cookies. New CLI subcommands `flow login`, `flow logout`, `flow whoami`. Integration-tested against `dexidp/dex` as a lightweight in-test OIDC IdP via testcontainers; production hits Authentik unchanged. Hexagonal: ports `AuthProvider`, `TokenStore` define the interfaces; adapters `oidcserver/`, `oidcclient/`, `keyringadapter/` are the implementations.

**Tech Stack:** Go 1.25, Chi (`github.com/go-chi/chi/v5`), `github.com/coreos/go-oidc/v3`, `golang.org/x/oauth2`, `github.com/zalando/go-keyring`, `github.com/gorilla/securecookie` for session encoding, `github.com/testcontainers/testcontainers-go` + `dexidp/dex:v2.41.1` for integration tests.

**Reference spec:** `docs/superpowers/specs/2026-06-02-flow-client-server-phase1-design.md` (commit `44b6a40`).

---

## File Structure

**New files:**

```
cmd/flow-server/main.go                          ← entry point, wiring
internal/ports/auth.go                           ← AuthProvider, TokenStore, SessionStore interfaces
internal/adapter/httpserver/server.go            ← Chi router assembly
internal/adapter/httpserver/healthz.go           ← /healthz, /readyz
internal/adapter/httpserver/auth_browser.go      ← /login, /auth/callback, /logout (browser)
internal/adapter/httpserver/me.go                ← /api/v1/me
internal/adapter/httpserver/middleware.go        ← request logger, auth middleware
internal/adapter/httpserver/config.go            ← env-var loading
internal/adapter/httpserver/session.go           ← cookie encode/decode
internal/adapter/oidcserver/jwks.go              ← JWKS cache (provider-backed)
internal/adapter/oidcserver/verifier.go          ← JWT verify
internal/adapter/oidcserver/allowlist.go         ← sub-/email-allowlist
internal/adapter/oidcclient/deviceflow.go        ← RFC 8628 device-authorization
internal/adapter/oidcclient/refresh.go           ← refresh-token rotation
internal/adapter/oidcclient/tokens.go            ← token-store wrapper (uses keyring)
internal/adapter/keyringadapter/keyring.go       ← zalando/go-keyring wrapper
internal/adapter/keyringadapter/fake.go          ← in-memory fake (for tests)
internal/testutil/oidctest/dex.go                ← testcontainers helper to boot dex
internal/testutil/oidctest/dex-config.yaml       ← embedded dex config

deploy/podman/docker-compose.yml                 ← dev: flow-server + dex
deploy/podman/Dockerfile.server                  ← multi-stage build
deploy/podman/.env.example                       ← env-var template
deploy/podman/dex-config.yaml                    ← dev dex config
```

**Modified files:**

```
cmd/flow/main.go                                 ← register new subcommands
cmd/flow/login.go                                ← NEW subcommand file (alongside existing pattern)
cmd/flow/logout.go                               ← NEW subcommand file
cmd/flow/whoami.go                               ← NEW subcommand file
go.mod                                           ← new deps
go.sum                                           ← (auto)
Makefile                                         ← targets build-server, test-server, dex-up, dex-down
CLAUDE-activeContext.md                          ← M1 status update
```

**Tests:** alongside each adapter package (`*_test.go` files); integration tests under `internal/adapter/httpserver/integration_test.go`.

---

## Conventions

- All packages follow the existing hexagonal pattern (`internal/ports/` defines interfaces, `internal/adapter/` implements).
- All new code uses `slog` for logging (handler injected via context).
- Test naming: `TestUnit_…` for unit, `TestIntegration_…` for tests that boot containers; integration tests use `//go:build integration` build tag.
- Commit-message style follows existing repo convention (Conventional Commits): `feat(server): …`, `test(oidcserver): …`, `chore(deps): …`. Include `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` line.
- After every task with passing tests, run `make ci` once to make sure the wider build still passes before committing.

---

## Tasks

### Task 1: cmd/flow-server skeleton with /healthz

**Files:**
- Create: `cmd/flow-server/main.go`
- Create: `internal/adapter/httpserver/server.go`
- Create: `internal/adapter/httpserver/healthz.go`
- Create: `internal/adapter/httpserver/healthz_test.go`

- [ ] **Step 1: Add Chi dependency**

```bash
go get github.com/go-chi/chi/v5@v5.2.1
```

- [ ] **Step 2: Write failing test for /healthz**

`internal/adapter/httpserver/healthz_test.go`:

```go
package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnit_Healthz_Returns200OK(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	NewHealthzHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != "ok\n" {
		t.Fatalf("body = %q, want %q", rr.Body.String(), "ok\n")
	}
}
```

- [ ] **Step 3: Run test, expect FAIL**

Run: `go test ./internal/adapter/httpserver/ -run TestUnit_Healthz`
Expected: build error — `NewHealthzHandler` undefined.

- [ ] **Step 4: Implement healthz**

`internal/adapter/httpserver/healthz.go`:

```go
package httpserver

import (
	"net/http"
)

// NewHealthzHandler returns a liveness probe. Always 200 while the process is
// up; readiness is separate (see /readyz). Used by Kubernetes / docker-compose
// healthchecks.
func NewHealthzHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
}
```

- [ ] **Step 5: Run test, expect PASS**

Run: `go test ./internal/adapter/httpserver/ -run TestUnit_Healthz -v`
Expected: PASS.

- [ ] **Step 6: Implement server assembly**

`internal/adapter/httpserver/server.go`:

```go
package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Server wires the HTTP handlers into a Chi router. Construction is
// deliberately small — every endpoint group lives in its own file so the
// router stays a wiring index, never a god-object.
type Server struct {
	router chi.Router
}

func New() *Server {
	r := chi.NewRouter()
	r.Handle("/healthz", NewHealthzHandler())
	return &Server{router: r}
}

func (s *Server) Handler() http.Handler { return s.router }
```

- [ ] **Step 7: Add cmd/flow-server entry point**

`cmd/flow-server/main.go`:

```go
package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	srv := httpserver.New()

	addr := os.Getenv("FLOW_SERVER_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	logger.Info("flow-server starting", slog.String("addr", addr))
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server crashed", slog.Any("err", err))
		os.Exit(1)
	}
}
```

- [ ] **Step 8: Verify build + smoke test**

```bash
go build ./cmd/flow-server
./flow-server &
SERVER_PID=$!
sleep 0.3
curl -fsS http://localhost:8080/healthz
kill $SERVER_PID
```
Expected: `ok` printed, no errors.

- [ ] **Step 9: Commit**

```bash
git add cmd/flow-server internal/adapter/httpserver go.mod go.sum
git commit -m "$(cat <<'EOF'
feat(server): cmd/flow-server skeleton with /healthz

Bootstrap der neuen flow-server-Binary. Chi-Router, Healthz-Endpoint,
JSON-slog. Erste Stufe von Phase-1-M1.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: /readyz endpoint with dependency check stub

**Files:**
- Modify: `internal/adapter/httpserver/healthz.go`
- Create: `internal/adapter/httpserver/readyz_test.go`
- Modify: `internal/adapter/httpserver/server.go`

- [ ] **Step 1: Write failing test**

`internal/adapter/httpserver/readyz_test.go`:

```go
package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnit_Readyz_AllChecksOK_Returns200(t *testing.T) {
	t.Parallel()
	h := NewReadyzHandler(func() error { return nil })

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

func TestUnit_Readyz_CheckFails_Returns503(t *testing.T) {
	t.Parallel()
	h := NewReadyzHandler(func() error { return errors.New("db down") })

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}
```

- [ ] **Step 2: Run test, expect FAIL**

Run: `go test ./internal/adapter/httpserver/ -run TestUnit_Readyz`
Expected: build error.

- [ ] **Step 3: Implement**

Append to `internal/adapter/httpserver/healthz.go`:

```go
// ReadinessCheck returns nil when the dependency is ready (e.g. DB ping
// succeeds, JWKS cache primed). Anything else means /readyz reports 503.
type ReadinessCheck func() error

func NewReadyzHandler(check ReadinessCheck) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := check(); err != nil {
			http.Error(w, "not ready: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
}
```

Update `server.go` to register and accept a check function:

```go
func New(readyCheck ReadinessCheck) *Server {
	r := chi.NewRouter()
	r.Handle("/healthz", NewHealthzHandler())
	r.Handle("/readyz", NewReadyzHandler(readyCheck))
	return &Server{router: r}
}
```

Update `cmd/flow-server/main.go` to pass an initially-OK check:

```go
srv := httpserver.New(func() error { return nil })
```

- [ ] **Step 4: Run tests, expect PASS**

Run: `go test ./internal/adapter/httpserver/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/httpserver cmd/flow-server
git commit -m "feat(server): add /readyz with injectable readiness check

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Config loading from environment

**Files:**
- Create: `internal/adapter/httpserver/config.go`
- Create: `internal/adapter/httpserver/config_test.go`
- Modify: `cmd/flow-server/main.go`

- [ ] **Step 1: Write failing test**

`internal/adapter/httpserver/config_test.go`:

```go
package httpserver

import (
	"testing"
)

func TestUnit_LoadConfig_DefaultsAndOverrides(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		env  map[string]string
		want Config
		err  bool
	}{
		{
			name: "defaults",
			env:  nil,
			want: Config{
				Addr:           ":8080",
				BaseURL:        "http://localhost:8080",
				OIDCIssuer:     "",
				OIDCClientID:   "",
				CookieHashKey:  "",
				CookieBlockKey: "",
				AllowedSubs:    nil,
			},
			err: false,
		},
		{
			name: "all set",
			env: map[string]string{
				"FLOW_SERVER_ADDR":             ":9000",
				"FLOW_SERVER_BASE_URL":         "https://flow.example.com",
				"FLOW_OIDC_ISSUER":             "https://auth.example.com/realms/flow",
				"FLOW_OIDC_CLIENT_ID":          "flow-server",
				"FLOW_OIDC_CLIENT_SECRET":      "secret",
				"FLOW_COOKIE_HASH_KEY":         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"FLOW_COOKIE_BLOCK_KEY":        "fedcba9876543210fedcba9876543210",
				"FLOW_ALLOWED_SUBS":            "user-a,user-b",
			},
			want: Config{
				Addr:           ":9000",
				BaseURL:        "https://flow.example.com",
				OIDCIssuer:     "https://auth.example.com/realms/flow",
				OIDCClientID:   "flow-server",
				OIDCClientSecret: "secret",
				CookieHashKey:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				CookieBlockKey: "fedcba9876543210fedcba9876543210",
				AllowedSubs:    []string{"user-a", "user-b"},
			},
			err: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			got, err := LoadConfig()
			if c.err && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !c.err && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
}
```

Note: Use a comparable form. Since `AllowedSubs` is a slice, change the test to deep-compare:

```go
// replace direct '!=' on Config with field-by-field compare:
if got.Addr != c.want.Addr || got.BaseURL != c.want.BaseURL ||
	got.OIDCIssuer != c.want.OIDCIssuer || got.OIDCClientID != c.want.OIDCClientID ||
	got.OIDCClientSecret != c.want.OIDCClientSecret ||
	got.CookieHashKey != c.want.CookieHashKey || got.CookieBlockKey != c.want.CookieBlockKey ||
	!slicesEq(got.AllowedSubs, c.want.AllowedSubs) {
	t.Errorf("got %+v, want %+v", got, c.want)
}
```

Add helper at file bottom:

```go
func slicesEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: Run test, expect FAIL**

Run: `go test ./internal/adapter/httpserver/ -run TestUnit_LoadConfig`
Expected: build error — `Config`/`LoadConfig` undefined.

- [ ] **Step 3: Implement**

`internal/adapter/httpserver/config.go`:

```go
package httpserver

import (
	"os"
	"strings"
)

// Config holds all flow-server configuration read from environment variables.
// One struct, one source of truth — flags would require a parser and we don't
// need them yet. All env-vars use the FLOW_ prefix.
type Config struct {
	Addr             string   // FLOW_SERVER_ADDR (default :8080)
	BaseURL          string   // FLOW_SERVER_BASE_URL (default http://localhost:8080)
	OIDCIssuer       string   // FLOW_OIDC_ISSUER (Authentik realm URL)
	OIDCClientID     string   // FLOW_OIDC_CLIENT_ID
	OIDCClientSecret string   // FLOW_OIDC_CLIENT_SECRET
	CookieHashKey    string   // FLOW_COOKIE_HASH_KEY (hex, 64 chars = 32 bytes)
	CookieBlockKey   string   // FLOW_COOKIE_BLOCK_KEY (hex, 32 or 64 chars)
	AllowedSubs      []string // FLOW_ALLOWED_SUBS (comma-separated OIDC 'sub' values)
}

func LoadConfig() (Config, error) {
	c := Config{
		Addr:             envOr("FLOW_SERVER_ADDR", ":8080"),
		BaseURL:          envOr("FLOW_SERVER_BASE_URL", "http://localhost:8080"),
		OIDCIssuer:       os.Getenv("FLOW_OIDC_ISSUER"),
		OIDCClientID:     os.Getenv("FLOW_OIDC_CLIENT_ID"),
		OIDCClientSecret: os.Getenv("FLOW_OIDC_CLIENT_SECRET"),
		CookieHashKey:    os.Getenv("FLOW_COOKIE_HASH_KEY"),
		CookieBlockKey:   os.Getenv("FLOW_COOKIE_BLOCK_KEY"),
		AllowedSubs:      splitCSV(os.Getenv("FLOW_ALLOWED_SUBS")),
	}
	return c, nil
}

func envOr(k, def string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
```

- [ ] **Step 4: Wire Config into main.go**

`cmd/flow-server/main.go`:

```go
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := httpserver.LoadConfig()
	if err != nil {
		logger.Error("config load failed", slog.Any("err", err))
		os.Exit(1)
	}

	srv := httpserver.New(func() error { return nil })

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	logger.Info("flow-server starting", slog.String("addr", cfg.Addr))
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server crashed", slog.Any("err", err))
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Run tests + commit**

```bash
go test ./internal/adapter/httpserver/
git add internal/adapter/httpserver cmd/flow-server
git commit -m "feat(server): config loading from FLOW_* env vars

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Ports for AuthProvider + TokenStore + SessionStore

**Files:**
- Create: `internal/ports/auth.go`

- [ ] **Step 1: Add deps**

```bash
go get github.com/coreos/go-oidc/v3
go get golang.org/x/oauth2
```

- [ ] **Step 2: Write the port definitions**

`internal/ports/auth.go`:

```go
package ports

import (
	"context"
	"time"
)

// Identity is the resolved OIDC identity after a successful token verification.
// Fields mirror the standard OIDC ID-Token claims that flow cares about.
type Identity struct {
	Sub           string
	Email         string
	EmailVerified bool
	Name          string
	IssuedAt      time.Time
	ExpiresAt     time.Time
}

// AuthProvider verifies an OIDC ID-Token (or access token, when audience-bound)
// and returns the corresponding Identity. Implementations cache JWKS keys.
type AuthProvider interface {
	Verify(ctx context.Context, rawToken string) (Identity, error)
}

// AccessChecker decides whether a verified Identity is allowed to use the
// server. Phase 1: allowlist of OIDC 'sub' values. Phase 2 will swap to a
// database-backed User table.
type AccessChecker interface {
	Allow(id Identity) bool
}

// Tokens holds the OAuth2 token bundle a client persists locally after login.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	Expiry       time.Time
}

// TokenStore persists Tokens locally (typically in the OS-Keychain) so a CLI
// user logs in once and subsequent commands reuse the token. SlotName lets a
// single physical store (Keychain) hold tokens for different flow instances
// (e.g. dev vs prod servers).
type TokenStore interface {
	Get(slotName string) (Tokens, error)
	Put(slotName string, t Tokens) error
	Delete(slotName string) error
}

// ErrTokenNotFound is returned by TokenStore.Get when no tokens exist for the
// slot. Callers use this to distinguish "not logged in" from real errors.
var ErrTokenNotFound = errSentinel("flow: token not found")

// SessionStore persists browser-session state (post-OIDC-login). Phase 1 uses
// signed/encrypted cookies via gorilla/securecookie so we have no server-side
// store yet; the interface lets us swap to Redis or DB later.
type SessionStore interface {
	Encode(name string, value any) (string, error)
	Decode(name, raw string, out any) error
}

type errSentinel string

func (e errSentinel) Error() string { return string(e) }
```

- [ ] **Step 3: Verify build**

```bash
go build ./internal/ports/
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/ports/auth.go go.mod go.sum
git commit -m "feat(ports): add AuthProvider, AccessChecker, TokenStore, SessionStore

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: keyringadapter — OS-Keychain wrapper

**Files:**
- Create: `internal/adapter/keyringadapter/keyring.go`
- Create: `internal/adapter/keyringadapter/keyring_test.go`
- Create: `internal/adapter/keyringadapter/fake.go`

- [ ] **Step 1: Add dep**

```bash
go get github.com/zalando/go-keyring
```

- [ ] **Step 2: Write failing test**

`internal/adapter/keyringadapter/keyring_test.go`:

```go
package keyringadapter

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

func TestUnit_Fake_PutGetDelete_Roundtrip(t *testing.T) {
	t.Parallel()
	ks := NewFake()

	want := ports.Tokens{
		AccessToken:  "access",
		RefreshToken: "refresh",
		IDToken:      "id",
		Expiry:       time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC),
	}
	if err := ks.Put("slot-a", want); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := ks.Get("slot-a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if err := ks.Delete("slot-a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := ks.Get("slot-a"); err != ports.ErrTokenNotFound {
		t.Fatalf("Get after Delete: err = %v, want ErrTokenNotFound", err)
	}
}

func TestUnit_Fake_Get_Missing_ReturnsErrTokenNotFound(t *testing.T) {
	t.Parallel()
	ks := NewFake()
	if _, err := ks.Get("nope"); err != ports.ErrTokenNotFound {
		t.Fatalf("err = %v, want ErrTokenNotFound", err)
	}
}
```

- [ ] **Step 3: Run test, expect FAIL**

Expected: build errors.

- [ ] **Step 4: Implement fake**

`internal/adapter/keyringadapter/fake.go`:

```go
package keyringadapter

import (
	"sync"

	"github.com/serverkraken/flow/internal/ports"
)

// Fake is an in-memory TokenStore for tests. Goroutine-safe.
type Fake struct {
	mu   sync.Mutex
	data map[string]ports.Tokens
}

func NewFake() *Fake { return &Fake{data: make(map[string]ports.Tokens)} }

func (f *Fake) Get(slot string) (ports.Tokens, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.data[slot]
	if !ok {
		return ports.Tokens{}, ports.ErrTokenNotFound
	}
	return t, nil
}

func (f *Fake) Put(slot string, t ports.Tokens) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[slot] = t
	return nil
}

func (f *Fake) Delete(slot string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, slot)
	return nil
}

// Compile-time assertion.
var _ ports.TokenStore = (*Fake)(nil)
```

- [ ] **Step 5: Run fake tests, expect PASS**

Run: `go test ./internal/adapter/keyringadapter/ -run TestUnit_Fake`
Expected: PASS.

- [ ] **Step 6: Implement real keyring adapter**

`internal/adapter/keyringadapter/keyring.go`:

```go
package keyringadapter

import (
	"encoding/json"
	"errors"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/zalando/go-keyring"
)

// service is the keyring "service name" under which all flow tokens are
// stored. SlotName from TokenStore becomes the "account" part. This way the
// macOS Keychain (and equivalents) shows tokens grouped under one entry
// labelled "flow".
const service = "flow"

// Keyring stores ports.Tokens in the OS keychain via zalando/go-keyring.
// Each slot is a separate keychain entry; values are JSON-encoded.
type Keyring struct{}

func New() *Keyring { return &Keyring{} }

func (Keyring) Get(slot string) (ports.Tokens, error) {
	raw, err := keyring.Get(service, slot)
	if errors.Is(err, keyring.ErrNotFound) {
		return ports.Tokens{}, ports.ErrTokenNotFound
	}
	if err != nil {
		return ports.Tokens{}, err
	}
	var t ports.Tokens
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return ports.Tokens{}, err
	}
	return t, nil
}

func (Keyring) Put(slot string, t ports.Tokens) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return keyring.Set(service, slot, string(b))
}

func (Keyring) Delete(slot string) error {
	err := keyring.Delete(service, slot)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// Compile-time assertion.
var _ ports.TokenStore = (*Keyring)(nil)
```

- [ ] **Step 7: Verify build (real keyring not testable in CI without OS support)**

```bash
go build ./internal/adapter/keyringadapter/
```
Expected: no errors. The fake covers test coverage; real Keyring is exercised via integration test in Task 25.

- [ ] **Step 8: Commit**

```bash
git add internal/adapter/keyringadapter go.mod go.sum
git commit -m "feat(adapter): OS-Keychain TokenStore via zalando/go-keyring + in-mem Fake

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: testutil/oidctest — embedded dex helper

**Files:**
- Create: `internal/testutil/oidctest/dex.go`
- Create: `internal/testutil/oidctest/dex-config.yaml`

This boots `dexidp/dex` in a testcontainer with a pre-configured static client and one static user. Used by every later integration test.

- [ ] **Step 1: Add deps**

```bash
go get github.com/testcontainers/testcontainers-go@v0.32.0
```

- [ ] **Step 2: Write the dex config**

`internal/testutil/oidctest/dex-config.yaml`:

```yaml
issuer: http://DEX_ISSUER_PLACEHOLDER
storage:
  type: memory
web:
  http: 0.0.0.0:5556
oauth2:
  skipApprovalScreen: true
  responseTypes: ["code", "token", "id_token"]
staticClients:
  - id: flow-server
    redirectURIs:
      - 'http://localhost:8080/auth/callback'
    name: 'Flow Server'
    secret: flow-server-secret
  - id: flow-cli
    public: true
    name: 'Flow CLI'
enablePasswordDB: true
staticPasswords:
  - email: "alice@example.com"
    # bcrypt hash of "password"
    hash: "$2a$10$2b2cu2a5yhHRk6PEpvb8d.O4WXuOdQ8KW58SOOJq.fxvKPVqMjyy."
    username: "alice"
    userID: "alice-static-uid"
```

- [ ] **Step 3: Write dex.go**

`internal/testutil/oidctest/dex.go`:

```go
// Package oidctest spins up dexidp/dex as a lightweight OIDC IdP for
// integration tests. Each call to StartDex returns a running container with
// a unique issuer URL and the matching client credentials.
package oidctest

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

//go:embed dex-config.yaml
var dexConfigYAML string

// Instance holds connection info for a running dex container.
type Instance struct {
	Issuer        string // http://host:port — fully qualified
	ClientID      string // pre-configured server client
	ClientSecret  string
	CLIClientID   string // pre-configured public CLI client (PKCE)
	StaticUser    StaticUser
	container     testcontainers.Container
}

type StaticUser struct {
	Email    string
	Password string
	Sub      string
}

// StartDex boots a dex container and returns an Instance. Cleanup is
// registered with t.Cleanup.
func StartDex(t *testing.T) *Instance {
	t.Helper()
	ctx := context.Background()

	// We don't know the host port until the container starts, so we boot
	// dex with a placeholder issuer, then dex serves discovery from
	// whatever the user requested via Host header. Modern dex accepts this.
	// For simplicity, we point issuer at the gateway host inside the
	// container network and rely on testcontainers' GetHost+MappedPort.

	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/dexidp/dex:v2.41.1",
		ExposedPorts: []string{"5556/tcp"},
		Cmd:          []string{"dex", "serve", "/etc/dex/config.yaml"},
		WaitingFor: wait.ForHTTP("/.well-known/openid-configuration").
			WithPort("5556/tcp").
			WithStartupTimeout(30 * time.Second),
		Files: []testcontainers.ContainerFile{},
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          false,
	})
	if err != nil {
		t.Fatalf("dex container request: %v", err)
	}

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("dex host: %v", err)
	}

	// Pre-start: write the config file with the right issuer.
	port, err := c.MappedPort(ctx, "5556/tcp")
	if err != nil {
		// Container not started yet; we need to start once to map port.
		if err := c.Start(ctx); err != nil {
			t.Fatalf("dex start: %v", err)
		}
		port, err = c.MappedPort(ctx, "5556/tcp")
		if err != nil {
			t.Fatalf("dex port: %v", err)
		}
	}
	issuer := fmt.Sprintf("http://%s:%s", host, port.Port())
	configured := strings.ReplaceAll(dexConfigYAML, "DEX_ISSUER_PLACEHOLDER", host+":"+port.Port())

	// Inject config and restart dex.
	if err := c.CopyToContainer(ctx, []byte(configured), "/etc/dex/config.yaml", 0644); err != nil {
		t.Fatalf("dex copy config: %v", err)
	}
	// Trigger a restart by stopping + starting via exec.
	timeout := 5 * time.Second
	if err := c.Stop(ctx, &timeout); err != nil {
		t.Fatalf("dex stop: %v", err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatalf("dex restart: %v", err)
	}

	t.Cleanup(func() {
		_ = c.Terminate(context.Background())
	})

	return &Instance{
		Issuer:       issuer,
		ClientID:     "flow-server",
		ClientSecret: "flow-server-secret",
		CLIClientID:  "flow-cli",
		StaticUser: StaticUser{
			Email:    "alice@example.com",
			Password: "password",
			Sub:      "alice-static-uid",
		},
		container: c,
	}
}
```

- [ ] **Step 4: Smoke-test by hand**

```bash
go build ./internal/testutil/oidctest/
```
(Real exercise comes in later integration tasks.)

- [ ] **Step 5: Commit**

```bash
git add internal/testutil/oidctest go.mod go.sum
git commit -m "test: testcontainers dex helper for OIDC integration tests

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: oidcserver — JWKS-cache + Verifier

**Files:**
- Create: `internal/adapter/oidcserver/jwks.go`
- Create: `internal/adapter/oidcserver/verifier.go`
- Create: `internal/adapter/oidcserver/verifier_test.go`

- [ ] **Step 1: Write failing integration test**

`internal/adapter/oidcserver/verifier_test.go`:

```go
//go:build integration

package oidcserver_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/oidcserver"
	"github.com/serverkraken/flow/internal/testutil/oidctest"
	"golang.org/x/oauth2"
	oidc "github.com/coreos/go-oidc/v3/oidc"
)

func TestIntegration_Verifier_AcceptsValidIDToken(t *testing.T) {
	dex := oidctest.StartDex(t)
	ctx := context.Background()

	prov, err := oidcserver.NewProvider(ctx, oidcserver.ProviderConfig{
		Issuer:   dex.Issuer,
		ClientID: dex.ClientID,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	idToken := mintIDTokenViaDex(t, dex)

	id, err := prov.Verify(ctx, idToken)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id.Sub != dex.StaticUser.Sub {
		t.Errorf("Sub = %q, want %q", id.Sub, dex.StaticUser.Sub)
	}
	if id.Email != dex.StaticUser.Email {
		t.Errorf("Email = %q, want %q", id.Email, dex.StaticUser.Email)
	}
}

// mintIDTokenViaDex runs the resource-owner password grant against dex
// (enabled by enablePasswordDB) and returns the ID token for the static
// user. Hex-only path that avoids us having to drive a browser.
func mintIDTokenViaDex(t *testing.T, dex *oidctest.Instance) string {
	t.Helper()
	ctx := context.Background()
	prov, err := oidc.NewProvider(ctx, dex.Issuer)
	if err != nil {
		t.Fatalf("oidc.NewProvider: %v", err)
	}

	cfg := oauth2.Config{
		ClientID:     dex.ClientID,
		ClientSecret: dex.ClientSecret,
		Endpoint:     prov.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile", "offline_access"},
	}

	values := url.Values{}
	values.Set("grant_type", "password")
	values.Set("username", dex.StaticUser.Email)
	values.Set("password", dex.StaticUser.Password)
	values.Set("scope", "openid email profile")

	tok, err := cfg.PasswordCredentialsToken(ctx, dex.StaticUser.Email, dex.StaticUser.Password)
	if err != nil {
		// Dex >=2.39 disables ROPC by default. Fallback to using a manual
		// HTTP request against dex's token endpoint with grant_type=password.
		req, _ := http.NewRequest(http.MethodPost, prov.Endpoint().TokenURL, nil)
		req.PostForm = values
		req.SetBasicAuth(cfg.ClientID, cfg.ClientSecret)
		t.Fatalf("password grant failed (dex may require auth-code flow): %v", err)
	}
	idToken, ok := tok.Extra("id_token").(string)
	if !ok || idToken == "" {
		t.Fatalf("no id_token in token response")
	}
	return idToken
}
```

(If ROPC is disabled in the chosen dex version, this test must drive the auth-code flow manually — see Task 9 integration where we do exactly that. For now, keep this as a smoke test and skip if it fails with a clear error.)

- [ ] **Step 2: Run test, expect FAIL**

Run: `go test -tags integration ./internal/adapter/oidcserver/`
Expected: build error.

- [ ] **Step 3: Implement provider**

`internal/adapter/oidcserver/jwks.go`:

```go
// Package oidcserver verifies OIDC ID-Tokens issued by an external IdP
// (Authentik in production, dex in tests). It uses coreos/go-oidc which
// internally maintains a JWKS cache keyed by issuer URL and refreshes on
// kid-miss.
package oidcserver

import (
	"context"
	"fmt"
	"time"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/serverkraken/flow/internal/ports"
)

// ProviderConfig captures everything Verify needs.
type ProviderConfig struct {
	Issuer   string
	ClientID string // expected 'aud' value
}

// Provider is the concrete AuthProvider.
type Provider struct {
	verifier *oidc.IDTokenVerifier
	provider *oidc.Provider
}

// NewProvider initialises the underlying oidc.Provider (which fetches the
// discovery document and JWKS endpoint) and a verifier scoped to clientID.
func NewProvider(ctx context.Context, cfg ProviderConfig) (*Provider, error) {
	p, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery (%s): %w", cfg.Issuer, err)
	}
	v := p.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	return &Provider{verifier: v, provider: p}, nil
}

// Endpoint exposes the OAuth2 endpoint discovered from the issuer; needed by
// the browser auth-code handler.
func (p *Provider) Endpoint() (authURL, tokenURL string) {
	e := p.provider.Endpoint()
	return e.AuthURL, e.TokenURL
}

// Verify implements ports.AuthProvider.
func (p *Provider) Verify(ctx context.Context, raw string) (ports.Identity, error) {
	tok, err := p.verifier.Verify(ctx, raw)
	if err != nil {
		return ports.Identity{}, fmt.Errorf("verify: %w", err)
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := tok.Claims(&claims); err != nil {
		return ports.Identity{}, fmt.Errorf("claims: %w", err)
	}
	return ports.Identity{
		Sub:           tok.Subject,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
		Name:          claims.Name,
		IssuedAt:      tok.IssuedAt,
		ExpiresAt:     tok.Expiry,
	}, nil
}

// Compile-time assertion.
var _ ports.AuthProvider = (*Provider)(nil)

// jwksRefreshInterval documents the internal refresh policy of coreos/go-oidc;
// kept as a constant so the spec-file can reference an exact number.
const jwksRefreshInterval = 12 * time.Hour
```

`internal/adapter/oidcserver/verifier.go`:

```go
package oidcserver

// This file is intentionally tiny — Verify lives on Provider in jwks.go.
// Kept as a placeholder so future helpers (claim transformers, multi-issuer
// fallback) have an obvious home.
```

- [ ] **Step 4: Run integration test against dex**

Run: `go test -tags integration ./internal/adapter/oidcserver/ -v`
Expected: PASS (assumes Docker is running locally).

If the ROPC path fails (newer dex versions block it), update `mintIDTokenViaDex` to drive the auth-code flow via an HTTP-client that follows redirects and submits the login form. Reference snippet:

```go
// Auth-code flow without browser:
//   1. GET dex /auth → follow redirects → login form HTML
//   2. POST credentials → dex 302 to callback w/ code
//   3. POST /token w/ code → token bundle
// Use net/http with a manual CookieJar + manual redirect handling.
```

(Defer the snippet to Task 9 where it lives in the test-helper.)

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/oidcserver
git commit -m "feat(oidcserver): JWKS-backed JWT verifier via coreos/go-oidc

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: oidcserver — Allowlist

**Files:**
- Create: `internal/adapter/oidcserver/allowlist.go`
- Create: `internal/adapter/oidcserver/allowlist_test.go`

- [ ] **Step 1: Write failing test**

`internal/adapter/oidcserver/allowlist_test.go`:

```go
package oidcserver

import (
	"testing"

	"github.com/serverkraken/flow/internal/ports"
)

func TestUnit_Allowlist_AllowsListedSub(t *testing.T) {
	t.Parallel()
	al := NewSubAllowlist([]string{"sub-1", "sub-2"})
	if !al.Allow(ports.Identity{Sub: "sub-1"}) {
		t.Error("sub-1 should be allowed")
	}
	if !al.Allow(ports.Identity{Sub: "sub-2"}) {
		t.Error("sub-2 should be allowed")
	}
}

func TestUnit_Allowlist_RejectsUnlistedSub(t *testing.T) {
	t.Parallel()
	al := NewSubAllowlist([]string{"sub-1"})
	if al.Allow(ports.Identity{Sub: "sub-other"}) {
		t.Error("sub-other should be rejected")
	}
}

func TestUnit_Allowlist_EmptyList_RejectsEverything(t *testing.T) {
	t.Parallel()
	al := NewSubAllowlist(nil)
	if al.Allow(ports.Identity{Sub: "anyone"}) {
		t.Error("empty allowlist must reject all")
	}
}
```

- [ ] **Step 2: Run test, expect FAIL**

Expected: build error.

- [ ] **Step 3: Implement**

`internal/adapter/oidcserver/allowlist.go`:

```go
package oidcserver

import (
	"github.com/serverkraken/flow/internal/ports"
)

// SubAllowlist is the Phase-1 AccessChecker — a finite set of OIDC 'sub'
// values that are permitted to use the server. Phase 2 will swap this for a
// User-table lookup once self-service registration is in scope.
//
// An empty list rejects everyone (fail-closed).
type SubAllowlist struct {
	set map[string]struct{}
}

func NewSubAllowlist(subs []string) *SubAllowlist {
	m := make(map[string]struct{}, len(subs))
	for _, s := range subs {
		if s == "" {
			continue
		}
		m[s] = struct{}{}
	}
	return &SubAllowlist{set: m}
}

func (a *SubAllowlist) Allow(id ports.Identity) bool {
	_, ok := a.set[id.Sub]
	return ok
}

// Compile-time assertion.
var _ ports.AccessChecker = (*SubAllowlist)(nil)
```

- [ ] **Step 4: Run tests, expect PASS**

```bash
go test ./internal/adapter/oidcserver/ -run TestUnit_Allowlist
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/oidcserver/allowlist.go internal/adapter/oidcserver/allowlist_test.go
git commit -m "feat(oidcserver): SubAllowlist as Phase-1 AccessChecker

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Session cookie via securecookie

**Files:**
- Create: `internal/adapter/httpserver/session.go`
- Create: `internal/adapter/httpserver/session_test.go`

- [ ] **Step 1: Add dep**

```bash
go get github.com/gorilla/securecookie
```

- [ ] **Step 2: Write failing test**

`internal/adapter/httpserver/session_test.go`:

```go
package httpserver

import (
	"encoding/hex"
	"testing"
)

type sessVal struct {
	Sub   string
	Email string
}

func TestUnit_Session_EncodeDecodeRoundtrip(t *testing.T) {
	t.Parallel()
	hash, _ := hex.DecodeString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	block, _ := hex.DecodeString("fedcba9876543210fedcba9876543210")
	s := NewSession(hash, block)

	in := sessVal{Sub: "user-1", Email: "alice@example.com"}
	enc, err := s.Encode("flow_session", in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	var out sessVal
	if err := s.Decode("flow_session", enc, &out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if out != in {
		t.Fatalf("got %+v, want %+v", out, in)
	}
}

func TestUnit_Session_TamperedValue_FailsDecode(t *testing.T) {
	t.Parallel()
	hash, _ := hex.DecodeString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	block, _ := hex.DecodeString("fedcba9876543210fedcba9876543210")
	s := NewSession(hash, block)

	enc, _ := s.Encode("flow_session", sessVal{Sub: "user-1"})
	// flip a character mid-string to corrupt the MAC
	tampered := enc[:len(enc)/2] + "X" + enc[len(enc)/2+1:]

	var out sessVal
	if err := s.Decode("flow_session", tampered, &out); err == nil {
		t.Fatal("expected decode error on tampered cookie")
	}
}
```

- [ ] **Step 3: Run test, expect FAIL**

Expected: build error.

- [ ] **Step 4: Implement**

`internal/adapter/httpserver/session.go`:

```go
package httpserver

import (
	"encoding/hex"
	"errors"

	"github.com/gorilla/securecookie"
	"github.com/serverkraken/flow/internal/ports"
)

// Session wraps gorilla/securecookie so handlers can encode/decode arbitrary
// values to a single signed+encrypted cookie blob. The hash key is HMAC, the
// block key is AES.
type Session struct {
	sc *securecookie.SecureCookie
}

func NewSession(hashKey, blockKey []byte) *Session {
	return &Session{sc: securecookie.New(hashKey, blockKey)}
}

func NewSessionFromHex(hashHex, blockHex string) (*Session, error) {
	hashKey, err := hex.DecodeString(hashHex)
	if err != nil {
		return nil, errors.New("FLOW_COOKIE_HASH_KEY: invalid hex")
	}
	blockKey, err := hex.DecodeString(blockHex)
	if err != nil {
		return nil, errors.New("FLOW_COOKIE_BLOCK_KEY: invalid hex")
	}
	return NewSession(hashKey, blockKey), nil
}

func (s *Session) Encode(name string, value any) (string, error) {
	return s.sc.Encode(name, value)
}

func (s *Session) Decode(name, raw string, out any) error {
	return s.sc.Decode(name, raw, out)
}

// Compile-time assertion.
var _ ports.SessionStore = (*Session)(nil)
```

- [ ] **Step 5: Run tests, expect PASS**

```bash
go test ./internal/adapter/httpserver/ -run TestUnit_Session
```

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/httpserver/session.go internal/adapter/httpserver/session_test.go go.mod go.sum
git commit -m "feat(server): signed+encrypted session cookie via gorilla/securecookie

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Browser auth-code handlers (/login, /auth/callback, /logout)

**Files:**
- Create: `internal/adapter/httpserver/auth_browser.go`
- Create: `internal/adapter/httpserver/auth_browser_test.go`
- Modify: `internal/adapter/httpserver/server.go`

- [ ] **Step 1: Write failing integration test**

`internal/adapter/httpserver/auth_browser_test.go`:

```go
//go:build integration

package httpserver_test

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/oidcserver"
	"github.com/serverkraken/flow/internal/testutil/oidctest"
)

func TestIntegration_BrowserFlow_LoginCallbackMe(t *testing.T) {
	dex := oidctest.StartDex(t)
	ctx := context.Background()

	prov, err := oidcserver.NewProvider(ctx, oidcserver.ProviderConfig{
		Issuer: dex.Issuer, ClientID: dex.ClientID,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	access := oidcserver.NewSubAllowlist([]string{dex.StaticUser.Sub})

	// Random keys for tests
	hashKey := strings.Repeat("a", 64)
	blockKey := strings.Repeat("b", 32)
	sess, err := httpserver.NewSessionFromHex(hashKey, blockKey)
	if err != nil {
		t.Fatalf("session: %v", err)
	}

	srv := httpserver.NewWithAuth(httpserver.AuthDeps{
		Provider:     prov,
		Access:       access,
		Session:      sess,
		BaseURL:      "http://localhost:0", // overridden by httptest.NewServer
		OIDCClientID: dex.ClientID,
		OIDCSecret:   dex.ClientSecret,
		Cookie:       httpserver.CookieConfig{Name: "flow_session", Secure: false},
		Ready:        func() error { return nil },
	})

	ts := newHTTPTestServer(t, srv.Handler())
	srv.SetBaseURL(ts.URL)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if strings.HasPrefix(req.URL.String(), ts.URL) {
				// stop on first hop back to flow-server
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	// drive the flow: GET /login → dex auth → submit form → callback → cookie set
	driveDexLogin(t, client, ts.URL, dex)

	// /api/v1/me now responds 200 with our identity
	resp, err := client.Get(ts.URL + "/api/v1/me")
	if err != nil {
		t.Fatalf("GET /me: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

// helpers — see auth_browser_test_helpers.go (next task) for newHTTPTestServer
// and driveDexLogin.
```

Companion file `auth_browser_test_helpers.go` (also `//go:build integration`) contains:

```go
//go:build integration

package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/testutil/oidctest"
)

func newHTTPTestServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

// driveDexLogin walks the user through dex's login form using plain net/http.
// Steps:
//   1. GET /login → flow redirects to dex /auth → dex serves HTML
//   2. POST form to dex with email + password
//   3. dex redirects to flow /auth/callback?code=...
//   4. flow exchanges code → ID token → sets cookie
func driveDexLogin(t *testing.T, client *http.Client, flowBase string, dex *oidctest.Instance) {
	t.Helper()

	resp, err := client.Get(flowBase + "/login")
	if err != nil {
		t.Fatalf("/login: %v", err)
	}
	resp.Body.Close()

	// follow redirect chain to dex's login page
	for resp.StatusCode == http.StatusFound {
		loc, err := resp.Location()
		if err != nil {
			t.Fatalf("Location: %v", err)
		}
		resp, err = client.Get(loc.String())
		if err != nil {
			t.Fatalf("redirect to %s: %v", loc, err)
		}
		resp.Body.Close()
		if strings.Contains(loc.Path, "/auth/callback") {
			break
		}
		if strings.Contains(loc.Path, "/auth/local/login") {
			// submit credentials
			form := url.Values{}
			form.Set("login", dex.StaticUser.Email)
			form.Set("password", dex.StaticUser.Password)
			req, _ := http.NewRequest(http.MethodPost, loc.String(), strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("submit dex form: %v", err)
			}
			resp.Body.Close()
		}
	}
}
```

- [ ] **Step 2: Run test, expect FAIL**

Run: `go test -tags integration ./internal/adapter/httpserver/`
Expected: build error.

- [ ] **Step 3: Implement auth_browser.go**

`internal/adapter/httpserver/auth_browser.go`:

```go
package httpserver

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/serverkraken/flow/internal/ports"
	"golang.org/x/oauth2"
)

// CookieConfig holds runtime options for the session cookie. Secure must be
// true in production but false for HTTP-only local dev (Docker, no TLS).
type CookieConfig struct {
	Name   string
	Secure bool
}

// AuthDeps bundles all dependencies needed by the browser-flow handlers.
// Provider is held as an interface (see oidcserverProvider below) so this
// package doesn't import internal/adapter/oidcserver and create an
// import cycle.
type AuthDeps struct {
	Provider     oidcserverProvider
	Access       ports.AccessChecker
	Session      ports.SessionStore
	BaseURL      string
	OIDCClientID string
	OIDCSecret   string
	Cookie       CookieConfig
	Ready        ReadinessCheck
	OIDCConfig   OIDCConfigResponse // populated by cmd/flow-server (see Task 17)
}

// authBrowser holds wiring for the three browser endpoints.
type authBrowser struct {
	deps AuthDeps
	oa   oauth2.Config
}

func newAuthBrowser(d AuthDeps) *authBrowser {
	authURL, tokenURL := d.Provider.Endpoint()
	return &authBrowser{
		deps: d,
		oa: oauth2.Config{
			ClientID:     d.OIDCClientID,
			ClientSecret: d.OIDCSecret,
			RedirectURL:  d.BaseURL + "/auth/callback",
			Endpoint:     oauth2.Endpoint{AuthURL: authURL, TokenURL: tokenURL},
			Scopes:       []string{oidc.ScopeOpenID, "email", "profile", "offline_access"},
		},
	}
}

func (ab *authBrowser) handleLogin(w http.ResponseWriter, r *http.Request) {
	state := randomState()
	// Persist state in a short-lived cookie so /auth/callback can verify
	// it. CSRF defence.
	http.SetCookie(w, &http.Cookie{
		Name:     "flow_oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   ab.deps.Cookie.Secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, ab.oa.AuthCodeURL(state), http.StatusFound)
}

func (ab *authBrowser) handleCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("flow_oauth_state")
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "state mismatch", http.StatusBadRequest)
		return
	}
	// Clear state cookie
	http.SetCookie(w, &http.Cookie{Name: "flow_oauth_state", Value: "", Path: "/", MaxAge: -1})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "no code", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	tok, err := ab.oa.Exchange(ctx, code)
	if err != nil {
		http.Error(w, "token exchange: "+err.Error(), http.StatusBadGateway)
		return
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok {
		http.Error(w, "no id_token in response", http.StatusBadGateway)
		return
	}
	id, err := ab.deps.Provider.Verify(ctx, rawID)
	if err != nil {
		http.Error(w, "id_token verify: "+err.Error(), http.StatusUnauthorized)
		return
	}
	if !ab.deps.Access.Allow(id) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	enc, err := ab.deps.Session.Encode(ab.deps.Cookie.Name, sessionValue{
		Sub:       id.Sub,
		Email:     id.Email,
		Name:      id.Name,
		ExpiresAt: id.ExpiresAt.Unix(),
	})
	if err != nil {
		http.Error(w, "session encode", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     ab.deps.Cookie.Name,
		Value:    enc,
		Path:     "/",
		Expires:  time.Now().Add(8 * time.Hour),
		HttpOnly: true,
		Secure:   ab.deps.Cookie.Secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (ab *authBrowser) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     ab.deps.Cookie.Name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   ab.deps.Cookie.Secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// sessionValue is the encrypted payload of the session cookie. Tiny on
// purpose — anything bigger goes in a server-side store later.
type sessionValue struct {
	Sub       string
	Email     string
	Name      string
	ExpiresAt int64
}

func randomState() string {
	b := make([]byte, 24)
	_, err := rand.Read(b)
	if err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// oidcserverProvider is a thin local alias to break the import cycle that
// arises if we import oidcserver here directly. We satisfy it via duck-typing
// at wiring time (cmd/flow-server passes a *oidcserver.Provider).
type oidcserverProvider interface {
	ports.AuthProvider
	Endpoint() (authURL, tokenURL string)
}

// ErrSessionMissing is returned by middleware when no/invalid session cookie
// is present.
var ErrSessionMissing = errors.New("no session")
```

Wire it in `server.go`:

```go
// NewWithAuth assembles the full Phase-1-M1 server: healthz + readyz +
// browser auth + /me. Plain New() is kept for tests that don't need auth.
func NewWithAuth(d AuthDeps) *Server {
	r := chi.NewRouter()
	r.Handle("/healthz", NewHealthzHandler())
	r.Handle("/readyz", NewReadyzHandler(d.Ready))

	ab := newAuthBrowser(d)
	r.Get("/login", ab.handleLogin)
	r.Get("/auth/callback", ab.handleCallback)
	r.Get("/logout", ab.handleLogout)

	// /me requires auth middleware (Task 11).
	r.Group(func(rr chi.Router) {
		rr.Use(NewAuthMiddleware(d.Session, d.Cookie.Name))
		rr.Get("/api/v1/me", NewMeHandler())
	})

	return &Server{router: r, baseURL: d.BaseURL}
}

// SetBaseURL allows tests to swap baseURL after the server is constructed
// (so RedirectURL matches httptest's random port).
func (s *Server) SetBaseURL(u string) { s.baseURL = u }
```

Note: Update `Server` struct to hold `baseURL string`; not strictly used at runtime but needed by tests.

- [ ] **Step 4: Run integration test**

Run: `go test -tags integration ./internal/adapter/httpserver/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/httpserver
git commit -m "feat(server): browser OIDC auth-code flow (login/callback/logout)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 11: Auth middleware + /api/v1/me

**Files:**
- Create: `internal/adapter/httpserver/middleware.go`
- Create: `internal/adapter/httpserver/me.go`
- Create: `internal/adapter/httpserver/middleware_test.go`
- Create: `internal/adapter/httpserver/me_test.go`

- [ ] **Step 1: Write failing tests**

`internal/adapter/httpserver/middleware_test.go`:

```go
package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnit_AuthMiddleware_NoCookie_Returns401(t *testing.T) {
	t.Parallel()
	mw := NewAuthMiddleware(stubSession{}, "flow_session")
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/anything", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestUnit_AuthMiddleware_ValidCookie_PassesThrough(t *testing.T) {
	t.Parallel()
	mw := NewAuthMiddleware(stubSession{decodeOK: true, sub: "user-1"}, "flow_session")
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got := SubFromContext(r.Context()); got != "user-1" {
			t.Errorf("sub from context = %q, want user-1", got)
		}
		w.WriteHeader(http.StatusOK)
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: "some-encoded-value"})
	mw(next).ServeHTTP(rr, req)
	if !called {
		t.Fatal("downstream handler was not called")
	}
}

// stubSession is a SessionStore for tests.
type stubSession struct {
	decodeOK bool
	sub      string
}

func (s stubSession) Encode(name string, value any) (string, error) { return "", nil }
func (s stubSession) Decode(name, raw string, out any) error {
	if !s.decodeOK {
		return errStubDecode
	}
	sv, ok := out.(*sessionValue)
	if !ok {
		return errStubDecode
	}
	sv.Sub = s.sub
	return nil
}

var errStubDecode = errSentinelMW("stub decode failed")

type errSentinelMW string

func (e errSentinelMW) Error() string { return string(e) }
```

`internal/adapter/httpserver/me_test.go`:

```go
package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnit_Me_WithContextSub_ReturnsJSON(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req = req.WithContext(WithSub(context.Background(), sessionValue{
		Sub: "user-1", Email: "alice@example.com", Name: "Alice",
	}))

	NewMeHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var out map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["sub"] != "user-1" {
		t.Errorf("sub = %v, want user-1", out["sub"])
	}
	if out["email"] != "alice@example.com" {
		t.Errorf("email = %v, want alice@example.com", out["email"])
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/adapter/httpserver/ -run 'AuthMiddleware|Me'`
Expected: build error.

- [ ] **Step 3: Implement middleware**

`internal/adapter/httpserver/middleware.go`:

```go
package httpserver

import (
	"context"
	"net/http"

	"github.com/serverkraken/flow/internal/ports"
)

type ctxKey int

const ctxKeySession ctxKey = 1

// WithSub attaches the resolved session value to a context. Used by handlers
// downstream of NewAuthMiddleware.
func WithSub(ctx context.Context, sv sessionValue) context.Context {
	return context.WithValue(ctx, ctxKeySession, sv)
}

// SubFromContext returns the sub claim of the authenticated user, empty
// string if not present.
func SubFromContext(ctx context.Context) string {
	sv, _ := ctx.Value(ctxKeySession).(sessionValue)
	return sv.Sub
}

// sessionFromContext returns the full sessionValue.
func sessionFromContext(ctx context.Context) (sessionValue, bool) {
	sv, ok := ctx.Value(ctxKeySession).(sessionValue)
	return sv, ok
}

// NewAuthMiddleware enforces a valid session cookie. Returns 401 if missing
// or invalid. Attaches the sessionValue to context on success.
func NewAuthMiddleware(sess ports.SessionStore, cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(cookieName)
			if err != nil || c.Value == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			var sv sessionValue
			if err := sess.Decode(cookieName, c.Value, &sv); err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithSub(r.Context(), sv)))
		})
	}
}
```

`internal/adapter/httpserver/me.go`:

```go
package httpserver

import (
	"encoding/json"
	"net/http"
)

// NewMeHandler serves /api/v1/me — returns the current user's identity as
// derived from the session cookie. Behind NewAuthMiddleware.
func NewMeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv, ok := sessionFromContext(r.Context())
		if !ok {
			http.Error(w, "no session", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":   sv.Sub,
			"email": sv.Email,
			"name":  sv.Name,
		})
	})
}
```

- [ ] **Step 4: Run tests, expect PASS**

Run: `go test ./internal/adapter/httpserver/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/httpserver/middleware.go internal/adapter/httpserver/me.go \
        internal/adapter/httpserver/middleware_test.go internal/adapter/httpserver/me_test.go
git commit -m "feat(server): session auth middleware + /api/v1/me

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 12: Bearer-token auth middleware (for CLI/MCP)

**Files:**
- Modify: `internal/adapter/httpserver/middleware.go`
- Create: `internal/adapter/httpserver/middleware_bearer_test.go`
- Modify: `internal/adapter/httpserver/server.go`

The browser flow gives users a cookie. The CLI / MCP holds an access token and sends `Authorization: Bearer <jwt>`. Same protected endpoints, different proof.

- [ ] **Step 1: Write failing test**

`internal/adapter/httpserver/middleware_bearer_test.go`:

```go
package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/ports"
)

func TestUnit_BearerMiddleware_NoHeader_Returns401(t *testing.T) {
	t.Parallel()
	mw := NewBearerMiddleware(stubAuthProvider{}, stubAccess{allowAll: true})
	rr := httptest.NewRecorder()
	mw(http.NotFoundHandler()).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestUnit_BearerMiddleware_ValidToken_PassesThrough(t *testing.T) {
	t.Parallel()
	mw := NewBearerMiddleware(stubAuthProvider{id: ports.Identity{Sub: "u-1"}}, stubAccess{allowAll: true})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer faketoken")
	got := ""
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = SubFromContext(r.Context())
	})).ServeHTTP(rr, req)
	if got != "u-1" {
		t.Errorf("sub = %q, want u-1", got)
	}
}

func TestUnit_BearerMiddleware_ForbiddenIdentity_Returns403(t *testing.T) {
	t.Parallel()
	mw := NewBearerMiddleware(stubAuthProvider{id: ports.Identity{Sub: "u-1"}}, stubAccess{allowAll: false})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer faketoken")
	mw(http.NotFoundHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}

type stubAuthProvider struct {
	id  ports.Identity
	err error
}

func (s stubAuthProvider) Verify(_ context.Context, _ string) (ports.Identity, error) {
	if s.err != nil {
		return ports.Identity{}, s.err
	}
	return s.id, nil
}

type stubAccess struct{ allowAll bool }

func (s stubAccess) Allow(_ ports.Identity) bool { return s.allowAll }
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/adapter/httpserver/ -run TestUnit_BearerMiddleware`

- [ ] **Step 3: Implement bearer middleware**

Append to `internal/adapter/httpserver/middleware.go`:

```go
// NewBearerMiddleware enforces Authorization: Bearer <jwt>. Verifies via
// AuthProvider, runs AccessChecker, attaches identity to context.
func NewBearerMiddleware(prov ports.AuthProvider, access ports.AccessChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if len(h) < 8 || h[:7] != "Bearer " {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			token := h[7:]
			id, err := prov.Verify(r.Context(), token)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if !access.Allow(id) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			sv := sessionValue{Sub: id.Sub, Email: id.Email, Name: id.Name}
			next.ServeHTTP(w, r.WithContext(WithSub(r.Context(), sv)))
		})
	}
}
```

Update `server.go` to add a parallel `/api/v1/...` group protected by bearer:

```go
// In NewWithAuth, after the cookie-protected /me group:
r.Group(func(rr chi.Router) {
	rr.Use(NewBearerMiddleware(d.Provider, d.Access))
	rr.Get("/api/v1/me-bearer", NewMeHandler()) // for CLI/MCP — same handler
})
```

(In M3+ we move /me to a single endpoint that accepts either cookie or bearer; for M1 the two routes prove both mechanisms work.)

- [ ] **Step 4: Run tests, expect PASS**

```bash
go test ./internal/adapter/httpserver/
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/httpserver
git commit -m "feat(server): bearer-token middleware for CLI/MCP auth

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 13: oidcclient — Device-Authorization-Flow

**Files:**
- Create: `internal/adapter/oidcclient/deviceflow.go`
- Create: `internal/adapter/oidcclient/deviceflow_test.go`

- [ ] **Step 1: Write failing test**

`internal/adapter/oidcclient/deviceflow_test.go`:

```go
package oidcclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUnit_DeviceFlow_RequestAndPoll(t *testing.T) {
	t.Parallel()
	step := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/device_authorization"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":      "dev-abc",
				"user_code":        "ABC-123",
				"verification_uri": "https://idp.example/activate",
				"verification_uri_complete": "https://idp.example/activate?code=ABC-123",
				"expires_in":       600,
				"interval":         1,
			})
		case strings.HasSuffix(r.URL.Path, "/token"):
			step++
			if step == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "a-token",
				"refresh_token": "r-token",
				"id_token":      "id-token",
				"expires_in":    3600,
				"token_type":    "Bearer",
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	df := NewDeviceFlow(Config{
		ClientID:                "flow-cli",
		DeviceAuthorizationURL:  srv.URL + "/device_authorization",
		TokenURL:                srv.URL + "/token",
		Scopes:                  []string{"openid", "profile", "email", "offline_access"},
		HTTPClient:              srv.Client(),
		PollIntervalOverride:    50 * time.Millisecond, // override slow default
	})

	codes, err := df.Init(context.Background())
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if codes.UserCode != "ABC-123" {
		t.Errorf("UserCode = %q, want ABC-123", codes.UserCode)
	}

	tok, err := df.PollForToken(context.Background(), codes)
	if err != nil {
		t.Fatalf("PollForToken: %v", err)
	}
	if tok.AccessToken != "a-token" {
		t.Errorf("AccessToken = %q, want a-token", tok.AccessToken)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/adapter/oidcclient/`
Expected: build error.

- [ ] **Step 3: Implement**

`internal/adapter/oidcclient/deviceflow.go`:

```go
// Package oidcclient implements the OAuth2 Device-Authorization-Grant
// (RFC 8628) so flow's CLI/TUI/MCP can authenticate against Authentik
// without a browser callback. The flow is:
//
//  1. POST /device_authorization → server returns user_code + device_code
//  2. User visits verification_uri, types user_code, approves access
//  3. Client polls /token with grant_type=urn:ietf:params:oauth:grant-type:device_code
//     until server stops returning "authorization_pending"
package oidcclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

const (
	grantTypeDeviceCode = "urn:ietf:params:oauth:grant-type:device_code"
)

// Config holds the runtime config of the device-flow client.
type Config struct {
	ClientID               string
	ClientSecret           string // optional — public clients leave empty
	DeviceAuthorizationURL string
	TokenURL               string
	Scopes                 []string
	HTTPClient             *http.Client
	PollIntervalOverride   time.Duration // 0 → use server's interval
}

// Codes is the response of the device-authorization request.
type Codes struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	ExpiresIn               int
	Interval                int
}

// DeviceFlow drives the RFC-8628 dance.
type DeviceFlow struct {
	cfg Config
}

func NewDeviceFlow(c Config) *DeviceFlow {
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
	return &DeviceFlow{cfg: c}
}

// Init kicks off device authorization and returns the codes to display to the
// user.
func (d *DeviceFlow) Init(ctx context.Context) (Codes, error) {
	body := url.Values{}
	body.Set("client_id", d.cfg.ClientID)
	body.Set("scope", strings.Join(d.cfg.Scopes, " "))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.cfg.DeviceAuthorizationURL,
		strings.NewReader(body.Encode()))
	if err != nil {
		return Codes{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if d.cfg.ClientSecret != "" {
		req.SetBasicAuth(d.cfg.ClientID, d.cfg.ClientSecret)
	}

	resp, err := d.cfg.HTTPClient.Do(req)
	if err != nil {
		return Codes{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return Codes{}, fmt.Errorf("device_authorization: status %d: %s", resp.StatusCode, string(b))
	}

	var raw struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int    `json:"expires_in"`
		Interval                int    `json:"interval"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Codes{}, err
	}
	if raw.Interval == 0 {
		raw.Interval = 5
	}
	return Codes(raw), nil
}

// PollForToken polls /token until the user approves or until expires_in is
// reached. ctx cancellation aborts immediately.
func (d *DeviceFlow) PollForToken(ctx context.Context, c Codes) (ports.Tokens, error) {
	interval := time.Duration(c.Interval) * time.Second
	if d.cfg.PollIntervalOverride > 0 {
		interval = d.cfg.PollIntervalOverride
	}
	deadline := time.Now().Add(time.Duration(c.ExpiresIn) * time.Second)

	for {
		if time.Now().After(deadline) {
			return ports.Tokens{}, errors.New("device authorization expired")
		}

		tok, err := d.exchange(ctx, c.DeviceCode)
		if err == nil {
			return tok, nil
		}
		if errors.Is(err, errAuthorizationPending) {
			select {
			case <-ctx.Done():
				return ports.Tokens{}, ctx.Err()
			case <-time.After(interval):
				continue
			}
		}
		if errors.Is(err, errSlowDown) {
			interval += 5 * time.Second
			select {
			case <-ctx.Done():
				return ports.Tokens{}, ctx.Err()
			case <-time.After(interval):
				continue
			}
		}
		return ports.Tokens{}, err
	}
}

var (
	errAuthorizationPending = errors.New("authorization_pending")
	errSlowDown             = errors.New("slow_down")
)

func (d *DeviceFlow) exchange(ctx context.Context, deviceCode string) (ports.Tokens, error) {
	body := url.Values{}
	body.Set("grant_type", grantTypeDeviceCode)
	body.Set("device_code", deviceCode)
	body.Set("client_id", d.cfg.ClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.cfg.TokenURL,
		strings.NewReader(body.Encode()))
	if err != nil {
		return ports.Tokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if d.cfg.ClientSecret != "" {
		req.SetBasicAuth(d.cfg.ClientID, d.cfg.ClientSecret)
	}

	resp, err := d.cfg.HTTPClient.Do(req)
	if err != nil {
		return ports.Tokens{}, err
	}
	defer resp.Body.Close()

	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ports.Tokens{}, err
	}

	if raw.Error != "" {
		switch raw.Error {
		case "authorization_pending":
			return ports.Tokens{}, errAuthorizationPending
		case "slow_down":
			return ports.Tokens{}, errSlowDown
		default:
			return ports.Tokens{}, fmt.Errorf("token error: %s", raw.Error)
		}
	}
	return ports.Tokens{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		IDToken:      raw.IDToken,
		Expiry:       time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second),
	}, nil
}
```

- [ ] **Step 4: Run tests, expect PASS**

```bash
go test ./internal/adapter/oidcclient/
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/oidcclient go.mod go.sum
git commit -m "feat(oidcclient): RFC 8628 device-authorization-flow client

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 14: oidcclient — Refresh-token rotation

**Files:**
- Create: `internal/adapter/oidcclient/refresh.go`
- Create: `internal/adapter/oidcclient/refresh_test.go`

- [ ] **Step 1: Write failing test**

`internal/adapter/oidcclient/refresh_test.go`:

```go
package oidcclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUnit_Refresh_ExchangesRefreshTokenForAccessToken(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got := r.PostFormValue("grant_type"); got != "refresh_token" {
			t.Errorf("grant_type = %q", got)
		}
		if got := r.PostFormValue("refresh_token"); got != "old-r-token" {
			t.Errorf("refresh_token = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-a-token",
			"refresh_token": "new-r-token",
			"expires_in":    3600,
		})
	}))
	t.Cleanup(srv.Close)

	got, err := Refresh(context.Background(), RefreshConfig{
		ClientID:    "flow-cli",
		TokenURL:    srv.URL,
		HTTPClient:  srv.Client(),
		RefreshToken: "old-r-token",
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got.AccessToken != "new-a-token" || got.RefreshToken != "new-r-token" {
		t.Fatalf("got %+v", got)
	}
	if time.Until(got.Expiry) < 30*time.Minute {
		t.Errorf("Expiry too soon: %v", got.Expiry)
	}
}

// strings import used to keep helper consistent across files
var _ = strings.HasPrefix
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

`internal/adapter/oidcclient/refresh.go`:

```go
package oidcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

type RefreshConfig struct {
	ClientID     string
	ClientSecret string
	TokenURL     string
	HTTPClient   *http.Client
	RefreshToken string
}

// Refresh exchanges a refresh token for a fresh access (+ optionally new
// refresh) token. Authentik rotates refresh tokens by default — we always
// persist whatever comes back.
func Refresh(ctx context.Context, c RefreshConfig) (ports.Tokens, error) {
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}

	body := url.Values{}
	body.Set("grant_type", "refresh_token")
	body.Set("refresh_token", c.RefreshToken)
	body.Set("client_id", c.ClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL,
		strings.NewReader(body.Encode()))
	if err != nil {
		return ports.Tokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if c.ClientSecret != "" {
		req.SetBasicAuth(c.ClientID, c.ClientSecret)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return ports.Tokens{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return ports.Tokens{}, fmt.Errorf("refresh: status %d: %s", resp.StatusCode, string(b))
	}

	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ports.Tokens{}, err
	}
	if raw.RefreshToken == "" {
		raw.RefreshToken = c.RefreshToken // some IdPs don't rotate
	}
	return ports.Tokens{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		IDToken:      raw.IDToken,
		Expiry:       time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second),
	}, nil
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
go test ./internal/adapter/oidcclient/
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/oidcclient/refresh.go internal/adapter/oidcclient/refresh_test.go
git commit -m "feat(oidcclient): refresh-token rotation"
```

---

### Task 15: oidcclient — Tokens wrapper around TokenStore

**Files:**
- Create: `internal/adapter/oidcclient/tokens.go`
- Create: `internal/adapter/oidcclient/tokens_test.go`

This unifies "get a current access token" so callers (TUI, MCP) don't repeat the "fetch from store → refresh if expired → save back" sequence.

- [ ] **Step 1: Write failing test**

`internal/adapter/oidcclient/tokens_test.go`:

```go
package oidcclient

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/serverkraken/flow/internal/ports"
)

func TestUnit_Tokens_CurrentReturnsFresh_WhenNotExpired(t *testing.T) {
	t.Parallel()
	store := keyringadapter.NewFake()
	_ = store.Put("slot", ports.Tokens{
		AccessToken: "still-valid",
		Expiry:      time.Now().Add(time.Hour),
	})

	tm := NewTokens(TokensConfig{Store: store, SlotName: "slot"})
	tok, err := tm.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if tok.AccessToken != "still-valid" {
		t.Fatalf("AccessToken = %q", tok.AccessToken)
	}
}

func TestUnit_Tokens_NotLoggedIn_ReturnsErrTokenNotFound(t *testing.T) {
	t.Parallel()
	tm := NewTokens(TokensConfig{Store: keyringadapter.NewFake(), SlotName: "slot"})
	_, err := tm.Current(context.Background())
	if err != ports.ErrTokenNotFound {
		t.Fatalf("err = %v, want ErrTokenNotFound", err)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

`internal/adapter/oidcclient/tokens.go`:

```go
package oidcclient

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// TokensConfig wires Tokens to its dependencies.
type TokensConfig struct {
	Store          ports.TokenStore
	SlotName       string
	Refresh        func(context.Context, string) (ports.Tokens, error) // optional, defaults to no-op
	RefreshLeeway  time.Duration                                       // refresh this much before expiry
}

// Tokens is the high-level "give me a usable access token" API. It hides
// the TokenStore + Refresh sequence from callers.
type Tokens struct {
	cfg TokensConfig
}

func NewTokens(c TokensConfig) *Tokens {
	if c.RefreshLeeway == 0 {
		c.RefreshLeeway = 60 * time.Second
	}
	return &Tokens{cfg: c}
}

// Current returns a valid access token, refreshing if needed.
func (t *Tokens) Current(ctx context.Context) (ports.Tokens, error) {
	cur, err := t.cfg.Store.Get(t.cfg.SlotName)
	if err != nil {
		return ports.Tokens{}, err
	}
	if time.Until(cur.Expiry) > t.cfg.RefreshLeeway {
		return cur, nil
	}
	if t.cfg.Refresh == nil || cur.RefreshToken == "" {
		return cur, nil // best effort
	}
	fresh, err := t.cfg.Refresh(ctx, cur.RefreshToken)
	if err != nil {
		return ports.Tokens{}, err
	}
	if err := t.cfg.Store.Put(t.cfg.SlotName, fresh); err != nil {
		return ports.Tokens{}, err
	}
	return fresh, nil
}

// Save persists a fresh token bundle.
func (t *Tokens) Save(tok ports.Tokens) error {
	return t.cfg.Store.Put(t.cfg.SlotName, tok)
}

// Delete clears the slot (used by `flow logout`).
func (t *Tokens) Delete() error { return t.cfg.Store.Delete(t.cfg.SlotName) }
```

- [ ] **Step 4: Run, expect PASS**

```bash
go test ./internal/adapter/oidcclient/
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/oidcclient/tokens.go internal/adapter/oidcclient/tokens_test.go
git commit -m "feat(oidcclient): Tokens — unified TokenStore+Refresh facade"
```

---

### Task 16: CLI subcommand `flow login`

**Files:**
- Create: `cmd/flow/login.go`
- Create: `cmd/flow/login_test.go`
- Modify: `cmd/flow/main.go`

- [ ] **Step 1: Inspect existing main.go**

```bash
go doc github.com/spf13/cobra
sed -n '1,80p' cmd/flow/main.go
```

(Verify the existing root-command name and how subcommands are registered. The implementation below assumes cobra; if a different pattern is used, follow it.)

- [ ] **Step 2: Write failing test**

`cmd/flow/login_test.go`:

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/serverkraken/flow/internal/adapter/oidcclient"
)

func TestUnit_Login_HappyPath_PersistsTokens(t *testing.T) {
	t.Parallel()
	step := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/device_authorization"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":      "dev-x",
				"user_code":        "WXYZ-1234",
				"verification_uri": "http://idp.example/activate",
				"expires_in":       600,
				"interval":         1,
			})
		case strings.HasSuffix(r.URL.Path, "/token"):
			step++
			if step == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "a-tok",
				"refresh_token": "r-tok",
				"expires_in":    3600,
			})
		}
	}))
	t.Cleanup(srv.Close)

	store := keyringadapter.NewFake()
	out := &bytes.Buffer{}
	err := runLogin(context.Background(), loginConfig{
		ClientID:               "flow-cli",
		DeviceAuthorizationURL: srv.URL + "/device_authorization",
		TokenURL:               srv.URL + "/token",
		HTTPClient:             srv.Client(),
		PollOverride:           50 * time.Millisecond,
		Store:                  store,
		SlotName:               "test-slot",
		Out:                    out,
		OpenBrowser:            func(string) error { return nil },
	})
	if err != nil {
		t.Fatalf("runLogin: %v", err)
	}

	got, _ := store.Get("test-slot")
	if got.AccessToken != "a-tok" {
		t.Errorf("AccessToken = %q", got.AccessToken)
	}
	if !strings.Contains(out.String(), "WXYZ-1234") {
		t.Errorf("output did not show user code: %s", out.String())
	}

	// silence unused
	_ = oidcclient.NewTokens
}
```

- [ ] **Step 3: Run, expect FAIL**

- [ ] **Step 4: Implement**

`cmd/flow/login.go`:

```go
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/serverkraken/flow/internal/adapter/oidcclient"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/spf13/cobra"
)

// loginConfig captures everything runLogin needs. Lifted to a struct so tests
// can inject HTTP server, store, and clock-replacements.
type loginConfig struct {
	ClientID               string
	ClientSecret           string
	DeviceAuthorizationURL string
	TokenURL               string
	HTTPClient             *http.Client
	PollOverride           time.Duration
	Store                  ports.TokenStore
	SlotName               string
	Out                    io.Writer
	OpenBrowser            func(string) error
}

func runLogin(ctx context.Context, c loginConfig) error {
	df := oidcclient.NewDeviceFlow(oidcclient.Config{
		ClientID:               c.ClientID,
		ClientSecret:           c.ClientSecret,
		DeviceAuthorizationURL: c.DeviceAuthorizationURL,
		TokenURL:               c.TokenURL,
		Scopes:                 []string{"openid", "profile", "email", "offline_access"},
		HTTPClient:             c.HTTPClient,
		PollIntervalOverride:   c.PollOverride,
	})
	codes, err := df.Init(ctx)
	if err != nil {
		return fmt.Errorf("device init: %w", err)
	}
	fmt.Fprintf(c.Out, "\nUm flow zu autorisieren:\n  1. öffne %s\n  2. gib den Code ein: %s\n\n",
		codes.VerificationURI, codes.UserCode)
	if c.OpenBrowser != nil {
		url := codes.VerificationURIComplete
		if url == "" {
			url = codes.VerificationURI
		}
		_ = c.OpenBrowser(url)
	}
	tok, err := df.PollForToken(ctx, codes)
	if err != nil {
		return fmt.Errorf("device poll: %w", err)
	}
	if err := c.Store.Put(c.SlotName, tok); err != nil {
		return fmt.Errorf("token store: %w", err)
	}
	fmt.Fprintln(c.Out, "✓ Login erfolgreich, Token im Keychain gespeichert.")
	return nil
}

// openBrowser is the real default for the CLI subcommand. Tests substitute a
// no-op.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
	return nil
}

func newLoginCmd() *cobra.Command {
	var (
		serverURL string
		clientID  string
	)
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Beim flow-server anmelden (OIDC Device-Flow)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Resolve OIDC URLs by hitting flow-server /api/v1/oidc/config.
			// Simpler: read FLOW_OIDC_ISSUER etc. directly from env for now.
			// Phase-1 keeps configuration explicit.
			deviceURL, tokenURL, err := resolveOIDCEndpoints(cmd.Context(), serverURL)
			if err != nil {
				return err
			}
			return runLogin(cmd.Context(), loginConfig{
				ClientID:               clientID,
				DeviceAuthorizationURL: deviceURL,
				TokenURL:               tokenURL,
				HTTPClient:             http.DefaultClient,
				Store:                  keyringadapter.New(),
				SlotName:               slotNameFor(serverURL),
				Out:                    cmd.OutOrStdout(),
				OpenBrowser:            openBrowser,
			})
		},
	}
	cmd.Flags().StringVar(&serverURL, "server", envOr("FLOW_SERVER_URL", "http://localhost:8080"), "flow-server base URL")
	cmd.Flags().StringVar(&clientID, "client-id", envOr("FLOW_OIDC_CLIENT_ID", "flow-cli"), "OIDC client id")
	return cmd
}

// resolveOIDCEndpoints asks flow-server for the IdP's device/token URLs.
// In Phase-1 we expose them via /api/v1/oidc/config (no auth required).
func resolveOIDCEndpoints(ctx context.Context, serverURL string) (deviceURL, tokenURL string, err error) {
	// see Task 17: server endpoint that returns these
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/api/v1/oidc/config", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var cfg struct {
		DeviceAuthorizationURL string `json:"device_authorization_endpoint"`
		TokenURL               string `json:"token_endpoint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return "", "", err
	}
	return cfg.DeviceAuthorizationURL, cfg.TokenURL, nil
}

// slotNameFor derives a per-server slot so a user can log into multiple
// flow-servers (dev / prod / homelab) without collisions.
func slotNameFor(serverURL string) string {
	return "tokens:" + serverURL
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
```

Note: this file imports `encoding/json` and `os` — add to the import block above as the code is pasted in.

Then register in `cmd/flow/main.go` (locate the existing rootCmd assembly and add):

```go
rootCmd.AddCommand(newLoginCmd())
```

- [ ] **Step 5: Run unit test, expect PASS**

```bash
go test ./cmd/flow/ -run TestUnit_Login_HappyPath
```

- [ ] **Step 6: Commit**

```bash
git add cmd/flow/login.go cmd/flow/login_test.go cmd/flow/main.go
git commit -m "feat(cli): flow login subcommand (OIDC Device-Flow + keychain)"
```

---

### Task 17: flow-server endpoint /api/v1/oidc/config

**Files:**
- Create: `internal/adapter/httpserver/oidc_config.go`
- Create: `internal/adapter/httpserver/oidc_config_test.go`
- Modify: `internal/adapter/httpserver/server.go`

`flow login` needs to know which IdP to talk to. Rather than duplicate the FLOW_OIDC_ISSUER env-var into every client config, we expose it via a public unauth'd endpoint.

- [ ] **Step 1: Write failing test**

`internal/adapter/httpserver/oidc_config_test.go`:

```go
package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnit_OIDCConfig_ReturnsEndpoints(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	h := NewOIDCConfigHandler(OIDCConfigResponse{
		Issuer:                  "https://auth.example.com/realms/flow",
		DeviceAuthorizationURL:  "https://auth.example.com/device",
		TokenURL:                "https://auth.example.com/token",
		ClientID:                "flow-cli",
	})
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/oidc/config", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var got OIDCConfigResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DeviceAuthorizationURL != "https://auth.example.com/device" {
		t.Errorf("DeviceAuthorizationURL = %q", got.DeviceAuthorizationURL)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

`internal/adapter/httpserver/oidc_config.go`:

```go
package httpserver

import (
	"encoding/json"
	"net/http"
)

// OIDCConfigResponse exposes the IdP endpoints clients need to perform a
// device-flow. Unauthenticated — same information is in the well-known
// discovery doc but reachable via flow-server lets clients avoid having to
// know the IdP URL directly.
type OIDCConfigResponse struct {
	Issuer                 string `json:"issuer"`
	DeviceAuthorizationURL string `json:"device_authorization_endpoint"`
	TokenURL               string `json:"token_endpoint"`
	ClientID               string `json:"client_id"`
}

func NewOIDCConfigHandler(resp OIDCConfigResponse) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
}
```

Wire in `server.go` (`NewWithAuth`):

```go
r.Handle("/api/v1/oidc/config", NewOIDCConfigHandler(d.OIDCConfig))
```

(The `OIDCConfig` field is already declared on `AuthDeps` in Task 10's struct definition. It's computed once in `cmd/flow-server/main.go` from `provider.DeviceAuthorizationURL()`.) To get the device endpoint from coreos/go-oidc, add a method to `oidcserver.Provider`:

```go
// In internal/adapter/oidcserver/jwks.go, add:
func (p *Provider) DeviceAuthorizationURL() string {
	var claims struct {
		DeviceAuth string `json:"device_authorization_endpoint"`
	}
	_ = p.provider.Claims(&claims)
	return claims.DeviceAuth
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
go test ./internal/adapter/httpserver/ -run TestUnit_OIDCConfig
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/httpserver/oidc_config.go internal/adapter/httpserver/oidc_config_test.go \
        internal/adapter/httpserver/server.go internal/adapter/oidcserver/jwks.go
git commit -m "feat(server): /api/v1/oidc/config for CLI device-flow discovery"
```

---

### Task 18: CLI subcommands `flow logout` and `flow whoami`

**Files:**
- Create: `cmd/flow/logout.go`
- Create: `cmd/flow/whoami.go`
- Modify: `cmd/flow/main.go`

- [ ] **Step 1: Implement logout**

`cmd/flow/logout.go`:

```go
package main

import (
	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/spf13/cobra"
)

func newLogoutCmd() *cobra.Command {
	var serverURL string
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Token aus dem Keychain entfernen",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store := keyringadapter.New()
			if err := store.Delete(slotNameFor(serverURL)); err != nil {
				return err
			}
			cmd.Println("✓ Token entfernt.")
			return nil
		},
	}
	cmd.Flags().StringVar(&serverURL, "server", envOr("FLOW_SERVER_URL", "http://localhost:8080"), "flow-server base URL")
	return cmd
}
```

- [ ] **Step 2: Implement whoami**

`cmd/flow/whoami.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/spf13/cobra"
)

func newWhoamiCmd() *cobra.Command {
	var serverURL string
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Aktuell eingeloggten User vom Server abrufen",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store := keyringadapter.New()
			tok, err := store.Get(slotNameFor(serverURL))
			if errors.Is(err, ports.ErrTokenNotFound) {
				return errors.New("nicht eingeloggt — bitte `flow login` ausführen")
			}
			if err != nil {
				return err
			}
			req, _ := http.NewRequestWithContext(cmd.Context(), http.MethodGet, serverURL+"/api/v1/me-bearer", nil)
			req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server response: %s", resp.Status)
			}
			var out map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				return err
			}
			cmd.Printf("Sub:   %v\nEmail: %v\nName:  %v\n", out["sub"], out["email"], out["name"])
			_ = context.Background() // imports
			return nil
		},
	}
	cmd.Flags().StringVar(&serverURL, "server", envOr("FLOW_SERVER_URL", "http://localhost:8080"), "flow-server base URL")
	return cmd
}
```

- [ ] **Step 3: Register**

In `cmd/flow/main.go`:

```go
rootCmd.AddCommand(newLoginCmd(), newLogoutCmd(), newWhoamiCmd())
```

- [ ] **Step 4: Verify build**

```bash
go build ./cmd/flow
```

- [ ] **Step 5: Commit**

```bash
git add cmd/flow/logout.go cmd/flow/whoami.go cmd/flow/main.go
git commit -m "feat(cli): flow logout + flow whoami"
```

---

### Task 19: Full integration test — TUI login → bearer call → /api/v1/me

**Files:**
- Create: `internal/adapter/httpserver/integration_e2e_test.go`

This is the end-to-end test that proves M1 works as a whole.

- [ ] **Step 1: Write test**

`internal/adapter/httpserver/integration_e2e_test.go`:

```go
//go:build integration

package httpserver_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/serverkraken/flow/internal/adapter/oidcclient"
	"github.com/serverkraken/flow/internal/adapter/oidcserver"
	"github.com/serverkraken/flow/internal/testutil/oidctest"
)

// TestIntegration_E2E_LoginPlusBearerMe boots dex + flow-server, runs the
// device-flow client against dex, takes the access token, and uses it to
// call /api/v1/me-bearer.
func TestIntegration_E2E_LoginPlusBearerMe(t *testing.T) {
	dex := oidctest.StartDex(t)
	ctx := context.Background()

	prov, err := oidcserver.NewProvider(ctx, oidcserver.ProviderConfig{
		Issuer: dex.Issuer, ClientID: dex.CLIClientID,
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	access := oidcserver.NewSubAllowlist([]string{dex.StaticUser.Sub})
	hashKey, _ := hex.DecodeString(strings.Repeat("11", 32))
	blockKey, _ := hex.DecodeString(strings.Repeat("22", 16))
	sess := httpserver.NewSession(hashKey, blockKey)

	srv := httpserver.NewWithAuth(httpserver.AuthDeps{
		Provider: prov, Access: access, Session: sess,
		OIDCClientID: dex.CLIClientID, OIDCSecret: "",
		Cookie: httpserver.CookieConfig{Name: "flow_session", Secure: false},
		Ready:  func() error { return nil },
		OIDCConfig: httpserver.OIDCConfigResponse{
			Issuer:                 dex.Issuer,
			DeviceAuthorizationURL: dex.Issuer + "/device/code",
			TokenURL:               dex.Issuer + "/token",
			ClientID:               dex.CLIClientID,
		},
	})

	ts := newHTTPTestServer(t, srv.Handler())

	// Drive device-flow: spawn parallel goroutine that approves the device
	// code by hitting dex's user-facing approval endpoint. Use credentials.
	approvedCh := make(chan struct{})
	go func() {
		// Wait briefly for /device_authorization to issue codes, then
		// approve them via dex's POST.
		approveDexDeviceCode(t, dex)
		close(approvedCh)
	}()

	out := &bytes.Buffer{}
	store := keyringadapter.NewFake()

	err = runLoginShim(ctx, oidcclient.Config{
		ClientID:               dex.CLIClientID,
		DeviceAuthorizationURL: dex.Issuer + "/device/code",
		TokenURL:               dex.Issuer + "/token",
	}, store, "test-slot", out)
	if err != nil {
		t.Fatalf("runLogin: %v", err)
	}

	<-approvedCh

	// Bearer-call /api/v1/me-bearer
	tok, _ := store.Get("test-slot")
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/me-bearer", nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("me-bearer: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["sub"] != dex.StaticUser.Sub {
		t.Fatalf("sub = %q, want %q", got["sub"], dex.StaticUser.Sub)
	}
}

// runLoginShim and approveDexDeviceCode live in companion files alongside
// driveDexLogin. Implementation pattern follows that file's HTTP-driver
// approach: POST to dex's /device/code, then POST to dex's /device approval
// page with the static user's credentials.
func runLoginShim(ctx context.Context, cfg oidcclient.Config, store *keyringadapter.Fake, slot string, out *bytes.Buffer) error {
	df := oidcclient.NewDeviceFlow(cfg)
	codes, err := df.Init(ctx)
	if err != nil {
		return err
	}
	out.WriteString("user_code: " + codes.UserCode + "\n")
	tok, err := df.PollForToken(ctx, codes)
	if err != nil {
		return err
	}
	return store.Put(slot, tok)
}

func approveDexDeviceCode(t *testing.T, dex *oidctest.Instance) {
	// Implementation note: dex 2.41 supports /device/auth which presents the
	// approval form. Real implementation parses the form for a CSRF token
	// and submits credentials + approve=yes. Skipped here for brevity — see
	// Task 19 review note in the spec backlog.
	t.Helper()
	t.Log("TODO: implement dex device-code approval helper")
}
```

Note: As stated in the test comment, the **dex device-code approval helper** is the hardest part of this milestone. If implementing this fully is too brittle, an acceptable alternative is to use ROPC for both the CLI and browser flows in tests (assume dex with `enablePasswordDB: true` and `--enable-password-grant`), accepting that production-grade device-flow is exercised only manually until Authentik becomes the test IdP.

- [ ] **Step 2: Run and accept manual-test fallback for the device-code approval step**

If the helper can't be implemented in this milestone, mark with `t.Skip("device-code approval against dex requires further work — see plan note")` and run a manual smoke test:

```bash
# Terminal 1: start flow-server with Authentik config
FLOW_OIDC_ISSUER=https://auth.example.com/realms/flow \
FLOW_OIDC_CLIENT_ID=flow-server \
FLOW_OIDC_CLIENT_SECRET=... \
FLOW_ALLOWED_SUBS=YOUR_AUTHENTIK_SUB \
FLOW_COOKIE_HASH_KEY=$(openssl rand -hex 32) \
FLOW_COOKIE_BLOCK_KEY=$(openssl rand -hex 16) \
FLOW_SERVER_BASE_URL=http://localhost:8080 \
./flow-server

# Terminal 2:
./flow login
# → opens browser to Authentik, type user-code, approve
./flow whoami
# expected: sub/email/name of the logged-in user
```

- [ ] **Step 3: Commit (incl. skipped test as a tracked TODO)**

```bash
git add internal/adapter/httpserver/integration_e2e_test.go
git commit -m "test(server): E2E integration scaffold (dex approval helper TODO)"
```

---

### Task 20: Update Makefile

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Inspect existing**

```bash
cat Makefile
```

- [ ] **Step 2: Add targets**

Append (style-match existing entries):

```makefile
.PHONY: build-server test-server test-integration dex-up dex-down

build-server:
	go build -o bin/flow-server ./cmd/flow-server

test-server:
	go test ./internal/adapter/httpserver/... ./internal/adapter/oidcserver/... \
	        ./internal/adapter/oidcclient/... ./internal/adapter/keyringadapter/...

test-integration:
	go test -tags integration -count=1 \
	        ./internal/adapter/httpserver/... ./internal/adapter/oidcserver/...

dex-up:
	cd deploy/podman && podman-compose up -d dex

dex-down:
	cd deploy/podman && podman-compose down dex
```

- [ ] **Step 3: Verify**

```bash
make build-server
make test-server
```

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "chore: Makefile targets for flow-server build + integration tests"
```

---

### Task 21: Dockerfile + docker-compose for dev

**Files:**
- Create: `deploy/podman/Dockerfile.server`
- Create: `deploy/podman/docker-compose.yml`
- Create: `deploy/podman/.env.example`
- Create: `deploy/podman/dex-config.yaml`

- [ ] **Step 1: Write Dockerfile.server**

`deploy/podman/Dockerfile.server`:

```dockerfile
# Multi-stage: build with full Go toolchain, run on distroless.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/flow-server ./cmd/flow-server

FROM gcr.io/distroless/static:nonroot
USER nonroot:nonroot
COPY --from=build /out/flow-server /flow-server
EXPOSE 8080
ENTRYPOINT ["/flow-server"]
```

- [ ] **Step 2: Write docker-compose.yml**

`deploy/podman/docker-compose.yml`:

```yaml
version: "3.9"

services:
  dex:
    image: ghcr.io/dexidp/dex:v2.41.1
    command: ["dex", "serve", "/etc/dex/config.yaml"]
    volumes:
      - ./dex-config.yaml:/etc/dex/config.yaml:Z
    ports:
      - "5556:5556"

  flow-server:
    build:
      context: ../..
      dockerfile: deploy/podman/Dockerfile.server
    env_file: .env
    depends_on:
      - dex
    ports:
      - "8080:8080"
    healthcheck:
      test: ["CMD", "/flow-server", "--healthcheck"]
      interval: 30s
      timeout: 3s
      retries: 3
```

Note: The healthcheck command above is a placeholder — `flow-server` doesn't yet support `--healthcheck`. Either omit the healthcheck block in M1 or add a sidecar `wget --spider`. Simplest:

```yaml
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://localhost:8080/healthz || exit 1"]
      interval: 30s
```

(distroless doesn't have wget. Use `nicolaka/netshoot` or a build-stage alpine target as the healthcheck container if needed. For M1, omitting healthcheck is fine.)

- [ ] **Step 3: Write .env.example**

`deploy/podman/.env.example`:

```bash
# flow-server config for local Podman/Compose dev.
# Copy this to .env and fill in values.

FLOW_SERVER_ADDR=:8080
FLOW_SERVER_BASE_URL=http://localhost:8080

FLOW_OIDC_ISSUER=http://dex:5556
FLOW_OIDC_CLIENT_ID=flow-server
FLOW_OIDC_CLIENT_SECRET=flow-server-secret

# 32 bytes hex (`openssl rand -hex 32`)
FLOW_COOKIE_HASH_KEY=
# 16 bytes hex (`openssl rand -hex 16`)
FLOW_COOKIE_BLOCK_KEY=

# OIDC sub of the user allowed to use this instance.
# In dex (alice@example.com), this is "alice-static-uid".
FLOW_ALLOWED_SUBS=alice-static-uid
```

- [ ] **Step 4: Copy dex-config from testutil**

`deploy/podman/dex-config.yaml`:

(Same content as `internal/testutil/oidctest/dex-config.yaml` but with concrete issuer URL `http://localhost:5556` — see existing file in Task 6.)

- [ ] **Step 5: Smoke test**

```bash
cd deploy/podman
cp .env.example .env
# Fill cookie keys:
sed -i.bak "s|FLOW_COOKIE_HASH_KEY=|FLOW_COOKIE_HASH_KEY=$(openssl rand -hex 32)|" .env
sed -i.bak "s|FLOW_COOKIE_BLOCK_KEY=|FLOW_COOKIE_BLOCK_KEY=$(openssl rand -hex 16)|" .env
rm .env.bak
podman-compose up -d
sleep 5
curl -fsS http://localhost:8080/healthz
podman-compose logs --tail=20 flow-server
podman-compose down
```

Expected: `ok` from healthz, no errors in logs.

- [ ] **Step 6: Commit**

```bash
git add deploy/podman
git commit -m "deploy: Podman compose + Dockerfile for flow-server + dex"
```

---

### Task 22: Update CLAUDE-activeContext.md

**Files:**
- Modify: `CLAUDE-activeContext.md`

- [ ] **Step 1: Prepend new section**

Open `CLAUDE-activeContext.md`. Add a new top-level section dated 2026-06-XX (date of completion):

```markdown
# Active Context — Stand 2026-06-XX (Phase-1 M1 abgeschlossen)

## Phase-1 M1: flow-server + OIDC End-to-End

Erstes Lieferdatum des Multi-Device-Sync-Umbaus aus dem Spec
`docs/superpowers/specs/2026-06-02-flow-client-server-phase1-design.md`
(Commit `44b6a40`).

Implementiert über Plan `docs/superpowers/plans/2026-06-02-flow-phase1-m1-server-skeleton-oidc.md`,
22 Tasks.

**Was funktioniert:**

- `flow-server` läuft als Container (Podman + Dockerfile distroless) mit /healthz und /readyz
- OIDC-Auth-Code-Flow im Browser (login/callback/logout) gegen Authentik / dex
- OIDC-Device-Flow per `flow login` mit Token-Persistenz im OS-Keychain
- `flow logout`, `flow whoami` zur Verifikation
- JWT-Bearer-Auth auf `/api/v1/me-bearer`, Cookie-Auth auf `/api/v1/me`
- Allowlist auf OIDC-`sub` (Phase 1 = ein User)
- `/api/v1/oidc/config` für Client-Endpoint-Discovery

**Noch nicht:**

- Keine Daten-Operationen (Sessions, Notes, Repos, Projekte) — kommt M2
- Keine SQLite-Anbindung server-side — kommt M2/M3
- Kein WebUI-HTML — kommt M6/M7
- MCP-Server — kommt M5
- Production-Deploy (K8s/Helm) — kommt M8
- Dex-Device-Code-Approval-Test-Helper offen (siehe Plan §Task 19)

**Nächste Schritte:** Plan B (M2-M3 — Domain-Erweiterung + Sessions-Sync) brainstormen
und schreiben.
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE-activeContext.md
git commit -m "docs(active-context): M1 abgeschlossen — Server-Skeleton + OIDC End-to-End"
```

---

## Self-Review Notes (already applied)

Reviewed against spec `2026-06-02-flow-client-server-phase1-design.md`:

- **§ Authentication / OIDC for browser:** covered by Task 10 (browser flow) + Task 11 (cookie middleware).
- **§ Authentication / OIDC for CLI/TUI/MCP:** covered by Tasks 13–18 (device flow + tokens + CLI commands).
- **§ Authentication / Token-Storage in Keychain:** Task 5.
- **§ Authentication / JWT-Verifikation server-side, JWKS-Cache:** Task 7.
- **§ Authentication / Allowlist:** Task 8.
- **§ Authentication / OIDC Discovery for CLI:** Task 17.
- **§ Server-Implementierung / Chi router, slog, healthz/readyz, embedded assets stub:** Tasks 1–3.
- **§ Deployment / Podman docker-compose for dev, distroless Dockerfile:** Task 21.
- **§ Layout / new cmd/flow-server, new adapters under internal/adapter/:** all of Tasks 1–18.

**Out of scope for M1, deferred to subsequent plans:**

- domain User/Project/Repo/RepoNote (Plan B / M2)
- sqliteclient + sqliteserver (Plan B / M2-M3)
- httpsync + Lamport-watermarks (Plan B / M3)
- ActiveSession server-authoritative (Plan B / M3)
- flow-mcp stdio server (Plan D / M5)
- WebUI Templ + HTMX + Tailwind (Plan E / M6-M7)
- Helm chart, Litestream, metrics (Plan F / M8-M9)

**Known TODO** within this plan:

- Task 19's dex device-code approval helper. If brittle to implement, document a manual-smoke fallback against real Authentik. This is explicitly an OK trade-off for a hobby project.

---

## Done Criteria for M1

A merged Plan A delivers:

1. `make test-server` green
2. `make build-server` produces `bin/flow-server`
3. `make test-integration` runs the dex-backed browser-flow test green (device-flow E2E may be manual against Authentik)
4. `cd deploy/podman && podman-compose up && curl /healthz` returns 200
5. `flow login --server=...` against a real Authentik succeeds and stores token in Keychain
6. `flow whoami --server=...` prints the authenticated user's sub/email/name
7. `CLAUDE-activeContext.md` updated to reflect M1 status
8. PR title: `feat: Phase-1 M1 — flow-server skeleton + OIDC end-to-end`
