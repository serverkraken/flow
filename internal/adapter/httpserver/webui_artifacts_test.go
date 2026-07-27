package httpserver_test

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// newWebArtifactsServer wires a Server with the node + artifact usecases (web
// session auth) needed for the cockpit gallery's upload/rename/delete routes
// and its GET fragment. Returns the test server, the u1 session cookie, and
// the NodeStore/ArtifactStore fakes + SSE bus for direct seeding/assertions.
func newWebArtifactsServer(t *testing.T) (*httptest.Server, *http.Cookie, *testutil.FakeNodeStore, *testutil.FakeArtifactStore, *sse.Bus) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	bus := sse.NewBus()
	emitter := sse.NewEmitter(bus, &fakeActivityStore{}, ids, clk)
	ns := testutil.NewFakeNodeStore()
	artifacts := testutil.NewFakeArtifactStore()
	ss := testutil.NewFakeSessionStore()
	users := testutil.NewFakeUserStore()
	u, err := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "M")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.UpsertBySub(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	codec := websession.NewCodec("test-secret-test-secret-test-12", time.Hour)

	srv := &httpserver.Server{
		Users:          users,
		Session:        codec,
		Bus:            bus,
		Emitter:        emitter,
		Clock:          clk,
		Ensure:         usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		CreateNode:     usecase.CreateNode{Nodes: ns, IDs: ids, Clock: clk},
		ListNodes:      usecase.ListNodes{Nodes: ns},
		GetNode:        usecase.GetNode{Nodes: ns},
		NodeAncestors:  usecase.NodeAncestors{Nodes: ns},
		Stats:          usecase.StatsComputer{Sessions: ss, Nodes: ns, Clock: clk, Loc: time.UTC},
		UploadArtifact: usecase.UploadArtifact{Nodes: ns, Artifacts: artifacts, IDs: ids, Clock: clk, Emitter: emitter},
		RenameArtifact: usecase.RenameArtifact{Artifacts: artifacts, Emitter: emitter},
		ListArtifacts:  usecase.ListArtifacts{Nodes: ns, Artifacts: artifacts},
		DeleteArtifact: usecase.DeleteArtifact{Artifacts: artifacts, Emitter: emitter},
		GetArtifact:    usecase.GetArtifact{Artifacts: artifacts},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cv, err := codec.Issue("u1")
	if err != nil {
		t.Fatal(err)
	}
	return ts, &http.Cookie{Name: "flow_session", Value: cv}, ns, artifacts, bus
}

func seedArtifactWebNode(t *testing.T, ns *testutil.FakeNodeStore, id, ownerID string, parent *string) domain.Node {
	t.Helper()
	n, err := domain.NewNode(id, ownerID, id, id, time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	n.Kind = domain.KindRepo
	n.ParentID = parent
	if _, err := ns.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	return n
}

// multipartUpload builds a "file" (+ optional "slug") multipart body for the
// gallery's upload/replace form.
func multipartUpload(t *testing.T, filename string, data []byte, slug string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if slug != "" {
		if err := mw.WriteField("slug", slug); err != nil {
			t.Fatal(err)
		}
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --- Upload: happy path + SSE ---------------------------------------------

func TestHandleWebNodeArtifactUpload_HappyPath_RendersFragmentAndEmitsCreated(t *testing.T) {
	ts, cookie, ns, _, bus := newWebArtifactsServer(t)
	seedArtifactWebNode(t, ns, "n1", "u1", nil)
	ch, cancel := bus.Subscribe("u1")
	defer cancel()

	body, ct := multipartUpload(t, "Diagram.png", pngPixelBytes(t), "")
	res := postNMultipart(t, ts, cookie, "/nodes/n1/artifacts", ct, body)
	got := readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%.400s", res.StatusCode, got)
	}
	if !strings.Contains(got, `class="artcard"`) || !strings.Contains(got, "Diagram.png") {
		t.Fatalf("response must be the re-rendered #cockpit-artifacts fragment with the new card:\n%.800s", got)
	}
	select {
	case ev := <-ch:
		if ev.Type != domain.EventArtifactCreated {
			t.Errorf("event type = %q, want artifact.created", ev.Type)
		}
	default:
		t.Error("want artifact.created event, got none")
	}
}

// TestHandleWebNodeArtifactUpload_Replace_EmitsUpdated exercises the
// gallery's "Ersetzen" affordance: a hidden "slug" field on the second
// upload overwrites the existing artifact in place (Codex-Fund #4).
func TestHandleWebNodeArtifactUpload_Replace_EmitsUpdated(t *testing.T) {
	ts, cookie, ns, as, bus := newWebArtifactsServer(t)
	seedArtifactWebNode(t, ns, "n1", "u1", nil)

	body1, ct1 := multipartUpload(t, "Diagram.png", pngPixelBytes(t), "")
	res1 := postNMultipart(t, ts, cookie, "/nodes/n1/artifacts", ct1, body1)
	_ = readBody(t, res1)
	if res1.StatusCode != http.StatusOK {
		t.Fatalf("first upload status = %d, want 200", res1.StatusCode)
	}
	first, err := as.Get(context.Background(), "u1", "n1", "diagram")
	if err != nil {
		t.Fatalf("expected seeded artifact 'diagram': %v", err)
	}

	ch, cancel := bus.Subscribe("u1")
	defer cancel()
	body2, ct2 := multipartUpload(t, "Diagram-v2.png", []byte("%PDF-1.4 mock content replaced"), "diagram")
	res2 := postNMultipart(t, ts, cookie, "/nodes/n1/artifacts", ct2, body2)
	got2 := readBody(t, res2)
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("replace status = %d, want 200; body=%.400s", res2.StatusCode, got2)
	}
	second, err := as.Get(context.Background(), "u1", "n1", "diagram")
	if err != nil {
		t.Fatalf("expected artifact 'diagram' to still exist after replace: %v", err)
	}
	if second.Slug != first.Slug {
		t.Errorf("slug changed on replace: got %q, want %q", second.Slug, first.Slug)
	}
	if second.Ref == first.Ref {
		t.Error("ref did not change after replacing with different content")
	}
	select {
	case ev := <-ch:
		if ev.Type != domain.EventArtifactUpdated {
			t.Errorf("event type = %q, want artifact.updated", ev.Type)
		}
	default:
		t.Error("want artifact.updated event, got none")
	}
}

// --- Rename -----------------------------------------------------------

func TestHandleWebNodeArtifactRename_NameChangesSlugRefStable_EmitsUpdated(t *testing.T) {
	ts, cookie, ns, as, bus := newWebArtifactsServer(t)
	seedArtifactWebNode(t, ns, "n1", "u1", nil)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: "u1", NodeID: "n1", Slug: "logo", Name: "Old.png", Mime: "image/png", Ref: "abc123def456",
	}); err != nil {
		t.Fatal(err)
	}
	ch, cancel := bus.Subscribe("u1")
	defer cancel()

	res := postN(t, ts, cookie, "/nodes/n1/artifacts/logo/rename", url.Values{"name": {"New Name.png"}})
	got := readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%.400s", res.StatusCode, got)
	}
	if !strings.Contains(got, "New Name.png") {
		t.Fatalf("fragment must show the new name:\n%.800s", got)
	}
	updated, err := as.GetMeta(context.Background(), "u1", "n1", "logo")
	if err != nil {
		t.Fatalf("artifact must still exist at the same slug: %v", err)
	}
	if updated.Name != "New Name.png" {
		t.Errorf("name = %q, want %q", updated.Name, "New Name.png")
	}
	if updated.Slug != "logo" || updated.Ref != "abc123def456" {
		t.Errorf("slug/ref must stay stable: slug=%q ref=%q", updated.Slug, updated.Ref)
	}
	select {
	case ev := <-ch:
		if ev.Type != domain.EventArtifactUpdated {
			t.Errorf("event type = %q, want artifact.updated", ev.Type)
		}
	default:
		t.Error("want artifact.updated event, got none")
	}
}

