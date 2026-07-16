package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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

type recordingWissenEmitter struct{ events []domain.Event }

func (e *recordingWissenEmitter) Emit(_ context.Context, event domain.Event) {
	e.events = append(e.events, event)
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

		ListDocuments:         usecase.ListDocuments{Docs: docs},
		ListDocumentsPage:     usecase.NewListDocumentsPage(docs),
		ListDocumentLibrary:   usecase.ListDocumentLibrary{Docs: docs},
		SearchDocumentLibrary: usecase.SearchDocumentLibrary{Docs: docs},
		ListNodes:             usecase.ListNodes{Nodes: projects},
		CreateDocument:        usecase.CreateDocument{Docs: docs, Nodes: projects, Tags: tags, IDs: &testutil.FakeIDGen{}, Clock: clk},
		GetDocument:           usecase.GetDocument{Docs: docs},
		UpdateDocument:        usecase.UpdateDocument{Docs: docs, Tags: tags, Clock: clk},
		MoveDocument:          usecase.MoveDocument{Docs: docs, Nodes: projects, Clock: clk},
		DeleteDocument:        usecase.DeleteDocument{Docs: docs},
		BacklinksDocument:     usecase.Backlinks{Docs: docs},
		ListTags:              usecase.ListTags{Tags: tags},
		SearchDocuments:       usecase.SearchDocuments{Docs: docs},
		GetEmbedStatus:        usecase.GetEmbedStatus{Docs: docs},
		RetryEmbedding:        usecase.RetryEmbedding{Docs: docs},
		NodeAncestors:         usecase.NodeAncestors{Nodes: projects},
		SetPinned:             usecase.SetPinned{Docs: docs},
		SetArchived:           usecase.SetArchived{Docs: docs, Curation: docs, Clock: clk},
		BulkCurateDocuments:   usecase.BulkCurateDocuments{Docs: docs, Clock: clk},
		ListArchived:          usecase.ListArchived{Docs: docs},
		SetContextMode:        usecase.SetContextMode{Docs: docs, Curation: docs, Clock: clk},
		ListArtifacts:         usecase.ListArtifacts{Nodes: projects, Artifacts: artifacts},
	}
	return srv, codec, docs, projects
}

