package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestFmtDateDe_ShortReturnsAbbreviatedMonth(t *testing.T) {
	d := time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC) // Mittwoch
	if got := domain.FmtDateDe(d, domain.DateShort); got != "Mi., 27. Mai" {
		t.Errorf("FmtDateDe(Short) = %q, want %q", got, "Mi., 27. Mai")
	}
}

func TestFmtDateDe_LongIncludesYear(t *testing.T) {
	d := time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC) // Mi.
	if got := domain.FmtDateDe(d, domain.DateLong); got != "Mi., 27. Mai 2026" {
		t.Errorf("FmtDateDe(Long) = %q, want %q", got, "Mi., 27. Mai 2026")
	}
}

func TestFmtDateDe_NumericIsSortable(t *testing.T) {
	d := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	if got := domain.FmtDateDe(d, domain.DateNumeric); got != "2026-05-28" {
		t.Errorf("FmtDateDe(Numeric) = %q, want %q", got, "2026-05-28")
	}
}

func TestFmtDateRangeDe_SameMonth(t *testing.T) {
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	if got := domain.FmtDateRangeDe(from, to); got != "1.–7. Mai" {
		t.Errorf("FmtDateRangeDe(same month) = %q, want %q", got, "1.–7. Mai")
	}
}

func TestFmtDateRangeDe_DifferentMonths(t *testing.T) {
	from := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	if got := domain.FmtDateRangeDe(from, to); got != "28. Mai – 3. Jun" {
		t.Errorf("FmtDateRangeDe(different months) = %q, want %q", got, "28. Mai – 3. Jun")
	}
}
