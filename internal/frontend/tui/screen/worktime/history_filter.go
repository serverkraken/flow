package worktime

// History filter — Filter-Dialog (öffnen, Key-Handling, Step), Filter-
// Renderer und die Pure-Filter-Funktionen (filteredHistory + by-Tag /
// by-Note / by-ISOWeek / by-Range). Split aus history.go (Skill
// §No-Monoliths): Filter-Logik formt einen geschlossenen Cluster und
// gehört nicht ins Mode-Routing.

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/form"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// — filter dialog —

func (h history) openFilter(seed string) (tea.Model, tea.Cmd) {
	h.dialog = historyDialogFilter
	h.input = form.NewTextInput("KWxx · YYYY · YYYY-MM · tag:foo · note:bar", h.pal)
	if seed != "" {
		h.input.SetValue(seed)
		h.input.CursorEnd()
	} else {
		h.input.SetValue(h.histQuery)
		h.input.CursorEnd()
	}
	h.input.Focus()
	h.errMsg = ""
	return h, textinput.Blink
}

func (h history) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		h.dialog = historyDialogNone
		h.input.Blur()
		h.input.SetValue("")
		h.errMsg = ""
		return h, nil
	case "enter":
		q := strings.TrimSpace(h.input.Value())
		if q != "" {
			if _, err := domain.ParseRange(h.deps.Clock.Now(), q); err != nil &&
				!isTagOrNote(q) && !isISOWeek(q) {
				h.errMsg = err.Error()
				return h, nil
			}
		}
		h.histQuery = q
		h.listCur = 0
		h.dialog = historyDialogNone
		h.input.Blur()
		h.input.SetValue("")
		h.errMsg = ""
		return h, nil
	}
	h.errMsg = ""
	var cmd tea.Cmd
	h.input, cmd = h.input.Update(msg)
	return h, cmd
}

func (h history) stepFilter(dir int) (tea.Model, tea.Cmd) {
	next, ok := stepHistFilter(h.histQuery, h.deps.Clock.Now(), dir)
	if !ok {
		return h, nil
	}
	h.histQuery = next
	h.listCur = 0
	return h, nil
}

func (h history) renderFilterDialog() string {
	inner := h.width - 4
	if inner <= 0 {
		inner = 80
	}
	rows := []string{
		picker.SectionHeader("filter", inner, h.pal),
		"  " + h.input.View(),
	}
	val := strings.ToLower(strings.TrimSpace(h.input.Value()))
	if strings.HasPrefix(val, "tag:") && len(h.topTags) > 0 {
		rows = append(rows, "")
		rows = append(rows, stDim(h.pal, "  häufigste Tags:"))
		rows = append(rows, "  "+strings.Join(h.topTags, "  ·  "))
	}
	rows = append(rows, "")
	rows = append(rows, stDim(h.pal,
		"  Beispiele:  KW18  ·  2026  ·  2026-04  ·  2026-04-01..2026-04-30  ·  tag:deep  ·  note:standup"))
	if h.errMsg != "" {
		rows = append(rows, "", theme.Err("  "+h.errMsg, h.pal))
	}
	rows = append(rows, "", stDim(h.pal,
		"  Enter → anwenden  ·  leer → alles  ·  Esc → abbrechen"))
	return strings.Join(rows, "\n")
}

// — pure filter helpers —

func filteredHistory(records []domain.DayRecord, query string, now time.Time) []domain.DayRecord {
	q := strings.TrimSpace(query)
	if q == "" {
		return records
	}
	if out, ok := filterByTag(records, q); ok {
		return out
	}
	if out, ok := filterByNote(records, q); ok {
		return out
	}
	if out, ok := filterByISOWeek(records, q, now); ok {
		return out
	}
	if out, ok := filterByRange(records, q, now); ok {
		return out
	}
	return records
}

func filterByTag(records []domain.DayRecord, q string) ([]domain.DayRecord, bool) {
	if !strings.HasPrefix(strings.ToLower(q), "tag:") {
		return nil, false
	}
	want := strings.TrimSpace(q[len("tag:"):])
	if want == "" {
		return records, true
	}
	out := make([]domain.DayRecord, 0, len(records))
	for _, rec := range records {
		var keep []domain.Session
		var total time.Duration
		for _, s := range rec.Sessions {
			if strings.EqualFold(s.Tag, want) {
				keep = append(keep, s)
				total += s.Elapsed
			}
		}
		if len(keep) > 0 {
			out = append(out, domain.DayRecord{
				Date: rec.Date, Sessions: keep, Total: total, Target: rec.Target,
			})
		}
	}
	return out, true
}

