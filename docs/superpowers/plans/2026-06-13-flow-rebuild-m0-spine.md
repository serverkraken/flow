# flow Rebuild — M0 Spine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the server-authoritative spine of the new flow: a `flow-server` that verifies Authentik JWTs, persists users in Postgres, and pushes live events over SSE — proven end-to-end (token → `/api/v1/me` → live event) plus a thin `flow whoami` client.

**Architecture:** Hexagonal Go. Pure `domain` + `usecase` depend only on `ports` (interfaces); `adapter/*` implements them (pgstore, oidc, sse, httpserver, apiclient). One composition root (`cmd/flow-server`) wires everything; `cmd/flow` is the client. Server is the single source of truth; clients are thin.

**Tech Stack:** Go 1.25, `pgx/v5` + `pgvector/pgvector:pg16` (Postgres), `pressly/goose/v3` (migrations), `coreos/go-oidc/v3` (JWT verify), std `net/http` (1.22 ServeMux + SSE), `google/uuid`, `testcontainers-go` (DB tests), `spf13/cobra` (CLI).

**Module:** `github.com/serverkraken/flow` · **Branch:** `rebuild` (orphan, green field) · **Worktree:** `/Users/msoent/SourceCode/serverkraken/flow-rebuild`

## Execution Setup (read this first)

**Run subagent-driven with Sonnet.** In a fresh session, from repo root `/Users/msoent/SourceCode/serverkraken/flow`, kick off with:

> *"Execute `docs/superpowers/plans/2026-06-13-flow-rebuild-m0-spine.md` with superpowers:subagent-driven-development — Sonnet task subagents, two-stage review between tasks."*

- **Task 1 creates the orphan worktree** `/Users/msoent/SourceCode/serverkraken/flow-rebuild`. **Every later task operates inside that worktree** — dispatch each subagent with absolute paths under `flow-rebuild/`. Do **NOT** use the Agent `isolation: worktree` option (we want the real `rebuild` branch, not a throwaway).
- One commit per task, exactly as the steps specify. Coverage gate starts at 80% for M0 (raised in later milestones).
- **Machine prereqs:** Docker/Podman (testcontainers + `make db-up`), Go 1.25, `golangci-lint`, `gofumpt`, `goimports`. For the final manual loop proof: a real Authentik access token for an allowlisted `sub`.
- **Model:** Sonnet for task subagents (set by the orchestrator); review/orchestration in the main session.
- **Effort:** set the orchestrator session to `/effort high` — its job is dispatch + the two-stage review that catches subagent slips. `/effort` is a per-session knob (it resets each session, so set it yourself); it does not apply per-subagent. Task subagents run as Sonnet at default effort — the plan is fully specified, so that is the intended quality/cost point.

**Scope decisions for M0 (resolving open points from the spec):**
- **Migrations tool:** `goose` (embedded SQL, run via `pgx/v5/stdlib` `*sql.DB`; app queries via `pgxpool`).
- **OIDC in M0:** server-side **token verification** only (real adapter, tested against an httptest JWKS). Interactive **device-flow login UX is deferred to M1** — M0's "login" is a verified bearer token (obtained out-of-band / via the smoke script), which proves the trust + API + SSE loop.
- **Live-event demo:** a dev-only `POST /api/v1/debug/ping` (enabled with `FLOW_DEV=1`) publishes a `ping` event to the caller's SSE stream — lets M0 demonstrate the loop before worktime exists. Removed once M1 emits real events.
- **Client in M0:** minimal `apiclient` + `flow whoami` (bearer from env). SSE client + TUI start in M1.

---

## File Structure

```
cmd/flow-server/main.go            composition root: config → adapters → usecases → http; graceful shutdown
cmd/flow/main.go                   cobra root for the client
cmd/flow/whoami.go                 `flow whoami` verb
internal/config/config.go          env config (DSN, OIDC issuer/clientID, allowlist, listen addr, dev flag)
internal/domain/user.go            User value type + NewUser
internal/domain/event.go           Event + EventType
internal/domain/errors.go          shared sentinel errors
internal/ports/ports.go            Clock, IDGen, UserStore, TokenVerifier, EventBus, Identity
internal/usecase/ensure_user.go    EnsureUser (allowlist + upsert-on-first-login)
internal/adapter/systemclock/clock.go     real Clock
internal/adapter/uuidgen/uuidgen.go       real IDGen (google/uuid)
internal/adapter/pgstore/pool.go          pgxpool constructor + Migrate()
internal/adapter/pgstore/migrations/0001_users.sql
internal/adapter/pgstore/users.go         UserStore (Postgres)
internal/adapter/oidcverify/verifier.go   TokenVerifier (go-oidc)
internal/adapter/sse/bus.go               EventBus (in-memory per-user pub/sub)
internal/adapter/httpserver/server.go     router + Server struct
internal/adapter/httpserver/middleware.go auth middleware (bearer → Identity → ctx)
internal/adapter/httpserver/handlers.go   /healthz, /api/v1/me, /api/v1/events, debug ping
internal/adapter/apiclient/client.go      client-side REST calls (Whoami)
internal/testutil/fakes.go                fake Clock/IDGen/UserStore/TokenVerifier/EventBus
deploy/docker-compose.yml          local postgres (pgvector image)
Makefile · .gitignore · go.mod
scripts/coverage-gate.sh · scripts/smoke-m0.sh
```

Principle: one responsibility per file; `cmd/*` is wiring only (excluded from the coverage gate, like v1).

---

## Task 1: Scaffold the orphan worktree & module

