# flow Phase 1 — M4 Notes-Sync Implementation Plan (Plan C)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build end-to-end RepoNote sync — Soenne can save a per-repo CLAUDE-style note via `flow repo note set`, it propagates to flow-server, and another device sees it via `flow repo note get`. Same Last-Writer-Wins / OCC pattern Plan B established for Sessions and Projects.

**Architecture:** Repos and RepoNotes get the full pipeline already worked out for Sessions in Plan B: sqliteclient adapter → use case → httpsync queue+worker → httpserver handler → sqliteserver adapter with Lamport-versioning. The 409-conflict path reuses the existing `conflict_overlay` component (variant `VariantSessionEdit` is generic enough for RepoNote diffs). CanonicalKey normalisation (git-remote or path-hash) lives in `usecase.RepoNotes` so CLI and future MCP server share one resolver.

**Tech Stack:** Go 1.24, modernc.org/sqlite, chi router, the same goose-NewProvider + Lamport pattern from Plan B. New deps: none (CanonicalKey uses stdlib `crypto/sha256` + `os/exec` for `git remote get-url`).

**Out of scope (deferred to a follow-up plan):**
- Kompendium-Notes sync — Kompendium today is a git-backed file store; migrating it to the sqlite cache is a milestone-scale refactor on its own.
- FTS5 server-side search — orthogonal to the sync pipeline; deserves its own plan once the RepoNote corpus exists.
- WebUI for note editing — covered by Plan E (M6/M7).
- TUI-resident RepoNote screen — CLI is enough for M4; the TUI lift waits until Soenne actually uses RepoNote daily.

---

## File Structure

**Modify (ports):**
- `internal/ports/repo_notes.go` — extend `RepoNoteStore` with `PullSince`, `Delete`, `ErrRepoNoteVersionConflict`.
- `internal/ports/repos.go` — extend `RepoStore` with `PullSince` (Lamport-paged).

**Create (sqliteclient):**
- `internal/adapter/sqliteclient/repos.go` — implements `ports.RepoStore` (EnsureByCanonicalKey, GetByID, Upsert, PullSince).
- `internal/adapter/sqliteclient/repos_test.go` — happy path + EnsureByCanonicalKey idempotent.
- `internal/adapter/sqliteclient/repo_notes.go` — implements `ports.RepoNoteStore` (GetByRepo, Upsert, Delete, PullSince).
- `internal/adapter/sqliteclient/repo_notes_test.go` — happy path + version-bump on update.
- `internal/adapter/sqliteclient/migrations/0003_repos_version.sql` — adds `version INTEGER NOT NULL DEFAULT 0` to `repos`, indexes for `(user_id, version)` on both tables (the initial schema already has the columns but no version on repos).

**Create (sqliteserver):**
- `internal/adapter/sqliteserver/migrations/0003_repo_notes.sql` — `repos` and `repo_notes` tables (the M2 initial schema only set up users/projects/sessions/active_sessions/lamport; repo-tables are missing server-side).
- `internal/adapter/sqliteserver/repos.go` — server-side `Repos` (Upsert w/ OCC + Lamport, EnsureByCanonicalKey, GetByID, PullSince).
- `internal/adapter/sqliteserver/repos_test.go` — version-bump, conflict, idempotent EnsureByCanonicalKey.
- `internal/adapter/sqliteserver/repo_notes.go` — server-side `RepoNotes` (Upsert w/ OCC + Lamport, GetByRepo, PullSince).
- `internal/adapter/sqliteserver/repo_notes_test.go` — same shape as sessions_test.

**Create (usecase):**
- `internal/usecase/repo_notes.go` — `RepoNotes{users, repos, notes, queue, device}` with `Resolve(canonicalKey) → Repo`, `GetForKey(canonicalKey) → RepoNote`, `Save(canonicalKey, content) → RepoNote`. Generates UUIDs, queues pushes, signals worker.
- `internal/usecase/repo_notes_test.go` — happy path + version-conflict propagation + canonical-key normalisation.
- `internal/usecase/canonical_key.go` — `CanonicalKey(pwd string) (string, error)` — runs `git remote get-url origin` first; falls back to `path:sha256(absPath)` when no remote exists. No `os/exec` import in tests — the function takes a `RemoteResolver` interface for injectability.
- `internal/usecase/canonical_key_test.go` — covers both branches via the `RemoteResolver` fake.

**Create (httpsync):**
- `internal/adapter/httpsync/repo_notes_client.go` — `PullRepos`, `PushRepo`, `PullRepoNotes`, `PushRepoNote` on the existing `Client` struct. Same shape as `PullProjects` / `PushProject`. Mirror `ConflictError{Sentinel, Current}` for 409.
- `internal/adapter/httpsync/repo_notes_client_test.go` — happy + 409 + 401 paths.
- Modify `internal/adapter/httpsync/queue.go` — add `EnqueueRepo` + `EnqueueRepoNote`.
- Modify `internal/adapter/httpsync/worker.go` — new `drainRepo` + `drainRepoNote`, new `pullReposPage` + `pullRepoNotesPage`, integrate into the loop's switch on `e.Resource`.
- Modify `internal/adapter/httpsync/worker_test.go` — drain + pull tests.

