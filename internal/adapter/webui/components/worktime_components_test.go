package components_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestProgressBarVariants(t *testing.T) {
	for _, v := range []string{"hit", "over", "under", "running"} {
		out := render(t, components.ProgressBar(60, v))
		if !strings.Contains(out, "role=\"progressbar\"") {
			t.Errorf("%s: missing role=progressbar", v)
		}
		if !strings.Contains(out, "width:60%") {
			t.Errorf("%s: missing width style", v)
		}
	}
}

func TestPaceDots(t *testing.T) {
	out := render(t, components.PaceDots([]components.PaceDot{{State: "ahead"}, {State: "behind"}, {State: "off"}}))
	for _, w := range []string{"●", "○"} { // ahead/behind dots filled, off hollow
		if !strings.Contains(out, w) {
			t.Errorf("PaceDots missing %q", w)
		}
	}
}

func TestSessionRow_Unassigned(t *testing.T) {
	out := render(t, components.SessionRow(components.SessionRowVM{
		ID: "s1", Title: "ohne Projekt", Glyph: "○", TimeRange: "10:15–11:05",
		Duration: "50m", Unassigned: true, Selectable: true,
	}))
	for _, w := range []string{"10:15–11:05", "50m", "type=\"checkbox\"", "border-dashed"} {
		if !strings.Contains(out, w) {
			t.Errorf("SessionRow missing %q", w)
		}
	}
}

func TestSessionBlock_PositionAndUnassigned(t *testing.T) {
	out := render(t, components.SessionBlock(components.SessionBlockVM{
		ID: "b1", TopPx: 120, HeightPx: 72, Title: "ohne Projekt", Glyph: "○",
		TimeRange: "10:15–11:05", Unassigned: true, Size: "sm",
	}))
	for _, w := range []string{"top:120px", "height:72px", "wtblock-unassigned", "data-session-id=\"b1\""} {
		if !strings.Contains(out, w) {
			t.Errorf("SessionBlock missing %q", w)
		}
	}
	// Unassigned blocks must NOT inject a --c custom property.
	if strings.Contains(out, "--c:") {
		t.Errorf("SessionBlock unassigned: must not emit --c custom property, got: %s", out)
	}
}

func TestSessionBlock_KnownHueEmitsCSSVar(t *testing.T) {
	out := render(t, components.SessionBlock(components.SessionBlockVM{
		ID: "b2", TopPx: 60, HeightPx: 90, Title: "flow", Glyph: "◆",
		TimeRange: "09:00–10:30", Hue: "blue", Size: "md",
	}))
	if !strings.Contains(out, "--c:var(--blue)") {
		t.Errorf("SessionBlock blue hue: expected --c:var(--blue), got: %s", out)
	}
	// Unknown hue must not inject anything.
	out2 := render(t, components.SessionBlock(components.SessionBlockVM{
		ID: "b3", TopPx: 0, HeightPx: 30, Title: "x", Glyph: "·",
		TimeRange: "08:00–08:30", Hue: "unknown-color",
	}))
	if strings.Contains(out2, "--c:") {
		t.Errorf("SessionBlock unknown hue: must not emit --c custom property, got: %s", out2)
	}
}

func TestProjectFuzzyPicker_InlineCreate(t *testing.T) {
	out := render(t, components.ProjectFuzzyPicker(components.NodePickerVM{
		ID: "pick", FormID: "bulkForm",
		Nodes: []components.NodePickerItem{{ID: "p1", Name: "flow", Hue: "blue", Glyph: "◆", Rate: "95 €/h"}},
	}))
	for _, w := range []string{"role=\"listbox\"", "flow", "data-new-project", "✚"} {
		if !strings.Contains(out, w) {
			t.Errorf("FuzzyPicker missing %q", w)
		}
	}
}
