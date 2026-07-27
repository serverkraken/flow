# M1 Slice 5 — Activity-Feed + Actor — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn flow's ephemeral SSE events into a persisted, owner-scoped, paginated **activity log** attributed to an **Actor** (human vs. named MCP agent), and surface it as a filterable **Home Logstream**.

**Architecture:** A single DRY `Emit(ctx, ev)` choke-point replaces every `Bus.Publish(ev)`: it publishes over the existing SSE bus AND (selectively, via one `activityFor` policy) persists a `domain.ActivityEntry` to a new `activity` table, then publishes a new `activity.logged` event for granular live-refresh. The Actor is derived in auth middleware from an `X-Flow-Actor` header (set by `cmd/flow-mcp` from the MCP `clientInfo.name`); absent header ⇒ human. WebUI Home renders the newest entries with a circle/hexagon actor glyph and class+actor filters.

**Tech Stack:** Go, hexagonal (domain → ports → adapters), Postgres via goose migrations + pgx, templ (`make generate`), Tailwind v4 (`make web`), htmx + SSE. MCP via `github.com/modelcontextprotocol/go-sdk` v1.6.1.

## Global Constraints

- **Spec:** `docs/superpowers/specs/2026-06-30-flow-m1-slice5-activity-feed-design.md`. Read it.
- **All commands run from** `/Users/msoent/SourceCode/serverkraken/flow-m1`.
- **`make ci = lint verify-generate verify-css verify-no-popups cover build`; coverage gate = 75%** (`COVER_THRESHOLD`). Report the exact % if it dips; never add fake tests to pad it.
- **Build discipline:** any `.templ` change → `make generate` + commit the regenerated `_templ.go`; any `web/tailwind.css` change → `make web` + commit `internal/adapter/webui/static/app.css`. NO color-emoji (geometric `○ ⬡ ▶ ■ ●` and inline SVG are fine); NO `window.alert/confirm/prompt`.
- **Each task leaves `go build ./...` + `go test ./internal/... ./cmd/...` green** and ends with a commit.
- **Migrations are goose-annotated** (`-- +goose Up` / `-- +goose Down`); bare SQL fails at apply and only the Docker pgstore tests catch it.
- **Use live event names** — the real constants are in `internal/domain/event.go` (`session.*`, `document.*`, `node.*`, `dayoff.changed`, `settings.changed`). `project.created` does NOT exist.

**Pre-step (one-time, before Task 1):** the Slice-3 and Slice-4 plans are untracked in this worktree. Commit them so they aren't a loose end:
```bash
git add docs/superpowers/plans/2026-06-30-m1-slice3-shell-ia.md docs/superpowers/plans/2026-06-30-m1-slice4-home.md
git commit -m "docs(m1): track Slice 3 + Slice 4 implementation plans"
```

---

### Task 1: Actor package + auth-middleware integration

**Files:**
- Create: `internal/actor/actor.go`, `internal/actor/actor_test.go`
- Modify: `internal/adapter/httpserver/middleware.go` (the `auth` middleware), `internal/adapter/httpserver/webauth.go` (`webAuth`, `authAny`)

**Interfaces — Produces:**
- `actor.Actor{Kind actor.Kind; Ref string}`; `actor.Human`/`actor.Agent` constants (string `Kind`).
- `actor.WithContext(ctx, Actor) context.Context`; `actor.FromContext(ctx) Actor` (default `{Human, ""}`).
- `actor.FromHeader(headerVal, displayName string) Actor` — header non-empty ⇒ `{Agent, headerVal}`; empty ⇒ `{Human, displayName}`.

- [ ] **Step 1 (test, RED):** `internal/actor/actor_test.go` — assert: `FromHeader("claude-code", "Soenne")` → `{Agent, "claude-code"}`; `FromHeader("", "Soenne")` → `{Human, "Soenne"}`; `FromContext(WithContext(ctx, a)) == a`; `FromContext(context.Background())` → `{Human, ""}`.
- [ ] **Step 2:** create `internal/actor/actor.go`:

```go
// Package actor identifies who performed an action: a human or a named AI agent.
package actor

import "context"

type Kind string

const (
	Human Kind = "human"
	Agent Kind = "agent"
)

// Actor is the principal behind a mutation. Ref is the human's display name or
// the agent's MCP client name (e.g. "claude-code").
type Actor struct {
	Kind Kind
	Ref  string
}

type ctxKey int

const key ctxKey = 0

func WithContext(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, key, a)
}

// FromContext returns the actor stored in ctx, or a zero human if none.
func FromContext(ctx context.Context) Actor {
	if a, ok := ctx.Value(key).(Actor); ok {
		return a
	}
	return Actor{Kind: Human}
}

// FromHeader builds an Actor from the X-Flow-Actor header value. A non-empty
// value means an AI agent identified by that name; empty means the human user.
func FromHeader(headerVal, displayName string) Actor {
	if headerVal != "" {
		return Actor{Kind: Agent, Ref: headerVal}
	}
	return Actor{Kind: Human, Ref: displayName}
}
```

