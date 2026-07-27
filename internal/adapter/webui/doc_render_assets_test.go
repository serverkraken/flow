package webui

// Rendertests für DocRenderScripts — der gemeinsame Mount-Punkt beider
// Doc-Oberflächen (document.templ, cockpit.templ). Ctx: context.Background()
// löst auf den deutschen Katalog auf (i18n.Default = DE), wie in
// cockpit_head_render_test.go.

import (
	"context"
	"strings"
	"testing"
)

// TestDocRenderScripts_MountsLightbox pins the single per-page lightbox
// overlay: exactly one <dialog id="doc-lightbox">, an empty target <img> for
// the script to fill, the translated zoom label the script reads, and the
// two scripts that make it work (dialog.js supplies Esc/backdrop/focus-trap
// generically, lightbox.js the opening).
func TestDocRenderScripts_MountsLightbox(t *testing.T) {
	out := renderToBuf(t, context.Background(), DocRenderScripts())

	for _, want := range []string{
		`id="doc-lightbox"`,
		`class="lightbox-img"`,
		"Bild vergrößern",
		"js/lightbox.js",
		"js/dialog.js",
		"js/mermaid-init.js",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("DocRenderScripts misses %q:\n%s", want, out)
		}
	}

	// Genau EIN Overlay pro Seite — DocRenderScripts wird einmal pro
	// Seitenaufruf gerendert, und mehrere <dialog> mit derselben id wären
	// ein doppelter getElementById-Treffer.
	if n := strings.Count(out, `id="doc-lightbox"`); n != 1 {
		t.Fatalf("expected exactly one lightbox dialog, got %d:\n%s", n, out)
	}

	// Das Overlay darf nicht offen ausgeliefert werden.
	if strings.Contains(out, "<dialog") && strings.Contains(out, " open") {
		t.Fatalf("the lightbox must start closed:\n%s", out)
	}
}
