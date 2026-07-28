# Dokument-Bilder Lightbox — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ein Klick auf ein eingebettetes Bitmap-Bild in der WebUI-Dokumentansicht öffnet es groß in einem Overlay.

**Architecture:** Der Server markiert zoombare Bilder ausschließlich mit `class="zoomable"` (kein neues `data-`-Attribut, damit die bluemonday-Policy unangetastet bleibt). Ein einziges `<dialog id="doc-lightbox">` pro Seite, gemountet in `DocRenderScripts`, plus `static/js/lightbox.js` heben das server-gerenderte HTML zur Interaktion an — Progressive Enhancement wie bei Mermaid. Alles Sicherheitsrelevante (welcher `src` überhaupt entsteht) bleibt server-seitig; das Skript kopiert nur, was schon im DOM steht.

**Tech Stack:** Go 1.x, goldmark, bluemonday, templ, Tailwind v4 (CLI), Vanilla JS (kein Framework, kein Build-Step für JS).

**Spec:** `docs/superpowers/specs/2026-07-27-doc-image-lightbox-design.md`

**Worktree:** `/Users/msoent/SourceCode/serverkraken/flow-fr-doc-lightbox`, Branch `fr-doc-lightbox`. Alle Pfade unten sind relativ dazu.

## Global Constraints

- **Multi-Tenant:** flow ist eine Multi-Tenant-App. „Ist nur ein User" ist als Begründung unzulässig. Für diesen Plan ohne Datenzugriff nicht praktisch relevant, aber die Regel gilt.
- **`make ci` muss grün sein**, bevor irgendeine Task „fertig" ist: `lint verify-generate verify-css verify-no-popups cover build`. Coverage-Gate 75 % (`*_templ.go` ausgenommen).
- **Keine nativen Popups:** `alert`/`confirm`/`prompt` sind in `internal/adapter/webui` verboten (`scripts/verify-no-popups.sh`). Verstoß bricht CI.
- **CSP:** `script-src 'self' 'nonce-…'` — JavaScript nur als eigenes Asset-File unter `static/js/`, niemals inline. `img-src 'self' data:`.
- **CSS:** Quelle ist `web/tailwind.css`. Nach jeder Änderung `make web` (schreibt `internal/adapter/webui/static/app.css`) und **beide** Dateien committen, sonst reißt `verify-css`.
- **templ:** Nach Änderung einer `.templ` `make generate` und die erzeugten `*_templ.go` mitcommitten, sonst reißt `verify-generate`.
- **Assets:** URLs immer über `AssetURL("js/foo.js")` bauen, nie als `"/static/…"`-Literal — der Cache-Buster hängt daran.
- **Suchwerkzeuge:** `rg` statt `grep`, `fd` statt `find`.
- **i18n:** Jeder neue Key muss in `catalog_de.go` **und** `catalog_en.go` stehen; `TestCatalogsParity` (`internal/i18n/catalog_test.go:7`) prüft beide Richtungen.

## File Structure

| Datei | Verantwortung | Task |
|---|---|---|
| `internal/adapter/webui/artifact_embed.go` | markiert das `![[slug]]`-Bild mit `zoomable` | 1 |
| `internal/adapter/webui/wikilink.go` | markiert das `![alt](url)`-Bild — nur bei gültigem `src` | 1 |
| `internal/adapter/webui/wikilink_test.go` | Rendertests für beide Markierungen inkl. Negativfall | 1 |
| `internal/i18n/catalog_de.go`, `catalog_en.go` | Key `document.image.zoom` | 2 |
| `internal/adapter/webui/doc_render_assets.templ` | Overlay-Markup + Script-Mounts, einmal pro Seite | 2 |
| `internal/adapter/webui/doc_render_assets_test.go` | **neu** — Rendertest + Asset-Existenz-Invariante | 2, 3 |
| `internal/adapter/webui/static/js/lightbox.js` | **neu** — Upgrade + Öffnen | 3 |
| `web/tailwind.css` → `static/app.css` | Cursor + Overlay-Panel-Styles | 4 |

**Reihenfolge-Logik:** Task 1 ist server-seitig autark und ohne Sichtbarkeit im UI. Task 2 mountet das leere Overlay. Task 3 macht es lebendig. Task 4 macht es hübsch. Task 5 ist das Gesamt-Gate. Jede Task ist einzeln lauffähig und commit-fähig.