- [ ] **Step 3:** Run `go test ./internal/actor/...` → PASS.
- [ ] **Step 4:** Wire the actor into the context alongside the user. In `middleware.go`, add a helper near `userFrom` (DRY — all auth paths funnel through it):

```go
// ctxWithUser stores the authenticated user AND the derived actor in ctx.
// The actor comes from the X-Flow-Actor header (set by the MCP client) or
// defaults to the human user.
func ctxWithUser(r *http.Request, u domain.User) context.Context {
	ctx := context.WithValue(r.Context(), userKey, u)
	return actor.WithContext(ctx, actor.FromHeader(r.Header.Get("X-Flow-Actor"), u.DisplayName))
}
```

Add the import `"github.com/serverkraken/flow/internal/actor"` to `middleware.go`. Then replace **every** `r.WithContext(context.WithValue(r.Context(), userKey, u))` with `r.WithContext(ctxWithUser(r, u))` in:
- `middleware.go` `auth` (1 site)
- `webauth.go` `webAuth` (1 site), `authAny` (2 sites — bearer + cookie)

- [ ] **Step 5 (test):** add an `httpserver`-package test (model on existing middleware tests; `rg -l "func Test.*[Aa]uth" internal/adapter/httpserver`) that wraps a probe handler with `s.auth`, issues a request with `X-Flow-Actor: claude-code` and a valid token, and asserts the probe sees `actor.FromContext(r.Context()) == {Agent,"claude-code"}`; without the header it sees `{Human, u.DisplayName}`. (Use the existing fake `Verifier`/`Ensure` test harness.)
- [ ] **Step 6:** `go build ./... && go test ./internal/actor/... ./internal/adapter/httpserver/...` → PASS.
- [ ] **Step 7:** Commit `feat(actor): actor package + X-Flow-Actor middleware derivation`.

---

### Task 2: MCP forwards clientInfo as X-Flow-Actor

**Files:**
- Modify: `internal/adapter/apiclient/client.go` (+ `internal/adapter/apiclient/client_test.go`)
- Modify: `cmd/flow-mcp/server.go` (or wherever `*handlers` lives — add `clientName` + a `do` wrapper), and the tool handlers that call `h.mgr.Do`

**Read first:** `internal/adapter/apiclient/client.go:39-67` (`NewTransport`, `staticBearer.RoundTrip` — the exact copy-pattern), and the tool-handler signatures in `cmd/flow-mcp/tools_docs.go` / `tools_write.go` / `tools_context.go` (they discard `_ *mcp.CallToolRequest`).

**Interfaces — Produces:**
- `func (c *apiclient.Client) WithActor(name string) *Client` — returns a copy whose requests also carry `X-Flow-Actor: name` (wraps the existing auth transport `c.rt`).

- [ ] **Step 1 (test, RED):** `client_test.go` — start an `httptest.Server` that echoes `r.Header.Get("X-Flow-Actor")`; build `New(srv.URL, "tok").WithActor("claude-code")`, make any GET via an exported client method (or add a tiny test through `do`), assert the server saw `claude-code` AND `Authorization: Bearer tok`.
- [ ] **Step 2:** in `client.go`, add (mirrors `staticBearer`):

```go
// actorTransport injects X-Flow-Actor on every outgoing request, wrapping the
// auth-bearing transport so both headers are present.
type actorTransport struct {
	name string
	base http.RoundTripper
}

func (a actorTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Header.Set("X-Flow-Actor", a.name)
	return a.base.RoundTrip(r2)
}

// WithActor returns a shallow copy of c whose requests carry X-Flow-Actor: name.
func (c *Client) WithActor(name string) *Client {
	rt := actorTransport{name: name, base: c.rt}
	return &Client{base: c.base, rt: rt, hc: &http.Client{Timeout: 15 * time.Second, Transport: rt}}
}
```

- [ ] **Step 3:** Run the client test → PASS.
- [ ] **Step 4:** In `cmd/flow-mcp` add a helper (one place; `req.Session.InitializeParams().ClientInfo.Name` is the SDK path, populated before any tool fires):

```go
func clientName(req *mcp.CallToolRequest) string {
	if v := os.Getenv("FLOW_ACTOR"); v != "" {
		return v
	}
	if req == nil || req.Session == nil {
		return ""
	}
	ip := req.Session.InitializeParams()
	if ip == nil || ip.ClientInfo == nil {
		return ""
	}
	return ip.ClientInfo.Name
}
```

Add a DRY wrapper on `*handlers` so each tool handler keeps one line:

```go
func (h *handlers) do(ctx context.Context, req *mcp.CallToolRequest, fn func(*apiclient.Client) error) error {
	name := clientName(req)
	return h.mgr.Do(ctx, func(c *apiclient.Client) error {
		if name != "" {
			c = c.WithActor(name)
		}
		return fn(c)
	})
}
```