**Create (httpserver):**
- `internal/adapter/httpserver/repos_handlers.go` — `NewReposPullHandler` (GET `/api/v1/repos?since=N&limit=N`), `NewReposPushHandler` (PUT `/api/v1/repos/{id}` with If-Match).
- `internal/adapter/httpserver/repo_notes_handlers.go` — `NewRepoNotesPullHandler` (GET `/api/v1/repo-notes?since=N`), `NewRepoNotePushHandler` (PUT `/api/v1/repos/{repo_id}/note`).
- `internal/adapter/httpserver/repos_handlers_test.go` and `_repo_notes_handlers_test.go` — mirror sessions_handlers_test.

**Modify (httpserver):**
- `internal/adapter/httpserver/server.go` — register the four new routes inside the existing `NewWithAuth` bearer-protected group, gated by nil-checks on the new dep fields.

**Modify (server config + wiring):**
- `cmd/flow-server/main.go` — construct `sqliteserver.NewRepos(store)` and `sqliteserver.NewRepoNotes(store)`, wire into `httpserver.AuthDeps` + the route registration.
- `cmd/flow/main.go` — construct `sqliteclient.NewRepos(cacheStore)` and `sqliteclient.NewRepoNotes(cacheStore)`, wire `usecase.RepoNotes` and the CLI command.

**Create (CLI):**
- `internal/frontend/cli/repo.go` — new `flow repo` subcommand tree with `note get` / `note set --file <path>` / `note edit` (drops user into `$EDITOR` on a tempfile) verbs.
- `internal/frontend/cli/repo_test.go` — exercises the new commands against the fake stores.

**Modify (CLI):**
- `cmd/flow/main.go` — register `cli.NewRepoCmd(...)` next to the existing worktime/sync/projects commands.

**Create (smoke):**
- `scripts/smoke-m4-repo-notes.sh` — extends Plan B's 10-phase E2E with two more phases that exercise `flow repo note set` and `flow repo note get` from device A to device B.
- `docs/runbook/m4-smoke-test.md` — manual smoke instructions (mirror of m2-m3-smoke-test.md).

---

## Task 1: Extend the RepoStore + RepoNoteStore ports

**Files:**
- Modify: `internal/ports/repos.go`
- Modify: `internal/ports/repo_notes.go`

- [ ] **Step 1: Extend RepoStore with PullSince**

```go
// internal/ports/repos.go
type RepoStore interface {
    EnsureByCanonicalKey(userID, key, displayName string) (domain.Repo, error)
    GetByID(userID, id string) (domain.Repo, error)
    Upsert(r domain.Repo) error
    // PullSince returns rows with version > since for userID, ordered by version
    // ASC. hasMore is true when len(out) == limit. The Repo struct gains a
    // Version field as part of this task — see domain.Repo update.
    PullSince(userID string, since int64, limit int) ([]domain.Repo, int64, bool, error)
}

var ErrRepoVersionConflict = errSentinel("flow: repo version conflict")
var ErrRepoNotFound = errSentinel("flow: repo not found")
```

- [ ] **Step 2: Extend RepoNoteStore + add sentinels**

```go
// internal/ports/repo_notes.go
type RepoNoteStore interface {
    GetByRepo(userID, repoID string) (domain.RepoNote, error)
    Upsert(n domain.RepoNote) error
    Delete(userID, id string) error
    PullSince(userID string, since int64, limit int) ([]domain.RepoNote, int64, bool, error)
}

var ErrRepoNoteNotFound = errSentinel("flow: repo note not found")
var ErrRepoNoteVersionConflict = errSentinel("flow: repo note version conflict")
```

- [ ] **Step 3: Add Version field to domain.Repo**

```go
// internal/domain/repo.go
type Repo struct {
    ID           string
    UserID       string
    CanonicalKey string
    DisplayName  string
    CreatedAt    time.Time
    Version      int64 // server-incremented Lamport, 0 until first server roundtrip
}
```

- [ ] **Step 4: Build to verify**

```bash
go build ./...
```

Expected: existing adapter implementations (there are none yet, but the rest of the codebase must still compile) build cleanly.

- [ ] **Step 5: Commit**

```bash
git add internal/ports/repos.go internal/ports/repo_notes.go internal/domain/repo.go
git commit -m "feat(ports): extend RepoStore/RepoNoteStore with PullSince + sentinels"
```

---

## Task 2: sqliteclient.Repos adapter

**Files:**
- Create: `internal/adapter/sqliteclient/repos.go`
- Create: `internal/adapter/sqliteclient/repos_test.go`
- Create: `internal/adapter/sqliteclient/migrations/0003_repos_version.sql`

- [ ] **Step 1: Migration to add version column to repos**

The initial schema (`0001_initial.sql`) already creates `repos` with no `version` column. M4 needs OCC, so add it via migration:

```sql
-- +goose Up
ALTER TABLE repos ADD COLUMN version INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_repos_user_version ON repos(user_id, version);

-- +goose Down
DROP INDEX IF EXISTS idx_repos_user_version;
CREATE TABLE repos_v1 (
    id             TEXT    PRIMARY KEY,
    user_id        TEXT    NOT NULL REFERENCES users(id),
    canonical_key  TEXT    NOT NULL,
    display_name   TEXT    NOT NULL DEFAULT '',
    created_at     TEXT    NOT NULL,
    UNIQUE(user_id, canonical_key)
);
INSERT INTO repos_v1 (id, user_id, canonical_key, display_name, created_at)
SELECT id, user_id, canonical_key, display_name, created_at FROM repos;
DROP TABLE repos;
ALTER TABLE repos_v1 RENAME TO repos;
```

- [ ] **Step 2: Implement Repos adapter**

Mirror `sqliteclient/projects.go` shape (Open/EnsureBy/GetByID/Upsert/PullSince). EnsureByCanonicalKey returns the existing row when (user_id, canonical_key) matches; otherwise inserts a fresh UUID and zero version.

Key methods:

```go
package sqliteclient

import (
    "database/sql"
    "errors"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/serverkraken/flow/internal/domain"
    "github.com/serverkraken/flow/internal/ports"
)

type Repos struct{ store *Store }

var _ ports.RepoStore = (*Repos)(nil)

func NewRepos(s *Store) *Repos { return &Repos{store: s} }

func (r *Repos) EnsureByCanonicalKey(userID, key, displayName string) (domain.Repo, error) {
    // Try GET first; on miss INSERT with UUIDv4 + version=0.
    // Same pattern as Projects.EnsureBySlug.
}

func (r *Repos) GetByID(userID, id string) (domain.Repo, error) {
    // SELECT … WHERE user_id = ? AND id = ?; sql.ErrNoRows → ErrRepoNotFound.
}

func (r *Repos) Upsert(in domain.Repo) error {
    // INSERT … ON CONFLICT(id) DO UPDATE SET display_name, version. No OCC
    // here — sqliteclient mirrors the server, server is the OCC authority.
}

func (r *Repos) PullSince(userID string, since int64, limit int) ([]domain.Repo, int64, bool, error) {
    // SELECT … WHERE user_id = ? AND version > ? ORDER BY version ASC LIMIT ?+1
    // (the +1 detects hasMore).
}
```

- [ ] **Step 3: Tests**

`repos_test.go` covers:
* `TestUnit_Repos_EnsureByCanonicalKey_Idempotent` — second call returns same row, no duplicate.
* `TestUnit_Repos_PullSince_ReturnsOnlyGreaterVersion`
* `TestUnit_Repos_Upsert_ReplacesDisplayName`

- [ ] **Step 4: Run tests + commit**

```bash
go test ./internal/adapter/sqliteclient/... -run TestUnit_Repos -v
git add internal/adapter/sqliteclient/repos.go internal/adapter/sqliteclient/repos_test.go internal/adapter/sqliteclient/migrations/0003_repos_version.sql
git commit -m "feat(sqliteclient): Repos adapter with version + PullSince"
```

---

## Task 3: sqliteclient.RepoNotes adapter

**Files:**
- Create: `internal/adapter/sqliteclient/repo_notes.go`
- Create: `internal/adapter/sqliteclient/repo_notes_test.go`

- [ ] **Step 1: Implement**

The initial schema already has `repo_notes` (id, repo_id, user_id, content, version, updated_at). Same shape as Projects.

```go
type RepoNotes struct{ store *Store }

var _ ports.RepoNoteStore = (*RepoNotes)(nil)

func NewRepoNotes(s *Store) *RepoNotes { return &RepoNotes{store: s} }

func (r *RepoNotes) GetByRepo(userID, repoID string) (domain.RepoNote, error) {
    // SELECT … WHERE user_id = ? AND repo_id = ?; ErrRepoNoteNotFound on no rows.
}

func (r *RepoNotes) Upsert(n domain.RepoNote) error {
    // INSERT … ON CONFLICT(id) DO UPDATE SET content, version, updated_at.
}

func (r *RepoNotes) Delete(userID, id string) error {
    // DELETE WHERE user_id = ? AND id = ?.
}

func (r *RepoNotes) PullSince(userID string, since int64, limit int) ([]domain.RepoNote, int64, bool, error) {
    // SELECT … WHERE user_id = ? AND version > ? ORDER BY version ASC LIMIT ?+1.
}
```

- [ ] **Step 2: Tests**

`repo_notes_test.go` covers Upsert insert/update, Delete, GetByRepo NotFound, PullSince ordering.

- [ ] **Step 3: Run + commit**

```bash
go test ./internal/adapter/sqliteclient/... -run TestUnit_RepoNotes -v
git add internal/adapter/sqliteclient/repo_notes.go internal/adapter/sqliteclient/repo_notes_test.go
git commit -m "feat(sqliteclient): RepoNotes adapter"
```

---

## Task 4: usecase.RepoNotes + canonical-key resolver

**Files:**
- Create: `internal/usecase/canonical_key.go`
- Create: `internal/usecase/canonical_key_test.go`
- Create: `internal/usecase/repo_notes.go`
- Create: `internal/usecase/repo_notes_test.go`

