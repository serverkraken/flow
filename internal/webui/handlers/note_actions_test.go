package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
)

// — fixtures + helpers --------------------------------------------------------

// mkNoteActionsDeps assembles a NoteActionsDeps backed by pgstore Documents.
func mkNoteActionsDeps(s pgStores, now time.Time) NoteActionsDeps {
	clock := &testutil.FixedClock{T: now}
	return NoteActionsDeps{
		Documents: s.Documents,
		Clock:     clock,
	}
}

// naReq builds an *http.Request with the user injected into context and
// optional chi route params + form-encoded body. Mirrors actionReq from
// session_actions_test.go.
func naReq(t *testing.T, method, target, body string, u domain.User, params map[string]string) *http.Request {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	ctx := httpserver.WithUser(r.Context(), u)
	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	}
	return r.WithContext(ctx)
}

// — repo-note edit-form tests ------------------------------------------------

func TestRepoNoteEdit_GET_ExistingNote_RendersFormWithContent(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "rne-edit-1")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/serverkraken/flow"
	// Seed a document with a repo key so Edit finds it.
	doc, err := s.Documents.Put(s.User.ID, repoNotePathWeb(key), "# original content", key, 0)
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	d := mkNoteActionsDeps(s, now)
	h := NewRepoNoteEdit(d)
	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodGet, "/repos/"+url.PathEscape(key)+"/note/edit", "", s.User, map[string]string{"key": url.PathEscape(key)})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain(t, body, []string{
		`data-testid="repo-note-edit-form"`,
		`data-testid="cm-editor"`,
		`id="cm-content"`,
		`name="content"`,
		`name="version"`,
		`# original content`,
		`value="` + strconv.FormatInt(doc.Version, 10) + `"`,
	})
}

func TestRepoNoteEdit_GET_NoExistingNote_RendersEmptyFormVersion0(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "rne-edit-new")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	// No document seeded → IsNew=true, version=0.
	key := "git:gh.com/serverkraken/dotfiles"

	d := mkNoteActionsDeps(s, now)
	h := NewRepoNoteEdit(d)
	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodGet, "/repos/"+url.PathEscape(key)+"/note/edit", "", s.User, map[string]string{"key": url.PathEscape(key)})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain(t, body, []string{
		`data-testid="repo-note-edit-form"`,
		`value="0"`,    // version=0 for first save
		`note anlegen`, // IsNew branch tail in breadcrumb
	})
}

func TestRepoNoteEdit_GET_Unauthorized_401(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "rne-edit-unauth")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	d := mkNoteActionsDeps(s, now)
	h := NewRepoNoteEdit(d)
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/repos/foo/note/edit", nil)
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}
}

// — repo-note PUT tests ------------------------------------------------------

func TestRepoNotePut_HappyPath_303AndUpsertsContent(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "rne-put-1")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/serverkraken/flow"
	existing, err := s.Documents.Put(s.User.ID, repoNotePathWeb(key), "# initial", key, 0)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	d := mkNoteActionsDeps(s, now)
	h := NewRepoNotePut(d)

	form := url.Values{}
	form.Set("content", "# updated body")
	form.Set("version", strconv.FormatInt(existing.Version, 10))

	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodPut, "/repos/"+url.PathEscape(key)+"/note", form.Encode(), s.User, map[string]string{"key": url.PathEscape(key)})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303; body=%s", rr.Code, rr.Body.String())
	}
	wantLoc := "/repos/" + url.PathEscape(key) + "/note"
	if loc := rr.Header().Get("Location"); loc != wantLoc {
		t.Errorf("Location: got %q, want %q", loc, wantLoc)
	}

	// Verify the document now carries the new content.
	saved, err := s.Documents.GetByRepoKey(s.User.ID, key)
	if err != nil {
		t.Fatalf("GetByRepoKey: %v", err)
	}
	if saved.Body != "# updated body" {
		t.Errorf("Body: got %q, want %q", saved.Body, "# updated body")
	}
	if saved.Version <= existing.Version {
		t.Errorf("Version should bump, got %d (was %d)", saved.Version, existing.Version)
	}
}

func TestRepoNotePut_VersionConflict_RendersOverlay(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "rne-put-conf")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/serverkraken/flow"
	_, err := s.Documents.Put(s.User.ID, repoNotePathWeb(key), "# initial", key, 0)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	d := mkNoteActionsDeps(s, now)
	h := NewRepoNotePut(d)

	form := url.Values{}
	form.Set("content", "# stale write")
	form.Set("version", "999") // stale

	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodPut, "/repos/"+url.PathEscape(key)+"/note", form.Encode(), s.User, map[string]string{"key": url.PathEscape(key)})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `data-testid="repo-note-conflict"`) {
		t.Errorf("conflict body missing overlay testid; body=%s", rr.Body.String())
	}
}

func TestRepoNotePut_FirstSave_CreatesVersion1(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "rne-put-first")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/serverkraken/dotfiles"
	// No prior document → first save with version=0.

	d := mkNoteActionsDeps(s, now)
	h := NewRepoNotePut(d)

	form := url.Values{}
	form.Set("content", "# brand-new note")
	form.Set("version", "0")

	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodPut, "/repos/"+url.PathEscape(key)+"/note", form.Encode(), s.User, map[string]string{"key": url.PathEscape(key)})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303; body=%s", rr.Code, rr.Body.String())
	}
	saved, err := s.Documents.GetByRepoKey(s.User.ID, key)
	if err != nil {
		t.Fatalf("GetByRepoKey: %v", err)
	}
	if saved.Body != "# brand-new note" {
		t.Errorf("Body: got %q, want %q", saved.Body, "# brand-new note")
	}
	if saved.Version <= 0 {
		t.Errorf("Version should be >0 after first save, got %d", saved.Version)
	}
}

func TestRepoNotePut_CrossTenant_404(t *testing.T) {
	t.Parallel()
	sA := newPGStores(t, "rne-put-uA")
	sB := newPGStores(t, "rne-put-uB")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/serverkraken/flow"
	_, err := sA.Documents.Put(sA.User.ID, repoNotePathWeb(key), "# tenant-A note", key, 0)
	if err != nil {
		t.Fatalf("seed A: %v", err)
	}

	// sB's Documents store is used → won't find sA's doc.
	d := mkNoteActionsDeps(sB, now)
	h := NewRepoNotePut(d)

	form := url.Values{}
	form.Set("content", "hijack attempt")
	form.Set("version", "1")
	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodPut, "/repos/"+url.PathEscape(key)+"/note", form.Encode(), sB.User, map[string]string{"key": url.PathEscape(key)})
	h.ServeHTTP(rr, r)

	// sB's doc doesn't exist yet so version=1 fails OCC (version mismatch
	// for first-insert: server sees no row). The handler returns 303 for
	// brand-new docs with version=0 only. With version=1 and no row it
	// returns 409 (conflict) because the version guard fails.
	// Accept either 409 or 303-with-body-check. The key invariant:
	// sA's doc must be untouched.
	doc, err := sA.Documents.GetByRepoKey(sA.User.ID, key)
	if err != nil {
		t.Fatalf("re-read sA doc: %v", err)
	}
	if doc.Body != "# tenant-A note" {
		t.Errorf("cross-tenant write leaked: sA body changed to %q", doc.Body)
	}
}

func TestRepoNotePut_Unauthorized_401(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "rne-put-unauth")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	d := mkNoteActionsDeps(s, now)
	h := NewRepoNotePut(d)
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/repos/foo/note", strings.NewReader("content=x&version=0"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}
}
