package httpserver_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

// pngPixelBytes is a valid 1x1 PNG (sniffs as image/png) — the REST-layer
// twin of usecase_test's pngPixel fixture.
func pngPixelBytes(t *testing.T) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// newArtifactServer wires a Server with just enough usecases for the
// artifact REST + serve routes. The authenticated user is a fixed "u1"
// (seeded directly, like newWebNodeLogoServer, rather than relying on
// FakeIDGen call order to land on some generated id) — both a bearer token
// (REST) and a session cookie (the webAuth-guarded serve route) resolve to
// it. Returns the server, its NodeStore/ArtifactStore fakes (for direct
// seeding/introspection), the SSE bus, and the session cookie for u1.
func newArtifactServer(t *testing.T) (*httpserver.Server, *testutil.FakeNodeStore, *testutil.FakeArtifactStore, *sse.Bus, *http.Cookie) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	bus := sse.NewBus()
	emitter := sse.NewEmitter(bus, &fakeActivityStore{}, ids, clk)
	nodes := testutil.NewFakeNodeStore()
	artifacts := testutil.NewFakeArtifactStore()
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
		Verifier:       testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:         usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:            bus,
		Emitter:        emitter,
		Clock:          clk,
		Session:        codec,
		Users:          users,
		ListNodes:      usecase.ListNodes{Nodes: nodes},
		UploadArtifact: usecase.UploadArtifact{Nodes: nodes, Artifacts: artifacts, IDs: ids, Clock: clk, Emitter: emitter},
		RenameArtifact: usecase.RenameArtifact{Nodes: nodes, Artifacts: artifacts, Emitter: emitter},
		ListArtifacts:  usecase.ListArtifacts{Nodes: nodes, Artifacts: artifacts},
		DeleteArtifact: usecase.DeleteArtifact{Artifacts: artifacts, Emitter: emitter},
		GetArtifact:    usecase.GetArtifact{Artifacts: artifacts},
	}
	cv, err := codec.Issue("u1")
	if err != nil {
		t.Fatal(err)
	}
	return srv, nodes, artifacts, bus, &http.Cookie{Name: "flow_session", Value: cv}
}

func seedArtifactNode(t *testing.T, ns *testutil.FakeNodeStore, id, ownerID string) {
	t.Helper()
	n, err := domain.NewNode(id, ownerID, id, id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	n.Kind = domain.KindEngagement
	if _, err := ns.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}
}

func doArtifact(t *testing.T, ts *httptest.Server, method, path string, body []byte) *http.Response {
	t.Helper()
	var r *bytes.Reader
	if body != nil {
		r = bytes.NewReader(body)
	} else {
		r = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, ts.URL+path, r)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer x")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestHandleUploadArtifact_HappyPath_EmitsCreated(t *testing.T) {
	srv, ns, _, bus, _ := newArtifactServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	owner := "u1"
	seedArtifactNode(t, ns, "n1", owner)
	ch, cancel := bus.Subscribe(owner)
	defer cancel()

	reqBody, _ := json.Marshal(map[string]string{
		"name": "Diagram.png", "mime": "application/octet-stream",
		"dataBase64": base64.StdEncoding.EncodeToString(pngPixelBytes(t)),
	})
	res := doArtifact(t, ts, http.MethodPost, "/api/v1/nodes/n1/artifacts", reqBody)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
	var got domain.Artifact
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Slug != "diagram" || got.Mime != "image/png" {
		t.Errorf("got slug=%q mime=%q, want diagram/image/png", got.Slug, got.Mime)
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

func TestHandleUploadArtifact_ReplaceSlug_EmitsUpdated(t *testing.T) {
	srv, ns, _, _, _ := newArtifactServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	owner := "u1"
	seedArtifactNode(t, ns, "n1", owner)

	firstBody, _ := json.Marshal(map[string]string{
		"name": "Diagram.png", "mime": "application/octet-stream",
		"dataBase64": base64.StdEncoding.EncodeToString(pngPixelBytes(t)),
	})
	res := doArtifact(t, ts, http.MethodPost, "/api/v1/nodes/n1/artifacts", firstBody)
	var first domain.Artifact
	_ = json.NewDecoder(res.Body).Decode(&first)
	_ = res.Body.Close()

	replaceBody, _ := json.Marshal(map[string]string{
		"name": "Diagram-v2.png", "mime": "application/pdf",
		"dataBase64": base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 mock content")),
		"slug":       first.Slug,
	})
	res2 := doArtifact(t, ts, http.MethodPost, "/api/v1/nodes/n1/artifacts", replaceBody)
	defer func() { _ = res2.Body.Close() }()
	if res2.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res2.StatusCode)
	}
	var second domain.Artifact
	if err := json.NewDecoder(res2.Body).Decode(&second); err != nil {
		t.Fatal(err)
	}
	if second.Slug != first.Slug {
		t.Errorf("slug changed on replace: got %q, want %q", second.Slug, first.Slug)
	}
	if second.Ref == first.Ref {
		t.Error("ref did not change after replacing with different content")
	}
}

