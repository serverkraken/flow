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
func (noopActivityStore) DistinctActors(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func newWebWissenServer(t *testing.T) (*Server, *websession.Codec, *testutil.FakeDocumentStore, *testutil.FakeNodeStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)}
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	// u2 exists purely so owner-scope negative tests (a second tenant that
	// must never see u1's documents) can issue a session for it via the
	// already-returned codec without needing the users store exposed too.
	u2, _ := domain.NewUser("u2", "sub-2", "other", "o@x", "Other")
	_, _ = users.UpsertBySub(context.Background(), u2)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	projects := testutil.NewFakeNodeStore()
	artifacts := testutil.NewFakeArtifactStore()
	bus := sse.NewBus()

	srv := &Server{
		Ensure:  usecase.EnsureUser{Users: users, IDs: &testutil.FakeIDGen{}, Allow: func(ports.Identity) bool { return true }},
		Bus:     bus,
		Emitter: sse.NewEmitter(bus, noopActivityStore{}, &testutil.FakeIDGen{}, clk),
		Clock:   clk,
		Users:   users,
		Session: codec,

		ListDocuments:     usecase.ListDocuments{Docs: docs},
		ListDocumentsPage: usecase.NewListDocumentsPage(docs),
		ListNodes:         usecase.ListNodes{Nodes: projects},
		CreateDocument:    usecase.CreateDocument{Docs: docs, Tags: tags, IDs: &testutil.FakeIDGen{}, Clock: clk},
		GetDocument:       usecase.GetDocument{Docs: docs},
		UpdateDocument:    usecase.UpdateDocument{Docs: docs, Tags: tags, Clock: clk},
		MoveDocument:      usecase.MoveDocument{Docs: docs, Nodes: projects, Clock: clk},
		DeleteDocument:    usecase.DeleteDocument{Docs: docs},
		BacklinksDocument: usecase.Backlinks{Docs: docs},
		ListTags:          usecase.ListTags{Tags: tags},
		SearchDocuments:   usecase.SearchDocuments{Docs: docs},
		GetEmbedStatus:    usecase.GetEmbedStatus{Docs: docs},
		RetryEmbedding:    usecase.RetryEmbedding{Docs: docs},
		NodeAncestors:     usecase.NodeAncestors{Nodes: projects},
		SetPinned:         usecase.SetPinned{Docs: docs},
		SetContextMode:    usecase.SetContextMode{Docs: docs},
		ListArtifacts:     usecase.ListArtifacts{Nodes: projects, Artifacts: artifacts},
	}
	return srv, codec, docs, projects
}

func TestWebWissenHomeShelvesAndRecent(t *testing.T) {
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
	for _, want := range []string{
		`href="/wissen/typ?type=project"`, `href="/wissen/typ?type=plan"`, `href="/wissen/typ?type=spec"`,
		`href="/wissen/typ?type=memory"`, `href="/wissen/typ?type=daily"`, `href="/wissen/typ?type=context"`, `href="/wissen/typ?type=free"`,
		"Daily Note", "Project Note", "Free Note", "System Memory",
		"4 Dokumente", // summary "%d Dokumente · %d angepinnt"
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("GET /wissen missing %q in %.1500s", want, body)
		}
	}
	// Old category sections are gone.
	for _, notWant := range []string{"daily-sec", "notes-sec", "free-sec", "system-sec"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("overview should not render old category section %q", notWant)
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

func TestWebWissenTypeRoutesFilterDocuments(t *testing.T) {
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
	body, status := getWissen(t, wissenTestMux(srv), "/wissen/typ?type=daily", codec)
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/typ?type=daily status=%d body=%.300s", status, body)
	}
	if !strings.Contains(body, "Daily One") {
		t.Fatalf("daily shelf page missing daily doc: %.1000s", body)
	}
	for _, notWant := range []string{"Free One", "Memory One"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("daily shelf page leaked %q in %.1000s", notWant, body)
		}
	}
}

func TestWebWissenTypeContextIsThreeTypeSet(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	for _, doc := range []domain.Document{
		{ID: "ac-1", OwnerID: "u1", Type: domain.DocActiveContext, Path: "activecontext/x", Title: "Active Context Doc", Body: "b", CreatedAt: now, UpdatedAt: now},
		{ID: "in-1", OwnerID: "u1", Type: domain.DocInstruction, Path: "instr/x", Title: "Instruction Doc", Body: "b", CreatedAt: now, UpdatedAt: now},
		{ID: "sk-1", OwnerID: "u1", Type: domain.DocSkill, Path: "skill/x", Title: "Skill Doc", Body: "b", CreatedAt: now, UpdatedAt: now},
		{ID: "fr-1", OwnerID: "u1", Type: domain.DocFree, Path: "free/x", Title: "Free Doc", Body: "b", CreatedAt: now, UpdatedAt: now},
	} {
		_, _ = docs.Create(context.Background(), doc)
	}
	body, status := getWissen(t, wissenTestMux(srv), "/wissen/typ?type=context", codec)
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/typ?type=context status=%d body=%.300s", status, body)
	}
	for _, want := range []string{"Active Context Doc", "Instruction Doc", "Skill Doc"} {
		if !strings.Contains(body, want) {
			t.Fatalf("context shelf missing %q: %.1000s", want, body)
		}
	}
	if strings.Contains(body, "Free Doc") {
		t.Fatalf("context shelf leaked Free Doc: %.1000s", body)
	}
}

