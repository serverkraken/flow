package worker

import (
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {
	base, ceiling := time.Minute, 6*time.Hour
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 1 * time.Minute},
		{2, 2 * time.Minute},
		{3, 4 * time.Minute},
		{4, 8 * time.Minute},
		{100, ceiling}, // overflow clamps to ceiling
	}
	for _, c := range cases {
		if got := backoff(c.attempts, base, ceiling); got != c.want {
			t.Fatalf("backoff(%d) = %v, want %v", c.attempts, got, c.want)
		}
	}
}