---

### Task 1: Renderer markiert zoombare Bilder

**Files:**
- Modify: `internal/adapter/webui/artifact_embed.go:117`
- Modify: `internal/adapter/webui/wikilink.go:181-205`
- Test: `internal/adapter/webui/wikilink_test.go` (anhängen)

**Interfaces:**
- Consumes: nichts.
- Produces: die CSS-Klasse `zoomable` auf `<img>`-Elementen im Output von `RenderDocument`. Task 3 selektiert exakt darauf (`img.zoomable`), Task 4 stylt exakt darauf.

**Hintergrund für den Umsetzer:** `safeImageHTMLRenderer` (`wikilink.go:175`) überschreibt goldmarks Kern-Image-Renderer und ist die einzige Stelle, an der ein `<img src>` aus Markdown entstehen kann. Passt die Ziel-URL nicht auf `artifactSrcRe`, setzt der Renderer `dst = ""` — das `<img>` bleibt also im Output, nur ohne Quelle (bluemonday wirft ein leeres `src=""` anschließend ganz weg). Genau dieses quellenlose Bild darf die Klasse **nicht** bekommen, sonst öffnet der Klick ein leeres Overlay.

Die bluemonday-Policy braucht **keine** Änderung: `class` auf `img` ist bereits erlaubt (`wikilink.go:69`).

- [ ] **Step 1: Die fehlschlagenden Tests schreiben**

Ans Ende von `internal/adapter/webui/wikilink_test.go` anhängen:

```go
// --- fr-doc-lightbox: zoombare Bilder ------------------------------------

// TestZoomable_ArtifactEmbedImage pins that a resolved ![[slug]] image embed
// carries the class the lightbox selects on. The file chip (non-image
// artifact) must NOT get it — there is nothing to enlarge.
func TestZoomable_ArtifactEmbedImage(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	resolve := func(slug string) (ArtifactRef, bool) {
		switch slug {
		case "bild":
			return ArtifactRef{
				Href: "/nodes/node-1/artifacts/bild", Ref: "abcdef123456",
				Name: "bild.png", Mime: "image/png", IsImage: true,
			}, true
		case "datei":
			return ArtifactRef{
				Href: "/nodes/node-1/artifacts/datei", Ref: "abcdef123456",
				Name: "datei.pdf", Mime: "application/pdf", SizeStr: "1.0 KB",
			}, true
		}
		return ArtifactRef{}, false
	}

	img := string(mustHTML(RenderDocument(ctx, "![[bild]]\n", resolveNone, resolve)))
	if !strings.Contains(img, `class="zoomable"`) {
		t.Fatalf("resolved image embed must be zoomable:\n%s", img)
	}

	chip := string(mustHTML(RenderDocument(ctx, "![[datei]]\n", resolveNone, resolve)))
	if strings.Contains(chip, "zoomable") {
		t.Fatalf("a non-image artifact chip must not be zoomable:\n%s", chip)
	}
}

// TestZoomable_CoreImageWithValidSrc covers the second image syntax: a core
// ![alt](url) pointing at an allowed artifact serve route earns the class,
// in both the node-scoped and the free-library form.
func TestZoomable_CoreImageWithValidSrc(t *testing.T) {
	for _, url := range []string{
		"/nodes/n1/artifacts/bild",
		"/nodes/n1/artifacts/bild?v=abcdef123456",
		"/artefakte/bild",
	} {
		out := string(mustHTML(RenderDocument(context.Background(), "![x]("+url+")\n", resolveNone, nil)))
		if !strings.Contains(out, `class="zoomable"`) {
			t.Fatalf("core image with allowed src %q must be zoomable:\n%s", url, out)
		}
	}
}

// TestZoomable_BlockedSrcHasNoClass is the load-bearing negative: a rejected
// destination leaves safeImageHTMLRenderer emitting an <img> with an empty
// src (bluemonday then drops the attribute entirely). Marking THAT image
// zoomable would open an empty lightbox on click, so the class must be tied
// to a usable src — not emitted unconditionally.
func TestZoomable_BlockedSrcHasNoClass(t *testing.T) {
	for _, url := range []string{
		"https://evil.example/x.png",
		"data:image/png;base64,AAAA",
		"//evil.example/x.png",
	} {
		out := string(mustHTML(RenderDocument(context.Background(), "![x]("+url+")\n", resolveNone, nil)))
		if !strings.Contains(out, "<img") {
			t.Fatalf("%s: the <img> element itself must survive:\n%s", url, out)
		}
		if strings.Contains(out, "zoomable") {
			t.Fatalf("%s: an image with a blocked (empty) src must not be zoomable:\n%s", url, out)
		}
	}
}
```

