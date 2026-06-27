package webui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// TestNodeMoveFormRender exercises nodeMoveForm with both a hidden (no targets,
// no parent) and a visible (targets present) cockpit — covers the template code
// added in D5.
func TestNodeMoveFormRender(t *testing.T) {
	t.Parallel()

	// Case 1: no targets, no parent → form must be empty (condition false).
	d1 := NodeCockpit{
		N:           domain.Node{ID: "eng1", Kind: domain.KindEngagement, Name: "Privat"},
		MoveTargets: nil,
	}
	var buf1 bytes.Buffer
	if err := nodeMoveForm(d1).Render(context.Background(), &buf1); err != nil {
		t.Fatalf("render hidden form: %v", err)
	}
	if strings.Contains(buf1.String(), "<form") {
		t.Errorf("form must be hidden when no targets and no parent, got: %s", buf1.String())
	}

	// Case 2: has move targets → form must render with a select.
	eng2 := domain.Node{ID: "eng2", Kind: domain.KindEngagement, Name: "RTL"}
	eng1ID := "eng1"
	d2 := NodeCockpit{
		N:           domain.Node{ID: "repo1", Kind: domain.KindRepo, Name: "flow", ParentID: &eng1ID},
		MoveTargets: []domain.Node{eng2},
	}
	var buf2 bytes.Buffer
	if err := nodeMoveForm(d2).Render(context.Background(), &buf2); err != nil {
		t.Fatalf("render visible form: %v", err)
	}
	body := buf2.String()
	if !strings.Contains(body, "<form") {
		t.Errorf("form must render when targets are present, got: %s", body)
	}
	if !strings.Contains(body, "RTL") {
		t.Errorf("form must include target name 'RTL', got: %s", body)
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
