package docrow_test

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components/docrow"
)

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var sb strings.Builder
	if err := c.Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

// TestItem_OneMeaningPerColumn pins the Katalog 3.10 row shape: a type marker
// in the row's own tone, a linked title, an origin/meta line that MAY carry
// reading time/effort (the caller composes Meta — docrow never invents it),
// and a right-hand date column that is always present, monospace,
// right-aligned — never mixed with anything else.
func TestItem_OneMeaningPerColumn(t *testing.T) {
	out := render(t, docrow.Item(docrow.Row{
		Href: "/wissen/x", Title: "Tourenansicht v2", TypeGlyph: "◆", TypeColor: "purple",
		Meta: "Plan · Straßenfuchs · 25 min Lesezeit", Date: "heute",
	}))
	for _, want := range []string{
		`href="/wissen/x"`, "Tourenansicht v2", "Plan · Straßenfuchs · 25 min Lesezeit",
		"heute", "tnum", "text-purple",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Item missing %q in: %s", want, out)
		}
	}
}

// TestItem_SelectedGetsAuswahlkante pins the 3px Auswahlkante + wash background
// (TOKENS.md Kartentypen: "3px-Kante links in der Typfarbe plus Wash als
// Zeilenhintergrund — nie als Rahmen, nie als Radius").
// The kante's width lives in the row's classes; its color and the wash come
// from the stylesheet keyed on data-tint + data-tint-on, so selection and
// hover paint identically from a single rule.
func TestItem_SelectedGetsAuswahlkante(t *testing.T) {
	sel := render(t, docrow.Item(docrow.Row{Title: "x", TypeColor: "teal", Date: "Fr", Selected: true}))
	if !strings.Contains(sel, "border-l-[3px]") || !strings.Contains(sel, `data-tint="teal"`) {
		t.Errorf("selected row missing the 3px Auswahlkante in its tone: %s", sel)
	}
	if !strings.Contains(sel, "data-tint-on") {
		t.Errorf("selected row missing the wash marker: %s", sel)
	}
	unsel := render(t, docrow.Item(docrow.Row{Title: "x", TypeColor: "teal", Date: "Fr"}))
	if strings.Contains(unsel, "data-tint-on") {
		t.Errorf("unselected row must not carry the wash marker: %s", unsel)
	}
}

// TestItem_CarriesTintForHover pins the Karteikasten Tint-Hover (Mockup CSS
// `[data-tint]:hover`): a row announces its card-type tone so hovering paints
// the type's wash plus the 3px Auswahlkante in the type's hue. The old neutral
// `hover:bg-sunken` is banned — on a panel surface --sunken IS --panel, so it
// rendered no visible hover at all.
func TestItem_CarriesTintForHover(t *testing.T) {
	for color, want := range map[string]string{
		"purple": `data-tint="purple"`,
		"teal":   `data-tint="teal"`,
		"":       `data-tint="accent"`, // no type → ocher accent, same as the Auswahlkante fallback
	} {
		out := render(t, docrow.Item(docrow.Row{Title: "x", TypeColor: color, Date: "Fr"}))
		if !strings.Contains(out, want) {
			t.Errorf("TypeColor %q: missing %s in: %s", color, want, out)
		}
		if strings.Contains(out, "hover:bg-sunken") {
			t.Errorf("TypeColor %q: row still carries the invisible neutral hover: %s", color, out)
		}
	}
}

// TestItem_PreviewRendersClamped pins that a Row's optional Preview snippet
// survives the docrow conversion (regression guard: wissenDocRow used to
// drop it entirely when first adopting docrow).
func TestItem_PreviewRendersClamped(t *testing.T) {
	out := render(t, docrow.Item(docrow.Row{Title: "x", Date: "Fr", Preview: "line one\nline two"}))
	if !strings.Contains(out, "preview-clamp") || !strings.Contains(out, "line one") {
		t.Errorf("Item missing clamped preview text: %s", out)
	}
}

// TestItem_NoHrefRendersPlainText pins that a Row with no Href (e.g. a
// non-clickable summary row) never emits a dead/empty <a href="">.
func TestItem_NoHrefRendersPlainText(t *testing.T) {
	out := render(t, docrow.Item(docrow.Row{Title: "Ohne Link", Date: "11.08."}))
	if strings.Contains(out, `href=""`) {
		t.Errorf("Row without Href must not render an empty href: %s", out)
	}
	if !strings.Contains(out, "Ohne Link") {
		t.Errorf("Item missing title: %s", out)
	}
}
