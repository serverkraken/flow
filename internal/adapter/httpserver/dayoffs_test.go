package httpserver_test

import (
	"encoding/json"
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

func newDayOffServer() *httpserver.Server {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)}
	bus := sse.NewBus()
	dos := testutil.NewFakeDayOffStore()
	settings := testutil.NewFakeUserSettingsStore()
	return &httpserver.Server{
		Verifier:     testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:       usecase.EnsureUser{Users: testutil.NewFakeUserStore(), IDs: &testutil.FakeIDGen{}, Allow: func(ports.Identity) bool { return true }},
		Bus:          bus,
		Clock:        clk,
		AddDayOffs:   usecase.AddDayOffs{Store: dos, Bus: bus},
		DeleteDayOff: usecase.DeleteDayOff{Store: dos, Bus: bus},
		ListDayOffs:  usecase.ListDayOffs{Store: dos, Settings: settings, Loc: time.UTC},
	}
}

func TestDayOffRoundTrip(t *testing.T) {
	ts := httptest.NewServer(newDayOffServer().Routes())
	defer ts.Close()

	do := func(method, path, body string) *http.Response {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer x")
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	res := do("POST", "/api/v1/dayoffs", `{"from":"2026-06-15","to":"2026-06-19","kind":"vacation","label":"Sommer","targetMin":0,"skipWeekends":true}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST status %d, want 201", res.StatusCode)
	}
	_ = res.Body.Close()

	res = do("GET", "/api/v1/dayoffs?from=2026-06-01&to=2026-06-30", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET status %d, want 200", res.StatusCode)
	}
	var list []map[string]any
	_ = json.NewDecoder(res.Body).Decode(&list)
	_ = res.Body.Close()
	var vac int
	for _, d := range list {
		if d["kind"] == "vacation" {
			vac++
		}
	}
	if vac != 5 {
		t.Fatalf("want 5 vacation days (Mon-Fri), got %d (list=%v)", vac, list)
	}

	// holiday kind is rejected (computed, not manual).
	res = do("POST", "/api/v1/dayoffs", `{"from":"2026-06-15","to":"2026-06-15","kind":"holiday"}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("holiday POST status %d, want 400", res.StatusCode)
	}
	_ = res.Body.Close()

	// delete one day.
	res = do("DELETE", "/api/v1/dayoffs/2026-06-15", "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE status %d, want 204", res.StatusCode)
	}
	_ = res.Body.Close()
}

func TestHandleDeleteDayOff_BadDate(t *testing.T) {
	ts := httptest.NewServer(newDayOffServer().Routes())
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/dayoffs/not-a-date", nil)
	req.Header.Set("Authorization", "Bearer x")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("DELETE bad date: want 400, got %d", res.StatusCode)
	}
}

func TestHandleAddDayOffs_BadJSON(t *testing.T) {
	ts := httptest.NewServer(newDayOffServer().Routes())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/dayoffs", strings.NewReader("{bad json}"))
	req.Header.Set("Authorization", "Bearer x")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST bad JSON: want 400, got %d", res.StatusCode)
	}
}

func TestHandleAddDayOffs_BadFromDate(t *testing.T) {
	ts := httptest.NewServer(newDayOffServer().Routes())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/dayoffs", strings.NewReader(`{"from":"not-a-date","to":"2026-06-20","kind":"vacation","targetMin":0}`))
	req.Header.Set("Authorization", "Bearer x")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST bad from date: want 400, got %d", res.StatusCode)
	}
}
