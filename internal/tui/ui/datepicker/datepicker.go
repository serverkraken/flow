// Package datepicker is a clock-free segment-stepper date picker: ‹YYYY›-‹MM›-‹DD›
// with a weekday label and an optional read-only month grid (Calendar). It never
// reads the host time; "today" for the grid is passed in. Drop it into a dialog
// row like a textinput; the embedding route routes keys to Update.
package datepicker

import (
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/tui/theme"
)

// Model holds the selected date and which segment is active. Zero value is not
// valid; use New.
type Model struct {
	y, mo, d int
	seg      int // 0=year, 1=month, 2=day
	typed    int // digits entered into the active segment since it became active
	focused  bool
	pal      theme.Palette
}

// New builds a picker initialised to initial's date.
func New(initial time.Time, pal theme.Palette) Model {
	return Model{y: initial.Year(), mo: int(initial.Month()), d: initial.Day(), pal: pal}
}

// Value returns the selected date as "YYYY-MM-DD".
func (m Model) Value() string { return fmt.Sprintf("%04d-%02d-%02d", m.y, m.mo, m.d) }

// SetValue parses "YYYY-MM-DD" and replaces the selection; bad input is rejected.
func (m *Model) SetValue(s string) error {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return err
	}
	m.y, m.mo, m.d, m.typed = t.Year(), int(t.Month()), t.Day(), 0
	return nil
}

func (m *Model) Focus()       { m.focused = true }
func (m *Model) Blur()        { m.focused = false; m.typed = 0 }
func (m Model) Focused() bool { return m.focused }

// weekdayShort renders a Go weekday as the German two-letter abbreviation.
func weekdayShort(w time.Weekday) string {
	return [...]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}[int(w)]
}

// daysIn returns the number of days in month mo of year y.
// Used by Update (Task 6) to clamp the day segment after month/year changes.
//
//nolint:unused
func daysIn(y, mo int) int {
	return time.Date(y, time.Month(mo)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// View renders the one-line stepper "‹2026›-‹07›-‹20›  (Mo)"; the active segment
// is accent-highlighted only while focused.
func (m Model) View() string {
	segs := [3]string{fmt.Sprintf("%04d", m.y), fmt.Sprintf("%02d", m.mo), fmt.Sprintf("%02d", m.d)}
	for i := range segs {
		if m.focused && i == m.seg {
			segs[i] = theme.Active("‹"+segs[i]+"›", m.pal)
		} else {
			segs[i] = " " + segs[i] + " "
		}
	}
	wd := weekdayShort(time.Date(m.y, time.Month(m.mo), m.d, 0, 0, 0, 0, time.UTC).Weekday())
	return segs[0] + "-" + segs[1] + "-" + segs[2] + "  (" + wd + ")"
}
