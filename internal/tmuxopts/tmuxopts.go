// Package tmuxopts reads flow's tmux status options in ONE `tmux show-options -g`
// call and maps them onto the statusline palette + active-session threshold.
package tmuxopts

import (
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/serverkraken/flow/internal/statusline"
)

// Read runs `tmux show-options -g` once. Outside tmux ($TMUX empty) or on any
// error it returns nil → callers fall back to defaults, never an error.
func Read() map[string]string {
	if os.Getenv("TMUX") == "" {
		return nil
	}
	out, err := exec.Command("tmux", "show-options", "-g").Output()
	if err != nil {
		return nil
	}
	return Parse(string(out))
}

// Parse turns show-options output ("@key value" / `@key "value"` per line) into
// a map. Only @-prefixed user options are kept.
func Parse(raw string) map[string]string {
	opts := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "@") {
			continue
		}
		key, val, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		opts[key] = strings.Trim(strings.TrimSpace(val), `"`)
	}
	return opts
}

// Palette layers @tn_* overrides on top of the tokyonight defaults.
func Palette(opts map[string]string) statusline.StatusPalette {
	def := statusline.DefaultStatusPalette()
	pick := func(key, fallback string) string {
		if v := opts[key]; v != "" {
			return v
		}
		return fallback
	}
	return statusline.StatusPalette{
		Green:  pick("@tn_green", def.Green),
		Yellow: pick("@tn_yellow", def.Yellow),
		Red:    pick("@tn_red", def.Red),
		Cyan:   pick("@tn_cyan", def.Cyan),
		Blue:   pick("@tn_blue", def.Blue),
		Purple: pick("@tn_purple", def.Purple),
		Orange: pick("@tn_orange", def.Orange),
		Dim:    pick("@tn_dim", def.Dim),
	}
}

// MaxStreak reads @flow_max_streak_min (0/missing/garbage → 0 = warning off).
func MaxStreak(opts map[string]string) int {
	n, err := strconv.Atoi(opts["@flow_max_streak_min"])
	if err != nil || n < 0 {
		return 0
	}
	return n
}
