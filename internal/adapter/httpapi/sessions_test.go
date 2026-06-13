package httpapi_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestSessions_LoadAndUpsert(t *testing.T) {
	api := newTestAPI(t)
	projects := httpapi.NewProjects(api.Client)
	sessions := httpapi.NewSessions(api.Client)

	// Ensure a project exists to reference
	proj, err := projects.EnsureBySlug("", "Sessions Test Project", "sessions-test")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	s := domain.Session{
		ProjectID: proj.ID,
		Date:      now.Truncate(24 * time.Hour),
		Start:     now.Add(-1 * time.Hour),
		Stop:      now,
		Tag:       "test",
		Note:      "integration test",
	}

	// Create new session
	if err := sessions.Upsert(s); err != nil {
		t.Fatalf("Upsert new: %v", err)
	}

	// Load and find the session
	all, err := sessions.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var found domain.Session
	for _, sess := range all {
		if sess.ProjectID == proj.ID && sess.Tag == "test" {
			found = sess
			break
		}
	}
	if found.ID == "" {
		t.Fatal("created session not found in Load result")
	}

	// Update the session (version must match)
	found.Note = "updated note"
	if err := sessions.Upsert(found); err != nil {
		t.Fatalf("Upsert update: %v", err)
	}

	// Reload and verify update
	all2, err := sessions.Load("")
	if err != nil {
		t.Fatalf("Load after update: %v", err)
	}
	for _, sess := range all2 {
		if sess.ID == found.ID {
			if sess.Note != "updated note" {
				t.Errorf("note = %q, want %q", sess.Note, "updated note")
			}
			return
		}
	}
	t.Error("updated session not found in second Load")
}

func TestSessions_LoadFiltered(t *testing.T) {
	api := newTestAPI(t)
	projects := httpapi.NewProjects(api.Client)
	sessions := httpapi.NewSessions(api.Client)

	proj, err := projects.EnsureBySlug("", "Filter Test Project", "sessions-filter-test")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	s := domain.Session{
		ProjectID: proj.ID,
		Date:      now.Truncate(24 * time.Hour),
		Start:     now.Add(-30 * time.Minute),
		Stop:      now,
		Tag:       "filtered-tag",
	}
	if err := sessions.Upsert(s); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	filtered, err := sessions.LoadFiltered("", func(sess domain.Session) bool {
		return sess.Tag == "filtered-tag"
	})
	if err != nil {
		t.Fatalf("LoadFiltered: %v", err)
	}
	if len(filtered) == 0 {
		t.Error("expected at least one filtered session")
	}
	for _, sess := range filtered {
		if sess.Tag != "filtered-tag" {
			t.Errorf("filter leaked session with tag %q", sess.Tag)
		}
	}
}

func TestSessions_Delete(t *testing.T) {
	api := newTestAPI(t)
	projects := httpapi.NewProjects(api.Client)
	sessions := httpapi.NewSessions(api.Client)

	proj, err := projects.EnsureBySlug("", "Delete Test Project", "sessions-delete-test")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	s := domain.Session{
		ProjectID: proj.ID,
		Date:      now.Truncate(24 * time.Hour),
		Start:     now.Add(-20 * time.Minute),
		Stop:      now,
		Tag:       "to-delete",
	}
	if err := sessions.Upsert(s); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	all, err := sessions.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var id string
	for _, sess := range all {
		if sess.Tag == "to-delete" && sess.ProjectID == proj.ID {
			id = sess.ID
			break
		}
	}
	if id == "" {
		t.Fatal("created session not found")
	}

	if err := sessions.Delete("", id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify gone
	all2, err := sessions.Load("")
	if err != nil {
		t.Fatalf("Load after delete: %v", err)
	}
	for _, sess := range all2 {
		if sess.ID == id {
			t.Error("deleted session still present in Load result")
		}
	}
}

func TestSessions_UpsertBatch(t *testing.T) {
	api := newTestAPI(t)
	projects := httpapi.NewProjects(api.Client)
	sessions := httpapi.NewSessions(api.Client)

	proj, err := projects.EnsureBySlug("", "Bulk Test Project", "sessions-bulk-test")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	batch := []domain.Session{
		{
			ID:        "bulk-id-0001-0001-0001-000000000001",
			ProjectID: proj.ID,
			Date:      now.Truncate(24 * time.Hour),
			Start:     now.Add(-2 * time.Hour),
			Stop:      now.Add(-1 * time.Hour),
			Tag:       "bulk",
		},
		{
			ID:        "bulk-id-0002-0002-0002-000000000002",
			ProjectID: proj.ID,
			Date:      now.Truncate(24 * time.Hour),
			Start:     now.Add(-1 * time.Hour),
			Stop:      now,
			Tag:       "bulk",
		},
	}

	if err := sessions.UpsertBatch(batch); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}

	all, err := sessions.Load("")
	if err != nil {
		t.Fatalf("Load after batch: %v", err)
	}
	found := 0
	for _, sess := range all {
		if sess.Tag == "bulk" && sess.ProjectID == proj.ID {
			found++
		}
	}
	if found < 2 {
		t.Errorf("expected at least 2 bulk sessions, got %d", found)
	}
}

