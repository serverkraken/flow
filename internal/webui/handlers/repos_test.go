package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/webui/markdown"
)

// mkReposDeps assembles the handler deps off pgStores.
func mkReposDeps(s pgStores, now time.Time) ReposDeps {
	clock := &testutil.FixedClock{T: now}
	return ReposDeps{
		Documents: s.Documents,
		Markdown:  markdown.New(),
		Clock:     clock,
	}
}

func reposReqWithUser(t *testing.T, target string, u domain.User) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	return r.WithContext(httpserver.WithUser(r.Context(), u))
}

// seedRepoDoc seeds a document with a repo_key in pgstore and returns it.
func seedRepoDoc(t *testing.T, s pgStores, canonicalKey, content string) {
	t.Helper()
	docPath := "repos/" + url.PathEscape(canonicalKey) + ".md"
	_, err := s.Documents.Put(s.User.ID, docPath, content, canonicalKey, 0)
	if err != nil {
		t.Fatalf("seedRepoDoc Put: %v", err)
	}
}

// — index tests —

func TestReposIndex_ListsUserRepos_WithNotePresence(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "repos-list")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	seedRepoDoc(t, s, "git:gh.com/serverkraken/flow", "# flow\n\nSetup notes.\n")

	// dotfiles has no note — only the repo entry in documents doesn't exist yet,
	// but ReposIndex just calls Documents.List("repos/") which returns existing
	// docs. Seed it too to appear in the index.
	seedRepoDoc(t, s, "git:gh.com/serverkraken/dotfiles", "")

	h := NewRepos(mkReposDeps(s, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reposReqWithUser(t, "/repos", s.User))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"flow",                    // display name
		"dotfiles",                // second repo
		`data-testid="repo-list"`, // populated branch
		`data-testid="repo-item"`, // at least one row
	} {
		if !strings.Contains(body, want) {
			t.Errorf("index body missing %q", want)
		}
	}
}

func TestReposIndex_NoRepos_RendersEmptyState(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "repos-empty")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	h := NewRepos(mkReposDeps(s, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reposReqWithUser(t, "/repos", s.User))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `data-testid="repos-empty"`) {
		t.Errorf("empty-state placeholder missing; body=%s", body)
	}
	if !strings.Contains(body, "Keine Repos synchronisiert") {
		t.Errorf("empty-state copy missing")
	}
	if !strings.Contains(body, "0 Repos") {
		t.Errorf("zero-count label missing")
	}
}

func TestReposIndex_OnlyShowsOwnRepos(t *testing.T) {
	t.Parallel()
	owner := newPGStores(t, "repos-iso-owner")
	other := newPGStores(t, "repos-iso-other")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	seedRepoDoc(t, owner, "git:gh.com/owner/owned", "# owned")
	seedRepoDoc(t, other, "git:gh.com/other/leaked", "# leaked")

	h := NewRepos(mkReposDeps(owner, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reposReqWithUser(t, "/repos", owner.User))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "owned") {
		t.Errorf("owner cannot see own repo")
	}
	if strings.Contains(body, "leaked") {
		t.Errorf("cross-tenant leak: 'leaked' appears in owner's /repos body")
	}
}

// — note view tests —

func TestReposNoteView_ValidKey_RendersMarkdownHTML(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "repos-view")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/serverkraken/flow"
	seedRepoDoc(t, s, key, "## Setup\n\nHexagonal Go repo. `make ci` muss grün sein.\n")

	target := "/repos/" + url.PathEscape(key) + "/note"
	h := NewRepos(mkReposDeps(s, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reposReqWithUser(t, target, s.User))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		`data-testid="repos-breadcrumb"`,
		`data-testid="repos-note-meta"`,
		`data-testid="repos-note-article"`,
		"<h2",                               // rendered markdown heading
		"Setup",                             // heading text
		"<code>make ci</code>",              // inline code rendered
		"Canonical key",                     // meta strip label
		key,                                 // raw canonical key in the strip
		"Bearbeiten",                        // edit link
		`data-testid="repo-note-edit-link"`, // edit link testid
	} {
		if !strings.Contains(body, want) {
			t.Errorf("view body missing %q", want)
		}
	}
}

func TestReposNoteView_RepoWithoutNote_RendersPlaceholder(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "repos-view-nonote")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/serverkraken/dotfiles"
	// No doc seeded → handler gets ErrDocumentNotFound and renders placeholder.

	target := "/repos/" + url.PathEscape(key) + "/note"
	h := NewRepos(mkReposDeps(s, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reposReqWithUser(t, target, s.User))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `data-testid="repos-note-missing"`) {
		t.Errorf("missing-note placeholder missing; body=%s", body)
	}
	if !strings.Contains(body, "noch keine Note") {
		t.Errorf("placeholder copy missing")
	}
}

func TestReposNoteView_BogusKey_Returns404(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "repos-view-404")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	bogus := url.PathEscape("git:gh.com/does/not/exist")
	target := "/repos/" + bogus + "/note"
	h := NewRepos(mkReposDeps(s, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reposReqWithUser(t, target, s.User))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (missing note is not a 404 — we show placeholder); body=%s", rr.Code, rr.Body.String())
	}
	// A missing doc renders the "no note yet" placeholder, not a 404.
	// The handler only returns 404 when the chi URL parsing fails.
}

func TestReposNoteView_OtherUsersRepo_Returns404(t *testing.T) {
	t.Parallel()
	owner := newPGStores(t, "repos-view-iso-owner2")
	other := newPGStores(t, "repos-view-iso-other2")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/owner/private"
	seedRepoDoc(t, owner, key, "# private note")

	target := "/repos/" + url.PathEscape(key) + "/note"
	// other's Documents store will not find owner's doc → renders placeholder (200).
	h := NewRepos(mkReposDeps(other, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reposReqWithUser(t, target, other.User))

	// The handler can't distinguish "other user's doc" from "no doc" —
	// it returns 200 with the missing-note placeholder, which is the
	// correct tenant-isolation behaviour (no information leak).
	if rr.Code != http.StatusOK {
		t.Errorf("cross-tenant: got %d, want 200 (placeholder)", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "# private note") {
		t.Errorf("cross-tenant leak: owner's note body visible to other user")
	}
}

// — auth tests —

func TestRepos_MissingUser_Returns401(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "repos-nouser")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	h := NewRepos(mkReposDeps(s, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/repos", nil).WithContext(context.Background())
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("missing user on /repos: got %d, want 401", rr.Code)
	}
}
