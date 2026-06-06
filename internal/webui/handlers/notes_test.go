package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/kompendium/adapter/fsstore"
	kompusecase "github.com/serverkraken/flow/internal/kompendium/usecase"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/webui/handlers"
	"github.com/serverkraken/flow/internal/webui/markdown"
)

// seedNotebook creates a temp notebook with a deterministic mix of
// daily / project / free notes so the index + filter tests run against
// a known shape. Returns the absolute root path.
func seedNotebook(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeNote := func(rel, content string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	writeNote("daily/2026-06-04.md", `---
id: daily/2026-06-04
type: daily
date: 2026-06-04
title: Donnerstag · 04.06.
tags: [review]
---
# Donnerstag · 04.06.

Plan E started today with the dashboard route. Worktime followed in the afternoon — both are merged on next.
`)
	writeNote("daily/2026-06-05.md", `---
id: daily/2026-06-05
type: daily
date: 2026-06-05
title: Freitag
tags: []
---
# Freitag

Notes route shipped. Index + view + markdown renderer.
`)
	writeNote("projects/serverkraken/flow/phase-1-review.md", `---
id: projects/serverkraken/flow/phase-1-review
type: project
project: github.com/serverkraken/flow
date: 2026-06-04
title: flow phase 1 review
tags: [flow, review]
---
# flow phase 1 review

Search-target-string: webui-mockups should match. Backlinks are a Phase 2 feature.
`)
	writeNote("notes/setup.md", `---
id: notes/setup
type: free
date: 2026-06-01
title: setup walkthrough
tags: [setup]
---
# setup walkthrough

Hairlines, tabular nums, sharp corners. The editorial terminal aesthetic.
`)
	return root
}

func mkNotesDeps(t *testing.T, root string, now time.Time) handlers.NotesDeps {
	t.Helper()
	clock := &testutil.FixedClock{T: now}
	if root == "" {
		return handlers.NotesDeps{
			Store:    nil,
			Lister:   nil,
			Markdown: markdown.New(),
			Clock:    clock,
		}
	}
	store, err := fsstore.New(root)
	if err != nil {
		t.Fatalf("fsstore.New: %v", err)
	}
	return handlers.NotesDeps{
		Store:    store,
		Lister:   kompusecase.NewListNotes(store),
		Markdown: markdown.New(),
		Clock:    clock,
	}
}

func notesReqWithUser(t *testing.T, target string, u domain.User) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	return r.WithContext(httpserver.WithUser(r.Context(), u))
}

// — index tests —

func TestNotesIndex_TypeAlle_ListsAllNotes(t *testing.T) {
	t.Parallel()
	root := seedNotebook(t)
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "notes-alle")
	now := time.Date(2026, 6, 5, 14, 0, 0, 0, time.UTC)

	h := handlers.NewNotes(mkNotesDeps(t, root, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, notesReqWithUser(t, "/notes?type=alle", u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain := []string{
		"Freitag",                       // daily title
		"flow phase 1 review",           // project title
		"setup walkthrough",             // free title
		"4 Notes",                       // total label
		"data-testid=\"note-list\"",   // list rendered (not empty)
		`class="subtab is-active"`,      // active sub-tab
	}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("alle body missing %q", s)
		}
	}
}

