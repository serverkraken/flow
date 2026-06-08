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
