package usecase_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func mkComposer(now time.Time, sessions []domain.Session, opts ...readerOpt) *usecase.StatusComposer {
	reader := mkReader(now, sessions, opts...)
	stats := &usecase.StatsComputer{
		Reader:  reader,
		Targets: reader.Targets,
		DayOffs: reader.Targets.DayOffs,
		State:   reader.State,
	}
	return &usecase.StatusComposer{
		Reader:       reader,
		DayOffs:      reader.Targets.DayOffs,
		Targets:      reader.Targets,
		Stats:        stats,
		Tmux:         &testutil.FakeTmux{},
		Clock:        &testutil.FixedClock{T: now},
		MaxStreakMin: 90,
	}
}

func TestStatusComposer_EmptyDayReturnsEmpty(t *testing.T) {
	now := time.Date(2026, 4, 29, 14, 30, 0, 0, time.Local)
	c := mkComposer(now, nil)
	if got := c.Compose(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestStatusComposer_RenderingIncludesBanner(t *testing.T) {
	now := time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local)
	c := mkComposer(now, []domain.Session{
		sessAt("2026-04-29", 9, 0, 8*time.Hour),
	})
	got := c.Compose()
	if !strings.Contains(got, "⏸ 08:00") {
		t.Errorf("expected ⏸ 08:00 banner, got %q", got)
	}
	if !strings.Contains(got, "✓") {
		t.Errorf("expected ✓ on hit day, got %q", got)
	}
}

func TestStatusComposer_TodayLoadErrorReturnsEmpty(t *testing.T) {
	now := time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local)
	c := mkComposer(now, nil)
	c.Reader.Sessions = &testutil.FakeSessionStore{Err: errors.New("boom")}
	if got := c.Compose(); got != "" {
		t.Errorf("Today error should yield empty, got %q", got)
	}
}

func TestStatusComposer_PaletteOverrideViaTmuxOptions(t *testing.T) {
	now := time.Date(2026, 4, 29, 18, 0, 0, 0, time.Local)
	c := mkComposer(now, []domain.Session{
		sessAt("2026-04-29", 9, 0, 8*time.Hour),
	})
	c.Tmux = &testutil.FakeTmux{Options: map[string]string{"tn_green": "#00ff00"}}
	got := c.Compose()
	if !strings.Contains(got, "#00ff00") {
		t.Errorf("expected tmux-overridden green in output, got %q", got)
	}
}

func TestStatusComposer_DayOffPicksUpFromStore(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	c := mkComposer(now, nil)
	dayOff := domain.DayOff{Date: now, Kind: domain.KindHoliday, Label: "Tag der Arbeit"}
	if err := c.DayOffs.Add(dayOff); err != nil {
		t.Fatal(err)
	}
	got := c.Compose()
	if !strings.Contains(got, "Tag der Arbeit") {
		t.Errorf("expected dayoff banner, got %q", got)
	}
}
