package webui

import (
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/i18n"
)

// Screen 20: die Fehlerseite sagt, WAS fehlt, bleibt in der Hülle (Schiene,
// Weg zurück) und führt weiter — mit Suche.
func TestErrorPage_SaysWhatIsMissingAndLeadsOn(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, ErrorPage(NotFoundVM("/wissen/lesesaal-l8", "msoent")))
	for _, want := range []string{
		`data-error-page="404"`, "Diese Karte gibt es nicht", "/wissen/lesesaal-l8",
		`href="/wissen"`, `data-palette-open`, "Angemeldet als msoent", `href="/auth/logout"`,
		`id="app-rail"`, // die Schiene bleibt — volle Sichtbarkeit gewinnt
	} {
		if !strings.Contains(out, want) {
			t.Errorf("404 ohne %q", want)
		}
	}
	node := renderToBuf(t, ctx, ErrorBody(NotFoundVM("/nodes/x", "")))
	if !strings.Contains(node, "Dieses Register gibt es nicht") || !strings.Contains(node, `href="/nodes"`) || strings.Contains(node, "Angemeldet") {
		t.Errorf("Register-404: %.600s", node)
	}
	srv := renderToBuf(t, ctx, ErrorBody(ServerErrorVM("/zeit", "msoent", "f7c2-0812")))
	for _, want := range []string{`data-error-page="500"`, "schiefgegangen", "f7c2-0812", `data-copy="f7c2-0812"`, "Neu laden", `href="/"`} {
		if !strings.Contains(srv, want) {
			t.Errorf("500 ohne %q", want)
		}
	}
	forb := renderToBuf(t, ctx, ErrorBody(ForbiddenVM("/", "msoent")))
	if !strings.Contains(forb, "Kein Zugriff") || !strings.Contains(forb, "⌀") {
		t.Errorf("403: %.400s", forb)
	}
}
