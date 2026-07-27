# Dokument-Bilder: Lightbox beim Klick (Design Spec)

- **Datum:** 2026-07-27
- **Branch:** `fr-doc-lightbox` (Worktree `../flow-fr-doc-lightbox`, von `main`)
- **Status:** Draft — zur Review
- **Umfang:** eine kleine, in sich geschlossene Anpassung am WebUI-Doc-Renderer.
  Kein Slice eines größeren Programms.
- **Betroffener Renderer:** ausschließlich `RenderDocument`
  (`internal/adapter/webui/wikilink.go`). Der TUI-Renderer
  (`internal/tui/markdown`) ist nicht betroffen — dort gibt es keinen Klick.

---

## 1. Kontext & Ziel

Eingebettete Bilder werden in der Lese-Ansicht auf Spaltenbreite gerendert.
Bei Diagrammen und Screenshots reicht das zum Lesen nicht. Ein Klick auf das
Bild soll es groß über der Seite zeigen.

`RenderDocument` erzeugt heute drei Bild-Sorten:

| Quelle | HTML heute | Emitter |
|---|---|---|
| `![[slug]]` Artifact-Embed | `<figure class="figure"><div class="frame"><img …></div><figcaption>…</figcaption></figure>` | `artifact_embed.go:115-128` |
| `![alt](url)` Core-Image | `<img src alt title>` | `wikilink.go:181-205` |
| ` ```mermaid ` | client-seitig gerendertes `<svg>` | `mermaid.go` + `static/js/mermaid-init.js` |

### Erfolgskriterien

1. Ein Klick auf ein Bitmap-Bild im gerenderten Dokument öffnet ein Overlay,
   das das Bild auf Viewport-Größe eingepasst zeigt.
2. Das Overlay schließt per ✕, `Esc` und Backdrop-Klick.
3. Beide Bild-Syntaxen verhalten sich gleich — `![[slug]]` und `![alt](url)`.
4. Die Funktion steht auf **beiden** Doc-Oberflächen zur Verfügung:
   Dokument-Ansicht (`/wissen/{id}`) und Node-Cockpit (README-Abschnitt).
5. `make ci` bleibt grün.

### Nicht-Ziele (bewusst YAGNI)

- Mermaid-SVGs. Das SVG entsteht erst nach `mermaid.run()`; die Kopplung an
  `mermaid-init.js` wäre ein Vielfaches der übrigen Änderung.
- Blättern zwischen Bildern, Zoom/Pan über die Einpassung hinaus,
  Bildunterschrift oder Download-Link im Overlay.
- Der TUI-Renderer.

---

## 2. Entscheidungen

Von Soenne im Brainstorming festgelegt:

1. **Lightbox-Overlay** als natives `<dialog>` im Design-System-Look — kein
   neuer Tab, kein Inline-Zoom im Textfluss.
2. **Alle Bitmap-Bilder**: `![[slug]]` **und** `![alt](url)`. Beide zeigen
   ohnehin nur Artefakt-Dateien (der `src` ist auf die Artifact-Routen
   beschränkt), also darf es für den Leser keinen Verhaltensunterschied geben.
3. **Minimale Ausstattung**: Bild + Schließen. Nichts weiter.

---

## 3. Architektur

> Der Server markiert vergrößerbare Bilder nur mit einer CSS-Klasse. Ein
> einziges `<dialog>` pro Seite plus ein kleines `lightbox.js` machen daraus
> die Interaktion.

Das ist dasselbe Progressive-Enhancement-Muster wie bei Mermaid: das
server-gerenderte HTML bleibt für sich sinnvoll, das Skript hebt es an. Ohne
JavaScript ist das Bild weiterhin sichtbar, nur nicht vergrößerbar.

Die Aufteilung folgt einer bewussten Grenze: **alles Sicherheitsrelevante
bleibt server-seitig.** Welcher `src` überhaupt in ein `<img>` gelangt,
entscheidet weiterhin allein `safeImageHTMLRenderer`; das Skript kopiert nur,
was bereits im DOM steht, und trifft keine eigene URL-Entscheidung.

### 3.1 Renderer — eine Klasse, keine Policy-Änderung

Beide `<img>`-Emitter bekommen `class="zoomable"`:

| Datei | Stelle | Bedingung |
|---|---|---|
| `internal/adapter/webui/artifact_embed.go:117` | `IsImage`-Zweig | immer |
| `internal/adapter/webui/wikilink.go:193` | `safeImageHTMLRenderer.render` | **nur wenn `dst != ""`** |

Die Bedingung im zweiten Fall ist der eigentliche Fallstrick. `wikilink.go:190-192`
setzt bei einem nicht erlaubten Ziel bewusst `dst = ""`, statt das Bild
wegzulassen — das `<img>` bleibt also im Output, nur ohne Quelle. Trüge es die
Klasse, öffnete der Klick ein leeres Overlay. Es ist damit ein Pflicht-Testfall,
kein Randfall (§6).

**Kein Eingriff in `getDocPolicy()`.** `class` auf `img` ist bereits erlaubt
(`wikilink.go:69`). Genau deshalb wird es eine Klasse und kein `data-`-Attribut:
jedes neue `data-*` auf `img` müsste die bluemonday-Policy aufweiten, und die
Policy ist die Sanitizer-Grenze für agent-verfasstes Markdown. Eine Klasse
kostet dort nichts.

### 3.2 Overlay-Markup in `DocRenderScripts`

`internal/adapter/webui/doc_render_assets.templ` ist laut eigenem
Doc-Kommentar der Ort, an dem beide Doc-Oberflächen gemeinsam gewinnen —
gemountet in `document.templ:22` und `cockpit.templ:78`, einmal pro
Seitenaufruf und außerhalb SSE-/htmx-getauschter Fragmente. Diese Regel gilt
für das Overlay unverändert: **ein** `<dialog>` pro Seite, nicht eines pro Bild.

Dazu kommen dort:

- `<dialog id="doc-lightbox">` mit
  `@components.IconButton("✕", components.T(ctx, "common.close"), templ.Attributes{"data-dialog-close": true, "type": "button"})`
  und einem leeren `<img>`, das JS befüllt. (Das File liegt in `package webui`,
  nicht in `components` — Aufrufe dorthin sind zu qualifizieren, wie in
  `document.templ`.)
- Das Zoom-`aria-label` als `data-`-Attribut am `<dialog>` (§3.3).
- `<script src={ AssetURL("js/lightbox.js") } defer data-v={ AssetVersion() }></script>`.

Der Doc-Kommentar des Files ist mitzupflegen — er zählt die gemounteten
Features namentlich auf und würde sonst driften.

CSP: `script-src 'self' 'nonce-…'` (`internal/adapter/httpserver/security_headers.go:27`)
deckt ein eigenes Asset-File ab; Inline-Skript wäre nicht erlaubt.
`img-src 'self' data:` deckt die Bildquelle ab.

### 3.3 `static/js/lightbox.js`

IIFE mit Idempotenz-Guard nach dem Muster von `dialog.js`
(`window.__flowLightboxInit`). Zwei Aufgaben:

**Upgrade** — bei `DOMContentLoaded` und `htmx:afterSwap`, jedes Element genau
einmal (`data-lb-done`, Muster aus `mermaid-init.js`): jedes `img.zoomable`
bekommt `tabindex="0"`, `role="button"` und ein `aria-label`. Nötig, weil ein
`<img>` von sich aus weder fokussierbar noch als Bedienelement erkennbar ist.

Der Label-Text ist übersetzt und wird als `data-`-Attribut am `<dialog>`
transportiert — das Markup stammt aus templ und läuft nicht durch bluemonday,
anders als der Dokument-Body. Dasselbe Muster nutzt `clipboard.js` mit
`data-copied-label`.

**Öffnen** — delegierter `click` sowie `keydown` auf `Enter`/`Space` an
`img.zoomable`: `src` und `alt` des geklickten Bildes ins Dialog-`<img>`
kopieren, dann `showModal()`. Den Auslöser merken und beim `close`-Event des
Dialogs wieder fokussieren.

Was das Overlay **nicht** selbst implementieren muss: `Esc` (nativ am
`<dialog>`), Backdrop-Klick und die Fokus-Falle. `dialog.js` hängt diese
Listener generisch an `dialog[open]` bzw. `document`, nicht an eine bestimmte
Dialog-Instanz — sie greifen also automatisch. Einzig „Fokus zurück zum
Auslöser" hängt dort an `[data-dialog-open]` und muss deshalb in `lightbox.js`
selbst passieren.

Damit `dialog.js` überhaupt geladen ist, mountet `DocRenderScripts` es mit
(die Dialog-Komponenten tun das heute je selbst; der Loader ist idempotent).

`verify-no-popups` bleibt grün: kein `alert`/`confirm`/`prompt`.

### 3.4 CSS

In `web/tailwind.css`, danach `make web` — sonst reißt `verify-css` (der
Check vergleicht das committete `app.css` gegen einen frischen Build).

- `img.zoomable { cursor: zoom-in }` plus sichtbarer `:focus-visible`-Ring,
  weil das Bild jetzt per Tastatur erreichbar ist.
- Overlay-Panel randlos, `max-width: 92vw`, Bild `max-height: 88vh;
  object-fit: contain`, Backdrop `ink/40` — analog `dialog.templ:11`, damit
  sich das Overlay wie die übrigen Dialoge anfühlt.

### 3.5 i18n

Ein neuer Schlüssel `document.image.zoom` (aria-label, z. B. „Bild vergrößern"
/ „Enlarge image") in `internal/i18n/catalog_de.go` **und**
`internal/i18n/catalog_en.go`. `catalog_test.go` prüft Key-Parität zwischen
den Katalogen; ein einseitiger Eintrag lässt die Suite fehlschlagen.

Der Schließen-Knopf nutzt den bestehenden `common.close`.

---

## 4. Datenfluss

```
Markdown-Quelle
   │
   ├─ ![[slug]] ──► artifactEmbedHTMLRenderer ──► <figure><div class="frame">
   │                                                <img class="zoomable" src="…">
   │
   └─ ![alt](url) ─► safeImageHTMLRenderer ────► src erlaubt? ─ nein ─► <img src="">
                                                      │                (keine Klasse)
                                                      ja
                                                      ▼
                                                <img class="zoomable" src="…">
   │
   ▼