func TestWebWissenTypeUnknownIs404(t *testing.T) {
	srv, codec, _, _ := newWebWissenServer(t)
	_, status := getWissen(t, wissenTestMux(srv), "/wissen/typ?type=bogus", codec)
	if status != http.StatusNotFound {
		t.Fatalf("GET /wissen/typ?type=bogus status=%d, want 404", status)
	}
}

func TestWebWissenTypeSearchIsShelfScoped(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	_, _ = docs.Create(context.Background(), domain.Document{ID: "daily-1", OwnerID: "u1", Type: domain.DocDaily, Path: "daily/2026-06-25", Title: "Daily Alpha", Body: "alpha", Date: &now, CreatedAt: now, UpdatedAt: now})
	_, _ = docs.Create(context.Background(), domain.Document{ID: "free-1", OwnerID: "u1", Type: domain.DocFree, Path: "free/alpha", Title: "Free Alpha", Body: "alpha", CreatedAt: now, UpdatedAt: now})
	body, status := getWissen(t, wissenTestMux(srv), "/wissen/typ?type=free&q=alpha", codec)
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/typ?type=free&q=alpha status=%d body=%.300s", status, body)
	}
	if !strings.Contains(body, "Free Alpha") || strings.Contains(body, "Daily Alpha") {
		t.Fatalf("free shelf search not scoped: %.1000s", body)
	}
}

// TestWebWissenOldSlugsRedirect pins the Task 7 cutover: retired category
// slugs redirect to their type-shelf successor, and /wissen/system (no 1:1
// successor — its five legacy types now spread across plan/memory/context/
// spec) redirects to the overview instead (Offene Entsch. #7 / Codex #17).
func TestWebWissenOldSlugsRedirect(t *testing.T) {
	srv, codec, _, _ := newWebWissenServer(t)
	h := srv.Routes()
	cases := map[string]string{
		"/wissen/daily":    "/wissen/typ?type=daily",
		"/wissen/projekte": "/wissen/typ?type=project",
		"/wissen/frei":     "/wissen/typ?type=free",
		"/wissen/system":   "/wissen",
	}
	for from, want := range cases {
		loc, status := getWissenRedirect(t, h, from, codec)
		if status != http.StatusFound {
			t.Errorf("GET %s status=%d, want 302", from, status)
			continue
		}
		if loc != want {
			t.Errorf("GET %s redirected to %q, want %q", from, loc, want)
		}
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

	// handleWebWissenTypeList — daily shelf fragment
	body2, status2 := getWissen(t, wissenTestMux(srv), "/ui/wissen/list/typ?type=daily", codec)
	if status2 != http.StatusOK {
		t.Fatalf("GET /ui/wissen/list/typ?type=daily status=%d body=%.300s", status2, body2)
	}
	if !strings.Contains(body2, "Daily Frag") {
		t.Fatalf("wissen daily fragment missing doc; body=%.500s", body2)
	}

	// invalid type → 404 from the handler (unknown shelf key)
	_, status3 := getWissen(t, wissenTestMux(srv), "/ui/wissen/list/typ?type=bogus", codec)
	if status3 != http.StatusNotFound {
		t.Fatalf("GET /ui/wissen/list/typ?type=bogus status=%d, want 404", status3)
	}
}

// TestWebWissenRecentAllExpandsCap pins the "Alle N ›" expand-in-place
// affordance: with more than the cap of recently-updated documents,
// ?recent=all must render every one of them instead of just the cap.
func TestWebWissenRecentAllExpandsCap(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		id := "d" + string(rune('a'+i))
		_, _ = docs.Create(context.Background(), domain.Document{
			ID: id, OwnerID: "u1", Type: domain.DocFree, Path: "free/" + id,
			Title: "Recent " + id, Body: "b", CreatedAt: now, UpdatedAt: now.Add(-time.Duration(i) * time.Minute),
		})
	}
	capped, status := getWissen(t, wissenTestMux(srv), "/wissen", codec)
	if status != http.StatusOK {
		t.Fatalf("GET /wissen status=%d", status)
	}
	if !strings.Contains(capped, "Alle 10") {
		t.Fatalf("expected 'Alle 10' expand link, got %.1500s", capped)
	}

	all, status := getWissen(t, wissenTestMux(srv), "/wissen?recent=all", codec)
	if status != http.StatusOK {
		t.Fatalf("GET /wissen?recent=all status=%d", status)
	}
	for i := 0; i < 10; i++ {
		id := "d" + string(rune('a'+i))
		if !strings.Contains(all, "Recent "+id) {
			t.Errorf("recent=all missing %q: %.2000s", "Recent "+id, all)
		}
	}
}

