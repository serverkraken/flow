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
		`aria-label="Hauptnavigation"`, // Navigations-Landmark, seit Slice 2 an der Schiene (i18n nav.primary)
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
	// Seit der Karteikasten-Hülle (Slice 2) trägt nicht mehr ein Topbar-Link
	// das aria-current, sondern der Ebenenstreifen den Ton des Bereichs — und
	// die Schiene markiert die Zeile serverseitig im /ui/nav/tree-Fragment.
	// Geprüft wird deshalb hier: der Streifen nennt den Bereich, und die
	// Schiene fragt ihr Fragment für genau diesen Bereich an.
	out := render(t, components.AppShell("zeit", nil, nil, templ.Raw(`<p id="c">x</p>`)))
	if !strings.Contains(out, "bg-live") {
		t.Errorf("AppShell(zeit) braucht den Zeit-Ebenenstreifen: %s", out)
	}
	if !strings.Contains(out, "/ui/nav/tree?active=zeit") {
		t.Errorf("die Schiene muss ihr Fragment für den aktiven Bereich laden: %s", out)
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

// Slice 2 kehrt eine frühere Entscheidung ausdrücklich um: unter "Lesesaal L1"
// war die Sidebar abgeschafft und die Topbar das Navigationsmodell. Das
// Karteikasten-Konzept (Kapitel 02) macht die 264px-Schiene wieder zum GANZEN
// Navigationsmodell — "es gibt keine zweite Hierarchie". Der Test hieß deshalb
// einmal TopbarNoSidebar und prüft jetzt das Gegenteil; das ist gewollt und
// keine Regression.
func TestAppShell_RailNoTopbarNav(t *testing.T) {
	var sb strings.Builder
	if err := components.AppShell("heute", nil, nil, components.Empty()).Render(testCtx(t), &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.Contains(out, `id="app-rail"`) {
		t.Fatal("die Schiene muss da sein")
	}
	if strings.Contains(out, "topbar-nav") {
		t.Fatal("die Topbar-Navigation muss weg sein")
	}
	// Was die Schiene weiterhin trägt: Uhr, Suche, Baum-Fragment, Werkzeuge.
	for _, want := range []string{`id="timer-pill"`, "data-palette-open", "/ui/nav/tree?active=heute", `href="/dayoffs"`, `href="/einstellungen"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("Schiene vermisst %q:\n%s", want, out)
		}
	}
	// Unter 768px ist die Schiene ausgeblendet — ohne die schmale Kopfzeile
	// käme man dort nirgendwo hin.
	for _, want := range []string{"md:hidden", `data-dialog-open="mobile-nav"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("mobiler Einstieg vermisst %q", want)
		}
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