- [ ] **Step 2: Tests laufen lassen und Fehlschlag bestätigen**

Run: `go test ./internal/adapter/webui/ -run 'TestZoomable' -v`
Expected: FAIL — alle drei Tests, jeweils weil `zoomable` im Output fehlt. `TestZoomable_BlockedSrcHasNoClass` ist die Ausnahme: der grünt schon vor der Implementierung (noch setzt niemand die Klasse). Das ist in Ordnung — er ist die Regressionsklammer für Step 3 und muss danach immer noch grün sein.

- [ ] **Step 3: Artifact-Embed markieren**

In `internal/adapter/webui/artifact_embed.go`, im `IsImage`-Zweig (Zeile 117), diese Zeile:

```go
		_, _ = w.WriteString(`<div class="frame"><img loading="lazy" src="`)
```

ersetzen durch:

```go
		// class="zoomable": Klick-zum-Vergrößern (static/js/lightbox.js). Ein
		// aufgelöster IsImage-Embed hat per Definition eine nutzbare Quelle,
		// also ist die Klasse hier bedingungslos korrekt — anders als beim
		// Core-Image in wikilink.go, wo ein geblocktes Ziel eine leere src
		// hinterlässt.
		_, _ = w.WriteString(`<div class="frame"><img class="zoomable" loading="lazy" src="`)
```

- [ ] **Step 4: Core-Image markieren — nur bei nutzbarer Quelle**

In `internal/adapter/webui/wikilink.go`, in `(*safeImageHTMLRenderer).render`, diesen Block:

```go
	dst := string(n.Destination)
	if !artifactSrcRe.MatchString(dst) {
		dst = ""
	}
	_, _ = w.WriteString(`<img src="`)
```

ersetzen durch:

```go
	dst := string(n.Destination)
	if !artifactSrcRe.MatchString(dst) {
		dst = ""
	}
	_, _ = w.WriteString(`<img`)
	// Nur ein Bild mit nutzbarer Quelle bekommt die Zoom-Affordanz. Ein
	// abgelehntes Ziel rendert oben zu dst == "" — das <img> bleibt (ohne
	// brauchbare src, bluemonday wirft das leere Attribut ganz weg), und ein
	// zoomable darauf würde beim Klick ein leeres Overlay öffnen.
	if dst != "" {
		_, _ = w.WriteString(` class="zoomable"`)
	}
	_, _ = w.WriteString(` src="`)
```

- [ ] **Step 5: Tests laufen lassen und Erfolg bestätigen**

Run: `go test ./internal/adapter/webui/ -run 'TestZoomable|TestSafeImageRenderer|TestArtifactEmbed' -v`
Expected: PASS — die drei neuen Tests plus die vorhandenen Renderer-Tests (Regression: `TestSafeImageRenderer_RejectsNonArtifactSrc` und `TestArtifactEmbed_ResolvedImage` dürfen nicht kippen).

- [ ] **Step 6: Ganzes Paket + Lint**

Run: `go test ./internal/adapter/webui/ && go vet ./internal/adapter/webui/`
Expected: beides ohne Fehler.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/webui/artifact_embed.go internal/adapter/webui/wikilink.go internal/adapter/webui/wikilink_test.go
git commit -m "feat(webui): mark zoomable document images with a css class"
```

---

### Task 2: Overlay-Markup und i18n-Schlüssel

**Files:**
- Modify: `internal/i18n/catalog_de.go` (bei den `document.figure.*`-Keys, ~Zeile 313)
- Modify: `internal/i18n/catalog_en.go` (analoge Stelle, ~Zeile 305)
- Modify: `internal/adapter/webui/doc_render_assets.templ`
- Create: `internal/adapter/webui/doc_render_assets_test.go`