// --- Delete -----------------------------------------------------------

func TestHandleWebNodeArtifactDelete_RemovesAndEmitsDeleted(t *testing.T) {
	ts, cookie, ns, as, bus := newWebArtifactsServer(t)
	seedArtifactWebNode(t, ns, "n1", "u1", nil)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: "u1", NodeID: "n1", Slug: "logo", Name: "Logo.png", Mime: "image/png",
	}); err != nil {
		t.Fatal(err)
	}
	ch, cancel := bus.Subscribe("u1")
	defer cancel()

	res := postN(t, ts, cookie, "/nodes/n1/artifacts/logo/delete", url.Values{})
	got := readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%.400s", res.StatusCode, got)
	}
	if strings.Contains(got, "Logo.png") {
		t.Fatalf("deleted artifact must not appear in the re-rendered fragment:\n%.800s", got)
	}
	if _, err := as.Get(context.Background(), "u1", "n1", "logo"); err == nil {
		t.Error("artifact must be gone from the store after delete")
	}
	select {
	case ev := <-ch:
		if ev.Type != domain.EventArtifactDeleted {
			t.Errorf("event type = %q, want artifact.deleted", ev.Type)
		}
	default:
		t.Error("want artifact.deleted event, got none")
	}
}

// --- Owner-scope negative tests -----------------------------------------

