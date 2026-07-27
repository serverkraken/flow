// Package datepicker is a clock-free segment-stepper date picker: ‹YYYY›-‹MM›-‹DD›
// with a weekday label and an optional read-only month grid (Calendar). It never
// reads the host time; "today" for the grid is passed in. Drop it into a dialog
// row like a textinput; the embedding route routes keys to Update.
package datepicker

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
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
// Used by Update to clamp the day segment after month/year changes.
func daysIn(y, mo int) int {
	return time.Date(y, time.Month(mo)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// Update applies one key: ←/→ move the active segment, ↑/↓ step it, digits fill
// it. It never returns a command and never reads the clock.
func (m Model) Update(k tea.KeyPressMsg) Model {
	switch k.Code {
	case tea.KeyLeft:
		m.commit()
		if m.seg > 0 {
			m.seg--
		}
	case tea.KeyRight:
		m.commit()
		if m.seg < 2 {
			m.seg++
		}
	case tea.KeyUp:
		m.commit()
		m.step(+1)
	case tea.KeyDown:
		m.commit()
		m.step(-1)
	default:
		if len(k.Text) == 1 && k.Text[0] >= '0' && k.Text[0] <= '9' {
			m.typeDigit(int(k.Text[0] - '0'))
		}
	}
	return m
}

// commit clamps the current fields to a valid date and resets the typing buffer.
func (m *Model) commit() {
	if m.y < 1 {
		m.y = 1
	}
	if m.mo < 1 {
		m.mo = 1
	} else if m.mo > 12 {
		m.mo = 12
	}
	if m.d < 1 {
		m.d = 1
	}
	if n := daysIn(m.y, m.mo); m.d > n {
		m.d = n
	}
	m.typed = 0
}

// step increments (delta +1) or decrements the active segment with rollover for
// month and day; the day is clamped when year/month change.
func (m *Model) step(delta int) {
	switch m.seg {
	case 0:
		m.y += delta
		if m.y < 1 {
			m.y = 1
		}
		if n := daysIn(m.y, m.mo); m.d > n {
			m.d = n
		}
	case 1:
		m.mo += delta
		if m.mo > 12 {
			m.mo = 1
		} else if m.mo < 1 {
			m.mo = 12
		}
		if n := daysIn(m.y, m.mo); m.d > n {
			m.d = n
		}
	case 2:
		n := daysIn(m.y, m.mo)
		m.d += delta
		if m.d > n {
			m.d = 1
		} else if m.d < 1 {
			m.d = n
		}
	}
}

// advance commits the active segment and moves focus to the next one.
func (m *Model) advance() {
	m.commit()
	if m.seg < 2 {
		m.seg++
	}
}

// typeDigit accumulates a typed digit into the active segment, auto-advancing
// when the segment is full or cannot take another digit.
func (m *Model) typeDigit(dg int) {
	switch m.seg {
	case 0: // year: 4 digits
		if m.typed == 0 {
			m.y = dg
		} else {
			m.y = (m.y*10 + dg) % 10000
		}
		m.typed++
		if m.typed >= 4 {
			m.advance()
		}
	case 1: // month: 1-2 digits
		if m.typed == 0 {
			m.mo = dg
			m.typed = 1
			if dg >= 2 { // 2..9 can't start a valid 2-digit month
				m.advance()
			}
		} else {
			m.mo = m.mo*10 + dg
			m.advance()
		}
	case 2: // day: 1-2 digits
		if m.typed == 0 {
			m.d = dg
			m.typed = 1
			if dg >= 4 { // 4..9 can't start a valid 2-digit day
				m.advance()
			}
		} else {
			m.d = m.d*10 + dg
			m.advance()
		}
	}
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

// monthNames are the German month names indexed 1..12.
var monthNames = [...]string{"", "Januar", "Februar", "März", "April", "Mai", "Juni",
	"Juli", "August", "September", "Oktober", "November", "Dezember"}

// Calendar renders a read-only Monday-first month grid of the selected date's
// month. The selected day is accent-highlighted; today (when it falls in the
// shown month) is dimmed. Pass the zero time to omit the today highlight. It is
// pure rendering — it does not change the selection.
func (m Model) Calendar(today time.Time) string {
	first := time.Date(m.y, time.Month(m.mo), 1, 0, 0, 0, 0, time.UTC)
	lead := (int(first.Weekday()) + 6) % 7 // Monday=0
	n := daysIn(m.y, m.mo)
	todayInMonth := !today.IsZero() && today.Year() == m.y && int(today.Month()) == m.mo

	var b strings.Builder
	fmt.Fprintf(&b, "  %s %d\n", monthNames[m.mo], m.y)
	b.WriteString("  Mo Di Mi Do Fr Sa So\n  ")
	for i := 0; i < lead; i++ {
		b.WriteString("   ")
	}
	for day := 1; day <= n; day++ {
		cell := fmt.Sprintf("%2d", day)
		switch {
		case day == m.d:
			cell = theme.Active(cell, m.pal)
		case todayInMonth && today.Day() == day:
			cell = theme.Dim(cell, m.pal)
		}
		b.WriteString(cell + " ")
		if (lead+day)%7 == 0 {
			b.WriteString("\n  ")
		}
	}
	return b.String()
}
