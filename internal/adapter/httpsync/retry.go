package httpsync

import (
	"math"
	"math/rand/v2"
	"time"
)

// Backoff computes per-attempt retry delays with exponential growth + jitter.
//
// Zero value yields sensible defaults: Base 500ms, Max 60s, Factor 2.0,
// Jitter 0.2 (= ±20% spread). Callers wire a non-zero Backoff on the
// httpsync.Worker to override; tests use a tight {Base: 1ms, Factor: 2} so
// retries land in milliseconds instead of seconds.
//
// # Determinism in tests
//
// Backoff reads from the default math/rand source. Tests that need fully
// deterministic delays set Jitter: 0 to skip the random multiplier; tests
// that care only about ordering (delay grows monotonically until capped at
// Max) tolerate the default jitter band.
type Backoff struct {
	Base   time.Duration // 0 → 500ms
	Max    time.Duration // 0 → 60s
	Factor float64       // 0 → 2.0
	Jitter float64       // 0 → 0.2
}

// For returns the delay before retry attempt `attempt` (0-indexed: attempt 0
// is the very first retry). The delay grows as Base * Factor^attempt, clamped
// to Max, with ±Jitter spread applied before the clamp.
func (b Backoff) For(attempt int) time.Duration {
	base, max, factor, jitter := b.defaults()
	if attempt < 0 {
		attempt = 0
	}
	d := float64(base) * math.Pow(factor, float64(attempt))
	if jitter > 0 {
		d *= 1 + (rand.Float64()*2-1)*jitter //nolint:gosec // jitter spread is not security-sensitive
	}
	if d > float64(max) {
		return max
	}
	if d < float64(base) {
		// Negative jitter pushed the value below base, or attempt math
		// underflowed. Either way: return base as the floor so callers
		// always wait at least one base interval.
		return base
	}
	return time.Duration(d)
}

// defaults resolves zero-valued fields to the documented defaults.
func (b Backoff) defaults() (time.Duration, time.Duration, float64, float64) {
	base := b.Base
	if base == 0 {
		base = 500 * time.Millisecond
	}
	max := b.Max
	if max == 0 {
		max = 60 * time.Second
	}
	factor := b.Factor
	if factor == 0 {
		factor = 2.0
	}
	jitter := b.Jitter
	if jitter == 0 {
		jitter = 0.2
	}
	return base, max, factor, jitter
}
