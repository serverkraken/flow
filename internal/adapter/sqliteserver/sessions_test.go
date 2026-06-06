package sqliteserver

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// mkServerSession builds a minimal valid server session with a new UUID.
func mkServerSession(userID, projectID string) domain.Session {
	now := time.Now().UTC().Truncate(time.Second)
	return domain.Session{
		ID:        uuid.NewString(),
		UserID:    userID,
		ProjectID: projectID,
		Date:      now,
		Start:     now,
		Stop:      now.Add(time.Hour),
		Elapsed:   time.Hour,
		Tag:       "deep",
		Note:      "test",
	}
}

func TestUnit_ServerSessions_PullSince_ReturnsOnlyGreaterVersions(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "ssess1")
	p := serverTestProject(t, store, u.ID, "ssess-proj1")
	sessions := NewSessions(store)

	var versions []int64
	for i := 0; i < 3; i++ {
		s, err := sessions.Upsert(mkServerSession(u.ID, p.ID), 0)
		if err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
		versions = append(versions, s.Version)
	}

	// PullSince versions[0] should return rows 1 and 2 only.
	got, high, hasMore, err := sessions.PullSince(u.ID, versions[0], 10)
	if err != nil {
		t.Fatalf("PullSince: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 rows, got %d", len(got))
	}
	if hasMore {
		t.Errorf("hasMore should be false")
	}
	if high != versions[2] {
		t.Errorf("high watermark: got %d, want %d", high, versions[2])
	}
}

func TestUnit_ServerSessions_PullSince_HasMore(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "ssess2")
	p := serverTestProject(t, store, u.ID, "ssess-proj2")
	sessions := NewSessions(store)

	for i := 0; i < 5; i++ {
		if _, err := sessions.Upsert(mkServerSession(u.ID, p.ID), 0); err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
	}

	_, _, hasMore, err := sessions.PullSince(u.ID, 0, 3)
	if err != nil {
		t.Fatalf("PullSince: %v", err)
	}
	if !hasMore {
		t.Errorf("hasMore should be true when more rows exist beyond limit")
	}
}

func TestUnit_ServerSessions_Upsert_Insert_Succeeds(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "ssess3")
	p := serverTestProject(t, store, u.ID, "ssess-proj3")
	sessions := NewSessions(store)

	sess := mkServerSession(u.ID, p.ID)
	out, err := sessions.Upsert(sess, 0)
	if err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}
	if out.Version == 0 {
		t.Errorf("version should be non-zero after insert")
	}
	if out.ID != sess.ID {
		t.Errorf("ID mismatch: got %q, want %q", out.ID, sess.ID)
	}
}

func TestUnit_ServerSessions_Upsert_Update_CorrectVersion_Bumps(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "ssess4")
	p := serverTestProject(t, store, u.ID, "ssess-proj4")
	sessions := NewSessions(store)

	s1, err := sessions.Upsert(mkServerSession(u.ID, p.ID), 0)
	if err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}

	updated := s1
	updated.Note = "updated note"
	s2, err := sessions.Upsert(updated, s1.Version)
	if err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}
	if s2.Version <= s1.Version {
		t.Errorf("version must bump: v1=%d v2=%d", s1.Version, s2.Version)
	}
}

func TestUnit_ServerSessions_Upsert_Update_WrongVersion_ConflictError(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "ssess5")
	p := serverTestProject(t, store, u.ID, "ssess-proj5")
	sessions := NewSessions(store)

	s1, err := sessions.Upsert(mkServerSession(u.ID, p.ID), 0)
	if err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}

	_, err = sessions.Upsert(s1, s1.Version+99)
	if !errors.Is(err, ports.ErrSessionVersionConflict) {
		t.Errorf("want ErrSessionVersionConflict, got %v", err)
	}
}

func TestUnit_ServerSessions_Upsert_InsertWithNonZeroExpected_ConflictError(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "ssess6")
	p := serverTestProject(t, store, u.ID, "ssess-proj6")
	sessions := NewSessions(store)

	_, err := sessions.Upsert(mkServerSession(u.ID, p.ID), 42)
	if !errors.Is(err, ports.ErrSessionVersionConflict) {
		t.Errorf("want ErrSessionVersionConflict for non-zero expected on insert, got %v", err)
	}
}

