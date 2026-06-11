package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sqliteserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/kompendium/adapter/fsstore"
	kompdomain "github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/webui/handlers"
)

// — fixtures + helpers --------------------------------------------------------

// mkNoteActionsDeps assembles a NoteActionsDeps off an optional
// notebook root. Pass root="" for the "kompendium unconfigured" branch
// (NoteStore stays nil).
func mkNoteActionsDeps(t *testing.T, _ *sqliteserver.Store, root string, now time.Time) handlers.NoteActionsDeps {
	t.Helper()
	clock := &testutil.FixedClock{T: now}
	deps := handlers.NoteActionsDeps{
		Documents: nil,
		Clock:     clock,
	}
	if root != "" {
		ns, err := fsstore.New(root)
		if err != nil {
			t.Fatalf("fsstore.New: %v", err)
		}
		deps.NoteStore = ns
	}
	return deps
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

// seedKompNotebook writes a single kompendium note under a fresh temp
// root and returns the root path + the ID of the seeded note. The note
// body intentionally includes a recognisable substring so PUT assertions
// can verify the disk content changed.
func seedKompNotebook(t *testing.T) (root string, id kompdomain.ID) {
	t.Helper()
	root = t.TempDir()
	rel := "notes/setup.md"
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `---
id: notes/setup
type: free
date: 2026-06-01
title: setup walkthrough
tags: [setup]
---
# setup walkthrough

Original body for the editing test.
`
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	parsed, err := kompdomain.ParseID("notes/setup")
	if err != nil {
		t.Fatalf("ParseID: %v", err)
	}
	return root, parsed
}

// — kompendium note edit-form tests ------------------------------------------

func TestNoteEdit_GET_RendersFormWithContent(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "ne-edit-1")
	root, id := seedKompNotebook(t)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	d := mkNoteActionsDeps(t, store, root, now)
	h := handlers.NewNoteEdit(d)
	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodGet, "/notes/"+id.String()+"/edit", "", u, map[string]string{"*": id.String() + "/edit"})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain(t, body, []string{
		`data-testid="note-edit-form"`,
		`data-testid="cm-editor"`,
		`id="cm-content"`,
		`name="content"`,
		`Original body for the editing test.`,
	})
}

func TestNoteEdit_GET_NoStore_404(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "ne-edit-noStore")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	d := mkNoteActionsDeps(t, store, "", now)
	h := handlers.NewNoteEdit(d)
	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodGet, "/notes/foo/edit", "", u, map[string]string{"*": "foo/edit"})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rr.Code)
	}
}

func TestNoteEdit_GET_Unauthorized_401(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	root, id := seedKompNotebook(t)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	d := mkNoteActionsDeps(t, store, root, now)
	h := handlers.NewNoteEdit(d)
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/notes/"+id.String()+"/edit", nil)
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}
}

// TestNoteEdit_GET_IDEndingInEdit_NotFound guards the dispatch-collision
// case: the GET /notes/* router hijacks any path ending in /edit and
// forwards it to NewNoteEdit. A kompendium note literally named
// ".../edit" would therefore be unreachable via the view route AND its
// own edit form would resolve to the wrong ID. The handler defensively
// returns 404 when the stripped ID itself still ends in /edit so the
// user does not silently mismatch-dispatch onto a sibling note.
func TestNoteEdit_GET_IDEndingInEdit_NotFound(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "ne-edit-collision")
	root, _ := seedKompNotebook(t)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	d := mkNoteActionsDeps(t, store, root, now)
	h := handlers.NewNoteEdit(d)
	rr := httptest.NewRecorder()
	// chi captures "some/path/edit/edit" — after the trailing /edit
	// strip, the residual ID still ends in /edit → handler returns 404.
	r := naReq(t, http.MethodGet, "/notes/some/path/edit/edit", "", u, map[string]string{"*": "some/path/edit/edit"})
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rr.Code)
	}
}

// — kompendium note PUT tests ------------------------------------------------