// TestHandleWebNodeArtifactUpload_ForeignNode_NoEffect is the mandatory
// owner-scope negative test for upload: a node owned by a different tenant
// must 404 and must not gain an artifact.
func TestHandleWebNodeArtifactUpload_ForeignNode_NoEffect(t *testing.T) {
	ts, cookie, ns, as, _ := newWebArtifactsServer(t)
	seedArtifactWebNode(t, ns, "n1", "other-owner", nil)

	body, ct := multipartUpload(t, "Diagram.png", pngPixelBytes(t), "")
	res := postNMultipart(t, ts, cookie, "/nodes/n1/artifacts", ct, body)
	_ = readBody(t, res)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (foreign node)", res.StatusCode)
	}
	list, err := as.List(context.Background(), "u1", "n1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("no artifact should have been created for the requesting owner: %+v", list)
	}
}

// TestHandleWebNodeArtifactRename_CrossTenant_NoEffect is the owner-scope
// negative test for rename: an artifact owned by a foreign tenant at the
// same node/slug this owner can also see must not be renamable through this
// owner's request.
func TestHandleWebNodeArtifactRename_CrossTenant_NoEffect(t *testing.T) {
	ts, cookie, ns, as, _ := newWebArtifactsServer(t)
	seedArtifactWebNode(t, ns, "n1", "u1", nil)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: "other-owner", NodeID: "n1", Slug: "logo", Name: "Foreign.png", Mime: "image/png",
	}); err != nil {
		t.Fatal(err)
	}

	res := postN(t, ts, cookie, "/nodes/n1/artifacts/logo/rename", url.Values{"name": {"Hijacked.png"}})
	_ = readBody(t, res)
	still, err := as.GetMeta(context.Background(), "other-owner", "n1", "logo")
	if err != nil {
		t.Fatalf("foreign owner's artifact must survive: %v", err)
	}
	if still.Name != "Foreign.png" {
		t.Errorf("foreign owner's artifact must be untouched, got name=%q", still.Name)
	}
}

// TestHandleWebNodeArtifactDelete_CrossTenant_NoEffect mirrors the REST
// layer's owner-scope negative test (artifacts_test.go) for the web route.
func TestHandleWebNodeArtifactDelete_CrossTenant_NoEffect(t *testing.T) {
	ts, cookie, ns, as, _ := newWebArtifactsServer(t)
	seedArtifactWebNode(t, ns, "n1", "u1", nil)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: "other-owner", NodeID: "n1", Slug: "logo", Name: "Foreign.png", Mime: "image/png",
	}); err != nil {
		t.Fatal(err)
	}

	res := postN(t, ts, cookie, "/nodes/n1/artifacts/logo/delete", url.Values{})
	_ = readBody(t, res)
	if _, err := as.Get(context.Background(), "other-owner", "n1", "logo"); err != nil {
		t.Error("foreign owner's artifact must survive an unrelated tenant's delete attempt")
	}
}

// --- Error paths: inline, never a 500, never a popup ----------------------

func TestHandleWebNodeArtifactUpload_TooLarge_InlineErrorNo500(t *testing.T) {
	ts, cookie, ns, as, _ := newWebArtifactsServer(t)
	seedArtifactWebNode(t, ns, "n1", "u1", nil)

	oversized := make([]byte, domain.MaxArtifactBytes+1)
	body, ct := multipartUpload(t, "huge.bin", oversized, "")
	res := postNMultipart(t, ts, cookie, "/nodes/n1/artifacts", ct, body)
	got := readBody(t, res)
	if res.StatusCode == http.StatusInternalServerError {
		t.Fatalf("must not 500 on an oversized upload; body=%.400s", got)
	}
	if !strings.Contains(got, "role=\"alert\"") {
		t.Fatalf("oversized upload must surface an inline role=alert error:\n%.800s", got)
	}
	list, _ := as.List(context.Background(), "u1", "n1")
	if len(list) != 0 {
		t.Errorf("no artifact should have been stored for an oversized upload: %+v", list)
	}
}

func TestHandleWebNodeArtifactUpload_QuotaExceeded_InlineErrorNo500(t *testing.T) {
	ts, cookie, ns, as, _ := newWebArtifactsServer(t)
	seedArtifactWebNode(t, ns, "n1", "u1", nil)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: "u1", NodeID: "n1", Slug: "existing", Name: "existing.pdf",
		Mime: "application/pdf", SizeBytes: usecase.MaxArtifactBytesPerOwner - 4,
	}); err != nil {
		t.Fatal(err)
	}

	body, ct := multipartUpload(t, "report.pdf", []byte("%PDF-1.4 mock content overflowing quota"), "")
	res := postNMultipart(t, ts, cookie, "/nodes/n1/artifacts", ct, body)
	got := readBody(t, res)
	if res.StatusCode == http.StatusInternalServerError {
		t.Fatalf("must not 500 on quota exceeded; body=%.400s", got)
	}
	if !strings.Contains(got, "role=\"alert\"") {
		t.Fatalf("quota exceeded must surface an inline role=alert error:\n%.800s", got)
	}
	list, _ := as.List(context.Background(), "u1", "n1")
	if len(list) != 1 {
		t.Errorf("quota-exceeded upload must not add a second artifact: %+v", list)
	}
}
