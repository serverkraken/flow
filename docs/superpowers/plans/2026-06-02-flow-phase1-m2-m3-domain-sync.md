# flow Phase 1 — M2 + M3: Domain + Sessions-Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add multi-device worktime sync to flow: rename existing `domain.Project` → `SourceDir`, introduce new Worktime-Project / User / Repo / RepoNote / ActiveSession domain, replace TSV/JSON storage with sqliteclient + sqliteserver via embedded goose migrations, ship REST sync protocol with optimistic-concurrency + conflict overlays, migrate existing TSV history idempotently.

**Architecture:** Hexagonal layering preserved. Two new SQLite adapters (`sqliteclient` per-device, `sqliteserver` server-side) own the new entities. A background sync worker (`httpsync`) drains a local write-queue, polls server every 30s, surfaces conflicts via a Go channel that the TUI subscribes to. The existing tsvsessions adapter is deleted after the one-shot migration command runs.

**Tech Stack:** Go 1.25, `modernc.org/sqlite` (pure-Go), `github.com/pressly/goose/v3` (embedded migrations), `github.com/google/uuid` (UUIDv4 + deterministic v5 for migration idempotency), existing Chi/coreos-go-oidc/zalando-go-keyring/charm.land stack.

**Reference spec:** `docs/superpowers/specs/2026-06-02-flow-phase1-m2-m3-domain-sync-design.md` (commit `6ed4194` on main).

**Prerequisite:** M1 PR merged to main first. This plan starts from `main` after `36d5cda` (or its squashed equivalent post-merge). Do NOT run this in parallel with M1 review — Worktime-Project-rename would conflict.

---

## File Structure

**Renamed:**
```
internal/domain/project.go                → internal/domain/source_dir.go
internal/ports/projects.go                → internal/ports/source_dirs.go
internal/adapter/fsprojects/              → internal/adapter/fssourcedirs/
```
(Plus all callers in `internal/usecase/`, `internal/frontend/`, `cmd/flow/`.)

**New domain files:**
```
internal/domain/
  project.go                              ← NEW (Worktime-Project)
  user.go                                 ← NEW
  repo.go                                 ← NEW
  repo_note.go                            ← NEW
  active_session.go                       ← NEW
  session.go                              ← EXTENDED (ID, UserID, ProjectID, Version, UpdatedAt)
```

**New ports:**
```
internal/ports/
  projects.go                             ← NEW (ProjectStore — note name reused, different package-internal name from old)
  users.go                                ← NEW
  active_sessions.go                      ← NEW
  repos.go                                ← NEW
  repo_notes.go                           ← NEW
  sync.go                                 ← NEW (ConflictMsg + listener)
  sessions.go                             ← EXTENDED signature
```

**New adapters (client-side):**
```
internal/adapter/sqliteclient/
  doc.go
  store.go                                ← Open/Close/pragmas/migrations
  users.go
  projects.go
  sessions.go
  active_sessions.go
  sync_state.go
  write_queue.go
  migrations/
    embed.go
    0001_initial.sql
  *_test.go
```

**New adapters (server-side):**
```
internal/adapter/sqliteserver/
  doc.go
  store.go                                ← Open/Close/pragmas/migrations + lamport-counter
  users.go
  projects.go
  sessions.go
  active_sessions.go                      ← incl. atomic Stop→Session transaction
  lamport.go                              ← next-counter helper
  migrations/
    embed.go
    0001_initial.sql
  *_test.go
```

**New sync engine:**
```
internal/adapter/httpsync/
  doc.go
  client.go                               ← typed REST wrappers
  worker.go                               ← pull-loop + push-drain + conflict channel
  queue.go                                ← write_queue management
  *_test.go
```

