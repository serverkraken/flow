package httpserver

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/usecase"
)

// TestHeuteTargetVariant covers all four branches of heuteTargetVariant:
// running, over (Saldo > 0), hit (Saldo==0 && Target>0), and under (default).
func TestHeuteTargetVariant(t *testing.T) {
	cases := []struct {
		name    string
		today   usecase.TodaySummary
		running bool
		want    string
	}{
		{
			name:    "running session",
			today:   usecase.TodaySummary{Saldo: 0, Target: 8 * time.Hour},
			running: true,
			want:    "running",
		},
		{
			name:    "over target",
			today:   usecase.TodaySummary{Saldo: 30 * time.Minute, Target: 8 * time.Hour},
			running: false,
			want:    "over",
		},
		{
			name:    "hit exactly",
			today:   usecase.TodaySummary{Saldo: 0, Target: 8 * time.Hour},
			running: false,
			want:    "hit",
		},
		{
			name:    "under target",
			today:   usecase.TodaySummary{Saldo: -30 * time.Minute, Target: 8 * time.Hour},
			running: false,
			want:    "under",
		},
	}
	for _, tc := range cases {
		got := heuteTargetVariant(tc.today, tc.running)
		if got != tc.want {
			t.Errorf("%s: heuteTargetVariant = %q, want %q", tc.name, got, tc.want)
		}
	}
}
