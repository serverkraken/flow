package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func TestParseClock_LocalRange(t *testing.T) {
	got, err := parseClock("2026-06-18", "09:30")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 6, 18, 9, 30, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if _, err := parseClock("2026-06-18", "bad"); err == nil {
		t.Fatal("want error for bad clock")
	}
	if _, err := parseClock("bad", "09:00"); err == nil {
		t.Fatal("want error for bad date")
	}
}

func TestRunSessionAdd_PostsBackfill(t *testing.T) {
	var posted map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode([]domain.Project{{ID: "p1", Name: "Acme"}})
		case r.Method == "POST" && r.URL.Path == "/api/v1/sessions":
			_ = json.NewDecoder(r.Body).Decode(&posted)
			_ = json.NewEncoder(w).Encode(domain.WorkSession{ID: "s1"})
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	out, err := runSessionAdd(context.Background(), c, sessionAddInput{
		Date: "2026-06-18", From: "09:00", To: "11:00", Project: "Acme",
	})
	if err != nil {
		t.Fatal(err)
	}
	if posted["projectId"] != "p1" {
		t.Errorf("projectId = %v, want p1", posted["projectId"])
	}
	if out == "" {
		t.Error("expected a summary line")
	}
}

func TestRunSessionAdd_RejectsToBeforeFrom(t *testing.T) {
	c := apiclient.New("http://unused", "tkn")
	if _, err := runSessionAdd(context.Background(), c, sessionAddInput{
		Date: "2026-06-18", From: "11:00", To: "09:00",
	}); err == nil {
		t.Fatal("want error when to <= from")
	}
}

func TestRunSessionList_RendersDay(t *testing.T) {
	pid := "p1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/projects":
			_ = json.NewEncoder(w).Encode([]domain.Project{{ID: "p1", Name: "Acme"}})
		case "/api/v1/sessions":
			start := time.Date(2026, 6, 18, 9, 0, 0, 0, time.Local)
			stop := time.Date(2026, 6, 18, 11, 0, 0, 0, time.Local)
			_ = json.NewEncoder(w).Encode([]domain.WorkSession{
				{ID: "s1", ProjectID: &pid, Start: start, Stop: &stop},
			})
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	out, err := runSessionList(context.Background(), c, "2026-06-18", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"s1", "09:00", "11:00", "02:00", "Acme"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
}

func TestRunSessionDelete(t *testing.T) {
	var deleted string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			deleted = r.URL.Path
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")
	if err := runSessionDelete(context.Background(), c, "s1"); err != nil {
		t.Fatal(err)
	}
	if deleted != "/api/v1/sessions/s1" {
		t.Fatalf("deleted path = %q", deleted)
	}
}

func TestRunSessionEdit_MergesOnlyChangedFields(t *testing.T) {
	pid := "p1"
	existingStart := time.Date(2026, 6, 18, 9, 0, 0, 0, time.Local)
	existingStop := time.Date(2026, 6, 18, 11, 0, 0, 0, time.Local)
	var patched map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/sessions":
			_ = json.NewEncoder(w).Encode([]domain.WorkSession{{
				ID: "s1", ProjectID: &pid, Tag: "old", Note: "keep",
				Start: existingStart, Stop: &existingStop,
			}})
		case r.Method == "PATCH" && r.URL.Path == "/api/v1/sessions/s1":
			_ = json.NewDecoder(r.Body).Decode(&patched)
			_ = json.NewEncoder(w).Encode(domain.WorkSession{ID: "s1"})
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	newTo := "12:30"
	if _, err := runSessionEdit(context.Background(), c, "s1", sessionEditInput{To: &newTo}); err != nil {
		t.Fatal(err)
	}
	// note preserved (not blanked), stop changed to 12:30
	if patched["note"] != "keep" {
		t.Errorf("note = %v, want preserved 'keep'", patched["note"])
	}
	stopStr, _ := patched["stop"].(string)
	if !strings.Contains(stopStr, "12:30") {
		t.Errorf("stop = %v, want 12:30", patched["stop"])
	}
}

func TestSessionRange_FromTo(t *testing.T) {
	since, until, err := sessionRange("", "2026-06-01", "2026-06-07")
	if err != nil {
		t.Fatal(err)
	}
	wantSince := time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)
	wantUntil := time.Date(2026, 6, 8, 0, 0, 0, 0, time.Local) // +1 day inclusive
	if !since.Equal(wantSince) {
		t.Errorf("since = %v, want %v", since, wantSince)
	}
	if !until.Equal(wantUntil) {
		t.Errorf("until = %v, want %v", until, wantUntil)
	}
}