- [ ] **Step 1: CanonicalKey resolver**

```go
package usecase

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "path/filepath"
    "strings"
)

// RemoteResolver returns the git remote URL for the working directory.
// Production wires it to os/exec; tests inject a fake.
type RemoteResolver interface {
    RemoteURL(pwd string) (string, bool)
}

// CanonicalKey returns "git:<host>/<owner>/<repo>" for repos with a remote,
// "path:<sha256-hex>" otherwise. Lowercase host, strips .git suffix, merges
// git@/https:// variants per the M2 spec.
func CanonicalKey(pwd string, resolver RemoteResolver) (string, error) {
    if url, ok := resolver.RemoteURL(pwd); ok {
        return normalizeGitURL(url), nil
    }
    abs, err := filepath.Abs(pwd)
    if err != nil {
        return "", fmt.Errorf("abs: %w", err)
    }
    h := sha256.Sum256([]byte(abs))
    return "path:" + hex.EncodeToString(h[:]), nil
}

func normalizeGitURL(raw string) string {
    s := strings.TrimSpace(raw)
    s = strings.TrimSuffix(s, ".git")
    if strings.HasPrefix(s, "git@") {
        // git@github.com:foo/bar → github.com/foo/bar
        s = strings.Replace(strings.TrimPrefix(s, "git@"), ":", "/", 1)
    }
    s = strings.TrimPrefix(s, "https://")
    s = strings.TrimPrefix(s, "http://")
    s = strings.TrimPrefix(s, "ssh://")
    return "git:" + strings.ToLower(s)
}
```

- [ ] **Step 2: usecase.RepoNotes**

```go
type RepoNotes struct {
    repos    ports.RepoStore
    notes    ports.RepoNoteStore
    queue    ports.WriteQueue
    resolver RemoteResolver
    pushSignal func()
}

func NewRepoNotes(repos ports.RepoStore, notes ports.RepoNoteStore,
    queue ports.WriteQueue, resolver RemoteResolver) *RepoNotes {
    return &RepoNotes{repos: repos, notes: notes, queue: queue, resolver: resolver}
}

func (u *RepoNotes) SetPushSignal(fn func()) { u.pushSignal = fn }

// GetForPwd resolves the canonical key for pwd, ensures the Repo row exists,
// then returns the RepoNote (or zero value + nil error when no note exists).
func (u *RepoNotes) GetForPwd(userID, pwd string) (domain.RepoNote, domain.Repo, error) {
    key, err := CanonicalKey(pwd, u.resolver)
    if err != nil {
        return domain.RepoNote{}, domain.Repo{}, err
    }
    repo, err := u.repos.EnsureByCanonicalKey(userID, key, filepath.Base(pwd))
    if err != nil {
        return domain.RepoNote{}, domain.Repo{}, err
    }
    note, err := u.notes.GetByRepo(userID, repo.ID)
    if errors.Is(err, ports.ErrRepoNoteNotFound) {
        return domain.RepoNote{}, repo, nil // no note yet
    }
    return note, repo, err
}

// Save writes content for the resolved repo, queues a server push.
// Generates a new UUID on first write, otherwise updates the existing row.
func (u *RepoNotes) Save(userID, pwd, content string) (domain.RepoNote, error) {
    _, repo, err := u.GetForPwd(userID, pwd)
    if err != nil {
        return domain.RepoNote{}, err
    }
    existing, _ := u.notes.GetByRepo(userID, repo.ID)
    n := domain.RepoNote{
        ID:        existing.ID,
        RepoID:    repo.ID,
        UserID:    userID,
        Content:   content,
        Version:   existing.Version,
        UpdatedAt: time.Now().UTC(),
    }
    if n.ID == "" {
        n.ID = newUUID()
    }
    if err := u.notes.Upsert(n); err != nil {
        return domain.RepoNote{}, err
    }
    payload, _ := json.Marshal(n)
    _, _ = u.queue.Enqueue("repo_notes", n.ID, payload, n.Version)
    if u.pushSignal != nil { u.pushSignal() }
    return n, nil
}
```

- [ ] **Step 3: Tests**

`repo_notes_test.go` covers:
* `TestUnit_RepoNotes_Save_FirstWriteGeneratesID`
* `TestUnit_RepoNotes_Save_PreservesIDOnUpdate`
* `TestUnit_RepoNotes_GetForPwd_AutoCreatesRepo`
* `TestUnit_RepoNotes_GetForPwd_NoteNotFound_ReturnsZero` (Sentinel-Behavior)

`canonical_key_test.go` covers:
* `git@github.com:foo/bar.git` → `git:github.com/foo/bar`
* `https://github.com/foo/bar` → `git:github.com/foo/bar`
* no remote → `path:<sha256>` with stable hash

- [ ] **Step 4: Run + commit**

```bash
go test ./internal/usecase/... -run "TestUnit_RepoNotes|CanonicalKey" -v
git add internal/usecase/canonical_key.go internal/usecase/canonical_key_test.go internal/usecase/repo_notes.go internal/usecase/repo_notes_test.go
git commit -m "feat(usecase): RepoNotes use case + canonical key resolver"
```

