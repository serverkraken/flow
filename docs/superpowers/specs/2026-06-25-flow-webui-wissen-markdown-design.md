# flow WebUI — Wissen-Fläche + Markdown-Parität (Design Spec)

- **Datum:** 2026-06-25
- **Branch:** `rebuild`
- **Status:** Draft — zur Review
- **Slice:** „Wissen" aus dem WebUI-Overhaul (§5.7–5.9 + §6 von
  `docs/superpowers/specs/2026-06-23-flow-webui-overhaul-design.md`)
- **Vorgänger-Slices (fertig):** Slice 0 (Fundament), Slice 1 (Worktime),
  Slice 2 (Stats & Frei) — alle auf `rebuild`, ungemerged.
- **Mockups (Design-Referenz):**
  `docs/superpowers/specs/assets/2026-06-23-webui/studio-docs-categories.html`
  (Liste) und `…/studio-document-view.html` (Lese-Seite).
- **Ausführungs-Hinweis:** Dieser Slice soll **tool-agnostisch** ausführbar
  sein (Claude **oder** Gemini **oder** Codex). Der daraus folgende
  Implementierungs-Plan schreibt jede Task self-contained (explizite Pfade,
  Commands, TDD-Schritte, Verify) und legt eine `AGENTS.md` mit den
  Repo-Konventionen an. Diese Spec setzt **keine** Claude-spezifischen
  Skills/Hooks/Memory voraus.

---

## 1. Kontext & Ziel

Die WebUI wurde in Slice 0–2 auf ein semantisches Design-System (Dark/Light),
i18n (DE primär), die `AppShell`-Hülle und lokale Assets (kein CDN) umgestellt.
Worktime (Heute/Woche/Historie), Stats und Frei laufen bereits auf der neuen
Shell.

Die **Docs-/Wissen-Fläche hängt noch am alten Stand**: Routen unter `/docs`,
altes `docs.templ` (nicht auf `AppShell`), und ein **nur CommonMark + Wikilinks**
rendernder Markdown-Renderer (`internal/adapter/webui/markdown.go` /
`wikilink.go`). Dieser Slice bringt die **ganze Wissen-Fläche** auf die neue
Shell und den Markdown-Renderer auf **Parität** zur Funktionalität, die der
TUI-Renderer (`internal/tui/markdown`) bietet.

### Erfolgskriterien
1. **Wissen-Liste** (`/wissen`) voll kategorie-spezifisch (Daily-Timeline,
   Projekt-gruppiert, Frei-Grid, abgesetzter Agent/System-Bereich) auf
   `AppShell`, mit Kategorie-Tab-Strip (Scroll-Spy), Suche (Volltext +
   semantisch), Tag-Chip-Filter, „Neu", SSE-Live, serverseitiger Pagination.
2. **Dokument-Lese-Seite** (`/wissen/{id}`) auf `AppShell`: Header
   (Kind-Badge, Titel, Projekt-Link, Datum, Tags, Bearbeiten/Overflow),
   Lese-Spalte (~70ch) + ToC-Rail (Scroll-Spy) + „Referenziert von"
   (Backlinks), **volles Markdown**, SSE-Live.
3. **Editor** (`/wissen/neu`, `/wissen/{id}/bearbeiten`) auf `AppShell`:
   Titel + Body-Textarea + Tags (via Frontmatter, single source) +
   Projekt-Zuordnung + **htmx-Live-Vorschau** (debounced, gleicher Renderer);
   Save → Redirect auf Lese-Seite.
4. **Markdown feature-complete** (Web, ein gemeinsamer Renderer für Doc-View,
   Projekt-Beschreibung, Editor-Vorschau): GFM (Tabellen / Tasklists /
   Strikethrough / Autolink), Footnotes, Callouts (NOTE/TIP/WARNING/IMPORTANT/
   DANGER), Syntax-Highlighting (Chroma, **zwei** class-basierte Stylesheets
   Light/Dark), Wikilinks + Backlinks, Frontmatter-Strip.
5. **`make ci` grün** (inkl. `verify-css`, `verify-no-popups`, Coverage-Gate
   75 %). **Live-Done-Gate** gegen das Dev-Stack (Postgres + Dex): jede Route
   200, SSE-Live, Markdown-Parität sichtbar, Dark/Light + Mobile geprüft.

