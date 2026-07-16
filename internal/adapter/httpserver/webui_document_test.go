package httpserver

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

type failingArchiveStore struct {
	ports.DocumentStore
	err error
}

func (s failingArchiveStore) SetArchived(context.Context, string, string, bool) error { return s.err }

type archiveEventCapture struct{ events []domain.Event }

func (e *archiveEventCapture) Emit(_ context.Context, ev domain.Event) {
	e.events = append(e.events, ev)
}

func TestWebWissenDocumentView(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	_, _ = docs.Create(ctx, domain.Document{
		ID:      "target",
		OwnerID: "u1",
		Type:    domain.DocFree,
		Path:    "target/path",
		Title:   "Target Doc",
		Body: strings.Join([]string{
			"> [!NOTE]",
			"> important",
			"",
			"| A | B |",
			"|---|---|",
			"| 1 | 2 |",
			"",
			"```go",
			"package main",
			"```",
			"",
			"See [[source/path]].",
		}, "\n"),
		Tags:      []string{"alpha"},
		CreatedAt: now,
		UpdatedAt: now,
	})
	_, _ = docs.Create(ctx, domain.Document{
		ID:        "source",
		OwnerID:   "u1",
		Type:      domain.DocFree,
		Path:      "source/path",
		Title:     "Source Link",
		Body:      "Links [[target/path]].",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err := docs.ReplaceLinks(ctx, "source", "u1", []string{"target/path"}); err != nil {
		t.Fatal(err)
	}

	body, status := getWissenDocumentAs(t, srv, codec, "u1", "/wissen/target")
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/target status=%d body=%.400s", status, body)
	}
	for _, want := range []string{
		"<table",
		"callout-",
		`class="chroma"`,
		`class="spine"`,
		`class="prov"`,
		`href="/wissen/target/bearbeiten"`,
		`hx-post="/wissen/target/pin"`,
		"Source Link", // inline wikilink resolution inside the prose body
		`href="/wissen/source"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("GET /wissen/target missing %q in %.1200s", want, body)
		}
	}
	// Delete remains on the edit page. Archive legitimately adds the shared
	// ConfirmDialog, so scope old Lesesaal chrome checks to the content before
	// that component.
	fragStart := strings.Index(body, `id="document-fragment"`)
	articleEnd := strings.Index(body, `</article>`)
	if fragStart < 0 || articleEnd < 0 || articleEnd < fragStart {
		t.Fatalf("could not locate document-fragment markers in body: %.1200s", body)
	}
	fragment := body[fragStart:articleEnd]
	if dialog := strings.Index(fragment, "<dialog"); dialog >= 0 {
		fragment = fragment[:dialog]
	}
	for _, gone := range []string{"glass", "bg-surface", "shadow-soft", "font-display", "data-dialog-open=\"del-"} {
		if strings.Contains(fragment, gone) {
			t.Errorf("Document fragment should not carry Kristall/delete remnant %q, got fragment:\n%s", gone, fragment)
		}
	}
	// Edit/pin actions (structure, hrefs, hx-attrs) must still be present.
	if !strings.Contains(body, `id="document-fragment"`) {
		t.Errorf("expected DocumentFragment structure intact, got:\n%s", body)
	}
	if strings.Contains(body, `hx-post="/wissen/target/delete"`) {
		t.Errorf("delete must no longer be reachable from the document view page, got:\n%s", body)
	}
}

// TestWebWissenDocumentView_OwnerScoped covers the Task 5 owner-scope
// negative test explicitly required by the brief: a second tenant (u2) must
// never be able to load u1's document via the web document-view route.
func TestWebWissenDocumentView_OwnerScoped(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{
		ID: "secret", OwnerID: "u1", Type: domain.DocFree, Path: "p/secret",
		Title: "Secret", Body: "shh", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	body, status := getWissenDocumentAs(t, srv, codec, "u2", "/wissen/secret")
	if status != http.StatusNotFound {
		t.Fatalf("u2 GET /wissen/secret status=%d, want 404 (owner-scoped), body=%.400s", status, body)
	}
}

// TestWebDocumentView_SanitizerBoundaryEndToEnd is the Task 9 end-to-end
// smoke for the sanitizer boundary already unit-tested at the RenderDocument
// level in Task 2 (markdown_test.go: TestRenderDocument_XSSStripped,
// TestRenderDocument_RawHTMLNeutralized): agent-authored Markdown with
// <script>, hx-get, onclick and a javascript: href must come out neutralized
// through the REAL /wissen/{id} HTTP route — the full Server.Routes() chain,
// including the securityHeaders middleware — not just through the
// RenderDocument unit under test.
func TestWebDocumentView_SanitizerBoundaryEndToEnd(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	malicious := strings.Join([]string{
		"<script>alert(1)</script>",
		"",
		`<div hx-get="/evil" onclick="alert(1)">poke</div>`,
		"",
		"[click me](javascript:alert(1))",
	}, "\n")
	if _, err := docs.Create(ctx, domain.Document{
		ID: "evil", OwnerID: "u1", Type: domain.DocFree, Path: "p/evil",
		Title: "Evil", Body: malicious, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookieVal, err := codec.Issue("u1")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/wissen/evil", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookieVal})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /wissen/evil status=%d", res.StatusCode)
	}
	if res.Header.Get("Content-Security-Policy-Report-Only") == "" {
		t.Fatal("expected the securityHeaders middleware on the real document route too")
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	// Scope the check to the rendered document content, not the whole page —
	// base.templ legitimately carries <script src="/static/vendor/...">
	// tags and the nonce'd theme-init/live-timer inline scripts outside the
	// article; only the agent-authored fragment must be free of the
	// malicious markers.
	fragStart := strings.Index(body, `id="document-fragment"`)
	articleEnd := strings.Index(body, `</article>`)
	if fragStart < 0 || articleEnd < 0 || articleEnd < fragStart {
		t.Fatalf("could not locate document-fragment markers in body: %.1200s", body)
	}
	fragment := body[fragStart:articleEnd]
	// The archive ConfirmDialog legitimately mounts the shared dialog script;
	// the sanitizer boundary applies to the rendered agent prose before that
	// trusted component, not to application-owned chrome.
	if dialog := strings.Index(fragment, "<dialog"); dialog >= 0 {
		fragment = fragment[:dialog]
	}
	for _, want := range []string{"<script", "hx-get", "onclick", "javascript:"} {
		if strings.Contains(fragment, want) {
			t.Fatalf("agent markdown not neutralized end-to-end, found %q in fragment:\n%s", want, fragment)
		}
	}
}

// TestWebDocumentView_ContextRankShownForMemoryDoc is Task 6's Step 5
// coverage: a memory doc that sits within a node's composed chain shows
// "enthalten ✓" plus the "Rang N/M" krow, and the Anpinnen button (Bestand,
// L3) is untouched.
func TestWebDocumentView_ContextRankShownForMemoryDoc(t *testing.T) {
	srv, codec, docs, projects := newWebWissenServer(t)
	srv.ComposeContext = usecase.ComposeContext{Nodes: projects, Docs: docs, Tags: testutil.NewFakeTagStore()}
	srv.ContextBudget = 12000
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	if _, err := projects.Create(ctx, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	nodeID := "n1"
	if _, err := docs.Create(ctx, domain.Document{
		ID: "mem-1", OwnerID: "u1", NodeID: &nodeID, Type: domain.DocMemory,
		Path: "mem-1", Title: "Tailwind v4 gotchas", Body: "some memory body",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed memory doc: %v", err)
	}

	body, status := getWissenDocumentAs(t, srv, codec, "u1", "/wissen/mem-1")
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/mem-1 status=%d body=%.400s", status, body)
	}
	for _, want := range []string{
		"Im Agenten-Kontext",
		"flow · enthalten",
		"Rang",
		"01 / 01",
		`hx-post="/wissen/mem-1/pin"`, // Anpinnen bleibt Bestand, unangetastet
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /wissen/mem-1 missing %q in %.1200s", want, body)
		}
	}
}

// TestWebDocumentView_ContextRankAbsentForNonContextType covers the "kein
// Kontext-Typ" state: a non-context-type doc (free) never shows the "Im
// Agenten-Kontext" block, and Compose is never even consulted for it.
func TestWebDocumentView_ContextRankAbsentForNonContextType(t *testing.T) {
	srv, codec, docs, projects := newWebWissenServer(t)
	srv.ComposeContext = usecase.ComposeContext{Nodes: projects, Docs: docs, Tags: testutil.NewFakeTagStore()}
	srv.ContextBudget = 12000
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	if _, err := docs.Create(ctx, domain.Document{
		ID: "free-1", OwnerID: "u1", Type: domain.DocFree,
		Path: "free-1", Title: "Free Doc", Body: "body",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed free doc: %v", err)
	}

	body, status := getWissenDocumentAs(t, srv, codec, "u1", "/wissen/free-1")
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/free-1 status=%d body=%.400s", status, body)
	}
	if strings.Contains(body, "Im Agenten-Kontext") {
		t.Errorf("non-context-type doc must never show the context block: %.800s", body)
	}
}

// TestWebDocumentView_ContextRankOwnerScoped is the owner-scope negative test
// (Global Constraints): a second tenant's own view of their own memory doc
// must compose against their OWN chain, never leaking u1's rank/total.
func TestWebDocumentView_ContextRankOwnerScoped(t *testing.T) {
	srv, codec, docs, projects := newWebWissenServer(t)
	srv.ComposeContext = usecase.ComposeContext{Nodes: projects, Docs: docs, Tags: testutil.NewFakeTagStore()}
	srv.ContextBudget = 12000
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	if _, err := projects.Create(ctx, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	nodeID := "n1"
	if _, err := docs.Create(ctx, domain.Document{
		ID: "mem-1", OwnerID: "u1", NodeID: &nodeID, Type: domain.DocMemory,
		Path: "mem-1", Title: "u1 memory", Body: "body",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed memory doc: %v", err)
	}
	// A foreign owner's memory doc in the same node must never inflate u1's
	// Rang total (Compose is owner-scoped end to end).
	if _, err := projects.Create(ctx, domain.Node{ID: "n2", OwnerID: "u2", Name: "other", Kind: domain.KindRepo}); err != nil {
		t.Fatalf("seed foreign node: %v", err)
	}
	nodeID2 := "n2"
	if _, err := docs.Create(ctx, domain.Document{
		ID: "mem-foreign", OwnerID: "u2", NodeID: &nodeID2, Type: domain.DocMemory,
		Path: "mem-foreign", Title: "foreign memory", Body: "body",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed foreign doc: %v", err)
	}

	body, status := getWissenDocumentAs(t, srv, codec, "u1", "/wissen/mem-1")
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/mem-1 status=%d body=%.400s", status, body)
	}
	if !strings.Contains(body, "01 / 01") {
		t.Errorf("owner-scoped compose must show total 1 (foreign doc excluded), got: %.800s", body)
	}
}

func TestWebDocumentArchiveAndRestoreRoundTrip(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	if _, err := docs.Create(ctx, domain.Document{
		ID: "doc-archive", OwnerID: "u1", Type: domain.DocMemory,
		Path: "memory/archive-me", Title: "Archive Me", Body: "body",
		CreatedAt: now, UpdatedAt: now, Pinned: true,
	}); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /wissen/{id}", srv.webAuth(http.HandlerFunc(srv.handleWebDocumentView)))
	mux.Handle("POST /wissen/{id}/archive", srv.webAuth(http.HandlerFunc(srv.handleWebDocArchive)))

	body, status := getWissenDocument(t, srv, codec, "/wissen/doc-archive")
	if status != http.StatusOK {
		t.Fatalf("GET active document status=%d", status)
	}
	for _, want := range []string{`data-dialog-open="archive-doc-archive"`, `hx-post="/wissen/doc-archive/archive"`} {
		if !strings.Contains(body, want) {
			t.Errorf("active document missing archive affordance %q in %.1600s", want, body)
		}
	}

	body, status = postWissenArchive(t, mux, codec, "u1", "doc-archive", true)
	if status != http.StatusOK {
		t.Fatalf("POST archive status=%d body=%.500s", status, body)
	}
	archived, err := docs.Get(ctx, "u1", "doc-archive")
	if err != nil {
		t.Fatal(err)
	}
	if !archived.Archived || archived.Pinned {
		t.Fatalf("archive state = %+v, want archived and unpinned", archived)
	}
	for _, want := range []string{`data-document-archived="true"`, "Archiviertes Dokument", "Wiederherstellen", `value="false"`} {
		if !strings.Contains(body, want) {
			t.Errorf("archive response missing %q in %.1800s", want, body)
		}
	}
	for _, gone := range []string{"Bearbeiten", "Anpinnen"} {
		if strings.Contains(body, gone) {
			t.Errorf("archived document must not offer %q in %.1800s", gone, body)
		}
	}

	body, status = postWissenArchive(t, mux, codec, "u1", "doc-archive", false)
	if status != http.StatusOK {
		t.Fatalf("POST restore status=%d body=%.500s", status, body)
	}
	restored, err := docs.Get(ctx, "u1", "doc-archive")
	if err != nil {
		t.Fatal(err)
	}
	if restored.Archived || restored.ArchivedAt != nil {
		t.Fatalf("restore state = %+v, want active", restored)
	}
	if !strings.Contains(body, `data-dialog-open="archive-doc-archive"`) {
		t.Fatalf("restore response did not return active actions: %.1800s", body)
	}
}

func TestWebDocumentArchiveOwnerScoped(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	if _, err := docs.Create(context.Background(), domain.Document{
		ID: "theirs", OwnerID: "u2", Type: domain.DocMemory,
		Path: "memory/theirs", Title: "Theirs", Body: "body", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("POST /wissen/{id}/archive", srv.webAuth(http.HandlerFunc(srv.handleWebDocArchive)))
	_, status := postWissenArchive(t, mux, codec, "u1", "theirs", true)
	if status != http.StatusNotFound {
		t.Fatalf("foreign archive status=%d, want 404", status)
	}
	got, err := docs.Get(context.Background(), "u2", "theirs")
	if err != nil || got.Archived {
		t.Fatalf("foreign document mutated: doc=%+v err=%v", got, err)
	}
}

func TestWebDocumentArchiveStoreFailureLeavesStateAndEmitsNothing(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	if _, err := docs.Create(context.Background(), domain.Document{
		ID: "doc-fail", OwnerID: "u1", Type: domain.DocMemory,
		Path: "memory/fail", Title: "Fail", Body: "body", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	capture := &archiveEventCapture{}
	srv.Emitter = capture
	srv.SetArchived = usecase.SetArchived{Docs: failingArchiveStore{DocumentStore: docs, err: errors.New("write failed")}}
	mux := http.NewServeMux()
	mux.Handle("POST /wissen/{id}/archive", srv.webAuth(http.HandlerFunc(srv.handleWebDocArchive)))

	_, status := postWissenArchive(t, mux, codec, "u1", "doc-fail", true)
	if status != http.StatusInternalServerError {
		t.Fatalf("archive store failure status=%d, want 500", status)
	}
	got, err := docs.Get(context.Background(), "u1", "doc-fail")
	if err != nil || got.Archived {
		t.Fatalf("failed archive mutated document: doc=%+v err=%v", got, err)
	}
	if len(capture.events) != 0 {
		t.Fatalf("failed archive emitted events: %+v", capture.events)
	}
}

func postWissenArchive(t *testing.T, h http.Handler, codec SessionCodec, userID, id string, archived bool) (string, int) {
	t.Helper()
	cookieVal, err := codec.Issue(userID)
	if err != nil {
		t.Fatal(err)
	}
	value := "false"
	if archived {
		value = "true"
	}
	req := httptest.NewRequest(http.MethodPost, "/wissen/"+id+"/archive", strings.NewReader("archived="+value))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookieVal})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Body.String(), rec.Code
}

func getWissenDocument(t *testing.T, s *Server, codec SessionCodec, target string) (string, int) {
	t.Helper()
	return getWissenDocumentAs(t, s, codec, "u1", target)
}

func getWissenDocumentAs(t *testing.T, s *Server, codec SessionCodec, userID, target string) (string, int) {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET /wissen/{id}", s.webAuth(http.HandlerFunc(s.handleWebDocumentView)))
	cookieVal, err := codec.Issue(userID)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookieVal})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Body.String(), rec.Code
}
