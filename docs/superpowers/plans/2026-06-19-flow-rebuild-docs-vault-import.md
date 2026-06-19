# flow docs import (typen-treuer Vault-Import) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ein `flow docs import <dir>` Kommando, das einen flow-geformten Markdown-Vault typen-treu (daily/project/free, mit Original-Datum & -Projekt) idempotent in flow übernimmt.

**Architecture:** Neue `usecase.ImportDocument` persistiert Dokumente verbatim (ohne den Daily/now-Override von `CreateDocument`) hinter `POST /api/v1/documents/import`. Eine Cobra-Subcommand `flow docs import` walkt den Vault, parst Frontmatter, slugifiziert Pfade, löst `project:`-Pfade clientseitig find-or-create auf und ruft den Import-Endpoint je Datei — idempotent über einen Pre-List-Pfad-Check.

**Tech Stack:** Go, hexagonale Schichten (`domain`/`ports`/`usecase`/`adapter`), `net/http` ServeMux (Go 1.22 Patterns), Cobra, `gopkg.in/yaml.v3`, `testutil`-Fakes.

**Spec:** `docs/superpowers/specs/2026-06-19-flow-rebuild-docs-vault-import-design.md`

## Global Constraints

- Hexagonale Regeln: `usecase` hängt nur an `ports` + `domain`; HTTP-Adapter ruft `usecase`; CLI ruft `apiclient`. Kleine fokussierte Dateien (eine Verantwortung). Composition-Root ist `cmd/flow-server/main.go` (Server) bzw. `cmd/flow/*.go` (CLI).
- `Document.Validate` verlangt `SlugOK(Path)` mit `slugRe = ^[a-z0-9]+(?:-[a-z0-9]+)*(?:/[a-z0-9]+(?:-[a-z0-9]+)*)*$` (nur `[a-z0-9]`, `-`, `/`). **Alle** Import-Pfade müssen durch `slugify` (Task 4) — sonst `400`.
- `daily` braucht `Date`, `project` braucht `ProjectID` (`Document.Validate`). Import liefert beide explizit.
- Idempotenz primär über Pre-List der vorhandenen Pfade; `409` (`ports.ErrDocumentExists`) ist der Race-Backstop.
- `apiclient.CreateProject(ctx, name)` setzt **nur** den Namen → Projekte werden mit Name = vollem `project:`-Pfad angelegt (idempotenter Schlüssel).
- `make ci` muss grün bleiben: lint (`golangci-lint`), `verify-generate`, build, Coverage ≥ 80 %.
- **Never stage** vorhandenes Working-Tree-Rauschen: `.gitignore`, `flow`, `cover*.out`. Nur die je Task genannten Dateien stagen.
- Charm/v2-Importpfade nur dort, wo TUI berührt wird (hier nicht relevant).
- Referenz-Patterns zum Spiegeln: `internal/usecase/create_document.go`, `internal/adapter/httpserver/documents.go` (`handleCreateDocument`), `internal/adapter/apiclient/{documents.go,client.go}`, `cmd/flow/docs.go`, `cmd/flow-server/main.go`.

## File Structure

**New:**
- `internal/usecase/import_document.go` — `ImportDocument` Usecase (verbatim persist).
- `internal/usecase/import_document_test.go` — Usecase-Tests.
- `cmd/flow/docs_import.go` — CLI-Subcommand + Helper (slugify, frontmatter, title, date, project-resolver, walker).
- `cmd/flow/docs_import_test.go` — CLI-Tests (Helper + httptest-Integration).

**Modified:**
- `internal/adapter/httpserver/server.go` — `ImportDocument`-Feld + Route-Registrierung.
- `internal/adapter/httpserver/documents.go` — `handleImportDocument`.
- `internal/adapter/httpserver/documents_test.go` — Handler-Tests.
- `internal/adapter/apiclient/client.go` — typisierter `APIError` aus `do` + `IsConflict`.
- `internal/adapter/apiclient/documents.go` — `ImportDocumentInput` + `ImportDocument`.
- `internal/adapter/apiclient/documents_test.go` / `client_test.go` — apiclient-Tests.
- `cmd/flow/docs.go` — `docsCmd()` hängt das `import`-Subcommand an.
- `cmd/flow-server/main.go` — `ImportDocument`-Usecase wiren.

---

## Task 1: `usecase.ImportDocument` (verbatim persist)

**Files:**
- Create: `internal/usecase/import_document.go`, `internal/usecase/import_document_test.go`

**Interfaces:**
- Consumes: `ports.DocumentStore`, `ports.IDGen`, `ports.Clock`, `ports.DocChangeNotifier`; `domain.{Document,DocumentType,ParseFrontmatter,WikilinkTargets,StripHighlightSentinels,ErrInvalidDocument}`; `ports.ErrDocumentExists`.
- Produces: `usecase.ImportDocument{Docs,IDs,Clock,Notifier}` mit `Execute(ctx, ownerID string, in ImportDocumentInput) (domain.Document, error)`; `ImportDocumentInput{Type domain.DocumentType; Path, Title, Body string; Date *time.Time; ProjectID *string}`. Consumed by Tasks 2 & 7.

