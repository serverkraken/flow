package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func TestRunWorktimeImport_Sessions(t *testing.T) {
	var added []map[string]any
	conflictOnSecond := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode([]domain.Project{})
		case r.Method == "POST" && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode(domain.Project{ID: "p-import", Name: "Import"})
		case r.Method == "POST" && r.URL.Path == "/api/v1/sessions":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			added = append(added, in)
			conflictOnSecond++
			if conflictOnSecond == 2 { // simulate an already-imported (overlap) row
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "overlap"})
				return
			}
			_ = json.NewEncoder(w).Encode(domain.WorkSession{ID: "s1"})
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	dir := t.TempDir()
	// line 1 is the anomaly (8min clock, 259703s recorded) → warning + import
	// line 2 will get the simulated 409 → skipped
	log := "2026-04-24\t07:34\t07:42\t259703\n2026-05-04\t08:16\t16:18\t28920\n"
	writeFile(t, dir, "worktime.log", log)

	st, err := runWorktimeImport(context.Background(), c, dir, "Import", false)
	if err != nil {
		t.Fatal(err)
	}
	if st.Sessions != 1 || st.Skipped != 1 {
		t.Fatalf("stats = %+v (want Sessions 1, Skipped 1)", st)
	}
	if st.ProjectsCreated != 1 {
		t.Fatalf("ProjectsCreated = %d, want 1", st.ProjectsCreated)
	}
	if len(st.Warnings) != 1 {
		t.Fatalf("want 1 divergence warning, got %v", st.Warnings)
	}
	if len(added) != 2 {
		t.Fatalf("AddSession calls = %d, want 2", len(added))
	}
}

func TestRunWorktimeImport_SessionsDryRun(t *testing.T) {
	var posts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode([]domain.Project{})
		case r.Method == "POST":
			posts++
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")
	dir := t.TempDir()
	writeFile(t, dir, "worktime.log", "2026-05-04\t08:16\t16:18\t28920\n")

	st, err := runWorktimeImport(context.Background(), c, dir, "Import", true)
	if err != nil {
		t.Fatal(err)
	}
	if posts != 0 {
		t.Fatalf("dry-run made %d POSTs, want 0", posts)
	}
	if st.Sessions != 1 {
		t.Fatalf("dry-run Sessions = %d, want 1", st.Sessions)
	}
}

func TestParseDateTimeBerlin(t *testing.T) {
	got, err := parseDateTimeBerlin("2026-05-04", "08:16")
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Europe/Berlin")
	want := time.Date(2026, 5, 4, 8, 16, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if _, err := parseDateTimeBerlin("nope", "08:16"); err == nil {
		t.Fatal("bad date should error")
	}
}

func TestParseDayOffLine(t *testing.T) {
	// comment line → skipped
	if _, ok, err := parseDayOffLine(1, "# worktime day-offs — TSV: ..."); ok || err != nil {
		t.Fatalf("comment: ok=%v err=%v", ok, err)
	}
	// blank line → skipped
	if _, ok, err := parseDayOffLine(2, ""); ok || err != nil {
		t.Fatalf("blank: ok=%v err=%v", ok, err)
	}
	// vacation row
	e, ok, err := parseDayOffLine(3, "2026-04-29\tvacation\tJules Geburtstag")
	if err != nil || !ok {
		t.Fatalf("vacation: ok=%v err=%v", ok, err)
	}
	if e.Kind != domain.KindVacation || e.Date != "2026-04-29" || e.Label != "Jules Geburtstag" || e.TargetMin != 0 {
		t.Fatalf("entry = %+v", e)
	}
	// holiday row parses with KindHoliday (caller skips it)
	h, ok, err := parseDayOffLine(4, "2026-01-01\tholiday\tNeujahr")
	if err != nil || !ok || h.Kind != domain.KindHoliday {
		t.Fatalf("holiday: ok=%v err=%v kind=%v", ok, err, h.Kind)
	}
	// optional hours column → TargetMin
	hr, ok, err := parseDayOffLine(5, "2026-06-05\tvacation\tHalbtag\t4")
	if err != nil || !ok || hr.TargetMin != 240 {
		t.Fatalf("hours: ok=%v err=%v target=%d", ok, err, hr.TargetMin)
	}
	// unknown kind → error
	if _, _, err := parseDayOffLine(6, "2026-06-05\tbogus\tX"); err == nil {
		t.Fatal("unknown kind should error")
	}
	// too few columns → error
	if _, _, err := parseDayOffLine(7, "2026-06-05"); err == nil {
		t.Fatal("too few columns should error")
	}
}

func TestParseLogLine(t *testing.T) {
	// valid line: 08:16→16:18 = 28920s
	e, ok, err := parseLogLine(5, "2026-05-04\t08:16\t16:18\t28920")
	if err != nil || !ok {
		t.Fatalf("valid line: ok=%v err=%v", ok, err)
	}
	if e.Seconds != 28920 || e.Stop.Sub(e.Start) != 8*time.Hour+2*time.Minute {
		t.Fatalf("entry = %+v", e)
	}
	// blank line → ok=false, no error
	if _, ok, err := parseLogLine(1, "   "); ok || err != nil {
		t.Fatalf("blank: ok=%v err=%v", ok, err)
	}
	// malformed: too few columns
	if _, _, err := parseLogLine(2, "2026-05-04\t08:16"); err == nil {
		t.Fatal("too few columns should error")
	}
	// malformed: bad time
	if _, _, err := parseLogLine(3, "2026-05-04\t8h16\t16:18\t10"); err == nil {
		t.Fatal("bad time should error")
	}
	// anomaly line still parses (seconds wildly off, clock times valid)
	e2, ok, err := parseLogLine(1, "2026-04-24\t07:34\t07:42\t259703")
	if err != nil || !ok || e2.Seconds != 259703 {
		t.Fatalf("anomaly: ok=%v err=%v e=%+v", ok, err, e2)
	}
}