func TestHandleUploadArtifact_TooLarge_400(t *testing.T) {
	srv, ns, _, _, _ := newArtifactServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	owner := "u1"
	seedArtifactNode(t, ns, "n1", owner)

	oversized := make([]byte, domain.MaxArtifactBytes+1)
	body, _ := json.Marshal(map[string]string{
		"name": "huge.bin", "mime": "application/octet-stream",
		"dataBase64": base64.StdEncoding.EncodeToString(oversized),
	})
	res := doArtifact(t, ts, http.MethodPost, "/api/v1/nodes/n1/artifacts", body)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", res.StatusCode)
	}
}

func TestHandleUploadArtifact_QuotaExceeded_413(t *testing.T) {
	srv, ns, as, _, _ := newArtifactServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	owner := "u1"
	seedArtifactNode(t, ns, "n1", owner)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: owner, NodeID: "n1", Slug: "existing", Name: "existing.pdf",
		Mime: "application/pdf", SizeBytes: usecase.MaxArtifactBytesPerOwner - 4,
	}); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{
		"name": "report.pdf", "mime": "application/pdf",
		"dataBase64": base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 mock content overflowing quota")),
	})
	res := doArtifact(t, ts, http.MethodPost, "/api/v1/nodes/n1/artifacts", body)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", res.StatusCode)
	}
}

func TestHandleUploadArtifact_ForeignNode_404(t *testing.T) {
	srv, ns, _, _, _ := newArtifactServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	seedArtifactNode(t, ns, "n1", "other-owner") // belongs to a different owner

	body, _ := json.Marshal(map[string]string{
		"name": "x.pdf", "mime": "application/pdf",
		"dataBase64": base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 x")),
	})
	res := doArtifact(t, ts, http.MethodPost, "/api/v1/nodes/n1/artifacts", body)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (User B cannot upload onto A's node)", res.StatusCode)
	}
}

func TestHandleListArtifacts_AncestorChain(t *testing.T) {
	srv, ns, as, _, _ := newArtifactServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	owner := "u1"
	seedArtifactNode(t, ns, "n1", owner)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: owner, NodeID: "n1", Slug: "logo", Name: "Logo.png", Mime: "image/png",
	}); err != nil {
		t.Fatal(err)
	}

	res := doArtifact(t, ts, http.MethodGet, "/api/v1/nodes/n1/artifacts", nil)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var list []domain.Artifact
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Slug != "logo" {
		t.Errorf("list = %+v, want one artifact 'logo'", list)
	}
}

func TestHandleDeleteArtifact_NoContent_EmitsDeleted(t *testing.T) {
	srv, ns, as, bus, _ := newArtifactServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	owner := "u1"
	seedArtifactNode(t, ns, "n1", owner)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: owner, NodeID: "n1", Slug: "logo", Name: "Logo.png", Mime: "image/png",
	}); err != nil {
		t.Fatal(err)
	}
	ch, cancel := bus.Subscribe(owner)
	defer cancel()

	res := doArtifact(t, ts, http.MethodDelete, "/api/v1/nodes/n1/artifacts/logo", nil)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}
	select {
	case ev := <-ch:
		if ev.Type != domain.EventArtifactDeleted {
			t.Errorf("event type = %q, want artifact.deleted", ev.Type)
		}
	default:
		t.Error("want artifact.deleted event, got none")
	}
	if _, err := as.Get(context.Background(), owner, "n1", "logo"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Errorf("artifact still present after delete: %v", err)
	}
}

func TestHandleDeleteArtifact_NotFound_404(t *testing.T) {
	srv, ns, _, _, _ := newArtifactServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	owner := "u1"
	seedArtifactNode(t, ns, "n1", owner)

	res := doArtifact(t, ts, http.MethodDelete, "/api/v1/nodes/n1/artifacts/ghost", nil)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

// TestHandleDeleteArtifact_CrossTenant_404 is the owner-scope negative test:
// an artifact owned by a foreign tenant sitting at the SAME node/slug the
// authenticated owner can also see must not be deletable (or even visible)
// through this owner's request.
func TestHandleDeleteArtifact_CrossTenant_404(t *testing.T) {
	srv, ns, as, _, _ := newArtifactServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	owner := "u1"
	seedArtifactNode(t, ns, "n1", owner)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: "other-owner", NodeID: "n1", Slug: "logo", Name: "Logo.png", Mime: "image/png",
	}); err != nil {
		t.Fatal(err)
	}

	res := doArtifact(t, ts, http.MethodDelete, "/api/v1/nodes/n1/artifacts/logo", nil)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (cross-tenant delete must not succeed)", res.StatusCode)
	}
	if _, err := as.Get(context.Background(), "other-owner", "n1", "logo"); err != nil {
		t.Error("foreign owner's artifact must survive an unrelated tenant's delete attempt")
	}
}

