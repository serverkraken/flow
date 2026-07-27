package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func allowMsoent(id ports.Identity) bool { return id.Subject == "msoent" }

func TestEnsureUserRejectsNonAllowlisted(t *testing.T) {
	uc := usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: &testutil.FakeIDGen{}, Allow: allowMsoent}
	if _, err := uc.Execute(context.Background(), ports.Identity{Subject: "eve"}); !errors.Is(err, usecase.ErrNotAllowed) {
		t.Fatalf("want ErrNotAllowed, got %v", err)
	}
}

var errBoom = errors.New("boom")

type boomStore struct{ upsertCalled bool }

func (s *boomStore) GetBySub(context.Context, string) (domain.User, error) {
	return domain.User{}, errBoom
}

func (s *boomStore) UpsertBySub(_ context.Context, u domain.User) (domain.User, error) {
	s.upsertCalled = true
	return u, nil
}

func (s *boomStore) GetByID(context.Context, string) (domain.User, error) {
	return domain.User{}, ports.ErrUserNotFound
}

func TestEnsureUserPropagatesStoreError(t *testing.T) {
	store := &boomStore{}
	uc := usecase.EnsureUser{Users: store, IDs: &testutil.FakeIDGen{}, Allow: allowMsoent}
	_, err := uc.Execute(context.Background(), ports.Identity{Subject: "msoent"})
	if !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom propagated, got %v", err)
	}
	if store.upsertCalled {
		t.Fatal("upsert must not be called when GetBySub errors")
	}
}

func TestEnsureUserCreatesOnFirstLoginThenReturnsExisting(t *testing.T) {
	store := testutil.NewFakeUserStore()
	uc := usecase.EnsureUser{Users: store, IDs: &testutil.FakeIDGen{}, Allow: allowMsoent}
	id := ports.Identity{Subject: "msoent", Username: "msoent", Email: "m@x.de", Name: "Martin"}

	u1, err := uc.Execute(context.Background(), id)
	if err != nil || u1.ID == "" {
		t.Fatalf("first login: %+v %v", u1, err)
	}
	u2, err := uc.Execute(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if u2.ID != u1.ID {
		t.Fatalf("second login should return same user: %q vs %q", u1.ID, u2.ID)
	}
}
