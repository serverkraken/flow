package httpserver

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// TestGridWindow verifies the hybrid time-window math: a default 06:00–20:00
// band that expands (snapped to the hour) to fit out-of-band sessions and
// clamps into [0,1440].
func TestGridWindow(t *testing.T) {
	if f, c := gridWindow(nil); f != 360 || c != 1200 {
		t.Fatalf("default = %d,%d want 360,1200", f, c)
	}
	if f, c := gridWindow([]int{310, 1260}); f != 300 || c != 1260 {
		t.Fatalf("expand = %d,%d want 300,1260", f, c)
	}
	// In-band sessions must not shrink the default band.
	if f, c := gridWindow([]int{400, 1100}); f != 360 || c != 1200 {
		t.Fatalf("in-band = %d,%d want 360,1200", f, c)
	}
	// Out-of-band low/high snap to the hour and clamp at the edges.
	if f, c := gridWindow([]int{5, 1438}); f != 0 || c != 1440 {
		t.Fatalf("clamp = %d,%d want 0,1440", f, c)
	}
}

// TestHistorieBulkErr covers all three branches of historieBulkErr:
// ErrNoSessions → German string, ErrProjectNotFound → German string,
// unknown error → fallback with error message.
func TestHistorieBulkErr(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{usecase.ErrNoSessions, "keine Sitzungen ausgewählt"},
		{ports.ErrProjectNotFound, "Projekt nicht gefunden"},
		{errors.New("disk full"), "Aktion fehlgeschlagen: disk full"},
	}
	for _, tc := range cases {
		got := historieBulkErr(tc.err)
		if !strings.Contains(got, tc.want) {
			t.Errorf("historieBulkErr(%v) = %q, want to contain %q", tc.err, got, tc.want)
		}
	}
}

// TestEffectiveStopMin is the #13 regression: a session ending at/after the
// start day's midnight (e.g. a split chunk 18:51→00:00 next day) must fill to
// 24:00 (1440) on its start-day column, not collapse to minute 0.
func TestEffectiveStopMin(t *testing.T) {
	loc := time.UTC
	start := time.Date(2026, 6, 23, 18, 51, 0, 0, loc)
	t.Run("same-day stop → its minute-of-day", func(t *testing.T) {
		stop := time.Date(2026, 6, 23, 20, 0, 0, 0, loc)
		if got := effectiveStopMin(start, stop, loc); got != 20*60 {
			t.Fatalf("got %d, want %d", got, 20*60)
		}
	})
	t.Run("midnight next day → 1440 (fills to end of start day)", func(t *testing.T) {
		stop := time.Date(2026, 6, 24, 0, 0, 0, 0, loc)
		if got := effectiveStopMin(start, stop, loc); got != 24*60 {
			t.Fatalf("got %d, want 1440", got)
		}
	})
	t.Run("later day afternoon → 1440", func(t *testing.T) {
		stop := time.Date(2026, 6, 24, 14, 0, 0, 0, loc)
		if got := effectiveStopMin(start, stop, loc); got != 24*60 {
			t.Fatalf("got %d, want 1440", got)
		}
	})
}