**Files:**
- Create: worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`, `go.mod`, `Makefile`, `.gitignore`, `deploy/docker-compose.yml`, `scripts/coverage-gate.sh`, package `doc.go` stubs.

- [ ] **Step 1: Create the orphan worktree (green field)**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow
git worktree list                       # confirm existing worktrees first (per CLAUDE.md)
# git 2.42+: one shot. Empty tree, no history.
git worktree add --orphan -b rebuild /Users/msoent/SourceCode/serverkraken/flow-rebuild
# Fallback for older git:
#   git worktree add --detach /Users/msoent/SourceCode/serverkraken/flow-rebuild
#   cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && git checkout --orphan rebuild && git rm -rf . 2>/dev/null || true
```

Expected: `/Users/msoent/SourceCode/serverkraken/flow-rebuild` exists and is empty (only `.git` linkage). **All remaining tasks run inside this worktree.**

- [ ] **Step 2: Init module + base files**

`go.mod`:
```
module github.com/serverkraken/flow

go 1.25.0
```
Run: `go mod init github.com/serverkraken/flow` (then edit `go 1.25.0`).

`.gitignore`:
```
/bin/
/dist/
coverage.out
coverage.html
.env
.DS_Store
# Claude memory-bank files (never committed)
CLAUDE.md
CLAUDE-*.md
```

`deploy/docker-compose.yml`:
```yaml
services:
  db:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_DB: flow
      POSTGRES_USER: flow
      POSTGRES_PASSWORD: flow
    ports: ["5432:5432"]
    volumes: ["flow_pgdata:/var/lib/postgresql/data"]
volumes:
  flow_pgdata:
```

`Makefile` (mirrors v1 conventions; gate starts at 80 for M0, raised later):
```make
BIN             := flow-server
PKG             := ./cmd/flow-server
COVER_OUT       := coverage.out
COVER_THRESHOLD := 80
COVER_PKG       := ./internal/...

.PHONY: build test cover lint fmt ci db-up db-down smoke
build:
	@mkdir -p bin
	go build -o bin/flow-server ./cmd/flow-server
	go build -o bin/flow ./cmd/flow
test:
	go test -race ./...
cover:
	go test -covermode=atomic -coverprofile=$(COVER_OUT) -coverpkg=$(COVER_PKG) ./...
	@./scripts/coverage-gate.sh $(COVER_OUT) $(COVER_THRESHOLD)
lint:
	golangci-lint run
fmt:
	gofumpt -w . && goimports -w .
db-up:
	docker compose -f deploy/docker-compose.yml up -d
db-down:
	docker compose -f deploy/docker-compose.yml down
smoke:
	./scripts/smoke-m0.sh
ci: lint cover build
```

`scripts/coverage-gate.sh` (copy v1's; minimal version):
```bash
#!/usr/bin/env bash
set -euo pipefail
out="$1"; threshold="$2"
pct=$(go tool cover -func="$out" | awk '/^total:/ {gsub(/%/,"",$3); print $3}')
echo "coverage: ${pct}% (threshold ${threshold}%)"
awk -v p="$pct" -v t="$threshold" 'BEGIN { exit (p+0 >= t+0) ? 0 : 1 }' \
  || { echo "FAIL: coverage ${pct}% < ${threshold}%"; exit 1; }
```
Run: `chmod +x scripts/coverage-gate.sh`

- [ ] **Step 3: Verify it builds (empty module)**

Run: `go build ./... && echo OK`
Expected: `OK` (nothing to build yet, exit 0).

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore: scaffold rebuild orphan branch — module, Makefile, compose"
```

---

## Task 2: Config

**Files:**
- Create: `internal/config/config.go`, `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
package config

import "testing"

func TestLoadFromEnv(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":      "postgres://flow:flow@localhost:5432/flow?sslmode=disable",
		"FLOW_OIDC_ISSUER":  "https://id.thebackend.org/application/o/flow/",
		"FLOW_OIDC_CLIENT_ID": "flow",
		"FLOW_ALLOWED_SUBS": "msoent, alice",
		"FLOW_LISTEN_ADDR":  ":8080",
		"FLOW_DEV":          "1",
	}
	c, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DatabaseURL == "" || c.OIDCIssuer == "" {
		t.Fatal("required fields empty")
	}
	if len(c.AllowedSubs) != 2 || !c.AllowedSubs["alice"] {
		t.Fatalf("allowlist parse: %v", c.AllowedSubs)
	}
	if !c.Dev {
		t.Fatal("dev flag not parsed")
	}
}

