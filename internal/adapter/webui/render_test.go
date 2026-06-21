package webui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestFragmentShowsRunningTimer(t *testing.T) {
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	run, _ := domain.NewWorkSession("s1", "u1", nil, start)
	d := WorktimeData{User: "msoent", Running: &run, Now: start.Add(90 * time.Minute), IsToday: true}
	var b bytes.Buffer
	if err := WorktimeFragment(d).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "01:30") {
		t.Fatalf("running elapsed not rendered:\n%s", b.String())
	}
}

func TestFragmentIdleShowsStart(t *testing.T) {
	var b bytes.Buffer
	if err := WorktimeFragment(WorktimeData{User: "x", IsToday: true}).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "start timer") {
		t.Fatal("idle fragment missing start button")
	}
}

// TestFragmentWithProjectsAndSessions exercises the for/if branches in the
// generated templ (project <option>s + Today session rows) — also keeps the
// coverage denominator (templ-generated code) honest.
func TestFragmentWithProjectsAndSessions(t *testing.T) {
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	run, _ := domain.NewWorkSession("s1", "u1", nil, start)
	stop := start.Add(30 * time.Minute)
	done, _ := domain.NewWorkSession("s2", "u1", nil, start.Add(-2*time.Hour))
	done.Stop = &stop
	d := WorktimeData{
		User:     "msoent",
		Running:  &run,
		Now:      start.Add(45 * time.Minute),
		Sessions: []domain.WorkSession{run, done},
		Projects: []domain.Project{{ID: "p1", Name: "Flow"}, {ID: "p2", Name: "Kompendium"}},
		IsToday:  true,
	}
	var b bytes.Buffer
	if err := WorktimeFragment(d).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"Flow", "Kompendium", "Sessions", "09:00"} {
		if !strings.Contains(out, want) {
			t.Fatalf("fragment missing %q:\n%s", want, out)
		}
	}
}

// Regression: the per-row edit form must carry a projectId <select> that
// pre-selects the session's current project. Without it, saving an edit
// (e.g. changing the stop time) submits an empty projectId and silently
// wipes the session's project association.
func TestFragmentEditFormPreselectsProject(t *testing.T) {
	day := time.Date(2026, 6, 18, 0, 0, 0, 0, time.Local)
	pid := "p2"
	done, _ := domain.NewWorkSession("s2", "u1", &pid, day.Add(9*time.Hour))
	stop := day.Add(11 * time.Hour)
	done.Stop = &stop
	d := WorktimeData{
		User:     "msoent",
		Now:      time.Date(2026, 6, 21, 12, 0, 0, 0, time.Local),
		Date:     day,
		Sessions: []domain.WorkSession{done},
		Projects: []domain.Project{{ID: "p1", Name: "Flow"}, {ID: "p2", Name: "Kompendium"}},
		IsToday:  false,
	}
	var b bytes.Buffer
	if err := WorktimeFragment(d).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `name="projectId"`) {
		t.Fatalf("edit form has no projectId select:\n%s", out)
	}
	if !strings.Contains(out, `value="p2" selected`) {
		t.Errorf("current project p2 not pre-selected in edit form:\n%s", out)
	}
}

func TestWorktimePageWrapsFragment(t *testing.T) {
	var b bytes.Buffer
	if err := WorktimePage(WorktimeData{User: "x", IsToday: true}).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "sse-connect=\"/api/v1/events\"") || !strings.Contains(out, "start timer") {
		t.Fatalf("page wrapper missing SSE wiring or fragment:\n%s", out)
	}
}

