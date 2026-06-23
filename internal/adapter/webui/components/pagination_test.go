package components_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestPageNavMath(t *testing.T) {
	p := components.PageNav{Page: 2, Total: 95, PageSize: 20, BaseHref: "/wissen"}
	if p.Pages() != 5 {
		t.Errorf("Pages() = %d, want 5", p.Pages())
	}
	if !p.HasPrev() || !p.HasNext() {
		t.Errorf("page 2 of 5 should have both prev and next")
	}
	if p.PrevHref() != "/wissen?page=1" {
		t.Errorf("PrevHref = %q", p.PrevHref())
	}
	if p.NextHref() != "/wissen?page=3" {
		t.Errorf("NextHref = %q", p.NextHref())
	}
}

func TestPageNavHrefWithExistingQuery(t *testing.T) {
	p := components.PageNav{Page: 1, Total: 10, PageSize: 5, BaseHref: "/wissen?tag=go"}
	if p.NextHref() != "/wissen?tag=go&page=2" {
		t.Errorf("NextHref with query = %q", p.NextHref())
	}
}

func TestPaginationDisabledAtEdges(t *testing.T) {
	first := render(t, components.Pagination(components.PageNav{Page: 1, Total: 30, PageSize: 10, BaseHref: "/x"}))
	if !strings.Contains(first, "Zurück") {
		t.Errorf("missing Zurück label")
	}
	if !strings.Contains(first, "aria-disabled=\"true\"") {
		t.Errorf("first page must disable Zurück (aria-disabled): %s", first)
	}
	if !strings.Contains(first, "Seite 1") {
		t.Errorf("missing page indicator: %s", first)
	}
	last := render(t, components.Pagination(components.PageNav{Page: 3, Total: 30, PageSize: 10, BaseHref: "/x"}))
	if !strings.Contains(last, "Weiter") {
		t.Errorf("missing Weiter label")
	}
	// last page disables Weiter and hides "Mehr laden"
	if strings.Contains(last, "Mehr laden") {
		t.Errorf("last page must not show 'Mehr laden': %s", last)
	}
}