- [ ] **Step 1: Write the failing test**

Create `internal/usecase/import_document_test.go` (mirrors `document_test.go` style — `package usecase_test`, `testutil.NewFakeDocumentStore()`, `testutil.FakeClock`, `testutil.FakeIDGen`):
```go
package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func newImport(docs ports.DocumentStore) usecase.ImportDocument {
	return usecase.ImportDocument{
		Docs:  docs,
		IDs:   &testutil.FakeIDGen{},
		Clock: testutil.FakeClock{T: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)},
	}
}

// A daily import keeps its ORIGINAL date and path (no CreateDocument now-override).
func TestImportDocument_DailyKeepsHistoricalDateAndPath(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	uc := newImport(docs)
	d0 := time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)
	got, err := uc.Execute(context.Background(), "owner-1", usecase.ImportDocumentInput{
		Type: domain.DocDaily, Path: "daily/2026-04-28", Title: "2026-04-28",
		Body: "# 2026-04-28\n", Date: &d0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "daily/2026-04-28" {
		t.Fatalf("path overridden: %q", got.Path)
	}
	if got.Date == nil || !got.Date.Equal(d0) {
		t.Fatalf("date not preserved: %v", got.Date)
	}
}

// A project import persists the provided ProjectID and parses frontmatter tags.
func TestImportDocument_ProjectAndTags(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	uc := newImport(docs)
	pid := "proj-1"
	got, err := uc.Execute(context.Background(), "owner-1", usecase.ImportDocumentInput{
		Type: domain.DocProject, Path: "projects/foo/readme", Title: "Foo",
		Body: "---\ntags: [infra, gcp]\n---\n# Foo\n", ProjectID: &pid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectID == nil || *got.ProjectID != "proj-1" {
		t.Fatalf("projectID not persisted: %v", got.ProjectID)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "infra" {
		t.Fatalf("tags not parsed from frontmatter: %v", got.Tags)
	}
}

// Re-importing the same path surfaces the store's duplicate error for the caller to skip.
func TestImportDocument_DuplicatePath(t *testing.T) {
	docs := testutil.NewFakeDocumentStore()
	uc := newImport(docs)
	in := usecase.ImportDocumentInput{Type: domain.DocFree, Path: "notes/onboarding", Title: "O", Body: "x"}
	if _, err := uc.Execute(context.Background(), "owner-1", in); err != nil {
		t.Fatal(err)
	}
	_, err := uc.Execute(context.Background(), "owner-1", in)
	if !errors.Is(err, ports.ErrDocumentExists) {
		t.Fatalf("want ErrDocumentExists on duplicate path, got %v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/usecase/ -run TestImportDocument -v`
Expected: FAIL — `usecase.ImportDocument` / `ImportDocumentInput` undefined.

- [ ] **Step 3: Implement the usecase**

Create `internal/usecase/import_document.go`:
```go
package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ImportDocument persists a document verbatim: it honours the caller's path,
// type, date and project (unlike CreateDocument, which stamps daily docs with
// today's date and the canonical daily path). Used to re-home an existing
// corpus with its original identity. It still stamps id+timestamps, derives
// tags from frontmatter, validates, and extracts wikilinks.
type ImportDocument struct {
	Docs     ports.DocumentStore
	IDs      ports.IDGen
	Clock    ports.Clock
	Notifier ports.DocChangeNotifier // optional; nil → no notification
}

// ImportDocumentInput is the caller-supplied shape for a verbatim import.
type ImportDocumentInput struct {
	Type      domain.DocumentType
	Path      string
	Title     string
	Body      string
	Date      *time.Time
	ProjectID *string
}

func (uc ImportDocument) Execute(ctx context.Context, ownerID string, in ImportDocumentInput) (domain.Document, error) {
	now := uc.Clock.Now()
	d := domain.Document{
		ID: uc.IDs.NewID(), OwnerID: ownerID, ProjectID: in.ProjectID, Type: in.Type,
		Path:  in.Path,
		Title: domain.StripHighlightSentinels(in.Title),
		Body:  domain.StripHighlightSentinels(in.Body),
		Date:  in.Date, CreatedAt: now, UpdatedAt: now,
	}
	tags, bodyStart := domain.ParseFrontmatter(d.Body)
	d.Tags = tags
	if err := d.Validate(); err != nil {
		return domain.Document{}, err
	}
	created, err := uc.Docs.Create(ctx, d)
	if err != nil {
		return domain.Document{}, err
	}
	if err := uc.Docs.ReplaceLinks(ctx, created.ID, ownerID, domain.WikilinkTargets(created.Body[bodyStart:])); err != nil {
		return domain.Document{}, err
	}
	if uc.Notifier != nil {
		uc.Notifier.DocumentChanged()
	}
	return created, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/usecase/ -run TestImportDocument -v`
Expected: PASS (3/3).

- [ ] **Step 5: Lint + commit**

Run: `golangci-lint run ./internal/usecase/`
```bash
git add internal/usecase/import_document.go internal/usecase/import_document_test.go
git commit -m "feat(import): ImportDocument usecase — persist a document verbatim"
```

---

## Task 2: Import endpoint + server wiring (`POST /api/v1/documents/import`)

