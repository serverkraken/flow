package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestWriteStats_FullBody(t *testing.T) {
	t.Parallel()
	st := domain.Stats{
		Days:       5,
		Workdays:   4,
		Total:      20 * time.Hour,
		Avg:        5 * time.Hour,
		Max:        8 * time.Hour,
		MaxDate:    time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		Min:        2 * time.Hour,
		MinDate:    time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC),
		Hits:       3,
		Streak:     2,
		BestStreak: 5,
		Overtime:   -1 * time.Hour,
	}
	var b strings.Builder
	if err := domain.WriteStats(&b, "week", st); err != nil {
		t.Fatalf("WriteStats: %v", err)
	}
	got := b.String()
	for _, want := range []string{"Range:", "week", "Tage:", "Werktage:", "Total:", "Max:", "Min:", "Ziele:", "Streak:", "Saldo:"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q, got:\n%s", want, got)
		}
	}
}

func TestWriteStats_SkipsZeroMaxMin(t *testing.T) {
	t.Parallel()
	st := domain.Stats{
		Days:     1,
		Workdays: 1,
		Total:    time.Hour,
		Avg:      time.Hour,
		// MaxDate/MinDate zero → those rows are skipped.
	}
	var b strings.Builder
	if err := domain.WriteStats(&b, "today", st); err != nil {
		t.Fatalf("WriteStats: %v", err)
	}
	got := b.String()
	if strings.Contains(got, "Max:") || strings.Contains(got, "Min:") {
		t.Errorf("zero MaxDate/MinDate should suppress Max/Min rows, got:\n%s", got)
	}
}

func TestWriteStats_TagsSection(t *testing.T) {
	t.Parallel()
	st := domain.Stats{
		Days:     2,
		Workdays: 2,
		Total:    4 * time.Hour,
		Avg:      2 * time.Hour,
		ByTag: map[string]time.Duration{
			"deep": 3 * time.Hour,
			"ops":  time.Hour,
		},
		CountByTag: map[string]int{"deep": 2, "ops": 1},
	}
	var b strings.Builder
	if err := domain.WriteStats(&b, "week", st); err != nil {
		t.Fatalf("WriteStats: %v", err)
	}
	got := b.String()
	if !strings.Contains(got, "Tags:") {
		t.Errorf("Tags section should appear when Tags populated, got:\n%s", got)
	}
	if !strings.Contains(got, "deep") {
		t.Errorf("tag deep should appear, got:\n%s", got)
	}
}

func TestWriteStats_DaysOffSection(t *testing.T) {
	t.Parallel()
	st := domain.Stats{
		DaysOff: []domain.DayOff{
			{Kind: domain.KindVacation},
			{Kind: domain.KindVacation},
			{Kind: domain.KindSick},
		},
	}
	var b strings.Builder
	if err := domain.WriteStats(&b, "month", st); err != nil {
		t.Fatalf("WriteStats: %v", err)
	}
	got := b.String()
	if !strings.Contains(got, "Frei:") {
		t.Errorf("Frei section should appear when DaysOff populated, got:\n%s", got)
	}
}

func TestWriteStats_EmptyTagsAndDaysOff_NoExtraSections(t *testing.T) {
	t.Parallel()
	st := domain.Stats{Days: 1}
	var b strings.Builder
	if err := domain.WriteStats(&b, "today", st); err != nil {
		t.Fatalf("WriteStats: %v", err)
	}
	got := b.String()
	if strings.Contains(got, "Tags:") || strings.Contains(got, "Frei:") {
		t.Errorf("empty Tags/DaysOff should suppress sections, got:\n%s", got)
	}
}
