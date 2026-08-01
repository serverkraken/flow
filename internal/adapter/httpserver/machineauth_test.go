package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// machineTestServer wires a Server whose verifier returns the given identity,
// with one owner already present in the store.
func machineTestServer(t *testing.T, id ports.Identity) (*Server, domain.User) {
	t.Helper()
	users := testutil.NewFakeUserStore()
	owner, err := domain.NewUser("owner-id", "owner-sub", "soenne", "s@x.de", "Soenne")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.UpsertBySub(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	return &Server{
		Verifier: testutil.FakeVerifier{ID: id},
		Users:    users,
		Ensure: usecase.EnsureUser{
			Users: users,
			IDs:   &testutil.FakeIDGen{},
			Allow: func(ports.Identity) bool { return true },
		},
		Machines: map[string]MachineAccount{
			"machine-sub": {OwnerSub: "owner-sub", Label: "wartung-agent"},
		},
	}, owner
}

func machineProbe(gotUser *domain.User, gotActor *actor.Actor) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotUser, _ = userFrom(r.Context())
		*gotActor = actor.FromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
}

func doBearer(h http.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/documents", nil)
	req.Header.Set("Authorization", "Bearer t")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMachineTokenOnOptedInRouteDelegatesToOwner(t *testing.T) {
	srv, owner := machineTestServer(t, ports.Identity{Subject: "machine-sub", Machine: true})
	var gotUser domain.User
	var gotActor actor.Actor

	rec := doBearer(srv.authMachineOK(machineProbe(&gotUser, &gotActor)))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body %q)", rec.Code, rec.Body.String())
	}
	if gotUser.ID != owner.ID {
		t.Fatalf("delegated user = %q, want the owner %q", gotUser.ID, owner.ID)
	}
	if gotActor.Kind != actor.Agent || gotActor.Ref != "wartung-agent" {
		t.Fatalf("actor = %+v, want {agent wartung-agent}", gotActor)
	}
}

func TestMachineTokenNeverCreatesItsOwnUser(t *testing.T) {
	srv, _ := machineTestServer(t, ports.Identity{Subject: "machine-sub", Machine: true})
	var gotUser domain.User
	var gotActor actor.Actor

	doBearer(srv.authMachineOK(machineProbe(&gotUser, &gotActor)))

	// A user record for the MACHINE subject would be a second tenant — exactly
	// what delegation exists to avoid.
	if _, err := srv.Users.GetBySub(context.Background(), "machine-sub"); err == nil {
		t.Fatal("a user record was created for the machine subject")
	}
}

func TestMachineTokenRejectedOnPlainAuthRoute(t *testing.T) {
	srv, _ := machineTestServer(t, ports.Identity{Subject: "machine-sub", Machine: true})
	var gotUser domain.User
	var gotActor actor.Actor

	rec := doBearer(srv.auth(machineProbe(&gotUser, &gotActor)))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "machine tokens are not accepted on this route") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestMachineTokenOnAuthAnyIsForbiddenNotUnauthorized(t *testing.T) {
	srv, _ := machineTestServer(t, ports.Identity{Subject: "machine-sub", Machine: true})
	var gotUser domain.User
	var gotActor actor.Actor

	rec := doBearer(srv.authAny(machineProbe(&gotUser, &gotActor)))

	// Falling through to the cookie would answer 401 to a caller that did in
	// fact authenticate — a misleading answer the operator has to debug.
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestMachineTokenWithUnmappedSubject(t *testing.T) {
	srv, _ := machineTestServer(t, ports.Identity{Subject: "stranger", Machine: true})
	var gotUser domain.User
	var gotActor actor.Actor

	rec := doBearer(srv.authMachineOK(machineProbe(&gotUser, &gotActor)))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "machine token not mapped to an owner") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestMachineAccountMappedToUnknownOwner(t *testing.T) {
	srv, _ := machineTestServer(t, ports.Identity{Subject: "machine-sub", Machine: true})
	srv.Machines = map[string]MachineAccount{
		"machine-sub": {OwnerSub: "nobody", Label: "wartung-agent"},
	}
	var gotUser domain.User
	var gotActor actor.Actor

	rec := doBearer(srv.authMachineOK(machineProbe(&gotUser, &gotActor)))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `machine account "wartung-agent" maps to an unknown owner`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestHumanTokenUnaffectedByMachineWrappers(t *testing.T) {
	srv, _ := machineTestServer(t, ports.Identity{Subject: "owner-sub", Username: "soenne"})
	var gotUser domain.User
	var gotActor actor.Actor

	for name, h := range map[string]http.Handler{
		"auth":          srv.auth(machineProbe(&gotUser, &gotActor)),
		"authMachineOK": srv.authMachineOK(machineProbe(&gotUser, &gotActor)),
	} {
		rec := doBearer(h)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s: status = %d, want 204 (body %q)", name, rec.Code, rec.Body.String())
		}
		if gotActor.Kind != actor.Human {
			t.Fatalf("%s: actor = %+v, want a human", name, gotActor)
		}
	}
}
