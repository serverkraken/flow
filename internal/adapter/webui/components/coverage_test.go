package components_test

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

// ── Tn (i18n plural helper) ─────────────────────────────────────────────────

func TestTnSingularAndPlural(t *testing.T) {
	ctx := context.Background()
	one := components.Tn(ctx, "list.entries", 1)
	if !strings.Contains(one, "1") || !strings.Contains(one, "Eintrag") {
		t.Errorf("Tn(1) = %q, want '1 Eintrag'", one)
	}
	many := components.Tn(ctx, "list.entries", 3)
	if !strings.Contains(many, "3") || !strings.Contains(many, "Einträge") {
		t.Errorf("Tn(3) = %q, want '3 Einträge'", many)
	}
}

// ── btnClass default branch ─────────────────────────────────────────────────

func TestButtonUnknownVariantFallsBack(t *testing.T) {
	out := render(t, components.Button(components.ButtonVariant("bogus"), "Fallback", "", nil))
	if !strings.Contains(out, "Fallback") || !strings.Contains(out, "<button") {
		t.Errorf("Button with unknown variant should still render: %s", out)
	}
}

// ── docBadge default + missing variants ────────────────────────────────────

func TestBadgeDefaultBranch(t *testing.T) {
	out := render(t, components.Badge(components.DocKind("bogus")))
	if !strings.Contains(out, "bg-sunken") {
		t.Errorf("Badge with unknown kind should use default style: %s", out)
	}
}

func TestBadgeFreeAndAgentKinds(t *testing.T) {
	free := render(t, components.Badge(components.KindFree))
	if !strings.Contains(free, "text-purple") {
		t.Errorf("KindFree badge should be purple: %s", free)
	}
	agent := render(t, components.Badge(components.KindAgent))
	if !strings.Contains(agent, "text-yellow") {
		t.Errorf("KindAgent badge should be yellow: %s", agent)
	}
}

// ── hueText default branch (Chip unknown hue) ───────────────────────────────

func TestChipUnknownHueFallsBack(t *testing.T) {
	out := render(t, components.Chip("mytag", "notacolor"))
	if !strings.Contains(out, "text-body") || !strings.Contains(out, "bg-sunken") {
		t.Errorf("Chip with unknown hue should use neutral classes: %s", out)
	}
	if !strings.Contains(out, "mytag") {
		t.Errorf("Chip should still render label: %s", out)
	}
}

// ── valueHue default + empty branches (StatTile) ───────────────────────────

func TestStatTileEmptyHue(t *testing.T) {
	out := render(t, components.StatTile("nav.today", "5h", ""))
	if !strings.Contains(out, "text-ink") {
		t.Errorf("StatTile with empty hue should use text-ink: %s", out)
	}
}

func TestStatTileUnknownHue(t *testing.T) {
	out := render(t, components.StatTile("nav.today", "5h", "notacolor"))
	if !strings.Contains(out, "text-ink") {
		t.Errorf("StatTile with unknown hue should use text-ink: %s", out)
	}
}

// ── PageNav.Pages() edge cases ──────────────────────────────────────────────

func TestPageNavPagesZeroPageSize(t *testing.T) {
	p := components.PageNav{Page: 1, Total: 50, PageSize: 0}
	if got := p.Pages(); got != 1 {
		t.Errorf("Pages() with PageSize=0 = %d, want 1", got)
	}
}

func TestPageNavPagesZeroTotal(t *testing.T) {
	p := components.PageNav{Page: 1, Total: 0, PageSize: 10}
	if got := p.Pages(); got != 1 {
		t.Errorf("Pages() with Total=0 = %d, want 1", got)
	}
}

// ── Pagination edge renders ─────────────────────────────────────────────────

func TestPaginationFirstPageDisabledZuruck(t *testing.T) {
	out := render(t, components.Pagination(components.PageNav{Page: 1, Total: 30, PageSize: 10, BaseHref: "/x"}))
	if !strings.Contains(out, "Zurück") {
		t.Errorf("Pagination first page missing Zurück: %s", out)
	}
	// HasPrev=false → disabled span, not an anchor
	if !strings.Contains(out, `aria-disabled="true"`) {
		t.Errorf("Pagination first page should disable Zurück: %s", out)
	}
}

func TestPaginationLastPageDisabledWeiter(t *testing.T) {
	out := render(t, components.Pagination(components.PageNav{Page: 3, Total: 30, PageSize: 10, BaseHref: "/x"}))
	if !strings.Contains(out, "Weiter") {
		t.Errorf("Pagination last page missing Weiter: %s", out)
	}
	if strings.Contains(out, "Mehr laden") {
		t.Errorf("Pagination last page must not show 'Mehr laden': %s", out)
	}
}

// ── AppShell slot combinations ──────────────────────────────────────────────

