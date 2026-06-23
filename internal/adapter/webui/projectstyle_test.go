package webui_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// Drift guard: every domain palette color MUST have a WebUI hex, else a project
// could carry a color the WebUI renders as blank.
func TestColorHexCoversWholePalette(t *testing.T) {
	for _, name := range domain.ProjectColors {
		hex := webui.ColorHex(name)
		if !strings.HasPrefix(hex, "#") || len(hex) != 7 {
			t.Errorf("color %q → %q, want a #rrggbb hex", name, hex)
		}
	}
	if webui.ColorHex("") != "" {
		t.Error("empty color → empty hex")
	}
	if webui.ColorHex("chartreuse") != "" {
		t.Error("unknown color → empty hex (not a guess)")
	}
}

func TestStatusBadge(t *testing.T) {
	for _, st := range []domain.ProjectStatus{domain.ProjectActive, domain.ProjectPaused, domain.ProjectArchived} {
		label, classes := webui.StatusBadge(st)
		if label == "" || classes == "" {
			t.Errorf("status %q → empty label/classes", st)
		}
	}
	if l, _ := webui.StatusBadge(domain.ProjectActive); l != "aktiv" {
		t.Errorf("active label = %q, want aktiv", l)
	}
	if l, _ := webui.StatusBadge(domain.ProjectPaused); l != "pausiert" {
		t.Errorf("paused label = %q, want pausiert", l)
	}
	if l, _ := webui.StatusBadge(domain.ProjectArchived); l != "archiviert" {
		t.Errorf("archived label = %q, want archiviert", l)
	}
}