**New server handlers (extend M1's httpserver):**
```
internal/adapter/httpserver/
  users_ensure.go                         ← find-or-create user on bearer ingress
  projects_handlers.go
  sessions_handlers.go
  active_sessions_handlers.go
```

**New use cases:**
```
internal/usecase/
  projects.go                             ← List/Create/Rename/Archive
  active_sessions.go                      ← Start/Stop with race-detection
  migrate_tsv.go                          ← one-shot migration
  sync.go                                 ← Status/ForcePull
  sessions.go                             ← REFACTORED (uses ProjectID)
```

**New CLI:**
```
internal/frontend/cli/
  projects/cmd.go                         ← `flow projects list/create/rename/archive`
  sync/cmd.go                             ← `flow sync status/force-pull`
  migrate/cmd.go                          ← `flow worktime migrate-from-tsv`
  worktime/cmd.go                         ← REFACTORED (project resolution)
```

**New TUI components:**
```
internal/frontend/tui/components/
  project_picker/                         ← MRU + fuzzy + create-new
  conflict_overlay/                       ← session-edit + active-session-race variants
```

**Modified TUI screens:**
```
internal/frontend/tui/screen/
  worktime/                               ← picker trigger on `s`, conflict-overlay listener
  projects/                               ← split into SourceDirs + WorktimeProjects sub-tabs
```

**Deleted (after migration):**
```
internal/adapter/tsvsessions/             ← DELETE in Task 34 (post-migration)
```

**Reduced:**
```
internal/adapter/jsonflowstate/           ← Pause-state only after Task 9 (Active-state moves to sqliteclient)
```

---

## Conventions (from M1 + memory)

- Pure Go (no CGo) — `modernc.org/sqlite` is already in deps.
- One responsibility per file, focused packages. No god-files.
- `errors.Is` for sentinels. `doc.go` per package. Compile-time `var _ ports.X = (*Y)(nil)` assertions.
- TDD: failing test → run → impl → run → commit per micro-cycle.
- Conventional Commits with `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` trailer.
- After each task: `go build ./...` and `go test ./...` must be clean.
- No emoji in TUI (memory `feedback_no_icons.md`). Glyphs `▶ ✓ ●` etc are fine.
- `make ci` must pass at the very end (Task 34).

---

## Task index

| # | Phase | What |
| --- | --- | --- |
| 1 | M2a | Rename `domain.Project` → `domain.SourceDir` (+ ports + adapter + callers) |
| 2 | M2b | New domain entities (User, Project, Repo, RepoNote, ActiveSession) + Session extension |
| 3 | M2c | New ports (UserStore, ProjectStore, ActiveSessionStore, RepoStore, RepoNoteStore, ConflictListener) + SessionStore extension |
| 4 | M2d | sqliteclient.Store — Open/Close/pragmas/embedded-goose-migrations |
| 5 | M2d | sqliteclient.Users + sqliteclient.Projects |
| 6 | M2d | sqliteclient.Sessions (CRUD + version handling) |
| 7 | M2d | sqliteclient.ActiveSessions |
| 8 | M2d | sqliteclient.SyncState + sqliteclient.WriteQueue |
| 9 | M2e | Reduce jsonflowstate to Pause-only |
| 10 | M2f | usecase.projects (List/Create/Rename/Archive) |
| 11 | M2f | usecase.sessions refactor for ProjectID |
| 12 | M2f | usecase.active_sessions (Start/Stop with race) |
| 13 | M2f | CLI: `flow projects ...` subcommand |
| 14 | M2f | CLI: `flow worktime start/stop` refactored with project-resolution smart-default |
| 15 | M2f | CLI: `flow sync status / force-pull` |
| 16 | M2g | TUI project_picker component |
| 17 | M2g | TUI worktime screen — wire picker on `s` |
| 18 | M2g | TUI projects screen — SourceDirs + WorktimeProjects sub-tabs |
| 19 | M2h | usecase + CLI `flow worktime migrate-from-tsv` (idempotent) |
| 20 | M3a | sqliteserver.Store + migrations + lamport-counter |
| 21 | M3a | sqliteserver.Users + Projects + Sessions stores |
| 22 | M3a | sqliteserver.ActiveSessions (atomic Stop→Session transaction) |
| 23 | M3b | httpserver users-ensure on bearer ingress |
| 24 | M3b | httpserver projects handlers (GET pull / PUT push) |
| 25 | M3b | httpserver sessions handlers (GET pull / PUT push) |
| 26 | M3b | httpserver active_sessions handlers (GET / POST start / DELETE stop) |
| 27 | M3c | httpsync.Client typed REST wrappers |
| 28 | M3c | httpsync.Queue (write_queue persistence) |
| 29 | M3c | httpsync.Worker (pull/push loop + conflict channel) |
| 30 | M3d | TUI conflict_overlay component |
| 31 | M3d | TUI worktime — subscribe to conflict channel, render overlay |
| 32 | M3e | WIRING: cmd/flow/main.go full assembly |
| 33 | M3e | WIRING: cmd/flow-server/main.go full assembly + handler registration |
| 34 | M3e | SMOKE: multi-device E2E + manual runbook + `make ci` green |

---

## Task 1: Rename `domain.Project` → `domain.SourceDir`

**Why first:** All other tasks reference the new `domain.Project` (worktime category). The existing `domain.Project` (source-dir) must move out of the way first.

**Files:**
- Rename: `internal/domain/project.go` → `internal/domain/source_dir.go`
- Rename: `internal/ports/projects.go` → `internal/ports/source_dirs.go`
- Rename: `internal/adapter/fsprojects/` → `internal/adapter/fssourcedirs/`
- Modify: every caller of `domain.Project`, `ports.ProjectScanner`, `fsprojects.*` — sweep via `rg` and rename.

**Steps:**

- [ ] **Step 1: Inventory all references**

```bash
cd $REPO
rg -l "domain\.Project\b" --type go
rg -l "ports\.ProjectScanner\b" --type go
rg -l "fsprojects\." --type go
rg -l "\"github\.com/serverkraken/flow/internal/adapter/fsprojects\"" --type go
```

Save the list; you'll edit each one.

- [ ] **Step 2: Rename domain file + type**

```bash
git mv internal/domain/project.go internal/domain/source_dir.go
```

Edit `internal/domain/source_dir.go`:
- Replace `type Project struct` with `type SourceDir struct`.
- Update doc-comment from "Project is one entry in the projects screen" to "SourceDir is one entry in the SourceDir / Projects screen — a directory under `$SOURCECODE_ROOT`."

- [ ] **Step 3: Rename ports file + interface**

```bash
git mv internal/ports/projects.go internal/ports/source_dirs.go
```

Edit `internal/ports/source_dirs.go`:
- `type ProjectScanner interface` → `type SourceDirScanner interface`.
- Return type `[]domain.Project` → `[]domain.SourceDir`.
- Update doc-comment to mention SourceDir explicitly.

- [ ] **Step 4: Rename adapter package**

```bash
git mv internal/adapter/fsprojects internal/adapter/fssourcedirs
```

Edit each `.go` in the renamed dir:
- `package fsprojects` → `package fssourcedirs`.
- Any internal references to `domain.Project` → `domain.SourceDir`.
- `var _ ports.ProjectScanner = ...` → `var _ ports.SourceDirScanner = ...`.

- [ ] **Step 5: Sweep callers**

For each file from Step 1:
- `domain.Project` → `domain.SourceDir`
- `ports.ProjectScanner` → `ports.SourceDirScanner`
- `fsprojects.` → `fssourcedirs.`
- Imports updated.

Files likely affected:
- `internal/usecase/projects.go` (or similar) — the use case that lists source-dirs.
- `internal/frontend/tui/screen/projects/*.go` — the TUI screen.
- `internal/frontend/cli/projects/*.go` — CLI handler.
- `cmd/flow/main.go` — wiring.
- `internal/testutil/*.go` — fakes if any.

**Important:** the existing use case may be named `usecase.ProjectsUseCase` or similar — it stays named for now (it's about SourceDirs but renaming it is Task 18's concern). Just fix the type references.

- [ ] **Step 6: Verify build + tests**

```bash
go build ./...
go test ./...
make lint
```

Expected: all green. Any lingering `domain.Project` reference → fix.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor(domain): rename Project → SourceDir (existing source-dir-listing)

Befreit den Namen `Project` für die neue Worktime-Kategorie-Entity in
M2b. SourceDir war was der existing `domain.Project` semantisch immer
war: ein Eintrag im Quellverzeichnis-Listing unter $SOURCECODE_ROOT.

ports.ProjectScanner → ports.SourceDirScanner.
internal/adapter/fsprojects → internal/adapter/fssourcedirs.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: New domain entities + Session extension

**Files:**
- Create: `internal/domain/user.go`
- Create: `internal/domain/project.go` (Worktime-Project, freshly created)
- Create: `internal/domain/repo.go`
- Create: `internal/domain/repo_note.go`
- Create: `internal/domain/active_session.go`
- Modify: `internal/domain/session.go` (add ID, UserID, ProjectID, Version, UpdatedAt)
- Create: `internal/domain/repo_canonical_key.go` (helper for git-remote-URL normalization + path-hash)

**Steps:**

- [ ] **Step 1: Write failing test for Session backward-compat**

Existing tests reference `domain.Session{Date: ..., Start: ..., Stop: ..., Elapsed: ..., Tag: ..., Note: ...}`. New fields are additive; existing literals must keep compiling because of struct-field-init style. But test that the new fields exist via reflection:

`internal/domain/session_extension_test.go`:

```go
package domain

import (
	"reflect"
	"testing"
	"time"
)

func TestUnit_Session_HasNewFields(t *testing.T) {
	t.Parallel()
	s := Session{}
	rv := reflect.TypeOf(s)
	for _, want := range []string{"ID", "UserID", "ProjectID", "Version", "UpdatedAt"} {
		if _, ok := rv.FieldByName(want); !ok {
			t.Errorf("Session is missing field %q", want)
		}
	}
	// existing fields still present
	for _, want := range []string{"Date", "Start", "Stop", "Elapsed", "Tag", "Note"} {
		if _, ok := rv.FieldByName(want); !ok {
			t.Errorf("Session lost legacy field %q", want)
		}
	}
	// Version field is int64
	v, _ := rv.FieldByName("Version")
	if v.Type.Kind() != reflect.Int64 {
		t.Errorf("Version field kind = %s, want int64", v.Type.Kind())
	}
	// UpdatedAt is time.Time
	u, _ := rv.FieldByName("UpdatedAt")
	if u.Type != reflect.TypeOf(time.Time{}) {
		t.Errorf("UpdatedAt type = %v, want time.Time", u.Type)
	}
}
```

- [ ] **Step 2: Run test, expect FAIL**

```bash
go test ./internal/domain/ -run TestUnit_Session_HasNewFields
```
Expected: FAIL (fields missing).

- [ ] **Step 3: Extend Session**

Edit `internal/domain/session.go`:

```go
package domain

import "time"

// Session is a completed work session as logged on disk.
//
// Phase-1 M2 extends the struct with ID (UUID), UserID + ProjectID
// (required for multi-device sync), Version (Lamport per-row from server),
// and UpdatedAt (last mutation timestamp). Legacy callers that build
// Sessions without these fields still compile — fields zero-initialise —
// but the sqliteclient adapter rejects writes with empty UserID/ProjectID.
type Session struct {
	ID        string // UUID v4; legacy TSV rows get UUIDv5(date+start+tag+note) during migration
	UserID    string
	ProjectID string
	Date      time.Time
	Start     time.Time
	Stop      time.Time
	Elapsed   time.Duration
	Tag       string // optional category, e.g. "deep", "meeting"
	Note      string // optional one-line annotation
	Version   int64  // Lamport per row, increments on server-side update
	UpdatedAt time.Time
}
```

- [ ] **Step 4: Add new entities**

`internal/domain/user.go`:

```go
package domain

import "time"

// User is the authenticated identity that owns Projects, Sessions, Repos
// and RepoNotes. One-to-one with an OIDC `sub` claim. Phase 1 ships with
// exactly one User per server instance (the allowlisted owner); Phase 2
// adds multi-user via Authentik group claims.
type User struct {
	ID          string // UUID v4
	OIDCSub     string // unique
	Email       string
	DisplayName string
	CreatedAt   time.Time
}
```

`internal/domain/project.go`:

```go
package domain

import "time"

// Project is a worktime-tracking category — distinct from `SourceDir`
// (file-system project directory). A Session belongs to exactly one
// Project; the TUI picker on `s` sorts Projects MRU-first via LastUsedAt.
//
// Slug is auto-generated from Name (lowercase, ASCII, `-` for spaces) and
// unique per UserID. ArchivedAt is a soft-delete: archived Projects are
// hidden from the picker but their historic Sessions stay intact.
type Project struct {
	ID         string // UUID v4
	UserID     string
	Name       string
	Slug       string
	CreatedAt  time.Time
	LastUsedAt time.Time
	ArchivedAt *time.Time // nil = active
	Version    int64
}
```

`internal/domain/active_session.go`:

```go
package domain

import "time"

// ActiveSession is the in-progress worktime tracker for one (User, Project)
// pair. Multiple may coexist for a single User — Option-2 mode allows
// parallel tracking across Projects. Server-authoritative: clients POST to
// `/api/v1/active/<project-id>/start` and the server decides whether the
// start is allowed (rejected with 409 if another device holds it).
//
// StartedOnDevice is informational only; used by the conflict overlay to
// tell the user where the parallel session is running.
type ActiveSession struct {
	UserID          string
	ProjectID       string
	StartedAt       time.Time
	StartedOnDevice string
	Version         int64 // Optimistic-Concurrency token, server-incremented
}
```

`internal/domain/repo.go`:

```go
package domain

import "time"

// Repo is a git or local-path-identified working directory that can hold
// RepoNotes (cf. M4 / Plan C). The CanonicalKey is what makes the Repo
// addressable across devices:
//
//   git:<host>/<owner>/<repo>   — from `git remote get-url origin`, normalised
//   path:<sha256-hex>           — for repos without a git remote
//
// Two devices that clone the same upstream see the same Repo even when
// the local path differs.
type Repo struct {
	ID           string // UUID v4
	UserID       string
	CanonicalKey string
	DisplayName  string
	CreatedAt    time.Time
}
```

`internal/domain/repo_note.go`:

```go
package domain

import "time"

// RepoNote is a Markdown note tied to a Repo + User. Phase 1 / M4 ships
// one note per (Repo, User) — the canonical "CLAUDE.md for this repo"
// content. The schema allows multiple notes per repo in the future
// (e.g. shared sticky-notes in Phase 2), so RepoNote has its own ID.
type RepoNote struct {
	ID        string
	RepoID    string
	UserID    string
	Content   string
	Version   int64
	UpdatedAt time.Time
}
```

`internal/domain/repo_canonical_key.go`:

```go
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"path/filepath"
	"strings"
)

// RepoCanonicalKeyFromRemote normalises a `git remote get-url origin`
// output into the canonical `git:<host>/<owner>/<repo>` form.
//
// Accepts both SSH (`git@github.com:owner/repo.git`) and HTTPS forms; the
// `.git` suffix is stripped and host/path are lowercased.
//
// Returns an empty string for inputs we can't parse — caller should fall
// back to RepoCanonicalKeyFromPath.
func RepoCanonicalKeyFromRemote(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}

	// SSH form: git@host:owner/repo.git
	if strings.HasPrefix(remote, "git@") {
		rest := strings.TrimPrefix(remote, "git@")
		host, path, ok := strings.Cut(rest, ":")
		if !ok {
			return ""
		}
		path = strings.TrimSuffix(path, ".git")
		return "git:" + strings.ToLower(host) + "/" + path
	}

	// HTTP(S) form
	if u, err := url.Parse(remote); err == nil && u.Host != "" {
		path := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), ".git")
		return "git:" + strings.ToLower(u.Host) + "/" + path
	}

	return ""
}

// RepoCanonicalKeyFromPath returns `path:<sha256-hex>` of the absolute
// path. Used when a directory has no git remote — only the same absolute
// path on the same device matches.
func RepoCanonicalKeyFromPath(absPath string) string {
	clean := filepath.Clean(absPath)
	sum := sha256.Sum256([]byte(clean))
	return "path:" + hex.EncodeToString(sum[:])
}
```

- [ ] **Step 5: Add tests for canonical-key**

`internal/domain/repo_canonical_key_test.go`:

```go
package domain

import "testing"

func TestUnit_RepoCanonicalKeyFromRemote(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"git@github.com:foo/bar.git", "git:github.com/foo/bar"},
		{"git@github.com:foo/bar", "git:github.com/foo/bar"},
		{"https://github.com/Foo/Bar.git", "git:github.com/Foo/Bar"},
		{"https://gitlab.example.com/group/sub/repo", "git:gitlab.example.com/group/sub/repo"},
		{"", ""},
		{"not-a-url", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := RepoCanonicalKeyFromRemote(c.in)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestUnit_RepoCanonicalKeyFromPath_StablePerPath(t *testing.T) {
	t.Parallel()
	a := RepoCanonicalKeyFromPath("/Users/x/code/foo")
	b := RepoCanonicalKeyFromPath("/Users/x/code/foo/")
	c := RepoCanonicalKeyFromPath("/Users/x/code/bar")
	if a == "" {
		t.Fatal("empty key")
	}
	if a != b {
		t.Errorf("clean(path) should normalize trailing slash; %q != %q", a, b)
	}
	if a == c {
		t.Error("different paths should produce different keys")
	}
}
```

- [ ] **Step 6: Run all tests + build**

```bash
go test ./internal/domain/
go build ./...
```

Expected: all pass. New struct fields are zero-valued in existing tests so legacy code still works.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/
git commit -m "$(cat <<'EOF'
feat(domain): add User, Project, Repo, RepoNote, ActiveSession + Session ext

M2b: neue Worktime-Domain für Multi-Device-Sync. Session bekommt
ID/UserID/ProjectID/Version/UpdatedAt additiv — Legacy-Konstruktor-
Stellen kompilieren weiter (zero-init), aber sqliteclient lehnt
Writes mit leerem UserID/ProjectID ab.

Project ist die Worktime-Kategorie (nicht zu verwechseln mit dem in
Task 1 umbenannten SourceDir). Slug ist unique per User, LastUsedAt
treibt die MRU-Sortierung im TUI-Picker, ArchivedAt ist Soft-Delete.

RepoCanonicalKeyFromRemote normalisiert git@/https URLs, FromPath
fallt-back-hash für lokale Pfade ohne Remote.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: New ports + Session port extension

**Files:**
- Create: `internal/ports/users.go`
- Create: `internal/ports/projects.go` (NEW — different from the old one renamed in Task 1)
- Create: `internal/ports/active_sessions.go`
- Create: `internal/ports/repos.go`
- Create: `internal/ports/repo_notes.go`
- Create: `internal/ports/sync.go`
- Modify: `internal/ports/sessions.go` (extend signatures to include UserID + ProjectID filters)

**Steps:**

- [ ] **Step 1: Add UserStore port**

`internal/ports/users.go`:

```go
package ports

import "github.com/serverkraken/flow/internal/domain"

// UserStore persists the locally-cached User row. The client cache holds
// exactly one User (the logged-in account); the server holds multiple.
//
// EnsureBySub is the canonical entry point: given an OIDC `sub` claim
// (Email + DisplayName are best-effort enrichment), it inserts a new row
// or returns the existing one. Adapter-side it's idempotent.
type UserStore interface {
	EnsureBySub(sub, email, displayName string) (domain.User, error)
	GetByID(id string) (domain.User, error)
	GetBySub(sub string) (domain.User, error)
}
```

- [ ] **Step 2: Add ProjectStore port (Worktime)**

`internal/ports/projects.go`:

```go
package ports

import "github.com/serverkraken/flow/internal/domain"

// ProjectStore persists Worktime-Projects. NOT to be confused with
// SourceDirScanner (file-system source directory listing).
//
// ListActive returns non-archived Projects sorted MRU-first (LastUsedAt
// DESC, then CreatedAt DESC) — directly drives the TUI picker.
//
// EnsureBySlug is used by the TSV-migration command and by `flow
// projects create` when the slug doesn't exist yet.
type ProjectStore interface {
	ListActive(userID string) ([]domain.Project, error)
	ListAll(userID string) ([]domain.Project, error) // incl archived
	GetByID(userID, id string) (domain.Project, error)
	GetBySlug(userID, slug string) (domain.Project, error)
	EnsureBySlug(userID, name, slug string) (domain.Project, error)
	Upsert(p domain.Project) error // for sync ingestion
	TouchLastUsed(userID, id string) error
	Archive(userID, id string) error
}
```

- [ ] **Step 3: Add ActiveSessionStore port**

`internal/ports/active_sessions.go`:

```go
package ports

import "github.com/serverkraken/flow/internal/domain"

// ActiveSessionStore tracks in-progress worktime per (User, Project).
//
// On the client, ActiveSessions mirror the server's view: a row exists
// only after the server accepted a Start (and the next pull brought the
// confirmation). Clients should treat the local row as a cache, not
// authoritative.
type ActiveSessionStore interface {
	ListByUser(userID string) ([]domain.ActiveSession, error)
	Get(userID, projectID string) (domain.ActiveSession, error) // ErrActiveSessionNotFound
	Upsert(a domain.ActiveSession) error                        // sync ingestion
	Delete(userID, projectID string) error                      // local-clear after server-side delete
}

var ErrActiveSessionNotFound = errSentinel("flow: active session not found")
```

- [ ] **Step 4: Add RepoStore + RepoNoteStore ports**

`internal/ports/repos.go`:

```go
package ports

import "github.com/serverkraken/flow/internal/domain"

// RepoStore persists Repos. Phase 1 / M2 lays the schema; M4 fills it
// in via the MCP server. Methods kept minimal here.
type RepoStore interface {
	EnsureByCanonicalKey(userID, key, displayName string) (domain.Repo, error)
	GetByID(userID, id string) (domain.Repo, error)
	Upsert(r domain.Repo) error
}
```

`internal/ports/repo_notes.go`:

```go
package ports

import "github.com/serverkraken/flow/internal/domain"

// RepoNoteStore persists RepoNotes. Phase 1 / M2 lays the schema; M4
// drives the actual editing + sync.
type RepoNoteStore interface {
	GetByRepo(userID, repoID string) (domain.RepoNote, error) // ErrRepoNoteNotFound
	Upsert(n domain.RepoNote) error
}

var ErrRepoNoteNotFound = errSentinel("flow: repo note not found")
```

- [ ] **Step 5: Add sync ports**

`internal/ports/sync.go`:

```go
package ports

import "github.com/serverkraken/flow/internal/domain"

// ConflictMsg is delivered to listeners when a push gets 409.
// Listener (the TUI) decides whether to override or accept.
type ConflictMsg struct {
	Resource string // "sessions" | "projects" | "active_sessions"
	RowID    string // ID of the conflicting row
	QueueSeq int64  // write_queue.seq — UI uses this to act on the queue
	Local    any    // local row that failed to push (typed per Resource)
	Server   any    // current server row (typed per Resource)
}

// ConflictListener subscribes to conflict messages. Sync worker pushes
// messages to a channel; UI consumes with a select-loop in the bubbletea
// update.
type ConflictListener interface {
	Conflicts() <-chan ConflictMsg
}

// SyncStatus is the snapshot returned by `flow sync status`.
type SyncStatus struct {
	QueueLen      int
	LastPullAt    string // RFC3339 or "" if never
	LastPullError string // empty if last pull was ok
	Watermarks    map[string]int64
}

// SyncController exposes operator-facing controls (CLI `flow sync ...`).
type SyncController interface {
	Status() (SyncStatus, error)
	ForcePull() error
	AcceptServerVersion(queueSeq int64) error  // resolve conflict: drop local
	OverwriteServerVersion(queueSeq int64) error // resolve conflict: re-push with new expected_version
}

// SyncWatermarkStore — used by the worker to remember per-resource Lamport
// progress.
type SyncWatermarkStore interface {
	Get(resource string) (int64, error)
	Set(resource string, watermark int64) error
}

// WriteQueue persists outgoing mutations until the server confirms.
type WriteQueue interface {
	Enqueue(resource, rowID string, payload []byte, expectedVersion int64) (seq int64, err error)
	Peek(limit int) ([]WriteQueueEntry, error)
	Remove(seq int64) error
	SetError(seq int64, errMsg string) error
}

type WriteQueueEntry struct {
	Seq             int64
	Resource        string
	RowID           string
	Payload         []byte
	ExpectedVersion int64
	EnqueuedAt      string
	LastError       string
}

// Compile-time guard so anyone embedding ConflictMsg into context can
// recover the domain types behind `Local`/`Server` via type assertion.
// Documented in conflict_overlay component.
var _ = domain.Session{}
```

- [ ] **Step 6: Extend SessionStore signature**

Edit `internal/ports/sessions.go`. The existing interface is:

```go
type SessionStore interface {
	LoadAll() ([]domain.Session, error)
	LoadFiltered(keep func(domain.Session) bool) ([]domain.Session, error)
	Append(s domain.Session) error
	AppendBatch(sessions []domain.Session) error
	Rewrite(sessions []domain.Session) error
}
```

Replace with the new shape — the existing TUI/use-case callers still need filtered-load, but the new sqliteclient adapter also needs version-aware writes:

```go
type SessionStore interface {
	// Load returns all sessions for the user, ordered by Date ASC, Start ASC.
	// Replaces the legacy LoadAll() (single-user implicit).
	Load(userID string) ([]domain.Session, error)

	// LoadFiltered returns sessions for which keep returns true.
	LoadFiltered(userID string, keep func(domain.Session) bool) ([]domain.Session, error)

	// Upsert inserts or updates a single Session by ID. Sets version on
	// success. ExpectedVersion is the version the caller believes is
	// currently in the store; pass 0 for new rows. Returns
	// ErrSessionVersionConflict if the stored version differs.
	Upsert(s domain.Session) error

	// UpsertBatch is the multi-row form used by sync ingestion.
	UpsertBatch(sessions []domain.Session) error

	// Delete removes by ID. Used by edit→delete flow in the TUI.
	Delete(userID, id string) error
}

var ErrSessionVersionConflict = errSentinel("flow: session version conflict")
```

The old `Append` / `AppendBatch` / `Rewrite` go away — replaced by `Upsert` / `UpsertBatch` / `Delete`. All callers (use case + sqliteclient) work with IDs from now on.

- [ ] **Step 7: Build + run existing tests**

```bash
go build ./...
go test ./...
```

Many tests will FAIL because the SessionStore signature changed and callers haven't been updated yet. That's expected — Task 11 (sessions use case refactor) will fix them. For now: capture a list of failing tests, expect those failures, ensure no NEW failures unrelated to the signature change.

Strategy: keep the legacy tsvsessions adapter compiling by ALSO implementing the new methods (translate Upsert/Delete to in-memory Append/Rewrite of equivalent rows for now). This keeps the test suite green until Task 9 deletes tsvsessions entirely.

Edit `internal/adapter/tsvsessions/store.go`:

Add shim methods that satisfy the new interface but keep legacy semantics:

```go
// Upsert implements ports.SessionStore. Legacy adapter ignores ID and
// dedupes by (Date, Start). Used during the M2 transition period; this
// adapter is deleted in Task 19 (post-migration).
func (s *Store) Upsert(in domain.Session) error {
	// load, replace or append, rewrite — preserves old semantics
	cur, err := s.LoadAllLegacy()
	if err != nil {
		return err
	}
	idx := -1
	for i := range cur {
		if cur[i].Date.Equal(in.Date) && cur[i].Start.Equal(in.Start) {
			idx = i
			break
		}
	}
	if idx >= 0 {
		cur[idx] = in
	} else {
		cur = append(cur, in)
	}
	return s.Rewrite(cur)
}

func (s *Store) UpsertBatch(in []domain.Session) error {
	for _, ss := range in {
		if err := s.Upsert(ss); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Delete(_userID, _id string) error {
	// legacy adapter cannot delete by ID — silently no-op (deleted by
	// rewriting via Upsert with empty new state). The TUI's Delete path
	// goes through the use case which loads → filters → Rewrite.
	return nil
}

func (s *Store) Load(_userID string) ([]domain.Session, error) {
	return s.LoadAllLegacy()
}

func (s *Store) LoadFiltered(_userID string, keep func(domain.Session) bool) ([]domain.Session, error) {
	all, err := s.LoadAllLegacy()
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, ss := range all {
		if keep(ss) {
			out = append(out, ss)
		}
	}
	return out, nil
}

// LoadAllLegacy is the original LoadAll, kept temporarily for shim.
func (s *Store) LoadAllLegacy() ([]domain.Session, error) {
	// (rename of the existing LoadAll)
}
```

Rename the existing `LoadAll` to `LoadAllLegacy`. Keep `Append`, `AppendBatch`, `Rewrite` working as before — but mark with a doc-comment that they're legacy and the new use cases call `Upsert*`.

- [ ] **Step 8: Verify build + tests after shim**

```bash
go build ./...
go test ./...
```

Now should be GREEN — tsvsessions shim satisfies both old + new interface.

- [ ] **Step 9: Commit**

```bash
git add internal/ports/ internal/adapter/tsvsessions/
git commit -m "$(cat <<'EOF'
feat(ports): new ports (User/Project/ActiveSession/Repo/RepoNote/sync) + Session ext

M2c: alle Interfaces für die neue Worktime-Domain. SessionStore-Signatur
wird auf Upsert/Delete (ID-basiert + Version) umgestellt; tsvsessions
kriegt einen Shim damit der Tree grün bleibt bis Task 19 ihn löscht.

ConflictMsg ist resource-agnostisch (Local/Server als any) — TUI cast
type-aware. WriteQueue + SyncWatermarkStore werden später von der
sqliteclient implementiert. SyncController ist die Operator-Schnittstelle
für `flow sync ...` (Task 15).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: sqliteclient.Store — Open/Close/pragmas/embedded-goose-migrations

**Files:**
- Create: `internal/adapter/sqliteclient/doc.go`
- Create: `internal/adapter/sqliteclient/store.go`
- Create: `internal/adapter/sqliteclient/migrations/embed.go`
- Create: `internal/adapter/sqliteclient/migrations/0001_initial.sql`
- Create: `internal/adapter/sqliteclient/store_test.go`

**Steps:**

- [ ] **Step 1: Add goose dep**

```bash
go get github.com/pressly/goose/v3
```

- [ ] **Step 2: Create the SQL migration**

`internal/adapter/sqliteclient/migrations/0001_initial.sql`:

```sql
-- +goose Up
CREATE TABLE users (
    id            TEXT    PRIMARY KEY,
    oidc_sub      TEXT    NOT NULL UNIQUE,
    email         TEXT    NOT NULL DEFAULT '',
    display_name  TEXT    NOT NULL DEFAULT '',
    created_at    TEXT    NOT NULL
);

CREATE TABLE projects (
    id            TEXT    PRIMARY KEY,
    user_id       TEXT    NOT NULL REFERENCES users(id),
    name          TEXT    NOT NULL,
    slug          TEXT    NOT NULL,
    created_at    TEXT    NOT NULL,
    last_used_at  TEXT    NOT NULL DEFAULT '',
    archived_at   TEXT,
    version       INTEGER NOT NULL DEFAULT 0,
    UNIQUE(user_id, slug)
);
CREATE INDEX idx_projects_user_last_used ON projects(user_id, last_used_at DESC);

CREATE TABLE sessions (
    id            TEXT    PRIMARY KEY,
    user_id       TEXT    NOT NULL REFERENCES users(id),
    project_id    TEXT    NOT NULL REFERENCES projects(id),
    date          TEXT    NOT NULL,
    start         TEXT    NOT NULL,
    stop          TEXT    NOT NULL,
    elapsed_ns    INTEGER NOT NULL,
    tag           TEXT    NOT NULL DEFAULT '',
    note          TEXT    NOT NULL DEFAULT '',
    version       INTEGER NOT NULL DEFAULT 0,
    updated_at    TEXT    NOT NULL
);
CREATE INDEX idx_sessions_user_date ON sessions(user_id, date);
CREATE INDEX idx_sessions_user_project ON sessions(user_id, project_id);
CREATE INDEX idx_sessions_version ON sessions(version);

CREATE TABLE active_sessions (
    user_id            TEXT    NOT NULL REFERENCES users(id),
    project_id         TEXT    NOT NULL REFERENCES projects(id),
    started_at         TEXT    NOT NULL,
    started_on_device  TEXT    NOT NULL,
    version            INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, project_id)
);

CREATE TABLE repos (
    id             TEXT    PRIMARY KEY,
    user_id        TEXT    NOT NULL REFERENCES users(id),
    canonical_key  TEXT    NOT NULL,
    display_name   TEXT    NOT NULL DEFAULT '',
    created_at     TEXT    NOT NULL,
    UNIQUE(user_id, canonical_key)
);

CREATE TABLE repo_notes (
    id         TEXT    PRIMARY KEY,
    repo_id    TEXT    NOT NULL REFERENCES repos(id),
    user_id    TEXT    NOT NULL REFERENCES users(id),
    content    TEXT    NOT NULL DEFAULT '',
    version    INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT    NOT NULL
);
CREATE INDEX idx_repo_notes_user ON repo_notes(user_id);

CREATE TABLE sync_state (
    resource    TEXT    PRIMARY KEY,
    watermark   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE write_queue (
    seq               INTEGER PRIMARY KEY AUTOINCREMENT,
    resource          TEXT    NOT NULL,
    row_id            TEXT    NOT NULL,
    payload           TEXT    NOT NULL,
    expected_version  INTEGER NOT NULL,
    enqueued_at       TEXT    NOT NULL,
    last_error        TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX idx_write_queue_resource ON write_queue(resource);

-- +goose Down
DROP TABLE write_queue;
DROP TABLE sync_state;
DROP TABLE repo_notes;
DROP TABLE repos;
DROP TABLE active_sessions;
DROP TABLE sessions;
DROP TABLE projects;
DROP TABLE users;
```

- [ ] **Step 3: Create embed loader**

`internal/adapter/sqliteclient/migrations/embed.go`:

```go
// Package migrations embeds the client-side SQL migrations.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 4: Implement Store**

`internal/adapter/sqliteclient/doc.go`:

```go
// Package sqliteclient is the per-device local cache backing flow's
// worktime + repo-note data.
package sqliteclient
```

`internal/adapter/sqliteclient/store.go`:

```go
package sqliteclient

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/serverkraken/flow/internal/adapter/sqliteclient/migrations"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqliteclient open: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqliteclient ping: %w", err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqliteclient dialect: %w", err)
	}
	if err := goose.Up(db, "."); err != nil && !errors.Is(err, goose.ErrNoNextVersion) {
		_ = db.Close()
		return nil, fmt.Errorf("sqliteclient migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}
```

- [ ] **Step 5: Tests**

`internal/adapter/sqliteclient/store_test.go`:

```go
package sqliteclient

import (
	"path/filepath"
	"testing"
)

func TestUnit_OpenStore_CreatesAllTables(t *testing.T) {
	t.Parallel()
	s := mustOpen(t)
	for _, name := range []string{"users", "projects", "sessions", "active_sessions", "repos", "repo_notes", "sync_state", "write_queue"} {
		var got string
		row := s.DB().QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name)
		if err := row.Scan(&got); err != nil {
			t.Errorf("table %q missing: %v", name, err)
		}
	}
}

func TestUnit_OpenStore_Reentrant(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "client.db")
	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	_ = s1.Close()
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	_ = s2.Close()
}

func TestUnit_OpenStore_ForeignKeysEnabled(t *testing.T) {
	t.Parallel()
	s := mustOpen(t)
	var on int
	if err := s.DB().QueryRow("PRAGMA foreign_keys").Scan(&on); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if on != 1 {
		t.Errorf("foreign_keys = %d", on)
	}
}

func mustOpen(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "client.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
```

- [ ] **Step 6: Run + commit**

```bash
go test ./internal/adapter/sqliteclient/ -count=1
git add internal/adapter/sqliteclient/ go.mod go.sum
git commit -m "feat(sqliteclient): Store + embedded goose migrations + full M2 schema

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: sqliteclient.Users + Projects

**Files:**
- Create: `internal/adapter/sqliteclient/users.go` + `_test.go`
- Create: `internal/adapter/sqliteclient/projects.go` + `_test.go`
- Modify: `internal/ports/users.go` + `internal/ports/projects.go` — add `ErrUserNotFound` / `ErrProjectNotFound` sentinels.

**Steps:**

- [ ] **Step 1: Add sentinels to ports**

Append to `internal/ports/users.go`:
```go
var ErrUserNotFound = errSentinel("flow: user not found")
```
Append to `internal/ports/projects.go`:
```go
var ErrProjectNotFound = errSentinel("flow: project not found")
```

- [ ] **Step 2: Implement Users**

`internal/adapter/sqliteclient/users.go`:

```go
package sqliteclient

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type Users struct{ store *Store }

func NewUsers(store *Store) *Users { return &Users{store: store} }

func (u *Users) EnsureBySub(sub, email, displayName string) (domain.User, error) {
	got, err := u.GetBySub(sub)
	if err == nil {
		return got, nil
	}
	if !errors.Is(err, ports.ErrUserNotFound) {
		return domain.User{}, err
	}
	row := domain.User{
		ID: uuid.NewString(), OIDCSub: sub, Email: email,
		DisplayName: displayName, CreatedAt: time.Now().UTC(),
	}
	_, err = u.store.DB().Exec(
		`INSERT INTO users(id, oidc_sub, email, display_name, created_at) VALUES (?, ?, ?, ?, ?)`,
		row.ID, row.OIDCSub, row.Email, row.DisplayName, row.CreatedAt.Format(time.RFC3339))
	return row, err
}

func (u *Users) GetByID(id string) (domain.User, error) {
	return u.scanOne(`SELECT id, oidc_sub, email, display_name, created_at FROM users WHERE id = ?`, id)
}

func (u *Users) GetBySub(sub string) (domain.User, error) {
	return u.scanOne(`SELECT id, oidc_sub, email, display_name, created_at FROM users WHERE oidc_sub = ?`, sub)
}

func (u *Users) scanOne(q string, arg any) (domain.User, error) {
	var out domain.User
	var createdAt string
	err := u.store.DB().QueryRow(q, arg).Scan(&out.ID, &out.OIDCSub, &out.Email, &out.DisplayName, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, ports.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, err
	}
	out.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return out, nil
}

var _ ports.UserStore = (*Users)(nil)
```

- [ ] **Step 3: Implement Projects**

`internal/adapter/sqliteclient/projects.go`:

```go
package sqliteclient

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type Projects struct{ store *Store }

func NewProjects(store *Store) *Projects { return &Projects{store: store} }

func (p *Projects) ListActive(userID string) ([]domain.Project, error) { return p.list(userID, false) }
func (p *Projects) ListAll(userID string) ([]domain.Project, error)    { return p.list(userID, true) }

func (p *Projects) list(userID string, includeArchived bool) ([]domain.Project, error) {
	q := `SELECT id, user_id, name, slug, created_at, last_used_at, archived_at, version FROM projects WHERE user_id = ?`
	if !includeArchived {
		q += ` AND archived_at IS NULL`
	}
	q += ` ORDER BY last_used_at DESC, created_at DESC`
	rows, err := p.store.DB().Query(q, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Project
	for rows.Next() {
		pr, err := scanProjectRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

func (p *Projects) GetByID(userID, id string) (domain.Project, error) {
	return scanProjectRow(p.store.DB().QueryRow(
		`SELECT id, user_id, name, slug, created_at, last_used_at, archived_at, version FROM projects WHERE user_id = ? AND id = ?`,
		userID, id))
}

func (p *Projects) GetBySlug(userID, slug string) (domain.Project, error) {
	return scanProjectRow(p.store.DB().QueryRow(
		`SELECT id, user_id, name, slug, created_at, last_used_at, archived_at, version FROM projects WHERE user_id = ? AND slug = ?`,
		userID, slug))
}

func (p *Projects) EnsureBySlug(userID, name, slug string) (domain.Project, error) {
	got, err := p.GetBySlug(userID, slug)
	if err == nil {
		return got, nil
	}
	if !errors.Is(err, ports.ErrProjectNotFound) {
		return domain.Project{}, err
	}
	pr := domain.Project{
		ID: uuid.NewString(), UserID: userID, Name: name, Slug: slug,
		CreatedAt: time.Now().UTC(),
	}
	if err := p.Upsert(pr); err != nil {
		return domain.Project{}, err
	}
	return pr, nil
}

func (p *Projects) Upsert(pr domain.Project) error {
	var archived any
	if pr.ArchivedAt != nil {
		archived = pr.ArchivedAt.Format(time.RFC3339)
	}
	var lastUsed string
	if !pr.LastUsedAt.IsZero() {
		lastUsed = pr.LastUsedAt.Format(time.RFC3339Nano)
	}
	_, err := p.store.DB().Exec(`
		INSERT INTO projects (id, user_id, name, slug, created_at, last_used_at, archived_at, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name, slug = excluded.slug,
			last_used_at = excluded.last_used_at,
			archived_at = excluded.archived_at, version = excluded.version`,
		pr.ID, pr.UserID, pr.Name, pr.Slug,
		pr.CreatedAt.Format(time.RFC3339), lastUsed, archived, pr.Version)
	return err
}

func (p *Projects) TouchLastUsed(userID, id string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := p.store.DB().Exec(`UPDATE projects SET last_used_at = ? WHERE user_id = ? AND id = ?`, now, userID, id)
	return err
}

func (p *Projects) Archive(userID, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := p.store.DB().Exec(`UPDATE projects SET archived_at = ? WHERE user_id = ? AND id = ?`, now, userID, id)
	return err
}

type rowScanner interface{ Scan(...any) error }

func scanProjectRow(r rowScanner) (domain.Project, error) {
	var pr domain.Project
	var createdAt, lastUsedAt string
	var archivedAt sql.NullString
	err := r.Scan(&pr.ID, &pr.UserID, &pr.Name, &pr.Slug, &createdAt, &lastUsedAt, &archivedAt, &pr.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Project{}, ports.ErrProjectNotFound
	}
	if err != nil {
		return domain.Project{}, err
	}
	pr.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if lastUsedAt != "" {
		pr.LastUsedAt, _ = time.Parse(time.RFC3339Nano, lastUsedAt)
		if pr.LastUsedAt.IsZero() {
			pr.LastUsedAt, _ = time.Parse(time.RFC3339, lastUsedAt)
		}
	}
	if archivedAt.Valid {
		t, _ := time.Parse(time.RFC3339, archivedAt.String)
		pr.ArchivedAt = &t
	}
	return pr, nil
}

var _ ports.ProjectStore = (*Projects)(nil)
```

- [ ] **Step 4: Tests for both adapters**

`internal/adapter/sqliteclient/users_test.go` — Test EnsureBySub idempotency + GetBySub-not-found.
`internal/adapter/sqliteclient/projects_test.go` — Test EnsureBySlug idempotency, ListActive MRU sort, Archive hides from ListActive but not ListAll, Upsert insert-then-update.

(See spec § DB Schema for behavior these tests anchor.)

- [ ] **Step 5: Run + commit**

```bash
go test ./internal/adapter/sqliteclient/ -count=1
git add internal/adapter/sqliteclient/users*.go internal/adapter/sqliteclient/projects*.go internal/ports/
git commit -m "feat(sqliteclient): Users + Projects adapters

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: sqliteclient.Sessions

**Files:**
- Create: `internal/adapter/sqliteclient/sessions.go` + `_test.go`

**Implementation Outline:**

`internal/adapter/sqliteclient/sessions.go` implements `ports.SessionStore`:
- `Load(userID)` — SELECT ordered by date ASC, start ASC
- `LoadFiltered(userID, keep)` — Load + in-memory filter
- `Upsert(s)` — INSERT…ON CONFLICT(id) DO UPDATE; requires non-empty ID + UserID + ProjectID
- `UpsertBatch(ss)` — transactional bulk for sync ingestion
- `Delete(userID, id)` — DELETE WHERE

Use same scan helper pattern as Projects. Store dates as `"2006-01-02"`, timestamps as RFC3339, elapsed as int64-nanoseconds.

Tests cover: insert-then-update preserves version semantics, LoadFiltered project-filter, Delete removes row, Load sort order across multi-day rows.

Commit message: `feat(sqliteclient): Sessions adapter with version-aware Upsert`.

---

## Task 7: sqliteclient.ActiveSessions

**Files:**
- Create: `internal/adapter/sqliteclient/active_sessions.go` + `_test.go`

**Implementation Outline:**

Implements `ports.ActiveSessionStore`:
- `ListByUser(userID)` — SELECT WHERE user_id = ?
- `Get(userID, projectID)` — SELECT … LIMIT 1 → `ErrActiveSessionNotFound` on no rows
- `Upsert(a)` — INSERT…ON CONFLICT(user_id, project_id) DO UPDATE for sync ingestion
- `Delete(userID, projectID)` — DELETE WHERE

Tests cover: Upsert insert + replace, Delete + Get returns ErrActiveSessionNotFound, ListByUser returns multiple parallel-active per Option-2.

Commit: `feat(sqliteclient): ActiveSessions adapter`.

---

## Task 8: sqliteclient.SyncState + WriteQueue

**Files:**
- Create: `internal/adapter/sqliteclient/sync_state.go` + `_test.go`
- Create: `internal/adapter/sqliteclient/write_queue.go` + `_test.go`

**SyncState:**

Implements `ports.SyncWatermarkStore`:
- `Get(resource)` → SELECT watermark FROM sync_state WHERE resource = ?; on no rows return 0 + nil error (no-watermark-yet is normal).
- `Set(resource, w)` → INSERT…ON CONFLICT(resource) DO UPDATE.

**WriteQueue:**

Implements `ports.WriteQueue`:
- `Enqueue` → INSERT and RETURNING seq (or last_insert_rowid()).
- `Peek(limit)` → SELECT … ORDER BY seq ASC LIMIT ?.
- `Remove(seq)` → DELETE WHERE seq = ?.
- `SetError(seq, msg)` → UPDATE last_error WHERE seq = ?.

Payload is `[]byte` JSON; stored as TEXT.

Tests cover: FIFO order via Peek, Remove drops row, SetError records, multi-Enqueue gets monotonic seqs.

Commit: `feat(sqliteclient): SyncState + WriteQueue adapters`.

---

## Task 9: Reduce jsonflowstate to Pause-state only

**Files:**
- Modify: `internal/adapter/jsonflowstate/store.go` — drop ActiveSessionStore methods (`GetActive` / `SetActive` / `ClearActive`).
- Modify: `internal/ports/sessions.go` — split `ActiveSessionStore` (the old single-active-state interface) is replaced by `ports.ActiveSessionStore` (Task 3); the legacy interface from `ports/sessions.go` goes away. Keep `Lock`.
- Modify: callers — use case + CLI references to ActiveSessionStore (old).
- Modify: `cmd/flow/main.go` wiring — stop constructing jsonflowstate ActiveSession-store-half.

**Steps:**

- [ ] **Step 1: Inventory legacy ActiveSessionStore callers**

```bash
rg "GetActive\(\)|SetActive\(|ClearActive\(\)" --type go
rg "ports\.ActiveSessionStore\b" --type go
```

The Task-3 commit added a NEW `ports.ActiveSessionStore` in a new file (`active_sessions.go`). The OLD one lives in `ports/sessions.go`. Identify each caller and route to the new store via the use case (Task 12).

- [ ] **Step 2: Remove legacy ActiveSessionStore from `ports/sessions.go`**

Delete the `ActiveSessionStore` interface (the one for active timestamps + pause markers). Keep only `SessionStore` (extended in Task 3) and `Lock`.

The pause-marker methods (`GetPause` / `SetPause` / `ClearPause`) move to a separate `ports.PauseStore` interface in a new file:

`internal/ports/pause.go`:

```go
package ports

import "time"

// PauseStore is per-device (never synced) — a tiny marker for the
// worktime pause flow. Lives in jsonflowstate; not part of the multi-
// device sync model.
type PauseStore interface {
	GetPause() (*time.Time, error)
	SetPause(t time.Time) error
	ClearPause() error
}
```

- [ ] **Step 3: Reduce jsonflowstate**

Edit `internal/adapter/jsonflowstate/store.go` (and any companion files):
- Drop methods `GetActive`, `SetActive`, `ClearActive`.
- Keep `GetPause`, `SetPause`, `ClearPause`.
- Compile-time assertion: `var _ ports.PauseStore = (*Store)(nil)`.
- Rename type `Store` → `PauseStore` if it's not used elsewhere as `Store`.

- [ ] **Step 4: Update use-case + CLI callers**

Anywhere that called `flowstate.GetActive()` now needs to call the active_sessions usecase (Task 12) — but Task 12 isn't ready yet. So **temporarily** in this task: leave callers stubbed with a `panic("active-session use case wiring is Task 12")` or guard via `// TODO Task 12`.

Actually, cleaner: don't break callers yet. Keep the old behaviour available behind a thin shim that calls into a `_legacy` package, OR move the legacy active-state to a sub-package so existing TUI keeps compiling until Task 12 rewires.

**Pragmatic choice:** keep `internal/adapter/jsonflowstate/legacy_active.go` that retains the old methods for now, with a `// Deprecated: removed in Task 12 of Plan B` comment. Task 12 will delete it.

- [ ] **Step 5: Build + tests**

```bash
go build ./...
go test ./...
```

Should be GREEN — legacy shim keeps the tree compiling.

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/jsonflowstate/ internal/ports/
git commit -m "refactor(jsonflowstate): isolate Pause-state, legacy-shim active-state

PauseStore-Interface in eigenem ports-File. Active-State-Methoden bleiben
übergangsweise als legacy-Shim bis Task 12 sie auf ports.ActiveSessionStore
(neu) umstellt.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: usecase.projects (List/Create/Rename/Archive)

**Files:**
- Create: `internal/usecase/projects.go` + `_test.go`

**Implementation:**

```go
package usecase

import (
	"errors"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Projects orchestrates Worktime-Project operations: list (driving the
// TUI picker), create (incl. inline-create from the picker), rename,
// archive. Calls TouchLastUsed when a Session starts (called via
// usecase.ActiveSessions.Start, Task 12).
type Projects struct {
	users    ports.UserStore
	projects ports.ProjectStore
}

func NewProjects(users ports.UserStore, projects ports.ProjectStore) *Projects {
	return &Projects{users: users, projects: projects}
}

// ListActive returns active Projects MRU-first, used by the TUI picker.
func (p *Projects) ListActive(userID string) ([]domain.Project, error) {
	return p.projects.ListActive(userID)
}

// Create creates a new Project with auto-generated slug.
//
// Slug rules: lowercase ASCII, spaces → "-", non-[a-z0-9-] stripped,
// collapsed dashes. If the slug collides with an existing one for this
// User, suffix "-2", "-3", … until unique.
func (p *Projects) Create(userID, name string) (domain.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Project{}, errors.New("project name required")
	}
	base := SlugFromName(name)
	slug := base
	i := 2
	for {
		_, err := p.projects.GetBySlug(userID, slug)
		if errors.Is(err, ports.ErrProjectNotFound) {
			break
		}
		if err != nil {
			return domain.Project{}, err
		}
		slug = base + "-" + intToStr(i)
		i++
	}
	return p.projects.EnsureBySlug(userID, name, slug)
}

// Rename changes the human-readable name only — slug stays stable.
func (p *Projects) Rename(userID, id, newName string) error {
	pr, err := p.projects.GetByID(userID, id)
	if err != nil {
		return err
	}
	pr.Name = strings.TrimSpace(newName)
	pr.Version++ // local optimistic bump; server may overwrite
	return p.projects.Upsert(pr)
}

// Archive soft-deletes a Project.
func (p *Projects) Archive(userID, id string) error {
	return p.projects.Archive(userID, id)
}

// MarkUsedNow updates LastUsedAt — called from active_sessions.Start.
func (p *Projects) MarkUsedNow(userID, id string) error {
	return p.projects.TouchLastUsed(userID, id)
}

// SlugFromName is the canonical slug-generation. Exposed so the picker
// can preview "the slug we'd assign" for inline-create.
func SlugFromName(name string) string {
	var sb strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			sb.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_':
			if !prevDash && sb.Len() > 0 {
				sb.WriteRune('-')
				prevDash = true
			}
		}
	}
	s := strings.TrimRight(sb.String(), "-")
	if s == "" {
		s = "unnamed"
	}
	return s
}

func intToStr(i int) string {
	// avoid importing fmt for one int — and avoid strconv just to be safe-import
	// (use strconv)
	return string(rune('0'+i)) // (placeholder — see note)
}

// Tiny note: the intToStr above is a placeholder. Real impl uses strconv.Itoa.
// In your edit, replace with: `import "strconv"` and `return strconv.Itoa(i)`.
```

Important: When implementing, use `strconv.Itoa(i)`. The placeholder above is illustrative.

**Tests** cover: SlugFromName for German/Umlaut/punctuation (e.g. "Flow!" → "flow", "Mein Projekt" → "mein-projekt"), Create slug-collision-suffix, Rename keeps slug stable, Archive marks ArchivedAt non-nil.

Commit: `feat(usecase): Projects use case (List/Create/Rename/Archive)`.

---

## Task 11: usecase.sessions refactor for ProjectID

**Files:**
- Modify: `internal/usecase/sessions.go` (existing file from current codebase — read first to understand current shape)
- Modify: tests under `internal/usecase/sessions_test.go`

**Steps:**

- [ ] **Step 1: Read existing use case**

```bash
cat internal/usecase/sessions.go | head -200
```

Identify which methods need to change. Common ones: `StartSession`, `StopSession`, `Aggregate`, `EditSession`, `DeleteSession`.

- [ ] **Step 2: Refactor signatures**

Every method that touched a Session now also takes (or derives) `userID` + `projectID`. The use case constructor takes `ports.UserStore` + `ports.ProjectStore` + `ports.SessionStore` + `ports.ActiveSessionStore`.

New canonical operations:

```go
// ResolveProject implements the smart-default cascade from the spec:
// explicit projectID → $PWD → SourceDirScanner → MRU → "Allgemein" auto-create.
func (s *Sessions) ResolveProject(userID, explicitID, pwd string) (domain.Project, error) { … }

// Edit replaces the row identified by id with the new shape; bumps version.
func (s *Sessions) Edit(userID, id string, mutate func(*domain.Session)) error { … }

// Delete removes by id.
func (s *Sessions) Delete(userID, id string) error { … }

// Aggregate returns saldo / streaks for a date range — existing logic, but
// queries via SessionStore.LoadFiltered(userID, ...) instead of LoadAll.
func (s *Sessions) Aggregate(userID string, from, to time.Time) (domain.Aggregate, error) { … }
```

Project resolution (smart-default-fallback):
1. If `explicitID != ""` → GetByID; error if missing.
2. Else $PWD → SourceDirScanner.List → match by name → ProjectStore.GetBySlug(slug-of-sourceDir-name) → if found use, else continue.
3. Else ProjectStore.ListActive[0] (MRU first) → use.
4. Else ProjectStore.EnsureBySlug(userID, "Allgemein", "allgemein").

- [ ] **Step 3: Update tests**

Existing tests likely use `Append` / `Rewrite`; switch to `Upsert` / `Delete`. Add test for `ResolveProject` covering all 4 cascade branches with fakes.

- [ ] **Step 4: Commit**

```bash
git add internal/usecase/sessions*.go
git commit -m "refactor(usecase): sessions use case on Upsert/Delete + ResolveProject

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 12: usecase.active_sessions (Start/Stop with race-detection)

**Files:**
- Create: `internal/usecase/active_sessions.go` + `_test.go`
- Modify: callers — remove jsonflowstate-legacy-shim references.

**Implementation:**

```go
package usecase

import (
	"errors"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ActiveSessions orchestrates Start/Stop with optimistic-concurrency
// against the local ActiveSessionStore. Push to server happens via the
// httpsync write_queue (the queue's payload is the new ActiveSession row
// JSON; sync worker handles the 409 race).
type ActiveSessions struct {
	users          ports.UserStore
	projects       ports.ProjectStore
	active         ports.ActiveSessionStore
	sessions       ports.SessionStore
	queue          ports.WriteQueue
	device         string // hostname; informational in ActiveSession.StartedOnDevice
}

func NewActiveSessions(
	users ports.UserStore, projects ports.ProjectStore,
	active ports.ActiveSessionStore, sessions ports.SessionStore,
	queue ports.WriteQueue,
) *ActiveSessions {
	host, _ := os.Hostname()
	return &ActiveSessions{
		users: users, projects: projects, active: active,
		sessions: sessions, queue: queue, device: host,
	}
}

// Start records an ActiveSession locally and queues a server-start POST.
// In option-2 mode, parallel ActiveSessions across different Projects are
// allowed; this method assumes the caller (CLI or TUI picker) already
// resolved exactly one ProjectID.
//
// If an ActiveSession for (userID, projectID) already exists locally,
// returns ErrActiveSessionExists — caller shows the conflict overlay.
func (a *ActiveSessions) Start(userID, projectID string) (domain.ActiveSession, error) {
	if _, err := a.active.Get(userID, projectID); err == nil {
		return domain.ActiveSession{}, ErrActiveSessionExists
	} else if !errors.Is(err, ports.ErrActiveSessionNotFound) {
		return domain.ActiveSession{}, err
	}
	row := domain.ActiveSession{
		UserID:          userID,
		ProjectID:       projectID,
		StartedAt:       time.Now().UTC(),
		StartedOnDevice: a.device,
		Version:         0, // server assigns
	}
	if err := a.active.Upsert(row); err != nil {
		return domain.ActiveSession{}, err
	}
	if err := a.projects.TouchLastUsed(userID, projectID); err != nil {
		return domain.ActiveSession{}, err
	}
	payload := encodeActiveStart(row)
	if _, err := a.queue.Enqueue("active_sessions", projectID, payload, 0); err != nil {
		return domain.ActiveSession{}, err
	}
	return row, nil
}

// Stop closes the ActiveSession, creates a finished Session row locally,
// and queues a DELETE to the server. The server's atomic Stop-transaction
// will create the canonical Session row server-side; the next pull
// reconciles (local row gets replaced with server-version row).
func (a *ActiveSessions) Stop(userID, projectID, tag, note string) (domain.Session, error) {
	cur, err := a.active.Get(userID, projectID)
	if err != nil {
		return domain.Session{}, err
	}
	now := time.Now().UTC()
	sess := domain.Session{
		ID:        uuid.NewString(),
		UserID:    userID,
		ProjectID: projectID,
		Date:      cur.StartedAt.Truncate(24 * time.Hour),
		Start:     cur.StartedAt,
		Stop:      now,
		Elapsed:   now.Sub(cur.StartedAt),
		Tag:       tag,
		Note:      note,
		Version:   0,
		UpdatedAt: now,
	}
	if err := a.sessions.Upsert(sess); err != nil {
		return domain.Session{}, err
	}
	if err := a.active.Delete(userID, projectID); err != nil {
		return domain.Session{}, err
	}
	// queue both: session push + active-stop signal
	if payload, _ := encodeSession(sess); payload != nil {
		_, _ = a.queue.Enqueue("sessions", sess.ID, payload, 0)
	}
	_, _ = a.queue.Enqueue("active_sessions_stop", projectID, []byte(`{"action":"stop","version":`+i64(cur.Version)+`}`), cur.Version)
	return sess, nil
}

// ListActive returns currently running sessions across all projects.
func (a *ActiveSessions) ListActive(userID string) ([]domain.ActiveSession, error) {
	return a.active.ListByUser(userID)
}

// ForceTakeover is what the conflict-overlay calls on `[t]` press — it
// requeues the start with If-Match: <current-server-version> instead of 0.
func (a *ActiveSessions) ForceTakeover(userID, projectID string, currentServerVersion int64) error {
	row := domain.ActiveSession{
		UserID: userID, ProjectID: projectID,
		StartedAt: time.Now().UTC(), StartedOnDevice: a.device,
	}
	if err := a.active.Upsert(row); err != nil {
		return err
	}
	_, err := a.queue.Enqueue("active_sessions", projectID, encodeActiveStart(row), currentServerVersion)
	return err
}

var ErrActiveSessionExists = errors.New("flow: active session for this project already exists")

// Helpers (i64, encodeActiveStart, encodeSession) — implement using encoding/json.
// Define in the same file.

func i64(v int64) string { /* strconv.FormatInt(v,10) */ return "" }
func encodeActiveStart(_ domain.ActiveSession) []byte { /* json.Marshal */ return nil }
func encodeSession(_ domain.Session) ([]byte, error)  { /* json.Marshal */ return nil, nil }
```

Replace the placeholder helpers with real `encoding/json`-Marshal calls and `strconv.FormatInt`.

**Tests:**
- Happy-path Start → ActiveSession row appears in store, write-queue has one entry, Project's LastUsedAt bumped.
- Start when already-running → returns ErrActiveSessionExists.
- Stop → Session row created, ActiveSession deleted, two queue entries.
- ForceTakeover → queue entry has expectedVersion > 0.

Commit: `feat(usecase): ActiveSessions use case (Start/Stop/ForceTakeover/ListActive)`.

After this task: delete the jsonflowstate legacy-shim file from Task 9.

---

## Task 13: CLI `flow projects ...`

**Files:**
- Create: `internal/frontend/cli/projects/cmd.go`
- Modify: `cmd/flow/main.go` to register `newProjectsCmd(deps)`.

**Subcommands:**
- `flow projects list [--archived]` — prints table (name, slug, last-used, sessions-count).
- `flow projects create <name>` — usecase.Projects.Create, prints created project's slug.
- `flow projects rename <slug-or-id> <new-name>` — finds by slug-or-id, then Rename.
- `flow projects archive <slug-or-id>` — soft-delete.

Use cobra, follow existing CLI conventions (see `internal/frontend/cli/worktime/` for style).

Commit: `feat(cli): flow projects subcommand (list/create/rename/archive)`.

---

## Task 14: CLI `flow worktime start/stop` refactor

**Files:**
- Modify: `internal/frontend/cli/worktime/cmd.go` (and companions)
- Add: smart-default project resolution using `usecase.Sessions.ResolveProject`.

**Signature changes:**
- `flow worktime start [--project=slug-or-id] [--tag=...] [--note=...]`
- `flow worktime stop [--project=slug-or-id] [--tag=...] [--note=...]`

Smart-default cascade (from spec § CLI/TUI-Integration):
1. `--project=foo` if set → resolve to ID via `usecase.Projects.GetBySlug` or `GetByID`.
2. `$PWD` → SourceDirScanner.List → match by name → `Projects.GetBySlug(slug-of-name)`. If found, use.
3. `Projects.ListActive[0]` (MRU first).
4. `Projects.EnsureBySlug(userID, "Allgemein", "allgemein")` — auto-create on demand.

**Important wiring guard:** before any worktime command runs, check if `~/.tmux/worktime.log` exists AND `~/.flow/cache.db` is empty (no Session rows) → emit "TSV detected, run `flow worktime migrate-from-tsv` first" and exit non-zero.

Commit: `refactor(cli): flow worktime start/stop on new ProjectID resolution + TSV guard`.

---

## Task 15: CLI `flow sync status / force-pull`

**Files:**
- Create: `internal/frontend/cli/sync/cmd.go`
- Use `ports.SyncController` from Task 3.

Subcommands:
- `flow sync status` — prints WriteQueue length, LastPullAt, LastPullError, per-resource watermarks.
- `flow sync force-pull` — calls SyncController.ForcePull().

Implementation reads from sqliteclient.SyncState + WriteQueue directly via a tiny `usecase.SyncStatus` adapter that wraps them. Real SyncController (with goroutine + channel) comes in Task 29; until then the CLI surfaces "sync worker not yet wired (Task 32)".

Commit: `feat(cli): flow sync status + force-pull subcommand`.

---

## Task 16: TUI project_picker component

**Files:**
- Create: `internal/frontend/tui/components/project_picker/`
  - `doc.go`
  - `model.go` — bubbletea Model
  - `update.go` — Update msg → cmd
  - `view.go` — View() string
  - `styles.go` — theme.Palette-driven
  - `*_test.go`

**Component contract:**

```go
package project_picker

import (
	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Model is a bubbletea-screen-level component: full TUI take-over until
// the user picks a Project or cancels. Reuse the markdown_overlay chrome
// pattern (centered box, esc to close, single hot-key bindings).
type Model struct {
	items    []domain.Project
	filter   string
	cursor   int
	palette  theme.Palette
	onPick   func(domain.Project) tea.Msg     // injected
	onCreate func(name string) tea.Msg        // emits CreateProjectMsg
	onCancel tea.Msg                          // single msg value
}

func New(items []domain.Project, p theme.Palette, onPick func(domain.Project) tea.Msg, onCreate func(string) tea.Msg, onCancel tea.Msg) Model { … }

// Update handles keyboard nav (up/down/tab), filter (rune input), enter
// (pick or create-new), esc (cancel). Returns tea.Msg via onPick/onCreate
// when the user commits.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) { … }

func (m Model) View() string { … }
```

**Filter behavior:** type-as-you-go fuzzy-match using `github.com/sahilm/fuzzy` (already in deps). Score-sort filtered list. The sticky "+ Neues Projekt anlegen" entry is always last and not filtered out.

**Keybindings:** `↑/↓` or `j/k` navigate; `enter` selects; `tab` jumps to "+ Neu"; `esc` cancels; printable runes append to filter; `backspace` removes last filter rune.

**Tests:** filter narrows list, cursor wraps, enter on "+ Neu" with non-empty filter creates project with that filter as name, enter on a row emits onPick.

Commit: `feat(tui): project_picker component (MRU + fuzzy + inline-create)`.

---

## Task 17: TUI worktime screen — wire picker on `s`

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/` model + update

**Behavior:**
- Press `s` (start) when no ActiveSession running → opens project_picker.
- Picker emits `PickedMsg{ProjectID}` → worktime invokes `usecase.ActiveSessions.Start(userID, projectID)`.
- Picker emits `CreateProjectMsg{Name}` → worktime invokes `usecase.Projects.Create(userID, name)` → on success invokes Start with the new ID.
- Picker emits CancelMsg → screen restores previous focus.

Press `s` when ActiveSession is running → no-op (or status hint "already running"). Actually correct behavior: open picker, but exclude already-running projects from the list (or grey them out) so user can start a parallel Project session.

**Active-Session display:** the existing "running" indicator becomes a list (Option-2 mode allows multiple). Render as `▶ flow 2h 30m · Allgemein 0h 12m` in the status header.

Commit: `feat(tui): worktime — wire project_picker on s-press`.

---

## Task 18: TUI projects screen — sub-tabs

**Files:**
- Modify: `internal/frontend/tui/screen/projects/` model + view

**Two sub-tabs:**
- "Quellverzeichnisse" — existing SourceDir-listing (uses SourceDirScanner).
- "Worktime-Projekte" — list from `usecase.Projects.ListActive` + ListAll (archived toggle); per-row: name, last-used, session-count (computed cheaply via `SELECT COUNT(*) FROM sessions WHERE project_id = ?`).

Use existing sub-tab pattern from `worktime` screen (today/woche/verlauf/frei).

Worktime-Projekte sub-tab keybindings: `n` new (opens picker-like name-input), `r` rename, `a` archive, `enter` → switch to worktime/verlauf filtered to that project.

Per memory `feedback_navigation_discoverability_over_minimalism.md`: keep both sub-tabs visible in the strip even when one is active.

Commit: `feat(tui): projects screen — Quellverzeichnisse + Worktime-Projekte sub-tabs`.

---

## Task 19: TSV-Migration use case + CLI

**Files:**
- Create: `internal/usecase/migrate_tsv.go` + `_test.go`
- Create: `internal/frontend/cli/migrate/cmd.go`
- Modify: `cmd/flow/main.go` to register `flow worktime migrate-from-tsv` subcommand
- Delete (at end of this task): `internal/adapter/tsvsessions/` package + the legacy ActiveStore-shim in jsonflowstate

**Use case:**

```go
package usecase

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// MigrateTSV reads the legacy ~/.tmux/worktime.log, maps every row to a
// Session in the new SQLite store, and renames the TSV to .migrated-<ts>.
//
// Idempotency: each row's UUID is UUIDv5(namespace, date|start|tag|note).
// Re-running the migration produces the same UUIDs → INSERT ON CONFLICT
// becomes a no-op. The rename-to-.migrated suffix prevents the legacy
// adapter from being asked to load again, but if a caller deletes the
// suffix and reruns: still idempotent.
type MigrateTSV struct {
	users    ports.UserStore
	projects ports.ProjectStore
	sessions ports.SessionStore
}

func NewMigrateTSV(u ports.UserStore, p ports.ProjectStore, s ports.SessionStore) *MigrateTSV {
	return &MigrateTSV{users: u, projects: p, sessions: s}
}

type MigrateResult struct {
	Inserted        int
	SkippedExisting int
	DefaultProject  domain.Project
	ArchivedTo      string
}

// migrationNamespace is a fixed namespace UUID used for deterministic
// UUIDv5 generation. Never change once Plan B ships.
var migrationNamespace = uuid.MustParse("a9c8b5d2-7e3f-4d1e-9c0a-1234567890ab")

func (m *MigrateTSV) Run(userID, tsvPath, defaultProjectName string) (MigrateResult, error) {
	if defaultProjectName == "" {
		defaultProjectName = "Allgemein"
	}
	defaultSlug := SlugFromName(defaultProjectName)

	defaultProj, err := m.projects.EnsureBySlug(userID, defaultProjectName, defaultSlug)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("ensure default project: %w", err)
	}

	f, err := os.Open(tsvPath)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("open tsv: %w", err)
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(bufio.NewReader(f))
	r.Comma = '\t'
	r.FieldsPerRecord = -1

	result := MigrateResult{DefaultProject: defaultProj}
	now := time.Now().UTC()

	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return result, fmt.Errorf("read tsv: %w", err)
		}
		if len(rec) < 4 {
			continue // skip malformed
		}
		// TSV format: date \t start \t stop \t elapsed [\t tag [\t note]]
		date, _ := time.Parse("2006-01-02", strings.TrimSpace(rec[0]))
		start, _ := time.Parse(time.RFC3339, strings.TrimSpace(rec[1]))
		stop, _ := time.Parse(time.RFC3339, strings.TrimSpace(rec[2]))
		var elapsed time.Duration
		if d, perr := time.ParseDuration(strings.TrimSpace(rec[3])); perr == nil {
			elapsed = d
		} else {
			elapsed = stop.Sub(start)
		}
		tag, note := "", ""
		if len(rec) > 4 {
			tag = strings.TrimSpace(rec[4])
		}
		if len(rec) > 5 {
			note = strings.TrimSpace(rec[5])
		}

		// Deterministic UUID
		keyBytes := []byte(date.Format("2006-01-02") + "|" + start.Format(time.RFC3339) + "|" + tag + "|" + note)
		id := uuid.NewSHA1(migrationNamespace, keyBytes).String()

		sess := domain.Session{
			ID: id, UserID: userID, ProjectID: defaultProj.ID,
			Date: date, Start: start, Stop: stop, Elapsed: elapsed,
			Tag: tag, Note: note, Version: 0, UpdatedAt: now,
		}
		if err := m.sessions.Upsert(sess); err != nil {
			return result, fmt.Errorf("upsert row: %w", err)
		}
		result.Inserted++
	}

	// Rename the TSV
	archived := tsvPath + ".migrated-" + now.Format("2006-01-02T15-04-05Z")
	if err := os.Rename(tsvPath, archived); err != nil {
		// non-fatal: log and continue
		result.ArchivedTo = ""
	} else {
		result.ArchivedTo = archived
	}
	_ = filepath.Clean // keep import
	return result, nil
}
```

**Tests:**
- Empty TSV → 0 inserted, default project created.
- TSV with 3 rows → 3 inserted, all bound to default project.
- Re-run on same TSV-file (without rename in between) → same row count, no duplicates.
- TSV with malformed lines → skipped, others inserted.
- Different tags → still go to default project (per spec: "alle in Allgemein" by default).

**CLI:** `internal/frontend/cli/migrate/cmd.go`:

```go
package migrate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/usecase"
)

func NewCmd(deps Deps) *cobra.Command {
	var (
		tsvPath           string
		defaultProjectName string
	)
	cmd := &cobra.Command{
		Use:   "migrate-from-tsv",
		Short: "TSV-Worktime-History in den neuen SQLite-Store importieren",
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, _ := os.UserHomeDir()
			if tsvPath == "" {
				tsvPath = filepath.Join(home, ".tmux", "worktime.log")
			}
			user, err := deps.Users.GetBySub(deps.SubResolver())
			if err != nil {
				return fmt.Errorf("user not found — run `flow login` first: %w", err)
			}
			mt := usecase.NewMigrateTSV(deps.Users, deps.Projects, deps.Sessions)
			res, err := mt.Run(user.ID, tsvPath, defaultProjectName)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"✓ %d Sessions migriert in Projekt %q (slug %q).\n",
				res.Inserted, res.DefaultProject.Name, res.DefaultProject.Slug)
			if res.ArchivedTo != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  TSV archiviert nach %s\n", res.ArchivedTo)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&tsvPath, "tsv", "", "TSV-Dateipfad (default ~/.tmux/worktime.log)")
	cmd.Flags().StringVar(&defaultProjectName, "default-project", "Allgemein", "Projekt-Name für untagged Sessions")
	return cmd
}

type Deps struct {
	Users       ports.UserStore
	Projects    ports.ProjectStore
	Sessions    ports.SessionStore
	SubResolver func() string // reads OIDC sub from keychain token
}
```

Register as `flow worktime migrate-from-tsv` under the worktime subcommand tree (so it's discoverable next to start/stop).

**After tests pass:** delete `internal/adapter/tsvsessions/` and the `legacy_active.go` shim in jsonflowstate.

```bash
git rm -r internal/adapter/tsvsessions/
git rm internal/adapter/jsonflowstate/legacy_active.go
```

Now nothing implements the old `ActiveSessionStore` from `ports/sessions.go` — which was already removed in Task 9.

Commit: `feat(usecase): TSV→SQLite migration + delete legacy tsvsessions adapter`.

---

## Task 20: sqliteserver.Store + migrations + lamport counter

**Files:**
- Create: `internal/adapter/sqliteserver/doc.go`
- Create: `internal/adapter/sqliteserver/store.go`
- Create: `internal/adapter/sqliteserver/lamport.go` + `_test.go`
- Create: `internal/adapter/sqliteserver/migrations/embed.go`
- Create: `internal/adapter/sqliteserver/migrations/0001_initial.sql`
- Create: `internal/adapter/sqliteserver/store_test.go`

**Steps:**

- [ ] **Step 1: SQL migration**

`internal/adapter/sqliteserver/migrations/0001_initial.sql`:

```sql
-- +goose Up
CREATE TABLE users (
    id            TEXT    PRIMARY KEY,
    oidc_sub      TEXT    NOT NULL UNIQUE,
    email         TEXT    NOT NULL DEFAULT '',
    display_name  TEXT    NOT NULL DEFAULT '',
    created_at    TEXT    NOT NULL
);

CREATE TABLE projects (
    id            TEXT    PRIMARY KEY,
    user_id       TEXT    NOT NULL REFERENCES users(id),
    name          TEXT    NOT NULL,
    slug          TEXT    NOT NULL,
    created_at    TEXT    NOT NULL,
    last_used_at  TEXT    NOT NULL DEFAULT '',
    archived_at   TEXT,
    version       INTEGER NOT NULL DEFAULT 0,
    UNIQUE(user_id, slug)
);
CREATE INDEX idx_projects_user_version ON projects(user_id, version);

CREATE TABLE sessions (
    id            TEXT    PRIMARY KEY,
    user_id       TEXT    NOT NULL REFERENCES users(id),
    project_id    TEXT    NOT NULL REFERENCES projects(id),
    date          TEXT    NOT NULL,
    start         TEXT    NOT NULL,
    stop          TEXT    NOT NULL,
    elapsed_ns    INTEGER NOT NULL,
    tag           TEXT    NOT NULL DEFAULT '',
    note          TEXT    NOT NULL DEFAULT '',
    version       INTEGER NOT NULL DEFAULT 0,
    updated_at    TEXT    NOT NULL
);
CREATE INDEX idx_sessions_user_version ON sessions(user_id, version);
CREATE INDEX idx_sessions_user_date ON sessions(user_id, date);

CREATE TABLE active_sessions (
    user_id            TEXT    NOT NULL REFERENCES users(id),
    project_id         TEXT    NOT NULL REFERENCES projects(id),
    started_at         TEXT    NOT NULL,
    started_on_device  TEXT    NOT NULL,
    version            INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, project_id)
);

CREATE TABLE repos (
    id             TEXT    PRIMARY KEY,
    user_id        TEXT    NOT NULL REFERENCES users(id),
    canonical_key  TEXT    NOT NULL,
    display_name   TEXT    NOT NULL DEFAULT '',
    created_at     TEXT    NOT NULL,
    UNIQUE(user_id, canonical_key)
);

CREATE TABLE repo_notes (
    id         TEXT    PRIMARY KEY,
    repo_id    TEXT    NOT NULL REFERENCES repos(id),
    user_id    TEXT    NOT NULL REFERENCES users(id),
    content    TEXT    NOT NULL DEFAULT '',
    version    INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT    NOT NULL
);

CREATE TABLE lamport (
    id        INTEGER PRIMARY KEY CHECK (id = 1),
    counter   INTEGER NOT NULL DEFAULT 0
);
INSERT INTO lamport(id, counter) VALUES (1, 0);

-- +goose Down
DROP TABLE lamport;
DROP TABLE repo_notes;
DROP TABLE repos;
DROP TABLE active_sessions;
DROP TABLE sessions;
DROP TABLE projects;
DROP TABLE users;
```

- [ ] **Step 2: embed.go + doc.go**

`internal/adapter/sqliteserver/migrations/embed.go`:

```go
// Package migrations embeds the server-side SQL migrations.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

`internal/adapter/sqliteserver/doc.go`:

```go
// Package sqliteserver is flow-server's central SQLite store. Holds
// multi-user data (Phase 1 single-user via allowlist, schema is multi-
// user-ready). Mutations increment a global lamport counter; the
// counter value becomes the row's `version` and is what clients use to
// pull-watermark.
package sqliteserver
```

- [ ] **Step 3: Store**

`internal/adapter/sqliteserver/store.go`:

```go
package sqliteserver

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/serverkraken/flow/internal/adapter/sqliteserver/migrations"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqliteserver open: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqliteserver ping: %w", err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqliteserver dialect: %w", err)
	}
	if err := goose.Up(db, "."); err != nil && !errors.Is(err, goose.ErrNoNextVersion) {
		_ = db.Close()
		return nil, fmt.Errorf("sqliteserver migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}
```

- [ ] **Step 4: Lamport counter**

`internal/adapter/sqliteserver/lamport.go`:

```go
package sqliteserver

import "database/sql"

// NextLamport atomically increments the global counter and returns the new
// value. Must be called inside a transaction by the caller; the helper
// uses the supplied *sql.Tx so the increment + the row-update share one
// commit boundary.
func NextLamport(tx *sql.Tx) (int64, error) {
	if _, err := tx.Exec(`UPDATE lamport SET counter = counter + 1 WHERE id = 1`); err != nil {
		return 0, err
	}
	var v int64
	if err := tx.QueryRow(`SELECT counter FROM lamport WHERE id = 1`).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}
```

- [ ] **Step 5: Tests**

`internal/adapter/sqliteserver/lamport_test.go`:

```go
package sqliteserver

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestUnit_NextLamport_Monotonic(t *testing.T) {
	t.Parallel()
	s := mustOpenServer(t)

	prev := int64(0)
	for i := 0; i < 10; i++ {
		tx, _ := s.DB().Begin()
		v, err := NextLamport(tx)
		if err != nil {
			t.Fatalf("NextLamport: %v", err)
		}
		_ = tx.Commit()
		if v <= prev {
			t.Errorf("v = %d, want > %d", v, prev)
		}
		prev = v
	}
}

func TestUnit_NextLamport_ConcurrentCallers_NoSkip(t *testing.T) {
	t.Parallel()
	s := mustOpenServer(t)
	var wg sync.WaitGroup
	got := make(chan int64, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, _ := s.DB().Begin()
			v, _ := NextLamport(tx)
			_ = tx.Commit()
			got <- v
		}()
	}
	wg.Wait()
	close(got)
	seen := map[int64]bool{}
	for v := range got {
		if seen[v] {
			t.Errorf("duplicate lamport %d", v)
		}
		seen[v] = true
	}
}

func mustOpenServer(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "server.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
```

- [ ] **Step 6: Commit**

```bash
go test ./internal/adapter/sqliteserver/ -count=1
git add internal/adapter/sqliteserver/
git commit -m "feat(sqliteserver): Store + migrations + atomic lamport-counter helper

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 21: sqliteserver.Users + Projects + Sessions stores

**Files:**
- Create: `internal/adapter/sqliteserver/users.go` + `_test.go`
- Create: `internal/adapter/sqliteserver/projects.go` + `_test.go`
- Create: `internal/adapter/sqliteserver/sessions.go` + `_test.go`

**Pattern (same as sqliteclient but server-side):**

Each Upsert runs in a transaction and bumps `version` via `NextLamport(tx)`. Reads (List/Get) are non-transactional.

`internal/adapter/sqliteserver/sessions.go` example skeleton:

```go
package sqliteserver

import (
	"database/sql"
	"errors"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type Sessions struct{ store *Store }

func NewSessions(s *Store) *Sessions { return &Sessions{store: s} }

// PullSince returns rows with version > watermark, ordered by version ASC.
func (s *Sessions) PullSince(userID string, since int64, limit int) ([]domain.Session, int64, bool, error) {
	rows, err := s.store.DB().Query(`
		SELECT id, user_id, project_id, date, start, stop, elapsed_ns, tag, note, version, updated_at
		FROM sessions WHERE user_id = ? AND version > ?
		ORDER BY version ASC LIMIT ?`, userID, since, limit+1)
	if err != nil {
		return nil, since, false, err
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Session
	for rows.Next() {
		ss, err := scanServerSession(rows)
		if err != nil {
			return nil, since, false, err
		}
		out = append(out, ss)
	}
	hasMore := false
	if len(out) > limit {
		out = out[:limit]
		hasMore = true
	}
	high := since
	if len(out) > 0 {
		high = out[len(out)-1].Version
	}
	return out, high, hasMore, nil
}

// Upsert applies the row with optimistic concurrency: if a row with the
// same id exists and stored.version != expectedVersion → 409. Else inserts
// or updates, bumping version via NextLamport.
func (s *Sessions) Upsert(in domain.Session, expectedVersion int64) (domain.Session, error) {
	tx, err := s.store.DB().Begin()
	if err != nil {
		return domain.Session{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var curVersion int64
	row := tx.QueryRow(`SELECT version FROM sessions WHERE id = ?`, in.ID)
	switch err := row.Scan(&curVersion); {
	case errors.Is(err, sql.ErrNoRows):
		if expectedVersion != 0 {
			return domain.Session{}, ports.ErrSessionVersionConflict
		}
	case err != nil:
		return domain.Session{}, err
	default:
		if curVersion != expectedVersion {
			return domain.Session{}, ports.ErrSessionVersionConflict
		}
	}

	v, err := NextLamport(tx)
	if err != nil {
		return domain.Session{}, err
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(`
		INSERT INTO sessions (id, user_id, project_id, date, start, stop, elapsed_ns, tag, note, version, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_id = excluded.project_id,
			date = excluded.date, start = excluded.start, stop = excluded.stop,
			elapsed_ns = excluded.elapsed_ns, tag = excluded.tag, note = excluded.note,
			version = excluded.version, updated_at = excluded.updated_at`,
		in.ID, in.UserID, in.ProjectID,
		in.Date.Format("2006-01-02"),
		in.Start.Format(time.RFC3339), in.Stop.Format(time.RFC3339),
		int64(in.Elapsed), in.Tag, in.Note, v, now.Format(time.RFC3339),
	); err != nil {
		return domain.Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Session{}, err
	}
	in.Version = v
	in.UpdatedAt = now
	return in, nil
}

func scanServerSession(r interface{ Scan(...any) error }) (domain.Session, error) {
	var s domain.Session
	var dateStr, startStr, stopStr, updStr string
	var elapsedNs int64
	if err := r.Scan(&s.ID, &s.UserID, &s.ProjectID, &dateStr, &startStr, &stopStr, &elapsedNs, &s.Tag, &s.Note, &s.Version, &updStr); err != nil {
		return domain.Session{}, err
	}
	s.Date, _ = time.Parse("2006-01-02", dateStr)
	s.Start, _ = time.Parse(time.RFC3339, startStr)
	s.Stop, _ = time.Parse(time.RFC3339, stopStr)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updStr)
	s.Elapsed = time.Duration(elapsedNs)
	return s, nil
}
```

Apply the **same pattern** to `internal/adapter/sqliteserver/users.go` (EnsureBySub finds-or-creates, no version), `projects.go` (PullSince + Upsert with optimistic concurrency).

**Tests cover:** PullSince returns only versions > since, hasMore true when more than limit; Upsert insert (expectedVersion 0) succeeds, Upsert update with wrong expectedVersion returns ErrSessionVersionConflict; concurrent Upserts get distinct versions.

Commit: `feat(sqliteserver): Users + Projects + Sessions stores with optimistic concurrency`.

---

## Task 22: sqliteserver.ActiveSessions (atomic Stop transaction)

**Files:**
- Create: `internal/adapter/sqliteserver/active_sessions.go` + `_test.go`

**Special-case methods:**

```go
package sqliteserver

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type ActiveSessions struct{ store *Store }

func NewActiveSessions(s *Store) *ActiveSessions { return &ActiveSessions{store: s} }

// Start creates an active_sessions row. expectedVersion = 0 means "must
// not exist"; any other value means "force-takeover from this version".
func (a *ActiveSessions) Start(userID, projectID, device string, expectedVersion int64) (domain.ActiveSession, error) {
	tx, err := a.store.DB().Begin()
	if err != nil {
		return domain.ActiveSession{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var curVersion int64
	switch err := tx.QueryRow(`SELECT version FROM active_sessions WHERE user_id = ? AND project_id = ?`, userID, projectID).Scan(&curVersion); {
	case errors.Is(err, sql.ErrNoRows):
		if expectedVersion != 0 {
			return domain.ActiveSession{}, ports.ErrActiveSessionConflict
		}
	case err != nil:
		return domain.ActiveSession{}, err
	default:
		if curVersion != expectedVersion {
			return domain.ActiveSession{}, ports.ErrActiveSessionConflict
		}
	}

	v, err := NextLamport(tx)
	if err != nil {
		return domain.ActiveSession{}, err
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(`
		INSERT INTO active_sessions (user_id, project_id, started_at, started_on_device, version)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, project_id) DO UPDATE SET
			started_at = excluded.started_at,
			started_on_device = excluded.started_on_device,
			version = excluded.version`,
		userID, projectID, now.Format(time.RFC3339), device, v); err != nil {
		return domain.ActiveSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ActiveSession{}, err
	}
	return domain.ActiveSession{
		UserID: userID, ProjectID: projectID,
		StartedAt: now, StartedOnDevice: device, Version: v,
	}, nil
}

// Stop is atomic: in one transaction, deletes the active_sessions row AND
// inserts a finished sessions row from the (started_at..now) range. Both
// get incremented lamport versions so clients pull both updates.
//
// Returns ErrActiveSessionConflict if expectedVersion doesn't match the
// stored row, ErrActiveSessionNotFound if no row exists.
func (a *ActiveSessions) Stop(userID, projectID string, expectedVersion int64, tag, note string) (domain.Session, error) {
	tx, err := a.store.DB().Begin()
	if err != nil {
		return domain.Session{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var startedAt string
	var curVersion int64
	if err := tx.QueryRow(
		`SELECT started_at, version FROM active_sessions WHERE user_id = ? AND project_id = ?`,
		userID, projectID).Scan(&startedAt, &curVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Session{}, ports.ErrActiveSessionNotFound
		}
		return domain.Session{}, err
	}
	if curVersion != expectedVersion {
		return domain.Session{}, ports.ErrActiveSessionConflict
	}

	start, _ := time.Parse(time.RFC3339, startedAt)
	now := time.Now().UTC()

	sessV, err := NextLamport(tx)
	if err != nil {
		return domain.Session{}, err
	}
	sess := domain.Session{
		ID: uuid.NewString(), UserID: userID, ProjectID: projectID,
		Date:      start.Truncate(24 * time.Hour),
		Start:     start, Stop: now, Elapsed: now.Sub(start),
		Tag: tag, Note: note, Version: sessV, UpdatedAt: now,
	}
	if _, err := tx.Exec(`
		INSERT INTO sessions (id, user_id, project_id, date, start, stop, elapsed_ns, tag, note, version, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.UserID, sess.ProjectID,
		sess.Date.Format("2006-01-02"), sess.Start.Format(time.RFC3339),
		sess.Stop.Format(time.RFC3339), int64(sess.Elapsed),
		sess.Tag, sess.Note, sess.Version, sess.UpdatedAt.Format(time.RFC3339)); err != nil {
		return domain.Session{}, err
	}

	if _, err := tx.Exec(
		`DELETE FROM active_sessions WHERE user_id = ? AND project_id = ?`, userID, projectID); err != nil {
		return domain.Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Session{}, err
	}
	return sess, nil
}

// ListByUser, Get, PullSince — straightforward.
func (a *ActiveSessions) ListByUser(userID string) ([]domain.ActiveSession, error) { … }
func (a *ActiveSessions) Get(userID, projectID string) (domain.ActiveSession, error) { … }
func (a *ActiveSessions) PullSince(userID string, since int64) ([]domain.ActiveSession, int64, error) { … }
```

Add `ErrActiveSessionConflict` to `internal/ports/active_sessions.go`:
```go
var ErrActiveSessionConflict = errSentinel("flow: active session version conflict")
```

**Tests cover:** Start when row absent + expectedVersion=0 → ok; Start when row present + expectedVersion=0 → 409; Stop creates Session + deletes active_sessions atomically; rollback on mid-tx failure leaves both tables untouched.

Commit: `feat(sqliteserver): ActiveSessions with atomic Stop→Session transaction`.

---

## Task 23: httpserver — users-ensure on bearer ingress

**Files:**
- Create: `internal/adapter/httpserver/users_ensure.go` + `_test.go`
- Modify: `internal/adapter/httpserver/middleware.go` — bearer middleware now also calls UserStore.EnsureBySub.

**Pattern:** When a request comes in with a valid bearer token, the bearer middleware:
1. Verifies JWT (existing M1 code).
2. Checks allowlist (existing M1 code).
3. **NEW**: Calls `users.EnsureBySub(sub, email, name)` → injects `*domain.User` into request context.

Add to `NewBearerMiddleware` an optional `UserStore` parameter (extend `AuthDeps`); if non-nil, ensure-on-each-request. Cache by sub→user-id in-memory with sync.Map to avoid hammering DB on every request.

**ContextKey:**
```go
const ctxKeyUser ctxKey = 2

func WithUser(ctx context.Context, u domain.User) context.Context {
	return context.WithValue(ctx, ctxKeyUser, u)
}
func UserFromContext(ctx context.Context) (domain.User, bool) {
	u, ok := ctx.Value(ctxKeyUser).(domain.User)
	return u, ok
}
```

All later handlers use `UserFromContext(r.Context())` to get the user-id.

Tests: bearer middleware on first request inserts user row; second request returns cached user-id without DB-hit.

Commit: `feat(server): users-ensure middleware on bearer ingress`.

---

## Task 24: httpserver — projects handlers

**Files:**
- Create: `internal/adapter/httpserver/projects_handlers.go` + `_test.go`
- Modify: `internal/adapter/httpserver/server.go` — register `/api/v1/projects` routes under bearer middleware.

**Endpoints:**

`GET /api/v1/projects?since=<lamport>&limit=200`:

```go
func NewProjectsPullHandler(store ports.ProjectStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := UserFromContext(r.Context())
		since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
		limit := 200
		if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 500 {
			limit = l
		}
		items, hi, hasMore, err := store.PullSince(user.ID, since, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": items, "high_watermark": hi, "has_more": hasMore,
		})
	})
}
```

`PUT /api/v1/projects/<id>` with `If-Match: <version>`:

```go
func NewProjectsPushHandler(store ports.ProjectStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := UserFromContext(r.Context())
		id := chi.URLParam(r, "id")
		expected, _ := strconv.ParseInt(r.Header.Get("If-Match"), 10, 64)
		var in domain.Project
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		in.ID = id
		in.UserID = user.ID
		// Server.Upsert (analog zu sessions, mit optimistic concurrency)
		out, err := store.UpsertWithVersion(in, expected)
		if errors.Is(err, ports.ErrProjectVersionConflict) {
			cur, _ := store.GetByID(user.ID, id)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"current": cur})
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": out.Version, "updated_at": out.LastUsedAt.Format(time.RFC3339),
		})
	})
}
```

(Add `UpsertWithVersion` to `ports.ProjectStore` if not already there. Server-side uses `NextLamport`. Add sentinel `ErrProjectVersionConflict`.)

Wiring in `server.go` `NewWithAuth`:
```go
r.Group(func(rr chi.Router) {
    rr.Use(NewBearerMiddleware(d.Provider, d.Access, d.Users))
    rr.Get("/api/v1/projects", NewProjectsPullHandler(d.ProjectStore).ServeHTTP)
    rr.Put("/api/v1/projects/{id}", NewProjectsPushHandler(d.ProjectStore).ServeHTTP)
    // ... sessions + active in Tasks 25/26
})
```

(Extend `AuthDeps` with `Users ports.UserStore` + `ProjectStore ports.ProjectStore` + later Stores; wiring happens in Task 33.)

Tests: GET returns paginated items, PUT inserts + returns version, PUT with stale If-Match returns 409 with current row.

Commit: `feat(server): /api/v1/projects pull + push handlers`.

---

## Task 25: httpserver — sessions handlers

**Files:**
- Create: `internal/adapter/httpserver/sessions_handlers.go` + `_test.go`

Same pattern as projects: `GET /api/v1/sessions?since=&limit=` + `PUT /api/v1/sessions/{id}` + `If-Match`. Use `sqliteserver.Sessions.PullSince` + `Upsert(in, expectedVersion)`.

Add `domain.SessionAggregate`-friendly JSON shape (the Session struct already marshals cleanly).

Tests parallel to Task 24.

Commit: `feat(server): /api/v1/sessions pull + push handlers`.

---

## Task 26: httpserver — active_sessions handlers

**Files:**
- Create: `internal/adapter/httpserver/active_sessions_handlers.go` + `_test.go`

**Endpoints:**

- `GET /api/v1/active` — list all currently-active for the user (optional `?since=<lamport>` for pull semantics).
- `POST /api/v1/active/{project-id}/start` + `If-Match: 0 or <version>` — see Task 22.Start behavior.
- `DELETE /api/v1/active/{project-id}` + `If-Match: <version>` — see Task 22.Stop; body `{"tag":"...","note":"..."}` optional.

```go
func NewActiveStartHandler(store ports.ActiveSessionStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := UserFromContext(r.Context())
		projectID := chi.URLParam(r, "project_id")
		expected, _ := strconv.ParseInt(r.Header.Get("If-Match"), 10, 64)

		var body struct{ StartedOnDevice string `json:"started_on_device"` }
		_ = json.NewDecoder(r.Body).Decode(&body)

		a, err := store.Start(user.ID, projectID, body.StartedOnDevice, expected)
		if errors.Is(err, ports.ErrActiveSessionConflict) {
			cur, _ := store.Get(user.ID, projectID)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"current": cur})
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(a)
	})
}

