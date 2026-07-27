# `flow worktime import` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `flow worktime import [dir]` CLI command that imports a legacy `~/worktime` installation's booked sessions, days off, and the day→doc link into the rebuild server.

**Architecture:** Pure CLI feature — a new cobra subcommand on the existing `worktimeCmd()`, a thin client over existing apiclient methods (`AddSession`, `AddDayOffs`, plus the existing `projectResolver` for find-or-create). No server, usecase, or migration change. Mirrors the structure of `cmd/flow/docs_import.go` (per-row isolation, summary line, non-zero exit on failure).

**Tech Stack:** Go, cobra, `internal/adapter/apiclient`, `internal/domain` (Kind/ParseKind), `net/http/httptest` for tests.

## Global Constraints

- **No backend change.** Only `cmd/flow/` is touched. The apiclient already exposes everything.
- **Clock times are the source of truth; ignore the `elapsed_seconds` column.** Using it would push the 72h anomaly's stop 3 days out and break `AddSession`'s same-local-day rule.
- **Timezone is Europe/Berlin** (`time.LoadLocation("Europe/Berlin")`) for all `date+HH:MM` → `time.Time` construction.
- **Skip `holiday`-kind day-offs** — the server rejects them (`ErrHolidayNotManual`) and computes them from the Bundesland at read time.
- **Default placeholder project name `"Import"`**, overridable via `--project`. Imported sessions carry no tag/note.
- **German CLI copy**, matching `docs import`.
- **Verify (`make ci` from repo root) must stay green;** the coverage gate (~80%) must hold.
- Apiclient facts (verbatim): `apiclient.New(url, token)`; `AddSession(ctx, projectID *string, start, stop time.Time, tag, note string) (domain.WorkSession, error)` → `POST /api/v1/sessions` body `{projectId,tag,note,start,stop}`; `AddDayOffs(ctx, from, to, kind, label string, targetMin int, skipWeekends bool) error` → `POST /api/v1/dayoffs` body `{from,to,kind,label,targetMin,skipWeekends}`; `IsConflict(err) bool` (HTTP 409); `ListProjects`/`CreateProject` → `GET`/`POST /api/v1/projects`; `domain.Project{ID,Name,Slug}`; `domain.ParseKind(s) (domain.Kind, bool)`, `domain.KindHoliday`.
- Reuse the existing `projectResolver` (same package, `cmd/flow/docs_import.go`): `newProjectResolver(c, dryRun)` + `.resolve(ctx, name) (*string, error)` find-or-creates by Name/Slug and tracks `.created`.

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `cmd/flow/worktime_import.go` | parsers (`parseLogLine`, `parseDayOffLine`, `parseDateTimeBerlin`), orchestrator (`runWorktimeImport`), stats type, cobra command (`worktimeImportCmd`) | Create |
| `cmd/flow/worktime_import_test.go` | parser unit tests + httptest orchestration/idempotency tests | Create |
| `cmd/flow/testdata/worktime/{worktime.log,worktime-dayoffs.tsv,worktime-links.tsv}` | fixture for the full-run + idempotency tests | Create |
| `cmd/flow/worktime.go:15-40` | register `worktimeImportCmd()` as a subcommand | Modify |

---

### Task 1: Log line parser + Berlin time construction

**Files:**
- Create: `cmd/flow/worktime_import.go`
- Test: `cmd/flow/worktime_import_test.go`

**Interfaces:**
- Produces: `parseDateTimeBerlin(date, hhmm string) (time.Time, error)`; `type logEntry struct { Line int; Start, Stop time.Time; Seconds int }`; `parseLogLine(lineNo int, raw string) (e logEntry, ok bool, err error)` — `ok=false` for blank lines; `err` non-nil for malformed lines (wrong column count, unparseable date/time/seconds). `Seconds` is captured but the caller ignores it for time math.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"testing"
	"time"
)

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/flow/ -run 'TestParseDateTimeBerlin|TestParseLogLine' -v`
Expected: FAIL — `undefined: parseDateTimeBerlin` / `parseLogLine`.

- [ ] **Step 3: Write minimal implementation**

```go
package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// berlinLoc is the timezone all legacy worktime timestamps are interpreted in.
// Loaded once; falls back to UTC only if the tz database is unavailable.
var berlinLoc = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// parseDateTimeBerlin builds a time from a "2006-01-02" date and a "15:04"
// clock string, interpreted in Europe/Berlin.
func parseDateTimeBerlin(date, hhmm string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04", date+" "+hhmm, berlinLoc)
}