---

## Task 5: sqliteserver migration for repos + repo_notes

**Files:**
- Create: `internal/adapter/sqliteserver/migrations/0003_repo_notes.sql`

- [ ] **Step 1: Migration**

The M2 initial server schema set up users / projects / sessions / active_sessions / lamport but skipped the repo-tables. Add them now:

```sql
-- +goose Up
CREATE TABLE repos (
    id             TEXT    PRIMARY KEY,
    user_id        TEXT    NOT NULL REFERENCES users(id),
    canonical_key  TEXT    NOT NULL,
    display_name   TEXT    NOT NULL DEFAULT '',
    created_at     TEXT    NOT NULL,
    version        INTEGER NOT NULL DEFAULT 0,
    UNIQUE(user_id, canonical_key)
);
CREATE INDEX idx_repos_user_version ON repos(user_id, version);

CREATE TABLE repo_notes (
    id         TEXT    PRIMARY KEY,
    repo_id    TEXT    NOT NULL REFERENCES repos(id),
    user_id    TEXT    NOT NULL REFERENCES users(id),
    content    TEXT    NOT NULL DEFAULT '',
    version    INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT    NOT NULL
);
CREATE INDEX idx_repo_notes_user_version ON repo_notes(user_id, version);
CREATE INDEX idx_repo_notes_repo ON repo_notes(repo_id);

-- +goose Down
DROP INDEX IF EXISTS idx_repo_notes_repo;
DROP INDEX IF EXISTS idx_repo_notes_user_version;
DROP TABLE repo_notes;
DROP INDEX IF EXISTS idx_repos_user_version;
DROP TABLE repos;
```

- [ ] **Step 2: Verify migration runs against a fresh DB**

```bash
go test ./internal/adapter/sqliteserver/... -run TestUnit_Store_OpensFreshDB -v
```

Expected: PASS (the existing store_test confirms migrations run cleanly on a tempfile).

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/sqliteserver/migrations/0003_repo_notes.sql
git commit -m "feat(sqliteserver): migration 0003 — repos + repo_notes tables"
```

---

## Task 6: sqliteserver.Repos adapter

**Files:**
- Create: `internal/adapter/sqliteserver/repos.go`
- Create: `internal/adapter/sqliteserver/repos_test.go`

- [ ] **Step 1: Adapter**

Mirror `sqliteserver/projects.go` exactly — Upsert is OCC + Lamport, PullSince is paged, EnsureByCanonicalKey is idempotent. Type does **not** implement `ports.RepoStore` because the Upsert signature includes `expectedVersion` (same pattern as `sqliteserver.Projects` vs `ports.ProjectStore`).

Public shape:

```go
type Repos struct{ store *Store }

func NewRepos(s *Store) *Repos { return &Repos{store: s} }

