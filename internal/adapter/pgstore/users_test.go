// internal/adapter/pgstore/users_test.go
package pgstore_test

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/ports"
)

func TestUsers_EnsureBySub_CreateThenUpdate(t *testing.T) {
	t.Parallel()
	u := pgstore.NewUsers(testStore)

	created, err := u.EnsureBySub("sub-users-1", "a@b.de", "Alice")
	if err != nil {
		t.Fatalf("EnsureBySub create: %v", err)
	}
	if created.ID == "" || created.OIDCSub != "sub-users-1" || created.Email != "a@b.de" {
		t.Fatalf("unexpected user: %+v", created)
	}

	updated, err := u.EnsureBySub("sub-users-1", "neu@b.de", "Alice Neu")
	if err != nil {
		t.Fatalf("EnsureBySub update: %v", err)
	}
	if updated.ID != created.ID {
		t.Errorf("ID changed on upsert: %s != %s", updated.ID, created.ID)
	}
	if updated.Email != "neu@b.de" || updated.DisplayName != "Alice Neu" {
		t.Errorf("fields not updated: %+v", updated)
	}
}

func TestUsers_Get_NotFound(t *testing.T) {
	t.Parallel()
	u := pgstore.NewUsers(testStore)
	if _, err := u.GetByID("00000000-0000-0000-0000-000000000000"); !errors.Is(err, ports.ErrUserNotFound) {
		t.Errorf("GetByID: want ErrUserNotFound, got %v", err)
	}
	if _, err := u.GetBySub("does-not-exist"); !errors.Is(err, ports.ErrUserNotFound) {
		t.Errorf("GetBySub: want ErrUserNotFound, got %v", err)
	}
}

func TestUsers_GetBySub_RoundTrip(t *testing.T) {
	t.Parallel()
	u := pgstore.NewUsers(testStore)
	created, _ := u.EnsureBySub("sub-users-2", "x@y.de", "X")
	got, err := u.GetBySub("sub-users-2")
	if err != nil {
		t.Fatalf("GetBySub: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID mismatch: %s != %s", got.ID, created.ID)
	}
}