// logEntry is one parsed worktime.log row. Seconds is the legacy elapsed-seconds
// column; it is captured for a divergence warning but never used for time math.
type logEntry struct {
	Line          int
	Start, Stop   time.Time
	Seconds       int
}

// parseLogLine parses a tab-separated "date<TAB>start<TAB>end<TAB>seconds" row.
// ok=false marks a blank line (skip, no error); a non-nil error marks a
// malformed row.
func parseLogLine(lineNo int, raw string) (logEntry, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return logEntry{}, false, nil
	}
	f := strings.Split(raw, "\t")
	if len(f) != 4 {
		return logEntry{}, false, fmt.Errorf("expected 4 tab-separated columns, got %d", len(f))
	}
	start, err := parseDateTimeBerlin(f[0], f[1])
	if err != nil {
		return logEntry{}, false, fmt.Errorf("start: %w", err)
	}
	stop, err := parseDateTimeBerlin(f[0], f[2])
	if err != nil {
		return logEntry{}, false, fmt.Errorf("stop: %w", err)
	}
	secs, err := strconv.Atoi(strings.TrimSpace(f[3]))
	if err != nil {
		return logEntry{}, false, fmt.Errorf("seconds: %w", err)
	}
	return logEntry{Line: lineNo, Start: start, Stop: stop, Seconds: secs}, true, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/flow/ -run 'TestParseDateTimeBerlin|TestParseLogLine' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/flow/worktime_import.go cmd/flow/worktime_import_test.go
git commit -m "feat(worktime-import): worktime.log line parser + Berlin time"
```

---

### Task 2: Day-off line parser

**Files:**
- Modify: `cmd/flow/worktime_import.go`
- Test: `cmd/flow/worktime_import_test.go`

**Interfaces:**
- Consumes: `domain.ParseKind`.
- Produces: `type dayOffEntry struct { Line int; Date string; Kind domain.Kind; Label string; TargetMin int }`; `parseDayOffLine(lineNo int, raw string) (e dayOffEntry, ok bool, err error)` — `ok=false` for blank lines and `#` comment lines; `err` non-nil for malformed rows (too few columns, unknown kind, bad hours). The returned `Kind` may be `domain.KindHoliday`; the caller is responsible for skipping holidays. `Date` is the raw `YYYY-MM-DD` string (validated by re-parse). `TargetMin` is `0` unless an `hours` column is present.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/flow/ -run TestParseDayOffLine -v`
Expected: FAIL — `undefined: parseDayOffLine`.

- [ ] **Step 3: Write minimal implementation**

```go
// add import "github.com/serverkraken/flow/internal/domain"

type dayOffEntry struct {
	Line      int
	Date      string
	Kind      domain.Kind
	Label     string
	TargetMin int
}

// parseDayOffLine parses "date<TAB>kind<TAB>label[<TAB>hours]". ok=false for
// blank and "#"-comment lines. The Kind may be KindHoliday; the caller skips
// holidays (the server refuses to store them).
func parseDayOffLine(lineNo int, raw string) (dayOffEntry, bool, error) {
	t := strings.TrimSpace(raw)
	if t == "" || strings.HasPrefix(t, "#") {
		return dayOffEntry{}, false, nil
	}
	f := strings.Split(raw, "\t")
	if len(f) < 3 {
		return dayOffEntry{}, false, fmt.Errorf("expected at least 3 tab-separated columns, got %d", len(f))
	}
	if _, err := time.Parse("2006-01-02", f[0]); err != nil {
		return dayOffEntry{}, false, fmt.Errorf("date: %w", err)
	}
	kind, ok := domain.ParseKind(f[1])
	if !ok {
		return dayOffEntry{}, false, fmt.Errorf("unknown kind %q", f[1])
	}
	e := dayOffEntry{Line: lineNo, Date: f[0], Kind: kind, Label: strings.TrimSpace(f[2])}
	if len(f) >= 4 && strings.TrimSpace(f[3]) != "" {
		hours, err := strconv.ParseFloat(strings.TrimSpace(f[3]), 64)
		if err != nil {
			return dayOffEntry{}, false, fmt.Errorf("hours: %w", err)
		}
		e.TargetMin = int(hours * 60)
	}
	return e, true, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/flow/ -run TestParseDayOffLine -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/flow/worktime_import.go cmd/flow/worktime_import_test.go
git commit -m "feat(worktime-import): day-off line parser"
```