func filterByNote(records []domain.DayRecord, q string) ([]domain.DayRecord, bool) {
	if !strings.HasPrefix(strings.ToLower(q), "note:") {
		return nil, false
	}
	want := strings.ToLower(strings.TrimSpace(q[len("note:"):]))
	if want == "" {
		return records, true
	}
	out := make([]domain.DayRecord, 0, len(records))
	for _, rec := range records {
		var keep []domain.Session
		var total time.Duration
		for _, s := range rec.Sessions {
			if strings.Contains(strings.ToLower(s.Note), want) {
				keep = append(keep, s)
				total += s.Elapsed
			}
		}
		if len(keep) > 0 {
			out = append(out, domain.DayRecord{
				Date: rec.Date, Sessions: keep, Total: total, Target: rec.Target,
			})
		}
	}
	return out, true
}

func filterByISOWeek(records []domain.DayRecord, q string, now time.Time) ([]domain.DayRecord, bool) {
	if !strings.HasPrefix(strings.ToUpper(q), "KW") {
		return nil, false
	}
	var w int
	if _, err := fmt.Sscanf(strings.ToUpper(q), "KW%d", &w); err != nil || w <= 0 {
		return nil, false
	}
	out := make([]domain.DayRecord, 0, len(records))
	for _, r := range records {
		_, rw := r.Date.ISOWeek()
		if rw == w && r.Date.Year() == now.Year() {
			out = append(out, r)
		}
	}
	return out, true
}

func filterByRange(records []domain.DayRecord, q string, now time.Time) ([]domain.DayRecord, bool) {
	r, err := domain.ParseRange(now, q)
	if err != nil || (r.From.IsZero() && r.To.IsZero()) {
		return nil, false
	}
	out := make([]domain.DayRecord, 0, len(records))
	for _, rec := range records {
		if r.ContainsDate(rec.Date) {
			out = append(out, rec)
		}
	}
	return out, true
}

func isTagOrNote(q string) bool {
	low := strings.ToLower(q)
	return strings.HasPrefix(low, "tag:") || strings.HasPrefix(low, "note:")
}

func isISOWeek(q string) bool {
	if !strings.HasPrefix(strings.ToUpper(q), "KW") {
		return false
	}
	var w int
	_, err := fmt.Sscanf(strings.ToUpper(q), "KW%d", &w)
	return err == nil && w > 0
}

// stepHistFilter advances `q` by `dir` units. KWnn → ±1 week, YYYY-MM →
// ±1 month, YYYY → ±1 year. tag: / note: filters return ok=false. Empty
// is seeded to the current ISO week so paginating without a manual step
// still works.
func stepHistFilter(q string, now time.Time, dir int) (string, bool) {
	q = strings.TrimSpace(q)
	if q == "" {
		_, wn := now.ISOWeek()
		seed := fmt.Sprintf("KW%d", wn)
		return stepHistFilter(seed, now, dir)
	}
	if isTagOrNote(q) {
		return q, false
	}
	if strings.HasPrefix(strings.ToUpper(q), "KW") {
		var w int
		if _, err := fmt.Sscanf(strings.ToUpper(q), "KW%d", &w); err != nil {
			return q, false
		}
		mon := isoMondayOfISOWeek(now.Year(), w, now.Location())
		shifted := mon.AddDate(0, 0, 7*dir)
		_, ww := shifted.ISOWeek()
		return fmt.Sprintf("KW%d", ww), true
	}
	if len(q) == 7 && q[4] == '-' {
		t, err := time.ParseInLocation("2006-01", q, now.Location())
		if err != nil {
			return q, false
		}
		shifted := t.AddDate(0, dir, 0)
		return shifted.Format("2006-01"), true
	}
	if len(q) == 4 {
		var y int
		if _, err := fmt.Sscanf(q, "%d", &y); err != nil {
			return q, false
		}
		return fmt.Sprintf("%d", y+dir), true
	}
	return q, false
}

func isoMondayOfISOWeek(year, week int, loc *time.Location) time.Time {
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, loc)
	wd := int(jan4.Weekday())
	if wd == 0 {
		wd = 7
	}
	mon1 := jan4.AddDate(0, 0, -(wd - 1))
	return mon1.AddDate(0, 0, 7*(week-1))
}
