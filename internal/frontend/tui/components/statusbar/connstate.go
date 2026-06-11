package statusbar

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/ports"
)

// ConnState rendert die Server-Statuszeile (Spec §8): still wenn ok, laut
// wenn nicht. Glyph + Farbe, nie Farbe allein (A11y-Regel).
func ConnState(s ports.StatusSnapshot, pal theme.Palette) string {
	sem := pal.Sem()
	switch s.State {
	case ports.StateOnline:
		return lipgloss.NewStyle().Foreground(pal.FgMuted).
			Render(fmt.Sprintf("● %s", hostOnly(s.Host)))
	case ports.StateOffline:
		stand := "—"
		if !s.LastFetched.IsZero() {
			stand = s.LastFetched.Local().Format("15:04")
		}
		return lipgloss.NewStyle().Foreground(sem.Warning).
			Render(fmt.Sprintf("○ offline · Stand %s (read-only)", stand))
	case ports.StateLoggedOut:
		return lipgloss.NewStyle().Foreground(sem.Warning).
			Render("○ nicht angemeldet · flow login")
	case ports.StateNotConfigured:
		return lipgloss.NewStyle().Foreground(sem.Warning).
			Render("○ kein Server · FLOW_SERVER_URL setzen")
	case ports.StateOutdated:
		return lipgloss.NewStyle().Foreground(sem.Danger).
			Render("▲ Client veraltet · Update nötig")
	default:
		return ""
	}
}

func hostOnly(base string) string {
	s := base
	for _, pre := range []string{"https://", "http://"} {
		if len(s) > len(pre) && s[:len(pre)] == pre {
			s = s[len(pre):]
		}
	}
	if i := indexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	return s
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
