# Worktime Nachbuchen — Slice 4 (CLI) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `flow session` cobra command with `add` / `edit` / `delete` / `list` subcommands, backfilling and managing past worktime sessions via the already-shipped apiclient methods.

**Architecture:** Each subcommand's `RunE` obtains a `*apiclient.Client` via the existing `clientFromStore(ctx)` and delegates to a small, client-injectable helper (`runSessionAdd`, `runSessionEdit`, `runSessionDelete`, `runSessionList`) so the logic is unit-testable against an `httptest` server — mirroring the existing `runImport` pattern in `docs_import.go`. Project flags resolve by name (find-or-create) by reusing the existing `newProjectResolver`. Edit prefills unchanged fields by fetching the current session through `ListSessionsRange` (there is no GET-single endpoint), then applying only the flags cobra reports as `Changed`.

**Tech Stack:** Go, `github.com/spf13/cobra`, `internal/adapter/apiclient`, `internal/domain`. Tests use `net/http/httptest` + `apiclient.New(url, token)`.

## Global Constraints

- Module path: `github.com/serverkraken/flow`. Package `main` under `cmd/flow/`.
- No monoliths: all of Slice 4 lives in a new `cmd/flow/session.go` (+ `session_test.go`); only `main.go` gets a one-line `AddCommand`.
- Local tz for all `HH:MM` parsing (matches WebUI/TUI). Date flags are `yyyy-mm-dd`.
- Cross-midnight backfill out of scope: reject `--to <= --from`.
- Reuse `newProjectResolver(c, dryRun=false)` from `docs_import.go` for `--project` (name → find-or-create id). Do **not** duplicate project lookup.
- Reuse the apiclient methods verbatim (signatures below); do not add backend endpoints.
- `make ci` green at the end (lint included — run `make ci`, not just `go test`).

---

## Reference: existing shapes (read before starting)

- `cmd/flow/dayoff.go` — canonical cobra subcommand style: parent `&cobra.Command{Use, Short}` + `AddCommand`, `RunE` calls `clientFromStore(cmd.Context())`, flags via `cmd.Flags().StringVar(...)`.
- `cmd/flow/main.go:11-23` — `rootCmd()` with the `root.AddCommand(...)` list.
- `cmd/flow/docs_import.go` — `newProjectResolver(c *apiclient.Client, dryRun bool)` with `(pr).resolve(ctx, name string) (*string, error)` (matches existing name → create-if-missing → cached id; empty name → nil id). Confirm the exact constructor + method names with `rg -n "func newProjectResolver|func.*resolve" cmd/flow/docs_import.go`.
- `cmd/flow/docs_import_test.go` — the httptest+apiclient test pattern to copy (`apiclient.New(srv.URL, "tkn")`, table of routes).
- apiclient methods (all in `internal/adapter/apiclient/client.go`):
  - `AddSession(ctx, projectID *string, start, stop time.Time, tag, note string) (domain.WorkSession, error)` → `POST /api/v1/sessions`
  - `EditSession(ctx, id string, projectID *string, tag, note string, start time.Time, stop *time.Time) (domain.WorkSession, error)` → `PATCH /api/v1/sessions/{id}`
  - `DeleteSession(ctx, id string) error` → `DELETE /api/v1/sessions/{id}`
  - `ListSessionsRange(ctx, since, until time.Time) ([]domain.WorkSession, error)` → `GET /api/v1/sessions?since=&until=`
  - `ListProjects(ctx) ([]domain.Project, error)`
- `domain.WorkSession{ID, ProjectID *string, Tag, Note string, Start time.Time, Stop *time.Time}`; `Running()`, `Elapsed(now)`.
- The server returns HTTP 409 on overlap; verify how `apiclient.do` surfaces non-2xx (`rg -n "func (c \*Client) do" -A 25 internal/adapter/apiclient/client.go`) so the CLI error message is meaningful. (Task 1 Step 1.)

---

### Task 1: Shared parsing helpers + `flow session add`

