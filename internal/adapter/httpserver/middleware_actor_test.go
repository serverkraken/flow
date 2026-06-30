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

// TestAuthMiddlewareSetsActor verifies that the auth middleware derives the
// correct actor from the X-Flow-Actor header and places it into the context.
func TestAuthMiddlewareSetsActor(t *testing.T) {
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

	// With X-Flow-Actor header: expect Agent actor.
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer xyz")
	req.Header.Set("X-Flow-Actor", "claude-code")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	wantAgent := actor.Actor{Kind: actor.Agent, Ref: "claude-code"}
	if capturedActor != wantAgent {
		t.Errorf("with X-Flow-Actor: got %+v, want %+v", capturedActor, wantAgent)
	}

	// Without X-Flow-Actor header: expect Human actor with display name.
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("Authorization", "Bearer xyz")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr2.Code)
	}
	wantHuman := actor.Actor{Kind: actor.Human, Ref: u.DisplayName}
	if capturedActor != wantHuman {
		t.Errorf("without X-Flow-Actor: got %+v, want %+v", capturedActor, wantHuman)
	}
}