func TestLoadMissingRequired(t *testing.T) {
	if _, err := Load(func(string) string { return "" }); err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** · `go test ./internal/config/...` → undefined: Load

- [ ] **Step 3: Implement**

```go
// Package config loads flow-server configuration from the environment.
package config

import (
	"fmt"
	"strings"
)

type Config struct {
	DatabaseURL  string
	OIDCIssuer   string
	OIDCClientID string
	AllowedSubs  map[string]bool
	ListenAddr   string
	Dev          bool
}

// Load reads config via getenv (injected for testability).
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		DatabaseURL:  getenv("DATABASE_URL"),
		OIDCIssuer:   getenv("FLOW_OIDC_ISSUER"),
		OIDCClientID: getenv("FLOW_OIDC_CLIENT_ID"),
		ListenAddr:   getenv("FLOW_LISTEN_ADDR"),
		Dev:          getenv("FLOW_DEV") == "1",
		AllowedSubs:  map[string]bool{},
	}
	for _, s := range strings.Split(getenv("FLOW_ALLOWED_SUBS"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			c.AllowedSubs[s] = true
		}
	}
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	for k, v := range map[string]string{"DATABASE_URL": c.DatabaseURL, "FLOW_OIDC_ISSUER": c.OIDCIssuer, "FLOW_OIDC_CLIENT_ID": c.OIDCClientID} {
		if v == "" {
			return Config{}, fmt.Errorf("config: %s is required", k)
		}
	}
	return c, nil
}
```

- [ ] **Step 4: Run — expect PASS** · `go test ./internal/config/...`
- [ ] **Step 5: Commit** · `git add -A && git commit -m "feat(config): env loader with allowlist + dev flag"`

---

## Task 3: Domain — User

**Files:**
- Create: `internal/domain/user.go`, `internal/domain/errors.go`, `internal/domain/user_test.go`

- [ ] **Step 1: Write the failing test**

```go
package domain

import "testing"

func TestNewUserValidates(t *testing.T) {
	if _, err := NewUser("", "sub", "u", "e", "n"); err == nil {
		t.Fatal("expected error: empty id")
	}
	if _, err := NewUser("id", "", "u", "e", "n"); err == nil {
		t.Fatal("expected error: empty sub")
	}
	u, err := NewUser("id-1", "sub-1", "msoent", "m@x.de", "Martin")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if u.OIDCSub != "sub-1" || u.Username != "msoent" {
		t.Fatalf("fields not set: %+v", u)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** · `go test ./internal/domain/...`

- [ ] **Step 3: Implement**

`internal/domain/errors.go`:
```go
// Package domain holds the pure value types and rules of flow.
// It must not import any I/O package.
package domain

import "errors"

var ErrInvalidUser = errors.New("invalid user")
```

`internal/domain/user.go`:
```go
package domain

import "fmt"

// User is an authenticated flow user, keyed by the Authentik OIDC subject.
type User struct {
	ID          string `json:"id"`
	OIDCSub     string `json:"-"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

func NewUser(id, sub, username, email, displayName string) (User, error) {
	if id == "" {
		return User{}, fmt.Errorf("%w: id required", ErrInvalidUser)
	}
	if sub == "" {
		return User{}, fmt.Errorf("%w: oidc sub required", ErrInvalidUser)
	}
	return User{ID: id, OIDCSub: sub, Username: username, Email: email, DisplayName: displayName}, nil
}
```

- [ ] **Step 4: Run — expect PASS** · `go test ./internal/domain/...`
- [ ] **Step 5: Commit** · `git add -A && git commit -m "feat(domain): User value type"`

---

## Task 4: Domain Event + Ports

**Files:**
- Create: `internal/domain/event.go`, `internal/ports/ports.go`

These are interfaces/types exercised by later tasks (no standalone test; `go build` is the check).

- [ ] **Step 1: Implement `internal/domain/event.go`**

```go
package domain

// EventType identifies a live event pushed to clients over SSE.
type EventType string

const (
	EventPing EventType = "ping" // dev-only loop proof; real events arrive in M1+
)

// Event is a server-originated change notification. UserID is the routing
// key and is never serialized to the client.
type Event struct {
	Type   EventType      `json:"type"`
	UserID string         `json:"-"`
	Data   map[string]any `json:"data,omitempty"`
}
```

- [ ] **Step 2: Implement `internal/ports/ports.go`**

```go
// Package ports declares the interfaces the application core depends on.
package ports

import (
	"context"
	"errors"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

var ErrUserNotFound = errors.New("user not found")

type Clock interface{ Now() time.Time }

type IDGen interface{ NewID() string }

type UserStore interface {
	UpsertBySub(ctx context.Context, u domain.User) (domain.User, error)
	GetBySub(ctx context.Context, sub string) (domain.User, error)
}

// Identity is the verified result of a bearer token.
type Identity struct {
	Subject  string
	Username string
	Email    string
	Name     string
	Groups   []string
}

type TokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (Identity, error)
}

// EventBus is in-process pub/sub for live events. Subscribe returns a channel
// and a cancel func that unsubscribes and drains.
type EventBus interface {
	Publish(ev domain.Event)
	Subscribe(userID string) (events <-chan domain.Event, cancel func())
}
```

- [ ] **Step 3: Run — expect build OK** · `go build ./...`
- [ ] **Step 4: Commit** · `git add -A && git commit -m "feat(domain,ports): Event + core port interfaces"`

---

## Task 5: testutil fakes

**Files:**
- Create: `internal/testutil/fakes.go`, `internal/testutil/fakes_test.go`

- [ ] **Step 1: Implement fakes**

```go
// Package testutil provides in-memory fakes for the ports, for use in tests.
package testutil

import (
	"context"
	"sync"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type FakeClock struct{ T time.Time }

func (c FakeClock) Now() time.Time { return c.T }

type FakeIDGen struct {
	mu sync.Mutex
	n  int
}

func (g *FakeIDGen) NewID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return "id-" + itoa(g.n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

type FakeUserStore struct {
	mu    sync.Mutex
	bySub map[string]domain.User
}

func NewFakeUserStore() *FakeUserStore { return &FakeUserStore{bySub: map[string]domain.User{}} }

func (s *FakeUserStore) UpsertBySub(_ context.Context, u domain.User) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bySub[u.OIDCSub] = u
	return u, nil
}

func (s *FakeUserStore) GetBySub(_ context.Context, sub string) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.bySub[sub]
	if !ok {
		return domain.User{}, ports.ErrUserNotFound
	}
	return u, nil
}

type FakeVerifier struct {
	ID  ports.Identity
	Err error
}

func (v FakeVerifier) Verify(context.Context, string) (ports.Identity, error) {
	return v.ID, v.Err
}
```

- [ ] **Step 2: Self-test the fakes**

```go
package testutil

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestFakeUserStoreRoundTrip(t *testing.T) {
	s := NewFakeUserStore()
	if _, err := s.GetBySub(context.Background(), "x"); !errors.Is(err, ports.ErrUserNotFound) {
		t.Fatalf("want ErrUserNotFound, got %v", err)
	}
	u, _ := domain.NewUser("id-1", "x", "u", "e", "n")
	if _, err := s.UpsertBySub(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetBySub(context.Background(), "x")
	if err != nil || got.ID != "id-1" {
		t.Fatalf("round trip failed: %+v %v", got, err)
	}
}

func TestFakeIDGenMonotonic(t *testing.T) {
	g := &FakeIDGen{}
	if g.NewID() == g.NewID() {
		t.Fatal("ids should differ")
	}
}
```

- [ ] **Step 3: Run — expect PASS** · `go test ./internal/testutil/...`
- [ ] **Step 4: Commit** · `git add -A && git commit -m "test(testutil): in-memory port fakes + self-tests"`

---

## Task 6: Real Clock + IDGen adapters

**Files:**
- Create: `internal/adapter/systemclock/clock.go`, `internal/adapter/uuidgen/uuidgen.go`, `internal/adapter/uuidgen/uuidgen_test.go`

- [ ] **Step 1: Add dependency** · `go get github.com/google/uuid@v1.6.0`

- [ ] **Step 2: Implement**

`internal/adapter/systemclock/clock.go`:
```go
// Package systemclock is the production ports.Clock.
package systemclock

import "time"

type Clock struct{}

func (Clock) Now() time.Time { return time.Now() }
```

`internal/adapter/uuidgen/uuidgen.go`:
```go
// Package uuidgen is the production ports.IDGen (UUIDv4).
package uuidgen

import "github.com/google/uuid"

type Gen struct{}

func (Gen) NewID() string { return uuid.NewString() }
```

- [ ] **Step 3: Test**

```go
package uuidgen

import "testing"

func TestNewIDUnique(t *testing.T) {
	g := Gen{}
	a, b := g.NewID(), g.NewID()
	if a == "" || a == b {
		t.Fatalf("bad ids: %q %q", a, b)
	}
}
```

- [ ] **Step 4: Run — expect PASS** · `go test ./internal/adapter/uuidgen/...`
- [ ] **Step 5: Commit** · `git add -A && git commit -m "feat(adapter): systemclock + uuidgen"`

---

## Task 7: pgstore — migrations + UserStore (testcontainers)

**Files:**
- Create: `internal/adapter/pgstore/pool.go`, `internal/adapter/pgstore/migrations/0001_users.sql`, `internal/adapter/pgstore/users.go`, `internal/adapter/pgstore/users_test.go`

- [ ] **Step 1: Add deps**

```bash
go get github.com/jackc/pgx/v5@latest
go get github.com/pressly/goose/v3@latest
go get github.com/testcontainers/testcontainers-go@latest
go get github.com/testcontainers/testcontainers-go/modules/postgres@latest
```

- [ ] **Step 2: Migration `0001_users.sql`**

```sql
-- +goose Up
CREATE TABLE users (
    id           TEXT PRIMARY KEY,
    oidc_sub     TEXT NOT NULL UNIQUE,
    username     TEXT NOT NULL DEFAULT '',
    email        TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE users;
```

- [ ] **Step 3: Implement `pool.go` (pool + Migrate)**

```go
// Package pgstore implements the Postgres-backed stores.
package pgstore

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgstore: pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgstore: ping: %w", err)
	}
	return pool, nil
}

// Migrate runs all up migrations using a stdlib *sql.DB derived from the pool's config.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("pgstore: dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("pgstore: migrate: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Implement `users.go`**

```go
package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type UserStore struct{ pool *pgxpool.Pool }

func NewUserStore(pool *pgxpool.Pool) *UserStore { return &UserStore{pool: pool} }

func (s *UserStore) UpsertBySub(ctx context.Context, u domain.User) (domain.User, error) {
	const q = `
INSERT INTO users (id, oidc_sub, username, email, display_name)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (oidc_sub) DO UPDATE
  SET username=EXCLUDED.username, email=EXCLUDED.email, display_name=EXCLUDED.display_name
RETURNING id, oidc_sub, username, email, display_name`
	var out domain.User
	err := s.pool.QueryRow(ctx, q, u.ID, u.OIDCSub, u.Username, u.Email, u.DisplayName).
		Scan(&out.ID, &out.OIDCSub, &out.Username, &out.Email, &out.DisplayName)
	if err != nil {
		return domain.User{}, fmt.Errorf("pgstore: upsert user: %w", err)
	}
	return out, nil
}

func (s *UserStore) GetBySub(ctx context.Context, sub string) (domain.User, error) {
	const q = `SELECT id, oidc_sub, username, email, display_name FROM users WHERE oidc_sub=$1`
	var out domain.User
	err := s.pool.QueryRow(ctx, q, sub).Scan(&out.ID, &out.OIDCSub, &out.Username, &out.Email, &out.DisplayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ports.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("pgstore: get user: %w", err)
	}
	return out, nil
}
```

- [ ] **Step 5: Integration test (testcontainers)**

```go
package pgstore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startPG(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	c, err := tcpg.Run(ctx, "pgvector/pgvector:pg16",
		tcpg.WithDatabase("flow_test"), tcpg.WithUsername("flow"), tcpg.WithPassword("flow"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })
	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	return dsn
}

func TestUserStoreUpsertGet(t *testing.T) {
	ctx := context.Background()
	dsn := startPG(t)
	pool, err := pgstore.NewPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	store := pgstore.NewUserStore(pool)

	if _, err := store.GetBySub(ctx, "nope"); !errors.Is(err, ports.ErrUserNotFound) {
		t.Fatalf("want ErrUserNotFound, got %v", err)
	}
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "Martin")
	if _, err := store.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	// upsert again with changed profile keeps id, updates fields
	u2, _ := domain.NewUser("u-other", "sub-1", "msoent", "new@x.de", "Martin S")
	got, err := store.UpsertBySub(ctx, u2)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "u1" {
		t.Fatalf("upsert must keep original id, got %q", got.ID)
	}
	if got.Email != "new@x.de" {
		t.Fatalf("email not updated: %q", got.Email)
	}
}
```

- [ ] **Step 6: Tidy + run** · `go mod tidy && go test ./internal/adapter/pgstore/...`
  Expected: PASS (or SKIP if Docker is unavailable — acceptable locally; CI has Docker).

- [ ] **Step 7: Commit** · `git add -A && git commit -m "feat(pgstore): users table + UserStore with goose migrations"`

---

## Task 8: OIDC token verifier

**Files:**
- Create: `internal/adapter/oidcverify/verifier.go`, `internal/adapter/oidcverify/verifier_test.go`

- [ ] **Step 1: Add dep** · `go get github.com/coreos/go-oidc/v3/oidc@latest`

- [ ] **Step 2: Implement**

```go
// Package oidcverify verifies Authentik-issued JWT access/ID tokens.
package oidcverify

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/serverkraken/flow/internal/ports"
)

