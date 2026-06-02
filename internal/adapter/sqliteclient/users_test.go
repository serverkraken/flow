package sqliteclient

import (
	"errors"
	"path/filepath"
	"testing"

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