func TestNotePut_HappyPath_303AndWritesContent(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "ne-put-1")
	root, id := seedKompNotebook(t)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	d := mkNoteActionsDeps(t, store, root, now)
	h := handlers.NewNotePut(d)

	form := url.Values{}
	form.Set("content", "Updated body via WebUI.")
	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodPut, "/notes/"+id.String(), form.Encode(), u, map[string]string{"*": id.String()})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303; body=%s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/notes/"+id.String() {
		t.Errorf("Location: got %q, want /notes/%s", loc, id.String())
	}
	// Verify the file on disk now carries the new content via the store.
	ns, err := fsstore.New(root)
	if err != nil {
		t.Fatalf("fsstore.New: %v", err)
	}
	updated, err := ns.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get after put: %v", err)
	}
	if !strings.Contains(string(updated.Body), "Updated body via WebUI.") {
		t.Errorf("file body missing update; got=%s", string(updated.Body))
	}
}

func TestNotePut_NoStore_404(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "ne-put-noStore")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	d := mkNoteActionsDeps(t, store, "", now)
	h := handlers.NewNotePut(d)
	form := url.Values{}
	form.Set("content", "anything")

	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodPut, "/notes/foo", form.Encode(), u, map[string]string{"*": "foo"})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rr.Code)
	}
}

func TestNotePut_Unauthorized_401(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	root, id := seedKompNotebook(t)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	d := mkNoteActionsDeps(t, store, root, now)
	h := handlers.NewNotePut(d)
	form := url.Values{}
	form.Set("content", "anything")
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/notes/"+id.String(), strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}
}

// — repo-note edit-form tests ------------------------------------------------

func TestRepoNoteEdit_GET_ExistingNote_RendersFormWithContent(t *testing.T) {
	t.Skip("R1: wird in Task 19 auf pgstore/documents umgezogen")
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "rne-edit-1")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/serverkraken/flow"
	repo := seedRepo(t, store, u.ID, key, "flow")
	note := seedRepoNote(t, store, u.ID, repo.ID, "# original content")

	d := mkNoteActionsDeps(t, store, "", now)
	h := handlers.NewRepoNoteEdit(d)
	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodGet, "/repos/"+url.PathEscape(key)+"/note/edit", "", u, map[string]string{"key": url.PathEscape(key)})
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
		`value="` + strconv.FormatInt(note.Version, 10) + `"`,
	})
}

func TestRepoNoteEdit_GET_NoExistingNote_RendersEmptyFormVersion0(t *testing.T) {
	t.Skip("R1: wird in Task 19 auf pgstore/documents umgezogen")
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "rne-edit-new")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/serverkraken/dotfiles"
	_ = seedRepo(t, store, u.ID, key, "dotfiles")

	d := mkNoteActionsDeps(t, store, "", now)
	h := handlers.NewRepoNoteEdit(d)
	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodGet, "/repos/"+url.PathEscape(key)+"/note/edit", "", u, map[string]string{"key": url.PathEscape(key)})
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

func TestRepoNoteEdit_GET_UnknownRepo_404(t *testing.T) {
	t.Skip("R1: wird in Task 19 auf pgstore/documents umgezogen")
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "rne-edit-404")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	d := mkNoteActionsDeps(t, store, "", now)
	h := handlers.NewRepoNoteEdit(d)
	rr := httptest.NewRecorder()
	key := "git:gh.com/missing/repo"
	r := naReq(t, http.MethodGet, "/repos/"+url.PathEscape(key)+"/note/edit", "", u, map[string]string{"key": url.PathEscape(key)})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rr.Code)
	}
}

func TestRepoNoteEdit_GET_Unauthorized_401(t *testing.T) {
	t.Skip("R1: wird in Task 19 auf pgstore/documents umgezogen")
	t.Parallel()
	store := mustOpenServerStore(t)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	d := mkNoteActionsDeps(t, store, "", now)
	h := handlers.NewRepoNoteEdit(d)
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/repos/foo/note/edit", nil)
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}
}

// — repo-note PUT tests ------------------------------------------------------