bluemonday getDocPolicy()   (class auf img erlaubt → bleibt erhalten)
   │
   ▼
Seite: DocRenderScripts mountet <dialog id="doc-lightbox"> + lightbox.js
   │
   ▼
lightbox.js: img.zoomable → tabindex/role/aria-label
   │
   ▼
Klick / Enter / Space ──► src+alt ins Dialog-<img> ──► showModal()
                                                          │
                          ✕ / Esc / Backdrop ◄────────────┘
                                  │
                                  ▼
                          close ──► Fokus zurück aufs Bild
```

---

## 5. Fehlerverhalten

| Situation | Verhalten |
|---|---|
| Kein JavaScript / Skript lädt nicht | Bild bleibt sichtbar und gelesen; kein Klick-Effekt. Kein toter Knopf, weil `tabindex`/`role` erst das Skript setzt. |
| Bild mit geblocktem (leerem) `src` | Trägt keine Klasse, wird nicht angefasst — wie heute. |
| Dialog fehlt im DOM (Fragment ohne `DocRenderScripts`) | `lightbox.js` findet `#doc-lightbox` nicht und tut nichts, statt zu werfen. |
| Bild lädt im Overlay nicht (404/gelöschtes Artefakt) | Browser zeigt das `alt` im Overlay; schließen funktioniert normal. |
| Fragment-Swap via htmx/SSE | `htmx:afterSwap` upgradet nachgeladene Bilder; `data-lb-done` verhindert Doppelarbeit an bereits behandelten. |

