package worktime

// History list-mode rendering — die Default-Ansicht des History-Tabs.
// Pro KW gruppierte Tagezeile mit Bar / Pct / Total. Plus die Render-
// Hülle (renderMain) und der Header (Volumen / Performance Strip).

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func (h history) renderMain() string {
	inner := h.width - 4
	if inner <= 0 {
		inner = 80
	}
	records := filteredHistory(h.records, h.histQuery, h.deps.Clock.Now())

	// Header (volumen / performance strip) pinned at the top, footer
	// (toast + hints) at the bottom; the day list / grid in between is the
	// scrollable middle fitHeight windows around the cursor when the
	// terminal is too short to show every row at once.
	header := append(strings.Split(h.renderHeader(records, inner), "\n"), "")

	var mid []string
	focus := 0
	switch h.mode {
	case historyModeHeatmap:
		mid = strings.Split(h.renderHeatmap(records, inner), "\n")
	case historyModeTagClock:
		mid = strings.Split(h.renderTagClock(records, inner), "\n")
	case historyModeMonth:
		mid = strings.Split(h.renderMonth(records, inner), "\n")
	default:
		if len(records) == 0 {
			msg := "  Keine Treffer."
			if h.histQuery != "" {
				msg += "  ·  T → Filter zurücksetzen"
			}
			mid = []string{stDim(h.pal, msg)}
		} else {
			mid, focus = h.renderListRows(records, inner)
		}
	}

	footer := toast.SlotRows(h.listToast, "  ")
	footer = append(footer, "", renderFooterHints(h.pal, h.footerHints(), inner))

	return fitHeight(header, mid, footer, focus, bodyBudget(h.height), h.pal)
}

func (h history) renderHeader(records []domain.DayRecord, inner int) string {
	st := h.deps.Stats.Aggregate(records)
	if st.Days == 0 {
		// Empty mit aktivem Filter: Recovery-Hint + Filter-Chip, sonst
		// nur "Keine Treffer." — sonst wirkt der Zustand „kaputt" (selbe
		// Meldung wie wenn nie gearbeitet wurde, aber der User sieht den
		// Filter nicht oder weiß nicht, wie er ihn löscht).
		if h.histQuery != "" {
			filterChip := "  ·  " + lipgloss.NewStyle().Foreground(h.pal.Sem().Info).Render("filter: "+h.histQuery)
			recovery := "  ·  T → Filter zurücksetzen"
			return stDim(h.pal, "  Keine Treffer."+recovery) + filterChip
		}
		return stDim(h.pal, "  Keine Treffer.")
	}
	sem := h.pal.Sem()
	balColor := h.pal.FgMuted
	switch {
	case st.Overtime > 0:
		balColor = sem.Success
	case st.Overtime < 0:
		balColor = sem.Warning
	}
	bal := lipgloss.NewStyle().Foreground(balColor).Bold(true).Render(domain.FmtSignedDuration(st.Overtime))
	// Label-Value-Hierarchie: Label dim, Wert bold/colored. SectionHeader
	// trennt Volumen / Performance optisch — gleiche Konvention wie
	// SESSIONS HEUTE / WOCHE GESAMT, damit der User die Strip-Hierarchie
	// sofort liest.
	kvBold := func(label, value string) string {
		return stDim(h.pal, label+" ") + lipgloss.NewStyle().Bold(true).Render(value)
	}
	kv := func(label, value string) string {
		return stDim(h.pal, label+" ") + value
	}
	volume := []string{
		kvBold("Tage", fmt.Sprintf("%d", st.Days)),
		kv("Werktage", fmt.Sprintf("%d", st.Workdays)),
		kvBold("Total", formatDur(st.Total)),
		kv("Schnitt", formatDur(st.Avg)),
		kv("Max", formatDur(st.Max)),
		kv("Min", formatDur(st.Min)),
	}
	performance := []string{
		kv("Ziele", fmt.Sprintf("%d/%d", st.Hits, st.Workdays)),
		kv("Streak", fmt.Sprintf("%d", st.Streak)),
		kv("Beststreak", fmt.Sprintf("%d", st.BestStreak)),
		stDim(h.pal, "Saldo ") + bal,
	}
	header := picker.SectionHeader("volumen", inner, h.pal) + "\n" +
		joinWrapped(volume, "  ·  ", "  ", "  ", inner) + "\n" +
		picker.SectionHeader("performance", inner, h.pal) + "\n" +
		joinWrapped(performance, "  ·  ", "  ", "  ", inner)
	if h.histQuery != "" {
		header += "\n  " + stDim(h.pal, "filter: ") +
			lipgloss.NewStyle().Foreground(sem.Info).Bold(true).Render(h.histQuery)
	}
	return header
}

