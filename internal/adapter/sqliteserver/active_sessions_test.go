package sqliteserver

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// Test 1: Start with no existing row and expectedVersion=0 → OK.
func TestUnit_ServerActiveSessions_Start_NewRow_Succeeds(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sas1")
	p := serverTestProject(t, store, u.ID, "sas-proj1")
	as := NewActiveSessions(store)

	got, err := as.Start(u.ID, p.ID, "laptop", 0, "", "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got.Version == 0 {
		t.Error("version should be non-zero after Start")
	}
	if got.UserID != u.ID {
		t.Errorf("UserID: got %q, want %q", got.UserID, u.ID)
	}
	if got.ProjectID != p.ID {
		t.Errorf("ProjectID: got %q, want %q", got.ProjectID, p.ID)
	}
	if got.StartedOnDevice != "laptop" {
		t.Errorf("StartedOnDevice: got %q, want %q", got.StartedOnDevice, "laptop")
	}

	// Verify row persisted.
	fetched, err := as.Get(u.ID, p.ID)
	if err != nil {
		t.Fatalf("Get after Start: %v", err)
	}
	if fetched.Version != got.Version {
		t.Errorf("Get version: got %d, want %d", fetched.Version, got.Version)
	}
}

// Test 2: Start with existing row and expectedVersion=0 → ErrActiveSessionConflict.
func TestUnit_ServerActiveSessions_Start_RowExists_ZeroExpected_Conflict(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sas2")
	p := serverTestProject(t, store, u.ID, "sas-proj2")
	as := NewActiveSessions(store)

	if _, err := as.Start(u.ID, p.ID, "laptop", 0, "", ""); err != nil {
		t.Fatalf("first Start: %v", err)
	}

	_, err := as.Start(u.ID, p.ID, "phone", 0, "", "")
	if !errors.Is(err, ports.ErrActiveSessionConflict) {
		t.Errorf("want ErrActiveSessionConflict, got %v", err)
	}
}

// Test 3: Start with existing row and matching expectedVersion → force-takeover succeeds.
func TestUnit_ServerActiveSessions_Start_Takeover_MatchingVersion_Succeeds(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sas3")
	p := serverTestProject(t, store, u.ID, "sas-proj3")
	as := NewActiveSessions(store)

	first, err := as.Start(u.ID, p.ID, "laptop", 0, "", "")
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}

	second, err := as.Start(u.ID, p.ID, "phone", first.Version, "", "")
	if err != nil {
		t.Fatalf("takeover Start: %v", err)
	}
	if second.Version <= first.Version {
		t.Errorf("version must bump on takeover: v1=%d v2=%d", first.Version, second.Version)
	}
	if second.StartedOnDevice != "phone" {
		t.Errorf("StartedOnDevice: got %q, want %q", second.StartedOnDevice, "phone")
	}

	// Verify row was updated (not duplicated).
	list, err := as.ListByUser(u.ID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("want exactly 1 active session after takeover, got %d", len(list))
	}
}

// Test 4: Start with no row and expectedVersion>0 → ErrActiveSessionConflict.
func TestUnit_ServerActiveSessions_Start_NoRow_NonZeroExpected_Conflict(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sas4")
	p := serverTestProject(t, store, u.ID, "sas-proj4")
	as := NewActiveSessions(store)

	_, err := as.Start(u.ID, p.ID, "laptop", 42, "", "")
	if !errors.Is(err, ports.ErrActiveSessionConflict) {
		t.Errorf("want ErrActiveSessionConflict for non-zero expected on absent row, got %v", err)
	}
}

