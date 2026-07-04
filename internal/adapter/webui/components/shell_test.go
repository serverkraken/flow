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
	wantKeys := []string{"projekte", "docs", "zeit"}
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

func TestUtilityNavItems(t *testing.T) {
	// Lesesaal L1 Task 4: the sidebar's SecondaryNav died with the sidebar;
	// its destinations now live in the avatar-menu UtilityNav (Zeit moved to
	// PrimaryNav, so it's no longer a utility-menu item).
	items := components.UtilityNav()
	keys := make([]string, len(items))
	for i, it := range items {
		keys[i] = it.Key
	}
	wantKeys := []string{"frei", "export", "einstellungen"}
	if len(items) != len(wantKeys) {
		t.Fatalf("UtilityNav len=%d, want %d; keys=%v", len(items), len(wantKeys), keys)
	}
	for i, k := range wantKeys {
		if items[i].Key != k {
			t.Errorf("UtilityNav[%d].Key=%q, want %q", i, items[i].Key, k)
		}
	}
}

func TestAreaFor(t *testing.T) {
	cases := []struct{ active, want string }{
		{"projekte", "projekte"},
		{"docs", "docs"},
		{"zeit", "zeit"},
		{"heute", "zeit"},
		{"woche", "zeit"},
		{"historie", "zeit"},
		{"stats", "zeit"},
		{"frei", "zeit"},
		{"export", "zeit"},
		{"", ""},
		{"einstellungen", ""},
	}
	for _, c := range cases {
		if got := components.AreaFor(c.active); got != c.want {
			t.Errorf("AreaFor(%q) = %q, want %q", c.active, got, c.want)
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
		`aria-label="Hauptnavigation"`, // topbar primary-nav landmark (i18n nav.primary)
	} {
		if !strings.Contains(out, w) {
			t.Errorf("AppShell missing %q", w)
		}
	}
	// Lesesaal L1: theme is fixed light, the toggle is gone (Dunkel-Zwilling = L7).
	if strings.Contains(out, "data-theme-toggle") {
		t.Errorf("AppShell must not carry the removed theme toggle")
	}
}

func TestAppShellUserMenu(t *testing.T) {
	// Lesesaal L1 Task 4: the mobile bottom-tab "More" drawer died with the
	// sidebar; its utility destinations now live behind the avatar-menu dialog.
	out := render(t, components.AppShell("home", nil, nil, templ.Raw(`<p id="c">x</p>`)))
	if !strings.Contains(out, `data-dialog-open="user-menu"`) {
		t.Errorf("AppShell missing avatar-menu trigger: %s", out)
	}
	if !strings.Contains(out, `id="user-menu"`) {
		t.Errorf("AppShell missing user-menu dialog: %s", out)
	}
	// All UtilityNav destinations reachable from the avatar menu.
	for _, want := range []string{`href="/dayoffs"`, `href="/export"`, `href="/einstellungen"`} {
		if !strings.Contains(out, want) {
			t.Errorf("AppShell user-menu missing link %q: %s", want, out)
		}
	}
	// dialog.js loaded for the trigger to work.
	if !strings.Contains(out, `dialog.js`) {
		t.Errorf("AppShell user-menu missing dialog.js script: %s", out)
	}
	// Logout is a full-page POST, must NOT be hx-boosted.
	if !strings.Contains(out, `action="/auth/logout"`) || !strings.Contains(out, `hx-boost="false"`) {
		t.Errorf("AppShell logout form must post with hx-boost=false: %s", out)
	}
}

func TestAppShellNilSlotsAreSafe(t *testing.T) {
	out := render(t, components.AppShell("today", nil, nil, templ.Raw(`<p id="only">x</p>`)))
	if !strings.Contains(out, `id="only"`) {
		t.Errorf("AppShell with nil breadcrumb/subnav should still render content")
	}
}

func TestAppShellPrimaryAreaActive(t *testing.T) {
	// "zeit" is now a PrimaryNav key (topbar area), not a drawer item — its
	// topbar link must carry aria-current, and it must be the only one.
	out := render(t, components.AppShell("zeit", nil, nil, templ.Raw(`<p id="c">x</p>`)))
	if !strings.Contains(out, `aria-current="page"`) {
		t.Errorf("AppShell(zeit) must emit aria-current=page for the Zeit topbar link: %s", out)
	}
	if n := strings.Count(out, `aria-current="page"`); n != 1 {
		t.Errorf("AppShell(zeit) must have exactly 1 aria-current=page, got %d: %s", n, out)
	}
	if !strings.Contains(out, `aria-current="page" href="/zeit"`) {
		t.Errorf("AppShell(zeit) aria-current must sit on the /zeit topbar link: %s", out)
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

func TestAppShell_TopbarNoSidebar(t *testing.T) {
	var sb strings.Builder
	err := components.AppShell("heute", nil, nil, components.Empty()).Render(testCtx(t), &sb)
	if err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if strings.Contains(out, "<aside") {
		t.Fatal("sidebar <aside> must be gone")
	}
	for _, want := range []string{`id="timer-pill"`, `href="/nodes"`, `href="/wissen"`, `href="/zeit"`, "data-palette-open"} {
		if !strings.Contains(out, want) {
			t.Fatalf("topbar missing %q:\n%s", want, out)
		}
	}
	// active "heute" gehört zum Bereich Zeit
	if !strings.Contains(out, `aria-current="page" href="/zeit"`) && !strings.Contains(out, `href="/zeit" aria-current="page"`) {
		t.Fatal("Zeit area not marked current for active=heute")
	}
	if strings.Contains(out, "/ui/nav/tree") {
		t.Fatal("nav tree mount must be gone")
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
