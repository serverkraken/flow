# flow Rebuild M2a — Document-Spine · Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ein server-autoritativer Document-Spine: `domain.Document` + Postgres-Store + REST-CRUD + apiclient + WebUI-`/docs` (volles CRUD, Markdown-Render) + TUI-`flow docs` (Liste/Ansicht + `$EDITOR`-Editing), **live-synced** via SSE.

**Architecture:** Hexagonal, spiegelt M1a (Worktime-Spine). Reine Domain (`Document.Validate`) → Usecases → pgstore (Migration 0006) → httpserver REST + SSE → apiclient → WebUI (templ+goldmark) + eigenes TUI-Model (`flow docs`) + Editor-Adapter (`$EDITOR`). Composition-Root verdrahtet beide.

**Tech Stack:** Go, pgx/v5, goose (embedded migrations), net/http, Cobra, templ+HTMX, **goldmark + bluemonday** (neu), Bubbletea v2, testcontainers.

---

## Worktree & Branch

**Alle Code-Tasks im Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild` auf Branch `rebuild`** (HEAD aktuell `47029a5`, nach M1). Plan-/Spec-Docs auf `main` — **nicht** ins `rebuild` committen. Modulpfad `github.com/serverkraken/flow`. Kleine fokussierte Commits pro Task; am Ende `make ci` grün inkl. Coverage-Gate **≥80%**.

## Bestehende Patterns (verifiziert — Referenzdateien zum Spiegeln)

- **Domain-Entität**: `internal/domain/project.go` (`NewProject` validiert, sentinel-Errors in `errors.go`).
- **Event**: `internal/domain/event.go` — `EventType` consts + `Event{Type, UserID, Data map[string]any}`.
- **Port**: `internal/ports/ports.go` — `Clock{Now}`, `IDGen{NewID}`, `XStore`-Interfaces, sentinel-Errors. `EventBus{Publish, Subscribe}`.
- **Store**: `internal/adapter/pgstore/projects.go` — `NewXStore(pool)`, `scanX(rowScanner)`, `fmt.Errorf("pgstore: …: %w")`, `pgx.ErrNoRows`→sentinel. `rowScanner` ist dort definiert (wiederverwenden).
- **Usecase**: `internal/usecase/create_project.go` — Struct mit Ports, `Execute(ctx, …)`, `Slugify`.
- **REST-Handler**: `internal/adapter/httpserver/worktime.go` — `writeJSON(w, status, v)`, `userFrom(r.Context())` → `u.ID`, `s.Bus.Publish(domain.Event{…})`, `json.NewDecoder(r.Body).Decode`. Routen in `server.go` `Routes()` mit `s.auth(...)`/`s.webAuth(...)`.
- **apiclient**: `internal/adapter/apiclient/client.go` — `c.base`, `c.hc`, `c.do(ctx, method, path, body, out)`; `New(base, token)`. Resource-Methoden in eigenen Dateien (`export.go`).
- **WebUI**: `internal/adapter/httpserver/webui_stats.go` + `internal/adapter/webui/stats.templ` — Page/Fragment, `webAuth`, templ-Render; Nav in allen `*.templ`. Codegen: `go tool templ generate`; `make ci` prüft `verify-generate`.
- **TUI**: `internal/tui/worktime.go` (Model + SSE via `client.Events()` → `eventMsg`) + `cmd/flow/worktime.go` (Launch). Test-Muster `internal/tui/worktime_test.go` (`New(nil,…)` + `m.Update`, httptest-`newFakeSrv`).
- **Fakes**: `internal/testutil/fakes.go` — `FakeProjectStore` etc. (mutex + map). Hier neu: `FakeDocumentStore`.
- **Migrationen**: `internal/adapter/pgstore/migrations/000N_*.sql` (goose Up/Down), bisher bis `0005`.

---

## File Structure

**Neu (Domain/Usecase/Ports):**
- `internal/domain/document.go` — `Document`, `DocumentType`, `Validate`, `DailyPath`, `slugOK`; + `ErrInvalidDocument` in `errors.go`.
- `internal/usecase/create_document.go`, `get_document.go`, `list_documents.go`, `update_document.go`, `delete_document.go`.
- `internal/ports/ports.go` (modify) — `DocumentStore`, `ErrDocumentNotFound`, `ErrDocumentExists`, `Editor`.

**Neu (Adapter):**
- `internal/adapter/pgstore/migrations/0006_documents.sql`, `internal/adapter/pgstore/documents.go`.
- `internal/adapter/httpserver/documents.go` (REST), `internal/adapter/httpserver/webui_docs.go` (WebUI handler).
- `internal/adapter/apiclient/documents.go`.
- `internal/adapter/webui/docs.templ` (+ generated), `internal/adapter/webui/markdown.go` (goldmark+bluemonday).
- `internal/adapter/editor/editor.go` (`$EDITOR` shell-out).
- `internal/tui/docs.go` (Model), `cmd/flow/docs.go` (verb).
- `internal/testutil/fakes.go` (modify) — `FakeDocumentStore`.

**Geändert (Wiring/Events):**
- `internal/domain/event.go` — `EventDocumentCreated/Updated/Deleted`.
- `internal/adapter/httpserver/server.go` — Felder + Routen + Nav.
- `cmd/flow-server/main.go`, `cmd/flow/main.go` — Composition-Root.
- `go.mod`/`go.sum` — goldmark, bluemonday.

---

## Task 1: domain.Document + Validierung

**Files:**
- Create: `internal/domain/document.go`, `internal/domain/document_test.go`
- Modify: `internal/domain/errors.go`

- [ ] **Step 1: Failing test** — `internal/domain/document_test.go`:

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestDocument_Validate(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	base := func() domain.Document {
		return domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocFree, Path: "docs/architecture", Title: "Arch", CreatedAt: now, UpdatedAt: now}
	}
	if err := base().Validate(); err != nil {
		t.Fatalf("valid free doc: %v", err)
	}
	// bad type
	d := base(); d.Type = "bogus"
	if err := d.Validate(); err == nil {
		t.Error("bad type should fail")
	}
	// project doc without project id
	d = base(); d.Type = domain.DocProject
	if err := d.Validate(); err == nil {
		t.Error("project doc without projectID should fail")
	}
	// empty/invalid slug
	d = base(); d.Path = "Bad Slug!"
	if err := d.Validate(); err == nil {
		t.Error("non-slug path should fail")
	}
	d = base(); d.Path = ""
	if err := d.Validate(); err == nil {
		t.Error("empty path should fail")
	}
	// daily requires date + derived path
	d = base(); d.Type = domain.DocDaily; d.Path = "daily/2026-06-15"; d.Date = &now
	if err := d.Validate(); err != nil {
		t.Errorf("valid daily: %v", err)
	}
	d = base(); d.Type = domain.DocDaily // no date
	if err := d.Validate(); err == nil {
		t.Error("daily without date should fail")
	}
}

func TestDailyPath(t *testing.T) {
	d := time.Date(2026, 6, 15, 23, 0, 0, 0, time.UTC)
	if got := domain.DailyPath(d); got != "daily/2026-06-15" {
		t.Errorf("DailyPath = %q", got)
	}
}

func TestSlugOK(t *testing.T) {
	ok := []string{"docs/architecture", "plans/2026-06-13-rebuild", "a", "x/y/z-1"}
	bad := []string{"", "Bad Slug", "with space", "UPPER", "trailing/", "/leading", "a//b"}
	for _, s := range ok {
		if !domain.SlugOK(s) {
			t.Errorf("SlugOK(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if domain.SlugOK(s) {
			t.Errorf("SlugOK(%q) = true, want false", s)
		}
	}
}
```