**Files:**
- Modify: `internal/adapter/httpserver/server.go` (add `ImportDocument` field + route), `internal/adapter/httpserver/documents.go` (handler), `cmd/flow-server/main.go` (wire usecase)
- Test: `internal/adapter/httpserver/documents_test.go`

**Interfaces:**
- Consumes: `usecase.ImportDocument` (Task 1).
- Produces: HTTP `POST /api/v1/documents/import` → `201` + `domain.Document` / `400` (`domain.ErrInvalidDocument`) / `409` (`ports.ErrDocumentExists`). Consumed by Task 3 (apiclient) & Task 7 (smoke).

- [ ] **Step 1: Write the failing test**

In `internal/adapter/httpserver/documents_test.go`, add (mirror the existing `doDoc` helper + `Server` construction used by the create tests — reuse whatever `newTestServer`/inline `Server{…}` the file already uses, just adding `ImportDocument: usecase.ImportDocument{Docs: docs, IDs: ids, Clock: clk}` to the struct literal):
```go
func TestImportDocument_HappyDailyHistorical(t *testing.T) {
	ts, _ := newDocServer(t) // use the file's existing test-server constructor
	defer ts.Close()
	body := `{"type":"daily","path":"daily/2026-04-28","title":"2026-04-28","body":"# 2026-04-28\n","date":"2026-04-28T00:00:00Z"}`
	res := doDoc(t, ts, "POST", "/api/v1/documents/import", body)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
}

func TestImportDocument_BadType(t *testing.T) {
	ts, _ := newDocServer(t)
	defer ts.Close()
	res := doDoc(t, ts, "POST", "/api/v1/documents/import", `{"type":"bogus","path":"x","title":"T","body":"B"}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

func TestImportDocument_DuplicatePath(t *testing.T) {
	ts, _ := newDocServer(t)
	defer ts.Close()
	body := `{"type":"free","path":"notes/dup","title":"T","body":"B"}`
	doDoc(t, ts, "POST", "/api/v1/documents/import", body)
	res := doDoc(t, ts, "POST", "/api/v1/documents/import", body)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", res.StatusCode)
	}
}
```
> Adapt `newDocServer`/`doDoc`/the `Server{…}` literal names to whatever `documents_test.go` already defines (the create-document tests in this file show the exact helpers). The only new wiring in the test server is the `ImportDocument` usecase field.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/adapter/httpserver/ -run TestImportDocument -v`
Expected: FAIL — route 404 / `ImportDocument` field undefined.

- [ ] **Step 3: Add the `ImportDocument` field + route**

In `internal/adapter/httpserver/server.go`, in the `// m2a documents` block of the `Server` struct, add after `CreateDocument`:
```go
	ImportDocument    usecase.ImportDocument
```
And next to the documents route registrations (where `mux.Handle("POST /api/v1/documents", …)` is), add:
```go
	mux.Handle("POST /api/v1/documents/import", s.auth(http.HandlerFunc(s.handleImportDocument)))
```

- [ ] **Step 4: Add the handler**

In `internal/adapter/httpserver/documents.go`, add `"time"` to the imports and append:
```go
type importDocReq struct {
	Type      string     `json:"type"`
	Path      string     `json:"path"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Date      *time.Time `json:"date"`
	ProjectID *string    `json:"projectId"`
}

