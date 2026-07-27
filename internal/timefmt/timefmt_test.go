package timefmt_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/timefmt"
)

func TestFormatMin(t *testing.T) {
	cases := map[int]string{0: "0h 00m", 5: "0h 05m", 65: "1h 05m", 600: "10h 00m", -30: "0h 00m"}
	for in, want := range cases {
		if got := timefmt.FormatMin(in); got != want {
			t.Errorf("FormatMin(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatSaldo(t *testing.T) {
	cases := map[int]string{0: "+0h 00m", 65: "+1h 05m", -65: "-1h 05m"}
	for in, want := range cases {
		if got := timefmt.FormatSaldo(in); got != want {
			t.Errorf("FormatSaldo(%d) = %q, want %q", in, got, want)
		}
	}
}
