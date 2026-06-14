package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

type dayoffsLoadedMsg struct{ list []apiclient.DayOff }

func (m Model) reloadDayOffs() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		year := time.Now().Year()
		list, err := m.client.ListDayOffs(ctx,
			fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
		if err != nil {
			return errMsg{err}
		}
		return dayoffsLoadedMsg{list: list}
	}
}

func (m Model) dayOffView() tea.View {
	var b strings.Builder
	b.WriteString(styleHeader.Render("flow · dayoffs") + styleMuted.Render("  "+m.user) + "\n\n")
	if len(m.dayoffs) == 0 {
		b.WriteString(styleMuted.Render("  no day-offs this year") + "\n")
	}
	for _, d := range m.dayoffs {
		glyph := dayOffGlyph(d.Holiday)
		label := d.Label
		if label == "" {
			label = d.Kind
		}
		fmt.Fprintf(&b, "  %s %s  %s\n", glyph, d.Day, label)
	}
	b.WriteString("\n" + styleMuted.Render("esc back · (add/remove via WebUI or `flow dayoff`)") + "\n")
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// dayOffGlyph mirrors the v1 Dayoff-Glyph-Unification: one ○ marker; holidays
// render dimmed.
func dayOffGlyph(holiday bool) string {
	if holiday {
		return styleMuted.Render("○")
	}
	return "○"
}
