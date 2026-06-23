package apiclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

func TestClient_AddAndListDayOffs(t *testing.T) {
	var gotPost bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/dayoffs":
			gotPost = true
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/dayoffs":
			_, _ = w.Write([]byte(`[{"day":"2026-06-15","kind":"vacation","label":"Sommer","targetMin":0,"holiday":false}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)

	c := apiclient.New(ts.URL, "tok")
	if err := c.AddDayOffs(context.Background(), "2026-06-15", "2026-06-19", "vacation", "Sommer", 0, true); err != nil {
		t.Fatalf("add: %v", err)
	}
	if !gotPost {
		t.Fatal("POST not issued")
	}
	list, err := c.ListDayOffs(context.Background(), "2026-06-01", "2026-06-30")
	if err != nil || len(list) != 1 || list[0].Label != "Sommer" {
		t.Fatalf("list = %+v err=%v", list, err)
	}
}

func TestClient_DeleteDayOff(t *testing.T) {
	var gotMethod, gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(ts.Close)

	c := apiclient.New(ts.URL, "tok")
	if err := c.DeleteDayOff(context.Background(), "2026-06-20"); err != nil {
		t.Fatalf("DeleteDayOff: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method: got %s, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/dayoffs/2026-06-20" {
		t.Errorf("path: got %s, want /api/v1/dayoffs/2026-06-20", gotPath)
	}
}

func TestClient_GetSettings(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method: got %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/settings" {
			t.Errorf("path: got %s, want /api/v1/settings", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"bundesland":"BY","feedUrls":["https://example.com/feed.ics"],"defaultTargetMin":480,"weekdayTargetMin":{"monday":480}}`))
	}))
	t.Cleanup(ts.Close)

	c := apiclient.New(ts.URL, "tok")
	s, err := c.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s.Bundesland != "BY" {
		t.Errorf("Bundesland: got %q, want BY", s.Bundesland)
	}
	if s.DefaultTargetMin != 480 {
		t.Errorf("DefaultTargetMin: got %d, want 480", s.DefaultTargetMin)
	}
	if len(s.FeedURLs) != 1 || s.FeedURLs[0] != "https://example.com/feed.ics" {
		t.Errorf("FeedURLs: got %v", s.FeedURLs)
	}
	if s.WeekdayTargetMin["monday"] != 480 {
		t.Errorf("WeekdayTargetMin[monday]: got %d, want 480", s.WeekdayTargetMin["monday"])
	}
}

func TestClient_SetBundesland(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody struct {
		Bundesland string `json:"bundesland"`
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(ts.Close)

	c := apiclient.New(ts.URL, "tok")
	if err := c.SetBundesland(context.Background(), "NW"); err != nil {
		t.Fatalf("SetBundesland: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method: got %s, want POST", gotMethod)
	}
	if gotPath != "/api/v1/settings/bundesland" {
		t.Errorf("path: got %s, want /api/v1/settings/bundesland", gotPath)
	}
	if gotBody.Bundesland != "NW" {
		t.Errorf("bundesland: got %q, want NW", gotBody.Bundesland)
	}
}

func TestClient_RegenIcsToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method: got %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/ics-token/regenerate" {
			t.Errorf("path: got %s, want /api/v1/ics-token/regenerate", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"newtoken123","feedUrl":"https://flow.example.com/ics/newtoken123.ics"}`))
	}))
	t.Cleanup(ts.Close)

	c := apiclient.New(ts.URL, "tok")
	feedURL, err := c.RegenIcsToken(context.Background())
	if err != nil {
		t.Fatalf("RegenIcsToken: %v", err)
	}
	if feedURL != "https://flow.example.com/ics/newtoken123.ics" {
		t.Errorf("feedURL: got %q, want https://flow.example.com/ics/newtoken123.ics", feedURL)
	}
}