**Interfaces:**
- Consumes: `class="zoomable"` aus Task 1 (nur konzeptionell — dieser Task rendert das Ziel-Overlay).
- Produces:
  - `<dialog id="doc-lightbox">` mit `data-zoom-label="<übersetzter Text>"` und einem leeren `<img class="lightbox-img">`. Task 3 selektiert per `document.getElementById("doc-lightbox")`, liest `dlg.dataset.zoomLabel` und befüllt `dlg.querySelector(".lightbox-img")`.
  - Script-Mounts für `js/dialog.js` und `js/lightbox.js`.
  - i18n-Key `document.image.zoom`.

**Hintergrund:** `DocRenderScripts` ist der gemeinsame Mount-Punkt beider Doc-Oberflächen — `document.templ:22` (Dokumentansicht) und `cockpit.templ:78` (Node-Cockpit README). Er wird **einmal pro Seitenaufruf** gerendert, außerhalb SSE-/htmx-getauschter Fragmente. Deshalb genau **ein** `<dialog>` für die ganze Seite, nicht eines pro Bild.

`dialog.js` wird mitgemountet, weil das Overlay dessen generische Listener erbt (Esc via nativem `<dialog>`, Backdrop-Klick, Fokus-Falle). Der Loader ist idempotent (`window.__flowDialogInit`), doppeltes Einbinden auf einer Seite mit anderen Dialogen ist also unschädlich.

Das File liegt in `package webui`, nicht in `components` — Aufrufe dorthin müssen qualifiziert werden (`components.T`, `@components.IconButton`), wie in `document.templ`.

- [ ] **Step 1: i18n-Schlüssel ergänzen**

In `internal/i18n/catalog_de.go`, direkt nach der Zeile `"document.figure.unresolved": "Unaufgelöste Artefakt-Referenz",`:

```go
			// Bild-Lightbox (fr-doc-lightbox): aria-label des klickbaren Bildes
			"document.image.zoom": "Bild vergrößern",
```

In `internal/i18n/catalog_en.go`, direkt nach `"document.figure.unresolved": "Unresolved artifact reference",`:

```go
			// Image lightbox (fr-doc-lightbox): aria-label of the clickable image
			"document.image.zoom": "Enlarge image",
```

- [ ] **Step 2: Katalog-Parität prüfen**

Run: `go test ./internal/i18n/ -run TestCatalogsParity -v`
Expected: PASS. Bei FAIL fehlt der Key in einem der beiden Kataloge.

- [ ] **Step 3: Den fehlschlagenden Rendertest schreiben**

Neue Datei `internal/adapter/webui/doc_render_assets_test.go`:

```go
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
```

- [ ] **Step 4: Test laufen lassen und Fehlschlag bestätigen**

Run: `go test ./internal/adapter/webui/ -run TestDocRenderScripts -v`
Expected: FAIL mit `DocRenderScripts misses "id=\"doc-lightbox\""` — bisher gibt der Component nur das Mermaid-Script aus.

- [ ] **Step 5: Overlay implementieren**

`internal/adapter/webui/doc_render_assets.templ` vollständig ersetzen durch:

```templ
package webui

import "github.com/serverkraken/flow/internal/adapter/webui/components"

// DocRenderScripts mounts the client-side scripts and shared markup that turn
// RenderDocument's server-rendered document HTML into its full presentation:
//   - mermaid-init.js — renders <pre class="mermaid"> blocks into diagrams
//   - lightbox.js + #doc-lightbox — click an image to see it enlarged
//   - dialog.js — supplies the lightbox with Esc / backdrop-click / focus-trap
//     (its listeners are bound generically to dialog[open], not to a specific
//     Dialog component instance); the loader is idempotent, so mounting it
//     here on a page that also renders other Dialogs is harmless
//
// chroma code-highlighting and prose styling are pure CSS from components.Base
// and need no script.
//
// It exists so every surface that renders a Kompendium document body mounts the
// SAME set: the document view (/wissen/{id}, document.templ) and the node
// cockpit (README section, cockpit.templ). Add a new render feature here once
// and both surfaces gain it — the two must not drift apart.
//
// Mount it ONCE per full page load, OUTSIDE any SSE-/htmx-swapped fragment: each
// script re-scans the whole document on htmx:afterSwap, so a fragment swap must
// never re-add the <script> tag itself — and the lightbox is ONE dialog for the
// whole page, not one per image. Scripts are page-agnostic and lazy-load their
// heavy libs only when a matching element is present, so a page with no diagram
// pays nothing.
templ DocRenderScripts() {
	<script src={ AssetURL("js/mermaid-init.js") } defer data-v={ AssetVersion() }></script>
	<script src={ AssetURL("js/dialog.js") } defer></script>
	<script src={ AssetURL("js/lightbox.js") } defer></script>
	<dialog
		id="doc-lightbox"
		aria-modal="true"
		aria-label={ components.T(ctx, "document.image.zoom") }
		data-zoom-label={ components.T(ctx, "document.image.zoom") }
		class="lightbox m-auto border-0 bg-transparent p-0 backdrop:bg-ink/40"
	>
		<div class="lightbox-bar">
			@components.IconButton("✕", components.T(ctx, "common.close"), templ.Attributes{"data-dialog-close": true, "type": "button"})
		</div>
		<img class="lightbox-img" alt=""/>
	</dialog>
}
```