---

### Task 3: Orchestrator — sessions import (find-or-create project, AddSession, 409-skip, divergence warning)

**Files:**
- Modify: `cmd/flow/worktime_import.go`
- Test: `cmd/flow/worktime_import_test.go`

**Interfaces:**
- Consumes: `parseLogLine` (Task 1); `newProjectResolver`/`.resolve` (existing, `docs_import.go`); `apiclient.AddSession`, `apiclient.IsConflict`.
- Produces: `type wtImportStats struct { Sessions, DayOffs, Skipped, Links, Failed, ProjectsCreated int; Warnings, Failures []string }`; `runWorktimeImport(ctx context.Context, c *apiclient.Client, dir, projectName string, dryRun bool) (wtImportStats, error)`. In this task `runWorktimeImport` handles ONLY `worktime.log` (day-offs + links added in Tasks 4–5). A divergence (`|seconds − clock-duration| > 5min`) appends a `Warnings` entry but still imports the clock-time session. A 409 from `AddSession` counts as `Skipped` (idempotent re-run). Missing `worktime.log` is not an error (other files may still import).

- [ ] **Step 1: Write the failing test**

```go
import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/flow/ -run TestRunWorktimeImport_Sessions -v`
Expected: FAIL — `undefined: runWorktimeImport`.

- [ ] **Step 3: Write minimal implementation**

```go
// add imports: "context", "os", "github.com/serverkraken/flow/internal/adapter/apiclient"

type wtImportStats struct {
	Sessions, DayOffs, Skipped, Links, Failed, ProjectsCreated int
	Warnings, Failures                                          []string
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// runWorktimeImport reads ~/worktime's three files from dir and imports them.
// Per-row failures are isolated and collected; the run continues. (Day-offs and
// links are added in later tasks.)
func runWorktimeImport(ctx context.Context, c *apiclient.Client, dir, projectName string, dryRun bool) (wtImportStats, error) {
	var st wtImportStats
	if err := importSessions(ctx, c, dir, projectName, dryRun, &st); err != nil {
		return st, err
	}
	return st, nil
}

func importSessions(ctx context.Context, c *apiclient.Client, dir, projectName string, dryRun bool, st *wtImportStats) error {
	raw, err := os.ReadFile(filepath.Join(dir, "worktime.log"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no sessions to import
		}
		return fmt.Errorf("read worktime.log: %w", err)
	}
	pr := newProjectResolver(c, dryRun)
	projectID, err := pr.resolve(ctx, projectName)
	if err != nil {
		return fmt.Errorf("resolve project %q: %w", projectName, err)
	}
	defer func() { st.ProjectsCreated += pr.created }()

	for i, line := range strings.Split(string(raw), "\n") {
		e, ok, perr := parseLogLine(i+1, line)
		if perr != nil {
			st.Failed++
			st.Failures = append(st.Failures, fmt.Sprintf("worktime.log:%d: %v", i+1, perr))
			continue
		}
		if !ok {
			continue
		}
		if diff := absDur(time.Duration(e.Seconds)*time.Second - e.Stop.Sub(e.Start)); diff > 5*time.Minute {
			st.Warnings = append(st.Warnings, fmt.Sprintf(
				"worktime.log:%d: Sekunden %d ≠ Uhrzeit-Dauer %s (importiere Uhrzeit)",
				e.Line, e.Seconds, e.Stop.Sub(e.Start)))
		}
		if dryRun {
			st.Sessions++
			continue
		}
		if _, aerr := c.AddSession(ctx, projectID, e.Start, e.Stop, "", ""); aerr != nil {
			if apiclient.IsConflict(aerr) {
				st.Skipped++
				continue
			}
			st.Failed++
			st.Failures = append(st.Failures, fmt.Sprintf("worktime.log:%d: %v", e.Line, aerr))
			continue
		}
		st.Sessions++
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/flow/ -run TestRunWorktimeImport_Sessions -v`
Expected: PASS (both subtests).

