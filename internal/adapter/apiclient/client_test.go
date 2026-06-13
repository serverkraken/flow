package apiclient_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

func TestWhoamiSendsBearerAndParses(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"id":"u1","username":"msoent","email":"m@x.de","displayName":"Martin"}`))
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok-123")
	u, err := c.Whoami(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer tok-123" {
		t.Fatalf("auth header: %q", gotAuth)
	}
	if u.Username != "msoent" {
		t.Fatalf("parse: %+v", u)
	}
}

func TestWhoamiNon200ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	_, err := c.Whoami(t.Context())
	if err == nil {
		t.Fatal("expected error on non-200")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error should mention status: %v", err)
	}
}
