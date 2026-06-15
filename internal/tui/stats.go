package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

type weekLoadedMsg struct{ week []apiclient.WeekDay }
type rangeLoadedMsg struct {
	rng   string
	stats apiclient.Stats
}

func (m Model) reloadWeek() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		wk, err := m.client.GetWeek(ctx, "")
		if err != nil {
			return errMsg{err}
		}
		return weekLoadedMsg{week: wk}
	}
}

func (m Model) reloadRange() tea.Cmd {
	if m.client == nil {
		return nil
	}
	rng := m.statsRng
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		st, err := m.client.GetStats(ctx, rng)
		if err != nil {
			return errMsg{err}
		}
		return rangeLoadedMsg{rng: rng, stats: st}
	}
}

func (m Model) weekView() tea.View {
	var b strings.Builder
	b.WriteString(styleHeader.Render("flow · Woche") + "\n\n")
	for _, d := range m.week {
		marker := "  "
		if d.IsToday {
			marker = styleRunning.Render("▶") + " "
		}
		line := fmt.Sprintf("%s%s  %s %s/%s",
			marker, d.Date, bar(d.LoggedMin, d.TargetMin, 20),
			fmtMin(d.LoggedMin), fmtMin(d.TargetMin))
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + styleMuted.Render("esc back · q quit") + "\n")
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func (m Model) statsView() tea.View {
	var b strings.Builder
	label := "KW"
	if m.statsRng == "month" {
		label = "Monat"
	}
	b.WriteString(styleHeader.Render("flow · Stats · "+label) + "\n\n")
	s := m.stats
	rows := [][2]string{
		{"Total", fmtMin(s.TotalMin)},
		{"⌀/Tag", fmtMin(s.AvgMin)},
		{"Max", fmtMin(s.MaxMin)},
		{"Min", fmtMin(s.MinMin)},
		{"Arbeitstage", fmt.Sprintf("%d", s.Workdays)},
		{"Treffer", fmt.Sprintf("%d/%d", s.Hits, s.Workdays)},
		{"Streak", fmt.Sprintf("%d (best %d)", s.Streak, s.BestStreak)},
		{"Saldo", fmtSaldo(s.OvertimeMin)},
	}
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("  %-12s %s\n", r[0], r[1]))
	}
	b.WriteString("\n" + styleMuted.Render("W Woche · m Monat · esc back · q quit") + "\n")
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// bar renders a fixed-width horizontal progress bar (filled vs target).
func bar(logged, target, width int) string {
	if target <= 0 {
		target = 1
	}
	filled := logged * width / target
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

// weekdayShort maps weekday key strings "0".."6" (Sunday=0) to short German names.
func weekdayShort(key string) string {
	names := map[string]string{
		"0": "So", "1": "Mo", "2": "Di",
		"3": "Mi", "4": "Do", "5": "Fr", "6": "Sa",
	}
	if n, ok := names[key]; ok {
		return n
	}
	return key
}
