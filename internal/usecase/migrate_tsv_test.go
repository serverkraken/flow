package usecase_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// migrateUserStore is a minimal ports.UserStore for migration tests.
// Only GetByID is exercised by MigrateTSV.Run (to verify the user exists).
type migrateUserStore struct {
	user domain.User
}

var _ ports.UserStore = (*migrateUserStore)(nil)

func (f *migrateUserStore) EnsureBySub(_, _, _ string) (domain.User, error) {
	return f.user, nil
}

func (f *migrateUserStore) GetByID(id string) (domain.User, error) {
	if id == f.user.ID {
		return f.user, nil
	}
	return domain.User{}, ports.ErrUserNotFound
}

func (f *migrateUserStore) GetBySub(_ string) (domain.User, error) {
	return f.user, nil
}

// writeTSV writes rows to path in the legacy worktime.log format.
// Each row is already a formatted string; rows are joined with "\n".
func writeTSV(t *testing.T, path string, rows []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("writeTSV mkdir: %v", err)
	}
	content := ""
	for _, r := range rows {
		content += r + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTSV write: %v", err)
	}
}

// makeMigrateTSV builds a MigrateTSV use case backed by in-memory fakes
// with user ID "u1".
func makeMigrateTSV() (uc *usecase.MigrateTSV, projects *testutil.FakeProjectStore, sessions *testutil.FakeSessionStore) {
	users := &migrateUserStore{user: domain.User{ID: "u1"}}
	projects = &testutil.FakeProjectStore{}
	sessions = &testutil.FakeSessionStore{}
	uc = usecase.NewMigrateTSV(users, projects, sessions)
	return uc, projects, sessions
}

// TestMigrateTSV_EmptyTSV verifies a file with no data rows inserts 0 sessions
// but still creates the default project and archives the file.
func TestMigrateTSV_EmptyTSV(t *testing.T) {
	dir := t.TempDir()
	tsvPath := filepath.Join(dir, "worktime.log")
	// Write only comments and blank lines.
	writeTSV(t, tsvPath, []string{"# comment", "", "   "})

	uc, projects, sessions := makeMigrateTSV()
	res, err := uc.Run("u1", tsvPath, "Allgemein")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Inserted != 0 {
		t.Errorf("Inserted: got %d, want 0", res.Inserted)
	}
	if res.DefaultProject.Slug == "" {
		t.Errorf("DefaultProject not set")
	}
	if res.ArchivedTo == "" {
		t.Errorf("ArchivedTo should be set")
	}
	// TSV renamed — original should be gone.
	if _, err := os.Stat(tsvPath); !os.IsNotExist(err) {
		t.Errorf("original TSV should have been renamed")
	}
	// Project created.
	if len(projects.Projects) != 1 {
		t.Errorf("expected 1 project, got %d", len(projects.Projects))
	}
	// No sessions inserted.
	if len(sessions.Sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions.Sessions))
	}
}

// TestMigrateTSV_ThreeRows verifies three valid rows are parsed and inserted.
func TestMigrateTSV_ThreeRows(t *testing.T) {
	dir := t.TempDir()
	tsvPath := filepath.Join(dir, "worktime.log")
	// date\tstart\tstop\telapsed[\ttag[\tnote]]
	// elapsed = integer seconds
	writeTSV(t, tsvPath, []string{
		"2026-05-01\t09:00\t10:30\t5400\tdeep\tmorning focus",
		"2026-05-01\t13:00\t14:00\t3600\t\t",
		"2026-05-02\t08:00\t09:15\t4500\tmeeting\tstandup",
	})

	uc, _, sessions := makeMigrateTSV()
	res, err := uc.Run("u1", tsvPath, "Allgemein")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Inserted != 3 {
		t.Errorf("Inserted: got %d, want 3", res.Inserted)
	}
	if len(sessions.Sessions) != 3 {
		t.Errorf("sessions in store: got %d, want 3", len(sessions.Sessions))
	}
	// All sessions belong to the default project.
	for _, s := range sessions.Sessions {
		if s.UserID != "u1" {
			t.Errorf("session UserID: %q, want u1", s.UserID)
		}
		if s.ProjectID == "" {
			t.Errorf("session ProjectID should be set")
		}
		if s.ID == "" {
			t.Errorf("session ID should be set (UUIDv5)")
		}
	}
}