func NewActiveStopHandler(store ports.ActiveSessionStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := UserFromContext(r.Context())
		projectID := chi.URLParam(r, "project_id")
		expected, _ := strconv.ParseInt(r.Header.Get("If-Match"), 10, 64)
		var body struct{ Tag, Note string }
		_ = json.NewDecoder(r.Body).Decode(&body)

		sess, err := store.Stop(user.ID, projectID, expected, body.Tag, body.Note)
		if errors.Is(err, ports.ErrActiveSessionNotFound) {
			http.Error(w, "no active session", http.StatusNotFound)
			return
		}
		if errors.Is(err, ports.ErrActiveSessionConflict) {
			cur, _ := store.Get(user.ID, projectID)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"current": cur})
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sess)
	})
}
```

(Adjust `ports.ActiveSessionStore` interface to add `Start(userID, projectID, device string, expectedVersion int64)` and `Stop(userID, projectID string, expectedVersion int64, tag, note string)` — these are server-side-only methods. Client store stays minimal Upsert/Delete.)

Tests cover: Start when free → 200, Start with stale If-Match → 409, Stop atomic semantics (in unit test, verify subsequent GET /sessions returns the row server created).

Commit: `feat(server): /api/v1/active handlers (start/stop with server-side atomic Stop transaction)`.

---

## Task 27: httpsync.Client — typed REST wrappers

**Files:**
- Create: `internal/adapter/httpsync/doc.go`
- Create: `internal/adapter/httpsync/client.go` + `_test.go`

**Pattern:** thin wrapper around `http.Client` with bearer-token injection. Methods:

```go
type Client struct {
	base    string
	tokens  ports.TokenStore
	slot    string
	httpc   *http.Client
}

