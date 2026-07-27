# Worktime Nachbuchen — Slice 3 (WebUI) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the WebUI `/ui/worktime` page (today-only) into a date-navigable day view with prev/next day controls, per-row edit/delete, and a Nachbuchen (backfill) form — all wired to the already-shipped backend usecases.

**Architecture:** The server-rendered `WorktimeFragment` gains a viewed-date dimension. `worktimeData` is parameterized by a day; sessions for that day come from `ListSessionsRange([start, start+24h))` (already wired into `Server`). Three new HTMX `POST` handlers (`add`/`edit`/`delete`) mutate via `AddSession`/`EditSession`/`DeleteSession`, publish the existing SSE events, and re-render the fragment for the **same** viewed day. Overlap rejection (HTTP 409 from the usecases) surfaces as an inline error string in the view model.

**Tech Stack:** Go, `templ` (regenerate `*_templ.go` with `templ generate`), Go stdlib `net/http` mux, Tailwind classes (inline), HTMX 2 + SSE ext (already loaded by the page shell).

## Global Constraints

- Module path: `github.com/serverkraken/flow`. Go 1.x as per `go.mod`.
- No monoliths: new handlers + helpers go in a **new** file `internal/adapter/httpserver/webui_worktime.go`, not appended to `webui.go`.
- All times are handled in **local** tz for parsing HH:MM (matches the TUI's `wtfmt` and the today-fragment's `s.Start.Local().Format("15:04")`).
- Cross-midnight backfill is **out of scope**: reject `to <= from`.
- Every mutating handler must `s.Bus.Publish(...)` the matching `domain.Event` (so other clients live-update), exactly like `handleWebStart`/`handleWebStop`.
- After any change: `make ci` green (~83% gate). Run `templ generate` whenever a `.templ` file changes, and commit the regenerated `_templ.go`.
- Verb/label copy stays lowercase-terse to match the existing fragment ("start timer", "running", "stop").

---

## Reference: existing shapes (read before starting)

- `internal/adapter/webui/worktime.templ` — `WorktimeData{User, Running *domain.WorkSession, Now time.Time, Sessions []domain.WorkSession, Projects []domain.Project}`; `WorktimePage` (full doc) + `WorktimeFragment` (the `#wt` inner HTML). The fragment's `#wt` div has `hx-get="/ui/worktime"` and reloads on SSE `session.*`/`project.created`.
- `internal/adapter/httpserver/webui.go` — `worktimeData(ctx, u)`, `renderFragment`, `handleWebHome`, `handleWebFragment`, `handleWebStart`, `handleWebStop`. `userFrom(ctx)` returns `(domain.User, bool)`.
- `internal/adapter/httpserver/server.go:111-114` — web route registration block (`s.webAuth(http.HandlerFunc(...))`).
- `internal/adapter/httpserver/worktime.go:220` — `startOfDay(t time.Time) time.Time`.
- `Server` usecase fields (server.go:20-28): `AddSession`, `EditSession`, `DeleteSession`, `ListSessionsRange`, `CreateProject`, `ListProjects`, plus `Bus`, `Clock`.
- `internal/adapter/webui/format.go` — `fmtDur`, `fmtInt`; templ helpers live here.
- `domain.WorkSession{ID, ProjectID *string, Tag, Note string, Start time.Time, Stop *time.Time}`; `Running()`, `Elapsed(now)`.
- Event constants: `domain.EventSessionStarted/Stopped`, `domain.EventProjectCreated`. **Check** for `domain.EventSessionUpdated` (grep `EventSession`); the TUI reloads on `session.updated`. If it exists, use it for add/edit/delete; if only started/stopped exist, publish `EventSessionStopped` for add/edit and `EventSessionStarted`... no — verify and use the truthful event. (Task 1 resolves this.)

---

### Task 1: View-model + date helpers (no behavior change yet)