func (r *Repos) PullSince(userID string, since int64, limit int) ([]domain.Repo, int64, bool, error)
func (r *Repos) Upsert(in domain.Repo, expectedVersion int64) (domain.Repo, error) // ErrRepoVersionConflict on stale
func (r *Repos) EnsureByCanonicalKey(userID, key, displayName string) (domain.Repo, error)
func (r *Repos) GetByID(userID, id string) (domain.Repo, error)
```

- [ ] **Step 2: Tests**

Mirror `sqliteserver/projects_test.go`:
* Upsert insert + update with matching version
* Upsert stale version → ErrRepoVersionConflict
* PullSince ordering
* EnsureByCanonicalKey idempotent

- [ ] **Step 3: Run + commit**

```bash
go test ./internal/adapter/sqliteserver/... -run TestUnit_ServerRepos -v
git add internal/adapter/sqliteserver/repos.go internal/adapter/sqliteserver/repos_test.go
git commit -m "feat(sqliteserver): Repos with OCC + Lamport"
```

---

## Task 7: sqliteserver.RepoNotes adapter

**Files:**
- Create: `internal/adapter/sqliteserver/repo_notes.go`
- Create: `internal/adapter/sqliteserver/repo_notes_test.go`

- [ ] **Step 1: Adapter**

Same shape as Repos. Upsert validates the row's RepoID exists for the same userID — server-side FK protection. Use `sqliteserver.NextLamport(tx)` for the version bump.

```go
type RepoNotes struct{ store *Store }
func NewRepoNotes(s *Store) *RepoNotes
func (n *RepoNotes) PullSince(userID string, since int64, limit int) ([]domain.RepoNote, int64, bool, error)
func (n *RepoNotes) Upsert(in domain.RepoNote, expectedVersion int64) (domain.RepoNote, error)
func (n *RepoNotes) GetByRepo(userID, repoID string) (domain.RepoNote, error)
```

- [ ] **Step 2: Tests**

* Upsert insert (expectedVersion=0)
* Upsert update with matching version
* Upsert stale version → ErrRepoNoteVersionConflict
* Upsert with bogus repoID → error (FK)
* PullSince ordering

- [ ] **Step 3: Run + commit**

```bash
go test ./internal/adapter/sqliteserver/... -run TestUnit_ServerRepoNotes -v
git add internal/adapter/sqliteserver/repo_notes.go internal/adapter/sqliteserver/repo_notes_test.go
git commit -m "feat(sqliteserver): RepoNotes with OCC + Lamport"
```

---

## Task 8: httpsync client methods for repos + repo_notes

**Files:**
- Modify: `internal/adapter/httpsync/client.go`
- Modify: `internal/adapter/httpsync/client_test.go`

- [ ] **Step 1: Add four methods to Client**

Mirror `PullProjects` + `PushProject` shape exactly. Endpoints from the spec:

```go
func (c *Client) PullRepos(ctx context.Context, since int64, limit int) ([]domain.Repo, int64, bool, error)
func (c *Client) PushRepo(ctx context.Context, r domain.Repo, expectedVersion int64) (int64, error)
func (c *Client) PullRepoNotes(ctx context.Context, since int64, limit int) ([]domain.RepoNote, int64, bool, error)
func (c *Client) PushRepoNote(ctx context.Context, n domain.RepoNote, expectedVersion int64) (int64, error)
```

`PushRepo` returns `*ConflictError{Sentinel: ports.ErrRepoVersionConflict, Current: ...}` on 409. Same for `PushRepoNote` with `ErrRepoNoteVersionConflict`.

- [ ] **Step 2: Tests**

Add httptest-based tests mirroring TestPushSession_409_ConflictError for each new method.

- [ ] **Step 3: Run + commit**

```bash
go test ./internal/adapter/httpsync/... -run "TestPushRepo|TestPullRepo" -v
git add internal/adapter/httpsync/client.go internal/adapter/httpsync/client_test.go
git commit -m "feat(httpsync): PullRepos/PushRepo/PullRepoNotes/PushRepoNote"
```

---

## Task 9: httpsync Queue + Worker for repo_notes

**Files:**
- Modify: `internal/adapter/httpsync/queue.go`
- Modify: `internal/adapter/httpsync/worker.go`
- Modify: `internal/adapter/httpsync/queue_test.go`
- Modify: `internal/adapter/httpsync/worker_test.go`

- [ ] **Step 1: Queue helpers**

```go
// queue.go — append below existing helpers
func (q *Queue) EnqueueRepoNote(n domain.RepoNote, expectedVersion int64) (int64, error) {
    payload, err := json.Marshal(n)
    if err != nil {
        return 0, err
    }
    return q.inner.Enqueue("repo_notes", n.ID, payload, expectedVersion)
}
```

No Enqueue helper for repos — repos are only created via the server's `EnsureByCanonicalKey` path; clients pull them, never push. The first repo a client knows about is the one returned from `repos.EnsureByCanonicalKey(...)` on first save, and that row is pushed implicitly via the `repo_notes` push (server resolves repo from `repo_id` on the note).

Actually, no: the client needs to push the new Repo first so the server has a `repos` row to reference. Add:

```go
func (q *Queue) EnqueueRepo(r domain.Repo, expectedVersion int64) (int64, error) {
    payload, err := json.Marshal(r)
    if err != nil {
        return 0, err
    }
    return q.inner.Enqueue("repos", r.ID, payload, expectedVersion)
}
```

And `usecase.RepoNotes.Save` now enqueues the Repo *before* the RepoNote on first write — both via the existing queue's FIFO drain, so order is preserved.

- [ ] **Step 2: Worker drain handlers**

In `worker.go`, add cases for `"repos"` and `"repo_notes"` to the resource switch inside `runDrain`:

```go
case "repos":
    return w.drainRepo(ctx, e)
case "repo_notes":
    return w.drainRepoNote(ctx, e)
```

```go
func (w *Worker) drainRepo(ctx context.Context, e ports.WriteQueueEntry) (bool, error) {
    var r domain.Repo
    if err := json.Unmarshal(e.Payload, &r); err != nil { return false, err }
    newV, err := w.client.PushRepo(ctx, r, e.ExpectedVersion)
    if errors.Is(err, ports.ErrRepoVersionConflict) {
        w.emitConflictFromError(ctx, "repos", r.ID, e.Seq, r, err)
        return false, nil
    }
    if err != nil { return false, err }
    r.Version = newV
    _ = w.repos.Upsert(r)
    return true, nil
}

