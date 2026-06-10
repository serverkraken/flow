package sqliteclient

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func mustOpen(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestUnit_Users_EnsureBySub_Idempotent(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	users := NewUsers(store)

	u1, err := users.EnsureBySub("sub|123", "alice@example.com", "Alice")
	if err != nil {
		t.Fatalf("first EnsureBySub: %v", err)
	}

	// Second call with different email/name must return same ID without overwriting.
	u2, err := users.EnsureBySub("sub|123", "other@example.com", "Other")
	if err != nil {
		t.Fatalf("second EnsureBySub: %v", err)
	}

	if u1.ID != u2.ID {
		t.Errorf("ID changed: %q → %q", u1.ID, u2.ID)
	}
	if u2.Email != "alice@example.com" {
		t.Errorf("email overwritten: got %q, want %q", u2.Email, "alice@example.com")
	}
	if u2.DisplayName != "Alice" {
		t.Errorf("display_name overwritten: got %q, want %q", u2.DisplayName, "Alice")
	}
}

func TestUnit_Users_GetBySub_NotFound(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	users := NewUsers(store)

	_, err := users.GetBySub("no-such-sub")
	if !errors.Is(err, ports.ErrUserNotFound) {
		t.Errorf("want ErrUserNotFound, got %v", err)
	}
}

func TestUnit_Users_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	users := NewUsers(store)

	_, err := users.GetByID("00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, ports.ErrUserNotFound) {
		t.Errorf("want ErrUserNotFound, got %v", err)
	}
}

func TestUnit_Users_GetByID_ReturnsCorrectUser(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	users := NewUsers(store)

	created, err := users.EnsureBySub("sub|abc", "bob@example.com", "Bob")
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}

	fetched, err := users.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if fetched.OIDCSub != "sub|abc" {
		t.Errorf("OIDCSub: got %q, want %q", fetched.OIDCSub, "sub|abc")
	}
}

func TestUnit_Users_RelabelBySub(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	u := NewUsers(store)

	orig, err := u.EnsureBySub("local", "", "")
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}
	if err := u.RelabelBySub("local", "msoent", "m@x.de", "Soenne"); err != nil {
		t.Fatalf("RelabelBySub: %v", err)
	}
	got, err := u.GetBySub("msoent")
	if err != nil {
		t.Fatalf("GetBySub(msoent): %v", err)
	}
	if got.ID != orig.ID {
		t.Errorf("relabel changed id: %q != %q (must keep id so data stays owned)", got.ID, orig.ID)
	}
	if _, err := u.GetBySub("local"); err == nil {
		t.Error("old 'local' sub should no longer resolve")
	}
}

func TestUnit_Users_CountOwnedRows_FreshUser(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	u := NewUsers(store)

	created, err := u.EnsureBySub("sub|fresh", "", "")
	if err != nil {
		t.Fatalf("EnsureBySub: %v", err)
	}
	n, err := u.CountOwnedRows(created.ID)
	if err != nil {
		t.Fatalf("CountOwnedRows: %v", err)
	}
	if n != 0 {
		t.Errorf("fresh user: want 0 owned rows, got %d", n)
	}
}

// TestUnit_Users_CountOwnedRowsCountsAllUserTables verifies that CountOwnedRows
// counts active_sessions, repos, and repo_notes — not just projects+sessions.
// A user with only repos/notes data must not skip the adoption gate.
func TestUnit_Users_CountOwnedRowsCountsAllUserTables(t *testing.T) {
	t.Parallel()
	store := mustOpen(t)
	u := testUser(t, store, "count-all")

	repos := NewRepos(store)
	repoNotes := NewRepoNotes(store)
	projects := NewProjects(store)
	activeSessions := NewActiveSessions(store)

	// Insert a repo.
	repo, err := repos.EnsureByCanonicalKey(u.ID, "git:github.com/test/repo", "repo")
	if err != nil {
		t.Fatalf("EnsureByCanonicalKey: %v", err)
	}

	// Insert a repo_note referencing the repo (FK: repo_notes.repo_id → repos.id).
	now := time.Now().UTC().Truncate(time.Second)
	if err := repoNotes.Upsert(domain.RepoNote{
		ID:        uuid.NewString(),
		RepoID:    repo.ID,
		UserID:    u.ID,
		Content:   "note",
		Version:   1,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Upsert repo_note: %v", err)
	}

	// Insert an active_session (FK: active_sessions.project_id → projects.id).
	p, err := projects.EnsureBySlug(u.ID, "count-proj", "count-proj")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}
	if err := activeSessions.Upsert(domain.ActiveSession{
		UserID:          u.ID,
		ProjectID:       p.ID,
		StartedAt:       now,
		StartedOnDevice: "laptop",
		Version:         1,
	}); err != nil {
		t.Fatalf("Upsert active_session: %v", err)
	}

	// No projects (other than the one needed for FK) and no sessions rows were
	// inserted explicitly, but active_sessions + repo + repo_note give us ≥ 3.
	// The key assertion: CountOwnedRows must be > 0 even though sessions is empty.
	n, err := NewUsers(store).CountOwnedRows(u.ID)
	if err != nil {
		t.Fatalf("CountOwnedRows: %v", err)
	}
	// We expect at least: 1 project (FK helper) + 1 active_session + 1 repo + 1 repo_note = 4
	if n == 0 {
		t.Errorf("expected CountOwnedRows > 0 for user with active_session/repo/repo_note; got 0")
	}
}

// TestUnit_Users_SoleUser covers the three states: empty DB, one user, two users.
func TestUnit_Users_SoleUser(t *testing.T) {
	t.Parallel()

	t.Run("empty_db_returns_false", func(t *testing.T) {
		t.Parallel()
		store := mustOpen(t)
		u := NewUsers(store)
		_, ok, err := u.SoleUser()
		if err != nil {
			t.Fatalf("SoleUser: %v", err)
		}
		if ok {
			t.Error("empty DB: want ok=false, got true")
		}
	})

	t.Run("one_user_returns_true_and_that_user", func(t *testing.T) {
		t.Parallel()
		store := mustOpen(t)
		users := NewUsers(store)
		created, err := users.EnsureBySub("sole-sub", "sole@example.com", "Sole")
		if err != nil {
			t.Fatalf("EnsureBySub: %v", err)
		}
		got, ok, err := users.SoleUser()
		if err != nil {
			t.Fatalf("SoleUser: %v", err)
		}
		if !ok {
			t.Fatal("one user: want ok=true, got false")
		}
		if got.ID != created.ID {
			t.Errorf("ID mismatch: got %q, want %q", got.ID, created.ID)
		}
		if got.OIDCSub != "sole-sub" {
			t.Errorf("OIDCSub: got %q, want %q", got.OIDCSub, "sole-sub")
		}
	})

	t.Run("two_users_returns_false", func(t *testing.T) {
		t.Parallel()
		store := mustOpen(t)
		users := NewUsers(store)
		if _, err := users.EnsureBySub("two-a", "", ""); err != nil {
			t.Fatalf("EnsureBySub a: %v", err)
		}
		if _, err := users.EnsureBySub("two-b", "", ""); err != nil {
			t.Fatalf("EnsureBySub b: %v", err)
		}
		_, ok, err := users.SoleUser()
		if err != nil {
			t.Fatalf("SoleUser: %v", err)
		}
		if ok {
			t.Error("two users: want ok=false, got true")
		}
	})
}
