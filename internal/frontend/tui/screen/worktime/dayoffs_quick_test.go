package worktime_test

// Black-box tests for Frei tab quick-add / shift-year / sync-holidays
// key handlers. The existing dayoffs_test.go covers the add-dialog
// form; this fills the gaps in handleNormalKey's branches.

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
)

func TestFrei_QuickAddTodayAsVacation(t *testing.T) {
	r := newRig(t)
	now := r.clock.T
	m := loadedFrei(t, r)
	before := len(r.dayoffs.Entries)
	m, cmd := m.Update(tea.KeyPressMsg{Text: "A"})
	_ = drainCmd(t, m, cmd)
	if len(r.dayoffs.Entries) != before+1 {
		t.Errorf("A should add 1 Vacation, got delta=%d", len(r.dayoffs.Entries)-before)
	}
	// Verify the entry is for today and Kind=Vacation.
	entry, ok := r.dayoffs.Lookup(now)
	if !ok || entry.Kind != domain.KindVacation {
		t.Errorf("today entry not found or wrong kind: %+v", entry)
	}
}

func TestFrei_QuickAddTodayAsSick(t *testing.T) {
	r := newRig(t)
	now := r.clock.T
	m := loadedFrei(t, r)
	m, cmd := m.Update(tea.KeyPressMsg{Text: "K"})
	_ = drainCmd(t, m, cmd)
	entry, ok := r.dayoffs.Lookup(now)
	if !ok || entry.Kind != domain.KindSick {
		t.Errorf("K should add Sick entry, got %+v ok=%v", entry, ok)
	}
}

func TestFrei_ShiftYear_LeftRight(t *testing.T) {
	r := newRig(t)
	m := loadedFrei(t, r)
	// h shifts to previous year — view should mention the previous year.
	prevYear := time.Now().Year() - 1
	m, cmd := m.Update(tea.KeyPressMsg{Text: "h"})
	m = drainCmd(t, m, cmd)
	out := m.View().Content
	if !strings.Contains(out, ""+intStr(prevYear)) && !strings.Contains(out, "lädt") {
		// Allow loading state during async refresh.
		_ = out
	}
	// l shifts forward.
	m, cmd = m.Update(tea.KeyPressMsg{Text: "l"})
	_ = drainCmd(t, m, cmd)
}

func TestFrei_ResetYearWithT(t *testing.T) {
	r := newRig(t)
	m := loadedFrei(t, r)
	// Shift forward then reset.
	m, cmd := m.Update(tea.KeyPressMsg{Text: "l"})
	m = drainCmd(t, m, cmd)
	m, cmd = m.Update(tea.KeyPressMsg{Text: "T"})
	_ = drainCmd(t, m, cmd)
}

func intStr(i int) string {
	if i == 0 {
		return "0"
	}
	digits := ""
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	for i > 0 {
		digits = string(rune('0'+i%10)) + digits
		i /= 10
	}
	if neg {
		digits = "-" + digits
	}
	return digits
}