func (w *Worker) drainRepoNote(ctx context.Context, e ports.WriteQueueEntry) (bool, error) {
    var n domain.RepoNote
    if err := json.Unmarshal(e.Payload, &n); err != nil { return false, err }
    newV, err := w.client.PushRepoNote(ctx, n, e.ExpectedVersion)
    if errors.Is(err, ports.ErrRepoNoteVersionConflict) {
        w.emitConflictFromError(ctx, "repo_notes", n.ID, e.Seq, n, err)
        return false, nil
    }
    if err != nil { return false, err }
    n.Version = newV
    _ = w.notes.Upsert(n)
    return true, nil
}
```

Worker struct gains `repos ports.RepoStore` and `notes ports.RepoNoteStore` fields, set by an extended `NewWorker` signature. Update both call sites in `cmd/flow/main.go` and any tests.

- [ ] **Step 3: Pull integration**

In `loop` / `runPull`, add `pullReposPage` + `pullRepoNotesPage` mirroring `pullProjectsPage`. Each ingests via the new client's PullSince and upserts into the local store; watermark stored in `sync_state` keyed `"repos"` / `"repo_notes"`.

- [ ] **Step 4: Tests + commit**

```bash
go test ./internal/adapter/httpsync/... -run "TestWorker_DrainRepo|TestWorker_PullRepo" -v
git add internal/adapter/httpsync/queue.go internal/adapter/httpsync/worker.go internal/adapter/httpsync/queue_test.go internal/adapter/httpsync/worker_test.go
git commit -m "feat(httpsync): queue + worker drain/pull for repos and repo_notes"
```

---

## Task 10: httpserver handlers

**Files:**
- Create: `internal/adapter/httpserver/repos_handlers.go`
- Create: `internal/adapter/httpserver/repo_notes_handlers.go`
- Create: `internal/adapter/httpserver/repos_handlers_test.go`
- Create: `internal/adapter/httpserver/repo_notes_handlers_test.go`
- Modify: `internal/adapter/httpserver/server.go`

- [ ] **Step 1: Handlers**

```go
// repos_handlers.go
type ReposServer interface {
    PullSince(userID string, since int64, limit int) ([]domain.Repo, int64, bool, error)
    Upsert(in domain.Repo, expectedVersion int64) (domain.Repo, error)
    GetByID(userID, id string) (domain.Repo, error)
}

func NewReposPullHandler(store ReposServer) http.Handler { /* mirror projects_handlers */ }
func NewReposPushHandler(store ReposServer) http.Handler { /* PUT /api/v1/repos/{id} with If-Match */ }
```

Same shape for `repo_notes_handlers.go` with `RepoNotesServer` interface (PullSince + Upsert + GetByRepo).

- [ ] **Step 2: Register in server.go**

Inside `NewWithAuth`, in the existing `chi.Router.Group` for bearer-protected routes, add nil-guarded blocks:

```go
if d.ReposServer != nil {
    rr.Get("/api/v1/repos", NewReposPullHandler(d.ReposServer).ServeHTTP)
    rr.Put("/api/v1/repos/{id}", NewReposPushHandler(d.ReposServer).ServeHTTP)
}
if d.RepoNotesServer != nil {
    rr.Get("/api/v1/repo-notes", NewRepoNotesPullHandler(d.RepoNotesServer).ServeHTTP)
    rr.Put("/api/v1/repos/{repo_id}/note", NewRepoNotePushHandler(d.RepoNotesServer).ServeHTTP)
}
```

`AuthDeps` gains `ReposServer ReposServer` and `RepoNotesServer RepoNotesServer` fields.

- [ ] **Step 3: Tests**

Mirror `projects_handlers_test.go` for each. Cover happy path, 401 without bearer, 409 with stale If-Match.

- [ ] **Step 4: Run + commit**

```bash
go test ./internal/adapter/httpserver/... -run "Repos|RepoNote" -v
git add internal/adapter/httpserver/repos_handlers.go \
        internal/adapter/httpserver/repos_handlers_test.go \
        internal/adapter/httpserver/repo_notes_handlers.go \
        internal/adapter/httpserver/repo_notes_handlers_test.go \
        internal/adapter/httpserver/server.go
git commit -m "feat(httpserver): /api/v1/repos and /api/v1/repos/{id}/note"
```

---

## Task 11: CLI — `flow repo` subcommand tree

**Files:**
- Create: `internal/frontend/cli/repo.go`
- Create: `internal/frontend/cli/repo_test.go`

- [ ] **Step 1: Implement**

```go
package cli

import (
    "fmt"
    "io"
    "os"
    "os/exec"

    "github.com/spf13/cobra"
    "github.com/serverkraken/flow/internal/usecase"
)

type RepoDeps struct {
    UserID string
    Notes  *usecase.RepoNotes
}

func NewRepoCmd(d RepoDeps) *cobra.Command {
    root := &cobra.Command{Use: "repo", Short: "Repo-Notes verwalten"}
    root.AddCommand(newRepoNoteCmd(d))
    return root
}

func newRepoNoteCmd(d RepoDeps) *cobra.Command {
    note := &cobra.Command{Use: "note", Short: "CLAUDE-style note für das Repo im PWD"}
    note.AddCommand(newRepoNoteGetCmd(d))
    note.AddCommand(newRepoNoteSetCmd(d))
    note.AddCommand(newRepoNoteEditCmd(d))
    return note
}
```

`note get` → prints the resolved note's `Content` to stdout (empty if none exists).
`note set --file X` (or stdin) → reads content, calls `Save`, prints `[saved] <repo-name>`.
`note edit` → opens `$EDITOR` on a tempfile pre-filled with the current content, saves on clean exit.

- [ ] **Step 2: Tests**

Drive against a fake `usecase.RepoNotes` + fake `RemoteResolver`. Verify each verb dispatches correctly and surfaces save errors.

- [ ] **Step 3: Commit**

```bash
go test ./internal/frontend/cli/... -run TestRepo -v
git add internal/frontend/cli/repo.go internal/frontend/cli/repo_test.go
git commit -m "feat(cli): flow repo note get/set/edit"
```

---

## Task 12: Wiring — cmd/flow + cmd/flow-server

**Files:**
- Modify: `cmd/flow/main.go`
- Modify: `cmd/flow-server/main.go`

- [ ] **Step 1: cmd/flow wiring**

After the existing `cacheSessions := sqliteclient.NewSessions(cacheStore)` block, add:

```go
cacheRepos := sqliteclient.NewRepos(cacheStore)
cacheRepoNotes := sqliteclient.NewRepoNotes(cacheStore)