type Verifier struct{ v *oidc.IDTokenVerifier }

// New builds a verifier from the issuer's discovery document.
func New(ctx context.Context, issuer, clientID string) (*Verifier, error) {
	p, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidcverify: provider: %w", err)
	}
	return &Verifier{v: p.Verifier(&oidc.Config{ClientID: clientID})}, nil
}

type claims struct {
	Sub               string   `json:"sub"`
	PreferredUsername string   `json:"preferred_username"`
	Email             string   `json:"email"`
	Name              string   `json:"name"`
	Groups            []string `json:"groups"`
}

func (vr *Verifier) Verify(ctx context.Context, raw string) (ports.Identity, error) {
	tok, err := vr.v.Verify(ctx, raw)
	if err != nil {
		return ports.Identity{}, fmt.Errorf("oidcverify: verify: %w", err)
	}
	var c claims
	if err := tok.Claims(&c); err != nil {
		return ports.Identity{}, fmt.Errorf("oidcverify: claims: %w", err)
	}
	return ports.Identity{Subject: c.Sub, Username: c.PreferredUsername, Email: c.Email, Name: c.Name, Groups: c.Groups}, nil
}
```

- [ ] **Step 3: Test against an httptest OIDC server (no live Authentik)**

```go
package oidcverify_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/serverkraken/flow/internal/adapter/oidcverify"
)

func TestVerifyValidToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	const kid = "test-key"
	var issuer string
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	issuer = srv.URL

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": issuer, "jwks_uri": issuer + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{
			{"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig", "n": n, "e": e},
		}})
	})

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": issuer, "aud": "flow", "sub": "msoent",
		"preferred_username": "msoent", "email": "m@x.de", "name": "Martin",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
	})
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	ctx := oidc.InsecureIssuerURLContext(context.Background(), issuer)
	v, err := oidcverify.New(ctx, issuer, "flow")
	if err != nil {
		t.Fatal(err)
	}
	id, err := v.Verify(context.Background(), signed)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if id.Subject != "msoent" || id.Username != "msoent" {
		t.Fatalf("identity mismatch: %+v", id)
	}

	// And a garbage token must be rejected by the same verifier.
	if _, err := v.Verify(context.Background(), "not.a.valid.jwt"); err == nil {
		t.Fatal("expected error for garbage token")
	}
}
```

> **Implementer note:** add the test-only signer dep: `go get github.com/golang-jwt/jwt/v5@latest`.

- [ ] **Step 4: Run — expect PASS** · `go test ./internal/adapter/oidcverify/...`
- [ ] **Step 5: Commit** · `git add -A && git commit -m "feat(oidcverify): JWT verification against issuer JWKS"`

---

## Task 9: SSE EventBus

**Files:**
- Create: `internal/adapter/sse/bus.go`, `internal/adapter/sse/bus_test.go`

- [ ] **Step 1: Write the failing test**

```go
package sse_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
)

