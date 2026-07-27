package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestSplitDaily(t *testing.T) {
	loc := time.UTC
	mk := func(y, m, d, hh, mm int) time.Time {
		return time.Date(y, time.Month(m), d, hh, mm, 0, 0, loc)
	}

	t.Run("same day → single range", func(t *testing.T) {
		got := domain.SplitDaily(mk(2026, 6, 24, 9, 0), mk(2026, 6, 24, 17, 0), loc)
		if len(got) != 1 || !got[0].Start.Equal(mk(2026, 6, 24, 9, 0)) || !got[0].Stop.Equal(mk(2026, 6, 24, 17, 0)) {
			t.Fatalf("same-day = %+v, want one unchanged range", got)
		}
	})

	t.Run("crosses one midnight → two ranges split at 00:00", func(t *testing.T) {
		got := domain.SplitDaily(mk(2026, 6, 23, 18, 51), mk(2026, 6, 24, 14, 0), loc)
		if len(got) != 2 {
			t.Fatalf("got %d ranges, want 2: %+v", len(got), got)
		}
		mid := mk(2026, 6, 24, 0, 0)
		if !got[0].Start.Equal(mk(2026, 6, 23, 18, 51)) || !got[0].Stop.Equal(mid) {
			t.Errorf("chunk 0 = %v..%v, want 18:51..midnight", got[0].Start, got[0].Stop)
		}
		if !got[1].Start.Equal(mid) || !got[1].Stop.Equal(mk(2026, 6, 24, 14, 0)) {
			t.Errorf("chunk 1 = %v..%v, want midnight..14:00", got[1].Start, got[1].Stop)
		}
	})

	t.Run("crosses two midnights → three ranges", func(t *testing.T) {
		got := domain.SplitDaily(mk(2026, 6, 23, 22, 0), mk(2026, 6, 25, 3, 0), loc)
		if len(got) != 3 {
			t.Fatalf("got %d ranges, want 3: %+v", len(got), got)
		}
		// contiguous: each chunk's stop is the next chunk's start
		for i := 1; i < len(got); i++ {
			if !got[i].Start.Equal(got[i-1].Stop) {
				t.Errorf("chunk %d not contiguous: %v != %v", i, got[i].Start, got[i-1].Stop)
			}
		}
		if !got[0].Start.Equal(mk(2026, 6, 23, 22, 0)) || !got[2].Stop.Equal(mk(2026, 6, 25, 3, 0)) {
			t.Errorf("endpoints wrong: %+v", got)
		}
	})

	t.Run("stop not after start → single range (no panic)", func(t *testing.T) {
		s := mk(2026, 6, 24, 9, 0)
		got := domain.SplitDaily(s, s, loc)
		if len(got) != 1 {
			t.Fatalf("degenerate = %+v, want 1 range", got)
		}
	})
}