func NewClient(base string, tokens ports.TokenStore, slot string) *Client { ... }

func (c *Client) PullSessions(ctx context.Context, since int64, limit int) ([]domain.Session, int64, bool, error)
func (c *Client) PushSession(ctx context.Context, s domain.Session, expectedVersion int64) (int64, error)  // returns new version; ErrSessionVersionConflict on 409
func (c *Client) PullProjects(ctx context.Context, since int64, limit int) ([]domain.Project, int64, bool, error)
func (c *Client) PushProject(ctx context.Context, p domain.Project, expectedVersion int64) (int64, error)
func (c *Client) PullActive(ctx context.Context, since int64) ([]domain.ActiveSession, int64, error)
func (c *Client) StartActive(ctx context.Context, projectID, device string, expectedVersion int64) (domain.ActiveSession, error)
func (c *Client) StopActive(ctx context.Context, projectID string, expectedVersion int64, tag, note string) (domain.Session, error)
```

Each method:
1. Reads token from store (refresh if near-expiry via `oidcclient.Tokens.Current`).
2. Sets `Authorization: Bearer <token>` + `If-Match: <expectedVersion>` where needed.
3. Maps status codes to errors: 409 → `Err...VersionConflict`, 401 → `ErrUnauthorized` (signal re-login), 5xx → error with body, 200 → decode JSON.

Tests against httptest server.

Commit: `feat(httpsync): typed REST client with bearer-auth + conflict mapping`.

---

## Task 28: httpsync.Queue — write_queue management

**Files:**
- Create: `internal/adapter/httpsync/queue.go` + `_test.go`

Thin wrapper around `ports.WriteQueue` providing:
- `EnqueueSession(s domain.Session, expectedVersion int64) error`
- `EnqueueProject(p domain.Project, expectedVersion int64) error`
- `EnqueueActiveStart(...)`, `EnqueueActiveStop(...)`
- Each method JSON-marshals the payload and calls WriteQueue.Enqueue with the right `resource` tag.

Plus a `Drain(callback func(WriteQueueEntry) (ok bool, err error)) error` helper:
- Peeks N entries (e.g. 50).
- For each, invokes callback. If ok=true → Remove(seq). If err != nil → SetError(seq, err.Error()).
- Stops on the first non-ok-non-error entry (e.g. conflict — caller handles).

Tests cover happy-path drain, error-recorded-but-not-removed, conflict-halts-drain.

Commit: `feat(httpsync): write_queue typed enqueue + drain helper`.

---

## Task 29: httpsync.Worker — pull loop + push drain + conflict channel

**Files:**
- Create: `internal/adapter/httpsync/worker.go` + `_test.go`

**Worker shape:**

```go
package httpsync

