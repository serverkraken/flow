package httpserver_test

// Web gallery tests for /wissen/artefakte (Task 4, free-artifacts) — mirrors
// webui_artifacts_test.go's node-gallery suite, reusing newWebArtifactsServer
// (it already wires UploadArtifact/RenameArtifact/ListArtifacts/DeleteArtifact/
// GetArtifact; the free path never touches Nodes, so the node fakes it also
// returns are simply unused here).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// --- Page vs fragment route separation (gemini-Fund #2, CRITICAL) ---------

// TestHandleWebWissenArtifacts_PageRendersAppShell verifies GET
// /wissen/artefakte returns the FULL page (AppShell chrome + pagehead),
// unlike the fragment route.
func TestHandleWebWissenArtifacts_PageRendersAppShell(t *testing.T) {
	ts, cookie, _, _, _ := newWebArtifactsServer(t)
	res := getWith(t, ts, cookie, "/wissen/artefakte")
	got := readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%.400s", res.StatusCode, got)
	}
	if !strings.Contains(got, `id="wissen-artefakte"`) {
		t.Fatalf("full page must mount the #wissen-artefakte SSE container:\n%.800s", got)
	}
	if !strings.Contains(got, `hx-get="/ui/wissen/artefakte"`) {
		t.Fatalf("SSE container must hx-get the FRAGMENT route, not itself:\n%.800s", got)
	}
	if !strings.Contains(got, "topbar") && !strings.Contains(got, "nav") {
		t.Fatalf("full page must render AppShell chrome (topbar/nav):\n%.800s", got)
	}
}

// TestHandleWebWissenArtifactsFragment_NoAppShell verifies GET
// /ui/wissen/artefakte returns ONLY the grid/upload-form/error slot — no
// AppShell, no pagehead. This is the CRITICAL route-separation guard: if the
// SSE container's hx-get ever pointed at the full page instead, this route
// would still be reachable and distinguishable from the page route.
func TestHandleWebWissenArtifactsFragment_NoAppShell(t *testing.T) {
	ts, cookie, _, _, _ := newWebArtifactsServer(t)
	res := getWith(t, ts, cookie, "/ui/wissen/artefakte")
	got := readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%.400s", res.StatusCode, got)
	}
	if strings.Contains(got, `id="wissen-artefakte"`) {
		t.Fatalf("fragment route must NOT re-mount the #wissen-artefakte SSE container (that would nest it):\n%.800s", got)
	}
	if strings.Contains(got, "<html") || strings.Contains(got, "<body") {
		t.Fatalf("fragment route must not render a full HTML document:\n%.800s", got)
	}
	if !strings.Contains(got, `class="artupload mt-3"`) {
		t.Fatalf("fragment must include the upload form:\n%.800s", got)
	}
}

// --- Upload: happy path + SSE ----------------------------------------------

func TestHandleWebWissenArtifactUpload_HappyPath_RendersFragmentAndEmitsCreated(t *testing.T) {
	ts, cookie, _, _, bus := newWebArtifactsServer(t)
	ch, cancel := bus.Subscribe("u1")
	defer cancel()

	body, ct := multipartUpload(t, "Diagram.png", pngPixelBytes(t), "")
	res := postNMultipart(t, ts, cookie, "/wissen/artefakte", ct, body)
	got := readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%.400s", res.StatusCode, got)
	}
	if !strings.Contains(got, `class="artcard"`) || !strings.Contains(got, "Diagram.png") {
		t.Fatalf("response must be the re-rendered fragment with the new card:\n%.800s", got)
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

// --- Rename -----------------------------------------------------------

func TestHandleWebWissenArtifactRename_NameChangesSlugRefStable_EmitsUpdated(t *testing.T) {
	ts, cookie, _, as, bus := newWebArtifactsServer(t)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: "u1", NodeID: "", Slug: "logo", Name: "Old.png", Mime: "image/png", Ref: "abc123def456",
	}); err != nil {
		t.Fatal(err)
	}
	ch, cancel := bus.Subscribe("u1")
	defer cancel()

	res := postN(t, ts, cookie, "/wissen/artefakte/logo/rename", url.Values{"name": {"New Name.png"}})
	got := readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%.400s", res.StatusCode, got)
	}
	if !strings.Contains(got, "New Name.png") {
		t.Fatalf("fragment must show the new name:\n%.800s", got)
	}
	updated, err := as.GetMeta(context.Background(), "u1", "", "logo")
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

func TestHandleWebWissenArtifactDelete_RemovesAndEmitsDeleted(t *testing.T) {
	ts, cookie, _, as, bus := newWebArtifactsServer(t)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: "u1", NodeID: "", Slug: "logo", Name: "Logo.png", Mime: "image/png",
	}); err != nil {
		t.Fatal(err)
	}
	ch, cancel := bus.Subscribe("u1")
	defer cancel()

	res := postN(t, ts, cookie, "/wissen/artefakte/logo/delete", url.Values{})
	got := readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%.400s", res.StatusCode, got)
	}
	if strings.Contains(got, "Logo.png") {
		t.Fatalf("deleted artifact must not appear in the re-rendered fragment:\n%.800s", got)
	}
	if _, err := as.Get(context.Background(), "u1", "", "logo"); err == nil {
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

// TestHandleWebWissenArtifactRename_CrossTenant_NoEffect is the mandatory
// owner-scope negative test for rename: a foreign owner's free artifact at
// the same slug must survive untouched, and ErrArtifactNotFound must stay
// silent (no 500, no popup).
func TestHandleWebWissenArtifactRename_CrossTenant_NoEffect(t *testing.T) {
	ts, cookie, _, as, _ := newWebArtifactsServer(t)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: "other-owner", NodeID: "", Slug: "logo", Name: "Foreign.png", Mime: "image/png",
	}); err != nil {
		t.Fatal(err)
	}

	res := postN(t, ts, cookie, "/wissen/artefakte/logo/rename", url.Values{"name": {"Hijacked.png"}})
	_ = readBody(t, res)
	if res.StatusCode == http.StatusInternalServerError {
		t.Fatalf("cross-tenant rename attempt must not 500")
	}
	still, err := as.GetMeta(context.Background(), "other-owner", "", "logo")
	if err != nil {
		t.Fatalf("foreign owner's free artifact must survive: %v", err)
	}
	if still.Name != "Foreign.png" {
		t.Errorf("foreign owner's free artifact must be untouched, got name=%q", still.Name)
	}
}

