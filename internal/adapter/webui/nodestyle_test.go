package webui_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// Drift guard: every domain palette color MUST map to a token-reactive CSS
// color expression, else a node could carry a color the WebUI renders blank —
// and inline hexes would not flip with the theme.
func TestColorHexCoversWholePalette(t *testing.T) {
	for _, name := range domain.NodeColors {
		got := webui.ColorHex(name)
		want := "rgb(var(--" + name + "))"
		if got != want {
			t.Errorf("color %q → %q, want %q", name, got, want)
		}
	}
	if webui.ColorHex("") != "" {
		t.Error("empty color → empty expression")
	}
	if webui.ColorHex("chartreuse") != "" {
		t.Error("unknown color → empty expression (not a guess)")
	}
}

func TestStatusBadge(t *testing.T) {
	for _, st := range []domain.NodeStatus{domain.NodeActive, domain.NodePaused, domain.NodeArchived} {
		label, classes := webui.StatusBadge(st)
		if label == "" || classes == "" {
			t.Errorf("status %q → empty label/classes", st)
		}
	}
	if l, _ := webui.StatusBadge(domain.NodeActive); l != "aktiv" {
		t.Errorf("active label = %q, want aktiv", l)
	}
	if l, _ := webui.StatusBadge(domain.NodePaused); l != "pausiert" {
		t.Errorf("paused label = %q, want pausiert", l)
	}
	if l, _ := webui.StatusBadge(domain.NodeArchived); l != "archiviert" {
		t.Errorf("archived label = %q, want archiviert", l)
	}
}