---

## 6. Tests

Die Sicherheits- und Korrektheitslogik liegt server-seitig, deshalb liegt dort
auch die Testlast — für JavaScript existiert im Repo keine Test-Infrastruktur.

1. **`artifact_embed`-Rendertest:** `class="zoomable"` erscheint im
   `IsImage`-Output; der Datei-Chip-Zweig bleibt unverändert.
2. **`wikilink`-Rendertest, positiv:** `![alt](/nodes/…/artifacts/…)` mit
   gültigem Ziel trägt die Klasse.
3. **`wikilink`-Rendertest, negativ:** ein nicht erlaubtes Ziel (externer Host,
   `data:`-URI) erzeugt weiterhin `src=""` **und trägt die Klasse nicht**.
4. **Sanitizer-Durchlauf:** die Klasse überlebt `getDocPolicy()` — abgedeckt,
   weil `RenderDocument` in den obigen Tests den kompletten Pfad inkl.
   Sanitizer durchläuft.
5. **`doc_render_assets`-Rendertest:** `DocRenderScripts` gibt das
   `<dialog id="doc-lightbox">` und das `lightbox.js`-Script aus.

Gate: `make ci` (`lint verify-generate verify-css verify-no-popups cover build`),
Coverage-Schwelle 75 %.

---

## 7. Betroffene Dateien

| Datei | Änderung |
|---|---|
| `internal/adapter/webui/artifact_embed.go` | Klasse am `<img>` |
| `internal/adapter/webui/wikilink.go` | Klasse am `<img>`, nur bei gültigem `src` |
| `internal/adapter/webui/doc_render_assets.templ` | Dialog + Script-Mount, Doc-Kommentar |
| `internal/adapter/webui/static/js/lightbox.js` | neu |
| `web/tailwind.css` → `internal/adapter/webui/static/app.css` | Styles, via `make web` |
| `internal/i18n/catalog_de.go`, `catalog_en.go` | `document.image.zoom` |
| zugehörige `*_test.go` | §6 |
| `*_templ.go` | via `make generate`, mit committen |
