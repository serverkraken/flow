package components_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestBurndownBanner_OnTrack(t *testing.T) {
	out := render(t, components.BurndownBanner(components.BurndownVM{
		Total: "78h 00m", Target: "160h 00m", Pct: 48, PacePct: 47, Variant: "hit", OnTrack: true,
	}))
	for _, w := range []string{"78h 00m", "160h 00m", "role=\"progressbar\"", "left:47%", "▲"} {
		if !strings.Contains(out, w) {
			t.Errorf("BurndownBanner(on-track) missing %q\n%s", w, out)
		}
	}
}

func TestBurndownBanner_Behind(t *testing.T) {
	out := render(t, components.BurndownBanner(components.BurndownVM{
		Total: "40h 00m", Target: "160h 00m", Pct: 25, PacePct: 60, Variant: "under", OnTrack: false,
	}))
	for _, w := range []string{"left:60%", "▼"} {
		if !strings.Contains(out, w) {
			t.Errorf("BurndownBanner(behind) missing %q\n%s", w, out)
		}
	}
}