- [ ] **Step 5:** In every tool handler, rename the discarded `_ *mcp.CallToolRequest` parameter to `req` and change `h.mgr.Do(ctx, …)` → `h.do(ctx, req, …)`. (`rg -n "mgr\.Do\(" cmd/flow-mcp` lists them.) Reads are harmless; writes (`tools_write.go`) are the ones that produce attributed activity.
- [ ] **Step 6:** `go build ./... && go test ./internal/adapter/apiclient/... ./cmd/flow-mcp/...` → PASS.
- [ ] **Step 7:** Commit `feat(mcp): forward MCP clientInfo as X-Flow-Actor header`.

---

### Task 3: Activity domain type + ActivityStore port + pgstore + migration

**Files:**
- Create: `internal/domain/activity.go`, `internal/adapter/pgstore/migrations/0024_activity_log.sql`, `internal/adapter/pgstore/activity.go`, `internal/adapter/pgstore/activity_test.go`
- Modify: `internal/ports/ports.go` (add `ActivityStore`)

**Read first:** the `SetArchived`/`ListArchived` slice as template — `internal/adapter/pgstore/documents.go` (`docCols`, `scanDocument`, `ListPage` two-query pattern at L142-170), and an existing `*_test.go` Docker pgstore test (`rg -l "pgtest|NewTestPool|testing.Short" internal/adapter/pgstore`).

**Interfaces — Produces:**
- `domain.ActivityEntry{ID, OwnerID, ActorKind, ActorRef, Kind string; TargetRef, Label, NodeRef *string; At time.Time}`
- `ports.ActivityStore`:
  - `Append(ctx context.Context, e domain.ActivityEntry) error`
  - `ListPage(ctx context.Context, ownerID string, classes []string, actorRef *string, limit, offset int) (items []domain.ActivityEntry, total int, err error)`
- `pgstore.NewActivityStore(pool) *ActivityStore` (the store does NOT generate IDs — the Emitter sets `entry.ID`; mirrors `NewSessionStore(pool)`)

- [ ] **Step 1:** create `internal/domain/activity.go`:

```go
package domain

import "time"

// ActivityEntry is one persisted, owner-scoped activity-log row: who (Actor)
// did what (Kind = a raw EventType) to which target, with a readable Label
// snapshot taken at action time.
type ActivityEntry struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"-"`
	ActorKind string    `json:"actorKind"` // "human" | "agent"
	ActorRef  string    `json:"actorRef"`  // display name or agent client name
	Kind      string    `json:"kind"`      // raw EventType, e.g. "document.updated"
	TargetRef *string   `json:"targetRef,omitempty"`
	Label     *string   `json:"label,omitempty"`
	NodeRef   *string   `json:"nodeRef,omitempty"`
	At        time.Time `json:"at"`
}
```

- [ ] **Step 2:** add to `ports.go` (near `DocumentStore`):

```go
// ActivityStore persists and queries the owner-scoped activity log.
type ActivityStore interface {
	Append(ctx context.Context, e domain.ActivityEntry) error
	// ListPage returns one page newest-first plus the total matching the
	// owner/class/actor filter. `classes` matches kind prefixes (e.g. "session",
	// "document"); empty = all. `actorRef` nil = any actor.
	ListPage(ctx context.Context, ownerID string, classes []string, actorRef *string, limit, offset int) (items []domain.ActivityEntry, total int, err error)
}
```

- [ ] **Step 3:** create migration `0024_activity_log.sql`:

```sql
-- +goose Up
CREATE TABLE activity (
    id          TEXT PRIMARY KEY,
    owner_id    TEXT        NOT NULL,
    actor_kind  TEXT        NOT NULL,
    actor_ref   TEXT        NOT NULL,
    kind        TEXT        NOT NULL,
    target_ref  TEXT,
    label       TEXT,
    node_ref    TEXT,
    at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX activity_owner_at ON activity (owner_id, at DESC);

-- +goose Down
DROP TABLE activity;
```

- [ ] **Step 4 (test, RED):** `activity_test.go` (Docker pgstore harness, mirror an existing one): seed 3 entries for owner A (mixed kinds + actor_refs) + 1 for owner B; assert `ListPage(A, nil, nil, 50, 0)` returns A's 3 newest-first with correct `total=3`; `ListPage(A, []string{"document"}, nil, …)` returns only `document.*`; `ListPage(A, nil, ptr("claude-code"), …)` returns only that actor; owner B's row never leaks.
- [ ] **Step 5:** create `internal/adapter/pgstore/activity.go`. `Append` inserts all columns (set `at` from the entry; if zero, let the DB default apply — but the Emitter always sets it). `ListPage` follows the documents `ListPage` dynamic-WHERE shape:

```go
const activityCols = `id, owner_id, actor_kind, actor_ref, kind, target_ref, label, node_ref, at`

func (s *ActivityStore) ListPage(ctx context.Context, ownerID string, classes []string, actorRef *string, limit, offset int) ([]domain.ActivityEntry, int, error) {
	where := ` WHERE owner_id=$1`
	args := []any{ownerID}
	if len(classes) > 0 {
		// kind prefix match: kind LIKE 'session.%' OR kind LIKE 'document.%' …
		ors := make([]string, 0, len(classes))
		for _, c := range classes {
			args = append(args, c+".%")
			ors = append(ors, fmt.Sprintf("kind LIKE $%d", len(args)))
		}
		where += " AND (" + strings.Join(ors, " OR ") + ")"
	}
	if actorRef != nil {
		args = append(args, *actorRef)
		where += fmt.Sprintf(" AND actor_ref=$%d", len(args))
	}
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM activity`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("pgstore: count activity: %w", err)
	}
	args = append(args, limit, offset)
	q := `SELECT ` + activityCols + ` FROM activity` + where +
		fmt.Sprintf(` ORDER BY at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("pgstore: list activity: %w", err)
	}
	defer rows.Close()
	out, err := scanActivities(rows)
	return out, total, err
}
```

Write `Append`, `NewActivityStore(pool)`, and `scanActivity`/`scanActivities` (mirror `scanDocument`/`scanDocuments`; scan into the `*string` nullable fields directly — pgx handles `*string` for nullable TEXT). The store does NOT take `ids` — `Append` inserts the entry's pre-set `ID` (the Emitter generates it in Task 4); this mirrors `NewSessionStore(pool)`.
- [ ] **Step 6:** Run the Docker pgstore test (`make db-up` if needed; `DOCKER_HOST=$(podman …)` per the env note in CLAUDE-troubleshooting if pgstore tests can't reach a daemon) → PASS.
- [ ] **Step 7:** Commit `feat(activity): ActivityEntry + ActivityStore + migration 0024`.

---

### Task 4: activityFor policy + Emitter + wiring

**Files:**
- Modify: `internal/domain/event.go` (add `EventActivityLogged`), `internal/ports/ports.go` (add `Emitter`), `internal/adapter/httpserver/server.go` (add `Emitter` field), `cmd/flow-server/main.go` (construct + inject)
- Create: `internal/adapter/sse/activityfor.go`, `internal/adapter/sse/emitter.go`, `internal/adapter/sse/emitter_test.go`

**Read first:** `internal/adapter/sse/bus.go` (Publish/Subscribe), `internal/domain/event.go`, and `cmd/flow-server/main.go:65-77` (where `bus`, `ids`, `clock`, stores are built) + the `&httpserver.Server{…}` literal.

**Interfaces:**
- Consumes: `ports.ActivityStore` (Task 3), `actor.FromContext` (Task 1), `ports.IDGen`, `ports.Clock`, `*sse.Bus`.
- Produces: `ports.Emitter{ Emit(ctx context.Context, ev domain.Event) }`; `sse.NewEmitter(bus *Bus, store ports.ActivityStore, ids ports.IDGen, clock ports.Clock) *Emitter`. `domain.EventActivityLogged EventType = "activity.logged"`.

- [ ] **Step 1:** `event.go` — add to the const block: `EventActivityLogged EventType = "activity.logged"`.
- [ ] **Step 2:** `ports.go` — add:

```go
// Emitter publishes a live event and, for loggable mutations, persists an
// activity entry. It replaces direct EventBus.Publish at mutation sites.
type Emitter interface {
	Emit(ctx context.Context, ev domain.Event)
}
```

- [ ] **Step 3:** create `internal/adapter/sse/activityfor.go` — the single entry-construction policy:

```go
package sse