func TestNotesIndex_TypeDaily_FiltersToDailyOnly(t *testing.T) {
	t.Parallel()
	root := seedNotebook(t)
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "notes-daily")
	now := time.Date(2026, 6, 5, 14, 0, 0, 0, time.UTC)

	h := handlers.NewNotes(mkNotesDeps(t, root, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, notesReqWithUser(t, "/notes?type=daily", u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Freitag") {
		t.Errorf("daily body missing Freitag")
	}
	if strings.Contains(body, "flow phase 1 review") {
		t.Errorf("daily body leaked project note title")
	}
	if strings.Contains(body, "setup walkthrough") {
		t.Errorf("daily body leaked free note title")
	}
	if !strings.Contains(body, "2 Notes") {
		t.Errorf("expected '2 Notes' count; body=%s", body)
	}
}

func TestNotesIndex_QueryFiltersList(t *testing.T) {
	t.Parallel()
	root := seedNotebook(t)
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "notes-search")
	now := time.Date(2026, 6, 5, 14, 0, 0, 0, time.UTC)

	h := handlers.NewNotes(mkNotesDeps(t, root, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, notesReqWithUser(t, "/notes?type=alle&q=phase-1-review", u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "flow phase 1 review") {
		t.Errorf("search hit missing target row")
	}
	if strings.Contains(body, "setup walkthrough") {
		t.Errorf("search leaked unrelated row")
	}
}

func TestNotesIndex_StoreNil_RendersPlaceholder(t *testing.T) {
	t.Parallel()
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "notes-notcfg")
	now := time.Date(2026, 6, 5, 14, 0, 0, 0, time.UTC)

	h := handlers.NewNotes(mkNotesDeps(t, "", now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, notesReqWithUser(t, "/notes", u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "data-testid=\"notes-not-configured\"") {
		t.Errorf("not-configured placeholder missing")
	}
	if !strings.Contains(body, "FLOW_NOTEBOOK_ROOT") {
		t.Errorf("env-var hint missing")
	}
}

func TestNotesIndex_InvalidTabFallsThroughToAlle(t *testing.T) {
	t.Parallel()
	root := seedNotebook(t)
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "notes-invtab")
	now := time.Date(2026, 6, 5, 14, 0, 0, 0, time.UTC)

	h := handlers.NewNotes(mkNotesDeps(t, root, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, notesReqWithUser(t, "/notes?type=bogus", u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; got body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "4 Notes") {
		t.Errorf("invalid tab should fall through to alle (4 notes); body=%s", rr.Body.String())
	}
}

// — view tests —

func TestNotesView_ValidID_RendersTitleAndBody(t *testing.T) {
	t.Parallel()
	root := seedNotebook(t)
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "notes-view-valid")
	now := time.Date(2026, 6, 5, 14, 0, 0, 0, time.UTC)

	h := handlers.NewNotes(mkNotesDeps(t, root, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, notesReqWithUser(t, "/notes/projects/serverkraken/flow/phase-1-review", u))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	mustContain := []string{
		"flow phase 1 review",                  // title
		"<article",                             // prose-flow article
		"data-testid=\"notes-article\"",        // article test marker
		"data-testid=\"notes-breadcrumb\"",     // breadcrumb test marker
		"Project",                              // type label in breadcrumb
		"Phase 2 · Indexer",                    // backlinks placeholder
		"Metadaten",                            // rail head
	}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("view body missing %q", s)
		}
	}
}

func TestNotesView_NonexistentID_Returns404(t *testing.T) {
	t.Parallel()
	root := seedNotebook(t)
	store := mustOpenServerStore(t)
	u := seedUser(t, store, "notes-view-404")
	now := time.Date(2026, 6, 5, 14, 0, 0, 0, time.UTC)

	h := handlers.NewNotes(mkNotesDeps(t, root, now))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, notesReqWithUser(t, "/notes/does/not/exist", u))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "data-testid=\"notes-not-found\"") {
		t.Errorf("404 helper template missing")
	}
	if !strings.Contains(body, "does/not/exist") {
		t.Errorf("404 body should echo requested ID")
	}
}

// — auth tests —

func TestNotes_MissingUser_Returns401(t *testing.T) {
	t.Parallel()
	root := seedNotebook(t)
	now := time.Date(2026, 6, 5, 14, 0, 0, 0, time.UTC)

	h := handlers.NewNotes(mkNotesDeps(t, root, now))
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/notes", nil).WithContext(context.Background())
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("missing user: got %d, want 401", rr.Code)
	}
}