// --- Serve route ---

func TestHandleServeArtifact_Image_InlineWithETag(t *testing.T) {
	srv, ns, as, _, cookie := newArtifactServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	owner := "u1"
	seedArtifactNode(t, ns, "n1", owner)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: owner, NodeID: "n1", Slug: "logo", Name: "Logo.png", Mime: "image/png",
		Ref: "abc123def456", Bytes: pngPixelBytes(t),
	}); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/nodes/n1/artifacts/logo", nil)
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if res.Header.Get("Content-Type") != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", res.Header.Get("Content-Type"))
	}
	if res.Header.Get("Content-Disposition") != "inline" {
		t.Errorf("Content-Disposition = %q, want inline", res.Header.Get("Content-Disposition"))
	}
	if res.Header.Get("ETag") != `"abc123def456"` {
		t.Errorf("ETag = %q, want %q", res.Header.Get("ETag"), `"abc123def456"`)
	}
	if res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", res.Header.Get("X-Content-Type-Options"))
	}
	if res.Header.Get("Cache-Control") != "private, no-cache" {
		t.Errorf("Cache-Control (bare url) = %q, want private, no-cache", res.Header.Get("Cache-Control"))
	}
}

func TestHandleServeArtifact_VersionedQuery_Immutable(t *testing.T) {
	srv, ns, as, _, cookie := newArtifactServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	owner := "u1"
	seedArtifactNode(t, ns, "n1", owner)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: owner, NodeID: "n1", Slug: "logo", Name: "Logo.png", Mime: "image/png",
		Ref: "abc123def456", Bytes: pngPixelBytes(t),
	}); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/nodes/n1/artifacts/logo?v=abc123def456", nil)
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if got := res.Header.Get("Cache-Control"); got != "private, max-age=31536000, immutable" {
		t.Errorf("Cache-Control (?v=ref) = %q, want immutable", got)
	}
}

func TestHandleServeArtifact_IfNoneMatch_304(t *testing.T) {
	srv, ns, as, _, cookie := newArtifactServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	owner := "u1"
	seedArtifactNode(t, ns, "n1", owner)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: owner, NodeID: "n1", Slug: "logo", Name: "Logo.png", Mime: "image/png",
		Ref: "abc123def456", Bytes: pngPixelBytes(t),
	}); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/nodes/n1/artifacts/logo", nil)
	req.AddCookie(cookie)
	req.Header.Set("If-None-Match", `"abc123def456"`)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotModified {
		t.Errorf("status = %d, want 304", res.StatusCode)
	}
	// 304 must still carry the caching headers (RFC 7232).
	if res.Header.Get("ETag") != `"abc123def456"` {
		t.Errorf("304 missing ETag: %q", res.Header.Get("ETag"))
	}
}

func TestHandleServeArtifact_NonImage_Attachment(t *testing.T) {
	srv, ns, as, _, cookie := newArtifactServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	owner := "u1"
	seedArtifactNode(t, ns, "n1", owner)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: owner, NodeID: "n1", Slug: "report", Name: "Report.pdf", Mime: "application/pdf",
		Ref: "deadbeef0000", Bytes: []byte("%PDF-1.4 mock"),
	}); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/nodes/n1/artifacts/report", nil)
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	want := `attachment; filename="Report.pdf"`
	if got := res.Header.Get("Content-Disposition"); got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}
}

// TestHandleServeArtifact_CrossTenant_404 is the serve-route owner-scope
// negative test: a foreign owner's artifact at the same node/slug must 404
// for this tenant, never leaking its bytes.
func TestHandleServeArtifact_CrossTenant_404(t *testing.T) {
	srv, ns, as, _, cookie := newArtifactServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	owner := "u1"
	seedArtifactNode(t, ns, "n1", owner)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: "other-owner", NodeID: "n1", Slug: "secret", Name: "Secret.pdf", Mime: "application/pdf",
		Ref: "cafebabef00d", Bytes: []byte("%PDF-1.4 secret"),
	}); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/nodes/n1/artifacts/secret", nil)
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (cross-tenant serve)", res.StatusCode)
	}
	body, _ := readAll(res)
	if strings.Contains(body, "secret") {
		t.Error("response body must not leak the foreign artifact's content")
	}
}

func readAll(res *http.Response) (string, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(res.Body)
	return buf.String(), err
}
