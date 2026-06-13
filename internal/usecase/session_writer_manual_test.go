package usecase_test

// Manual-edit tests for SessionWriter: AddManual / Edit / Delete /
// SetTag / SetNote happy paths. Error propagation lives next door in
// session_writer_errors_test.go; lifecycle in
// session_writer_lifecycle_test.go.

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
)

func TestSessionWriter_AddManual_HappyPath(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	if err := w.AddManual(d, d.Add(9*time.Hour), d.Add(11*time.Hour)); err != nil {
		t.Fatal(err)
	}
	store := w.Sessions.(*testutil.FakeSessionStore)
	if len(store.Sessions) != 1 || store.Sessions[0].Elapsed != 2*time.Hour {
		t.Errorf("got %+v", store.Sessions)
	}
}

func TestSessionWriter_AddManual_StopBeforeStartFails(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	err := w.AddManual(d, d.Add(11*time.Hour), d.Add(9*time.Hour))
	if err == nil {
		t.Error("expected error")
	}
}

func TestSessionWriter_AddManual_OverlapDetected(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	// Pre-seed an existing 09:00–11:00 session.
	w.Sessions = &testutil.FakeSessionStore{Sessions: []domain.Session{
		{ID: "addm-overlap-1", Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour)},
	}}
	w.Reader.Sessions = w.Sessions

	err := w.AddManual(d, d.Add(10*time.Hour), d.Add(12*time.Hour))
	if !errors.Is(err, domain.ErrOverlap) {
		t.Errorf("expected ErrOverlap, got %v", err)
	}
}

func TestSessionWriter_Edit_PreservesTagAndNote(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	w.Sessions = &testutil.FakeSessionStore{Sessions: []domain.Session{
		{ID: "edit-pres-1", Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour), Tag: "deep", Note: "auth"},
	}}
	w.Reader.Sessions = w.Sessions

	if err := w.Edit(d, 0, d.Add(9*time.Hour), d.Add(12*time.Hour)); err != nil {
		t.Fatal(err)
	}
	store := w.Sessions.(*testutil.FakeSessionStore)
	if len(store.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(store.Sessions))
	}
	got := store.Sessions[0]
	if got.Stop.Hour() != 12 {
		t.Errorf("stop not updated: got %v", got.Stop)
	}
	if got.Tag != "deep" || got.Note != "auth" {
		t.Errorf("tag/note clobbered: %+v", got)
	}
}

func TestSessionWriter_Edit_OverlapDetected(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	w.Sessions = &testutil.FakeSessionStore{Sessions: []domain.Session{
		{ID: "edit-overlap-1", Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour)},
		{ID: "edit-overlap-2", Date: d, Start: d.Add(13 * time.Hour), Stop: d.Add(15 * time.Hour)},
	}}
	w.Reader.Sessions = w.Sessions

	// Edit session 0 to span 10:00–14:00 — overlaps session 1.
	err := w.Edit(d, 0, d.Add(10*time.Hour), d.Add(14*time.Hour))
	if !errors.Is(err, domain.ErrOverlap) {
		t.Errorf("expected ErrOverlap, got %v", err)
	}
}

func TestSessionWriter_Edit_StopBeforeStartFails(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	if err := w.Edit(d, 0, d.Add(11*time.Hour), d.Add(9*time.Hour)); err == nil {
		t.Error("expected error")
	}
}

func TestSessionWriter_Delete(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	w.Sessions = &testutil.FakeSessionStore{Sessions: []domain.Session{
		{ID: "del-1", Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour)},
		{ID: "del-2", Date: d, Start: d.Add(13 * time.Hour), Stop: d.Add(15 * time.Hour)},
	}}
	if err := w.Delete(d, 0); err != nil {
		t.Fatal(err)
	}
	store := w.Sessions.(*testutil.FakeSessionStore)
	if len(store.Sessions) != 1 {
		t.Fatalf("expected 1 session left, got %d", len(store.Sessions))
	}
	if store.Sessions[0].Start.Hour() != 13 {
		t.Errorf("wrong session deleted: %+v", store.Sessions[0])
	}
}

func TestSessionWriter_Delete_KeepsOtherDates(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d1, _ := time.ParseInLocation("2006-01-02", "2026-04-27", time.Local)
	d2, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	w.Sessions = &testutil.FakeSessionStore{Sessions: []domain.Session{
		{ID: "delkeep-d1", Date: d1, Start: d1.Add(9 * time.Hour), Stop: d1.Add(11 * time.Hour)},
		{ID: "delkeep-d2", Date: d2, Start: d2.Add(9 * time.Hour), Stop: d2.Add(11 * time.Hour)},
	}}
	if err := w.Delete(d2, 0); err != nil {
		t.Fatal(err)
	}
	store := w.Sessions.(*testutil.FakeSessionStore)
	if len(store.Sessions) != 1 || !store.Sessions[0].Date.Equal(d1) {
		t.Errorf("only d2's session should be deleted, got %+v", store.Sessions)
	}
}

func TestSessionWriter_SetTag(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	w.Sessions = &testutil.FakeSessionStore{Sessions: []domain.Session{
		{ID: "settag-1", Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour)},
	}}

	if err := w.SetTag(d, 0, "deep\ttab\nnewline"); err != nil {
		t.Fatal(err)
	}
	got := w.Sessions.(*testutil.FakeSessionStore).Sessions[0].Tag
	// tabs/newlines in tag must be sanitized to spaces.
	if got != "deep tab newline" {
		t.Errorf("Tag = %q, want sanitized", got)
	}
}

func TestSessionWriter_SetNote(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 0, 0, 0, time.Local)
	w := mkWriter(now)
	d, _ := time.ParseInLocation("2006-01-02", "2026-04-28", time.Local)
	w.Sessions = &testutil.FakeSessionStore{Sessions: []domain.Session{
		{ID: "setnote-1", Date: d, Start: d.Add(9 * time.Hour), Stop: d.Add(11 * time.Hour)},
	}}

	if err := w.SetNote(d, 0, "  some note  "); err != nil {
		t.Fatal(err)
	}
	if got := w.Sessions.(*testutil.FakeSessionStore).Sessions[0].Note; got != "some note" {
		t.Errorf("Note = %q, want trimmed", got)
	}
}
