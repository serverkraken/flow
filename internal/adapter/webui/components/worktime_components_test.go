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
	for _, w := range []string{"top:120px", "height:72px", "block-unassigned", "data-session-id=\"b1\""} {
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

func TestKennzahlenPanel(t *testing.T) {
	out := render(t, components.KennzahlenPanel(components.KennzahlenVM{
		AvgPerDay: "7h 04m", GoalsHit: 4, GoalsTotal: 5, Balance: "+2h 18m", BalancePos: true,
		Dots: []components.PaceDot{{State: "ahead"}}, OnTrack: true,
	}))
	for _, w := range []string{"7h 04m", "4", "+2h 18m"} {
		if !strings.Contains(out, w) {
			t.Errorf("Kennzahlen missing %q", w)
		}
	}
}

func TestWeekTotalBanner(t *testing.T) {
	out := render(t, components.WeekTotalBanner(components.WeekTotalVM{Total: "33h 41m", Target: "40h 00m", Pct: 84, Variant: "under"}))
	if !strings.Contains(out, "33h 41m") || !strings.Contains(out, "40h 00m") {
		t.Errorf("WeekTotalBanner missing totals")
	}
}

func TestProjectFuzzyPicker_InlineCreate(t *testing.T) {
	out := render(t, components.ProjectFuzzyPicker(components.FuzzyPickerVM{
		ID: "pick", FormID: "bulkForm",
		Projects: []components.FuzzyProjectVM{{ID: "p1", Name: "flow", Hue: "blue", Glyph: "◆", Rate: "95 €/h"}},
	}))
	for _, w := range []string{"role=\"listbox\"", "flow", "data-new-project", "✚"} {
		if !strings.Contains(out, w) {
			t.Errorf("FuzzyPicker missing %q", w)
		}
	}
}

func TestSelectionActionBar(t *testing.T) {
	out := render(t, components.SelectionActionBar(components.SelectionBarVM{
		AssignURL: "/ui/historie/reassign", DeleteURL: "/ui/historie/bulk-delete",
		Picker: components.FuzzyPickerVM{ID: "pick", FormID: "bulkForm"},
	}))
	for _, w := range []string{"data-sel-count", "/ui/historie/reassign", "/ui/historie/bulk-delete"} {
		if !strings.Contains(out, w) {
			t.Errorf("SelectionActionBar missing %q", w)
		}
	}
}

func TestSegToggle(t *testing.T) {
	out := render(t, components.SegToggle([]components.SegOption{
		{Key: "cal", LabelKey: "historie.calendar", Href: "/historie?view=cal"},
		{Key: "list", LabelKey: "historie.list", Href: "/historie?view=list"},
	}, "cal"))
	if !strings.Contains(out, "aria-pressed=\"true\"") {
		t.Errorf("SegToggle missing active aria-pressed")
	}
}
