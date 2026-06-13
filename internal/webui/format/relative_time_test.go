package format_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/webui/format"
)

func TestHumanRelativeTime(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{"8s ago", now.Add(-8 * time.Second), "vor 8s"},
		{"2m ago", now.Add(-2 * time.Minute), "vor 2m"},
		{"today 09:28", time.Date(2026, 6, 4, 9, 28, 0, 0, time.UTC), "heute · 09:28"},
		{"yesterday", time.Date(2026, 6, 3, 17, 45, 0, 0, time.UTC), "gestern · 17:45"},
		{"2 days ago", time.Date(2026, 6, 2, 14, 12, 0, 0, time.UTC), "vor 2 Tagen · 14:12"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := format.HumanRelativeTime(c.in, now); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
