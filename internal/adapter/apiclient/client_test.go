package apiclient_test

import (
	"net/http"
	"net/http/httptest"
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