- [ ] **Step 2: Run — FAIL**: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/domain/ -run 'TestDocument|TestDailyPath|TestSlugOK'` → undefined.

- [ ] **Step 3: Implement** — `internal/domain/document.go`:

```go
package domain

import (
	"fmt"
	"regexp"
	"time"
)

// DocumentType is the kind of compendium note.
type DocumentType string

const (
	DocDaily   DocumentType = "daily"
	DocProject DocumentType = "project"
	DocFree    DocumentType = "free"
	DocAgent   DocumentType = "agent"
)

func (t DocumentType) valid() bool {
	switch t {
	case DocDaily, DocProject, DocFree, DocAgent:
		return true
	}
	return false
}

// Document is a compendium note. Path is a human-readable slug, unique per
// owner(+project). Tags/Role/Extra are carried by the schema from M2a but
// exercised by later slices (M2c tags, M3 brief role, M2d search).
type Document struct {
	ID        string         `json:"id"`
	OwnerID   string         `json:"-"`
	ProjectID *string        `json:"projectId,omitempty"`
	Type      DocumentType   `json:"type"`
	Path      string         `json:"path"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Tags      []string       `json:"tags,omitempty"`
	Date      *time.Time     `json:"date,omitempty"`
	Role      *string        `json:"role,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*(?:/[a-z0-9]+(?:-[a-z0-9]+)*)*$`)

// SlugOK reports whether s is a valid hierarchical slug: lowercase
// alphanumeric segments joined by '/', words separated by single '-'. No
// leading/trailing/double slash, no spaces or uppercase.
func SlugOK(s string) bool {
	return s != "" && slugRe.MatchString(s)
}

// DailyPath is the canonical slug for a daily note on day d.
func DailyPath(d time.Time) string {
	return "daily/" + d.Format("2006-01-02")
}

// Validate checks the document invariants (type, project rule, slug form,
// daily date). It does not check ID/owner presence — the use case stamps those.
func (d Document) Validate() error {
	if !d.Type.valid() {
		return fmt.Errorf("%w: bad type %q", ErrInvalidDocument, d.Type)
	}
	if d.Type == DocProject && (d.ProjectID == nil || *d.ProjectID == "") {
		return fmt.Errorf("%w: project document needs a projectId", ErrInvalidDocument)
	}
	if d.Type == DocDaily && d.Date == nil {
		return fmt.Errorf("%w: daily document needs a date", ErrInvalidDocument)
	}
	if !SlugOK(d.Path) {
		return fmt.Errorf("%w: invalid path %q", ErrInvalidDocument, d.Path)
	}
	return nil
}
```

- [ ] **Step 4: Add error** — in `internal/domain/errors.go`, add to the sentinel `var (...)` block: `ErrInvalidDocument = errors.New("invalid document")`.

- [ ] **Step 5: Run — PASS**: `go test ./internal/domain/ -run 'TestDocument|TestDailyPath|TestSlugOK'`, full `go test ./internal/domain/`, `go vet ./internal/domain/`, `gofmt -l internal/domain`.

- [ ] **Step 6: Commit**:
```bash
git add internal/domain/document.go internal/domain/document_test.go internal/domain/errors.go
git commit -m "feat(docs): domain.Document + validation (slug/type/daily)"
```

---

## Task 2: Ports + Migration 0006 + pgstore CRUD

**Files:**
- Modify: `internal/ports/ports.go`
- Create: `internal/adapter/pgstore/migrations/0006_documents.sql`, `internal/adapter/pgstore/documents.go`, `internal/adapter/pgstore/documents_test.go`
- Modify: `internal/testutil/fakes.go` (FakeDocumentStore — needed so other packages compile once the port grows)

- [ ] **Step 1: Port** — in `internal/ports/ports.go`, add to the sentinel block: `ErrDocumentNotFound = errors.New("document not found")` and `ErrDocumentExists = errors.New("document already exists")`. Add the interfaces:

```go
// DocumentStore persists compendium documents. All reads are owner-scoped.
// Create returns ErrDocumentExists on a (owner, project, path) collision.
type DocumentStore interface {
	Create(ctx context.Context, d domain.Document) (domain.Document, error)
	Get(ctx context.Context, ownerID, id string) (domain.Document, error)
	List(ctx context.Context, ownerID string) ([]domain.Document, error)
	Update(ctx context.Context, d domain.Document) (domain.Document, error)
	Delete(ctx context.Context, ownerID, id string) error
}

// Editor opens an interactive editor on initial content and returns the
// edited bytes. Used by the TUI for document bodies.
type Editor interface {
	Edit(ctx context.Context, initial []byte) ([]byte, error)
}
```

- [ ] **Step 2: Migration** — `internal/adapter/pgstore/migrations/0006_documents.sql` (confirm the naming with `rg --files internal/adapter/pgstore/migrations`):

```sql
-- +goose Up
CREATE TABLE documents (
    id          TEXT PRIMARY KEY,
    owner_id    TEXT NOT NULL REFERENCES users(id),
    project_id  TEXT REFERENCES projects(id),
    type        TEXT NOT NULL,
    path        TEXT NOT NULL,
    title       TEXT NOT NULL DEFAULT '',
    body        TEXT NOT NULL DEFAULT '',
    tags        TEXT[] NOT NULL DEFAULT '{}',
    doc_date    DATE,
    role        TEXT,
    extra       JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX documents_owner_project_path
    ON documents (owner_id, coalesce(project_id, ''), path);

-- +goose Down
DROP TABLE documents;
```

- [ ] **Step 3: pgstore** — `internal/adapter/pgstore/documents.go`:

```go
package pgstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type DocumentStore struct{ pool *pgxpool.Pool }

func NewDocumentStore(pool *pgxpool.Pool) *DocumentStore { return &DocumentStore{pool: pool} }

const docCols = `id, owner_id, project_id, type, path, title, body, tags, doc_date, role, extra, created_at, updated_at`

func (s *DocumentStore) Create(ctx context.Context, d domain.Document) (domain.Document, error) {
	const q = `INSERT INTO documents (` + docCols + `)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING ` + docCols
	extra, err := json.Marshal(orEmpty(d.Extra))
	if err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: marshal extra: %w", err)
	}
	out, err := scanDocument(s.pool.QueryRow(ctx, q,
		d.ID, d.OwnerID, d.ProjectID, string(d.Type), d.Path, d.Title, d.Body,
		orEmptyTags(d.Tags), d.Date, d.Role, extra, d.CreatedAt, d.UpdatedAt))
	if isUniqueViolation(err) {
		return domain.Document{}, ports.ErrDocumentExists
	}
	return out, err
}

func (s *DocumentStore) Get(ctx context.Context, ownerID, id string) (domain.Document, error) {
	const q = `SELECT ` + docCols + ` FROM documents WHERE owner_id=$1 AND id=$2`
	d, err := scanDocument(s.pool.QueryRow(ctx, q, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Document{}, ports.ErrDocumentNotFound
	}
	return d, err
}

func (s *DocumentStore) List(ctx context.Context, ownerID string) ([]domain.Document, error) {
	const q = `SELECT ` + docCols + ` FROM documents WHERE owner_id=$1 ORDER BY updated_at DESC`
	rows, err := s.pool.Query(ctx, q, ownerID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list documents: %w", err)
	}
	defer rows.Close()
	var out []domain.Document
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *DocumentStore) Update(ctx context.Context, d domain.Document) (domain.Document, error) {
	const q = `UPDATE documents SET title=$1, body=$2, tags=$3, extra=$4, updated_at=$5
WHERE owner_id=$6 AND id=$7
RETURNING ` + docCols
	extra, err := json.Marshal(orEmpty(d.Extra))
	if err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: marshal extra: %w", err)
	}
	out, err := scanDocument(s.pool.QueryRow(ctx, q,
		d.Title, d.Body, orEmptyTags(d.Tags), extra, d.UpdatedAt, d.OwnerID, d.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Document{}, ports.ErrDocumentNotFound
	}
	return out, err
}

func (s *DocumentStore) Delete(ctx context.Context, ownerID, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM documents WHERE owner_id=$1 AND id=$2`, ownerID, id)
	if err != nil {
		return fmt.Errorf("pgstore: delete document: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrDocumentNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func orEmptyTags(t []string) []string {
	if t == nil {
		return []string{}
	}
	return t
}

func scanDocument(r rowScanner) (domain.Document, error) {
	var d domain.Document
	var typ string
	var extra []byte
	if err := r.Scan(&d.ID, &d.OwnerID, &d.ProjectID, &typ, &d.Path, &d.Title, &d.Body,
		&d.Tags, &d.Date, &d.Role, &extra, &d.CreatedAt, &d.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Document{}, err
		}
		return domain.Document{}, fmt.Errorf("pgstore: scan document: %w", err)
	}
	d.Type = domain.DocumentType(typ)
	if len(extra) > 0 {
		if err := json.Unmarshal(extra, &d.Extra); err != nil {
			return domain.Document{}, fmt.Errorf("pgstore: unmarshal extra: %w", err)
		}
	}
	return d, nil
}
```
`rowScanner` is already defined in `projects.go` — reuse it (same package).

- [ ] **Step 4: Round-trip test** — `internal/adapter/pgstore/documents_test.go`. Match the package's testcontainer pool/seed helper names (find them: `rg -n "func newTestPool|func seedUser" internal/adapter/pgstore/*_test.go`). Test: create free doc → Get returns it; List returns it; Update body/title → reflected; second Create with same (owner,path) → `ErrDocumentExists`; Get/Delete unknown id → `ErrDocumentNotFound`; Delete removes it.

```go
func TestDocumentStore_CRUDRoundTrip(t *testing.T) {
	pool := newTestPool(t)
	st := pgstore.NewDocumentStore(pool)
	ctx := context.Background()
	uid := seedUser(t, pool)
	now := time.Now().UTC().Truncate(time.Second)
	d := domain.Document{ID: "d1", OwnerID: uid, Type: domain.DocFree, Path: "docs/arch", Title: "Arch", Body: "# Hi", CreatedAt: now, UpdatedAt: now}
	got, err := st.Create(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "docs/arch" || got.Title != "Arch" {
		t.Fatalf("create roundtrip: %+v", got)
	}
	if g, _ := st.Get(ctx, uid, "d1"); g.Body != "# Hi" {
		t.Errorf("get body: %q", g.Body)
	}
	list, _ := st.List(ctx, uid)
	if len(list) != 1 {
		t.Fatalf("list len %d", len(list))
	}
	d.Title, d.Body, d.UpdatedAt = "Arch2", "# Bye", now.Add(time.Minute)
	if u, err := st.Update(ctx, d); err != nil || u.Title != "Arch2" || u.Body != "# Bye" {
		t.Fatalf("update: %+v err %v", u, err)
	}
	// duplicate path
	dup := domain.Document{ID: "d2", OwnerID: uid, Type: domain.DocFree, Path: "docs/arch", CreatedAt: now, UpdatedAt: now}
	if _, err := st.Create(ctx, dup); !errors.Is(err, ports.ErrDocumentExists) {
		t.Errorf("dup path: want ErrDocumentExists, got %v", err)
	}
	if _, err := st.Get(ctx, uid, "nope"); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Errorf("get unknown: %v", err)
	}
	if err := st.Delete(ctx, uid, "d1"); err != nil {
		t.Fatal(err)
	}
	if err := st.Delete(ctx, uid, "d1"); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Errorf("delete twice: %v", err)
	}
}
```

- [ ] **Step 5: FakeDocumentStore** — in `internal/testutil/fakes.go`, add a `FakeDocumentStore` mirroring `FakeProjectStore` (mutex + `map[string]domain.Document` keyed by id, owner-scoped reads, `(owner,project,path)` collision → `ports.ErrDocumentExists`, `NewFakeDocumentStore()` constructor). Implement all 5 `ports.DocumentStore` methods.

- [ ] **Step 6: Run + commit**:
```bash
go test ./internal/adapter/pgstore/ ./internal/testutil/ 2>&1 | tail -8   # pgstore needs Docker
go build ./... && go vet ./internal/...
git add internal/ports/ports.go internal/adapter/pgstore/migrations/0006_documents.sql internal/adapter/pgstore/documents.go internal/adapter/pgstore/documents_test.go internal/testutil/fakes.go
git commit -m "feat(docs): DocumentStore port + migration 0006 + pgstore CRUD + fake"
```

---

## Task 3: Usecases (Create/Get/List/Update/Delete)

**Files:**
- Create: `internal/usecase/create_document.go`, `get_document.go`, `list_documents.go`, `update_document.go`, `delete_document.go`, `document_test.go`

- [ ] **Step 1: Implement** the five usecases.

`create_document.go`:
```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// CreateDocument stamps id+timestamps, derives the daily path from the date,
// validates, and persists an owner-scoped document.
type CreateDocument struct {
	Docs  ports.DocumentStore
	IDs   ports.IDGen
	Clock ports.Clock
}

// CreateDocumentInput is the caller-supplied shape (the use case fills the rest).
type CreateDocumentInput struct {
	Type      domain.DocumentType
	ProjectID *string
	Path      string
	Title     string
	Body      string
}

func (uc CreateDocument) Execute(ctx context.Context, ownerID string, in CreateDocumentInput) (domain.Document, error) {
	now := uc.Clock.Now()
	d := domain.Document{
		ID: uc.IDs.NewID(), OwnerID: ownerID, ProjectID: in.ProjectID, Type: in.Type,
		Path: in.Path, Title: in.Title, Body: in.Body, CreatedAt: now, UpdatedAt: now,
	}
	if in.Type == domain.DocDaily {
		d.Date = &now
		d.Path = domain.DailyPath(now)
	}
	if err := d.Validate(); err != nil {
		return domain.Document{}, err
	}
	return uc.Docs.Create(ctx, d)
}
```

`get_document.go`:
```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type GetDocument struct{ Docs ports.DocumentStore }

func (uc GetDocument) Execute(ctx context.Context, ownerID, id string) (domain.Document, error) {
	return uc.Docs.Get(ctx, ownerID, id)
}
```

`list_documents.go`:
```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type ListDocuments struct{ Docs ports.DocumentStore }

func (uc ListDocuments) Execute(ctx context.Context, ownerID string) ([]domain.Document, error) {
	return uc.Docs.List(ctx, ownerID)
}
```

`update_document.go`:
```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// UpdateDocument edits title/body/tags of an owner's document. Path/type/project
// are immutable in the spine.
type UpdateDocument struct {
	Docs  ports.DocumentStore
	Clock ports.Clock
}

type UpdateDocumentInput struct {
	Title string
	Body  string
	Tags  []string
}

func (uc UpdateDocument) Execute(ctx context.Context, ownerID, id string, in UpdateDocumentInput) (domain.Document, error) {
	cur, err := uc.Docs.Get(ctx, ownerID, id)
	if err != nil {
		return domain.Document{}, err
	}
	cur.Title, cur.Body, cur.Tags = in.Title, in.Body, in.Tags
	cur.UpdatedAt = uc.Clock.Now()
	return uc.Docs.Update(ctx, cur)
}
```

`delete_document.go`:
```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/ports"
)

type DeleteDocument struct{ Docs ports.DocumentStore }

func (uc DeleteDocument) Execute(ctx context.Context, ownerID, id string) error {
	return uc.Docs.Delete(ctx, ownerID, id)
}
```

- [ ] **Step 2: Tests** — `internal/usecase/document_test.go`. Use `testutil.NewFakeDocumentStore()` + an existing fake IDGen/Clock (find them: `rg -n "FakeIDGen|fixedClock|FakeClock" internal/testutil internal/usecase`). Cover: create free doc (id+timestamps stamped, persisted); create daily (path derived `daily/<date>`, Date set); create project without projectID → `ErrInvalidDocument`; update changes title/body + bumps updatedAt; update unknown → `ErrDocumentNotFound`; delete; list owner-scoped.

```go
func TestCreateDocument_FreeAndDaily(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)}
	uc := usecase.CreateDocument{Docs: docs, IDs: &testutil.FakeIDGen{}, Clock: clk}
	free, err := uc.Execute(context.Background(), "u1", usecase.CreateDocumentInput{Type: domain.DocFree, Path: "docs/x", Title: "X"})
	if err != nil || free.ID == "" || free.OwnerID != "u1" {
		t.Fatalf("free: %+v err %v", free, err)
	}
	daily, err := uc.Execute(context.Background(), "u1", usecase.CreateDocumentInput{Type: domain.DocDaily})
	if err != nil || daily.Path != "daily/2026-06-15" || daily.Date == nil {
		t.Fatalf("daily: %+v err %v", daily, err)
	}
	if _, err := uc.Execute(context.Background(), "u1", usecase.CreateDocumentInput{Type: domain.DocProject, Path: "p/x"}); err == nil {
		t.Error("project without projectID should fail")
	}
}
```
(Adapt `testutil.FakeClock`/`FakeIDGen` to the real names. Add the update/delete/list tests analogously.)

- [ ] **Step 3: Run + commit**:
```bash
go test ./internal/usecase/ ./internal/domain/ 2>&1 | tail
go vet ./internal/usecase/ && gofmt -l internal/usecase
git add internal/usecase/create_document.go internal/usecase/get_document.go internal/usecase/list_documents.go internal/usecase/update_document.go internal/usecase/delete_document.go internal/usecase/document_test.go
git commit -m "feat(docs): document usecases (create/get/list/update/delete)"
```

---

## Task 4: REST handlers + routes + SSE events

**Files:**
- Modify: `internal/domain/event.go`, `internal/adapter/httpserver/server.go`
- Create: `internal/adapter/httpserver/documents.go`, `internal/adapter/httpserver/documents_test.go`

- [ ] **Step 1: Events** — in `internal/domain/event.go`, add consts: `EventDocumentCreated EventType = "document.created"`, `EventDocumentUpdated EventType = "document.updated"`, `EventDocumentDeleted EventType = "document.deleted"`.

- [ ] **Step 2: Handlers** — `internal/adapter/httpserver/documents.go`:

```go
package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

type createDocReq struct {
	Type      string  `json:"type"`
	ProjectID *string `json:"projectId"`
	Path      string  `json:"path"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
}

func (s *Server) handleCreateDocument(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req createDocReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	doc, err := s.CreateDocument.Execute(r.Context(), u.ID, usecase.CreateDocumentInput{
		Type: domain.DocumentType(req.Type), ProjectID: req.ProjectID, Path: req.Path, Title: req.Title, Body: req.Body,
	})
	switch {
	case errors.Is(err, domain.ErrInvalidDocument):
		http.Error(w, "invalid document", http.StatusBadRequest)
	case errors.Is(err, ports.ErrDocumentExists):
		http.Error(w, "path already exists", http.StatusConflict)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Bus.Publish(domain.Event{Type: domain.EventDocumentCreated, UserID: u.ID, Data: map[string]any{"id": doc.ID}})
		writeJSON(w, http.StatusCreated, doc)
	}
}

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	list, err := s.ListDocuments.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	doc, err := s.GetDocument.Execute(r.Context(), u.ID, r.PathValue("id"))
	switch {
	case errors.Is(err, ports.ErrDocumentNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		writeJSON(w, http.StatusOK, doc)
	}
}

type updateDocReq struct {
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Tags  []string `json:"tags"`
}

func (s *Server) handleUpdateDocument(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req updateDocReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	doc, err := s.UpdateDocument.Execute(r.Context(), u.ID, r.PathValue("id"), usecase.UpdateDocumentInput{Title: req.Title, Body: req.Body, Tags: req.Tags})
	switch {
	case errors.Is(err, ports.ErrDocumentNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Bus.Publish(domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": doc.ID}})
		writeJSON(w, http.StatusOK, doc)
	}
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	err := s.DeleteDocument.Execute(r.Context(), u.ID, id)
	switch {
	case errors.Is(err, ports.ErrDocumentNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Bus.Publish(domain.Event{Type: domain.EventDocumentDeleted, UserID: u.ID, Data: map[string]any{"id": id}})
		w.WriteHeader(http.StatusNoContent)
	}
}
```

- [ ] **Step 3: Server fields + routes** — in `internal/adapter/httpserver/server.go`, add to `Server`:
```go
	// m2a documents
	CreateDocument usecase.CreateDocument
	GetDocument    usecase.GetDocument
	ListDocuments  usecase.ListDocuments
	UpdateDocument usecase.UpdateDocument
	DeleteDocument usecase.DeleteDocument
```
In `Routes()`, after the export routes:
```go
	mux.Handle("POST /api/v1/documents", s.auth(http.HandlerFunc(s.handleCreateDocument)))
	mux.Handle("GET /api/v1/documents", s.auth(http.HandlerFunc(s.handleListDocuments)))
	mux.Handle("GET /api/v1/documents/{id}", s.auth(http.HandlerFunc(s.handleGetDocument)))
	mux.Handle("PUT /api/v1/documents/{id}", s.auth(http.HandlerFunc(s.handleUpdateDocument)))
	mux.Handle("DELETE /api/v1/documents/{id}", s.auth(http.HandlerFunc(s.handleDeleteDocument)))
```

- [ ] **Step 4: Tests** — `internal/adapter/httpserver/documents_test.go`, mirroring `export_test.go`'s harness (build a `Server` with the 5 usecases backed by `testutil.NewFakeDocumentStore()` + `FakeIDGen`/`FakeClock`; reuse `primeUser`/bearer wiring). Cover: POST create → 201 + body; POST bad type → 400; POST duplicate path → 409; GET list → 200; GET id → 200 / unknown → 404; PUT → 200; PUT unknown → 404; DELETE → 204 / unknown → 404. Also add the two new routes to `routes_test.go`'s registration table (POST/GET/PUT/DELETE `/api/v1/documents`, GET/PUT/DELETE `/api/v1/documents/x`). Assert a create publishes `document.created` if the harness exposes the bus (mirror how export/stats tests check events, if any).

- [ ] **Step 5: Run + commit**:
```bash
go test ./internal/adapter/httpserver/ 2>&1 | tail -10
go vet ./internal/adapter/httpserver/ && gofmt -w internal/adapter/httpserver/documents.go
git add internal/domain/event.go internal/adapter/httpserver/documents.go internal/adapter/httpserver/server.go internal/adapter/httpserver/documents_test.go internal/adapter/httpserver/routes_test.go
git commit -m "feat(docs): REST CRUD /api/v1/documents + SSE document.* events"
```

---

## Task 5: apiclient — document methods

**Files:**
- Create: `internal/adapter/apiclient/documents.go`, `internal/adapter/apiclient/documents_test.go`

- [ ] **Step 1: Implement** — `internal/adapter/apiclient/documents.go`:

```go
package apiclient

import (
	"context"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
)

// CreateDocumentInput mirrors the server's create payload.
type CreateDocumentInput struct {
	Type      string  `json:"type"`
	ProjectID *string `json:"projectId,omitempty"`
	Path      string  `json:"path"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
}

func (c *Client) CreateDocument(ctx context.Context, in CreateDocumentInput) (domain.Document, error) {
	var out domain.Document
	err := c.do(ctx, http.MethodPost, "/api/v1/documents", in, &out)
	return out, err
}

func (c *Client) ListDocuments(ctx context.Context) ([]domain.Document, error) {
	var out []domain.Document
	err := c.do(ctx, http.MethodGet, "/api/v1/documents", nil, &out)
	return out, err
}

func (c *Client) GetDocument(ctx context.Context, id string) (domain.Document, error) {
	var out domain.Document
	err := c.do(ctx, http.MethodGet, "/api/v1/documents/"+id, nil, &out)
	return out, err
}

// UpdateDocumentInput mirrors the server's update payload.
type UpdateDocumentInput struct {
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Tags  []string `json:"tags,omitempty"`
}

func (c *Client) UpdateDocument(ctx context.Context, id string, in UpdateDocumentInput) (domain.Document, error) {
	var out domain.Document
	err := c.do(ctx, http.MethodPut, "/api/v1/documents/"+id, in, &out)
	return out, err
}

func (c *Client) DeleteDocument(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/documents/"+id, nil, nil)
}
```
(Confirm `c.do` tolerates a non-2xx by returning an error and a nil `out` — read `client.go`'s `do` tail; if `do` needs a status check it already has one. If `DELETE` returns 204 with empty body, ensure `do` doesn't fail decoding when `out == nil` — verify in `client.go`; the existing `SetProjectRate` uses `do(..., nil)` for a 204 so this pattern already works.)

- [ ] **Step 2: Test** — `internal/adapter/apiclient/documents_test.go` (httptest, mirror `export_test.go`): assert `CreateDocument` POSTs the body to `/api/v1/documents` and decodes the returned doc; `ListDocuments` GETs and decodes; `DeleteDocument` issues DELETE to the id path. Assert the `Authorization` header is present on at least one call.

- [ ] **Step 3: Run + commit**:
```bash
go test ./internal/adapter/apiclient/ && go vet ./internal/adapter/apiclient/ && gofmt -l internal/adapter/apiclient
git add internal/adapter/apiclient/documents.go internal/adapter/apiclient/documents_test.go
git commit -m "feat(docs): apiclient document CRUD methods"
```

---

## Task 6: Markdown renderer (goldmark + bluemonday)

**Files:**
- Create: `internal/adapter/webui/markdown.go`, `internal/adapter/webui/markdown_test.go`
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add deps**:
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
go get github.com/yuin/goldmark@latest
go get github.com/microcosm-cc/bluemonday@latest
```

- [ ] **Step 2: Failing test** — `internal/adapter/webui/markdown_test.go`:
```go
package webui

import (
	"strings"
	"testing"
)

func TestRenderMarkdown_Basic(t *testing.T) {
	got := string(RenderMarkdown("# Title\n\nsome **bold** text"))
	if !strings.Contains(got, "<h1") || !strings.Contains(got, "<strong>bold</strong>") {
		t.Errorf("markdown not rendered: %q", got)
	}
}

func TestRenderMarkdown_SanitizesScript(t *testing.T) {
	got := string(RenderMarkdown("hi\n\n<script>alert(1)</script>\n\n[x](javascript:alert(1))"))
	if strings.Contains(got, "<script>") || strings.Contains(got, "javascript:") {
		t.Errorf("unsafe content not sanitized: %q", got)
	}
}
```

- [ ] **Step 3: Run — FAIL**: `go test ./internal/adapter/webui/ -run TestRenderMarkdown` → undefined.

- [ ] **Step 4: Implement** — `internal/adapter/webui/markdown.go`:
```go
package webui

import (
	"bytes"
	"html/template"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

var (
	md       = goldmark.New()
	ugcPolicy = bluemonday.UGCPolicy()
)

// RenderMarkdown converts user-authored Markdown to sanitised HTML safe for
// embedding in a template. The bluemonday UGC policy strips <script>,
// javascript: URLs, and other XSS vectors.
func RenderMarkdown(src string) template.HTML {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(src))
	}
	clean := ugcPolicy.SanitizeBytes(buf.Bytes())
	return template.HTML(clean)
}
```

- [ ] **Step 5: Run — PASS**: `go test ./internal/adapter/webui/ -run TestRenderMarkdown`, `go vet ./internal/adapter/webui/`, `gofmt -l internal/adapter/webui`.

- [ ] **Step 6: Commit**:
```bash
git add internal/adapter/webui/markdown.go internal/adapter/webui/markdown_test.go go.mod go.sum
git commit -m "feat(docs): sanitised markdown renderer (goldmark + bluemonday)"
```

---

## Task 7: WebUI /docs — list/view/form + nav

Mirror the Stats WebUI (`stats.templ` + `webui_stats.go`). READ both first.

**Files:**
- Create: `internal/adapter/webui/docs.templ` (+ generated `_templ.go`), `internal/adapter/httpserver/webui_docs.go`, `internal/adapter/httpserver/webui_docs_test.go`
- Modify: `internal/adapter/httpserver/server.go` (routes), `internal/adapter/webui/{worktime,dayoffs,stats,export}.templ` (nav link)

- [ ] **Step 1: templ page** — `internal/adapter/webui/docs.templ` with a `DocsPageData` view struct (`User string`, `Docs []DocRow{ID, Type, Path, Title string}`, and for the single-view: `Current *DocDetail{ID, Type, Path, Title string; HTML templ.HTML; Body string}`). Components: `DocsPage(d DocsPageData)` (full page, same layout/nav as `StatsPage`; list of docs linking to `/docs/{id}`), `DocsListFragment(d DocsPageData)` (HTMX-swappable list), `DocView(d DocDetail)` (renders `d.HTML` from `RenderMarkdown`, with edit/delete buttons), `DocForm(d DocsPageData, editing *DocDetail)` (create/edit form: `type` select [free/project/daily/agent], `projectId` input, `path` input, `title` input, `body` textarea; posts to `/docs` or `/docs/{id}`). Mirror `stats.templ`'s container/classes precisely. Nav header includes the new `docs` link.

- [ ] **Step 2: nav link** — in `worktime.templ`, `dayoffs.templ`, `stats.templ`, `export.templ` nav headers, add `<a href="/docs">docs</a>` matching the existing nav-link markup/casing (5-way symmetry: worktime/dayoffs/stats/export/docs).

- [ ] **Step 3: generate templ** — `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go tool templ generate`.

- [ ] **Step 4: handlers** — `internal/adapter/httpserver/webui_docs.go` mirroring `webui_stats.go`. Build `DocsPageData` from `s.ListDocuments.Execute`; `DocDetail` from `s.GetDocument.Execute` + `webui.RenderMarkdown(doc.Body)`. Handlers (all `webAuth`):
  - `handleWebDocsHome` — `GET /docs` (full page, list).
  - `handleWebDocsList` — `GET /ui/docs/list` (HTMX list fragment, for live refresh).
  - `handleWebDocView` — `GET /docs/{id}` (view).
  - `handleWebDocNew` — `GET /docs/new` (empty form).
  - `handleWebDocEdit` — `GET /docs/{id}/edit` (prefilled form).
  - `handleWebDocCreate` — `POST /docs` (parse form → `s.CreateDocument.Execute` → redirect to `/docs/{id}` or re-render form with error on 400/409).
  - `handleWebDocUpdate` — `POST /docs/{id}` (→ `s.UpdateDocument.Execute` → redirect).
  - `handleWebDocDelete` — `POST /docs/{id}/delete` (→ `s.DeleteDocument.Execute` → redirect to `/docs`).
  Use `r.FormValue(...)` for form fields. Map create errors: `ErrInvalidDocument`→form with 400 message, `ErrDocumentExists`→form with 409 message. Forms set `hx-boost="false"` is not needed (full nav); follow how stats' target-form posts.

- [ ] **Step 5: routes** — in `server.go` `Routes()`:
```go
	mux.Handle("GET /docs", s.webAuth(http.HandlerFunc(s.handleWebDocsHome)))
	mux.Handle("GET /ui/docs/list", s.webAuth(http.HandlerFunc(s.handleWebDocsList)))
	mux.Handle("GET /docs/new", s.webAuth(http.HandlerFunc(s.handleWebDocNew)))
	mux.Handle("POST /docs", s.webAuth(http.HandlerFunc(s.handleWebDocCreate)))
	mux.Handle("GET /docs/{id}", s.webAuth(http.HandlerFunc(s.handleWebDocView)))
	mux.Handle("GET /docs/{id}/edit", s.webAuth(http.HandlerFunc(s.handleWebDocEdit)))
	mux.Handle("POST /docs/{id}", s.webAuth(http.HandlerFunc(s.handleWebDocUpdate)))
	mux.Handle("POST /docs/{id}/delete", s.webAuth(http.HandlerFunc(s.handleWebDocDelete)))
```
(`GET /docs/new` must be registered so it isn't shadowed by `GET /docs/{id}` — Go's ServeMux prefers the more specific pattern, so both coexist; verify with a test.)

- [ ] **Step 6: test + generate + commit** — `internal/adapter/httpserver/webui_docs_test.go` (cookie harness mirror `webui_stats_test.go`): seed a doc via the fake store; `GET /docs` → 200 + body contains the doc path/title; `GET /docs/{id}` → 200 + rendered body; `POST /docs` with form values → 303/redirect + doc created; `GET /docs/new` → 200 form.
```bash
go tool templ generate && go build ./... && go test ./internal/adapter/httpserver/ ./internal/adapter/webui/ 2>&1 | tail -8
go vet ./internal/adapter/httpserver/ && git diff --quiet -- ':*_templ.go' && echo "templ in sync"
git add internal/adapter/webui/docs.templ internal/adapter/webui/docs_templ.go internal/adapter/httpserver/webui_docs.go internal/adapter/httpserver/server.go internal/adapter/httpserver/webui_docs_test.go
git add -A internal/adapter/webui/   # regenerated nav _templ.go
git commit -m "feat(docs): WebUI /docs (list/view/form, markdown render) + nav"
```

---

## Task 8: Editor adapter ($EDITOR)

**Files:**
- Create: `internal/adapter/editor/editor.go`, `internal/adapter/editor/editor_test.go`

- [ ] **Step 1: Failing test** — `internal/adapter/editor/editor_test.go`. Use a fake editor command via env: the adapter runs `$EDITOR <file>`; a test sets `EDITOR` to a small shell command that appends text. Cross-platform-safe approach: set `EDITOR` to `sh -c 'printf "edited\n" >> "$1"' sh` won't work as a single token — instead support a multi-word `$EDITOR` by splitting on spaces, OR have the test write a helper script into `t.TempDir()` and point `EDITOR` at it.

```go
package editor_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/editor"
)

func TestEditor_RoundTrip(t *testing.T) {
	// Fake editor: a shell script that appends " EDITED" to the file it's given.
	dir := t.TempDir()
	script := filepath.Join(dir, "fakeed.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf ' EDITED' >> \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", script)
	e := editor.New()
	out, err := e.Edit(context.Background(), []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "hello") || !strings.Contains(string(out), "EDITED") {
		t.Errorf("editor round-trip got %q", out)
	}
}
```

- [ ] **Step 2: Run — FAIL**: `go test ./internal/adapter/editor/` → no package.

- [ ] **Step 3: Implement** — `internal/adapter/editor/editor.go`:
```go
// Package editor opens the user's $EDITOR on a temp file (implements ports.Editor).
package editor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Editor struct{}

func New() Editor { return Editor{} }

// Edit writes initial to a temp .md file, opens $EDITOR (falling back to vi)
// on it inheriting the terminal, then reads the result back.
func (Editor) Edit(ctx context.Context, initial []byte) ([]byte, error) {
	f, err := os.CreateTemp("", "flow-doc-*.md")
	if err != nil {
		return nil, fmt.Errorf("editor: temp file: %w", err)
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := f.Write(initial); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("editor: write: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("editor: close: %w", err)
	}

	ed := os.Getenv("EDITOR")
	if ed == "" {
		ed = "vi"
	}
	parts := strings.Fields(ed)
	args := append(parts[1:], name)
	cmd := exec.CommandContext(ctx, parts[0], args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("editor: run %q: %w", ed, err)
	}
	return os.ReadFile(name)
}
```

- [ ] **Step 4: Run — PASS**: `go test ./internal/adapter/editor/`, `go vet ./internal/adapter/editor/`, `gofmt -l internal/adapter/editor`.

- [ ] **Step 5: Commit**:
```bash
git add internal/adapter/editor/editor.go internal/adapter/editor/editor_test.go
git commit -m "feat(docs): editor adapter ($EDITOR temp-file round-trip)"
```

---

## Task 9: TUI flow docs

Mirror `internal/tui/worktime.go` (Model + SSE) but a SEPARATE Model in a new file; launched by `cmd/flow/docs.go`. READ `worktime.go` + `cmd/flow/worktime.go` first.

**Files:**
- Create: `internal/tui/docs.go`, `internal/tui/docs_test.go`, `cmd/flow/docs.go`

- [ ] **Step 1: Failing test** — `internal/tui/docs_test.go` (Model-driven, mirror `worktime_test.go`). The docs Model is `DocsModel` built by `NewDocs(client *apiclient.Client, ed ports.Editor, user string)`; `client`/`ed` may be nil in tests. Cover: a `docsLoadedMsg{docs}` populates the list + View renders the paths; `j`/`k` move selection; an `eventMsg{document.created}` triggers a reload cmd; pressing Enter on a row sets a "viewing" state and the View shows the body.

```go
func TestDocs_LoadedRendersList(t *testing.T) {
	m := NewDocs(nil, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: []domain.Document{
		{ID: "d1", Type: domain.DocFree, Path: "docs/a", Title: "A"},
		{ID: "d2", Type: domain.DocProject, Path: "p/b", Title: "B"},
	}})
	out := next.(DocsModel).View().Content
	if !strings.Contains(out, "docs/a") || !strings.Contains(out, "p/b") {
		t.Fatalf("list missing paths:\n%s", out)
	}
}

func TestDocs_JKNavigation(t *testing.T) {
	m := NewDocs(nil, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: []domain.Document{{ID: "d1", Path: "a"}, {ID: "d2", Path: "b"}}})
	m = next.(DocsModel)
	n2, _ := m.Update(tea.KeyPressMsg{Text: "j"})
	if n2.(DocsModel).sel != 1 {
		t.Fatalf("j → sel 1, got %d", n2.(DocsModel).sel)
	}
}
```
(Adapt to the real field name for the selection index. Add the view/enter + event-reload tests analogously, mirroring `worktime_test.go`'s `eventMsg` test.)

- [ ] **Step 2: Run — FAIL**: `go test ./internal/tui/ -run TestDocs` → undefined.

- [ ] **Step 3: Implement** — `internal/tui/docs.go`: a `DocsModel` with fields `client *apiclient.Client`, `editor ports.Editor`, `user string`, `docs []domain.Document`, `sel int`, `viewing *domain.Document`, `events <-chan apiclient.ClientEvent`, `status string`, `err error`. Implement `Init` (reload + subscribe), `Update` (handle `docsLoadedMsg`, `eventMsg` (document.* → reload), `tea.KeyPressMsg`: `j`/`k` nav, `enter` view, `esc` back-to-list, `n` new (prompt slug/type/title then `$EDITOR` body → create), `e` edit (load body → `$EDITOR` → update), `d` delete (confirm), `q` quit), `View` (list or viewing). Reuse the existing `tui` styles (`styleHeader`/`styleMuted`/`styleSel`/`styleErr`) and the SSE `subscribe`/`waitForEvent` pattern from `worktime.go` (those helpers are package-level — reuse them; if they're methods on `Model`, replicate small equivalents for `DocsModel`). The `$EDITOR` flows run as a `tea.Cmd` that calls `m.editor.Edit(...)` then `client.CreateDocument`/`UpdateDocument`, returning a result msg. Keep the new/edit prompts minimal (mirror the booking sub-state in `worktime.go`).

  IMPORTANT: bubbletea owns the terminal; to shell out to `$EDITOR` you MUST use `tea.ExecProcess` (suspends the TUI, runs the editor, resumes) rather than calling `editor.Edit` directly inside a normal `tea.Cmd`. Check the bubbletea v2 API: `rg -n "ExecProcess|tea.Exec" $(go env GOMODCACHE)/charm.land/bubbletea*/`. Implement the editor flow via `tea.ExecProcess(exec.Command(...))` OR a `tea.Cmd` returned wrapper that bubbletea runs with the terminal released. If `tea.ExecProcess` is the available primitive, the editor adapter's tempfile logic moves into a command the TUI builds; adapt `ports.Editor` usage accordingly (the adapter can expose a `Command(initial) (*exec.Cmd, read func() []byte, cleanup func())` helper). Resolve this against the real bubbletea v2 API and keep the adapter testable. If this proves too large, STOP and report DONE_WITH_CONCERNS so the controller can split the editor-in-TUI flow into its own task.

- [ ] **Step 4: verb** — `cmd/flow/docs.go` mirroring `cmd/flow/worktime.go`:
```go
package main

import (
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/editor"
	"github.com/serverkraken/flow/internal/tui"
	"github.com/spf13/cobra"
)

func docsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "docs",
		Short: "Compendium documents (TUI)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			logf, err := os.OpenFile(filepath.Join(os.TempDir(), "flow-tui.log"),
				os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err == nil {
				defer func() { _ = logf.Close() }()
				os.Stderr = logf
			}
			m := tui.NewDocs(client, editor.New(), os.Getenv("USER"))
			_, err = tea.NewProgram(m, tea.WithContext(cmd.Context())).Run()
			return err
		},
	}
}
```

- [ ] **Step 5: Run + commit**:
```bash
go test ./internal/tui/ 2>&1 | tail -8
go build ./... && go vet ./internal/tui/ ./cmd/flow/ && gofmt -l internal/tui cmd/flow
git add internal/tui/docs.go internal/tui/docs_test.go cmd/flow/docs.go
git commit -m "feat(docs): flow docs TUI (list/view + \$EDITOR create/edit, live-synced)"
```

---

## Task 10: Wiring + curl smoke + make ci + Done-Gate

**Files:**
- Modify: `cmd/flow-server/main.go`, `cmd/flow/main.go`

- [ ] **Step 1: Server composition root** — in `cmd/flow-server/main.go`, add `documentStore := pgstore.NewDocumentStore(pool)` near the other stores, and in the `srv := &httpserver.Server{...}` add:
```go
		CreateDocument: usecase.CreateDocument{Docs: documentStore, IDs: ids, Clock: clock},
		GetDocument:    usecase.GetDocument{Docs: documentStore},
		ListDocuments:  usecase.ListDocuments{Docs: documentStore},
		UpdateDocument: usecase.UpdateDocument{Docs: documentStore, Clock: clock},
		DeleteDocument: usecase.DeleteDocument{Docs: documentStore},