import (
	"context"
	"log/slog"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

type Worker struct {
	client     *Client
	sessions   ports.SessionStore
	projects   ports.ProjectStore
	active     ports.ActiveSessionStore
	watermarks ports.SyncWatermarkStore
	queue      *Queue
	userID     string

	conflicts  chan ports.ConflictMsg
	pushSignal chan struct{}
	stop       context.CancelFunc

	pullInterval time.Duration
}

func NewWorker(client *Client, ss ports.SessionStore, ps ports.ProjectStore, as ports.ActiveSessionStore, ws ports.SyncWatermarkStore, q *Queue, userID string) *Worker {
	return &Worker{
		client: client, sessions: ss, projects: ps, active: as,
		watermarks: ws, queue: q, userID: userID,
		conflicts: make(chan ports.ConflictMsg, 16),
		pushSignal: make(chan struct{}, 1),
		pullInterval: 30 * time.Second,
	}
}

func (w *Worker) Conflicts() <-chan ports.ConflictMsg { return w.conflicts }

// SignalPush triggers an immediate push-drain (called by use cases after
// Enqueue). Non-blocking — drop signal if channel full (worker will drain
// on its next iteration anyway).
func (w *Worker) SignalPush() {
	select {
	case w.pushSignal <- struct{}{}:
	default:
	}
}

func (w *Worker) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	w.stop = cancel
	go w.loop(ctx)
}