func TestSessions_Offline_FallsBackToCache(t *testing.T) {
	api := newTestAPI(t)
	projects := httpapi.NewProjects(api.Client)
	sessions := httpapi.NewSessions(api.Client)

	proj, err := projects.EnsureBySlug("", "Offline Test Project", "sessions-offline-test")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	s := domain.Session{
		ProjectID: proj.ID,
		Date:      now.Truncate(24 * time.Hour),
		Start:     now.Add(-15 * time.Minute),
		Stop:      now,
		Tag:       "offline-test",
	}
	if err := sessions.Upsert(s); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Populate cache
	first, err := sessions.Load("")
	if err != nil {
		t.Fatalf("initial Load: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("expected sessions in initial load")
	}

	// Kill the server
	api.Close()

	// Should return cached data
	second, err := sessions.Load("")
	if err != nil {
		t.Fatalf("offline Load returned error: %v", err)
	}
	if len(second) == 0 {
		t.Error("expected cached sessions offline, got empty slice")
	}
}

func TestSessions_VersionConflict(t *testing.T) {
	api := newTestAPI(t)
	projects := httpapi.NewProjects(api.Client)
	sessions := httpapi.NewSessions(api.Client)

	proj, err := projects.EnsureBySlug("", "Conflict Test Project", "sessions-conflict-test")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	s := domain.Session{
		ProjectID: proj.ID,
		Date:      now.Truncate(24 * time.Hour),
		Start:     now.Add(-10 * time.Minute),
		Stop:      now,
		Tag:       "conflict",
	}
	if err := sessions.Upsert(s); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	all, err := sessions.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var found domain.Session
	for _, sess := range all {
		if sess.Tag == "conflict" && sess.ProjectID == proj.ID {
			found = sess
			break
		}
	}
	if found.ID == "" {
		t.Fatal("session not found")
	}

	// Simulate stale version by creating a second Sessions adapter that
	// doesn't know about the version and tries to update with version 0 on a
	// known ID. We manipulate via a second adapter that loads fresh and then
	// updates via the first (which now has a stale version baked in via
	// manually crafting a session with wrong version).

	// Update via a separate Sessions adapter (simulating another device)
	sessions2 := httpapi.NewSessions(api.Client)
	found2 := found
	found2.Note = "first update"
	if err := sessions2.Upsert(found2); err != nil {
		t.Fatalf("Upsert from second adapter: %v", err)
	}

	// Now try to update from original found (stale version) — should conflict
	// We have to reload the original sessions to force it to re-discover the
	// new version; then craft a stale update by using a brand-new adapter
	// that has old version baked in from the original load.
	staleSession := found // version from before second adapter's update
	staleSession.Note = "stale update"
	// sessions has the original version in its map; after sessions2 updated,
	// the server version changed. Invalidate sessions cache and force reload.
	// Then manually set a stale entry by issuing a PUT to an ID that we know
	// about but with wrong version — but sessions adapter auto-tracks.
	// Actually: reload sessions to get current version, then issue a PUT
	// with an intentionally wrong version by crafting a domain.Session
	// with Version manually set wrong.
	// The sessions adapter uses its internal versions map. After sessions2.Upsert,
	// sessions still has the old version. Calling sessions.Upsert(staleSession)
	// will send the old version, which should trigger 412.
	err = sessions.Upsert(staleSession)
	if !errors.Is(err, ports.ErrSessionVersionConflict) {
		t.Errorf("expected ErrSessionVersionConflict, got: %v", err)
	}
}