- [ ] **Step 5: Commit**

```bash
git add cmd/flow/worktime_import.go cmd/flow/worktime_import_test.go
git commit -m "feat(worktime-import): session import (find-or-create project, 409-skip, divergence warn)"
```

---

### Task 4: Orchestrator — day-offs import (skip comment/holiday, AddDayOffs upsert)

**Files:**
- Modify: `cmd/flow/worktime_import.go`
- Test: `cmd/flow/worktime_import_test.go`

**Interfaces:**
- Consumes: `parseDayOffLine` (Task 2); `apiclient.AddDayOffs`.
- Produces: `importDayOffs(ctx, c, dir, dryRun, *wtImportStats) error`, called from `runWorktimeImport` after `importSessions`. Holiday-kind rows are counted as `Skipped`. Each imported non-holiday row is a single-day `AddDayOffs(date, date, kind, label, targetMin, false)` (upsert → idempotent), incrementing `DayOffs`. Missing `worktime-dayoffs.tsv` is not an error.

- [ ] **Step 1: Write the failing test**

```go
func TestRunWorktimeImport_DayOffs(t *testing.T) {
	var added []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode([]domain.Project{})
		case r.Method == "POST" && r.URL.Path == "/api/v1/dayoffs":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			added = append(added, in)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	dir := t.TempDir()
	tsv := "# comment header\n" +
		"2026-01-01\tholiday\tNeujahr\n" + // skipped (holiday)
		"2026-04-29\tvacation\tJules Geburtstag\n" +
		"2026-06-01\tsick\tKrank\n"
	writeFile(t, dir, "worktime-dayoffs.tsv", tsv)

	st, err := runWorktimeImport(context.Background(), c, dir, "Import", false)
	if err != nil {
		t.Fatal(err)
	}
	if st.DayOffs != 2 {
		t.Fatalf("DayOffs = %d, want 2", st.DayOffs)
	}
	if st.Skipped != 1 { // the holiday
		t.Fatalf("Skipped = %d, want 1 (holiday)", st.Skipped)
	}
	if len(added) != 2 {
		t.Fatalf("AddDayOffs calls = %d, want 2", len(added))
	}
	if added[0]["from"] != added[0]["to"] || added[0]["from"] != "2026-04-29" {
		t.Fatalf("first dayoff from/to = %v/%v", added[0]["from"], added[0]["to"])
	}
	if added[0]["kind"] != "vacation" {
		t.Fatalf("first dayoff kind = %v, want vacation", added[0]["kind"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/flow/ -run TestRunWorktimeImport_DayOffs -v`
Expected: FAIL — `st.DayOffs` stays 0 (`importDayOffs` not wired / undefined).

- [ ] **Step 3: Write minimal implementation**

```go
// in runWorktimeImport, after importSessions(...):
	if err := importDayOffs(ctx, c, dir, dryRun, &st); err != nil {
		return st, err
	}
```

```go
func importDayOffs(ctx context.Context, c *apiclient.Client, dir string, dryRun bool, st *wtImportStats) error {
	raw, err := os.ReadFile(filepath.Join(dir, "worktime-dayoffs.tsv"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read worktime-dayoffs.tsv: %w", err)
	}
	for i, line := range strings.Split(string(raw), "\n") {
		e, ok, perr := parseDayOffLine(i+1, line)
		if perr != nil {
			st.Failed++
			st.Failures = append(st.Failures, fmt.Sprintf("worktime-dayoffs.tsv:%d: %v", i+1, perr))
			continue
		}
		if !ok {
			continue
		}
		if e.Kind == domain.KindHoliday {
			st.Skipped++ // computed from Bundesland, never stored
			continue
		}
		if dryRun {
			st.DayOffs++
			continue
		}
		if derr := c.AddDayOffs(ctx, e.Date, e.Date, string(e.Kind), e.Label, e.TargetMin, false); derr != nil {
			st.Failed++
			st.Failures = append(st.Failures, fmt.Sprintf("worktime-dayoffs.tsv:%d: %v", e.Line, derr))
			continue
		}
		st.DayOffs++
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/flow/ -run TestRunWorktimeImport_DayOffs -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/flow/worktime_import.go cmd/flow/worktime_import_test.go
git commit -m "feat(worktime-import): day-off import (skip holiday, AddDayOffs upsert)"
```

