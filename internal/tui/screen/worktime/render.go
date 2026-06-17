package worktime

import (
	"fmt"
	"image/color"
	"time"

	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
	"github.com/serverkraken/flow/internal/tui/ui/picker"
	"github.com/serverkraken/flow/internal/tui/ui/statusbar"
	"github.com/serverkraken/flow/internal/tui/ui/toast"
)

func renderBody(st todayState, cursor, width, height int, now time.Time, tt *toast.Model, pal theme.Palette) string {
	inner := width - 4
	if inner <= 0 {
		inner = theme.WideBox
	}
	header := []string{
		renderDateLine(now, pal),
		renderHeadline(st, now, pal),
		"",
		renderProgressBar(st, inner, now, pal),
		renderSummary(st, now, pal),
	}
	mid, focus := renderSessionsList(st, cursor, inner, now, pal)

	var footer []string
	if tt != nil {
		footer = toast.SlotRows(tt, "  ")
	}
	return fitHeight(header, mid, footer, focus, bodyBudget(height), pal)
}

func renderDateLine(now time.Time, pal theme.Palette) string {
	return theme.Gap(theme.PadSM) + theme.Dim(fmtDateDe(now), pal)
}

func renderHeadline(st todayState, now time.Time, pal theme.Palette) string {
	total := st.Total(now)
	target := st.Target
	glyph, label, statusColor := todayStatusBadge(pal, st.Running, target > 0 && total >= target)

	totalText := formatDur(total)
	if st.Running && st.Active != nil && now.Sub(*st.Active) < time.Minute {
		totalText = formatDurLive(total)
	}
	totalStr := fgStyle.Foreground(totalThresholdColor(pal, total, target, st.Running)).Render(totalText)
	statusStr := boldStyle.Foreground(statusColor).Render(glyph + " " + label)
	pctStr := theme.Dim("kein Ziel", pal)
	if target > 0 {
		pctStr = theme.Dim(fmt.Sprintf("Ziel %d%%", pctOfTarget(total, target)), pal)
	}
	gap4 := theme.Gap(theme.PadMD + theme.PadXS)
	return theme.Gap(theme.PadSM) + totalStr + gap4 + statusStr + gap4 + pctStr
}

func renderProgressBar(st todayState, inner int, now time.Time, pal theme.Palette) string {
	total := st.Total(now)
	pct := pctOfTarget(total, st.Target)
	cells := inner - 4
	if cells < 4 {
		cells = 4
	}
	barColor := totalThresholdColor(pal, total, st.Target, st.Running)
	return "  " + statusbar.BarColored(pct, cells, barColor, pal)
}

func renderSummary(st todayState, now time.Time, pal theme.Palette) string {
	if st.Target <= 0 {
		return joinHints([]string{"kein Tagesziel"}, pal)
	}
	total := st.Total(now)
	remaining := st.Target - total
	if remaining < 0 {
		remaining = 0
	}
	parts := []string{
		fmt.Sprintf("Ziel %s", formatDur(st.Target)),
		fmt.Sprintf("noch %s", formatDur(remaining)),
	}
	if eta, ok := st.ETA(); ok {
		parts = append(parts, "ETA "+eta.Format("15:04"))
	}
	return joinHints(parts, pal)
}

func renderSessionsList(st todayState, cursor, inner int, now time.Time, pal theme.Palette) (rows []string, focus int) {
	total := len(st.Completed)
	if st.Running {
		total++
	}
	if total == 0 {
		return []string{"", theme.Dim("  Noch nichts erfasst — `s` startet", pal)}, 0
	}
	rows = []string{"", picker.SectionHeader(fmt.Sprintf("sessions heute (%d)", total), inner, pal)}

	if st.Running && st.Active != nil {
		elapsed := now.Sub(*st.Active)
		rows = append(rows, theme.Active(
			fmt.Sprintf("  %s %s → …   %s", glyphs.Active, st.Active.Format("15:04"), formatDurLive(elapsed)), pal))
	}
	for i, s := range st.Completed {
		if s.GapBefore > 0 {
			rows = append(rows, theme.Dim(
				fmt.Sprintf("%s%s Pause %s", theme.Gap(theme.PadMD*2+theme.PadXS), glyphs.BulletDot, formatDur(s.GapBefore)), pal))
		}
		dur := durationWidth8Style.Render(formatDur(s.Elapsed))
		label := fmt.Sprintf("%s → %s   %s", s.Start.Format("15:04"), s.Stop.Format("15:04"), dur)
		hint := ""
		if s.Tag != "" {
			hint = "[" + s.Tag + "]"
		}
		if i == cursor {
			focus = len(rows)
		}
		rows = append(rows, picker.Row(i == cursor, label, hint, inner, pal))
		if s.Note != "" {
			rows = append(rows, theme.Dim("       "+s.Note, pal))
		}
	}
	return rows, focus
}

func todayStatusBadge(p theme.Palette, running, achieved bool) (string, string, color.Color) {
	sem := p.Sem()
	switch {
	case running && achieved:
		return glyphs.Active, "läuft " + glyphs.Done, sem.Success
	case running:
		return glyphs.Active, "läuft", sem.Active
	case achieved:
		return glyphs.Done, "Ziel erreicht", sem.Success
	}
	return glyphs.Paused, "pausiert", p.FgMuted
}

func totalThresholdColor(p theme.Palette, total, target time.Duration, running bool) color.Color {
	sem := p.Sem()
	if target <= 0 {
		if running {
			return sem.Active
		}
		return p.FgMuted
	}
	switch {
	case total >= target+4*time.Hour:
		return sem.Danger
	case total >= target:
		return sem.Success
	case running && total >= target-2*time.Hour:
		return sem.Warning
	case running:
		return sem.Active
	}
	return p.FgMuted
}