func (w *Worker) Stop() {
	if w.stop != nil {
		w.stop()
	}
}

func (w *Worker) loop(ctx context.Context) {
	pullT := time.NewTicker(w.pullInterval)
	defer pullT.Stop()

	// Initial pull on start so the cache catches up before first user action.
	w.runPull(ctx)
	w.runDrain(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-pullT.C:
			w.runPull(ctx)
			w.runDrain(ctx)
		case <-w.pushSignal:
			w.runDrain(ctx)
		}
	}
}

func (w *Worker) runPull(ctx context.Context) {
	if err := w.pullResource(ctx, "projects"); err != nil {
		slog.Warn("sync: pull projects", slog.Any("err", err))
	}
	if err := w.pullResource(ctx, "sessions"); err != nil {
		slog.Warn("sync: pull sessions", slog.Any("err", err))
	}
	if err := w.pullResource(ctx, "active_sessions"); err != nil {
		slog.Warn("sync: pull active_sessions", slog.Any("err", err))
	}
}

// pullResource handles one resource type: get watermark, GET .../<resource>,
// ingest items, update watermark. Loops while has_more.
func (w *Worker) pullResource(ctx context.Context, resource string) error {
	for {
		wm, _ := w.watermarks.Get(resource)
		var hi int64
		var more bool
		var err error
		switch resource {
		case "sessions":
			items, h, m, e := w.client.PullSessions(ctx, wm, 200)
			if e != nil {
				return e
			}
			if err := w.sessions.UpsertBatch(items); err != nil {
				return err
			}
			hi, more = h, m
		case "projects":
			items, h, m, e := w.client.PullProjects(ctx, wm, 200)
			if e != nil {
				return e
			}
			for _, p := range items {
				if err := w.projects.Upsert(p); err != nil {
					return err
				}
			}
			hi, more = h, m
		case "active_sessions":
			items, h, e := w.client.PullActive(ctx, wm)
			if e != nil {
				return e
			}
			// Sync into local: any active row we don't have locally → upsert;
			// any local row absent from server response is left alone (server
			// might just not have returned it — only the "stop" flow deletes).
			for _, a := range items {
				if err := w.active.Upsert(a); err != nil {
					return err
				}
			}
			hi, more = h, false
		default:
			return nil
		}
		if err := w.watermarks.Set(resource, hi); err != nil {
			return err
		}
		if !more {
			return nil
		}
	}
}