remoteResolver := usecase.NewExecRemoteResolver() // wraps exec.Command("git", "remote", "get-url", "origin")
repoNotesUC := usecase.NewRepoNotes(cacheRepos, cacheRepoNotes, cacheWriteQueue, remoteResolver)
repoNotesUC.SetPushSignal(syncWorker.SignalPush)
```

And in the NewWorker call, thread `cacheRepos` and `cacheRepoNotes`. Register `cli.NewRepoCmd(cli.RepoDeps{UserID: localUser.ID, Notes: repoNotesUC})` next to the existing worktime/sync commands.

- [ ] **Step 2: cmd/flow-server wiring**

After the existing `serverSessions := sqliteserver.NewSessions(serverStore)` block, add:

```go
serverRepos := sqliteserver.NewRepos(serverStore)
serverRepoNotes := sqliteserver.NewRepoNotes(serverStore)
```

And in the `httpserver.AuthDeps{...}` block, add `ReposServer: serverRepos, RepoNotesServer: serverRepoNotes`.

- [ ] **Step 3: Verify**

```bash
go build ./...
make ci
```

Expected: lint + tests green at ≥ 85% coverage.

- [ ] **Step 4: Commit**

```bash
git add cmd/flow/main.go cmd/flow-server/main.go
git commit -m "feat(wiring): M4 — RepoNotes + Repos wired client + server"
```

---

## Task 13: ExecRemoteResolver (small but needs a clean home)

**Files:**
- Create: `internal/adapter/gitremote/resolver.go`
- Create: `internal/adapter/gitremote/resolver_test.go`

- [ ] **Step 1: Implement**

The CanonicalKey resolver in Task 4 takes a `RemoteResolver` interface; the production version lives in an adapter so the use-case layer stays free of `os/exec`.

```go
package gitremote

import (
    "os/exec"
    "strings"
)

type Resolver struct{}

func New() *Resolver { return &Resolver{} }

func (Resolver) RemoteURL(pwd string) (string, bool) {
    cmd := exec.Command("git", "-C", pwd, "remote", "get-url", "origin")
    out, err := cmd.Output()
    if err != nil {
        return "", false
    }
    s := strings.TrimSpace(string(out))
    return s, s != ""
}
```

`usecase.NewExecRemoteResolver()` is then just `gitremote.New()` exposed under a use-case-shaped name. (Or skip the indirection — `usecase.NewRepoNotes` takes the interface, callers pass whatever adapter satisfies it.)

- [ ] **Step 2: Test**

The unit test exec's `git init` in a tempdir, sets a remote, asserts the resolver returns it. Run only when `git` is on PATH (`t.Skip` otherwise).

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/gitremote/
git commit -m "feat(gitremote): exec-based RemoteResolver for CanonicalKey"
```

---

## Task 14: Smoke script extension

**Files:**
- Create: `scripts/smoke-m4-repo-notes.sh`
- Create: `docs/runbook/m4-smoke-test.md`

- [ ] **Step 1: Smoke**

Two new phases on top of the M2/M3 smoke harness:

* Phase 11 — `flow repo note set --file /tmp/note-A.md` on device A inside a tempo git repo with a remote, verify it appears server-side via curl.
* Phase 12 — `flow repo note get` on device B (same tempo git repo cloned with the same remote), verify the content matches.

Runbook documents both phases plus the canonical-key edge cases from the spec ("Repos ohne Remote", "Worktrees").

- [ ] **Step 2: Commit**

```bash
git add scripts/smoke-m4-repo-notes.sh docs/runbook/m4-smoke-test.md
chmod +x scripts/smoke-m4-repo-notes.sh
git commit -m "test(smoke): M4 — RepoNote round-trip smoke script + runbook"
```

---

## Task 15: Verification

- [ ] **Step 1: Full CI**

```bash
make ci
```

Expected: PASS, coverage ≥ 85%. Repo+RepoNote adapters are well-unit-tested so aggregate should hold or rise.

- [ ] **Step 2: Manual smoke** (Soenne's machine)

```bash
scripts/smoke-m4-repo-notes.sh
```

Expected: 12 phases complete green.

- [ ] **Step 3: Memory note**

Update `~/.claude/projects/-Users-msoent-SourceCode-serverkraken-flow/memory/project_plan_b_progress.md` with M4-done status + commit range. Promote MEMORY.md index entry.
