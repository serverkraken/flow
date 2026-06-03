package httpsync_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpsync"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// fakeTokenStore is an in-test ports.TokenStore.
type fakeTokenStore struct {
	token string
	err   error
}

func (f *fakeTokenStore) Get(_ string) (ports.Tokens, error) {
	if f.err != nil {
		return ports.Tokens{}, f.err
	}
	return ports.Tokens{AccessToken: f.token}, nil
}
func (f *fakeTokenStore) Put(_ string, _ ports.Tokens) error { return nil }
func (f *fakeTokenStore) Delete(_ string) error              { return nil }

// newClient builds a Client pointed at srv with the given token.
func newClient(srv *httptest.Server, token string) *httpsync.Client {
	store := &fakeTokenStore{token: token}
	return httpsync.NewClient(srv.URL, store, "test")
}

// ---- PullSessions ----

func TestPullSessions_HappyPath(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	items := []domain.Session{{ID: "s1", Version: 3, Date: now}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items":          items,
			"high_watermark": int64(7),
			"has_more":       false,
		})
	}))
	defer srv.Close()

	c := newClient(srv, "tok")
	got, hw, hasMore, err := c.PullSessions(context.Background(), 0, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "s1" {
		t.Errorf("unexpected items: %v", got)
	}
	if hw != 7 {
		t.Errorf("high_watermark: got %d, want 7", hw)
	}
	if hasMore {
		t.Error("has_more should be false")
	}
}

func TestPullSessions_URLParams(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []domain.Session{}, "high_watermark": int64(0), "has_more": false,
		})
	}))
	defer srv.Close()

	c := newClient(srv, "tok")
	_, _, _, err := c.PullSessions(context.Background(), 42, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotURL != "/api/v1/sessions?since=42&limit=100" {
		t.Errorf("unexpected URL: %s", gotURL)
	}
}

// ---- PushSession ----

func TestPushSession_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(domain.Session{ID: "s1", Version: 5})
	}))
	defer srv.Close()

	c := newClient(srv, "tok")
	ver, err := c.PushSession(context.Background(), domain.Session{ID: "s1"}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != 5 {
		t.Errorf("version: got %d, want 5", ver)
	}
}

func TestPushSession_409_ConflictError(t *testing.T) {
	current := domain.Session{ID: "s1", Version: 3}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"current": current})
	}))
	defer srv.Close()

	c := newClient(srv, "tok")
	_, err := c.PushSession(context.Background(), domain.Session{ID: "s1"}, 2)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ports.ErrSessionVersionConflict) {
		t.Errorf("errors.Is(ErrSessionVersionConflict): got false for %v", err)
	}
	var ce *httpsync.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ConflictError, got %T", err)
	}
	if ce.Current == nil {
		t.Error("ConflictError.Current should not be nil")
	}
}

// ---- Token not found ----

func TestClient_TokenNotFound_ReturnsErrUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := &fakeTokenStore{err: ports.ErrTokenNotFound}
	c := httpsync.NewClient(srv.URL, store, "test")
	_, _, _, err := c.PullSessions(context.Background(), 0, 50)
	if !errors.Is(err, httpsync.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

// ---- 401 from server ----

func TestClient_401_ReturnsErrUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := newClient(srv, "tok")
	_, _, _, err := c.PullSessions(context.Background(), 0, 50)
	if !errors.Is(err, httpsync.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

// ---- 5xx from server ----

func TestClient_5xx_ReturnsGenericError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	c := newClient(srv, "tok")
	_, _, _, err := c.PullSessions(context.Background(), 0, 50)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, httpsync.ErrUnauthorized) {
		t.Error("should not be ErrUnauthorized")
	}
}

// ---- PullActive — no since param when since==0 ----

func TestPullActive_NoSinceParam_WhenZero(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []domain.ActiveSession{}, "high_watermark": int64(0),
		})
	}))
	defer srv.Close()

	c := newClient(srv, "tok")
	_, _, err := c.PullActive(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotURL != "/api/v1/active" {
		t.Errorf("unexpected URL: %s (want /api/v1/active)", gotURL)
	}
}

func TestPullActive_WithSince(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []domain.ActiveSession{}, "high_watermark": int64(5),
		})
	}))
	defer srv.Close()

	c := newClient(srv, "tok")
	_, hw, err := c.PullActive(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotURL != "/api/v1/active?since=3" {
		t.Errorf("unexpected URL: %s", gotURL)
	}
	if hw != 5 {
		t.Errorf("high_watermark: got %d, want 5", hw)
	}
}

// ---- PullProjects ----

func TestPullProjects_HappyPath(t *testing.T) {
	items := []domain.Project{{ID: "p1", Version: 2}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": items, "high_watermark": int64(2), "has_more": false,
		})
	}))
	defer srv.Close()

	c := newClient(srv, "tok")
	got, hw, hasMore, err := c.PullProjects(context.Background(), 0, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "p1" {
		t.Errorf("unexpected items: %v", got)
	}
	if hw != 2 || hasMore {
		t.Errorf("hw=%d hasMore=%v", hw, hasMore)
	}
}

func TestPushProject_409_ConflictError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"current": domain.Project{ID: "p1", Version: 1}})
	}))
	defer srv.Close()

	c := newClient(srv, "tok")
	_, err := c.PushProject(context.Background(), domain.Project{ID: "p1"}, 0)
	if !errors.Is(err, ports.ErrProjectVersionConflict) {
		t.Errorf("expected ErrProjectVersionConflict, got %v", err)
	}
}

// ---- StartActive ----

func TestStartActive_HappyPath(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	expected := domain.ActiveSession{ProjectID: "p1", Version: 1, StartedAt: now}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	c := newClient(srv, "tok")
	got, err := c.StartActive(context.Background(), "p1", "laptop", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ProjectID != "p1" {
		t.Errorf("ProjectID: got %s, want p1", got.ProjectID)
	}
}

func TestStartActive_409_ConflictError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"current": domain.ActiveSession{ProjectID: "p1", Version: 2}})
	}))
	defer srv.Close()

	c := newClient(srv, "tok")
	_, err := c.StartActive(context.Background(), "p1", "laptop", 0)
	if !errors.Is(err, ports.ErrActiveSessionConflict) {
		t.Errorf("expected ErrActiveSessionConflict, got %v", err)
	}
}

// ---- StopActive ----

func TestStopActive_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(domain.Session{ID: "s1", Version: 1})
	}))
	defer srv.Close()

	c := newClient(srv, "tok")
	sess, err := c.StopActive(context.Background(), "p1", 1, "deep", "done")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess.ID != "s1" {
		t.Errorf("session ID: got %s, want s1", sess.ID)
	}
}

func TestStopActive_404_ErrActiveSessionNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := newClient(srv, "tok")
	_, err := c.StopActive(context.Background(), "p1", 1, "", "")
	if !errors.Is(err, ports.ErrActiveSessionNotFound) {
		t.Errorf("expected ErrActiveSessionNotFound, got %v", err)
	}
}

func TestStopActive_409_ConflictError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"current": domain.ActiveSession{ProjectID: "p1", Version: 2}})
	}))
	defer srv.Close()

	c := newClient(srv, "tok")
	_, err := c.StopActive(context.Background(), "p1", 1, "", "")
	if !errors.Is(err, ports.ErrActiveSessionConflict) {
		t.Errorf("expected ErrActiveSessionConflict, got %v", err)
	}
}
