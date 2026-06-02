package sqliteclient

import (
	"path/filepath"
	"testing"
)

func TestUnit_OpenStore_RunsMigrations_CreatesAllTables(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "client.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	wantTables := []string{
		"users", "projects", "sessions", "active_sessions",
		"repos", "repo_notes", "sync_state", "write_queue",
	}
	for _, name := range wantTables {
		var got string
		row := s.DB().QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name)
		if err := row.Scan(&got); err != nil {
			t.Errorf("table %q missing: %v", name, err)
		}
	}
}

func TestUnit_OpenStore_ReentrantOnSameFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "client.db")

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	_ = s1.Close()

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open should be idempotent: %v", err)
	}
	_ = s2.Close()
}

func TestUnit_OpenStore_EnablesForeignKeys(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "client.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	var on int
	if err := s.DB().QueryRow("PRAGMA foreign_keys").Scan(&on); err != nil {
		t.Fatalf("PRAGMA: %v", err)
	}
	if on != 1 {
		t.Errorf("foreign_keys = %d, want 1", on)
	}
}
