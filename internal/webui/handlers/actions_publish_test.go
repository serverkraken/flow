package handlers

// actions_publish_test.go — Plan E · Task 14 (M7).
//
// Asserts that the mutating handlers publish the expected SSE events
// when their Deps bag carries a real broadcaster. One test per area
// (session, project, repo-note) — kompendium note publishing is covered
// indirectly via the broadcaster_test.go fan-out tests since the
// kompendium NoteStore is filesystem-backed and gnarlier to seed.
//
// Pattern: subscribe to the broadcaster BEFORE the mutation, run the
// handler, then drain the subscriber channel with a short timeout to
// avoid flaky waits.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/webui/sse"
)

// drainEvent returns the next event delivered on ch or fails the test
// after a short wait. 200ms is generous — the publish runs synchronously
// inside the handler, the channel send is buffered and non-blocking.
func drainEvent(t *testing.T, ch <-chan sse.Event, want string) sse.Event {
	t.Helper()
	select {
	case ev := <-ch:
		if ev.Type != want {
			t.Fatalf("event type: got %q, want %q", ev.Type, want)
		}
		return ev
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("no event delivered within 200ms; want %q", want)
		return sse.Event{}
	}
}

func TestActiveStart_PublishesSessionStarted(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pub-start-1")
	p := seedProject(t, s.Projects, s.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	b := sse.New()

	d := mkActionsDeps(s, now)
	d.Bus = b
	h := NewActiveStart(d)

	ch, cancel := b.Subscribe(s.User.ID)
	defer cancel()

	form := url.Values{}
	form.Set("project_id", p.ID)
	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPost, "/worktime/active/start", form.Encode(), s.User, nil)
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	drainEvent(t, ch, "session.started")
}

func TestActiveStop_PublishesSessionStopped(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pub-stop-1")
	p := seedProject(t, s.Projects, s.User.ID, "webui-mockups")
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	b := sse.New()

	if _, err := s.Active.Start(s.User.ID, p.ID, time.Time{}, "mac", 0, "", ""); err != nil {
		t.Fatalf("seed Start: %v", err)
	}

	d := mkActionsDeps(s, now)
	d.Bus = b
	h := NewActiveStop(d)

	ch, cancel := b.Subscribe(s.User.ID)
	defer cancel()

	rr := httptest.NewRecorder()
	r := actionReq(t, http.MethodPost, "/worktime/active/stop", "", s.User, nil)
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	drainEvent(t, ch, "session.stopped")
}

func TestProjectCreate_PublishesProjectCreated(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pub-proj-create")
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	b := sse.New()

	d := ProjectActionsDeps{
		Projects: s.Projects,
		Clock:    &testutil.FixedClock{T: now},
		Bus:      b,
	}
	h := NewProjectCreate(d)

	ch, cancel := b.Subscribe(s.User.ID)
	defer cancel()

	form := url.Values{}
	form.Set("name", "SSE Smoke")
	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodPost, "/projects", form.Encode(), s.User, nil)
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	drainEvent(t, ch, "project.created")
}

func TestProjectPut_PublishesProjectRenamed(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pub-proj-rename")
	p, err := s.Projects.EnsureBySlug(s.User.ID, "Old", "old")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	b := sse.New()

	d := ProjectActionsDeps{
		Projects: s.Projects,
		Clock:    &testutil.FixedClock{T: now},
		Bus:      b,
	}
	h := NewProjectPut(d)

	ch, cancel := b.Subscribe(s.User.ID)
	defer cancel()

	form := url.Values{}
	form.Set("name", "New")
	form.Set("version", strconv.FormatInt(p.Version, 10))
	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodPut, "/projects/"+p.ID, form.Encode(), s.User, map[string]string{"id": p.ID})
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	drainEvent(t, ch, "project.renamed")
}

func TestProjectArchive_PublishesProjectArchived(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pub-proj-arch")
	p := seedProject(t, s.Projects, s.User.ID, "doomed")
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	b := sse.New()

	d := ProjectActionsDeps{
		Projects: s.Projects,
		Clock:    &testutil.FixedClock{T: now},
		Bus:      b,
	}
	h := NewProjectArchive(d)

	ch, cancel := b.Subscribe(s.User.ID)
	defer cancel()

	rr := httptest.NewRecorder()
	r := paReq(t, http.MethodPost, "/projects/"+p.ID+"/archive", "", s.User, map[string]string{"id": p.ID})
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	drainEvent(t, ch, "project.archived")
}

// TestRepoNotePut_PublishesRepoNoteUpdated exercises the repo-note path
// through the full handler backed by pgstore Documents.
func TestRepoNotePut_PublishesRepoNoteUpdated(t *testing.T) {
	t.Parallel()
	s := newPGStores(t, "pub-rn-1")

	canonicalKey := "gh/serverkraken/flow"
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	b := sse.New()
	d := NoteActionsDeps{
		Documents: s.Documents,
		Clock:     &testutil.FixedClock{T: now},
		Bus:       b,
	}
	h := NewRepoNotePut(d)

	ch, cancel := b.Subscribe(s.User.ID)
	defer cancel()

	form := url.Values{}
	form.Set("content", "hello sse")
	form.Set("version", "0")
	encoded := url.PathEscape(canonicalKey)
	rr := httptest.NewRecorder()
	r := naReq(t, http.MethodPut, "/repos/"+encoded+"/note", form.Encode(), s.User, map[string]string{"key": encoded})
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303; body=%s", rr.Code, rr.Body.String())
	}
	drainEvent(t, ch, "repo_note.updated")
}
