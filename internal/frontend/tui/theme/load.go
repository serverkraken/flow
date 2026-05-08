package theme

import (
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Load returns the canonical Default palette overlaid with the
// per-machine tmux user-options @tn_* (bg/fg/accent/dim/border/<hues>).
// Missing tmux server, missing options, or empty values silently fall
// back to the canonical defaults — Load never fails, so libraries and
// stand-alone tools can call it unconditionally.
//
// The tmux key surface mirrors the legacy 11-field shape historically
// used by components/theme: @tn_accent overrides the canonical
// Blue (= Sem.Accent), @tn_dim overrides FgMuted, @tn_border overrides
// BgCode, the hue keys (@tn_purple / @tn_green / …) override their
// like-named canonical hues 1:1.
func Load() Palette {
	p := Default
	overlay := []struct {
		key  string
		dest *lipgloss.Color
	}{
		{"@tn_bg", &p.Bg},
		{"@tn_fg", &p.Fg},
		{"@tn_accent", &p.Blue},
		{"@tn_dim", &p.FgMuted},
		{"@tn_border", &p.BgCode},
		{"@tn_purple", &p.Purple},
		{"@tn_green", &p.Green},
		{"@tn_red", &p.Red},
		{"@tn_orange", &p.Orange},
		{"@tn_yellow", &p.Yellow},
		{"@tn_cyan", &p.Cyan},
	}
	for _, o := range overlay {
		if v := tmuxOption(o.key); v != "" {
			*o.dest = lipgloss.Color(v)
		}
	}
	return p
}

// Init configures the lipgloss default renderer for TrueColor output
// when running inside tmux. tmux sets TERM=screen-256color which causes
// termenv's auto-detection to downgrade to ANSI256; this override
// restores full 24-bit rendering so the canonical hex colours render
// faithfully.
//
// Init must be called once at program startup, before any lipgloss
// styles are rendered. Outside tmux the function is a no-op — the
// terminal's own profile is already correct.
func Init() {
	if os.Getenv("TMUX") == "" {
		return
	}
	lipgloss.SetDefaultRenderer(
		lipgloss.NewRenderer(os.Stdout, termenv.WithProfile(termenv.TrueColor)),
	)
}

// tmuxOption reads a single tmux global user-option. Returns "" when
// tmux is not running, the option is unset, or the lookup fails.
func tmuxOption(name string) string {
	out, err := exec.Command("tmux", "show-options", "-gqv", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