### Non-Goals (YAGNI)
- Keine SPA / kein JS-Framework. Server-rendered (templ + htmx) bleibt.
- **Kein slug-basiertes Routing** und **kein neuer slug-Lookup-Usecase** —
  Routing bleibt id-basiert (`/wissen/{id}`), siehe §2.
- Keine TUI-Renderer-Wiederverwendung (gibt ANSI aus, nicht HTML).
- Keine neuen API-Endpunkte außer der additiven Documents-Pagination und dem
  Editor-Preview-Web-Endpoint.
- Keine Projekt-Hub-Seite (das ist Overhaul-Slice „Projekte", separat).

---

## 2. Entscheidungen (mit Begründung)

| Entscheidung | Wahl | Begründung |
|---|---|---|
| Routing | **id-basiert** `/wissen/{id}` (Spec §4 schrieb „{slug}") | Bestehender `GetDocument`-Usecase + `ResolveWikilink` arbeiten mit Doc-ID; ein slug-Lookup wäre ein neuer Store/Usecase + Kollisions-/Migrations-Frage → unnötiger Scope. „slug" in der Overhaul-Sitemap ist nur Lesbarkeits-Notation. |
| Alte `/docs`-Routen | **ersetzen** durch `/wissen`-Routen | Eine Fläche, ein IA-Pfad; alte `docs.templ`-Templates + `/docs`-Handler entfernen, nachdem `/wissen` steht. Web-only Umstellung — die REST-API `/api/v1/documents` (TUI/MCP) bleibt unberührt. |
| Markdown-Sharing | **ein** Web-Renderer (`RenderDocument`) für Doc-View + Projekt-Beschreibung + Editor-Preview; **kein** TUI-Sharing | TUI rendert ANSI; nur Domain-Logik (`domain.ParseFrontmatter`, `domain.ResolveWikilink`) ist geteilt. „Gemeinsamer Renderer" aus §6 = innerhalb der Web-Schicht. |
| Chroma-Theme | **zwei** class-basierte Stylesheets (Light + Dark), via `data-theme` umgeschaltet | Thementreu; keine inline-styles; via `chroma/v2` `html.WithClasses(true)` + `formatter.WriteCSS` generiert. Etwas mehr Token-/AA-Pflege, dafür sauber im Design-System. |
| Editor-Vorschau | **htmx-Live-Vorschau** (debounced POST an Preview-Endpoint) | Vom Nutzer gewählt; gleich feature-complete; nutzt denselben Renderer (DRY). |
| Liste | **voll kategorie-spezifisch** (4 Layouts) | Vom Nutzer gewählt; entspricht Mockup `studio-docs-categories`. |
| Pagination | serverseitig `?limit&offset` + Gesamtzahl, additiv durch Usecase/Store | Listen skalieren (besonders nach Vault-Import); Default-Seitengröße pro Liste. |
| Dialoge/Confirm | bestehende `Dialog`/`ConfirmDialog`-Komponenten; **keine** Browser-Popups | `verify-no-popups`-Guard ist Pflicht; jede destruktive Aktion (Doc löschen) über `ConfirmDialog`. |
| neue Dep | `github.com/yuin/goldmark-highlighting/v2` | Bridge goldmark↔chroma; GFM/Footnote sind goldmark-core (keine Dep). |

---

## 3. Architektur & Datei-Organisation

Hexagonal beibehalten; „keine Monolithen" — fokussierte Dateien. Muster wie
Slice 1/2: Handler baut typisiertes Viewmodel → `Page`/`Fragment`-templ.

```
internal/adapter/webui/
  markdown.go            # gemeinsamer Renderer (ausgebaut) — GFM/Footnote/
                         #   Callouts/Chroma/Wikilink/Frontmatter
  markdown_callout.go    # Callout-AST-Transformer + Renderer (eigene Datei)
  markdown_chroma.go     # Chroma-Highlighting-Anbindung (class-based)
  wikilink.go            # bestehender Wikilink-Parser/Renderer (unverändert)
  wissen.templ           # Liste (kategorie-spezifisch) — Page + Fragment
  wissen_vm.go           # Viewmodel-Builder für die Liste
  document.templ         # Lese-Seite — Page (+ Fragment für SSE-Reload)
  document_vm.go         # Viewmodel für die Lese-Seite
  editor.templ           # Editor + Live-Vorschau-Fragment
  editor_vm.go           # Viewmodel für den Editor
  components/
    doctimeline.templ    # Daily-Journal-Timeline (Monats-gruppiert)
    docprojectgroup.templ# Projekt-Notizen unter Projekt-Header
    doccardgrid.templ    # Frei-Karten-Grid
    docsystemlist.templ  # Agent/System abgesetzt
    categorystrip.templ  # Kategorie-Tab-Strip (Scroll-Spy)
    markdownprose.templ  # Prose-Wrapper um gerendertes HTML
    toc.templ            # ToC-Rail
    backlinks.templ      # „Referenziert von"
  static/
    chroma-light.css chroma-dark.css   # generiert, go:embed
internal/adapter/httpserver/
  webui_wissen.go        # Handler Liste (Home + List-Fragment)
  webui_document.go      # Handler Lese-Seite (+ SSE-Fragment)
  webui_editor.go        # Handler Editor (New/Edit/Create/Update/Delete/Preview)
web/tailwind.css         # Prose-/Callout-/Chroma-Token-Layer ergänzen
```

> Die alten `internal/adapter/webui/docs.templ` +
> `internal/adapter/httpserver/webui_docs.go` werden in der Wiring-Task
> entfernt, sobald `/wissen` vollständig steht (inkl. ihrer Tests, soweit nicht
> portiert).

---

## 4. Markdown-Renderer (Herzstück, §6)

### 4.1 Pipeline
`RenderDocument(src string, resolve WikilinkResolver) template.HTML` baut eine
goldmark-Instanz mit:
- `extension.GFM` — Tabellen, Strikethrough, Tasklists, Autolinks.
- `extension.Footnote` — Fußnoten.
- **Callout-Extension** (eigene, `markdown_callout.go`): erkennt
  Blockquotes der Form `> [!NOTE]` / `[!TIP]` / `[!WARNING]` / `[!IMPORTANT]` /
  `[!DANGER]` (GitHub-Alert-Syntax), transformiert sie in ein
  `<div class="callout callout-note">…`-Markup mit Titelzeile + Glyph. Farben
  aus den semantischen Tokens (note=accent, tip=success, warning=warning,
  important=highlight, danger=danger).
- **Chroma-Highlighting** (`markdown_chroma.go`): goldmark-highlighting/v2 mit
  `html.WithClasses(true)` → `<span class="…">`; **keine** inline-styles. Zwei
  Stylesheets via Chroma-Formatter generiert (siehe §4.2).
- **Wikilink-Parser** (bestehend in `wikilink.go`) — `[[target]]` /
  `[[target|display]]`; resolved → `<a class="wikilink">`, broken →
  `<span class="wikilink-broken">`.
- **Frontmatter-Strip** vorab via `domain.ParseFrontmatter` (bestehend).

### 4.2 Chroma-Stylesheets (zwei, class-based)
- Build-Zeit oder `go:generate`: Chroma-Formatter (`html.New(html.WithClasses(true))`)
  schreibt `chroma-light.css` (z. B. Style `github`) und `chroma-dark.css`
  (z. B. `github-dark`) nach `internal/adapter/webui/static/`.
- Beide via `go:embed` eingebettet, unter `/static/` ausgeliefert.
- `base.templ`-Head lädt **beide**; per `data-theme`-Selektor ist nur eines
  aktiv (z. B. `chroma-dark.css`-Regeln unter `:root[data-theme="dark"] .chroma`,
  light unter `:root:not([data-theme="dark"]) .chroma`). Umsetzung: entweder
  zwei `<link>` mit media-/attr-Scoping **oder** beide CSS in ein
  `chroma.css` mit gescopten Selektoren mergen (bevorzugt: ein File, gescopt —
  kein FOUC, keine doppelte Netzwerk-Anfrage).
- **AA-Kontrast** für Code-Tokens in **beiden** Themes prüfen.

### 4.3 Sanitize
Bestehende `docPolicy` (bluemonday) erweitern: zusätzlich erlauben — Tabellen-
Elemente (`table/thead/tbody/tr/th/td` + `align`), Task-Checkboxen
(`input[type=checkbox][disabled][checked]`), Chroma-Spans (`span[class]`,
`pre[class]`, `code[class]`), Footnote-Markup (`sup`, `a[href^="#"]`,
`section[class]`, `ol/li[id]`), Callout-Container (`div[class]`,
`p[class]`, `span[class]`). Relative hrefs bleiben erlaubt. **Kein**
`style`-Attribut zulassen (Highlighting läuft über Klassen).

### 4.4 Tests (TDD, `markdown_test.go` + neue)
Tabelle, Tasklist (☑/☐), Strikethrough, Autolink, Footnote (Marker + Rücklink),
jeder Callout-Typ (Klasse + Glyph), Code-Block (Chroma-Spans vorhanden,
**keine** inline-styles, Sprache erkannt), Wikilink resolved/broken,
Frontmatter-Strip, XSS (`<script>` / `javascript:` / `onerror=` gestrippt,
`style=` gestrippt).

---

## 5. Wissen-Liste (`/wissen`, §5.7)

### 5.1 Kategorie-spezifisches Rendering
Quelle: `ListDocuments` (alle Kinds), gruppiert **client-/handler-seitig** nach
Kind in vier Darstellungen (Komponenten in `components/`):
- **Daily** → `doctimeline` (chronologische Journal-Timeline, Monats-gruppiert,
  Glyph ● accent).
- **Projekt-Notizen** → `docprojectgroup` (gruppiert unter Projekt-Header mit
  Projektfarbe+Glyph; Glyph ◆ success).
- **Frei** → `doccardgrid` (Karten-Grid, Glyph ○ highlight).
- **Agent/System** (memory/instruction/skill/plan) → `docsystemlist`
  (abgesetzter, dichterer Bereich, eigene Badges, klar getrennt von
  menschlichen Notizen).

Oben: `categorystrip` (Tab-Strip mit Scroll-Spy, reines client-JS, springt zu
Sektionen), `SearchInput` (Volltext + semantisch via bestehendem
`SearchDocuments` `?q=`), Tag-Chip-Filter (bestehend, `?tag=`-Mehrfach),
„Neu"-Button → `/wissen/neu`.

### 5.2 Suche
Bei `q != ""` rendert die Seite **Suchergebnisse** (flache Liste mit
`<mark>`-Snippet, bestehende `renderSnippet`-Logik portieren) statt der
kategorie-Sektionen — wie heute. Hybrid (FTS+Trigram+semantisch) liefert der
bestehende Usecase.

### 5.3 Pagination
`ListDocuments` + Store + ggf. `SearchDocuments` um `limit`/`offset` + Gesamtzahl
erweitern (§7). Liste rendert die `Pagination`-Komponente (bestehend) mit
„Zurück/Weiter" bzw. „Mehr laden" (htmx). Default-Seitengröße: 50.
Tag-/Such-Filter bleiben in der Query erhalten.

### 5.4 Live (SSE)
`document.created/updated/deleted` → List-Fragment-Refresh
(`hx-get="/wissen/list…"` mit erhaltenen Filtern, `sse-swap`). Muster wie
heutige `handleWebDocsList`.

### 5.5 Zustände
Leer („Kompendium leer — Neu anlegen"), Fehler (deutsche Meldung), Lade
(htmx-Indikator). Suche ohne Treffer: „Keine Treffer für »q«".

---

## 6. Dokument-Lese-Seite (`/wissen/{id}`, §5.8)

Auf `AppShell` (active = „wissen"). Aufbau:
- **Header:** Kind-Badge (Farbe+Glyph), Titel, Projekt-Link (falls
  `ProjectID`), Datum, Tag-Chips (`singleTagHref`), „Bearbeiten" →
  `/wissen/{id}/bearbeiten`, Overflow-Menü (Löschen → `ConfirmDialog`,
  Reembed bei `EmbedFailed`).
- **Lese-Spalte (~70ch):** `markdownprose` mit dem feature-completen
  `RenderDocument`-HTML.
- **Meta-/ToC-Rail:** `toc` (aus den Headings, client-Scroll-Spy) +
  `backlinks` („Referenziert von", bestehendes `BacklinksDocument`).
- **Embed-Status:** sichtbar (ok/pending/failed) + Reembed-Aktion (bestehend
  `EmbedView`/`EmbedBadge` portieren).
- **Live (SSE):** `document.updated/deleted` für **dieses** Doc →
  Page-/Fragment-Reload.

Wikilink-`resolve` baut jetzt `/wissen/{id}`-hrefs (statt `/docs/{id}`).

---

## 7. Editor (`/wissen/neu`, `/wissen/{id}/bearbeiten`, §5.9)

Auf `AppShell`. Felder: Titel, Body (Textarea, Monospace), Projekt-Zuordnung,
Kind/Type. **Tags via Frontmatter** (single source — keine separaten Tag-Felder;
das entspricht dem bestehenden Tag-Modell M2c). Bestehende Create/Update-Usecases
(`CreateDocument`/`UpdateDocument`) bleiben.

**Live-Vorschau (htmx):**
- Neuer Web-Endpoint `POST /wissen/preview` (auth, owner-scoped nur fürs
  Rendern — keine Persistenz): nimmt `body`, rendert via `RenderDocument`
  (Wikilink-resolve gegen die Doc-Liste des Users), gibt das `markdownprose`-
  Fragment zurück.
- Textarea: `hx-post="/wissen/preview"`, `hx-trigger="keyup changed delay:400ms"`
  (debounced), `hx-target="#preview"`. Vorschau neben/unter dem Editor.
- Validierung **inline & in-design** (keine native Form-Bubble).

Save → bestehender Create/Update → Redirect auf `/wissen/{id}`.
**Keine destruktive Aktion ohne `ConfirmDialog`.**

---

## 8. Backend-Änderungen (minimal, additiv)

1. **Documents-Pagination:** `ListDocuments`-Usecase + zugehöriger Store-Port +
   pgstore-Implementierung um `limit int, offset int` + Rückgabe der Gesamtzahl
   erweitern (oder `ListDocumentsPage`-Variante, um Bestandssignaturen nicht zu
   brechen — Implementierungs-Detail im Plan). `SearchDocuments` analog, falls
   die Ergebnislisten lang werden. Migration: **keine** (reines
   Query-`LIMIT/OFFSET` + `COUNT`).
2. **Editor-Preview-Endpoint:** `POST /wissen/preview` (Web-Handler, nicht
   REST-API) — siehe §7.
3. Sonst nur bestehende Usecases (`GetDocument`, `BacklinksDocument`,
   `ListTags`, `SearchDocuments`, Create/Update/Delete, Embed-Status/Retry).

> **Keine** Änderung an `/api/v1/documents` (REST), an der TUI oder am MCP.

---

## 9. Live-Updates (SSE)

Bestehend: `GET /api/v1/events` (per-User, Bearer **oder** Cookie),
`<body hx-ext="sse" sse-connect="/api/v1/events">`. Event → Fläche:

| Event | aktualisiert |
|---|---|
| `document.created/updated/deleted` | Wissen-Liste, Lese-Seite (eigenes Doc) |
| `project.created/updated` | Projekt-Gruppen-Header / Projekt-Links der Liste |

Muster: Fragment-Endpoint + `sse-swap`, wie `handleWebDocsList` heute.

---

## 10. i18n

Alle neuen Anzeige-Strings über die bestehende `T(ctx, "key", …)`-Schicht
(DE-Katalog primär, EN-Stub). **Keine** hartkodierten deutschen Literale in den
neuen `.templ`-Dateien. Neue Keys im `de`-Katalog vollständig; EN als Stub
nachziehen (Bestandsmuster).

---

## 11. Accessibility & Responsive

- Semantisches HTML (Landmarks, `nav` für Kategorie-Strip, `aside` für ToC/
  Backlinks), sichtbarer Fokus, `aria-label` an Glyphen wo sinntragend,
  `aria-hidden` wo dekorativ.
- **AA-Kontrast in beiden Themes** (inkl. Callout-Flächen + Chroma-Code-Token).
- Responsive 360→1440: Desktop = Lese-Spalte + ToC-Rail nebeneinander; Mobile =
  ToC/Backlinks unter den Text gestapelt, Kategorie-Strip scrollbar.
- `prefers-reduced-motion` respektiert (Scroll-Spy ohne ruckartige Sprünge).

---

## 12. Testing & Done-Gate

- **Unit:** Markdown-Renderer-Feature-Tests (§4.4); Handler-Tests (`httptest`)
  für Liste/Lese/Editor/Preview (inkl. Pagination-Grenzen, Suche, leere/Fehler-
  Zustände); Komponenten-Render-Tests (Timeline/ProjectGroup/Grid/SystemList/
  ToC/Backlinks); Pagination-Store-/Usecase-Test (limit/offset + Count);
  i18n-Key-Vollständigkeit.
- **Guards:** `verify-no-popups` (kein `window.alert/confirm/prompt`),
  `verify-css` (committetes `app.css` = frischer Build), `verify-generate`
  (templ generiert). **Ausführende führen `make ci` (Lint!), nicht nur
  `go test`.**
- **Coverage-Gate 75 %** (`make cover`).
- **Live-Done-Gate** (Dev-Stack Postgres + Dex, `make dev-up`/`dev-run`/
  `dev-token`): `/wissen`, `/wissen/{id}`, `/wissen/neu`,
  `/wissen/{id}/bearbeiten`, `POST /wissen/preview` liefern erwartete Antworten;
  SSE spiegelt Doc-Create/Update/Delete; Markdown-Parität sichtbar (Tabelle,
  Tasklist, Callout, Footnote, Code-Highlight, Wikilink, Backlink); Dark/Light +
  Mobile geprüft; alte `/docs`-Routen entfernt (404 / weg).

---

## 13. Implementierungs-Slicing (für writing-plans)

Eine **lineare, self-contained Task-Serie** (jede Task: Kontext + Dateien +
TDD-Schritte + Verify-Commands inline; ausführbar von Claude/Gemini/Codex).
Plus eine **`AGENTS.md`** im Repo-Root mit Repo-Konventionen (Hexagonal,
„keine Monolithen", `templ generate`, Make-Targets, Done-Gate, Commit-Stil).
Letzte Task = **Main-Wiring + Done-Gate** (Routen verdrahtet, alte `/docs`
entfernt, curl-Smoke je Route, `make ci` grün).

1. **Markdown-Renderer-Ausbau** — GFM + Footnote + Callouts + Chroma
   (class-based) + erweiterte Sanitize; `markdownprose`-Komponente. (Fundament,
   zuerst.)
2. **Chroma-Stylesheets + Tokens** — `chroma-light/dark.css` generieren +
   `go:embed` + `base.templ`-Head + Tailwind-Prose-/Callout-Token-Layer;
   AA-Check.
3. **Documents-Pagination-Backend** — Usecase/Port/Store um limit/offset +
   Count; Tests.
4. **Wissen-Liste** (`/wissen`) — kategorie-spezifisch + Strip + Suche +
   Tag-Filter + Pagination + SSE; auf `AppShell`.
5. **Dokument-Lese-Seite** (`/wissen/{id}`) — Header + Prose + ToC + Backlinks +
   Embed + SSE.
6. **Editor** (`/wissen/neu` · `/wissen/{id}/bearbeiten`) + **Live-Vorschau**
   (`POST /wissen/preview`).
7. **Main-Wiring + Done-Gate** — Routen verdrahten, alte `/docs`-Routen +
   `docs.templ` + `webui_docs.go` entfernen, curl-Smoke, `make ci`, Live-Gate.

---

## 14. Offene Punkte / Risiken

- **Chroma-Style-Wahl** (welcher Light-/Dark-Style die AA-/Marken-Optik am
  besten trifft) → Task 2, gegen Mockup `studio-document-view`.
- **Scroll-Spy-JS:** minimal vanilla (IntersectionObserver), lokal gebündelt —
  kein CDN, CSP-konform.
- **Pagination-Signatur:** Bestandssignatur erweitern vs. neue `…Page`-Methode
  — Plan-Detail Task 3 (Bestands-Tests nicht brechen).
- **Alt/Neu-Koexistenz:** `/docs` läuft bis Task 7; Wikilink-hrefs erst in der
  Lese-Seite/Renderer auf `/wissen` umstellen — konsistent halten.
- **Routing-Notation:** Spec §4 des Overhauls schreibt „{slug}"; hier bewusst
  `{id}` (siehe §2). Falls später echte Slugs gewünscht: eigener Mini-Slice.

---

## 15. Referenzen
- Overhaul-Spec: `docs/superpowers/specs/2026-06-23-flow-webui-overhaul-design.md`
  (§5.7–5.9, §6, §7, §11).
- Mockups: `docs/superpowers/specs/assets/2026-06-23-webui/studio-docs-categories.html`,
  `…/studio-document-view.html`.
- Bestandscode: `internal/adapter/webui/{markdown.go,wikilink.go,docs.templ}`,
  `internal/adapter/httpserver/webui_docs.go`,
  `internal/adapter/webui/components/appshell.templ`.
- TUI-Feature-Referenz (Parität, **nicht** Code-Reuse):
  `internal/tui/markdown/` (callout/footnote/table/code/frontmatter).