---

### Task 5: Orchestrator — links + full-fixture run + idempotency

**Files:**
- Modify: `cmd/flow/worktime_import.go`
- Create: `cmd/flow/testdata/worktime/worktime.log`, `cmd/flow/testdata/worktime/worktime-dayoffs.tsv`, `cmd/flow/testdata/worktime/worktime-links.tsv`
- Test: `cmd/flow/worktime_import_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces: `importLinks(ctx, dir, *wtImportStats) error`, called last from `runWorktimeImport`. Each parsed link row increments `Links` and appends a "covered by daily/<date> convention" note to `Warnings`; nothing is written. Link lines are `date<TAB>doc-path`; blank lines skipped.

- [ ] **Step 1: Create the testdata fixture**

`cmd/flow/testdata/worktime/worktime.log`:
```
2026-04-24	07:34	07:42	259703
2026-05-08	07:45	08:10	1500
2026-05-08	08:10	14:28	22685
2026-05-08	14:28	16:04	5760
```

`cmd/flow/testdata/worktime/worktime-dayoffs.tsv`:
```
# worktime day-offs — TSV: date<TAB>kind<TAB>label[<TAB>hours]
# kinds: holiday | vacation | sick
2026-01-01	holiday	Neujahr
2026-04-29	vacation	Jules Geburtstag
2026-06-01	sick	Krank
```

`cmd/flow/testdata/worktime/worktime-links.tsv`:
```
2026-05-11	daily/2026-05-11
```

- [ ] **Step 2: Write the failing test**

```go
// helper: a server that accepts every write and returns fresh projects/sessions.
func acceptAllServer(t *testing.T, sessionConflict bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode([]domain.Project{})
		case r.Method == "POST" && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode(domain.Project{ID: "p-import", Name: "Import"})
		case r.Method == "POST" && r.URL.Path == "/api/v1/sessions":
			if sessionConflict {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "overlap"})
				return
			}
			_ = json.NewEncoder(w).Encode(domain.WorkSession{ID: "s"})
		case r.Method == "POST" && r.URL.Path == "/api/v1/dayoffs":
			w.WriteHeader(http.StatusNoContent)
		}
	}))
}

func TestRunWorktimeImport_FullFixture(t *testing.T) {
	srv := acceptAllServer(t, false)
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	dir := filepath.Join("testdata", "worktime")
	st, err := runWorktimeImport(context.Background(), c, dir, "Import", false)
	if err != nil {
		t.Fatal(err)
	}
	if st.Sessions != 4 {
		t.Fatalf("Sessions = %d, want 4", st.Sessions)
	}
	if st.DayOffs != 2 {
		t.Fatalf("DayOffs = %d, want 2", st.DayOffs)
	}
	if st.Skipped != 1 { // one holiday
		t.Fatalf("Skipped = %d, want 1", st.Skipped)
	}
	if st.Links != 1 {
		t.Fatalf("Links = %d, want 1", st.Links)
	}
	if st.Failed != 0 {
		t.Fatalf("Failed = %d (%v), want 0", st.Failed, st.Failures)
	}
}