func TestRepoNotePut_HappyPath_303AndUpsertsContent(t *testing.T) {
	t.Skip("R1: wird in Task 19 auf pgstore/documents umgezogen")
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "rne-put-1")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/serverkraken/flow"
	repo := seedRepo(t, store, u.ID, key, "flow")
	existing := seedRepoNote(t, store, u.ID, repo.ID, "# initial")

	d := mkNoteActionsDeps(t, store, "", now)
	h := handlers.NewRepoNotePut(d)

	form := url.Values{}
	form.Set("content", "# updated body")
	form.Set("version", strconv.FormatInt(existing.Version, 10))

	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodPut, "/repos/"+url.PathEscape(key)+"/note", form.Encode(), u, map[string]string{"key": url.PathEscape(key)})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303; body=%s", rr.Code, rr.Body.String())
	}
	wantLoc := "/repos/" + url.PathEscape(key) + "/note"
	if loc := rr.Header().Get("Location"); loc != wantLoc {
		t.Errorf("Location: got %q, want %q", loc, wantLoc)
	}

	// Verify the row is now at the new content + a bumped version.
	saved, err := sqliteserver.NewRepoNotes(store).GetByRepo(u.ID, repo.ID)
	if err != nil {
		t.Fatalf("GetByRepo: %v", err)
	}
	if saved.Content != "# updated body" {
		t.Errorf("Content: got %q, want %q", saved.Content, "# updated body")
	}
	if saved.Version <= existing.Version {
		t.Errorf("Version should bump, got %d (was %d)", saved.Version, existing.Version)
	}
}

func TestRepoNotePut_VersionConflict_RendersOverlay(t *testing.T) {
	t.Skip("R1: wird in Task 19 auf pgstore/documents umgezogen")
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "rne-put-conf")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/serverkraken/flow"
	repo := seedRepo(t, store, u.ID, key, "flow")
	_ = seedRepoNote(t, store, u.ID, repo.ID, "# initial")

	d := mkNoteActionsDeps(t, store, "", now)
	h := handlers.NewRepoNotePut(d)

	form := url.Values{}
	form.Set("content", "# stale write")
	form.Set("version", "999") // stale

	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodPut, "/repos/"+url.PathEscape(key)+"/note", form.Encode(), u, map[string]string{"key": url.PathEscape(key)})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `data-testid="repo-note-conflict"`) {
		t.Errorf("conflict body missing overlay testid; body=%s", rr.Body.String())
	}
}

func TestRepoNotePut_FirstSave_CreatesVersion1(t *testing.T) {
	t.Skip("R1: wird in Task 19 auf pgstore/documents umgezogen")
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "rne-put-first")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/serverkraken/dotfiles"
	repo := seedRepo(t, store, u.ID, key, "dotfiles")

	d := mkNoteActionsDeps(t, store, "", now)
	h := handlers.NewRepoNotePut(d)

	form := url.Values{}
	form.Set("content", "# brand-new note")
	form.Set("version", "0")

	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodPut, "/repos/"+url.PathEscape(key)+"/note", form.Encode(), u, map[string]string{"key": url.PathEscape(key)})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303; body=%s", rr.Code, rr.Body.String())
	}
	saved, err := sqliteserver.NewRepoNotes(store).GetByRepo(u.ID, repo.ID)
	if err != nil {
		t.Fatalf("GetByRepo: %v", err)
	}
	if saved.Content != "# brand-new note" {
		t.Errorf("Content: got %q, want %q", saved.Content, "# brand-new note")
	}
	if saved.Version <= 0 {
		t.Errorf("Version should be >0 after first save, got %d", saved.Version)
	}
}

func TestRepoNotePut_CrossTenant_404(t *testing.T) {
	t.Skip("R1: wird in Task 19 auf pgstore/documents umgezogen")
	t.Parallel()
	store := mustOpenServerStore(t)
	uA := seedUser(t, store, "rne-put-uA")
	uB := seedUser(t, store, "rne-put-uB")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/serverkraken/flow"
	repoA := seedRepo(t, store, uA.ID, key, "flow")
	_ = seedRepoNote(t, store, uA.ID, repoA.ID, "# tenant-A note")

	d := mkNoteActionsDeps(t, store, "", now)
	h := handlers.NewRepoNotePut(d)

	form := url.Values{}
	form.Set("content", "hijack attempt")
	form.Set("version", "1")
	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodPut, "/repos/"+url.PathEscape(key)+"/note", form.Encode(), uB, map[string]string{"key": url.PathEscape(key)})
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusNotFound {
		t.Errorf("cross-tenant put: got %d, want 404", rr.Code)
	}
}

func TestRepoNotePut_Unauthorized_401(t *testing.T) {
	t.Skip("R1: wird in Task 19 auf pgstore/documents umgezogen")
	t.Parallel()
	store := mustOpenServerStore(t)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	d := mkNoteActionsDeps(t, store, "", now)
	h := handlers.NewRepoNotePut(d)
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/repos/foo/note", strings.NewReader("content=x&version=0"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}
}
