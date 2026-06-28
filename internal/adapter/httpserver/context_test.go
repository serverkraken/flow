package httpserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/usecase"
)

func TestHandleGetContext_UnresolvedReturns200Global(t *testing.T) {
	srv, _ := newDocServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)

	res := doDoc(t, ts, "GET", "/api/v1/context?remote=does-not-exist", "")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("unresolved context must be 200, got %d", res.StatusCode)
	}
	var cc usecase.ComposedContext
	if err := json.NewDecoder(res.Body).Decode(&cc); err != nil {
		t.Fatal(err)
	}
	if !cc.Resolution.Unresolved {
		t.Errorf("want Unresolved=true for unknown repo")
	}
}
