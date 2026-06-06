package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sqliteserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/webui/handlers"
	"github.com/serverkraken/flow/internal/webui/markdown"
)

// mkReposDeps assembles the handler deps off a server store. Tests
// reuse mustOpenServerStore + seedUser from dashboard_test.go.
func mkReposDeps(store *sqliteserver.Store, now time.Time) handlers.ReposDeps {
	clock := &testutil.FixedClock{T: now}
	return handlers.ReposDeps{
		Repos:     sqliteserver.NewRepos(store),
		RepoNotes: sqliteserver.NewRepoNotes(store),
		Markdown:  markdown.New(),
		Clock:     clock,
	}
}

func reposReqWithUser(t *testing.T, target string, u domain.User) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	return r.WithContext(httpserver.WithUser(r.Context(), u))
}

func seedRepo(t *testing.T, store *sqliteserver.Store, userID, canonicalKey, displayName string) domain.Repo {
	t.Helper()
	repos := sqliteserver.NewRepos(store)
	r, err := repos.EnsureByCanonicalKey(userID, canonicalKey, displayName)
	if err != nil {
		t.Fatalf("EnsureByCanonicalKey: %v", err)
	}
	// Bump version so the row has a non-zero Version (the "version N"
	// meta cell exercises the populated branch).
	r2, err := repos.Upsert(domain.Repo{
		ID: r.ID, UserID: userID, CanonicalKey: canonicalKey, DisplayName: displayName,
	}, r.Version)
	if err != nil {
		t.Fatalf("Upsert (bump): %v", err)
	}
	return r2
}

func seedRepoNote(t *testing.T, store *sqliteserver.Store, userID, repoID, content string) domain.RepoNote {
	t.Helper()
	notes := sqliteserver.NewRepoNotes(store)
	n, err := notes.Upsert(domain.RepoNote{
		ID:      uuid.NewString(),
		RepoID:  repoID,
		UserID:  userID,
		Content: content,
	}, 0)
	if err != nil {
		t.Fatalf("RepoNotes.Upsert: %v", err)
	}
	return n
}

// — index tests —

func TestReposIndex_ListsUserRepos_WithNotePresence(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "repos-list")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	withNote := seedRepo(t, store, u.ID, "git:gh.com/serverkraken/flow", "flow")
	seedRepoNote(t, store, u.ID, withNote.ID, "# flow\n\nSetup notes.\n")
	_ = seedRepo(t, store, u.ID, "git:gh.com/serverkraken/dotfiles", "dotfiles")

	h := handlers.NewRepos(mkReposDeps(store, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reposReqWithUser(t, "/repos", u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain := []string{
		"flow",                          // display name
		"dotfiles",                      // second repo
		"2 Repos · 1 mit Notes",         // total label
		`data-testid="repo-list"`,       // populated branch
		`data-testid="repo-item"`,       // at least one row
		"note ✓",                        // note-presence marker
		"git@gh.com:serverkraken/flow",  // SSH-style remote display
	}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("index body missing %q", s)
		}
	}
}

func TestReposIndex_NoRepos_RendersEmptyState(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "repos-empty")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	h := handlers.NewRepos(mkReposDeps(store, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reposReqWithUser(t, "/repos", u))

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
	if !strings.Contains(body, "0 Repos · 0 mit Notes") {
		t.Errorf("zero-count label missing")
	}
}

func TestReposIndex_OnlyShowsOwnRepos(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	owner := seedUser(t, store, "repos-iso-owner")
	other := seedUser(t, store, "repos-iso-other")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	seedRepo(t, store, owner.ID, "git:gh.com/owner/owned", "owned")
	seedRepo(t, store, other.ID, "git:gh.com/other/leaked", "leaked")

	h := handlers.NewRepos(mkReposDeps(store, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reposReqWithUser(t, "/repos", owner))

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
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "repos-view")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/serverkraken/flow"
	repo := seedRepo(t, store, u.ID, key, "flow")
	seedRepoNote(t, store, u.ID, repo.ID, "## Setup\n\nHexagonal Go repo. `make ci` muss grün sein.\n")

	target := "/repos/" + url.PathEscape(key) + "/note"
	h := handlers.NewRepos(mkReposDeps(store, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reposReqWithUser(t, target, u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain := []string{
		`data-testid="repos-breadcrumb"`,
		`data-testid="repos-note-meta"`,
		`data-testid="repos-note-article"`,
		"<h2",                              // rendered markdown heading
		"Setup",                            // heading text
		"<code>make ci</code>",             // inline code rendered
		"Canonical key",                    // meta strip label
		key,                                // raw canonical key in the strip
		"Bearbeiten",                       // M7 button (disabled)
		"aria-disabled=\"true\"",           // M7 button is disabled
	}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("view body missing %q", s)
		}
	}
}

func TestReposNoteView_RepoWithoutNote_RendersPlaceholder(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "repos-view-nonote")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/serverkraken/dotfiles"
	_ = seedRepo(t, store, u.ID, key, "dotfiles")

	target := "/repos/" + url.PathEscape(key) + "/note"
	h := handlers.NewRepos(mkReposDeps(store, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reposReqWithUser(t, target, u))

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
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "repos-view-404")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	bogus := url.PathEscape("git:gh.com/does/not/exist")
	target := "/repos/" + bogus + "/note"
	h := handlers.NewRepos(mkReposDeps(store, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reposReqWithUser(t, target, u))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `data-testid="repos-not-found"`) {
		t.Errorf("404 placeholder missing")
	}
}

func TestReposNoteView_OtherUsersRepo_Returns404(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	owner := seedUser(t, store, "repos-view-iso-owner")
	other := seedUser(t, store, "repos-view-iso-other")
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	key := "git:gh.com/owner/private"
	_ = seedRepo(t, store, owner.ID, key, "private")

	target := "/repos/" + url.PathEscape(key) + "/note"
	h := handlers.NewRepos(mkReposDeps(store, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reposReqWithUser(t, target, other))

	if rr.Code != http.StatusNotFound {
		t.Errorf("cross-tenant: got %d, want 404", rr.Code)
	}
}

// — auth tests —

func TestRepos_MissingUser_Returns401(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	h := handlers.NewRepos(mkReposDeps(store, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/repos", nil).WithContext(context.Background())
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("missing user on /repos: got %d, want 401", rr.Code)
	}
}