func TestFmtDur(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{90 * time.Minute, "01:30"},
		{0, "00:00"},
		{-time.Minute, "00:00"},
		{61 * time.Minute, "01:01"},
	}
	for _, tc := range cases {
		got := fmtDur(tc.d)
		if got != tc.want {
			t.Errorf("fmtDur(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestMonthBarPct_Clamping(t *testing.T) {
	// Below zero → clamp to 0.
	if got := monthBarPct(StatsData{MonthPct: -10}); got != 0 {
		t.Errorf("monthBarPct(-10): want 0, got %d", got)
	}
	// Above 100 → clamp to 100.
	if got := monthBarPct(StatsData{MonthPct: 150}); got != 100 {
		t.Errorf("monthBarPct(150): want 100, got %d", got)
	}
	// In range → pass through.
	if got := monthBarPct(StatsData{MonthPct: 75}); got != 75 {
		t.Errorf("monthBarPct(75): want 75, got %d", got)
	}
}

func TestWeekBarStyle_Clamping(t *testing.T) {
	// Below zero → width: 0%.
	got := weekBarStyle(StatsWeekRow{Pct: -5})
	if got != "width: 0%" {
		t.Errorf("weekBarStyle(pct=-5): want %q, got %q", "width: 0%", got)
	}
	// Above 100 → width: 100%.
	got = weekBarStyle(StatsWeekRow{Pct: 120})
	if got != "width: 100%" {
		t.Errorf("weekBarStyle(pct=120): want %q, got %q", "width: 100%", got)
	}
	// Normal → pass through.
	got = weekBarStyle(StatsWeekRow{Pct: 60})
	if got != "width: 60%" {
		t.Errorf("weekBarStyle(pct=60): want %q, got %q", "width: 60%", got)
	}
}

func TestWeekDay_Total_ActivePath(t *testing.T) {
	now := time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)
	// An active session that started today at 12:00 → 2h elapsed.
	activeStart := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	wd := domain.WeekDay{
		Date:    time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		Logged:  30 * time.Minute,
		Active:  &activeStart,
		Target:  8 * time.Hour,
		IsToday: true,
	}
	got := wd.Total(now)
	// Logged (30m) + active elapsed (14:00 - 12:00 = 2h) = 2h30m.
	want := 2*time.Hour + 30*time.Minute
	if got != want {
		t.Errorf("WeekDay.Total(active): want %v, got %v", want, got)
	}
}

func TestWeekDay_Total_ActiveBeforeMidnight(t *testing.T) {
	now := time.Date(2026, 6, 15, 1, 0, 0, 0, time.UTC)
	// Active session started yesterday before midnight: should be clamped to midnight.
	activeStart := time.Date(2026, 6, 14, 23, 0, 0, 0, time.UTC)
	wd := domain.WeekDay{
		Date:    time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		Logged:  0,
		Active:  &activeStart,
		Target:  8 * time.Hour,
		IsToday: true,
	}
	got := wd.Total(now)
	// Clamped to midnight; elapsed = 01:00 - 00:00 = 1h.
	want := time.Hour
	if got != want {
		t.Errorf("WeekDay.Total(active before midnight): want %v, got %v", want, got)
	}
}

func ptr[T any](v T) *T { return &v }

// TestFragmentShowsBindings verifies that WorktimeFragment renders the
// project-bindings panel when WorktimeData.Bindings is populated.
func TestFragmentShowsBindings(t *testing.T) {
	bindings := []domain.ProjectBinding{
		{
			ID: "b1", OwnerID: "u1", ProjectID: "p1",
			Kind:       domain.BindingRemote,
			RemoteSlug: "serverkraken/flow",
		},
		{
			ID: "b2", OwnerID: "u1", ProjectID: "p1",
			Kind:         domain.BindingPath,
			MachineLabel: "laptop",
			Path:         "/home/user/projects/flow",
		},
	}
	d := WorktimeData{
		User:     "msoent",
		IsToday:  false,
		Date:     time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC),
		Now:      time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC),
		Bindings: bindings,
	}
	var b bytes.Buffer
	if err := WorktimeFragment(d).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "serverkraken/flow") {
		t.Errorf("fragment missing remote slug %q:\n%s", "serverkraken/flow", out)
	}
	if !strings.Contains(out, "laptop") {
		t.Errorf("fragment missing machine label %q:\n%s", "laptop", out)
	}
	if !strings.Contains(out, "/home/user/projects/flow") {
		t.Errorf("fragment missing path %q:\n%s", "/home/user/projects/flow", out)
	}
}

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
		"2026-06-17",                 // prev-day link target
		"2026-06-19",                 // next-day link target
		"09:00",                      // session start HH:MM
		"Acme",                       // project name
		`name="from"`,                // Nachbuchen form field
		`hx-post="/ui/worktime/add"`, // Nachbuchen form hx-post target
		`/ui/worktime/delete`,        // per-row delete target
	} {
		if !strings.Contains(html, want) {
			t.Errorf("fragment missing %q", want)
		}
	}
	if strings.Contains(html, "start timer") {
		t.Error("past day must not show the start-timer card")
	}
}