func TestUnit_ServerSessions_Upsert_Concurrent_DistinctVersions(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "ssess7")
	p := serverTestProject(t, store, u.ID, "ssess-proj7")
	sessions := NewSessions(store)

	var mu sync.Mutex
	versions := make(map[int64]bool)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := sessions.Upsert(mkServerSession(u.ID, p.ID), 0)
			if err != nil {
				t.Errorf("concurrent Upsert: %v", err)
				return
			}
			mu.Lock()
			if versions[s.Version] {
				t.Errorf("duplicate version %d", s.Version)
			}
			versions[s.Version] = true
			mu.Unlock()
		}()
	}
	wg.Wait()
}

func TestUnit_ServerSessions_GetByID_HappyPath(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "ssess-get1")
	p := serverTestProject(t, store, u.ID, "ssess-get-proj")
	sessions := NewSessions(store)

	stored, err := sessions.Upsert(mkServerSession(u.ID, p.ID), 0)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := sessions.GetByID(u.ID, stored.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != stored.ID {
		t.Errorf("ID: got %q, want %q", got.ID, stored.ID)
	}
	if got.Tag != "deep" {
		t.Errorf("Tag: got %q, want %q", got.Tag, "deep")
	}
	if got.Note != "test" {
		t.Errorf("Note: got %q, want %q", got.Note, "test")
	}
	if got.Version != stored.Version {
		t.Errorf("Version: got %d, want %d", got.Version, stored.Version)
	}
	if got.Elapsed != time.Hour {
		t.Errorf("Elapsed: got %v, want %v", got.Elapsed, time.Hour)
	}
}

func TestUnit_ServerSessions_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "ssess-get2")
	sessions := NewSessions(store)

	_, err := sessions.GetByID(u.ID, "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, ports.ErrSessionNotFound) {
		t.Errorf("want ErrSessionNotFound, got %v", err)
	}
}

// mkSessionOnDate builds a session on a specific calendar date for date-range tests.
func mkSessionOnDate(userID, projectID string, date time.Time, dur time.Duration) domain.Session {
	day := time.Date(date.Year(), date.Month(), date.Day(), 9, 0, 0, 0, time.UTC)
	return domain.Session{
		ID:        uuid.NewString(),
		UserID:    userID,
		ProjectID: projectID,
		Date:      day,
		Start:     day,
		Stop:      day.Add(dur),
		Elapsed:   dur,
		Tag:       "deep",
		Note:      "test",
	}
}

func TestUnit_ServerSessions_ListByUserDateRange_FiltersByDate(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "ssess-range1")
	p := serverTestProject(t, store, u.ID, "ssess-range-proj")
	sessions := NewSessions(store)

	base := time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC)
	// Three sessions on different days.
	for i := -2; i <= 0; i++ {
		if _, err := sessions.Upsert(mkSessionOnDate(u.ID, p.ID, base.AddDate(0, 0, i), time.Hour), 0); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	// Range [base-1d, base] → 2 sessions.
	got, err := sessions.ListByUserDateRange(u.ID, base.AddDate(0, 0, -1), base)
	if err != nil {
		t.Fatalf("ListByUserDateRange: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 sessions in range, got %d", len(got))
	}
}

func TestUnit_ServerSessions_ListByUserDateRange_OrderedByStartAsc(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "ssess-range2")
	p := serverTestProject(t, store, u.ID, "ssess-range-proj2")
	sessions := NewSessions(store)

	base := time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC)
	// Insert out-of-order; expect ordering by start ASC.
	later := mkSessionOnDate(u.ID, p.ID, base, time.Hour)
	later.Start = base.Add(15 * time.Hour)
	later.Stop = later.Start.Add(time.Hour)
	earlier := mkSessionOnDate(u.ID, p.ID, base, time.Hour)
	earlier.Start = base.Add(9 * time.Hour)
	earlier.Stop = earlier.Start.Add(time.Hour)
	if _, err := sessions.Upsert(later, 0); err != nil {
		t.Fatalf("Upsert later: %v", err)
	}
	if _, err := sessions.Upsert(earlier, 0); err != nil {
		t.Fatalf("Upsert earlier: %v", err)
	}

	got, err := sessions.ListByUserDateRange(u.ID, base, base)
	if err != nil {
		t.Fatalf("ListByUserDateRange: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(got))
	}
	if !got[0].Start.Before(got[1].Start) {
		t.Errorf("want sessions ordered by start ASC, got %v then %v", got[0].Start, got[1].Start)
	}
}

