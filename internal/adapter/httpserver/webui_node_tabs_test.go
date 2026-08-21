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
	if !strings.Contains(rec.Body.String(), `id="cockpit-main"`) {
		t.Errorf("unknown tab must render the register page; body=%.400s", rec.Body.String())
	}
}