// TestHandleWebWissenArtifactDelete_CrossTenant_NoEffect mirrors the rename
// guard for delete: a foreign owner's free artifact must survive.
func TestHandleWebWissenArtifactDelete_CrossTenant_NoEffect(t *testing.T) {
	ts, cookie, _, as, _ := newWebArtifactsServer(t)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: "other-owner", NodeID: "", Slug: "logo", Name: "Foreign.png", Mime: "image/png",
	}); err != nil {
		t.Fatal(err)
	}

	res := postN(t, ts, cookie, "/wissen/artefakte/logo/delete", url.Values{})
	_ = readBody(t, res)
	if res.StatusCode == http.StatusInternalServerError {
		t.Fatalf("cross-tenant delete attempt must not 500")
	}
	if _, err := as.Get(context.Background(), "other-owner", "", "logo"); err != nil {
		t.Error("foreign owner's free artifact must survive an unrelated tenant's delete attempt")
	}
}

// TestHandleWebWissenArtifacts_GET_OwnAndForeignScoping verifies GET
// /wissen/artefakte only ever surfaces the CALLER's own free artifacts —
// a foreign owner's free artifact must never appear.
func TestHandleWebWissenArtifacts_GET_OwnAndForeignScoping(t *testing.T) {
	ts, cookie, _, as, _ := newWebArtifactsServer(t)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: "u1", NodeID: "", Slug: "mine", Name: "Mine.pdf", Mime: "application/pdf",
	}); err != nil {
		t.Fatal(err)
	}
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: "other-owner", NodeID: "", Slug: "theirs", Name: "Theirs.pdf", Mime: "application/pdf",
	}); err != nil {
		t.Fatal(err)
	}

	res := getWith(t, ts, cookie, "/wissen/artefakte")
	got := readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%.400s", res.StatusCode, got)
	}
	if !strings.Contains(got, "Mine.pdf") {
		t.Fatalf("own free artifact must appear:\n%.800s", got)
	}
	if strings.Contains(got, "Theirs.pdf") {
		t.Fatalf("foreign owner's free artifact must NOT appear:\n%.800s", got)
	}
}

// --- Error paths: inline, never a 500, never a popup ----------------------

func TestHandleWebWissenArtifactUpload_TooLarge_InlineErrorNo500(t *testing.T) {
	ts, cookie, _, as, _ := newWebArtifactsServer(t)

	oversized := make([]byte, domain.MaxArtifactBytes+1)
	body, ct := multipartUpload(t, "huge.bin", oversized, "")
	res := postNMultipart(t, ts, cookie, "/wissen/artefakte", ct, body)
	got := readBody(t, res)
	if res.StatusCode == http.StatusInternalServerError {
		t.Fatalf("must not 500 on an oversized upload; body=%.400s", got)
	}
	if !strings.Contains(got, "role=\"alert\"") {
		t.Fatalf("oversized upload must surface an inline role=alert error:\n%.800s", got)
	}
	list, _ := as.ListFree(context.Background(), "u1")
	if len(list) != 0 {
		t.Errorf("no free artifact should have been stored for an oversized upload: %+v", list)
	}
}

func TestHandleWebWissenArtifactUpload_QuotaExceeded_InlineErrorNo500(t *testing.T) {
	ts, cookie, _, as, _ := newWebArtifactsServer(t)
	if err := as.Put(context.Background(), domain.Artifact{
		OwnerID: "u1", NodeID: "", Slug: "existing", Name: "existing.pdf",
		Mime: "application/pdf", SizeBytes: usecase.MaxArtifactBytesPerOwner - 4,
	}); err != nil {
		t.Fatal(err)
	}

	body, ct := multipartUpload(t, "report.pdf", []byte("%PDF-1.4 mock content overflowing quota"), "")
	res := postNMultipart(t, ts, cookie, "/wissen/artefakte", ct, body)
	got := readBody(t, res)
	if res.StatusCode == http.StatusInternalServerError {
		t.Fatalf("must not 500 on quota exceeded; body=%.400s", got)
	}
	if !strings.Contains(got, "role=\"alert\"") {
		t.Fatalf("quota exceeded must surface an inline role=alert error:\n%.800s", got)
	}
	list, _ := as.ListFree(context.Background(), "u1")
	if len(list) != 1 {
		t.Errorf("quota-exceeded upload must not add a second free artifact: %+v", list)
	}
}

// getWith performs an authenticated GET against the test server, following
// redirects (none expected here) — mirrors postN's cookie/client wiring
// (webui_nodes_test.go) for a plain GET.
func getWith(t *testing.T, ts *httptest.Server, c *http.Cookie, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(c)
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
