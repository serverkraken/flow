package httpserver_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
)

func TestReadyzNilReady(t *testing.T) {
	// Ready == nil means always-ready: must return 200.
	srv := &httpserver.Server{
		Bus: sse.NewBus(),
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200 when Ready=nil, got %d", res.StatusCode)
	}
}

func TestReadyzReadyOK(t *testing.T) {
	// Ready returns nil → 200.
	srv := &httpserver.Server{
		Bus:   sse.NewBus(),
		Ready: func(_ context.Context) error { return nil },
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
}

func TestReadyzReadyError(t *testing.T) {
	// Ready returns an error → 503.
	srv := &httpserver.Server{
		Bus:   sse.NewBus(),
		Ready: func(_ context.Context) error { return errors.New("db unreachable") },
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", res.StatusCode)
	}
}
