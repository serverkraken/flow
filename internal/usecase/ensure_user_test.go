package usecase_test

import (
	"context"
	"errors"
	"testing"

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
