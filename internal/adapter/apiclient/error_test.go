package apiclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

// TestAPIError_IncludesResponseBody — a non-2xx response's body (e.g. the friendly
// "a sibling node already uses this slug" 409) must reach the caller's error string,
// not just the bare status code, so TUI/CLI surfaces show why a call failed.
func TestAPIError_IncludesResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "a sibling node already uses this slug", http.StatusConflict)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tkn")
	_, err := c.CreateNode(context.Background(), apiclient.CreateNodeFields{Name: "x", Kind: "repo"})
	if err == nil {
		t.Fatal("want an error for a 409 response")
	}
	if !apiclient.IsConflict(err) {
		t.Errorf("want IsConflict true, got %v", err)
	}
	if !strings.Contains(err.Error(), "a sibling node already uses this slug") {
		t.Errorf("error should carry the response body, got: %v", err)
	}
}
