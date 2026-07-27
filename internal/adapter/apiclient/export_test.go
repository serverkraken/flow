package apiclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

func ptrInt64(v int64) *int64 { return &v }

func TestExport_FetchesBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/export" {
			t.Errorf("bad path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("format") != "md" {
			t.Errorf("bad format: %s", r.URL.Query().Get("format"))
		}
		if r.URL.Query().Get("from") != "2026-06-01" {
			t.Errorf("bad from: %s", r.URL.Query().Get("from"))
		}
		if r.URL.Query().Get("to") != "2026-06-30" {
			t.Errorf("bad to: %s", r.URL.Query().Get("to"))
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing/wrong Authorization header: %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte("# Worktime\n"))
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	b, err := c.Export(context.Background(), "2026-06-01", "2026-06-30", "md", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(b) != "# Worktime\n" {
		t.Fatalf("got %q, want %q", string(b), "# Worktime\n")
	}
}

func TestExport_WithProjectID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("project") != "p42" {
			t.Errorf("missing project param: %s", r.URL.Query().Get("project"))
		}
		if r.URL.Query().Get("from") != "2026-06-01" {
			t.Errorf("bad from: %s", r.URL.Query().Get("from"))
		}
		if r.URL.Query().Get("to") != "2026-06-30" {
			t.Errorf("bad to: %s", r.URL.Query().Get("to"))
		}
		_, _ = w.Write([]byte("date,minutes\n"))
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	b, err := c.Export(context.Background(), "2026-06-01", "2026-06-30", "csv", "p42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(b) != "date,minutes\n" {
		t.Fatalf("got %q", string(b))
	}
}

func TestExport_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	_, err := c.Export(context.Background(), "2026-06-01", "2026-06-30", "csv", "")
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected status 401 in error, got: %v", err)
	}
}

func TestSetProjectRate_PostsBody(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody struct {
		Amount   *int64 `json:"amount"`
		Currency string `json:"currency"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	err := c.SetNodeRate(context.Background(), "p1", ptrInt64(8000), "EUR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method: got %s, want POST", gotMethod)
	}
	if gotPath != "/api/v1/nodes/p1/rate" {
		t.Errorf("path: got %s, want /api/v1/nodes/p1/rate", gotPath)
	}
	if gotBody.Amount == nil || *gotBody.Amount != 8000 {
		t.Errorf("amount: got %v, want 8000", gotBody.Amount)
	}
	if gotBody.Currency != "EUR" {
		t.Errorf("currency: got %s, want EUR", gotBody.Currency)
	}
}

func TestSetProjectRate_ClearsRate(t *testing.T) {
	var gotBody struct {
		Amount   *int64 `json:"amount"`
		Currency string `json:"currency"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	err := c.SetNodeRate(context.Background(), "p1", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody.Amount != nil {
		t.Errorf("amount: got %v, want nil", gotBody.Amount)
	}
	if gotBody.Currency != "" {
		t.Errorf("currency: got %q, want empty string", gotBody.Currency)
	}
}