- [ ] **Step 6: templ generieren**

Run: `make generate`
Expected: `internal/adapter/webui/doc_render_assets_templ.go` wird neu geschrieben.

- [ ] **Step 7: Tests laufen lassen und Erfolg bestätigen**

Run: `go test ./internal/adapter/webui/ -run TestDocRenderScripts -v && go test ./internal/adapter/webui/ ./internal/i18n/`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/i18n/catalog_de.go internal/i18n/catalog_en.go \
        internal/adapter/webui/doc_render_assets.templ \
        internal/adapter/webui/doc_render_assets_templ.go \
        internal/adapter/webui/doc_render_assets_test.go
git commit -m "feat(webui): mount a shared lightbox dialog for document images"
```

---

### Task 3: `lightbox.js`

**Files:**
- Create: `internal/adapter/webui/static/js/lightbox.js`
- Modify: `internal/adapter/webui/doc_render_assets_test.go` (Test anhängen)

**Interfaces:**
- Consumes: `img.zoomable` (Task 1), `#doc-lightbox` mit `data-zoom-label` und `.lightbox-img` (Task 2).
- Produces: keine Go-Symbole. Setzt zur Laufzeit `tabindex`/`role`/`aria-label` auf jedem `img.zoomable` und markiert behandelte Bilder mit `data-lb-done`.

**Hintergrund:** Für JavaScript existiert im Repo keine Test-Infrastruktur, die Logik lässt sich also nicht direkt testen. Der eine automatisierbare — und realistische — Fehlerfall ist ein Script-Tag, das auf ein nicht existierendes Asset zeigt: `AssetURL` baut nur einen String, ein Tippfehler im Pfad fällt erst als 404 im Browser auf. Genau diese Invariante prüft der Test unten.

Was das Skript **nicht** selbst tun muss, weil `dialog.js` es generisch erledigt: Esc (nativ am `<dialog>`), Backdrop-Klick, Fokus-Falle. Einzig „Fokus zurück zum Auslöser" hängt dort an `[data-dialog-open]` — ein Attribut, das im sanitisierten Dokument-Body nicht möglich ist — und passiert deshalb hier selbst.

- [ ] **Step 1: Den fehlschlagenden Test schreiben**

An `internal/adapter/webui/doc_render_assets_test.go` anhängen:

```go
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
```

- [ ] **Step 2: Test laufen lassen und Fehlschlag bestätigen**

Run: `go test ./internal/adapter/webui/ -run TestDocRenderScripts_ScriptAssetsExist -v`
Expected: FAIL mit `references a missing asset "js/lightbox.js"` — Task 2 mountet das Script bereits, die Datei existiert aber noch nicht.

- [ ] **Step 3: Das Skript schreiben**

Neue Datei `internal/adapter/webui/static/js/lightbox.js`:

```javascript
// Klick auf ein eingebettetes Dokument-Bild öffnet es groß in #doc-lightbox.
// Der Server markiert zoombare Bilder mit class="zoomable" (nur solche mit
// nutzbarer Quelle — siehe safeImageHTMLRenderer); dieses Skript macht sie
// bedienbar. Esc, Backdrop-Klick und Fokus-Falle liefert dialog.js generisch
// für jedes dialog[open]; nur der Fokus-Rücksprung hängt dort an
// [data-dialog-open] und passiert deshalb hier selbst.
// Idempotent über [data-lb-done], re-scannt bei htmx:afterSwap.
(function () {
  if (window.__flowLightboxInit) return;
  window.__flowLightboxInit = true;

  var opener = null;

  function dlg() { return document.getElementById('doc-lightbox'); }

  // upgrade macht jedes noch unbehandelte zoombare Bild bedienbar: ein <img>
  // ist von sich aus weder fokussierbar noch als Bedienelement erkennbar.
  function upgrade() {
    var d = dlg();
    if (!d) return;                                  // Seite ohne Overlay → nichts zu tun
    var label = d.dataset.zoomLabel || '';
    document.querySelectorAll('img.zoomable:not([data-lb-done])').forEach(function (img) {
      img.setAttribute('data-lb-done', '1');
      img.setAttribute('tabindex', '0');
      img.setAttribute('role', 'button');
      if (label) img.setAttribute('aria-label', label);
    });
  }

  function open(img) {
    var d = dlg();
    if (!d || typeof d.showModal !== 'function') return;
    var target = d.querySelector('.lightbox-img');
    if (!target) return;
    // Nur kopieren, was schon im DOM steht — das Skript trifft KEINE eigene
    // URL-Entscheidung; welcher src überhaupt entstehen darf, hat der
    // Renderer server-seitig entschieden.
    target.setAttribute('src', img.currentSrc || img.src);
    target.setAttribute('alt', img.getAttribute('alt') || '');
    opener = img;
    d.showModal();
  }

  document.addEventListener('click', function (e) {
    var img = e.target.closest ? e.target.closest('img.zoomable') : null;
    if (img) open(img);
  });

  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Enter' && e.key !== ' ' && e.key !== 'Spacebar') return;
    var el = document.activeElement;
    if (!el || !el.matches || !el.matches('img.zoomable')) return;
    e.preventDefault();                              // Space soll nicht scrollen
    open(el);
  });

  // Fokus zurück aufs Bild, sobald das Overlay schließt — egal ob per ✕, Esc
  // oder Backdrop. 'close' bubbelt nicht, daher Capture-Phase (wie dialog.js).
  document.addEventListener('close', function (e) {
    if (e.target.id !== 'doc-lightbox') return;
    var t = e.target.querySelector('.lightbox-img');
    if (t) t.removeAttribute('src');                 // Bild freigeben, kein Aufblitzen beim nächsten Öffnen
    if (opener) { opener.focus(); opener = null; }
  }, true);

  if (document.readyState !== 'loading') upgrade();
  else document.addEventListener('DOMContentLoaded', upgrade);
  document.body.addEventListener('htmx:afterSwap', upgrade);
})();
```

- [ ] **Step 4: Test laufen lassen und Erfolg bestätigen**

Run: `go test ./internal/adapter/webui/ -run TestDocRenderScripts -v`
Expected: PASS — beide Tests.

- [ ] **Step 5: Popup-Verbot prüfen**

Run: `make verify-no-popups`
Expected: `verify-no-popups: OK`. Das Skript enthält bewusst kein `alert`/`confirm`/`prompt`.

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/webui/static/js/lightbox.js internal/adapter/webui/doc_render_assets_test.go
git commit -m "feat(webui): add lightbox.js to open document images enlarged"
```

---

### Task 4: Styling

**Files:**
- Modify: `web/tailwind.css` (im Lesesaal-L3-Block, direkt nach der `.filechip`-Regel)
- Modify: `internal/adapter/webui/static/app.css` (generiert, nicht von Hand)

**Interfaces:**
- Consumes: `img.zoomable` (Task 1), `dialog.lightbox` / `.lightbox-bar` / `.lightbox-img` (Task 2).
- Produces: keine.

**Hintergrund:** `web/tailwind.css:201` definiert bereits global `*:focus-visible { outline: 2px solid rgb(var(--accent)) }`. Das zoombare Bild erbt den Fokus-Ring also automatisch, sobald Task 3 ihm `tabindex="0"` gibt — eine eigene Fokus-Regel wäre Doppelung und entfällt bewusst.

Der Backdrop kommt aus der Tailwind-Utility `backdrop:bg-ink/40` am `<dialog>` (Task 2), identisch zu `components/dialog.templ:11`.

- [ ] **Step 1: Styles ergänzen**

In `web/tailwind.css`, direkt nach der Zeile `.filechip:hover { background: rgb(var(--wash)); }` einfügen:

```css
  /* Bild-Lightbox (fr-doc-lightbox): jedes Bitmap-Bild im Dokument öffnet
     beim Klick #doc-lightbox. Die Klasse setzt der Renderer server-seitig,
     bedienbar macht sie static/js/lightbox.js — ohne JS bleibt genau der
     Cursor-Hinweis hier wirkungslos, das Bild aber lesbar. Der Fokus-Ring
     kommt vom globalen *:focus-visible weiter oben. */
  img.zoomable { cursor: zoom-in; }
  dialog.lightbox { max-width: 92vw; max-height: 92vh; overflow: visible; }
  dialog.lightbox .lightbox-bar { display: flex; justify-content: flex-end; margin-bottom: 8px; }
  dialog.lightbox .lightbox-img { display: block; max-width: 92vw; max-height: 88vh; object-fit: contain; border-radius: 10px; background: rgb(var(--surface)); }