// Idempotency: when the server reports every session as an overlap (409) and
// day-offs upsert, a re-run imports nothing new.
func TestRunWorktimeImport_Idempotent(t *testing.T) {
	srv := acceptAllServer(t, true) // every AddSession → 409
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	st, err := runWorktimeImport(context.Background(), c, filepath.Join("testdata", "worktime"), "Import", false)
	if err != nil {
		t.Fatal(err)
	}
	if st.Sessions != 0 {
		t.Fatalf("re-run Sessions = %d, want 0", st.Sessions)
	}
	if st.Skipped != 5 { // 4 overlapping sessions + 1 holiday
		t.Fatalf("re-run Skipped = %d, want 5", st.Skipped)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./cmd/flow/ -run TestRunWorktimeImport_Full -v`
Expected: FAIL — `st.Links` stays 0 (`importLinks` not wired).

- [ ] **Step 4: Write minimal implementation**

```go
// in runWorktimeImport, after importDayOffs(...):
	if err := importLinks(dir, &st); err != nil {
		return st, err
	}
```

```go
// importLinks parses worktime-links.tsv but writes nothing: the day↔daily-doc
// relationship is already encoded by the daily/<date> path convention that
// `flow docs import` preserves.
func importLinks(dir string, st *wtImportStats) error {
	raw, err := os.ReadFile(filepath.Join(dir, "worktime-links.tsv"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read worktime-links.tsv: %w", err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Split(strings.TrimSpace(line), "\t")
		if len(f) < 2 || f[0] == "" {
			continue
		}
		st.Links++
		st.Warnings = append(st.Warnings, fmt.Sprintf(
			"worktime-links.tsv: %s → %s (covered by daily/<date> convention, nicht importiert)", f[0], f[1]))
	}
	return nil
}
```

Note: `importLinks` takes `(dir, *wtImportStats)` — no ctx/client (it writes nothing). Update the call site accordingly.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./cmd/flow/ -run TestRunWorktimeImport -v`
Expected: PASS (all `runWorktimeImport` tests).

- [ ] **Step 6: Commit**

```bash
git add cmd/flow/worktime_import.go cmd/flow/worktime_import_test.go cmd/flow/testdata/worktime/
git commit -m "feat(worktime-import): link counting + full-fixture + idempotency tests"
```

---

### Task 6: CLI command + registration + `make ci` + done-gate

**Files:**
- Modify: `cmd/flow/worktime_import.go` (add `worktimeImportCmd`)
- Modify: `cmd/flow/worktime.go:15-40` (register the subcommand)
- Test: `cmd/flow/worktime_import_test.go`

**Interfaces:**
- Consumes: `runWorktimeImport`; `clientFromStore` (existing, used by `docsImportCmd`).
- Produces: `worktimeImportCmd() *cobra.Command` with `--dry-run` (bool) and `--project` (string, default `"Import"`); optional positional `[dir]` defaulting to `~/worktime`; a German summary line; non-zero exit when `st.Failed > 0`. Registered on `worktimeCmd()` via `cmd.AddCommand(worktimeImportCmd())` so `flow worktime import` works while `flow worktime` still launches the TUI.

- [ ] **Step 1: Write the failing test**

```go
func TestWorktimeImportCmd_DefaultsAndFlags(t *testing.T) {
	cmd := worktimeImportCmd()
	if cmd.Use != "import [dir]" {
		t.Fatalf("Use = %q", cmd.Use)
	}
	pf := cmd.Flags().Lookup("project")
	if pf == nil || pf.DefValue != "Import" {
		t.Fatalf("--project flag missing or wrong default: %+v", pf)
	}
	if cmd.Flags().Lookup("dry-run") == nil {
		t.Fatal("--dry-run flag missing")
	}
}

func TestWorktimeCmd_HasImportSubcommand(t *testing.T) {
	var found bool
	for _, sub := range worktimeCmd().Commands() {
		if sub.Name() == "import" {
			found = true
		}
	}
	if !found {
		t.Fatal("flow worktime is missing the import subcommand")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/flow/ -run 'TestWorktimeImportCmd|TestWorktimeCmd_HasImport' -v`
Expected: FAIL — `undefined: worktimeImportCmd`.

- [ ] **Step 3: Write the command**

```go
// add imports: "github.com/spf13/cobra"

func worktimeImportCmd() *cobra.Command {
	var dryRun bool
	var projectName string
	cmd := &cobra.Command{
		Use:   "import [dir]",
		Short: "Importiere Worktime-Daten aus einer alten flow-Installation (~/worktime)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := args0OrDefault(args)
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			st, err := runWorktimeImport(cmd.Context(), c, dir, projectName, dryRun)
			if err != nil {
				return err
			}
			mode := ""
			if dryRun {
				mode = " (dry-run)"
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"gebucht %d · freie Tage %d · übersprungen %d · Links %d · Projekt %q · Projekte angelegt %d · Fehler %d%s\n",
				st.Sessions, st.DayOffs, st.Skipped, st.Links, projectName, st.ProjectsCreated, st.Failed, mode)
			for _, wmsg := range st.Warnings {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  ⚠ "+wmsg)
			}
			for _, f := range st.Failures {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+f)
			}
			if st.Failed > 0 {
				return fmt.Errorf("%d Zeile(n) fehlgeschlagen", st.Failed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "parse und plane den Import, ohne zu schreiben")
	cmd.Flags().StringVar(&projectName, "project", "Import", "Projekt, dem importierte Sessions zugeordnet werden")
	return cmd
}

// args0OrDefault returns the positional dir arg, or ~/worktime when omitted.
func args0OrDefault(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "worktime"
	}
	return filepath.Join(home, "worktime")
}
```

- [ ] **Step 4: Register on `worktimeCmd()`**

In `cmd/flow/worktime.go`, change the function to build the command, attach the subcommand, and return it. Replace the `return &cobra.Command{…}` with:
```go
func worktimeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worktime",
		Short: "Worktime timer (TUI)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			// slog/stderr must never corrupt the TUI: send logs to a file.
			logf, err := os.OpenFile(filepath.Join(os.TempDir(), "flow-tui.log"),
				os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err == nil {
				defer func() { _ = logf.Close() }()
				os.Stderr = logf
			}
			pal := theme.Load()
			m := shell.New(client, os.Getenv("USER"), pal).
				WithTabs([]shell.Route{
					worktime.NewTodayRoute(client, time.Now, pal, worktime.BuildRegistry(client, pal)),
				})
			_, err = tea.NewProgram(m, tea.WithContext(cmd.Context())).Run()
			return err
		},
	}
	cmd.AddCommand(worktimeImportCmd())
	return cmd
}
```

- [ ] **Step 5: Run the command-level tests**

Run: `go test ./cmd/flow/ -run 'TestWorktimeImportCmd|TestWorktimeCmd_HasImport' -v`
Expected: PASS.

- [ ] **Step 6: Full verify**

Run: `make ci` (from repo root)
Expected: lint + templ + build + all tests green; coverage gate holds (~80%). If lint flags `logEntry` field alignment or unused imports, fix and re-run.

- [ ] **Step 7: Manual done-gate (against the dev stack or PROD)**

```bash
go build -o bin/flow ./cmd/flow
# dry-run first — no writes, eyeball counts + the line-1 divergence warning
./bin/flow worktime import --dry-run
# expected: gebucht 31 · freie Tage ~36 · übersprungen ~14 (holidays) · Links 1 · … (dry-run)
# then the real import:
./bin/flow worktime import
# verify in the TUI/WebUI: sessions appear under project "Import"; Urlaub/Krank
# days show in Frei; a second `flow worktime import` reports gebucht 0 (all skipped).
```
Report the dry-run + real-run summary lines and the second-run idempotency line back for the gate.

- [ ] **Step 8: Commit**

```bash
git add cmd/flow/worktime_import.go cmd/flow/worktime.go cmd/flow/worktime_import_test.go
git commit -m "feat(worktime-import): flow worktime import command + registration"
```

---

## Self-Review

**Spec coverage:**
- CLI command `flow worktime import [dir]` default `~/worktime` → Task 6.
- Placeholder project find-or-create, `--project` override → Task 3 (resolver) + Task 6 (flag).
- Sessions: clock-times truth, ignore seconds, Europe/Berlin, divergence warning → Tasks 1 + 3.
- Day-offs: skip `#`/holiday, map kind, optional hours, upsert → Tasks 2 + 4.
- Links: count + log, no write → Task 5.
- Idempotency (409-skip sessions, upsert day-offs) → Tasks 3 + 5.
- `--dry-run` no-write → Tasks 3, 4, 6.
- German summary + non-zero exit on failure → Task 6.
- Risk (contiguous multi-session days, half-open overlap) → exercised by the 3-session `2026-05-08` rows in the Task 5 fixture and confirmed in the Task 6 live done-gate.

**Placeholder scan:** none — every code step is complete.

**Type consistency:** `wtImportStats` fields (`Sessions/DayOffs/Skipped/Links/Failed/ProjectsCreated/Warnings/Failures`) are used identically across Tasks 3–6; `runWorktimeImport(ctx, c, dir, projectName, dryRun)` signature stable from Task 3; `logEntry`/`dayOffEntry` field names match between parser (Tasks 1–2) and orchestrator (Tasks 3–4); `importLinks(dir, *wtImportStats)` (no client) noted explicitly in Task 5.
