package worktime

// Paket-shared Render- und Format-Helpers, die alle vier Sub-Tab-Files
// (today.go, week.go, history.go, dayoffs.go) teilen. Vorher saßen sie
// am Boden von today.go und liefen bei jedem Heute-Edit mit Diff —
// gehören semantisch in eine eigene Datei (Skill §No-Monoliths).

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Shared lipgloss styles for the render hot paths. These have no
// palette-dependent fields, so a single package-level value is sound
// — promoting them out of per-render `lipgloss.NewStyle()` calls
// avoids 2–4 style allocations per session row per frame in Heute /
// History rendering. Palette-dependent styling stays at the call site
// (via `.Foreground(c)` on top of these bases).
var (
	durationWidth8Style = lipgloss.NewStyle().Width(8)
	boldStyle           = lipgloss.NewStyle().Bold(true)
	// fgStyle ist die foreground-only-Base ohne Bold. Wird vom Heute-
	// Headline gebraucht, damit Total keine Bold+Accent-Adjacency mit
	// der Status-Pille bildet (Skill §Color semantics).
	fgStyle = lipgloss.NewStyle()
)

// renderFormField liefert die zwei Zeilen für ein Eingabe-Form-Feld:
// SectionHeader plus entweder den Live-Input (ti.View() bei focused)
// oder den ungetippten Wert/Placeholder (dim). Pattern C1 aus dem
// TUI-Review — vorher 2× wortgleich in dayoffs.renderAddFields und
// history_edit.renderDrillFormDialog kopiert.
func renderFormField(label string, ti textinput.Model, focused bool, inner int, pal theme.Palette) []string {
	rows := []string{picker.SectionHeader(label, inner, pal)}
	if focused {
		rows = append(rows, "  "+ti.View())
		return rows
	}
	v := ti.Value()
	if v == "" {
		v = stDim(pal, ti.Placeholder)
	}
	rows = append(rows, "    "+v)
	return rows
}

// formatDur formats a duration as "Xh YYm" with zero-padded minutes.
// Negative values clamp to 0 so a clock-skew or pre-start preview can
// never render an angry "-3h 14m" cell.
func formatDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

// formatDurLive adds zero-padded seconds for the first-minute live view.
func formatDurLive(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%dh %02dm %02ds", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

// stDim is the worktime-screen-local thin wrapper over the central
// theme.Dim builder. Kept as a wrapper (rather than open-coding
// theme.Dim at call-sites) because every worktime-tab calls this
// dozens of times — the short name + arg order is the screen's
// existing idiom. Same for stErr below, plus the "  "-indent prefix
// that's load-bearing for error rows under the box border.
func stDim(p theme.Palette, s string) string { return theme.Dim(s, p) }

func stErr(p theme.Palette, s string) string { return theme.Err("  "+s, p) }

// renderFooterHints joins the action chips into one or more dim lines that
// fit inside `inner`. Each wrapped line is dim-styled separately because
// lipgloss pads multi-line styled strings (see TestStDimMultilinePadsShorterLines)
// — passing the whole "\n"-joined string through stDim would leak trailing
// spaces into the previous box border.
func renderFooterHints(p theme.Palette, parts []string, inner int) string {
	wrapped := joinWrapped(parts, "  ·  ", "  ", "  ", inner)
	lines := strings.Split(wrapped, "\n")
	for i, l := range lines {
		lines[i] = stDim(p, l)
	}
	return strings.Join(lines, "\n")
}

// joinWrapped joins parts with sep, wrapping when the line would exceed
// maxWidth. prefix on the first wrapped line; cont on the followers.
//
// A single part wider than maxWidth (e.g. a paste-bombed Note token) is
// emitted on its own line, even though it overshoots — the helper can't
// split a chip and silently dropping data is worse than visual overrun.
// See wrap_test.go: TestJoinWrapped_SinglePartLongerThanWidth.
func joinWrapped(parts []string, sep, prefix, cont string, maxWidth int) string {
	if len(parts) == 0 {
		return ""
	}
	if maxWidth <= 0 {
		return prefix + strings.Join(parts, sep)
	}
	var lines []string
	cur := prefix + parts[0]
	for _, p := range parts[1:] {
		cand := cur + sep + p
		if lipgloss.Width(cand) > maxWidth {
			lines = append(lines, cur)
			cur = cont + p
		} else {
			cur = cand
		}
	}
	lines = append(lines, cur)
	return strings.Join(lines, "\n")
}

// sessionHint builds the right-hand hint column for a session row in the
// Heute and History-Drill session lists. The hint carries the project name
// (when available) and/or the tag — displayed inside picker.Row's right slot.
//
// Priority: "ProjectName  [tag]" when both are present; only the project name
// or only the tag when just one is set; empty string when neither.
// The tag bracket stays canonical ("[tag]") so existing hint styling is
// unchanged. The project name prefixes the tag with two-space separation so
// both can appear without running together.
func sessionHint(s domain.Session) string {
	name := s.ProjectName
	if name == "" && len(s.ProjectID) > 0 {
		// Fallback: last 8 chars of UUID — project not enriched (offline /
		// unregistered project). Keep the hint recognisable as an ID, not
		// a name, so the user knows the display is incomplete.
		id := s.ProjectID
		if len(id) > 8 {
			id = id[len(id)-8:]
		}
		name = id
	}
	switch {
	case name != "" && s.Tag != "":
		return name + "  [" + s.Tag + "]"
	case name != "":
		return name
	case s.Tag != "":
		return "[" + s.Tag + "]"
	}
	return ""
}

// sameDay reports whether a and b fall on the same calendar day.
func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

// startOfDay normalises t to midnight in its location.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
