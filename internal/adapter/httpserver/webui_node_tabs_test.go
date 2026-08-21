package httpserver_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// TestNodeTabPages_EachSurfaceStandsOnItsOwn covers Task 10: the register's
// sections become surfaces of their own instead of stacking on one page. Each
// carries a way back to the register — a surface you can reach but not leave
// is a trap, and these are linked from the entry point.
func TestNodeTabPages_EachSurfaceStandsOnItsOwn(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo, Color: "cyan"})

	for _, tc := range []struct {
		tab     string
		wants   []string
		unwants []string
	}{
		{
			tab:     "wissen",
			wants:   []string{"flow", `href="/nodes/n1"`},
			unwants: []string{`id="cockpit-rail"`},
		},
		{
			tab:     "worktime",
			wants:   []string{"flow", `href="/nodes/n1"`},
			unwants: []string{`id="cockpit-rail"`},
		},
		{
			tab:     "struktur",
			wants:   []string{"flow", `href="/nodes/n1"`},
			unwants: []string{`id="cockpit-rail"`},
		},
	} {
		rec := c.do(t, "GET", "/nodes/n1?tab="+tc.tab, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("tab %s: status %d body=%.300s", tc.tab, rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		for _, want := range tc.wants {
			if !strings.Contains(body, want) {
				t.Errorf("tab %s missing %q", tc.tab, want)
			}
		}
		for _, unwanted := range tc.unwants {
			if strings.Contains(body, unwanted) {
				t.Errorf("tab %s must not carry the whole cockpit (%q)", tc.tab, unwanted)
			}
		}
	}
}

// TestNodeTabPages_UnknownTabFallsBackToTheRegister keeps an old bookmark or a
// typo from ending on an error page: an unknown surface is not a 404, it is
// simply the register.
func TestNodeTabPages_UnknownTabFallsBackToTheRegister(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo, Color: "cyan"})

	rec := c.do(t, "GET", "/nodes/n1?tab=gibtsnicht", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `id="einstieg-kasten"`) {
		t.Errorf("unknown tab must render the register entry point; body=%.400s", rec.Body.String())
	}
}

// TestEinstieg_KastenReloadsItself pins which fragment the entry point's
// Kasten column pulls on an SSE event. The draft branch REPURPOSED
// /nodes/{id}/head to serve the Kasten; here that route still serves the
// cockpit head, so the ported wiring would have swapped the Kasten column for
// a different component on every timer start or document edit — silently, and
// only in the browser.
func TestEinstieg_KastenReloadsItself(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo, Color: "cyan"})

	rec := c.do(t, "GET", "/nodes/n1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `hx-get="/nodes/n1/kasten`) {
		t.Errorf("the Kasten column must reload ITSELF, not another fragment; body=%.900s", body)
	}
	if strings.Contains(body, `hx-get="/nodes/n1/head`) {
		t.Errorf("the entry point must not pull the cockpit head into its Kasten column; body=%.900s", body)
	}

	// And the route it names must actually answer with the Kasten.
	frag := c.do(t, "GET", "/nodes/n1/kasten", nil)
	if frag.Code != http.StatusOK {
		t.Fatalf("GET /nodes/n1/kasten = %d, want 200", frag.Code)
	}
	fb := frag.Body.String()
	if strings.Contains(fb, "<!doctype html>") {
		t.Errorf("the fragment route must answer with a fragment, not a whole page")
	}
	if !strings.Contains(fb, "flow") {
		t.Errorf("Kasten fragment missing the register's name; body=%.500s", fb)
	}
}

// TestNodeTab_KontextSurfaceIsReachable pins Soenne's decision of 21.08.: the
// context instrument (meter + pin preview) was built and works, but after the
// entry point took over /nodes/{id} nothing linked to it any more. It is a
// surface of its own now, and the entry point's ways row leads there.
func TestNodeTab_KontextSurfaceIsReachable(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo, Color: "cyan"})

	rec := c.do(t, "GET", "/nodes/n1?tab=kontext", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.300s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `href="/nodes/n1"`) {
		t.Errorf("the context surface needs its way back; body=%.600s", body)
	}

	// And the entry point must lead there — an instrument nobody can reach is
	// the state this decision was made to end.
	entry := c.do(t, "GET", "/nodes/n1", nil)
	if !strings.Contains(entry.Body.String(), `href="/kontext/n1"`) {
		t.Errorf("entry point must link to the context surface; body=%.900s", entry.Body.String())
	}
}
