package httpserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// TestSession_MultiTagsRoundTrip Nachbucht a past session with two tags and
// asserts both come back on the JSON response — proving the Tag string → Tags
// []string cutover plus the StartSession/AddSession TagStore wiring (B2 D1).
func TestSession_MultiTagsRoundTrip(t *testing.T) {
	srv, _ := newDocServer(t) // StartSession/AddSession/EditSession wired with FakeTagStore
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)
	// Nachbuchen a past session with two tags.
	body := `{"start":"2026-06-14T09:00:00Z","stop":"2026-06-14T11:00:00Z","tags":["deep","django"],"note":"x"}`
	res := doDoc(t, ts, "POST", "/api/v1/sessions", body)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d", res.StatusCode)
	}
	var s domain.WorkSession
	_ = json.NewDecoder(res.Body).Decode(&s)
	if len(s.Tags) != 2 {
		t.Fatalf("want 2 session tags, got %+v", s.Tags)
	}
}
