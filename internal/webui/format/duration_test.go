package format_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/webui/format"
)

func TestFormatHHMM(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0:00"},
		{30 * time.Minute, "0:30"},
		{8 * time.Hour, "8:00"},
		{8*time.Hour + 14*time.Minute, "8:14"},
		{-time.Hour, "0:00"}, // clamped
	}
	for _, c := range cases {
		if got := format.FormatHHMM(c.in); got != c.want {
			t.Errorf("FormatHHMM(%v): got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatSignedHHMM(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0:00"},
		{30 * time.Minute, "+0:30"},
		{-30 * time.Minute, "-0:30"},
		{12*time.Hour + 42*time.Minute, "+12:42"},
		{-(8*time.Hour + 0*time.Minute), "-8:00"},
	}
	for _, c := range cases {
		if got := format.FormatSignedHHMM(c.in); got != c.want {
			t.Errorf("FormatSignedHHMM(%v): got %q, want %q", c.in, got, c.want)
		}
	}
}