// TestWebWissenOwnerScope_Overview verifies Regale + Recent on /wissen never
// leak another owner's documents (Global Constraint owner-scope negative
// test, surface a).
func TestWebWissenOwnerScope_Overview(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	_, _ = docs.Create(context.Background(), domain.Document{ID: "mine", OwnerID: "u1", Type: domain.DocFree, Path: "free/mine", Title: "Mine Doc", Body: "b", CreatedAt: now, UpdatedAt: now})
	_, _ = docs.Create(context.Background(), domain.Document{ID: "theirs", OwnerID: "u2", Type: domain.DocFree, Path: "free/theirs", Title: "Their Doc", Body: "b", CreatedAt: now, UpdatedAt: now})

	body, status := getWissenAs(t, wissenTestMux(srv), "/wissen", codec, "u1")
	if status != http.StatusOK {
		t.Fatalf("GET /wissen (u1) status=%d", status)
	}
	if !strings.Contains(body, "Mine Doc") {
		t.Fatalf("u1 should see own doc: %.1000s", body)
	}
	if strings.Contains(body, "Their Doc") {
		t.Fatalf("u1 must not see u2's doc: %.1000s", body)
	}
	if !strings.Contains(body, "1 Dokumente") {
		t.Fatalf("u1 summary should count only own doc: %.1000s", body)
	}
}

// TestWebWissenOwnerScope_Search verifies /wissen?q= (surface b,
// s.SearchDocuments) never returns another owner's documents.
func TestWebWissenOwnerScope_Search(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	_, _ = docs.Create(context.Background(), domain.Document{ID: "mine", OwnerID: "u1", Type: domain.DocFree, Path: "free/mine", Title: "Mine Doc", Body: "alpha needle", CreatedAt: now, UpdatedAt: now})
	_, _ = docs.Create(context.Background(), domain.Document{ID: "theirs", OwnerID: "u2", Type: domain.DocFree, Path: "free/theirs", Title: "Their Doc", Body: "alpha needle", CreatedAt: now, UpdatedAt: now})

	body, status := getWissenAs(t, wissenTestMux(srv), "/wissen?q=alpha", codec, "u1")
	if status != http.StatusOK {
		t.Fatalf("GET /wissen?q=alpha (u1) status=%d", status)
	}
	if !strings.Contains(body, "Mine Doc") {
		t.Fatalf("u1 search should find own doc: %.1000s", body)
	}
	if strings.Contains(body, "Their Doc") {
		t.Fatalf("u1 search must not leak u2's doc: %.1000s", body)
	}
}

// TestWebWissenOwnerScope_TypeShelf verifies /wissen/typ?type= (surface c)
// never returns another owner's documents.
func TestWebWissenOwnerScope_TypeShelf(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	_, _ = docs.Create(context.Background(), domain.Document{ID: "mine", OwnerID: "u1", Type: domain.DocFree, Path: "free/mine", Title: "Mine Doc", Body: "b", CreatedAt: now, UpdatedAt: now})
	_, _ = docs.Create(context.Background(), domain.Document{ID: "theirs", OwnerID: "u2", Type: domain.DocFree, Path: "free/theirs", Title: "Their Doc", Body: "b", CreatedAt: now, UpdatedAt: now})

	body, status := getWissenAs(t, wissenTestMux(srv), "/wissen/typ?type=free", codec, "u1")
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/typ?type=free (u1) status=%d", status)
	}
	if !strings.Contains(body, "Mine Doc") {
		t.Fatalf("u1 shelf listing should show own doc: %.1000s", body)
	}
	if strings.Contains(body, "Their Doc") {
		t.Fatalf("u1 shelf listing must not leak u2's doc: %.1000s", body)
	}
}

func wissenTestMux(s *Server) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /wissen", s.webAuth(http.HandlerFunc(s.handleWebWissenHome)))
	mux.Handle("GET /ui/wissen/list", s.webAuth(http.HandlerFunc(s.handleWebWissenList)))
	mux.Handle("GET /wissen/typ", s.webAuth(http.HandlerFunc(s.handleWebWissenType)))
	mux.Handle("GET /ui/wissen/list/typ", s.webAuth(http.HandlerFunc(s.handleWebWissenTypeList)))
	return mux
}

func getWissen(t *testing.T, h http.Handler, url string, codec *websession.Codec) (string, int) {
	t.Helper()
	return getWissenAs(t, h, url, codec, "u1")
}

func getWissenAs(t *testing.T, h http.Handler, url string, codec *websession.Codec, userID string) (string, int) {
	t.Helper()
	cookieVal, err := codec.Issue(userID)
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

// getWissenRedirect issues the request but does not follow the redirect,
// returning the Location header instead of the (empty) body.
func getWissenRedirect(t *testing.T, h http.Handler, url string, codec *websession.Codec) (string, int) {
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
	return rec.Header().Get("Location"), rec.Code
}