// Test 5: Stop happy path — returns Session with correct fields; active row gone; sessions row present.
func TestUnit_ServerActiveSessions_Stop_HappyPath(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sas5")
	p := serverTestProject(t, store, u.ID, "sas-proj5")
	as := NewActiveSessions(store)

	started, err := as.Start(u.ID, p.ID, "laptop", 0, "", "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Small sleep so Stop > Start (sub-millisecond same-second is fine since
	// RFC3339 has second resolution; just ensure timestamps are not identical).
	time.Sleep(time.Millisecond)

	sess, err := as.Stop(u.ID, p.ID, started.Version, "deep", "done")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if sess.UserID != u.ID {
		t.Errorf("UserID: got %q, want %q", sess.UserID, u.ID)
	}
	if sess.ProjectID != p.ID {
		t.Errorf("ProjectID: got %q, want %q", sess.ProjectID, p.ID)
	}
	if sess.Tag != "deep" {
		t.Errorf("Tag: got %q, want %q", sess.Tag, "deep")
	}
	if sess.Note != "done" {
		t.Errorf("Note: got %q, want %q", sess.Note, "done")
	}
	if sess.Start.IsZero() {
		t.Error("Start must not be zero")
	}
	if !sess.Stop.After(sess.Start) || sess.Stop.Equal(sess.Start) {
		// RFC3339 rounds to seconds; if Start and Stop are within the same second
		// they may be equal. Accept that edge but not Stop < Start.
		if sess.Stop.Before(sess.Start) {
			t.Errorf("Stop must not be before Start: start=%v stop=%v", sess.Start, sess.Stop)
		}
	}
	if sess.Elapsed < 0 {
		t.Errorf("Elapsed must be non-negative, got %v", sess.Elapsed)
	}
	if sess.Version == 0 {
		t.Error("Version must be non-zero after Stop")
	}
}

// Test 6: Stop atomicity — active_sessions count = 0, sessions count = 1 after Stop;
// sessions.version > active_sessions version before Stop.
func TestUnit_ServerActiveSessions_Stop_Atomicity(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sas6")
	p := serverTestProject(t, store, u.ID, "sas-proj6")
	as := NewActiveSessions(store)

	started, err := as.Start(u.ID, p.ID, "laptop", 0, "", "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	activeVersionBefore := started.Version

	sess, err := as.Stop(u.ID, p.ID, started.Version, "", "")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// active_sessions row must be gone.
	var activeCount int
	if err := store.DB().QueryRow(
		`SELECT COUNT(*) FROM active_sessions WHERE user_id = ? AND project_id = ?`,
		u.ID, p.ID,
	).Scan(&activeCount); err != nil {
		t.Fatalf("count active_sessions: %v", err)
	}
	if activeCount != 0 {
		t.Errorf("active_sessions count: got %d, want 0", activeCount)
	}

	// sessions row must exist.
	var sessCount int
	if err := store.DB().QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE id = ?`, sess.ID,
	).Scan(&sessCount); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessCount != 1 {
		t.Errorf("sessions count: got %d, want 1", sessCount)
	}

	// Session version must be strictly greater than the active session version.
	if sess.Version <= activeVersionBefore {
		t.Errorf("sessions.version (%d) must exceed active_sessions.version (%d)",
			sess.Version, activeVersionBefore)
	}
}

// Test 7: Stop with no active row → ErrActiveSessionNotFound.
func TestUnit_ServerActiveSessions_Stop_NoRow_NotFound(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sas7")
	p := serverTestProject(t, store, u.ID, "sas-proj7")
	as := NewActiveSessions(store)

	_, err := as.Stop(u.ID, p.ID, 0, "", "")
	if !errors.Is(err, ports.ErrActiveSessionNotFound) {
		t.Errorf("want ErrActiveSessionNotFound, got %v", err)
	}
}

// Test 8: Stop with wrong expectedVersion → ErrActiveSessionConflict; both
// tables unchanged.
//
// Rollback path analysis: the version mismatch check (curVersion !=
// expectedVersion) executes BEFORE NextLamport is called and before any
// INSERT/DELETE, so the transaction is simply rolled back via defer. No
// partial writes can occur. We verify this by asserting both table counts
// are unchanged after the failed Stop.
//
// A contrived mid-transaction FK failure (e.g. inserting a sessions row
// with an invalid project_id) cannot be used here without changing the
// Stop signature or adding test-only injection hooks; the guard before
// NextLamport is the right level to test. See comment for a paranoia
// observation instead.
func TestUnit_ServerActiveSessions_Stop_WrongVersion_Conflict_NoSideEffects(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sas8")
	p := serverTestProject(t, store, u.ID, "sas-proj8")
	as := NewActiveSessions(store)

	started, err := as.Start(u.ID, p.ID, "laptop", 0, "", "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Count before attempted Stop.
	var activeBefore, sessBefore int
	if err := store.DB().QueryRow(
		`SELECT COUNT(*) FROM active_sessions WHERE user_id = ? AND project_id = ?`,
		u.ID, p.ID,
	).Scan(&activeBefore); err != nil {
		t.Fatalf("count active_sessions before: %v", err)
	}
	if err := store.DB().QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE user_id = ? AND project_id = ?`,
		u.ID, p.ID,
	).Scan(&sessBefore); err != nil {
		t.Fatalf("count sessions before: %v", err)
	}

	_, err = as.Stop(u.ID, p.ID, started.Version+99, "", "")
	if !errors.Is(err, ports.ErrActiveSessionConflict) {
		t.Errorf("want ErrActiveSessionConflict, got %v", err)
	}

	// Assert no side-effects: both counts unchanged.
	var activeAfter, sessAfter int
	if err := store.DB().QueryRow(
		`SELECT COUNT(*) FROM active_sessions WHERE user_id = ? AND project_id = ?`,
		u.ID, p.ID,
	).Scan(&activeAfter); err != nil {
		t.Fatalf("count active_sessions after: %v", err)
	}
	if err := store.DB().QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE user_id = ? AND project_id = ?`,
		u.ID, p.ID,
	).Scan(&sessAfter); err != nil {
		t.Fatalf("count sessions after: %v", err)
	}
	if activeAfter != activeBefore {
		t.Errorf("active_sessions count changed: before=%d after=%d", activeBefore, activeAfter)
	}
	if sessAfter != sessBefore {
		t.Errorf("sessions count changed: before=%d after=%d", sessBefore, sessAfter)
	}
}

// Test 9: ListByUser returns all active sessions across different projects.
func TestUnit_ServerActiveSessions_ListByUser_MultipleProjects(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sas9")
	p1 := serverTestProject(t, store, u.ID, "sas-proj9a")
	p2 := serverTestProject(t, store, u.ID, "sas-proj9b")
	p3 := serverTestProject(t, store, u.ID, "sas-proj9c")
	as := NewActiveSessions(store)

	for i, pID := range []string{p1.ID, p2.ID, p3.ID} {
		if _, err := as.Start(u.ID, pID, "device", 0, "", ""); err != nil {
			t.Fatalf("Start project %d: %v", i, err)
		}
	}

	list, err := as.ListByUser(u.ID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("want 3 active sessions, got %d", len(list))
	}
}

// Test 10: PullSince returns rows with version > since, ordered ascending.
func TestUnit_ServerActiveSessions_PullSince_ReturnsOnlyGreaterVersions(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sas10")
	p1 := serverTestProject(t, store, u.ID, "sas-proj10a")
	p2 := serverTestProject(t, store, u.ID, "sas-proj10b")
	p3 := serverTestProject(t, store, u.ID, "sas-proj10c")
	as := NewActiveSessions(store)

	var versions []int64
	for _, pID := range []string{p1.ID, p2.ID, p3.ID} {
		a, err := as.Start(u.ID, pID, "device", 0, "", "")
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		versions = append(versions, a.Version)
	}

	// PullSince versions[0] should return rows 1 and 2 only.
	got, high, err := as.PullSince(u.ID, versions[0])
	if err != nil {
		t.Fatalf("PullSince: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 rows after since=%d, got %d", versions[0], len(got))
	}
	if high != versions[2] {
		t.Errorf("high watermark: got %d, want %d", high, versions[2])
	}

	// Verify ascending order.
	for i := 1; i < len(got); i++ {
		if got[i].Version <= got[i-1].Version {
			t.Errorf("PullSince result not ascending: [%d].Version=%d >= [%d].Version=%d",
				i-1, got[i-1].Version, i, got[i].Version)
		}
	}
}

// Test: Get with no row → ErrActiveSessionNotFound.
func TestUnit_ServerActiveSessions_Get_NotFound(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sas11")
	p := serverTestProject(t, store, u.ID, "sas-proj11")
	as := NewActiveSessions(store)

	_, err := as.Get(u.ID, p.ID)
	if !errors.Is(err, ports.ErrActiveSessionNotFound) {
		t.Errorf("want ErrActiveSessionNotFound, got %v", err)
	}
}

// Test: PullSince with no rows returns since as watermark and empty slice.
func TestUnit_ServerActiveSessions_PullSince_EmptyResult(t *testing.T) {
	t.Parallel()
	store := mustOpenServer(t)
	u := serverTestUser(t, store, "sas12")
	as := NewActiveSessions(store)

	got, high, err := as.PullSince(u.ID, 999)
	if err != nil {
		t.Fatalf("PullSince: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 rows, got %d", len(got))
	}
	if high != 999 {
		t.Errorf("high watermark: got %d, want 999 (since passthrough)", high)
	}
}