// runDrain pushes pending writes; on 409 emits a ConflictMsg.
func (w *Worker) runDrain(ctx context.Context) {
	err := w.queue.Drain(func(e ports.WriteQueueEntry) (bool, error) {
		switch e.Resource {
		case "sessions":
			var s domain.Session
			if err := json.Unmarshal(e.Payload, &s); err != nil {
				return false, err
			}
			newV, err := w.client.PushSession(ctx, s, e.ExpectedVersion)
			if errors.Is(err, ports.ErrSessionVersionConflict) {
				// fetch current server row and emit conflict
				items, _, _, _ := w.client.PullSessions(ctx, 0, 1) // simplified
				_ = items
				w.emitConflict(ports.ConflictMsg{Resource: "sessions", RowID: s.ID, QueueSeq: e.Seq, Local: s /* Server: TODO */})
				return false, nil // halt drain; user resolves
			}
			if err != nil {
				return false, err
			}
			s.Version = newV
			_ = w.sessions.Upsert(s)
			return true, nil
		// ... similar cases for projects + active_sessions
		}
		return false, nil
	})
	if err != nil {
		slog.Warn("sync: drain", slog.Any("err", err))
	}
}

func (w *Worker) emitConflict(c ports.ConflictMsg) {
	select {
	case w.conflicts <- c:
	default:
		slog.Warn("sync: conflict channel full, dropping", slog.String("resource", c.Resource))
	}
}
```

**Tests:**
- pullResource with mock client returning paginated items → all ingested, watermark updated.
- runDrain on happy push → entry removed.
- runDrain on 409 → entry kept, conflict emitted on channel.
- Stop() cancels the loop.

Commit: `feat(httpsync): background worker — pull loop + push drain + conflict channel`.

---

## Task 30: TUI conflict_overlay component

**Files:**
- Create: `internal/frontend/tui/components/conflict_overlay/`
  - `doc.go`
  - `model.go` + `update.go` + `view.go` + `styles.go`
  - `variants.go` (presets for session-edit + active-session-race)
  - `*_test.go`

**Variants:**

```go
package conflict_overlay