import (
	"context"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
)

// activityFor maps a live event to an activity entry. Returns ok=false for
// events that must not be logged (settings.changed; activity.logged itself).
func activityFor(ctx context.Context, ev domain.Event) (domain.ActivityEntry, bool) {
	switch ev.Type {
	case domain.EventSettingsChanged, domain.EventActivityLogged:
		return domain.ActivityEntry{}, false
	}
	a := actor.FromContext(ctx)
	e := domain.ActivityEntry{
		OwnerID:   ev.UserID,
		ActorKind: string(a.Kind),
		ActorRef:  a.Ref,
		Kind:      string(ev.Type),
	}
	if ev.Data != nil {
		if v, ok := ev.Data["id"].(string); ok && v != "" {
			e.TargetRef = &v
		}
		// title (documents) or name (nodes) → readable label snapshot.
		if v, ok := ev.Data["title"].(string); ok && v != "" {
			e.Label = &v
		} else if v, ok := ev.Data["name"].(string); ok && v != "" {
			e.Label = &v
		}
		if v, ok := ev.Data["node"].(string); ok && v != "" {
			e.NodeRef = &v
		}
	}
	return e, true
}
```

- [ ] **Step 4:** create `internal/adapter/sse/emitter.go`:

```go
package sse

import (
	"context"
	"log/slog"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Emitter publishes ev over the bus and, for loggable mutations, persists an
// activity entry then publishes EventActivityLogged for live refresh.
type Emitter struct {
	bus   *Bus
	store ports.ActivityStore
	ids   ports.IDGen
	clock ports.Clock
}

func NewEmitter(bus *Bus, store ports.ActivityStore, ids ports.IDGen, clock ports.Clock) *Emitter {
	return &Emitter{bus: bus, store: store, ids: ids, clock: clock}
}

func (e *Emitter) Emit(ctx context.Context, ev domain.Event) {
	e.bus.Publish(ev)
	entry, ok := activityFor(ctx, ev)
	if !ok {
		return
	}
	entry.ID = e.ids.NewID()
	entry.At = e.clock.Now()
	if err := e.store.Append(ctx, entry); err != nil {
		slog.ErrorContext(ctx, "activity: append failed", "kind", entry.Kind, "err", err)
		return
	}
	e.bus.Publish(domain.Event{Type: domain.EventActivityLogged, UserID: ev.UserID})
}
```

- [ ] **Step 5 (test):** `emitter_test.go` — fake `ports.ActivityStore` (records `Append` calls) + a real `*Bus` with a subscriber. With `actor.WithContext(ctx, {Agent,"claude-code"})`: `Emit(EventDocumentUpdated, Data{"id":"d1","title":"Foo"})` → subscriber receives `document.updated` then `activity.logged`; fake store has 1 entry with `ActorRef="claude-code"`, `Label="Foo"`. `Emit(EventSettingsChanged)` → subscriber receives `settings.changed` only; store empty.
- [ ] **Step 6:** wire it. `server.go`: add field `Emitter ports.Emitter` to `Server`. `cmd/flow-server/main.go`: after `bus := sse.NewBus()` add `activityStore := pgstore.NewActivityStore(pool)`, then build `emitter := sse.NewEmitter(bus, activityStore, ids, clock)`; in the `&httpserver.Server{…}` literal set `Emitter: emitter`. (Keep `Bus: bus` — the SSE handler still subscribes via it.)
- [ ] **Step 7:** `go build ./... && go test ./internal/adapter/sse/...` → PASS.
- [ ] **Step 8:** Commit `feat(activity): Emit choke-point (publish + selective persist) + activity.logged`.

---

### Task 5: ListActivity usecase + REST endpoint

**Files:**
- Create: `internal/usecase/list_activity.go`, `internal/adapter/httpserver/activity.go` (+ test)
- Modify: `internal/adapter/httpserver/server.go` (field + route), `cmd/flow-server/main.go` (wire usecase)

**Read first:** `usecase/list_archived.go` (thin-usecase shape), `httpserver/documents.go:268-279` (`handleListArchived` + `writeJSON`), `server.go:167` (static route registration).

**Interfaces — Produces:**
- `usecase.ListActivity{ Activities ports.ActivityStore }` with `Execute(ctx, ownerID string, classes []string, actorRef *string, limit, offset int) ([]domain.ActivityEntry, int, error)`.
- `GET /api/v1/activity?class=&actor=&limit=&offset=` → JSON array `[]domain.ActivityEntry`.

- [ ] **Step 1:** `list_activity.go`:

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type ListActivity struct{ Activities ports.ActivityStore }

func (uc ListActivity) Execute(ctx context.Context, ownerID string, classes []string, actorRef *string, limit, offset int) ([]domain.ActivityEntry, int, error) {
	return uc.Activities.ListPage(ctx, ownerID, classes, actorRef, limit, offset)
}
```

- [ ] **Step 2 (test, RED):** `activity_test.go` (httpserver) — with a fake `ListActivity` store seeded, `GET /api/v1/activity?limit=10` (with a valid bearer via the test harness) → 200, JSON array; `?class=document` passes `classes=["document"]`; `?actor=claude-code` passes that ref. Guard: nil result serializes as `[]`.
- [ ] **Step 3:** `httpserver/activity.go` — `handleListActivity`: `u, _ := userFrom(r.Context())`; parse `limit`/`offset` (default limit 50; reuse the existing query-int helper if one exists, else `strconv.Atoi` with fallback); `class` may repeat (`r.URL.Query()["class"]`) → `classes`; `actor` → `*string` if non-empty; call `s.ListActivity.Execute(...)`; nil→`[]domain.ActivityEntry{}`; `writeJSON(w, http.StatusOK, list)`.
- [ ] **Step 4:** `server.go` — add field `ListActivity usecase.ListActivity`; register `mux.Handle("GET /api/v1/activity", s.auth(http.HandlerFunc(s.handleListActivity)))`. `main.go` — set `ListActivity: usecase.ListActivity{Activities: activityStore}`.
- [ ] **Step 5:** `go build ./... && go test ./internal/adapter/httpserver/... ./internal/usecase/...` → PASS.
- [ ] **Step 6:** Commit `feat(activity): ListActivity usecase + GET /api/v1/activity`.

---

### Task 6: Call-site migration — REST handlers (Bus.Publish → Emit + label enrichment)

**Files (modify):** `internal/adapter/httpserver/worktime.go`, `documents.go`, `nodemove.go`, `nodetags.go`, `context.go` (+ touch their tests).

**Reference:** the exhaustive call-site/label table in `.superpowers/sdd/slice5-touchpoints.md` (§"Call-site / label table") — for each site, change `s.Bus.Publish(ev)` → `s.Emitter.Emit(r.Context(), ev)` and enrich `ev.Data` where a name/title is already in scope.

- [ ] **Step 1:** `documents.go` — at the 7 publish sites, switch to `Emit` and enrich:
  - `handleCreateDocument`, `handleImportDocument`: `Data: map[string]any{"id": doc.ID, "title": doc.Title}` (+ `"node"` if `doc.NodeID != nil`).
  - `handleUpdateDocument`: `{"id": doc.ID, "title": doc.Title}`.
  - `handleUpsertByPath`: `{"id": id, "title": req.Title}`.
  - `handleDeleteDocument`: pre-fetch before delete (`doc, _ := s.GetDocument.Execute(r.Context(), u.ID, id)`) so the label survives; `{"id": id, "title": doc.Title}`.
  - `handleArchiveDocument`, `handlePinDocument`: keep `{"id": id}` (title not in scope; label optional for these).
- [ ] **Step 2:** `worktime.go` — switch the 9 sites to `Emit`. Nodes carry names: `handleCreateNode`/`handleUpdateNode` → `{"id": p.ID, "name": p.Name}`. Sessions stay `{"id": sess.ID}` (no project name in scope — acceptable; verb alone reads fine). Bulk ops (`handleReassignSessions`, `handleBulkDeleteSessions`) keep nil Data.
- [ ] **Step 3:** `nodemove.go` (`{"id": n.ID, "name": n.Name, "node": n.ID}`), `nodetags.go` (`{"id": id}`), `context.go` (`{"id": id, "title": req.Title}`) → `Emit`.
- [ ] **Step 4 (test):** extend/representative — in the documents handler test, after a successful create, assert an activity row was appended (use a fake `ActivityStore` injected via the test `Emitter`, or assert via the `ListActivity` endpoint). At minimum: one create-doc and one create-node test confirming `actor_kind`/`label`.
- [ ] **Step 5:** `go build ./... && go test ./internal/adapter/httpserver/...` → PASS.
- [ ] **Step 6:** Commit `feat(activity): record REST mutations via Emit (+ label enrichment)`.

---

### Task 7: Call-site migration — WebUI handlers + dayoff usecases

**Files (modify):** `internal/adapter/httpserver/webui.go`, `webui_home.go`, `webui_worktime.go`, `webui_nodes.go`, `webui_historie.go`, `webui_editor.go`, `webui_dayoffs.go`, `webui_einstellungen.go`, `dayoffs.go`; `internal/usecase/add_dayoffs.go`, `delete_dayoff.go`; `cmd/flow-server/main.go` (usecase wiring).

- [ ] **Step 1:** WebUI handlers — switch every `s.Bus.Publish(ev)` → `s.Emitter.Emit(r.Context(), ev)`. Enrich where a name/title is trivially in scope: `webui_nodes.go` create/update/status carry `n.Name`/`cur.Name` → `{"id": …, "name": …}`; `webui_editor.go` create → `{"id": doc.ID, "title": doc.Title}`, update → `{"id": id, "title": r.FormValue("title")}`, delete (pre-fetched `doc`) → `{"id": id, "title": doc.Title}`; inline-create-node sites (`webui.go`, `webui_home.go`, `webui_worktime.go resolveWebNode`) → `{"id": p.ID, "name": p.Name}`. Settings sites (`webui_dayoffs.go`, `webui_einstellungen.go`, `dayoffs.go`) still go through `Emit` (policy drops them — keeps one path).
- [ ] **Step 2:** dayoff usecases — replace the injected `Bus ports.EventBus` field with `Emitter ports.Emitter` in `add_dayoffs.go` + `delete_dayoff.go`; change `uc.Bus.Publish(domain.Event{…})` → `uc.Emitter.Emit(ctx, domain.Event{Type: domain.EventDayOffChanged, UserID: ownerID})` (the actor is in the request `ctx` threaded from the handler). `main.go`: `AddDayOffs{Store: dayOffStore, Emitter: emitter}`, `DeleteDayOff{Store: dayOffStore, Emitter: emitter}`.
- [ ] **Step 3 (test):** confirm no remaining `Bus.Publish` at mutation sites except the SSE subscribe path and the Emitter's internal publishes: `rg -n "Bus\.Publish" internal/adapter/httpserver internal/usecase` should return **only** nothing at mutation handlers (the only `Publish` callers left are inside `sse/`). Update any dayoff usecase tests that constructed the struct with `Bus:` to use `Emitter:` (a fake `ports.Emitter`).
- [ ] **Step 4:** `go build ./... && go test ./internal/...` → PASS.
- [ ] **Step 5:** Commit `feat(activity): record WebUI + dayoff mutations via Emit`.

---

### Task 8: Actor-glyph component + i18n strings

**Files:**
- Create: `internal/adapter/webui/components/actor_glyph.templ`
- Modify: `internal/i18n/catalog_de.go`, `internal/i18n/catalog_en.go`

**Read first:** `components/appshell.templ:6-17` (BrandMark hexagon — viewBox `0 0 34 34`, `rgb(var(--…))`), `components/themetoggle.templ` (currentColor SVG), `i18n/catalog_de.go` (string-map format).

- [ ] **Step 1:** create `actor_glyph.templ` — `ActorGlyph(kind string)`: `kind == "agent"` → a small hexagon polygon (viewBox `0 0 20 20`, `class="h-4 w-4"`, `fill="currentColor"`/`stroke`), else a `<circle>`. Both `aria-hidden="true"`, colors via `currentColor` so the row's text color drives them. Keep it ~12 lines, no gradient (solid currentColor).
- [ ] **Step 2:** add i18n keys to **both** catalogs (DE values shown; EN equivalents):
  - UI: `home.activity` = "Aktivität"; `activity.empty` = "Noch keine Aktivität."; `activity.filter.all`/`.zeit`/`.wissen`/`.struktur`/`.frei` = "Alle"/"Zeit"/"Wissen"/"Struktur"/"Frei"; `activity.actor.all` = "Alle".
  - Verbs (key per kind): `activity.verb.session.started` = "startete einen Timer"; `…session.stopped` = "stoppte den Timer"; `…session.updated` = "änderte eine Sitzung"; `…session.deleted` = "löschte eine Sitzung"; `…document.created` = "legte an"; `…document.updated` = "bearbeitete"; `…document.deleted` = "löschte"; `…node.created` = "legte ein Projekt an"; `…node.updated` = "änderte ein Projekt"; `…node.deleted` = "löschte ein Projekt"; `…node.moved` = "verschob ein Projekt"; `…dayoff.changed` = "aktualisierte Frei-Zeiten".
- [ ] **Step 3:** `make generate` (for the new templ); `go build ./...` → green.
- [ ] **Step 4:** Commit `feat(webui): actor-glyph component + activity i18n strings`.

---

### Task 9: Home Logstream section + VM + fragment handler + filters

**Files:**
- Modify: `internal/adapter/webui/home_vm.go`, `internal/adapter/webui/home.templ`, `internal/adapter/httpserver/webui_home.go`, `internal/adapter/httpserver/server.go` (route)
- Create: `internal/adapter/webui/activity_row.go` (VM builder + helpers); tests alongside the home handler test

**Read first:** `home.templ:18-62` (homeOuter/HomeFragment + the BurndownBanner/NewestDocs slot), `webui_home.go:89-216` (`homeDataFor` guard pattern), `wissen.templ:188-213` (chip pattern: active = `bg-ink text-canvas`, inactive = `border border-line bg-surface`), `components/segtoggle.templ`.

**Interfaces — Produces:**
- `webui.ActivityRowVM{ ActorKind, ActorRef, Verb, Label, Href, RelTime string }`
- `webui.BuildActivityRows(entries []domain.ActivityEntry, now time.Time) []ActivityRowVM`
- `HomeVM` gains `LogEntries []ActivityRowVM`, `LogClass string`, `LogActor string`, `LogActors []string`.

- [ ] **Step 1:** `activity_row.go` — `BuildActivityRows`: for each entry, map `kind` → verb via the i18n key `"activity.verb."+kind` (resolve in the template, not here — store the key OR resolve via a passed translator; simplest: store `VerbKey` and call `components.T` in the templ). Set `Label` from `*entry.Label` (empty if nil); `Href` = `/wissen/{TargetRef}` when `kind` starts with `document.` and `TargetRef != nil`, else ""; `RelTime` via a local `fmtRelTime(at, now)` ("vor 3 Min" / "vor 2 Std" / date fallback — German, no i18n interpolation). Add `ActorKind`/`ActorRef`. (Decision: store `VerbKey string` on the VM and resolve in templ — keeps i18n in the view.)
- [ ] **Step 2:** `home_vm.go` — add the four fields above to `HomeVM`.
- [ ] **Step 3 (test, RED):** extend the home handler test: seed activity (via the test server's fake/real `ListActivity`), `GET /ui/home/logstream` renders rows (verb text, actor glyph differs by kind, doc rows link `/wissen/{id}`); `?class=wissen` filters to document.*; `?actor=claude-code` filters; empty → the `activity.empty` text.
- [ ] **Step 4:** `home.templ` — add a `logstream` templ between the BurndownBanner `<section>` (L51) and the NewestDocs block (L52). Structure it as its own htmx container so it refreshes granularly:

```
<section id="logstream"
    hx-get={ "/ui/home/logstream" + vm.logQuery() }
    hx-trigger="sse:activity.logged"
    hx-swap="outerHTML">
    @homeLogstreamInner(vm)
</section>
```

`homeLogstreamInner` renders: heading `home.activity`; the class chips `[Alle][Zeit][Wissen][Struktur][Frei]` as `<a>` links carrying `hx-get="/ui/home/logstream?class=…&actor=…"` `hx-target="#logstream"` `hx-swap="outerHTML"` (active = `bg-ink text-canvas`); an actor `SegToggle`-or-`<select>` driving `?actor=`; then the rows (each: `@components.ActorGlyph(row.ActorKind)`, `row.ActorRef`, `components.T(ctx, row.VerbKey)`, label link, `row.RelTime`); empty → `activity.empty`. Add `logQuery()`/`logstream` helpers on `HomeVM` for the current `class`/`actor` querystring. Class→prefix map: zeit→session, wissen→document, struktur→node, frei→dayoff.
- [ ] **Step 5:** `webui_home.go` — in `homeDataFor`, after the NewestDocs block, guard-wire activity: `if s.ListActivity.Activities != nil { entries, _, _ := s.ListActivity.Execute(ctx, u.ID, nil, nil, 15, 0); vm.LogEntries = webui.BuildActivityRows(entries, now); vm.LogActors = distinctActors(entries) }`. Add `handleHomeLogstream` (parse `class`→prefix, `actor`; call `ListActivity` with those filters + limit 15; set `vm.LogClass`/`vm.LogActor`; render a new `webui.HomeLogstream(vm)` templ = just the `<section id="logstream">`). Register `GET /ui/home/logstream` under `webAuth` in `server.go`.
- [ ] **Step 6:** optional — add `sse:activity.logged` to the `homeOuter` `#content` trigger list too (so a full `#content` reload also shows new activity); not strictly needed since the section self-refreshes.
- [ ] **Step 7:** `make generate`; `go build ./...`; `go test ./internal/adapter/...` → PASS.
- [ ] **Step 8:** Commit `feat(webui): Home logstream with actor glyph + class/actor filters`.

---

### Task 10: Build artifacts + wiring verification + done-gate

- [ ] **Step 1:** `make generate` + `git diff --exit-code '*_templ.go'` → clean (commit drift if any). `make web` + `git diff --exit-code internal/adapter/webui/static/app.css` → clean (commit drift).
- [ ] **Step 2:** **`make ci`** → green. Report exact coverage % (gate 75; don't pad with fake tests).
- [ ] **Step 3:** **Route audit:** `rg -n "mux.Handle" internal/adapter/httpserver/server.go` confirms `GET /api/v1/activity` (auth) and `GET /ui/home/logstream` (webAuth) registered.
- [ ] **Step 4:** **Publish-path audit:** `rg -n "Bus\.Publish" internal/adapter/httpserver internal/usecase` returns nothing (all mutation sites now go through `Emitter.Emit`; the only `Publish` callers live in `internal/adapter/sse/`).
- [ ] **Step 5:** **Manual done-gate (human, dev stack — `make dev-up`, `make dev-run`, scripted Dex login):**
  1. Start a timer in the WebUI → logstream shows `○ <DisplayName>` + "startete einen Timer".
  2. Via the MCP server (Claude — and if available Gemini/Codex), create/update a document → logstream shows `⬡ claude-code` + "bearbeitete <Titel>", clickable to `/wissen/{id}`.
  3. Class chips (Zeit/Wissen/Struktur/Frei) and the actor filter both narrow the list.
  4. A mutation in another tab live-appends (no reload) via `activity.logged`.
  5. Delete a document → entry keeps its title snapshot ("löschte <Titel>").
- [ ] **Step 6:** Commit any fixups: `fix(activity): done-gate fixups`.

---

## Self-Review (done)

1. **Spec coverage:** §3 datamodel → T3; §4 emit choke-point → T4 (+ migration T6/T7 call-sites); §5 actor (middleware + MCP clientInfo) → T1+T2; §6 feed API → T5; §7 logstream + glyph + both filters → T8+T9; §8 realtime `activity.logged` → T4 (emit) + T9 (trigger); §9 testing/done-gate → every task + T10. Non-goals (no retention cap, no backfill, no TUI feed, no cockpit tab, settings excluded) honored — `activityFor` drops `settings.changed`, no backfill task exists by design.
2. **Placeholder scan:** new artifacts (actor pkg, activityFor, Emitter, ActivityStore SQL+ListPage, ListActivity, actorTransport/WithActor, clientName) are inlined verbatim; established patterns (templ rows, handler tests) cite exact Read-first files + line ranges per the repo's slice-plan convention (cf. the Slice-4 plan). i18n keys + verb strings are enumerated, not "TBD".
3. **Type consistency:** `Emit(ctx, ev)` signature used identically in `ports.Emitter`, `sse.Emitter`, and every call site (T6/T7). `ActivityEntry` fields match across domain (T3), pgstore `activityCols` (T3), `activityFor` (T4), and `BuildActivityRows` (T9). `ListPage(ctx, ownerID, classes []string, actorRef *string, limit, offset)` is identical in `ports.ActivityStore`, pgstore, `ListActivity`, and the REST handler. `actor.FromContext`/`FromHeader`/`WithContext` consistent T1↔T4. `WithActor(name)` consistent T2.

## Notes for the executor
- **Coverage:** the big surface here is the WebUI templ (low coverage by nature). If `cover` dips below 75, add real handler/store tests (T5/T6/T9 are the levers), never fake ones.
- **The `ev.Data` enrichment is the only distributed work** of the DRY choke-point — the table in Task 6/7 is exhaustive; sessions intentionally carry no label (verb reads fine alone), and the only pre-fetch added is the REST doc-delete (so its label snapshot survives).
- **Don't log `activity.logged` or `settings.changed`** — `activityFor` guards both; the Emitter publishes `activity.logged` via `bus.Publish` directly (never through `Emit`), so no loop.