```

- [ ] **Step 2: CSS bauen**

Run: `make web`
Expected: `internal/adapter/webui/static/app.css` wird neu geschrieben. Falls `tailwindcss` nicht auf dem PATH liegt, hier stoppen und melden — ohne die CLI ist `verify-css` nicht erfüllbar.

- [ ] **Step 3: Drift-Check**

Run: `make verify-css`
Expected: `verify-css: OK`.

- [ ] **Step 4: Commit**

```bash
git add web/tailwind.css internal/adapter/webui/static/app.css
git commit -m "style(webui): style the document image lightbox overlay"
```

---

### Task 5: Gesamt-Gate und Sichtprüfung

**Files:** keine Änderung erwartet. Falls doch etwas anzupassen ist, gehört die Änderung in die Task, zu der sie inhaltlich passt — hier nur committen, was das Gate erzwingt.

**Interfaces:**
- Consumes: Tasks 1–4.
- Produces: nichts.

**Hintergrund:** Die JavaScript-Logik ist von CI nicht abgedeckt. Diese Task ist der einzige Punkt, an dem das tatsächliche Verhalten überhaupt verifiziert wird — sie ist nicht optional und nicht durch „Tests sind grün" ersetzbar.

- [ ] **Step 1: Volles CI-Gate**

Run: `make ci`
Expected: alle Stufen grün — `lint`, `verify-generate`, `verify-css`, `verify-no-popups`, `cover` (≥ 75 %), `build`.

- [ ] **Step 2: Server starten**

Run: `make dev-run` (führt `./scripts/dev-run.sh` aus)
Expected: Server läuft lokal.

- [ ] **Step 3: Sichtprüfung Dokumentansicht**

Ein Dokument unter `/wissen/{id}` öffnen, das mindestens ein Bild enthält (`![[slug]]` oder `![alt](/artefakte/…)`). Prüfen:

1. Cursor über dem Bild ist `zoom-in`.
2. Klick öffnet das Overlay, Bild ist auf den Viewport eingepasst, nicht abgeschnitten.
3. ✕ schließt. Esc schließt. Klick auf den Backdrop schließt.
4. Nach dem Schließen liegt der Fokus wieder auf dem Bild.
5. Nur `Tab` bis zum Bild, dann `Enter` → Overlay öffnet. Ebenso mit `Space`, ohne dass die Seite scrollt.
6. Browser-Konsole ohne Fehler, Netzwerk-Tab ohne 404 auf `lightbox.js`.
7. Bei aktivem `CSPEnforce`: keine CSP-Violation in der Konsole.

- [ ] **Step 4: Sichtprüfung Cockpit**

Ein Node-Cockpit mit README öffnen, das ein Bild enthält. Dieselbe Prüfung wie Step 3, Punkte 1–4. Damit ist belegt, dass beide Doc-Oberflächen die Funktion haben und nicht nur die Dokumentansicht.

- [ ] **Step 5: Negativfall gegenprüfen**

In einem Testdokument ein Bild mit externer Quelle notieren, z. B. `![extern](https://example.com/x.png)`. Erwartet: das Bild ist nicht klickbar, hat keinen `zoom-in`-Cursor und öffnet kein leeres Overlay. Das ist die Sichtprüfung zu `TestZoomable_BlockedSrcHasNoClass`.

- [ ] **Step 6: Ergebnis festhalten**

Falls Steps 3–5 sauber durchlaufen: nichts zu committen, Branch ist fertig. Falls nicht: Fehler beschreiben, die passende Task oben korrigieren, deren Test-Zyklus erneut durchlaufen.