func TestUnit_ServerSessions_ListByUserDateRange_UserIsolation(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	uA := serverTestUser(t, store, "ssess-range-uA")
	uB := serverTestUser(t, store, "ssess-range-uB")
	pA := serverTestProject(t, store, uA.ID, "ssess-range-pA")
	pB := serverTestProject(t, store, uB.ID, "ssess-range-pB")
	sessions := NewSessions(store)

	base := time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC)
	if _, err := sessions.Upsert(mkSessionOnDate(uA.ID, pA.ID, base, time.Hour), 0); err != nil {
		t.Fatalf("Upsert A: %v", err)
	}
	if _, err := sessions.Upsert(mkSessionOnDate(uB.ID, pB.ID, base, time.Hour), 0); err != nil {
		t.Fatalf("Upsert B: %v", err)
	}

	got, err := sessions.ListByUserDateRange(uA.ID, base, base)
	if err != nil {
		t.Fatalf("ListByUserDateRange uA: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 session for uA, got %d", len(got))
	}
	if got[0].UserID != uA.ID {
		t.Errorf("returned row leaks userID %q (want %q)", got[0].UserID, uA.ID)
	}
}

func TestUnit_ServerSessions_Delete_HappyPath(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "ssess-del1")
	p := serverTestProject(t, store, u.ID, "ssess-del-proj1")
	sessions := NewSessions(store)

	s, err := sessions.Upsert(mkServerSession(u.ID, p.ID), 0)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := sessions.Delete(u.ID, s.ID, s.Version); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := sessions.GetByID(u.ID, s.ID); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound after delete, got %v", err)
	}
}

func TestUnit_ServerSessions_Delete_WrongVersion_ConflictError(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "ssess-del2")
	p := serverTestProject(t, store, u.ID, "ssess-del-proj2")
	sessions := NewSessions(store)

	s, err := sessions.Upsert(mkServerSession(u.ID, p.ID), 0)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := sessions.Delete(u.ID, s.ID, s.Version+99); !errors.Is(err, ports.ErrSessionVersionConflict) {
		t.Errorf("expected ErrSessionVersionConflict, got %v", err)
	}
	// Row must still exist.
	if _, err := sessions.GetByID(u.ID, s.ID); err != nil {
		t.Errorf("row should still exist after conflict, got %v", err)
	}
}

func TestUnit_ServerSessions_Delete_NotFound(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "ssess-del3")
	sessions := NewSessions(store)

	if err := sessions.Delete(u.ID, "00000000-0000-0000-0000-000000000000", 0); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestUnit_ServerSessions_Delete_CrossTenant_NotFound(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	uA := serverTestUser(t, store, "ssess-del-uA")
	uB := serverTestUser(t, store, "ssess-del-uB")
	pA := serverTestProject(t, store, uA.ID, "ssess-del-pA")
	sessions := NewSessions(store)

	s, err := sessions.Upsert(mkServerSession(uA.ID, pA.ID), 0)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	// User B tries to delete user A's session — must look like not found.
	if err := sessions.Delete(uB.ID, s.ID, s.Version); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Errorf("cross-tenant delete should be ErrSessionNotFound, got %v", err)
	}
	// Row must still exist for owner.
	if _, err := sessions.GetByID(uA.ID, s.ID); err != nil {
		t.Errorf("row should still exist for owner after cross-tenant delete attempt, got %v", err)
	}
}

func TestUnit_ServerSessions_ListByUserDateRange_EmptyWhenNoMatch(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "ssess-range3")
	sessions := NewSessions(store)

	base := time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC)
	got, err := sessions.ListByUserDateRange(u.ID, base, base)
	if err != nil {
		t.Fatalf("ListByUserDateRange: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty result, got %d rows", len(got))
	}
}