**Files:**
- Create: `cmd/flow/session.go`
- Modify: `cmd/flow/main.go` (register `sessionCmd()`)
- Test: `cmd/flow/session_test.go`

**Interfaces:**
- Produces: `func sessionCmd() *cobra.Command` (parent; this task wires `add` only, more added later).
- Produces: `func parseClock(dateStr, hhmm string) (time.Time, error)` — `yyyy-mm-dd` + `HH:MM` → local `time.Time`.
- Produces: `func runSessionAdd(ctx context.Context, c *apiclient.Client, in sessionAddInput) (string, error)` where `sessionAddInput struct{ Date, From, To, Project, Tag, Note string }`. Returns a one-line summary; resolves `Project` via `newProjectResolver`.

- [ ] **Step 1: Read `apiclient.do` error surfacing.** Run `rg -n "func \(c \*Client\) do" -A 30 internal/adapter/apiclient/client.go`. Note the error type/text for a 409 so test assertions and user messages line up.

- [ ] **Step 2: Write failing tests** in `cmd/flow/session_test.go`:

```go
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
			_ = json.NewEncoder(w).Encode(domain.Document{}) // any 2xx body; client decodes WorkSession
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
```

  (Fix the double-encode in the happy-path handler — keep only the `WorkSession` encode; the `Document` line above is a deliberate reminder to encode exactly one 2xx body. Delete it when writing.)

- [ ] **Step 3: Run to verify failure**

Run: `go test ./cmd/flow/ -run 'TestParseClock|TestRunSessionAdd' -v`
Expected: FAIL (undefined symbols).

- [ ] **Step 4: Implement** `cmd/flow/session.go`:

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/spf13/cobra"
)

func sessionCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "session", Short: "manage past worktime sessions (Nachbuchen)"}
	cmd.AddCommand(sessionAddCmd())
	return cmd
}

// parseClock combines a yyyy-mm-dd date with an HH:MM clock in local tz.
func parseClock(dateStr, hhmm string) (time.Time, error) {
	d, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("bad --date %q: %w", dateStr, err)
	}
	c, err := time.ParseInLocation("15:04", hhmm, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("bad time %q (want HH:MM): %w", hhmm, err)
	}
	return time.Date(d.Year(), d.Month(), d.Day(), c.Hour(), c.Minute(), 0, 0, time.Local), nil
}

type sessionAddInput struct {
	Date, From, To, Project, Tag, Note string
}

func runSessionAdd(ctx context.Context, c *apiclient.Client, in sessionAddInput) (string, error) {
	start, err := parseClock(in.Date, in.From)
	if err != nil {
		return "", err
	}
	stop, err := parseClock(in.Date, in.To)
	if err != nil {
		return "", err
	}
	if !stop.After(start) {
		return "", fmt.Errorf("--to (%s) must be after --from (%s)", in.To, in.From)
	}
	pid, err := newProjectResolver(c, false).resolve(ctx, in.Project)
	if err != nil {
		return "", err
	}
	s, err := c.AddSession(ctx, pid, start, stop, in.Tag, in.Note)
	if err != nil {
		return "", fmt.Errorf("add session: %w", err)
	}
	return fmt.Sprintf("added %s  %s–%s", s.ID, in.From, in.To), nil
}