func (s *Server) handleImportDocument(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req importDocReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	doc, err := s.ImportDocument.Execute(r.Context(), u.ID, usecase.ImportDocumentInput{
		Type: domain.DocumentType(req.Type), Path: req.Path, Title: req.Title,
		Body: req.Body, Date: req.Date, ProjectID: req.ProjectID,
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
```

- [ ] **Step 5: Wire the usecase in the composition root**

In `cmd/flow-server/main.go`, find the `CreateDocument: usecase.CreateDocument{Docs: documentStore, IDs: ids, Clock: clock, Notifier: embedWorker},` line in the `httpserver.Server{…}` literal and add directly after it:
```go
		ImportDocument:    usecase.ImportDocument{Docs: documentStore, IDs: ids, Clock: clock, Notifier: embedWorker},
```

- [ ] **Step 6: Build, run tests**

Run: `go build ./...`
Run: `go test ./internal/adapter/httpserver/ -run TestImportDocument -v`
Expected: PASS (3/3). Then full package: `go test ./internal/adapter/httpserver/`.

- [ ] **Step 7: Lint + commit**

Run: `golangci-lint run ./internal/adapter/httpserver/ ./cmd/flow-server/`
```bash
git add internal/adapter/httpserver/server.go internal/adapter/httpserver/documents.go internal/adapter/httpserver/documents_test.go cmd/flow-server/main.go
git commit -m "feat(import): POST /documents/import endpoint + server wiring"
```

---

## Task 3: apiclient — `ImportDocument` + typed `APIError`/`IsConflict`

**Files:**
- Modify: `internal/adapter/apiclient/client.go` (typed error from `do`), `internal/adapter/apiclient/documents.go` (import method)
- Test: `internal/adapter/apiclient/documents_test.go`

**Interfaces:**
- Produces: `apiclient.APIError{Method, Path string; StatusCode int}` (implements `error`); `apiclient.IsConflict(err error) bool`; `apiclient.ImportDocumentInput{Type, Path, Title, Body string; Date *time.Time; ProjectID *string}`; `func (c *Client) ImportDocument(ctx, in) (domain.Document, error)`. Consumed by Tasks 5 & 6.

- [ ] **Step 1: Write the failing test**

In `internal/adapter/apiclient/documents_test.go`, add (mirror the existing httptest-based `TestCreateDocument_*` setup in that file):
```go
func TestImportDocument_PostsBodyAndPath(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(domain.Document{ID: "d1"})
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")
	d0 := time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)
	pid := "p1"
	_, err := c.ImportDocument(context.Background(), apiclient.ImportDocumentInput{
		Type: "project", Path: "projects/foo/readme", Title: "Foo", Body: "B", Date: &d0, ProjectID: &pid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/documents/import" {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, `"projectId":"p1"`) || !strings.Contains(gotBody, `"date":"2026-04-28`) {
		t.Fatalf("body missing fields: %s", gotBody)
	}
}

func TestImportDocument_ConflictIsTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")
	_, err := c.ImportDocument(context.Background(), apiclient.ImportDocumentInput{Type: "free", Path: "x", Title: "T", Body: "B"})
	if !apiclient.IsConflict(err) {
		t.Fatalf("want IsConflict, got %v", err)
	}
}
```
> Ensure the test file imports `io`, `time`, `strings`, `net/http`, `net/http/httptest`, `encoding/json` as needed (the create tests already import most).

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/adapter/apiclient/ -run TestImportDocument -v`
Expected: FAIL — `ImportDocument`/`IsConflict`/`APIError` undefined; conflict currently returns an untyped error.

- [ ] **Step 3: Typed error from `do`**

In `internal/adapter/apiclient/client.go`, add `"errors"` to imports and replace the non-2xx return in `do`:
```go
	if res.StatusCode >= 300 {
		return &APIError{Method: method, Path: path, StatusCode: res.StatusCode}
	}
```
Then add (anywhere in the file, e.g. after `do`):
```go
// APIError is returned by do for any non-2xx response so callers can branch on
// the status (e.g. skip a 409 conflict). The message is unchanged, so existing
// `err != nil` callers are unaffected.
type APIError struct {
	Method, Path string
	StatusCode   int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("apiclient: %s %s: status %d", e.Method, e.Path, e.StatusCode)
}

// IsConflict reports whether err is an APIError with HTTP 409.
func IsConflict(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.StatusCode == http.StatusConflict
}
```
> `fmt` and `http` are already imported in `client.go`.

- [ ] **Step 4: Add `ImportDocument`**

In `internal/adapter/apiclient/documents.go`, add `"time"` to imports and append:
```go
// ImportDocumentInput mirrors the server's import payload (verbatim persist).
type ImportDocumentInput struct {
	Type      string     `json:"type"`
	Path      string     `json:"path"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Date      *time.Time `json:"date,omitempty"`
	ProjectID *string    `json:"projectId,omitempty"`
}

func (c *Client) ImportDocument(ctx context.Context, in ImportDocumentInput) (domain.Document, error) {
	var out domain.Document
	err := c.do(ctx, http.MethodPost, "/api/v1/documents/import", in, &out)
	return out, err
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/adapter/apiclient/ -v`
Expected: PASS — new tests green AND existing tests (which assert `err != nil` on non-2xx) still pass (the typed error still satisfies `error`).

- [ ] **Step 6: Lint + commit**

Run: `golangci-lint run ./internal/adapter/apiclient/`
```bash
git add internal/adapter/apiclient/client.go internal/adapter/apiclient/documents.go internal/adapter/apiclient/documents_test.go
git commit -m "feat(import): apiclient.ImportDocument + typed APIError/IsConflict"
```

---

## Task 4: CLI helpers — slugify, frontmatter, title, date

**Files:**
- Create: `cmd/flow/docs_import.go` (helpers only this task), `cmd/flow/docs_import_test.go`

**Interfaces:**
- Consumes: `domain.{ParseFrontmatter,SlugOK}`, `gopkg.in/yaml.v3`.
- Produces: `slugify(p string) string`; `vaultFrontmatter{ID,Type,Date,Project string}`; `parseVaultFrontmatter(body string) vaultFrontmatter`; `titleFromBody(body string) string`; `importDate(fm vaultFrontmatter, filename string) *time.Time`. Consumed by Task 6.

- [ ] **Step 1: Write the failing test**

Create `cmd/flow/docs_import_test.go`:
```go
package main

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestSlugify_ProducesValidSlugs(t *testing.T) {
	cases := map[string]string{
		"daily/2026-04-28":   "daily/2026-04-28",
		"notes/Onboarding":   "notes/onboarding",
		"notes/Jira Service Manager": "notes/jira-service-manager",
		"projects/gitlab.com/dataalliance/sql-credentials/_project": "projects/gitlab-com/dataalliance/sql-credentials/project",
		"notes/product42":    "notes/product42",
	}
	for in, want := range cases {
		got := slugify(in)
		if got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
		if !domain.SlugOK(got) {
			t.Errorf("slugify(%q) = %q is not SlugOK", in, got)
		}
	}
}

func TestParseVaultFrontmatter(t *testing.T) {
	body := "---\nid: notes/Onboarding\ntype: free\n---\n# Onboarding\nbody"
	fm := parseVaultFrontmatter(body)
	if fm.ID != "notes/Onboarding" || fm.Type != "free" {
		t.Fatalf("fm = %+v", fm)
	}
}

func TestTitleFromBody(t *testing.T) {
	body := "---\ntype: free\n---\n\n# Onboarding: Neues Projekt\n\ntext"
	if got := titleFromBody(body); got != "Onboarding: Neues Projekt" {
		t.Fatalf("title = %q", got)
	}
	if got := titleFromBody("no heading here"); got != "" {
		t.Fatalf("want empty title, got %q", got)
	}
}

func TestImportDate(t *testing.T) {
	fm := vaultFrontmatter{Date: "2026-04-28"}
	d := importDate(fm, "daily/whatever.md")
	if d == nil || !d.Equal(time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date from frontmatter = %v", d)
	}
	d2 := importDate(vaultFrontmatter{}, "daily/2026-05-09.md")
	if d2 == nil || d2.Day() != 9 {
		t.Fatalf("date from filename = %v", d2)
	}
	if importDate(vaultFrontmatter{}, "notes/foo.md") != nil {
		t.Fatal("non-date file should yield nil")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/flow/ -run 'TestSlugify|TestParseVault|TestTitleFromBody|TestImportDate' -v`
Expected: FAIL — helpers undefined.

- [ ] **Step 3: Implement the helpers**

Create `cmd/flow/docs_import.go`:
```go
package main

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"gopkg.in/yaml.v3"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slugify maps a vault path (the frontmatter id or relative path without .md)
// onto a flow-valid Path matching domain.SlugOK: per "/" segment lowercase,
// collapse every run of non-[a-z0-9] to a single "-", trim leading/trailing
// "-", and drop empty segments.
func slugify(p string) string {
	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, seg := range parts {
		seg = nonSlug.ReplaceAllString(strings.ToLower(seg), "-")
		seg = strings.Trim(seg, "-")
		if seg != "" {
			out = append(out, seg)
		}
	}
	return strings.Join(out, "/")
}

// vaultFrontmatter is the subset of a note's YAML frontmatter the importer reads.
type vaultFrontmatter struct {
	ID      string `yaml:"id"`
	Type    string `yaml:"type"`
	Date    string `yaml:"date"`
	Project string `yaml:"project"`
}

// parseVaultFrontmatter extracts the leading "---\n … \n---" YAML block.
// Returns the zero value when there is no frontmatter or it is malformed.
func parseVaultFrontmatter(body string) vaultFrontmatter {
	const open = "---\n"
	var fm vaultFrontmatter
	if !strings.HasPrefix(body, open) {
		return fm
	}
	rest := body[len(open):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return fm
	}
	_ = yaml.Unmarshal([]byte(rest[:end]), &fm)
	return fm
}

// titleFromBody returns the first markdown H1 ("# …") after any frontmatter,
// or "" when there is none.
func titleFromBody(body string) string {
	_, start := domain.ParseFrontmatter(body)
	for _, ln := range strings.Split(body[start:], "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "# ") {
			return strings.TrimSpace(t[2:])
		}
	}
	return ""
}

// importDate resolves a daily document's date: the frontmatter `date`
// (YYYY-MM-DD) first, else a YYYY-MM-DD filename, else nil.
func importDate(fm vaultFrontmatter, filename string) *time.Time {
	if fm.Date != "" {
		if t, err := time.Parse("2006-01-02", fm.Date); err == nil {
			return &t
		}
	}
	base := strings.TrimSuffix(filepath.Base(filename), ".md")
	if t, err := time.Parse("2006-01-02", base); err == nil {
		return &t
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/flow/ -run 'TestSlugify|TestParseVault|TestTitleFromBody|TestImportDate' -v`
Expected: PASS.

- [ ] **Step 5: Lint + commit**

Run: `golangci-lint run ./cmd/flow/`
```bash
git add cmd/flow/docs_import.go cmd/flow/docs_import_test.go
git commit -m "feat(import): CLI helpers — slugify, frontmatter, title, date"
```

---

## Task 5: CLI project resolver (find-or-create)

**Files:**
- Modify: `cmd/flow/docs_import.go` (add resolver), `cmd/flow/docs_import_test.go` (test)

**Interfaces:**
- Consumes: `apiclient.Client.{ListProjects,CreateProject}`, `slugify` (Task 4).
- Produces: `newProjectResolver(c *apiclient.Client, dryRun bool) *projectResolver`; `(*projectResolver).resolve(ctx, projectPath string) (*string, error)`; field `created int`. Consumed by Task 6.

- [ ] **Step 1: Write the failing test**

Add to `cmd/flow/docs_import_test.go` (httptest project server):
```go
func TestProjectResolver_MatchesThenCreates(t *testing.T) {
	var created []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode([]domain.Project{{ID: "p-existing", Name: "gitlab.com/a/existing"}})
		case r.Method == "POST" && r.URL.Path == "/api/v1/projects":
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			created = append(created, in["name"])
			_ = json.NewEncoder(w).Encode(domain.Project{ID: "p-new", Name: in["name"]})
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")
	pr := newProjectResolver(c, false)

	// existing → matched by Name, no create
	id, err := pr.resolve(context.Background(), "gitlab.com/a/existing")
	if err != nil || id == nil || *id != "p-existing" {
		t.Fatalf("match existing: id=%v err=%v", id, err)
	}
	// unknown → created with full path as name
	id2, _ := pr.resolve(context.Background(), "gitlab.com/a/brand-new")
	if id2 == nil || *id2 != "p-new" {
		t.Fatalf("create: id=%v", id2)
	}
	// same path again → cached, no second create
	_, _ = pr.resolve(context.Background(), "gitlab.com/a/brand-new")
	if len(created) != 1 || created[0] != "gitlab.com/a/brand-new" {
		t.Fatalf("created = %v (want exactly one, full path)", created)
	}
	if pr.created != 1 {
		t.Fatalf("pr.created = %d, want 1", pr.created)
	}
	// empty project path → nil id, no error
	if id3, err := pr.resolve(context.Background(), ""); err != nil || id3 != nil {
		t.Fatalf("empty path: id=%v err=%v", id3, err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/flow/ -run TestProjectResolver -v`
Expected: FAIL — `newProjectResolver` undefined.

- [ ] **Step 3: Implement the resolver**

Append to `cmd/flow/docs_import.go` (add `"context"` and the apiclient import to the file's import block):
```go
// projectResolver find-or-creates flow projects for vault `project:` paths,
// caching results for the run. Existing projects are matched by Name or Slug
// (the vault path, or its slugified form); unknown paths are created with the
// full path as the project name (the only field apiclient.CreateProject sets,
// and a stable idempotency key across re-runs).
type projectResolver struct {
	client   *apiclient.Client
	cache    map[string]string // vault project path → flow project id
	existing map[string]string // Name/Slug → flow project id (lazy-loaded)
	dryRun   bool
	created  int
}

func newProjectResolver(c *apiclient.Client, dryRun bool) *projectResolver {
	return &projectResolver{client: c, cache: map[string]string{}, dryRun: dryRun}
}

func (pr *projectResolver) load(ctx context.Context) error {
	if pr.existing != nil {
		return nil
	}
	pr.existing = map[string]string{}
	list, err := pr.client.ListProjects(ctx)
	if err != nil {
		return err
	}
	for _, p := range list {
		if p.Name != "" {
			pr.existing[p.Name] = p.ID
		}
		if p.Slug != "" {
			pr.existing[p.Slug] = p.ID
		}
	}
	return nil
}

func (pr *projectResolver) resolve(ctx context.Context, projectPath string) (*string, error) {
	if projectPath == "" {
		return nil, nil
	}
	if id, ok := pr.cache[projectPath]; ok {
		return &id, nil
	}
	if err := pr.load(ctx); err != nil {
		return nil, err
	}
	if id, ok := pr.existing[projectPath]; ok {
		pr.cache[projectPath] = id
		return &id, nil
	}
	if id, ok := pr.existing[slugify(projectPath)]; ok {
		pr.cache[projectPath] = id
		return &id, nil
	}
	pr.created++
	if pr.dryRun {
		id := "(dry-run)"
		pr.cache[projectPath] = id
		return &id, nil
	}
	p, err := pr.client.CreateProject(ctx, projectPath)
	if err != nil {
		return nil, err
	}
	pr.cache[projectPath] = p.ID
	pr.existing[p.Name] = p.ID
	return &p.ID, nil
}
```
> The file's import block now needs `"context"` and `"github.com/serverkraken/flow/internal/adapter/apiclient"` in addition to the Task-4 imports.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/flow/ -run TestProjectResolver -v`
Expected: PASS.

- [ ] **Step 5: Lint + commit**

Run: `golangci-lint run ./cmd/flow/`
```bash
git add cmd/flow/docs_import.go cmd/flow/docs_import_test.go
git commit -m "feat(import): CLI project resolver (find-or-create by path)"
```

---

## Task 6: CLI command — `flow docs import <dir>`

**Files:**
- Modify: `cmd/flow/docs_import.go` (the cobra command + walker), `cmd/flow/docs.go` (attach subcommand), `cmd/flow/docs_import_test.go` (integration test)

**Interfaces:**
- Consumes: `clientFromStore` (existing in `cmd/flow`), `apiclient.Client.{ListDocuments,ImportDocument,UpdateDocument}`, `apiclient.IsConflict`, all Task 4/5 helpers.
- Produces: `docsImportCmd() *cobra.Command` (Use `import <dir>`, flags `--dry-run`, `--update`); attached to `docsCmd()`.

- [ ] **Step 1: Write the failing test**

Add to `cmd/flow/docs_import_test.go` an integration test that drives the walker against a temp vault + httptest server. Because the command builds its client via `clientFromStore`, factor the core loop into a testable function `runImport(ctx, c *apiclient.Client, dir string, dryRun, update bool) (importStats, error)` and test THAT directly:
```go
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunImport_ImportsSkipsAndDryRun(t *testing.T) {
	var posts int
	existing := []domain.Document{{ID: "d-exist", Path: "notes/existing"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/documents":
			_ = json.NewEncoder(w).Encode(existing)
		case r.Method == "GET" && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode([]domain.Project{})
		case r.URL.Path == "/api/v1/documents/import":
			posts++
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(domain.Document{ID: "new"})
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	dir := t.TempDir()
	writeFile(t, dir, "daily/2026-04-28.md", "---\nid: daily/2026-04-28\ntype: daily\ndate: \"2026-04-28\"\n---\n# 2026-04-28\n")
	writeFile(t, dir, "notes/Onboarding.md", "---\nid: notes/Onboarding\ntype: free\n---\n# Onboarding\n")
	writeFile(t, dir, "notes/existing.md", "---\nid: notes/existing\ntype: free\n---\n# Existing\n") // path already on server

	// dry-run writes nothing
	st, err := runImport(context.Background(), c, dir, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if posts != 0 {
		t.Fatalf("dry-run posted %d (want 0)", posts)
	}
	if st.imported != 2 || st.skipped != 1 {
		t.Fatalf("dry-run stats = %+v (want imported 2, skipped 1)", st)
	}

	// real run imports the 2 new, skips the existing
	posts = 0
	st, err = runImport(context.Background(), c, dir, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if posts != 2 || st.imported != 2 || st.skipped != 1 {
		t.Fatalf("run posts=%d stats=%+v (want 2 imported, 1 skipped)", posts, st)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/flow/ -run TestRunImport -v`
Expected: FAIL — `runImport`/`importStats` undefined.

- [ ] **Step 3: Implement `runImport` + the walker**

Append to `cmd/flow/docs_import.go` (extend the import block with `"fmt"`, `"io/fs"`, `"os"`):
```go
type importStats struct {
	imported, skipped, updated, failed, projectsCreated int
	failures                                            []string // "path: reason"
}

// runImport walks dir for *.md, derives each note's flow identity, resolves its
// project, and imports it (skip-existing, or --update). Errors are isolated
// per file; the walk continues.
func runImport(ctx context.Context, c *apiclient.Client, dir string, dryRun, update bool) (importStats, error) {
	var st importStats

	// Pre-list existing paths once for idempotency (+ id for --update).
	docs, err := c.ListDocuments(ctx)
	if err != nil {
		return st, fmt.Errorf("list documents: %w", err)
	}
	existingID := make(map[string]string, len(docs))
	for _, d := range docs {
		existingID[d.Path] = d.ID
	}

	pr := newProjectResolver(c, dryRun)

	walkErr := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		raw, rerr := os.ReadFile(p)
		if rerr != nil {
			st.failed++
			st.failures = append(st.failures, rel+": read: "+rerr.Error())
			return nil
		}
		body := string(raw)
		fm := parseVaultFrontmatter(body)

		rawID := fm.ID
		if rawID == "" {
			rawID = strings.TrimSuffix(rel, ".md")
		}
		path := slugify(rawID)
		if path == "" {
			st.failed++
			st.failures = append(st.failures, rel+": empty path after slugify")
			return nil
		}
		typ := fm.Type
		if typ == "" {
			typ = "free"
		}
		title := titleFromBody(body)
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(rel), ".md")
		}
		var date *time.Time
		if typ == "daily" {
			date = importDate(fm, rel)
		}
		projectID, perr := pr.resolve(ctx, fm.Project)
		if perr != nil {
			st.failed++
			st.failures = append(st.failures, rel+": project: "+perr.Error())
			return nil
		}

		if id, exists := existingID[path]; exists {
			if !update {
				st.skipped++
				return nil
			}
			if dryRun {
				st.updated++
				return nil
			}
			if _, uerr := c.UpdateDocument(ctx, id, apiclient.UpdateDocumentInput{Title: title, Body: body}); uerr != nil {
				st.failed++
				st.failures = append(st.failures, rel+": update: "+uerr.Error())
				return nil
			}
			st.updated++
			return nil
		}

		if dryRun {
			st.imported++
			existingID[path] = "(dry-run)"
			return nil
		}
		if _, ierr := c.ImportDocument(ctx, apiclient.ImportDocumentInput{
			Type: typ, Path: path, Title: title, Body: body, Date: date, ProjectID: projectID,
		}); ierr != nil {
			if apiclient.IsConflict(ierr) { // race backstop
				st.skipped++
				return nil
			}
			st.failed++
			st.failures = append(st.failures, rel+": import: "+ierr.Error())
			return nil
		}
		st.imported++
		existingID[path] = "(new)"
		return nil
	})
	st.projectsCreated = pr.created
	return st, walkErr
}

func docsImportCmd() *cobra.Command {
	var dryRun, update bool
	cmd := &cobra.Command{
		Use:   "import <dir>",
		Short: "Import a markdown vault into the compendium",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			st, err := runImport(cmd.Context(), c, args[0], dryRun, update)
			if err != nil {
				return err
			}
			mode := ""
			if dryRun {
				mode = " (dry-run)"
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"importiert %d · aktualisiert %d · übersprungen %d · Projekte angelegt %d · Fehler %d%s\n",
				st.imported, st.updated, st.skipped, st.projectsCreated, st.failed, mode)
			for _, f := range st.failures {
				fmt.Fprintln(cmd.OutOrStdout(), "  "+f)
			}
			if st.failed > 0 {
				return fmt.Errorf("%d Datei(en) fehlgeschlagen", st.failed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "parse und plane den Import, ohne zu schreiben")
	cmd.Flags().BoolVar(&update, "update", false, "vorhandene Dokumente (per Pfad) überschreiben statt überspringen")
	return cmd
}
```
> Add `"github.com/spf13/cobra"` to the file's import block.

- [ ] **Step 4: Attach the subcommand**

In `cmd/flow/docs.go`, change `docsCmd()` to build the command in a variable and attach the child before returning:
```go
func docsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Compendium documents (TUI)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// ... unchanged body ...
		},
	}
	cmd.AddCommand(docsImportCmd())
	return cmd
}
```
> Keep the existing `RunE` body verbatim; only wrap the literal in a `cmd :=` variable and add the `AddCommand` line.

- [ ] **Step 5: Build, run tests**

Run: `go build ./...`
Run: `go test ./cmd/flow/ -v`
Expected: PASS (helpers + resolver + runImport). Fix wiring until green.

- [ ] **Step 6: Lint + commit**

Run: `golangci-lint run ./cmd/flow/`
```bash
git add cmd/flow/docs_import.go cmd/flow/docs.go cmd/flow/docs_import_test.go
git commit -m "feat(import): flow docs import <dir> command (walk, idempotent, dry-run)"
```

---

## Task 7: Done-gate — full CI + live vault import

**Files:** none (verification only).

- [ ] **Step 1: Full CI gate**

Run: `make ci`
Expected: lint + verify-generate + build + tests green; Coverage ≥ 80 %. If Coverage gefallen ist, fokussierte Tests an der dünnsten neuen Fläche ergänzen (runImport-Branches: update-Pfad, IsConflict-Skip, failure-Sammlung) — Threshold nicht senken.

- [ ] **Step 2: Live curl-smoke der neuen Route**

Dev-Stack starten ([[reference_flow_dev_env]]): `make dev-up && make dev-run`, Token sicherstellen (`make dev-token`). Dann:
```bash
TOKEN=$(cat .dev-token 2>/dev/null || echo "$FLOW_TOKEN")
curl -sS -X POST localhost:8080/api/v1/documents/import \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"type":"daily","path":"daily/2026-04-28","title":"2026-04-28","body":"# 2026-04-28\n","date":"2026-04-28T00:00:00Z"}' -w '\n%{http_code}\n'
```
Expected: `201` + JSON-Dokument mit `"date":"2026-04-28..."` und `"path":"daily/2026-04-28"`. Zweiter Aufruf → `409`.

- [ ] **Step 3: Dry-run gegen den echten Vault**

```bash
go run ./cmd/flow docs import ~/notes --dry-run
```
Expected: Zusammenfassung `importiert N · … (dry-run)` ohne Schreibzugriffe; keine Fehlerzeilen (oder nur erklärbare, z.B. die `gitlab.com>`-Kollision).

- [ ] **Step 4: Echter Import + Verifikation**

```bash
go run ./cmd/flow docs import ~/notes
go run ./cmd/flow docs import ~/notes   # zweiter Lauf: alles übersprungen (idempotent)
```
Dann in der TUI prüfen (`go run ./cmd/flow docs` bzw. `flow ui` → Docs):
- [ ] Dailies erscheinen mit ihrem historischen Datum.
- [ ] Projekt-Notizen hängen an einem Projekt (neu angelegte Projekte sichtbar in der Worktime-Projektliste).
- [ ] Free-Notizen da, Titel = ursprüngliche H1.
- [ ] Tags (aus Frontmatter) gesetzt; Suche (`/`) findet Inhalte.
- [ ] Zweiter Import-Lauf meldet `übersprungen N` (0 importiert).

- [ ] **Step 5: Final commit (nur falls Coverage-Top-up-Tests ergänzt wurden)**

```bash
git add -A -- internal/ cmd/
git commit -m "test(import): coverage top-up for the vault importer"
```

---

## Notes / accepted scope boundaries

- Pfade werden **slugifiziert** (SlugOK-Zwang) — Import-Pfade weichen ggf. vom Vault-Dateinamen ab; verlustfrei, da keine Wikilinks und der Original-Name im Title bleibt.
- `--update` aktualisiert nur **Title + Body** (die Update-API kann nicht Typ/Datum/Projekt ändern); für eine volle Neuzuordnung Dokument löschen + neu importieren.
- Projekte werden mit **vollem Pfad als Name** angelegt (apiclient setzt nur den Namen); Umbenennen später in der Worktime-UI.
- Kein Archiv-/Zip-Import, kein Watch/Sync, keine Wikilink-Umschreibung (Vault hat keine).
- `gitlab.com` und das Shell-Artefakt `gitlab.com>` slugifizieren beide zu `gitlab-com`; eine Kollision wird als Duplicate übersprungen.
