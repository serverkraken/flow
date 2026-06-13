package httpapi

import (
	"testing"
	"time"
)

// TestBackoff_ZeroValueDefaults verifies that all fields fall back to the
// documented defaults when left zero.
func TestBackoff_ZeroValueDefaults(t *testing.T) {
	t.Parallel()
	b := Backoff{}
	base, max, factor, jitter := b.defaults()
	if base != 500*time.Millisecond {
		t.Errorf("base: got %v, want 500ms", base)
	}
	if max != 60*time.Second {
		t.Errorf("max: got %v, want 60s", max)
	}
	if factor != 2.0 {
		t.Errorf("factor: got %v, want 2.0", factor)
	}
	if jitter != 0.2 {
		t.Errorf("jitter: got %v, want 0.2", jitter)
	}
}

// TestBackoff_NoJitter_ExactPow verifies that with Jitter: 0 the delay is
// exactly Base * Factor^attempt (no random component).
func TestBackoff_NoJitter_ExactPow(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		b       Backoff
		attempt int
		want    time.Duration
	}{
		{"attempt-0", Backoff{Base: time.Second, Max: time.Hour, Factor: 2.0, Jitter: -1}, 0, time.Second},
		{"attempt-1", Backoff{Base: time.Second, Max: time.Hour, Factor: 2.0, Jitter: -1}, 1, 2 * time.Second},
		{"attempt-2", Backoff{Base: time.Second, Max: time.Hour, Factor: 2.0, Jitter: -1}, 2, 4 * time.Second},
		{"attempt-3", Backoff{Base: time.Second, Max: time.Hour, Factor: 2.0, Jitter: -1}, 3, 8 * time.Second},
		{"attempt-4", Backoff{Base: time.Second, Max: time.Hour, Factor: 2.0, Jitter: -1}, 4, 16 * time.Second},
		// Negative jitter is treated as "no jitter" by the > 0 check in For.
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.b.For(tc.attempt)
			if got != tc.want {
				t.Errorf("For(%d) = %v, want %v", tc.attempt, got, tc.want)
			}
		})
	}
}

// TestBackoff_CapsAtMax verifies that runaway attempts are clamped to Max.
func TestBackoff_CapsAtMax(t *testing.T) {
	t.Parallel()
	b := Backoff{
		Base:   time.Millisecond,
		Max:    100 * time.Millisecond,
		Factor: 2.0,
		Jitter: -1, // disable jitter
	}
	// attempt 10 → 2^10 ms = 1024ms, clamped to 100ms.
	got := b.For(10)
	if got != 100*time.Millisecond {
		t.Errorf("For(10) = %v, want 100ms (capped)", got)
	}
	// attempt 100 → would overflow float without cap; still capped.
	got = b.For(100)
	if got != 100*time.Millisecond {
		t.Errorf("For(100) = %v, want 100ms (capped)", got)
	}
}

// TestBackoff_MonotonicallyIncreasing verifies that with jitter enabled
// the *median* delay still grows monotonically across attempts (we sample
// repeatedly per attempt to average out the jitter band).
func TestBackoff_MonotonicallyIncreasing(t *testing.T) {
	t.Parallel()
	b := Backoff{
		Base:   10 * time.Millisecond,
		Max:    10 * time.Second, // well above the attempts we test
		Factor: 2.0,
		Jitter: 0.2,
	}
	const samples = 50
	mean := func(attempt int) float64 {
		var sum time.Duration
		for i := 0; i < samples; i++ {
			sum += b.For(attempt)
		}
		return float64(sum) / float64(samples)
	}
	prev := mean(0)
	for i := 1; i < 6; i++ {
		curr := mean(i)
		if curr <= prev {
			t.Errorf("mean For(%d) = %v not > For(%d) = %v", i, curr, i-1, prev)
		}
		prev = curr
	}
}

// TestBackoff_JitterWithinBand verifies that a single call lands inside the
// expected ±Jitter band around the pow result.
func TestBackoff_JitterWithinBand(t *testing.T) {
	t.Parallel()
	b := Backoff{
		Base:   100 * time.Millisecond,
		Max:    10 * time.Second,
		Factor: 2.0,
		Jitter: 0.2,
	}
	// attempt 0 → expected centre = 100ms, band = [80ms, 120ms].
	lo := 80 * time.Millisecond
	hi := 120 * time.Millisecond
	for i := 0; i < 100; i++ {
		d := b.For(0)
		if d < lo || d > hi {
			t.Errorf("For(0) = %v, outside band [%v, %v]", d, lo, hi)
		}
	}
}

// TestBackoff_NegativeAttempt verifies that a negative attempt index (which
// would normally make the math undefined) is clamped to attempt 0.
func TestBackoff_NegativeAttempt(t *testing.T) {
	t.Parallel()
	b := Backoff{
		Base:   100 * time.Millisecond,
		Max:    10 * time.Second,
		Factor: 2.0,
		Jitter: -1, // no jitter
	}
	got := b.For(-1)
	if got != 100*time.Millisecond {
		t.Errorf("For(-1) = %v, want 100ms (clamped to attempt 0)", got)
	}
}
