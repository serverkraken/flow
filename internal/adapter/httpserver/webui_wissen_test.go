package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// noopActivityStore satisfies ports.ActivityStore for tests that don't assert on activity.
type noopActivityStore struct{}

func (noopActivityStore) Append(_ context.Context, _ domain.ActivityEntry) error { return nil }
func (noopActivityStore) ListPage(_ context.Context, _ string, _ []string, _ *string, _, _ int) ([]domain.ActivityEntry, int, error) {
	return nil, 0, nil
}

func newWebWissenServer(t *testing.T) (*Server, *websession.Codec, *testutil.FakeDocumentStore, *testutil.FakeNodeStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)}
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	projects := testutil.NewFakeNodeStore()
	bus := sse.NewBus()

	srv := &Server{
		Ensure:   usecase.EnsureUser{Users: users, IDs: &testutil.FakeIDGen{}, Allow: func(ports.Identity) bool { return true }},
		Bus:      bus,
		Emitter:  sse.NewEmitter(bus, noopActivityStore{}, &testutil.FakeIDGen{}, clk),
		Clock:    clk,
		Users:    users,
		Session:  codec,

		ListDocuments:     usecase.ListDocuments{Docs: docs},
		ListDocumentsPage: usecase.NewListDocumentsPage(docs),
		ListNodes:      usecase.ListNodes{Nodes: projects},
		CreateDocument:    usecase.CreateDocument{Docs: docs, Tags: tags, IDs: &testutil.FakeIDGen{}, Clock: clk},
		GetDocument:       usecase.GetDocument{Docs: docs},
		UpdateDocument:    usecase.UpdateDocument{Docs: docs, Tags: tags, Clock: clk},
		DeleteDocument:    usecase.DeleteDocument{Docs: docs},
		BacklinksDocument: usecase.Backlinks{Docs: docs},
		ListTags:          usecase.ListTags{Tags: tags},
		SearchDocuments:   usecase.SearchDocuments{Docs: docs},
		GetEmbedStatus:    usecase.GetEmbedStatus{Docs: docs},
		RetryEmbedding:    usecase.RetryEmbedding{Docs: docs},
	}
	return srv, codec, docs, projects
}

func TestWebWissenHomeSections(t *testing.T) {
	srv, codec, docs, projects := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	pid := "p1"
	_, _ = projects.Create(ctx, domain.Node{ID: pid, OwnerID: "u1", Name: "Alpha", Slug: "alpha", Color: "blue", Status: domain.NodeActive})
	for _, doc := range []domain.Document{
		{ID: "daily-1", OwnerID: "u1", Type: domain.DocDaily, Path: "daily/2026-06-15", Title: "Daily Note", Body: "morning", Tags: []string{"log"}, CreatedAt: now, UpdatedAt: now},
		{ID: "project-1", OwnerID: "u1", Type: domain.DocProject, NodeID: &pid, Path: "alpha/note", Title: "Project Note", Body: "alpha needle", Tags: []string{"alpha"}, CreatedAt: now, UpdatedAt: now.Add(-time.Minute)},
		{ID: "free-1", OwnerID: "u1", Type: domain.DocFree, Path: "free/idea", Title: "Free Note", Body: "loose", Tags: []string{"idea"}, CreatedAt: now, UpdatedAt: now.Add(-2 * time.Minute)},
		{ID: "memory-1", OwnerID: "u1", Type: domain.DocMemory, Path: "memory/system", Title: "System Memory", Body: "system", Tags: []string{"ops"}, CreatedAt: now, UpdatedAt: now.Add(-3 * time.Minute)},
	} {
		_, _ = docs.Create(ctx, doc)
	}

	body, status := getWissen(t, wissenTestMux(srv), "/wissen", codec)
	if status != http.StatusOK {
		t.Fatalf("GET /wissen status=%d body=%.300s", status, body)
	}
	for _, want := range []string{`href="/wissen/daily"`, `href="/wissen/projekte"`, `href="/wissen/frei"`, `href="/wissen/system"`, "Daily Note", "Project Note", "Free Note", "System Memory"} {
		if !strings.Contains(body, want) {
			t.Fatalf("GET /wissen missing %q in %.800s", want, body)
		}
	}
}