func TestAppShellSubnavNilBreadcrumbSet(t *testing.T) {
	bc := templ.Raw(`<nav id="bc-only">crumbs</nav>`)
	out := render(t, components.AppShell("today", bc, nil, templ.Raw(`<p id="body">c</p>`)))
	if !strings.Contains(out, `id="bc-only"`) {
		t.Errorf("AppShell: breadcrumb should render when subnav is nil: %s", out)
	}
	if !strings.Contains(out, `id="body"`) {
		t.Errorf("AppShell: content should render: %s", out)
	}
}

func TestAppShellBreadcrumbNilSubnavSet(t *testing.T) {
	sn := templ.Raw(`<div id="sn-only">subnav</div>`)
	out := render(t, components.AppShell("today", nil, sn, templ.Raw(`<p id="body2">c</p>`)))
	if !strings.Contains(out, `id="sn-only"`) {
		t.Errorf("AppShell: subnav should render when breadcrumb is nil: %s", out)
	}
	if !strings.Contains(out, `id="body2"`) {
		t.Errorf("AppShell: content should render: %s", out)
	}
}

// ── AppShell topbar: non-primary active key and empty active ───────────────
// (Lesesaal L1 Task 4: SiteNav/sidebar died; AreaFor(active) now carries the
// same "which area is this page under" wayfinding logic the old SiteNav did.)

func TestAppShellNonPrimaryActiveKeyMarksArea(t *testing.T) {
	// "frei" is a utility-menu destination, not a PrimaryNav key — but it
	// belongs to the Zeit area, so the Zeit topbar link must get aria-current.
	out := render(t, components.AppShell("frei", nil, nil, components.Empty()))
	if !strings.Contains(out, "Zeit") {
		t.Errorf("AppShell should still render all primary areas: %s", out)
	}
	if !strings.Contains(out, `aria-current="page"`) {
		t.Errorf("AppShell with active 'frei' must mark the Zeit area as aria-current: %s", out)
	}
	// Exactly one aria-current in the primary nav landmark: the Zeit item only.
	// (The mobile-nav dialog — Burger-Fix 2026-07-05 — mirrors the same
	// aria-current on its own copy of the link, so the global count is 2.)
	topbarNav := out[strings.Index(out, `class="topbar-nav`):strings.Index(out, "</nav>")]
	if n := strings.Count(topbarNav, `aria-current="page"`); n != 1 {
		t.Errorf("AppShell(frei) must have exactly 1 aria-current=page in topbar-nav, got %d: %s", n, topbarNav)
	}
}

func TestAppShellEmptyActiveKeyMarksNoArea(t *testing.T) {
	out := render(t, components.AppShell("", nil, nil, components.Empty()))
	if !strings.Contains(out, "Projekte") || !strings.Contains(out, "Wissen") {
		t.Errorf("AppShell with empty active should render all primary areas: %s", out)
	}
	if strings.Contains(out, `aria-current="page"`) {
		t.Errorf("AppShell with empty active must not mark any area current: %s", out)
	}
}

// ── Breadcrumb: single-item (current only) and multi-item ──────────────────

func TestBreadcrumbSingleItem(t *testing.T) {
	out := render(t, components.Breadcrumb([]components.Crumb{
		{Label: "Heute"},
	}))
	if !strings.Contains(out, "Heute") {
		t.Errorf("Breadcrumb single item should render label: %s", out)
	}
	if !strings.Contains(out, `aria-current="page"`) {
		t.Errorf("Breadcrumb single item should be aria-current: %s", out)
	}
}

func TestBreadcrumbMultiItem(t *testing.T) {
	out := render(t, components.Breadcrumb([]components.Crumb{
		{Href: "/wissen", Label: "Wissen"},
		{Label: "Notiz"},
	}))
	if !strings.Contains(out, `href="/wissen"`) {
		t.Errorf("Breadcrumb multi-item: first item should be linked: %s", out)
	}
	if !strings.Contains(out, "Notiz") {
		t.Errorf("Breadcrumb multi-item: last item should render: %s", out)
	}
	if !strings.Contains(out, `aria-current="page"`) {
		t.Errorf("Breadcrumb multi-item: last should be aria-current: %s", out)
	}
}

// ── ConfirmDialog: non-default keys ────────────────────────────────────────

func TestConfirmDialogCustomKeys(t *testing.T) {
	out := render(t, components.ConfirmDialog(components.ConfirmSpec{
		ID:              "customDlg",
		TitleKey:        "confirm.title",
		BodyKey:         "confirm.deleteBody",
		ConfirmLabelKey: "common.delete",
		ConfirmAttrs:    templ.Attributes{"hx-post": "/items/x/delete"},
	}))
	// All three keys are set — withDefaults() won't replace any of them.
	for _, w := range []string{
		"Bist du sicher?",
		"kann nicht rückgängig",
		"Löschen",
		`hx-post="/items/x/delete"`,
	} {
		if !strings.Contains(out, w) {
			t.Errorf("ConfirmDialog(custom keys) missing %q: %s", w, out)
		}
	}
}
