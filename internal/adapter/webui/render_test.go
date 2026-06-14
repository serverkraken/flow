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
	d := WorktimeData{User: "msoent", Running: &run, Now: start.Add(90 * time.Minute)}
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
	if err := WorktimeFragment(WorktimeData{User: "x"}).Render(context.Background(), &b); err != nil {
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
	}
	var b bytes.Buffer
	if err := WorktimeFragment(d).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"Flow", "Kompendium", "Today", "09:00"} {
		if !strings.Contains(out, want) {
			t.Fatalf("fragment missing %q:\n%s", want, out)
		}
	}
}

func TestWorktimePageWrapsFragment(t *testing.T) {
	var b bytes.Buffer
	if err := WorktimePage(WorktimeData{User: "x"}).Render(context.Background(), &b); err != nil {
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