func TestBusDeliversToSubscriberOfSameUser(t *testing.T) {
	b := sse.NewBus()
	ch, cancel := b.Subscribe("user-1")
	defer cancel()

	b.Publish(domain.Event{Type: domain.EventPing, UserID: "user-1"})

	select {
	case ev := <-ch:
		if ev.Type != domain.EventPing {
			t.Fatalf("wrong event: %v", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestBusIsolatesUsers(t *testing.T) {
	b := sse.NewBus()
	ch, cancel := b.Subscribe("user-1")
	defer cancel()
	b.Publish(domain.Event{Type: domain.EventPing, UserID: "user-2"})
	select {
	case <-ch:
		t.Fatal("user-1 must not receive user-2 events")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestCancelUnsubscribes(t *testing.T) {
	b := sse.NewBus()
	ch, cancel := b.Subscribe("u")
	cancel()
	b.Publish(domain.Event{Type: domain.EventPing, UserID: "u"})
	if _, ok := <-ch; ok {
		t.Fatal("channel should be closed after cancel")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** · `go test ./internal/adapter/sse/...`

- [ ] **Step 3: Implement**

```go
// Package sse provides an in-process EventBus and SSE writing helpers.
package sse

import (
	"sync"

	"github.com/serverkraken/flow/internal/domain"
)

type subscriber struct {
	userID string
	ch     chan domain.Event
}

type Bus struct {
	mu   sync.Mutex
	subs map[*subscriber]struct{}
}

func NewBus() *Bus { return &Bus{subs: map[*subscriber]struct{}{}} }

func (b *Bus) Subscribe(userID string) (<-chan domain.Event, func()) {
	s := &subscriber{userID: userID, ch: make(chan domain.Event, 16)}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, s)
			b.mu.Unlock()
			close(s.ch)
		})
	}
	return s.ch, cancel
}

// Publish fans out to subscribers of ev.UserID. Non-blocking: a full
// subscriber buffer drops the event for that subscriber (it will full-refresh
// on reconnect) rather than stalling the publisher.
func (b *Bus) Publish(ev domain.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		if s.userID != ev.UserID {
			continue
		}
		select {
		case s.ch <- ev:
		default:
		}
	}
}
```

- [ ] **Step 4: Run — expect PASS** · `go test -race ./internal/adapter/sse/...`
- [ ] **Step 5: Commit** · `git add -A && git commit -m "feat(sse): in-memory per-user EventBus"`

---

## Task 10: usecase — EnsureUser

**Files:**
- Create: `internal/usecase/ensure_user.go`, `internal/usecase/ensure_user_test.go`

- [ ] **Step 1: Write the failing test**

```go
package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func allowMsoent(id ports.Identity) bool { return id.Subject == "msoent" }

func TestEnsureUserRejectsNonAllowlisted(t *testing.T) {
	uc := usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: &testutil.FakeIDGen{}, Allow: allowMsoent}
	if _, err := uc.Execute(context.Background(), ports.Identity{Subject: "eve"}); !errors.Is(err, usecase.ErrNotAllowed) {
		t.Fatalf("want ErrNotAllowed, got %v", err)
	}
}

func TestEnsureUserCreatesOnFirstLoginThenReturnsExisting(t *testing.T) {
	store := testutil.NewFakeUserStore()
	uc := usecase.EnsureUser{Users: store, IDs: &testutil.FakeIDGen{}, Allow: allowMsoent}
	id := ports.Identity{Subject: "msoent", Username: "msoent", Email: "m@x.de", Name: "Martin"}

	u1, err := uc.Execute(context.Background(), id)
	if err != nil || u1.ID == "" {
		t.Fatalf("first login: %+v %v", u1, err)
	}
	u2, err := uc.Execute(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if u2.ID != u1.ID {
		t.Fatalf("second login should return same user: %q vs %q", u1.ID, u2.ID)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** · `go test ./internal/usecase/...`

- [ ] **Step 3: Implement**

```go
// Package usecase holds application services. They depend only on ports.
package usecase

import (
	"context"
	"errors"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

var ErrNotAllowed = errors.New("user not in allowlist")

// EnsureUser maps a verified Identity to a stored User, creating it on first
// login. Allow gates access (Phase-1 static allowlist).
type EnsureUser struct {
	Users ports.UserStore
	IDs   ports.IDGen
	Allow func(ports.Identity) bool
}

func (uc EnsureUser) Execute(ctx context.Context, id ports.Identity) (domain.User, error) {
	if uc.Allow == nil || !uc.Allow(id) {
		return domain.User{}, ErrNotAllowed
	}
	switch u, err := uc.Users.GetBySub(ctx, id.Subject); {
	case err == nil:
		return u, nil
	case !errors.Is(err, ports.ErrUserNotFound):
		return domain.User{}, err
	}
	nu, err := domain.NewUser(uc.IDs.NewID(), id.Subject, id.Username, id.Email, id.Name)
	if err != nil {
		return domain.User{}, err
	}
	return uc.Users.UpsertBySub(ctx, nu)
}
```

- [ ] **Step 4: Run — expect PASS** · `go test ./internal/usecase/...`
- [ ] **Step 5: Commit** · `git add -A && git commit -m "feat(usecase): EnsureUser — allowlist + first-login upsert"`

---

## Task 11: httpserver — health, auth, /me, /events, debug ping

**Files:**
- Create: `internal/adapter/httpserver/server.go`, `middleware.go`, `handlers.go`, `server_test.go`

- [ ] **Step 1: Implement `server.go` (deps + routes)**

```go
// Package httpserver exposes the REST + SSE API.
package httpserver

import (
	"net/http"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

type Server struct {
	Verifier ports.TokenVerifier
	Ensure   usecase.EnsureUser
	Bus      ports.EventBus
	Dev      bool
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.Handle("GET /api/v1/me", s.auth(http.HandlerFunc(s.handleMe)))
	mux.Handle("GET /api/v1/events", s.auth(http.HandlerFunc(s.handleEvents)))
	if s.Dev {
		mux.Handle("POST /api/v1/debug/ping", s.auth(http.HandlerFunc(s.handleDebugPing)))
	}
	return mux
}
```

- [ ] **Step 2: Implement `middleware.go`**

```go
package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

type ctxKey int

const userKey ctxKey = 0

func userFrom(ctx context.Context) (domain.User, bool) {
	u, ok := ctx.Value(userKey).(domain.User)
	return u, ok
}

// auth verifies the bearer token, ensures the user, and stores it in context.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if raw == "" || raw == r.Header.Get("Authorization") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		id, err := s.Verifier.Verify(r.Context(), raw)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		u, err := s.Ensure.Execute(r.Context(), id)
		if errors.Is(err, usecase.ErrNotAllowed) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}
```

- [ ] **Step 3: Implement `handlers.go`**

```go
package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(u)
}

func (s *Server) handleDebugPing(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	s.Bus.Publish(domain.Event{Type: domain.EventPing, UserID: u.ID, Data: map[string]any{"msg": "pong"}})
	w.WriteHeader(http.StatusAccepted)
}

// handleEvents streams the user's events as SSE until the client disconnects.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	u, _ := userFrom(r.Context())
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, cancel := s.Bus.Subscribe(u.ID)
	defer cancel()
	flusher.Flush() // commit headers so clients (and tests) see 200 immediately

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			payload, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload)
			flusher.Flush()
		}
	}
}
```

- [ ] **Step 4: Write the test (health, auth reject, /me, SSE ping loop)**

```go
package httpserver_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func newServer() *httpserver.Server {
	store := testutil.NewFakeUserStore()
	return &httpserver.Server{
		Verifier: testutil.FakeVerifier{ID: ports.Identity{Subject: "msoent", Username: "msoent"}},
		Ensure:   usecase.EnsureUser{Users: store, IDs: &testutil.FakeIDGen{}, Allow: func(id ports.Identity) bool { return id.Subject == "msoent" }},
		Bus:      sse.NewBus(),
		Dev:      true,
	}
}

func TestHealth(t *testing.T) {
	srv := httptest.NewServer(newServer().Routes())
	defer srv.Close()
	res, err := http.Get(srv.URL + "/healthz")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("health: %v status=%v", err, res.StatusCode)
	}
}

func TestMeRequiresAuth(t *testing.T) {
	srv := httptest.NewServer(newServer().Routes())
	defer srv.Close()
	res, _ := http.Get(srv.URL + "/api/v1/me")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", res.StatusCode)
	}
}

func TestMeReturnsUser(t *testing.T) {
	srv := httptest.NewServer(newServer().Routes())
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer xyz")
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("me: %v status=%v", err, res.StatusCode)
	}
	body := make([]byte, 256)
	n, _ := res.Body.Read(body)
	if !strings.Contains(string(body[:n]), `"username":"msoent"`) {
		t.Fatalf("unexpected body: %s", body[:n])
	}
}

func TestEventsStreamsDebugPing(t *testing.T) {
	srv := httptest.NewServer(newServer().Routes())
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/v1/events", nil)
	req.Header.Set("Authorization", "Bearer xyz")
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("events: %v status=%v", err, res.StatusCode)
	}
	defer res.Body.Close()

	// fire the ping after the stream is open
	go func() {
		time.Sleep(50 * time.Millisecond)
		pr, _ := http.NewRequest("POST", srv.URL+"/api/v1/debug/ping", nil)
		pr.Header.Set("Authorization", "Bearer xyz")
		_, _ = http.DefaultClient.Do(pr)
	}()

	sc := bufio.NewScanner(res.Body)
	deadline := time.Now().Add(3 * time.Second)
	for sc.Scan() {
		if strings.Contains(sc.Text(), "event: ping") {
			return // success
		}
		if time.Now().After(deadline) {
			break
		}
	}
	t.Fatal("did not receive ping event")
}
```

- [ ] **Step 5: Run — expect PASS** · `go test -race ./internal/adapter/httpserver/...`
- [ ] **Step 6: Commit** · `git add -A && git commit -m "feat(httpserver): health, bearer auth, /me, SSE /events + dev ping"`

---

## Task 12: Composition root — cmd/flow-server + smoke

**Files:**
- Create: `cmd/flow-server/main.go`, `scripts/smoke-m0.sh`

- [ ] **Step 1: Implement `main.go` (wire everything, graceful shutdown)**

```go
// Command flow-server is the flow API + SSE server (composition root).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/oidcverify"
	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/uuidgen"
	"github.com/serverkraken/flow/internal/config"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

