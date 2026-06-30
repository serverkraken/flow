package components_test

import (
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestPrimaryNavItems(t *testing.T) {
	items := components.PrimaryNav()
	keys := make([]string, len(items))
	for i, it := range items {
		keys[i] = it.Key
	}
	wantKeys := []string{"home", "docs", "projekte"}
	if len(items) != len(wantKeys) {
		t.Fatalf("PrimaryNav len=%d, want %d; keys=%v", len(items), len(wantKeys), keys)
	}
	for i, k := range wantKeys {
		if items[i].Key != k {
			t.Errorf("PrimaryNav[%d].Key=%q, want %q", i, items[i].Key, k)
		}
	}
	// Stats must NOT appear in primary nav.
	for _, it := range items {
		if it.Key == "stats" || it.LabelKey == "nav.stats" {
			t.Errorf("PrimaryNav must not contain stats item: %+v", it)
		}
	}
}

func TestSecondaryNavItems(t *testing.T) {
	items := components.SecondaryNav()
	keys := make([]string, len(items))
	for i, it := range items {
		keys[i] = it.Key
	}
	wantKeys := []string{"zeit", "frei", "export", "einstellungen"}
	if len(items) != len(wantKeys) {
		t.Fatalf("SecondaryNav len=%d, want %d; keys=%v", len(items), len(wantKeys), keys)
	}
	for i, k := range wantKeys {
		if items[i].Key != k {
			t.Errorf("SecondaryNav[%d].Key=%q, want %q", i, items[i].Key, k)
		}
	}
}

func TestSiteNavInjectsNavTreeContainer(t *testing.T) {
	out := render(t, components.SiteNav("projekte"))
	if !strings.Contains(out, `hx-get="/ui/nav/tree"`) {
		t.Errorf("SiteNav must inject htmx nav-tree container: %s", out)
	}
}

func TestSiteNavMarksActive(t *testing.T) {
	out := render(t, components.SiteNav("docs"))
	// nav items link to their REAL routes (/wissen, /nodes, /dayoffs), not the
	// German label-named paths — see fix for dead sidebar links.
	for _, w := range []string{"Home", "Wissen", "Projekte", `href="/wissen"`, `aria-current="page"`} {
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
