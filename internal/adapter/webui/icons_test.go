package webui_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// Drift guard: every whitelisted icon key MUST have a vendored SVG (else the
// form offers an icon that renders blank) and the SVG must be render-ready.
func TestNodeIconSVGCoversWholeWhitelist(t *testing.T) {
	for _, key := range domain.NodeIcons {
		svg := webui.NodeIconSVG(key)
		if !strings.Contains(svg, "<svg") {
			t.Errorf("icon %q → no SVG markup", key)
			continue
		}
		if !strings.Contains(svg, `width="100%"`) || !strings.Contains(svg, `height="100%"`) {
			t.Errorf("icon %q → fixed dimensions not rewritten to 100%%", key)
		}
		if !strings.Contains(svg, `stroke="currentColor"`) {
			t.Errorf("icon %q → not currentColor-tintable", key)
		}
	}
	if webui.NodeIconSVG("") != "" {
		t.Error("empty key → empty SVG")
	}
	if webui.NodeIconSVG("skull") != "" {
		t.Error("unknown key → empty SVG (not a guess)")
	}
}

// Reverse drift guard: no orphan assets beyond the whitelist.
func TestNodeIconAssetsMatchWhitelistCount(t *testing.T) {
	if got, want := webui.NodeIconCount(), len(domain.NodeIcons); got != want {
		t.Errorf("embedded icons = %d, whitelist = %d — keep them identical", got, want)
	}
}
