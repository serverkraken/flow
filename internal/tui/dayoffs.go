package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

type dayoffsLoadedMsg struct{ list []apiclient.DayOff }
type settingsLoadedMsg struct{ settings apiclient.Settings }
type setTargetDoneMsg struct{}

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

func (m Model) reloadSettings() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s, err := m.client.GetSettings(ctx)
		if err != nil {
			return errMsg{err}
		}
		return settingsLoadedMsg{settings: s}
	}
}

func (m Model) setTargetCmd(defaultMin int) tea.Cmd {
	if m.client == nil {
		return nil
	}
	weekday := m.settings.WeekdayTargetMin
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := m.client.SetTargetConfig(ctx, defaultMin, weekday); err != nil {
			return errMsg{err}
		}
		return setTargetDoneMsg{}
	}
}

func (m Model) dayOffView() tea.View {
	var b strings.Builder
	b.WriteString(styleHeader.Render("flow · dayoffs") + styleMuted.Render("  "+m.user) + "\n\n")

	// Settings: target config display / edit.
	if m.editingTarget {
		b.WriteString(styleMuted.Render("  Neues Tagesziel (min): ") + m.targetInput + "▏\n")
		b.WriteString(styleMuted.Render("  Ziffern eingeben · Enter bestätigen · esc abbrechen") + "\n\n")
	} else {
		targetLine := fmt.Sprintf("  Tagesziel: %s", fmtMin(m.settings.DefaultTargetMin))
		if len(m.settings.WeekdayTargetMin) > 0 {
			// Build sorted compact summary of weekday overrides.
			type kv struct{ k, v string }
			var overrides []kv
			for k, v := range m.settings.WeekdayTargetMin {
				overrides = append(overrides, kv{k: k, v: fmtMin(v)})
			}
			sort.Slice(overrides, func(i, j int) bool { return overrides[i].k < overrides[j].k })
			parts := make([]string, 0, len(overrides))
			for _, o := range overrides {
				parts = append(parts, weekdayShort(o.k)+" "+o.v)
			}
			targetLine += styleMuted.Render("  (" + strings.Join(parts, ", ") + ")")
		}
		b.WriteString(targetLine + "\n\n")
	}

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
	var hint string
	if m.editingTarget {
		hint = "enter bestätigen · esc abbrechen"
	} else {
		hint = "g Ziel ändern · esc back · (add/remove via WebUI or `flow dayoff`)"
	}
	b.WriteString("\n" + styleMuted.Render(hint) + "\n")
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

// handleDayOffKey handles keys within the DayOff sub-view (called from handleKey
// when showDayOffs is true and it's not a generic esc).
func (m Model) handleDayOffKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.editingTarget {
		switch {
		case k.Code == tea.KeyEsc:
			m.editingTarget = false
			m.targetInput = ""
			return m, nil
		case k.Code == tea.KeyEnter:
			mins := parseMinutes(m.targetInput)
			if mins <= 0 {
				// Invalid input — stay in edit mode.
				return m, nil
			}
			m.editingTarget = false
			m.targetInput = ""
			return m, m.setTargetCmd(mins)
		case k.Code == tea.KeyBackspace:
			if r := []rune(m.targetInput); len(r) > 0 {
				m.targetInput = string(r[:len(r)-1])
			}
			return m, nil
		case k.Text != "" && unicode.IsDigit([]rune(k.Text)[0]):
			m.targetInput += k.Text
			return m, nil
		}
		return m, nil
	}
	// Normal DayOff view.
	if k.Text == "g" {
		m.editingTarget = true
		m.targetInput = ""
		return m, nil
	}
	return m, nil
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

// parseMinutes interprets the raw digit string as minutes directly.
func parseMinutes(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
