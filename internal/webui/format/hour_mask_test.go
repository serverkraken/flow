package format_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/webui/format"
)

func TestHourMask_MarksHoursTouched(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 4, 14, 30, 0, 0, time.UTC)
	sess := []domain.Session{
		{
			Start: time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC),
			Stop:  time.Date(2026, 6, 4, 10, 30, 0, 0, time.UTC),
		},
		{
			Start: time.Date(2026, 6, 4, 13, 0, 0, 0, time.UTC),
			Stop:  time.Date(2026, 6, 4, 13, 45, 0, 0, time.UTC),
		},
	}
	mask := format.HourMask(sess, nil, now)

	worked := []int{9, 10, 13}
	for _, h := range worked {
		if mask[h] != 1 {
			t.Errorf("hour %d: got %d, want 1", h, mask[h])
		}
	}
	if mask[11] != 0 || mask[12] != 0 {
		t.Errorf("non-worked hours got marked: 11=%d 12=%d", mask[11], mask[12])
	}
	// Hour 14 has no completed session, no active → must remain 0.
	if mask[14] != 0 {
		t.Errorf("hour 14 incorrectly marked")
	}
}

func TestHourMask_ActiveExtendsMaskToNow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 4, 14, 30, 0, 0, time.UTC)
	active := time.Date(2026, 6, 4, 13, 0, 0, 0, time.UTC)
	mask := format.HourMask(nil, &active, now)

	for _, h := range []int{13, 14} {
		if mask[h] != 1 {
			t.Errorf("active hour %d: got %d, want 1", h, mask[h])
		}
	}
	if mask[12] != 0 || mask[15] != 0 {
		t.Errorf("non-active hours got marked: 12=%d 15=%d", mask[12], mask[15])
	}
}