func sessionAddCmd() *cobra.Command {
	var in sessionAddInput
	cmd := &cobra.Command{
		Use:   "add",
		Short: "backfill a past session (--date --from --to)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			out, err := runSessionAdd(cmd.Context(), c, in)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&in.Date, "date", "", "day to book (yyyy-mm-dd, required)")
	cmd.Flags().StringVar(&in.From, "from", "", "start time HH:MM (required)")
	cmd.Flags().StringVar(&in.To, "to", "", "stop time HH:MM (required)")
	cmd.Flags().StringVar(&in.Project, "project", "", "project name (created if new)")
	cmd.Flags().StringVar(&in.Tag, "tag", "", "optional tag")
	cmd.Flags().StringVar(&in.Note, "note", "", "optional note")
	_ = cmd.MarkFlagRequired("date")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}
```

  **Verify** `newProjectResolver`'s real constructor + method names against `docs_import.go` (Step from References) and adjust the call if they differ (e.g. `resolve` vs `Resolve`).

- [ ] **Step 5: Register the command** in `cmd/flow/main.go` after `root.AddCommand(worktimeCmd())`:

```go
	root.AddCommand(sessionCmd())
```

- [ ] **Step 6: Run the tests**

Run: `go test ./cmd/flow/ -run 'TestParseClock|TestRunSessionAdd' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/flow/session.go cmd/flow/session_test.go cmd/flow/main.go
git commit -m "feat(cli): flow session add — backfill a past session"
```

---

### Task 2: `flow session list`

**Files:**
- Modify: `cmd/flow/session.go`
- Test: `cmd/flow/session_test.go`

**Interfaces:**
- Consumes: `parseClock`, `ListSessionsRange`.
- Produces: `func runSessionList(ctx, c *apiclient.Client, dateStr, from, to string) (string, error)` — if `dateStr` set, range is `[startOfDay, +24h)`; else `[from 00:00, to 00:00+24h)`. Returns a printable table (one line per session: `id  HH:MM–HH:MM  dur  project  tag`).
- Produces: `func sessionListCmd() *cobra.Command` wired into `sessionCmd`.

- [ ] **Step 1: Write the failing test:**

```go
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
```

  Add `"strings"` to the test imports.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./cmd/flow/ -run TestRunSessionList -v`
Expected: FAIL (undefined `runSessionList`).

- [ ] **Step 3: Implement** in `session.go`:

```go
func sessionRange(dateStr, from, to string) (since, until time.Time, err error) {
	if dateStr != "" {
		d, e := time.ParseInLocation("2006-01-02", dateStr, time.Local)
		if e != nil {
			return since, until, fmt.Errorf("bad --date %q: %w", dateStr, e)
		}
		d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.Local)
		return d, d.AddDate(0, 0, 1), nil
	}
	if from == "" || to == "" {
		return since, until, fmt.Errorf("need --date or both --from and --to (yyyy-mm-dd)")
	}
	a, e1 := time.ParseInLocation("2006-01-02", from, time.Local)
	b, e2 := time.ParseInLocation("2006-01-02", to, time.Local)
	if e1 != nil || e2 != nil {
		return since, until, fmt.Errorf("bad --from/--to (want yyyy-mm-dd)")
	}
	return a, b.AddDate(0, 0, 1), nil
}

func fmtHM(t time.Time) string { return t.Local().Format("15:04") }

func runSessionList(ctx context.Context, c *apiclient.Client, dateStr, from, to string) (string, error) {
	since, until, err := sessionRange(dateStr, from, to)
	if err != nil {
		return "", err
	}
	sessions, err := c.ListSessionsRange(ctx, since, until)
	if err != nil {
		return "", err
	}
	projects, err := c.ListProjects(ctx)
	if err != nil {
		return "", err
	}
	name := func(id *string) string {
		if id == nil {
			return "—"
		}
		for _, p := range projects {
			if p.ID == *id {
				return p.Name
			}
		}
		return "—"
	}
	var b strings.Builder
	for _, s := range sessions {
		stop := "…"
		dur := "running"
		if s.Stop != nil {
			stop = fmtHM(*s.Stop)
			d := s.Stop.Sub(s.Start)
			dur = fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
		}
		fmt.Fprintf(&b, "%s  %s–%s  %s  %-16s %s\n",
			s.ID, fmtHM(s.Start), stop, dur, name(s.ProjectID), s.Tag)
	}
	return b.String(), nil
}

func sessionListCmd() *cobra.Command {
	var dateStr, from, to string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list sessions for a day (--date) or range (--from --to)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			out, err := runSessionList(cmd.Context(), c, dateStr, from, to)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&dateStr, "date", "", "single day (yyyy-mm-dd)")
	cmd.Flags().StringVar(&from, "from", "", "range start day (yyyy-mm-dd)")
	cmd.Flags().StringVar(&to, "to", "", "range end day inclusive (yyyy-mm-dd)")
	return cmd
}
```

  Add `"strings"` to `session.go` imports. Wire it: `cmd.AddCommand(sessionAddCmd(), sessionListCmd())` in `sessionCmd()`.

  **Note:** `fmtHM` is defined here for the CLI package; it is independent of the webui `fmtHM` (different package). If a `fmtHM`/duration helper already exists in `cmd/flow` (grep first), reuse it instead of redefining.

- [ ] **Step 4: Run the test**

Run: `go test ./cmd/flow/ -run TestRunSessionList -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/flow/session.go cmd/flow/session_test.go
git commit -m "feat(cli): flow session list — day/range session table"
```

---

### Task 3: `flow session delete` + `flow session edit`

**Files:**
- Modify: `cmd/flow/session.go`
- Test: `cmd/flow/session_test.go`

**Interfaces:**
- Consumes: `parseClock`, `DeleteSession`, `EditSession`, `ListSessionsRange`, `newProjectResolver`.
- Produces: `func runSessionDelete(ctx, c, id string) error`.
- Produces: `func findSession(ctx, c *apiclient.Client, id string) (domain.WorkSession, error)` — scans `ListSessionsRange(now-366d, now+2d)` and returns the match or an error.
- Produces: `func runSessionEdit(ctx, c, id string, in sessionEditInput) (string, error)` where `sessionEditInput` carries `*string` fields (`From, To, Project, Tag, Note`); nil = leave unchanged. Merges over the fetched session, then calls `EditSession`.
- Produces: `sessionDeleteCmd()`, `sessionEditCmd()` wired into `sessionCmd`. The edit command builds `sessionEditInput` from `cmd.Flags().Changed(...)`.

- [ ] **Step 1: Write failing tests:**

```go
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
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./cmd/flow/ -run 'TestRunSessionDelete|TestRunSessionEdit' -v`
Expected: FAIL.

- [ ] **Step 3: Implement** in `session.go`:

```go
func runSessionDelete(ctx context.Context, c *apiclient.Client, id string) error {
	return c.DeleteSession(ctx, id)
}

func findSession(ctx context.Context, c *apiclient.Client, id string) (domain.WorkSession, error) {
	now := time.Now()
	list, err := c.ListSessionsRange(ctx, now.AddDate(-1, 0, -1), now.AddDate(0, 0, 2))
	if err != nil {
		return domain.WorkSession{}, err
	}
	for _, s := range list {
		if s.ID == id {
			return s, nil
		}
	}
	return domain.WorkSession{}, fmt.Errorf("session %q not found in the last year", id)
}

type sessionEditInput struct {
	From, To, Project, Tag, Note *string
}

func runSessionEdit(ctx context.Context, c *apiclient.Client, id string, in sessionEditInput) (string, error) {
	cur, err := findSession(ctx, c, id)
	if err != nil {
		return "", err
	}
	dateStr := cur.Start.Local().Format("2006-01-02")
	start := cur.Start
	if in.From != nil {
		if start, err = parseClock(dateStr, *in.From); err != nil {
			return "", err
		}
	}
	stop := cur.Stop
	if in.To != nil {
		t, err := parseClock(dateStr, *in.To)
		if err != nil {
			return "", err
		}
		stop = &t
	}
	if stop == nil || !stop.After(start) {
		return "", fmt.Errorf("resulting range is invalid (stop must be after start)")
	}
	projectID := cur.ProjectID
	if in.Project != nil {
		if projectID, err = newProjectResolver(c, false).resolve(ctx, *in.Project); err != nil {
			return "", err
		}
	}
	tag := cur.Tag
	if in.Tag != nil {
		tag = *in.Tag
	}
	note := cur.Note
	if in.Note != nil {
		note = *in.Note
	}
	if _, err := c.EditSession(ctx, id, projectID, tag, note, start, stop); err != nil {
		return "", fmt.Errorf("edit session: %w", err)
	}
	return fmt.Sprintf("edited %s", id), nil
}
```

- [ ] **Step 4: Add the cobra commands** and a `flag.Changed`-based input builder:

```go
func sessionDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "delete a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			return runSessionDelete(cmd.Context(), c, args[0])
		},
	}
}

func sessionEditCmd() *cobra.Command {
	var from, to, project, tag, note string
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "edit a session (only provided flags change)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			in := sessionEditInput{}
			f := cmd.Flags()
			if f.Changed("from") {
				in.From = &from
			}
			if f.Changed("to") {
				in.To = &to
			}
			if f.Changed("project") {
				in.Project = &project
			}
			if f.Changed("tag") {
				in.Tag = &tag
			}
			if f.Changed("note") {
				in.Note = &note
			}
			out, err := runSessionEdit(cmd.Context(), c, args[0], in)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "new start HH:MM")
	cmd.Flags().StringVar(&to, "to", "", "new stop HH:MM")
	cmd.Flags().StringVar(&project, "project", "", "new project name")
	cmd.Flags().StringVar(&tag, "tag", "", "new tag")
	cmd.Flags().StringVar(&note, "note", "", "new note")
	return cmd
}
```

  Update `sessionCmd()`: `cmd.AddCommand(sessionAddCmd(), sessionListCmd(), sessionEditCmd(), sessionDeleteCmd())`.

- [ ] **Step 5: Run the tests**

Run: `go test ./cmd/flow/ -run 'TestRunSessionDelete|TestRunSessionEdit' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/flow/session.go cmd/flow/session_test.go
git commit -m "feat(cli): flow session edit/delete — partial-flag edit + delete"
```

---

### Task 4: Full gate + help-text sanity

**Files:** none new

- [ ] **Step 1: Sanity-check the command tree.** Run `go run ./cmd/flow session --help` and `go run ./cmd/flow session add --help`. Confirm all four subcommands appear and required flags are marked. (No store/network needed for `--help`.)

- [ ] **Step 2: Run the full CI gate**

Run: `make ci`
Expected: lint clean (watch `ineffassign`, `QF1002`, `unused`), build, tests pass, coverage ≥ gate (~83%). The new helpers are well-covered by Tasks 1-3 tests; if coverage dipped, add a `TestSessionRange_FromTo` test for the `--from/--to` branch of `sessionRange`.

- [ ] **Step 3: Commit any fixups**

```bash
git add -A
git commit -m "test(cli): cover session range + lint fixups"
```

---

## Done-gate (live, after merge-readiness)

With the dev stack running (`make dev-up && make dev-run`) and a logged-in CLI (`flow login`):

1. `flow session add --date 2026-06-18 --from 09:00 --to 11:00 --project Acme` → prints `added …`.
2. `flow session list --date 2026-06-18` → shows the row with `02:00` duration + `Acme`.
3. `flow session add --date 2026-06-18 --from 10:00 --to 10:30` (overlap) → non-zero exit, error mentions overlap/409.
4. `flow session edit <id> --to 12:00` → `flow session list` shows the extended stop, tag/note preserved.
5. `flow session delete <id>` → row gone from `list`.
6. Cross-check the TUI/WebUI day view reflects the same sessions (shared backend).

## Self-review checklist (run before handoff)

- Spec §Slice 4 coverage: `add --date --from --to --project [--tag --note]` ✓ (Task 1); `edit <id> [flags]` partial ✓ (Task 3); `delete <id>` ✓ (Task 3); `list [--date | --from --to]` ✓ (Task 2). Cmd-level happy paths + validation error surfacing ✓ (every helper has an error-path test or validation).
- No placeholders except the **flagged read** of `newProjectResolver`'s exact name (Task 1 References + Step 4) and `apiclient.do` error surfacing (Task 1 Step 1) — resolve from code, don't guess.
- Type consistency: `sessionAddInput` (value fields) vs `sessionEditInput` (pointer fields) are intentionally different — add needs all, edit needs "changed-only". `parseClock` signature identical across all callers.
- Reuse check: `newProjectResolver` reused, not duplicated; `fmtHM` only defined if `cmd/flow` lacks an equivalent.