**Files:**
- Modify: `internal/adapter/webui/worktime.templ` (struct only this task)
- Create: `internal/adapter/httpserver/webui_worktime.go`
- Test: `internal/adapter/httpserver/webui_worktime_test.go`

**Interfaces:**
- Produces: `WorktimeData` gains fields `Date time.Time`, `IsToday bool`, `PrevDate string`, `NextDate string`, `CanForward bool`, `Err string` (all additive; existing fields unchanged).
- Produces: `func (s *Server) worktimeDataFor(ctx context.Context, u domain.User, day time.Time, errMsg string) (webui.WorktimeData, error)` — `day` is any instant on the target date; uses `startOfDay`.
- Produces: `func parseDayParam(s *Server, q string) time.Time` — parses `yyyy-mm-dd` (local) from a query value; empty/invalid → today (`startOfDay(s.Clock.Now())`).
- Produces: `func dayTime(day time.Time, hhmm string) (time.Time, error)` — combines a start-of-day date with `HH:MM` in local tz.

- [ ] **Step 1: Resolve the SSE event truth.** Run `rg -n "EventSession" internal/domain` and note exactly which constants exist. Record the set to use for add/edit/delete in a comment at the top of `webui_worktime.go`.

- [ ] **Step 2: Write the failing test** for the date/time helpers.

```go
package httpserver

import (
	"testing"
	"time"
)

func TestParseDayParam_DefaultsToToday(t *testing.T) {
	s := &Server{Clock: fixedClock{now: time.Date(2026, 6, 21, 14, 0, 0, 0, time.Local)}}
	got := parseDayParam(s, "")
	want := time.Date(2026, 6, 21, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("empty: got %v want %v", got, want)
	}
	got = parseDayParam(s, "2026-06-18")
	want = time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("valid: got %v want %v", got, want)
	}
	if !parseDayParam(s, "garbage").Equal(time.Date(2026, 6, 21, 0, 0, 0, 0, time.Local)) {
		t.Fatal("garbage should fall back to today")
	}
}

func TestDayTime_LocalHHMM(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	got, err := dayTime(day, "09:30")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 6, 18, 9, 30, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if _, err := dayTime(day, "nope"); err == nil {
		t.Fatal("expected error for bad HH:MM")
	}
}
```

- [ ] **Step 3: Check whether `fixedClock` already exists** in the httpserver test package (`rg -n "fixedClock|type.*Clock" internal/adapter/httpserver`). If a test clock exists, use it and delete the `fixedClock` definition from your test. If not, add this minimal one to your test file:

```go
type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }
```

(Match the real `Clock` interface — grep `type Clock` to confirm the method set; adjust if it has more methods.)

- [ ] **Step 4: Run the test to verify it fails**

Run: `go test ./internal/adapter/httpserver/ -run 'TestParseDayParam|TestDayTime' -v`
Expected: FAIL (undefined `parseDayParam`, `dayTime`).

- [ ] **Step 5: Implement the helpers** in `internal/adapter/httpserver/webui_worktime.go`:

```go
package httpserver

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// dayLayout is the yyyy-mm-dd form used by the date-nav query param and forms.
const dayLayout = "2006-01-02"

// parseDayParam resolves a yyyy-mm-dd query value to a local start-of-day,
// falling back to today on empty/invalid input.
func parseDayParam(s *Server, q string) time.Time {
	if t, err := time.ParseInLocation(dayLayout, q, time.Local); err == nil {
		return startOfDay(t)
	}
	return startOfDay(s.Clock.Now())
}

// dayTime combines a start-of-day date with an HH:MM clock time in local tz.
func dayTime(day time.Time, hhmm string) (time.Time, error) {
	clock, err := time.ParseInLocation("15:04", hhmm, time.Local)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(day.Year(), day.Month(), day.Day(),
		clock.Hour(), clock.Minute(), 0, 0, time.Local), nil
}

// worktimeDataFor builds the worktime view model for a specific local day.
func (s *Server) worktimeDataFor(ctx context.Context, u domain.User, day time.Time, errMsg string) (webui.WorktimeData, error) {
	day = startOfDay(day)
	sessions, err := s.ListSessionsRange.Execute(ctx, u.ID, day, day.AddDate(0, 0, 1))
	if err != nil {
		return webui.WorktimeData{}, err
	}
	projects, err := s.ListProjects.Execute(ctx, u.ID)
	if err != nil {
		return webui.WorktimeData{}, err
	}
	today := startOfDay(s.Clock.Now())
	isToday := day.Equal(today)
	var running *domain.WorkSession
	if isToday {
		for i := range sessions {
			if sessions[i].Running() {
				r := sessions[i]
				running = &r
			}
		}
	}
	next := day.AddDate(0, 0, 1)
	return webui.WorktimeData{
		User:       u.Username,
		Running:    running,
		Now:        s.Clock.Now(),
		Sessions:   sessions,
		Projects:   projects,
		Date:       day,
		IsToday:    isToday,
		PrevDate:   day.AddDate(0, 0, -1).Format(dayLayout),
		NextDate:   next.Format(dayLayout),
		CanForward: !next.After(today), // clamp: never navigate past today
		Err:        errMsg,
	}, nil
}
```

- [ ] **Step 6: Add the new struct fields** to `WorktimeData` in `worktime.templ`:

```go
type WorktimeData struct {
	User       string
	Running    *domain.WorkSession
	Now        time.Time
	Sessions   []domain.WorkSession
	Projects   []domain.Project
	Date       time.Time
	IsToday    bool
	PrevDate   string
	NextDate   string
	CanForward bool
	Err        string
}
```

- [ ] **Step 7: Regenerate templ + run the test**

Run: `templ generate ./internal/adapter/webui/ && go test ./internal/adapter/httpserver/ -run 'TestParseDayParam|TestDayTime' -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/adapter/webui/worktime.templ internal/adapter/webui/worktime_templ.go internal/adapter/httpserver/webui_worktime.go internal/adapter/httpserver/webui_worktime_test.go
git commit -m "feat(webui/worktime): date-aware view model + day/time helpers"
```

---

### Task 2: Render the day view (nav + rows + form), still read-only

**Files:**
- Modify: `internal/adapter/webui/worktime.templ` (fragment markup)
- Modify: `internal/adapter/webui/format.go` (add `fmtHM`, `projName`)
- Modify: `internal/adapter/httpserver/webui.go` (route the date param into the existing GET handlers)
- Test: `internal/adapter/webui/render_test.go` (extend) + `internal/adapter/httpserver/webui_worktime_test.go`

**Interfaces:**
- Consumes: `WorktimeData` fields from Task 1; `worktimeDataFor`, `parseDayParam`.
- Produces: `func fmtHM(t time.Time) string` (local `15:04`); `func projName(projects []domain.Project, id *string) string` (name or "—").
- Produces: GET `/ui/worktime?date=yyyy-mm-dd` and GET `/{$}?date=…` render the requested day. The Nachbuchen/edit/delete forms are present in markup but their POST targets are wired in Task 3 (forms still render now; posting 404s until Task 3 — acceptable mid-plan).

- [ ] **Step 1: Write the failing render test** in `internal/adapter/webui/render_test.go`:

```go
func TestWorktimeFragment_PastDayShowsNavAndForm(t *testing.T) {
	pid := "p1"
	d := WorktimeData{
		User: "alice",
		Now:  time.Date(2026, 6, 21, 12, 0, 0, 0, time.Local),
		Date: time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local),
		Sessions: []domain.WorkSession{{
			ID: "s1", ProjectID: &pid,
			Start: time.Date(2026, 6, 18, 9, 0, 0, 0, time.Local),
			Stop:  ptr(time.Date(2026, 6, 18, 11, 30, 0, 0, time.Local)),
		}},
		Projects:   []domain.Project{{ID: "p1", Name: "Acme"}},
		IsToday:    false,
		PrevDate:   "2026-06-17",
		NextDate:   "2026-06-19",
		CanForward: true,
	}
	var b strings.Builder
	if err := WorktimeFragment(d).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	for _, want := range []string{
		"2026-06-17",            // prev-day link target
		"2026-06-19",            // next-day link target
		"09:00",                 // session start HH:MM
		"Acme",                  // project name
		`name="from"`,           // Nachbuchen form field
		`action="/ui/worktime/add"`,    // (htmx hx-post target string)
		`/ui/worktime/delete`,   // per-row delete target
	} {
		if !strings.Contains(html, want) {
			t.Errorf("fragment missing %q", want)
		}
	}
	if strings.Contains(html, "start timer") {
		t.Error("past day must not show the start-timer card")
	}
}
```

Add a `ptr` helper to the test file if absent: `func ptr[T any](v T) *T { return &v }`. Ensure imports include `strings`, `context`, `time`, and the `domain` package.

- [ ] **Step 2: Run it to verify failure**

Run: `go test ./internal/adapter/webui/ -run TestWorktimeFragment_PastDayShowsNavAndForm -v`
Expected: FAIL (markup not present).

- [ ] **Step 3: Add the format helpers** to `internal/adapter/webui/format.go`:

```go
// fmtHM renders a timestamp as local HH:MM.
func fmtHM(t time.Time) string { return t.Local().Format("15:04") }

// projName resolves a project id to its name, or "—" when unset/unknown.
func projName(projects []domain.Project, id *string) string {
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
```

Add `"github.com/serverkraken/flow/internal/domain"` to format.go imports.

- [ ] **Step 4: Rewrite `WorktimeFragment`** in `worktime.templ`. Keep the header link bar. Replace the Today section. Structure:
  - **Date-nav row:** `‹` link `hx-get="/ui/worktime?date={ d.PrevDate }"` (always); the day label `{ d.Date.Format("Mon 02.01.2006") }`; `›` link `hx-get="/ui/worktime?date={ d.NextDate }"` rendered only `if d.CanForward` (else a dimmed inert span). All nav links use `hx-target="#wt" hx-swap="innerHTML"`.
  - **Running card / start button:** render the existing running-card + start/stop block **only** `if d.IsToday`.
  - **Error banner:** `if d.Err != "" { <p class="...text-rose-600">{ d.Err }</p> }`.
  - **Session list:** for each `s`, a row showing `{ fmtHM(s.Start) }–{ stopLabel }` · `{ projName(d.Projects, s.ProjectID) }` · `{ fmtDur(s.Elapsed(d.Now)) }`, plus a `<details>` inline **edit** form and a **delete** form. Use a small templ helper `stopHM(s)` returning `fmtHM(*s.Stop)` or `"…"` when running.
  - **Nachbuchen form** (always shown): fields `from`, `to` (`type="text" placeholder="09:00"`), a project `<select name="projectId">` over `d.Projects` + `<input name="newProject">`, `tag`, `note`, and a hidden `<input type="hidden" name="date" value={ d.Date.Format("2006-01-02") }>`. `hx-post="/ui/worktime/add" hx-target="#wt" hx-swap="innerHTML"`.

  Concrete markup (drop into the fragment, after the header):

```html
<div class="mt-4 flex items-center justify-between text-sm">
	<a class="rounded px-2 py-1 hover:bg-slate-100" hx-get={ "/ui/worktime?date=" + d.PrevDate } hx-target="#wt" hx-swap="innerHTML">‹ prev</a>
	<span class="font-medium text-slate-700">{ d.Date.Format("Mon 02.01.2006") }</span>
	if d.CanForward {
		<a class="rounded px-2 py-1 hover:bg-slate-100" hx-get={ "/ui/worktime?date=" + d.NextDate } hx-target="#wt" hx-swap="innerHTML">next ›</a>
	} else {
		<span class="px-2 py-1 text-slate-300">next ›</span>
	}
</div>
if d.Err != "" {
	<p class="mt-2 rounded bg-rose-50 px-3 py-2 text-sm text-rose-600">{ d.Err }</p>
}
```

  Running/start block, wrapped:

```html
if d.IsToday {
	// ... existing running-card + start/stop form, unchanged ...
}
```

  Session rows (replace the old `<ul>`):

```html
<section class="mt-6">
	<h2 class="mb-2 text-sm font-semibold uppercase tracking-wide text-slate-500">Sessions</h2>
	if len(d.Sessions) == 0 {
		<p class="text-sm text-slate-400">no sessions</p>
	}
	<ul class="divide-y divide-slate-100">
		for _, s := range d.Sessions {
			<li class="py-2 text-sm">
				<div class="flex items-center justify-between">
					<span>{ fmtHM(s.Start) }–{ stopHM(s) } · { projName(d.Projects, s.ProjectID) }</span>
					<span class="flex items-center gap-2">
						<span class="tabular-nums text-slate-500">{ fmtDur(s.Elapsed(d.Now)) }</span>
						if !s.Running() {
							<form hx-post="/ui/worktime/delete" hx-target="#wt" hx-swap="innerHTML" hx-confirm="delete this session?">
								<input type="hidden" name="sessionId" value={ s.ID }/>
								<input type="hidden" name="date" value={ d.Date.Format("2006-01-02") }/>
								<button class="text-slate-400 hover:text-rose-600">✕</button>
							</form>
						}
					</span>
				</div>
				if !s.Running() {
					<details class="mt-1">
						<summary class="cursor-pointer text-xs text-slate-400 hover:text-slate-600">edit</summary>
						<form hx-post="/ui/worktime/edit" hx-target="#wt" hx-swap="innerHTML" class="mt-2 flex flex-wrap gap-2">
							<input type="hidden" name="sessionId" value={ s.ID }/>
							<input type="hidden" name="date" value={ d.Date.Format("2006-01-02") }/>
							<input name="from" value={ fmtHM(s.Start) } class="w-16 rounded border px-2 py-1"/>
							<input name="to" value={ stopHM(s) } class="w-16 rounded border px-2 py-1"/>
							<input name="tag" value={ s.Tag } placeholder="tag" class="w-24 rounded border px-2 py-1"/>
							<input name="note" value={ s.Note } placeholder="note" class="flex-1 rounded border px-2 py-1"/>
							<button class="rounded bg-slate-900 px-3 py-1 text-white">save</button>
						</form>
					</details>
				}
			</li>
		}
	</ul>
</section>
<section class="mt-6">
	<h2 class="mb-2 text-sm font-semibold uppercase tracking-wide text-slate-500">Nachbuchen</h2>
	<form hx-post="/ui/worktime/add" hx-target="#wt" hx-swap="innerHTML" class="flex flex-wrap gap-2">
		<input type="hidden" name="date" value={ d.Date.Format("2006-01-02") }/>
		<input name="from" placeholder="09:00" class="w-20 rounded border px-2 py-1"/>
		<input name="to" placeholder="17:00" class="w-20 rounded border px-2 py-1"/>
		<select name="projectId" class="rounded border px-2 py-1">
			<option value="">— project —</option>
			for _, p := range d.Projects {
				<option value={ p.ID }>{ p.Name }</option>
			}
		</select>
		<input name="newProject" placeholder="or new…" class="rounded border px-2 py-1"/>
		<input name="tag" placeholder="tag" class="w-24 rounded border px-2 py-1"/>
		<input name="note" placeholder="note" class="flex-1 rounded border px-2 py-1"/>
		<button class="rounded bg-slate-900 px-3 py-1 text-white">add</button>
	</form>
</section>
```

  Add the `stopHM` templ helper to `format.go`:

```go
// stopHM renders a session's stop time as HH:MM, or "…" while running.
func stopHM(s domain.WorkSession) string {
	if s.Stop == nil {
		return "…"
	}
	return fmtHM(*s.Stop)
}
```

