package worker

import "time"

// backoff returns the wait before the next retry of a document that has failed
// `attempts` times (attempts >= 1): exponential from base, clamped to ceiling.
// Pure; ceiling also guards against left-shift overflow.
func backoff(attempts int, base, ceiling time.Duration) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	d := base << (attempts - 1)
	if d <= 0 || d > ceiling {
		d = ceiling
	}
	return d
}
