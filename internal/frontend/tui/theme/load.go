package theme

import (
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// tmuxKeys ist die 1:1-Liste der vom Adapter konsumierten tmux-Optionen.
// Aus der Liste leitet Load() den Overlay-Mapping ab; tmuxOption-Caching
// liest sie als Schlüsselmenge (siehe loadTmuxCache).
var tmuxKeys = []string{
	"@tn_bg", "@tn_fg", "@tn_accent", "@tn_dim", "@tn_border",
	"@tn_purple", "@tn_green", "@tn_red", "@tn_orange",
	"@tn_yellow", "@tn_cyan",
}

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
//
// Caching: Load liest tmux-Optionen einmal pro Prozess (siehe
// tmuxCacheOnce). Worktime-Status-Segment kann pro tmux-Refresh-Tick
// laufen; ohne Cache spawnten wir 11 `tmux show-options`-Subprozesse pro
// 5 s — gemessbar im perf-Profile.
func Load() Palette {
	p := Default
	cache := loadTmuxCache()
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
		if v := cache[o.key]; v != "" {
			*o.dest = lipgloss.Color(v)
		}
	}
	return p
}

// Init configures the lipgloss default renderer for TrueColor output
// when the terminal can render it. Two paths:
//
//  1. Inside tmux, TERM=screen-256color causes termenv to downgrade
//     auto-detected to ANSI256 even though tmux passes through 24-bit
//     colour from the host. Force TrueColor unconditionally.
//  2. Outside tmux, COLORTERM=truecolor (gesetzt von Ghostty / iTerm2 /
//     Alacritty / WezTerm / Kitty / Modern xterm) verlangt TrueColor —
//     vorher hat Init() die Detektion termenv überlassen, was bei nicht
//     korrekt-gesetztem TERM (z.B. xterm-256color) auf ANSI256 fiel.
//
// TMUX/COLORTERM are read here (and in loadTmuxCache below) rather
// than via main.go's Env: they describe the runtime terminal, not
// app config. See the A1 platform-detection carve-out in
// cmd/flow/main.go's Env doc.
//
// Init must be called once at program startup, before any lipgloss
// styles are rendered.
func Init() {
	switch {
	case os.Getenv("TMUX") != "":
	case strings.EqualFold(os.Getenv("COLORTERM"), "truecolor"):
	case strings.EqualFold(os.Getenv("COLORTERM"), "24bit"):
	default:
		return
	}
	lipgloss.SetDefaultRenderer(
		lipgloss.NewRenderer(os.Stdout, termenv.WithProfile(termenv.TrueColor)),
	)
}

var (
	tmuxCacheOnce sync.Once
	tmuxCacheMap  map[string]string
)

// loadTmuxCache liest alle tmux-Keys einmal pro Prozess in eine Map und
// gibt sie cached zurück. Vorher rief Load() pro Aufruf 11×
// `tmux show-options` als Subprozess (200-500 µs pro spawn auf macOS),
// und Load() wird jeden tmux-Refresh-Tick im Worktime-Status-Segment
// erneut aufgerufen — das war messbar.
//
// Trade-off: ein Live-Update der @tn_*-Variablen während eines
// laufenden flow-Prozesses greift nicht mehr. In der Praxis kein
// Verlust — die Variablen werden in tmux.conf gesetzt und beim Reload
// gibt es einen flow-Restart.
func loadTmuxCache() map[string]string {
	tmuxCacheOnce.Do(func() {
		tmuxCacheMap = make(map[string]string, len(tmuxKeys))
		if os.Getenv("TMUX") == "" {
			// Außerhalb tmux: keine Spawns versuchen.
			return
		}
		for _, k := range tmuxKeys {
			tmuxCacheMap[k] = readTmuxOption(k)
		}
	})
	return tmuxCacheMap
}

// readTmuxOption reads a single tmux global user-option. Returns "" when
// tmux is not running, the option is unset, or the lookup fails.
// Unbeschränkt aufrufbar — wird von loadTmuxCache nur einmal pro Key
// pro Prozess aufgerufen.
func readTmuxOption(name string) string {
	out, err := exec.Command("tmux", "show-options", "-gqv", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
