package httpapi

// copied from internal/adapter/httpsync/retry.go (R2a); source deleted in Task 12.

import (
	"math"
	"math/rand/v2"
	"time"
)

type Backoff struct {
	Base   time.Duration
	Max    time.Duration
	Factor float64
	Jitter float64
}

func (b Backoff) For(attempt int) time.Duration {
	base, max, factor, jitter := b.defaults()
	if attempt < 0 {
		attempt = 0
	}
	d := float64(base) * math.Pow(factor, float64(attempt))
	if jitter > 0 {
		d *= 1 + (rand.Float64()*2-1)*jitter //nolint:gosec
	}
	if d > float64(max) {
		return max
	}
	if d < float64(base) {
		return base
	}
	return time.Duration(d)
}

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
