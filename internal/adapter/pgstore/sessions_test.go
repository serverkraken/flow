// internal/adapter/pgstore/sessions_test.go
package pgstore_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func mustProject(t *testing.T, userID, slug string) string {
	t.Helper()
	p, err := pgstore.NewProjects(testStore).EnsureBySlug(userID, slug, slug)
	if err != nil {
		t.Fatalf("mustProject: %v", err)
	}
	return p.ID
}

func mkSession(uid, pid string, start time.Time, dur time.Duration) domain.Session {
	return domain.Session{
		ID: uuid.NewString(), UserID: uid, ProjectID: pid,
		Date:  time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC),
		Start: start, Stop: start.Add(dur), Elapsed: dur, Version: 0,
	}
}

func TestSessions_UpsertListSingleDay(t *testing.T) {
	t.Parallel()
	s := pgstore.NewSessions(testStore)
	uid := mustUser(t, "sess-1")
	pid := mustProject(t, uid, "work")

	start := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	in := mkSession(uid, pid, start, time.Hour)
	in.Tag, in.Note = "deep", "fokus"
	saved, err := s.Upsert(in, 0)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if saved.Version != 1 {
		t.Errorf("version: got %d want 1", saved.Version)
	}

	// inklusives Einzeltages-Fenster — exakt der WebUI-Aufruf
	day := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	list, err := s.ListByUserDateRange(uid, day, day)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListByUserDateRange single day: err=%v len=%d", err, len(list))
	}
	if list[0].Tag != "deep" || list[0].Elapsed != time.Hour {
		t.Errorf("roundtrip: %+v", list[0])
	}
}

func TestSessions_UpsertOCCAndDelete(t *testing.T) {
	t.Parallel()
	s := pgstore.NewSessions(testStore)
	uid := mustUser(t, "sess-2")
	pid := mustProject(t, uid, "occ")

	in := mkSession(uid, pid, time.Date(2026, 6, 11, 8, 0, 0, 0, time.UTC), time.Hour)
	saved, _ := s.Upsert(in, 0)

	saved.Note = "edit"
	edited, err := s.Upsert(saved, saved.Version)
	if err != nil || edited.Version != 2 {
		t.Fatalf("edit: err=%v version=%d", err, edited.Version)
	}

	if _, err := s.Upsert(saved, 1); !errors.Is(err, ports.ErrSessionVersionConflict) {
		t.Errorf("stale upsert: want conflict, got %v", err)
	}

	if err := s.Delete(uid, saved.ID, 1); !errors.Is(err, ports.ErrSessionVersionConflict) {
		t.Errorf("stale delete: want conflict, got %v", err)
	}
	if err := s.Delete(uid, saved.ID, 2); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetByID(uid, saved.ID); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Errorf("after delete: want not found, got %v", err)
	}
}

func TestSessions_BulkUpsertIdempotent(t *testing.T) {
	t.Parallel()
	s := pgstore.NewSessions(testStore)
	uid := mustUser(t, "sess-3")
	pid := mustProject(t, uid, "bulk")

	batch := []domain.Session{
		mkSession(uid, pid, time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC), time.Hour),
		mkSession(uid, pid, time.Date(2026, 1, 6, 9, 0, 0, 0, time.UTC), 2*time.Hour),
	}
	if err := s.BulkUpsert(batch); err != nil {
		t.Fatalf("BulkUpsert: %v", err)
	}
	// Re-Run mit denselben IDs ist ein No-op, kein Fehler, keine Duplikate
	if err := s.BulkUpsert(batch); err != nil {
		t.Fatalf("BulkUpsert rerun: %v", err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	list, _ := s.ListByUserDateRange(uid, from, to)
	if len(list) != 2 {
		t.Errorf("after bulk rerun: want 2 sessions, got %d", len(list))
	}
}

func TestBookingDay_UserTimezone(t *testing.T) {
	t.Parallel()
	berlin, _ := time.LoadLocation("Europe/Berlin")
	// 22:30 UTC am 11.6. = 00:30 am 12.6. in Berlin (CEST) → Buchungstag 12.6.
	started := time.Date(2026, 6, 11, 22, 30, 0, 0, time.UTC)
	day := pgstore.BookingDay(started, berlin)
	if day.Format("2006-01-02") != "2026-06-12" {
		t.Errorf("BookingDay: got %s want 2026-06-12", day.Format("2006-01-02"))
	}
}
