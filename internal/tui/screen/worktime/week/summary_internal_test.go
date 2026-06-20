// Tests in package week (not week_test) so they can access unexported helpers.
package week

import (
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// TestComputeWeekSummary exercises the aggregation for a mixed week:
// Mon hit, Tue today+partial, Wed/Fri future, Thu vacation, Sat/Sun weekend.
func TestComputeWeekSummary(t *testing.T) {
	days := []apiclient.WeekDay{
		{Date: "2026-06-15", TargetMin: 480, LoggedMin: 480},                // Mon hit (past)
		{Date: "2026-06-16", TargetMin: 480, LoggedMin: 120, IsToday: true}, // Tue today, not hit
		{Date: "2026-06-17", TargetMin: 480, LoggedMin: 0},                  // Wed future
		{Date: "2026-06-18", TargetMin: 0, LoggedMin: 0},                    // Thu vacation (netted target 0)
		{Date: "2026-06-19", TargetMin: 480, LoggedMin: 0},                  // Fri future
		{Date: "2026-06-20", TargetMin: 0, LoggedMin: 0},                    // Sat weekend
		{Date: "2026-06-21", TargetMin: 0, LoggedMin: 0},                    // Sun weekend
	}
	offs := map[string]apiclient.DayOff{
		"2026-06-18": {Day: "2026-06-18", Kind: "vacation", Label: "Urlaub"},
	}
	s := computeWeekSummary(days, offs)
	if s.workdays != 4 { // Mon, Tue, Wed, Fri (Thu=vacation, Sat/Sun=weekend)
		t.Fatalf("workdays=%d, want 4", s.workdays)
	}
	if s.hits != 1 { // only Mon
		t.Fatalf("hits=%d, want 1", s.hits)
	}
	if s.expected != 1 { // Mon is past; Tue today not hit → not expected; Wed/Fri future
		t.Fatalf("expected=%d, want 1", s.expected)
	}
	if s.totalLogged != 600 || s.totalTarget != 1920 { // logged 480+120; target 480*4
		t.Fatalf("totals logged=%d target=%d, want 600/1920", s.totalLogged, s.totalTarget)
	}
}

// TestIsWeekendDate verifies the weekend-detection helper.
func TestIsWeekendDate(t *testing.T) {
	if !isWeekendDate("2026-06-20") { // Saturday
		t.Fatal("2026-06-20 is a Saturday")
	}
	if isWeekendDate("2026-06-15") { // Monday
		t.Fatal("2026-06-15 is a Monday, not weekend")
	}
}

// TestPaceColor covers all 7 paceColor branches to close the Task-4 coverage gap.
func TestPaceColor(t *testing.T) {
	pal := theme.Default
	sem := pal.Sem()

	tests := []struct {
		name string
		day  apiclient.WeekDay
		off  *apiclient.DayOff
		want theme.Color
	}{
		{
			name: "Hit→Success",
			day:  apiclient.WeekDay{TargetMin: 480, LoggedMin: 480},
			want: sem.Success,
		},
		{
			name: "Running→Active",
			day:  apiclient.WeekDay{TargetMin: 480, LoggedMin: 120, IsToday: true},
			want: sem.Active,
		},
		{
			name: "Missed→Border",
			day:  apiclient.WeekDay{TargetMin: 480, LoggedMin: 0},
			want: sem.Border,
		},
		{
			name: "holiday→Schedule",
			day:  apiclient.WeekDay{},
			off:  &apiclient.DayOff{Kind: "holiday"},
			want: sem.Schedule,
		},
		{
			name: "vacation→Highlight",
			day:  apiclient.WeekDay{},
			off:  &apiclient.DayOff{Kind: "vacation"},
			want: sem.Highlight,
		},
		{
			name: "sick→Notice",
			day:  apiclient.WeekDay{},
			off:  &apiclient.DayOff{Kind: "sick"},
			want: sem.Notice,
		},
		{
			name: "unknown-kind→FgMuted",
			day:  apiclient.WeekDay{},
			off:  &apiclient.DayOff{Kind: "other"},
			want: pal.FgMuted,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k := classifyPaceDot(tc.day, tc.off)
			got := paceColor(k, tc.off, pal)
			if got != tc.want {
				t.Fatalf("paceColor(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}