```

- [ ] **Step 2: CLI wiring** — in `cmd/flow/main.go` `rootCmd()`, add `root.AddCommand(docsCmd())`.

- [ ] **Step 3: Build + vet + gofmt + templ sync**:
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
gofmt -w cmd/flow-server/main.go cmd/flow/main.go
go build ./... && go vet ./...
go tool templ generate && git diff --quiet -- ':*_templ.go' && echo "templ in sync"
```

- [ ] **Step 4: curl smoke (dev stack)** — bring up the dev stack ([[reference_flow_dev_env]]):
```bash
make dev-up && make dev-run &
sleep 4; TOKEN=$(make -s dev-token); BASE=http://localhost:8080
# create a free doc
DID=$(curl -fsS -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"type":"free","path":"docs/smoke","title":"Smoke","body":"# Hello\n\nbody"}' "$BASE/api/v1/documents" \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')
echo "DID=$DID"
echo "list:";   curl -fsS -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/documents" | head -c 300; echo
echo "get:";    curl -fsS -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/documents/$DID" | head -c 300; echo
echo "update:"; curl -fsS -X PUT -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"title":"Smoke2","body":"# Edited"}' "$BASE/api/v1/documents/$DID" -o /dev/null -w '%{http_code}\n'
echo "dup (409):"; curl -s -o /dev/null -w '%{http_code}\n' -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"type":"free","path":"docs/smoke","title":"x"}' "$BASE/api/v1/documents"
echo "bad type (400):"; curl -s -o /dev/null -w '%{http_code}\n' -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"type":"bogus","path":"x","title":"x"}' "$BASE/api/v1/documents"
echo "delete (204):"; curl -s -o /dev/null -w '%{http_code}\n' -X DELETE -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/documents/$DID"
```
Expected: create returns an id; list/get show the doc; update 200; dup 409; bad type 400; delete 204.