func TestWebWissenHomeOverviewCards(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	for _, doc := range []domain.Document{
		{ID: "daily-1", OwnerID: "u1", Type: domain.DocDaily, Path: "daily/2026-06-25", Title: "Daily One", Body: "daily body", Date: &now, CreatedAt: now, UpdatedAt: now},
		{ID: "free-1", OwnerID: "u1", Type: domain.DocFree, Path: "free/idea", Title: "Free One", Body: "free body", CreatedAt: now, UpdatedAt: now},
	} {
		_, _ = docs.Create(context.Background(), doc)
	}
	body, status := getWissen(t, wissenTestMux(srv), "/wissen", codec)
	if status != http.StatusOK {
		t.Fatalf("GET /wissen status=%d body=%.300s", status, body)
	}
	for _, want := range []string{`href="/wissen/daily"`, `href="/wissen/projekte"`, `href="/wissen/frei"`, `href="/wissen/system"`, "Daily One", "Free One"} {
		if !strings.Contains(body, want) {
			t.Fatalf("overview missing %q in %.1200s", want, body)
		}
	}
	for _, notWant := range []string{"daily-sec", "notes-sec", "free-sec", "system-sec"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("overview should not render old long section %q", notWant)
		}
	}
}

func TestWebWissenSearch(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	_, _ = docs.Create(context.Background(), domain.Document{
		ID: "search-1", OwnerID: "u1", Type: domain.DocFree, Path: "search/hit",
		Title: "Search Hit", Body: "alpha needle", CreatedAt: now, UpdatedAt: now,
	})

	body, status := getWissen(t, wissenTestMux(srv), "/wissen?q=alpha", codec)
	if status != http.StatusOK {
		t.Fatalf("GET /wissen?q=alpha status=%d body=%.300s", status, body)
	}
	if !strings.Contains(body, "Search Hit") || !strings.Contains(body, "<mark>alpha</mark>") {
		t.Fatalf("expected search result and highlighted snippet, got %.800s", body)
	}
}

func TestWebWissenCategoryRoutesFilterDocuments(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	for _, doc := range []domain.Document{
		{ID: "daily-1", OwnerID: "u1", Type: domain.DocDaily, Path: "daily/2026-06-25", Title: "Daily One", Body: "daily preview\nline two", Date: &now, CreatedAt: now, UpdatedAt: now},
		{ID: "free-1", OwnerID: "u1", Type: domain.DocFree, Path: "free/idea", Title: "Free One", Body: "free preview", CreatedAt: now, UpdatedAt: now},
		{ID: "mem-1", OwnerID: "u1", Type: domain.DocMemory, Path: "memory/x", Title: "Memory One", Body: "memory preview", CreatedAt: now, UpdatedAt: now},
	} {
		_, _ = docs.Create(ctx, doc)
	}
	body, status := getWissen(t, wissenTestMux(srv), "/wissen/daily", codec)
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/daily status=%d body=%.300s", status, body)
	}
	if !strings.Contains(body, "Daily One") || !strings.Contains(body, "daily preview") {
		t.Fatalf("daily page missing daily doc/preview: %.1000s", body)
	}
	for _, notWant := range []string{"Free One", "Memory One"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("daily page leaked %q in %.1000s", notWant, body)
		}
	}
}

func TestWebWissenCategorySearchIsCategoryScoped(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	_, _ = docs.Create(context.Background(), domain.Document{ID: "daily-1", OwnerID: "u1", Type: domain.DocDaily, Path: "daily/2026-06-25", Title: "Daily Alpha", Body: "alpha", Date: &now, CreatedAt: now, UpdatedAt: now})
	_, _ = docs.Create(context.Background(), domain.Document{ID: "free-1", OwnerID: "u1", Type: domain.DocFree, Path: "free/alpha", Title: "Free Alpha", Body: "alpha", CreatedAt: now, UpdatedAt: now})
	body, status := getWissen(t, wissenTestMux(srv), "/wissen/frei?q=alpha", codec)
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/frei?q=alpha status=%d body=%.300s", status, body)
	}
	if !strings.Contains(body, "Free Alpha") || strings.Contains(body, "Daily Alpha") {
		t.Fatalf("free search not category scoped: %.1000s", body)
	}
}

func TestWissenRoutesCutover(t *testing.T) {
	srv, codec, _, _ := newWebWissenServer(t)
	h := srv.Routes()

	for _, path := range []string{"/wissen", "/wissen/neu"} {
		body, status := getWissen(t, h, path, codec)
		if status != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%.300s", path, status, body)
		}
	}
	_, status := getWissen(t, h, "/docs", codec)
	if status != http.StatusNotFound {
		t.Fatalf("GET /docs status=%d, want 404", status)
	}
}

