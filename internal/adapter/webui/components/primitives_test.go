package components_test

import (
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestButtonVariantsAndAttrs(t *testing.T) {
	out := render(t, components.Button(components.BtnDanger, "Löschen", "■",
		templ.Attributes{"hx-post": "/x/delete", "type": "submit"}))
	for _, w := range []string{"Löschen", "bg-red", `hx-post="/x/delete"`, `type="submit"`, "■"} {
		if !strings.Contains(out, w) {
			t.Errorf("Button missing %q: %s", w, out)
		}
	}
	prim := render(t, components.Button(components.BtnPrimary, "Speichern", "", nil))
	if strings.Contains(prim, "bg-red") {
		t.Errorf("primary button should not use danger color")
	}
}

func TestIconButtonRequiresAriaLabel(t *testing.T) {
	out := render(t, components.IconButton("✚", "Neu", templ.Attributes{"hx-get": "/new"}))
	for _, w := range []string{`aria-label="Neu"`, "✚", `hx-get="/new"`} {
		if !strings.Contains(out, w) {
			t.Errorf("IconButton missing %q", w)
		}
	}
}

func TestCardWrapsBody(t *testing.T) {
	out := render(t, components.Card("lg:col-span-2", templ.Raw(`<p id="cb">x</p>`)))
	if !strings.Contains(out, `id="cb"`) || !strings.Contains(out, "lg:col-span-2") || !strings.Contains(out, "glass") {
		t.Errorf("Card missing class or body: %s", out)
	}
}

func TestBadgeDocKinds(t *testing.T) {
	if out := render(t, components.Badge(components.KindProject)); !strings.Contains(out, "Projekt") || !strings.Contains(out, "text-green") {
		t.Errorf("project badge wrong: %s", out)
	}
	if out := render(t, components.Badge(components.KindDaily)); !strings.Contains(out, "Daily") {
		t.Errorf("daily badge wrong: %s", out)
	}
}

func TestChipAndTag(t *testing.T) {
	if out := render(t, components.Chip("flow", "teal")); !strings.Contains(out, "flow") || !strings.Contains(out, "text-teal") {
		t.Errorf("Chip wrong: %s", out)
	}
	if out := render(t, components.Tag("rebuild")); !strings.Contains(out, "rebuild") {
		t.Errorf("Tag wrong: %s", out)
	}
}

func TestStatTile(t *testing.T) {
	out := render(t, components.StatTile("nav.week", "32h 10m", "green"))
	for _, w := range []string{"Woche", "32h 10m", "text-green"} {
		if !strings.Contains(out, w) {
			t.Errorf("StatTile missing %q: %s", w, out)
		}
	}
}

func TestEmptyState(t *testing.T) {
	out := render(t, components.EmptyState("·", "empty.default", "empty.default"))
	if !strings.Contains(out, "Nichts vorhanden") {
		t.Errorf("EmptyState should render i18n title: %s", out)
	}
}