// TestMigrateTSV_Idempotency verifies re-running the migration on the same rows
// yields the same UUIDs and does not create duplicates in the session store.
func TestMigrateTSV_Idempotency(t *testing.T) {
	dir := t.TempDir()
	tsvPath := filepath.Join(dir, "worktime.log")
	rows := []string{
		"2026-05-01\t09:00\t10:30\t5400\tdeep\tmorning focus",
		"2026-05-01\t13:00\t14:00\t3600\t\t",
		"2026-05-02\t08:00\t09:15\t4500\tmeeting\tstandup",
	}
	writeTSV(t, tsvPath, rows)

	uc, _, sessions := makeMigrateTSV()
	res1, err := uc.Run("u1", tsvPath, "Allgemein")
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if res1.Inserted != 3 {
		t.Errorf("first run Inserted: got %d, want 3", res1.Inserted)
	}

	// Copy TSV back to simulate re-run on the same file.
	writeTSV(t, tsvPath, rows)

	res2, err := uc.Run("u1", tsvPath, "Allgemein")
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if res2.Inserted != 3 {
		t.Errorf("second run Inserted: got %d, want 3", res2.Inserted)
	}
	// UUIDv5 idempotency: store still has exactly 3 sessions.
	if len(sessions.Sessions) != 3 {
		t.Errorf("after re-run sessions in store: got %d, want 3 (no duplicates)", len(sessions.Sessions))
	}
}

// TestMigrateTSV_MalformedLines verifies that malformed rows are skipped and
// valid rows are still inserted.
func TestMigrateTSV_MalformedLines(t *testing.T) {
	dir := t.TempDir()
	tsvPath := filepath.Join(dir, "worktime.log")
	writeTSV(t, tsvPath, []string{
		"not-a-date\t09:00\t10:00\t3600",
		"2026-05-01\t09:00\t10:30\t5400\tdeep\tmorning focus",
		"this is garbage",
		"2026-05-02\t08:00\t09:15\t4500",
	})

	uc, _, sessions := makeMigrateTSV()
	res, err := uc.Run("u1", tsvPath, "Allgemein")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Inserted != 2 {
		t.Errorf("Inserted: got %d, want 2", res.Inserted)
	}
	if res.SkippedMalformed != 2 {
		t.Errorf("SkippedMalformed: got %d, want 2", res.SkippedMalformed)
	}
	if len(sessions.Sessions) != 2 {
		t.Errorf("sessions in store: got %d, want 2", len(sessions.Sessions))
	}
}

// TestMigrateTSV_MissingFile verifies a graceful no-op when the TSV does not exist.
func TestMigrateTSV_MissingFile(t *testing.T) {
	dir := t.TempDir()
	tsvPath := filepath.Join(dir, "worktime.log") // does not exist

	uc, _, sessions := makeMigrateTSV()
	res, err := uc.Run("u1", tsvPath, "Allgemein")
	if err != nil {
		t.Fatalf("unexpected error for missing file: %v", err)
	}
	if res.Inserted != 0 {
		t.Errorf("Inserted: got %d, want 0", res.Inserted)
	}
	if res.ArchivedTo != "" {
		t.Errorf("ArchivedTo should be empty for missing file, got %q", res.ArchivedTo)
	}
	if len(sessions.Sessions) != 0 {
		t.Errorf("sessions in store: got %d, want 0", len(sessions.Sessions))
	}
}

// TestMigrateTSV_DifferentTagsSameProject verifies that sessions with different
// tags all go to the same default project (per spec: "alle in Allgemein").
func TestMigrateTSV_DifferentTagsSameProject(t *testing.T) {
	dir := t.TempDir()
	tsvPath := filepath.Join(dir, "worktime.log")
	writeTSV(t, tsvPath, []string{
		"2026-05-01\t09:00\t10:00\t3600\tdeep\t",
		"2026-05-01\t11:00\t12:00\t3600\tmeeting\t",
		"2026-05-01\t13:00\t14:00\t3600\treview\t",
	})

	uc, projects, sessions := makeMigrateTSV()
	res, err := uc.Run("u1", tsvPath, "Allgemein")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Inserted != 3 {
		t.Errorf("Inserted: got %d, want 3", res.Inserted)
	}
	// Exactly one project created.
	if len(projects.Projects) != 1 {
		t.Errorf("projects: got %d, want 1", len(projects.Projects))
	}
	projectID := projects.Projects[0].ID
	for _, s := range sessions.Sessions {
		if s.ProjectID != projectID {
			t.Errorf("session %q: ProjectID %q, want %q", s.ID, s.ProjectID, projectID)
		}
	}
}