func TestWebWissenListFragments(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	_, _ = docs.Create(context.Background(), domain.Document{
		ID: "daily-1", OwnerID: "u1", Type: domain.DocDaily, Path: "daily/2026-06-15",
		Title: "Daily Frag", Body: "body", Date: &now, CreatedAt: now, UpdatedAt: now,
	})

	// handleWebWissenList — overview fragment at /ui/wissen/list
	body, status := getWissen(t, wissenTestMux(srv), "/ui/wissen/list", codec)
	if status != http.StatusOK {
		t.Fatalf("GET /ui/wissen/list status=%d body=%.300s", status, body)
	}
	if !strings.Contains(body, "Daily Frag") {
		t.Fatalf("wissen list fragment missing doc; body=%.500s", body)
	}

	// handleWebWissenCategoryList — daily fragment
	body2, status2 := getWissen(t, wissenTestMux(srv), "/ui/wissen/list/daily", codec)
	if status2 != http.StatusOK {
		t.Fatalf("GET /ui/wissen/list/daily status=%d body=%.300s", status2, body2)
	}
	if !strings.Contains(body2, "Daily Frag") {
		t.Fatalf("wissen daily fragment missing doc; body=%.500s", body2)
	}

	// projekte fragment — exercises wissenCategoryProjectGroups (has color swatch branch)
	pid := "p1"
	_, _ = docs.Create(context.Background(), domain.Document{
		ID: "proj-1", OwnerID: "u1", Type: domain.DocProject, NodeID: &pid,
		Path: "alpha/note", Title: "Proj Note", Body: "notes body", CreatedAt: now, UpdatedAt: now,
	})
	body3, status3 := getWissen(t, wissenTestMux(srv), "/ui/wissen/list/projekte", codec)
	if status3 != http.StatusOK {
		t.Fatalf("GET /ui/wissen/list/projekte status=%d body=%.300s", status3, body3)
	}

	// invalid category → 404 from mux (route not registered)
	_, status4 := getWissen(t, wissenTestMux(srv), "/ui/wissen/list/bogus", codec)
	if status4 != http.StatusNotFound {
		t.Fatalf("GET /ui/wissen/list/bogus status=%d, want 404", status4)
	}
}

func wissenTestMux(s *Server) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /wissen", s.webAuth(http.HandlerFunc(s.handleWebWissenHome)))
	mux.Handle("GET /ui/wissen/list", s.webAuth(http.HandlerFunc(s.handleWebWissenList)))
	for _, slug := range []string{"daily", "projekte", "frei", "system"} {
		mux.Handle("GET /wissen/"+slug, s.webAuth(http.HandlerFunc(s.handleWebWissenCategory)))
		mux.Handle("GET /ui/wissen/list/"+slug, s.webAuth(http.HandlerFunc(s.handleWebWissenCategoryList)))
	}
	return mux
}

func getWissen(t *testing.T, h http.Handler, url string, codec *websession.Codec) (string, int) {
	t.Helper()
	cookieVal, err := codec.Issue("u1")
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookieVal})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Body.String(), rec.Code
}

// TestWissenProjectGroup_KindBadge verifies that a project-type document grouped
// under a repo node shows the node's kind badge in the group header.
// D7: doc-group headers must render @nodeKindBadge(group.Kind).
func TestWissenProjectGroup_KindBadge(t *testing.T) {
	srv, codec, docs, projects := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)

	// Seed an engagement root, then a repo child.
	// Node names deliberately do NOT contain the badge label text ("Repo")
	// so we can test the badge independently.
	eng, _ := projects.Create(ctx, domain.Node{
		ID: "eng1", OwnerID: "u1", Name: "MainEngagement", Slug: "main-eng",
		Kind: domain.KindEngagement, Status: domain.NodeActive,
	})
	codebase, _ := projects.Create(ctx, domain.Node{
		ID: "cb1", OwnerID: "u1", Name: "Codebase", Slug: "codebase",
		Kind: domain.KindRepo, ParentID: &eng.ID, Status: domain.NodeActive,
	})

	// Seed a project-type doc linked to the repo node.
	pid := codebase.ID
	_, _ = docs.Create(ctx, domain.Document{
		ID: "proj-doc-1", OwnerID: "u1", Type: domain.DocProject,
		NodeID: &pid, Path: "codebase/design", Title: "Design Doc",
		Body: "content", CreatedAt: now, UpdatedAt: now,
	})

	body, status := getWissen(t, wissenTestMux(srv), "/wissen/projekte", codec)
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/projekte status=%d body=%.300s", status, body)
	}

	// Group header must show the node name.
	if !strings.Contains(body, "Codebase") {
		t.Errorf("projekte group header missing repo node name; body=%.800s", body)
	}
	// Kind badge for repo must be present as a standalone badge element.
	// nodeKindBadge renders: <span class="inline-flex ... rounded-md border px-2 py-0.5 text-[.72rem] ...">
	// This CSS combination is unique to nodeKindBadge (doc-row glyphs use rounded-lg + h-7/h-8, not px-2 py-0.5).
	if !strings.Contains(body, "px-2 py-0.5") {
		t.Errorf("projekte group header missing node kind badge (px-2 py-0.5 marker); body=%.800s", body)
	}
}
