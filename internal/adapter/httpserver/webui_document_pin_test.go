package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// TestWebDocPin_TogglesAndEmitsSSE covers the Anpinnen button's round-trip
// (POST /wissen/{id}/pin): SetPinned flips the current state, document.updated
// is emitted on the owner's SSE bus, and the returned fragment reflects the
// new Pinned label ("Angepinnt" once pinned, back to "Anpinnen" on a second
// toggle).
func TestWebDocPin_TogglesAndEmitsSSE(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u1", Type: domain.DocFree, Path: "p/x",
		Title: "X", Body: "b", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	ch, cancel := srv.Bus.Subscribe("u1")
	defer cancel()

	body, status := postWissenPin(t, srv, codec, "u1", "d1")
	if status != http.StatusOK {
		t.Fatalf("POST /wissen/d1/pin status=%d body=%.400s", status, body)
	}
	if !strings.Contains(body, "Angepinnt") {
		t.Fatalf("expected pinned fragment to show Angepinnt, got %.600s", body)
	}
	select {
	case ev := <-ch:
		if ev.Type != domain.EventDocumentUpdated {
			t.Errorf("want event type %q, got %q", domain.EventDocumentUpdated, ev.Type)
		}
		if id, _ := ev.Data["id"].(string); id != "d1" {
			t.Errorf("want event id %q, got %q", "d1", id)
		}
	default:
		t.Error("want document.updated SSE event after pin, got none")
	}

	body2, status2 := postWissenPin(t, srv, codec, "u1", "d1")
	if status2 != http.StatusOK {
		t.Fatalf("second POST /wissen/d1/pin status=%d body=%.400s", status2, body2)
	}
	if !strings.Contains(body2, "Anpinnen") || strings.Contains(body2, "Angepinnt") {
		t.Fatalf("expected second toggle to unpin (Anpinnen, not Angepinnt), got %.600s", body2)
	}
}

func TestWebDocPin_NotFound(t *testing.T) {
	srv, codec, _, _ := newWebWissenServer(t)
	_, status := postWissenPin(t, srv, codec, "u1", "no-such-id")
	if status != http.StatusNotFound {
		t.Fatalf("POST /wissen/no-such-id/pin status=%d, want 404", status)
	}
}

// TestWebDocPin_OwnerScoped mirrors TestWebWissenDocumentView_OwnerScoped for
// the mutating Anpinnen route: a second tenant must not be able to pin (or
// even discover the existence of) another owner's document.
func TestWebDocPin_OwnerScoped(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	_, _ = docs.Create(ctx, domain.Document{
		ID: "secret", OwnerID: "u1", Type: domain.DocFree, Path: "p/secret",
		Title: "Secret", Body: "shh", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	_, status := postWissenPin(t, srv, codec, "u2", "secret")
	if status != http.StatusNotFound {
		t.Fatalf("u2 POST /wissen/secret/pin status=%d, want 404 (owner-scoped)", status)
	}
}

func postWissenPin(t *testing.T, s *Server, codec SessionCodec, userID, id string) (string, int) {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("POST /wissen/{id}/pin", s.webAuth(http.HandlerFunc(s.handleWebDocPin)))
	cookieVal, err := codec.Issue(userID)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/wissen/"+id+"/pin", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookieVal})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Body.String(), rec.Code
}
