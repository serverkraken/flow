package worktime_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/worktime"
)

func TestAddManual_SplitsAcrossMidnight(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Span 2026-04-28 22:30 -> 2026-04-29 01:15 (2h 45m total).
	loc := time.Local
	day := time.Date(2026, 4, 28, 0, 0, 0, 0, loc)
	start := time.Date(2026, 4, 28, 22, 30, 0, 0, loc)
	stop := time.Date(2026, 4, 29, 1, 15, 0, 0, loc)
	if err := worktime.AddManual(day, start, stop); err != nil {
		t.Fatal(err)
	}

	hist, err := worktime.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 {
		t.Fatalf("expected 2 day rows after midnight split, got %d (%v)", len(hist), hist)
	}

	var d28, d29 worktime.DayRecord
	for _, r := range hist {
		switch r.Date.Day() {
		case 28:
			d28 = r
		case 29:
			d29 = r
		}
	}
	if d28.Total != 90*time.Minute {
		t.Errorf("28th total = %v, want 1h 30m", d28.Total)
	}
	if d29.Total != 75*time.Minute {
		t.Errorf("29th total = %v, want 1h 15m", d29.Total)
	}
}

func TestStart_Concurrent_NoCorruption(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Hammer Start from N goroutines. The flock should serialize writes; the
	// resulting state file must be a valid epoch (one of the timestamps).
	const N = 16
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			ts := time.Now().Add(time.Duration(i) * time.Second)
			_ = worktime.Start(ts)
		}()
	}
	wg.Wait()

	// Subsequent LoadToday should find an active session without errors.
	day, err := worktime.LoadToday()
	if err != nil {
		t.Fatal(err)
	}
	if !day.IsRunning() {
		t.Fatal("expected an active session after concurrent Start writes")
	}
}

func TestSession_TagAndNote_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	day := time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local)
	start := time.Date(2026, 4, 28, 9, 0, 0, 0, time.Local)
	stop := time.Date(2026, 4, 28, 12, 0, 0, 0, time.Local)
	if err := worktime.AddManual(day, start, stop); err != nil {
		t.Fatal(err)
	}
	if err := worktime.SetTag(day, 0, "deep"); err != nil {
		t.Fatal(err)
	}
	if err := worktime.SetNote(day, 0, "early sprint"); err != nil {
		t.Fatal(err)
	}
	hist, _ := worktime.LoadHistory()
	if len(hist) != 1 || len(hist[0].Sessions) != 1 {
		t.Fatalf("unexpected history shape: %+v", hist)
	}
	s := hist[0].Sessions[0]
	if s.Tag != "deep" {
		t.Errorf("Tag = %q, want deep", s.Tag)
	}
	if s.Note != "early sprint" {
		t.Errorf("Note = %q, want 'early sprint'", s.Note)
	}

	// Clear tag + note.
	if err := worktime.SetTag(day, 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := worktime.SetNote(day, 0, ""); err != nil {
		t.Fatal(err)
	}
	hist, _ = worktime.LoadHistory()
	s = hist[0].Sessions[0]
	if s.Tag != "" || s.Note != "" {
		t.Errorf("expected cleared tag/note, got %q / %q", s.Tag, s.Note)
	}
}

func TestSetTag_StripsTabAndNewline(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	day := time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local)
	start := time.Date(2026, 4, 28, 9, 0, 0, 0, time.Local)
	stop := time.Date(2026, 4, 28, 12, 0, 0, 0, time.Local)
	if err := worktime.AddManual(day, start, stop); err != nil {
		t.Fatal(err)
	}
	if err := worktime.SetTag(day, 0, "wi\tth\ttabs"); err != nil {
		t.Fatal(err)
	}
	hist, _ := worktime.LoadHistory()
	if strings.ContainsAny(hist[0].Sessions[0].Tag, "\t\n") {
		t.Errorf("tag still contains forbidden chars: %q", hist[0].Sessions[0].Tag)
	}
}
