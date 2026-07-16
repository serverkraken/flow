package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// TestAuthMiddlewareDerivesActorFromVerifiedUser verifies that bearer auth
// never accepts request metadata as audit provenance.
func TestAuthMiddlewareDerivesActorFromVerifiedUser(t *testing.T) {
	users := testutil.NewFakeUserStore()
	identity := ports.Identity{Subject: "sub-1", Username: "msoent"}
	u, _ := domain.NewUser("u1", identity.Subject, identity.Username, "m@x", "Martin Soentgenrath")
	_, _ = users.UpsertBySub(t.Context(), u)

	srv := &Server{
		Verifier: testutil.FakeVerifier{ID: identity},
		Ensure:   usecase.EnsureUser{Users: users, IDs: &testutil.FakeIDGen{}, Allow: func(ports.Identity) bool { return true }},
		Bus:      sse.NewBus(),
		Dev:      true,
	}

	var capturedActor actor.Actor
	probe := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedActor = actor.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := srv.auth(probe)

	// A caller-controlled actor header must not override the authenticated user.
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer xyz")
	req.Header.Set("X-Flow-Actor", "other-tenant-agent")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	wantHuman := actor.Actor{Kind: actor.Human, Ref: u.DisplayName}
	if capturedActor != wantHuman {
		t.Errorf("with spoofed X-Flow-Actor: got %+v, want %+v", capturedActor, wantHuman)
	}

	// Without X-Flow-Actor header: expect Human actor with display name.
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("Authorization", "Bearer xyz")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr2.Code)
	}
	if capturedActor != wantHuman {
		t.Errorf("without X-Flow-Actor: got %+v, want %+v", capturedActor, wantHuman)
	}
}
