package components_test

import (
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestSiteNavMarksActive(t *testing.T) {
	out := render(t, components.SiteNav("wissen"))
	for _, w := range []string{"Heute", "Wissen", "Projekte", "Stats", `href="/wissen"`, `aria-current="page"`} {
		if !strings.Contains(out, w) {
			t.Errorf("SiteNav missing %q", w)
		}
	}
}

func TestAppShellRendersSlotsAndChrome(t *testing.T) {
	bc := templ.Raw(`<nav id="bc">crumbs</nav>`)
	sn := templ.Raw(`<div id="sn">subnav</div>`)
	body := templ.Raw(`<section id="main">page</section>`)
	out := render(t, components.AppShell("today", bc, sn, body))
	for _, w := range []string{
		`id="bc"`, `id="sn"`, `id="main"`,
		`data-theme-toggle`,                 // mobile topbar carries the toggle
		`aria-label="Hauptnavigation"`,      // sidebar nav landmark (i18n nav.primary)
	} {
		if !strings.Contains(out, w) {
			t.Errorf("AppShell missing %q", w)
		}
	}
}

func TestAppShellNilSlotsAreSafe(t *testing.T) {
	out := render(t, components.AppShell("today", nil, nil, templ.Raw(`<p id="only">x</p>`)))
	if !strings.Contains(out, `id="only"`) {
		t.Errorf("AppShell with nil breadcrumb/subnav should still render content")
	}
}

func TestTabStripActive(t *testing.T) {
	tabs := []components.Tab{
		{Key: "today", Href: "/", LabelKey: "nav.today"},
		{Key: "week", Href: "/woche", LabelKey: "nav.week"},
	}
	out := render(t, components.TabStrip(tabs, "week"))
	if !strings.Contains(out, "Woche") || !strings.Contains(out, `aria-current="page"`) {
		t.Errorf("TabStrip should render labels and mark active: %s", out)
	}
}

func TestBreadcrumbLastIsCurrent(t *testing.T) {
	out := render(t, components.Breadcrumb([]components.Crumb{
		{Href: "/wissen", Label: "Wissen"},
		{Label: "Dokument"},
	}))
	if !strings.Contains(out, `href="/wissen"`) || !strings.Contains(out, "Dokument") {
		t.Errorf("Breadcrumb missing items: %s", out)
	}
	if !strings.Contains(out, `aria-current="page"`) {
		t.Errorf("Breadcrumb last item should be aria-current")
	}
}
