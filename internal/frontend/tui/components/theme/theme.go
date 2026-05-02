// Package theme provides the Tokyonight Storm color palette and lipgloss renderer
// initialisation for the tui-kit component library.
package theme

import (
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Palette holds the full Tokyonight Storm color set, mirroring the @tn_* user-options
// defined in .tmux.conf.
type Palette struct {
	Bg, Fg, Accent, Dim, Border              lipgloss.Color
	Purple, Green, Red, Orange, Yellow, Cyan lipgloss.Color
}

// fallback is the canonical Tokyonight Storm palette, identical to the @tn_* values
// hard-coded in .tmux.conf. Used when tmux is unavailable or an option is unset.
var fallback = Palette{
	Bg:     "#24283b",
	Fg:     "#c0caf5",
	Accent: "#7aa2f7",
	Dim:    "#565f89",
	Border: "#414868",
	Purple: "#bb9af7",
	Green:  "#9ece6a",
	Red:    "#f7768e",
	Orange: "#ff9e64",
	Yellow: "#e0af68",
	Cyan:   "#7dcfff",
}

// Load reads the @tn_* user-options from a running tmux server and returns a Palette.
// When tmux is unavailable or an option is unset the Tokyonight Storm hex fallbacks are
// used. Load never returns an error; a missing tmux server is silently ignored so the
// library remains usable in tests and stand-alone demos.
func Load() Palette {
	p := fallback
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

// Init configures the lipgloss default renderer for TrueColor output when running inside
// tmux. tmux sets TERM=screen-256color which causes termenv's auto-detection to downgrade
// to ANSI256; this override restores full 24-bit rendering.
//
// Init must be called once at program startup, before any lipgloss styles are rendered.
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
