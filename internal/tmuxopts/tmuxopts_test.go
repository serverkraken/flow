package tmuxopts_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/statusline"
	"github.com/serverkraken/flow/internal/tmuxopts"
)

func TestRead_OutsideTmuxReturnsNil(t *testing.T) {
	t.Setenv("TMUX", "") // no tmux → no exec, defaults
	if got := tmuxopts.Read(); got != nil {
		t.Errorf("outside tmux Read() must return nil, got %v", got)
	}
}

func TestParseAndPalette(t *testing.T) {
	raw := "@tn_green \"#00ff00\"\n@tn_cyan #11aabb\n@flow_max_streak_min 90\nstatus on\n"
	opts := tmuxopts.Parse(raw)
	p := tmuxopts.Palette(opts)
	if p.Green != "#00ff00" || p.Cyan != "#11aabb" {
		t.Fatalf("override failed: %+v", p)
	}
	if p.Red != statusline.DefaultStatusPalette().Red {
		t.Error("unset slot should keep default")
	}
	if tmuxopts.MaxStreak(opts) != 90 {
		t.Errorf("max streak = %d", tmuxopts.MaxStreak(opts))
	}
	if tmuxopts.MaxStreak(map[string]string{}) != 0 {
		t.Error("missing max streak must be 0")
	}
}

func TestParse_IgnoresNonAtLinesAndBlankValues(t *testing.T) {
	opts := tmuxopts.Parse("set -g status on\n@no_value\n@flow_max_streak_min notanumber\n")
	if _, ok := opts["status"]; ok {
		t.Error("non-@ lines must be dropped")
	}
	if _, ok := opts["@no_value"]; ok {
		t.Error("@ line without a value must be dropped")
	}
	if tmuxopts.MaxStreak(opts) != 0 {
		t.Errorf("garbage max streak must be 0, got %d", tmuxopts.MaxStreak(opts))
	}
}

func TestMaxStreak_NegativeIsOff(t *testing.T) {
	if got := tmuxopts.MaxStreak(map[string]string{"@flow_max_streak_min": "-5"}); got != 0 {
		t.Errorf("negative max streak must be 0, got %d", got)
	}
}
