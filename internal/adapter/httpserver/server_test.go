package httpserver_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func newServer() *httpserver.Server {
	store := testutil.NewFakeUserStore()
	return &httpserver.Server{
		Verifier: testutil.FakeVerifier{ID: ports.Identity{Subject: "msoent", Username: "msoent"}},
		Ensure:   usecase.EnsureUser{Users: store, IDs: &testutil.FakeIDGen{}, Allow: func(id ports.Identity) bool { return id.Subject == "msoent" }},
		Bus:      sse.NewBus(),
		Dev:      true,
	}
}

func TestHealth(t *testing.T) {
	srv := httptest.NewServer(newServer().Routes())
	defer srv.Close()
	res, err := http.Get(srv.URL + "/healthz")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("health: %v status=%v", err, res.StatusCode)
	}
}

func TestMeRequiresAuth(t *testing.T) {
	srv := httptest.NewServer(newServer().Routes())
	defer srv.Close()
	res, _ := http.Get(srv.URL + "/api/v1/me")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", res.StatusCode)
	}
}

func TestMeReturnsUser(t *testing.T) {
	srv := httptest.NewServer(newServer().Routes())
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer xyz")
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("me: %v status=%v", err, res.StatusCode)
	}
	body := make([]byte, 256)
	n, _ := res.Body.Read(body)
	if !strings.Contains(string(body[:n]), `"username":"msoent"`) {
		t.Fatalf("unexpected body: %s", body[:n])
	}
}

func TestEventsStreamsDebugPing(t *testing.T) {
	srv := httptest.NewServer(newServer().Routes())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/v1/events", nil)
	req.Header.Set("Authorization", "Bearer xyz")
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("events: %v status=%v", err, res.StatusCode)
	}
	defer res.Body.Close()

	// fire the ping after the stream is open
	go func() {
		time.Sleep(50 * time.Millisecond)
		pr, _ := http.NewRequest("POST", srv.URL+"/api/v1/debug/ping", nil)
		pr.Header.Set("Authorization", "Bearer xyz")
		_, _ = http.DefaultClient.Do(pr)
	}()

	sc := bufio.NewScanner(res.Body)
	deadline := time.Now().Add(3 * time.Second)
	for sc.Scan() {
		if strings.Contains(sc.Text(), "event: ping") {
			return // success
		}
		if time.Now().After(deadline) {
			break
		}
	}
	t.Fatal("did not receive ping event")
}
