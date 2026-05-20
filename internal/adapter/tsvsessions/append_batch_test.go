package tsvsessions_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/tsvsessions"
	"github.com/serverkraken/flow/internal/domain"
)

// AppendBatch atomically appends multiple sessions to the log. The
// pre-existing test suite covers single-Append; this test pins the
// batched variant introduced by review finding B1.

func TestAppendBatch_EmptyIsNoop(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "sessions.log")
	s := tsvsessions.New(path)
	if err := s.AppendBatch(nil); err != nil {
		t.Errorf("AppendBatch(nil): %v", err)
	}
	if _, err := s.LoadAll(); err != nil {
		t.Errorf("LoadAll after empty batch: %v", err)
	}
}

func TestAppendBatch_PersistsAllSessions(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "sessions.log")
	s := tsvsessions.New(path)
	batch := []domain.Session{
		mkSession(t, "2026-05-01", "08:00", "12:00", 4*3600, "deep", ""),
		mkSession(t, "2026-05-01", "13:00", "17:00", 4*3600, "ops", "review"),
		mkSession(t, "2026-05-02", "09:00", "11:00", 2*3600, "", ""),
	}
	if err := s.AppendBatch(batch); err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}
	got, err := s.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d sessions, want 3", len(got))
	}
	if !strings.EqualFold(got[1].Tag, "ops") {
		t.Errorf("ops tag missing in second session: %+v", got[1])
	}
}

func TestAppendBatch_AppendsToExistingFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "sessions.log")
	s := tsvsessions.New(path)
	first := []domain.Session{mkSession(t, "2026-05-01", "08:00", "12:00", 4*3600, "", "")}
	if err := s.AppendBatch(first); err != nil {
		t.Fatalf("AppendBatch first: %v", err)
	}
	second := []domain.Session{mkSession(t, "2026-05-01", "13:00", "17:00", 4*3600, "", "")}
	if err := s.AppendBatch(second); err != nil {
		t.Fatalf("AppendBatch second: %v", err)
	}
	got, err := s.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected accumulation, got %d sessions", len(got))
	}
}
