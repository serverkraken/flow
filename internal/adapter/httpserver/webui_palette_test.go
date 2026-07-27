package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// TestPalette_FuzzyFindsNodeAndDoc drives GET /ui/palette?q=... end-to-end
// through the real route (mirroring TestTimerWidget_Lifecycle's harness) and
// checks that a fuzzy query surfaces both a matching node (by short name) and
// a matching document (by title).
func TestPalette_FuzzyFindsNodeAndDoc(t *testing.T) {
	c := newCockpitTestServer(t)
	c.srv.ListSessions = usecase.ListSessions{Sessions: c.ss, Clock: c.clk}
	c.srv.ListDocumentsPage = usecase.NewListDocumentsPage(c.ds)

	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "gitlab.com/dataalliance/backstage", Slug: "backstage", Kind: domain.KindRepo, Color: "cyan"})
	if _, err := c.ds.Create(context.Background(), domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocFree, Title: "Backstage Probleme", Path: "docs/backstage-probleme"}); err != nil {
		t.Fatalf("seed doc: %v", err)
	}

	rec := c.do(t, "GET", "/ui/palette?q=backst", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, ">backstage<") {
		t.Fatalf("node row missing:\n%s", body)
	}
	if !strings.Contains(body, "Backstage Probleme") {
		t.Fatalf("doc row missing:\n%s", body)
	}
}

// TestPalette_RequiresAuth ensures the fragment route is webAuth-gated like
// its timer-pill neighbors — an anonymous request must not reach the handler.
func TestPalette_RequiresAuth(t *testing.T) {
	c := newCockpitTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/palette", nil)
	rec := httptest.NewRecorder()
	c.srv.Routes().ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("palette must not be public")
	}
}
