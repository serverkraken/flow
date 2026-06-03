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