- [ ] **Step 5: make ci** — `make ci`. Expected: green incl. coverage gate ≥80%. New handlers/render/usecases carry coverage; if the gate dips, add happy-path tests (do NOT lower the threshold).

- [ ] **Step 6: Done-Gate manuell**
  - WebUI `/docs`: Notiz anlegen (type=free, path, title, body) → erscheint in der Liste; öffnen zeigt den Body als gerendertes Markdown.
  - `flow docs` (TUI): dieselbe Notiz erscheint live in der Liste (SSE); `e` öffnet `$EDITOR`, Body ändern + speichern → Änderung erscheint live in der WebUI-Ansicht.
  - SSE-Roundtrip via `curl -N "$BASE/api/v1/events"` parallel zu einem Create → `document.created` erscheint auf dem Stream.

- [ ] **Step 7: Commit + verify HEAD**:
```bash
git add cmd/flow-server/main.go cmd/flow/main.go
git commit -m "feat(docs): wire DocumentStore + usecases + flow docs into composition roots"
git log --oneline -12 && git status
```
Expected: alle M2a-Commits auf `rebuild`, Worktree clean. (Lesson [[feedback_subagent_git_commits_isolated]] — HEAD nach jedem Subagent prüfen, finales Wiring selbst fahren.)

---

## Self-Review-Notiz (vom Planautor)