- [ ] **Step 5: Route the date param** in `internal/adapter/httpserver/webui.go`. Replace `worktimeData`-based renders so GET handlers honor `?date=`:

```go
func (s *Server) handleWebHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	day := parseDayParam(s, r.URL.Query().Get("date"))
	d, err := s.worktimeDataFor(r.Context(), u, day, "")
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WorktimePage(d).Render(r.Context(), w)
}

func (s *Server) handleWebFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	day := parseDayParam(s, r.URL.Query().Get("date"))
	d, err := s.worktimeDataFor(r.Context(), u, day, "")
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WorktimeFragment(d).Render(r.Context(), w)
}
```

  Leave the old `worktimeData` + `renderFragment` (used by start/stop) in place for now; Task 3 migrates start/stop to carry the date and you can delete `worktimeData`/`renderFragment` then if unused. (Check with `rg -n "renderFragment\|worktimeData\b"` at end of Task 3.)

- [ ] **Step 6: Regenerate templ + run tests**

Run: `templ generate ./internal/adapter/webui/ && go test ./internal/adapter/webui/ ./internal/adapter/httpserver/ -run 'Worktime' -v`
Expected: PASS (the new render test + existing worktime tests). If an existing test asserted the literal "Today" heading or old `<li>` shape, update it to the new "Sessions" markup — these are intentional copy changes.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/webui/ internal/adapter/httpserver/webui.go
git commit -m "feat(webui/worktime): date-nav day view with edit/delete/Nachbuchen forms (markup)"
```

---

### Task 3: Wire the mutating POST handlers

**Files:**
- Modify: `internal/adapter/httpserver/webui_worktime.go` (handlers)
- Modify: `internal/adapter/httpserver/server.go` (register 3 routes)
- Test: `internal/adapter/httpserver/webui_worktime_test.go`

**Interfaces:**
- Consumes: `worktimeDataFor`, `dayTime`, `parseDayParam`, the SSE event set from Task 1 Step 1.
- Produces: `handleWebAdd`, `handleWebEdit`, `handleWebDelete` — each parses the form, mutates, publishes the event, and re-renders the fragment for the form's `date`. On a usecase error (e.g. overlap) it re-renders with `Err` set instead of an HTTP error.

- [ ] **Step 1: Write the failing handler tests.** Use the existing httpserver test harness. Find how other web POST tests build an authenticated `Server` + request (`rg -n "handleWebStart\|webAuth\|newTestServer\|doWeb" internal/adapter/httpserver/*_test.go`) and mirror it. The tests must assert:
  - **add happy path:** POST `/ui/worktime/add` with `date,from,to` for a non-overlapping window → response 200, body contains the new session's HH:MM range; the underlying store gained one session.
  - **add overlap:** posting a window overlapping an existing session → response 200 (not 500) and body contains an error string (the `Err` banner); store unchanged.
  - **delete:** POST `/ui/worktime/delete` with a valid `sessionId` → session gone from store; response lists remaining.
  - **edit:** POST `/ui/worktime/edit` changing `to` → session's stop updated.

```go
func TestWebAdd_BackfillsSession(t *testing.T) {
	srv := newWorktimeTestServer(t) // helper you add or adapt from existing harness
	form := url.Values{
		"date": {"2026-06-18"}, "from": {"09:00"}, "to": {"11:00"},
	}
	res := srv.postForm(t, "/ui/worktime/add", form)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "09:00–11:00") {
		t.Errorf("expected new session row, got:\n%s", res.Body.String())
	}
}

func TestWebAdd_OverlapShowsError(t *testing.T) {
	srv := newWorktimeTestServer(t)
	srv.seedSession(t, "2026-06-18", "09:00", "12:00")
	form := url.Values{"date": {"2026-06-18"}, "from": {"10:00"}, "to": {"11:00"}}
	res := srv.postForm(t, "/ui/worktime/add", form)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d (want 200 with inline error)", res.Code)
	}
	if !strings.Contains(strings.ToLower(res.Body.String()), "overlap") {
		t.Errorf("expected overlap error banner, got:\n%s", res.Body.String())
	}
}
```

  Adapt `newWorktimeTestServer`/`postForm`/`seedSession`/`seedSession` to the existing harness conventions — **do not invent a parallel harness if one exists**; extend it. The overlap assertion substring (`"overlap"`) must match the `Err` text you render in Step 2.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/adapter/httpserver/ -run 'TestWebAdd|TestWebEdit|TestWebDelete' -v`
Expected: FAIL (handlers/routes missing → 404).

- [ ] **Step 3: Implement the handlers** in `webui_worktime.go`:

```go
import "net/http"

// renderDay re-renders the worktime fragment for the given local day,
// optionally with an inline error banner.
func (s *Server) renderDay(w http.ResponseWriter, r *http.Request, u domain.User, day time.Time, errMsg string) {
	d, err := s.worktimeDataFor(r.Context(), u, day, errMsg)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WorktimeFragment(d).Render(r.Context(), w)
}

func (s *Server) handleWebAdd(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	day := parseDayParam(s, r.FormValue("date"))
	start, err1 := dayTime(day, r.FormValue("from"))
	stop, err2 := dayTime(day, r.FormValue("to"))
	if err1 != nil || err2 != nil {
		s.renderDay(w, r, u, day, "invalid time — use HH:MM")
		return
	}
	if !stop.After(start) {
		s.renderDay(w, r, u, day, "to must be after from")
		return
	}
	projectID := s.resolveWebProject(r, u) // shared with stop handler; see Step 4
	if _, err := s.AddSession.Execute(r.Context(), u.ID, projectID, start, stop,
		r.FormValue("tag"), r.FormValue("note")); err != nil {
		s.renderDay(w, r, u, day, "could not add: "+err.Error()) // err includes "overlap"
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionStopped, UserID: u.ID}) // use the verified event constant
	s.renderDay(w, r, u, day, "")
}

func (s *Server) handleWebDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	day := parseDayParam(s, r.FormValue("date"))
	if err := s.DeleteSession.Execute(r.Context(), u.ID, r.FormValue("sessionId")); err != nil {
		s.renderDay(w, r, u, day, "could not delete: "+err.Error())
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionStopped, UserID: u.ID})
	s.renderDay(w, r, u, day, "")
}

func (s *Server) handleWebEdit(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	day := parseDayParam(s, r.FormValue("date"))
	start, err1 := dayTime(day, r.FormValue("from"))
	stop, err2 := dayTime(day, r.FormValue("to"))
	if err1 != nil || err2 != nil || !stop.After(start) {
		s.renderDay(w, r, u, day, "invalid time range")
		return
	}
	projectID := s.resolveWebProject(r, u)
	if _, err := s.EditSession.Execute(r.Context(), u.ID, r.FormValue("sessionId"),
		usecase.EditSessionInput{ /* match the real input shape — see note */ }); err != nil {
		s.renderDay(w, r, u, day, "could not edit: "+err.Error())
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSessionStopped, UserID: u.ID})
	s.renderDay(w, r, u, day, "")
}
```

  **NOTE on EditSession shape:** the apiclient calls `PATCH` with `{projectId, tag, note, start, stop}`. The server's `usecase.EditSession.Execute` signature differs from the client — **read `internal/usecase/edit_session.go` and the existing `handleEditSession` in httpserver** and call it exactly as that handler does (same input struct / argument order). Do not guess the signature; copy the call shape from `handleEditSession`.

- [ ] **Step 4: Extract the shared project resolver.** `handleWebStop` already does select-or-create-new-project. Factor that into:

```go
// resolveWebProject returns the chosen project id from the form, creating a
// new project when "newProject" is filled. Returns nil when neither is set.
func (s *Server) resolveWebProject(r *http.Request, u domain.User) *string {
	projectID := r.FormValue("projectId")
	if name := r.FormValue("newProject"); name != "" {
		if p, err := s.CreateProject.Execute(r.Context(), u.ID, name, "", "", ""); err == nil {
			projectID = p.ID
			s.Bus.Publish(domain.Event{Type: domain.EventProjectCreated, UserID: u.ID})
		}
	}
	if projectID == "" {
		return nil
	}
	return &projectID
}
```

  Then refactor `handleWebStop` to use it (its current inline logic passes `&projectID` even when empty — preserve its existing behavior if a test depends on it; otherwise switch it to `resolveWebProject` and adjust). Keep this refactor minimal and covered by existing stop tests.

- [ ] **Step 5: Register the routes** in `server.go` after line 114:

```go
mux.Handle("POST /ui/worktime/add", s.webAuth(http.HandlerFunc(s.handleWebAdd)))
mux.Handle("POST /ui/worktime/edit", s.webAuth(http.HandlerFunc(s.handleWebEdit)))
mux.Handle("POST /ui/worktime/delete", s.webAuth(http.HandlerFunc(s.handleWebDelete)))
```

  If `routes_test.go` enumerates expected routes, add these three there too.

- [ ] **Step 6: Run the handler tests + full package**

Run: `go test ./internal/adapter/httpserver/ -run 'TestWebAdd|TestWebEdit|TestWebDelete' -v && go test ./internal/adapter/httpserver/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/httpserver/
git commit -m "feat(webui/worktime): add/edit/delete handlers + overlap error banner"
```

---

### Task 4: Full gate + dead-code sweep

**Files:** none new (cleanup only)

- [ ] **Step 1: Remove now-unused code.** Run `rg -n "func \(s \*Server\) worktimeData\b|renderFragment" internal/adapter/httpserver`. If `worktimeData`/`renderFragment` are unused after Task 2/3, delete them; if start/stop still use `renderFragment`, leave it. Confirm with `go build ./...`.

- [ ] **Step 2: Run the full CI gate**

Run: `make ci`
Expected: lint clean (watch `ineffassign`/`QF1002` — run, don't just `go test`), `templ generate` produces no diff (`git diff --stat` shows nothing under `*_templ.go`), tests pass, coverage ≥ gate (~83%).

- [ ] **Step 3: If coverage dipped below the gate,** add a focused render/handler test for the uncovered branch (most likely the `CanForward=false` clamp and the `Err != ""` banner). Re-run `make ci`.

- [ ] **Step 4: Commit any cleanup**

```bash
git add -A
git commit -m "chore(webui/worktime): drop dead worktimeData path + cover nav clamp"
```

---

## Done-gate (live, after merge-readiness)

1. `make dev-up && make dev-run` (Postgres + Dex) — log in via the scripted Dex flow.
2. Open `/` → date-nav `‹ prev`, navigate to a past weekday.
3. Nachbuchen a 09:00–11:00 session → row appears; SSE reloads other tabs.
4. Backfill an overlapping window → inline rose error banner, no 500.
5. Edit the session's `to` via the `<details>` form → duration updates.
6. Delete it → row gone.
7. Confirm `next ›` is inert/absent on today (clamp holds).

## Self-review checklist (run before handoff)

- Spec §Slice 3 coverage: prev/next ✓ (Task 2), list day sessions ✓ (Task 2), Nachbuchen form ✓ (Task 2/3), per-row edit/delete ✓ (Task 2/3), overlap error ✓ (Task 3), new templ fragment + regen ✓.
- No placeholders except the two **explicitly flagged reads** (SSE event constant in Task 1 Step 1; EditSession call shape in Task 3 Step 3) — both are "copy the existing truthful shape", not invented APIs. The implementer MUST resolve them from the codebase, not guess.
- Type consistency: `WorktimeData` field names identical across templ + helpers; `dayLayout` constant reused for every `Format`/`Parse`.