func TestWebWissenArchiveFilterIsVisibleAndOwnerScoped(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	for _, doc := range []domain.Document{
		{ID: "active-mine", OwnerID: "u1", Type: domain.DocMemory, Path: "memory/active", Title: "Active Mine", Body: "body", CreatedAt: now, UpdatedAt: now},
		{ID: "archived-mine", OwnerID: "u1", Type: domain.DocMemory, Path: "memory/archived", Title: "Archived Mine", Body: "body", CreatedAt: now, UpdatedAt: now},
		{ID: "archived-theirs", OwnerID: "u2", Type: domain.DocMemory, Path: "memory/theirs", Title: "Archived Theirs", Body: "body", CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := docs.Create(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	if err := docs.SetArchived(ctx, "u1", "archived-mine", true); err != nil {
		t.Fatal(err)
	}
	if err := docs.SetArchived(ctx, "u2", "archived-theirs", true); err != nil {
		t.Fatal(err)
	}

	body, status := getWissenAs(t, wissenTestMux(srv), "/wissen?status=archived", codec, "u1")
	if status != http.StatusOK {
		t.Fatalf("GET archived Wissen status=%d body=%.500s", status, body)
	}
	for _, want := range []string{
		`data-wissen-status="archived"`, "Archived Mine", "Archiviert", "1 aktiv", "1 archiviert",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("archived Wissen missing %q in %.1800s", want, body)
		}
	}
	for _, notWant := range []string{"Active Mine", "Archived Theirs"} {
		if strings.Contains(body, notWant) {
			t.Errorf("archived Wissen leaked %q in %.1800s", notWant, body)
		}
	}
}

func TestWebWissenArchiveFiltersByNodeAndTag(t *testing.T) {
	srv, codec, docs, nodes := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	for _, node := range []domain.Node{
		{ID: "n1", OwnerID: "u1", Name: "Flow", Slug: "flow", Kind: domain.KindRepo, Status: domain.NodeActive},
		{ID: "n2", OwnerID: "u1", Name: "Other", Slug: "other", Kind: domain.KindRepo, Status: domain.NodeActive},
	} {
		if _, err := nodes.Create(ctx, node); err != nil {
			t.Fatal(err)
		}
	}
	n1, n2 := "n1", "n2"
	for _, doc := range []domain.Document{
		{ID: "match", OwnerID: "u1", NodeID: &n1, Type: domain.DocMemory, Path: "memory/match", Title: "Flow Ops", Body: "body", Tags: []string{"ops"}, CreatedAt: now, UpdatedAt: now},
		{ID: "wrong-tag", OwnerID: "u1", NodeID: &n1, Type: domain.DocMemory, Path: "memory/design", Title: "Flow Design", Body: "body", Tags: []string{"design"}, CreatedAt: now, UpdatedAt: now},
		{ID: "wrong-node", OwnerID: "u1", NodeID: &n2, Type: domain.DocMemory, Path: "memory/other", Title: "Other Ops", Body: "body", Tags: []string{"ops"}, CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := docs.Create(ctx, doc); err != nil {
			t.Fatal(err)
		}
		if err := docs.SetArchived(ctx, "u1", doc.ID, true); err != nil {
			t.Fatal(err)
		}
	}

	body, status := getWissen(t, wissenTestMux(srv), "/wissen?status=archived&node=n1&tag=ops", codec)
	if status != http.StatusOK {
		t.Fatalf("GET filtered archive status=%d body=%.500s", status, body)
	}
	for _, want := range []string{"Flow Ops", `name="node"`, `value="n1" selected`} {
		if !strings.Contains(body, want) {
			t.Errorf("filtered archive missing %q in %.1800s", want, body)
		}
	}
	for _, notWant := range []string{"Flow Design", "Other Ops"} {
		if strings.Contains(body, notWant) {
			t.Errorf("filtered archive contains %q in %.1800s", notWant, body)
		}
	}
}

func TestWebWissenSubtreeScopeMatchesCockpitInventoryAndStaysOwnerScoped(t *testing.T) {
	srv, codec, docs, nodes := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	engID, vorID := "eng", "vor"
	for _, node := range []domain.Node{
		{ID: engID, OwnerID: "u1", Name: "Engagement", Slug: "engagement", Kind: domain.KindEngagement, Status: domain.NodeActive},
		{ID: vorID, OwnerID: "u1", ParentID: &engID, Name: "Vorhaben", Slug: "vorhaben", Kind: domain.KindVorhaben, Status: domain.NodeActive},
		{ID: "repo", OwnerID: "u1", ParentID: &vorID, Name: "Flow", Slug: "flow", Kind: domain.KindRepo, Status: domain.NodeActive},
		{ID: "other", OwnerID: "u1", Name: "Other", Slug: "other", Kind: domain.KindRepo, Status: domain.NodeActive},
	} {
		if _, err := nodes.Create(ctx, node); err != nil {
			t.Fatal(err)
		}
	}
	repoID, otherID := "repo", "other"
	for _, doc := range []domain.Document{
		{ID: "root-active", OwnerID: "u1", NodeID: &engID, Type: domain.DocMemory, Path: "memory/root", Title: "Root Active", Body: "body", CreatedAt: now, UpdatedAt: now},
		{ID: "middle-active", OwnerID: "u1", NodeID: &vorID, Type: domain.DocMemory, Path: "memory/middle", Title: "Middle Active", Body: "body", CreatedAt: now, UpdatedAt: now},
		{ID: "child-active", OwnerID: "u1", NodeID: &repoID, Type: domain.DocMemory, Path: "memory/child", Title: "Child Active", Body: "body", CreatedAt: now, UpdatedAt: now},
		{ID: "child-archived", OwnerID: "u1", NodeID: &repoID, Type: domain.DocMemory, Path: "memory/archive", Title: "Child Archived", Body: "body", CreatedAt: now, UpdatedAt: now},
		{ID: "outside", OwnerID: "u1", NodeID: &otherID, Type: domain.DocMemory, Path: "memory/outside", Title: "Outside", Body: "body", CreatedAt: now, UpdatedAt: now},
		{ID: "foreign-collision", OwnerID: "u2", NodeID: &repoID, Type: domain.DocMemory, Path: "memory/foreign", Title: "Foreign Collision", Body: "body", CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := docs.Create(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	if err := docs.SetArchived(ctx, "u1", "child-archived", true); err != nil {
		t.Fatal(err)
	}

	body, status := getWissen(t, wissenTestMux(srv), "/wissen?node=eng&status=all", codec)
	if status != http.StatusOK {
		t.Fatalf("GET subtree Wissen status=%d body=%.500s", status, body)
	}
	for _, want := range []string{"Root Active", "Middle Active", "Child Active", "Child Archived", "3 aktiv", "1 archiviert", `name="scope" value="subtree"`} {
		if !strings.Contains(body, want) {
			t.Errorf("subtree Wissen missing %q in %.2200s", want, body)
		}
	}
	for _, notWant := range []string{"Outside", "Foreign Collision"} {
		if strings.Contains(body, notWant) {
			t.Errorf("subtree Wissen leaked %q in %.2200s", notWant, body)
		}
	}

	searchBody, searchStatus := getWissen(t, wissenTestMux(srv), "/wissen?node=eng&q=Child", codec)
	if searchStatus != http.StatusOK {
		t.Fatalf("GET subtree search status=%d body=%.500s", searchStatus, searchBody)
	}
	if !strings.Contains(searchBody, "Child Active") || strings.Contains(searchBody, "Outside") || strings.Contains(searchBody, "Foreign Collision") {
		t.Errorf("subtree search scope mismatch in %.2200s", searchBody)
	}

	typeBody, typeStatus := getWissen(t, wissenTestMux(srv), "/wissen/typ?type=memory&node=eng", codec)
	if typeStatus != http.StatusOK {
		t.Fatalf("GET subtree type shelf status=%d body=%.500s", typeStatus, typeBody)
	}
	if !strings.Contains(typeBody, "Middle Active") || !strings.Contains(typeBody, "Child Active") || strings.Contains(typeBody, "Outside") {
		t.Errorf("subtree type shelf scope mismatch in %.2200s", typeBody)
	}

	selfBody, selfStatus := getWissen(t, wissenTestMux(srv), "/wissen?node=eng&scope=self&status=all", codec)
	if selfStatus != http.StatusOK {
		t.Fatalf("GET explicit self Wissen status=%d body=%.500s", selfStatus, selfBody)
	}
	if !strings.Contains(selfBody, "Root Active") || strings.Contains(selfBody, "Middle Active") || strings.Contains(selfBody, "Child Active") {
		t.Errorf("explicit cockpit self scope mismatch in %.2200s", selfBody)
	}
	if !strings.Contains(selfBody, `name="scope" value="subtree"`) {
		t.Errorf("Wissen filter must reset a newly selected node to subtree scope: %.2200s", selfBody)
	}
}

func TestWebWissenTypeStatusCountsAreShelfScopedAndNewKeepsNode(t *testing.T) {
	srv, codec, docs, nodes := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	nodeID := "n1"
	if _, err := nodes.Create(ctx, domain.Node{ID: nodeID, OwnerID: "u1", Name: "Flow", Slug: "flow", Kind: domain.KindRepo, Status: domain.NodeActive}); err != nil {
		t.Fatal(err)
	}
	for _, doc := range []domain.Document{
		{ID: "daily-active", OwnerID: "u1", NodeID: &nodeID, Type: domain.DocDaily, Path: "daily/active", Title: "Daily Active", Body: "body", CreatedAt: now, UpdatedAt: now},
		{ID: "daily-archived", OwnerID: "u1", NodeID: &nodeID, Type: domain.DocDaily, Path: "daily/archive", Title: "Daily Archived", Body: "body", CreatedAt: now, UpdatedAt: now},
		{ID: "free-active", OwnerID: "u1", NodeID: &nodeID, Type: domain.DocFree, Path: "free/active", Title: "Free Active", Body: "body", CreatedAt: now, UpdatedAt: now},
		{ID: "free-archived", OwnerID: "u1", NodeID: &nodeID, Type: domain.DocFree, Path: "free/archive", Title: "Free Archived", Body: "body", CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := docs.Create(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"daily-archived", "free-archived"} {
		if err := docs.SetArchived(ctx, "u1", id, true); err != nil {
			t.Fatal(err)
		}
	}

	body, status := getWissen(t, wissenTestMux(srv), "/wissen/typ?type=daily&node=n1&status=all", codec)
	if status != http.StatusOK {
		t.Fatalf("GET daily shelf status=%d body=%.500s", status, body)
	}
	for _, want := range []string{"Daily Active", "Daily Archived", "Aktiv 1", "Archiv 1", `href="/wissen/neu?node=n1&amp;type=daily"`} {
		if !strings.Contains(body, want) {
			t.Errorf("daily shelf missing %q in %.2200s", want, body)
		}
	}
	for _, notWant := range []string{"Free Active", "Free Archived", "Aktiv 2", "Archiv 2"} {
		if strings.Contains(body, notWant) {
			t.Errorf("daily shelf contains cross-shelf value %q in %.2200s", notWant, body)
		}
	}

	overviewBody, overviewStatus := getWissen(t, wissenTestMux(srv), "/wissen?node=n1", codec)
	if overviewStatus != http.StatusOK {
		t.Fatalf("GET node-scoped overview status=%d body=%.500s", overviewStatus, overviewBody)
	}
	if !strings.Contains(overviewBody, `href="/wissen/neu?node=n1"`) {
		t.Errorf("node-scoped overview new-document link lost node in %.2200s", overviewBody)
	}
}

func TestWebWissenAllAndArchivedSearch(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	for _, doc := range []domain.Document{
		{ID: "active", OwnerID: "u1", Type: domain.DocFree, Path: "notes/active", Title: "Active Alpha", Body: "alpha active", CreatedAt: now, UpdatedAt: now},
		{ID: "archived", OwnerID: "u1", Type: domain.DocFree, Path: "notes/archived", Title: "Archived Alpha", Body: "alpha archived", CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := docs.Create(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	if err := docs.SetArchived(ctx, "u1", "archived", true); err != nil {
		t.Fatal(err)
	}

	body, status := getWissen(t, wissenTestMux(srv), "/wissen?status=all&q=alpha", codec)
	if status != http.StatusOK {
		t.Fatalf("GET all search status=%d body=%.500s", status, body)
	}
	for _, want := range []string{"Active Alpha", "Archived Alpha", "<mark>Alpha</mark>", "Archiviert"} {
		if !strings.Contains(body, want) {
			t.Errorf("all search missing %q in %.1800s", want, body)
		}
	}
}

func TestWebWissenSearchPaginatesOneRankedActiveAndArchivedResultSet(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	for i := 0; i < wissenPageSize+2; i++ {
		id := fmt.Sprintf("alpha-%02d", i)
		if _, err := docs.Create(ctx, domain.Document{
			ID: id, OwnerID: "u1", Type: domain.DocFree, Path: "notes/" + id,
			Title: "Alpha " + id, Body: "alpha result", CreatedAt: now, UpdatedAt: now.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
		if i < 2 {
			if err := docs.SetArchived(ctx, "u1", id, true); err != nil {
				t.Fatal(err)
			}
		}
	}

	page1, status := getWissen(t, wissenTestMux(srv), "/wissen?status=all&q=alpha", codec)
	if status != http.StatusOK {
		t.Fatalf("GET search page 1 status=%d body=%.500s", status, page1)
	}
	for _, want := range []string{"Aktiv 50", "Archiv 2", "Seite 1 / 2", "q=alpha", "status=all", "page=2"} {
		if !strings.Contains(page1, want) {
			t.Errorf("search page 1 missing %q in %.2600s", want, page1)
		}
	}
	if strings.Contains(page1, "Alpha alpha-00") {
		t.Fatalf("oldest result leaked onto first ranked page: %.2600s", page1)
	}

	page2, status := getWissen(t, wissenTestMux(srv), "/wissen?status=all&q=alpha&page=2", codec)
	if status != http.StatusOK {
		t.Fatalf("GET search page 2 status=%d body=%.500s", status, page2)
	}
	for _, want := range []string{"Alpha alpha-00", "Alpha alpha-01", "Archiviert", "Seite 2 / 2"} {
		if !strings.Contains(page2, want) {
			t.Errorf("search page 2 missing %q in %.2600s", want, page2)
		}
	}
	if strings.Contains(page2, "Alpha alpha-51") {
		t.Fatalf("newest result repeated on second ranked page: %.2600s", page2)
	}
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

func TestWebWissenOverviewRecentAllIsStorePaginated(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 202; i++ {
		id := fmt.Sprintf("overview-%03d", i)
		if _, err := docs.Create(ctx, domain.Document{
			ID: id, OwnerID: "u1", Type: domain.DocFree, Path: "free/" + id,
			Title: "Overview " + id, CreatedAt: now, UpdatedAt: now.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}

	body, status := getWissen(t, wissenTestMux(srv), "/wissen?status=all&recent=all&page=5", codec)
	if status != http.StatusOK {
		t.Fatalf("GET paginated overview status=%d body=%.500s", status, body)
	}
	for _, want := range []string{"202 Dokumente", "Overview overview-000", "Overview overview-001", "Seite 5 / 5", "recent=all"} {
		if !strings.Contains(body, want) {
			t.Errorf("paginated overview missing %q in %.3000s", want, body)
		}
	}
	if strings.Contains(body, "Overview overview-201") {
		t.Fatalf("newest overview document repeated on page 5: %.3000s", body)
	}
}

func TestWebWissenFacetsFollowStatusNodeTypeAndUseHierarchicalNodeLabels(t *testing.T) {
	srv, codec, _, nodes := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	for _, node := range []domain.Node{
		{ID: "eng-one", OwnerID: "u1", Kind: domain.KindEngagement, Name: "Engagement One", Slug: "eng-one", Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now},
		{ID: "eng-two", OwnerID: "u1", Kind: domain.KindEngagement, Name: "Engagement Two", Slug: "eng-two", Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := nodes.Create(ctx, node); err != nil {
			t.Fatal(err)
		}
	}
	engOne, engTwo := "eng-one", "eng-two"
	for _, node := range []domain.Node{
		{ID: "repo-one", OwnerID: "u1", ParentID: &engOne, Kind: domain.KindRepo, Name: "Shared", Slug: "shared-one", Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now},
		{ID: "repo-two", OwnerID: "u1", ParentID: &engTwo, Kind: domain.KindRepo, Name: "Shared", Slug: "shared-two", Status: domain.NodeActive, CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := nodes.Create(ctx, node); err != nil {
			t.Fatal(err)
		}
	}
	repoOne, repoTwo := "repo-one", "repo-two"
	create := func(typ domain.DocumentType, nodeID *string, title string, tags []string, archived bool) {
		doc, err := srv.CreateDocument.Execute(ctx, "u1", usecase.CreateDocumentInput{
			Type: typ, NodeID: nodeID, Path: strings.ToLower(strings.ReplaceAll(title, " ", "-")), Title: title, Body: "body", Tags: tags,
		})
		if err != nil {
			t.Fatal(err)
		}
		if archived {
			if err := srv.SetArchived.Execute(ctx, "u1", doc.ID, true); err != nil {
				t.Fatal(err)
			}
		}
	}
	create(domain.DocProject, &repoOne, "Scoped Archived", []string{"ops", "common"}, true)
	create(domain.DocProject, &repoOne, "Scoped Active", []string{"active-only"}, false)
	create(domain.DocProject, &repoTwo, "Other Archived", []string{"other"}, true)
	create(domain.DocMemory, &repoOne, "Memory Archived", []string{"memory-only"}, true)

	body, status := getWissen(t, wissenTestMux(srv), "/wissen/typ?type=project&node=repo-one&status=archived", codec)
	if status != http.StatusOK {
		t.Fatalf("GET scoped facets status=%d body=%.500s", status, body)
	}
	for _, want := range []string{"Scoped Archived", "ops", "common", "Engagement One / Shared", "Engagement Two / Shared"} {
		if !strings.Contains(body, want) {
			t.Errorf("scoped facets missing %q in %.3200s", want, body)
		}
	}
	for _, notWant := range []string{"Scoped Active", "Other Archived", "Memory Archived", "active-only", "other", "memory-only"} {
		if strings.Contains(body, notWant) {
			t.Errorf("scoped facets leaked %q in %.3200s", notWant, body)
		}
	}
}

func TestWebWissenBulkCurationIsSelectableAtomicOwnerScopedAndEmitsAfterCommit(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	recorder := &recordingWissenEmitter{}
	srv.Emitter = recorder
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	for _, doc := range []domain.Document{
		{ID: "bulk-one", OwnerID: "u1", Type: domain.DocMemory, Path: "memory/one", Title: "Bulk One", Pinned: true, CreatedAt: now, UpdatedAt: now},
		{ID: "bulk-two", OwnerID: "u1", Type: domain.DocMemory, Path: "memory/two", Title: "Bulk Two", Pinned: true, CreatedAt: now, UpdatedAt: now},
		{ID: "bulk-free", OwnerID: "u1", Type: domain.DocFree, Path: "free/one", Title: "Bulk Free", CreatedAt: now, UpdatedAt: now},
		{ID: "bulk-foreign", OwnerID: "u2", Type: domain.DocMemory, Path: "memory/foreign", Title: "Bulk Foreign", CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := docs.Create(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	mux := wissenTestMux(srv)
	body, status := getWissen(t, mux, "/wissen/typ?type=memory", codec)
	if status != http.StatusOK {
		t.Fatalf("GET selectable Wissen status=%d", status)
	}
	for _, want := range []string{
		`/static/js/wissen-select.js`, `data-wissen-select-toggle`, `data-document-id="bulk-one"`,
		`data-context-eligible="true"`, `id="wissenBulkForm"`, `data-wissen-action-bar`,
		`data-wissen-action="archive"`, `data-wissen-mode="immer"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("selectable Wissen missing %q in %.4000s", want, body)
		}
	}

	_, status = postWissenBulk(t, mux, codec, url.Values{"ids": {"bulk-one,bulk-foreign"}, "action": {"archive"}})
	if status != http.StatusNotFound {
		t.Fatalf("mixed-owner bulk status=%d, want 404", status)
	}
	one, _ := docs.Get(ctx, "u1", "bulk-one")
	if one.Archived || len(recorder.events) != 0 {
		t.Fatalf("failed bulk changed state or emitted: doc=%+v events=%+v", one, recorder.events)
	}

	_, status = postWissenBulk(t, mux, codec, url.Values{"ids": {"bulk-one,bulk-two"}, "action": {"archive"}})
	if status != http.StatusNoContent {
		t.Fatalf("archive bulk status=%d, want 204", status)
	}
	if len(recorder.events) != 2 {
		t.Fatalf("successful bulk emitted %d events, want 2", len(recorder.events))
	}
	for _, event := range recorder.events {
		if event.Type != domain.EventDocumentUpdated || event.Data["action"] != "archive" || event.Data["title"] == "" {
			t.Fatalf("bulk event lacks action/title: %+v", event)
		}
	}
	for _, id := range []string{"bulk-one", "bulk-two"} {
		doc, _ := docs.Get(ctx, "u1", id)
		if !doc.Archived || doc.ArchivedAt == nil || doc.UpdatedByRef != "Martin" {
			t.Fatalf("bulk archive lacks state/provenance for %s: %+v", id, doc)
		}
	}

	_, status = postWissenBulk(t, mux, codec, url.Values{"ids": {"bulk-one,bulk-two"}, "action": {"restore"}})
	if status != http.StatusNoContent {
		t.Fatalf("restore bulk status=%d, want 204", status)
	}
	_, status = postWissenBulk(t, mux, codec, url.Values{"ids": {"bulk-one,bulk-two"}, "action": {"mode"}, "mode": {"immer"}})
	if status != http.StatusNoContent {
		t.Fatalf("context bulk status=%d, want 204", status)
	}
	if len(recorder.events) != 6 || recorder.events[2].Data["action"] != "restore" || recorder.events[4].Data["action"] != "context.immer" {
		t.Fatalf("bulk restore/context events = %+v", recorder.events)
	}
	for _, id := range []string{"bulk-one", "bulk-two"} {
		doc, _ := docs.Get(ctx, "u1", id)
		if doc.Archived || doc.ArchivedAt != nil || doc.ContextMode != domain.ContextModeImmer {
			t.Fatalf("bulk restore/context incomplete for %s: %+v", id, doc)
		}
	}

	_, status = postWissenBulk(t, mux, codec, url.Values{"ids": {"bulk-one,bulk-free"}, "action": {"mode"}, "mode": {"nie"}})
	if status != http.StatusBadRequest {
		t.Fatalf("mixed context eligibility status=%d, want 400", status)
	}
	one, _ = docs.Get(ctx, "u1", "bulk-one")
	if one.ContextMode != domain.ContextModeImmer || len(recorder.events) != 6 {
		t.Fatalf("invalid context bulk changed state or emitted: doc=%+v events=%+v", one, recorder.events)
	}
}

func postWissenBulk(t *testing.T, handler http.Handler, codec *websession.Codec, values url.Values) (string, int) {
	t.Helper()
	cookie, err := codec.Issue("u1")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/wissen/bulk", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Body.String(), rec.Code
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

func TestWebWissenTypeSearchPaginatesWithinShelf(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	for i := 0; i < wissenPageSize+2; i++ {
		id := fmt.Sprintf("free-alpha-%02d", i)
		if _, err := docs.Create(ctx, domain.Document{
			ID: id, OwnerID: "u1", Type: domain.DocFree, Path: "free/" + id,
			Title: "Alpha " + id, Body: "alpha", CreatedAt: now, UpdatedAt: now.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
		if i < 2 {
			if err := docs.SetArchived(ctx, "u1", id, true); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := docs.Create(ctx, domain.Document{
		ID: "daily-alpha", OwnerID: "u1", Type: domain.DocDaily, Path: "daily/alpha",
		Title: "Daily Alpha Outside Shelf", Body: "alpha", CreatedAt: now, UpdatedAt: now.Add(3 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	body, status := getWissen(t, wissenTestMux(srv), "/wissen/typ?type=free&status=all&q=alpha&page=2", codec)
	if status != http.StatusOK {
		t.Fatalf("GET free search page 2 status=%d body=%.500s", status, body)
	}
	for _, want := range []string{"Alpha free-alpha-00", "Alpha free-alpha-01", "Archiviert", "Seite 2 / 2"} {
		if !strings.Contains(body, want) {
			t.Errorf("free search page 2 missing %q in %.2600s", want, body)
		}
	}
	if strings.Contains(body, "Daily Alpha Outside Shelf") || strings.Contains(body, "Alpha free-alpha-51") {
		t.Fatalf("free search page escaped shelf or repeated page 1: %.2600s", body)
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
	mux.Handle("POST /wissen/bulk", s.webAuth(http.HandlerFunc(s.handleWebWissenBulk)))
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