func main() {
	if err := run(); err != nil {
		slog.Error("flow-server exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	pool, err := pgstore.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pgstore.Migrate(ctx, pool); err != nil {
		return err
	}
	verifier, err := oidcverify.New(ctx, cfg.OIDCIssuer, cfg.OIDCClientID)
	if err != nil {
		return err
	}

	srv := &httpserver.Server{
		Verifier: verifier,
		Ensure: usecase.EnsureUser{
			Users: pgstore.NewUserStore(pool),
			IDs:   uuidgen.Gen{},
			Allow: func(id ports.Identity) bool { return cfg.AllowedSubs[id.Username] || cfg.AllowedSubs[id.Subject] },
		},
		Bus: sse.NewBus(),
		Dev: cfg.Dev,
	}

	httpSrv := &http.Server{Addr: cfg.ListenAddr, Handler: srv.Routes(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		slog.Info("listening", "addr", cfg.ListenAddr, "dev", cfg.Dev)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutCtx)
}
```

- [ ] **Step 2: Implement `scripts/smoke-m0.sh` (wiring verification — every route)**

```bash
#!/usr/bin/env bash
# Boots flow-server against the compose DB with a FAKE issuer is NOT possible
# (real OIDC needed). This smoke runs the routes that don't need a real token
# plus prints manual steps for the authed ones. Run `make db-up` first.
set -euo pipefail
BASE="${BASE:-http://localhost:8080}"

echo "== /healthz =="
curl -fsS "$BASE/healthz" && echo

echo "== /api/v1/me without token (expect 401) =="
code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/me")
[ "$code" = "401" ] && echo "OK 401" || { echo "FAIL: $code"; exit 1; }

cat <<'EOF'
== authed routes (manual, needs a real Authentik token) ==
  TOKEN=<paste access token for an allowlisted sub>
  curl -fsS -H "Authorization: Bearer $TOKEN" $BASE/api/v1/me ; echo
  # in one shell: stream events
  curl -N  -H "Authorization: Bearer $TOKEN" $BASE/api/v1/events &
  # in another: fire a ping (FLOW_DEV=1)
  curl -fsS -X POST -H "Authorization: Bearer $TOKEN" $BASE/api/v1/debug/ping
  # the streaming shell should print:  event: ping
EOF
```
Run: `chmod +x scripts/smoke-m0.sh`

- [ ] **Step 3: Build + vet** · `go build ./... && go vet ./...`
  Expected: builds clean.

- [ ] **Step 4: Manual wiring verification**

```bash
make db-up
DATABASE_URL='postgres://flow:flow@localhost:5432/flow?sslmode=disable' \
FLOW_OIDC_ISSUER='https://id.thebackend.org/application/o/flow/' \
FLOW_OIDC_CLIENT_ID='flow' FLOW_ALLOWED_SUBS='msoent' FLOW_DEV=1 \
go run ./cmd/flow-server &
./scripts/smoke-m0.sh    # /healthz 200, /me 401; then the manual token steps
```
Expected: `/healthz` → `{"status":"ok"}`, `/me` → 401, migrations applied (users table exists).

- [ ] **Step 5: Commit** · `git add -A && git commit -m "feat(flow-server): composition root + graceful shutdown + smoke"`

---

## Task 13: Thin client — apiclient + `flow whoami`

**Files:**
- Create: `internal/adapter/apiclient/client.go`, `internal/adapter/apiclient/client_test.go`, `cmd/flow/main.go`, `cmd/flow/whoami.go`

- [ ] **Step 1: Write the failing test (apiclient against httptest)**

```go
package apiclient_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

func TestWhoamiSendsBearerAndParses(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"id":"u1","username":"msoent","email":"m@x.de","displayName":"Martin"}`))
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok-123")
	u, err := c.Whoami(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer tok-123" {
		t.Fatalf("auth header: %q", gotAuth)
	}
	if u.Username != "msoent" {
		t.Fatalf("parse: %+v", u)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** · `go test ./internal/adapter/apiclient/...`

- [ ] **Step 3: Implement `client.go`**

```go
// Package apiclient is the client-side REST adapter used by the TUI/CLI.
package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

type Client struct {
	base  string
	token string
	hc    *http.Client
}

func New(base, token string) *Client {
	return &Client{base: base, token: token, hc: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) Whoami(ctx context.Context) (domain.User, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+"/api/v1/me", nil)
	if err != nil {
		return domain.User{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := c.hc.Do(req)
	if err != nil {
		return domain.User{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return domain.User{}, fmt.Errorf("apiclient: /me status %d", res.StatusCode)
	}
	var u domain.User
	if err := json.NewDecoder(res.Body).Decode(&u); err != nil {
		return domain.User{}, fmt.Errorf("apiclient: decode: %w", err)
	}
	return u, nil
}
```

- [ ] **Step 4: Run — expect PASS** · `go test ./internal/adapter/apiclient/...`

- [ ] **Step 5: Implement the CLI** (`cmd/flow/main.go` + `whoami.go`)

`cmd/flow/main.go`:
```go
// Command flow is the flow client (CLI + later TUI).
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func rootCmd() *cobra.Command {
	root := &cobra.Command{Use: "flow", Short: "flow client"}
	root.AddCommand(whoamiCmd())
	return root
}

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

`cmd/flow/whoami.go`:
```go
package main

import (
	"fmt"
	"os"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/spf13/cobra"
)

func whoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated flow user",
		RunE: func(cmd *cobra.Command, _ []string) error {
			base := envOr("FLOW_SERVER_URL", "http://localhost:8080")
			token := os.Getenv("FLOW_TOKEN") // device-flow login lands in M1
			if token == "" {
				return fmt.Errorf("set FLOW_TOKEN (device-flow login comes in M1)")
			}
			u, err := apiclient.New(base, token).Whoami(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Printf("%s <%s> (%s)\n", u.DisplayName, u.Email, u.Username)
			return nil
		},
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
```

- [ ] **Step 6: Build + run** · `go build ./... && go vet ./...`
- [ ] **Step 7: Commit** · `git add -A && git commit -m "feat(client): apiclient + flow whoami"`

---

## Task 14: Green gate + close M0

**Files:** none new — verification only.

- [ ] **Step 1: Format + tidy** · `gofumpt -w . && goimports -w . && go mod tidy`
- [ ] **Step 2: Full test (race)** · `make test` → all PASS (pgstore may SKIP without Docker)
- [ ] **Step 3: Coverage gate** · `make cover` → ≥ 80% on `./internal/...`
  If below: add focused tests for the lowest-covered internal package (likely httpserver error paths). Do **not** lower the threshold without a note here.
- [ ] **Step 4: Lint** · `make lint` (install golangci-lint if missing) → clean
- [ ] **Step 5: Full CI** · `make ci` → green
- [ ] **Step 6: Final manual loop proof** (the M0 done-criteria)
  With `make db-up` + a real allowlisted Authentik token: `flow whoami` prints your user; the SSE stream prints `event: ping` when `debug/ping` fires. Record the result in the commit body.
- [ ] **Step 7: Commit** · `git add -A && git commit -m "chore(m0): green ci — spine complete (token → /me → live SSE event)"`

---

## Done criteria for M0

- `make ci` green on the `rebuild` worktree.
- A bearer token from an allowlisted Authentik sub → `flow whoami` succeeds; `/api/v1/events` delivers a live `ping` pushed via `/api/v1/debug/ping`. The server-authoritative + live-push loop is proven.
- Postgres schema is migration-managed (`users`).
- Hexagonal boundaries intact: `domain`/`usecase` import no I/O; adapters implement ports; `cmd/flow-server` is wiring-only.

**Next:** M1 — Worktime vertical (Sessions/Timer/DayOff in server + TUI + WebUI, live-synced) + device-flow login. New plan: `docs/superpowers/plans/2026-06-13-flow-rebuild-m1-worktime.md`.
