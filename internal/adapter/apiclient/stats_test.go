package apiclient_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

func TestGetBurndown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/burndown" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"totalMin":120,"targetMin":480,"saldoMin":-360,"onTrack":false,"workdaysAll":22,"workdaysDue":10}`))
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	b, err := c.GetBurndown(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if b.TotalMin != 120 {
		t.Errorf("TotalMin: got %d, want 120", b.TotalMin)
	}
	if b.OnTrack {
		t.Errorf("OnTrack: got true, want false")
	}
	if b.WorkdaysAll != 22 {
		t.Errorf("WorkdaysAll: got %d, want 22", b.WorkdaysAll)
	}
	if b.WorkdaysDue != 10 {
		t.Errorf("WorkdaysDue: got %d, want 10", b.WorkdaysDue)
	}
}

func TestGetWeek_NoRef(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/week" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query string, got: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[{"date":"2026-06-15","loggedMin":60,"targetMin":480,"isToday":true,"workday":true}]`))
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	wk, err := c.GetWeek(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(wk) != 1 {
		t.Fatalf("len: got %d, want 1", len(wk))
	}
	if !wk[0].Workday {
		t.Errorf("Workday: got false, want true")
	}
	if !wk[0].IsToday {
		t.Errorf("IsToday: got false, want true")
	}
	if wk[0].Date != "2026-06-15" {
		t.Errorf("Date: got %s, want 2026-06-15", wk[0].Date)
	}
}

func TestGetWeek_WithRef(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/week" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("ref") != "2026-06-08" {
			t.Errorf("unexpected ref: %s", r.URL.Query().Get("ref"))
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	wk, err := c.GetWeek(t.Context(), "2026-06-08")
	if err != nil {
		t.Fatal(err)
	}
	if len(wk) != 0 {
		t.Fatalf("len: got %d, want 0", len(wk))
	}
}

func TestGetToday(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/today" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"date":"2026-06-15","loggedMin":90,"targetMin":480,"saldoMin":-390,"running":true}`))
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	td, err := c.GetToday(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if td.LoggedMin != 90 {
		t.Errorf("LoggedMin: got %d, want 90", td.LoggedMin)
	}
	if !td.Running {
		t.Errorf("Running: got false, want true")
	}
}

func TestGetStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/stats" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("range") != "week" {
			t.Errorf("unexpected range: %s", r.URL.Query().Get("range"))
		}
		_, _ = w.Write([]byte(`{"totalMin":2400,"avgMin":480,"maxMin":540,"minMin":420,"workdays":5,"daysWithWork":5,"hits":5,"streak":5,"bestStreak":10,"overtimeMin":60}`))
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	s, err := c.GetStats(t.Context(), "week")
	if err != nil {
		t.Fatal(err)
	}
	if s.TotalMin != 2400 {
		t.Errorf("TotalMin: got %d, want 2400", s.TotalMin)
	}
	if s.Streak != 5 {
		t.Errorf("Streak: got %d, want 5", s.Streak)
	}
}

func TestSetTargetConfig(t *testing.T) {
	var gotPath string
	var gotDefault int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var body struct {
			DefaultTargetMin int            `json:"defaultTargetMin"`
			WeekdayTargetMin map[string]int `json:"weekdayTargetMin"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		gotDefault = body.DefaultTargetMin
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := apiclient.New(srv.URL, "tok")
	weekday := map[string]int{"1": 480, "2": 480, "3": 480, "4": 480, "5": 480}
	err := c.SetTargetConfig(t.Context(), 480, weekday)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/settings/target" {
		t.Errorf("path: got %s, want /api/v1/settings/target", gotPath)
	}
	if gotDefault != 480 {
		t.Errorf("defaultTargetMin: got %d, want 480", gotDefault)
	}
}
