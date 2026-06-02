package sqliteclient

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
)

// testUser and testProject are helpers to quickly provision required FK rows.
func testUser(t *testing.T, store *Store, subSuffix string) domain.User {
	t.Helper()
	u, err := NewUsers(store).EnsureBySub("sub|"+subSuffix, subSuffix+"@example.com", subSuffix)
	if err != nil {
		t.Fatalf("testUser EnsureBySub: %v", err)
	}
	return u
}

func testProject(t *testing.T, store *Store, userID, slug string) domain.Project {
	t.Helper()
	p, err := NewProjects(store).EnsureBySlug(userID, slug, slug)
	if err != nil {
		t.Fatalf("testProject EnsureBySlug: %v", err)
	}
	return p
}

func TestUnit_Sessions_Upsert_InsertThenUpdate(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	sessions := NewSessions(store)

	u := testUser(t, store, "sess1")
	p := testProject(t, store, u.ID, "proj-sess1")

	now := time.Now().UTC().Truncate(time.Second)
	sess := domain.Session{
		ID:        uuid.NewString(),
		UserID:    u.ID,
		ProjectID: p.ID,
		Date:      now,
		Start:     now,
		Stop:      now.Add(time.Hour),
		Elapsed:   time.Hour,
		Tag:       "deep",
		Note:      "initial",
		Version:   1,
		UpdatedAt: now,
	}

	if err := sessions.Upsert(sess); err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}

	// Update the note and version.
	sess.Note = "updated"
	sess.Version = 2
	if err := sessions.Upsert(sess); err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}

	all, err := sessions.Load(u.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 session, got %d", len(all))
	}
	if all[0].Note != "updated" {
		t.Errorf("note not updated: got %q, want %q", all[0].Note, "updated")
	}
	if all[0].Version != 2 {
		t.Errorf("version not updated: got %d, want 2", all[0].Version)
	}
}

func TestUnit_Sessions_LoadFiltered_ByProjectID(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	sessions := NewSessions(store)

	u := testUser(t, store, "sess2")
	pA := testProject(t, store, u.ID, "proj-a")
	pB := testProject(t, store, u.ID, "proj-b")

	now := time.Now().UTC().Truncate(time.Second)
	mkSess := func(projectID string) domain.Session {
		return domain.Session{
			ID:        uuid.NewString(),
			UserID:    u.ID,
			ProjectID: projectID,
			Date:      now,
			Start:     now,
			Stop:      now.Add(time.Hour),
			Elapsed:   time.Hour,
			Version:   1,
			UpdatedAt: now,
		}
	}

	for i := 0; i < 2; i++ {
		if err := sessions.Upsert(mkSess(pA.ID)); err != nil {
			t.Fatalf("Upsert pA: %v", err)
		}
	}
	if err := sessions.Upsert(mkSess(pB.ID)); err != nil {
		t.Fatalf("Upsert pB: %v", err)
	}

	filtered, err := sessions.LoadFiltered(u.ID, func(s domain.Session) bool {
		return s.ProjectID == pA.ID
	})
	if err != nil {
		t.Fatalf("LoadFiltered: %v", err)
	}
	if len(filtered) != 2 {
		t.Errorf("expected 2 sessions for pA, got %d", len(filtered))
	}
	for _, s := range filtered {
		if s.ProjectID != pA.ID {
			t.Errorf("unexpected project_id %q in filtered result", s.ProjectID)
		}
	}
}

func TestUnit_Sessions_Delete_RemovesRow(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	sessions := NewSessions(store)

	u := testUser(t, store, "sess3")
	p := testProject(t, store, u.ID, "proj-del")

	now := time.Now().UTC().Truncate(time.Second)
	sess := domain.Session{
		ID:        uuid.NewString(),
		UserID:    u.ID,
		ProjectID: p.ID,
		Date:      now,
		Start:     now,
		Stop:      now.Add(time.Hour),
		Elapsed:   time.Hour,
		Version:   1,
		UpdatedAt: now,
	}
	if err := sessions.Upsert(sess); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := sessions.Delete(u.ID, sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	all, err := sessions.Load(u.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, s := range all {
		if s.ID == sess.ID {
			t.Errorf("deleted session %q still present", sess.ID)
		}
	}
}

func TestUnit_Sessions_Load_SortOrder(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	sessions := NewSessions(store)

	u := testUser(t, store, "sess4")
	p := testProject(t, store, u.ID, "proj-sort")

	day1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	toUpsert := []domain.Session{
		{ID: uuid.NewString(), UserID: u.ID, ProjectID: p.ID, Date: day2, Start: day2.Add(10 * time.Hour), Stop: day2.Add(11 * time.Hour), Elapsed: time.Hour, Version: 1, UpdatedAt: day2},
		{ID: uuid.NewString(), UserID: u.ID, ProjectID: p.ID, Date: day1, Start: day1.Add(9 * time.Hour), Stop: day1.Add(10 * time.Hour), Elapsed: time.Hour, Version: 1, UpdatedAt: day1},
		{ID: uuid.NewString(), UserID: u.ID, ProjectID: p.ID, Date: day1, Start: day1.Add(8 * time.Hour), Stop: day1.Add(9 * time.Hour), Elapsed: time.Hour, Version: 1, UpdatedAt: day1},
	}
	if err := sessions.UpsertBatch(toUpsert); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}

	all, err := sessions.Load(u.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(all))
	}
	// Expect: day1@08h, day1@09h, day2@10h
	if !all[0].Date.Equal(day1) || all[0].Start.Hour() != 8 {
		t.Errorf("row[0]: want day1 08h, got %v %v", all[0].Date, all[0].Start)
	}
	if !all[1].Date.Equal(day1) || all[1].Start.Hour() != 9 {
		t.Errorf("row[1]: want day1 09h, got %v %v", all[1].Date, all[1].Start)
	}
	if !all[2].Date.Equal(day2) {
		t.Errorf("row[2]: want day2, got %v", all[2].Date)
	}
}

func TestUnit_Sessions_Upsert_RequiresIDs(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	sessions := NewSessions(store)

	u := testUser(t, store, "sess5")
	p := testProject(t, store, u.ID, "proj-req")
	now := time.Now().UTC()

	cases := []struct {
		name string
		sess domain.Session
	}{
		{
			name: "missing ID",
			sess: domain.Session{UserID: u.ID, ProjectID: p.ID, Date: now, Start: now, Stop: now, Version: 1, UpdatedAt: now},
		},
		{
			name: "missing UserID",
			sess: domain.Session{ID: uuid.NewString(), ProjectID: p.ID, Date: now, Start: now, Stop: now, Version: 1, UpdatedAt: now},
		},
		{
			name: "missing ProjectID",
			sess: domain.Session{ID: uuid.NewString(), UserID: u.ID, Date: now, Start: now, Stop: now, Version: 1, UpdatedAt: now},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := sessions.Upsert(tc.sess); err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}
