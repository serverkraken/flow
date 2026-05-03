// Package theme is a thin compatibility view over the canonical token
// package at internal/frontend/tui/theme.
//
// It exposes the smaller 11-field Palette that the existing component
// library was built against, plus the tmux @tn_* overlay flow and the
// TrueColor renderer setup. New component code should reach for the
// canonical theme package directly; this package stays for back-compat
// while screens migrate (P3/P4 of the design-system roadmap).
package theme

import (
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	canonical "github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// Palette is the 11-field colour set consumed by the component library.
// Field roles (mapped onto canonical tokens):
//
//	Bg     → canonical.Bg
//	Fg     → canonical.Fg
//	Accent → canonical.Sem().Accent  (Blue)
//	Dim    → canonical.FgMuted       (hint / meta)
//	Border → canonical.BgCode        (panel border)
//	Hues   → canonical hues (1:1)
type Palette struct {
	Bg, Fg, Accent, Dim, Border              lipgloss.Color
	Purple, Green, Red, Orange, Yellow, Cyan lipgloss.Color
}

// Fallback projects the canonical default palette (theme.Default) onto
// the 11-field shape this package exposes. Used as the starting point
// for Load() before tmux @tn_* overlays are applied; exported so tests
// can assert against canonical values without depending on whether a
// tmux server happens to be running on the dev machine.
func Fallback() Palette {
	p := canonical.Default
	sem := p.Sem()
	return Palette{
		Bg:     lipgloss.Color(p.Bg),
		Fg:     lipgloss.Color(p.Fg),
		Accent: lipgloss.Color(sem.Accent),
		Dim:    lipgloss.Color(p.FgMuted),
		Border: lipgloss.Color(p.BgCode),
		Purple: lipgloss.Color(p.Purple),
		Green:  lipgloss.Color(p.Green),
		Red:    lipgloss.Color(p.Red),
		Orange: lipgloss.Color(p.Orange),
		Yellow: lipgloss.Color(p.Yellow),
		Cyan:   lipgloss.Color(p.Cyan),
	}
}

// Load reads the @tn_* user-options from a running tmux server and
// returns a Palette. When tmux is unavailable or an option is unset
// the canonical defaults are used. Load never returns an error; a
// missing tmux server is silently ignored so the library remains
// usable in tests and stand-alone demos.
func Load() Palette {
	p := Fallback()
	entries := []struct {
		name string
		dest *lipgloss.Color
	}{
		{"bg", &p.Bg},
		{"fg", &p.Fg},
		{"accent", &p.Accent},
		{"dim", &p.Dim},
		{"border", &p.Border},
		{"purple", &p.Purple},
		{"green", &p.Green},
		{"red", &p.Red},
		{"orange", &p.Orange},
		{"yellow", &p.Yellow},
		{"cyan", &p.Cyan},
	}
	for _, e := range entries {
		if v := tmuxOption("@tn_" + e.name); v != "" {
			*e.dest = lipgloss.Color(v)
		}
	}
	return p
}

// Init configures the lipgloss default renderer for TrueColor output
// when running inside tmux. tmux sets TERM=screen-256color which causes
// termenv's auto-detection to downgrade to ANSI256; this override
// restores full 24-bit rendering.
//
// Init must be called once at program startup, before any lipgloss
// styles are rendered.
func Init() {
	if os.Getenv("TMUX") == "" {
		return
	}
	lipgloss.SetDefaultRenderer(
		lipgloss.NewRenderer(os.Stdout, termenv.WithProfile(termenv.TrueColor)),
	)
}

// tmuxOption reads a single tmux global option value.
// Returns "" if tmux is not running or the option is unset.
func tmuxOption(name string) string {
	out, err := exec.Command("tmux", "show-options", "-gqv", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
