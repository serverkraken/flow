package webui_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui"
)

func TestLogoShape(t *testing.T) {
	cases := []struct {
		w, h int
		want string
	}{
		{100, 100, "hex"}, {120, 100, "hex"}, {80, 100, "hex"},
		{300, 100, "tile"}, {100, 300, "tile"}, {126, 100, "tile"},
		{0, 0, "hex"}, // Bestandslogos ohne Maße: bisheriges Hex-Verhalten
	}
	for _, c := range cases {
		if got := webui.LogoShape(c.w, c.h); got != c.want {
			t.Errorf("LogoShape(%d,%d)=%q want %q", c.w, c.h, got, c.want)
		}
	}
}