// renderListRows builds the per-day list rows (grouped by ISO week) and
// reports the index of the row carrying the cursor, so fitHeight can keep
// the selected day visible when the list overflows the terminal height.
func (h history) renderListRows(records []domain.DayRecord, inner int) (lines []string, focus int) {
	const barW = 12
	prevWeek := -1
	prevYear := -1
	for i, rec := range records {
		y, w := rec.Date.ISOWeek()
		if w != prevWeek || y != prevYear {
			if prevWeek != -1 {
				lines = append(lines, "")
			}
			// SectionHeader (`KW 19 — ─────`) statt theme.Heading: KW-Gruppen
			// sind List-Sub-Sections, keine Screen-Titel; Heading-Style war
			// purple-bold und damit visuell auf Dialog-Titel-Niveau.
			lines = append(lines, picker.SectionHeader(fmt.Sprintf("KW %d / %d", w, y), inner, h.pal))
			prevWeek, prevYear = w, y
		}
		pct := 0
		if rec.Target > 0 {
			pct = int(rec.Total * 100 / rec.Target)
			if pct > 100 {
				pct = 100
			}
		}
		name := lipgloss.NewStyle().Foreground(h.pal.Fg).Width(3).
			Render(domain.WeekdayShortDe(rec.Date.Weekday()))
		date := lipgloss.NewStyle().Foreground(h.pal.FgMuted).Width(9).
			Render(fmt.Sprintf("%02d.%02d.%02d", rec.Date.Day(), rec.Date.Month(), rec.Date.Year()%100))
		bar := statusbar.Bar(pct, barW, h.pal)
		pctStr := stDim(h.pal, fmt.Sprintf("%3d%%", pct))
		durStr := lipgloss.NewStyle().Foreground(h.pal.Fg).Bold(rec.Total >= rec.Target).
			Render(formatDur(rec.Total))
		done := ""
		if rec.Total >= rec.Target {
			done = "  " + theme.Success(glyphs.Done, h.pal)
		}
		marker := "  "
		if i == h.listCur {
			marker = lipgloss.NewStyle().Foreground(h.pal.Sem().Accent).Render(picker.AccentBarRune) + " "
			focus = len(lines)
		}
		notes := h.attachedChip(rec.Date)
		lines = append(lines, marker+name+" "+date+"  "+bar+"  "+pctStr+"  "+durStr+done+notes)
	}
	return lines, focus
}

// attachedChip rendert den Note-Indikator-Suffix einer Zeile, leer wenn
// am Tag keine Notes haengen. Ein einzelnes ● fuer 1 Note, ●N fuer 2+ —
// gleicher Glyph wie im Drill-Chip (renderDrillAttachedNotes), damit
// das visuelle Vokabular zwischen Liste und Drill konsistent bleibt.
// Highlight-Foreground statt Dim, weil der Marker sonst neben dem
// dim-Done-Haken zu unauffaellig wuerde.
func (h history) attachedChip(date time.Time) string {
	n := h.attachedCounts[date.Format("2006-01-02")]
	if n <= 0 {
		return ""
	}
	label := glyphs.Filled
	if n > 1 {
		label = fmt.Sprintf("%s %d", glyphs.Filled, n)
	}
	return "  " + theme.Highlight(label, h.pal)
}

// footerHints — Skill §Hint format max 4. Top-4 nach Frequenz:
// navigieren, drill, Ansicht-Cycle, filter. Der `v`-Hint zeigt den
// *nächsten* Mode statt des aktuellen — sonst muss der User raten,
// was er drückt. `:` (Aktions-Menü), `[/]`, `T` und `F` leben im
// `?`-Overlay; die View-Modi sind das wertvollste verborgene Feature
// und gehören in den Footer.
func (h history) footerHints() []string {
	return []string{
		"j/k → bewegen",
		"enter → drill",
		"v → " + h.mode.next().label(),
		"/ → filter",
	}
}