**Spec-Coverage:** Document-Domain+Validierung (T1) ✓; Port+Migration0006+pgstore-CRUD+Unique→Exists (T2) ✓; Usecases inkl. daily-Pfad-Ableitung (T3) ✓; REST-CRUD je Route + 400/404/409 + SSE document.* (T4) ✓; apiclient (T5) ✓; Markdown-Render+Sanitizing (T6) ✓; WebUI /docs list/view/form + Nav-Symmetrie (T7) ✓; Editor-Adapter $EDITOR (T8) ✓; TUI flow docs live-synced + $EDITOR-Flow (T9) ✓; Wiring+curl+make-ci+Done-Gate (T10) ✓.

**Bewusste Defaults/Risiken:** Volle Schema-Spalten in 0006 (tags/role/date/extra) trotz späterer Nutzung — vermeidet ALTER-Churn. PUT ändert nur title/body/tags (kein path/type/project-Rename). **Risiko T9:** der `$EDITOR`-Aufruf aus Bubbletea braucht `tea.ExecProcess` (Terminal-Suspend) — gegen die echte v2-API auflösen; bei zu großem Umfang T9 in „TUI-Liste/Ansicht" + „TUI-$EDITOR-Flow" splitten (DONE_WITH_CONCERNS melden). pgstore-Tests brauchen Docker.
