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

// TestDocRenderScripts_ScriptAssetsExist ties the mounted <script src> values
// to the embedded asset tree: AssetURL only builds a string, so a typo in the
// path compiles, renders, and then 404s silently in the browser. Every
// /static/ URL DocRenderScripts emits must resolve to a real embedded file.
func TestDocRenderScripts_ScriptAssetsExist(t *testing.T) {
	out := renderToBuf(t, context.Background(), DocRenderScripts())

	const marker = `src="/static/`
	found := 0
	for rest := out; ; {
		i := strings.Index(rest, marker)
		if i < 0 {
			break
		}
		rest = rest[i+len(marker):]
		end := strings.IndexByte(rest, '"')
		if end < 0 {
			t.Fatalf("unterminated src attribute in:\n%s", out)
		}
		path := rest[:end]
		if q := strings.IndexByte(path, '?'); q >= 0 { // "?v=<hash>" abschneiden
			path = path[:q]
		}
		if _, err := staticFS.ReadFile("static/" + path); err != nil {
			t.Fatalf("DocRenderScripts references a missing asset %q: %v", path, err)
		}
		found++
		rest = rest[end:]
	}
	if found == 0 {
		t.Fatalf("expected at least one /static/ asset reference:\n%s", out)
	}
}