// Variant selects the layout + key bindings.
type Variant int

const (
	VariantSessionEdit   Variant = iota // [s] Server / [l] Lokal / [esc] Abbrechen
	VariantActiveRace                   // [t] Übernehmen / [n] Neue Session / [esc] Abbrechen
)

type Model struct {
	variant   Variant
	title     string
	body      string
	choices   []choice
	palette   theme.Palette
}

type choice struct {
	key   string // "s", "l", "t", "n"
	label string // "Server-Version übernehmen"
	msg   tea.Msg // emitted on press
}

func NewSessionEditConflict(local, server domain.Session, p theme.Palette, onResolve func(accept bool) tea.Msg) Model
func NewActiveRaceConflict(server domain.ActiveSession, p theme.Palette, onTakeover func() tea.Msg, onParallel func() tea.Msg) Model

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) { ... }
func (m Model) View() string                         { ... }
```

Use `internal/frontend/tui/components/markdown_overlay`'s chrome (centered, double-border, palette-driven) as the visual template — adapt to text-only body.

**Tests:** snapshot test of View() output for both variants with fixed palette; key-press routing emits expected msg.

Commit: `feat(tui): conflict_overlay component (session-edit + active-race variants)`.

---

## Task 31: TUI worktime — subscribe to conflict channel + render overlay

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/` model.go / update.go / view.go

**Wire-in:**
- Worktime-Model gets a `conflicts <-chan ports.ConflictMsg` from the sync worker (passed via SidekickDeps).
- A bubbletea command listens on the channel and emits a `ConflictMsg`-equivalent tea-Msg.
- Update-loop on ConflictMsg → build conflict_overlay Model + set as active overlay.
- On overlay-resolved-Msg → call `usecase.Sync.AcceptServerVersion(queueSeq)` or `OverwriteServerVersion(queueSeq)` or `usecase.ActiveSessions.ForceTakeover(...)`.

Commit: `feat(tui): worktime subscribes to sync-conflict channel + renders overlay`.

---

## Task 32: WIRING — cmd/flow/main.go full assembly

**Files:**
- Modify: `cmd/flow/main.go` — replace existing wiring with the M2-M3 dependency graph.

**Wiring contract:**

```go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/serverkraken/flow/internal/adapter/httpsync"
	"github.com/serverkraken/flow/internal/adapter/jsonflowstate"
	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/serverkraken/flow/internal/adapter/sqliteclient"
	// ... existing imports
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	home, _ := os.UserHomeDir()
	cachePath := filepath.Join(xdgDataHome(home), "flow", "cache.db")
	_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)

	clientDB, err := sqliteclient.Open(cachePath)
	if err != nil {
		logger.Error("open client db", slog.Any("err", err))
		os.Exit(1)
	}
	defer clientDB.Close()

	// keychain + token store (from M1)
	keyring := keyringadapter.New()

	// pause-only flowstate (legacy)
	pauseStore := jsonflowstate.New(filepath.Join(home, ".tmux"))

	// new adapters
	users := sqliteclient.NewUsers(clientDB)
	projects := sqliteclient.NewProjects(clientDB)
	sessions := sqliteclient.NewSessions(clientDB)
	activeStore := sqliteclient.NewActiveSessions(clientDB)
	syncState := sqliteclient.NewSyncState(clientDB)
	writeQueue := sqliteclient.NewWriteQueue(clientDB)

	// User bootstrap from keychain token (subject claim)
	sub := readSubFromKeychain(keyring) // helper that reads ID-token, extracts sub
	user, err := users.EnsureBySub(sub, "", "") // email/name enrichment via /me later
	if err != nil {
		logger.Error("user bootstrap", slog.Any("err", err))
		os.Exit(1)
	}

	// sync client + worker
	serverURL := envOrDefault("FLOW_SERVER_URL", "http://localhost:8080")
	syncClient := httpsync.NewClient(serverURL, keyring, "tokens:"+serverURL)
	syncQueue := httpsync.NewQueue(writeQueue)
	syncWorker := httpsync.NewWorker(syncClient, sessions, projects, activeStore, syncState, syncQueue, user.ID)
	syncWorker.Start(ctx)
	defer syncWorker.Stop()

	// use cases
	projectsUC := usecase.NewProjects(users, projects)
	activeUC := usecase.NewActiveSessions(users, projects, activeStore, sessions, writeQueue)
	sessionsUC := usecase.NewSessions(users, projects, sessions, activeStore, sourceDirs, /* ... */)

	deps := cliDeps{
		User: user,
		Projects: projectsUC, Active: activeUC, Sessions: sessionsUC,
		SyncWorker: syncWorker, // for `flow sync status` to read its state
	}

	rootCmd := buildRootCmd(deps)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		logger.Error("command failed", slog.Any("err", err))
		os.Exit(1)
	}
}
```

(The actual cmd/flow/main.go in M1 has more wiring for existing TUI screens — preserve those, just add the new adapters/use-cases. The shape above shows the new additions only.)

**TUI conflict wire-up:** `SidekickDeps` for the worktime screen factory gets a `ConflictChan <-chan ports.ConflictMsg` field, passed from `syncWorker.Conflicts()`.

**TSV migration guard:** before any worktime command runs, check tsv-exists + sessions-count-zero (the guard from Task 14).

Commit: `wire(cmd/flow): assemble sqliteclient + sync-worker + new use cases

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.

---

## Task 33: WIRING — cmd/flow-server/main.go full assembly + handler registration

**Files:**
- Modify: `cmd/flow-server/main.go` — open sqliteserver, register projects/sessions/active handlers under bearer-auth, expose user-store to bearer middleware.

**Additions on top of M1 wiring:**

```go
serverDBPath := envOrDefault("FLOW_SERVER_DB", "/var/lib/flow/server.db")
serverDB, err := sqliteserver.Open(serverDBPath)
if err != nil {
	logger.Error("open server db", slog.Any("err", err))
	os.Exit(1)
}
defer serverDB.Close()

users := sqliteserver.NewUsers(serverDB)
projects := sqliteserver.NewProjects(serverDB)
sessions := sqliteserver.NewSessions(serverDB)
activeStore := sqliteserver.NewActiveSessions(serverDB)

srv := httpserver.NewWithAuth(httpserver.AuthDeps{
	Provider:     provider,
	Access:       access,
	Session:      session,
	BaseURL:      cfg.BaseURL,
	OIDCClientID: cfg.OIDCClientID,
	OIDCSecret:   cfg.OIDCClientSecret,
	Cookie:       httpserver.CookieConfig{Name: "flow_session", Secure: secure},
	Ready: func() error {
		// readyz now checks DB-ping too
		return serverDB.DB().Ping()
	},
	OIDCConfig: oidcCfg,
	// NEW:
	Users:        users,
	Projects:     projects,
	Sessions:     sessions,
	ActiveStore:  activeStore,
})
```

**Add new env-vars to `LoadConfig`:**
- `FLOW_SERVER_DB` (default `/var/lib/flow/server.db` — Podman volume mount).

**Server-Dockerfile update (deploy/podman/Dockerfile.server):** add volume mount path or document via docker-compose.

**docker-compose.yml** (deploy/podman): bind-mount `./flow-server-data:/var/lib/flow` for persistent DB.

Commit: `wire(flow-server): assemble sqliteserver + register projects/sessions/active handlers`.

---

## Task 34: SMOKE — Multi-device E2E + manual runbook + `make ci`

**Goal (mandatorisch laut [[plan-main-wiring-task]]):** verify the whole stack works end-to-end against a real dex+flow-server, on at least two simulated clients, before declaring Plan B done.

**Files:**
- Create: `scripts/smoke-m2-m3.sh` — automated smoke script.
- Modify: `docs/runbook/m1-smoke-test.md` → rename/extend to `docs/runbook/m2-m3-smoke-test.md`.
- Modify: `CLAUDE-activeContext.md` (gitignored, local-only) — append "M2-M3 done" status.

**Smoke script `scripts/smoke-m2-m3.sh`:**

```bash
#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "== Phase 1: clean state =="
rm -rf /tmp/flow-smoke
mkdir -p /tmp/flow-smoke/{a,b}

echo "== Phase 2: build =="
make build-server
go build -o bin/flow ./cmd/flow

echo "== Phase 3: dex up =="
make dex-up
sleep 2

echo "== Phase 4: flow-server up (background) =="
FLOW_SERVER_DB=/tmp/flow-smoke/server.db \
FLOW_OIDC_ISSUER=http://localhost:5556 \
FLOW_OIDC_CLIENT_ID=flow-server \
FLOW_OIDC_CLIENT_SECRET=flow-server-secret \
FLOW_COOKIE_HASH_KEY=$(openssl rand -hex 32) \
FLOW_COOKIE_BLOCK_KEY=$(openssl rand -hex 16) \
FLOW_ALLOWED_SUBS=ChBhbGljZS1zdGF0aWMtdWlkEgVsb2NhbA \
FLOW_SERVER_BASE_URL=http://localhost:8080 \
./bin/flow-server &
SERVER_PID=$!
trap "kill $SERVER_PID 2>/dev/null; make dex-down" EXIT
sleep 2

echo "== Phase 5: client A — login + project create + worktime start =="
XDG_DATA_HOME=/tmp/flow-smoke/a ./bin/flow login --server=http://localhost:8080 --client-id=flow-cli
XDG_DATA_HOME=/tmp/flow-smoke/a ./bin/flow projects create "Smoke A"
XDG_DATA_HOME=/tmp/flow-smoke/a ./bin/flow worktime start --project=smoke-a
sleep 35  # wait for one full sync tick

echo "== Phase 6: client B — login + projects list should show 'Smoke A' =="
XDG_DATA_HOME=/tmp/flow-smoke/b ./bin/flow login --server=http://localhost:8080 --client-id=flow-cli
XDG_DATA_HOME=/tmp/flow-smoke/b ./bin/flow projects list | grep -q "Smoke A" \
  && echo "  ✓ Project synced to client B" \
  || { echo "  ✗ Project missing on client B"; exit 1; }

XDG_DATA_HOME=/tmp/flow-smoke/b ./bin/flow worktime status | grep -q "Smoke A" \
  && echo "  ✓ Active session visible on client B" \
  || { echo "  ✗ Active session not synced"; exit 1; }

echo "== Phase 7: client B tries to start same project (expects conflict) =="
XDG_DATA_HOME=/tmp/flow-smoke/b ./bin/flow worktime start --project=smoke-a 2>&1 | grep -qiE "konflikt|already|läuft" \
  && echo "  ✓ Server-side race detected" \
  || { echo "  ✗ Race NOT detected"; exit 1; }

echo "== Phase 8: client A stop, client B should see Session row =="
XDG_DATA_HOME=/tmp/flow-smoke/a ./bin/flow worktime stop --project=smoke-a
sleep 35
XDG_DATA_HOME=/tmp/flow-smoke/b ./bin/flow worktime today | grep -q "Smoke A" \
  && echo "  ✓ Stopped session synced to client B" \
  || { echo "  ✗ Stopped session missing"; exit 1; }

echo "== Phase 9: cleanup =="
echo "== ALL SMOKE PASSED =="
```

Manual runbook `docs/runbook/m2-m3-smoke-test.md` documents the same flow plus TSV-migration smoke against a real ~/.tmux/worktime.log copy.

**Final `make ci` check:**

```bash
make ci
```
Expected: lint clean, coverage gate met (re-raise to 90% after this milestone — adjust Makefile COVER_THRESHOLD back to 90 and confirm aggregate is above; if not, surface gap to user).

**Update `CLAUDE-activeContext.md`** (gitignored, local-only) with a "M2-M3 done" section summarizing branch state.

Commit: `test(smoke): multi-device E2E smoke script + manual runbook + ci-green`.

---

## Self-Review Notes (run before declaring plan done)

1. **Spec coverage:** every section of the spec (`docs/superpowers/specs/2026-06-02-flow-phase1-m2-m3-domain-sync-design.md`) maps to a task:
   - § Naming-Kollision → Task 1.
   - § TSV-Migration → Task 19.
   - § Project-Resolution → Tasks 11 (CLI) + 16 + 17 (TUI picker).
   - § Konflikt-UX → Tasks 30 + 31.
   - § DB-Migrations → Tasks 4 + 20 (goose embedded).
   - § Domain-Model → Task 2 + 3 (entities + ports).
   - § DB-Schema-Details → Tasks 4 + 20 (full DDL).
   - § Sync-Protokoll → Tasks 24-26 (server handlers) + 27-29 (client worker).
   - § Active-Session-Spezialfall → Task 22 (atomic Stop transaction) + 26 (server endpoint) + 12 (client use case).
   - § Modul-Layout → all tasks reference the layout from the spec.
   - § Testing-Strategie → covered task-by-task; multi-device smoke is Task 34.
   - § Wiring + Smoke → Tasks 32 + 33 + 34.

2. **Placeholder scan:** the only intentional placeholders are the `intToStr` helper in Task 10 and the `encodeActiveStart`/`encodeSession`/`i64` helpers in Task 12. Each is annotated with "(placeholder — implement using stdlib X)". Implementer subagent will fix during task execution.

3. **Type consistency:** SessionStore methods (`Upsert`, `UpsertBatch`, `Delete`, `Load`, `LoadFiltered`) defined in Task 3 are used consistently in Tasks 6, 11, 19, 29, 32. ActiveSessionStore methods (client-side `Upsert/Delete/Get/ListByUser` vs server-side `Start/Stop/PullSince`) are split between separate interface files — Task 3 defines client-side; Task 22 + 26 use server-side extension methods (need a `ServerActiveSessionStore` super-interface or two interfaces in different packages — pick during implementation).

4. **Wiring task present:** Tasks 32-34 are the mandatory wiring + smoke per `feedback_plan_main_wiring_task.md`. ✓

---

## Done Criteria

A merged Plan B delivers:

1. `make ci` green with 90% coverage gate restored.
2. `scripts/smoke-m2-m3.sh` passes on developer machine (Podman + dex container available).
3. `flow worktime migrate-from-tsv` migrates Soenne's real `~/.tmux/worktime.log` without data loss.
4. Two `flow` instances against the same `flow-server` see each other's sessions within 30 s.
5. Conflict overlays render correctly when forced (parallel `flow worktime start --project=foo` on both clients).
6. PR title: `feat: Phase-1 M2-M3 — domain extension + sessions sync`.
7. `CLAUDE-activeContext.md` updated to reflect M2-M3 status (gitignored, local-only).

